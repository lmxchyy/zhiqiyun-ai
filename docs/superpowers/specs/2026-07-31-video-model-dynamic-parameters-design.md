# 小程序视频模型动态参数设计

## 目标与范围

本次只改造小程序正式视频创作入口：

`UserVideoCreationPage -> MiniProgramRoleWorkbench`

同时覆盖共享 SDK、后端 Schema 与 Provider 能力交集、Provider 协议适配证明、积分试算、任务快照和模型切换。桌面/Web `AiCreationPage` 不增加动态控件；仅在硬编码 `generate_audio: true` 会触发新校验时做最小兼容修复。

不修改数据库，不触碰小程序分享、灵感、代理、最近作品、Prompt 优化器、视频缩略图、任务恢复或 101 迁移，不部署生产，不上传微信版本。

## 基线

- 工作目录：`E:\code\work\先知AI-video-contract-2.0.36`
- 起始提交：`dfc70c84535c0d8b28c44c17248f08e0408fb37e`
- 专用分支：`codex/video-model-params-2.0.36`
- `dfc70c8` 的直接父提交 `7adc60d7a` 与其后端目录没有差异，保持当前生产后端源码兼容。
- 已上线分享实现存在于 `dbe6e656` 基线并由 `dfc70c8` 继承，本功能不修改分享代码。

## 单一参数契约

视频用户参数白名单：

- `duration`
- `resolution`
- `aspect_ratio`
- `fps`
- `generate_audio`
- `motion_strength`
- `camera_movement`

一个参数只有同时满足以下条件才成为用户可配置参数：

1. 在白名单中。
2. 当前模型 FinalSchema 中存在对应字段。
3. Schema `visible=true`。
4. Schema `userEditable=true`。
5. 当前 Provider `supported_parameters` 明确包含该参数。
6. Provider 适配器的能力函数只声明它确实会写入当前协议请求体的参数。

`ratio` 只作为旧请求输入兼容，进入系统后规范化为 `aspect_ratio`。新表单、新请求和新任务快照均不保存 `ratio`。

当前协议能力：

- 普通 OpenAI-compatible：`duration`、`resolution`、`aspect_ratio`。
- Seedance content：上述三项加 `generate_audio`。
- `fps`、`motion_strength`、`camera_movement`：适配器未真实写入前不展示，后端拒绝。

## SDK 设计

新增纯函数视频表单契约模块，集中负责：

- 解析 module-schema 的字段。
- 生成满足六项交集规则的动态字段列表。
- 将旧 `ratio` 规范化为 `aspect_ratio`。
- 在模型切换时保留共同且合法的值。
- 对非法值使用 Schema 默认值；无合法默认值时使用第一个合法选项。
- 删除新模型不支持的字段。
- 构造提交给后端的最终视频参数。

纯函数不依赖 Vue 或 uni-app，Node 测试直接覆盖模型 A/B 切换、布尔参数、非法值回退和隐藏字段清理。

## 小程序交互

视频创作页加载可用视频模型。选中模型后请求：

`GET /api/v1/module-schema?module_code=video_generation&model_name=<model>`

页面按 SDK 返回的动态字段渲染：

- 有 `options` 的字段：选项按钮。
- boolean/switch 字段：开关。
- number 字段：受 min/max 限制的数字输入。

模型切换采用候选配置事务：

1. 获取候选模型 Schema 和 Provider 能力，不立即修改页面状态。
2. 若当前有参考图而候选模型不支持图生视频，弹出：
   “当前模型不支持参考图，切换后将移除已上传图片，是否继续？”
3. 用户取消时保持原模型、参考图、参数、试算结果，不创建任务。
4. 用户确认后一次性提交候选配置，清理不支持参数和参考图。
5. 保留共同且合法的值；非法值按默认值/首个选项回退。
6. 根据最终模型和最终参数重新试算。

请求序号防止较早的 Schema 或试算响应覆盖较新的模型/参数状态。参数变化采用短防抖，页面不冻结。

参考图只在当前模型支持 `IMAGE_TO_VIDEO` 且当前模式为图生视频时显示。切到纯文生视频模型前必须经过上述确认。

## 积分试算

新增无副作用接口：

`POST /api/v1/generation-tasks/estimate`

请求使用正式 `generation.CreateRequest` 的视频子集：`type`、`model`、规范化后的 `params`。接口：

1. 鉴权并解析当前用户。
2. 走正式 `prepareGenerationRequest`，复用 FinalSchema、Provider 能力和套餐限制校验。
3. 调用正式 `generationPointCostForRequest`。
4. 返回 `model`、`estimatedPoints`、`billingType`、`quantityField`、`quantity` 和简短说明。
5. 不创建任务，不写数据库，不冻结或扣除积分，不调用 Provider。

正式提交仍重新执行相同准备和计费链。测试用同一请求对比试算结果与 `CreatePendingGenerationTask` 的 `PointCost`。

## 后端校验与快照

后端以 `resolved.FinalSchema` 为准，视频白名单字段若不在最终可编辑字段集合中则拒绝，不能静默删除。可编辑集合也要求 `visible=true`、`userEditable=true` 和 Provider 支持。

正式请求经过规范化和校验后才写任务：

- 删除旧 `ratio`。
- 不支持参数无法进入 `req.Params` 的可持久化路径。
- `final_schema_snapshot` 只包含当前模型最终 Schema。
- Provider 收到的仅是已验证参数；协议适配器仍负责 `aspect_ratio -> aspect_ratio/size/ratio` 的协议字段映射。

## Web 兼容

不增加 Web 动态控件、不调整布局。`AiCreationPage` 的无条件 `generate_audio: true` 改为由共享 SDK/当前模型能力过滤，或直接删除该无条件字段。测试保证普通 OpenAI-compatible 的 Web 提交不会因不支持音频而被新后端拒绝。

## 测试与验收

- SDK 单元测试：A→B 共同值保留、不支持字段删除、默认值回退、音频显隐、旧 `ratio` 规范化。
- 小程序静态/行为测试：模型控件、参考图确认、文生/图生显示、试算刷新、防旧响应覆盖。
- 后端单元/HTTP 测试：FinalSchema 交集、隐藏/不可编辑/Provider 不支持参数拒绝、试算无副作用、试算与正式计费一致、任务快照无旧字段。
- Provider 测试：普通 OpenAI-compatible 只写核心字段；Seedance content 写核心字段和音频；三项未支持高级参数在发送前拒绝。
- Web 兼容测试：现有 Web 视频提交不再无条件携带音频。
- 验证：TypeScript 类型检查、定向 Go 测试、Seedance bridge 测试、`git diff --check`、`mp-weixin` 2.0.36 构建。

## 回滚

本功能无数据库迁移。回滚只需回退本分支提交：

- 前端退回固定视频创作表单状态。
- 删除试算路由和处理器。
- 恢复原 FinalSchema 校验行为。

Provider 的既有 `aspect_ratio` 协议映射保持不变，回滚动态参数功能不会破坏 2.0.36 已上线视频契约修复。
