# Command-Line Examples

English | [简体中文](README.zh-CN.md)

Run these from the Go SDK repository with Go 1.22 or later. The first `go run` downloads the dependencies listed in go.mod.

## Environment configuration

By default `examples/trading` reads the APOLLO_* environment variables from the shell; you can export them from your own config file. Command-line flags with the same name take precedence over the environment.

| Environment variable | Flag | Description |
| --- | --- | --- |
| APOLLO_RPC_URL | — | Apollo HTTP(S) JSON-RPC URL |
| APOLLO_ACCOUNT | -account | Non-zero numeric account ID |
| APOLLO_GENESIS_HASH | — | Trusted genesis hash; required for signing operations |
| APOLLO_AGENT_SEED | — | Hex-encoded 32-byte seed of an approved agent |
| APOLLO_AGENT_SLOT | -slot | Agent slot, default 1 |
| APOLLO_COIN | -coin | Full market ticker; when querying orders, empty means all markets |
| APOLLO_PRICE | -price | Decimal price string |
| APOLLO_SIZE | -size | Base-asset size string |
| APOLLO_ORDER_ID | -order-id | Target order ID, digits only, without the n suffix |

The configuration contains no default account or private key. Querying orders does not need an agent seed; when a trusted genesis hash is set, order queries also verify the target chain. Every run of a trading operation prepares and signs afresh; save the nonce and transaction hash of the run you actually submit.

## 1. Query the chain and markets

```bash
go run ./examples/info
```

Requires only APOLLO_RPC_URL. Prints the chain identity, height and market list.

## 2. Prepare or submit an order

```bash
# Read account, market, price and size from the environment; by default prepare a Gtc limit buy order
go run ./examples/trading -action order

# Actually send the order
go run ./examples/trading -action order -submit
```

Pass -sell to sell instead. The full trading configuration is required, and price and size must match the market's precision. A successful prepare does not mean the order is filled; after submitting, check the execution status and query open orders and fills.

## 3. Query open orders

```bash
go run ./examples/trading -action orders
go run ./examples/trading -action orders -coin BTC-USD
go run ./examples/trading -action orders -order-id "$APOLLO_ORDER_ID"
```

The default action is orders. You can filter by market and order ID. The output shows order ID, market, side, price, remaining size and position slot. Prices and sizes are shown in human-readable decimal format.

## 4. Cancel a specific order

```bash
# Prepare the cancel signature
go run ./examples/trading -action cancel -order-id "$APOLLO_ORDER_ID"

# Actually send the cancel and query open orders afterwards
go run ./examples/trading -action cancel -order-id "$APOLLO_ORDER_ID" -submit
```

On success it prints `Still open after cancellation: false`; you can then query orders again to confirm. The nonce in the cancel response belongs to the new cancel action; the target order ID is unchanged.

## 5. Modify a specific order

```bash
# Prepare a price and size modification
go run ./examples/trading -action modify -order-id "$APOLLO_ORDER_ID" -price 65100 -size 0.001

# Actually submit the modification
go run ./examples/trading -action modify -order-id "$APOLLO_ORDER_ID" -price 65100 -size 0.001 -submit
```

Modifying an order cancels the old order first, then creates the new one; if the new order fails, the old one may already be canceled. After a successful replacement, take the new ID from the order_id in the execution result, and use the new ID for later queries and cancels.

## Results and errors

By default, trades are only prepared and signed, printing the hash, the nonce and Not broadcast. Broadcasting happens only with an explicit -submit.

After submitting, the tool prints the accepted status, execution status and execution details separately. If success is not confirmed, the tool exits with a non-zero status and keeps the account / nonce / hash for reconciliation; it never resends automatically. Successful execution and actual order fills are judged separately.

## Other examples

- `examples/offline`: offline signing with a temporarily generated key.
- `examples/order`: single-order example; pass order parameters explicitly via -account, -coin, -price, -size.
- `examples/trading`: the full command-line test workflow, selected with -action.

```bash
go run ./examples/trading -h
go test ./...
go test -race ./...
go vet ./...
```

Repository tests use a local mock node and test keys; tests against a target chain are run explicitly by the caller.
