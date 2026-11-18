# API Reference

English | [简体中文](zh-CN/API.md)

`Info` provides read-only queries and `Exchange` provides signed trading. Every method takes `context.Context` as its first parameter and returns data plus an `error`.

Browse by task: [Feature overview](FEATURES.md), [Queries](reference/INFO.md), [Trading](reference/EXCHANGE.md), [History and fees](reference/HISTORY.md) and [Results](reference/RESULTS.md).

## Initialization and configuration

```go
info, err := apollo.NewInfo(endpoint, apollo.WithTimeout(10*time.Second))
if err != nil {
    return err
}
exchange, err := apollo.NewExchange(endpoint, signer,
    apollo.WithAccount(accountID),
    apollo.WithExpectedGenesisHash(trustedGenesis),
)
if err != nil {
    return err
}
fmt.Println(info, exchange.Account())
```

| Option | Default | Description |
| --- | --- | --- |
| `WithAccount(id)` | Required | Non-vault account ID for Exchange, non-zero uint32 |
| `WithExpectedGenesisHash(hash)` | Pinned on first connection | Trusted genesis hash of the target chain |
| `WithHTTPClient(client)` | SDK HTTP client | Configure a proxy, certificates or a custom Transport |
| `WithTimeout(duration)` | 10 seconds | Per-HTTP-request timeout |
| `WithSizeLimits(request, response)` | 4 MiB / 16 MiB | Request and response size limits; server-side limits still apply |
| `WithNonceManager(manager)` | Per-instance allocator | Share nonces across instances on the same network and account |

Construction makes no network requests. The HTTP client rejects redirects; custom clients passed in follow the same rule.

## Info

| Method | Parameters | Returns |
| --- | --- | --- |
| `ChainInfo` | ctx | `*ChainInfo`, chain identity and height |
| `Meta` / `Market` | ctx / ctx, coin | `*Meta` / `Market` |
| `L2Snapshot` | ctx, market uint32, depth uint32 | `*L2Book` |
| `Ticker` | ctx, market uint32 | `*Ticker` |
| `UserState` / `UserStateByID` | ctx, owner Address / account uint32 | `*UserState` |
| `Positions` | ctx, account uint32 | `*PositionsResult` |
| `OpenOrders` | ctx, account uint32, market *uint32 | `*OpenOrdersResult` |
| `Agents` / `Nonce` / `UserRateLimit` | ctx, account uint32 | Authorizations, nonce / window, allowance |
| `ActionStatus` | ctx, []ActionKey | `*ActionStatuses`, at most 10,000 keys |
| `Transaction` | ctx, hash Hash | `*Transaction` |
| `IndexStatus` | ctx | `*IndexStatus` |
| `UserFills` | ctx, UserFillsParams | `*FillsResult` |
| `UserFees` | ctx, UserFeesParams | `*UserFeesResult`, fee summary for the current page |
| `HistoricalOrders` / `ActionHistory` | ctx, HistoryParams | `*HistoryResult` |
| `CandlesSnapshot` | ctx, CandlesParams | `*CandlesResult` |
| `FundingHistory` | ctx, FundingHistoryParams | `*FundingHistoryResult` |

For `OpenOrders`, passing nil as market means all markets; passing a pointer selects a specific market. `HistoricalOrders` returns order-related action records; verify actual fills together with UserFills and current open orders.

## Order parameters

`Order(ctx, CreateOrderRequest, opts...)` / `BulkOrders(ctx, []CreateOrderRequest, opts...)` return `*OrderResponse`.

| Field | Type | Description |
| --- | --- | --- |
| `Coin` | string | Full market ticker, e.g. BTC-USD |
| `IsBuy` | bool | true to buy, false to sell |
| `Price` / `Size` | string | Decimal price and base-asset size, matching tick / step |
| `OrderType.Limit.TIF` | Gtc / Ioc / PostOnly | Time in force for regular orders, default Gtc |
| `OrderType.Trigger` | *TriggerOrderType | Mutually exclusive with Limit |
| `ReduceOnly` | bool | Default false; reduce-only |
| `Position` | Long / Short | By default derived from side and ReduceOnly |
| `Isolated` | bool | Default false for cross margin, true for isolated margin |
| `GoodTilBlock` | uint64 | Expiry height, 0 for none |
| `Builder` | *Builder | Account and FeePPM, fee rate 1–10,000 ppm |
| `ReferencePrice` / `Slippage` | string | Reference price and protection ratio for market trigger orders; slippage defaults to 0.01 |

`TriggerOrderType` contains `TriggerPrice string`, `IsMarket bool` and `TPSL string`. TPSL is tp / sl; trigger limit orders also provide Price.

## Other trading

| Method | Request |
| --- | --- |
| `Cancel` / `BulkCancel` | CancelOrderRequest / []CancelOrderRequest, with Coin, OrderID uint64 |
| `CancelAll` | coin string; empty string means all markets |
| `ModifyOrder` / `BulkModify` | ModifyOrderRequest / []ModifyOrderRequest, with Coin, OrderID, Price, Size |
| `CancelTwap` | CancelOrderRequest; OrderID is the parent schedule ID |
| `MarketOpen` | MarketOrderRequest, with Coin, IsBuy, Size; optional Position, Isolated, ReduceOnly, ReferencePrice, Slippage |
| `MarketClose` | ClosePositionRequest, required Coin, Position, Size; optional Isolated, ReferencePrice, Slippage |
| `UpdateLeverage` | coin, leverage string |
| `UpdateIsolatedMargin` | coin, position, amount string; amount in USD, positive adds, negative removes |
| `ApproveAgent` / `RevokeAgent` | slot, public key bytes, expiry / slot; owner operations |

