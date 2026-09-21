# Issue #170 Phase 2：Design Review + Implementation Plan Freeze

> Review status: **PASS with required clarifications**
>
> Review scope: `docs/architecture/video-canonical-request-audit.md`
>
> Business code changes: **none**

## 1. Review decision

Target design可以进入 PR-A，但只能以“契约 + 纯 builder + tests”的范围开始；不得在 PR-A 中接入 Provider、修改 Billing 输入、替换 task storage 或改变线上执行行为。

本次审查的核心问题：

> Canonical Request 是否是唯一最终参数真源，而不是新增一份与 `req.Params` 并行的副本？

结论：**目标可以成立，但必须把 Canonical Request 定义为 downstream 的唯一读取对象，并明确 raw request 只存在于 boundary。**

## 2. 单一最终参数真源判定

### 2.1 三种对象必须分层

| 对象 | 生命周期 | 作用 | 是否允许下游执行层读取 |
|---|---|---|---|
| `VideoInputDTO` / legacy `CreateRequest` | HTTP/SDK/Connector boundary | 接收用户输入和兼容旧 alias | 否；只交给 builder |
| `PromptIntentHints` + `ConsistencyResult` | builder 期间及诊断快照 | 表达 Prompt 意图、冲突和建议动作 | 否；不得覆盖 execution |
| `CanonicalVideoRequest` | builder 完成后至 task/provider/history | 表达已经归一化、能力校验后的最终语义 | **是；唯一允许的最终参数来源** |

因此，Canonical Request 不是“再复制一份 Params”。它是 raw input 被消费后的唯一结果；`req.Params` 在过渡期只能作为输入兼容容器，不能继续作为 billing/provider/fingerprint/history 的独立读取入口。

### 2.2 Canonical Request 的最小核心

Canonical Request 只保留跨边界稳定且有业务意义的字段：

```text
CanonicalVideoRequest {
  schema_version
  content: {
    prompt
    prompt_hash
    prompt_length
  }
  execution: {
    model
    input_mode
    duration_seconds
    aspect_ratio
    resolution
    optional_controls
    references: {
      first_frame_asset_id?
      last_frame_asset_id?
      image_asset_ids[]
    }
  }
  intent_hints
  consistency
  capability_snapshot
}
```

以下内容不应被复制进 `execution`：

- billing 状态、reservation/capture/release 状态；
- Provider response、Provider task ID、retry attempt；
- 租户授权、密钥、短期下载 URL；
- Provider-specific `size`、`seconds`、`quality` 等 payload 字段；
- task status、history display status。

这些是各自系统的事实或派生视图，不是视频生成参数真源。

## 3. 七个关键字段的唯一归属

| 字段 | 唯一最终归属 | Prompt 的角色 | 兼容输入 | 明确废弃的读取方式 |
|---|---|---|---|---|
| `model` | `canonical.execution.model`，由后端授权/能力解析最终确定 | 不参与 | UI/model、Connector default | Provider 从 Prompt 或未解析 raw params 猜模型 |
| `duration` | `canonical.execution.duration_seconds` | `requested_duration_seconds` 只作 hint | `duration`、合法 `duration` alias | billing/provider 各自从 raw map 解析 |
| `aspect_ratio` | `canonical.execution.aspect_ratio` | `requested_aspect_ratio` 只作 hint | `aspect_ratio`、legacy `ratio` | 深层继续同时读取 `ratio/aspect_ratio` |
| `resolution` | `canonical.execution.resolution` | `requested_resolution` 只作 hint | structured `resolution`；小程序 `quality` 仅作为输入 alias | Provider 直接把业务 `quality` 当独立真源 |
| `input_mode` | `canonical.execution.input_mode` | 可产生 mode hint/conflict | task type、`inputMode`/`input_mode` | UI、Provider、preflight 各自推断不同 mode |
| `reference_images` | `canonical.execution.references` 的受控 asset identity | 只能表示“需要参考图”，不能制造素材 | first/last frame、legacy image aliases | Prompt 文本或未经授权 URL 作为引用来源 |
| `quality` | **当前视频域不作为独立 canonical 字段**；若输入 `quality`，builder 只将其映射到 `resolution` | 仅可作 resolution hint | 小程序 SDK 的 `quality` | 在 resolution 之外再保存/计费/指纹 `quality` 副本 |
| `prompt_intent` | `canonical.intent_hints`，诊断/UX only | 唯一来源是 Prompt parser | parser versioned output | 任何 hint 回写 execution |

### 关于 `quality`

当前视频代码中 `quality` 主要是 Provider payload 对 `resolution` 的映射（例如 Grok payload 的 `quality: videoResolution(params)`），而不是独立的用户业务维度。PR-A 必须把这一点写成测试：相同 resolution 不得因为 Provider adapter 叫它 quality 而在 Canonical、billing 或 fingerprint 中生成第二个语义字段。

如果未来某个视频模型确实把 `quality` 定义为不同于 `resolution` 的独立能力字段，必须另开设计变更，提升 schema version 后再加入 canonical，不得在 adapter 中偷偷引入。

## 4. 各子系统的最终读取位置

