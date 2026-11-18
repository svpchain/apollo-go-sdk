# 版本与运行环境

[English](PROVENANCE.md) | 简体中文

## 版本

- Module：`github.com/svpchain/apollo-go-sdk`
- Package：`apollo`
- 版本：遵循语义化版本，各版本变更见 [版本记录](CHANGELOG.zh-CN.md)
- Go：1.22 及以上

安装方式见 [快速开始](README.zh-CN.md#安装)。

## 客户端

`Info` 提供市场、盘口、行情、账户、仓位、挂单、成交、费用和历史查询。

`Exchange` 提供下单、批量交易、撤单、改单、平仓、杠杆、逐仓保证金、TWAP、分批订单及 agent 授权。

## 数据类型

交易输入的价格、数量和金额使用十进制字符串。RPC 的 64 位整数由 `Uint64` / `Int64` 保持精度，地址使用 `Address`，哈希使用 `Hash`，字节使用 `HexBytes`。

手续费汇总使用十进制整数字符串表示微 USD，适合保存和 JSON 序列化。支持的类型和方法见 [API](docs/zh-CN/API.md)。

## 接入配置

`endpoint` 使用目标环境的 Apollo HTTP(S) JSON-RPC 地址。交易配置账户 ID、signer 和可信创世哈希。历史查询使用提供 archive 索引的节点，通过 `IndexStatus()` 确认数据高度。

接入新环境时，使用测试账户完成授权、下单、查挂单、撤单和成交核对，再启用持续交易。
