# PPT Generation V2.1 Contract

本目录定义 Phase 1 Minimal Vertical Slice 的 canonical domain contract。`2.1` 显式取代 Phase 0 `2.0` 的 renderer-facing combined deck；不保留兼容 renderer、migration fallback 或双写。

权威边界：

- `DeckRevision / SlideIR` 是内容权威：文本、bullet、speaker notes、元素类型、slot 与顺序只在这里定义，不含几何。
- `LayoutResult` 是 geometry 权威：`x/y/width/height/zIndex/resolvedStyle/diagnostics` 只在这里定义，单位固定为 point。
- `RenderInput` 是 renderer port 输入：显式携带 deck revision、SlideIR、LayoutResult、DesignSystem、Asset Manifest 与 render options。
- renderer 只按 `elementId` 连接内容与布局，按 `zIndex` 输出；缺少映射或存在 error diagnostic 时 fail closed。
- Phase 1 renderer 只接受 `TextElement`、`ShapeElement` 与 speaker notes。没有图片下载、chart、table、diagram、research、citation 或 provider 调用。

Schema：

- `common.schema.json`：共享 ID、DesignSystem、resolved style、diagnostic 与 `960 × 540 pt` canvas。
- `slide-ir.schema.json`：content-only SlideIR。
- `deck.schema.json`：DeckSpec、Storyline、OutlinePlan、migration trace 与 DeckRevision。
- `layout-result.schema.json`：独立布局结果。
- `render-input.schema.json`：renderer port 输入。

Golden 1：

- `fixtures/golden-1-professional-business.legacy.json`
- `fixtures/golden-1-professional-business.slide-ir.json`
- `fixtures/golden-1-professional-business.layout-result.json`

验证：

```powershell
npm.cmd run test:ppt-v2
npm.cmd run render:ppt-v2:golden
officecli validate artifacts/ppt-v2/golden-1-professional-business.pptx
officecli view artifacts/ppt-v2/golden-1-professional-business.pptx issues
```

Golden 2～5、动态页数、图片、chart、table 与 diagram 不属于 Phase 1。后续能力必须在新合同版本中显式增加，不能把字段塞回 renderer 或把 geometry 放回 SlideIR。
