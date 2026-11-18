# 结果与异常

交易方法返回 `*OrderResponse, error`。节点接收、动作执行和订单成交分别确认。

## 返回结果

| 字段 | 含义 |
| --- | --- |
| ID | 提交批次 ID |
| Statuses | 与输入顺序对应的结果数组 |
| Detail | 节点附带的结果说明 |
| Successful() | 仅全部动作已执行且成功时为 true |

每个 OrderStatus 包含 Prepared、SubmitStatus、Outcome、Execution 和 Error。

- Prepared.Account / Nonce 是动作查询键，TransactionHash 是原交易哈希。
- SubmitStatus 是节点接收或拒绝状态。
- Execution 是已有执行详情，使用前判断是否为 nil。

```go
for _, item := range response.Statuses {
    fmt.Println(item.Prepared.TransactionHash, item.SubmitStatus, item.Outcome)
    if item.Outcome == apollo.OutcomeExecuted && item.Execution != nil {
        fmt.Println(item.Execution.OK, item.Execution.Reject, item.Execution.OrderID)
    }
}
```

SubmitAccepted 表示接收；SubmitMalformed、SubmitBadSignature、SubmitUnavailable 表示节点明确拒绝对应项。批量可能混合不同结果。

## 执行状态

| Outcome | 含义 | 处理 |
| --- | --- | --- |
| OutcomeExecuted，Execution.OK=true | 动作执行成功 | 继续查挂单和成交 |
| OutcomeExecuted，Execution.OK=false | 动作执行被拒 | 查看 Reject / Error |
| OutcomePending | 尚未确认或选择不等待 | 继续查询原动作 |
| OutcomeNonceUsed | nonce 已使用但当前无执行详情 | 查交易和历史动作 |
| OutcomeNotSubmitted | 节点明确未接收该项 | 检查接收状态和 Error |

限价动作成功可能只是挂入订单簿，IOC 可能部分成交或零成交，TWAP 可能只是计划建立。Execution.Fills 是动作直接产生的成交条目数，逐笔成交及实际数量使用 UserFills 查询。

## 查询原动作和交易

```go
statuses, err := info.ActionStatus(ctx, []apollo.ActionKey{
    {Account: apollo.Uint64(accountID), Nonce: apollo.Uint64(originalNonce)},
})
if err != nil {
    return err
}
fmt.Println(statuses.Statuses)
transaction, err := info.Transaction(ctx, transactionHash)
if err != nil {
    return err
}
fmt.Println(transaction)
```

ActionStatus 单次最多 10,000 个键，返回与请求键对应的状态。Transaction 使用 Hash。NOT_FOUND 只表示当前节点没有执行详情，不能证明原请求从未接收；NONCE_USED 同样不能视为成功。

历史查询需结合索引范围和进度。提交不确定时保留原键及哈希，避免直接换 nonce 下第二笔订单。

## 异常分类

| 错误 | 保留的证据 | 处理 |
| --- | --- | --- |
| `*SubmissionUnknownError` | Prepared 数组和 Cause | 查原动作 / 交易，再决定后续操作 |
| `*TrackingError` | Response 和 Cause | 保留已接收、已确认结果，继续查询未确认项 |
| ErrActionAllowance | 可用动作额度不足 | 停止重复提交，查额度 |
| `*ChainIdentityError` | Expected / Actual 创世哈希 | 核对网络配置 |
| `*RPCError` / `*HTTPError` | 错误码及消息 / HTTP 状态 | 结合请求发生阶段处理 |
| ErrRequestTooLarge / ErrResponseTooLarge | 大小超限 | 调整请求大小、过滤或分页 |

```go
response, err := exchange.Order(ctx, apollo.CreateOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Price: "65000", Size: "0.001",
})
if err != nil {
    var unknown *apollo.SubmissionUnknownError
    var tracking *apollo.TrackingError
    switch {
    case errors.As(err, &unknown):
        for _, prepared := range unknown.Prepared {
            fmt.Println(prepared.Account, prepared.Nonce, prepared.TransactionHash)
        }
    case errors.As(err, &tracking):
        fmt.Println(tracking.Response.Statuses)
    case errors.Is(err, apollo.ErrActionAllowance):
        fmt.Println("account action allowance is insufficient")
    default:
        return err
    }
    return nil
}
fmt.Println(response.Statuses)
```

业务执行拒绝保留在逐项结果中，不一律作为 RPC error 返回。SDK 不自动重发 Submit；批量部分成功时先对账，再处理明确需要后续操作的项。

## 等待与取消

默认执行等待 15 秒，250 毫秒轮询一次。`NoWait()` 只等待提交响应，accepted 项为 pending；`WaitTimeout` 和 `PollInterval` 调整本地执行等待。

正常等待到期返回 pending，保留已确认项。接收后查询失败或调用方 context 取消返回 TrackingError。取消 IO 或等待不会撤回已接收的链上动作。
