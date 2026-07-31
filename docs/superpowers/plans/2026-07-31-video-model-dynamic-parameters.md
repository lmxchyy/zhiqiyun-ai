# Video Model Dynamic Parameters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the mini-program video creation form derive its editable parameters from the selected model's FinalSchema and real Provider capabilities, with atomic model switching and exact read-only point estimation.

**Architecture:** A pure business-SDK module derives the seven-parameter editable contract and performs model transitions. `MiniProgramRoleWorkbench` owns UI state and asynchronous model/estimate sequencing. The Go backend remains authoritative by deriving the same FinalSchema/Provider intersection, rejecting unsupported input, and exposing a no-side-effect estimate endpoint that reuses the production billing function.

**Tech Stack:** Vue 3, TypeScript, uni-app, Node test runner, Go, Gin.

## Global Constraints

- Only the mini-program `UserVideoCreationPage -> MiniProgramRoleWorkbench` gets dynamic controls.
- Web `AiCreationPage` receives at most the minimal `generate_audio` compatibility fix and no layout/control changes.
- Video whitelist is exactly `duration`, `resolution`, `aspect_ratio`, `fps`, `generate_audio`, `motion_strength`, `camera_movement`.
- Do not silently drop unsupported parameters in the backend; reject them.
- Do not change database schema, deploy production, upload a WeChat version, or modify excluded features.
- Build the mini-program as version `2.0.36`.

---

### Task 1: Shared SDK video form contract

**Files:**
- Create: `packages/business-sdk/src/videoParameters.ts`
- Modify: `packages/business-sdk/src/index.ts`
- Test: `tests/video-model-parameters.test.mjs`

**Interfaces:**
- Produces: `VIDEO_PARAMETER_KEYS`, `deriveEditableVideoFields(schema, capabilities)`, `transitionVideoParameterValues(previous, fields)`, and `buildVideoSubmissionParameters(values, fields)`.
- Consumes: `VideoModelCapabilities` from `@xianzhi/shared-types`.

- [ ] **Step 1: Write failing pure-function tests**

Cover:

```js
assert.deepEqual(
  transitionVideoParameterValues(
    { duration: 5, resolution: "1080p", aspect_ratio: "9:16", generate_audio: true },
    modelBFields,
  ),
  { duration: 5, resolution: "720p", aspect_ratio: "9:16" },
);
```

Also assert `visible=false`, `userEditable=false`, absent Provider capability, unsupported audio, `fps`, `motion_strength`, and `camera_movement` are absent; legacy `ratio` becomes `aspect_ratio`; invalid values use Schema default then first option.

- [ ] **Step 2: Run tests and observe failure**

Run:

```powershell
node --test tests/video-model-parameters.test.mjs
```

Expected: FAIL because the new SDK exports do not exist.

- [ ] **Step 3: Implement the minimal pure contract**

Use canonical field shape:

```ts
export interface EditableVideoField {
  key: VideoParameterKey;
  label: string;
  type: string;
  defaultValue?: unknown;
  options: unknown[];
  min?: number;
  max?: number;
  unit?: string;
}
```

All public functions must return new objects/arrays and must not mutate prior model state.

- [ ] **Step 4: Run SDK tests**

Run:

```powershell
node --test tests/video-model-parameters.test.mjs tests/business-sdk-mappers.test.mjs
```

Expected: all tests PASS.

### Task 2: Authoritative backend contract and estimate endpoint

**Files:**
- Modify: `backend-go/internal/httpserver/video_generation_validation.go`
- Modify: `backend-go/internal/httpserver/ai_capability.go`
- Create: `backend-go/internal/httpserver/video_generation_estimate.go`
- Modify: `backend-go/internal/httpserver/server.go`
- Modify: `backend-go/internal/httpserver/video_generation_validation_test.go`
- Create: `backend-go/internal/httpserver/video_generation_estimate_test.go`

**Interfaces:**
- Produces: `POST /api/v1/generation-tasks/estimate`.
- Produces: `editableVideoParameterKeys(resolved resolvedModuleSchema) map[string]bool`.
- Consumes: `prepareGenerationRequest` and `generationPointCostForRequest`.

- [ ] **Step 1: Add failing backend tests**

