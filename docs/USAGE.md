# 接入指南

## 账户与网络

先通过 Info 查询链信息和账户状态。endpoint 使用目标环境提供的 Apollo HTTP(S) JSON-RPC 地址，交易配置可信创世哈希、已存在的账户 ID 和有效 signer。

账户 owner 使用 Address，交易账户使用 uint32 ID；市场使用完整 ticker。agent 先由 owner 授权，确认执行成功后再用于交易。

## 精度和金额

订单价格、数量、杠杆及保证金使用十进制字符串。限价和数量必须符合 tick / step，精度不符会报错，不会自动放大到最小数量。

查询中的 64 位整数为 Uint64 / Int64，JSON 序列化无损。费用汇总使用 math/big 精确计算，金额为微 USD 整数字符串，跨页累加也使用大整数。

盘口、挂单和成交字段使用各自规定的价格单位；显示工具见 [API](API.md#显示和单位转换)。

## 超时和取消

使用 `WithTimeout` 设置每次 HTTP 请求超时，默认 10 秒。context 的 deadline 控制整个调用；执行等待另有 WaitTimeout，默认 15 秒。

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

执行等待正常到期返回仍为 pending 的项。context 取消或查询错误可能发生在交易已接收后，使用专门异常内保留的原交易继续核查，见 [结果处理](reference/RESULTS.md)。

## 并发 nonce

同一账户的 owner 与所有 agent 共用 nonce 空间。多个 Exchange 实例应传入同一个分配器：

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

默认分配器支持当前进程的并发访问。多个进程需要外部原子协调，并按网络和账户隔离分配记录。

NonceManager 的 Next(ctx, account, lastOnChain) 返回大于 lastOnChain 的唯一非零 uint64。nonce 乱序窗口为 16，唯一分配仍需配合受控并发；对顺序敏感的操作，等待前一笔确认后继续。

## 批量与额度

BulkOrders、BulkCancel、BulkModify、ExecuteActions 最多 20,000 项，并受客户端、节点和代理字节上限约束。默认请求最多 4 MiB，响应最多 16 MiB，按实际环境控制批次。

批量请求逐项执行，先核对各项结果再决定下一步。对提交不确定或部分成功的批次，避免直接整体重发。

UserRateLimit 是账户累计动作额度，不是 HTTP 每秒限流或 SVP gas。owner 和 agent 共享；Cancel / CancelAll / CancelTwap 免累计额度。准备阶段会预检其他动作的所需额度，最终仍由链判断。

## 密钥与授权

agent seed 和 owner 私钥由调用方安全提供，不写入源码或日志。钱包 / HSM 可以使用 NewWalletSigner 回调；NewOwnerSigner 适用于受控服务。

SDK 复制传入本地 key / seed。agent 换 key、撤销或过期后，之前准备的交易可能无法提交；提交前会重新验证链身份和当前授权。

## 测试环境接入

先使用专门测试账户完成授权、下单、查询挂单、撤单、成交与仓位核对，再进入持续运行。仓库自动测试使用链测试向量和模拟服务，目标环境仍需完成实际接入验证。

默认示例只离线或只读。运行 examples/order 时，只有显式传 -submit 才会广播交易。
