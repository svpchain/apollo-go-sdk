package apollo

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

type Builder struct {
	Account uint32 `json:"account"`
	FeePPM  uint32 `json:"fee_ppm"`
}
type Trigger struct {
	Kind  string `json:"kind"`
	Price int64  `json:"price,string"`
}
type OrderOpts struct {
	ReduceOnly   bool         `json:"reduce_only"`
	GoodTilBlock uint64       `json:"good_til_block,string"`
	Trigger      *Trigger     `json:"trigger"`
	Builder      *Builder     `json:"builder"`
	Position     PositionSide `json:"position"`
	IsCross      bool         `json:"is_cross"`
}

// Action describes a canonical Apollo action using chain integer units.
// EncodeAction validates layout; Exchange.Order also checks market tick and step.
type Action struct {
	Type         ActionType    `json:"type"`
	Account      uint32        `json:"account"`
	Market       uint16        `json:"market"`
	Side         Side          `json:"side"`
	Price        int64         `json:"price,string"`
	Size         int64         `json:"size,string"`
	TIF          TIF           `json:"tif"`
	Position     PositionSide  `json:"position"`
	IsCross      bool          `json:"is_cross"`
	OrderID      uint64        `json:"order_id,string"`
	Slot         uint8         `json:"slot"`
	Key          string        `json:"key"`
	Expiry       uint64        `json:"expiry,string"`
	Opts         *OrderOpts    `json:"opts"`
	IMFPPM       uint32        `json:"imf_ppm"`
	DurationS    uint32        `json:"duration_s"`
	IntervalS    uint32        `json:"interval_s"`
	TolerancePPM uint32        `json:"tolerance_ppm"`
	Params       *ScaledParams `json:"params,omitempty"`
	Amount       int64         `json:"amount,string"`
}

