# 功能总览

[English](../FEATURES.md) | 简体中文

Apollo Go SDK 提供永续交易的查询、交易、授权与结果处理接口。

| 业务需求 | 接口 | 说明 |
| --- | --- | --- |
| 市场与精度 | `Info.Meta`、`Info.Market` | 市场状态、tick / step、公共费率档位与资金费配置 |
| 盘口与行情 | `Info.L2Snapshot`、`Info.Ticker` | 订单簿、买卖一和市场统计 |
| 账户资产与风险 | `Info.UserState`、`Info.UserStateByID` | 保证金、权益、风险状态及账户设置 |
| 持仓与挂单 | `Info.Positions`、`Info.OpenOrders` | 多空持仓、逐仓保证金及当前委托 |
| 单笔与批量下单 | `Exchange.Order`、`Exchange.BulkOrders` | 限价、IOC、只挂单与逐项结果 |
| 市价开仓和平仓 | `Exchange.MarketOpen`、`Exchange.MarketClose` | 带滑点保护的交易，指定多空方向与数量 |
| 止盈止损 | `Exchange.Order` 的 `OrderType.Trigger` | 触发市价或触发限价 |
| 撤单 | `Exchange.Cancel`、`Exchange.BulkCancel`、`Exchange.CancelAll` | 指定订单、市场或全部市场 |
| 单笔与批量改单 | `Exchange.ModifyOrder`、`Exchange.BulkModify` | 修改价格和数量，逐项处理执行结果 |
| 杠杆与逐仓保证金 | `Exchange.UpdateLeverage`、`Exchange.UpdateIsolatedMargin` | 账户风险设置 |
| TWAP 与分批订单 | `Exchange.TwapOrder`、`Exchange.CancelTwap`、`Exchange.ScaledOrder` | 创建计划、取消 TWAP、按价格分配委托 |
| agent 授权 | `Exchange.ApproveAgent`、`Exchange.RevokeAgent` | owner 批准及撤销交易 key |
| 成交和动作历史 | `Info.UserFills`、`Info.HistoricalOrders`、`Info.ActionHistory` | archive 数据及游标分页 |
| 手续费与成交返佣 | `Info.UserFees`、`SummarizeFees` | 按账户、角色和市场汇总成交费用 |
| K 线与资金费指数 | `Info.CandlesSnapshot`、`Info.FundingHistory` | 按时间或高度查询 |
| 链身份、授权、nonce 与额度 | `Info.ChainInfo`、`Info.Agents`、`Info.Nonce`、`Info.UserRateLimit` | 接入检查、并发与恢复 |
| 准备与执行 | `Exchange.PrepareOrder`、`Exchange.SubmitPrepared` | 分开签名和广播 |
| 原交易查询 | `Info.ActionStatus`、`Info.Transaction` | 按账户 / nonce 或哈希核查结果 |

## 阅读顺序

1. [快速开始](../../README.zh-CN.md)：安装、查询和第一笔交易。
2. [API 总览](API.md)：配置、参数与返回类型。
3. [市场和账户查询](reference/INFO.md)：市场、盘口、风险、持仓与挂单。
4. [交易接口](reference/EXCHANGE.md)：下单、撤单、改单及授权。
5. [历史与费用](reference/HISTORY.md)：成交、费用、K 线和分页。
6. [结果与异常](reference/RESULTS.md)：接收、执行、成交与恢复。
7. [接入指南](USAGE.md)：精度、超时、并发及密钥配置。
