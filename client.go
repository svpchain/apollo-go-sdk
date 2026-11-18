package apollo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

const Version = "1.0.2"

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("Apollo JSON-RPC %d: %s", e.Code, e.Message) }

type HTTPError struct {
	StatusCode int
	RetryAfter string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("Apollo HTTP status %d", e.StatusCode) }

var ErrResponseTooLarge = errors.New("Apollo response exceeds size limit")
var ErrRequestTooLarge = errors.New("Apollo request exceeds size limit")

type Option func(*config) error
type config struct {
	http                        *http.Client
	timeout                     time.Duration
	requestLimit, responseLimit int64
	account                     uint32
	expected                    *Hash
	nonces                      NonceManager
}

func defaults() config {
	return config{timeout: 10 * time.Second, requestLimit: 4 << 20, responseLimit: 16 << 20}
}
func WithHTTPClient(c *http.Client) Option {
	return func(o *config) error {
		if c == nil {
			return errors.New("nil HTTP client")
		}
		o.http = c
		return nil
	}
}
func WithTimeout(t time.Duration) Option {
	return func(o *config) error {
		if t <= 0 {
			return errors.New("timeout must be positive")
		}
		o.timeout = t
		return nil
	}
}
func WithSizeLimits(request, response int64) Option {
	return func(o *config) error {
		if request <= 0 || response <= 0 || request > 1<<30 || response > 1<<30 {
			return errors.New("size limits must be 1..1GiB")
		}
		o.requestLimit = request
		o.responseLimit = response
		return nil
	}
}
func WithAccount(id uint32) Option {
	return func(o *config) error {
		if id == 0 {
			return errors.New("account 0 is reserved")
		}
		o.account = id
		return nil
	}
}
func WithExpectedGenesisHash(h Hash) Option {
	return func(o *config) error { o.expected = &h; return nil }
}
func WithNonceManager(n NonceManager) Option {
	return func(o *config) error {
		if n == nil {
			return errors.New("nil nonce manager")
		}
		o.nonces = n
		return nil
	}
}
func options(opts []Option) (config, error) {
	o := defaults()
	for _, opt := range opts {
		if opt == nil {
			return o, errors.New("nil option")
		}
		if err := opt(&o); err != nil {
			return o, err
		}
	}
	return o, nil
}

type Client struct {
	endpoint                    string
	http                        *http.Client
	timeout                     time.Duration
	requestLimit, responseLimit int64
	nextID                      atomic.Uint64
}

// NewClient validates an absolute node endpoint. Construction never performs IO.
func NewClient(endpoint string, opts ...Option) (*Client, error) {
	o, e := options(opts)
	if e != nil {
		return nil, e
	}
	return newClient(endpoint, o)
}
func newClient(endpoint string, o config) (*Client, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return nil, errors.New("endpoint must be absolute HTTP(S), without credentials or fragment")
	}
	c := o.http
	if c == nil {
		c = &http.Client{}
	} else {
		copy := *c
		c = &copy
	}
	// Never follow redirects carrying a signed action to a different destination.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{endpoint: endpoint, http: c, timeout: o.timeout, requestLimit: o.requestLimit, responseLimit: o.responseLimit}, nil
}

// Call sends one unary request. It never retries automatically, including Submit.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	if method == "" {
		return errors.New("empty RPC method")
	}
	if name := method[strings.LastIndex(method, ".")+1:]; strings.HasPrefix(name, "Stream") || strings.HasSuffix(name, "Stream") {
		return errors.New("streaming methods are not supported over HTTP JSON-RPC")
	}
	if params == nil {
		params = struct{}{}
	}
	id := c.nextID.Add(1)
	body, e := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", id, method, params})
	if e != nil {
		return e
	}
	if int64(len(body)) > c.requestLimit {
		return ErrRequestTooLarge
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	request, e := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if e != nil {
		return e
	}
	request.Header.Set("Content-Type", "application/json")
	response, e := c.http.Do(request)
	if e != nil {
		// Do not include request URLs or signed bytes in returned error strings.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("Apollo transport failed: %w", redactTransportError(e))
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, c.responseLimit+1))
	if e != nil {
		return e
	}
	if int64(len(data)) > c.responseLimit {
		return ErrResponseTooLarge
	}
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if e = json.Unmarshal(data, &env); e != nil {
		return fmt.Errorf("invalid JSON-RPC envelope: %w", e)
	}
	if env.JSONRPC != "2.0" || string(env.ID) != fmt.Sprint(id) {
		return errors.New("JSON-RPC version or response id mismatch")
	}
	if len(env.Error) > 0 && len(env.Result) > 0 {
		return errors.New("ambiguous JSON-RPC envelope")
	}
	if len(env.Error) > 0 {
		var rpcErr RPCError
		if e = json.Unmarshal(env.Error, &rpcErr); e != nil || string(env.Error) == "null" {
			return errors.New("invalid JSON-RPC error")
		}
		return &rpcErr
	}
	if len(env.Result) == 0 || len(bytes.TrimSpace(env.Result)) == 0 || bytes.TrimSpace(env.Result)[0] != '{' {
		return errors.New("JSON-RPC result must be an object")
	}
	if result == nil {
		return nil
	}
	if e = json.Unmarshal(env.Result, result); e != nil {
		return fmt.Errorf("decode %s result: %w", method, e)
	}
	return nil
}
func redactTransportError(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}
