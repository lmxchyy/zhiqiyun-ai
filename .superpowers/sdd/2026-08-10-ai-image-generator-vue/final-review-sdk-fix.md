# Final review SDK fix

基线：`6f483b6e4e18a6af4a9f5dd7da80cc6b97a1851f`

## 结论

Business SDK 的图片最终请求已收敛为 canonical 契约，不再由 SDK 猜测 UI 画幅、清晰度或 schema 默认值：

- `CreateDraft.size / quality / count` 改为可选，调用方只传当前模型 exact schema 实际暴露的字段。
- 图片 mapper 仅发送调用方明确提供的 canonical 值：`size` 必须是正整数 `WIDTHxHEIGHT`，`quality` 仅接受 `standard/high`，`count` 必须是正整数并映射为 `n`。具体尺寸枚举和数量上限继续由后端 exact schema 校验。
- 图片请求不再补 `1024x1024 / standard / 1` 默认值；mock 或 CloudBase schema 未声明的 `quality/n` 不会被 SDK 重新添加。
- `ratio / aspect_ratio / imageRatio / imageQuality` 等废弃图片 alias 不进入最终 params；`auto / 4:3 / 1K / 2K` 若被误当作 canonical 顶层值会 fail-fast，不会静默回落。
- 自由 P 图仍保留 canonical 参数、参考图 payload、seed、自定义 schema 参数和来源追踪字段；视频参数过滤、视频 fallback、PPT/其他非图片 fallback 均保持原行为。
- `clientRequestId` 在最终 request body 中原样保留；未改 API Client 或既有 `Idempotency-Key` header fallback 语义。

## TDD 证据

### RED

先只新增真实 `taskRequestFromDraft` 深比较测试，执行：

```text
node --test tests/business-sdk-mappers.test.mjs
```

结果：`18 pass / 4 fail`。四项失败分别准确复现：

1. canonical `{size:"1536x1024", quality:"high", n:2}` 旁仍泄漏 `aspect_ratio:"4:3"`。
2. exact schema 未提供字段时仍被补成 `size:"1024x1024", quality:"standard", n:1`。
3. `auto / 4:3 / 1K / 2K / 非法数量` 未在 mapper 边界失败。
4. 带参考图的自由 P 图请求仍泄漏旧 `aspect_ratio`。

### GREEN

最小实现后，同一测试为 `22 pass / 0 fail`。新增覆盖包括：

- 完整最终 body 和 `clientRequestId` 深比较。
- CloudBase 只声明 `size` 时不补 `quality/n`。
- 废弃 alias 清理和非法 UI 值 fail-fast。
- 自由 P 图 reference payload、custom schema、provenance 与 canonical 参数保持。
- 非图片 mapper 默认行为保护。

## 最终验证

- `npm.cmd run typecheck:packages`：PASS。
- `node --test tests/business-sdk-mappers.test.mjs tests/user-mini-video-model-fallback.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs`：PASS，34/34。
- `node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs`：PASS，15/15。
- `node --test tests/user-mini-free-image-edit.test.mjs`：PASS，11/11。

## Protected surfaces 核对

- [ ] W1 侧边栏可用/总额（未触及）
- [ ] W2 平台首页首屏（未触及）
- [ ] W3 生图等工作台首屏（未触及页面与列表 limit）
- [ ] W4 首页摘要（未触及）
- [ ] M1 首页模板/灵感/登录落用户首页/游客浏览入口（未触及）
- [x] M2 视频模型/参数/预估积分（指定 Node 回归 15/15；视频 mapper 行为保持）
- [ ] M3 作品列表（未触及）
- [x] M4 灵感草稿带入（SDK 不再把图片 ratio alias 改写为最终 `aspect_ratio`；最终 UI 恢复/映射由 Frontend Task C 验证）
- [ ] M5 钱包积分展示（未触及）
- [x] M6 自由P图全页与入口文案（11/11；SDK reference/custom 参数行为测试通过）
- [ ] P1 发布门禁（未发版）
- [x] P2 相关回归测试（SDK、M2、M6 和 packages 类型检查通过）

说明：本次实际改动范围仅为 Business SDK mapper、`CreateDraft` 准确类型、SDK Node 行为测试和本报告；未修改 backend-go、apps/user-uni、构建脚本或既有未跟踪 junction。
