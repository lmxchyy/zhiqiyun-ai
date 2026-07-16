# UX Architecture Specification：先知 AI 用户端平台首页 / AI 创作工作台

> 文档状态：实施基线 v1.0  
> 输出日期：2026-07-15  
> 适用端：PC SaaS 用户端（`admin-vue`），兼顾平板降级；手机端以 `apps/user-uni` 的独立信息架构为主  
> 当前入口：`admin-vue/src/App.vue` 中 `store.activeModuleId === "userDashboard"`

## 1. 结论与设计边界

本页不是“功能导航大全”，而是用户完成一次 AI 创作的任务启动器。首屏只承担三件事：明确当前可创作的内容类型、收集足够启动任务的信息、让用户清楚知道下一步会发生什么。

### 1.1 推断的目标用户

- 首要用户：企业市场、运营、电商、品牌与销售团队中的非专业设计人员。
- 次要用户：需要批量生产图片、视频、PPT 或调用 Agent 的内容创作者。
- 管理型用户：关注点数、套餐和作品沉淀，但不会在首页进行复杂后台配置的企业成员。
- 使用特征：任务导向、时间敏感、对模型名和生成参数不熟悉，常从模糊自然语言或已有模板开始。

### 1.2 推断的核心任务

用户在 60 秒内选择正确的创作模式，输入或复用需求，确认关键参数与预计消耗，启动一次可追踪的 AI 创作，并能在生成后找到任务或作品。

### 1.3 不在本页承担的任务

- 模型渠道、API Key、企业级权限和计费规则配置。
- 完整作品编辑、复杂提示词工程、节点画布操作。
- 订单、发票、充值和用量明细管理。
- 企业组织、成员、角色和认证管理。

这些任务由独立工作区或管理页承担，首页只提供清晰入口和必要上下文。

### 1.4 当前基线中的关键 UX 风险

| 风险 | 当前表现 | 用户影响 | 规格决策 |
|---|---|---|---|
| 模式与参数不匹配 | 图片、视频、PPT、Agent 共用模型、比例、质量控件 | 用户无法判断参数是否对当前模式有效 | 每个模式拥有独立 schema、默认值和草稿 |
| “生成”语义不清 | 首页按钮主要跳转工作区，不一定立即提交任务 | 用户可能认为任务已经创建 | CTA 文案按行为区分“继续配置”和“立即生成” |
| 草稿串扰 | 模板与 Agent 会写入共享 `onlineImageForm.prompt` | 切换模式后可能带入错误内容 | 使用 `draftByMode`，模板只写入目标模式草稿 |
| 信息密度过高 | Agent、模板广场、灵感模板同时争夺注意力 | 首屏核心任务被稀释 | 首屏聚焦 Composer；发现型内容进入第二层 |
| 导航状态弱 | 主要依赖 `activeModuleId`，URL 恢复能力有限 | 刷新、分享、返回行为不稳定 | 路由状态作为事实来源，store 作为缓存 |
| 状态反馈不完整 | 缺少统一的配额不足、模型不可用、任务提交失败恢复模型 | 用户不知道如何修复 | 统一状态矩阵、错误恢复和任务追踪入口 |

## 2. 设计原则与成功指标

### 2.1 设计原则

1. 先表达任务，再暴露参数：用户先选“做什么”，系统再显示与该任务有关的设置。
2. 首页负责启动，工作区负责完成：不把专业编辑器压缩进首页。
3. 每次动作都有可恢复结果：草稿自动保存、失败可重试、生成任务可追踪。
4. 套餐和点数透明但不打断：提交前显示预计消耗，额度不足时给出具体补救动作。
5. 同一任务跨入口一致：模板、Agent 卡片、灵感卡片最终都生成同一种 `CreationIntent`。
6. 权限在服务端裁决，前端只负责解释：前端隐藏或禁用入口不能替代真实授权。

### 2.2 建议观测指标

| 指标 | 目标 | 埋点建议 |
|---|---:|---|
| 首次有效创作启动时间 | P50 ≤ 60 秒 | `workbench_view` → `creation_intent_confirmed` |
| Composer 启动转化率 | ≥ 35% | 页面访问用户中确认创作意图的比例 |
| 模式切换后参数纠错率 | ≤ 5% | 因参数不适用触发的前端校验次数 |
| 首页启动失败恢复率 | ≥ 70% | 失败后 10 分钟内重试或切换可用模型 |
| 任务可找回率 | ≥ 95% | 提交后能进入任务详情或作品中心 |
| 模板到提交转化率 | 单独统计 | `template_selected` → `creation_submitted` |

