# 市场和账户查询

[English](../../reference/INFO.md) | 简体中文

`Info` 提供只读查询。使用 `NewInfo(endpoint, opts...)` 创建，所有方法接收 context，返回数据和 error。

## 市场：Meta / Market

**用途**：查找市场并读取状态、精度、mark 和公共配置。

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

Meta 返回 Markets、LiquidityTiers、FeeTiers、Funding 和 At；Market 按完整 ticker 查找一个 Market，找不到返回错误。交易前确认市场状态为 MarketActive。

## 盘口：L2Snapshot

**用途**：获取聚合价格档位，展示订单簿或计算下单参考价。

`L2Snapshot(ctx, marketID, depth)` 的 depth 是每侧档位数，最大 1,000，0 使用节点默认 50。

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

返回 Bids、Asks、Market、At。买盘价格从高到低，卖盘从低到高；每档含 Price 和 Size。HTTP 查询返回当次快照，应用可按需轮询。

## 行情：Ticker

**用途**：查询 mark、买卖一、最近价格及市场统计。

```go
ticker, err := info.Ticker(ctx, marketID)
if err != nil {
    return err
}
fmt.Println(ticker.Mark, ticker.BestBid, ticker.BestAsk, ticker.FundingIndex)
```

返回 Mark、可选 BestBid / BestAsk / LastPrice、成交统计、OpenInterest 和 FundingIndex 等。统计范围受节点实际运行及观察时间影响，FundingIndex 是市场累计资金费指数。可选价格使用指针，展示前检查是否为 nil。

## 账户：UserState / UserStateByID

**用途**：读取账户资产、风险和设置。

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

`UserState(ctx, owner Address)` 按 owner 查询；`UserStateByID(ctx, accountID uint32)` 按账户 ID 查询。返回 Account、Owner、Collateral、Equity、MarginHeadroom、BelowMaintenance、Positions、Settings 和额度 / nonce 信息。

资产金额以微 USD 计。MarginHeadroom 表示超过初始保证金需求的权益，后续交易仍需通过链上风控。账户不存在时返回节点错误。

## 持仓：Positions

**用途**：读取多空数量、成本、资金费指数和逐仓保证金。

```go
positions, err := info.Positions(ctx, accountID)
if err != nil {
    return err
}
for _, position := range positions.Positions {
    fmt.Println(position.Market, position.Side, position.Size, position.IsolatedMargin)
}
```

返回 Positions 和 At。数量使用市场基础币精度，逐仓保证金使用微 USD；按 Position.Side 区分多空槽位。

## 挂单：OpenOrders

**用途**：读取当前仍在订单簿中的委托，用于展示、撤单和对账。

```go
orders, err := info.OpenOrders(ctx, accountID, &marketID)
if err != nil {
    return err
}
for _, order := range orders.Orders {
    fmt.Println(order.OrderID, order.Price, order.Remaining, order.Expiry)
}
```

market 传 nil 查询全部市场，传 &marketID 筛选指定市场，市场 0 是有效值。

返回 Orders 和 At，每项含 OrderID、Market、Side、Price、Remaining、Expiry、Position。Remaining 是剩余基础币整数数量，Expiry 是截止高度，0 为不设置。

不在当前挂单中的委托，需要结合 [成交和历史](HISTORY.md) 核实结果；不能单独据此认定全部成交。

## 授权、额度和链状态

| 方法 | 返回重点 |
| --- | --- |
| `Agents(ctx, accountID)` | Agents，每项包含 Slot、Key、Seq、Expiry |
| `Nonce(ctx, accountID)` | LastNonce、Window |
| `UserRateLimit(ctx, accountID)` | Used、Limit、Remaining |
| `ChainInfo(ctx)` | ChainID、GenesisHash、ExecutedHeight 等 |

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

授权到期按区块高度计，0 表示无到期高度。累计动作额度由 owner 和所有 agent 共享。
