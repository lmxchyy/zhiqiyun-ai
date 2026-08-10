# Frontend C2 final integration fix

## Outcome

Frontend C2 is implemented on `b4c1cb79e096dffc09f7585fff48d3261cefe865` without changing backend, SDK, package metadata, lockfiles, build scripts, generated artifacts, or the existing `node_modules.shared-junction` entry.

- The image page now loads the exact selected model schema through the project API client at `/api/v1/module-schema?module_code=image_generation&model_name=...`. The backend handler in this revision accepts `module_code=image_generation`; it does not expose the proposed `module=image` query shape.
- Only a response whose `model_name` matches the requested and still-selected model can update the controls. Stale requests, mismatches, missing schemas, loading, and errors have explicit states; no other model or fixed client option is borrowed.
- Size, quality, and count are schema-derived canonical values. Schema-undeclared quality/count controls and request fields are omitted. Model changes reset to a declared default or first exact option.
- Formal inspiration restore reads only `parameters.ratio`, `parameters.quality`, and `parameters.count` after the exact schema arrives. Exact 1:1, 2:3, and 3:2 matches restore; unsupported 4:3 remains visibly incompatible and generation-disabled.
- Image draft persistence and task creation use top-level canonical `size`, `quality`, and `count`. Deprecated aliases are removed before the real compiled `taskRequestFromDraft` mapping. The old fixed option shim, exports, imports, and calls were deleted.
- The image request fingerprint is computed from the real `taskRequestFromDraft` request snapshot. First/terminal attempts get a new `image_` key; a status-zero or equivalent network-uncertain retry with identical input reuses it; changed input rotates it. Once a task is returned, only task polling continues.
- Terminal `FAILED`/`ERROR` polling preserves a visible Chinese error and error tone. The component exposes “重新生成” through a retry event; terminal retry rotates the request key. Returned task `pointCost` is shown only as actual settlement evidence.
- Empty prompt, loading spinner, reduced motion, focus/hover/pressed guards, aria association/live regions, and idle/loading/success/error tones are covered by mounted component tests. The estimate remains “以生成时结算为准”.
- Video and free-image-edit submission stay on their existing independent configuration paths; free-image-edit copy, full-page structure, and `#ff6b00` action remain unchanged.

## TDD evidence

RED was recorded before each implementation group:

| Group | RED evidence |
| --- | --- |
| schema/canonical/view/retry helpers | `27` tests: `22` passed, `5` failed because the new helpers were absent |
| mounted component states | `31` tests: `27` passed, `4` failed for empty/loading/error-retry/dynamic-control behavior |
| Workbench integration | `32` tests: `31` passed, `1` failed because no canonical `clientRequestId` submit chain existed |
| equivalent uncertain network error | focused `1` test failed: `TypeError("Network request failed")` was incorrectly terminal |

GREEN:

- `node --test tests/user-mini-image-generator.test.mjs`: **32/32 passed**.
- The payload assertion invokes the real compiled Business SDK mapper and deep-compares the resulting generation-task request body; it is not inferred from source regex.
- Mounted Vue SFC tests execute empty prompt, loading, terminal retry emit, status tone, and dynamic schema control behavior.

## Verification

| Command | Result |
| --- | --- |
| `node --test tests/user-mini-image-generator.test.mjs` | PASS, **32/32** |
| M1/M2/M4/M6 aggregate Node command (image, guest/login, video dynamic/fallback/estimate, inspiration, free edit, auth gate) | PASS, **70/70** on the fresh rerun |
| `node --test tests/video-model-parameters.test.mjs` | PASS, **7/7** |
| `npm.cmd run typecheck:packages` | PASS |
| `npm.cmd run typecheck` in `apps/user-uni` | PASS |
| `go test ./internal/httpserver -run TestVideoGenerationEstimate -count=1` | PASS |
| focused generation reserve/refund/double-charge Go tests | PASS |
| `npm.cmd run build:h5` in `apps/user-uni` | PASS; existing dynamic/static import warning and local `os - Alias not found.` shell noise only |
| `git diff --check` | PASS |
| old fixed option/shim symbol scan in the three production files | PASS, no matches |

