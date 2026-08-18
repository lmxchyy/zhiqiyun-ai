# Git 分支治理 Phase 3：Identity 安全与迁移只读审计

审计日期：2026-08-15  
唯一基线：`main` @ `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`  
缓存远端基线：`origin/main`、`gitee/main` 均为 `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`  
审计模式：只读；未 fetch、未测试、未修改业务代码、未切换/合并/删除任何分支或 worktree  
历史清理状态：全部继续标记为 `DEFERRED_BY_ENVIRONMENT`

## Executive conclusion

Identity 旧分支不是当前 main 的可合并升级包。四个 Identity ref 中，只有 `codex/protect-identity-phase2-2-deployment-gates-20260814` 完整包含其余 Identity 工作，适合作为唯一的历史取证/迁移来源；真正实施仍必须从最新 `main` 新建分支并选择性重建，禁止直接 merge、rebase 或整体 cherry-pick 这三个旧提交。

当前 main 已具备会话/refresh、生产禁用不安全 auth fallback、管理员中间件 fail-closed、企业租户授权和 Connector `connector_key` 租户解析等基础能力，也包含晚于 Identity 分支的账号 refresh 防串号、账号合并锁与积分账户语义、运行时投影 migration 088。与此同时，main 仍明确缺少登录抗爆破/BCrypt 容量保护、运营中心敏感资料隔离、客户创建字段的存储层防御、完整商业身份合并阻断、Identity worker 发布门禁等高价值能力。

旧 migration `072/073` **不再适合原样应用**：当前 main 已存在不同用途的同号 migration，且 migration 序列已到 `107`；旧 072 的多数投影表已被 migration 088 覆盖，旧 073 的索引仍有性能价值，但必须先完成大小写重复账号审计，并与新的有界登录查询一起以全新 migration 编号发布。

最终建议：**保留 protect-identity 作为唯一历史候选，淘汰其余三个 ref 的独立开发地位；在最新 main 上选择性重建。**

`PHASE 3 IDENTITY STATUS: READY_FOR_PLAN`

## 1. Gate 0 — 治理对象数量变化

### 1.1 数量时间线

| 时点 | 本地分支 | Worktree | 变化解释 |
| --- | ---: | ---: | --- |
| Phase 2 报告 | 28 | 21 | 审计基准 |
| Phase 2A 报告 | 29 | 22 | 新增 `codex/ppt-v2-phase0-contract` 及 `E:/code/work/ppt-v2-phase0-contract` |
| Phase 3 实时检查 | 30 | 22 | 同一 PPT worktree 后续切到新建的 `codex/ppt-v2-phase1-vertical-slice`；Phase 0 分支仍保留，因此分支再加 1，worktree 数不变 |

### 1.2 Phase 2 → Phase 2A 的新增对象

| 项目 | 事实 |
| --- | --- |
| Branch | `codex/ppt-v2-phase0-contract` |
| 创建时间 | 2026-08-15 00:43:54 +08:00 |
| Reflog 来源 | `branch: Created from main` |
| HEAD | `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c` |
| Merge-base / base | 与 main 相同；ahead 0 / behind 0 |
| Worktree | `E:/code/work/ppt-v2-phase0-contract`，目录创建时间同为 00:43:54 |
| 初始用途 | 从命名、后续文件和 Phase 1 延续关系判断，为 PPT v2 Phase 0 合同/架构工作，不是 Git 治理过程产物 |
| 当前占用 | 已不再附着该分支；该分支当前无 worktree、无独有 commit |
| 是否保留 | 本阶段不删除。它没有独立代码价值，但是否清理由 PPT 工作负责人在 WIP 收口后确认 |

### 1.3 Phase 2A 之后的实时变化

| 项目 | 事实 |
| --- | --- |
| 新 Branch | `codex/ppt-v2-phase1-vertical-slice` |
| 创建时间 | 2026-08-15 08:45:15 +08:00 |
| Reflog 来源 | `branch: Created from HEAD`；当时 HEAD 为 main/Phase 0 的 `6ee5b36f...` |
| HEAD / base | `6ee5b36f...`；ahead 0 / behind 0，尚无 commit |
| Worktree | 复用 `E:/code/work/ppt-v2-phase0-contract`，没有新增第 23 个 worktree |
| 当前用途 | PPT v2 Phase 1 vertical slice；已有服务、存储、HTTP adapter、contract、renderer 与测试 WIP |
| 未提交状态 | 最终快照 staged 0；4 个 tracked modified：`backend-go/internal/app/ppt/postgres.go`、`service.go`、`package.json`、`package-lock.json`；另有 10 个 untracked 状态项：PPT relation test、3 个 HTTP vertical-slice/artifact 文件、`contracts/`、`docs/architecture/`、`packages/ppt-v2/` 及 3 个 Phase 1 contract/golden/renderer tests |
| 是否治理产生 | 否。文件内容与分支命名均表明这是并行产品开发，而非 Phase 2/2A 清理动作 |
| 是否保留 | **YES**。这是活跃业务 WIP，删除或移动具有明确数据丢失风险 |