| 子系统 | Phase 2 冻结的最终参数来源 | 不允许的替代来源 |
|---|---|---|
| Billing quote | `canonical.execution` 的 billing view；duration/resolution 等由同一 builder 归一化 | Prompt hint、Provider payload、task 创建后再次解释 raw Params |
| Billing state machine | 现有 reservation/capture/release 状态和 task billing snapshot | Canonical 不能承载或重写 billing 状态 |
| Fingerprint | `canonical.execution` 的 provider-semantic view + 已解析的 provider identity | mutable raw Params、pricing snapshot、retry metadata |
| Task | 持久化 canonical version/hash 和 canonical projection；状态仍由 task store 管理 | 用 task Params 的任意 metadata 推断最终参数 |
| Provider | adapter 只读取 canonical execution，并在 adapter 内做 endpoint mapping | Provider adapter 自己解析 Prompt、ratio alias 或 raw Params |
| Telemetry | canonical version/hash、Prompt hash/length、warning codes、最终状态 | 完整 Prompt、原始 URL、支付/授权信息 |
| History | task 中保存的 canonical projection + 真实 task/billing status | 本地 draft 覆盖服务端结果；Prompt hint 冒充执行参数 |

Provider identity/channel 仍由服务端 route 决定；它可以作为 fingerprint 的独立 scalar，但不是 Prompt 或用户结构化参数可控的字段。

## 5. 需要停止使用的旧参数来源

不是在 PR-A 中删除，而是冻结为迁移目标：

1. Provider 代码直接从 `req.Params` 读取 `duration`、`ratio/aspect_ratio`、`resolution`、`generateAudio/generate_audio`。
2. Billing 使用一套 `billingParamsForRequest(req.Model, req.Params, ...)`，而不是 canonical billing view。
3. Fingerprint 对 mutable Params 做排除列表投影，且由调用者决定哪些 metadata 是否存在。
4. History 从 task columns、Params 和本地 draft 各自拼装参数。
5. 多个 UI/SDK 入口分别重建 duration、ratio、resolution、mode 和参考素材。
6. Preflight/parser 直接作为 warning side effect 写入 Params，而不是返回版本化的 hints/result。
7. `quality` 作为视频独立业务字段继续传播。

过渡期允许 raw Params 双写和 legacy fallback，但只能由 canonical builder 负责生成；下游不应再新增 raw Params 读取。

## 6. PR-A 范围冻结

### 允许

- 新增共享类型：`PromptIntentHints`、`ConsistencyResult`、`CanonicalVideoRequest`；
- 新增纯函数 builder：输入 legacy/structured DTO + resolved capability，输出 canonical；
- 新增 alias normalization、capability normalization、时间轴排除和冲突检测测试；
- 规定 canonical schema version、稳定序列化和 provider-semantic projection；
- 测试 `quality -> resolution` 的视频兼容 alias 行为；
- 测试 builder 幂等性、未知字段 fail-closed/diagnostic 行为和不覆盖结构化字段。

### 禁止

- Provider adapter 改为读取 canonical；
- Billing quote 或 billing state machine 改输入/状态；
- Task 数据库 schema、task state、retry、channel 改动；
- 页面交互大改或将 Prompt hint 自动应用到 draft；
- 删除 `req.Params`、legacy alias 或旧 fingerprint 兼容逻辑；
- 生产任务 replay、delete 或主动烧积分。

### PR-A 的验收断言

1. 输入结构化 `duration=10`、Prompt 为“30 秒”，canonical execution 仍是 10，hint/result 有 mismatch。
2. `0-8s / 8-18s / 18-30s` 不生成总时长 hint。
3. `ratio=9:16` 与 `aspect_ratio=9:16` 输出完全相同 canonical。
4. `quality=720p`（视频 legacy input）只输出 `resolution=720p`，不输出第二个 canonical quality 语义。
5. 同一个 normalized input 重复 builder 结果稳定；warning 顺序稳定。
6. Prompt hint、warning、金融/合规语义不能改变 execution 或产生 hard reject。
7. 参考图只来自 structured asset inputs；Prompt 不能新增 image identity。
8. builder 不记录完整 Prompt telemetry；只产生 hash/length 所需的隐私安全字段。

## 7. PR-B / PR-C / PR-D 冻结边界

### PR-B：Intent Parser + conflict contract + UX/API

- 引入 parser 输出和 API/UI warning contract。
- Web、小程序复用三动作：按当前设置生成、应用提示词参数、修改提示词。
- “应用提示词参数”只修改 draft，重新 builder、校验和估价。
- Provider 最终 payload、Billing 输入和 task state 保持不变。

### PR-C：逐域切换到 Canonical

建议顺序：

1. task snapshot：先双写 canonical projection + legacy Params；
2. billing quote：读取 canonical billing view，保留旧状态机；
3. fingerprint/recovery：新任务使用 canonical provider view，legacy task 继续旧兼容路径；
4. Provider adapter：按模型逐个切换并做 payload golden diff；
5. telemetry/history：从 canonical projection 读取最终参数。

每一步都必须保留前后值 diff、可观测性和回滚开关，不进行一次性全链路切换。

### PR-D：旧路径清理

只有在自然流量观察完成、legacy task drain 且回归绿后进行：

- 删除下游 raw Params 读取；
- 删除深层 alias 重复逻辑；
- 删除不再使用的双写字段和 legacy fallback（需单独核准）；
- 删除旧 fingerprint 分支前，确认生产无未完成 legacy execution。

## 8. Review 结论

- `CURRENT → TARGET` 确实减少参数来源，前提是 Canonical builder 成为唯一 boundary exit。
- Canonical Request 不应承载 billing/task/provider/history 的状态副本，只承载生成语义及诊断元数据。
- `PromptIntentHints` 和 `ConsistencyResult` 是解释层，不是第二套执行参数。
- `quality` 在当前视频域必须收敛为 `resolution` 的输入 alias/Provider 映射，不得成为第二个 canonical 字段。
- 可以冻结并开始 PR-A；仍然**不应**在本阶段修改业务执行代码。

Phase 2 状态：**PASS**  
Implementation Plan：**FROZEN**  
下一允许动作：仅开始 PR-A 的契约、builder 和测试设计/实现。
