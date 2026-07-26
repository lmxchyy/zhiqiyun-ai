# CR-2026-OC-009 应用发布结果（V1.3.2 RC1）

## 结论

**应用发布完成，只读冒烟通过；安全状态保持 SHADOW / 调度器全关 / 白名单为空。已停止，未开启首笔白名单或真实 Canary。**

具备**申请**首笔单订单 Canary 变更的条件；本阶段**不执行**该变更。

## 发布身份

| 项 | 值 |
| --- | --- |
| 应用版本 | `v1.3.2-rc1` |
| Git Tag | `v1.3.2-rc1` |
| Git Commit | `ccad9405728a2007d608fca363559a52c1710838` |
| 源码发布包 | `xianzhi-v1.3.2-rc1-ccad94057.tar.gz` |
| 源码包 SHA-256 | `367c728253eb839f995430c2f619c95bd2a6b5d21c66d6f94e2222ac941e71fd` |
| 部署镜像 | `local/xianzhi-ai-platform:ccad94057` |
| 镜像 ID (SHA-256) | `7d078f47a8c65c589f71f1fd844ed48444f13131716246e9aefa859e18b1104f` |
| 部署节点 | `ai.zs-kjhn.cn` / `VM-0-11-centos` / 容器 `zhiqiyun-ai-prod-xianzhi-ai-1` |
| 部署开始 (UTC) | `2026-07-26T21:46:15Z` |
| 部署结束 (UTC) | `2026-07-26T21:47:04Z` |
| 变更日志目录 | `/root/cr-2026-oc-009-20260726T214615Z` |

## 发布前门禁

| 检查项 | 结果 |
| --- | --- |
| 发布 Commit 与 `v1.3.2-rc1` 一致 | PASS (`ccad94057`) |
| 工作区无未提交业务代码（RC 固化时） | PASS |
| `go build ./...` | PASS |
| `go test ./...` | PASS |
| 发布包 SHA-256 已记录 | PASS |
| 生产 `XIANZHI_ENV=production`，不读 TEST/DEVELOPMENT OC 前缀 | PASS |
| WECHAT_VIRTUAL 映射 `MANUAL`（活跃退款代码路径 UNSUPPORTED） | PASS |
| `manual_refund_auto_approval=false` | PASS |
| 三个调度器 false | PASS |
| rollout `SHADOW` / real_switch false / percentage false / 白名单空 | PASS |
| `MIGRATION_FILES=`（本次未执行迁移） | PASS |

## 健康检查 / 冒烟

| # | 检查 | 结果 |
| --- | --- | --- |
| 1 | API 健康检查 | PASS |
| 2 | 数据库连接 | PASS |
| 3 | 089–096 结构识别 | PASS（≥6 关键表） |
| 4 | OperationCenterRuntime 装配 | PASS（调度器配置日志出现，无 unavailable） |
| 5 | Provider 映射 | PASS `WECHAT_VIRTUAL=MANUAL` |
| 6 | 管理查询接口（无鉴权） | PASS 全部 HTTP 401 |
| 7 | 审核/退款写接口（无鉴权/坏 token） | PASS 401；退款任务数 `0→0` |
| 8 | 调度器未启动 | PASS `enabled=false` ×3 |
| 9 | Rollout 指纹未放量 | PASS `SHADOW\|false\|false\|0\|[]\|[]\|[]\|[]` |
| 10 | Legacy / Shadow / Canary 健康语义 | PASS（SHADOW，Canary 关） |
| - | migrate no-op | PASS |
| - | 回滚镜像仍存在 | PASS `local/xianzhi-ai-platform:27dde3b4` |

## Runtime / Provider 装配

- Runtime 已装配：`worker=operation-center-runtime`
- 调度器配置日志（节选）：
  - `refund_retry enabled=false`
  - `refund_verification enabled=false`
  - `referral_reward_release enabled=false`
- Provider：`XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_PROVIDER_MAPPINGS=WECHAT_VIRTUAL=MANUAL`
- 活跃退款网关仍返回 `UNSUPPORTED`（人工退款路径），未开启自动退款

## 调度器关闭证明

```
INFO operation center scheduler configured scheduler=refund_retry enabled=false ...
INFO operation center scheduler configured scheduler=refund_verification enabled=false ...
INFO operation center scheduler configured scheduler=referral_reward_release enabled=false ...
```

## Rollout / 白名单指纹

```
SHADOW|false|false|0|[]|[]|[]|[]
```

对应：

- `mode=SHADOW`
- `real_switch_enabled=false`
- `percentage_rollout_enabled=false`
- `allowTenantIds=[]`
- `allowUserIds=[]`
- `allowOrderIds=[]`
- `allowPlanIds=[]`（仓库无 `allow_package_ids` 列；套餐维白名单为空）

## HTTP 冒烟

- `GET /api/v1/health`：OK
- 管理查询无 token：`rollout-config` / `refunds` / `shadow-differences` / `operation-centers` → **401**
- 写接口无 token：`approve` / `refunds` / `manual-submit` → **401**
- 坏 token 退款请求：401，且 `xz_operation_center_refund_tasks` 计数不变（0）

## 日志异常

- 近 30 分钟未发现 panic / fatal
- 过滤后的异常摘要为空（无资金类错误）

## 回滚验证

- 旧镜像保留：`local/xianzhi-ai-platform:27dde3b4`（`sha256:234807478f1ecb4bc3bbf54022b0d48467971564dab8cd7676a40c1a04085535`）
- 回滚步骤已落盘于服务器 smoke 目录（**未执行回滚切换**）
- 回滚约束：保持 `MIGRATION_FILES=`，仅切回旧应用镜像

## 是否具备申请首笔单订单 Canary 的条件

**是（仅具备“申请下一变更单”的条件，本阶段不得执行）：**

已满足：生产 079–096 + Preflight PASS、应用 RC 已部署、Runtime/Provider 装配、调度器关闭、SHADOW/空白名单保持、只读冒烟 PASS。

下一变更必须另开审批，且仍需显式：单订单白名单、双人确认、观察窗口；**禁止**在本结果上直接写白名单或开 Canary。

## 明确未做

- 未开启真实 Canary
- 未写入任何白名单
- 未开启比例放量
- 未开启三类调度器
- 未发起真实支付/退款
- 未修改商业规则金额
- 未再执行数据库迁移
- 未修改已验收资金与 Saga 逻辑
