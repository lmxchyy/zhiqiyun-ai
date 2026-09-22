# Video Prompt Guard — Architecture Audit

Issue: #175  
Scope: design only; no production wiring or runtime behavior change.

## Guardrails

- `xz_generation_tasks.prompt` remains the original user prompt.
- `canonical_video_request.prompt` remains the original user prompt.
- Canonical execution, canonical hash, billing, provider/channel selection, retry behavior, capability validation, legacy Params fallback, and asset lifecycle are out of scope.
- This is a deterministic rules engine, not an LLM prompt rewrite, storyboard engine, segment generator, subtitle renderer, TTS system, fallback, or retry.

## Current prompt flow

```text
HTTP / connector request
  -> validateVideoGenerationRequest
  -> inspectVideoPromptPreflight (warning codes + original-prompt hash)
  -> buildCanonicalVideoRequestFromPreparedRequest
  -> persist canonical_video_request + canonical_video_hash
  -> persist PENDING task (task.prompt is original prompt)
  -> outbox/worker or connector dispatch
  -> canonicalVideoDownstreamRequest
       (hydrates model/duration/ratio/resolution/mode/references; sets req.Prompt = canonical.prompt)
  -> generation.Service.PrepareVideoTask
  -> provider execution hook / fingerprint
  -> video provider adapter payload { prompt: req.Prompt, canonical execution fields }
```

### Evidence

| Concern | Current anchor | Finding |
|---|---|---|
| Original prompt persistence | `ai_capability.go`, task store | `req.Prompt` is persisted as the user prompt. |
| Canonical boundary | `video_canonical_request.go` | `canonical_video_request` contains `Prompt` plus execution and diagnostics; hash covers original prompt + execution only. |
| Downstream projection | `canonicalVideoDownstreamRequest` | It intentionally restores original canonical prompt and canonical execution just before dispatch. |
| Billing | `generationQuoteForRequest` | Video quotes first apply canonical projection; only canonical execution fields determine quoted price. |
| Provider payload | `provider/video/openai_compatible.go` | Provider adapters read `req.Prompt`; execution fields are read separately from `req.Params`. |
| Fingerprint | `video_fingerprint.go` | For canonical tasks it hashes `{prompt: canonical.Prompt, execution: canonical.Execution}`. It does not hash task diagnostics. |
| Existing diagnostics | `video_prompt_preflight.go` | Emits stable warning codes and hashes/length only, but has no provider-prompt concept. |
| Connector dispatch | `connector_generation.go` | Uses the same canonical downstream projection before `PrepareVideoTask`. |

## Fingerprint and idempotency finding

The current canonical fingerprint deliberately uses the **original** canonical prompt. Replacing that with a provider-prompt hash would change established provider request identity and make rule/version changes affect idempotency. That must not happen.

A safe design therefore requires:

1. compute and durably persist a provider-prompt snapshot once when a new canonical task is prepared;
2. reuse that snapshot for worker redelivery, recovery, and connector dispatch;
3. leave `canonical_video_request`, `canonical_video_hash`, and `videoRequestFingerprint` unchanged;
4. never recompute an existing task using a later guard version.

## Observability finding

`video_prompt_preflight` already avoids prompt text in logs. Guard telemetry must follow this pattern and emit only hashes, lengths, stable transformation codes, model, task ID, and canonical hash.

## Risks discovered before implementation

1. Rule changes can unintentionally alter subject, brand, or narrative if replacements are broad. Rules must be narrow, deterministic, and no-op when confidence is low.
2. A provider prompt snapshot is business data; it must not be exposed by ordinary logs or copied to provider-error payloads.
3. Guard does not solve channel outages. It is a controlled provider-input experiment only; #162 remains the reliability investigation.
4. Existing / legacy tasks must not be backfilled or reinterpreted. They continue to send their original prompt.
