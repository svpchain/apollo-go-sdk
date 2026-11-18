package apollo

import (
	"context"
	"errors"
	"fmt"
)

type Info struct{ client *Client }

func NewInfo(endpoint string, opts ...Option) (*Info, error) {
	client, e := NewClient(endpoint, opts...)
	if e != nil {
		return nil, e
	}
	return &Info{client: client}, nil
}
func (i *Info) Client() *Client { return i.client }
func (i *Info) ChainInfo(ctx context.Context) (*ChainInfo, error) {
	r := new(ChainInfo)
	err := i.client.Call(ctx, "ChainService.GetChainInfo", struct{}{}, r)
	return r, err
}
func (i *Info) Meta(ctx context.Context) (*Meta, error) {
	r := new(Meta)
	err := i.client.Call(ctx, "MarketService.ListMarkets", struct{}{}, r)
	return r, err
}
func (i *Info) Market(ctx context.Context, coin string) (Market, error) {
	meta, err := i.Meta(ctx)
	if err != nil {
		return Market{}, err
	}
	for _, m := range meta.Markets {
		if m.Ticker == coin {
			return m, nil
		}
	}
	return Market{}, fmt.Errorf("market not found: %s", coin)
}
func (i *Info) L2Snapshot(ctx context.Context, market uint32, depth uint32) (*L2Book, error) {
	r := new(L2Book)
	err := i.client.Call(ctx, "MarketService.GetOrderBook", struct {
		Market uint32 `json:"market"`
		Depth  uint32 `json:"depth"`
	}{market, depth}, r)
	return r, err
}
func (i *Info) Ticker(ctx context.Context, market uint32) (*Ticker, error) {
	r := new(Ticker)
	err := i.client.Call(ctx, "MarketService.GetTicker", struct {
		Market uint32 `json:"market"`
	}{market}, r)
	return r, err
}
func (i *Info) UserState(ctx context.Context, owner Address) (*UserState, error) {
	r := new(UserState)
	err := i.client.Call(ctx, "AccountService.GetAccount", struct {
		Owner Address `json:"owner"`
	}{owner}, r)
	return r, err
}
func (i *Info) UserStateByID(ctx context.Context, account uint32) (*UserState, error) {
	r := new(UserState)
	err := i.client.Call(ctx, "AccountService.GetAccount", accountParams(account), r)
	return r, err
}
func (i *Info) Positions(ctx context.Context, account uint32) (*PositionsResult, error) {
	r := new(PositionsResult)
	err := i.client.Call(ctx, "AccountService.GetPositions", accountParams(account), r)
	return r, err
}
func (i *Info) OpenOrders(ctx context.Context, account uint32, market *uint32) (*OpenOrdersResult, error) {
	r := new(OpenOrdersResult)
	err := i.client.Call(ctx, "AccountService.GetOpenOrders", struct {
		Account Uint64  `json:"account"`
		Market  *uint32 `json:"market,omitempty"`
	}{Uint64(account), market}, r)
	return r, err
}
func (i *Info) Agents(ctx context.Context, account uint32) (*AgentsResult, error) {
	r := new(AgentsResult)
	err := i.client.Call(ctx, "AccountService.GetAgents", accountParams(account), r)
	return r, err
}
func (i *Info) Nonce(ctx context.Context, account uint32) (*NonceState, error) {
	r := new(NonceState)
	err := i.client.Call(ctx, "AccountService.GetNonce", accountParams(account), r)
	return r, err
}
func (i *Info) UserRateLimit(ctx context.Context, account uint32) (*UserRateLimit, error) {
	r := new(UserRateLimit)
	err := i.client.Call(ctx, "TxService.GetAllowance", accountParams(account), r)
	return r, err
}
func (i *Info) ActionStatus(ctx context.Context, keys []ActionKey) (*ActionStatuses, error) {
	if len(keys) == 0 || len(keys) > 10000 {
		return nil, errors.New("ActionStatus requires 1..10000 keys")
	}
	r := new(ActionStatuses)
	err := i.client.Call(ctx, "TxService.GetActionStatus", struct {
		Keys []ActionKey `json:"keys"`
	}{keys}, r)
	if err != nil {
		return nil, err
	}
	if len(r.Statuses) != len(keys) {
		return nil, errors.New("action status count mismatch")
	}
	for j, s := range r.Statuses {
		if s.Key != keys[j] {
			return nil, errors.New("action status key mismatch")
		}
		if s.State != ActionNotFound && s.State != ActionExecuted && s.State != ActionNonceUsed {
			return nil, fmt.Errorf("unknown action state %q", s.State)
		}
	}
	return r, nil
}
func (i *Info) Transaction(ctx context.Context, hash Hash) (*Transaction, error) {
	r := new(Transaction)
	err := i.client.Call(ctx, "TxService.GetTransaction", struct {
		Hash Hash `json:"transactionHash"`
	}{hash}, r)
	if err == nil && r.TransactionHash != hash {
		return nil, errors.New("transaction hash response mismatch")
	}
	return r, err
}
func (i *Info) IndexStatus(ctx context.Context) (*IndexStatus, error) {
	r := new(IndexStatus)
	err := i.client.Call(ctx, "HistoryService.GetIndexStatus", struct{}{}, r)
	return r, err
}
func (i *Info) UserFills(ctx context.Context, p UserFillsParams) (*FillsResult, error) {
	r := new(FillsResult)
	err := i.client.Call(ctx, "HistoryService.GetFills", p, r)
	return r, err
}
func (i *Info) HistoricalOrders(ctx context.Context, p HistoryParams) (*HistoryResult, error) {
	r := new(HistoryResult)
	err := i.client.Call(ctx, "HistoryService.GetOrderHistory", p, r)
	return r, err
}
func (i *Info) ActionHistory(ctx context.Context, p HistoryParams) (*HistoryResult, error) {
	r := new(HistoryResult)
	err := i.client.Call(ctx, "HistoryService.GetActionHistory", p, r)
	return r, err
}
func (i *Info) CandlesSnapshot(ctx context.Context, p CandlesParams) (*CandlesResult, error) {
	r := new(CandlesResult)
	err := i.client.Call(ctx, "HistoryService.GetCandles", p, r)
	return r, err
}
func (i *Info) FundingHistory(ctx context.Context, p FundingHistoryParams) (*FundingHistoryResult, error) {
	r := new(FundingHistoryResult)
	err := i.client.Call(ctx, "HistoryService.GetFundingHistory", p, r)
	return r, err
}
func accountParams(account uint32) any {
	return struct {
		Account Uint64 `json:"account"`
	}{Uint64(account)}
}