Batches accept at most 20,000 items and are bound by the request byte limit. Requests execute item by item; a batch is not atomic. Modify cancels the old order first, then creates the new one; if the new order fails, the old one may already be canceled.

## Advanced orders

`TwapOrder(ctx, CreateTwapOrderRequest, opts...)`:

- Required: Coin, IsBuy, Size, DurationS, IntervalS.
- Optional: Price, TolerancePPM, Position, ReduceOnly, Isolated.
- DurationS is 300–86,400 seconds; IntervalS is 30–3,600 seconds, and the duration must be an integer multiple of the interval.
- Each leg's size must be at least one market step; TolerancePPM is less than 1,000,000, and zero means zero tolerance.
- When Price is omitted, the chain derives the child orders' protective price from the oracle and TolerancePPM. For 1%, pass 10,000 explicitly.

`ScaledOrder(ctx, CreateScaledOrderRequest, opts...)`:

- Required: Coin, IsBuy, Size, LowestPrice, HighestPrice, NumOrders.
- NumOrders is 2–50; Size is the total size.
- Profile is Equal (default), IncRatio, IncDiff, DecRatio or DecDiff.
- Spacing is Linear (default) or Geometric; TIF is Gtc (default) or Ioc.
- MagnitudePPM is the distribution parameter; Equal uses 0, other profiles follow their respective parameter constraints.
- Optional: Position, ReduceOnly, Isolated, GoodTilBlock.

## History parameters

| Type | Fields |
| --- | --- |
| `UserFillsParams` | Account *Uint64, Market *uint32, FromHeight Uint64, After *Cursor, Limit uint32 |
| `UserFeesParams` | Account uint32 required; other filters same as the fills query |
| `HistoryParams` | Account Uint64, FromHeight Uint64, After *Cursor, Limit uint32 |
| `CandlesParams` | Market uint32, Interval CandleInterval, FromUnixS / ToUnixS Uint64, Limit uint32 |
| `FundingHistoryParams` | Market uint32, FromHeight Uint64, Limit uint32 |

History Limit defaults to 100, with a server maximum of 1,000. Cursor contains Height Uint64 and Index uint32; continue with the response's Next, keeping the original filters.

## Fee summary

`Info.UserFees()` returns one page of `{Summary, Index, Next}`; `SummarizeFees(account, fills)` aggregates an existing `[]Fill` without making a request.

| Summary field | Type | Meaning |
| --- | --- | --- |
| Account | uint32 | Account |
| MakerFees / TakerFees | string | Signed fees per role, micro-USD |
| FeesPaid / Rebates | string | Positive amounts paid / rebated, micro-USD |
| NetFees | string | Paid minus rebates, micro-USD, may be negative |
| FillCount | uint64 | Number of fill records involving this account |
| MakerFillCount / TakerFillCount | uint64 | Record count per role |
| ByMarket | []MarketFeeSummary | Sorted by market ID; each entry has Market and fees / counts |

The basis is each fill's MakerFee / TakerFee; when Next is present, keep paginating to get the total for the queried range. Exact aggregation uses big integers and returns decimal integer strings; when both sides of the same record are this account, fees on both sides are counted, but FillCount increases by only one.

## Prepare and submit

| Method | Purpose |
| --- | --- |
| `PrepareOrder` | Check and sign an order without broadcasting; returns PreparedAction |
| `Prepare` / `PrepareActions` | Prepare Actions, using the integer units declared by each type |
| `Execute` / `ExecuteActions` | Prepare and submit actions |
| `SubmitPrepared` | Verify prepared transactions, submit them and return per-item results |

`PreparedAction` exposes Account, Nonce, GenesisHash and TransactionHash for reconciliation, and holds the signed transaction for submission; it contains no private key.

All trading methods accept `NoWait()`, `WaitTimeout(duration)` and `PollInterval(duration)`. By default the execution wait is 15 seconds with 250 ms polling; the per-HTTP-request timeout is controlled separately by WithTimeout. Canceling the context stops IO and local waiting, but accepted transactions may still execute.

For return statuses and error types, see [Results](reference/RESULTS.md).

## Signing and extensions

Use `NewOwnerSigner(privateKey)` for a local owner, `NewWalletSigner(ownerAddress, callback)` for a wallet / HSM, and `NewAgentSigner(seed, slot)` for an agent.

The wallet callback receives TypedData and returns a 65-byte signature. Custom signers implement the Signer interface; concurrent allocators implement the NonceManager interface. For integration constraints, see [USAGE](USAGE.md).

`Info.Client().Call(ctx, method, params, result)` can call other unary queries. In extension structs, use Uint64 / Int64 for 64-bit fields, Address for addresses and HexBytes for bytes.

## Display and unit conversion

- Order book and open order prices: `SubticksToPrice(value, market)`.
- Fill, mark / oracle and candle prices: `PriceUnitsToPrice(value, market)`.
- Base-asset size: `QuantumsToSize(value, market)`.
- User input to integers: `PriceToSubticks`, `SizeToQuantums`, `USDToMicroUSD`.
- Leverage conversion: `LeverageToIMF`.

Conversion functions return a result and an error. High-level decimal inputs use plain numeric strings; scientific notation, NaN and Infinity are not accepted.
