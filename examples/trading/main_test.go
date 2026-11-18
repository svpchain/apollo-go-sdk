package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	apollo "github.com/svpchain/apollo-go-sdk"
)

type fixture struct {
	mu        sync.Mutex
	calls     int
	open      bool
	submitted []apollo.Action
	results   map[uint64]apollo.Action
	genesis   apollo.Hash
	key       ed25519.PrivateKey
}

func (f *fixture) snapshot() (int, []apollo.Action) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, append([]apollo.Action(nil), f.submitted...)
}
func (f *fixture) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var q struct {
		ID     uint64          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f.calls++
	var result any
	switch q.Method {
	case "ChainService.GetChainInfo":
		result = apollo.ChainInfo{GenesisHash: f.genesis, ExecutedHeight: 100}
	case "MarketService.ListMarkets":
		result = apollo.Meta{Markets: []apollo.Market{{ID: 0, Ticker: "BTC-USD", Status: apollo.MarketActive, AtomicResolution: -10, QuantumConversionExponent: -9, PriceExponent: -5, StepBaseQuantums: 1000000, SubticksPerTick: 100000}}}
	case "AccountService.GetOpenOrders":
		var p struct {
			Account apollo.Uint64 `json:"account"`
			Market  *uint32       `json:"market"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil || p.Account != 7 || p.Market == nil || *p.Market != 0 {
			http.Error(w, "invalid filters", 400)
			return
		}
		orders := []apollo.OpenOrder{{OrderID: 17, Market: 0, Side: "SIDE_BID", Price: 6000000000, Remaining: 10000000}}
		if f.open {
			orders = append(orders, apollo.OpenOrder{OrderID: 9007199254740993, Market: 0, Side: "SIDE_BID", Price: 6000000000, Remaining: 10000000})
		}
		result = apollo.OpenOrdersResult{Orders: orders}
	case "AccountService.GetNonce":
		result = apollo.NonceState{LastNonce: 10}
	case "AccountService.GetAgents":
		result = apollo.AgentsResult{Agents: []apollo.Agent{{Slot: 1, Seq: 6, Key: apollo.HexBytes(f.key.Public().(ed25519.PublicKey))}}}
	case "TxService.GetAllowance":
		result = apollo.UserRateLimit{Used: 0, Limit: 10000, Remaining: 10000}
	case "TxService.Submit":
		var p struct {
			ID      apollo.Uint64 `json:"id"`
			Actions []struct {
				Action apollo.HexBytes `json:"action"`
				Auth   apollo.HexBytes `json:"auth"`
			} `json:"actions"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		response := apollo.SubmitResult{ID: p.ID}
		for _, item := range p.Actions {
			if len(item.Action) != 48 || len(item.Auth) != 78 {
				http.Error(w, "wrong widths", 400)
				return
			}
			var raw [48]byte
			var signed [78]byte
			copy(raw[:], item.Action)
			copy(signed[:], item.Auth)
			action := apollo.Action{
				Type:    map[byte]apollo.ActionType{3: apollo.Place, 4: apollo.Cancel, 5: apollo.Modify}[raw[0]],
				Account: binary.LittleEndian.Uint32(raw[4:8]), Market: binary.LittleEndian.Uint16(raw[8:10]),
				OrderID: binary.LittleEndian.Uint64(raw[16:24]), Price: int64(binary.LittleEndian.Uint64(raw[32:40])), Size: int64(binary.LittleEndian.Uint64(raw[40:48])),
			}
			auth, err := apollo.DecodeAuth(signed)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			message, err := apollo.AgentMessage(f.genesis, raw, auth.Nonce, auth.Key, auth.Seq)
			if err != nil || auth.Key != 1 || auth.Seq != 6 || !ed25519.Verify(f.key.Public().(ed25519.PublicKey), message[:], auth.Signature[:64]) {
				http.Error(w, "invalid signature", 400)
				return
			}
			f.submitted = append(f.submitted, action)
			f.results[auth.Nonce] = action
			if action.Type == apollo.Cancel && action.OrderID == 9007199254740993 {
				f.open = false
			}
			hash := apollo.TransactionHash(raw, signed)
			response.Statuses = append(response.Statuses, apollo.SubmitAccepted)
			response.TransactionHashes = append(response.TransactionHashes, hash[:])
		}
		result = response
	case "TxService.GetActionStatus":
		var p struct {
			Keys []apollo.ActionKey `json:"keys"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		response := apollo.ActionStatuses{}
		for _, key := range p.Keys {
			response.Statuses = append(response.Statuses, apollo.ActionStatus{Key: key, State: apollo.ActionExecuted, Height: 101, OK: true})
		}
		result = response
	default:
		http.Error(w, "unexpected method "+q.Method, 400)
		return
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": result})
}

func TestTradingCLI(t *testing.T) {
	seed := bytes.Repeat([]byte{7}, 32)
	f := &fixture{open: true, genesis: apollo.Hash{3}, key: ed25519.NewKeyFromSeed(seed), results: make(map[uint64]apollo.Action)}
	server := httptest.NewServer(http.HandlerFunc(f.handle))
	defer server.Close()
	env := map[string]string{"APOLLO_RPC_URL": server.URL, "APOLLO_ACCOUNT": "7", "APOLLO_AGENT_SEED": hex.EncodeToString(seed), "APOLLO_GENESIS_HASH": f.genesis.String(), "APOLLO_COIN": "BTC-USD", "APOLLO_PRICE": "61000", "APOLLO_SIZE": "0.001"}
	invoke := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := run(args, func(k string) string { return env[k] }, &out, io.Discard)
		return out.String(), err
	}
	out, err := invoke("-order-id", "9007199254740993")
	if err != nil || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "0.001") {
		t.Fatal("query", out, err)
	}
	for _, action := range []string{"order", "cancel", "modify"} {
		out, err := invoke("-action", action, "-order-id", "9007199254740993")
		_, submits := f.snapshot()
		if err != nil || !strings.Contains(out, "Not broadcast") || len(submits) != 0 {
			t.Fatal("default mode submitted", action, out, err)
		}
	}
	calls, _ := f.snapshot()
	for _, id := range []string{"0", "1n", "1e3", "18446744073709551616"} {
		if _, err := invoke("-action", "cancel", "-order-id", id, "-submit"); err == nil {
			t.Fatal("invalid order ID accepted", id)
		}
	}
	if after, _ := f.snapshot(); after != calls {
		t.Fatal("invalid IDs reached node")
	}
	for _, args := range [][]string{{"-action", "bad"}, {"-action", "orders", "-submit"}, {"-account", "0"}, {"unexpected"}} {
		if _, err := invoke(args...); err == nil {
			t.Fatal("invalid arguments accepted", args)
		}
	}
	_, err = invoke("-action", "modify", "-order-id", "17", "-size", "0.00001", "-submit")
	if err == nil {
		t.Fatal("off-step modify accepted")
	}
	_, submits := f.snapshot()
	if len(submits) != 0 {
		t.Fatal("off-step modify broadcast")
	}
	out, err = invoke("-action", "modify", "-order-id", "17", "-price", "62000", "-size", "0.002", "-submit")
	if err != nil {
		t.Fatal("modify", out, err)
	}
	_, submits = f.snapshot()
	if len(submits) != 1 || submits[0].Type != apollo.Modify || submits[0].OrderID != 17 || submits[0].Price != 6200000000 || submits[0].Size != 20000000 {
		t.Fatal("wrong modify", submits)
	}
	out, err = invoke("-action", "cancel", "-order-id", "9007199254740993", "-submit")
	if err != nil || !strings.Contains(out, "Still open after cancellation: false") {
		t.Fatal("cancel", out, err)
	}
	_, submits = f.snapshot()
	if len(submits) != 2 || submits[1].Type != apollo.Cancel || submits[1].OrderID != 9007199254740993 {
		t.Fatal("wrong cancel", submits)
	}
	out, err = invoke("-order-id", "9007199254740993")
	if err != nil || !strings.Contains(out, "No matching open orders") {
		t.Fatal("cancelled order still shown", out, err)
	}
	env["APOLLO_AGENT_SEED"] = ""
	out, err = invoke("-order-id", "17")
	if err != nil || !strings.Contains(out, "17") {
		t.Fatal("read-only query required seed", out, err)
	}
	if err := run([]string{"-h"}, func(string) string { return "" }, io.Discard, io.Discard); err != nil {
		t.Fatal("help required config", err)
	}
	if strings.Contains(out, hex.EncodeToString(seed)) {
		t.Fatal("seed printed")
	}
}

func TestUnsignedOrderIDs(t *testing.T) {
	for _, tc := range []struct {
		text string
		want uint64
	}{
		{"1", 1}, {"9007199254740993", 9007199254740993}, {"18446744073709551615", 18446744073709551615},
	} {
		value, err := unsigned(tc.text, 64, "order-id")
		if err != nil || value != tc.want {
			t.Fatalf("wrong parsed ID %s: %d, %v", tc.text, value, err)
		}
	}
}
