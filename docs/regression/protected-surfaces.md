# 防回归保护面（Protected Surfaces）

改任何前端/用户工作台相关代码前先读本清单。  
**做完必须在交付说明里勾选本次已核对项**；未覆盖的留空并说明原因。

回归测试入口见各条目「验证」字段。总规则见 `.ai/CodexPrompt.md` 与 `AGENTS.md`「防回归」。

---

## 网页端用户控制台

### W1. 侧边栏可用点数 / 总点数
- **不可丢**：有消耗时「可用」与「总额」必须区分；总额为终身口径（可用 + 冻结 + 已消耗）。
- **实现锚点**：`/points/account` 返回 `total`/`totalGranted`/`totalUsed`；前端只用 `admin-vue/src/utils/sidebarPlanPoints.ts`。
- **禁止**：把 `total` 写成 `available + frozen`；在 `App.vue` 内重写点数换算。
- **验证**：
  - `go test ./internal/httpserver/ -run TestPointAccountTotalIncludesConsumedPoints`
  - `npm test -- tests/webWorkspacePointsRegression.spec.ts`（`admin-vue`）
- **核对**：`[ ]`

### W2. 平台首页首屏加载
- **不可丢**：首页可即时出壳；默认只拉少量近期任务/作品。
- **实现锚点**：`usesInstantWorkspace('userDashboard')`；`/user/dashboard` 默认 `taskLimit`/`assetLimit` ≤ 30；`moduleListQuery`。
- **禁止**：默认改回 120/300 或更大；去掉即时壳导致白屏干等大包。
- **验证**：
  - `go test ./internal/httpserver/ -run TestUserDashboardDefaultPayloadStaysSmallAndExposesTotalPoints`
  - `npm test -- tests/webWorkspacePointsRegression.spec.ts`
- **核对**：`[ ]`

### W3. AI 生图 / 无线画布 / 作品 / 视频工作台首屏
- **不可丢**：工作台即时壳；默认列表 ≤ 40；summary 含 `totalPoints`。
- **实现锚点**：`admin-vue/src/utils/userWorkspaceLoad.ts`；`/user/online-image` 默认 limit ≤ 40。
- **禁止**：首屏无限制拉全量并签名大量资产 URL。
- **验证**：
  - `go test ./internal/httpserver/ -run TestUserOnlineImageDefaultListLimitsStayCapped`
  - `npm test -- tests/webWorkspacePointsRegression.spec.ts`
- **核对**：`[ ]`

### W4. 用户首页近期任务 / 指标卡片
- **不可丢**：首页仍展示近期生成、可用点数等摘要；字段可增不可无故删。
- **核对**：`[ ]`

---

## 小程序用户端

### M1. 首页模板 / 灵感入口 / 登录落点 / 游客浏览
- **不可丢**：
  - 首页灵感/模板入口可浏览、可进入详情或创作；游客可见范围不被误删。
  - 登录成功后默认进入**用户首页**（`UserHomePage`），即使账号同时具备代理商/运营中心身份；代理/运营工作台只能通过「我的」角色切换主动进入。
  - 登录页「暂不登录，进入首页」为醒目主按钮（蓝底描边），并有「可先浏览功能，需要创作时再登录」提示；不得退化成灰色弱链且难发现。
- **锚点**：`MiniProgramRoleWorkbench`、`V531HomePage`、`WechatLoginPage`（`auth-guest-enter-button`）、`features/auth/guestBrowse.ts`、`defaultRole` 优先 `USER`、`redirectAfterAuth`。
- **禁止**：`defaultRole` 因含 `AGENT`/`OPERATION` 就默认跳代理/运营概览；把游客入口改成 muted 弱文案。
- **核对**：`[ ]`

### M2. 视频创作：模型 / 参数 / 预估积分
- **不可丢**：
  - 视频页可选模型与按模型能力的参数（时长、分辨率、画幅等），能力配置下架时有兼容回退，不白屏。
  - 登录后切换模型/参数须显示预估扣费（「试算中…」→「预计 N 积分」），不得静默消失。
  - Seedance（`doubao-seedance-2.0` / `seedance-fast-2.0`）默认 **5s + 720p** 预估与正式扣费一致，为 **600 积分**（单价 80/秒 × 5 × 1.5）；不得退回代码默认 12/秒（会变成约 90）。
