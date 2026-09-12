# RC Burn-down Plan

来源：`docs/acceptance/RC-FINAL-AUDIT-REPORT.md`

## 状态

- `[ ]` 未处理
- `[~]` 本地已处理，真实证据未闭环
- `[!]` 外部环境阻断
- `[x]` 已验证关闭

```text
START_SCORE=55
START_P0=1
START_P1=9
TARGET_P0=0
TARGET_P1=0
CURRENT_P1=8
```

## 执行顺序

### P0

- [!] SEC-P0-001：历史敏感凭据轮换（需要生产 Secret/IAM 外部轮换证据，本地不伪造）
  - 本地可自动完成：扫描当前工作区、示例配置、tracked history；禁止输出或删除未提交用户凭据。
  - 外部依赖：生产 Secret/IAM/支付/Provider 凭据实际轮换及失效确认。
  - 若无外部授权，状态保持 `BLOCKED`。

### P1

- [~] REL-P1-001：release 身份收口；部署脚本已阻断 dirty/untracked；当前 checkout 仍需提交并与远端对齐
- [!] PAY-P1-001：支付回调、查单、人工补发当前 release 证据（需要真实支付环境；PostgreSQL fixture 也未就绪）
- [~] PAY-P1-002：统一虚拟支付与通用 fulfillment 投影（已补齐 fulfillment record 与管理员重试分流；PostgreSQL 集成验证 BLOCKED）
- [~] CON-P1-001：Connector durable retry / DLQ / failed state（Redis 路径已实现；真实 Redis DLQ 验证未完成）
- [~] PPT-P1-001：PPTX 最终 File Object 持久化（已实现；真实对象存储 E2E 未验证）
- [~] IMG-P1-001：图片存储失败时禁止回退 Provider URL（已实现 fail-closed；真实图片 Canary 未验证）
- [!] DR-P1-001：备份上传、远端校验、独立库恢复证据（需要生产/远端对象存储和独立恢复环境）
- [!] INFRA-P1-001：API / Outbox / Worker / RabbitMQ 运行证据（当前只有依赖容器，无可核对 API/Worker release runtime）
- [x] VIDEO-P1-001：历史视频用户体验证据（真实 H5 E2E 已验证 EXPIRED 失效卡片、无 video/旧 Provider 请求、下载禁用、重新生成入口；旧 Provider 文件本体标记 `NOT_RECOVERABLE`）

### P2

- [ ] 读取审计报告剩余 P2，按风险排序处理

## 每项完成门槛

- [ ] 代码或运维变更最小化
- [ ] 新增 `docs/acceptance/fixes/P1-*.md`（若为 P1 代码项）
- [ ] 单元测试
- [ ] 集成测试或明确 `BLOCKED`
- [ ] `git diff --check`
- [ ] 相关 Protected Surface 回归
- [ ] 更新本计划状态
- [ ] 更新 `RC-FINAL-AUDIT-REPORT.md` 状态与证据

## 外部阻断规则

以下事项不能由本地代码代理伪造完成：

- 生产 Secret/IAM/支付/Provider 凭据轮换
- 生产真实支付回调、查单和人工补发
- 生产远端备份上传及独立库恢复
- 生产 API/Worker 发布与运行态证明

这些事项必须记录为 `BLOCKED`，直到获得真实环境证据。

## 模式切换

本计划已切换到 `RC Evidence Collection Loop`：

- 证据矩阵：`docs/acceptance/RC-EVIDENCE-MATRIX.md`
- 当前闭环报告：`docs/acceptance/RC-EVIDENCE-CLOSURE-REPORT.md`
- 在外部证据到位前停止业务代码修改。
