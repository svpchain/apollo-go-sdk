package apollo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type ChainIdentityError struct{ Expected, Actual Hash }

func (e *ChainIdentityError) Error() string {
	return fmt.Sprintf("Apollo genesis mismatch: expected %s, got %s", e.Expected, e.Actual)
}

// SubmissionUnknownError retains every original signature when broadcast could have succeeded.
// Query the original account/nonce before deciding whether to resend.
type SubmissionUnknownError struct {
	Prepared []PreparedAction
	Cause    error
}

func (e *SubmissionUnknownError) Error() string {
	return "Apollo submission result is unknown; reconcile original account/nonce: " + e.Cause.Error()
}
func (e *SubmissionUnknownError) Unwrap() error { return e.Cause }

type TrackingError struct {
	Response *OrderResponse
	Cause    error
}

func (e *TrackingError) Error() string {
	return "Apollo accepted action(s), but execution tracking failed: " + e.Cause.Error()
}
func (e *TrackingError) Unwrap() error { return e.Cause }

var ErrActionAllowance = errors.New("Apollo account action allowance is exhausted or insufficient")

type PreparedAction struct {
	Account         uint32
	Nonce           uint64
	GenesisHash     Hash
	Action          [48]byte
	Auth            [78]byte
	TransactionHash Hash
}
type SubmitStatus string

const (
	SubmitAccepted     SubmitStatus = "SUBMIT_STATUS_ACCEPTED"
	SubmitMalformed    SubmitStatus = "SUBMIT_STATUS_MALFORMED"
	SubmitBadSignature SubmitStatus = "SUBMIT_STATUS_BAD_SIGNATURE"
	SubmitUnavailable  SubmitStatus = "SUBMIT_STATUS_UNAVAILABLE"
)

type SubmitResult struct {
	ID                Uint64         `json:"id"`
	Statuses          []SubmitStatus `json:"statuses"`
	TransactionHashes []HexBytes     `json:"transactionHashes"`
	Detail            string         `json:"detail"`
}
type Outcome string

const (
	OutcomePending      Outcome = "pending"
	OutcomeExecuted     Outcome = "executed"
	OutcomeNonceUsed    Outcome = "nonce_used"
	OutcomeNotSubmitted Outcome = "not_submitted"
)

type OrderStatus struct {
	Prepared     PreparedAction
	SubmitStatus SubmitStatus
	Outcome      Outcome
	Execution    *ActionStatus
	Error        string
}
type OrderResponse struct {
	ID       uint64
	Statuses []OrderStatus
	Detail   string
}

// Successful requires execution success for all items. Accepted/pending is not success.
func (r *OrderResponse) Successful() bool {
	if r == nil || len(r.Statuses) == 0 {
		return false
	}
	for _, s := range r.Statuses {
		if s.Outcome != OutcomeExecuted || s.Execution == nil || !s.Execution.OK {
			return false
		}
	}
	return true
}

type SubmitOption func(*submitConfig) error
type submitConfig struct {
	wait                  bool
	waitTimeout, interval time.Duration
}

func NoWait() SubmitOption { return func(o *submitConfig) error { o.wait = false; return nil } }
func WaitTimeout(d time.Duration) SubmitOption {
	return func(o *submitConfig) error {
		if d <= 0 {
			return errors.New("wait timeout must be positive")
		}
		o.waitTimeout = d
		return nil
	}
}
func PollInterval(d time.Duration) SubmitOption {
	return func(o *submitConfig) error {
		if d <= 0 {
			return errors.New("poll interval must be positive")
		}
		o.interval = d
		return nil
	}
}
func submitOptions(opts []SubmitOption) (submitConfig, error) {
	o := submitConfig{wait: true, waitTimeout: 15 * time.Second, interval: 250 * time.Millisecond}
	for _, opt := range opts {
		if opt == nil {
			return o, errors.New("nil submit option")
		}
		if e := opt(&o); e != nil {
			return o, e
		}
	}
	return o, nil
}

