package apollo

import (
	"context"
	"errors"
)

type CreateTwapOrderRequest struct {
	Coin                               string
	IsBuy                              bool
	Size, Price                        string
	DurationS, IntervalS, TolerancePPM uint32
	Position                           PositionSide
	ReduceOnly, Isolated               bool
}

func (e *Exchange) TwapOrder(ctx context.Context, r CreateTwapOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	if r.IntervalS < 30 || r.IntervalS > 3600 || r.DurationS < 300 || r.DurationS > 86400 || r.DurationS%r.IntervalS != 0 || r.TolerancePPM >= 1_000_000 {
		return nil, errors.New("invalid TWAP interval/duration/tolerance")
	}
	m, err := e.info.Market(ctx, r.Coin)
	if err != nil {
		return nil, err
	}
	if m.ID >= 65535 || m.Status != MarketActive {
		return nil, errors.New("market is not active or encodable")
	}
	size, err := SizeToQuantums(r.Size, m)
	if err != nil {
		return nil, err
	}
	if size/int64(r.DurationS/r.IntervalS) < int64(m.StepBaseQuantums) {
		return nil, errors.New("TWAP slice is smaller than market step")
	}
	var price int64
	if r.Price != "" {
		price, err = PriceToSubticks(r.Price, m)
		if err != nil {
			return nil, err
		}
	}
	position, err := inferPosition(r.IsBuy, r.ReduceOnly, r.Position)
	if err != nil {
		return nil, err
	}
	side := Ask
	if r.IsBuy {
		side = Bid
	}
	return e.Execute(ctx, Action{Type: PlaceTwap, Account: e.account, Market: uint16(m.ID), Side: side, Size: size, Price: price, Position: position, IsCross: !r.Isolated, DurationS: r.DurationS, IntervalS: r.IntervalS, TolerancePPM: r.TolerancePPM}, opts...)
}

type CreateScaledOrderRequest struct {
	Coin                            string
	IsBuy                           bool
	Size, LowestPrice, HighestPrice string
	NumOrders, MagnitudePPM         uint32
	Profile, Spacing                string
	TIF                             TIF
	Position                        PositionSide
	ReduceOnly, Isolated            bool
	GoodTilBlock                    uint64
}

func (e *Exchange) ScaledOrder(ctx context.Context, r CreateScaledOrderRequest, opts ...SubmitOption) (*OrderResponse, error) {
	if r.NumOrders < 2 || r.NumOrders > 50 {
		return nil, errors.New("scaled order count must be 2..50")
	}
	if r.Profile == "" {
		r.Profile = "Equal"
	}
	if r.Spacing == "" {
		r.Spacing = "Linear"
	}
	if r.TIF == "" {
		r.TIF = Gtc
	}
	if r.TIF != Gtc && r.TIF != Ioc {
		return nil, errors.New("scaled TIF must be Gtc or Ioc")
	}
	switch r.Profile {
	case "Equal":
		if r.MagnitudePPM != 0 {
			return nil, errors.New("Equal magnitude must be zero")
		}
	case "IncRatio", "DecRatio":
		if r.MagnitudePPM == 0 || r.MagnitudePPM >= 1_000_000 {
			return nil, errors.New("scaled ratio magnitude outside range")
		}
	case "IncDiff", "DecDiff":
		if r.MagnitudePPM == 0 || r.MagnitudePPM >= 2_000_000/(r.NumOrders-1) {
			return nil, errors.New("scaled difference magnitude outside range")
		}
	default:
		return nil, errors.New("invalid scaled profile")
	}
	if r.Spacing != "Linear" && r.Spacing != "Geometric" {
		return nil, errors.New("invalid scaled spacing")
	}
	m, err := e.info.Market(ctx, r.Coin)
	if err != nil {
		return nil, err
	}
	if m.ID >= 65535 || m.Status != MarketActive {
		return nil, errors.New("market is not active or encodable")
	}
	size, err := SizeToQuantums(r.Size, m)
	if err != nil {
		return nil, err
	}
	low, err := PriceToSubticks(r.LowestPrice, m)
	if err != nil {
		return nil, err
	}
	high, err := PriceToSubticks(r.HighestPrice, m)
	if err != nil {
		return nil, err
	}
	if high <= low || high-low < int64(r.NumOrders-1) {
		return nil, errors.New("scaled price range too narrow")
	}
	position, err := inferPosition(r.IsBuy, r.ReduceOnly, r.Position)
	if err != nil {
		return nil, err
	}
	side := Ask
	if r.IsBuy {
		side = Bid
	}
	closing := (side == Ask && position == Long) || (side == Bid && position == Short)
	if r.ReduceOnly && !closing && r.TIF != Ioc {
		return nil, errors.New("opening-side reduce-only requires IOC")
	}
	return e.Execute(ctx, Action{Type: PlaceScaled, Account: e.account, Market: uint16(m.ID), Side: side, Size: size, TIF: r.TIF, Opts: &OrderOpts{Position: position, IsCross: !r.Isolated, ReduceOnly: r.ReduceOnly, GoodTilBlock: r.GoodTilBlock}, Params: &ScaledParams{LowestPrice: low, HighestPrice: high, NumOrders: r.NumOrders, Profile: r.Profile, Spacing: r.Spacing, MagnitudePPM: r.MagnitudePPM}}, opts...)
}
