# 会员/代理价格方案 V2 — 角色交接执行包

> **当前总状态 = `NO-GO`**
>
> 代码与上线材料已准备，真实环境未验收。未收回填前：**禁止生产迁移、禁止生产部署启用、禁止打开任何 V2 开关。**
> 第三阶段（V132 phase3）= **OUT OF SCOPE / NO-GO**，本包不涉及。
>
> **协调人预填（2026-07-28 23:30 +08）：** 已补生产只读实读（三开关、镜像、097–100 未应用、V1 会员/代理价与 productId、V132=0）。§1 冻结 / §2 正式签字 / §3 演练 / §4 微信后台 / §5 真机仍待真人。详见 `evidence/20260728/system-snapshot.md`。工作树 SHA **≠** 冻结 SHA。**总状态保持 NO-GO。**

## 交接总览

| 顺序 | 角色 | 执行内容 | 详细手册 | 回填物 |
|---:|---|---|---|---|
| 1 | 发布负责人 | 清理工作区 → 冻结 release commit → 同 commit 构建镜像 → 记录 RepoDigest → 重算 097–100 SHA256 | `release-freeze-runbook.md` | §1 回填表 + release manifest JSON |
| 2 | DBA | 生产只读预检；阻断项全 0；签字；不自动修数据 | `dba-readonly-preflight.sql` + `dba-preflight-decision-table.md` | §2 回填表 + 预检日志 |
| 3 | DBA + 发布 | 生产备份隔离库演练 097→098→099→100；VALIDATE；再做恢复演练 | `isolated-migration-rehearsal.md` | §3 回填表 + 演练证据目录 |
| 4 | 微信负责人 | 正式/沙箱、会员/代理、正常价/¥1 道具矩阵核对 | `wechat-goods-manual-checklist.md` | §4 回填表 + 微信截图 |
| 5 | 微信 + QA | 沙箱真机 V2 quote 验收 | `sandbox-v2-quote-real-device-acceptance.md` | §5 回填表 + 订单/履约证据 |
| 6 | 发布负责人 | **全部回填通过后**才讨论开关；顺序：履约 → 普通创建 → TEST | `go-no-go-gate.md` Gate B/C | §6 回填表（当前禁止执行） |

准备期三开关必须保持 `false`：

```text
SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=false
PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=false
PRICE_PLAN_TEST_ENTRY_ENABLED=false
```

证据建议落盘目录（由各角色自建，勿提交密钥）：

```text
docs/acceptance/price-plan-v2-production-gate/evidence/<YYYYMMDD>/
  release-manifest.json
  migration-sha256.txt
  dba-preflight-<ts>.log
  rehearsal/
  wechat-goods/
  sandbox-acceptance/
```

---

## §0 制品与迁移路径（全角色共用）

### 迁移文件（工作树已有，尚未进入 Git）

| 序号 | 仓库路径 |
|---:|---|
| 097 | `database/migrations/097-member-agent-price-plan-v2.sql` |
| 098 | `database/migrations/098-price-plan-admin-governance.sql` |
| 099 | `database/migrations/099-price-plan-default-switch.sql` |
| 100 | `database/migrations/100-price-plan-test-whitelist-audit.sql` |

**现状：** 四文件存在于工作区，但 **未 tracked**；当前也 **没有** 已冻结的 SHA256 manifest。冻结制品时必须按 `release-freeze-runbook.md` §3 从 release commit 的 `git archive` **重算**，禁止用脏工作树 hash 冒充。

**工作树参考 SHA（2026-07-28，不可用于部署）：** 见 `evidence/20260728/migration-sha256-worktree-ONLY.txt`

| 序号 | 工作树 SHA256（仅参考） |
|---:|---|
| 097 | `FF49625A52D8EB7F36EC9602C007CDDDC36D10A3446E87EB3DF49F1A5CC9504A` |
| 098 | `E0D183F34084E1663B7EF5488A42A9E91D77C7C8E8497F3458AED175FCD85047` |
| 099 | `EE004036214D44830BC89A52A6B38136061384D27A76C8852127344706FF1735` |
| 100 | `B1BA89180D283BAB6AF41D829ED395229D8EB7897B3388886AE8B99A89E63833` |

