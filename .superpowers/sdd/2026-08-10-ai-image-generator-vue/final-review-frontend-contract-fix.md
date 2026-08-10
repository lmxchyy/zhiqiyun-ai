# Final review frontend contract fix

基线：`3dc3fd6ae30d155acbc0f642561a3cc0376a4b50`

## 结论

Frontend Task C1 已把图片生成的 schema、选择、灵感恢复、最终 SDK 请求和幂等 key 收敛为纯函数契约。本提交不接入 `AiImageGenerator.vue` 或 `MiniProgramRoleWorkbench.vue`，也未修改后端、SDK、包配置、构建脚本或既有 junction。

- `/module-schema` 只接受真实响应形状：顶层 `module_code=image_generation`、完全相等的 `model_name`、嵌套 `schema.fields`。缺响应、错模块、模型不相等、或声明字段没有合法选项时，返回明确中文不可用原因；不借模型、不读 camelCase、不补客户端默认值。
- `size.options` 只保留正整数 `WIDTHxHEIGHT`，并使用最大公约数生成比例标签；覆盖 `1024x1024 → 1:1`、`1536x1024 → 3:2`、`1024x1536 → 2:3`、`1280x720 → 16:9`。
- `quality.options` 只保留 `standard/high`。`n` 只接受 schema enum 中符合正整数及 min/max 的值；schema 没有 enum 时，只能使用合法 default，绝不恢复旧 `[1,2,4]`。
- 选择转换只输出 schema 已声明、用户实际选择的 `{size?, quality?, count?}`。不支持值、未声明字段、缺失 required 字段和 alias 字段均 fail-fast。
- 正式灵感恢复只读取 `{ratio, quality, count}`。`ratio` 必须完全等于某个 schema size 的约分标签；`standard/high` 与正整数 count 必须也由当前 schema 支持。不兼容时只返回中文原因，不返回可提交 canonical 数据。
- 真实调用编译后的 `packages/business-sdk/dist/mappers.js#taskRequestFromDraft`，深比较最终 request，证明 `size/quality/count` 到达为 `size/quality/n`，且 `ratio/aspectRatio/aspect_ratio/imageRatio/imageQuality` 均未出现。
- 图片结算文案固定为“以生成时结算为准”，不再在客户端计算 `pointCost * count`。
- 最小幂等 helper 以固定字段顺序生成 canonical fingerprint。首次、输入变化、终态失败 retry 创建新的 `image_<uuid>`；仅网络结果不确定且 fingerprint 完全相同时复用 existing key。UUID factory 由调用方注入，helper 不处理账单。
- 图片模型列表继续严格要求 `online === true` 和图片能力；模型解析改为 exact-or-empty，不再切换到第一项。

## 真实 schema 响应依据

后端 `moduleSchemaResponse` 同时返回顶层 `fields` 和嵌套 `schema.fields`；C1 选择嵌套 exact schema 作为唯一权威输入：

```text
{
  module_code,
  model_name,
  schema: { fields: [{ key, type, required, default?, options?, min?, max? }] },
  fields
}
```

当前内置图片模型中，GPT 的 size 为三个真实尺寸、quality 为 `standard/high`、n 为 `default=1,min=1,max=8`；CloudBase 只有真实 size，不声明 quality/n；mock 为固定 `1920x1080` 并声明 n default。

## TDD 证据

### RED

先只改测试，执行：

```text
node --test tests/user-mini-image-generator.test.mjs
```

结果：`1 pass / 18 fail`。失败准确覆盖：旧模型选择仍 fallback 到第一项；新 schema/canonical/inspiration/fingerprint/key helper 尚未导出；旧积分函数仍返回“预计 40 积分”。

### GREEN

写入最小纯函数实现后，同一命令为 `19 pass / 0 fail`。测试直接执行生产函数和 compiled SDK mapper，没有源码 regex 假集成断言。

## 验证

