# PPT 文档生成页面 UI Design System 与开发交付说明

版本：v1.0  
适用模块：用户端 `/app/ppt-generation`  
目标页面：PPT 文档生成首页、生成工作流、作品历史入口  
目标用户：中小商户老板、个人创作者、业务负责人  
核心任务：输入主题，快速生成一份可预览、可编辑、可下载的演示文稿

## 1. 设计原则

### 1.1 体验定位

PPT 文档生成页面应保持「生成器优先」而不是「营销落地页」：

- 第一屏必须直接可输入主题、选择关键参数、点击生成。
- 页面视觉参考 Gamma / presentation-ai 的紧凑生成台，不新增分散注意力的业务入口卡片。
- 左侧继续沿用先知 AI SaaS 用户端导航，不改变生图、视频生成、AI 画布、作品中心等模块的路由、样式和功能。
- 新功能区只服务 PPT 生成，不复用视频生成里的模型搜索、ID 搜索、提示词文案或状态结构。

### 1.2 用户心理模型

中小商户老板和个人用户通常不会先配置复杂参数，他们更关心：

- 我今天要做什么主题的 PPT？
- 需要几页？
- 中文还是英文？
- 是否要联网搜索补资料？
- 生成后在哪里看、改、下载？

因此页面必须把「主题输入」放在最高优先级，把高级设置折叠或放入 pill 下拉菜单。

### 1.3 视觉调性

整体调性：专业、轻量、紧凑、有 AI 工具感。

- 首页生成器使用浅色卡片承载输入和参数，降低输入压力。
- 外层工作区可保留深色背景，与参考项目的沉浸式工具台接近。
- 历史/文件库使用深色卡片，和当前先知 SaaS 工具区保持一致。
- 不使用 ALLWEONE 品牌、Logo、版权文案和外部跳转。

## 2. 信息架构

### 2.1 首页结构

当前首页应维持以下顺序：

1. 顶部模块栏：左上角 PPT 工作台图标、模块标题。
2. 主生成区：大标题、辅助说明、提示词输入框。
3. 参数 pill 行：页数、格式、语言、更多、云模型。
4. 生成按钮：右侧圆形箭头按钮。
5. 文件库工具栏：全部、最近浏览、收藏夹、搜索、创建新的、筛选、网格/列表。
6. 历史记录/最近浏览卡片区。
7. 右侧辅助区：热门模板、创作灵感。

不要把「业务排版优化」或其他新增大入口插到热门模板前方。后续如果需要业务场景推荐，应放在「创作灵感」列表内作为内容项，而不是新增一张独立侧卡。

### 2.2 页面状态

首页状态：

- Empty：历史为空时展示空状态。
- Ready：可输入主题，生成按钮根据输入启用。
- Configuring：打开任意 pill 下拉菜单。
- Generating：点击生成后进入生成中，按钮 loading，防重复点击。
- Generated：生成结果进入历史卡片和预览/编辑工作区。
- Error：生成失败显示可重试状态，错误信息不覆盖输入内容。

生成工作流状态：

- Outline pending：等待生成大纲。
- Outline ready：可编辑大纲。
- Rendering：生成幻灯片页面。
- Success：可预览、继续编辑、导出。
- Failed：可重新生成。

## 3. Design Tokens

### 3.1 色彩

基础色：

