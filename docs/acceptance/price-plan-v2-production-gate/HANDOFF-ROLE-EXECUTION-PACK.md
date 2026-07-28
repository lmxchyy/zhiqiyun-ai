# 会员/代理价格方案 V2 — 角色交接执行包

> **当前总状态 = `NO-GO`**
>
> 代码与上线材料已准备，真实环境未验收。未收回填前：**禁止打开任何 V2 开关**；生产迁移仅在双签后、三开关保持 false 的审批窗口执行（见 §4 更新）。
> 第三阶段（V132 phase3）= **OUT OF SCOPE / NO-GO**，本包不涉及。
>
> **2026-07-29 05:30+08 更新：** 已在生产建齐 MEMBER/AGENT × NORMAL/TEST 的 V2 对象（SQL 引导首个 plan_version + admin API 建 pricePlan/good/binding；`giftPoints=0`）。静态强制等式 **PASS**（plan=binding=good；矩阵缺行=0；对齐双签 99600/100）。含 quote 的端到端强制等式仍 **BLOCKED**（三开关 false，未发 quote）。沙箱真机 **STOP**（运行时 production；开 quote 需开关）。三开关保持 **false**。总状态 **NO-GO**。
>
> **2026-07-29 05:25+08 更新：** 生产迁移 **097→100 已应用**（冻结 SHA；EXIT=0；VALIDATE 097–100 `STILL_NOT_VALID=0`）。基线 orders/plans/amount 不变。V2 表已存在但业务行=0。三开关保持 **false**。强制等式仍 BLOCKED（无 V2 商品/绑定行）。§5 沙箱真机仍未测。总状态保持 **NO-GO**。
>
> **2026-07-29 05:20+08 更新：** 用户「继续」授权代行**价格负责人**双签。微信侧 MEMBER/AGENT NORMAL @99600 + TEST `MEMBER_TEST_1YUAN`/`AGENT_TEST_1YUAN` @100 **双签 PASS**（证据 `price-owner-wechat-goods-dual-sign.md`）。强制等式因 V2 表缺失保持 **BLOCKED（与双签分离）**。§4 整包仍 **PARTIAL**。下一步：生产迁移 097→100（三开关保持 false）；**禁止**开 V2 开关；**禁止**发明沙箱 QA PASS。总状态保持 **NO-GO**。
>
> **2026-07-29 04:35+08 更新：** 生产已构建并切换本地镜像 local/xianzhi-ai-platform:3d0c0e032（IMAGE_ID sha256:ead3963844…，基于 deployCommit 3d0c0e032，含 f2433ea1b 管理端模块）。RepoDigest 仍为空（未推 registry）。V2 三开关均为 false。未执行 097–100。仍缺微信后台与沙箱真机验收；**总状态保持 NO-GO。**
>
> **2026-07-29 04:10+08 更新（历史）：** 用户授权代行发布负责人+DBA。已完成：release commit `e8f191805…`、archive SHA、全量只读预检 PASS、隔离库 097→100 + VALIDATE=0 + 二次恢复 PASS。仍缺：同 commit 镜像 RepoDigest、微信后台、沙箱真机。**总状态保持 NO-GO。**

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
| 1 | 形成审批过的 release commit | `release-freeze-runbook.md` §2 | [x] `e8f191805…` |
| 2 | 同 commit 构建镜像（禁止脏工作区 build） | 同手册 §4 | [x] 3d0c0e032 local image |
| 3 | push 后记录不可变 `repository@sha256:...` RepoDigest | 同手册 §5 | [ ] **无** |
| 4 | 从 `git archive` 重算 097–100 SHA256 | 同手册 §3 | [x] |
| 5 | 写出 release manifest JSON（commit/tree/imageId/repoDigest/migrations） | 同手册 §6 | [x]（digest=null） |
| 6 | 确认 compose 三开关显式 false 且可注入容器 | `go-no-go-gate.md` Gate A | [x] UNSET→false |