治理注意：worktree 路径仍叫 `ppt-v2-phase0-contract`，但当前关联 Phase 1 分支，容易造成误判。该 WIP 的 untracked 文件集合在本次只读审计期间仍发生变化，进一步证明它正在被并行开发使用；最终快照仍为 4 modified + 10 untracked、staged 0。仅建议负责人后续在 WIP 安全提交后统一命名；本阶段不改名、不移动、不删除。

## 2. 审计方法与证据边界

本次以以下只读证据交叉判断：

- `git for-each-ref`、`git branch -vv`、`git worktree list --porcelain`、各 worktree `git status --short`；
- `merge-base --is-ancestor`、双向 ancestor、`git cherry main <branch>`、`main...branch` ahead/behind；
- 三个 Identity 独有提交的逐文件 diff，而不是按分支名称推断；
- main 在共同基线 `8c3ea795...` 之后的相关提交、当前源码、migration 和发布配置；
- 对 auth source → control → sink、租户边界、敏感数据输出、数据库状态变更、proxy/client-IP 链路进行反证检查。

限制：未连接生产数据库，无法确认真实 schema、历史 migration 执行清单、大小写重复账号或数据量；未读取 GitHub 仓库级 self-hosted runner/外部 PR 策略；严格只读要求下未运行会创建测试缓存或临时数据的测试。相关结论分别标记为需要生产前预检或人工确认。

## 3. Identity Branch Inventory

共同 merge-base：`8c3ea79575af70acd8e89244bea0c391935c6993`（2026-07-22 18:28:55，`fix(generation): sanitize draft model parameters`）。以下 ref 均为本地分支，当前缓存 remote-tracking refs 中没有同名远端分支。

| Branch | HEAD / 时间 | Worktree | Dirty | 相对 main ahead / behind | 实际功能与状态 | 后续角色 |
| --- | --- | --- | --- | ---: | --- | --- |
| `codex/identity-phase2-1-security` | `dc7efceacd805efa73970fe5a3798697f413005c` / 2026-07-22 23:03 | `E:/code/work/先知AI-identity-phase2-1` | clean | 1 / 215 | 客户字段、商业账号合并、运营中心敏感资料、canonical auth、Identity 并发一致性、旧 072 | 已被 protect 完整包含；只保留历史参考 |
| `codex/identity-phase2-2-deployment-gates` | `867cf82e807a9ae8c5950171aed9e8985a224f37` / 2026-07-23 18:40 | 无 | N/A | 2 / 215 | 有界登录查询、单机限流、BCrypt 并发、worker/write gate、旧 073、性能测试 | 与 release-readiness 完全同 HEAD；无独立开发价值 |
| `codex/identity-phase2-2-release-readiness` | `867cf82e...` / 2026-07-23 18:40 | `E:/code/work/先知AI-identity-phase2-2` | clean | 2 / 215 | 与 deployment-gates 字节级同一提交 | 重复 ref；只保留历史参考 |
| `codex/protect-identity-phase2-2-deployment-gates-20260814` | `15889e5dbe5b8659db46c0e5a13a882ae7872ee2` / 2026-08-14 01:20 | `E:/code/work/先知AI-identity-phase2-2-deploy` | clean | 3 / 215 | 完整包含前两阶段，并新增 Redis/HMAC 限流、bounded limiter、readiness、CI、HAProxy/compose、环境 gate | **唯一 Identity 历史主候选；仅作迁移来源，不作代码基线** |

### 3.1 分支关系

```text
8c3ea795 (共同基线)
└─ dc7efce  phase2.1 security
   └─ 867cf82  phase2.2 deployment/release readiness
      ├─ ref: codex/identity-phase2-2-deployment-gates
      ├─ ref: codex/identity-phase2-2-release-readiness
      └─ 15889e5  protect identity deployment gates
```

验证结果：

- phase2.1 是 phase2.2 与 protect 的祖先；
- deployment-gates 与 release-readiness 是同一个 HEAD，双向 ancestor 且差异 0/0；
- phase2.2 是 protect 的祖先，protect 独有 `15889e5...` 一个提交；
- `git cherry main protect` 对三个提交均返回 `+`，说明 main 没有 patch-equivalent 吸收全部提交；
- 但直接 `main..protect` 会涉及 1231 个文件、10999 insertions、158466 deletions，这是 215 个 main-only 提交造成的历史分叉，不是可接受的功能合并范围；
- 仅共同基线到 protect 的 Identity 链也已涉及 96 个文件、6626 insertions、902 deletions，必须按能力拆分。

## 4. Main 在 2026-07-22 之后的等价或后续实现

