# API 参考

`Info` 提供只读查询，`Exchange` 提供签名交易。方法首参数均为 `context.Context`，返回数据及 `error`。

按业务查阅 [功能总览](FEATURES.md)、[查询](reference/INFO.md)、[交易](reference/EXCHANGE.md)、[历史与费用](reference/HISTORY.md) 和 [结果处理](reference/RESULTS.md)。

## 初始化和配置

```go
info, err := apollo.NewInfo(endpoint, apollo.WithTimeout(10*time.Second))
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
fmt.Println(info, exchange.Account())
```

| 选项 | 默认值 | 说明 |
| --- | --- | --- |
| `WithAccount(id)` | 必填 | Exchange 的非 vault 账户 ID，非零 uint32 |
| `WithExpectedGenesisHash(hash)` | 首次连接固定 | 可信的目标链创世哈希 |
| `WithHTTPClient(client)` | SDK HTTP 客户端 | 配置代理、证书或自定义 Transport |
| `WithTimeout(duration)` | 10 秒 | 单次 HTTP 请求超时 |
| `WithSizeLimits(request, response)` | 4 MiB / 16 MiB | 请求与响应大小上限，还需遵守服务端限制 |
| `WithNonceManager(manager)` | 实例内分配器 | 同网络、同账户多实例共享 nonce |

构造阶段不发网络请求。HTTP 客户端拒绝重定向；传入的自定义客户端也使用此规则。

## Info

| 方法 | 参数 | 返回 |
| --- | --- | --- |
| `ChainInfo` | ctx | `*ChainInfo`，链身份与高度 |
| `Meta` / `Market` | ctx / ctx、coin | `*Meta` / `Market` |
| `L2Snapshot` | ctx、market uint32、depth uint32 | `*L2Book` |
| `Ticker` | ctx、market uint32 | `*Ticker` |
| `UserState` / `UserStateByID` | ctx、owner Address / account uint32 | `*UserState` |
| `Positions` | ctx、account uint32 | `*PositionsResult` |
| `OpenOrders` | ctx、account uint32、market *uint32 | `*OpenOrdersResult` |
| `Agents` / `Nonce` / `UserRateLimit` | ctx、account uint32 | 授权、nonce / window、额度 |
| `ActionStatus` | ctx、[]ActionKey | `*ActionStatuses`，最多 10,000 个键 |
| `Transaction` | ctx、hash Hash | `*Transaction` |
| `IndexStatus` | ctx | `*IndexStatus` |
| `UserFills` | ctx、UserFillsParams | `*FillsResult` |
| `UserFees` | ctx、UserFeesParams | `*UserFeesResult`，当前页费用汇总 |
| `HistoricalOrders` / `ActionHistory` | ctx、HistoryParams | `*HistoryResult` |
| `CandlesSnapshot` | ctx、CandlesParams | `*CandlesResult` |
| `FundingHistory` | ctx、FundingHistoryParams | `*FundingHistoryResult` |

`OpenOrders` 的 market 传 nil 表示全部市场，传指针表示指定市场。`HistoricalOrders` 返回订单相关动作记录；实际成交结合 UserFills 和当前挂单核查。

## 下单参数

`Order(ctx, CreateOrderRequest, opts...)` / `BulkOrders(ctx, []CreateOrderRequest, opts...)` 返回 `*OrderResponse`。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `Coin` | string | 市场完整 ticker，例如 BTC-USD |
| `IsBuy` | bool | true 买入，false 卖出 |
| `Price` / `Size` | string | 十进制价格与基础币数量，符合 tick / step |
| `OrderType.Limit.TIF` | Gtc / Ioc / PostOnly | 普通委托有效方式，默认 Gtc |
| `OrderType.Trigger` | *TriggerOrderType | 与 Limit 二选一 |
| `ReduceOnly` | bool | 默认 false，仅减仓 |
| `Position` | Long / Short | 默认按方向与 ReduceOnly 推导 |
| `Isolated` | bool | 默认 false 全仓，true 逐仓 |
| `GoodTilBlock` | uint64 | 截止高度，0 为不设置 |
| `Builder` | *Builder | Account 及 FeePPM，费率 1–10,000 ppm |
| `ReferencePrice` / `Slippage` | string | 市价触发单参考价及保护比例；滑点默认 0.01 |

`TriggerOrderType` 包含 `TriggerPrice string`、`IsMarket bool`、`TPSL string`。TPSL 为 tp / sl；触发限价单同时提供 Price。

## 其他交易

| 方法 | 请求 |
| --- | --- |
| `Cancel` / `BulkCancel` | CancelOrderRequest / []CancelOrderRequest，含 Coin、OrderID uint64 |
| `CancelAll` | coin string，空字符串为全部市场 |
| `ModifyOrder` / `BulkModify` | ModifyOrderRequest / []ModifyOrderRequest，含 Coin、OrderID、Price、Size |
| `CancelTwap` | CancelOrderRequest，OrderID 为父计划 ID |
| `MarketOpen` | MarketOrderRequest，含 Coin、IsBuy、Size，可选 Position、Isolated、ReduceOnly、ReferencePrice、Slippage |
| `MarketClose` | ClosePositionRequest，必填 Coin、Position、Size，可选 Isolated、ReferencePrice、Slippage |
| `UpdateLeverage` | coin、leverage string |
| `UpdateIsolatedMargin` | coin、position、amount string，金额以 USD 计，正数增加、负数减少 |
| `ApproveAgent` / `RevokeAgent` | slot、公钥 bytes、expiry / slot，owner 操作 |

