# P1 Payment Fulfillment Projection

## 问题

微信虚拟支付的权益发放路径只更新 `xz_orders` / entitlement ledger；通用 Payment Center 的 `xz_fulfillment_records` 与管理员重试路径看不到同一履约事实。

## 根因

虚拟支付与通用支付分别实现了履约状态机。`RetryFulfillment` 只能重放通用 `fulfillment_type`，无法识别微信虚拟权益。

## 修改

- 微信虚拟权益统一写入 `xz_fulfillment_records`，使用稳定的 `wechat_virtual_entitlement` 类型和订单派生 ID。
- 处理开始、成功、失败均更新履约记录；重复发放保持幂等。
- 管理员 `/payment/fulfillments/:id/retry` 识别微信虚拟权益并转入 `GrantOrderEntitlements`，其他类型继续走通用 Payment Center。
- 测试 cleanup 覆盖新增履约记录。

## 测试

```text
cd backend-go && go test -count=1 ./internal/httpserver -run '^$'
PASS（编译验证）

cd backend-go && go test -count=1 ./internal/httpserver -run 'TestWechatVirtual...'
BLOCKED：PostgreSQL fixture 127.0.0.1:55441 不可连接
```

## 状态

`IMPLEMENTED_LOCALLY / POSTGRES_INTEGRATION_BLOCKED`
