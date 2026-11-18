# Versioning and Runtime

English | [简体中文](PROVENANCE.zh-CN.md)

## Version

- Module: `github.com/svpchain/apollo-go-sdk`
- Package: `apollo`
- Version: follows semantic versioning; see the [Changelog](CHANGELOG.md) for changes in each release
- Go: 1.22 or later

For installation, see [Quick start](README.md#installation).

## Clients

`Info` provides queries for markets, the order book, market data, accounts, positions, open orders, fills, fees and history.

`Exchange` provides placing orders, batch trading, canceling and modifying orders, closing positions, leverage, isolated margin, TWAP, scaled orders and agent approval.

## Data types

Prices, sizes and amounts in trading inputs are decimal strings. 64-bit integers from RPC keep their precision via `Uint64` / `Int64`; addresses use `Address`, hashes use `Hash`, and bytes use `HexBytes`.

Fee summaries express micro-USD as decimal integer strings, suitable for storage and JSON serialization. For supported types and methods, see the [API](docs/API.md).

## Integration configuration

`endpoint` is the target environment's Apollo HTTP(S) JSON-RPC URL. Trading requires an account ID, a signer and a trusted genesis hash. History queries use a node that provides the archive index; confirm the indexed height with `IndexStatus()`.

When integrating with a new environment, use a test account to go through authorization, placing an order, querying open orders, canceling, and checking fills before enabling continuous trading.
