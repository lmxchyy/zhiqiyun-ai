# 视频参数优先级与 Canonical Request 架构审计（Issue #170）

> 状态：Phase 1 Architecture Audit + Target Design
>
> 本文只记录现状、目标设计、迁移切片与回归边界，不在本阶段实施大范围代码改动。
>
> 审计基线：`feat/video-canonical-request`，基于 `dbffda382`（PR #169 merge commit）。

## 1. 设计结论

产品原则固定为：

> **结构化参数决定怎么生成，Prompt 决定生成什么。**

因此：

1. `model`、`input_mode`、`duration`、`aspect_ratio`、`resolution`、音频开关、参考素材等结构化字段是执行真相。
2. Prompt 中出现的时长、比例、清晰度、参考图等内容只解析为 `PromptIntentHints`，不得静默覆盖结构化字段。
3. 冲突默认只产生 soft warning；用户可以继续按当前设置生成，也可以显式应用可用提示词参数或返回修改 Prompt。
4. Canonical Request 是后续计费、任务快照、Provider payload、指纹、遥测和历史展示的共同语义源；Provider-specific payload 仍由适配器生成，不把平台 SDK 类型泄漏到核心业务。
5. Prompt 全文不进入 telemetry。仅允许记录 `prompt_hash`、长度和 warning codes；金融/合规语义不作为 Prompt 参数冲突拦截条件。

## 2. CURRENT PARAMETER FLOW

### 2.1 入口与参数构造

| 层 | 现状 | 结构化参数来源 | Prompt 是否作为执行输入 |
|---|---|---|---|
| Web 用户页 | `admin-vue/src/App.vue` 的视频工作台 | `videoDuration`、`videoRatio`、`videoResolution`、模式、模型、音频开关、上传文件 | 是，原样作为 `prompt`；不覆盖结构化字段 |
| Web 提交 | `POST /generation-tasks` | `params.duration`、`params.ratio`、`params.resolution`、`inputMode`、帧图/参考图 URL | 是，后端继续传给 Provider |
| 小程序 AI Creation | `apps/user-uni/src/pages/AiCreationPage.vue` | `duration`、`size`、`quality`、`videoMode`、首尾帧、`parameters` | 是，Prompt 仅是视频内容描述 |
| 小程序角色工作台 | `MiniProgramRoleWorkbench.vue` + `buildVideoSubmissionParameters` | 根据 schema/capabilities 重新构造合法字段，统一 `ratio -> aspect_ratio` | 是，作为 `CreateRequest.Prompt` |
| Connector | `connector_generation.go` / `connector_capabilities.go` | Connector 命令参数，经企业授权和后端能力解析 | 是；Connector 生成的 Prompt 仍进入同一业务服务 |
| 重试/恢复 | `asset_center_api.go`、`generation_recovery_api.go`、worker | 从任务快照重建请求，并附加本地 retry/recovery 元数据 | 是，但不应成为新的参数来源 |

当前存在的第一个分叉是：Web 直接组装 `params`，小程序 SDK/组件根据 draft、schema 和 capabilities 再组装一次；Connector 也有自己的请求构造。它们目前通过后端准备函数汇合，但汇合前的 alias、默认值和参考素材形态不完全一致。

### 2.2 后端准备与校验顺序

主入口 `createGenerationTask`（`backend-go/internal/httpserver/api.go`）当前顺序为：

1. 解码 `generation.CreateRequest`，读取 Idempotency-Key，trim Prompt。
2. 注入请求 terminal，并执行法律确认、内容安全检查。
3. `prepareGenerationRequest`：
   - 推导/规范化 `module_code` 与 task type；
   - 缺省模型时选择模块默认模型；
   - `normalizeRequestParamAliases`：`ratio -> aspect_ratio`、`generateAudio -> generate_audio`；
   - 解析模型、schema、billing rule 和 capability；
   - 对视频执行 `validateVideoGenerationRequest`；
   - 调用 `inspectVideoPromptPreflight`，只生成 warning codes；
   - 写入 `prompt_hash`、`prompt_length`、schema/limit/租户/计费上下文快照；
   - 删除不应继续流转的 legacy metadata，过滤不支持的可选参数。
4. `generationQuoteForRequest` 根据 `req.Params` 计算权威报价；缺失 pricing fail closed。
5. 选择服务/Provider route，视频任务进入 pending task + 异步 worker。

