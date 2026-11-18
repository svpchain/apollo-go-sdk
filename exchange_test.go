package apollo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type mockState struct {
	genesis                             Hash
	owner                               Address
	agent                               SignerIdentity
	seq                                 uint32
	height                              uint64
	lastNonce                           Uint64
	approved                            bool
	expiry                              Uint64
	limit, used                         Uint64
	submitStatus                        SubmitStatus
	submitStatuses                      []SubmitStatus
	marketStatus                        MarketStatus
	executionState                      ActionState
	reject                              string
	failSubmit, failTracking, wrongHash bool
	calls                               map[string]int
	submitted                           []PreparedAction
}
type mockNode struct {
	mu     sync.Mutex
	state  mockState
	server *httptest.Server
}

func newMock(t *testing.T) (*mockNode, Signer, Signer) {
	t.Helper()
	owner, err := NewOwnerSigner(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := NewAgentSigner(bytes.Repeat([]byte{7}, 32), 1)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockNode{state: mockState{genesis: Hash{3}, owner: owner.Identity().Owner, agent: agent.Identity(), seq: 6, height: 100, approved: true, limit: 1000000, submitStatus: SubmitAccepted, executionState: ActionExecuted, calls: map[string]int{}}}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m, owner, agent
}
func (m *mockNode) change(fn func(*mockState)) { m.mu.Lock(); defer m.mu.Unlock(); fn(&m.state) }
func (m *mockNode) count(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.calls[method]
}
func (m *mockNode) exchange(t *testing.T, signer Signer, opts ...Option) *Exchange {
	t.Helper()
	n := NewNonceManager()
	n.now = func() time.Time { return time.Unix(0, 1_000_000) }
	all := []Option{WithAccount(7), WithNonceManager(n)}
	all = append(all, opts...)
	e, err := NewExchange(m.server.URL, signer, all...)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func (m *mockNode) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := &m.state
	var request struct {
		ID     uint64          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.calls[request.Method]++
	var result any
	switch request.Method {
	case "ChainService.GetChainInfo":
		result = ChainInfo{ChainID: "offline-fixture", GenesisHash: s.genesis, ExecutedHeight: Uint64(s.height)}
	case "MarketService.ListMarkets":
		status := s.marketStatus
		if status == "" {
			status = MarketActive
		}
		result = Meta{Markets: []Market{{ID: 0, Ticker: "BTC-USD", Status: status, AtomicResolution: -10, QuantumConversionExponent: -9, PriceExponent: -5, StepBaseQuantums: 1000000, SubticksPerTick: 100000, Mark: 6500000000}}}
	case "MarketService.GetOrderBook":
		result = L2Book{Market: 0, Asks: []Level{{Price: 6500100000, Size: 100000000}}, Bids: []Level{{Price: 6499900000, Size: 100000000}}}
	case "AccountService.GetNonce":
		result = NonceState{LastNonce: s.lastNonce}
	case "AccountService.GetAccount":
		result = UserState{Account: 7, Owner: s.owner}
	case "AccountService.GetAgents":
		agents := []Agent{}
		if s.approved {
			agents = append(agents, Agent{Slot: 1, Key: append(HexBytes(nil), s.agent.PublicKey[:]...), Seq: s.seq, Expiry: s.expiry})
		}
		result = AgentsResult{At: At{Uint64(s.height)}, Agents: agents}
	case "TxService.GetAllowance":
		remaining := Uint64(0)
		if s.limit > s.used {
			remaining = s.limit - s.used
		}
		result = UserRateLimit{Limit: s.limit, Used: s.used, Remaining: remaining}
	case "TxService.Submit":
		if s.failSubmit {
			http.Error(w, "transport fixture", 503)
			return
		}
		var p struct {
			ID      Uint64 `json:"id"`
			Actions []struct {
				Action HexBytes `json:"action"`
				Auth   HexBytes `json:"auth"`
			} `json:"actions"`
		}
		if err := json.Unmarshal(request.Params, &p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		out := SubmitResult{ID: p.ID, Detail: "fixture"}
		for j, signed := range p.Actions {
			if len(signed.Action) != 48 || len(signed.Auth) != 78 {
				http.Error(w, "bad byte widths", 400)
				return
			}
			var raw [48]byte
			var authRaw [78]byte
			copy(raw[:], signed.Action)
			copy(authRaw[:], signed.Auth)
			a, err := DecodeAuth(authRaw)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			identity := SignerIdentity{Owner: s.owner}
			if a.Key > 0 {
				identity = s.agent
			}
			if err := verifySignature(identity, SignRequest{s.genesis, raw, a.Nonce, s.seq}, a); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			prepared := PreparedAction{Account: actionAccount(raw), Nonce: a.Nonce, Action: raw, Auth: authRaw, GenesisHash: s.genesis, TransactionHash: TransactionHash(raw, authRaw)}
			s.submitted = append(s.submitted, prepared)
			status := s.submitStatus
			if j < len(s.submitStatuses) {
				status = s.submitStatuses[j]
			}
			out.Statuses = append(out.Statuses, status)
			hash := HexBytes{}
			if status == SubmitAccepted {
				hash = append(hash, prepared.TransactionHash[:]...)
				if s.wrongHash {
					hash[0] ^= 255
				}
				if raw[0] == 10 {
					s.approved = true
					s.seq++
					copy(s.agent.PublicKey[:], raw[16:48])
					s.agent.Slot = raw[3]
				}
				if raw[0] == 11 {
					s.approved = false
				}
			}
			out.TransactionHashes = append(out.TransactionHashes, hash)
		}
		result = out
	case "TxService.GetActionStatus":
		if s.failTracking {
			http.Error(w, "query fixture", 503)
			return
		}
		var p struct {
			Keys []ActionKey `json:"keys"`
		}
		if err := json.Unmarshal(request.Params, &p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		out := ActionStatuses{At: At{Uint64(s.height)}}
		for _, key := range p.Keys {
			nonce := key.Nonce
			out.Statuses = append(out.Statuses, ActionStatus{Key: key, State: s.executionState, Height: Uint64(s.height), OK: s.reject == "", Reject: s.reject, OrderID: &nonce})
		}
		result = out
	default:
		http.Error(w, "unexpected method "+request.Method, 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
}
func limitOrder() CreateOrderRequest {
	return CreateOrderRequest{Coin: "BTC-USD", IsBuy: true, Price: "65000", Size: "0.001", OrderType: OrderType{Limit: &LimitOrderType{TIF: Gtc}}}
}
func cancelAction() Action { return Action{Type: CancelAll, Account: 7, Market: 65535} }
func TestAgentHTTPOrder(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	r, err := e.Order(context.Background(), limitOrder())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Successful() {
		t.Fatalf("not executed: %+v", r)
	}
	status := r.Statuses[0]
	auth, err := DecodeAuth(status.Prepared.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if auth.Seq != 6 || auth.Key != 1 || status.Prepared.Nonce != 1000 {
		t.Fatalf("wrong auth %+v", auth)
	}
	expected, _ := EncodeAction(Action{Type: Place, Account: 7, Market: 0, Side: Bid, Price: 6500000000, Size: 10000000, TIF: Gtc, Position: Long, IsCross: true})
	if status.Prepared.Action != expected {
		t.Fatal("wrong order units")
	}
	if m.count("TxService.Submit") != 1 {
		t.Fatal("submit count")
	}
}
func TestPrepareDoesNotBroadcastAndRejectsNetworkChange(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	prepared, err := e.PrepareOrder(context.Background(), limitOrder())
	if err != nil {
		t.Fatal(err)
	}
	if m.count("TxService.Submit") != 0 {
		t.Fatal("prepare sent an order")
	}
	m.change(func(s *mockState) { s.genesis[0]++ })
	_, err = e.SubmitPrepared(context.Background(), []PreparedAction{prepared})
	var mismatch *ChainIdentityError
	if !errors.As(err, &mismatch) {
		t.Fatal("expected genesis error", err)
	}
	if m.count("TxService.Submit") != 0 {
		t.Fatal("wrong-chain broadcast")
	}
}
func TestOwnerApprovalAndAgentSequence(t *testing.T) {
	m, owner, agent := newMock(t)
	m.change(func(s *mockState) { s.approved = false })
	o := m.exchange(t, owner)
	id := agent.Identity()
	r, err := o.ApproveAgent(context.Background(), 1, id.PublicKey[:], 0)
	if err != nil || !r.Successful() {
		t.Fatal("approval failed", err)
	}
	e := m.exchange(t, agent)
	p, err := e.Prepare(context.Background(), cancelAction())
	if err != nil {
		t.Fatal(err)
	}
	auth, _ := DecodeAuth(p.Auth)
	if auth.Seq != 7 {
		t.Fatal("agent used stale sequence")
	}
	_, err = o.RevokeAgent(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Prepare(context.Background(), cancelAction()); err == nil {
		t.Fatal("revoked agent signed")
	}
}
func TestInvalidAgent(t *testing.T) {
	for _, reason := range []string{"missing", "expired", "wrong-key"} {
		t.Run(reason, func(t *testing.T) {
			m, _, agent := newMock(t)
			m.change(func(s *mockState) {
				switch reason {
				case "missing":
					s.approved = false
				case "expired":
					s.expiry = 99
				case "wrong-key":
					s.agent.PublicKey = [32]byte{9}
				}
			})
			_, err := m.exchange(t, agent).Prepare(context.Background(), cancelAction())
			if err == nil {
				t.Fatal("invalid agent accepted")
			}
		})
	}
}
func TestInvalidOrderIntents(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*CreateOrderRequest)
	}{
		{"off-step", func(r *CreateOrderRequest) { r.Size = "0.00101" }}, {"below-min", func(r *CreateOrderRequest) { r.Size = "0.00000001" }}, {"off-tick", func(r *CreateOrderRequest) { r.Price = "65000.1" }}, {"zero", func(r *CreateOrderRequest) { r.Size = "0" }}, {"unknown-tif", func(r *CreateOrderRequest) { r.OrderType.Limit.TIF = "bad" }}, {"unknown-position", func(r *CreateOrderRequest) { r.Position = "Both" }}, {"unknown-coin", func(r *CreateOrderRequest) { r.Coin = "OTHER" }}, {"overflow", func(r *CreateOrderRequest) { r.Size = "9999999999999999999999999" }}, {"ambiguous", func(r *CreateOrderRequest) {
			r.OrderType.Trigger = &TriggerOrderType{TPSL: "sl", TriggerPrice: "60000"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, agent := newMock(t)
			r := limitOrder()
			tc.edit(&r)
			_, err := m.exchange(t, agent).Order(context.Background(), r)
			if err == nil {
				t.Fatal("invalid order accepted")
			}
			if m.count("TxService.Submit") != 0 {
				t.Fatal("invalid intent broadcast")
			}
		})
	}
}
func TestAllowanceAndCancelExemption(t *testing.T) {
	m, _, agent := newMock(t)
	m.change(func(s *mockState) { s.used = s.limit })
	e := m.exchange(t, agent)
	_, err := e.Order(context.Background(), limitOrder())
	if !errors.Is(err, ErrActionAllowance) {
		t.Fatal(err)
	}
	r, err := e.CancelAll(context.Background(), "")
	if err != nil || !r.Successful() {
		t.Fatal("cancel blocked", err)
	}
	if m.count("TxService.GetAllowance") != 1 {
		t.Fatal("cancel consumed preflight allowance")
	}
}
func TestMarketAndTriggerProtection(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	_, err := e.MarketOpen(context.Background(), MarketOrderRequest{Coin: "BTC-USD", IsBuy: true, Size: "0.001"})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	p := m.state.submitted[0]
	m.mu.Unlock()
	expected, _ := EncodeAction(Action{Type: Place, Account: 7, Market: 0, Side: Bid, Price: 6565200000, Size: 10000000, TIF: Ioc, Position: Long, IsCross: true})
	if p.Action != expected {
		t.Fatalf("protection price: %x", p.Action)
	}
	order := limitOrder()
	order.Price = ""
	order.OrderType = OrderType{Trigger: &TriggerOrderType{TPSL: "sl", IsMarket: true, TriggerPrice: "60000"}}
	r, err := e.Order(context.Background(), order)
	if err != nil || !r.Successful() {
		t.Fatal(err)
	}
	if r.Statuses[0].Prepared.Action[0] != 19 {
		t.Fatal("missing PlaceWith")
	}
}
func TestSubmissionOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		config    func(*mockState)
		outcome   Outcome
		errorText string
	}{
		{"accepted", func(*mockState) {}, OutcomeExecuted, ""}, {"rejected-execution", func(s *mockState) { s.reject = "InsufficientMargin" }, OutcomeExecuted, "InsufficientMargin"}, {"nonce-used", func(s *mockState) { s.executionState = ActionNonceUsed }, OutcomeNonceUsed, ""}, {"pending", func(s *mockState) { s.executionState = ActionNotFound }, OutcomePending, ""}, {"bad-signature", func(s *mockState) { s.submitStatus = SubmitBadSignature }, OutcomeNotSubmitted, "SUBMIT_STATUS_BAD_SIGNATURE: fixture"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, agent := newMock(t)
			m.change(tc.config)
			r, err := m.exchange(t, agent).Execute(context.Background(), cancelAction(), WaitTimeout(20*time.Millisecond), PollInterval(time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			if r.Statuses[0].Outcome != tc.outcome || r.Statuses[0].Error != tc.errorText {
				t.Fatalf("wrong outcome %+v", r.Statuses[0])
			}
			if tc.name != "accepted" && r.Successful() {
				t.Fatal("nonexecuted/rejected treated as success")
			}
		})
	}
}
func TestUnknownSubmissionPreservesPreparedAndNeverRetries(t *testing.T) {
	for _, reason := range []string{"network", "hash", "unknown-status"} {
		t.Run(reason, func(t *testing.T) {
			m, _, agent := newMock(t)
			e := m.exchange(t, agent)
			p, err := e.Prepare(context.Background(), cancelAction())
			if err != nil {
				t.Fatal(err)
			}
			m.change(func(s *mockState) {
				s.failSubmit = reason == "network"
				s.wrongHash = reason == "hash"
				if reason == "unknown-status" {
					s.submitStatus = "SUBMIT_STATUS_UNSPECIFIED"
				}
			})
			_, err = e.SubmitPrepared(context.Background(), []PreparedAction{p})
			var unknown *SubmissionUnknownError
			if !errors.As(err, &unknown) || unknown.Prepared[0] != p {
				t.Fatal("lost original prepared action", err)
			}
			if m.count("TxService.Submit") != 1 {
				t.Fatal("unexpected retry")
			}
		})
	}
}
func TestTrackingFailureRetainsAcceptance(t *testing.T) {
	m, _, agent := newMock(t)
	m.change(func(s *mockState) { s.failTracking = true })
	r, err := m.exchange(t, agent).Execute(context.Background(), cancelAction())
	var tracking *TrackingError
	if !errors.As(err, &tracking) || r == nil || tracking.Response != r || r.Statuses[0].SubmitStatus != SubmitAccepted {
		t.Fatal("lost acceptance", err)
	}
}
func TestOwnerAndWalletMismatch(t *testing.T) {
	m, owner, _ := newMock(t)
	bad, err := NewOwnerSigner(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.exchange(t, bad).Prepare(context.Background(), cancelAction())
	if err == nil {
		t.Fatal("wrong owner accepted")
	}
	wallet, err := NewWalletSigner(owner.Identity().Owner, func(ctx context.Context, data TypedData) ([]byte, error) {
		var action [48]byte
		copy(action[:], data.Message.Action)
		a, e := owner.Sign(ctx, SignRequest{data.Domain.Salt, action, uint64(data.Message.Nonce), 0})
		return a.Signature[:], e
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.exchange(t, wallet).Execute(context.Background(), cancelAction())
	if err != nil || !r.Successful() {
		t.Fatal("wallet integration failed", err)
	}
	wrongWallet, _ := NewWalletSigner(owner.Identity().Owner, func(ctx context.Context, data TypedData) ([]byte, error) {
		var action [48]byte
		copy(action[:], data.Message.Action)
		a, e := bad.Sign(ctx, SignRequest{data.Domain.Salt, action, uint64(data.Message.Nonce), 0})
		return a.Signature[:], e
	})
	if _, err = m.exchange(t, wrongWallet).Prepare(context.Background(), cancelAction()); err == nil {
		t.Fatal("wallet signed as wrong key")
	}
}
func TestSharedConcurrentNonceAllocator(t *testing.T) {
	m, owner, agent := newMock(t)
	shared := NewNonceManager()
	a := m.exchange(t, agent, WithNonceManager(shared))
	o := m.exchange(t, owner, WithNonceManager(shared))
	var wg sync.WaitGroup
	nonces := make(chan uint64, 20)
	errs := make(chan error, 20)
	for j := 0; j < 20; j++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			e := a
			if index%2 == 1 {
				e = o
			}
			p, err := e.Prepare(context.Background(), cancelAction())
			if err != nil {
				errs <- err
				return
			}
			nonces <- p.Nonce
		}(j)
	}
	wg.Wait()
	close(nonces)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[uint64]bool{}
	for n := range nonces {
		if seen[n] {
			t.Fatal("duplicate nonce")
		}
		seen[n] = true
	}
	if len(seen) != 20 {
		t.Fatal(len(seen))
	}
}
func TestBatchAndPreparedValidation(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	r, err := e.BulkOrders(context.Background(), []CreateOrderRequest{limitOrder(), limitOrder()}, NoWait())
	if err != nil || len(r.Statuses) != 2 || r.Statuses[0].Outcome != OutcomePending || r.Statuses[0].Prepared.Nonce == r.Statuses[1].Prepared.Nonce {
		t.Fatal("bulk failed", err)
	}
	p := r.Statuses[0].Prepared
	p.Nonce++
	_, err = e.SubmitPrepared(context.Background(), []PreparedAction{p})
	if err == nil {
		t.Fatal("tampered metadata accepted")
	}
	if m.count("TxService.Submit") != 1 {
		t.Fatal("tampered broadcast")
	}
	m.change(func(s *mockState) { s.lastNonce = 1 << 56 })
	if _, err = e.PrepareOrder(context.Background(), limitOrder()); err == nil {
		t.Fatal("reserved order nonce accepted")
	}
	if _, err = e.Prepare(context.Background(), cancelAction()); err != nil {
		t.Fatal("non-order nonce rejected", err)
	}
}
func TestAgentCannotApprove(t *testing.T) {
	m, _, agent := newMock(t)
	_, err := m.exchange(t, agent).ApproveAgent(context.Background(), 1, bytes.Repeat([]byte{3}, 32), 0)
	if err == nil {
		t.Fatal("agent managed keys")
	}
	if m.count("TxService.Submit") != 0 {
		t.Fatal("broadcast owner-only action")
	}
}
func TestAdvancedOrderEndpoints(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	for _, fn := range []func() (*OrderResponse, error){
		func() (*OrderResponse, error) {
			return e.TwapOrder(context.Background(), CreateTwapOrderRequest{Coin: "BTC-USD", IsBuy: true, Size: "0.01", DurationS: 600, IntervalS: 60, TolerancePPM: 10000})
		},
		func() (*OrderResponse, error) {
			return e.ScaledOrder(context.Background(), CreateScaledOrderRequest{Coin: "BTC-USD", IsBuy: true, Size: "0.01", LowestPrice: "64000", HighestPrice: "65000", NumOrders: 10})
		},
		func() (*OrderResponse, error) { return e.UpdateLeverage(context.Background(), "BTC-USD", "5") },
		func() (*OrderResponse, error) {
			return e.UpdateIsolatedMargin(context.Background(), "BTC-USD", Long, "10")
		},
		func() (*OrderResponse, error) {
			return e.MarketClose(context.Background(), ClosePositionRequest{Coin: "BTC-USD", Position: Long, Size: "0.001"})
		},
	} {
		r, err := fn()
		if err != nil || !r.Successful() {
			t.Fatal("advanced order failed", err)
		}
	}
}
func TestWaitBatchesKeysAbove10000(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	keys := make([]ActionKey, 10001)
	indices := make([]int, len(keys))
	result := &OrderResponse{Statuses: make([]OrderStatus, len(keys))}
	for i := range keys {
		keys[i] = ActionKey{7, Uint64(i + 1)}
		indices[i] = i
		result.Statuses[i].Outcome = OutcomePending
	}
	r, err := e.wait(context.Background(), result, keys, indices, submitConfig{wait: true, waitTimeout: 5 * time.Second, interval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if m.count("TxService.GetActionStatus") != 2 || r.Statuses[10000].Outcome != OutcomeExecuted {
		t.Fatal("status batching failed")
	}
}
func TestPreCancelledAndInvalidOptionsDoNotSubmit(t *testing.T) {
	m, _, agent := newMock(t)
	e := m.exchange(t, agent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.Execute(ctx, cancelAction())
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, err = e.Order(context.Background(), limitOrder(), WaitTimeout(0))
	if err == nil {
		t.Fatal("invalid wait option")
	}
	if m.count("TxService.Submit") != 0 {
		t.Fatal("unexpected submit")
	}
}
func ExampleCreateOrderRequest() {
	r := CreateOrderRequest{Coin: "BTC-USD", IsBuy: true, Price: "65000", Size: "0.001", OrderType: OrderType{Limit: &LimitOrderType{TIF: Gtc}}}
	fmt.Println(r.Coin, r.Price, r.Size)
	// Output: BTC-USD 65000 0.001
}

func TestCallerCancelDuringTrackingPreservesReceipt(t *testing.T) {
	m, _, agent := newMock(t)
	m.change(func(s *mockState) { s.executionState = ActionNotFound })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &cancelTrackingTransport{base: http.DefaultTransport, cancel: cancel}
	client := &http.Client{Transport: transport}
	e, err := NewExchange(m.server.URL, agent, WithAccount(7), WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	response, err := e.Execute(ctx, cancelAction())
	var tracking *TrackingError
	if !errors.As(err, &tracking) || !errors.Is(err, context.Canceled) || response == nil || response.Statuses[0].SubmitStatus != SubmitAccepted {
		t.Fatal("lost cancellation/receipt", err)
	}
	if m.count("TxService.Submit") != 1 {
		t.Fatal("unexpected broadcast retry")
	}
}

type cancelTrackingTransport struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (t *cancelTrackingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	data, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(data))
	if bytes.Contains(data, []byte("TxService.GetActionStatus")) {
		t.cancel()
	}
	return t.base.RoundTrip(r)
}

func TestBulkModifyOneSubmitWithPerItemResults(t *testing.T) {
	m, _, agent := newMock(t)
	m.change(func(s *mockState) { s.submitStatuses = []SubmitStatus{SubmitAccepted, SubmitBadSignature} })
	response, err := m.exchange(t, agent).BulkModify(context.Background(), []ModifyOrderRequest{
		{Coin: "BTC-USD", OrderID: 9007199254740993, Price: "65100", Size: "0.001"},
		{Coin: "BTC-USD", OrderID: 9007199254740994, Price: "65200", Size: "0.002"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.count("TxService.Submit") != 1 || m.count("MarketService.ListMarkets") != 1 {
		t.Fatal("batch was not submitted once")
	}
	if len(response.Statuses) != 2 || response.Statuses[0].Outcome != OutcomeExecuted || response.Statuses[1].Outcome != OutcomeNotSubmitted || response.Successful() {
		t.Fatalf("wrong statuses: %+v", response)
	}
	for j, st := range response.Statuses {
		expected, err := EncodeAction(Action{Type: Modify, Account: 7, Market: 0, OrderID: 9007199254740993 + uint64(j), Price: 6510000000 + int64(j)*10000000, Size: 10000000 + int64(j)*10000000})
		if err != nil || st.Prepared.Action != expected {
			t.Fatalf("wrong modify %d: %v", j, err)
		}
	}
	if response.Statuses[0].Prepared.Nonce == response.Statuses[1].Prepared.Nonce {
		t.Fatal("duplicate nonce")
	}
}
func TestBulkModifyValidatesWholeBatchBeforeSigning(t *testing.T) {
	for _, reason := range []string{"price", "size", "market", "inactive", "empty", "oversize", "options"} {
		t.Run(reason, func(t *testing.T) {
			m, _, agent := newMock(t)
			requests := []ModifyOrderRequest{{Coin: "BTC-USD", OrderID: 1, Price: "65100", Size: "0.001"}, {Coin: "BTC-USD", OrderID: 2, Price: "65200", Size: "0.002"}}
			var opts []SubmitOption
			switch reason {
			case "price":
				requests[1].Price = "65200.1"
			case "size":
				requests[1].Size = "0.00001"
			case "market":
				requests[1].Coin = "UNKNOWN"
			case "inactive":
				m.change(func(s *mockState) { s.marketStatus = MarketStatus("MARKET_STATUS_CANCEL_ONLY") })
			case "empty":
				requests = nil
			case "oversize":
				requests = make([]ModifyOrderRequest, 20001)
			case "options":
				opts = []SubmitOption{WaitTimeout(0)}
			}
			_, err := m.exchange(t, agent).BulkModify(context.Background(), requests, opts...)
			if err == nil {
				t.Fatal("invalid batch accepted")
			}
			if m.count("TxService.Submit") != 0 || m.count("AccountService.GetNonce") != 0 {
				t.Fatal("invalid batch reached signing or submit")
			}
			if reason == "empty" || reason == "oversize" || reason == "options" {
				if m.count("MarketService.ListMarkets") != 0 {
					t.Fatal("queried markets for invalid batch options/count")
				}
			}
		})
	}
}
func TestBulkModifyChecksTotalAllowance(t *testing.T) {
	m, _, agent := newMock(t)
	m.change(func(s *mockState) { s.used = s.limit - 1 })
	_, err := m.exchange(t, agent).BulkModify(context.Background(), []ModifyOrderRequest{
		{Coin: "BTC-USD", OrderID: 1, Price: "65100", Size: "0.001"},
		{Coin: "BTC-USD", OrderID: 2, Price: "65200", Size: "0.001"},
	})
	if !errors.Is(err, ErrActionAllowance) {
		t.Fatalf("expected allowance error: %v", err)
	}
	if m.count("TxService.Submit") != 0 || m.count("AccountService.GetNonce") != 0 {
		t.Fatal("signed without allowance")
	}
}
func TestCancelTwapUsesExemptCancelWithParentID(t *testing.T) {
	m, _, agent := newMock(t)
	m.change(func(s *mockState) { s.used = s.limit })
	response, err := m.exchange(t, agent).CancelTwap(context.Background(), CancelOrderRequest{Coin: "BTC-USD", OrderID: 9007199254740993})
	if err != nil || !response.Successful() {
		t.Fatal("TWAP cancel failed", err)
	}
	expected, err := EncodeAction(Action{Type: Cancel, Account: 7, Market: 0, OrderID: 9007199254740993})
	if err != nil || response.Statuses[0].Prepared.Action != expected {
		t.Fatal("wrong TWAP cancellation action", err)
	}
	if m.count("TxService.GetAllowance") != 0 || m.count("TxService.Submit") != 1 {
		t.Fatal("cancellation quota or submit changed")
	}
}
