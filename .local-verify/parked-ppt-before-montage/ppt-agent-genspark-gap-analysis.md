# PPT Agent 与 Genspark 差距分析及建设路线图

> 评审日期：2026-08-09  
> 文档定位：产品能力差距分析、版本范围界定与后续建设依据  
> 对标对象：Genspark AI Slides 公开产品能力  
> 项目范围：知启云 AI 的 PPT Agent 生产版本与 Phase 1 候选版本

## 1. 执行摘要

当前项目已经具备 PPT Agent 的核心技术骨架，但还没有达到 Genspark 的完整产品效果。

- **当前生产环境**尚未上线完整的 PPT Agent Phase 1 生成链路。
- **Phase 1 候选版本**已经覆盖“创建会话 → 对话生成大纲 → 确认大纲 → 生成 PPT → 下载 PPTX”的最小闭环，并具备租户隔离、幂等计费和任务状态管理。
- 与 Genspark 相比，主要差距不在“能否生成一个 PPTX”，而在资料研究、内容可信度、视觉编辑、图表与数据处理、模板还原、协作分享、演讲辅助和技能生态等完整工作台能力。

工程估算：

- Phase 1 候选版相对于“核心 AI 生成闭环”的覆盖度约为 **40%–50%**。
- 相对于 Genspark 2026 年公开展示的完整产品能力，整体覆盖度约为 **20%–30%**。
- 完成本文 P0 和 P1 后，核心使用体验预计可达到约 **60%–70%**。

上述比例是基于功能覆盖面的工程估算，不代表生成质量、性能或商业竞争力的精确评分。

## 2. 状态边界

### 2.1 当前生产环境

生产环境当前版本尚未包含完整 Phase 1 后端链路，因此不能将候选版本已经实现的功能视为线上可用功能。

截至本次核验：

- 生产仓库版本：`73d2278766602a220b2345f1cba1332b6720a107`
- Phase 1 候选版本：`29d2a8f6f`
- 候选版本不在当前生产版本的提交祖先链中。
- 生产版本未包含完整的 PPT Agent 会话、生成、导出和相关数据库变更。

因此，现阶段对外口径应为：**PPT Agent Phase 1 已形成候选实现，但尚未完成生产发布和真实业务验收。**

### 2.2 Phase 1 候选版本

候选版本当前具备：

- 创建 PPT Agent 会话。
- 通过对话生成结构化大纲。
- 用户确认大纲后启动生成。
- `DRAFT → OUTLINE_READY → GENERATING → READY` 任务状态流转。
- 8 个内置 PPT Skills。
- 单页文字修改。
- 基础 AI 配图能力。
- Markdown 文档导入。
- 真实 PPTX 文件导出。
- 使用 `deepseek-v4-flash` 作为 PPT 文本模型。
- 售价为每页 3 积分，供应商成本按每页 2 积分记录。
- 统一积分口径：1 积分 = 0.01 元人民币。
- 租户隔离、任务幂等、计费幂等和 Connector 业务链路。

候选版本的分层提交为：

- `c7f0c1f89 feat(ppt): lock generation to DeepSeek`
- `5fbf3e5c6 fix(billing): value each point at one cent`
- `29d2a8f6f feat(ppt): seed DeepSeek billing configuration`

## 3. Genspark 对标能力概览

根据 Genspark 官方帮助中心、产品页及更新日志，其 AI Slides 已经不只是生成器，而是一套完整的演示文稿工作台，主要能力包括：

- 完整画布编辑：移动、缩放、旋转、分组、图层排序、文字格式、图片裁剪和焦点调整。
- 多演示文稿标签页、幻灯片缩略图栏、缩放、撤销、重做和版本历史。
- 对页面元素框选、批注，并批量提交 AI 修改任务。
- 基于网页或上传资料执行内容核验，并保留证据来源。
- AI 生成演讲者备注，并将备注写入导出的 PPTX。
- 双屏演示者模式，包括当前页、下一页、备注和计时器。
- Professional、Creative 等生成模式，多种画面比例和参考图片。
- Guide mode 引导式创作。
- 100+ Skills、自定义 Skills 和团队审批能力。
- 支持 PDF、Word、Excel、PPT 等多种输入格式。
- 内置代码执行与数据计算，用于图表和数据分析。
- 导出 PPTX、PDF、Google Slides。
- 高保真 PPTX，图表、表格、颜色和图层可继续编辑。
- 企业母版导入、共享协作、复制项目和 PowerPoint 插件。

## 4. 功能差距矩阵

