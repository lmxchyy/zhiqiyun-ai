# Video Prompt Guard — Phase 1 Design

Issue: #175  
Status: proposal; requires approval before implementation.

## Goal

For new canonical video tasks, create a deterministic `provider_prompt` that reduces provider-facing execution burden while preserving the user request and every structured execution field.

```text
user_prompt (immutable)
  -> prompt intent / consistency check
  -> Canonical Video Request (immutable execution truth)
  -> Prompt Guard (deterministic, versioned)
  -> provider_prompt (transport only)
  -> provider adapter payload
```

This does not promise to eliminate `PROVIDER_ASYNC_GENERATION_FAILED`; it makes the submitted prompt observable and lower-risk without changing routing or retry semantics.

## Data contract

Store a new, versioned snapshot adjacent to (not inside) `canonical_video_request`:

```json
{
  "schema_version": 1,
  "guard_version": 1,
  "user_prompt_hash": "sha256...",
  "provider_prompt": "...",
  "provider_prompt_hash": "sha256...",
  "transformation_codes": ["VIDEO_PROMPT_SIMPLIFIED_TEXT_RENDERING"]
}
```

Suggested Params key: `video_prompt_execution`.

Rules:

- `user_prompt` is `generationTask.Prompt` and `canonical_video_request.prompt`; it is never modified.
- `provider_prompt` is never inserted into the Canonical request and never used for pricing or user-request fingerprints.
- SHA-256 hashes cover trimmed exact text. Empty transformation list means `provider_prompt == user_prompt`.
- The snapshot is created once for a new task and reused exactly for worker redelivery, recovery, and connector dispatch.
- Existing canonical tasks without the snapshot, and all legacy tasks without canonical data, retain the current original-prompt behavior. No backfill.

## Deterministic transformation rules

The engine receives only:

```text
user_prompt + canonical execution + existing intent hints
```

It returns only the snapshot above. It must never load provider configuration, alter Params execution keys, call an LLM, or invent people, brands, products, scenes, or claims.

| Code | Conservative rule | Provider-prompt result |
|---|---|---|
| `VIDEO_PROMPT_STRIPPED_EXECUTION_PARAMS` | Remove exact canonical duration / ratio / resolution declarations only when they are request-level declarations. Preserve `0-8s`, `8-18s`, `18-30s`, clock ranges, and other timeline ranges. | Execution repetition is removed; scene order remains. |
| `VIDEO_PROMPT_REMOVED_MISSING_REFERENCE` | If intent asks for reference images but canonical `reference_images` is empty, remove only the reference-dependent clause. Preserve independent, explicit style words in surrounding text. | No nonexistent image is claimed or synthesized. |
| `VIDEO_PROMPT_SIMPLIFIED_TEXT_RENDERING` | Replace demands for exact on-screen/UI/logo/table copy with a request for equivalent visual UI/signage without readable text. Keep the compliance/business scene and semantic warning intact. | No exact Chinese text rendering demand. |
| `VIDEO_PROMPT_SIMPLIFIED_SUBTITLE_REQUIREMENT` | Remove exact subtitle content and replace with “leave a clear lower subtitle-safe area” only when subtitles were explicitly requested. | No subtitle rendering or post-production is performed. |
| `VIDEO_PROMPT_SIMPLIFIED_AUDIO_REQUIREMENT` | Replace exact narration, spoken wording, lip sync, or word-level audio synchronization demand with natural expression / audiovisual atmosphere. | No TTS, voice, or audio generation is added. |
| `VIDEO_PROMPT_SIMPLIFIED_TIMELINE` | For recognised, ordered timeline segments, remove time labels and collapse only their framing into an ordered “scene 1/2/3” sequence. Preserve each scene's surviving subject/action and declared style. | Story order remains; no storyboard or segment generation is created. |
| `VIDEO_PROMPT_COMPLEXITY_REDUCED` | Add only if at least one reduction above was made, or the existing complexity detector finds the defined high-complexity combination. | Stable telemetry / experiment flag; no user-facing copy contract. |