### SHA256 计算（冻结后必做）

```powershell
# 见 release-freeze-runbook.md §3；关键：
# git archive → 解包 → Get-FileHash -Algorithm SHA256
# 结果写入 release-manifest.json 的 migrations[]，并交叉比对：
#   1) release commit 内文件
#   2) 镜像构建上下文
#   3) DBA 挂载到 /migrations 的只读文件
```

---

## §1 发布负责人 — 冻结制品

**执行人：** 发布负责人  
**手册：** `release-freeze-runbook.md`  
**本轮禁止擅自 commit**（除非另有发布审批单）。本清单描述「批准后」必须完成的动作。

### 1.1 冻结前必须清理的工作区噪音

当前工作区约 **130+** 条 modified/untracked，**不得** `git add .` / `git add -A` 整体提交。

| # | 清理项 | 勾选 |
|---|---|---|
| 1 | 丢弃/移出构建产物：`admin-vue/dist/**`、本地 tar、临时探测脚本 | [ ] |
| 2 | 确认 097–100 进入「审批文件清单」；编号无冲突（各仅 1 个文件） | [ ] |
| 3 | 排除第三阶段 / phase3 / V132 功能开发文件（本 release 范围外） | [ ] |
| 4 | 排除 `.env*` 真密钥、AppKey、sessionKey、数据库密码、微信截图中的敏感值 | [ ] |
| 5 | 未审批热修（视频 provider、无关合规改动等）不混入本 release；另开变更单或回退 | [ ] |
| 6 | 使用干净 checkout 或人工逐文件 `git add -- <path>`；`git status --short` 最终为空（相对冻结 tree） | [ ] |
| 7 | 已知测试失败已有修复或书面豁免，并绑定同一 release commit | [ ] |

### 1.2 执行清单

| # | 动作 | 命令/出处 | 勾选 |
|---|---|---|---|
| 1 | 形成审批过的 release commit | `release-freeze-runbook.md` §2 | [ ] |
| 2 | 同 commit 构建镜像（禁止脏工作区 build） | 同手册 §4 | [ ] |
| 3 | push 后记录不可变 `repository@sha256:...` RepoDigest | 同手册 §5 | [ ] |
| 4 | 从 `git archive` 重算 097–100 SHA256 | 同手册 §3 | [ ] |
| 5 | 写出 release manifest JSON（commit/tree/imageId/repoDigest/migrations） | 同手册 §6 | [ ] |
| 6 | 确认 compose 三开关显式 false 且可注入容器 | `go-no-go-gate.md` Gate A | [ ] |

### 1.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 日期 | 协调人系统预填 2026-07-28；**正式冻结待发布负责人审批后执行** |
| releaseCommit (40 hex) | `待冻结`（当前脏基线 HEAD=`81315774436cbe540f8e5c0891c89e03669bc044`，**不是** release） |
| releaseTree | `待冻结`（当前 tree=`3a3f69cdf45699670b507d77625b857739eb13e1`） |
| imageRef / imageId | **非冻结生产现状：** `local/xianzhi-ai-platform:ccad94057` / `sha256:71f110f7123b34387881f84b8cc66edec964e921cb146c2053c93dc1eb26af66`（容器 healthy）；**正式 release 镜像 = 未构建** |
| repoDigest (`@sha256:...`) | **无**（生产镜像 `RepoDigests=[]`，非 registry digest 部署） |
| 097 SHA256 | 工作树参考见上表；**冻结后必须从 git archive 重算** |
| 098 SHA256 | 同上 |
| 099 SHA256 | 同上 |
| 100 SHA256 | 同上 |
| 证据路径 | `evidence/20260728/migration-sha256-worktree-ONLY.txt` + `system-snapshot.md`（非正式 manifest） |
| 单项结论 | **`NO-GO`**（未冻结；脏工作区约 134 条；097–100 未入 Git；无 RepoDigest） |
| 签字 | 待发布负责人 |

**单项 PASS 不等于生产 GO。**

---

## §2 DBA — 生产只读预检