```text
node --test tests/user-mini-image-generator.test.mjs
# PASS 19/19

node --test tests/inspiration-photo-restoration.test.mjs
# PASS 4/4

node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs
# PASS 15/15

node --test tests/user-mini-free-image-edit.test.mjs
# PASS 11/11

npm.cmd run typecheck  # apps/user-uni
# PASS

git diff --check
# PASS
```

没有运行 `build:packages`：本提交未修改 package 源码，最终 body 测试使用并验证了当前 HEAD 已构建的 Business SDK `dist`。

## C1 中间依赖 / C2 必做

`imageAspectOptions`、`imageQualityOptions`、`imageCountOptions` 及旧宽类型名称当前仅以**空数组/编译类型**保留，使未获授权修改的旧 Vue/Workbench 在 C1 后仍能通过类型检查。它们不含 `auto/4:3/1K/2K/[1,2,4]`，也不提供任何行为 fallback。

这是仅限 C1 的中间编译依赖，不是最终兼容层：**C2 必须让组件和 Workbench 消费 exact contract，并删除这些旧导出和所有旧调用；最终提交不得保留 shim 或 fallback。** 因此 C1 单独提交不可发布，必须与 C2 接线、失败/重试 UI 和真实交互验证一起完成。

## Protected surfaces 核对

- [ ] W1 侧边栏可用/总额（未触及）
- [ ] W2 平台首页首屏（未触及）
- [ ] W3 生图等工作台首屏（C1 未接页面/列表）
- [ ] W4 首页摘要（未触及）
- [ ] M1 首页模板/灵感/登录落用户首页/游客浏览入口（未触及）
- [x] M2 视频模型/参数/预估积分（指定 Node 回归 15/15）
- [ ] M3 作品列表（未触及）
- [x] M4 灵感草稿带入（正式 `{ratio,quality,count}` 的 1:1、2:3、3:2 与不支持 4:3 行为测试通过；现有 restoration 4/4）
- [ ] M5 钱包积分展示（未触及）
- [x] M6 自由P图全页与入口文案（11/11）
- [ ] P1 发布门禁（未发版）
- [x] P2 相关回归测试（focused、M2、M4、M6、typecheck 与 diff check 均通过）

说明：实际代码范围仅为 `apps/user-uni/src/features/generation/imageCreation.ts` 和 `tests/user-mini-image-generator.test.mjs`；另新增本报告。

## Important review 修复：完整 request fingerprint

已处理 `final-review-frontend-contract-review.md` 的唯一 Important：旧 fingerprint 只编码 `model/prompt/referenceImages/size/quality/count`，遗漏最终 body 中的 `negative_prompt`、任意 schema custom parameter 和 source provenance。

修复后的 `imageRequestFingerprint` 直接接受即将提交的完整 canonical create-task request：

- 顶层只移除纯幂等元数据 `clientRequestId`；当前 request 没有其他需要排除的幂等字段。
- 不维护任何图片业务字段白名单。最终 request 新增字段会自动进入 fingerprint。
- 所有对象键递归排序后序列化；对象插入顺序不影响 fingerprint。
- 数组顺序和值原样保留，因此 reference image 顺序或内容变化会生成新 fingerprint。
- `negative_prompt`、任意 custom/seed、`sourceReferenceAssetId` 或 `sourceReferenceTaskId` 任一变化都会生成新 fingerprint；网络结果不确定时因此不会误复用旧 key。

追加 TDD 证据：

```text
RED: node --test tests/user-mini-image-generator.test.mjs
     15 pass / 7 fail
     失败均为旧 helper 拒绝 compiled taskRequestFromDraft 的完整 request。

GREEN: node --test tests/user-mini-image-generator.test.mjs
       22 pass / 0 fail
```

追加验证：

```text
npm.cmd run typecheck  # apps/user-uni
# PASS

node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs
# PASS 15/15

node --test tests/inspiration-photo-restoration.test.mjs tests/user-mini-free-image-edit.test.mjs
# PASS 15/15

git diff --check
# PASS
```
