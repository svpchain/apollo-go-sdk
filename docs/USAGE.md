# Integration Guide

English | [简体中文](zh-CN/USAGE.md)

## Accounts and network

Start by querying chain info and account state through Info. endpoint is the Apollo HTTP(S) JSON-RPC URL provided by the target environment; trading requires a trusted genesis hash, an existing account ID and a valid signer.

Account owners are identified by Address and trading accounts by a uint32 ID; markets use the full ticker. An agent must first be approved by the owner, and used for trading only after the approval is confirmed as executed successfully.

## Precision and amounts

Order prices, sizes, leverage and margin are decimal strings. Limit prices and sizes must match tick / step; mismatched precision returns an error and is never automatically scaled up to the minimum size.

64-bit integers in queries are Uint64 / Int64 and serialize to JSON losslessly. Fee summaries are computed exactly with math/big; amounts are micro-USD integer strings, and accumulation across pages also uses big integers.

Order book, open order and fill fields each use their own specified price units; for display helpers, see [API](API.md#display-and-unit-conversion).

## Timeouts and cancellation

Use `WithTimeout` to set the per-HTTP-request timeout, default 10 seconds. The context deadline governs the entire call; waiting for execution has its own WaitTimeout, default 15 seconds.

```go
ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
defer cancel()
response, err := exchange.Order(ctx, apollo.CreateOrderRequest{
    Coin: "BTC-USD", IsBuy: true, Price: "65000", Size: "0.001",
}, apollo.WaitTimeout(20*time.Second), apollo.PollInterval(500*time.Millisecond))
if err != nil {
    return err
}
fmt.Println(response.Statuses)
```

When the execution wait expires normally, items that are still pending are returned as pending. A context cancellation or query error may happen after the transaction has been accepted; keep verifying using the original transaction preserved in the dedicated error type, see [Results](reference/RESULTS.md).

## Concurrent nonces

The owner and all agents of the same account share one nonce space. Multiple Exchange instances should be given the same allocator:

```go
shared := apollo.NewNonceManager()
exchange, err := apollo.NewExchange(endpoint, signer,
    apollo.WithAccount(accountID),
    apollo.WithExpectedGenesisHash(trustedGenesis),
    apollo.WithNonceManager(shared),
)
if err != nil {
    return err
}
fmt.Println(exchange.Account())
```

The default allocator supports concurrent access within the current process. Multiple processes need external atomic coordination, with allocation records isolated per network and account.

NonceManager's Next(ctx, account, lastOnChain) returns a unique non-zero uint64 greater than lastOnChain. The nonce out-of-order window is 16, so unique allocation still has to be paired with controlled concurrency; for order-sensitive operations, wait for the previous one to be confirmed before continuing.

## Batches and allowance

BulkOrders, BulkCancel, BulkModify and ExecuteActions accept at most 20,000 items, and are also bound by client, node and proxy byte limits. By default requests are at most 4 MiB and responses at most 16 MiB; size batches according to your actual environment.

Batch requests execute item by item; check every item's result before deciding the next step. For batches whose submission is uncertain or that partially succeeded, avoid resending the whole batch.

UserRateLimit is the account's cumulative action allowance, not an HTTP per-second rate limit or SVP gas. It is shared by the owner and agents; Cancel / CancelAll / CancelTwap are exempt from the cumulative allowance. The prepare step pre-checks the allowance required by other actions, but the chain makes the final decision.

## Keys and authorization

Agent seeds and owner private keys are supplied securely by the caller and must not be written into source code or logs. Wallets / HSMs can use the NewWalletSigner callback; NewOwnerSigner is suited to controlled services.

The SDK copies the local key / seed passed in. After an agent key is rotated, revoked or expired, previously prepared transactions may fail to submit; chain identity and current authorization are re-verified before submitting.

## Test environment integration

First use a dedicated test account to go through authorization, placing an order, querying open orders, canceling, and checking fills and positions before running continuously. The repository's automated tests use chain test vectors and mock services; the target environment still needs real integration verification.

The default examples are offline-only or read-only. When running examples/order, transactions are broadcast only with an explicit -submit.
