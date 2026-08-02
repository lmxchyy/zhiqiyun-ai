# Grok Imagine 1.5 Video Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the exact upstream model `grok-imagine-1.5-video` with text-to-video, 1-7 reference-image video generation, and 15 points/second billing.

**Architecture:** Keep the existing OpenAI-compatible video provider and add an exact model-specific request/polling contract. Represent multi-reference input internally as canonical `reference_images`, retain `first_frame` only for compatibility, translate the upstream request to `image_urls`, and drive the mini-program uploader from `max_reference_images` rather than a hard-coded limit.

**Tech Stack:** Go/Gin provider and capability services, Vue 3 + TypeScript + uni-app, shared TypeScript business SDK, Node test runner.

## Global Constraints

- Exact model ID: `grok-imagine-1.5-video`.
- Upstream base URL: `https://newapi.zs-kjhn.cn`.
- Text-to-video accepts zero images; image/reference-to-video accepts 1-7 images.
- The documented upstream contract is `POST /v1/videos`, with `size`, numeric `duration` (6-30), `quality`, and public HTTP(S) `image_urls`; polling is `GET /v1/videos/{id}`.
- Billing is 15 points/second, with resolution multipliers 480p x1, 720p x1.2, and 1080p x2.
- The current upstream document exposes only 480p and 720p. Keep the approved 1080p multiplier in billing configuration for future enablement, but do not expose 1080p or 4K in the model schema until the upstream supports it.
- Do not modify database structure, Seedance protocol, Doubao protocol, payment flow, or unrelated pages.
- Do not deploy, upload a WeChat version, commit, or push without separate authorization.

---

### Task 1: Provider protocol contract

**Files:**
- Modify: `backend-go/internal/provider/video/openai_compatible.go`
- Test: `backend-go/internal/provider/video/openai_compatible_test.go`

**Interfaces:**
- Consumes: `generation.CreateRequest` with canonical `duration`, `resolution`, `aspect_ratio`, and `reference_images`.
- Produces: `POST /v1/videos`, then polls `GET /v1/videos/{id}`.

- [x] Add failing tests proving the exact model translates canonical parameters to `duration`, `size`, `quality`, and `image_urls`, while omitting internal `aspect_ratio`, `resolution`, `reference_images`, `seconds`, and `input_reference`.
- [x] Add failing tests for text mode with no image, seven images accepted, eight images rejected, and non-HTTP(S) images rejected before any upstream call.
- [x] Add a failing test proving the returned task `id` is used to poll `/v1/videos/{id}` and the nested `data.result.videos[0].url` result is returned.
- [x] Run the focused Provider tests and verify RED failures are caused by the missing exact-model contract.
- [x] Implement exact model detection, request-body mapping, endpoint selection, polling ID extraction, and the 0-7 image guard.
- [x] Re-run the same test command and verify GREEN.

### Task 2: Model capability and billing configuration

**Files:**
- Modify: `backend-go/internal/httpserver/ai_capability.go`
- Modify: `backend-go/internal/httpserver/video_generation_validation.go`
- Test: `backend-go/internal/httpserver/server_test.go`
- Test: `backend-go/internal/httpserver/video_generation_validation_test.go`

**Interfaces:**
- Produces: model capability `supports_text_to_video=true`, `supports_image_to_video=true`, `max_reference_images=7`, duration 6-30, supported resolutions `480p/720p`, five documented aspect ratios, and core supported parameters.
- Produces: per-second billing rule with base price 15 and approved resolution multipliers.

- [x] Add failing tests proving capability normalization preserves a seven-image limit and the request validator accepts seven distinct references but rejects eight.
- [x] Add failing tests proving default model/schema/tenant-limit/channel entries resolve the exact model while hiding unsupported 1080p/4K controls.
- [x] Add billing tests for 6 seconds 480p = 90 points, 10 seconds 720p = 180 points, and the approved future 10 seconds 1080p price = 300 points.
- [x] Run the focused HTTP server tests and verify RED.
- [x] Implement the capability normalization, canonical `reference_images` snapshot, default model/schema/limit/channel entry, and billing rule.
- [x] Re-run focused tests and verify GREEN.

### Task 3: Shared SDK and mini-program multi-reference flow

**Files:**
- Modify: `packages/business-sdk/src/mappers.ts`
- Modify: `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`
- Test: `tests/business-sdk-mappers.test.mjs`
- Test: `tests/user-mini-video-dynamic-parameters.test.mjs`

**Interfaces:**
- Consumes: `VideoModelCapabilities.maxReferenceImages`.
- Produces: an image-mode task containing every uploaded remote URL in `reference_images`, plus a compatible `first_frame` for the first URL.

- [x] Add failing SDK tests proving seven references survive into the task request, text mode carries none, and eight references are rejected.
- [x] Add failing source-contract tests proving the mini-program picker count comes from the model capability and all selected images are uploaded for video submission.
- [x] Run the focused Node tests and verify RED, while recording the three pre-existing missing-import failures separately.
- [x] Update the mapper and mini-program workbench so model switches retain at most the new model limit, the picker accepts up to seven, the UI shows the selected count, estimates include all references, and task creation uploads all references.
- [x] Re-run focused Node tests and verify all new tests GREEN.

### Task 4: Integrated verification

**Files:**
- Verify all files above; no additional production files.

- [x] Run `gofmt` on changed Go files.
- [x] Run Provider and HTTP focused Go tests.
- [x] Run shared SDK and mini-program focused Node tests.
- [x] Run the repository-equivalent TypeScript checks.
- [x] Run the mp-weixin production build without uploading it.
- [x] Run `git diff --check` and inspect the complete diff for unrelated changes.
- [x] Report exact changed files, test evidence, upstream protocol assumptions, and the separate production admin configuration needed to bind the model to the New API channel.
