# Apollo Go SDK

[English](README.md) | 简体中文

Apollo 的 Go 交易 SDK，通过 HTTP JSON-RPC 提供市场与账户查询、下单、撤单、改单、签名授权和交易结果跟踪。价格、数量、保证金使用十进制字符串。

包名为 `apollo`，Go module 为 `github.com/svpchain/apollo-go-sdk`，运行环境为 **Go 1.22+**。

| 客户端 | 常用方法 |
| --- | --- |
| `Info` | `Meta`、`L2Snapshot`、`UserState`、`OpenOrders`、`UserFills`、`UserFees` |
| `Exchange` | `Order`、`BulkOrders`、`BulkModify`、`CancelTwap`、`MarketOpen`、`MarketClose` |

完整接口目录见 [功能总览](docs/zh-CN/FEATURES.md)。

## 安装

```bash
go get github.com/svpchain/apollo-go-sdk@latest
```

`@latest` 取最新正式版，并把具体版本写入你项目的 `go.mod`。需要固定版本时写明版本号，例如 `go get github.com/svpchain/apollo-go-sdk@v1.0.0`。各版本的变更见 [版本记录](CHANGELOG.zh-CN.md)。

`endpoint` 使用目标环境提供的完整 Apollo HTTP(S) JSON-RPC 地址，支持路径和查询参数。

## 查询市场和账户

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

查询不需要签名。`UserState(ctx, owner)` 按 owner 地址查询，`UserStateByID(ctx, account)` 按账户数字 ID 查询；`owner` 可以通过 `ParseAddress()` 解析。

`OpenOrders(ctx, account, nil)` 查询全部市场。传 `&marketID` 则筛选指定市场，市场 0 是有效 ID。

## 用已授权 agent 下单

`seed` 为 32 字节 Ed25519 种子，公钥需要事先由账户 owner 授权。交易账户使用链上数字 ID，`Coin` 使用市场完整 ticker。

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

`NewExchange` 不发网络请求。交易方法会检查市场规则、链身份、签名授权、动作额度和 nonce。首次连接后会固定目标链的创世哈希，后续签名与提交重新核对。

默认等待执行最多 15 秒。`response.Successful()` 表示所有动作执行成功；订单实际成交应继续查询挂单与成交记录。

## 常用交易

以下调用分别执行对应操作，实际接入时按业务选择，并检查每次返回的 `err` 和逐项状态。

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

市价意图使用 IOC 保护限价，默认滑点为 1%。平仓指定多空槽位和数量，`Isolated: true` 选择逐仓，默认全仓。

改单先撤旧单再创建新单；新单被拒时旧单可能已经撤销。批量逐项执行，结果需要逐项处理。

## 批量改单、TWAP 取消和费用

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

TWAP 取消使用父订单 ID，会取消计划及当前子单。费用按成交中的 MakerFee / TakerFee 统计；金额为微 USD 的十进制整数字符串，支持超出 int64 的汇总。`UserFees()` 返回当前页汇总，存在 `Next` 时继续分页。已取得成交记录可调用 `SummarizeFees()` 直接汇总。

## Owner 钱包与 agent 授权

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

授权到期时间使用区块高度，0 表示无到期高度。`RevokeAgent(ctx, slot)` 撤销授权。钱包或 HSM 接入可使用 `NewWalletSigner(ownerAddress, callback)`，由回调完成签名。SDK 会核对返回签名与链上 owner。

## 只准备，不广播

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

准备只查询和签名，提交需要显式调用。SDK 不自动重发交易。提交超时或响应丢失时，先使用原 account / nonce 查询结果，详见 [结果处理](docs/zh-CN/reference/RESULTS.md)。

## 文档与示例

- [功能总览](docs/zh-CN/FEATURES.md)
- [API 参数与配置](docs/zh-CN/API.md)
- [市场和账户查询](docs/zh-CN/reference/INFO.md)
- [交易接口](docs/zh-CN/reference/EXCHANGE.md)
- [历史与费用](docs/zh-CN/reference/HISTORY.md)
- [结果与异常](docs/zh-CN/reference/RESULTS.md)
- [并发、精度和接入](docs/zh-CN/USAGE.md)
- [版本与运行环境](PROVENANCE.zh-CN.md)
- [版本记录](CHANGELOG.zh-CN.md)
- [命令行测试流程](examples/README.zh-CN.md)

`examples/info` 只读查询；`examples/offline` 演示离线签名；`examples/order` 提供单笔订单示例。

`examples/trading` 直接读取 APOLLO_* 环境变量，通过 `-action orders / order / cancel / modify` 查询挂单或测试交易；默认只准备签名，显式 `-submit` 才广播。账户、RPC、创世哈希和密钥由使用方提供。

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