这条链路已经保证 Prompt 不会直接覆盖 UI 参数，但“归一化参数”“能力裁剪后的参数”“计费参数”和“Provider 参数”仍以 `map[string]any` 的不同投影存在。

### 2.3 视频校验与 Prompt 解析现状

`video_generation_validation.go` 负责：

- 文生/图生模式和模型能力校验；
- 首帧、尾帧、参考图数量及 legacy 图片字段归一化；
- `duration`、`resolution`、`ratio/aspect_ratio` 的能力选项校验；
- 删除当前 Provider 不支持的 optional parameter。

`video_prompt_preflight.go` 负责：

- 识别明确总时长表达；
- 排除 `0-8s`、`8-18s`、`18-30s` 等时间轴范围，避免把分镜区间当作总时长；
- 识别参考图指令、模式冲突和复杂 Prompt；
- 只返回 warning codes，不改变 `req.Params`。

目前没有通用的 Prompt Intent 对象，也没有独立的“一致性结果”对象；Prompt 解析结果只在 preflight 内部以临时结果存在。

### 2.4 Billing、Task、Provider、Telemetry、History

**Billing**

- `generationQuoteForRequest` 通过 `billingParamsForRequest(req.Model, req.Params, ...)` 读取结构化参数。
- 视频时长会影响按秒报价；报价后由 task store 写入 pricing snapshot，并在个人积分账户中 reserve。
- `CreatePendingGenerationTask` 再次从 request 计算 quote，随后把计费/预留字段写入 `Params`。
- 因而 billing 的业务事实是结构化参数，但还没有一个显式的 Canonical Request 字段作为唯一输入。

**Task**

- `generationTask` 保存 `Prompt`、`Model`、`Type`、`Params`、点数、schema/limit snapshot、billing 状态和结果。
- Postgres 对外投影会排除参考图、帧图和 billing 内部字段，但内部请求和 task Params 仍承载多种 metadata。
- Task 是 Provider 异步执行和历史读取的主要 durable snapshot。

**Provider**

- `internal/provider/video/openai_compatible.go` 及 Seedance bridge 从 `req.Params` 读取 duration、ratio、resolution、audio 和参考图，并按 endpoint 重新映射为 `size`、`quality`、`seconds`、`ratio` 等 Provider 字段。
- 当前 provider adapter 会再次处理 alias/default 读取，如 `aspect_ratio/ratio`、`generate_audio/generateAudio`。
- Provider 收到的是 `generation.CreateRequest`，不是明确的稳定视频契约。

**Fingerprint / Recovery**

- 现有 `videoRequestFingerprint` 已有稳定性保护：只排除确认不属于 Provider 语义的本地、路由、计费和结果 metadata；未知 future key 默认参与 fingerprint。
- 这是重要的安全基线，但它仍然是对 mutable `Params` 的投影，不等同于 Canonical Request。
- legacy execution 有兼容分支，迁移不可直接替换为新的 hash 规则。

**Telemetry**

- `videoPromptPreflightTelemetry` 记录 task、model、duration、aspect ratio、input mode、`prompt_hash`、Prompt 长度和 warning codes。
- 当前没有统一记录 canonical version、canonical hash 或一致性字段；不能把完整 Prompt 加入 telemetry。

**History**

- Web history 从本地 snapshot 和 `taskToVideoHistoryEntry` 读取真实 task 状态、model、duration、ratio、resolution、输入素材和 billing failure 文案。
- History 的参数字段来自 task `Params` 与 task columns 的组合，不保证与 Provider adapter 读取路径完全同源。

## 3. Prompt 参数是否会覆盖结构化参数

以当前实现和已完成 #169 回归为准：**不会自动覆盖**。

- Prompt 时长只产生 mismatch warning；执行仍使用当前 `params.duration`。
- Prompt 中的比例、清晰度目前没有执行覆盖逻辑；未来 parser 也必须只生成 hint。
- Prompt 中的参考图要求不会自动制造上传素材；文生模式携带真实图片仍由已有后端校验决定是否拒绝。
- `normalizeRequestParamAliases` 只归一化请求字段 alias，不读取 Prompt。

应避免未来在 Provider adapter、billing 或 UI submit handler 中加入“发现 Prompt 参数后直接写回 params”的快捷实现。

## 4. TARGET PARAMETER FLOW