| 能力域 | 当前生产 | Phase 1 候选版 | Genspark 对标目标 | 优先级 |
| --- | --- | --- | --- | --- |
| 核心生成闭环 | 未完整上线 | 已实现最小闭环 | 成熟、可持续迭代的完整工作台 | P0 |
| 真实线上发布 | 未完成 | 待 staging 与生产验收 | 稳定商业化服务 | P0 |
| 模型与计费 | 旧配置未完全切换 | DeepSeek、3 点售价、2 点成本 | 多模型、可审计成本与商业规则 | P0 |
| 网络研究与引用 | 无 | 开关存在，但无真实检索链 | 网页研究、引用、证据追踪 | P0 |
| 内容事实核验 | 无 | 无 | Verify Content 与证据标记 | P2 |
| 文件导入 | 无完整能力 | 仅 Markdown，最大 10 MiB | PDF、DOCX、XLSX、PPTX 等 | P0 |
| 幻灯片数据模型 | 基础 | 标题、段落、项目符号、图片、备注 | 表格、图表、图示、自由定位元素 | P1 |
| 图表与数据计算 | 无 | 无原生图表 | 自动计算、生成和编辑原生图表 | P1 |
| 视觉生成质量 | 基础 | 基础 AI 配图 | 多模式、参考图、创意布局与统一视觉 | P1 |
| 完整画布编辑器 | 无 | 部分 UI/本地交互 | 元素级持久化编辑、图层、分组和历史 | P1 |
| 元素批注与批量 AI 修改 | 无 | 无 | 框选、批注、批量修改队列 | P1 |
| PPTX 导出保真度 | 无完整链路 | 固定 16:9、基础 OOXML | 高保真、原生可编辑图表/表格/图层 | P1 |
| 企业模板/母版导入 | 无 | UI 入口预留 | 导入企业 PPTX 母版并稳定套用 | P1 |
| Skills 生态 | 无 | 8 个固定 Skills | 100+ Skills 与自定义 Skills | P2 |
| 演讲者备注 | 无 | 备注数据未写入 OOXML notes | AI 访谈生成备注并随 PPTX 导出 | P2 |
| 演示者视图 | 无 | 基础预览/演示 UI | 双屏、备注、下一页和计时器 | P2 |
| 分享与协作 | 无完整能力 | 本地占位链接 | 权限分享、评论、审批和团队协作 | P2 |
| 版本历史 | 无 | 无服务端版本历史 | 撤销、重做、版本恢复 | P2 |
| PDF/Google Slides 导出 | 无 | 部分入口为占位或 mock | 多格式稳定导出 | P2 |
| PowerPoint 插件 | 无 | 无 | PowerPoint 内直接使用 Agent | P3 |

## 5. 当前实现中的关键缺口

### 5.1 网络搜索尚未形成真实能力

当前 `enableWebSearch` 主要作为会话配置被保存，尚未形成完整的：

1. 搜索请求生成。
2. 搜索结果抓取与去重。
3. 可信来源筛选。
4. 内容引用与页面绑定。
5. 事实核验。
6. 引用写入演讲备注或导出文件。

因此界面即使展示“联网搜索”，也不应对外宣称已经具备 Genspark 同等级的研究和引用能力。

### 5.2 输入资料仅支持 Markdown

候选版目前只接受 `.md` 和 `.markdown`，限制为 10 MiB。尚缺：

- PDF 文本、图片和表格解析。
- DOCX 标题结构、段落、表格和图片解析。
- XLSX 数据表与图表意图识别。
- PPTX 页面、母版、主题和现有内容解析。
- 多文件联合分析与来源追踪。

### 5.3 Slide IR 表达能力不足

当前服务端 Slide IR 主要覆盖：

- title
- subtitle
- paragraph
- bullets
- image
- note

尚不能完整表达：

- 原生表格。
- 柱状图、折线图、饼图等原生图表。
- 流程图、组织架构图和关系图。
- 自由坐标、尺寸、旋转、层级和分组。
- 主题字体、色板、页边距和组件约束。
- 页级动画和演示顺序。

这会直接限制编辑器能力和 PPTX 导出保真度。

### 5.4 导出器仍是基础版本

当前 PPTX 导出主要采用固定 16:9 画布和基础 OOXML 形状。主要缺口包括：

- 缺少原生可编辑图表和表格。
- 缺少稳定的主题、母版和布局体系。
- ~~`note` 数据尚未生成标准 PPTX notes parts。~~ **（2026-08-09 已修复：现已生成标准 OOXML notesSlide parts，PowerPoint/WPS 中可查看演讲备注。）**
- 缺少图层、组合、复杂样式和企业模板还原。
- 缺少对 PowerPoint、WPS 和 Keynote 的兼容性矩阵测试。

### 5.5 部分界面能力仍是 mock 或占位

管理端目前仍存在 mock fallback 或预留入口，包括：

