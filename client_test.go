package apollo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestJSONRPCBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reply   func(uint64) string
		wantRPC bool
	}{
		{"normal", func(id uint64) string {
			return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"counter":"18446744073709551615"}}`, id)
		}, false},
		{"error", func(id uint64) string {
			return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":-32004,"message":"not found"}}`, id)
		}, true},
		{"bad-id", func(uint64) string { return `{"jsonrpc":"2.0","id":99,"result":{}}` }, false},
		{"bad-version", func(id uint64) string { return fmt.Sprintf(`{"jsonrpc":"1.0","id":%d,"result":{}}`, id) }, false},
		{"missing-result", func(id uint64) string { return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d}`, id) }, false},
		{"null", func(id uint64) string { return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":null}`, id) }, false},
		{"array", func(id uint64) string { return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":[]}`, id) }, false},
		{"ambiguous", func(id uint64) string {
			return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{},"error":{"code":-32004}}`, id)
		}, false},
		{"bad-json", func(uint64) string { return `<html>upstream</html>` }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.RequestURI() != "/rpc?key=fixture/" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("wrong HTTP request")
				}
				var request struct {
					ID uint64 `json:"id"`
				}
				_ = json.NewDecoder(r.Body).Decode(&request)
				fmt.Fprint(w, tc.reply(request.ID))
			}))
			defer server.Close()
			client, err := NewClient(server.URL + "/rpc?key=fixture/")
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Counter Uint64 `json:"counter"`
			}
			err = client.Call(context.Background(), "ChainService.GetChainInfo", nil, &result)
			if tc.name == "normal" {
				if err != nil || uint64(result.Counter) != ^uint64(0) {
					t.Fatal("integer precision", result, err)
				}
			} else {
				if err == nil {
					t.Fatal("accepted malformed/error response")
				}
				var rpc *RPCError
				if tc.wantRPC && (!errors.As(err, &rpc) || rpc.Code != -32004) {
					t.Fatal("RPC error not typed", err)
				}
			}
		})
	}
}
func TestHTTPStatusAndNoRedirect(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls++ }))
	defer target.Close()
	for _, status := range []int{301, 302, 307, 308, 429, 502} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(status)
			}))
			defer server.Close()
			httpClient := server.Client()
			client, err := NewClient(server.URL, WithHTTPClient(httpClient))
			if err != nil {
				t.Fatal(err)
			}
			err = client.Call(context.Background(), "ChainService.GetChainInfo", nil, nil)
			var typed *HTTPError
			if !errors.As(err, &typed) || typed.StatusCode != status || typed.RetryAfter != "2" {
				t.Fatal("HTTP error missing", err)
			}
		})
	}
	if targetCalls != 0 {
		t.Fatal("followed signed POST redirect")
	}
}
func TestSizeLimits(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, strings.Repeat("x", 1024)) }))
	defer server.Close()
	c, e := NewClient(server.URL, WithSizeLimits(4, 2048))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Call(context.Background(), "M", nil, nil); !errors.Is(e, ErrRequestTooLarge) || calls != 0 {
		t.Fatal(e)
	}
	c, e = NewClient(server.URL, WithSizeLimits(2048, 32))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Call(context.Background(), "M", nil, nil); !errors.Is(e, ErrResponseTooLarge) {
		t.Fatal(e)
	}
}
func TestRequestDeadlineAndCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() { close(release); server.Close() }()
	c, e := NewClient(server.URL, WithTimeout(20*time.Millisecond))
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Call(context.Background(), "M", nil, nil); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = c.Call(ctx, "M", nil, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestConcurrentRequestIDs(t *testing.T) {
	seen := map[uint64]bool{}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID uint64 `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		if seen[request.ID] {
			t.Error("duplicate RPC ID")
		}
		seen[request.ID] = true
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{}})
	}))
	defer server.Close()
	c, _ := NewClient(server.URL)
	var wg sync.WaitGroup
	for j := 0; j < 30; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := c.Call(context.Background(), "M", nil, nil); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if len(seen) != 30 {
		t.Fatal("lost request IDs")
	}
}
func TestInvalidEndpointsAndOptions(t *testing.T) {
	for _, url := range []string{"/rpc", "file:///tmp/rpc", "https://user:password@node", "https://node/#fragment", "http://"} {
		if _, e := NewClient(url); e == nil {
			t.Fatal("accepted", url)
		}
	}
	for _, option := range []Option{WithTimeout(0), WithSizeLimits(0, 1), WithHTTPClient(nil), nil} {
		if _, e := NewClient("https://node", option); e == nil {
			t.Fatal("accepted invalid option")
		}
	}
	c, _ := NewClient("https://node")
	if e := c.Call(context.Background(), "ExampleService.WatchStream", nil, nil); e == nil {
		t.Fatal("streaming accepted")
	}
}
func TestWireTypes(t *testing.T) {
	for _, value := range []string{`"9223372036854775807"`, `9223372036854775807`, `"-9223372036854775808"`} {
		var n Int64
		if e := json.Unmarshal([]byte(value), &n); e != nil {
			t.Fatal(value, e)
		}
	}
	for _, bad := range []string{`null`, `"9223372036854775808"`, `1.5`, `"NaN"`} {
		var n Int64
		if e := json.Unmarshal([]byte(bad), &n); e == nil {
			t.Fatal("accepted", bad)
		}
	}
	var u Uint64
	if e := json.Unmarshal([]byte(`"18446744073709551615"`), &u); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal([]byte(`"-1"`), &u); e == nil {
		t.Fatal("negative u64")
	}
	raw := []byte(`{"value":"0x00ff"}`)
	var x struct {
		Value HexBytes `json:"value"`
	}
	if e := json.Unmarshal(raw, &x); e != nil || !bytes.Equal(x.Value, []byte{0, 255}) {
		t.Fatal(e)
	}
	for _, bad := range []string{`"0xzz"`, `"AQID"`, `null`} {
		var h Hash
		if e := json.Unmarshal([]byte(bad), &h); e == nil {
			t.Fatal("invalid fixed hash")
		}
	}
}
func TestInfoOptionalFiltersAndDefaults(t *testing.T) {
	var params json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     uint64          `json:"id"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		params = append(params[:0], request.Params...)
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{}}`, request.ID)
	}))
	defer server.Close()
	i, _ := NewInfo(server.URL)
	ctx := context.Background()
	r, e := i.OpenOrders(ctx, 7, nil)
	if e != nil || len(r.Orders) != 0 {
		t.Fatal(e)
	}
	if bytes.Contains(params, []byte(`"market"`)) {
		t.Fatal("nil filter sent")
	}
	market := uint32(0)
	_, e = i.OpenOrders(ctx, 7, &market)
	if e != nil || !bytes.Contains(params, []byte(`"market":0`)) {
		t.Fatal("zero filter missing", e)
	}
}
