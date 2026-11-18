# Apollo Go SDK

English | [简体中文](README.zh-CN.md)

The Go trading SDK for Apollo. Over HTTP JSON-RPC it provides market and account queries, placing, canceling and modifying orders, signing and authorization, and execution tracking. Prices, sizes and margin are decimal strings.

The package name is `apollo`, the Go module is `github.com/svpchain/apollo-go-sdk`, and it requires **Go 1.22+**.

| Client | Common methods |
| --- | --- |
| `Info` | `Meta`, `L2Snapshot`, `UserState`, `OpenOrders`, `UserFills`, `UserFees` |
| `Exchange` | `Order`, `BulkOrders`, `BulkModify`, `CancelTwap`, `MarketOpen`, `MarketClose` |

For the full API catalog, see the [Feature overview](docs/FEATURES.md).

## Installation

```bash
go get github.com/svpchain/apollo-go-sdk@latest
```

`@latest` fetches the latest stable release and records the exact version in your project's `go.mod`. To pin a version, specify it explicitly, for example `go get github.com/svpchain/apollo-go-sdk@v1.0.0`. Changes in each release are listed in the [Changelog](CHANGELOG.md).

`endpoint` is the full Apollo HTTP(S) JSON-RPC URL provided by the target environment; paths and query parameters are supported.

## Query markets and accounts

```go
import (
    "context"
    "fmt"

    apollo "github.com/svpchain/apollo-go-sdk"
)

ctx := context.Background()
info, err := apollo.NewInfo(endpoint)
if err != nil {
    return err
}
meta, err := info.Meta(ctx)
if err != nil {
    return err
}
for _, market := range meta.Markets {
    fmt.Println(market.ID, market.Ticker, market.Status)
}
market, err := info.Market(ctx, "BTC-USD")
if err != nil {
    return err
}
book, err := info.L2Snapshot(ctx, market.ID, 20)
if err != nil {
    return err
}
fmt.Println(book.Bids, book.Asks)
```

Queries do not require signing. `UserState(ctx, owner)` queries by owner address and `UserStateByID(ctx, account)` queries by numeric account ID; `owner` can be parsed with `ParseAddress()`.

`OpenOrders(ctx, account, nil)` queries all markets. Pass `&marketID` to filter by a single market; market 0 is a valid ID.

## Place orders with an approved agent

`seed` is a 32-byte Ed25519 seed whose public key must first be approved by the account owner. Trading accounts use their on-chain numeric ID, and `Coin` uses the market's full ticker.

```go
signer, err := apollo.NewAgentSigner(seed, 1)
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
response, err := exchange.Order(ctx, apollo.CreateOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Price: "65000", Size: "0.001",
    OrderType: apollo.OrderType{
        Limit: &apollo.LimitOrderType{TIF: apollo.Gtc},
    },
})
if err != nil {
    return err
}
for _, item := range response.Statuses {
    fmt.Println(item.Prepared.TransactionHash, item.SubmitStatus, item.Outcome, item.Error)
}
```

`NewExchange` makes no network requests. Trading methods check market rules, chain identity, signing authorization, action allowance and nonce. On first connection the SDK pins the target chain's genesis hash and re-verifies it on every subsequent sign and submit.

By default it waits up to 15 seconds for execution. `response.Successful()` means all actions executed successfully; to see actual order fills, keep querying open orders and fills.

## Common trades

Each call below performs the corresponding operation. Pick the ones your integration needs, and check the returned `err` and every per-item status.