| Token | Value | 用途 |
| --- | --- | --- |
| `--ppt-bg-canvas` | `#070707` | 页面外层背景 |
| `--ppt-bg-panel` | `#090909` | 历史面板、工作流板块 |
| `--ppt-bg-card-dark` | `#0d0d0d` | 深色卡片 |
| `--ppt-bg-card-light` | `rgba(255,255,255,0.92)` | 首页输入卡片 |
| `--ppt-border-dark` | `#242424` | 深色控件边框 |
| `--ppt-border-light` | `rgba(210,212,214,0.7)` | 浅色侧栏卡片边框 |
| `--ppt-text-main-dark` | `#f4f4f5` | 深色背景主文字 |
| `--ppt-text-main-light` | `#111827` | 浅色背景主文字 |
| `--ppt-text-muted` | `#8d8d93` | 次级说明、placeholder |
| `--ppt-brand` | `#5a4db2` | 模块强调色 |
| `--ppt-brand-soft` | `#7d8df6` | 聚焦、渐变、模板色 |
| `--ppt-accent-orange` | `#ff771b` | 生成器暖色点缀 |
| `--ppt-success` | `#20d4bf` | 成功、筛选数量 |
| `--ppt-warning` | `#facc15` | 收藏星标 |
| `--ppt-danger` | `#fecaca` | 删除、失败状态 |

使用约束：

- 输入文字和光标必须高对比度。浅色输入框使用 `#111827`，深色输入框使用 `#f8fafc`。
- 首页不要形成单一紫色调，品牌紫只用于聚焦、链接、模板强调。
- 禁止使用大面积装饰渐变背景、漂浮光斑或纯营销式 hero。

### 3.2 字体与字号

| 层级 | 字号 | 字重 | 行高 | 使用场景 |
| --- | --- | --- | --- | --- |
| Page H1 | `34-44px clamp` | `860` | `1.25` | 首页标题 |
| Panel H2 | `18px` | `800` | `1.3` | 右侧卡片标题 |
| Control Label | `15px` | `760-780` | `1.2` | pill、tab、按钮 |
| Body | `14-16px` | `400-700` | `1.5-1.7` | 描述、输入 |
| Caption | `12-13px` | `500-760` | `1.35` | 字数、状态、卡片副文案 |

规则：

- 不使用负 letter-spacing。
- 不使用 viewport-width 直接驱动字号。
- 中文按钮文本必须完整显示，移动端允许换布局，不压缩到不可读。

### 3.3 圆角

| Token | Value | 用途 |
| --- | --- | --- |
| `--ppt-radius-sm` | `8px` | 历史卡片、菜单项、列表卡片 |
| `--ppt-radius-md` | `10px` | 下拉菜单、筛选菜单 |
| `--ppt-radius-lg` | `14px` | 模板封面、小卡片 |
| `--ppt-radius-xl` | `22px` | 首页输入卡片、右侧侧栏卡片 |
| `--ppt-radius-pill` | `999px` | pill 参数按钮、圆形图标按钮 |

注意：当前项目对 PPT 首页已形成 22px 的参考项目式生成卡片圆角，可保留。常规卡片和列表仍以 8px 为主。

### 3.4 间距

| Token | Value | 用途 |
| --- | --- | --- |
| `--ppt-space-1` | `4px` | 图标与短文字 |
| `--ppt-space-2` | `8px` | 菜单项内部 |
| `--ppt-space-3` | `10px` | pill gap、列表 gap |
| `--ppt-space-4` | `14px` | 生成器内部 gap |
| `--ppt-space-5` | `18px` | 卡片 padding、小区块间距 |
| `--ppt-space-6` | `24px` | 主栏与侧栏 gap |
| `--ppt-space-7` | `32px` | 页面左右安全边距 |
| `--ppt-space-8` | `52px` | 页面底部留白 |

### 3.5 阴影和动效

| Token | Value | 用途 |
| --- | --- | --- |
| `--ppt-shadow-composer` | `0 18px 48px rgba(90,77,178,0.10)` | 首页生成器 |
| `--ppt-shadow-menu` | `0 18px 60px rgba(0,0,0,0.50)` | 下拉菜单 |
| `--ppt-shadow-card-hover` | `0 14px 30px rgba(90,77,178,0.14)` | 模板卡 hover |
| `--ppt-focus-ring` | `0 0 0 4px rgba(125,141,246,0.12)` | 输入聚焦 |

动效：

- Hover 位移不超过 `-2px`。
- 菜单出现不做复杂弹跳，保持即时反馈。
- loading spinner 使用线性旋转。
- 所有 transition 控制在 `0.16s-0.24s`。