## 3. Experience Map

| 用户目标 | 入口 | 关键步骤 | 成功状态 | 失败与恢复 |
|---|---|---|---|---|
| 从一句需求开始创作 | 平台首页 Composer | 选模式 → 输入需求 → 系统补齐默认参数 → 确认预计点数 → 继续或提交 | 进入正确工作区且草稿完整，或创建任务并返回任务 ID | 保留草稿；标出无效字段；提供重试、换模型或充值 |
| 使用成熟模板快速开始 | 模板广场 / 灵感模板 | 预览模板 → 应用 → 根据模式补充变量 → 继续配置 | 目标模式草稿已创建，模板来源可追踪 | 模板不可用时推荐同类模板，不清空用户原输入 |
| 调用专用 Agent | 快捷 Agent | 选 Agent → 查看能力与所需输入 → 进入 Agent 对话 | Agent 工作区打开，首条任务已预填但未擅自发送 | 无权限时说明原因并提供申请/升级入口 |
| 继续未完成任务 | 最近任务模块 | 选择草稿/进行中任务 → 恢复上下文 | 回到原工作区与原步骤 | 任务已失效时允许复制配置创建新任务 |
| 查看生成结果 | 全局任务状态 / 作品中心 | 打开任务 → 查看进度或结果 → 下载/编辑/复用 | 作品进入资产体系，来源与消耗可查看 | 失败任务显示可执行原因、重试策略和退款/返还状态 |
| 处理额度不足 | CTA 前置校验 / 服务端返回 | 查看预计消耗 → 选择充值或降级参数 → 返回原草稿 | 原草稿和参数保留，额度恢复后可继续 | 充值取消或失败不丢失创作上下文 |

## 4. 信息架构

### 4.1 全局导航

PC 用户端保持现有左侧主导航，但按任务域分组，不按技术能力堆叠：

```text
用户控制台
├─ 平台首页
├─ 创作
│  ├─ 图片创作
│  ├─ 视频创作
│  ├─ PPT 文档
│  ├─ 无限画布
│  └─ 智能体中心
├─ 作品与任务
│  ├─ 作品中心
│  └─ 生成任务
├─ 账户
│  ├─ 身份 / 充值 / 订阅
│  ├─ 使用记录
│  └─ 订单明细
└─ 条件入口
   ├─ 代理中心（仅代理身份）
   └─ 运营中心（仅运营身份）
```

信息架构规则：

- “平台首页”是默认登录落点，不复制侧栏的全部能力说明。
- “生成任务”与“作品中心”语义分离：任务包含排队、生成中、失败；作品只包含可消费的成功资产。
- 代理、运营、企业身份使用显式上下文切换，不把不同权限域混入普通用户导航。
- 手机端保留 `首页 / 创作 / 作品 / 我的` 四个一级 Tab；PC 的十个侧栏入口不可原样压缩为手机底栏。

### 4.2 页面内容层级

```text
平台首页
├─ L1：页面标题与账户状态
│  ├─ 当前身份 / 工作空间
│  ├─ 全局搜索
│  ├─ API / 服务状态
│  └─ 账户菜单
├─ L1：Creation Composer（唯一首要任务）
│  ├─ 模式选择
│  ├─ Prompt / 文件输入
│  ├─ 模式关键参数
│  ├─ 预计点数与可用性
│  └─ 主 CTA
├─ L2：快捷方案
│  ├─ 常用 Agent
│  └─ 最近使用 / 收藏
├─ L2：继续工作
│  ├─ 草稿
│  ├─ 进行中任务
│  └─ 最近作品
└─ L3：发现内容
   ├─ 推荐模板
   └─ 灵感模板
```

### 4.3 内容优先级

1. 用户正在输入的需求和当前创作模式。
2. 提交任务所需的关键参数、预计消耗和可用性。
3. 最近草稿、进行中任务和失败任务恢复。
4. 常用 Agent 与个性化快捷入口。
5. 模板与灵感发现内容。
6. 套餐营销和升级信息。

### 4.4 权限模型