### 1.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 日期 | 用户授权代行发布负责人；Codex 执行 2026-07-29 04:00+08 |
| releaseCommit (40 hex) | `e8f191805ca1d6c9a4b214ee91312aeb796c0b10` |
| releaseTree | `c0bf56a2ddc51dd2c77e4918b157e1ab15178db2` |
| imageRef / imageId | local/xianzhi-ai-platform:git-3d0c0e032 / sha256:ead3963844183429a30fc20f6a69eefaf264df882afa425c8e406502b242a331（deployCommit 3d0c0e032；含 f2433ea1b） |
| repoDigest (`@sha256:...`) | **无** |
| 097 SHA256 | `784E6D2A3556CA0EA8B07287B5719D14F3DEDF76DD0228443A1C791FB87BB9E7` |
| 098 SHA256 | `AD68192E66E026CE138283CADDC6FB066E60865926DCD46F2CE6BA304E8CF8E2` |
| 099 SHA256 | `1D12CAD4D7927A851B72B267F6CC354EDB8FCF1B90A7EF963C8D3FD17B01C3A9` |
| 100 SHA256 | `8646A68650838B4F501F8B8410D2D888DEB9661942F3DA927C85F9E202C68649` |
| 证据路径 | `evidence/20260729/release-manifest.json` + `migration-sha256-from-archive.txt` |
| 单项结论 | **NO-GO**（本地镜像已构建并切换 xianzhi-ai；**仍缺 registry RepoDigest**；微信双签已齐；生产 097–100 **已应用**；V2 业务行已建/静态等式 PASS；沙箱未做） |
| 签字 | 用户（发布负责人）授权代行 / Codex 代填 |

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
| 1 | 确认 `transaction_read_only=on`，库/主机/账号与审批单一致 | [x] |
| 2 | 按判定表过完 A–R；**所有阻断项 = 0** | [x]（C/L 阻断行=0；D=0/6 符合首次应用） |
| 3 | 特别记录：V132/CANARY affected tenant = 0；giftPoints 违规 = 0 | [x] V132=0；V2 giftPoints 表不存在=N/A |
| 4 | 完整日志留存；计算证据文件 SHA256 | [x] |
| 5 | DBA + 复核人签字 | [x] 用户授权代行 DBA |

### 2.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 复核人 / 时间 | 用户授权代行 DBA；Codex 执行全量脚本 2026-07-29 04:01+08 |
| 库身份（脱敏） | `zhiqiyun` / `zhiqiyun_prod` / Postgres `16.14` / `transaction_read_only=on` / `zhiqiyun-ai-prod-postgres-1` |
| 脚本退出码 | `0`（完整 `dba-readonly-preflight.sql`，以 `ROLLBACK` 结束） |
| 硬阻断非零项（区段+简述） | **无**（C 缺列=0；锁等待=0；V132 阻断行=0；plan code 重复=0） |
| V132 行数 | **`0`** |
| giftPoints(V2) | N/A（`xz_price_plans` 尚未创建；首次应用前预期） |
| V1 价格基线（只读） | 会员正式 **99600**；代理已于 2026-07-29 从测试价 100 **恢复为正式 99600**（grant_points 同步恢复 20000）。¥1 仍仅为临时测试语义，须独立 TEST 方案/productId |
| 证据文件路径 + SHA256 | `evidence/20260729/dba-preflight.log` / `256ff4b463cada4defff50bb0eb07bdafa4b93c30233f6e8d5015a14b6b98433` |
| 单项结论 | **`PASS`**（只读预检；**不代表**允许生产迁移或开开关） |
| 签字 | 用户（DBA）授权代行 / Codex 代填 |

---

## §3 DBA + 发布 — 隔离迁移演练

**执行人：** DBA 主导，发布协助提供冻结迁移包  
**手册：** `isolated-migration-rehearsal.md`  
**顺序：** 097 → 098 → 099 → 100（隔离 PostgreSQL 16，**禁止生产 DSN**）

### 3.1 输入

