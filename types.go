package apollo

// At identifies the executed snapshot behind a unary response.
type At struct {
	ExecutedHeight Uint64 `json:"executedHeight"`
}
type IndexStatus struct {
	FirstHeight    Uint64 `json:"firstHeight"`
	IndexedHeight  Uint64 `json:"indexedHeight"`
	ExecutedHeight Uint64 `json:"executedHeight"`
}
type Cursor struct {
	Height Uint64 `json:"height"`
	Index  uint32 `json:"index"`
}
type ChainInfo struct {
	ChainID          string   `json:"chainId"`
	GenesisHash      Hash     `json:"genesisHash"`
	NodeName         string   `json:"nodeName"`
	Role             string   `json:"role"`
	NodeID           Uint64   `json:"nodeId"`
	Round            Uint64   `json:"round"`
	CommittedRound   Uint64   `json:"committedRound"`
	CommittedBlocks  Uint64   `json:"committedBlocks"`
	Epoch            Uint64   `json:"epoch"`
	Markets          uint32   `json:"markets"`
	LastCommitUnixMS Uint64   `json:"lastCommitUnixMs"`
	ExecutedHeight   Uint64   `json:"executedHeight"`
	BlockTimeMS      Uint64   `json:"blockTimeMs"`
	Services         []string `json:"services"`
}
type MarketStatus string

const (
	MarketInitializing    MarketStatus = "MARKET_STATUS_INITIALIZING"
	MarketActive          MarketStatus = "MARKET_STATUS_ACTIVE"
	MarketCancelOnly      MarketStatus = "MARKET_STATUS_CANCEL_ONLY"
	MarketFinalSettlement MarketStatus = "MARKET_STATUS_FINAL_SETTLEMENT"
	MarketClosed          MarketStatus = "MARKET_STATUS_CLOSED"
)