Add table cases for fields that are hidden, not user-editable, absent from Provider supported parameters, or outside the whitelist. Each supplied field must return `VIDEO_PROVIDER_PARAMETER_NOT_SUPPORTED` or `VIDEO_PARAMETER_NOT_EDITABLE`, not be silently removed.

Add estimate test:

```go
estimate := estimateVideoGenerationCost(prepared, data)
pending, err := store.CreatePendingGenerationTask(prepared)
if err != nil { t.Fatal(err) }
if estimate.EstimatedPoints != pending.PointCost {
    t.Fatalf("estimate=%d formal=%d", estimate.EstimatedPoints, pending.PointCost)
}
```

Assert account balances, tasks, and ledgers are unchanged by estimate alone.

- [ ] **Step 2: Run targeted tests and observe failure**

Run:

```powershell
go test ./internal/httpserver -run 'Test(VideoEditable|VideoGenerationEstimate|VideoTaskSnapshot)' -count=1
```

from `backend-go`.

Expected: FAIL because the endpoint and editable-field enforcement do not exist.

- [ ] **Step 3: Implement FinalSchema enforcement**

Canonicalize `ratio` before validation. Build the allowed video parameter set from `resolved.FinalSchema.Fields`, requiring whitelist membership, `Visible`, `UserEditable`, and `resolved.Model.VideoCapabilities.SupportedParameters`. Reject any supplied whitelist parameter outside that set.

Do not silently delete rejected user input.

- [ ] **Step 4: Implement no-side-effect estimate handler**

Decode `generation.CreateRequest`, force `module_code=video_generation`, use a non-empty internal prompt only for Schema validation when the caller omits prompt, call `prepareGenerationRequest`, then:

```go
points := generationPointCostForRequest(prepared, data)
```

Return `estimatedPoints`, resolved `model`, `billingType`, `quantityField`, `quantity`, and a note that formal creation recalculates the charge.

- [ ] **Step 5: Run backend tests**

Run:

```powershell
go test ./internal/httpserver -run 'Test(VideoEditable|VideoGenerationEstimate|VideoTaskSnapshot|VideoProviderParameter)' -count=1
```

Expected: PASS.

### Task 3: Mini-program model selector, dynamic controls, and atomic switch

**Files:**
- Modify: `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`
- Test: `tests/user-mini-video-dynamic-parameters.test.mjs`
- Test: `tests/user-mini-video-model-fallback.test.mjs`

**Interfaces:**
- Consumes: Task 1 SDK functions and `/api/v1/module-schema`.
- Consumes: `businessSdk.models.list()`.
- Produces: local `videoParameterFields`, `videoParameterValues`, `videoEstimate`, and `requestVideoModelSwitch(modelCode)`.

- [ ] **Step 1: Add failing mini-program behavior tests**

Assert source contains and behavior helpers cover:

- model A→B retains common legal values;
- unsupported values/fields are removed;
- Schema defaults are applied;
- audio appears only for supported Seedance content;
- text-to-video hides reference upload;
- image-to-video shows it;
- switch with an existing reference calls `uni.showModal` with the exact approved message and cancellation preserves all state;
- request sequence guards Schema and estimate responses.

- [ ] **Step 2: Run tests and observe failure**

Run:

```powershell
node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/user-mini-video-model-fallback.test.mjs
```

Expected: FAIL because dynamic UI/state does not exist.

- [ ] **Step 3: Implement model and parameter state**

Load only video-capable entries from `businessSdk.models.list()`. Fetch candidate module Schema before committing a switch. Use a monotonically increasing sequence for both Schema and estimate calls.

Commit a model transition only after any reference-image confirmation succeeds:

```ts
const nextValues = transitionVideoParameterValues(videoParameterValues.value, nextFields);
selectedVideoModelCode.value = candidate.model;
videoParameterFields.value = nextFields;
videoParameterValues.value = nextValues;
```

- [ ] **Step 4: Render controls without changing the surrounding page layout**

Insert a compact video settings block in the existing creation form. Render select/radio options as buttons, boolean as a switch, and numeric fields as bounded inputs. Do not introduce Web controls.

- [ ] **Step 5: Submit only final SDK parameters**

Build `parameters` with:

```ts
buildVideoSubmissionParameters(videoParameterValues.value, videoParameterFields.value)
```

Do not pass stale `restoredCreationParams` video whitelist fields.

- [ ] **Step 6: Run mini-program tests and typecheck**

Run:

