# 历史与费用

[English](../../reference/HISTORY.md) | 简体中文

历史查询使用 archive 节点的索引，返回当次查询的数据高度。应用保存结果时应同时保留索引状态和游标。

## 索引范围：IndexStatus

```go
index, err := info.IndexStatus(ctx)
if err != nil {
    return err
}
fmt.Println(index.FirstHeight, index.IndexedHeight, index.ExecutedHeight)
```

FirstHeight 是历史起始高度，IndexedHeight 是已索引高度，ExecutedHeight 是节点执行高度。索引落后时最近交易可能尚未可查；空列表不能单独证明交易从未发生。

## 成交：UserFills

**用途**：读取某账户或某市场的实际成交。

```go
account := apollo.Uint64(accountID)
page, err := info.UserFills(ctx, apollo.UserFillsParams{
    Account: &account, Market: &marketID, Limit: 100,
})
if err != nil {
    return err
}
fmt.Println(page.Fills, page.Index, page.Next)
```

参数包括可选 Account、Market、FromHeight、After 和 Limit。Account 使用 *Uint64，Market 使用 *uint32，指针区分省略与有效值 0。Limit 最多 1,000，0 为默认 100。

返回 Fills、Index 和可选 Next。每项成交提供 Height、Index、Market、双方账户及订单 ID、Price、Qty、MakerFee、TakerFee 和仓位方向。价格使用 PriceUnitsToPrice，数量使用 QuantumsToSize 转换。

## 手续费和成交返佣：UserFees

**用途**：按账户、角色和市场汇总成交记录中的交易手续费。

```go
page, err := info.UserFees(ctx, apollo.UserFeesParams{Account: accountID, Limit: 100})
if err != nil {
    return err
}
fmt.Println(page.Summary.FeesPaid, page.Summary.Rebates, page.Summary.NetFees)
fmt.Println(page.Summary.ByMarket, page.Index, page.Next)
```

Account 为必填 uint32，其余过滤条件与成交查询一致。返回 Summary、Index 和 Next，每次汇总一页。

| Summary 字段 | 含义 |
| --- | --- |
| MakerFees / TakerFees | 对应角色的有符号费用，负数为成交返佣 |
| FeesPaid / Rebates | 支付与返佣的正数金额 |
| NetFees | 支付减返佣，可为负 |
| FillCount | 涉及账户的成交记录数 |
| MakerFillCount / TakerFillCount | 各角色的记录数 |
| ByMarket | 按市场 ID 排序的分市场汇总 |

金额以微 USD 的十进制整数字符串返回，内部使用大整数精确累加。统计口径为每条成交的 MakerFee / TakerFee，返佣按负手续费计。同条成交两侧均为本账户时，费用同时计入两侧，FillCount 仅计一次。

## 继续分页

用同样的过滤条件将 Next 传入下一页 After，直至 Next 为 nil。跨页累计费用使用 math/big：

```go
var after *apollo.Cursor
netFees := new(big.Int)
for {
    page, err := info.UserFees(ctx, apollo.UserFeesParams{
        Account: accountID, FromHeight: 0, After: after, Limit: 1000,
    })
    if err != nil {
        return err
    }
    amount, ok := new(big.Int).SetString(page.Summary.NetFees, 10)
    if !ok {
        return fmt.Errorf("invalid fee amount")
    }
    netFees.Add(netFees, amount)
    after = page.Next
    if after == nil {
        break
    }
}
fmt.Println(netFees.String())
```

结果覆盖节点可查询的历史范围，展示时记录 FirstHeight 和 IndexedHeight。扫描期间链和索引可能继续前进，各页记录各自的索引状态。避免重复累加同一页。

已取得成交记录时可直接汇总，不发新请求：

```go
account := apollo.Uint64(accountID)
page, err := info.UserFills(ctx, apollo.UserFillsParams{Account: &account, Limit: 100})
if err != nil {
    return err
}
summary, err := apollo.SummarizeFees(accountID, page.Fills)
if err != nil {
    return err
}
fmt.Println(summary.NetFees, summary.ByMarket)
```

## 订单和账户动作历史

`HistoricalOrders(ctx, HistoryParams)` 返回订单相关动作；`ActionHistory(ctx, HistoryParams)` 返回账户动作。

```go
page, err := info.HistoricalOrders(ctx, apollo.HistoryParams{
    Account: apollo.Uint64(accountID), Limit: 100,
})
if err != nil {
    return err
}
for _, action := range page.Actions {
    fmt.Println(action.Kind, action.Result.OK, action.Result.Reject)
}
fmt.Println(page.Next)
```

HistoryParams 包含 Account、FromHeight、After 和 Limit。响应包括 Actions、Index 和 Next。动作执行成功与订单全部成交分开判断，结合当前挂单及成交核查。

## K 线：CandlesSnapshot

```go
candles, err := info.CandlesSnapshot(ctx, apollo.CandlesParams{
    Market: marketID, Interval: apollo.Candle1M,
    FromUnixS: 1700000000, ToUnixS: 1700003600, Limit: 100,
})
if err != nil {
    return err
}
fmt.Println(candles.Candles, candles.Index)
```

示例时间仅演示参数，实际使用链有数据的时间范围。支持 1m、5m、15m、1h、4h、1d，时间为 Unix 秒，ToUnixS 为 0 表示当前时间。

每根 K 线提供 StartUnixS、Open、High、Low、Close、Volume、Trades。OHLC 使用 PriceUnitsToPrice，Volume 使用 QuantumsToSize。无成交区间可能不返回 K 线。

## 市场资金费指数：FundingHistory

```go
funding, err := info.FundingHistory(ctx, apollo.FundingHistoryParams{
    Market: marketID, FromHeight: 0, Limit: 100,
})
if err != nil {
    return err
}
for _, point := range funding.Points {
    fmt.Println(point.Height, point.FundingIndex)
}
```

返回 Points 和 Index，每项是区块高度及市场累计资金费指数，用于分析资金费累计变化。