type Exchange struct {
	info     *Info
	signer   Signer
	identity SignerIdentity
	account  uint32
	nonces   NonceManager
	mu       sync.Mutex
	pinned   *Hash
	submitID atomic.Uint64
}

// NewExchange never reads private keys from disk or connects until a method is called.
// WithAccount is required. A shared NonceManager is necessary across concurrent exchanges for one account.
func NewExchange(endpoint string, signer Signer, opts ...Option) (*Exchange, error) {
	if signer == nil {
		return nil, errors.New("signer is required")
	}
	o, err := options(opts)
	if err != nil {
		return nil, err
	}
	if o.account == 0 {
		return nil, errors.New("WithAccount is required")
	}
	id := signer.Identity()
	if id.Slot > 4 {
		return nil, errors.New("signer slot must be 0..4")
	}
	if id.Slot == 0 && id.Owner == (Address{}) {
		return nil, errors.New("owner address cannot be empty")
	}
	if id.Slot > 0 && id.PublicKey == ([32]byte{}) {
		return nil, errors.New("agent public key cannot be empty")
	}
	client, err := newClient(endpoint, o)
	if err != nil {
		return nil, err
	}
	if o.nonces == nil {
		o.nonces = NewNonceManager()
	}
	return &Exchange{info: &Info{client: client}, signer: signer, identity: id, account: o.account, nonces: o.nonces, pinned: o.expected}, nil
}
func (e *Exchange) Info() *Info     { return e.info }
func (e *Exchange) Account() uint32 { return e.account }

