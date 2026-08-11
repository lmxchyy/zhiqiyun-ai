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

### M2. 视频创作：模型 / 参数 / 预估积分 / 错误文案
- **不可丢**：
  - 视频页可选模型与按模型能力的参数（时长、分辨率、画幅等），能力配置下架时有兼容回退，不白屏。
  - 登录后切换模型/参数须显示预估扣费（「试算中…」→「预计 N 积分」），不得静默消失。
  - Seedance（`doubao-seedance-2.0` / `seedance-fast-2.0`）默认 **5s + 720p** 预估与正式扣费一致，为 **600 积分**（单价 80/秒 × 5 × 1.5）；不得退回代码默认 12/秒（会变成约 90）。
  - **正式接线视频模型**（不得从默认绑定 / 套餐限额 / NewAPI 通道模型列表里悄悄删掉）：
    - `grok-imagine-video-1.5-preview`：仅图生视频；参考图恰好 1 张；时长仅 **10 / 15**；计费 **100 积分/次**。
    - `grok-imagine-1.5-video`：文生 + 图生；最多 7 张参考图；时长 **6–30s**；计费 **15 积分/秒**。
    - `doubao-seedance-2.0` / `seedance-fast-2.0`：须出现在 `channel_newapi_gateway` / `channel_newapi_grok_imagine` 等 NewAPI 通道 `models` 列表（Seedance Fast 不得只留在 CME 通道导致 NewAPI 路由失败）。
  - **套餐限额**：`tenantModuleLimits` / 业务套餐视频 `allowed` 列表若覆盖默认值，必须继续包含 Preview；禁止因缺模型把「not allowed by tenant/package limit」映射成笼统的「暂无权限执行此操作」。
  - **错误文案**：视频/生图失败须经 `localizeGenerationErrorMessage`（或等价链路）给出中文可读提示；小程序 toast 展示真实错误，不得一律吞成「生成失败」。
- **锚点**：
  - 前端：`MiniProgramRoleWorkbench` 视频区、`UserVideoCreationPage`、`packages/business-sdk` 的 `estimateVideo` / `videoParameters`。
  - 后端：`POST /api/v1/generation-tasks/estimate`；`postgresStore.AdminData` 必须加载并应用已发布计费规则（与 `aiCapabilityAdminData` / 正式创建任务同价）；线上 `xz_billing_rule_versions` 已发布 Seedance `base_price_points=80`。
  - 能力默认：`ai_capability.go`（`ensureGrokImagineVideoModels` / 通道 models 合并、`tenantModuleLimits` 默认 allowed、BillingRules）；`video_generation_validation.go`（Preview/Grok 能力）；`api.go` 的 `localizeGenerationErrorMessage`。
- **禁止**：
  - 删掉模型/参数控件或预估 pill。
  - 试算只读代码默认 `BillingRules`、跳过已发布 `BillingRuleVersions`。
  - 把 Seedance 默认单价改回 `BasePrice: 12` 且不经已发布规则覆盖。
  - 从视频套餐 `allowed` / 模块绑定 / NewAPI 通道 models 中移除 `grok-imagine-video-1.5-preview` 或 `seedance-fast-2.0`。
  - 把 Preview 改成文生视频，或把其时长改回非 10/15、计费改回按秒。
  - 把套餐拒模英文错误退回通用「暂无权限」且不经中文本地化。
- **验证**：
  - `node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs`
  - `go test ./internal/httpserver/ -run "TestVideoGenerationEstimate|TestBillingCenterV1Acceptance|TestNormalizeAICapabilityDefaultsMergesMissingBillingRules"`
- **核对**：`[ ]`

### M3. 作品列表与详情 / 视频下载可分享
- **不可丢**：作品列表可加载、筛选/多选删除提示、详情可返回列表。
- **视频下载（微信可发）**：网页/作品下载接口必须输出 **`Content-Type: video/mp4`** 且文件名以 **`.mp4`** 结尾；不得把上游 `.m4v` / `video/x-m4v` 原样传给浏览器导致微信群无法播放。有 ffmpeg 时应对 m4v/HEVC 等做 remux 或转 H.264+AAC。
- **锚点**：`UserAssetsListPage`、`AssetDetailCenterPage`、作品中心 tab；`/api/v1/video/download`、`writeNormalizedVideoDownload` / `normalizeVideoBytesForShare`、`sanitizeVideoDownloadFilename`。
- **验证**：`go test ./internal/httpserver/ -run "TestSanitizeVideoDownloadFilename|TestNormalizeVideoBytesForShare|TestDownloadAssetNameStripsM4V"`
- **核对**：`[ ]`

### M4. 图片创作与灵感草稿带入
- **不可丢**：从灵感「做同款」可带入草稿到图片/PPT 创作页（视频灵感见 M7，审核期不下发）。
- **锚点**：`inspiration/draft`、`UserImageCreationPage`。
- **核对**：`[ ]`

### M7. 小程序视频灵感临时下架（微信类目审核）
- **背景**：微信审核缺「文娱-其他视频」类目时，小程序端临时隐藏视频灵感，避免详情播放器 /「AI视频」标签 / 「生成同款」进视频创作被拒。
- **不可丢（审核期默认生效）**：
  - `platform=miniprogram`（及空 / `mp-weixin` / `weixin` / `wechat`）时：
    - 分类不下发 `code=video` / 名称「AI视频」。
    - 精选/列表排除 `contentType=video`；显式 `contentType=video` 查询返回空列表。
    - 详情对视频灵感返回 not found。
  - 小程序前端：`inspirationAPI` 二次过滤；详情页不得再渲染 `<video>`；首页/广场隐藏 AI视频 Tab。
