# Market and Account Queries

English | [简体中文](../zh-CN/reference/INFO.md)

`Info` provides read-only queries. Create it with `NewInfo(endpoint, opts...)`; every method takes a context and returns data and an error.

## Markets: Meta / Market

**Use**: find markets and read their status, precision, mark price and public configuration.

```go
meta, err := info.Meta(ctx)
if err != nil {
    return err
}
market, err := info.Market(ctx, "BTC-USD")
if err != nil {
    return err
}
fmt.Println(market.ID, market.Status, market.StepBaseQuantums, market.SubticksPerTick)
fmt.Println(meta.FeeTiers, meta.Funding)
```

Meta returns Markets, LiquidityTiers, FeeTiers, Funding and At; Market looks up a single Market by full ticker and returns an error if not found. Before trading, confirm the market status is MarketActive.

## Order book: L2Snapshot

**Use**: get aggregated price levels to display the order book or compute a reference price for orders.

In `L2Snapshot(ctx, marketID, depth)`, depth is the number of levels per side, at most 1,000; 0 uses the node default of 50.

```go
market, err := info.Market(ctx, "BTC-USD")
if err != nil {
    return err
}
book, err := info.L2Snapshot(ctx, market.ID, 20)
if err != nil {
    return err
}
for _, level := range book.Bids {
    price, err := apollo.SubticksToPrice(int64(level.Price), market)
    if err != nil {
        return err
    }
    size, err := apollo.QuantumsToSize(int64(level.Size), market)
    if err != nil {
        return err
    }
    fmt.Println(price, size)
}
```

Returns Bids, Asks, Market and At. Bids are sorted from high to low price, asks from low to high; each level has Price and Size. An HTTP query returns a snapshot at that moment; applications can poll as needed.

## Market data: Ticker

**Use**: query the mark price, best bid/ask, latest price and market statistics.

```go
ticker, err := info.Ticker(ctx, marketID)
if err != nil {
    return err
}
fmt.Println(ticker.Mark, ticker.BestBid, ticker.BestAsk, ticker.FundingIndex)
```

Returns Mark, optional BestBid / BestAsk / LastPrice, trade statistics, OpenInterest, FundingIndex and more. The statistics window depends on how long the node has actually been running and observing; FundingIndex is the market's cumulative funding index. Optional prices are pointers; check for nil before displaying.

## Accounts: UserState / UserStateByID

**Use**: read account assets, risk and settings.

```go
owner, err := apollo.ParseAddress(ownerAddress)
if err != nil {
    return err
}
state, err := info.UserState(ctx, owner)
if err != nil {
    return err
}
fmt.Println(state.Account, state.Collateral, state.Equity, state.MarginHeadroom, state.BelowMaintenance)
```

`UserState(ctx, owner Address)` queries by owner; `UserStateByID(ctx, accountID uint32)` queries by account ID. Returns Account, Owner, Collateral, Equity, MarginHeadroom, BelowMaintenance, Positions, Settings and allowance / nonce information.

Asset amounts are in micro-USD. MarginHeadroom is the equity in excess of the initial margin requirement; subsequent trades must still pass on-chain risk checks. If the account does not exist, a node error is returned.

## Positions: Positions

**Use**: read long/short sizes, cost, funding index and isolated margin.

```go
positions, err := info.Positions(ctx, accountID)
if err != nil {
    return err
}
for _, position := range positions.Positions {
    fmt.Println(position.Market, position.Side, position.Size, position.IsolatedMargin)
}
```

Returns Positions and At. Sizes use the market's base-asset precision and isolated margin uses micro-USD; Position.Side distinguishes the long and short slots.

## Open orders: OpenOrders

**Use**: read orders currently resting in the order book, for display, canceling and reconciliation.

```go
orders, err := info.OpenOrders(ctx, accountID, &marketID)
if err != nil {
    return err
}
for _, order := range orders.Orders {
    fmt.Println(order.OrderID, order.Price, order.Remaining, order.Expiry)
}
```

Pass nil as market to query all markets, or &marketID to filter by a specific market; market 0 is a valid value.

Returns Orders and At; each entry has OrderID, Market, Side, Price, Remaining, Expiry and Position. Remaining is the remaining base-asset integer quantity; Expiry is the expiry height, 0 for none.

For orders no longer among open orders, verify the result with [Fills and history](HISTORY.md); absence alone does not prove the order was fully filled.

## Authorization, allowance and chain status

| Method | Key fields returned |
| --- | --- |
| `Agents(ctx, accountID)` | Agents, each with Slot, Key, Seq, Expiry |
| `Nonce(ctx, accountID)` | LastNonce, Window |
| `UserRateLimit(ctx, accountID)` | Used, Limit, Remaining |
| `ChainInfo(ctx)` | ChainID, GenesisHash, ExecutedHeight, etc. |

```go
agents, err := info.Agents(ctx, accountID)
if err != nil {
    return err
}
allowance, err := info.UserRateLimit(ctx, accountID)
if err != nil {
    return err
}
fmt.Println(agents.Agents, allowance.Remaining)
```

Authorization expiry is measured in block height; 0 means no expiry height. The cumulative action allowance is shared by the owner and all agents.