type Market struct {
	ID                        uint32       `json:"id"`
	Ticker                    string       `json:"ticker"`
	Status                    MarketStatus `json:"status"`
	PriceExponent             int32        `json:"priceExponent"`
	AtomicResolution          int32        `json:"atomicResolution"`
	StepBaseQuantums          Int64        `json:"stepBaseQuantums"`
	SubticksPerTick           Int64        `json:"subticksPerTick"`
	QuantumConversionExponent int32        `json:"quantumConversionExponent"`
	LiquidityTier             uint32       `json:"liquidityTier"`
	MinPriceChangePPM         uint32       `json:"minPriceChangePpm"`
	DefaultFundingPPM         int32        `json:"defaultFundingPpm"`
	Mark                      Int64        `json:"mark"`
	OpenInterest              Int64        `json:"openInterest"`
	FundingIndex              Int64        `json:"fundingIndex"`
}
type LiquidityTier struct {
	ID                     uint32 `json:"id"`
	InitialMarginPPM       uint32 `json:"initialMarginPpm"`
	MaintenanceFractionPPM uint32 `json:"maintenanceFractionPpm"`
	ImpactNotional         Uint64 `json:"impactNotional"`
	OpenInterestLowerCap   Uint64 `json:"openInterestLowerCap"`
	OpenInterestUpperCap   Uint64 `json:"openInterestUpperCap"`
}
type FeeTier struct {
	AbsoluteVolumeRequirement      Uint64 `json:"absoluteVolumeRequirement"`
	TotalVolumeShareRequirementPPM uint32 `json:"totalVolumeShareRequirementPpm"`
	MakerVolumeShareRequirementPPM uint32 `json:"makerVolumeShareRequirementPpm"`
	MakerFeePPM                    int32  `json:"makerFeePpm"`
	TakerFeePPM                    int32  `json:"takerFeePpm"`
}
type FundingConfig struct {
	SampleSeconds             uint32 `json:"sampleSeconds"`
	TickSeconds               uint32 `json:"tickSeconds"`
	FundingRateClampFactorPPM uint32 `json:"fundingRateClampFactorPpm"`
	PremiumVoteClampFactorPPM uint32 `json:"premiumVoteClampFactorPpm"`
	MinNumVotesPerSample      uint32 `json:"minNumVotesPerSample"`
}
type Meta struct {
	At             At              `json:"at"`
	Markets        []Market        `json:"markets"`
	LiquidityTiers []LiquidityTier `json:"liquidityTiers"`
	FeeTiers       []FeeTier       `json:"feeTiers"`
	Funding        *FundingConfig  `json:"funding"`
}
type Level struct {
	Price Int64 `json:"price"`
	Size  Int64 `json:"size"`
}
type L2Book struct {
	At     At      `json:"at"`
	Market uint32  `json:"market"`
	Bids   []Level `json:"bids"`
	Asks   []Level `json:"asks"`
}
type Ticker struct {
	At           At     `json:"at"`
	Market       uint32 `json:"market"`
	Mark         Int64  `json:"mark"`
	BestBid      *Int64 `json:"bestBid"`
	BestAsk      *Int64 `json:"bestAsk"`
	LastPrice    *Int64 `json:"lastPrice"`
	Volume24H    Int64  `json:"volume24h"`
	High24H      *Int64 `json:"high24h"`
	Low24H       *Int64 `json:"low24h"`
	Trades24H    Uint64 `json:"trades24h"`
	OpenInterest Int64  `json:"openInterest"`
	FundingIndex Int64  `json:"fundingIndex"`
}
type Position struct {
	Market         uint32 `json:"market"`
	Side           string `json:"side"`
	Size           Int64  `json:"size"`
	Entry          Int64  `json:"entry"`
	OpenBids       Int64  `json:"openBids"`
	OpenAsks       Int64  `json:"openAsks"`
	FundingIndex   Int64  `json:"fundingIndex"`
	IsolatedMargin Int64  `json:"isolatedMargin"`
}
type MarketSetting struct {
	Market   uint32 `json:"market"`
	IMFPPM   uint32 `json:"imfPpm"`
	Isolated bool   `json:"isolated"`
}
type UserState struct {
	At               At              `json:"at"`
	Account          Uint64          `json:"account"`
	Owner            Address         `json:"owner"`
	Collateral       Int64           `json:"collateral"`
	Equity           Int64           `json:"equity"`
	MarginHeadroom   Int64           `json:"marginHeadroom"`
	BelowMaintenance bool            `json:"belowMaintenance"`
	ActionsUsed      Uint64          `json:"actionsUsed"`
	VolumeUSD        Uint64          `json:"volumeUsd"`
	LastNonce        Uint64          `json:"lastNonce"`
	OpenOrders       uint32          `json:"openOrders"`
	Settings         []MarketSetting `json:"settings"`
	Positions        []Position      `json:"positions"`
}
type PositionsResult struct {
	At        At         `json:"at"`
	Positions []Position `json:"positions"`
}
type OpenOrder struct {
	OrderID   Uint64 `json:"orderId"`
	Market    uint32 `json:"market"`
	Side      string `json:"side"`
	Price     Int64  `json:"price"`
	Remaining Int64  `json:"remaining"`
	Expiry    Uint64 `json:"expiry"`
	Position  string `json:"position"`
}
type OpenOrdersResult struct {
	At     At          `json:"at"`
	Orders []OpenOrder `json:"orders"`
}
type Agent struct {
	Slot   uint32   `json:"slot"`
	Key    HexBytes `json:"key"`
	Seq    uint32   `json:"seq"`
	Expiry Uint64   `json:"expiry"`
}
type AgentsResult struct {
	At     At      `json:"at"`
	Agents []Agent `json:"agents"`
}
type NonceState struct {
	At        At       `json:"at"`
	LastNonce Uint64   `json:"lastNonce"`
	Window    []Uint64 `json:"window"`
}
type UserRateLimit struct {
	At        At     `json:"at"`
	Used      Uint64 `json:"used"`
	Limit     Uint64 `json:"limit"`
	Remaining Uint64 `json:"remaining"`
	VolumeUSD Uint64 `json:"volumeUsd"`
}
type ActionKey struct {
	Account Uint64 `json:"account"`
	Nonce   Uint64 `json:"nonce"`
}
type ActionState string

const (
	ActionNotFound  ActionState = "ACTION_STATE_NOT_FOUND"
	ActionExecuted  ActionState = "ACTION_STATE_EXECUTED"
	ActionNonceUsed ActionState = "ACTION_STATE_NONCE_USED"
)