| 当前 main 能力 | 证据 | 对旧 Identity 的影响 |
| --- | --- | --- |
| refresh 后账号一致性校验 | `666c226d...`；`packages/shared-auth/src/index.ts` 的 `AuthAccountMismatchError` | 旧 shared-auth 不具备，禁止整文件回退 |
| 账号合并锁与积分账户合并 | `374d1fb0...`；`backend-go/internal/httpserver/postgres_store.go` | phase2.1 的并发锁已有部分后续实现，但商业身份阻断仍缺 |
| 合并时保留目标 V2 订单 | `054a8fca...` | 旧账号合并代码不了解后续订单语义，不能覆盖当前实现 |
| 运行时投影基线 | migration `088-runtime-projection-baseline-completion.sql` | 已覆盖旧 072 的 account-merge、model route、system settings、billing events、AI state、API channel/key 表 |
| 企业租户/组织/角色与模型调用授权 | `enterprise_runtime.go`、`user_rbac_store.go` | main 已是更新的租户边界；旧 auth/RBAC 只能移植局部逻辑 |
| Connector 按 connector key 解析企业并隔离任务/上下文 | `connector_api.go`、`connector_postgres.go` | main 的当前租户链必须保持，旧分支不能成为基线 |
| 生产禁用 auth/mock fallback 与基础 secret 校验 | `config.go:ValidateProduction` | main 已 fail-closed；旧 env 配置只补 Identity 专属 gate |
| 运营中心 089–096 迁移与生产 release gate | `internal/app/operationcenter/production_release_gate.go` | 商业身份合并阻断必须覆盖新退款、奖励、钱包、审核数据，旧 guard 不完整 |

## 5. Identity Capability Matrix

状态含义：`PRESENT` 已存在；`PARTIAL` 部分满足；`MISSING` 缺失；`SUPERSEDED` 已被 main 的后续实现替代；`CONFLICTING` 与当前架构/发布方式冲突；`UNKNOWN` 需要运行环境证据。

| Capability | main | phase2.1 | phase2.2 | protect | 推荐与证据 |
| --- | --- | --- | --- | --- | --- |
| access/refresh session、logout | PRESENT | PRESENT | PRESENT | PRESENT | 保留 main；`auth_api.go`、session store |
| refresh 防账号串号 | PRESENT | SUPERSEDED | SUPERSEDED | SUPERSEDED | 只保留 main 的 `AuthAccountMismatchError` |
| 登录有界 DB 查询 | MISSING | MISSING | PRESENT | PRESENT | 重建 `GetUserByLoginAccount`；main 当前 `login()` 仍加载 `AdminData()` |
| BCrypt 并发容量与 dummy hash | MISSING | MISSING | PRESENT | PRESENT | 选择性重建 `passwordSlots`、unknown-account dummy bcrypt |
| brute-force / distributed account limit | MISSING | MISSING | PARTIAL | CONFLICTING | phase2.2 仅单实例且 map 未有界；protect 有 Redis/HMAC，但其 HAProxy 拓扑导致 app IP 聚合，不能照搬 |
| 管理员鉴权 fail-closed | PRESENT | PRESENT | PRESENT | PRESENT | main 未开启 RBAC 时仅允许 SUPER_ADMIN，不存在已确认的直接 bypass |
| canonical RBAC 注入登录/auth-me 响应 | PARTIAL | PRESENT | PRESENT | PRESENT | main 的 `/user/profile` 使用 DB RBAC，但 login/auth-me 仍按 legacy projection，需在 main 上重建 |
| tenant / owner / enterprise scope | PRESENT | PARTIAL | PARTIAL | PARTIAL | main 的 enterprise/connector 链为准；旧分支仅可参考个人/商业角色逻辑 |
| 客户 role/plan/referral 受保护字段 | PARTIAL | PRESENT | PRESENT | PRESENT | main update 已保护，create 未拒绝 `planId` 且 store 接受受保护字段；移植 store-level guard |
| 商业身份账号合并阻断 | PARTIAL | PRESENT | PRESENT | PARTIAL | main 有锁但 guard 过窄；旧 guard 不覆盖 main 后续 089–096/企业数据，需扩展重建 |
| Identity command per-user serialization | PARTIAL | PRESENT | PRESENT | PRESENT | 按当前 transaction flow 选择性补齐 downgrade/worker 路径 |
| downgrade worker CAS、attempt、失败可观测性 | PARTIAL | PARTIAL | PRESENT | PRESENT | 需要新的 forward migration 和 worker rollout gate |
| Identity write kill switch | MISSING | MISSING | PRESENT | PRESENT | 可迁移设计，不可覆盖当前 config/startup 主流程 |
| 运营中心敏感 profile 分级读取/遮罩 | MISSING | PARTIAL | PARTIAL | PARTIAL | 旧分支有独立权限、mask 与 patch；仍缺 typed schema/at-rest protection，需重建 |
| login lower(email/name) 索引 | MISSING | MISSING | PRESENT | PRESENT | 概念有效；先查大小写重复，再使用新 migration 编号 |
| frontend 环境 fail-closed/artifact scan | PARTIAL | PARTIAL | PRESENT | PRESENT | main App release check 仅部分覆盖；旧实现硬编码域名，需配置化重建 |
| startup config validation | PARTIAL | PARTIAL | PRESENT | PRESENT | 合并 Identity 专属严格 bool/int/HMAC 校验到 main 当前 `ValidateProduction` |
| DB/schema readiness | MISSING | MISSING | PARTIAL | PRESENT | protect 检查旧 071/072/073 签名；应改为当前 schema capability checks |
| multi-replica runtime/HA gate | PARTIAL | MISSING | PARTIAL | CONFLICTING | 设计可参考；compose 与 main 已大幅漂移，client-IP 链有缺陷 |
| CI performance gate | PARTIAL | MISSING | PARTIAL | CONFLICTING | 测试阈值可参考；self-hosted runner 的 PR 信任边界需先批准 |
| migration 072/073 可直接应用 | MISSING | CONFLICTING | CONFLICTING | CONFLICTING | 同号冲突、部分被 088 替代、缺生产数据预检；禁止原样应用 |