| 对象 | 前端展示规则 | 服务端规则 | 无权限反馈 |
|---|---|---|---|
| 创作模式 | 根据 capability catalog 标记可用、受限、维护中 | 校验用户、租户、套餐、模型和调用额度 | 禁用并说明具体原因；可升级时展示 CTA |
| 模型 | 仅展示当前模式可用模型，不暴露无权模型的敏感配置 | 服务端根据商品、角色和租户过滤 | 自动推荐可用替代项，不静默切换 |
| Agent | 根据角色与发布范围展示 | 校验 Agent ACL、租户和知识库权限 | 显示“申请权限”或“联系管理员” |
| 模板 | 公开、企业、个人三种范围 | 校验模板可见范围与素材访问权 | 不展示受限素材详情；可提供无素材版本 |
| 任务 / 作品 | 只展示当前主体可读对象 | 所有 ID 读取做服务端归属校验 | 返回明确的不存在或无权限状态，不泄露对象信息 |

## 5. CSS 与 Layout 系统

### 5.1 主题策略

- 第一阶段维持现有浅色主题，补齐语义 token，不直接重绘页面。
- 第二阶段支持 `light / dark / system`：根节点使用 `data-theme`，`system` 监听 `prefers-color-scheme`。
- Element Plus 颜色通过语义 token 映射，不在页面组件中继续增加十六进制常量。
- 用户显式选择高于系统偏好；选择持久化到用户设置，未登录时存本地。
- 图表、封面遮罩、焦点环和骨架屏必须分别验证深色对比度，不能只反转背景。

### 5.2 语义 Token

建议保留现有品牌色并收敛为以下合同：

```css
:root {
  color-scheme: light;

  /* Color */
  --ux-bg-canvas: #f8faff;
  --ux-bg-surface: #ffffff;
  --ux-bg-subtle: #f4f7fb;
  --ux-bg-elevated: rgba(255, 255, 255, 0.96);
  --ux-border-default: rgba(210, 212, 214, 0.70);
  --ux-border-focus: #7d8df6;
  --ux-text-primary: #111827;
  --ux-text-secondary: #6b7280;
  --ux-text-muted: #9ca3af;
  --ux-action-primary: #5a4db2;
  --ux-action-primary-hover: #493d9d;
  --ux-action-accent: #ff771b;
  --ux-action-accent-hover: #ff8a2a;
  --ux-success: #168f4b;
  --ux-warning: #c56a00;
  --ux-danger: #c9362b;
  --ux-focus-ring: 0 0 0 3px rgba(125, 141, 246, 0.28);

  /* Type */
  --ux-font-sans: Inter, "Microsoft YaHei", system-ui, sans-serif;
  --ux-text-xs: 12px;
  --ux-text-sm: 14px;
  --ux-text-md: 16px;
  --ux-text-lg: 20px;
  --ux-text-xl: clamp(28px, 3vw, 44px);
  --ux-leading-tight: 1.2;
  --ux-leading-body: 1.6;

  /* Space: 4px base */
  --ux-space-1: 4px;
  --ux-space-2: 8px;
  --ux-space-3: 12px;
  --ux-space-4: 16px;
  --ux-space-5: 20px;
  --ux-space-6: 24px;
  --ux-space-8: 32px;
  --ux-space-10: 40px;

  /* Shape and elevation */
  --ux-radius-sm: 8px;
  --ux-radius-md: 12px;
  --ux-radius-lg: 16px;
  --ux-radius-xl: 24px;
  --ux-shadow-card: 0 12px 32px rgba(90, 77, 178, 0.08);
  --ux-shadow-overlay: 0 24px 64px rgba(17, 24, 39, 0.18);

  /* Shell */
  --ux-sidebar-width: 200px;
  --ux-sidebar-collapsed-width: 64px;
  --ux-topbar-height: 50px;
  --ux-content-max: 1600px;
}

[data-theme="dark"] {
  color-scheme: dark;
  --ux-bg-canvas: #0e1118;
  --ux-bg-surface: #171b24;
  --ux-bg-subtle: #202633;
  --ux-bg-elevated: rgba(23, 27, 36, 0.96);
  --ux-border-default: rgba(226, 232, 240, 0.16);
  --ux-text-primary: #f7f8fb;
  --ux-text-secondary: #bcc3d0;
  --ux-text-muted: #8f98a8;
}
```

禁止事项：

- 页面业务组件不新增品牌色硬编码；特殊模板封面色除外，但必须集中为数据配置。
- 不用颜色作为唯一状态标识；状态同时使用文本、图标或形状。
- 不用 `transition: all`；只声明 `transform`、`opacity`、`box-shadow` 等必要属性。

### 5.3 页面网格