**执行人：** DBA（只读账号）  
**脚本：** `dba-readonly-preflight.sql`  
**判定：** `dba-preflight-decision-table.md`  
**硬规则：** 任何异常只报告；**不自动修数据、不跑 DDL/DML。**

### 2.1 命令

```powershell
$evidence = 'docs/acceptance/price-plan-v2-production-gate/evidence/<date>/dba-preflight-<UTC>.log'
psql 'service=xianzhi_prod_readonly' -X -v ON_ERROR_STOP=1 `
  -f 'docs/acceptance/price-plan-v2-production-gate/dba-readonly-preflight.sql' 2>&1 |
  Tee-Object -FilePath $evidence
# 退出码非 0 → NO-GO
```

密码只用 DBA 自管 `PGSERVICE`/`.pgpass`，禁止进命令行/仓库。

### 2.2 执行清单

| # | 动作 | 勾选 |
|---|---|---|
| 1 | 确认 `transaction_read_only=on`，库/主机/账号与审批单一致 | [ ] |
| 2 | 按判定表过完 A–R；**所有阻断项 = 0** | [ ] |
| 3 | 特别记录：V132/CANARY affected tenant = 0；giftPoints 违规 = 0 | [ ] |
| 4 | 完整日志留存；计算证据文件 SHA256 | [ ] |
| 5 | DBA + 复核人签字 | [ ] |

### 2.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 复核人 / 时间 | **协调人只读点查** 2026-07-28 23:22+08（`docker exec` + `BEGIN … READ ONLY`）；**正式 DBA + 复核人签字 = 待签** |
| 库身份（脱敏） | `zhiqiyun` / `zhiqiyun_prod` / Postgres `16.14` / `transaction_read_only=on` / 容器 `zhiqiyun-ai-prod-postgres-1` |
| 脚本退出码 | **非正式全量脚本**（未跑完整 `dba-readonly-preflight.sql`）；点查 SQL 在 giftPoints 子查询因表缺失曾 ERROR 后已改安全查询 |
| 硬阻断非零项（区段+简述） | **097–100 未应用**（6 张 V2 表 missing；097 订单列 0；098/099/100 特征列 false）。无编号 `schema_migrations` 表。`MIGRATION_FILES=` 空。主机 migrations 目录无 097–100 文件。 |
| V132 行数 | **`0`**（仅存在 `SHADOW`/`enabled=t`/`real_switch_enabled=f` 1 行） |
| giftPoints(V2) | **系统无 / N/A**（`xz_price_plans` 不存在；不能计为 0 通过，应视为门禁暂不适用直至 097） |
| V1 价格基线（只读） | 会员 `plan_ai_creator_996` **99600** 分；代理 `plan_agent_join_996` **100** 分（异常偏低，待价格确认） |
| 证据文件路径 + SHA256 | `evidence/20260728/system-snapshot.md` + `.json`（非正式 DBA 预检日志） |
| 单项结论 | **`NO-GO`**（未做正式预检签字；且 097–100 未落地，不满足 Gate A 迁移前置） |
| 签字 | 待 DBA + 复核人 |

---

## §3 DBA + 发布 — 隔离迁移演练

**执行人：** DBA 主导，发布协助提供冻结迁移包  
**手册：** `isolated-migration-rehearsal.md`  
**顺序：** 097 → 098 → 099 → 100（隔离 PostgreSQL 16，**禁止生产 DSN**）

### 3.1 输入

- [ ] 生产备份文件 + 备份 SHA256  
- [ ] 冻结 release 的 `git archive` 解包根目录（含 097–100）  
- [ ] `release-manifest.json` 中的迁移 SHA（演练前交叉比对）  
- [ ] 身份匹配 `*_rehearsal_YYYYMMDDHHMM` 的空库  

### 3.2 执行清单

| # | 动作 | 勾选 |
|---|---|---|
| 1 | 恢复备份到隔离库；记录恢复耗时/库大小 | [ ] |
| 2 | 迁移前跑只读预检 | [ ] |
| 3 | 按序执行 097→100；逐文件记录耗时、锁等待 | [ ] |
| 4 | 记录订单数/金额等数据基线（前后不变） | [ ] |
| 5 | 验证 NOT VALID 约束清单（见演练手册 §VALIDATE；共 16 个） | [ ] |
| 6 | 同副本重放 097→100，记录第二次耗时 | [ ] |
| 7 | **再做一次**备份→恢复演练（第二副本或同等证明） | [ ] |
| 8 | 特别关注 100 对 `xz_audit_logs` 非 CONCURRENTLY 索引的锁耗时 | [ ] |

### 3.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 日期 | **待 DBA + 发布**（无冻结迁移包前禁止开练） |
| 备份 SHA256 | 无 |
| 使用的 releaseCommit / 迁移 SHA 是否一致 | **否**（尚无冻结 release） |
| 097/098/099/100 耗时（秒） | 未执行 |
| 最大锁等待 | 未执行 |
| 基线（订单数/金额）前后 | 未执行 |
| VALIDATE 失败项 | 未执行 |
| 重放结果 | 未执行 |
| 第二次恢复演练 | 未执行 |
| 证据目录 | `evidence/20260728/rehearsal/`（目录待建）；生产规模点查见 `system-snapshot.md`（orders≈49，非演练结果） |
| 单项结论 | **`NO-GO`**（未执行；依赖 §1；生产侧已确认 097–100 未应用） |
| 签字（DBA / 发布） | 待签 |

---

## §4 微信负责人 — 核对道具

**执行人：** 微信支付负责人 + 价格负责人（双人）  
**手册：** `wechat-goods-manual-checklist.md`  

### 4.1 强制分离

| 维度 | 要求 |
|---|---|
| 环境 | 正式 / 沙箱 **分开**，禁止交叉绑定 |
| 业务 | 会员 / 代理 **分开** |
| 价格 | 正常价 / ¥1 TEST（100 分）**分开**，独立 productId |
| 一致性 | productId、offerId、mode、environment、价格分 **完全一致** |

生产 TEST 默认 **不创建**（见商品矩阵末两行 = 默认 NO-GO）。

### 4.2 执行清单

| # | 动作 | 勾选 |
|---|---|---|
| 1 | 按矩阵填完 SANDBOX/PRODUCTION × MEMBER/AGENT × NORMAL/TEST | [ ] |
| 2 | 微信后台按 productId 定位（禁止只凭商品名） | [ ] |
| 3 | 核对强制等式：quote = 方案 = 绑定 = 本地商品 = 微信后台价（差 1 分即 NO-GO） | [ ] |
| 4 | 证明 good.offerId/mode 与运行时 offer/AppKey/AppID 同一套（代码不自动证明） | [ ] |
| 5 | 截图 + 双人签字；密钥只记 Secret 版本，不写明文 | [ ] |

### 4.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 复核人 / 时间 | **待微信 + 价格负责人**（后台不可代登）；协调人已填系统侧 ID/价格 |
| 绑定的 releaseCommit / RepoDigest | 无（§1 未冻结） |
| 矩阵完成行数 / 失败行 | 系统字段已填 6 行可查基线；**微信发布状态/截图/双人签仍待人工**；生产 TEST 两行默认 NO-GO；沙箱 TEST productId=**系统无** |
| 系统已填关键值 | MEMBER productId=`MEMBER_YEAR_996` 价 **99600**；AGENT productId=`AGENT_JOIN_996` 价 **100**；offerId 运行时=`1450579876`；mode=`short_series_goods`；AppID=`wx42428e761551a7fb`；V2 pricePlan/binding/good=**系统无** |
| 跨环境或 productId 复用问题 | V1 正式/沙箱 **共用同一 productId**（仅 env 0/1 分行）。会员本地码 `MEMBER_PRO_YEAR_996` ≠ 微信 `MEMBER_YEAR_996`；代理本地码 `AGENT_STANDARD_996` ≠ 微信 `AGENT_JOIN_996`；代理本地价 **100 分** vs 996 命名道具 |
| 证据路径 | `evidence/20260728/system-snapshot.md`；微信截图目录 `evidence/20260728/wechat-goods/`（待建） |
| 单项结论 | **`NO-GO`**（系统基线已填，微信后台与双人签未完成；无 V2） |
| 签字 | 待双人签 |

---

## §5 微信 + QA — 沙箱真机验收

**执行人：** QA 主导，微信负责人协同  
**手册：** `sandbox-v2-quote-real-device-acceptance.md`  
**硬规则：** 必须走 **V2 quote**（`quoteId`）；旧 `productCode` 脚本 **不算** 证据。

### 5.1 前置（全部勾选后才开测）

| # | 前置 | 勾选 |
|---|---|---|
| 1 | 沙箱服务 = 冻结 commit + RepoDigest | [ ] **未满足** — 无冻结 commit；RepoDigest 空 |
| 2 | 沙箱库已 097–100 且约束证据通过 | [ ] **未满足** — 生产库 V2 表均不存在（097–100 未应用） |
| 3 | `WECHAT_VIRTUAL_PAY_ENV=sandbox` | [ ] **未满足** — 当前运行时为 `production` |
| 4 | §4 沙箱道具矩阵 PASS | [ ] **未满足** — 矩阵仍 NO-GO（缺微信后台核） |
| 5 | V132=0、giftPoints=0 | [ ] 点查 **V132=0**；V2 giftPoints 计数=**系统无/N/A**（无 `xz_price_plans`）；禁止用 V1 `grant_points` 冒充门禁通过 |

验收窗口临时开开关时仍按：**履约 → 普通创建 → TEST**；测完收回 `false`。

### 5.2 必测清单

| # | 用例 | 结果 |
|---|---|---|
| 1 | MEMBER NORMAL：quote → 下单 → 真机付 → status/sync → V2 履约一次 | **未测** |
| 2 | AGENT NORMAL：同上 | **未测** |
| 3 | MEMBER TEST ¥1：`goodsPrice=100`、独立 productId | **未测** |
| 4 | AGENT TEST ¥1：同上，且不再走 99600 | **未测** |
| 5 | 重复/并发微信回调：权益/Token/分润各一次 | **未测** |
| 6 | 回调丢失后官方查单补偿：仍只履约一次 | **未测** |
| 7 | quote 后白名单失效：下单拒绝，不改正式价 | **未测** |
| 8 | 价格差 1 分：`PRICE_PLAN_WECHAT_PRICE_MISMATCH` | **未测** |
| 9 | U0 无白名单请求 TEST：403 | **未测** |
| 10 | V1 历史订单回归仍可用 | **未测** |

### 5.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 微信 / 后端复核 / 日期 | **待 QA + 微信**（前置 §1/§3/§4 未 PASS；系统点查见 `system-snapshot.md`） |
| 体验版版本 / 真机与基础库 | 系统无 / 待人工 |
| 失败用例编号 | 全部未测（1–10）；前置 1–5 均未满足 |
| 证据路径 | `evidence/20260728/sandbox-acceptance/`（待建） |
| 单项结论 | **`NO-GO`**（前置未满足，禁止开测；禁止为开测打开生产 V2 开关） |
| 签字 | 待签 |

---

## §6 发布负责人 — 最后才讨论启用开关

**当前禁止执行本节启用动作。** 仅当 §1–§5 全部 `PASS` 且 `go-no-go-gate.md` Gate A/B（及如需则 Gate C）签字后，另开变更单。

### 6.1 启用顺序（不可颠倒）

```text
1) SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=true   # 先开 V2 履约
2) 健康检查通过后
   PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=true       # 再开普通订单创建