批量最多 20,000 项，并受请求字节上限限制。请求逐项执行，整批非原子。Modify 先撤旧单后建新单，新单失败时旧单可能已撤销。

## 高级订单

`TwapOrder(ctx, CreateTwapOrderRequest, opts...)`：

- 必填 Coin、IsBuy、Size、DurationS、IntervalS。
- 可选 Price、TolerancePPM、Position、ReduceOnly、Isolated。
- DurationS 为 300–86,400 秒；IntervalS 为 30–3,600 秒，时长为间隔的整数倍。
- 每腿数量至少一个市场 step；TolerancePPM 小于 1,000,000，零值表示零容差。
- Price 省略时，链根据 oracle 和 TolerancePPM 生成子单保护价格。按 1% 配置时显式传 10,000。

`ScaledOrder(ctx, CreateScaledOrderRequest, opts...)`：

- 必填 Coin、IsBuy、Size、LowestPrice、HighestPrice、NumOrders。
- NumOrders 为 2–50；Size 为总数量。
- Profile 为 Equal（默认）、IncRatio、IncDiff、DecRatio、DecDiff。
- Spacing 为 Linear（默认）、Geometric；TIF 为 Gtc（默认）或 Ioc。
- MagnitudePPM 为分配参数，Equal 使用 0，其他分布遵守对应参数约束。
- 可选 Position、ReduceOnly、Isolated、GoodTilBlock。

## 历史参数

| 类型 | 字段 |
| --- | --- |
| `UserFillsParams` | Account *Uint64、Market *uint32、FromHeight Uint64、After *Cursor、Limit uint32 |
| `UserFeesParams` | Account uint32 必填，其余过滤条件与成交查询一致 |
| `HistoryParams` | Account Uint64、FromHeight Uint64、After *Cursor、Limit uint32 |
| `CandlesParams` | Market uint32、Interval CandleInterval、FromUnixS / ToUnixS Uint64、Limit uint32 |
| `FundingHistoryParams` | Market uint32、FromHeight Uint64、Limit uint32 |

历史 Limit 默认 100，服务端最多 1,000。Cursor 包含 Height Uint64、Index uint32，使用响应的 Next 继续查询，保留原过滤条件。

## 费用汇总

`Info.UserFees()` 返回一页 `{Summary, Index, Next}`；`SummarizeFees(account, fills)` 对已有 `[]Fill` 汇总，不发请求。

| Summary 字段 | 类型 | 含义 |
| --- | --- | --- |
| Account | uint32 | 账户 |
| MakerFees / TakerFees | string | 各角色有符号费用，微 USD |
| FeesPaid / Rebates | string | 支付 / 返佣的正数金额，微 USD |
| NetFees | string | 支付减返佣，微 USD，可为负 |
| FillCount | uint64 | 涉及本账户的成交记录数 |
| MakerFillCount / TakerFillCount | uint64 | 各角色记录数 |
| ByMarket | []MarketFeeSummary | 按市场 ID 排序，各项含 Market 和费用 / 计数 |

统计口径为每条成交的 MakerFee / TakerFee；存在 Next 时继续分页得到查询范围内的总额。精确汇总使用大整数，返回十进制整数字符串；同条记录双方都是本账户时，两侧费用都会统计，FillCount 仅加一。

## 准备与提交

| 方法 | 用途 |
| --- | --- |
| `PrepareOrder` | 检查订单并签名，不广播，返回 PreparedAction |
| `Prepare` / `PrepareActions` | 准备 Action，使用该类型声明的整数单位 |
| `Execute` / `ExecuteActions` | 准备并提交动作 |
| `SubmitPrepared` | 核对已准备交易，提交并返回逐项结果 |

`PreparedAction` 提供 Account、Nonce、GenesisHash、TransactionHash 用于对账，并保存已签名交易供提交，不包含私钥。

所有交易方法可追加 `NoWait()`、`WaitTimeout(duration)`、`PollInterval(duration)`。默认执行等待 15 秒、轮询 250 毫秒；单次 HTTP 请求超时另由 WithTimeout 控制。context 取消停止 IO 和本地等待，已接收交易仍可能执行。

返回状态和错误类见 [结果处理](reference/RESULTS.md)。

## 签名与扩展

本地 owner 使用 `NewOwnerSigner(privateKey)`，钱包 / HSM 使用 `NewWalletSigner(ownerAddress, callback)`，agent 使用 `NewAgentSigner(seed, slot)`。

wallet callback 接收 TypedData，返回 65 字节签名。自定义 signer 实现 Signer 接口；并发分配器实现 NonceManager 接口，接入约束见 [USAGE](USAGE.md)。

`Info.Client().Call(ctx, method, params, result)` 可调用其他一元查询。扩展结构体中 64 位字段使用 Uint64 / Int64，地址使用 Address，字节使用 HexBytes。

## 显示和单位转换

- 盘口、挂单价格：`SubticksToPrice(value, market)`。
- 成交、mark / oracle、K 线价格：`PriceUnitsToPrice(value, market)`。
- 基础币数量：`QuantumsToSize(value, market)`。
- 用户输入转整数：`PriceToSubticks`、`SizeToQuantums`、`USDToMicroUSD`。
- 杠杆转换：`LeverageToIMF`。

转换方法返回结果及 error。高层十进制输入使用普通数字字符串，不接受科学计数法、NaN 和 Infinity。