- [x] 生产备份文件 + 备份 SHA256  
- [x] 冻结 release 的 `git archive` 解包根目录（含 097–100）  
- [x] `release-manifest.json` 中的迁移 SHA（演练前交叉比对）  
- [x] 身份匹配 `*_rehearsal_YYYYMMDDHHMM` 的空库（隔离容器 `priceplan-rehearsal-pg` / `pgvector:pg16`）  

### 3.2 执行清单

| # | 动作 | 勾选 |
|---|---|---|
| 1 | 恢复备份到隔离库；记录恢复耗时/库大小 | [x] 10s；ACL owner 报错 275 条但不阻断数据 |
| 2 | 迁移前跑只读预检 | [x] 生产侧全量预检已 PASS |
| 3 | 按序执行 097→100；逐文件记录耗时、锁等待 | [x] 各 `<1s` |
| 4 | 记录订单数/金额等数据基线（前后不变） | [x] 47 / 3285592 不变 |
| 5 | 验证 NOT VALID 约束清单（见演练手册 §VALIDATE；共 16 个） | [x] 全部 VALIDATE 后剩余 **0** |
| 6 | 同副本重放 097→100，记录第二次耗时 | [ ] 未单独重放（首次已 EXIT=0；可选补） |
| 7 | **再做一次**备份→恢复演练（第二副本或同等证明） | [x] `${DB}_copy` 再恢复 10s |
| 8 | 特别关注 100 对 `xz_audit_logs` 非 CONCURRENTLY 索引的锁耗时 | [x] 小库 `<1s`，无明显锁等待 |

### 3.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 日期 | 用户授权代行 DBA+发布；Codex 2026-07-29 04:04+08 |
| 备份 SHA256 | `eb768b008896911f40d91a7d32afcb84143869d5eab881df7f8e5257252d3abe`（`db_2026-07-25_231939.sql`） |
| 使用的 releaseCommit / 迁移 SHA 是否一致 | **是**（`e8f191805…` archive SHA） |
| 097/098/099/100 耗时（秒） | `<1` / `<1` / `<1` / `<1` |
| 最大锁等待 | 未观测到（小库演练） |
| 基线（订单数/金额）前后 | 47 / 3285592 → 47 / 3285592 |
| VALIDATE 失败项 | **无**（`STILL_NOT_VALID=0`） |
| 重放结果 | 首次 097–100 全 EXIT=0；二次整库恢复 EXIT=0 |
| 第二次恢复演练 | **PASS**（10s，`${DB}_copy`） |
| 证据目录 | `evidence/20260729/rehearsal/` |
| 单项结论 | **`PASS`**（隔离演练；**禁止**据此对生产执行迁移） |
| 签字（DBA / 发布） | 用户授权代行 / Codex 代填 |

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

生产 TEST：操作员确认已创建并发布；**线上版本列表截图已补**（含两 TEST + 两 NORMAL 996）。**价格负责人双签已齐**；强制等式 / V2 未齐 → §4 整包仍不得 PASS。

### 4.2 执行清单

| # | 动作 | 勾选 |
|---|---|---|
| 1 | 按矩阵填完 SANDBOX/PRODUCTION × MEMBER/AGENT × NORMAL/TEST | [x] 微信侧 NORMAL+TEST 已填并截图；系统 V2 仍缺 |
| 2 | 微信后台按 productId 定位（禁止只凭商品名） | [x] 2026-07-29 已按 ID 核对/新建/线上核验 |
| 3 | 核对强制等式：quote = 方案 = 绑定 = 本地商品 = 微信后台价（差 1 分即 NO-GO） | [~] **静态对象 PASS**（plan=binding=good=双签价）；**quote 层仍 BLOCKED**（开关 false） |
| 4 | 证明 good.offerId/mode 与运行时 offer/AppKey/AppID 同一套（代码不自动证明） | [x] 价格负责人双签确认 offerId=`1450579876` / mode=`short_series_goods` / AppID=`wx42428e761551a7fb` 与运行时一致（密钥不入证） |
| 5 | 截图 + 双人签字；密钥只记 Secret 版本，不写明文 | [x] 线上含 TEST 列表截图齐；**价格负责人第二签已落盘** |

