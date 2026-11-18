# History and Fees

English | [简体中文](../zh-CN/reference/HISTORY.md)

History queries use the archive node's index and return the data height of each query. When storing results, applications should also keep the index status and cursor.

## Index range: IndexStatus

```go
index, err := info.IndexStatus(ctx)
if err != nil {
    return err
}
fmt.Println(index.FirstHeight, index.IndexedHeight, index.ExecutedHeight)
```

FirstHeight is the starting height of history, IndexedHeight is the indexed height, and ExecutedHeight is the node's executed height. When the index lags, recent trades may not be queryable yet; an empty list alone does not prove a trade never happened.

## Fills: UserFills

**Use**: read actual fills for an account or a market.

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

Parameters include optional Account, Market, FromHeight, After and Limit. Account is a *Uint64 and Market a *uint32; the pointers distinguish omission from a valid value of 0. Limit is at most 1,000; 0 means the default of 100.

Returns Fills, Index and an optional Next. Each fill provides Height, Index, Market, both sides' accounts and order IDs, Price, Qty, MakerFee, TakerFee and position side. Convert prices with PriceUnitsToPrice and sizes with QuantumsToSize.

## Fees and fill rebates: UserFees

**Use**: aggregate trading fees from fill records by account, role and market.

```go
page, err := info.UserFees(ctx, apollo.UserFeesParams{Account: accountID, Limit: 100})
if err != nil {
    return err
}
fmt.Println(page.Summary.FeesPaid, page.Summary.Rebates, page.Summary.NetFees)
fmt.Println(page.Summary.ByMarket, page.Index, page.Next)
```

Account is a required uint32; other filters are the same as the fills query. Returns Summary, Index and Next, aggregating one page per call.

| Summary field | Meaning |
| --- | --- |
| MakerFees / TakerFees | Signed fees for the corresponding role; negative values are fill rebates |
| FeesPaid / Rebates | Positive amounts paid and rebated |
| NetFees | Paid minus rebates, may be negative |
| FillCount | Number of fill records involving the account |
| MakerFillCount / TakerFillCount | Record count per role |
| ByMarket | Per-market summaries sorted by market ID |

Amounts are returned as decimal integer strings in micro-USD, accumulated exactly with big integers internally. The basis is each fill's MakerFee / TakerFee, with rebates counted as negative fees. When both sides of the same fill are this account, fees are counted on both sides but FillCount counts it once.

## Pagination

With the same filters, pass Next as After for the next page until Next is nil. Accumulate fees across pages with math/big:

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

The result covers the history range queryable on the node; record FirstHeight and IndexedHeight when displaying it. The chain and index may keep advancing during the scan, so record each page's own index status. Avoid accumulating the same page twice.

If you already have fill records, aggregate them directly without a new request:

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

## Order and account action history

`HistoricalOrders(ctx, HistoryParams)` returns order-related actions; `ActionHistory(ctx, HistoryParams)` returns account actions.

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

HistoryParams contains Account, FromHeight, After and Limit. The response includes Actions, Index and Next. Successful action execution and an order being fully filled are judged separately; verify with current open orders and fills.

## Candles: CandlesSnapshot

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

The times in the example only illustrate the parameters; in practice, use a time range for which the chain has data. Supported intervals are 1m, 5m, 15m, 1h, 4h and 1d; times are Unix seconds, and ToUnixS of 0 means the current time.

Each candle provides StartUnixS, Open, High, Low, Close, Volume and Trades. Convert OHLC with PriceUnitsToPrice and Volume with QuantumsToSize. Intervals without fills may return no candle.

## Market funding index: FundingHistory

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

Returns Points and Index; each point is a block height and the market's cumulative funding index, used to analyze how cumulative funding changes.
