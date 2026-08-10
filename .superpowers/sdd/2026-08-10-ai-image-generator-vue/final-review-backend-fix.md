# Final review backend fix

基线：`076319c785b93996f13593cb5ea9580e56f59ed8`

## 结论

后端图片契约已按真实 provider 能力收紧，不保留旧 alias 或静默尺寸回落：

- OpenAI-compatible 只接受 canonical `params.size`：`1024x1024`、`1024x1536`、`1536x1024`；`standard` 表示 provider 默认质量，`high` 原样发送，其他 quality 在发出 HTTP 请求前失败；数量只读 canonical `n`。
- CloudBase 两个官方模型只接受 `1024x1024`、`1280x1280`、`1280x720`、`720x1280`；不再读取 `aspect_ratio`，非法/缺失 size 在发出 HTTP 请求前失败；请求体不发送 `quality` 或 `n`。
- `mock-standard`、`gpt-image-2` 和两个 CloudBase 模型使用各自独立的图片 schema。mock 固定 `1920x1080` 且无 quality；GPT 使用三个真实尺寸及 `standard/high`；CloudBase 使用四个真实尺寸且无 quality/n。
- 内置图片 schema 是 provider 契约的权威定义；已有数据中的旧内置图片 schema 会被直接替换，不保留过时选项。视频/PPT schema 仍沿用原有缺失字段合并逻辑。
- 图片 module-schema 不再切换 fallback model，也不借用其他模型或通用 schema；响应继续返回精确 `model_name`。`/models` 不再伪造 `supportedRatios`，无专属图片 schema 的动态模型不再暴露。
- 正式图片灵感 seed 统一为 `ratio/quality/count`；纵向 `3:4` 改为 GPT 真实 `2:3`，横向 `16:9` 改为 `3:2`。
- PPT 配图的 `mock-standard` 请求改为 canonical `1920x1080`，不发送 quality；同时移除该调用方的 `imageRatio/count` alias。

## 代码证据与架构核对

架构报告的 provider 支持矩阵与实际实现一致。实现前额外发现两处报告未展开但会阻止契约落地的问题：

1. `mergeDefaultAIParameterSchemaFields` 只补字段、不替换旧字段，存量 `mock-standard` 会继续保留 GPT 尺寸和 quality。
2. PPT 配图在选择 `mock-standard` 时仍硬编码 `1536x1024 + standard`，会被精确 mock schema 正确拒绝。

这两处均以行为测试先复现，再做最小修复；未扩展 estimate、幂等数据库、视频参数或计费实现。

## TDD 证据

### RED

- provider 定向测试：
  - OpenAI 最终 body 实际得到 `size=1024x1536,n=1`，而 canonical 输入为 `1536x1024,n=2`。
  - 缺失 canonical size、非法 size、非法 quality 均错误地请求成功。
  - CloudBase 两模型把 `size=16:9` 静默转换并请求成功。
- schema/models/seed 定向测试：
  - mock 错宣称 GPT 三尺寸；GPT/CloudBase 无精确 schema。
  - 未配置 schema 的图片模型被 fallback 成 mock。
  - `/models` 对所有模型伪造 `supportedRatios`。
  - 图片 seed 缺 `count`，纵横比仍为 `3:4/16:9`。
- 存量 schema 测试：旧 mock 的三尺寸在归一化后仍被保留。
- `TestPPTImageGenerationCreatesImageUsageEvent`：mock 请求的 `1536x1024` 被新精确 schema 拒绝。

### GREEN

- `go test ./internal/provider/image -count=1`：PASS。
- 图片 provider/schema/models/seed 定向测试：PASS。
- `TestPPTImageGenerationCreatesImageUsageEvent`：PASS。
- M2 保护回归：
  - `go test ./internal/httpserver/ -run 'TestVideoGenerationEstimate|TestBillingCenterV1Acceptance|TestNormalizeAICapabilityDefaultsMergesMissingBillingRules' -count=1`：PASS。

## 完整回归说明

已实际执行要求的：

```text
go test ./internal/provider/... ./internal/httpserver/...
```

所有 provider 包（含 image/video）通过；`internal/httpserver` 仍有与本次 diff 无关的既有/环境失败：

- 8 个 PostgreSQL 测试无法连接现有 `127.0.0.1:55441` 测试库。
- `TestMiniProgramRejectsGatewayAsFilingSubjectAndClosedCreationMode` 单独运行亦失败；本次未修改其实现。
- `TestUserGenerationAssetPointsAdminLoop` 与两个 recharge commission 测试读取到既有 seed 状态，断言 usage/commission 失败；本次未修改对应实现。

本次新增/受影响的图片、PPT、视频 estimate 与计费定向测试均通过。

## Protected surfaces 核对

- [ ] W1 侧边栏可用/总额（未触及）
- [ ] W2 平台首页首屏（未触及）
- [ ] W3 生图等工作台首屏（未触及前端/列表 limit）
- [ ] W4 首页摘要（未触及）
- [ ] M1 首页模板/登录/游客入口（未触及）
- [x] M2 视频模型/参数/预估积分（指定 Go 回归通过，Seedance 默认规则未改）
- [ ] M3 作品列表（未触及）
- [x] M4 灵感草稿带入（正式图片 seed 契约测试通过）
- [ ] M5 钱包积分展示（未触及）
- [ ] M6 自由P图全页与入口文案（未触及）
- [ ] P1 发布门禁（未发版）
- [x] P2 相关回归测试（图片/PPT/M2 定向通过；完整 httpserver 的既有失败如上）
