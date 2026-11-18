# 交易接口

下面使用已经初始化的 `exchange`。每个方法都返回 `*OrderResponse, error`，业务逐项检查 Statuses；owner 授权操作使用 owner 签名客户端。

## 单笔和批量下单：Order / BulkOrders

**用途**：提交指定价格和数量的委托，选择 Gtc、Ioc 或 PostOnly。

```go
response, err := exchange.BulkOrders(ctx, []apollo.CreateOrderRequest{
    {Coin: "BTC-USD", IsBuy: true, Price: "64000", Size: "0.001"},
    {Coin: "BTC-USD", IsBuy: false, Price: "66000", Size: "0.001", Position: apollo.Short},
})
if err != nil {
    return err
}
for _, status := range response.Statuses {
    fmt.Println(status.SubmitStatus, status.Outcome)
}
```

单笔使用 `Order(ctx, request)`，批量使用请求数组，最多 20,000 项并受请求大小限制。Coin 是市场完整 ticker，Price / Size 使用十进制字符串，必须满足 tick / step。

OrderType.Limit.TIF 默认为 Gtc；Position 按方向推导，也可明确指定 Long / Short；Isolated 默认 false 表示全仓。完整字段见 [API](../API.md#下单参数)。

SDK 先校验整批再准备签名，广播后的接收和执行结果按输入顺序返回，可能部分成功。限价动作成功后，继续通过挂单和成交查询确认业务状态。

## 市价开仓和平仓：MarketOpen / MarketClose

**用途**：按当前价格尝试成交，并限制可接受滑点。

```go
opened, err := exchange.MarketOpen(ctx, apollo.MarketOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Size: "0.001", Slippage: "0.01",
})
if err != nil {
    return err
}
closed, err := exchange.MarketClose(ctx, apollo.ClosePositionRequest{
    Coin: "BTC-USD", Position: apollo.Long, Size: "0.001", Slippage: "0.01",
})
if err != nil {
    return err
}
fmt.Println(opened.Statuses, closed.Statuses)
```

市价意图使用 IOC 保护限价。Slippage 是比例字符串，默认 0.01，范围为大于等于 0、小于 1。ReferencePrice 可显式传参考价，省略时取对手盘最佳价，空盘口回退 mark。

平仓必须提供 Position 和 Size，SDK 自动确定方向并只减仓。实际成交量以成交及剩余持仓为准。

## 止盈止损：OrderType.Trigger

**用途**：建立触发市价或触发限价订单。

```go
response, err := exchange.Order(ctx, apollo.CreateOrderRequest{
    Coin: "BTC-USD", IsBuy: false, Size: "0.001", Position: apollo.Long, ReduceOnly: true,
    OrderType: apollo.OrderType{Trigger: &apollo.TriggerOrderType{
        TriggerPrice: "60000", IsMarket: true, TPSL: "sl",
    }},
    Slippage: "0.01",
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

TriggerPrice 是触发价，必须符合 tick。TPSL 为 tp 止盈或 sl 止损。IsMarket 为 false 时同时提供 Price，表示触发后的限价。

返回触发订单创建动作的结果。创建成功、触发和成交是不同阶段；多个订单分别返回自己的标识和执行结果。

## 撤单：Cancel / BulkCancel / CancelAll

**用途**：撤销指定委托、多个委托、某市场或全部市场委托。

```go
response, err := exchange.BulkCancel(ctx, []apollo.CancelOrderRequest{
    {Coin: "BTC-USD", OrderID: firstOrderID},
    {Coin: "BTC-USD", OrderID: secondOrderID},
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
response, err = exchange.CancelAll(ctx, "BTC-USD")
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

单笔使用 Cancel。CancelAll 的 coin 为空字符串时处理所有市场。OrderID 为 uint64。

撤单免累计动作额度，但授权及 nonce 仍需有效。撤单与成交可能竞争，执行后继续核对挂单及实际成交。

## 单笔和批量改单：ModifyOrder / BulkModify

**用途**：替换旧订单的价格和数量。

```go
response, err := exchange.BulkModify(ctx, []apollo.ModifyOrderRequest{
    {Coin: "BTC-USD", OrderID: firstOrderID, Price: "65100", Size: "0.001"},
    {Coin: "BTC-USD", OrderID: secondOrderID, Price: "65200", Size: "0.002"},
})
if err != nil {
    return err
}
for _, item := range response.Statuses {
    fmt.Println(item.Prepared.Nonce, item.Outcome, item.Error)
}
```

ModifyOrder 接收单个 ModifyOrderRequest，BulkModify 接收数组。字段为 Coin、OrderID、Price、Size，数量和价格必须符合市场精度。

批量先校验全部输入和总动作额度，再单次提交，逐项返回结果。新订单 ID 按新动作结果确认。改单先撤旧单再建新单，新单失败时旧单可能已撤；部分失败后不能直接重发整批。

## TWAP 创建和取消

**用途**：创建按时间拆分的交易计划，或停止父计划和当前子单。

```go
response, err := exchange.TwapOrder(ctx, apollo.CreateTwapOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Size: "0.01",
    DurationS: 600, IntervalS: 30, TolerancePPM: 10000,
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

DurationS 和 IntervalS 按秒计，TolerancePPM 为价格容差，10,000 表示 1%，零值为 0%。Price 省略时由 oracle 和容差生成保护价格；完整时长、精度规则见 [API](../API.md#高级订单)。

创建成功后保存父动作的 Prepared.Nonce。取消时使用它作为 OrderID：

```go
response, err := exchange.CancelTwap(ctx, apollo.CancelOrderRequest{
    Coin: "BTC-USD", OrderID: twapParentID,
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

取消父计划及当前子单，已成交部分仍然有效。取消免累计动作额度。父动作执行成功代表计划创建成功，实际成交继续查询成交和持仓。

## 分批订单：ScaledOrder

**用途**：在价格区间内按分配规则建立多个子单。

```go
response, err := exchange.ScaledOrder(ctx, apollo.CreateScaledOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Size: "0.01",
    LowestPrice: "62000", HighestPrice: "64000", NumOrders: 5,
    Profile: "Equal", Spacing: "Linear", TIF: apollo.Gtc,
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

Size 为总数量，NumOrders 为 2–50。Profile 控制数量分配，Spacing 控制价格间距。每个子单仍需符合市场规则；动作成功和全部子单成交分别判断。

## 杠杆和逐仓保证金

```go
response, err := exchange.UpdateLeverage(ctx, "BTC-USD", "5")
if err != nil {
    return err
}
fmt.Println(response.Statuses)
response, err = exchange.UpdateIsolatedMargin(ctx, "BTC-USD", apollo.Long, "10.50")
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

杠杆使用十进制字符串，输入范围为 1–1,000,000，实际可用值受市场风控约束。保证金单位为 USD，最多 6 位小数，正数增加、负数减少；执行后重新查询账户核实设置。

## agent 管理

`owner.ApproveAgent(ctx, slot, publicKey, expiry)` 授权交易 key；`owner.RevokeAgent(ctx, slot)` 撤销指定槽位。

slot 为 1–4，publicKey 为 32 字节，expiry 是截止高度，0 表示无到期高度。确认授权动作执行成功后再使用 agent。创建 signer 及完整示例见 [README](../../README.md#owner-钱包与-agent-授权)。