| 区域 | ≥ 1500px | 1200–1499px | 768–1199px | < 768px |
|---|---|---|---|---|
| Shell | 200px 侧栏 + 内容 | 200px 侧栏 + 内容 | 64px 折叠侧栏或抽屉 | 顶栏 + 抽屉；优先提示使用小程序 |
| 内容容器 | 最大 1600px，左右 28px | 左右 24px | 左右 16px | 左右 12px |
| 首页主体 | 12 栏：主区 9、辅助区 3 | 辅助区下移，单列 | 单列 | 单列 |
| 模式 Tab | 4 等分 | 4 等分 | 2 × 2 | 横向滚动或 2 × 2 |
| Agent | 8 / 行，自适应 | 4–6 / 行 | 2–4 / 行 | 2 / 行 |
| 模板 | 4–5 / 行 | 3–4 / 行 | 2 / 行 | 1 / 行 |

布局约束：

- Composer 宽度不小于 640px 时才把参数放在同一工具栏；否则参数进入可展开的“生成设置”。
- 页面主列使用 `minmax(0, 1fr)`，所有卡片容器必须有 `min-width: 0`，防止长提示词撑破网格。
- Sticky 灵感栏只在主区和辅助区并排时启用；折叠后恢复普通文档流。
- 页面纵向节奏以 24px section gap 为基线；首屏标题与 Composer 的距离不超过 24px。
- 触控环境交互目标至少 44 × 44px；纯桌面鼠标控件也不小于 36px 高。

### 5.4 文本与内容规则

- 页面只允许一个 `h1`：“开始一次 AI 创作”或等价业务标题。
- 模式名称用用户语言：图片、视频、PPT、Agent；模型名只在“高级设置”中出现。
- 主 CTA 必须描述结果：`继续配置`、`立即生成（预计 12 点）`，不使用无语义的“确定”。
- 错误文案结构：发生了什么 → 为什么 → 用户现在能做什么。
- 数字使用等宽数字特性 `font-variant-numeric: tabular-nums`，点数和进度避免跳动。

## 6. 组件架构与边界

### 6.1 建议目录

```text
admin-vue/src/
├─ pages/user-dashboard/
│  └─ UserDashboardPage.vue
├─ components/user-dashboard/
│  ├─ WorkbenchHero.vue
│  ├─ CreationComposer.vue
│  ├─ CreationModeTabs.vue
│  ├─ PromptField.vue
│  ├─ ReferenceAssetPicker.vue
│  ├─ ModeParameterBar.vue
│  ├─ CostAndAvailability.vue
│  ├─ CreationPrimaryAction.vue
│  ├─ QuickAgentGrid.vue
│  ├─ ContinueWorking.vue
│  ├─ TemplateGallery.vue
│  ├─ InspirationRail.vue
│  └─ WorkbenchStatePanel.vue
├─ composables/
│  ├─ useCreationDrafts.ts
│  ├─ useCreationIntent.ts
│  └─ useCapabilityAvailability.ts
├─ stores/
│  ├─ creationDrafts.ts
│  └─ generationTasks.ts
├─ api/
│  ├─ capabilities.ts
│  ├─ creation.ts
│  └─ tasks.ts
└─ styles/
   ├─ tokens.css
   ├─ foundations.css
   └─ user-dashboard.css
```

`App.vue` 只保留 Shell、顶层路由出口与跨模块能力，不继续承载首页模板、首页数据和交互逻辑。

### 6.2 核心领域合同

```ts
type CreationMode = "image" | "video" | "ppt" | "agent";

interface CreationIntent {
  mode: CreationMode;
  prompt: string;
  referenceAssetIds: string[];
  templateId?: string;
  agentId?: string;
  modelId?: string;
  parameters: Record<string, string | number | boolean>;
  source: "manual" | "template" | "agent-card" | "inspiration" | "resume";
}

interface ModeSchema {
  mode: CreationMode;
  available: boolean;
  unavailableReason?: string;
  defaultModelId?: string;
  fields: Array<{
    key: string;
    label: string;
    type: "select" | "number" | "toggle" | "ratio";
    required: boolean;
    options?: Array<{ label: string; value: string }>;
  }>;
}

interface CreationEstimate {
  canSubmit: boolean;
  estimatedCredits?: number;
  balance?: number;
  blockingReason?: "insufficient_balance" | "model_unavailable" | "permission_denied" | "invalid_input";
}
```