// Connect pins the first genesis, or verifies a configured genesis. Every write rechecks it.
func (e *Exchange) Connect(ctx context.Context) (*ChainInfo, error) {
	identity, err := e.info.ChainInfo(ctx)
	if err != nil {
		return nil, err
	}
	if identity.GenesisHash == (Hash{}) {
		return nil, errors.New("node returned an empty genesis hash")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pinned != nil && *e.pinned != identity.GenesisHash {
		return nil, &ChainIdentityError{*e.pinned, identity.GenesisHash}
	}
	hash := identity.GenesisHash
	e.pinned = &hash
	return identity, nil
}
func (e *Exchange) signerSequence(ctx context.Context, chain *ChainInfo) (uint32, error) {
	if e.identity.Slot == 0 {
		state, err := e.info.UserStateByID(ctx, e.account)
		if err != nil {
			return 0, err
		}
		if uint64(state.Account) != uint64(e.account) || state.Owner != e.identity.Owner {
			return 0, errors.New("signer is not this account's owner")
		}
		return 0, nil
	}
	agents, err := e.info.Agents(ctx, e.account)
	if err != nil {
		return 0, err
	}
	height := chain.ExecutedHeight
	if agents.At.ExecutedHeight > height {
		height = agents.At.ExecutedHeight
	}
	for _, agent := range agents.Agents {
		if agent.Slot == uint32(e.identity.Slot) && bytes.Equal(agent.Key, e.identity.PublicKey[:]) && (agent.Expiry == 0 || agent.Expiry >= height) {
			return agent.Seq, nil
		}
	}
	return 0, errors.New("agent slot/key is unapproved, revoked or expired")
}
func (e *Exchange) Prepare(ctx context.Context, action Action) (PreparedAction, error) {
	items, err := e.PrepareActions(ctx, []Action{action})
	if err != nil {
		return PreparedAction{}, err
	}
	return items[0], nil
}
func (e *Exchange) PrepareActions(ctx context.Context, actions []Action) ([]PreparedAction, error) {
	if len(actions) == 0 || len(actions) > 20000 {
		return nil, errors.New("prepare requires 1..20000 actions")
	}
	encoded := make([][48]byte, len(actions))
	charged := uint64(0)
	for i, action := range actions {
		if action.Account != e.account || !isTradingAction(action.Type) {
			return nil, errors.New("action belongs to another account or is unsupported")
		}
		if e.identity.Slot > 0 && !AgentMaySign(action) {
			return nil, errors.New("action requires owner signature")
		}
		raw, err := EncodeAction(action)
		if err != nil {
			return nil, fmt.Errorf("action %d: %w", i, err)
		}
		encoded[i] = raw
		if action.Type != Cancel && action.Type != CancelAll {
			charged++
		}
	}
	chain, err := e.Connect(ctx)
	if err != nil {
		return nil, err
	}
	seq, err := e.signerSequence(ctx, chain)
	if err != nil {
		return nil, err
	}
	if charged > 0 {
		allowance, err := e.info.UserRateLimit(ctx, e.account)
		if err != nil {
			return nil, err
		}
		if allowance.Used > allowance.Limit || uint64(allowance.Limit-allowance.Used) < charged {
			return nil, ErrActionAllowance
		}
	}
	state, err := e.info.Nonce(ctx, e.account)
	if err != nil {
		return nil, err
	}
	out := make([]PreparedAction, 0, len(actions))
	lastAllocated := uint64(state.LastNonce)
	used := make(map[uint64]bool, len(actions))
	for i, raw := range encoded {
		nonce, err := e.nonces.Next(ctx, e.account, lastAllocated)
		if err != nil {
			return nil, err
		}
		if nonce == 0 || nonce <= lastAllocated || used[nonce] {
			return nil, errors.New("nonce manager returned a non-increasing or duplicate nonce")
		}
		if createsOrder(actions[i].Type) && nonce >= 1<<56 {
			return nil, errors.New("order nonce occupies protocol-reserved high bits")
		}
		used[nonce] = true
		lastAllocated = nonce
		request := SignRequest{GenesisHash: chain.GenesisHash, Action: raw, Nonce: nonce, Seq: seq}
		auth, err := e.signer.Sign(ctx, request)
		if err != nil {
			return nil, err
		}
		if err = verifySignature(e.identity, request, auth); err != nil {
			return nil, err
		}
		authBytes, err := EncodeAuth(auth)
		if err != nil {
			return nil, err
		}
		out = append(out, PreparedAction{Account: e.account, Nonce: nonce, GenesisHash: chain.GenesisHash, Action: raw, Auth: authBytes, TransactionHash: TransactionHash(raw, authBytes)})
	}
	return out, nil
}
func (e *Exchange) Execute(ctx context.Context, action Action, opts ...SubmitOption) (*OrderResponse, error) {
	return e.ExecuteActions(ctx, []Action{action}, opts...)
}
func (e *Exchange) ExecuteActions(ctx context.Context, actions []Action, opts ...SubmitOption) (*OrderResponse, error) {
	if _, err := submitOptions(opts); err != nil {
		return nil, err
	}
	prepared, err := e.PrepareActions(ctx, actions)
	if err != nil {
		return nil, err
	}
	return e.SubmitPrepared(ctx, prepared, opts...)
}
func (e *Exchange) SubmitPrepared(ctx context.Context, prepared []PreparedAction, opts ...SubmitOption) (*OrderResponse, error) {
	config, err := submitOptions(opts)
	if err != nil {
		return nil, err
	}
	if len(prepared) == 0 || len(prepared) > 20000 {
		return nil, errors.New("submit requires 1..20000 actions")
	}
	// Own a copy before any asynchronous work. All nested signed fields are fixed arrays.
	prepared = append([]PreparedAction(nil), prepared...)
	keys := make(map[uint64]bool, len(prepared))
	for _, p := range prepared {
		auth, err := DecodeAuth(p.Auth)
		if err != nil {
			return nil, err
		}
		if p.Account != e.account || p.Account != actionAccount(p.Action) || p.Nonce != auth.Nonce || !isTradingAction(actionType(p.Action)) || p.TransactionHash != TransactionHash(p.Action, p.Auth) || keys[p.Nonce] {
			return nil, errors.New("prepared action metadata/hash mismatch or duplicate nonce")
		}
		keys[p.Nonce] = true
		if createsOrder(actionType(p.Action)) && p.Nonce >= 1<<56 {
			return nil, errors.New("order nonce occupies reserved bits")
		}
		if e.identity.Slot > 0 && (actionType(p.Action) == ApproveAgent || actionType(p.Action) == RevokeAgent) {
			return nil, errors.New("agent cannot manage agent keys")
		}
	}
	chain, err := e.Connect(ctx)
	if err != nil {
		return nil, err
	}
	seq, err := e.signerSequence(ctx, chain)
	if err != nil {
		return nil, err
	}
	for _, p := range prepared {
		if p.GenesisHash != chain.GenesisHash {
			return nil, &ChainIdentityError{p.GenesisHash, chain.GenesisHash}
		}
		auth, _ := DecodeAuth(p.Auth)
		if err := verifySignature(e.identity, SignRequest{p.GenesisHash, p.Action, p.Nonce, seq}, auth); err != nil {
			return nil, err
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	id := e.submitID.Add(1)
	type signed struct {
		Action HexBytes `json:"action"`
		Auth   HexBytes `json:"auth"`
	}
	request := struct {
		ID      Uint64   `json:"id"`
		Actions []signed `json:"actions"`
	}{ID: Uint64(id), Actions: make([]signed, len(prepared))}
	for i, p := range prepared {
		request.Actions[i] = signed{p.Action[:], p.Auth[:]}
	}
	var submitted SubmitResult
	err = e.info.client.Call(ctx, "TxService.Submit", request, &submitted)
	if err == nil {
		err = checkSubmitResponse(id, prepared, &submitted)
	}
	if err != nil {
		return nil, &SubmissionUnknownError{Prepared: prepared, Cause: err}
	}
	result := &OrderResponse{ID: id, Detail: submitted.Detail, Statuses: make([]OrderStatus, len(prepared))}
	pending := []ActionKey{}
	indices := []int{}
	for i, p := range prepared {
		s := OrderStatus{Prepared: p, SubmitStatus: submitted.Statuses[i], Outcome: OutcomeNotSubmitted}
		if s.SubmitStatus == SubmitAccepted {
			s.Outcome = OutcomePending
			pending = append(pending, ActionKey{Uint64(p.Account), Uint64(p.Nonce)})
			indices = append(indices, i)
		} else {
			s.Error = string(s.SubmitStatus) + ": " + submitted.Detail
		}
		result.Statuses[i] = s
	}
	if !config.wait || len(pending) == 0 {
		return result, nil
	}
	return e.wait(ctx, result, pending, indices, config)
}
func checkSubmitResponse(id uint64, prepared []PreparedAction, r *SubmitResult) error {
	if uint64(r.ID) != id || len(r.Statuses) != len(prepared) {
		return errors.New("Submit response id or item count mismatch")
	}
	if len(r.TransactionHashes) != 0 && len(r.TransactionHashes) != len(prepared) {
		return errors.New("Submit response hash count mismatch")
	}
	for i, status := range r.Statuses {
		switch status {
		case SubmitAccepted, SubmitMalformed, SubmitBadSignature, SubmitUnavailable:
		default:
			return fmt.Errorf("unknown Submit status %q", status)
		}
		if len(r.TransactionHashes) > 0 {
			hash := r.TransactionHashes[i]
			if status == SubmitAccepted {
				if len(hash) != 32 || !bytes.Equal(hash, prepared[i].TransactionHash[:]) {
					return errors.New("Submit returned a different transaction hash")
				}
			} else if len(hash) != 0 {
				return errors.New("rejected Submit item has a nonempty hash")
			}
		}
	}
	return nil
}
func (e *Exchange) wait(ctx context.Context, result *OrderResponse, keys []ActionKey, indices []int, config submitConfig) (*OrderResponse, error) {
	waiting, cancel := context.WithTimeout(ctx, config.waitTimeout)
	defer cancel()
	for len(keys) > 0 {
		statuses := &ActionStatuses{}
		var err error
		for start := 0; start < len(keys); start += 10000 {
			end := start + 10000
			if end > len(keys) {
				end = len(keys)
			}
			var page *ActionStatuses
			page, err = e.info.ActionStatus(waiting, keys[start:end])
			if err != nil {
				break
			}
			statuses.Statuses = append(statuses.Statuses, page.Statuses...)
		}
		if err != nil {
			if ctx.Err() != nil {
				return result, &TrackingError{result, ctx.Err()}
			}
			if errors.Is(waiting.Err(), context.DeadlineExceeded) {
				return result, nil
			}
			return result, &TrackingError{result, err}
		}
		nextKeys := make([]ActionKey, 0, len(keys))
		nextIndices := make([]int, 0, len(keys))
		for j, s := range statuses.Statuses {
			item := &result.Statuses[indices[j]]
			switch s.State {
			case ActionExecuted:
				copy := s
				item.Execution = &copy
				item.Outcome = OutcomeExecuted
				if !s.OK {
					item.Error = s.Reject
				}
			case ActionNonceUsed:
				item.Outcome = OutcomeNonceUsed
			case ActionNotFound:
				nextKeys = append(nextKeys, keys[j])
				nextIndices = append(nextIndices, indices[j])
			}
		}
		keys, indices = nextKeys, nextIndices
		if len(keys) == 0 {
			return result, nil
		}
		timer := time.NewTimer(config.interval)
		select {
		case <-waiting.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return result, &TrackingError{result, ctx.Err()}
			}
			return result, nil
		case <-timer.C:
		}
	}
	return result, nil
}

// PrepareOrder checks human-unit inputs and signs, but never broadcasts.
func (e *Exchange) PrepareOrder(ctx context.Context, r CreateOrderRequest) (PreparedAction, error) {
	m, err := e.info.Market(ctx, r.Coin)
	if err != nil {
		return PreparedAction{}, err
	}
	r, err = e.resolveReference(ctx, r, m)
	if err != nil {
		return PreparedAction{}, err
	}
	a, err := buildOrder(e.account, r, m)
	if err != nil {
		return PreparedAction{}, err
	}
	return e.Prepare(ctx, a)
}
func (e *Exchange) Order(ctx context.Context, r CreateOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	return e.BulkOrders(ctx, []CreateOrderRequest{r}, opts...)
}
func (e *Exchange) BulkOrders(ctx context.Context, orders []CreateOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	if _, err := submitOptions(opts); err != nil {
		return nil, err
	}
	if len(orders) == 0 || len(orders) > 20000 {
		return nil, errors.New("BulkOrders requires 1..20000 orders")
	}
	meta, err := e.info.Meta(ctx)
	if err != nil {
		return nil, err
	}
	markets := make(map[string]Market, len(meta.Markets))
	for _, m := range meta.Markets {
		markets[m.Ticker] = m
	}
	actions := make([]Action, len(orders))
	for i, r := range orders {
		m, ok := markets[r.Coin]
		if !ok {
			return nil, fmt.Errorf("unknown coin %q", r.Coin)
		}
		r, err = e.resolveReference(ctx, r, m)
		if err != nil {
			return nil, err
		}
		actions[i], err = buildOrder(e.account, r, m)
		if err != nil {
			return nil, fmt.Errorf("order %d: %w", i, err)
		}
	}
	return e.ExecuteActions(ctx, actions, opts...)
}
func (e *Exchange) resolveReference(ctx context.Context, r CreateOrderRequest, m Market) (CreateOrderRequest, error) {
	if r.OrderType.Trigger == nil || !r.OrderType.Trigger.IsMarket || r.ReferencePrice != "" {
		return r, nil
	}
	reference, err := e.reference(ctx, m, r.IsBuy)
	r.ReferencePrice = reference
	return r, err
}
func (e *Exchange) reference(ctx context.Context, m Market, isBuy bool) (string, error) {
	book, err := e.info.L2Snapshot(ctx, m.ID, 1)
	if err != nil {
		return "", err
	}
	levels := book.Bids
	if isBuy {
		levels = book.Asks
	}
	if len(levels) > 0 {
		return SubticksToPrice(int64(levels[0].Price), m)
	}
	return PriceUnitsToPrice(int64(m.Mark), m)
}
func (e *Exchange) Cancel(ctx context.Context, r CancelOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	return e.BulkCancel(ctx, []CancelOrderRequest{r}, opts...)
}
func (e *Exchange) BulkCancel(ctx context.Context, requests []CancelOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	if len(requests) == 0 || len(requests) > 20000 {
		return nil, errors.New("BulkCancel requires 1..20000 requests")
	}
	meta, err := e.info.Meta(ctx)
	if err != nil {
		return nil, err
	}
	actions := make([]Action, len(requests))
	for j, r := range requests {
		found := false
		for _, m := range meta.Markets {
			if m.Ticker == r.Coin {
				if m.ID >= 65535 {
					return nil, errors.New("invalid market id")
				}
				actions[j] = Action{Type: Cancel, Account: e.account, Market: uint16(m.ID), OrderID: r.OrderID}
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown coin %q", r.Coin)
		}
	}
	return e.ExecuteActions(ctx, actions, opts...)
}

// CancelAll cancels all markets when coin is empty, without spending action allowance.
func (e *Exchange) CancelAll(ctx context.Context, coin string, opts ...SubmitOption) (*OrderResponse, error) {
	market := uint16(math.MaxUint16)
	if coin != "" {
		m, err := e.info.Market(ctx, coin)
		if err != nil {
			return nil, err
		}
		if m.ID >= 65535 {
			return nil, errors.New("invalid market id")
		}
		market = uint16(m.ID)
	}
	return e.Execute(ctx, Action{Type: CancelAll, Account: e.account, Market: market}, opts...)
}

// ModifyOrder replaces one order; execution failure can leave the old order cancelled.
func (e *Exchange) ModifyOrder(ctx context.Context, r ModifyOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	return e.BulkModify(ctx, []ModifyOrderRequest{r}, opts...)
}

// BulkModify validates the entire batch before signing and submits once.
// Results follow request order; the batch is not atomic.
func (e *Exchange) BulkModify(ctx context.Context, requests []ModifyOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	if _, err := submitOptions(opts); err != nil {
		return nil, err
	}
	if len(requests) == 0 || len(requests) > 20000 {
		return nil, errors.New("BulkModify requires 1..20000 requests")
	}
	requests = append([]ModifyOrderRequest(nil), requests...)
	meta, err := e.info.Meta(ctx)
	if err != nil {
		return nil, err
	}
	markets := make(map[string]Market, len(meta.Markets))
	for _, m := range meta.Markets {
		markets[m.Ticker] = m
	}
	actions := make([]Action, len(requests))
	for j, r := range requests {
		m, ok := markets[r.Coin]
		if !ok {
			return nil, fmt.Errorf("modify %d: unknown coin %q", j, r.Coin)
		}
		if m.ID >= 65535 || m.Status != MarketActive {
			return nil, fmt.Errorf("modify %d: market is not active or encodable", j)
		}
		price, err := PriceToSubticks(r.Price, m)
		if err != nil {
			return nil, fmt.Errorf("modify %d: %w", j, err)
		}
		size, err := SizeToQuantums(r.Size, m)
		if err != nil {
			return nil, fmt.Errorf("modify %d: %w", j, err)
		}
		actions[j] = Action{Type: Modify, Account: e.account, Market: uint16(m.ID), OrderID: r.OrderID, Price: price, Size: size}
	}
	return e.ExecuteActions(ctx, actions, opts...)
}

// CancelTwap cancels a TWAP parent and its current child using the existing Cancel action.
// OrderID is the parent nonce retained from the TwapOrder result.
func (e *Exchange) CancelTwap(ctx context.Context, r CancelOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	return e.Cancel(ctx, r, opts...)
}
func (e *Exchange) MarketOpen(ctx context.Context, r MarketOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	m, err := e.info.Market(ctx, r.Coin)
	if err != nil {
		return nil, err
	}
	reference := r.ReferencePrice
	if reference == "" {
		reference, err = e.reference(ctx, m, r.IsBuy)
		if err != nil {
			return nil, err
		}
	}
	slip := r.Slippage
	if slip == "" {
		slip = "0.01"
	}
	price, err := marketProtection(reference, r.IsBuy, slip, m)
	if err != nil {
		return nil, err
	}
	priceText, err := SubticksToPrice(price, m)
	if err != nil {
		return nil, err
	}
	return e.Order(ctx, CreateOrderRequest{Coin: r.Coin, IsBuy: r.IsBuy, Price: priceText, Size: r.Size, ReduceOnly: r.ReduceOnly, OrderType: OrderType{Limit: &LimitOrderType{TIF: Ioc}}, Position: r.Position, Isolated: r.Isolated}, opts...)
}

// MarketClose requires an explicit hedge slot and size; it never guesses a position.
func (e *Exchange) MarketClose(ctx context.Context, r ClosePositionRequest, opts ...SubmitOption) (*OrderResponse, error) {
	if r.Position != Long && r.Position != Short {
		return nil, errors.New("close requires Long or Short")
	}
	return e.MarketOpen(ctx, MarketOrderRequest{Coin: r.Coin, IsBuy: r.Position == Short, Size: r.Size, Slippage: r.Slippage, ReferencePrice: r.ReferencePrice, Position: r.Position, Isolated: r.Isolated, ReduceOnly: true}, opts...)
}
func (e *Exchange) ApproveAgent(ctx context.Context, slot uint8, key []byte, expiry uint64, opts ...SubmitOption) (*OrderResponse, error) {
	if len(key) != 32 {
		return nil, errors.New("agent key must be 32 bytes")
	}
	return e.Execute(ctx, Action{Type: ApproveAgent, Account: e.account, Slot: slot, Key: Hex(key), Expiry: expiry}, opts...)
}
func (e *Exchange) RevokeAgent(ctx context.Context, slot uint8, opts ...SubmitOption) (*OrderResponse, error) {
	return e.Execute(ctx, Action{Type: RevokeAgent, Account: e.account, Slot: slot}, opts...)
}
func (e *Exchange) UpdateLeverage(ctx context.Context, coin, leverage string, opts ...SubmitOption) (*OrderResponse, error) {
	m, err := e.info.Market(ctx, coin)
	if err != nil {
		return nil, err
	}
	if m.ID >= 65535 {
		return nil, errors.New("invalid market id")
	}
	imf, err := LeverageToIMF(leverage)
	if err != nil {
		return nil, err
	}
	return e.Execute(ctx, Action{Type: SetLeverage, Account: e.account, Market: uint16(m.ID), IMFPPM: imf}, opts...)
}
func (e *Exchange) UpdateIsolatedMargin(ctx context.Context, coin string, position PositionSide, amount string, opts ...SubmitOption) (*OrderResponse, error) {
	m, err := e.info.Market(ctx, coin)
	if err != nil {
		return nil, err
	}
	if m.ID >= 65535 {
		return nil, errors.New("invalid market id")
	}
	value, err := USDToMicroUSD(amount)
	if err != nil {
		return nil, err
	}
	if value == 0 {
		return nil, errors.New("margin update must be nonzero")
	}
	return e.Execute(ctx, Action{Type: UpdateIsolatedMargin, Account: e.account, Market: uint16(m.ID), Position: position, Amount: value}, opts...)
}