```go
response, err := exchange.MarketOpen(ctx, apollo.MarketOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Size: "0.001", Slippage: "0.01",
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)

response, err = exchange.Cancel(ctx, apollo.CancelOrderRequest{
    Coin: "BTC-USD", OrderID: orderID,
})
if err != nil {
    return err
}
response, err = exchange.ModifyOrder(ctx, apollo.ModifyOrderRequest{
    Coin: "BTC-USD", OrderID: orderID, Price: "65100", Size: "0.001",
})
if err != nil {
    return err
}
response, err = exchange.MarketClose(ctx, apollo.ClosePositionRequest{
    Coin: "BTC-USD", Position: apollo.Long, Size: "0.001", Slippage: "0.01",
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

Market intents use an IOC protective limit price, with a default slippage of 1%. Closing a position specifies the long/short slot and the size; `Isolated: true` selects isolated margin, and cross margin is the default.

Modifying an order cancels the old order first, then creates the new one; if the new order is rejected, the old one may already be canceled. Batches execute item by item, so handle results item by item.

## Bulk modify, TWAP cancel and fees

```go
response, err := exchange.BulkModify(ctx, []apollo.ModifyOrderRequest{
    {Coin: "BTC-USD", OrderID: firstOrderID, Price: "65100", Size: "0.001"},
    {Coin: "BTC-USD", OrderID: secondOrderID, Price: "65200", Size: "0.002"},
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
response, err = exchange.CancelTwap(ctx, apollo.CancelOrderRequest{
    Coin: "BTC-USD", OrderID: twapParentID,
})
if err != nil {
    return err
}
page, err := info.UserFees(ctx, apollo.UserFeesParams{Account: accountID, Limit: 100})
if err != nil {
    return err
}
fmt.Println(page.Summary.FeesPaid, page.Summary.Rebates, page.Summary.NetFees, page.Next)
```

TWAP cancel takes the parent order ID and cancels both the schedule and the current child order. Fees are aggregated from MakerFee / TakerFee on fills; amounts are decimal integer strings in micro-USD, so totals beyond int64 are supported. `UserFees()` returns the summary for the current page; when `Next` is present, keep paginating. For fills you already have, call `SummarizeFees()` to aggregate them directly.

## Owner wallet and agent approval

```go
ownerSigner, err := apollo.NewOwnerSigner(ownerPrivateKey)
if err != nil {
    return err
}
owner, err := apollo.NewExchange(endpoint, ownerSigner,
    apollo.WithAccount(accountID),
    apollo.WithExpectedGenesisHash(trustedGenesis),
)
if err != nil {
    return err
}
publicKey := signer.Identity().PublicKey
approval, err := owner.ApproveAgent(ctx, 1, publicKey[:], 0)
if err != nil {
    return err
}
if !approval.Successful() {
    return fmt.Errorf("agent approval not confirmed")
}
```

Authorization expiry is a block height; 0 means no expiry height. `RevokeAgent(ctx, slot)` revokes the authorization. For wallet or HSM integrations, use `NewWalletSigner(ownerAddress, callback)` and let the callback do the signing. The SDK verifies the returned signature against the on-chain owner.

## Prepare without broadcasting

```go
prepared, err := exchange.PrepareOrder(ctx, apollo.CreateOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Price: "65000", Size: "0.001",
})
if err != nil {
    return err
}
fmt.Println(prepared.TransactionHash, prepared.Nonce)

response, err := exchange.SubmitPrepared(ctx, []apollo.PreparedAction{prepared})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

Prepare only queries and signs; submitting requires an explicit call. The SDK never resends transactions automatically. If a submit times out or the response is lost, first query the result using the original account / nonce; see [Results](docs/reference/RESULTS.md).

## Docs and examples

- [Feature overview](docs/FEATURES.md)
- [API parameters and configuration](docs/API.md)
- [Market and account queries](docs/reference/INFO.md)
- [Trading API](docs/reference/EXCHANGE.md)
- [History and fees](docs/reference/HISTORY.md)
- [Results and errors](docs/reference/RESULTS.md)
- [Concurrency, precision and integration](docs/USAGE.md)
- [Versioning and runtime](PROVENANCE.md)
- [Changelog](CHANGELOG.md)
- [Command-line test workflow](examples/README.md)

`examples/info` runs read-only queries; `examples/offline` demonstrates offline signing; `examples/order` provides a single-order example.

`examples/trading` reads the APOLLO_* environment variables directly and uses `-action orders / order / cancel / modify` to query open orders or run test trades; by default it only prepares and signs, and broadcasts only with an explicit `-submit`. The account, RPC, genesis hash and keys are supplied by the user.

```bash
go run ./examples/trading -action orders
go run ./examples/trading -action order
go run ./examples/trading -action cancel -order-id "$APOLLO_ORDER_ID" -submit
```

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./examples/offline
```