## 6. Auth / RBAC / Identity boundary review

### 6.1 当前 main 的正向控制

- `/auth/login`、`/auth/me`、`/auth/refresh`、logout/logout-all 均存在，session 同时支持 Redis/本地实现；refresh token 旋转和账号一致性检查存在。
- 生产环境显式拒绝 `XIANZHI_DEV_AUTH_FALLBACK`、`XIANZHI_ALLOW_INSECURE_AUTH_TOKEN`、mock login。
- admin middleware 验证 session 与 active user；`XIANZHI_ENFORCE_RBAC` 未开启时不是放行，而是只允许 `SUPER_ADMIN`。
- `GetUserRoleAccess` 会验证 enterprise tenant/member/organization 状态，并同步商业角色的 active/expired 状态。
- enterprise model call 重新验证 tenant、member、organization、service state 和 permission；Connector 外部事件由 path 中的 `connectorKey` 反查企业，而不是信任消息体 tenant ID。

### 6.2 当前 main 仍有的 Identity 缺口

1. `authAPI.login` 对 PostgreSQL 仍先加载整份 `AdminData()` 再查账号，无请求/账号失败限流、无 BCrypt semaphore、未知账号无统一 dummy bcrypt。公共登录入口可被用于 CPU/DB 放大和口令爆破。
2. login/auth-me 响应仍走 legacy `authResponse()`：`currentRole` 固定 USER，商业 workspace/permission 由 profile 投影推断；而 `/user/profile` 才读取 canonical DB RBAC。前端登录后把 auth 响应标记为 loaded，通常不会立即强制刷新 profile，因此两个身份视图可能漂移。
3. `workspaceFromAuth` 的 main 版本仍通过 legacy `user.role` 回推 agent/admin，且 shared type 不包含 protect 分支的 `operation` workspace；这是旧 Identity 中被修正、随后未进入 main 的行为。
4. 当前 admin operation-center list/profile 返回原始 `contactInfo` 与 `settlementProfile`；list GET 落到 `admin.read`，profile GET 使用 `identity:operation-profile:update`。migration 071 同时将 update 权限授予 `ADMIN`，未做到敏感读取独立授权。
5. 客户 create handler 只拒绝 `role`、`referredBy`，不拒绝 `planId`；PostgreSQL/memory store 也接受 `Role/Status/PlanID/ReferredBy`。update 已有更完整保护，但缺 store-level defense-in-depth。
6. main 的账号合并只阻止双方同时拥有代理/运营中心身份；无法阻止“商业账号 + 普通账号”、历史商业身份、分润/提现/退款/奖励/未完成 Identity 请求等高风险合并。

## 7. Migration 072 / 073 审计

### 7.1 编号与执行模型

当前 main 已有：

- `072-publish-commercial-service-agreements.sql`
- `073-restore-published-wechat-virtual-product-ids.sql`
- 后续 migration 已到 `107-storage-multipart-upload.sql`

生产 compose 的 migrate 容器通过人工提供 `MIGRATION_FILES` 逐文件执行，没有看到统一的 schema migration ledger。重复编号会破坏治理可读性；`IF NOT EXISTS` 也无法证明已有对象的列、constraint 或语义与预期一致。因此旧 Identity 072/073 不得以原文件名加入 main。

### 7.2 旧 072 分项结论