## 4. Layout System

### 4.1 页面栅格

首页容器：

- `.ppt-reference-main.is-home-layout`
- 宽度：`min(1480px, calc(100% - 48px))`
- 栅格：`minmax(0, 1fr) minmax(300px, 356px)`
- Gap：`24px`

主栏：

- 首页生成器、文件库工具栏、历史面板都放在 `grid-column: 1`。
- 主栏不可被右侧栏挤压到小于 0，必须保留 `min-width: 0`。

右侧栏：

- `.ppt-home-side-panel`
- 桌面端 sticky，`top: 18px`
- 只包含热门模板和创作灵感。
- 不承载生成主任务，不放新的大型 CTA。

### 4.2 响应式断点

| Breakpoint | 行为 |
| --- | --- |
| `<=1280px` | 右侧栏缩至 `280-320px`，历史网格 3 列 |
| `<=1120px` | 主栏与右侧栏变单列，右侧栏改为两列卡片 |
| `<=980px` | 页面最大宽度 760px，右侧栏单列，工具栏换行 |
| `<=640px` | 参数 pill 变两列网格，模板卡单列，生成按钮 40px |

移动端要求：

- 不出现横向滚动。
- pill 下拉菜单最大宽度不超过 `calc(100vw - 24px)`。
- 历史卡片列表/网格可扫描，不压缩状态按钮。

## 5. 组件规格

### 5.1 顶部模块栏 `PptTopbar`

实现位置：`PptDocumentGeneratePage.vue`

元素：

- 左上角图标按钮：返回 PPT 工作台。
- 模块标题：`PPT文档生成`。

状态：

- Default：灰色图标和标题。
- Hover：图标背景变深，图标变亮。
- Focus-visible：必须有可见焦点。

交付要求：

- 图标只代表 PPT 工作台，不使用 ALLWEONE 图形。
- 不改变外层用户端左侧菜单结构。

### 5.2 首页生成器 `PptHeroComposer`

实现位置：`PptDocumentGeneratePage.vue`

元素：

- H1：今天想制作什么样的演示文稿。
- Subtitle：说明输入主题或上传参考资料，AI 生成演示文稿。
- Composer card：提示词输入、参数 pill、生成按钮。

尺寸：

- 输入卡最小高度：`268px`。
- textarea 首页态最小高度：`152px`。
- card radius：`22px`。
- card overflow 必须 `visible`，防止 pill 下拉被遮挡。

状态：

- Default：浅色半透明卡片。
- Focus-within：品牌紫边框和 focus ring。
- Loading：保留输入可读性，生成按钮显示 loading。

### 5.3 提示词输入 `PptPromptInput`

实现位置：`admin-vue/src/components/ppt/PptPromptInput.vue`

内容：

- 多行输入。
- placeholder：`请输入你想生成的PPT主题，例如：AI赋能企业营销增长方案`
- 字数：`0/500`。
- 清空按钮：有内容时显示。

视觉规格：

- 首页浅色态文字：`#111827`。
- 深色态文字：`#f8fafc`。
- caret：必须高对比度。
- placeholder：`#8d8d93` 或浅色态 `#9ca3af`。

交互：

- `Ctrl + Enter` / `Command + Enter` 提交。
- 清空后同步重置选中示例。
- 输入为空时生成按钮 disabled。

验收：

- 输入文字不能出现“看不见”的问题。
- 字数统计不能遮挡输入内容。
- 移动端 textarea 宽度必须等于容器宽度。

### 5.4 参数 Pill `PptSettingPill`

实现位置：`PptDocumentGeneratePage.vue`

Pill 列表：

- 幻灯片页数：默认 5，可选 1-12 页。
- 演示格式：动态的、16:9。
- 语言：中文、英文。
- 更多：联网搜索、主题风格、自动主题等高级项。
- 云模型：按 provider 分组展示可用模型。

视觉：