## WeChat post-build limitation

`npm.cmd run build:mp-weixin:local` completed the uni-app WeChat compilation, then failed inside the existing post-build patcher before its relocation/size stages:

```text
Error: MiniProgramRoleWorkbench component registration not found
apps/user-uni/scripts/patch-mp-native-login.cjs:1660
```

The generated component and all page registrations exist. Its valid final registration is:

```js
wx.createComponent($);
```

The unchanged patcher accepts only `wx.createComponent((\w+))`; JavaScript `$` is a valid identifier but is excluded by `\w`. This is an out-of-scope build-script matcher defect, not a missing Workbench build artifact. Per task boundary, no build-script change or source-level minifier-name workaround was made.

Because the patcher stopped before the remaining transformations, the resulting artifact is not trustworthy for package-size or click verification:

- `test:wallet-build`: **2/4 passed, 2/4 failed** on the incomplete artifact.
- `verify:user-mini-clicks`: failed on expected missing post-patch native bindings.
- Click E2E was not run against that incomplete artifact.
- Exact final MAIN / `pages/user-creation` / TOTAL bytes are therefore **not available and are intentionally not claimed**. A separate build-script fix must rerun `build:mp-weixin:local`, wallet 4/4, package bytes, and click E2E.

The known production H5 route exclusion and temporary-route DCloud slot bug were not changed or wrapped in a compatibility layer.

## Follow-up review fix: uncertain reference upload reuse

The integration review found one important retry gap: a network-uncertain task submission retried the local reference uploads before computing the real SDK request snapshot. If the upload service returned different remote URLs, the request fingerprint and `clientRequestId` changed even though the user's inputs had not.

The fix adds a small ordered source-reference/uploaded-URL cache at the image submission boundary:

- A retry reuses the uploaded URLs only when the preceding task request outcome was `network-uncertain` and the ordered local source-reference snapshot is identical.
- First attempts, changed reference values, changed reference order, explicit terminal request failures, and terminal task retries follow the normal upload path.
- Upload failures create no cache entry.
- The cached URLs enter the existing canonical draft, real compiled `taskRequestFromDraft` request snapshot, fingerprint, and idempotency-key flow. No request-shape fallback or compatibility alias was added.

### Follow-up TDD evidence

RED command:

```text
node --test --test-name-pattern "uploaded URLs|reference values|reference order|terminal retry|failed reference upload|workbench wires" tests/user-mini-image-generator.test.mjs
```

RED result: **7 tests, 1 passed / 6 failed**. Five behavior tests failed because `resolveImageReferenceUploads` did not exist, and the Workbench integration assertion failed because the cache helper was not wired.

GREEN result for the same command: **7/7 passed**. The behavior tests use the real compiled Business SDK `taskRequestFromDraft`, canonical request fingerprint, and request-key state machine. They prove:

- same ordered local references plus a network-uncertain retry call the uploader once and preserve the uploaded URLs, SDK request snapshot, fingerprint, final request, and `clientRequestId`;
- changed reference values and changed order call the uploader again and rotate the fingerprint/key;
- terminal retry calls the uploader normally and uses a new key;
- a rejected upload is not cached.

### Follow-up verification

| Command | Result |
| --- | --- |
| `node --test tests/user-mini-image-generator.test.mjs` | PASS, **37/37** |
| M1/M2/M4/M6 aggregate Node command | PASS, **75/75** |
| `npm.cmd run typecheck:packages` | PASS |
| `npm.cmd run typecheck` in `apps/user-uni` | PASS |
| `npm.cmd run build:h5` in `apps/user-uni` | PASS; existing dynamic/static import warning and local `os - Alias not found.` shell noise only |
| first `npm.cmd run build:mp-weixin:local` | PASS |
| first `npm.cmd run test:wallet-build` | PASS, **4/4** |
| second `npm.cmd run build:mp-weixin:local` | PASS |
| second `npm.cmd run test:wallet-build` | PASS, **4/4** |

