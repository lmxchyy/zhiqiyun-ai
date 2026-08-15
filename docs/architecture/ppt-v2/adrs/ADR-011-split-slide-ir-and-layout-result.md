# ADR-011：拆分 content-only SlideIR 与 LayoutResult

- 状态：Accepted
- 日期：2026-08-15
- 取代/修订：ADR-001 的 combined Deck IR、ADR-002 的 exact `2.0`、ADR-008 的 element-array z-order、ADR-009 的 Phase 0 Golden 集合

## Contract Gap

Phase 0 `2.0` 把 `box` 和显式 style 直接放在 renderer-facing element 上。这可以验证 PptxGenJS spike，但无法落实 Phase 1 冻结的双权威边界：SlideIR 必须独占内容，LayoutResult 必须独占 geometry。继续沿用 `2.0` 会迫使 renderer 或 adapter 把内容与布局重新合并成第二套临时 model。

## 决策

合同显式升级为 `2.1`，不保留 `2.0` compatibility renderer、fallback 或双写：

1. `DeckRevision / SlideIR` 保存文本、bullet、speaker notes、元素类型、slot、style role 与元素顺序；禁止 `box`、resolved style 和 PptxGenJS 坐标。
2. `LayoutResult` 保存 `elementId`、point geometry、`zIndex`、`resolvedStyle` 与 diagnostics；禁止标题、正文、bullet 或 notes。
3. `RenderInput` 明确携带 deck revision、SlideIR、LayoutResult、DesignSystem、Asset Manifest 与 options。
4. Renderer 只通过 `elementId` 连接两侧，按 `zIndex` 输出；缺失映射、非法 geometry 或 error diagnostic 直接拒绝。
5. 画布与几何继续遵循 ADR-003：`960 × 540 pt`；仅 PptxGenJS adapter 在边界换算 inch。
6. PptxGenJS adapter 规范化 OOXML 时间戳和 ZIP entry date；同一 RenderInput 必须 byte-for-byte 可重复。
7. Phase 1 发布门只执行固定两页 Professional Business Golden 1；Golden 2～5 留给后续显式阶段。

## 后果

- 同一 SlideIR 可被独立布局，renderer 无权修改内容或 geometry。
- Phase 1 只实现 Cover 与 Standard Content 两个 layout，以及 text/shape/notes 子集。
- Phase 0 spike 与 `2.0` fixtures 被正式 package 和 Golden 1 取代，不再参与测试。
- 新 element 类型必须通过后续显式合同版本加入；不得在 `2.1` renderer 中静默降级。