- 高度：`44px`，移动端 `38px`。
- Radius：`999px`。
- Gap：`8px`。
- 图标：`17px`，stroke `2`。

下拉菜单：

- `.ppt-pill-menu`
- 宽度：普通 `210px`，更多 `230px`，模型 `320px`。
- z-index：`24`，模型不低于 `24`。
- 最大高度：`min(420px, calc(100vh - 160px))`。
- 打开后点击其他区域必须关闭。
- `Esc` 必须关闭。

状态：

- Default：深色按钮。
- Hover：`#181818`。
- Expanded：与 Hover 一致。
- Active item：菜单项背景 `#1f1f1f`。
- Disabled item：透明度 `0.48`，不可点击。

### 5.5 生成按钮 `PptSubmitButton`

实现位置：`PptDocumentGeneratePage.vue`

内容：

- 首页使用圆形箭头按钮。
- 有输入且非 loading 时可点击。
- Loading 文案或 aria 状态：正在生成 PPT，请稍候。

状态：

- Disabled：主题为空、正在生成、权限不足时禁用。
- Loading：防重复点击。
- Success：进入工作流或历史更新。
- Error：保留提示词并显示错误提示。

交付要求：

- 不写死无效下载链接。
- 不在按钮区域显示视频生成文案。

### 5.6 右侧热门模板 `PptHomeTemplates`

实现位置：`PptDocumentGeneratePage.vue`

内容：

- 标题：热门模板。
- 操作：更多模板。
- 模板卡：2 列网格，移动端 1 列。
- 每张卡包含 tag、subtitle、title。

视觉：

- 侧卡 radius：`22px`。
- 模板封面 radius：`14px`。
- 封面 min-height：`78px`。
- Hover：封面上移 `-2px`，阴影增强。

交互：

- 点击模板填充 prompt。
- 不自动进入生成，用户仍需确认并点击生成。
- 选中模板后关闭其他配置菜单。

### 5.7 创作灵感 `PptHomeInspirations`

实现位置：`PptDocumentGeneratePage.vue`

内容：

- 标题：创作灵感。
- 操作：换一批。
- 每项包含 34px 图标、标题、右箭头。

视觉：

- 行高最小 `48px`。
- 行内 icon 使用柔和品牌色背景。
- 每项底部分隔线，最后一项无分隔线。

交互：

- 点击灵感填充 prompt。
- 换一批轮换可见灵感列表。
- 灵感列表可以承载未来业务场景推荐，不新增独立业务入口卡。

### 5.8 文件库工具栏 `PptLibraryToolbar`

实现位置：`PptDocumentGeneratePage.vue`

左侧 Tab：

- 全部。
- 最近浏览。
- 收藏夹。

右侧 Actions：

- 搜索。
- 创建新的。
- 排序筛选。
- 网格/列表切换。

视觉：

- Tab 高度 `44px`。
- Action 按钮高度 `42px`。
- View toggle 使用 segmented control。

交互：

- 搜索按钮点击后展开输入框。
- 筛选菜单打开后可排序、仅看收藏、筛选类型。
- 网格/列表切换必须即时改变历史卡片布局。

### 5.9 历史卡片 `PptHistoryCard`

实现位置：`admin-vue/src/components/ppt/PptHistoryList.vue`

Grid 模式：

- 默认 3 列，页面库面板内可 4 列。
- 卡片包含预览、标题、时间、页数/语言/主题、状态。
- Hover 显示星标和更多按钮。

List 模式：

- 缩略图 `96x58px`。
- 内容区居中垂直对齐。
- 操作按钮常显，便于扫描。

操作：

- 点击卡片主体：预览。
- 星标：收藏/取消收藏。
- 更多菜单：继续编辑、重新生成、收藏、下载 PPT、下载 PDF、删除。

状态：

- 成功：绿色 tag。
- 失败：红色 tag。
- 生成中：信息 tag。
- 下载不可用：按钮 disabled，并说明文件尚未生成。

### 5.10 空状态与错误状态

实现位置：

