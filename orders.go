package apollo

import "fmt"

type OrderType struct {
	Limit   *LimitOrderType
	Trigger *TriggerOrderType
}
type LimitOrderType struct{ TIF TIF }
type TriggerOrderType struct {
	TriggerPrice string
	IsMarket     bool
	TPSL         string
}

// CreateOrderRequest uses exact decimal strings. Coin must match ListMarkets.ticker.
// Position defaults from IsBuy/ReduceOnly; Isolated=false means cross margin.
type CreateOrderRequest struct {
	Coin           string
	IsBuy          bool
	Price          string
	Size           string
	ReduceOnly     bool
	OrderType      OrderType
	Position       PositionSide
	Isolated       bool
	GoodTilBlock   uint64
	Builder        *Builder
	ReferencePrice string
	Slippage       string
}
type CancelOrderRequest struct {
	Coin    string
	OrderID uint64
}
type ModifyOrderRequest struct {
	Coin    string
	OrderID uint64
	Price   string
	Size    string
}
type MarketOrderRequest struct {
	Coin           string
	IsBuy          bool
	Size           string
	Slippage       string
	ReferencePrice string
	Position       PositionSide
	Isolated       bool
	ReduceOnly     bool
}
type ClosePositionRequest struct {
	Coin           string
	Position       PositionSide
	Size           string
	Slippage       string
	ReferencePrice string
	Isolated       bool
}

func inferPosition(buy, reduce bool, explicit PositionSide) (PositionSide, error) {
	if explicit != "" {
		if explicit != Long && explicit != Short {
			return "", fmt.Errorf("invalid position")
		}
		return explicit, nil
	}
	if buy != reduce {
		return Long, nil
	}
	return Short, nil
}
func buildOrder(account uint32, r CreateOrderRequest, m Market) (Action, error) {
	if m.Status != MarketActive {
		return Action{}, fmt.Errorf("market is not active: %s", m.Status)
	}
	if m.ID >= 65535 {
		return Action{}, fmt.Errorf("market id cannot be encoded")
	}
	size, e := SizeToQuantums(r.Size, m)
	if e != nil {
		return Action{}, e
	}
	p, e := inferPosition(r.IsBuy, r.ReduceOnly, r.Position)
	if e != nil {
		return Action{}, e
	}
	side := Ask
	if r.IsBuy {
		side = Bid
	}
	tif := Gtc
	marketLike := false
	var trigger *Trigger
	if r.OrderType.Limit != nil && r.OrderType.Trigger != nil {
		return Action{}, fmt.Errorf("choose limit or trigger order type")
	}
	if r.OrderType.Limit != nil {
		tif = r.OrderType.Limit.TIF
		if tif == "" {
			tif = Gtc
		}
	}
	if t := r.OrderType.Trigger; t != nil {
		var kind string
		switch t.TPSL {
		case "tp":
			kind = "TakeProfit"
		case "sl":
			kind = "StopLoss"
		default:
			return Action{}, fmt.Errorf("trigger TPSL must be tp or sl")
		}
		value, err := PriceToSubticks(t.TriggerPrice, m)
		if err != nil {
			return Action{}, err
		}
		trigger = &Trigger{Kind: kind, Price: value}
		marketLike = t.IsMarket
		if marketLike {
			tif = Ioc
		}
	}
	if tif != Gtc && tif != Ioc && tif != PostOnly {
		return Action{}, fmt.Errorf("invalid TIF")
	}
	var price int64
	if marketLike {
		slip := r.Slippage
		if slip == "" {
			slip = "0.01"
		}
		price, e = marketProtection(r.ReferencePrice, r.IsBuy, slip, m)
	} else {
		price, e = PriceToSubticks(r.Price, m)
	}
	if e != nil {
		return Action{}, e
	}
	closing := (side == Ask && p == Long) || (side == Bid && p == Short)
	if r.ReduceOnly && !closing && tif != Ioc {
		return Action{}, fmt.Errorf("opening-side reduce-only requires IOC")
	}
	action := Action{Type: Place, Account: account, Market: uint16(m.ID), Side: side, Price: price, Size: size, TIF: tif, Position: p, IsCross: !r.Isolated}
	if r.ReduceOnly || r.GoodTilBlock != 0 || trigger != nil || r.Builder != nil {
		action.Type = PlaceWith
		action.Opts = &OrderOpts{ReduceOnly: r.ReduceOnly, GoodTilBlock: r.GoodTilBlock, Trigger: trigger, Builder: r.Builder, Position: p, IsCross: !r.Isolated}
	}
	return action, nil
}