| 072 内容 | 当前 main | 结论 |
| --- | --- | --- |
| `xz_identity_downgrade_requests.attempt_count` | 缺失 | 仍有价值；以新 forward migration 添加，并补 `>= 0` constraint/worker rollout 检查 |
| account merge / model route / system settings / billing events / AI state / API channel/key 投影表 | migration 088 已创建 | `SUPERSEDED`；禁止重复搬运 |
| 把 USER role context 修复为 AGENT/OPERATION | main 运行时已有 `syncCommercialRBACForUser` 与显式 current-context/role switch | 不能全局盲写；需先定义“升级后是否自动切工作台”的当前产品规则 |
| `identity:operation-profile:view-sensitive` | 缺失 | 仍有价值；使用新 migration，并同时收紧 route/response |
| 修改 071 rollback，保留运营中心敏感列 | main 当前 rollback 仍会 DROP 四列 | 安全意图正确；不要假设旧 rollback 可无损执行，应建立备份优先的 forward recovery/runbook |
| 072 rollback | 故意保留业务表和 attempt column，只回退 permission | 数据保护优先但不是严格逆操作；需在发布计划中明确 roll-forward，而不是宣称可完全 rollback |

### 7.3 旧 073 分项结论

旧 073 创建：

- `idx_xz_users_login_email_lower`
- `idx_xz_users_login_name_lower`

当前 main 不存在这两个索引，且 phase2.2 的 `GetUserByLoginAccount` 会使用 `lower(email)` / `lower(name)`，因此索引思想仍适用。但不能直接应用：

- 当前 `xz_users.email` 的 UNIQUE 是大小写敏感，可能已存在仅大小写不同的重复账号；旧 073 只建普通索引，不能消除认证歧义；
- 是否应使用唯一 functional index，必须先对生产数据做只读 duplicate audit；
- `CREATE INDEX CONCURRENTLY` 不能包在 transaction 中，发布器需明确单独步骤；
- 应先建兼容索引、验证 valid，再启用有界登录查询；回滚应以 roll-forward 为主；
- 必须分配当前 main 之后的唯一 migration 编号，不能重用 073。

### 7.4 最终回答

`MIGRATION 072/073 APPLICABLE AS-IS: NO`

- 072：**部分概念仍适用**（attempt_count、敏感读取权限、安全 rollback 原则），大部分投影表已被 088 替代。
- 073：**条件适用**，仅在生产重复账号审计、索引策略评审和新登录查询计划通过后，以新编号重建。

## 8. Protect branch 中 main 真正缺失的能力

以下是高价值迁移候选，不等于可直接 cherry-pick：

1. 有界账号查询、unknown-account dummy bcrypt、BCrypt 并发 semaphore、登录阶段诊断。
2. 账号级共享失败计数的 HMAC key 思路，以及 local limiter 容量上限/过期清理。
3. customer create/update 的 store-level protected-field guard 与可信创建 API。
4. 商业身份账号合并的历史/关系/订单/分润/提现/未完成请求阻断框架。
5. operation-center 列表遮罩、敏感读取独立 permission、patch-only settlement 更新和 unknown-field 拒绝。
6. canonical auth response 使用数据库角色上下文，避免 legacy role/profile 授权漂移。
7. downgrade per-user lock、数据库时钟、状态 CAS、attempt 计数、worker audit/可观测性。
8. Identity command kill switch、worker 单独启用、readiness schema capability check。
9. 前端环境 fail-closed、生产 artifact 禁止值扫描、非生产环境标识。
10. 固定资源条件下的 login/health/plans/auth-me 性能门槛与结构化测试证据。

以下内容 **main 已有或已有更晚实现**，不得从旧分支覆盖：session/refresh 主链、refresh 账号一致性、enterprise/connector tenant isolation、账号合并中的当前积分/订单语义、migration 088 投影表、当前 production config 的支付/存储/Connector 配置、当前 Docker/compose 和 graceful shutdown 主流程。

## 9. 不能直接迁移的内容

| 内容 | 原因 | 安全处理方式 |
| --- | --- | --- |
| 三个 Identity commit 整体 merge/cherry-pick | 与 main 漂移 215 个提交，endpoint diff 1231 文件 | 从 main 新建 Phase 4 分支，按能力手工重建 |
| 旧 072/073 文件 | 编号冲突、部分 superseded、无生产数据预检 | 新编号 forward migration + schema/data preflight |
| protect 的 `compose.prod.yml` / Dockerfile / env example | main 后续 compose 已有约 243 行差异及新服务/配置 | 在当前 compose 上最小化加入 gate，不覆盖文件 |
| HAProxy + app IP limiter 原方案 | HAProxy 到 API 的连接使 `r.RemoteAddr` 成为代理地址；应用忽略 `X-Forwarded-For`，100 次失败可聚合阻断全部用户 | 先定义可信 proxy 链；边缘 IP 限流和应用 account 限流分责，客户端 IP 只从受信 hop 获取 |
| self-hosted PR performance workflow | `pull_request` 会 checkout PR merge SHA 并在 self-hosted runner 执行 Docker/仓库代码；若允许非可信 PR，runner 风险可达 CRITICAL | 人工确认 PR 信任策略、runner 隔离、environment approval 后重建 workflow |
| 旧 shared-auth 整文件 | 会丢失 main 后续 refresh 账号一致性修复 | 只补 operation/canonical workspace 逻辑并做回归 |
| 旧 commercial merge guard 原查询集合 | 不认识 main 后续 operation-center refund/reward/wallet、enterprise/Connector 关联 | 按当前 schema 重做 blocker inventory |
| settlement 任意 JSON map | 旧方案只做 response mask，不能证明敏感字段 at-rest 安全 | 先确定 typed DTO、加密字段、密钥轮换与审计策略 |
| 硬编码 `ai.zs-kjhn.cn` 环境 policy | 环境/域名治理被写死在构建脚本 | 由明确、受审计的环境配置提供 allowlist，生产缺失时 fail-closed |