```powershell
node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/user-mini-video-model-fallback.test.mjs
npm.cmd --workspace apps/user-uni run typecheck
```

Expected: PASS.

### Task 4: Point estimate integration and Web compatibility boundary

**Files:**
- Modify: `packages/business-sdk/src/types.ts`
- Modify: `packages/business-sdk/src/generation.ts`
- Modify: `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`
- Modify only if required: `apps/user-uni/src/pages/AiCreationPage.vue`
- Test: `tests/video-generation-estimate-sdk.test.mjs`
- Test: `tests/user-web-video-compatibility.test.mjs`

**Interfaces:**
- Produces: `businessSdk.generation.estimateVideo(request)`.
- Consumes: `POST /api/v1/generation-tasks/estimate`.

- [ ] **Step 1: Add failing SDK estimate and Web compatibility tests**

Verify estimate serialization includes final model, type, and filtered params. Verify Web generic OpenAI-compatible submission does not unconditionally include `generate_audio: true`.

- [ ] **Step 2: Run tests and observe failure**

Run:

```powershell
node --test tests/video-generation-estimate-sdk.test.mjs tests/user-web-video-compatibility.test.mjs
```

Expected: FAIL before the SDK method and Web compatibility fix exist.

- [ ] **Step 3: Add estimate SDK method**

Return:

```ts
interface VideoGenerationEstimate {
  model: string;
  estimatedPoints: number;
  billingType: string;
  quantityField: string;
  quantity: number;
  note: string;
}
```

- [ ] **Step 4: Wire debounced estimate refresh**

Trigger after committed model transitions and parameter changes. Display `预计消耗 X 积分`; on failure show a non-blocking “暂无法试算，提交时以后端为准” state.

- [ ] **Step 5: Apply minimal Web compatibility change if needed**

Remove the unconditional `generate_audio: true` or pass it through existing SDK capability filtering. Do not add controls or change layout.

- [ ] **Step 6: Run SDK, Web, and mini-program tests**

Run:

```powershell
node --test tests/video-generation-estimate-sdk.test.mjs tests/user-web-video-compatibility.test.mjs tests/user-mini-video-dynamic-parameters.test.mjs
```

Expected: PASS.

### Task 5: Provider, snapshot, and release verification

**Files:**
- Modify if tests expose a gap: `backend-go/internal/provider/video/openai_compatible.go`
- Test: `backend-go/internal/provider/video/openai_compatible_test.go`
- Test: `backend-go/internal/provider/video/seedance_bridge_test.py`
- Test: `backend-go/internal/httpserver/video_generation_estimate_test.go`

**Interfaces:**
- Verifies all earlier tasks end to end; no new runtime interface is expected.

- [ ] **Step 1: Extend Provider request-body tables**

Verify:

- generic OpenAI-compatible maps core parameters only;
- Doubao video maps `aspect_ratio` to `size`;
- Seedance content maps it to `ratio` and includes `generate_audio`;
- `fps`, `motion_strength`, `camera_movement` fail before any HTTP call;
- old `ratio` normalizes without leaking to task snapshots or generic Provider bodies.

- [ ] **Step 2: Run Provider and bridge tests**

Run:

```powershell
go test ./internal/provider/video -count=1
python -m unittest internal/provider/video/seedance_bridge_test.py
```

from `backend-go`.

Expected: PASS.

- [ ] **Step 3: Run complete scoped validation**

Run:

```powershell
node --test tests/video-model-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/user-mini-video-dynamic-parameters.test.mjs tests/user-mini-video-model-fallback.test.mjs tests/user-web-video-compatibility.test.mjs tests/business-sdk-mappers.test.mjs
npm.cmd --workspace apps/user-uni run typecheck
go test ./internal/httpserver -run 'Test(Video|ModuleSchema)' -count=1
go test ./internal/provider/video -count=1
python -m unittest internal/provider/video/seedance_bridge_test.py
git diff --check
```

Expected: PASS.

- [ ] **Step 4: Build mp-weixin 2.0.36**

Run the existing package build and package-size checks with version `2.0.36`; do not invoke the WeChat upload CLI.

Expected: build exits 0 and package-size gate passes.

- [ ] **Step 5: Review exact diff and report**

Classify every modified file as dynamic parameters, estimate/backend validation, Provider test, Web compatibility, documentation, or build artifact. Build artifacts remain untracked/ignored and are not committed.
