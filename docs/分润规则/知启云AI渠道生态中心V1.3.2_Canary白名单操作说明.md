# 知启云AI渠道生态中心 V1.3.2 Canary 白名单操作说明

## 当前阶段边界

- 仅允许 `tenant_id`、`user_id`、`order_id`、`package_id` 白名单。
- 当前代码中的 `package_id` 对应商业订单 `plan_id`。API 推荐使用 `allowPackageIds`，并兼容原字段 `allowPlanIds`。
- 运营中心套餐始终排除，即使命中租户、用户、订单或套餐白名单也继续走 Legacy。
- 管理 API 拒绝 `percentageRolloutEnabled=true`，不得开启比例放量。

## 开启方式

调用 `PUT /api/v1/admin/channel-ecosystem/rollout-config`，提交当前配置版本、已发布规则集及白名单：

```json
{
  "expectedVersion": 1,
  "mode": "CANARY",
  "enabled": true,
  "pinnedRuleSetId": "channel_rules_v132_default_v1",
  "pinnedRuleSetVersion": 1,
  "allowOrderIds": ["待灰度订单ID"],
  "allowUserIds": [],
  "allowTenantIds": [],
  "allowPackageIds": [],
  "percentageRolloutEnabled": false,
  "realSwitchEnabled": true,
  "reason": "V1.3.2 白名单真实结算"
}
```

建议先使用 `order_id` 单订单白名单，再扩展到账号、套餐或租户。启用前必须确认规则集状态为 `PUBLISHED`。

## 回滚方式

将配置更新为：

```json
{
  "mode": "SHADOW",
  "enabled": true,
  "percentageRolloutEnabled": false,
  "realSwitchEnabled": false,
  "reason": "停止 Canary 新订单真实切换"
}
```

回滚只影响尚未生成结算决策的新订单。已经固化 `settlement_engine=V132` 的订单继续使用原规则版本、商业快照和 V1.3.2 钱包账本处理支付重试、补发和退款。

## 上线前检查

- 默认配置仍为 Shadow。
- 规则集已发布且版本固定。
- 百分比放量关闭。
- 白名单不包含运营中心套餐。
- 单笔白名单订单完成佣金、钱包、平台收入和退款冲正核对。