- **锚点**：
  - 后端：`inspirationHidesVideoContent`、`ExcludeContentTypes`、`inspiration_api.go` / `inspiration_repository.go`
  - 前端：`features/inspiration/api.ts`、`InspirationDetailPage.vue`、`InspirationSquarePage.vue`、`V531HomePage` 灵感 Tab 过滤
  - 版本：`apps/user-uni/mp-weixin.release.json`（隐藏方案上线时需升版本重新提审）
- **禁止**：
  - 在未恢复类目资质前，把视频灵感详情播放器或「生成同款→视频页」重新开给小程序。
  - 只改前端不过滤后端（或反之），导致线上 API 仍可打开视频灵感详情。
- **恢复条件**：取得视听证/广电制作证或类目通过后，再显式关闭 `inspirationHidesVideoContent` 并回归测 M1/M4。
- **验证**：`go test ./internal/httpserver/ -run TestInspirationPublicSummaryDetailAndAuthGate`
- **核对**：`[ ]`

### M5. 钱包可用/赠送积分展示
- **不可丢**：个人钱包展示可用与到期赠送等信息；客户端不得改余额。
- **锚点**：`UserWalletPage`、`personalPointsWallet`。
- **核对**：`[ ]`

### M6. 自由P图全页、首页入口与 Figma 对齐
- **不可丢**：
  1. **入口文案**  
     - 首页能力位、顶部快捷入口、创作中心、工作台、我的能力、`pages.json` 标题一律为 **「自由P图」**。  
     - 不得回退成「信息图」「AI办公」「AI 办公」。  
     - 首页 `v531Capabilities` 中自由P图必须紧挨「AI设计」之后，进入 **主能力区 featured 双卡**（`slice(0, 2)`），不得再沉到次要区导致看起来像旧的「AI办公」入口。  
     - `home.capability.office` 的本地 slot `altText`、后端媒体种子名/alt 也应叫自由P图相关文案，避免运营素材继续写「AI办公」。
  2. **落地全页壳**  
     - 进入后是独立全页：返回 +「自由P图」标题 → 上传区 →「选择图片效果」→ 可编辑提示词 + 预设卡片 → 底部「开始生成」。  
     - `isFreeImageEditPage` 时禁止再叠外层返回栏、「返回不会丢失草稿」、studio 横幅、顶部安全区占位、游客条、合规提示等壳层。
  3. **默认效果与交互**  
     - 进入时若提示词为空，默认选中并填入首个预设（`magazine-cover` / `defaultFreeImageEditPrompt`）。  
     - 上传空态：蓝色「图片+号」图标 + 虚线框 +「请添加图片」（不得用半透明灰色旧图标冒充）。  
     - 选中预设：蓝边 + 浅蓝底 `#f0f4ff` + 右上勾选。  
     - 提示词区字数 `n/3000` 与「清空」胶囊在输入框内右下对齐。  
     - 「开始生成」按钮色保持 **`#ff6b00`**，不得随便改橙。
  4. **链路**  
     - 仍走既有 `businessSdk.generation.createTask` / 作品与账单链路；单图参考；校验先图后文案（`freeImageEditValidationMessage`）。
- **锚点**：
  - UI：`FreeImageEditCreation.vue`、`static/icons/add-image-blue.svg`
  - 壳：`MiniProgramRoleWorkbench` 的 `isFreeImageEditPage` / `ensureFreeImageEditDefaultPrompt`
  - 文案与排序：`config/v531.ts`（`v531Capabilities`、`home.capability.office`）、`V531HomePage` heroTools、`V531StudioPage` / `V531ProfilePage`、`pages.json`
  - 逻辑：`features/generation/freeImageEdit.ts`
- **说明**：小程序与 App 共用 uni-app 源码；改本面时两边一起核对。改首页能力排序/文案时必须勾本条并跑验证。
- **验证**：`node --test tests/user-mini-free-image-edit.test.mjs`
- **核对**：`[ ]`

---

## 发布与生产

### P1. 只发 Git 已推送提交
- **不可丢**：`deploy.sh` dirty tree 必须失败；禁止长期生产热修覆盖 Git。
- **核对**：`[ ]`

### P2. 防回归测试在相关改动后保持绿
- **不可丢**：触及 W1–W3 / M1 / M2 / M6 / M7 时，对应 Go / Node 回归必须绿。
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
- [ ] M2 视频模型/参数/预估积分（Seedance 默认 600；Grok Preview 套餐与通道；中文错误）
- [ ] M3 作品列表 / 视频下载强制 mp4（禁原样 m4v）
- [ ] M4 灵感草稿带入
- [ ] M5 钱包积分展示
- [ ] M6 自由P图全页/首页主能力文案（禁回「信息图」「AI办公」）/默认预设/#ff6b00
- [ ] M7 小程序视频灵感临时下架（类目审核）
- [ ] P1 发布门禁
- [ ] P2 相关回归测试
说明：本次实际改动范围 = …
```
