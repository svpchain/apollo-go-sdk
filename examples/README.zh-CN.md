# 命令行示例

[English](README.md) | 简体中文

在 Go SDK 仓库中运行，使用 Go 1.22 及以上。首次执行 go run 会下载 go.mod 中的依赖。

## 环境配置

`examples/trading` 默认从 shell 中读取 APOLLO_* 环境变量，可通过调用方自己的配置文件导出。命令行同名参数优先于环境配置。

| 环境变量 | 对应参数 | 说明 |
| --- | --- | --- |
| APOLLO_RPC_URL | — | Apollo HTTP(S) JSON-RPC 地址 |
| APOLLO_ACCOUNT | -account | 非零账户数字 ID |
| APOLLO_GENESIS_HASH | — | 可信创世哈希，签名操作必填 |
| APOLLO_AGENT_SEED | — | 已授权 agent 的 32 字节 seed 十六进制 |
| APOLLO_AGENT_SLOT | -slot | agent 槽位，默认 1 |
| APOLLO_COIN | -coin | 市场完整 ticker；查单时为空表示全部市场 |
| APOLLO_PRICE | -price | 十进制价格字符串 |
| APOLLO_SIZE | -size | 基础币数量字符串 |
| APOLLO_ORDER_ID | -order-id | 目标订单 ID，纯数字，不带 n |

配置不包含默认账户或私钥。查单不需要 agent seed；设置可信创世哈希时，查单也会核对目标链。每次运行交易操作会重新准备签名，保存实际提交那次的 nonce 和交易哈希。

## 1. 查询链和市场

```bash
go run ./examples/info
```

只需要 APOLLO_RPC_URL，打印链身份、高度和市场列表。

## 2. 准备或提交订单

```bash
# 读取环境中的账户、市场、价格和数量，默认准备 Gtc 限价买单
go run ./examples/trading -action order

# 真实发送订单
go run ./examples/trading -action order -submit
```

可以传 -sell 改为卖出。完整交易配置必填，价格和数量必须符合市场精度。准备成功不代表最终成交；提交后检查执行状态，并查询挂单和成交。

## 3. 查询挂单

```bash
go run ./examples/trading -action orders
go run ./examples/trading -action orders -coin BTC-USD
go run ./examples/trading -action orders -order-id "$APOLLO_ORDER_ID"
```

默认 action 为 orders。可按市场和订单 ID 筛选，输出订单 ID、市场、方向、价格、剩余数量及持仓槽位。价格和数量使用用户可读十进制格式。

## 4. 撤销指定订单

```bash
# 准备撤单签名
go run ./examples/trading -action cancel -order-id "$APOLLO_ORDER_ID"

# 真实发送撤单并查询撤单后的挂单
go run ./examples/trading -action cancel -order-id "$APOLLO_ORDER_ID" -submit
```

执行成功后输出 `Still open after cancellation: false`，然后可重复查单确认。撤单响应中的 nonce 属于新撤单动作，目标订单 ID 保持不变。

## 5. 修改指定订单

```bash
# 准备修改价格和数量
go run ./examples/trading -action modify -order-id "$APOLLO_ORDER_ID" -price 65100 -size 0.001

# 真实提交修改
go run ./examples/trading -action modify -order-id "$APOLLO_ORDER_ID" -price 65100 -size 0.001 -submit
```

改单先撤旧单再建新单，新单失败时旧单可能已撤。成功替换后从执行结果的 order_id 获取新 ID，后续查询和撤单使用新 ID。

## 结果和异常

交易默认只准备并签名，打印哈希、nonce 和 Not broadcast。显式 -submit 才广播。

提交后分别输出接收状态、执行状态及执行详情。未确认成功时工具以非零状态退出，并保留可用于对账的 account / nonce / hash；不会自动重发。执行成功和订单实际成交分别判断。

## 其他示例

- `examples/offline`：使用临时生成的 key 离线签名。
- `examples/order`：单笔下单示例，通过 -account、-coin、-price、-size 显式传订单参数。
- `examples/trading`：按 -action 选择完整命令行测试流程。

```bash
go run ./examples/trading -h
go test ./...
go test -race ./...
go vet ./...
```

仓库测试使用本地模拟节点与测试密钥，目标链测试由调用方显式运行。
