# 协调人预填状态（2026-07-28）

> **总状态仍为 `NO-GO`。**  
> 本文件记录可从仓库 + 生产只读探测采集的客观基线；**不代替** DBA / 微信 / 发布签字，不构成冻结制品，**未开启任何 V2 开关**。

## 已预填（客观事实）

| 项 | 值 |
|---|---|
| 预填人 | coordinator-assist |
| 预填时间 | 2026-07-28 23:18 → 23:30 +08:00（第二轮补系统实读） |
| 当前 HEAD | `81315774436cbe540f8e5c0891c89e03669bc044` |
| 当前 tree | `3a3f69cdf45699670b507d77625b857739eb13e1` |
| 工作区脏条目约 | 134（禁止整体 `git add -A`） |
| 097–100 Git 状态 | 全部 `??` untracked |
| 工作树 SHA256 证据 | `migration-sha256-worktree-ONLY.txt` |
| 生产系统快照 | `system-snapshot.md` / `system-snapshot.json` |
| compose / 容器三开关 | 容器内 **UNSET** → 生效 **false**（未开启） |
| 生产镜像 | `local/xianzhi-ai-platform:ccad94057` @ `sha256:71f110f7…`；**无 RepoDigest** |
| 097–100 生产应用 | **未应用**（V2 表/列全缺；主机 migrations 目录无 097–100） |
| V132 阻断行 | **0** |
| giftPoints(V2) 阻断 | **N/A**（无 `xz_price_plans`） |
| V1 会员价 / productId | `99600` / `MEMBER_YEAR_996` |
| V1 代理价 / productId | `100` / `AGENT_JOIN_996`（异常偏低，待价格确认） |
| 运行时 offerId / mode / AppID | `1450579876` / `short_series_goods` / `wx42428e761551a7fb` |
| §6 启用 | **禁止 / N/A** |

## 未预填 / 仍必须真人（不得伪造）

| 包 | 原因 |
|---|---|
| §1 release commit / RepoDigest / archive SHA | 需审批后冻结；工作树 hash 与本地镜像不可冒充 registry digest |
| §2 正式 DBA 预检签字 + 完整脚本退出码证据 | 已有协调人只读点查，非正式 `dba-readonly-preflight.sql` 全量日志 |
| §3 隔离迁移/恢复演练 | 需生产备份与隔离环境 |
| §4 微信后台发布状态/截图/双人签 | 未登录微信后台；矩阵系统字段已填 |
| §5 沙箱真机 | 需真机 + 冻结沙箱环境 + V2 |
| 代理价 100 分是否正式 | 价格负责人确认 |

## 工作树迁移 SHA256（仅参考）

| 文件 | SHA256 |
|---|---|
| 097 | `FF49625A52D8EB7F36EC9602C007CDDDC36D10A3446E87EB3DF49F1A5CC9504A` |
| 098 | `E0D183F34084E1663B7EF5488A42A9E91D77C7C8E8497F3458AED175FCD85047` |
| 099 | `EE004036214D44830BC89A52A6B38136061384D27A76C8852127344706FF1735` |
| 100 | `B1BA89180D283BAB6AF41D829ED395229D8EB7897B3388886AE8B99A89E63833` |
