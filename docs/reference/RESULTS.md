# Results and Errors

English | [简体中文](../zh-CN/reference/RESULTS.md)

Trading methods return `*OrderResponse, error`. Node acceptance, action execution and order fills are confirmed separately.

## Response

| Field | Meaning |
| --- | --- |
| ID | Submission batch ID |
| Statuses | Result slice in input order |
| Detail | Result description attached by the node |
| Successful() | true only when all actions have executed successfully |

Each OrderStatus contains Prepared, SubmitStatus, Outcome, Execution and Error.

- Prepared.Account / Nonce is the action lookup key, and TransactionHash is the original transaction hash.
- SubmitStatus is the node's accepted or rejected status.
- Execution holds the available execution details; check for nil before use.

```go
for _, item := range response.Statuses {
    fmt.Println(item.Prepared.TransactionHash, item.SubmitStatus, item.Outcome)
    if item.Outcome == apollo.OutcomeExecuted && item.Execution != nil {
        fmt.Println(item.Execution.OK, item.Execution.Reject, item.Execution.OrderID)
    }
}
```

SubmitAccepted means accepted; SubmitMalformed, SubmitBadSignature and SubmitUnavailable mean the node explicitly rejected that item. A batch may mix different results.

## Execution status

| Outcome | Meaning | Handling |
| --- | --- | --- |
| OutcomeExecuted, Execution.OK=true | Action executed successfully | Keep querying open orders and fills |
| OutcomeExecuted, Execution.OK=false | Action execution rejected | Inspect Reject / Error |
| OutcomePending | Not yet confirmed, or not waiting was chosen | Keep querying the original action |
| OutcomeNonceUsed | Nonce already used but no execution details available | Query the transaction and action history |
| OutcomeNotSubmitted | The node explicitly did not accept this item | Check the accepted status and Error |

A successful limit action may only mean the order is resting in the order book; IOC may fill partially or not at all; TWAP may only mean the schedule was created. Execution.Fills is the number of fill entries produced directly by the action; query individual fills and actual sizes with UserFills.

## Query the original action and transaction

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

ActionStatus accepts at most 10,000 keys per call and returns statuses matching the requested keys. Transaction takes a Hash. NOT_FOUND only means the current node has no execution details; it does not prove the original request was never accepted. NONCE_USED likewise cannot be treated as success.

History queries must take the index range and progress into account. When a submission is uncertain, keep the original key and hash, and avoid placing a second order with a new nonce.

## Error types

| Error | Evidence preserved | Handling |
| --- | --- | --- |
| `*SubmissionUnknownError` | Prepared slice and Cause | Query the original action / transaction, then decide what to do next |
| `*TrackingError` | Response and Cause | Keep the accepted and confirmed results; keep querying unconfirmed items |
| ErrActionAllowance | Insufficient available action allowance | Stop resubmitting; check the allowance |
| `*ChainIdentityError` | Expected / Actual genesis hash | Check the network configuration |
| `*RPCError` / `*HTTPError` | Error code and message / HTTP status | Handle according to the stage at which the request failed |
| ErrRequestTooLarge / ErrResponseTooLarge | Size limit exceeded | Adjust request size, filters or pagination |

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

Business-level execution rejections stay in the per-item results and are not uniformly returned as RPC errors. The SDK never resends Submit automatically; when a batch partially succeeds, reconcile first, then handle the items that clearly need follow-up.

## Waiting and cancellation

By default the execution wait is 15 seconds, polling every 250 ms. `NoWait()` waits only for the submit response, and accepted items are pending; `WaitTimeout` and `PollInterval` adjust the local execution wait.

When the wait expires normally, pending is returned and confirmed items are kept. A query failure after acceptance, or cancellation of the caller's context, returns TrackingError. Canceling IO or the wait does not withdraw on-chain actions that have already been accepted.
