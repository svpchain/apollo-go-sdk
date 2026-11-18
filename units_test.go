package apollo

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

func testMarket() Market {
	return Market{ID: 0, Ticker: "BTC-USD", Status: MarketActive, AtomicResolution: -10, QuantumConversionExponent: -9, PriceExponent: -5, StepBaseQuantums: 1000000, SubticksPerTick: 100000}
}
func TestExactUnitsAndRounding(t *testing.T) {
	m := testMarket()
	price, e := PriceToSubticks("65000", m)
	if e != nil || price != 6500000000 {
		t.Fatal(price, e)
	}
	size, e := SizeToQuantums("0.001", m)
	if e != nil || size != 10000000 {
		t.Fatal(size, e)
	}
	s, e := SubticksToPrice(price, m)
	if e != nil || s != "65000" {
		t.Fatal(s, e)
	}
	s, e = QuantumsToSize(size, m)
	if e != nil || s != "0.001" {
		t.Fatal(s, e)
	}
	s, e = PriceUnitsToPrice(6543210000, m)
	if e != nil || s != "65432.1" {
		t.Fatal(s, e)
	}
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "1e6", "0.00101", "0.00000001"} {
		if _, e := SizeToQuantums(value, m); e == nil {
			t.Fatal("invalid/off-step size", value)
		}
	}
	if _, e := PriceToSubticks("65000.1", m); e == nil {
		t.Fatal("silently rounded price")
	}
	micro, e := USDToMicroUSD("9007199254.740993")
	if e != nil || micro != 9007199254740993 {
		t.Fatal("lost >2^53 integer", micro, e)
	}
	if _, e := USDToMicroUSD("0.0000001"); e == nil {
		t.Fatal("rounded money")
	}
	leverage, e := LeverageToIMF("3")
	if e != nil || leverage != 333334 {
		t.Fatal(leverage, e)
	}
	ask, e := marketProtection("65001", true, "0.01", m)
	if e != nil || ask != 6565200000 {
		t.Fatal(ask, e)
	}
	bid, e := marketProtection("65001", false, "0.01", m)
	if e != nil || bid != 6435000000 {
		t.Fatal(bid, e)
	}
}
func TestMarketExponentValidation(t *testing.T) {
	m := testMarket()
	m.AtomicResolution = -2147483648
	if _, e := PriceToSubticks("1", m); e == nil {
		t.Fatal("unbounded exponent")
	}
	m = testMarket()
	m.SubticksPerTick = 0
	if _, e := SizeToQuantums("1", m); e == nil {
		t.Fatal("invalid market grid")
	}
}
func TestNonceConcurrentMonotonicAndOverflow(t *testing.T) {
	n := NewNonceManager()
	var wg sync.WaitGroup
	values := make(chan uint64, 200)
	for j := 0; j < 200; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := n.Next(context.Background(), 7, 100)
			if e != nil {
				t.Error(e)
				return
			}
			values <- v
		}()
	}
	wg.Wait()
	close(values)
	seen := map[uint64]bool{}
	for v := range values {
		if seen[v] {
			t.Fatal("duplicate")
		}
		seen[v] = true
	}
	if len(seen) != 200 {
		t.Fatal("nonce count")
	}
	if _, e := n.Next(context.Background(), 7, ^uint64(0)); e == nil {
		t.Fatal("u64 overflow")
	}
}
func TestOwnerTypedDataJSON(t *testing.T) {
	data := OwnerTypedData(Hash{1}, [48]byte{}, 9007199254740993)
	encoded, e := json.Marshal(data)
	if e != nil {
		t.Fatal(e)
	}
	var x map[string]json.RawMessage
	if e = json.Unmarshal(encoded, &x); e != nil {
		t.Fatal(e)
	}
	var message struct {
		Nonce  string `json:"nonce"`
		Action string `json:"action"`
	}
	if e = json.Unmarshal(x["message"], &message); e != nil || message.Nonce != "9007199254740993" || len(message.Action) != 98 {
		t.Fatal("typed data layout", e)
	}
}