```text
UI / Mini Program / Connector / Retry
        │
        ▼
Structured Video Input
(type, model, mode, duration, ratio, resolution, audio,
 reference asset IDs, first/last frame, optional controls)
        │
        ├──────────────► Prompt (content source: what to generate)
        │                         │
        │                         ▼
        │                 PromptIntentParser
        │                         │
        │                         ▼
        │                 PromptIntentHints
        │
        ▼
Normalize + Resolve Defaults + Capability Validation
        │
        ▼
ConsistencyChecker(hints, structured parameters)
        │
        ├── warning codes / evidence / suggested actions
        │
        ▼
CanonicalVideoRequest (single semantic snapshot)
        │
        ├── Quote / Billing snapshot
        ├── Task + idempotency snapshot
        ├── Provider adapter payload
        ├── Fingerprint / recovery identity
        ├── Privacy-safe telemetry
        └── History / workbench display
```

### 4.1 `PromptIntentHints`

建议为纯数据对象，不承担执行行为：

```text
PromptIntentHints {
  parser_version
  requested_duration_seconds?: number
  requested_aspect_ratio?: string
  requested_resolution?: string
  requested_input_mode?: TEXT_TO_VIDEO | IMAGE_TO_VIDEO | VIDEO_TO_VIDEO
  reference_image_requested: boolean
  requested_reference_count?: number
  audio_requested?: boolean
  evidence: [{ field, normalized_value, source_kind, confidence }]
}
```

约束：

- 只保存归一化 hint 和最小证据，不保存 Prompt 原文片段到 telemetry。
- 时间轴区间不是 `requested_duration_seconds`；只有明确总时长表达才进入该字段。
- 低置信度、歧义和金融/合规语义不应导致 hard reject。
- parser 可先使用确定性规则，未来可以替换/增强实现，但输出契约不变。

### 4.2 `ConsistencyResult`

```text
ConsistencyResult {
  status: ok | warning
  warnings: [{
    code,
    field,
    structured_value,
    hinted_value,
    severity: warning,
    message,
    actions: [use_structured, apply_hint, edit_prompt]
  }]
  ignored_hints: [{ field, reason }]
}
```

`ConsistencyResult` 不改变 canonical structured values。它是 UI 交互和 telemetry 的解释层。

### 4.3 `CanonicalVideoRequest`

建议以版本化、可序列化的语义对象作为跨层契约：

```text
CanonicalVideoRequest {
  schema_version
  request_identity {
    client_request_id
    task_id?                 // task 创建后补入，不参与首次用户意图计算
  }
  content {
    prompt
    prompt_hash
    prompt_length
  }
  execution {
    model
    input_mode
    duration_seconds
    aspect_ratio
    resolution
    fps?
    generate_audio?
    motion_strength?
    camera_movement?
    references {
      first_frame_asset_id?
      last_frame_asset_id?
      image_asset_ids[]
    }
  }
  capability_snapshot
  intent_hints
  consistency
  routing_context?           // server-owned; not Prompt-owned
  billing_context?           // server-owned quote/rule snapshot
}
```

边界：

- `execution` 是结构化参数在能力解析、alias 归一化和默认值应用后的结果，Provider 不再从任意 raw params 猜测核心字段。
- Provider adapter 从 canonical `execution` 生成各自的 `size`、`seconds`、`ratio` 等 payload；Provider-specific mapping 不上浮到业务层。
- 参考素材优先使用受控 asset/storage identity；不能把未经授权的 Prompt 文本或外部 URL 当作引用依据。
- `routing_context`、billing rule 和授权信息由服务端补全；Prompt 不能选择租户、通道或价格。
- `intent_hints`、warning 和 parser version 便于诊断，但不得反向覆盖 `execution`。

## 5. 字段归属与 source-of-truth 矩阵

