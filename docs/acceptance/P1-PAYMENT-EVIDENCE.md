# P1 Payment Evidence

- **采集时间**：2026-09-09
- **结论**：`BLOCKED / NOT VERIFIED`

## 已确认

- 代码中存在支付回调、官方查单补偿、人工补发和统一 `GrantOrderEntitlements` 路径。
- 本地支付相关 Go package tests 可编译/通过部分单元测试。
- 本轮新增虚拟支付 fulfillment record 投影和管理员重试分流代码。

## 阻断证据

1. `go test ./...` 中 pricing/payment PostgreSQL 测试因固定 fixture `127.0.0.1:55441` 不可连接失败。
2. 使用本地 PostgreSQL `54321` 尝试运行虚拟支付生命周期时，数据库缺少 `xz_order_settlement_engine_decisions` 迁移表。
3. 当前没有同一 release 绑定的真实支付 sandbox/生产白名单回调记录。

## 缺失验证

- payment callback signature / amount / product snapshot
- duplicate callback idempotency
- official query compensation
- manual grant retry
- `paid -> fulfillment -> ledger` 一致性
- refund / release / retry 对账
- `paid_without_fulfillment=0`
- `fulfilled_without_payment=0`

## 需要环境

- 与当前 release migrations 匹配的独立 PostgreSQL test database
- 微信虚拟支付 sandbox 或内部白名单真实账号
- 回调、查单、人工补发权限
- 不包含 Secret 值的订单号、事件 ID、账本和 fulfillment 证据

## 状态

`BLOCKED_EXTERNAL_PAYMENT_AND_DATABASE_FIXTURE`
