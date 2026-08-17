# PPT V2 Phase 3 Slice C 实施报告

## 1. 范围与结论

Slice C 实现 Geometry-authoritative Preview Workspace：用户可以在下载 PPTX 前查看已完成的 6～12 页 Professional Deck，并在同一工作台中切换页面、查看来源和下载既有 PPTX artifact。

本 Slice 只读展示 Deck，不修改 SlideIR，不实现自由编辑、局部重生成或对话式修改；这些能力属于 Slice D。

## 2. 架构

Preview 使用现有 `DeckRevision`、`SlideIR` 与 `LayoutResult`。后端只生成面向展示的 `PreviewProjection`，不创建第二套 Web Deck domain model。

渲染链路为：

```text
SlideIR + LayoutResult + stable asset identity
        ├── PptxRenderer  → PPTX
        └── PreviewRenderer → DOM/CSS Preview
```

`LayoutResult` 是唯一 geometry authority。Preview 只读取已编译的 x、y、width、height、z-order 和画布尺寸，不重新排版。

## 3. PreviewRenderer 与几何一致性

`PptAgentSlideCanvas` 支持 Slice B 的 P0 元素：Text、Bullets、Shape、Image。元素使用绝对定位，直接映射 LayoutResult；字体、对齐、填充、边框、透明度、圆角/椭圆、图片 fit/crop 和层级均从现有投影读取。

主画布和缩略图复用同一个 PreviewRenderer。浏览器尺寸变化只对整张 960×540 画布应用统一 scale transform，不触发文字重排、列重排或新的布局计算。Preview fidelity 测试验证 elementId、边界和 z-order 与 LayoutResult 一致，并允许按明确 tolerance 进行浮点比较。

## 4. 后端 Preview API

沿用 `/api/v1/ppt` API family，新增：

`GET /api/v1/ppt/agent/jobs/:jobId/preview?revision=<n>`

接口只允许读取已完成且 revision 匹配的 Deck。响应包含：

* Deck identity、revision、SlideIR projection；
* LayoutResult projection；
* 实际被引用的 private asset 的 stable assetId、短期访问 URL、过期时间、MIME type 和 alt text。

后端通过现有 tenant/owner scope 校验 job、revision 和 asset。跨租户、跨 owner、stale revision、未完成 job、缺失布局或非法元素均 fail closed。响应不会泄露 FileID、TenantID、UserID、provider raw response 或 secrets；任意 query assetId 不会扩大可访问资产集合。

## 5. Asset resolver 与安全

Preview 通过 stable asset identity 调用现有 private storage / `AccessURL`。signed URL 只存在于本次响应，不写回 SlideIR 或 Deck，也不作为领域权威。URL 过期时 UI 可重新请求 projection，复用已持久化 asset，不重新生成图片。

资产访问保持 tenant/owner 授权；缺失、过期或不可访问资产会显示可理解的错误和重试入口，不静默显示空白画布。

## 6. 工作台体验

现有唯一 PPT Agent 工作台在 Deck 完成后显示 Preview Workspace，包括：

* 当前页大画布；
* 全部 slide thumbnails、当前页高亮、页码和总页数；
* 上一页/下一页、点击缩略图切页和键盘方向键导航；
* 当前页 Sources/Evidence 面板，展示 claim、source title、citation、verification status 和 rationale；
* 使用既有 artifact 的“下载 PPTX”；
* loading、unauthorized、revision unavailable、asset missing/expired 和 API failure 的明确状态与重试。

缩略图与主画布使用同一 SlideIR + LayoutResult 投影。刷新后根据 durable job/deck identity 恢复当前 Preview，不需要重新生成 PPTX。

导航控件具备可理解 label、当前页状态和键盘操作；图片使用资产 alt text 或语义描述。

## 7. 测试与 Golden 2

新增后端测试覆盖：

* Preview projection 与持久化 Deck/LayoutResult 等价；
* 几何、slide identity、revision 和 asset URL；
* stale revision、跨 owner、跨 tenant 和任意 asset query 拒绝；
* provider/private payload 不泄露；
* replay 不重新生成 asset。

新增前端测试覆盖：

* 6、8、10、12 页缩略图与导航；
* geometry/style/element fidelity；
* evidence/source 展示、image、loading/error；
* store restore、asset URL refresh 和新任务清理旧 Preview。

新增 Golden 2 Preview visual regression。Golden 2 为 8 页 Professional Deck，覆盖 Cover、Section、Bullets、Text + Image、Two-column、Highlight、Image + Text 和 Closing；Playwright visual gate 验证 Cover、Bullets、Image、Two-column、Highlight、Closing。Golden 2 同时继续验证稳定 slide identity、布局几何和 PPTX integrity。

## 8. Durable 与回归边界

Slice C 是 read-oriented Preview，不新增核心 Job 状态机、不记录 last viewed slide 到 Deck domain，也不改变 Slice A/A.1/B 的 durable stages、lease、fencing、retry、artifact transaction 或任务关系。

本 Slice 不新增数据库 migration。现有 Phase 2、Slice A、Slice A.1、Slice B 的 PostgreSQL gate 继续由 CI 实际执行。

## 9. 最终 CI 证据

PR #6 最新 GitHub Actions run `31945770383` 已通过：

* `backend-go`: GREEN；PostgreSQL、PPT V2 gates 和完整 backend 测试成功；
* `user-core`: GREEN；
* Node regression、Admin unit tests、Golden 2 Preview visual regression、typecheck、API client boundary checks 全部成功；
* OfficeCLI 安装与 PPT integrity 流程成功；
* H5、Admin、WeChat、App Plus、Harmony App 和 Harmony mini program 构建成功；
* Golden 1、Golden 2 及 Phase 1/2、Slice A/A.1/B regression 保持绿色；
* 未出现新的 CI failure。

## 10. 已知限制

Slice C 只提供忠实预览，不提供文本编辑、元素拖拽、缩放、裁剪、EditCommand、局部 regenerate、Undo、Revision UI、Preview annotation 或协作能力。Chart/Table/Diagram、Creative Mode、PDF/Google Slides 和企业功能不在本 Slice。

Preview 仍依赖已完成的 SlideIR、LayoutResult 和可解析的 private asset；上游生成失败时不会伪造预览。浏览器端使用 DOM/CSS 投影，尚未提供像素级 Office 渲染等价性保证，但 geometry fidelity 和 Golden 2 visual regression 已建立自动化门槛。

## 11. Slice C Exit Gate

以下条件均已满足：

* 6～12 页 Deck 可完整 Preview；
* Preview 与 LayoutResult geometry 一致且不重新布局；
* Text、Bullets、Shape、Image 正确显示；
* thumbnails、导航、当前页和刷新恢复可用；
* asset resolver tenant/owner-safe，过期 URL 可重新 resolve；
* 当前页 evidence/source 可查看；
* 可从 Preview 下载既有 PPTX artifact；
* Golden 2 Preview visual regression、Golden 1、Golden 2 PPTX 和 OfficeCLI 通过；
* Phase 2、Slice A、Slice A.1、Slice B PostgreSQL/regression gates 通过；
* backend-go 与 user-core GREEN，无 NEW_FAILURE。

`SLICE C STATUS: READY`

本报告完成后停止，不进入 Slice D。