金额、点数、Token、会员时长、图片额度和权益只接受服务端商品/能力配置结果；`CreationEstimate` 是展示合同，不是前端计费来源。

### 6.3 组件责任表

| 组件 | 单一责任 | 输入 / 输出 | 必备状态 | 可访问性 |
|---|---|---|---|---|
| `UserDashboardPage` | 编排页面区块与加载顺序 | 读取页面 view model；不直接拼 API | loading / ready / partial / fatal | `main` landmark，管理页面标题 |
| `CreationComposer` | 维护单次创作意图的呈现与校验 | `modeSchema`、`draft`；emit `confirm` | pristine / dirty / validating / blocked / ready | `form`、错误摘要、提交后聚焦首个错误 |
| `CreationModeTabs` | 在四种任务模式间切换 | `modelValue`、可用性；emit `update` | active / disabled / loading | `tablist/tab/tabpanel`、方向键切换 |
| `PromptField` | 收集自然语言需求 | `value`、限制、建议；emit `update` | empty / valid / too-long / enhancing | 显式 label、字符数 `aria-live="polite"` |
| `ReferenceAssetPicker` | 上传或选择已有素材 | 文件限制、资产列表；emit `change` | idle / uploading / success / failed | 键盘可上传/删除，缩略图有 alt |
| `ModeParameterBar` | 按 schema 渲染模式参数 | `schema`、`values`; emit `change` | basic / advanced / invalid | 原生 label，折叠区 `aria-expanded` |
| `CostAndAvailability` | 展示预计消耗与阻塞原因 | `estimate` | loading / available / blocked / unknown | 状态文本不只靠颜色，更新用 `aria-live` |
| `CreationPrimaryAction` | 触发继续配置或提交 | `actionType`、loading、disabled reason | idle / submitting / success / failed | loading 不丢失按钮名，禁用原因可读 |
| `ContinueWorking` | 恢复草稿、任务和最近作品 | 统一 activity feed | empty / partial / loaded / failed | 列表语义，状态和更新时间可读 |
| `QuickAgentGrid` | 展示个性化 Agent 快捷入口 | Agent cards | empty / limited / loaded | 卡片按钮有完整名称与能力说明 |
| `TemplateGallery` | 模板筛选、预览和应用 | 模板查询条件 | skeleton / empty / loaded / error | 图片 alt、筛选控件 label、焦点返回 |
| `WorkbenchStatePanel` | 统一页面级空、错、无权限状态 | `kind`、message、actions | variant-defined | `role="status"` 或 `role="alert"` |

### 6.4 状态所有权

| 状态 | 所有者 | 持久化 | 说明 |
|---|---|---|---|
| 当前创作模式 | URL query + 页面 composable | session | 可刷新恢复，例如 `?mode=video` |
| 各模式草稿 | `creationDrafts` store | local cache + 服务端草稿可选 | `draftByMode` 独立存储，不能共用 form |
| 能力与模型可用性 | capability store | 短 TTL | 服务端为事实来源，切换身份/租户立即失效 |
| 点数与预计消耗 | estimate request | 不持久化 | 每次关键参数变化防抖刷新，提交时服务端重算 |
| 任务进度 | `generationTasks` store | 服务端 | 轮询、SSE 或事件通道更新；页面离开不丢失 |
| 模板 / Agent 推荐 | 页面 query cache | 可短期缓存 | 允许部分失败，不阻塞 Composer |
| 导航激活项 | Router | history | Pinia 不作为唯一导航事实来源 |

## 7. 交互模型

### 7.1 首屏 Composer 流程

```text
进入页面
  → 恢复上次模式或默认图片模式
  → 加载模式 schema 与可用性
  → 用户输入 / 上传 / 应用模板
  → 400ms 防抖校验与费用预估
  → CTA 根据 actionType 显示“继续配置”或“立即生成”
  → 服务端最终校验
  ├─ 成功：创建任务或进入目标工作区
  └─ 失败：保留草稿，定位错误，给出恢复动作
```

### 7.2 模式切换

- 切换模式不丢弃当前模式草稿；返回时完整恢复。
- 若用户已填写内容，切换时不弹确认框；顶部以轻量提示显示“图片草稿已保存”。
- 新模式首次进入时，可选择“沿用当前描述”，但不自动复制不兼容参数。
- 每个模式只显示与其有关的基础参数：
  - 图片：模型（高级）、比例、质量、数量、参考图。
  - 视频：模型（高级）、时长、比例、质量、首尾帧/参考图。
  - PPT：页数、语言、受众、风格、参考文档。
  - Agent：Agent、会话目标、知识范围、附件；不显示图片比例。