## 10. Validated security findings

### P3-ID-01 — Public login lacks brute-force and BCrypt capacity controls — HIGH

- Source：公开 `/api/v1/auth/login` 的 account/password。
- Control：main 仅做 JSON decode、账号查找和密码比较；没有 IP/account failure limiter、BCrypt semaphore 或 dummy hash。
- Sink：整份 `AdminData()` 读取与 BCrypt CPU 工作。
- Impact：密码爆破、CPU/DB 资源耗尽、合法登录延迟/不可用。
- Counterevidence：生产 insecure token/mock fallback 已被禁止；这降低 auth bypass 风险，但不缓解登录资源攻击。
- 建议：在 main 上重建有界查询、BCrypt semaphore、统一失败时序、分布式 account limiter 与受信边缘 IP limiter。

### P3-ID-02 — Operation-center sensitive profile is exposed under broad admin permissions — HIGH

- Source：admin operation-center list/profile GET。
- Control：list 使用通用 `admin.read`；profile GET 使用 `identity:operation-profile:update`，migration 071 授予 `ADMIN`。
- Sink：原始 `contactInfo` 与任意 `settlementProfile` JSON 被序列化返回。
- Impact：联系方式、银行卡/结算标识甚至错误存入 JSON 的 secret 泄漏。
- 建议：独立 sensitive-read permission、默认摘要/遮罩、typed patch、字段级加密/审计策略。

### P3-ID-03 — Account merge does not block all commercial identities and financial history — HIGH

- Source：admin账号合并请求。
- Control：main 只在双方都具备同类商业身份时阻止；后续 transaction lock 只解决并发，不解决业务资格。
- Sink：用户、订单、积分/历史关联被合并。
- Impact：商业身份、分润、提现、退款、奖励或企业归属错绑；审计链不可逆混淆。
- 建议：以 current schema 重建 fail-closed blocker inventory，并要求人工身份迁移流程。

### P3-ID-04 — Customer creation accepts protected plan fields below handler boundary — MEDIUM

- Source：有 `admin.write` 的 customer create payload 或任何内部 store caller。
- Control：handler 未拒绝 `planId`；store 接受 Role/Status/PlanID/ReferredBy，缺 defense-in-depth。
- Sink：`xz_users` plan/status/role projection 与权益语义。
- Impact：绕过受控会员/支付/身份流程；当前需已认证管理员或内部调用，因此低于公开登录问题。
- 建议：store 只接受可信 customer create DTO，外部 create 禁止受保护字段。

### P3-ID-05 — Auth response and canonical RBAC profile can diverge — MEDIUM

- Source：login/auth-me 与 `/user/profile` 两条身份读取路径。
- Control：前者使用 legacy projection，后者读取 DB role context；前端登录后通常将 profile 标记 loaded。
- Sink：client workspace/currentRole/permissions 与页面路由。
- Impact：错误工作台、陈旧权限 UI、角色切换/回归问题；未发现可直接绕过后端 tenant permission 的证据。
- 建议：统一由 canonical access 生成 auth response，同时保留 main 的 refresh 账号一致性修复。

### P3-ID-06 — Old migration/direct-merge path can corrupt release governance — CRITICAL

- Source：将 protect 分支、旧 072/073 或旧 compose 直接并入 main。
- Control：重复 migration 编号、215-commit 漂移、投影表重复、无 migration ledger、旧发布文件不了解当前服务。
- Sink：生产 schema、角色上下文、容器拓扑和发布历史。
- Impact：重复/遗漏 migration、不可证明的 schema、服务配置回退、大范围代码删除或安全能力倒退。
- 建议：明确禁止 direct merge/cherry-pick；只在 main 上重建。

### P3-ID-07 — Protect HAProxy topology makes application IP limiter global — HIGH

- Source：通过 protect HAProxy 访问 API 的失败登录。
- Control：HAProxy 设置 `X-Forwarded-For`，但 backend `loginClientIP()` 明确只读 `r.RemoteAddr`；backend server 行没有 `send-proxy`。
- Sink：应用 local IP failure bucket，阈值 100/15m。
- Impact：攻击者可让所有经同一 gateway 的用户共享并耗尽一个 IP bucket，形成全局登录拒绝服务。
- 建议：该拓扑禁止原样迁移；先审批可信代理模型并分离 edge-IP 与 application-account limiter。

