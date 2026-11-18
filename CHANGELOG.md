# Changelog

English | [简体中文](CHANGELOG.zh-CN.md)

## 1.0.1

- Added English documentation; English is now the default. The Chinese docs moved to `README.zh-CN.md` and `docs/zh-CN/`, with a language switcher at the top of every page.

## 1.0.0

First public release.

- `Info` queries: markets, order book, market data, accounts, positions, open orders, fills, fee summaries and historical data.
- `Exchange` trading: limit, trigger, market IOC, bulk place / modify / cancel, close position, leverage, isolated margin, TWAP, scaled orders and agent approval.
- Three signing modes: owner private key, wallet / HSM callback, and agent.
- Separate prepare and submit, chain identity checks, and execution tracking.
- Exact integer and decimal conversion, concurrent nonce allocation, and request cancellation.
- Command-line example trading: query, place, modify and cancel orders; by default it only prepares and signs, and tracks results after an explicit submit.