The two fresh WeChat builds were byte-identical after upload filtering:

| Package | First build | Second build |
| --- | ---: | ---: |
| MAIN | 1,535,642 B | 1,535,642 B |
| `pages/user-creation` | 141,194 B | 141,194 B |
| TOTAL | 2,380,145 B | 2,380,145 B |

The earlier post-build limitation above was historical evidence from C2 before the separate build matcher correction in `7cbc8c28f`. This follow-up did not change that build script; both fresh post-correction builds and wallet checks now complete successfully.

## Follow-up test-gap fix: production retry orchestration

The upload-reuse behavior was previously covered by a test-owned composition of the individual helpers. That proved each primitive could produce the desired result, but it did not execute the same orchestration that `MiniProgramRoleWorkbench.submitCreation` used in production.

The image submission sequence now has one production entry point, `submitCanonicalImageTask`. It receives canonical draft inputs and injected upload, task-create, and client-ID dependencies, then performs the actual sequence:

1. resolve or reuse the ordered reference upload snapshot;
2. build the canonical image draft;
3. map it through the public Business SDK `taskRequestFromDraft`;
4. fingerprint that request and resolve the `image_` idempotency key;
5. map the final keyed draft and call `createTask`;
6. return success or error together with the exact upload/key/outcome state required by the next attempt.

The Workbench image branch calls this entry point directly and no longer duplicates that sequence. It rethrows the original task error after applying the returned retry state, preserving existing `ApiClientError` handling such as the agreement-required 428 flow.

### Orchestration TDD evidence

RED command:

```text
node --test --test-name-pattern "production image task orchestration|delegates image submission" tests/user-mini-image-generator.test.mjs
```

RED result: **6 tests, 0 passed / 6 failed**. Five behavior tests failed because `submitCanonicalImageTask` was not exported; the auxiliary Workbench contract failed because `submitCreation` did not call it.

GREEN result for the same command: **6/6 passed**. The old test-owned orchestration was deleted. The new tests execute the production helper twice and use the real compiled public SDK mapper while mocking only the external upload/task-create boundaries. They cover:

- status-code-zero uncertainty followed by success: one upload, two task creates, and identical keyed drafts, final mapped requests, fingerprints, and `clientRequestId` values;
- changed reference values and changed reference order: a second upload and a new fingerprint/key;
- a terminal task response: normal retransmission and a new key;
- upload failure: zero task creates and no cache update;
- an explicit 4xx: normal retransmission and a new key on the next attempt.

### Orchestration verification

| Command | Result |
| --- | --- |
| `node --test tests/user-mini-image-generator.test.mjs` | PASS, **37/37** |
| M1/M2/M4/M6 aggregate Node command | PASS, **75/75** |
| `npm.cmd run typecheck:packages` | PASS |
| `npm.cmd run typecheck` in `apps/user-uni` | PASS |
| `npm.cmd run build:h5` in `apps/user-uni` | PASS; the existing dynamic/static import warning and local `os - Alias not found.` shell noise remain |
| first `npm.cmd run build:mp-weixin:local` | PASS |
| first `npm.cmd run test:wallet-build` | PASS, **4/4** |
| second `npm.cmd run build:mp-weixin:local` | PASS |
| second `npm.cmd run test:wallet-build` | PASS, **4/4** |

The two fresh WeChat builds were byte-identical after upload filtering:

| Package | First build | Second build |
| --- | ---: | ---: |
| MAIN | 1,536,531 B | 1,536,531 B |
| `pages/user-creation` | 141,194 B | 141,194 B |
| TOTAL | 2,381,034 B | 2,381,034 B |