### P3-ID-08 — Self-hosted PR performance workflow trust boundary — HIGH / conditional CRITICAL

- Evidence：workflow 在 `pull_request` 上使用 self-hosted runner，checkout PR SHA，并执行 Docker 与仓库脚本。
- 未知项：仓库是否允许 fork/非可信贡献者发起 PR、runner 是否一次性隔离。
- Impact：若 PR 不完全可信，可能执行攻击者代码并影响持久 runner/同网资源，等级升为 CRITICAL。
- 建议：Phase 4 前由仓库管理员确认 runner/PR policy；未确认前不得照搬 workflow。

## 11. Risk Matrix

| 风险类别 | 等级 | 当前判断 | 关键证据/处置 |
| --- | --- | --- | --- |
| Auth bypass | MEDIUM | 未发现 main 管理员中间件直接 fail-open；但 auth response 与 canonical RBAC 漂移 | `governance.go`、`auth_api.go`；统一身份来源 |
| Session/token | LOW | main session/refresh 与生产 fallback 禁令健全，且有 refresh 防串号 | 保留 main，不回退 shared-auth |
| Tenant isolation | HIGH | main 当前链路较强；旧分支直接覆盖会破坏 enterprise/connector 最新边界 | `enterprise_runtime.go`、`connector_api.go`；只局部移植 |
| Rate limiting | HIGH | main 缺失；protect 的 proxy/client-IP 链有全局 DoS 缺陷 | 重新设计 trusted proxy 与 limiter 分工 |
| Sensitive admin data | HIGH | operation-center 原始 contact/settlement 暴露 | 独立权限、遮罩、加密/typed DTO |
| Commercial merge | HIGH | guard 过窄，后续金融表未覆盖 | current-schema fail-closed blockers |
| Migration | CRITICAL | 072/073 同号冲突、部分 superseded、无实时 schema/data 证据 | 新编号、只读 preflight、forward-only rollout |
| Deployment gate | HIGH | main 缺 Identity 专用 gate；protect gate 与当前 compose 漂移 | 在当前发布链最小化重建 |
| Environment validation | MEDIUM | main 有基础生产校验，缺 Identity HMAC/worker/build artifact gate | 合并到当前 ValidateProduction/build scripts |
| Proxy/runtime | HIGH | protect 应用 IP limiter 看见 gateway 地址；容量值也未按当前生产确认 | 先审批拓扑、压测与容量模型 |
| Compatibility | CRITICAL | direct diff 1231 文件，215 main-only commits | 禁止直接 merge/cherry-pick |
| Regression | CRITICAL | 旧 auth/compose/config 会覆盖 main 后续账号、支付、存储、企业/Connector 变更 | 新 main 分支 + protected-surfaces 全回归 |

## 12. Duplicate / superseded Identity refs

| Ref | 独立价值 | 结论 |
| --- | --- | --- |
| `identity-phase2-1-security` | 无；内容被 867/protect 完整包含 | 历史参考，未来不继续开发 |
| `identity-phase2-2-deployment-gates` | 无；与 release-readiness 同 HEAD | 重复 ref |
| `identity-phase2-2-release-readiness` | 无；与 deployment-gates 同 HEAD，且被 protect 包含 | 重复 ref/历史 worktree |
| `protect-identity-phase2-2-deployment-gates-20260814` | 有；是唯一包含全部三提交的 source ref | 唯一保留的 Identity 治理候选 |

本阶段不清理任何 ref/worktree。即使旧 ref 无独立开发价值，清理仍属于后续显式批准事项，且当前环境问题未解除；状态统一为 `DEFERRED_BY_ENVIRONMENT`。

## 13. Main Health Review for Identity

### 结论

`main` **适合作为今后所有 Identity 工作的唯一代码基线**，但不能据此认为 Identity 已达到完整生产安全状态。

适合作为基线的理由：

- main 包含 215 个 Identity 分支没有的后续集成提交；
- 当前企业、Connector、支付、积分、运营中心退款/奖励、存储与 shutdown 语义只在 main 完整；
- main 有更晚的 refresh 防串号和账号合并数据语义；
- cached origin/gitee main 与本地 main 一致。

需要 Phase 4 优先修复的原因：

- 公共 password login 缺少抗爆破和容量保护；
- 运营中心敏感资料权限/输出边界不安全；
- 商业账号合并与 customer protected fields 缺 defense-in-depth；
- canonical RBAC 与 auth response 不统一；
- downgrade worker 和 Identity 写操作缺生产 kill switch/readiness gate；
- main 的 071 rollback 仍具有数据删除风险。

## 14. 最终必须回答的问题

1. **Identity 当前真正应该保留哪个分支？**  
   `codex/protect-identity-phase2-2-deployment-gates-20260814`。它是唯一完整包含三阶段工作的 ref，但只能作为历史迁移来源。