func Hex(b []byte) string { return "0x" + hex.EncodeToString(b) }
func position(s PositionSide) (byte, error) {
	switch s {
	case "Long":
		return 1, nil
	case "Short":
		return 2, nil
	}
	return 0, fmt.Errorf("position must be Long or Short")
}
func bit(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func EncodeAction(a Action) ([48]byte, error) {
	if a.Account == 0 {
		return [48]byte{}, fmt.Errorf("account 0 is reserved")
	}
	var b [48]byte
	tags := map[ActionType]byte{"Place": 3, "Cancel": 4, "Modify": 5, "ApproveAgent": 10, "RevokeAgent": 11, "PlaceWith": 19, "CancelAll": 20, "SetLeverage": 21, "UpdateIsolatedMargin": 18, "PlaceTwap": 22, "PlaceScaled": 23}
	if a.Type == PlaceTwap && a.TIF != "" && a.TIF != Gtc {
		return b, fmt.Errorf("TWAP has no configurable TIF byte")
	}
	tag, ok := tags[a.Type]
	if !ok {
		return b, fmt.Errorf("unsupported action %q", a.Type)
	}
	b[0] = tag
	binary.LittleEndian.PutUint32(b[4:8], a.Account)
	if a.Type != "ApproveAgent" && a.Type != "RevokeAgent" {
		binary.LittleEndian.PutUint16(b[8:10], a.Market)
	}
	if a.Type == "Place" || a.Type == "PlaceWith" || a.Type == "PlaceTwap" || a.Type == "PlaceScaled" {
		switch a.Side {
		case "Bid":
			b[1] = 0
		case "Ask":
			b[1] = 1
		default:
			return b, fmt.Errorf("side must be Bid or Ask")
		}
		switch a.TIF {
		case "":
			if a.Type != "PlaceTwap" {
				return b, fmt.Errorf("missing tif")
			}
		case "Gtc":
			b[2] = 0
		case "Ioc":
			b[2] = 1
		case "PostOnly":
			b[2] = 2
		default:
			return b, fmt.Errorf("invalid TIF")
		}
	}
	if a.Type == "Place" || a.Type == "PlaceTwap" {
		p, err := position(a.Position)
		if err != nil {
			return b, err
		}
		b[3] = p | bit(a.IsCross)<<2
	}
	if a.Type == "Place" || a.Type == "PlaceWith" || a.Type == "Modify" || a.Type == "PlaceTwap" {
		binary.LittleEndian.PutUint64(b[32:40], uint64(a.Price))
		binary.LittleEndian.PutUint64(b[40:48], uint64(a.Size))
	}
	if a.Type == "Cancel" || a.Type == "Modify" {
		binary.LittleEndian.PutUint64(b[16:24], a.OrderID)
	}
	if a.Type == "ApproveAgent" || a.Type == "RevokeAgent" {
		if a.Slot < 1 || a.Slot > 4 {
			return b, fmt.Errorf("agent slot must be 1..4")
		}
		b[3] = a.Slot
		if a.Type == "ApproveAgent" {
			key, err := hex.DecodeString(strings.TrimPrefix(a.Key, "0x"))
			if err != nil || len(key) != 32 {
				return b, fmt.Errorf("agent key must be 32-byte hex")
			}
			copy(b[16:48], key)
			binary.LittleEndian.PutUint64(b[8:16], a.Expiry)
		}
	}
	if a.Type == "PlaceWith" {
		if a.Opts == nil {
			return b, fmt.Errorf("missing order options")
		}
		o := a.Opts
		p, err := position(o.Position)
		if err != nil {
			return b, err
		}
		b[3] = bit(o.ReduceOnly) | p<<3 | bit(o.IsCross)<<5
		binary.LittleEndian.PutUint64(b[16:24], o.GoodTilBlock)
		if o.Trigger != nil {
			b[3] |= 2
			switch o.Trigger.Kind {
			case "StopLoss":
			case "TakeProfit":
				b[3] |= 4
			default:
				return b, fmt.Errorf("invalid trigger kind")
			}
			binary.LittleEndian.PutUint64(b[24:32], uint64(o.Trigger.Price))
		}
		if o.Builder != nil {
			if o.Builder.FeePPM < 1 || o.Builder.FeePPM > 10000 {
				return b, fmt.Errorf("builder fee must be 1..10000 ppm")
			}
			binary.LittleEndian.PutUint32(b[10:14], o.Builder.Account)
			binary.LittleEndian.PutUint16(b[14:16], uint16(o.Builder.FeePPM))
		}
	}
	if a.Type == "PlaceTwap" {
		binary.LittleEndian.PutUint32(b[16:20], a.DurationS)
		binary.LittleEndian.PutUint32(b[20:24], a.IntervalS)
		binary.LittleEndian.PutUint32(b[24:28], a.TolerancePPM)
	}
	if a.Type == "PlaceScaled" {
		if a.Opts == nil || a.Params == nil {
			return b, fmt.Errorf("scaled order requires opts and params")
		}
		o, p := a.Opts, a.Params
		if o.Trigger != nil || o.Builder != nil {
			return b, fmt.Errorf("scaled options cannot encode trigger or builder")
		}
		pos, err := position(o.Position)
		if err != nil {
			return b, err
		}
		profiles := map[string]byte{"Equal": 0, "IncRatio": 1, "IncDiff": 2, "DecRatio": 3, "DecDiff": 4}
		profile, ok := profiles[p.Profile]
		if !ok {
			return b, fmt.Errorf("unknown scaled profile")
		}
		spacing, ok := map[string]byte{"Linear": 0, "Geometric": 1}[p.Spacing]
		if !ok {
			return b, fmt.Errorf("unknown scaled spacing")
		}
		if p.NumOrders < 2 || p.NumOrders > 50 {
			return b, fmt.Errorf("scaled orders require 2..50 children")
		}
		b[3] = profile | spacing<<3 | bit(o.ReduceOnly)<<4 | pos<<5 | bit(o.IsCross)<<7
		b[10] = uint8(p.NumOrders)
		binary.LittleEndian.PutUint32(b[12:16], p.MagnitudePPM)
		binary.LittleEndian.PutUint64(b[16:24], o.GoodTilBlock)
		binary.LittleEndian.PutUint64(b[24:32], uint64(p.LowestPrice))
		binary.LittleEndian.PutUint64(b[32:40], uint64(p.HighestPrice))
		binary.LittleEndian.PutUint64(b[40:48], uint64(a.Size))
	}
	if a.Type == "SetLeverage" {
		binary.LittleEndian.PutUint32(b[16:20], a.IMFPPM)
	}
	if a.Type == "UpdateIsolatedMargin" {
		p, err := position(a.Position)
		if err != nil {
			return b, err
		}
		b[3] = p
		binary.LittleEndian.PutUint64(b[40:48], uint64(a.Amount))
	}
	return b, nil
}