| 字段 | 用户/UI输入 | Prompt hint | 后端权威化 | Canonical execution | Billing | Provider | History |
|---|---|---|---|---|---|---|---|
| `prompt` | 是 | 否 | trim + 安全检查 | content.prompt | 否 | 是 | 展示/回放 |
| `model` | 选择建议 | 否 | 是，按授权/能力解析 | 是 | 选择 pricing rule | 是 | 是 |
| `input_mode` | 是 | 可提示冲突 | 是，按 task type/能力校验 | 是 | 间接 | 是 | 是 |
| `duration` | 是 | hint only | 是，能力选项校验 | 是 | **按 canonical 值计量** | 是 | 是 |
| `aspect_ratio` | 是 | hint only | alias 归一化 + 能力校验 | 是 | 如规则需要 | 是 | 是 |
| `resolution` | 是 | hint only | 能力校验 | 是 | 如规则需要 | 是 | 是 |
| `generate_audio` 等 optional | 是 | 可作弱 hint（不默认解析） | 不支持则按现有规则裁剪 | 是 | 仅规则声明时使用 | 是 | 可选展示 |
| 首/尾帧、参考素材 | 上传/asset 选择 | 只能提示需要素材 | 是，数量/能力/授权校验 | asset identity | 通常不直接计价 | 是 | 是 |
| `provider/channel` | 否 | 否 | server route | routing context | 否 | adapter route | 可展示实际通道 |
| quote/rule/version | 否 | 否 | server billing | billing snapshot | **唯一计费事实** | 否 | 可展示点数 |
| warning codes | UI反馈 | 产生原因 | 后端/共享 parser | consistency | 否 | 否 | 可选诊断 |
| fingerprint | 否 | 否 | 从 canonical provider semantics 计算 | canonical-derived | 否 | recovery identity | 否 |
| task/billing status | 否 | 否 | task store/state machine | 否（引用） | state machine 真相 | 否 | **真实状态** |

## 6. 冲突 UX 提案

仅在存在 warning 时显示冲突面板；默认动作不改变现有生成能力：

1. **按当前设置生成**（默认、主按钮）
   - 明确文案：“将按当前的 5 秒 / 16:9 / 720p 生成，提示词中的 10 秒仅作为意图提示。”
   - acknowledgement 记录 warning codes，不修改 canonical execution。
2. **应用提示词参数**
   - 只对当前模型 capability 支持且可安全转换的字段生效。
   - 先回填 UI draft，再重新执行 capability 校验、估价/点数确认和一致性检查；不能在提交请求中静默改值。
   - 参考图 hint 只能引导“上传/切换图生视频”，不能凭空生成或读取未授权素材。
3. **修改提示词**
   - 关闭生成面板并聚焦 Prompt，给出可选建议；不修改结构化字段。

补充规则：

- 无冲突时不增加确认步骤。
- 复杂 Prompt、金融/合规语义、低置信度解析保持 soft warning 或不提示，不 hard reject。
- 既有能力校验（例如图生必须首帧、模型不支持尾帧、非法 duration）仍是独立的结构化参数校验，不被“soft warning”原则削弱。

## 7. 迁移与兼容性风险

| 风险 | 影响 | 迁移策略 |
|---|---|---|
| 旧任务只有 `Params`，没有 canonical snapshot | 恢复/历史无法直接读取新对象 | 只读 fallback：按 task type/model/params 重建 legacy canonical，并标记 `legacy_reconstructed` |
| `ratio`、`aspect_ratio`、camelCase 并存 | quote/provider/fingerprint 漂移 | builder 内单次归一化；保留 legacy adapter 读兼容，canonical 只输出 snake_case |
| store 会加入 billing/pricing/route metadata | fingerprint 误差 | 新 canonical fingerprint 只取 provider-semantic execution；保留现有 legacy fingerprint 兼容路径 |
| Provider 各 endpoint 字段不同 | payload 回归 | adapter golden tests：canonical 输入与旧 payload 逐模型对比，先 shadow 再切换 |
| optional 参数被 capability 裁剪 | UI 看见的 draft 与实际发送不同 | canonical 在 capability resolve 之后生成，并保存 capability snapshot/ignored fields |
| 参考图 URL/内部素材泄漏 | 隐私与日志风险 | canonical 对外/telemetry 用 asset IDs；Provider 适配器在受控边界解析短期 URL |
| quote 与 task 二次计算 | 计费不一致 | quote 接受 canonical billing view；task 只持久化 quote snapshot，不重新解释 Prompt |
| Connector / retry 另有入口 | 绕过 canonical builder | 所有视频入口统一调用 builder；retry 只能从 durable canonical/legacy fallback 恢复 |
| 现有 provider execution fingerprint | 影响进行中任务恢复 | 版本化 fingerprint；旧任务继续 legacy 校验，新任务使用 canonical-derived fingerprint，不能 replay 生产遗留任务做迁移验证 |
| `task.Params` 对外兼容 | 客户端依赖旧字段 | 过渡期双写 canonical projection + legacy params；先观测再删除旧字段 |