- 模式不可用时，Tab 仍可见但呈禁用态，并解释维护、权限或套餐原因。

### 7.3 CTA 规则

| 场景 | CTA | 行为 |
|---|---|---|
| 首页信息足以直接提交 | `立即生成 · 预计 N 点` | 调用创建任务 API，成功后显示任务追踪 |
| 目标工作区还有必填步骤 | `继续配置` | 以 `CreationIntent` 导航到目标工作区，不创建任务 |
| 输入不完整 | `补充创作需求`（禁用） | 同时在输入区显示具体缺失项 |
| 额度不足 | `去充值并保留草稿` | 保存草稿并带 return URL 进入充值页 |
| 服务不可用 | `切换可用模型` | 打开已过滤的模型列表；禁止静默替换 |

### 7.4 模板、Agent 与灵感

- 卡片点击先打开轻量预览或直接填入 Composer，行为必须在卡片上可预期。
- “应用模板”只创建草稿，不自动提交付费任务。
- 应用模板后将焦点移到第一个需要用户补充的变量。
- Agent 卡片必须展示：名称、擅长任务、需要的输入、权限/套餐状态。
- 推荐内容加载失败时，Composer 仍可用；辅助区显示局部重试。

### 7.5 表单与验证

- 客户端即时验证格式和必填项，服务端验证权限、价格、额度和最终参数。
- 验证发生在字段 blur、关键参数变化和提交时；输入过程不连续弹 Toast。
- 错误显示在字段旁，并在 Composer 顶部提供错误摘要；点击摘要定位字段。
- 上传先校验类型、大小和数量，再进入进度状态；失败文件可单独重试或移除。
- Prompt 润色前显示原文/建议对比，用户确认后替换；不得用字符串前缀模拟“润色”。

### 7.6 反馈与通知

- 提交成功：就地出现任务卡，包含状态、预计耗时、消耗、查看详情；不只显示 Toast。
- 生成中：全局任务入口显示计数；用户离开页面后继续更新。
- 成功：通知可跳到作品详情；当前页面的任务卡更新为可预览状态。
- 失败：展示可理解的错误分类、点数处理结果、重试和复制配置入口。
- Toast 只用于轻量确认；需要决策或恢复的错误使用 inline panel 或 dialog。
- 所有异步更新使用礼貌级 `aria-live`，避免屏幕阅读器被频繁打断。

### 7.7 键盘与焦点

- `Tab` 顺序：模式 → Prompt → 素材 → 基础参数 → 高级设置 → 费用 → 主 CTA → 辅助内容。
- 模式 Tab 支持左右方向键、Home、End；选择后焦点保持在当前 Tab。
- `Ctrl/Cmd + Enter` 触发主 CTA，但必须遵守同一校验和费用确认规则。
- Dialog 打开后焦点进入标题或第一个控件；关闭后返回触发元素。
- 路由切换后焦点移到目标页 `h1`，不落在浏览器顶部或侧栏第一项。
- 任何 hover 操作都有 focus-visible 等价行为。

### 7.8 减少动态效果

- 尊重 `prefers-reduced-motion: reduce`，关闭卡片上浮、平滑滚动和大面积渐变动画。
- 任务进度使用数值和文本，不依赖无限旋转动画表达状态。
- 骨架屏只用于结构稳定的加载，持续超过 10 秒切换为明确等待文案。

## 8. 响应式与跨端策略

### 8.1 PC / 平板

- ≥ 1500px：Composer + sticky 灵感栏双列；“继续工作”优先于模板广场。
- 1200–1499px：灵感栏下移；Composer 保持完整参数工具栏。
- 768–1199px：侧栏折叠或抽屉；参数进入两行或设置抽屉；主 CTA 独占整行。
- 页面缩放至 200% 时仍能完成核心任务，不能出现水平页面滚动。

### 8.2 手机 Web 与微信小程序

- 手机 Web 仅提供安全降级，不与小程序共用页面级 DOM。
- 小程序继续使用 `Vue 3 + TypeScript + uni-app` 与 `首页 / 创作 / 作品 / 我的` 四 Tab。
- 可共享领域类型、模式 schema 和 API 合同；不可共享依赖 Element Plus 的视图组件。
- 小程序请求统一走现有 API Client，页面不直接调用 `uni.request`，也不引入 Axios。
- Web 草稿与小程序草稿若要跨端同步，必须定义服务端 draft ID 和冲突策略；仅靠 localStorage 无法跨端。