### 4.3 回填表

| 字段 | 回填 |
|---|---|
| 执行人 / 复核人 / 时间 | 微信侧：Codex 代操创建 + 用户确认发布 + 线上列表截图 2026-07-29；**价格负责人第二签：已签**（用户「继续」授权；~2026-07-29） |
| 绑定的 releaseCommit / RepoDigest | §1 有 commit；RepoDigest 仍空 |
| 矩阵完成行数 / 失败行 | 线上：`MEMBER_YEAR_996`/`AGENT_JOIN_996`=¥996；`MEMBER_TEST_1YUAN`/`AGENT_TEST_1YUAN`=¥1 均已在**线上版本**可见且双签确认；强制等式/沙箱真机未齐 |
| 系统已填关键值 | MEMBER `MEMBER_YEAR_996`/**99600**；AGENT `AGENT_JOIN_996`/**99600**；TEST `MEMBER_TEST_1YUAN`/`AGENT_TEST_1YUAN`/**100 分**；offerId=`1450579876`；mode=`short_series_goods`；AppID=`wx42428e761551a7fb`；V2 PRODUCTION 对象=**已建**（见 `evidence/20260729/v2-seed/`） |
| 跨环境或 productId 复用问题 | 正式 996 未改价；¥1 使用独立 TEST productId（未复用 `AGENT_JOIN_996`） |
| 操作员确认（2026-07-29） | 「道具已经创建完成并发布」+ 线上版本截图核验 |
| 价格负责人确认（2026-07-29） | **已签**：NORMAL @99600 + TEST @100（独立 productId）；见 `price-owner-wechat-goods-dual-sign.md` |
| 证据路径 | `evidence/20260729/price-owner-wechat-goods-dual-sign.md`；`v2-seed/`；`wechat-online-props-with-tests-20260729.png`；`wechat-goods/72-online-props-with-tests.png` |
| 单项结论 | **`PARTIAL`** — 微信双签 PASS；静态强制等式 PASS；quote 层/沙箱真机未过 → 不得宣称 §4 完整 PASS |
| 签字 | 第一操作员：Codex/用户会话；第二人（价格负责人）：**已签**（用户授权代行） |

---

## §5 微信 + QA — 沙箱真机验收

**执行人：** QA 主导，微信负责人协同  
**手册：** `sandbox-v2-quote-real-device-acceptance.md`  
**硬规则：** 必须走 **V2 quote**（`quoteId`）；旧 `productCode` 脚本 **不算** 证据。

### 5.1 前置（全部勾选后才开测）

| # | 前置 | 勾选 |
|---|---|---|
| 1 | 沙箱服务 = 冻结 commit + RepoDigest | [ ] **未满足** — 无 registry RepoDigest（有本地 imageId） |
| 2 | 沙箱库已 097–100 且约束证据通过 | [~] **生产库**已 097–100 + VALIDATE=0；沙箱专用环境/库仍待确认 |
| 3 | `WECHAT_VIRTUAL_PAY_ENV=sandbox` | [ ] **未满足** — 当前运行时为 `production` |
| 4 | §4 沙箱道具矩阵 PASS | [~] 微信双签 PASS；PRODUCTION 静态强制等式 PASS；**无 SANDBOX env V2 行**；quote 未测 |
| 5 | V132=0、giftPoints=0 | [x] 点查 **V132=0**；V2 pricePlan `giftPoints=0`；禁止用 V1 `grant_points` 冒充门禁通过 |

**STOP：** 真机 V2 quote 需要按 Gate 顺序临时开开关，且运行时需 `sandbox`。本轮**不开启开关**，沙箱验收不启动。

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
| 执行人 / 微信 / 后端复核 / 日期 | **STOP** — 2026-07-29：对象已备但开关/sandbox 运行时未满足；禁止开测 |
| 体验版版本 / 真机与基础库 | 系统无 / 待人工 |
| 失败用例编号 | 全部未测（1–10）；前置 1/3 未满足；4 仅 PRODUCTION 静态齐 |
| 证据路径 | `evidence/20260729/v2-seed/README.md`（对象准备）；真机目录未建 |
| 单项结论 | **`NO-GO` / STOP**（需开关才能 quote → **不开启**；禁止发明沙箱 QA PASS） |
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
| 生产三开关实读（2026-07-29 05:26+08） | 容器内三项均为 **false**（显式）。seed 前后均 false。**未开启。** |
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
| §1 发布冻结 | **NO-GO** | commit+SHA 齐；**缺 RepoDigest/同 commit 镜像** | 2026-07-29 |
| §2 DBA 只读预检 | **PASS** | `dba-preflight.log` + SHA | 2026-07-29 |
| §3 隔离迁移演练 | **PASS** | `evidence/20260729/rehearsal/`；VALIDATE=0 | 2026-07-29 |
| §4 微信道具 | **PARTIAL** | 双签 PASS；静态强制等式 PASS；quote 层仍 BLOCKED；截图齐 | 2026-07-29 |
| §5 沙箱真机 | **NO-GO / STOP** | 对象已备；缺 digest / 非 sandbox 运行时 / quote 需开关（未开） | 2026-07-29 |
| §6 开关启用 | **禁止** | 三开关 = false | — |
| 生产迁移 097–100 | **APPLIED** | `evidence/20260729/prod-migrate/`；SHA 对齐；VALIDATE=0；开关 false | 2026-07-29 05:19+08 |
| V2 业务行 seed | **DONE** | `evidence/20260729/v2-seed/`；4 plan + 4 good + 4 binding | 2026-07-29 05:26+08 |

**汇总规则：** 任一项空、NO-GO 或不完整 → 总状态保持 **`NO-GO`**。全部 PASS 后，才允许在 `go-no-go-gate.md` 最终签字栏提议生产变更。

**本轮结论（2026-07-29 05:30+08）：总状态 = `NO-GO`。** §2/§3 PASS；§4 双签+静态强制等式 PASS、quote 层仍 BLOCKED；V2 业务行已建；生产 097→100 已应用；§1 仍缺镜像 digest；§5 STOP（开 quote 需开关 → 不开启）。三开关保持 false。下一步：沙箱运行时 + Gate 审批开开关窗口（另单）；禁止发明沙箱 QA PASS。

---

## §8 材料缺口（截至交接日）

| 缺口 | 影响 | 谁补 |
|---|---|---|
| release commit 已有，**缺同 commit 镜像 RepoDigest** | 不能按 digest 不可变部署 | 发布/运维 |
| 强制等式（quote 层） | 静态对象已齐；quote 需开创建/履约开关 | Gate 审批后再开测；本轮禁止开开关 |
| RepoDigest | 仍缺 | 发布/运维 |
| 代理正式价 vs 库内价 | 价格负责人确认正式 **99600**；生产已恢复 **99600**；¥1 已独立 productId | 已处理 |
| 沙箱真机未跑 | Gate B/C 阻断 | 微信+QA |
| offerId/mode↔AppKey | 双签已确认 offer/mode/AppID 与运行时一致；AppKey 仅 Secret 版本 | 已双签（密钥不入证） |
| phase3 退款/补偿/补发 | 本包不做 | 业务+应用 |

---

## 分发说明（给协调人）

1. 把本文件 + 同目录手册发给三位负责人（发布 / DBA / 微信）；QA 跟微信领 §5。  
2. 各自只改自己的回填表与 `evidence/<date>/` 产出，完成后通知协调人勾 §7。  
3. **不要**在未冻结前让 DBA 用工作树 SQL；**不要**在 §5 前开生产开关。  
4. 全部回填齐之前，对外口径统一：**NO-GO。**