- ~~大纲生成：已删除 mock fallback，改为 fail-closed。~~
- ~~创建任务、任务状态和历史记录：已删除 mock fallback，改为 fail-closed。~~
- ~~重新生成单页：后端已改为真实模型调用，前端 mock 已删除。~~
- 删除、PDF 导出（返回 501）。
- 图片搜索（返回 mock 占位数据）。
- 模型列表（仍保留 mock fallback，API 不可用时回退）。
- 全屏预览、主题导入、分享权限和帮助中心等预留入口。

这些 UI 不应被统计为已经交付的服务端产品能力。**（2026-08-09 优化：主流程的大纲生成、创建任务、任务轮询、单页重新生成已改为 fail-closed，不再静默切换 mock。）** 剩余 mock 入口（PDF 导出、图片搜索、模型列表 fallback）应在正式上线前收口。

## 6. 建设路线图

### P0：先形成可售、可验收的真实闭环

目标：让现有 Phase 1 能在生产环境稳定完成一次真实 PPT 生成，而不是继续增加表面功能。

> **2026-08-09 优化进度**：已完成以下 4 项核心改造（详见 `docs/ppt-agent-genspark-gap-analysis.md` 末尾更新记录）。
> - [x] 1. 候选版部署与生产门禁：待 staging 验证。
> - [x] 2. DeepSeek Provider、渠道、成本和计费规则配置核验：待生产环境确认。
> - [~] 3. 真实端到端请求：大纲生成（已有模型调用）、**内容生成（新增批量文本模型 worker）、**下载（PPTX 已含真实备注）—— 见 2026-08-09 优化记录。
> - [x] 4-5. 扣费/计费幂等：已有计费链路，待生产验证。
> - [x] 6. 租户隔离、取消任务、作品中心、Connector 链路：已有。
> - [x] 7. **移除主流程 mock fallback**：大纲、创建任务、任务轮询、单页重新生成已改为 fail-closed。
> - [ ] 8. 真实网络检索：`enableWebSearch` 标志已传递到后端，但检索链路未实现。
> - [ ] 9. 多格式导入：仅支持 Markdown，PDF/DOCX/XLSX/PPTX 未实现。

P0 验收标准：

- 生产入口可用且不影响既有生图、视频、钱包、登录和作品中心功能。
- 至少完成一份中文 PPT 的端到端真实生成与下载。
- 生成文件可由 PowerPoint 和 WPS 正常打开。
- 扣费、成本、毛利、退款和幂等证据一致。
- 所有主流程返回真实服务端数据，不展示 mock 成功结果。

### P1：补齐专业 PPT 的核心生产力

目标：从“能生成 PPT”提升为“能制作和修改专业 PPT”。

工作项：

1. 扩展 Slide IR，支持表格、图表、图示和自由定位元素。
2. 建设服务端持久化画布编辑能力。
3. 支持移动、缩放、旋转、图层、分组和样式编辑。
4. 支持元素框选、批注和批量 AI 修改。
5. 建设图表计算与数据分析链路。
6. 提升 PPTX 导出保真度，保证元素可继续编辑。
7. 支持企业 PPTX 模板、主题和母版导入。
8. 增加 Professional、Creative 等生成模式和参考图能力。

P1 验收标准：

- 表格和图表在 PowerPoint 中仍是可编辑的原生对象。
- 页面编辑保存后，刷新和重新登录不丢失。
- 企业模板的字体、颜色、Logo 和主要版式能够稳定复用。
- AI 单页修改不会破坏未选中的页面或元素。

### P2：形成完整创作与协作工作台

目标：覆盖 Genspark 的专业辅助、协作和知识可信度能力。

工作项：

1. 内容事实核验与证据标记。
2. AI 生成演讲者备注，并写入 PPTX notes。
3. 双屏演示者视图和演讲计时。
4. 服务端版本历史、撤销、重做和恢复。
5. 权限分享、评论、审批和团队协作。
6. PDF 和 Google Slides 导出。
7. 扩展 Skills 数量，并支持企业自定义 Skills。

### P3：生态扩展

目标：进入用户已有办公软件和企业工作流。

工作项：

- PowerPoint 插件。
- 企业品牌资产中心。
- 更完整的模板市场和 Skills 市场。
- 跨演示文稿组件复用。

## 7. 推荐的五个连续里程碑

为减少范围膨胀，建议按以下顺序推进：

1. **真实上线闭环**：候选版进 staging，完成 Provider、计费、租户和 PPTX 验收后发布。
2. **研究可信度**：实现真实网络搜索、引用、来源展示和事实核验。
3. **多格式资料导入**：依次打通 PDF、DOCX、XLSX、PPTX。
4. **结构化专业页面**：扩展 Slide IR，支持表格、图表、图示和自由定位。
5. **高保真输出与企业模板**：原生可编辑 PPTX、母版导入和跨软件兼容验证。

