package apollo

import "encoding/binary"

type Side string

const (
	Bid Side = "Bid"
	Ask Side = "Ask"
)

type TIF string

const (
	Gtc      TIF = "Gtc"
	Ioc      TIF = "Ioc"
	PostOnly TIF = "PostOnly"
)

type PositionSide string

const (
	Long  PositionSide = "Long"
	Short PositionSide = "Short"
)

type ActionType string

const (
	Place                ActionType = "Place"
	PlaceWith            ActionType = "PlaceWith"
	Cancel               ActionType = "Cancel"
	Modify               ActionType = "Modify"
	CancelAll            ActionType = "CancelAll"
	ApproveAgent         ActionType = "ApproveAgent"
	RevokeAgent          ActionType = "RevokeAgent"
	SetLeverage          ActionType = "SetLeverage"
	UpdateIsolatedMargin ActionType = "UpdateIsolatedMargin"
	PlaceTwap            ActionType = "PlaceTwap"
	PlaceScaled          ActionType = "PlaceScaled"
)

type ScaledParams struct {
	LowestPrice  int64  `json:"lowest_price,string"`
	HighestPrice int64  `json:"highest_price,string"`
	NumOrders    uint32 `json:"num_orders"`
	Profile      string `json:"profile"`
	Spacing      string `json:"spacing"`
	MagnitudePPM uint32 `json:"magnitude_ppm"`
}

func AgentMaySign(a Action) bool {
	return a.Type != ApproveAgent && a.Type != RevokeAgent && isTradingAction(a.Type)
}
func isTradingAction(a ActionType) bool {
	switch a {
	case Place, PlaceWith, Cancel, Modify, CancelAll, ApproveAgent, RevokeAgent, SetLeverage, UpdateIsolatedMargin, PlaceTwap, PlaceScaled:
		return true
	}
	return false
}
func createsOrder(a ActionType) bool {
	switch a {
	case Place, PlaceWith, Modify, PlaceTwap, PlaceScaled:
		return true
	}
	return false
}
func actionType(b [48]byte) ActionType {
	return map[byte]ActionType{3: Place, 4: Cancel, 5: Modify, 10: ApproveAgent, 11: RevokeAgent, 18: UpdateIsolatedMargin, 19: PlaceWith, 20: CancelAll, 21: SetLeverage, 22: PlaceTwap, 23: PlaceScaled}[b[0]]
}
func actionAccount(b [48]byte) uint32 { return binary.LittleEndian.Uint32(b[4:8]) }
