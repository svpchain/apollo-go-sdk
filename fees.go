package apollo

import (
	"context"
	"errors"
	"math/big"
	"sort"
)

// FeeTotals summarizes maker/taker trading fees recorded in fills.
// All amounts are decimal integer strings in micro-USD, preserving sums beyond int64.
// FeesPaid and Rebates are positive magnitudes; signed NetFees is paid minus rebates.
// Builder fees, funding and separate rewards are not part of these fill fields.
type FeeTotals struct {
	MakerFees      string `json:"makerFees"`
	TakerFees      string `json:"takerFees"`
	FeesPaid       string `json:"feesPaid"`
	Rebates        string `json:"rebates"`
	NetFees        string `json:"netFees"`
	FillCount      uint64 `json:"fillCount"`
	MakerFillCount uint64 `json:"makerFillCount"`
	TakerFillCount uint64 `json:"takerFillCount"`
}

type MarketFeeSummary struct {
	Market uint32 `json:"market"`
	FeeTotals
}

type UserFeeSummary struct {
	Account uint32 `json:"account"`
	FeeTotals
	ByMarket []MarketFeeSummary `json:"byMarket"`
}

// UserFeesParams selects one account's fills page for trading fee aggregation.
type UserFeesParams struct {
	Account    uint32
	Market     *uint32
	FromHeight Uint64
	After      *Cursor
	Limit      uint32
}

// UserFeesResult covers one fills page, not the account's complete history.
// Pass Next as After with the same filters to continue; Index identifies history coverage.
type UserFeesResult struct {
	Summary UserFeeSummary `json:"summary"`
	Index   IndexStatus    `json:"index"`
	Next    *Cursor        `json:"next,omitempty"`
}

type feeAccumulator struct {
	maker, taker, paid, rebates, net big.Int
	fills, makerFills, takerFills    uint64
}

func (a *feeAccumulator) addFee(fee Int64) {
	value := big.NewInt(int64(fee))
	a.net.Add(&a.net, value)
	if value.Sign() >= 0 {
		a.paid.Add(&a.paid, value)
	} else {
		a.rebates.Sub(&a.rebates, value)
	}
}
func (a *feeAccumulator) add(fill Fill, maker, taker bool) {
	a.fills++
	if maker {
		a.makerFills++
		a.maker.Add(&a.maker, big.NewInt(int64(fill.MakerFee)))
		a.addFee(fill.MakerFee)
	}
	if taker {
		a.takerFills++
		a.taker.Add(&a.taker, big.NewInt(int64(fill.TakerFee)))
		a.addFee(fill.TakerFee)
	}
}
func (a *feeAccumulator) totals() FeeTotals {
	return FeeTotals{MakerFees: a.maker.String(), TakerFees: a.taker.String(),
		FeesPaid: a.paid.String(), Rebates: a.rebates.String(), NetFees: a.net.String(),
		FillCount: a.fills, MakerFillCount: a.makerFills, TakerFillCount: a.takerFills}
}

// SummarizeFees aggregates supplied fills without IO. Pass distinct records to avoid
// double-counting pages. A self-trade counts one fill and includes both fee sides.
func SummarizeFees(account uint32, fills []Fill) (*UserFeeSummary, error) {
	if account == 0 {
		return nil, errors.New("account must be a non-vault u32")
	}
	var total feeAccumulator
	markets := make(map[uint32]*feeAccumulator)
	for _, fill := range fills {
		maker := fill.MakerAccount == Uint64(account)
		taker := fill.TakerAccount == Uint64(account)
		if !maker && !taker {
			continue
		}
		byMarket := markets[fill.Market]
		if byMarket == nil {
			byMarket = new(feeAccumulator)
			markets[fill.Market] = byMarket
		}
		total.add(fill, maker, taker)
		byMarket.add(fill, maker, taker)
	}
	result := &UserFeeSummary{Account: account, FeeTotals: total.totals(), ByMarket: make([]MarketFeeSummary, 0, len(markets))}
	for market, fees := range markets {
		result.ByMarket = append(result.ByMarket, MarketFeeSummary{market, fees.totals()})
	}
	sort.Slice(result.ByMarket, func(i, j int) bool { return result.ByMarket[i].Market < result.ByMarket[j].Market })
	return result, nil
}

// UserFees queries and summarizes one fills page, preserving index coverage and pagination.
func (i *Info) UserFees(ctx context.Context, p UserFeesParams) (*UserFeesResult, error) {
	if p.Account == 0 {
		return nil, errors.New("account must be a non-vault u32")
	}
	account := Uint64(p.Account)
	page, err := i.UserFills(ctx, UserFillsParams{Account: &account, Market: p.Market, FromHeight: p.FromHeight, After: p.After, Limit: p.Limit})
	if err != nil {
		return nil, err
	}
	summary, err := SummarizeFees(p.Account, page.Fills)
	if err != nil {
		return nil, err
	}
	return &UserFeesResult{Summary: *summary, Index: page.Index, Next: page.Next}, nil
}