- **锚点**：
  - 前端：`MiniProgramRoleWorkbench` 视频区、`UserVideoCreationPage`、`packages/business-sdk` 的 `estimateVideo` / `videoParameters`。
  - 后端：`POST /api/v1/generation-tasks/estimate`；`postgresStore.AdminData` 必须加载并应用已发布计费规则（与 `aiCapabilityAdminData` / 正式创建任务同价）；线上 `xz_billing_rule_versions` 已发布 Seedance `base_price_points=80`。
- **禁止**：
  - 删掉模型/参数控件或预估 pill。
  - 试算只读代码默认 `BillingRules`、跳过已发布 `BillingRuleVersions`。
  - 把 Seedance 默认单价改回 `BasePrice: 12` 且不经已发布规则覆盖。
- **验证**：
  - `node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs`
  - `go test ./internal/httpserver/ -run "TestVideoGenerationEstimate|TestBillingCenterV1Acceptance|TestNormalizeAICapabilityDefaultsMergesMissingBillingRules"`
- **核对**：`[ ]`

### M3. 作品列表与详情
- **不可丢**：作品列表可加载、筛选/多选删除提示、详情可返回列表。
- **锚点**：`UserAssetsListPage`、`AssetDetailCenterPage`、作品中心 tab。
- **核对**：`[ ]`

### M4. 图片创作与灵感草稿带入
- **不可丢**：从灵感「做同款」可带入草稿到图片/视频/PPT 创作页。
- **锚点**：`inspiration/draft`、`UserImageCreationPage`。
- **核对**：`[ ]`

### M5. 钱包可用/赠送积分展示
- **不可丢**：个人钱包展示可用与到期赠送等信息；客户端不得改余额。
- **锚点**：`UserWalletPage`、`personalPointsWallet`。
- **核对**：`[ ]`

### M6. 自由P图全页与入口文案
- **不可丢**：
  - 首页能力位 / 创作中心 / 工作台 / 我的能力等入口文案为 **「自由P图」**，不得回退成「信息图」或「AI办公」。
  - 进入后是独立全页：只保留返回 +「自由P图」标题 → 上传区 →「选择图片效果」→ 预设卡片 → 生成按钮。
  - 不得再叠外层返回栏、「返回不会丢失草稿」、studio 横幅、顶部安全区占位、游客条、合规提示等壳层。
- **锚点**：`FreeImageEditCreation.vue`；`MiniProgramRoleWorkbench` 的 `isFreeImageEditPage` 全页壳；`V531StudioPage` / `V531ProfilePage` / `pages.json` / `config/v531.ts`（`home.capability.office`）入口文案。
- **说明**：小程序与 App 共用 uni-app 源码；改本面时两边一起核对。
- **验证**：`node --test tests/user-mini-free-image-edit.test.mjs`
- **核对**：`[ ]`

---

## 发布与生产

### P1. 只发 Git 已推送提交
- **不可丢**：`deploy.sh` dirty tree 必须失败；禁止长期生产热修覆盖 Git。
- **核对**：`[ ]`

### P2. 防回归测试在相关改动后保持绿
- **不可丢**：触及 W1–W3 / M2 / M6 时，对应 Go / Node 回归必须绿。
- **核对**：`[ ]`

---

## 交付回复模板（复制填写）

```text
## Protected surfaces 核对
- [ ] W1 侧边栏可用/总额
- [ ] W2 平台首页首屏
- [ ] W3 生图等工作台首屏
- [ ] W4 首页摘要
- [ ] M1 首页模板/灵感/登录落用户首页/游客浏览入口
- [ ] M2 视频模型/参数/预估积分（Seedance 默认 600）
- [ ] M3 作品列表
- [ ] M4 灵感草稿带入
- [ ] M5 钱包积分展示
- [ ] M6 自由P图全页与入口文案
- [ ] P1 发布门禁
- [ ] P2 相关回归测试
说明：本次实际改动范围 = …
```