3) 单独审批 Gate C 后
   PRICE_PLAN_TEST_ENTRY_ENABLED=true                  # 最后开 TEST 入口
```

### 6.2 持续阻断

| 条件 | 动作 |
|---|---|
| 任一 V132/CANARY affected tenant | **保持创建关闭** |
| 任一候选方案 `giftPoints > 0` | **保持创建关闭** |
| §1–§5 任一项未回填或 NO-GO | **禁止讨论启用** |
| 第三阶段需求未另批 | **继续 OUT OF SCOPE** |

### 6.3 回填表（启用窗口专用，现在填 N/A）

| 字段 | 回填 |
|---|---|
| 变更单号 | N/A（当前 NO-GO） |
| 生产三开关实读（2026-07-28） | 容器内三项均为 **UNSET** → `boolEnv` 生效 **false**；`.env` 亦未写入这三项。**未开启。** |
| 履约开启时间 / 操作人 | N/A — **未开启**（保持 `SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED` 生效 false） |
| 创建开启时间 / 操作人 | N/A — **未开启**（保持 `PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED` 生效 false） |
| TEST 开启时间 / 操作人 | N/A — **未开启**（保持 `PRICE_PLAN_TEST_ENTRY_ENABLED` 生效 false） |
| pricing health blockedIssueCount | N/A（未开启用窗口；且 097 未应用无 V2 health 对象） |
| 最终 GO/NO-GO | **NO-GO**（直至门禁全过） |

事故回退顺序见 `go-no-go-gate.md`「事故回退顺序」。

---

## §7 回填汇总 → 最终 GO/NO-GO

| 角色包 | 结论 | 证据齐备 | 签字日期 |
|---|---|---|---|
| §1 发布冻结 | **NO-GO** | 工作树参考 SHA + 生产镜像现状（无 RepoDigest） | 2026-07-28 预填（非正式） |
| §2 DBA 只读预检 | **NO-GO** | 有协调人 `system-snapshot`；无正式预检日志/签字；097–100 未应用 | 待 DBA |
| §3 隔离迁移演练 | **NO-GO** | 无 | 待 DBA+发布 |
| §4 微信道具 | **NO-GO** | 系统 ID/价格已填；微信后台截图与双人签无 | 待微信 |
| §5 沙箱真机 | **NO-GO** | 前置 FAIL（无冻结 digest / V2 未迁 / 运行时非 sandbox / 矩阵 NO-GO） | 2026-07-28 |
| §6 开关启用 | **禁止** | 生产三开关实读 UNSET→false | — |

**汇总规则：** 任一项空、NO-GO 或不完整 → 总状态保持 **`NO-GO`**。全部 PASS 后，才允许在 `go-no-go-gate.md` 最终签字栏提议生产变更。

**本轮协调人结论（2026-07-28）：总状态 = `NO-GO`。** 可分发执行；不可启用；不可开 V2 开关。

---

## §8 材料缺口（截至交接日）

| 缺口 | 影响 | 谁补 |
|---|---|---|
| 无已审批 release commit / tag | 不能构建可部署镜像 | 发布 |
| 097–100 未入 Git；无冻结 SHA manifest | DBA 无法拿到不可变迁移包 | 发布 |
| 无 RepoDigest（生产亦为本地 tag） | 生产不能按 digest 部署 | 发布 |
| 生产 097–100 **未应用**（已只读确认） | V2 表不存在；不能开 creation/TEST | 发布冻结后由 DBA 按门禁执行（本包禁止现开） |
| 正式 DBA 全量预检日志/签字未齐 | Gate A 阻断 | DBA |
| 隔离迁移/恢复演练未跑 | 锁预算与基线未知 | DBA+发布 |
| 微信后台发布状态/截图/双人签 | 不能宣称道具 PASS | 微信 |
| 代理生产价 `price_cents=100` 未确认 | 可能与审批正式价不符 | 价格负责人 |
| 沙箱真机未跑 | Gate B/C 阻断 | 微信+QA |
| 部署脚本仍可能 `up -d --build` | digest 发布路径未批准前 NO-GO | 发布/运维（书面路径或改脚本另批） |
| offerId/mode↔AppKey 无自动证明 | 必须双人人工闭环 | 微信 |
| 2F 已知失败/导航债务 | 修复或书面豁免 | 应用负责人 |
| phase3 退款/补偿/补发 | 若生产硬依赖则 Gate B 继续 NO-GO | 业务+应用（本包不做） |

---

## 分发说明（给协调人）

1. 把本文件 + 同目录手册发给三位负责人（发布 / DBA / 微信）；QA 跟微信领 §5。  
2. 各自只改自己的回填表与 `evidence/<date>/` 产出，完成后通知协调人勾 §7。  
3. **不要**在未冻结前让 DBA 用工作树 SQL；**不要**在 §5 前开生产开关。  
4. 全部回填齐之前，对外口径统一：**NO-GO。**