## 9. 实施计划

### Phase 0：契约与观测（1–2 天）

1. 冻结 `CreationMode`、`CreationIntent`、`ModeSchema`、`CreationEstimate` 合同。
2. 明确每种模式的 CTA 是“继续配置”还是“立即生成”。
3. 定义 capability、estimate、draft、task 接口与服务端权限/计费职责。
4. 增加现状埋点，建立首页访问、模式选择、模板应用、CTA、失败分类基线。

验收：产品、前端、后端对四种模式的字段和提交语义无歧义。

### Phase 1：解耦首页与 CSS 基础（2–4 天）

1. 从 `App.vue` 提取 `UserDashboardPage.vue` 和页面级组件，保持视觉与行为基本不变。
2. 将现有 `styles.css` 中首页样式迁入 `user-dashboard.css`，建立语义 token 别名。
3. 用 Router 恢复首页和模式状态；保留 `selectAdminModule` 兼容层，避免一次重写全部模块。
4. 为 Composer、模式 Tab、模板卡和局部错误面板补语义 HTML 与焦点样式。

验收：现有主要路径无回归，`App.vue` 不再持有首页模板与首页专属交互实现。

### Phase 2：模式化 Composer（3–5 天）

1. 实现 `draftByMode` 和 `ModeSchema` 驱动的参数栏。
2. 修复 Agent 模式落入图片工作区、视频/PPT 显示图片参数等不一致。
3. 用真实 Prompt 增强交互替换字符串前缀模拟。
4. 增加预计点数、模型可用性、权限与余额阻塞状态。
5. 所有首页入口统一创建 `CreationIntent`，由目标工作区消费。

验收：四种模式切换 20 次后草稿不串扰；目标页恢复正确字段；前端不能决定点数或权益。

### Phase 3：任务闭环与恢复（3–5 天）

1. 增加“继续工作”，聚合草稿、生成中、失败任务和最近作品。
2. 提交成功返回 task ID，并在页面与全局任务入口同步状态。
3. 定义失败分类、点数返还状态、重试和复制配置流程。
4. 充值/升级返回后恢复原草稿和 return URL。

验收：断网、刷新、跨模块跳转和额度不足场景都能恢复，不产生重复提交。

### Phase 4：推荐、深色与体验优化（按优先级）

1. 基于最近使用和角色排序 Agent、模板与灵感，保留“关闭个性化推荐”能力。
2. 完成 dark/system token、Element Plus 变量、图片遮罩和图表对比度适配。
3. 针对首屏资源、模板缩略图和辅助数据做分级加载。
4. 使用真实指标做 A/B：标题、默认模式、CTA 和“继续工作”位置。

验收：深浅主题 WCAG 对比度通过；辅助内容失败不影响核心创作。

## 10. QA 清单

### 10.1 信息架构与导航

- [ ] 默认登录落到平台首页，浏览器刷新后页面和模式不丢失。
- [ ] 浏览器前进/后退恢复模块、模式和子页面状态。
- [ ] 代理、运营、企业入口只在对应身份出现，切换身份后缓存立即失效。
- [ ] 任务与作品入口语义清楚，失败任务不会出现在“作品”列表中。
- [ ] 首页不出现模型渠道、API Key 或企业管理等越界配置。

### 10.2 Composer 与状态覆盖

- [ ] 图片、视频、PPT、Agent 各自只显示适用参数。
- [ ] 四种模式各自保存草稿，切换、刷新和返回均可恢复。
- [ ] 模板、Agent、灵感和手动输入都生成同一 `CreationIntent` 结构。
- [ ] 空输入、超长输入、非法文件、上传失败都有字段级反馈。
- [ ] loading、empty、partial、ready、blocked、success、failure 状态均有实现。
- [ ] 能力列表、模型列表、费用预估任一局部失败时，页面不整体白屏。
- [ ] CTA 文案与实际行为一致，不把“跳转”误写成“生成”。
- [ ] 双击 CTA、慢网络和重复回调不会创建重复任务。

### 10.3 计费、支付与权限

- [ ] 前端不提交或决定金额、会员天数、credits、Token、图片额度或权益。
- [ ] 预计消耗与最终扣费均由服务端计算，提交时再次校验。
- [ ] 余额不足返回充值后，原模式、Prompt、附件和参数仍在。
- [ ] 模型不可用时不静默切换，用户能看到替代模型与影响。
- [ ] 无权限对象的详情不会通过 URL、缓存或错误文案泄露。
- [ ] 支付/充值失败不会触发前端直接修改余额或会员状态。

