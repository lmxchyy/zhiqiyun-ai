# CR-2026-OC-009 应用发布变更单（V1.3.2 RC1）

## 变更目标

在生产库 079–096 与 Preflight 已通过的前提下，部署运营中心 V1.3.2 RC1 应用版本，完成启动、健康检查与只读/无资金冒烟；**保持全部真实业务开关关闭**。

## 发布版本

| 项 | 值 |
| --- | --- |
| 应用版本 | `v1.3.2-rc1` |
| Git Tag | `v1.3.2-rc1` |
| Git Commit | `efe5ba73ca0f12f9a95e9ea7cddf06f465f30c17` |
| 发布分支 | `codex/channel-ecosystem-v132-phase3` |
| 相对生产基线 | 含生产 commit `27dde3b4e`（invite page style）之上的 OC/渠道冻结包 |

## 允许范围

1. 固化并核验待发布应用版本  
2. 生成发布包、版本号、Git Commit 与校验哈希  
3. 准备并执行本应用发布变更  
4. 部署新应用版本  
5. 启动、健康检查与只读冒烟  
6. 保持真实业务开关关闭  

## 严禁事项

- 开启真实 Canary / 写入白名单 / 开启比例放量  
- 开启退款重试、UNKNOWN 核验、奖励释放调度器  
- 发起真实支付或退款  
- 修改商业规则金额  
- 再执行数据库迁移  
- 修改已验收的资金与 Saga 逻辑  

## 发布前门禁（本地已核验）

- [x] 发布 Commit 与 `v1.3.2-rc1` 一致  
- [x] 工作区无未提交业务代码  
- [x] `go build ./...` 通过  
- [x] `go test ./...` 全量通过  
- [ ] 发布包 SHA-256 已记录（部署构建后回填）  
- [ ] 生产不读取 TEST/DEVELOPMENT 配置（部署后核验）  
- [ ] WECHAT_VIRTUAL 退款模式 MANUAL 或 UNSUPPORTED（部署后核验）  
- [ ] `manual_refund_auto_approval=false`  
- [ ] 三个调度器全部 false  
- [ ] rollout `mode=SHADOW`，`real_switch_enabled=false`，`percentage_rollout_enabled=false`  
- [ ] 所有白名单为空  

## 部署约束

- `MIGRATION_FILES` 必须为空（禁止自动迁移）  
- 不修改 rollout / 白名单 / 商业规则金额  
- 不启用任何 `XIANZHI_PRODUCTION_OPERATION_CENTER_*_ENABLED=true`  
- 回滚目标：镜像 `local/xianzhi-ai-platform:27dde3b4` / commit `27dde3b4e`  

## 部署后只读冒烟清单

1. API 健康检查  
2. 数据库连接检查  
3. 089–096 结构识别  
4. OperationCenterRuntime 装配检查  
5. Provider 映射检查  
6. 管理查询接口检查  
7. 审核/退款写接口仅验证权限拒绝与参数校验  
8. 调度器未启动检查  
9. Rollout 配置未改变检查  
10. Legacy / Shadow / Canary 健康检查  

## 部署后必须保持的安全状态

```
mode=SHADOW
real_switch_enabled=false
percentage_rollout_enabled=false
allowTenantIds=[]
allowUserIds=[]
allowOrderIds=[]
allowPackageIds=[]
refund_retry_scheduler_enabled=false
refund_verification_scheduler_enabled=false
referral_reward_release_scheduler_enabled=false
manual_refund_auto_approval=false
```

## 审批

- 变更单编号：CR-2026-OC-009  
- 前置：CR-2026-OC-008 窗口已执行且 Preflight PASS  
- 本单授权范围：应用部署 + 只读冒烟；**不含首笔白名单 / Canary**  
