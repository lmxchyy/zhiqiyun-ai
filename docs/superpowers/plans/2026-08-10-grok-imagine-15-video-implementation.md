# Grok Imagine 1.5 Video Implementation Plan

> **For Codex:** Execute this plan in the current task with `superpowers:executing-plans`. Follow TDD: add each focused regression first, observe the expected failure, then make the smallest implementation change.

**Goal:** Add `grok-imagine-1.5-video` as a distinct NewAPI video model with the documented `/v1/videos` protocol, 6–30 second duration options, 0–7 reference images, and flat 15 points/second pricing while leaving `grok-imagine-video-1.5-preview` on its existing per-request behavior.

**Architecture:** Keep the existing generation pipeline and add one exact model-specific protocol branch in the OpenAI-compatible video adapter. Declare the new model's capabilities and billing rule in the capability defaults, carry all selected reference images through the mini-program and business SDK as `image_urls`, and use the existing asynchronous task polling/result parser. Do not add a new provider abstraction or a compatibility alias.

**Tech Stack:** Go backend/provider tests, Vue 3 + TypeScript + uni-app, shared TypeScript business SDK, Node test runner, PostgreSQL runtime configuration.

---

## Task 1: Lock the NewAPI request contract with failing provider tests

**Files:**
- Modify: `backend-go/internal/provider/video/openai_compatible_test.go`
- Modify: `backend-go/internal/provider/video/openai_compatible.go`

1. Add a test for `grok-imagine-1.5-video` asserting `POST /v1/videos`, numeric `duration`, `size`, `quality`, and no legacy `seconds`, `aspect_ratio`, `resolution`, or `input_reference` fields.
2. Assert zero references are accepted and seven references are forwarded in `image_urls`; add an eight-reference rejection test that makes no HTTP request.
3. Return a documented completed response containing `data.result.videos[0].url` and assert the provider result URL is extracted.
4. Run the focused tests and confirm they fail because the current adapter posts to `/v1/video/generations`, uses legacy parameter names, and has no seven-image cap.
5. Add exact `isGrokImagine15VideoModel` routing, the documented body shape, and the seven-image validation. Preserve the existing `grok-video-1.5` exactly-one-image branch.
6. Run `go test ./internal/provider/video` and confirm the full provider package passes.

## Task 2: Declare capabilities and validate multi-reference requests

**Files:**
- Modify: `backend-go/internal/httpserver/video_generation_validation_test.go`
- Modify: `backend-go/internal/httpserver/video_generation_validation.go`
- Modify: `backend-go/internal/httpserver/ai_capability.go`

1. Add failing tests for the new model's normalized capabilities: text and image modes, no required reference in text mode, max seven images, all integer durations 6–30, resolutions `480p`/`720p`, and ratios `16:9`, `9:16`, `1:1`, `3:2`, `2:3`.
2. Add request validation tests proving seven `image_urls` survive canonicalization and eight are rejected with `VIDEO_IMAGE_LIMIT_EXCEEDED`.
3. Add a preview-model regression proving it still requires exactly one starting image and does not inherit the seven-image capability.
4. Run the focused Go validation tests and observe the max-two normalization and legacy-image clearing failures.
5. Stop coupling `maxReferenceImages` to last-frame support; cap declared multi-reference capacity at seven. Canonicalize image-mode references into `image_urls`, keep the first entry as `first_frame`, and preserve all references for the provider.
6. Add the exact new model to default models/module bindings/default tenant limits with explicit capabilities; set the video tenant duration ceiling to 30 while each model's declared options remain authoritative.
7. Run `go test ./internal/httpserver/ -run "TestVideoModel|TestValidateVideo|TestNormalizeVideo"`.

## Task 3: Make billing flat at 15 points per second and cost 0.13 CNY per second

**Files:**
- Modify: `backend-go/internal/httpserver/video_generation_estimate_test.go`
- Modify: `backend-go/internal/httpserver/ai_capability.go`
- Modify: `backend-go/internal/httpserver/billing_v1_store_json.go`