### 10.4 响应式

- [ ] 1920、1440、1280、1024、768、390px 宽度无水平页面滚动。
- [ ] 200% 浏览器缩放仍能访问所有字段与 CTA。
- [ ] Composer 工具栏换行后阅读顺序与 Tab 顺序一致。
- [ ] Sticky 辅助栏在空间不足时回到文档流，不遮挡内容。
- [ ] 安全区、抽屉、弹窗和虚拟键盘不会遮挡手机端主 CTA。

### 10.5 可访问性

- [ ] 页面只有一个 `h1`，标题层级连续。
- [ ] Shell 具备 `header/nav/main/aside` landmarks 和跳过导航链接。
- [ ] 所有控件可用键盘完成；无键盘陷阱。
- [ ] `:focus-visible` 清晰，深浅主题都满足对比度。
- [ ] 文本与图标对比度满足 WCAG AA；状态不只依赖颜色。
- [ ] 模式 Tab 使用正确 ARIA 语义和方向键模型。
- [ ] Dialog 焦点锁定、关闭回焦、ESC 行为正确。
- [ ] 异步费用和任务更新的 `aria-live` 不造成重复播报。
- [ ] 图片有用途匹配的 alt；装饰图使用空 alt。
- [ ] `prefers-reduced-motion` 下关闭非必要动画。

### 10.6 性能与可靠性

- [ ] 首屏先加载 Shell + Composer，模板和灵感延后加载。
- [ ] 模板缩略图有固定宽高、懒加载和适当尺寸，不请求原图。
- [ ] 输入 Prompt 不触发高频 API；预估请求 300–500ms 防抖并可取消旧请求。
- [ ] 切换模式不重复下载相同 schema；切换身份时缓存必定失效。
- [ ] 弱网、离线、超时、401、403、409、429、5xx 均有明确恢复策略。
- [ ] 轮询在页面隐藏或任务终态时停止；优先评估事件推送。
- [ ] 页面离开时未完成上传有明确确认，已保存草稿无需阻断。

### 10.7 视觉回归

- [ ] 浅色、深色、系统主题分别截图比对。
- [ ] 中英文、20 字标题、2000 字 Prompt、超长模型名不溢出。
- [ ] 0、1、8、20 个 Agent / 模板时网格稳定。
- [ ] 点数从 0 到 9,999,999，任务进度 0–100% 时布局不跳动。
- [ ] Element Plus 组件变量与业务 token 一致，没有局部蓝/紫/橙色冲突。

## 11. 风险与待确认问题

以下答案会改变部分架构，但不阻塞按本规格开始 Phase 0–1：

1. 首页主 CTA 是否允许直接创建付费任务，还是所有模式都必须先进入专业工作区？
2. 首要用户更偏企业营销运营，还是专业设计师？若是后者，高级参数需要更早暴露。
3. Agent 是一种创作模式，还是独立产品域？当前行为更接近独立工作区。
4. 草稿是否需要 PC 与小程序跨端同步？需要则必须新增服务端草稿实体与冲突版本。
5. 模板和灵感是否有真实运营后台、权限范围和素材授权信息？
6. 生成任务是否已有统一状态、进度、失败原因和点数返还字段？
7. 深色主题是否纳入近期版本？若否，也应先完成 token 化，避免以后逐页重写。
8. 是否保留“API ONLINE”技术状态给普通用户？建议改为“服务正常”，详细技术状态放诊断页。

## 12. Definition of Done

本页达到 UX Architecture 交付完成，至少满足：

- 四种模式具备独立 schema、独立草稿和清晰 CTA 语义。
- 所有入口统一生成 `CreationIntent`，目标工作区可稳定恢复上下文。
- 费用、权限、模型可用性和任务创建均由服务端最终裁决。
- 页面核心流程可用键盘和屏幕阅读器完成，200% 缩放可用。
- 首屏辅助内容失败不会阻塞 Composer；提交失败不丢草稿。
- 任务创建具备幂等保护，提交后能被任务中心追踪并最终进入作品中心。
- `App.vue` 不再承载首页专属模板、交互与 CSS；组件和状态边界与本规格一致。
- 通过本清单的自动化测试、视觉回归和真实 API 联调后再宣告完成。

