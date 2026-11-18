# Feature Overview

English | [简体中文](zh-CN/FEATURES.md)

The Apollo Go SDK provides query, trading, authorization and result-handling APIs for perpetuals trading.

| Need | API | Description |
| --- | --- | --- |
| Markets and precision | `Info.Meta`, `Info.Market` | Market status, tick / step, public fee tiers and funding configuration |
| Order book and market data | `Info.L2Snapshot`, `Info.Ticker` | Order book, best bid/ask and market statistics |
| Account assets and risk | `Info.UserState`, `Info.UserStateByID` | Margin, equity, risk status and account settings |
| Positions and open orders | `Info.Positions`, `Info.OpenOrders` | Long/short positions, isolated margin and current orders |
| Single and bulk orders | `Exchange.Order`, `Exchange.BulkOrders` | Limit, IOC, post-only and per-item results |
| Market open and close | `Exchange.MarketOpen`, `Exchange.MarketClose` | Slippage-protected trades with explicit side and size |
| Take profit / stop loss | `OrderType.Trigger` on `Exchange.Order` | Trigger market or trigger limit |
| Cancel orders | `Exchange.Cancel`, `Exchange.BulkCancel`, `Exchange.CancelAll` | Specific orders, one market or all markets |
| Single and bulk modify | `Exchange.ModifyOrder`, `Exchange.BulkModify` | Change price and size; handle execution results per item |
| Leverage and isolated margin | `Exchange.UpdateLeverage`, `Exchange.UpdateIsolatedMargin` | Account risk settings |
| TWAP and scaled orders | `Exchange.TwapOrder`, `Exchange.CancelTwap`, `Exchange.ScaledOrder` | Create schedules, cancel TWAP, distribute orders across prices |
| Agent approval | `Exchange.ApproveAgent`, `Exchange.RevokeAgent` | Owner approves and revokes trading keys |
| Fill and action history | `Info.UserFills`, `Info.HistoricalOrders`, `Info.ActionHistory` | Archive data with cursor pagination |
| Fees and fill rebates | `Info.UserFees`, `SummarizeFees` | Fill fees aggregated by account, role and market |
| Candles and funding index | `Info.CandlesSnapshot`, `Info.FundingHistory` | Query by time or height |
| Chain identity, authorization, nonce and allowance | `Info.ChainInfo`, `Info.Agents`, `Info.Nonce`, `Info.UserRateLimit` | Integration checks, concurrency and recovery |
| Prepare and execute | `Exchange.PrepareOrder`, `Exchange.SubmitPrepared` | Sign and broadcast separately |
| Original transaction lookup | `Info.ActionStatus`, `Info.Transaction` | Verify results by account / nonce or hash |

## Reading order

1. [Quick start](../README.md): installation, queries and your first trade.
2. [API overview](API.md): configuration, parameters and return types.
3. [Market and account queries](reference/INFO.md): markets, order book, risk, positions and open orders.
4. [Trading API](reference/EXCHANGE.md): placing, canceling and modifying orders, and authorization.
5. [History and fees](reference/HISTORY.md): fills, fees, candles and pagination.
6. [Results and errors](reference/RESULTS.md): acceptance, execution, fills and recovery.
7. [Integration guide](USAGE.md): precision, timeouts, concurrency and key configuration.