## 8. 实现切片（后续 PR，不在本阶段实施）

### Slice 0 — 契约和纯函数

- 定义 `PromptIntentHints`、`ConsistencyResult`、`CanonicalVideoRequest`、版本号。
- 将 Go/TS 的 duration timeline 排除、alias 归一化和冲突码纳入表驱动测试。
- 不改变 Provider、billing、task 行为。

### Slice 1 — Backend shadow builder

- 在 `prepareGenerationRequest` 完成现有校验后构建 canonical shadow。
- 与旧 `req.Params` provider projection、billing params、fingerprint projection 做 diff telemetry。
- diff 只告警，不影响任务执行。

### Slice 2 — Task/Billing snapshot

- task 创建时保存 canonical version/hash 和结构化 projection。
- billing 仍使用现有 state machine，但 quote 的输入切换为 canonical billing view。
- 保留旧字段和 legacy fallback，验证 reserve/capture/release 不变。

### Slice 3 — Provider adapter

- provider adapter 改为读取 canonical execution；Provider-specific payload 仍在 adapter 内生成。
- 用现有 grok、Seedance、通用 OpenAI-compatible endpoint 的 payload golden tests 做无差异验证。
- 不改变 channel、retry、provider error classification。

### Slice 4 — Frontend/SDK conflict UX

- Business SDK 返回 preflight/consistency 结果。
- Web 和小程序复用三动作 UX；应用 hint 后必须重新校验和估价。
- 不直接在页面散写 `uni.request`；小程序继续使用项目 API Client。

### Slice 5 — History/telemetry cleanup

- history 由 canonical projection + task 状态组成，失败/计费文案继续读取真实 billing state。
- telemetry 增加 canonical version/hash、warning code 和 final state 关联，不增加 Prompt 明文。
- 先自然流量观察，不主动烧积分。

## 9. 回归计划与验收门槛

### 纯函数与契约

- `0-8s / 8-18s / 18-30s` 不产生 duration mismatch。
- “生成 30 秒视频”在结构化 10 秒时产生 warning，但执行值仍是 10。
- 结构化参数与 Prompt 冲突时 canonical execution 不变。
- `ratio`/camelCase 输入归一化结果稳定且幂等。
- 时间轴、复杂 Prompt、参考图 hint 均不绕过现有硬能力校验。

### Billing/Task

- 同一 canonical execution 的 quote、reservation、capture、release 结果不变。
- Provider failure 仍正确释放积分；不得自动重试或重复扣费。
- idempotency replay 返回同一 task；retry/recovery 不因本地 metadata 产生错误 fingerprint drift。
- `RELEASED`、`RESERVED`、`CAPTURED` 等历史卡片继续按真实状态展示。

### Provider/Recovery

- grok 仍正确映射 duration/size/quality/aspect ratio；Seedance 仍正确映射 seconds/ratio/resolution/audio。
- 进行中 legacy execution 不被新 canonical rollout 误判为可重提交。
- Provider payload、channel、retry 和错误分类无行为变化。

### 前端/小程序/Connector

- Web、小程序、Connector 的同一组结构化输入生成同等 canonical execution。
- “按当前设置生成”“应用提示词参数”“修改提示词”三个动作均为显式行为。
- 小程序继续通过 API Client；不得引入页面直写 `uni.request` 或 Axios。
- 访客登录、首屏/视频入口、作品列表和自由 P 图等 protected surfaces 无回归。

### 观测指标

仅记录：`canonical_version`、`canonical_hash`（如采用）、Prompt hash、Prompt length、warning codes、task final state、billing final state、provider failure classification。禁止记录完整 Prompt、未脱敏参考 URL 或支付密钥。

## 10. Phase 1 结论与下一步

- 当前实现已经满足“Prompt 不覆盖结构化参数”的安全底线；#169 的 timeline/preflight 逻辑应作为新契约的回归基线。
- 主要架构缺口不是模型能力或 Provider payload，而是同一组视频参数在 Web、小程序、Connector、billing、task、Provider 和 history 之间以多个 `map[string]any` 投影重复构造。
- 推荐先做 Slice 0 + Slice 1 的纯函数/影子对比，经过设计确认后再切换任何真实执行路径。
- 本 Issue 后续不应混入 #159 磁盘治理、`plan_dead/render_dead` 运维调查或生产 replay/delete。