- `PptEmptyState.vue`
- `PptErrorState.vue`

空状态：

- 历史为空时提示：暂无演示文稿。
- 说明：生成完成后会出现在最近生成记录中。

错误状态：

- 显示错误原因。
- 提供重试入口。
- 不清空 prompt、配置项和已生成的大纲。

## 6. 交互模型

### 6.1 首页生成流程

1. 用户输入主题。
2. 可选页数、语言、联网搜索、主题、模型。
3. 点击生成按钮。
4. 按当前 store 状态进入大纲/生成工作流。
5. 成功后写入历史记录。
6. 用户可预览、继续编辑、导出或重新生成。

### 6.2 下拉互斥规则

同一时间只允许打开一个菜单：

- 页数菜单。
- 格式菜单。
- 语言菜单。
- 更多菜单。
- 模型菜单。
- 历史筛选菜单。

打开任意菜单时必须关闭其他菜单。点击 `.ppt-pill-dropdown` 外部区域时关闭所有 pill 菜单。

### 6.3 历史卡片操作规则

- 卡片主体点击打开预览。
- 卡片上的星标和更多菜单必须阻止事件冒泡，不能触发预览。
- 更多菜单点击操作后关闭。
- 删除前必须有确认提示。

### 6.4 生成按钮防重复

当 `status` 为 pending、outlining、generating、rendering 时：

- 生成按钮 disabled 或 loading。
- 文案/aria 表达为正在生成。
- 不允许重复发起创建任务。

## 7. 开发交付说明

### 7.1 相关文件

| 类型 | 路径 |
| --- | --- |
| 页面主组件 | `admin-vue/src/components/ppt/PptDocumentGeneratePage.vue` |
| 输入组件 | `admin-vue/src/components/ppt/PptPromptInput.vue` |
| 历史卡片 | `admin-vue/src/components/ppt/PptHistoryList.vue` |
| 生成进度 | `admin-vue/src/components/ppt/PptGenerationProgress.vue` |
| 空状态 | `admin-vue/src/components/ppt/PptEmptyState.vue` |
| 错误状态 | `admin-vue/src/components/ppt/PptErrorState.vue` |
| Store | `admin-vue/src/stores/ppt.ts` |
| API | `admin-vue/src/api/ppt.ts` |
| 类型 | `admin-vue/src/types/ppt.ts` |
| 主题配置 | `admin-vue/src/config/pptThemes.ts` |

### 7.2 CSS 组织建议

当前 `PptDocumentGeneratePage.vue` 已承担大量页面样式。后续维护建议按以下顺序拆分，但第一版不强制：

1. `PptHomeComposer.vue`
2. `PptSettingPill.vue`
3. `PptHomeSidePanel.vue`
4. `PptLibraryToolbar.vue`
5. `PptPresentationWorkspace.vue`

拆分前不要重命名现有 class，以免破坏已验证样式和测试脚本。

### 7.3 参数字段映射

| UI 控件 | Store/API 字段 | 说明 |
| --- | --- | --- |
| 提示词 | `prompt` | 必填，最大 500 字 |
| 幻灯片页数 | `slideCount` | 当前可选 1-12，默认 5 |
| 语言 | `language` | `zh` / `en` |
| 联网搜索 | `enableWebSearch` | 默认关闭 |
| 主题 | `theme` | business、techBlue、education 等 |
| 动态/16:9 | `generationAspectRatio` | dynamic / 16:9 |
| 文本模型 | `textModel` | 从模型分组中选择 |

### 7.4 API 对接要求

创建任务：

```http
POST /api/ppt/generate
```

查询任务：

```http
GET /api/ppt/tasks/{taskId}
```

历史记录：

```http
GET /api/ppt/history
```

删除记录：

```http
DELETE /api/ppt/tasks/{taskId}
```

接口未接入时允许 mock，但 UI 不能表现成假下载成功。下载按钮必须基于 `pptUrl` / `pdfUrl` 是否存在来启用。

