# Trading API

English | [简体中文](../zh-CN/reference/EXCHANGE.md)

The examples below use an already initialized `exchange`. Every method returns `*OrderResponse, error`; check Statuses item by item. Owner authorization operations use a client with an owner signer.

## Single and bulk orders: Order / BulkOrders

**Use**: submit orders at a given price and size, choosing Gtc, Ioc or PostOnly.

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

Use `Order(ctx, request)` for a single order and a request slice for a batch, up to 20,000 items and subject to the request size limit. Coin is the full market ticker; Price / Size are decimal strings and must satisfy tick / step.

OrderType.Limit.TIF defaults to Gtc; Position is derived from the side but can also be set explicitly to Long / Short; Isolated defaults to false, meaning cross margin. For all fields, see [API](../API.md#order-parameters).

The SDK validates the whole batch before preparing and signing. After broadcast, accepted and execution results are returned in input order and may partially succeed. After a limit action succeeds, keep querying open orders and fills to confirm the business state.

## Market open and close: MarketOpen / MarketClose

**Use**: try to fill at the current price while limiting acceptable slippage.

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

Market intents use an IOC protective limit price. Slippage is a ratio string, default 0.01, in the range greater than or equal to 0 and less than 1. ReferencePrice can be passed explicitly as the reference price; when omitted, the best opposite-side price is used, falling back to the mark price if the book is empty.

Closing a position requires Position and Size; the SDK determines the side automatically and makes the order reduce-only. The actual filled size is determined by fills and the remaining position.

## Take profit / stop loss: OrderType.Trigger

**Use**: create trigger market or trigger limit orders.

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

TriggerPrice is the trigger price and must match the tick. TPSL is tp for take profit or sl for stop loss. When IsMarket is false, also provide Price, the limit price after triggering.

Returns the result of the trigger order creation action. Successful creation, triggering and filling are separate stages; multiple orders each return their own identifiers and execution results.

## Cancel orders: Cancel / BulkCancel / CancelAll

**Use**: cancel a specific order, multiple orders, or all orders in one market or in all markets.

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

Use Cancel for a single order. When CancelAll's coin is an empty string, it applies to all markets. OrderID is a uint64.

Canceling is exempt from the cumulative action allowance, but authorization and nonce must still be valid. Cancels can race with fills; after execution, keep checking open orders and actual fills.

## Single and bulk modify: ModifyOrder / BulkModify

**Use**: replace an existing order's price and size.

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

ModifyOrder takes a single ModifyOrderRequest; BulkModify takes a slice. The fields are Coin, OrderID, Price and Size; size and price must match market precision.

A batch first validates all inputs and the total action allowance, then submits once and returns per-item results. Confirm the new order ID from the new action's result. Modifying an order cancels the old order first, then creates the new one; if the new order fails, the old one may already be canceled. After a partial failure, do not simply resend the whole batch.

## TWAP create and cancel

**Use**: create a trading schedule split over time, or stop the parent schedule and its current child order.

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

DurationS and IntervalS are in seconds; TolerancePPM is the price tolerance, where 10,000 means 1% and zero means 0%. When Price is omitted, the protective price is derived from the oracle and the tolerance; for full duration and precision rules, see [API](../API.md#advanced-orders).

After successful creation, save the parent action's Prepared.Nonce. Use it as the OrderID when canceling:

```go
response, err := exchange.CancelTwap(ctx, apollo.CancelOrderRequest{
    Coin: "BTC-USD", OrderID: twapParentID,
})
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

This cancels the parent schedule and the current child order; already filled portions remain in effect. Canceling is exempt from the cumulative action allowance. Successful execution of the parent action means the schedule was created; for actual fills, keep querying fills and positions.

## Scaled orders: ScaledOrder

**Use**: create multiple child orders across a price range according to a distribution rule.

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

Size is the total size and NumOrders is 2–50. Profile controls size distribution and Spacing controls price spacing. Each child order must still satisfy market rules; action success and all child orders being filled are judged separately.

## Leverage and isolated margin

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

Leverage is a decimal string with an input range of 1–1,000,000; the actually usable value is constrained by market risk controls. Margin is in USD with at most 6 decimal places; positive adds, negative removes. After execution, query the account again to verify the setting.

## Agent management

`owner.ApproveAgent(ctx, slot, publicKey, expiry)` approves a trading key; `owner.RevokeAgent(ctx, slot)` revokes the given slot.

slot is 1–4, publicKey is 32 bytes, and expiry is the expiry height; 0 means no expiry height. Use the agent only after confirming the approval action executed successfully. For creating a signer and a full example, see the [README](../../README.md#owner-wallet-and-agent-approval).
