package apollo

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSummarizeFeesRolesAndMarkets(t *testing.T) {
	fills := []Fill{
		{Market: 2, MakerAccount: 7, TakerAccount: 8, MakerFee: -20, TakerFee: 100},
		{Market: 0, MakerAccount: 8, TakerAccount: 7, MakerFee: -20, TakerFee: 100},
		{Market: 2, MakerAccount: 7, TakerAccount: 7, MakerFee: -10, TakerFee: 50},
		{Market: 9, MakerAccount: 8, TakerAccount: 9, MakerFee: -20, TakerFee: 100},
	}
	before := append([]Fill(nil), fills...)
	summary, err := SummarizeFees(7, fills)
	if err != nil {
		t.Fatal(err)
	}
	want := &UserFeeSummary{Account: 7, FeeTotals: FeeTotals{MakerFees: "-30", TakerFees: "150", FeesPaid: "150", Rebates: "30", NetFees: "120", FillCount: 3, MakerFillCount: 2, TakerFillCount: 2}, ByMarket: []MarketFeeSummary{
		{Market: 0, FeeTotals: FeeTotals{MakerFees: "0", TakerFees: "100", FeesPaid: "100", Rebates: "0", NetFees: "100", FillCount: 1, TakerFillCount: 1}},
		{Market: 2, FeeTotals: FeeTotals{MakerFees: "-30", TakerFees: "50", FeesPaid: "50", Rebates: "30", NetFees: "20", FillCount: 2, MakerFillCount: 2, TakerFillCount: 1}},
	}}
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("wrong summary: %+v", summary)
	}
	if !reflect.DeepEqual(fills, before) {
		t.Fatal("modified caller fills")
	}
}
func TestSummarizeFeesBeyondInt64AndJSON(t *testing.T) {
	summary, err := SummarizeFees(7, []Fill{{MakerAccount: 7, MakerFee: math.MaxInt64}, {MakerAccount: 7, MakerFee: math.MaxInt64}})
	if err != nil || summary.NetFees != "18446744073709551614" || summary.FeesPaid != summary.NetFees {
		t.Fatal("overflow", summary, err)
	}
	body, err := json.Marshal(summary)
	if err != nil || !strings.Contains(string(body), `"netFees":"18446744073709551614"`) {
		t.Fatal("lossy JSON", string(body), err)
	}
	summary, err = SummarizeFees(7, []Fill{{TakerAccount: 7, TakerFee: math.MinInt64}})
	if err != nil || summary.Rebates != "9223372036854775808" || summary.NetFees != "-9223372036854775808" {
		t.Fatal("rebate overflow", summary, err)
	}
}
func TestSummarizeFeesEmptyAndZeroFees(t *testing.T) {
	for _, fills := range [][]Fill{nil, {{MakerAccount: 8, TakerAccount: 9, MakerFee: 100, TakerFee: 200}}} {
		summary, err := SummarizeFees(7, fills)
		if err != nil || summary.NetFees != "0" || summary.MakerFees != "0" || summary.FillCount != 0 || len(summary.ByMarket) != 0 {
			t.Fatal("wrong empty summary", summary, err)
		}
		body, _ := json.Marshal(summary)
		if !strings.Contains(string(body), `"byMarket":[]`) {
			t.Fatal("empty markets must serialize as array")
		}
	}
	summary, err := SummarizeFees(7, []Fill{{MakerAccount: 7, MakerFee: 0}})
	if err != nil || summary.FillCount != 1 || summary.MakerFillCount != 1 || summary.NetFees != "0" {
		t.Fatal("zero fee missing", summary, err)
	}
	if _, err := SummarizeFees(0, nil); err == nil {
		t.Fatal("vault accepted")
	}
}
func TestUserFeesPagingAndArchiveErrors(t *testing.T) {
	calls := 0
	var mu sync.Mutex
	callCount := func() int { mu.Lock(); defer mu.Unlock(); return calls }
	var params []UserFillsParams
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var req struct {
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params UserFillsParams `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Method != "HistoryService.GetFills" {
			t.Errorf("unexpected method %s", req.Method)
		}
		calls++
		params = append(params, req.Params)
		page := FillsResult{Index: IndexStatus{FirstHeight: 50, IndexedHeight: 200, ExecutedHeight: 205}}
		if req.Params.After == nil {
			page.Fills = []Fill{{Market: 0, MakerAccount: 7, TakerAccount: 8, MakerFee: -20, TakerFee: 100}}
			page.Next = &Cursor{Height: 100, Index: 2}
		} else {
			page.Fills = []Fill{{Market: 0, MakerAccount: 8, TakerAccount: 7, MakerFee: -20, TakerFee: 100}}
		}
		if calls > 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32002, "message": "archive unavailable"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": page})
	}))
	defer server.Close()
	info, err := NewInfo(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	market := uint32(0)
	p := UserFeesParams{Account: 7, Market: &market, FromHeight: 50, Limit: 100}
	page, err := info.UserFees(context.Background(), p)
	if err != nil || page.Summary.NetFees != "-20" || page.Index.FirstHeight != 50 || page.Next == nil || page.Next.Height != 100 || page.Next.Index != 2 {
		t.Fatal("wrong first page", page, err)
	}
	p.After = page.Next
	page, err = info.UserFees(context.Background(), p)
	if err != nil || page.Summary.TakerFees != "100" || page.Summary.NetFees != "100" || page.Next != nil {
		t.Fatal("wrong next page", page, err)
	}
	mu.Lock()
	if calls != 2 || params[0].Account == nil || *params[0].Account != 7 || params[0].Market == nil || *params[0].Market != 0 || params[0].FromHeight != 50 || params[0].Limit != 100 || params[1].After == nil || *params[1].After != (Cursor{Height: 100, Index: 2}) {
		t.Fatal("filters or pagination lost", params)
	}
	mu.Unlock()
	if _, err := info.UserFees(context.Background(), UserFeesParams{}); err == nil || callCount() != 2 {
		t.Fatal("invalid account queried node")
	}
	page, err = info.UserFees(context.Background(), p)
	if err == nil || page != nil || !strings.Contains(err.Error(), "archive unavailable") {
		t.Fatal("archive error reported as zero fees", page, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if page, err = info.UserFees(ctx, p); err == nil || page != nil {
		t.Fatal("cancelled query succeeded")
	}
}