type ActionStatus struct {
	Key     ActionKey   `json:"key"`
	State   ActionState `json:"state"`
	Height  Uint64      `json:"height"`
	OK      bool        `json:"ok"`
	Reject  string      `json:"reject"`
	OrderID *Uint64     `json:"orderId"`
	Fills   uint32      `json:"fills"`
}
type ActionStatuses struct {
	At       At             `json:"at"`
	Statuses []ActionStatus `json:"statuses"`
}
type Transaction struct {
	TransactionHash Hash     `json:"transactionHash"`
	Height          Uint64   `json:"height"`
	Index           uint32   `json:"index"`
	OK              bool     `json:"ok"`
	Reject          string   `json:"reject"`
	Action          HexBytes `json:"action"`
	Auth            HexBytes `json:"auth"`
}
type Fill struct {
	Height        Uint64 `json:"height"`
	Index         uint32 `json:"index"`
	Kind          string `json:"kind"`
	Market        uint32 `json:"market"`
	TakerSide     string `json:"takerSide"`
	TakerAccount  Uint64 `json:"takerAccount"`
	TakerOrder    Uint64 `json:"takerOrder"`
	MakerAccount  Uint64 `json:"makerAccount"`
	MakerOrder    Uint64 `json:"makerOrder"`
	Price         Int64  `json:"price"`
	Qty           Int64  `json:"qty"`
	TakerFee      Int64  `json:"takerFee"`
	MakerFee      Int64  `json:"makerFee"`
	TakerPosition string `json:"takerPosition"`
	MakerPosition string `json:"makerPosition"`
}
type UserFillsParams struct {
	Account    *Uint64 `json:"account,omitempty"`
	Market     *uint32 `json:"market,omitempty"`
	FromHeight Uint64  `json:"fromHeight,omitempty"`
	After      *Cursor `json:"after,omitempty"`
	Limit      uint32  `json:"limit,omitempty"`
}
type FillsResult struct {
	Index IndexStatus `json:"index"`
	Fills []Fill      `json:"fills"`
	Next  *Cursor     `json:"next"`
}
type HistoryParams struct {
	Account    Uint64  `json:"account"`
	FromHeight Uint64  `json:"fromHeight,omitempty"`
	After      *Cursor `json:"after,omitempty"`
	Limit      uint32  `json:"limit,omitempty"`
}
type ActionResult struct {
	Height  Uint64  `json:"height"`
	Account Uint64  `json:"account"`
	Nonce   Uint64  `json:"nonce"`
	OK      bool    `json:"ok"`
	Reject  string  `json:"reject"`
	OrderID *Uint64 `json:"orderId"`
	Fills   uint32  `json:"fills"`
}
type HistoricAction struct {
	Result ActionResult `json:"result"`
	Action HexBytes     `json:"action"`
	Kind   string       `json:"kind"`
	Index  uint32       `json:"index"`
}
type HistoryResult struct {
	Index   IndexStatus      `json:"index"`
	Actions []HistoricAction `json:"actions"`
	Next    *Cursor          `json:"next"`
}
type CandleInterval string

const (
	Candle1M  CandleInterval = "CANDLE_INTERVAL_1M"
	Candle5M  CandleInterval = "CANDLE_INTERVAL_5M"
	Candle15M CandleInterval = "CANDLE_INTERVAL_15M"
	Candle1H  CandleInterval = "CANDLE_INTERVAL_1H"
	Candle4H  CandleInterval = "CANDLE_INTERVAL_4H"
	Candle1D  CandleInterval = "CANDLE_INTERVAL_1D"
)

type CandlesParams struct {
	Market    uint32         `json:"market"`
	Interval  CandleInterval `json:"interval"`
	FromUnixS Uint64         `json:"fromUnixS,omitempty"`
	ToUnixS   Uint64         `json:"toUnixS,omitempty"`
	Limit     uint32         `json:"limit,omitempty"`
}
type Candle struct {
	StartUnixS Uint64 `json:"startUnixS"`
	Open       Int64  `json:"open"`
	High       Int64  `json:"high"`
	Low        Int64  `json:"low"`
	Close      Int64  `json:"close"`
	Volume     Int64  `json:"volume"`
	Trades     Uint64 `json:"trades"`
}
type CandlesResult struct {
	Index   IndexStatus `json:"index"`
	Candles []Candle    `json:"candles"`
}
type FundingHistoryParams struct {
	Market     uint32 `json:"market"`
	FromHeight Uint64 `json:"fromHeight,omitempty"`
	Limit      uint32 `json:"limit,omitempty"`
}
type FundingPoint struct {
	Height       Uint64 `json:"height"`
	FundingIndex Int64  `json:"fundingIndex"`
}
type FundingHistoryResult struct {
	Index  IndexStatus    `json:"index"`
	Points []FundingPoint `json:"points"`
}