1. Add failing estimate tests for 6 seconds = 90 points and 30 seconds = 450 points at both 480p and 720p.
2. Add the default `per_second` billing rule with base price 15, minimum charge 15, cost price 13 (points accounting), and no resolution multiplier.
3. Add the provider-cost row for NewAPI with `PER_SECOND`, unit cost `0.13`, and CNY currency.
4. Run the estimate and billing normalization tests; verify changing resolution does not change the estimate.

## Task 4: Carry up to seven images through the business SDK

**Files:**
- Modify: `tests/business-sdk-mappers.test.mjs`
- Modify: `packages/business-sdk/src/mappers.ts`

1. Add failing mapper tests proving an image-to-video draft with seven references emits all seven URLs in `params.image_urls`, retains the first as `first_frame`, and rejects eight references.
2. Add a text-to-video test proving the same model works with zero images.
3. Change the video payload mapper to build canonical `image_urls` from all references while continuing to send `first_frame`/`last_frame` when supported.
4. Run `node --test tests/business-sdk-mappers.test.mjs`.

## Task 5: Make mini-program controls model-driven

**Files:**
- Modify carefully (preserve unrelated user edits): `apps/user-uni/src/components/MiniProgramRoleWorkbench.vue`
- Modify: `tests/user-mini-video-dynamic-parameters.test.mjs`

1. Add source-level protected-surface assertions for a model-driven reference limit, all-reference upload, canonical `image_urls`, and a truncation confirmation before switching to a lower-limit model.
2. Run the focused Node test and observe the hard-coded one-image behavior fail.
3. Use `maxReferenceImages` for the reference limit, upload all selected video references, pass them to the SDK, and show count-aware copy.
4. When switching models, confirm before removing unsupported images or truncating references to the target model's maximum; apply the selected model's duration/resolution/ratio fields without stale values.
5. Include all selected references in estimate parameters so displayed 15-points/second pricing matches the eventual submission.
6. Run `node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs`.

## Task 6: Expose the distinct model in admin generation controls

**Files:**
- Modify: `admin-vue/src/utils/videoGeneration.ts`
- Modify only if required by focused behavior: `admin-vue/src/App.vue`
- Add/modify the nearest admin video-generation test.

1. Add `Grok Imagine Video 1.5` mapped exactly to `grok-imagine-1.5-video` with durations 6 through 30, the five documented ratios, and `480p`/`720p`.
2. Keep `Grok Video 1.5` mapped to the old preview identifier and its single-image/short-duration behavior.
3. If the admin form hard-codes preview validation by label, isolate that rule so the new model can submit text-to-video or image-to-video without inheriting the preview constraint.
4. Run the nearest admin unit test and `npm.cmd run typecheck`/build command available in `admin-vue`.

## Task 7: Update local runtime configuration and verify end to end

**Files:**
- Local ignored config: `.env` (only the exact video-model selection if needed)
- Local PostgreSQL rows: exact model capability, billing-rule, provider-cost, tenant-limit, and channel-model records

1. Re-read exact target rows and update them transactionally: remove resolution multipliers, set 15 points/second, set supplier cost 0.13 CNY/second, store the explicit capability object, allow duration 30, and route the exact model through `https://newapi.zs-kjhn.cn`.
2. Do not copy or expose any API key. Before a real remote generation, obtain explicit authorization to send the configured secret to NewAPI.
3. Run the required protected-surface suites:
   - `node --test tests/user-mini-video-dynamic-parameters.test.mjs tests/video-generation-estimate-sdk.test.mjs tests/video-model-parameters.test.mjs`
   - `go test ./internal/httpserver/ -run "TestVideoGenerationEstimate|TestBillingCenterV1Acceptance|TestNormalizeAICapabilityDefaultsMergesMissingBillingRules"`
4. Run package regressions: `go test ./internal/provider/video`, the focused HTTP validation suite, business SDK mapper tests, and affected frontend typecheck/build.
5. With authorization, submit one minimal 6-second text-to-video job, poll `GET /v1/videos/{id}` to completion, and verify task output, work-center visibility, 90-point charge, and 0.78 CNY estimated supplier cost. If the remote service rejects or stalls, capture the sanitized status/body and identify the exact external failure without logging the key.
6. Review `git diff --check` and the relevant diff only; do not stage or alter unrelated dirty-worktree files.