### Safety behavior

- Rules work on bounded, explicit Chinese/English patterns. If a phrase cannot be safely isolated, leave it unchanged.
- Financial, compliance, tax, brand, product, subject, relationship, style, and core-scene language is never a removal target by itself.
- The pre-existing `VIDEO_PROMPT_REFERENCE_MISSING` consistency warning remains the warning contract. The guard's `VIDEO_PROMPT_REMOVED_MISSING_REFERENCE` records a transformation, not a replacement warning.
- The guard does not reject requests.

## Integration plan

1. **Preparation boundary** — after `persistCanonicalVideoRequest` in `prepareGenerationTaskRequest`, build and persist `video_prompt_execution`. This occurs before the task transaction so every dispatch sees the immutable snapshot.
2. **Worker dispatch** — in `runVideoGenerationTask`, call `canonicalVideoDownstreamRequest` first, then apply the persisted provider prompt to the cloned transport `generation.CreateRequest`, immediately before `PrepareVideoTask`.
3. **Connector dispatch** — apply the same helper after its canonical projection and before `PrepareVideoTask`.
4. **No provider adapter changes** — adapters already take their prompt from `req.Prompt`; the transport projection is sufficient and keeps model-specific payload builders unchanged.
5. **No billing/fingerprint changes** — `generationQuoteForRequest`, `canonicalVideoRequestHash`, and `videoRequestFingerprint` remain byte-for-byte unchanged.

## Idempotency behavior

```text
Same user prompt + same structured request
  -> same canonical hash / current request fingerprint
  -> same persisted provider-prompt snapshot for that task
  -> same provider prompt on delivery/recovery

Later Guard version
  -> applies only to newly created tasks
  -> cannot change an existing task's dispatched prompt or fingerprint
```

`provider_prompt_hash` is an execution-observability value, not a substitute for `user_request_fingerprint`.

## Telemetry

Emit a dedicated structured event only after a task ID exists:

```text
video_prompt_guard
  task_id
  model
  canonical_hash
  user_prompt_hash
  provider_prompt_hash
  user_prompt_length
  provider_prompt_length
  transformation_codes
  preflight_warning_codes
  guard_version
```

Forbidden: full user/provider prompt, reference URLs, signed URLs, API keys, and arbitrary error payloads.

## Unit-test matrix

1. Simple prompt is byte-for-byte unchanged.
2. Canonical duration/ratio/resolution duplication is removed; canonical execution is unchanged.
3. A duration conflict retains canonical `15s` and its existing consistency warning.
4. `0-8s`, `8-18s`, `18-30s` preserve three ordered scenes.
5. Missing reference phrase is removed only when canonical references are empty; existing missing-reference warning remains.
6. Existing reference input retains reference intent.
7. Exact Chinese UI/text requirements become non-readable visual UI.
8. Subtitle requirements become subtitle-safe space, with no text payload.
9. Voiceover/lip-sync requirements lose exact audio synchronization requirements.
10. Combined multi-scene/timeline/subtitle/voice input emits complexity reduction.
11. Finance/compliance/tax terms remain present and never reject.
12. Task prompt and canonical prompt remain byte-for-byte original.
13. Canonical hash and current video fingerprint remain unchanged before/after guard metadata is added.
14. Grok/OpenAI-compatible provider payload receives `provider_prompt`; model/duration/ratio/resolution/mode/reference inputs equal canonical execution.
15. Retry/recovery/connector projection uses the stored snapshot, not the current Guard implementation.
16. Legacy requests retain raw Params and original prompt behavior.
17. Telemetry contains hashes/lengths/codes only.

## Migration and rollout risk

- Phase 1 is code plus unit tests only; no production deploy or feature enablement is included.
- A later rollout must be separately reviewed, should be feature-gated, and must use a stable stored Guard snapshot per task.
- Success/failure-rate comparison must control for model, channel, input mode, duration, ratio, and resolution; it must not be used to conceal #162 channel reliability failures.