2. **哪些旧 Identity 分支已经没有独立开发价值？**  
   `codex/identity-phase2-1-security`、`codex/identity-phase2-2-deployment-gates`、`codex/identity-phase2-2-release-readiness`。后两者同 HEAD；三者均被 protect 包含。

3. **protect-identity 中 main 真正缺失的能力有哪些？**  
   有界登录、BCrypt/dummy hash/失败限流、customer store guard、完整商业账号合并阻断、operation-center 敏感读取隔离、canonical auth response、Identity worker CAS/attempt/write gate/readiness、Identity 专用环境/性能发布 gate。

4. **哪些内容不能直接迁移？**  
   三个完整 commit、072/073 原文件、旧 compose/Docker/env、HAProxy IP 限流拓扑、self-hosted PR workflow、旧 shared-auth 整文件、旧 commercial guard 查询集合和任意 JSON settlement 模型。

5. **migration 072/073 是否仍适用于当前 main？**  
   原样不适用。072 仅 attempt_count、敏感读取权限、安全 rollback 原则仍有价值；073 索引有条件适用，必须新编号并先做 duplicate/schema preflight。

6. **是否存在安全倒退风险？**  
   是，且直接合并风险为 CRITICAL：可能回退 refresh 防串号、企业/Connector tenant isolation、订单/积分合并语义、当前 compose/config，同时引入 proxy 全局登录 DoS 与重复 migration。

7. **建议直接放弃、选择性 backport、重建在最新 main，还是同步后继续？**  
   **重建在最新 main，并选择性 backport 能力设计。** 不建议同步旧分支后继续，不建议整体 cherry-pick；也不应直接放弃其中已验证有价值的安全能力。

8. **后续最安全的实施路径是什么？**  
   以最新 main 新建独立 Phase 4 分支，先做生产 schema/data 只读 preflight，再按小能力切片重建；每片独立 migration、测试、回滚/roll-forward 和人工审批，最后才评估旧 Identity ref 清理。

## 15. Phase 4 实施建议（仅建议，未执行）

推荐顺序：

1. **建立最新 main 的实施基线**：新建 Phase 4 分支/worktree；禁止以 protect 为 base，禁止 cherry-pick 三个旧提交。
2. **先做数据库只读 preflight**：盘点生产 migration 执行记录、072/073/088 对象签名、大小写重复 email/name、operation-center settlement 数据形态、商业账号与 089–096 关联。
3. **P0 敏感数据与账号完整性**：先重建 operation-center safe summary/sensitive permission/typed patch、customer store guard、current-schema commercial merge blockers。
4. **P0 登录安全**：有界 lookup + 索引、dummy bcrypt、semaphore、account limiter；先明确受信 proxy 模型，不能复用 protect 的 app-IP 方案。
5. **统一 canonical auth**：让 login/me/profile 共用 DB access 解析，同时保留 refresh account mismatch、enterprise context 和 Connector tenant boundary。
6. **Identity worker/gate**：新 migration 增加 attempt/constraint；引入 command kill switch、单 worker 所有权、schema capability readiness、audit/metrics。
7. **发布门禁重建**：在当前 compose/Docker/deploy.sh 上增量实现；self-hosted CI 仅在 runner 信任边界通过人工批准后启用。
8. **验证后再治理旧 ref**：完成安全、迁移演练、回归和生产批准后，单独发起 Identity 历史分支/worktree 清理阶段。

必须等待人工批准的事项：生产 schema/data 检查、new migration 编号与执行顺序、敏感字段加密模型、trusted proxy 列表、限流阈值、worker ownership、HA 容量、self-hosted runner 策略、任何 merge/发布以及任何旧 branch/worktree 清理。

## 16. Safety Verification

- `main` HEAD 在报告写入前保持 `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`。
- main tracked/staged working tree 无变化；仅已有治理报告与本报告为 untracked docs。
- 三个 Identity worktree 均为 clean，未切换、未修改。
- PPT Phase 1 活跃 WIP 被识别并保护，未读取后修改、未提交、未移动。
- Safe Area 两个受保护文件仍存在且未被本阶段操作；`harmony-output-path-contract.test.mjs` 的 SHA-256 仍与 Phase 2A 相同（`FEA419...E465`），`harmony-baseline-81a66692fc.md` 的最终 SHA-256 为 `3FBC31...3758`，不同于 Phase 2A 报告的 `ED22DD...2DEE`。这是本阶段只读核验发现的外部 WIP 变化，不归因于本审计；后续阶段必须按新状态重新设 Gate，禁止覆盖或清理。
- 未执行 merge、rebase、cherry-pick、reset、stash、clean、fetch、pull、push、prune。
- 未删除 branch、worktree、tag 或 Git metadata。
- 未修改生产代码、配置、migration 或 Git 历史。

`PHASE 3 IDENTITY STATUS: READY_FOR_PLAN`

下一阶段只建议：**Phase 4：基于最新 main 的 Identity 安全重建实施计划**。本报告不自动进入 Phase 4。