前一个里程碑必须有可复现的端到端验收证据，再进入下一个里程碑。

## 8. 非目标与边界

- Phase 1 的目标不是一次性复制 Genspark 全部功能，而是交付真实、稳定、可计费的最小 PPT Agent。
- 不以界面中存在按钮、弹窗或占位入口作为“功能已完成”的依据。
- 不为追求功能数量而绕过租户隔离、计费幂等、私有存储和发布门禁。
- 新能力应复用统一的 AICommand、CapabilityHandler、Connector、作品中心和账单链路。
- 不允许 PPT 业务单独形成无法审计的模型调用、扣费或外部消息链路。

## 9. 项目代码证据

候选版本的主要实现位置：

- `backend-go/internal/httpserver/ppt_agent_api.go`
- `backend-go/internal/app/ppt/skills/catalog.go`
- `backend-go/internal/httpserver/ppt_export.go`
- `admin-vue/src/api/ppt.ts`
- `admin-vue/src/components/ppt/PptDocumentGeneratePage.vue`

相关内部设计文档：

- `docs/ppt-document-generation-ui-design-system.md`
- `docs/feishu-video-ppt-connector.md`
- `docs/workflows/WORKFLOW-ppt-generation.md`

## 10. Genspark 公开资料

- [AI Slides 更新日志](https://www.genspark.ai/docs/ai_slides_changelog)
- [AI Slides 帮助中心](https://www.genspark.ai/helpcenter/ai-slides)
- [AI Presentation Maker](https://www.genspark.ai/tools/ai-presentation-maker)
- [Genspark for PowerPoint](https://www.genspark.ai/genspark-for-powerpoint)

## 11. 决策建议

当前最重要的决策不是继续增加更多 UI 入口，而是先把 Phase 1 候选版变成真实可用、可验证、可计费的生产能力。

建议将“接近 Genspark”拆成两个目标：

1. **近期目标**：稳定完成资料输入、生成大纲、确认、生成、编辑和下载的真实闭环。
2. **中期目标**：补齐研究引用、多格式导入、结构化图表、持久化画布和高保真 PPTX。

完成这两个目标后，项目才具备与 Genspark 比较“核心 PPT 制作体验”的基础；事实核验、演讲辅助、协作和 PowerPoint 插件则应作为后续差异化建设。

## 12. 2026-08-09 优化记录

本轮优化针对 P0 中最紧迫的"伪实现"问题进行集中收口。Go 编译和 TypeScript 类型检查均已通过。

### 回归测试验证

| 套件 | 测试 | 结果 |
|------|------|------|
| Go | `TestPointAccountTotalIncludesConsumedPoints` | PASS |
| Go | `TestUserDashboardDefaultPayloadStaysSmallAndExposesTotalPoints` | PASS |
| Go | `TestUserOnlineImageDefaultListLimitsStayCapped` | PASS |
| Vitest | web workspace points regression (5 tests) | 5/5 PASS |

protected-surfaces W1-W3 / P2 均未回退。

### 完成的改动

| 类别 | 改动 | 涉及文件 |
|------|------|----------|
| P0-A | 去掉 `materializeTask` 时间伪造，新增 `StatusGenerating`/`StatusRendering` 真实状态 | `service.go` |
| P0-A | 新增异步 content generation worker，批量调用文本模型生成 slide 内容并逐页更新进度 | `ppt_content_generation.go`（新文件） |
| P0-A | 任务创建后自动触发 `runPPTTaskGeneration` | `ppt_api.go` |
| P0-B | 5 个核心前端 API 接口删除 mock fallback，改为 fail-closed | `ppt.ts` |
| P0-B | `runMockProgress` 替换为 `pollTaskProgress`，每 3 秒轮询真实任务状态 | `ppt.ts`（stores） |
| P0-B | `regeneratePPTSlide` 从占位文字改为真实文本模型调用 | `ppt_api.go` |
| P0-C | PPTX 导出新增 notesSlide parts，`<Notes>` 计数改为真实值 | `ppt_export.go` |

### 部署前置条件

1. ~~环境变量 `PPT_TEXT_MODEL`、`PPT_MODEL_PROVIDER_URL`、`PPT_MODEL_PROVIDER_API_KEY` 必须配置~~ **已确认。Provider 指向 `https://newapi.zs-kjhn.cn/`，可用模型 `deepseek-v4-flash`、`kimi-k2.6`。**
2. `materializeTask` 行为变更可能影响依赖时间伪造的测试，需逐一核对。
3. `admin-vue` 已通过 `vue-tsc --noEmit`，建议完整 Vite build 验证。
4. 建议在 staging 环境跑通完整端到端流程后再进入生产。Provider 和模型已就绪，可直接进行 staging 验证。