### 7.5 可访问性要求

- 所有 icon-only button 必须有 `title` 和 `aria-label`。
- 下拉触发器必须有 `aria-expanded` 和 `aria-haspopup`。
- 菜单项使用 `role=menuitem` 或 `menuitemradio`。
- 网格/列表切换使用 `aria-pressed`。
- 搜索输入有明确 `aria-label`。
- 禁用按钮必须提供原因性 title。
- `Esc` 关闭菜单。
- Tab 焦点顺序应从输入、pill、生成按钮、文件库工具栏、历史卡片依次前进。

### 7.6 性能要求

- 首页首屏不应等待编辑器重组件全部加载。
- 大纲编辑器、幻灯片编辑器、主题面板等应继续使用 `defineAsyncComponent`。
- 首页模板和灵感数据保持本地常量，不发额外接口。
- 页面打开慢时优先检查是否有同步加载的编辑器资源、图片搜索或模型列表接口。

## 8. QA 清单

### 8.1 桌面端

- 首页标题、输入框、pill、生成按钮在第一屏可见。
- 右侧只有热门模板和创作灵感，不出现额外业务入口卡片。
- 输入中文后文字、光标、字数统计清晰可见。
- 未输入主题时生成按钮不可点击。
- 打开页数菜单后，菜单不被 composer 裁剪。
- 打开一个 pill 菜单后再打开另一个，前一个自动关闭。
- 点击页面其他区域，pill 下拉关闭。
- 搜索按钮可展开并输入文字。
- 网格/列表切换后卡片布局变化。
- 历史卡片星标和更多按钮不会误触发预览。
- 下载按钮在无 URL 时 disabled。

### 8.2 移动端

- 390px 宽度无横向滚动。
- 参数 pill 以两列显示，文本不溢出。
- 右侧侧栏内容下移并单列展示。
- 热门模板变单列。
- 历史卡片在窄屏可读，可操作。
- 下拉菜单不会超出视口。

### 8.3 状态与错误

- 生成中按钮 loading，不能重复点击。
- 生成失败保留 prompt 和配置。
- 删除历史前出现确认。
- mock 成功结果写入历史并能预览。
- 真实接口失败时展示错误提示，不静默失败。

### 8.4 回归范围

每次修改 PPT 页面后必须确认：

- 生图页面可进入。
- 视频生成页面可进入，输入文字仍清晰。
- AI 画布可进入。
- 作品中心可进入。
- 左侧菜单高亮不串路由。
- `/app/ppt-generation` 刷新后仍能定位到 PPT 页面。

## 9. 交付验收标准

设计验收：

- 页面第一屏与参考项目保持紧凑生成器结构。
- 先知 AI 品牌只体现在系统导航和必要强调色，不出现外部品牌。
- 所有控件状态完整：default、hover、focus、active、disabled、loading、empty、error。

开发验收：

- `npm.cmd --prefix admin-vue run build -- --emptyOutDir false` 通过。
- 本地容器同步后 `/app/ppt-generation` 返回 200。
- Playwright 或浏览器验证覆盖：输入、pill 下拉、生成按钮、历史网格/列表、移动端。
- 不引入批量删除、无关重构或跨模块样式污染。

## 10. 后续优化建议

短期：

- 将 pill 下拉抽成可复用组件，统一 outside click、Esc、焦点管理。
- 给首页生成器补 skeleton，降低慢加载感知。
- 给历史卡片增加最近打开排序的轻量提示。

中期：

- 将右侧热门模板和创作灵感配置化，允许后端或运营控制内容。
- 接入真实 PPT 生成服务后补任务轮询、失败重试和导出状态。
- 生成成功后提供「继续编辑」与「下载」双主路径，但不要在首页新增过多 CTA。

长期：

- 建立 PPT 专属视觉 token 文件，逐步从单文件 scoped CSS 迁移到组件化样式。
- 统一生图、视频、PPT 三个生成模块的任务状态语义，但保留各自独立页面和接口。
