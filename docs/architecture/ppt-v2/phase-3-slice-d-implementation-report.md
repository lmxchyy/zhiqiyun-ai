# PPT V2 Phase 3 Slice D Implementation Report

## Scope

Slice D completes the conversational editing loop on the existing PPT V2 deck and preview models. It does not add a free-form editor or enter Phase 4.

## Architecture

Natural-language edit requests are converted through the existing `EditPlanningPort` into validated `EditCommand` values. The application owns tenant, owner, deck identity, revision identity, and artifact identity. Commands never patch SlideIR or database JSON directly.

Supported commands are `UPDATE_TEXT`, `REGENERATE_SLIDE`, `CHANGE_LAYOUT`, `REPLACE_IMAGE`, `MOVE_SLIDE`, `ADD_SLIDE`, and `DELETE_SLIDE`.

## Durable edit worker

Edit requests reuse the Phase 2 `GenerationJob` lease, fencing, retry, and recovery machinery. The request is durably stored in the existing `agent_plans.deck_state` JSON as `PendingEdit`; the worker claims the existing `AGENT_OUTLINE` job and checkpoints planning, content, asset, layout, quality, render, and revision-commit stages.

Completed provider and render checkpoints are reused after restart. A stale lease cannot checkpoint or commit. Historical attempt numbers are preserved so a new edit attempt cannot collide with an existing PostgreSQL attempt identity.

No second job system or migration was introduced.

## Revision and idempotency

Each accepted edit creates an immutable parent-linked `DeckRevisionSnapshot`. Duplicate `commandId` replay returns the committed revision without creating another revision or artifact. A stale `baseRevision` is rejected. Undo moves the current revision pointer to its parent and keeps all historical revisions.

## Artifact and tenant safety

The worker continues to use the existing private PPTX storage, task relation, and artifact boundary. Revision file identity is persisted with the revision. All reads, edits, asset resolution, and commits remain tenant/owner scoped.

## Golden 3

`tests/ppt-v2-phase3-slice-d.test.mjs` adds the deterministic Golden 3 Professional Deck Revision fixture. It applies `UPDATE_TEXT`, `CHANGE_LAYOUT`, and `REPLACE_IMAGE`, then verifies:

- unaffected slide semantic parity;
- unaffected geometry parity;
- expected differences on affected slides;
- stable element and slide identities;
- preview projection fidelity and private asset references.

The fixture does not call a real provider. Existing Golden 1, Golden 2, PPTX renderer, and OfficeCLI gates remain green.

## PostgreSQL integration gate

The real PostgreSQL integration suite covers the edit worker path together with the existing gates. It verifies durable enqueue/claim/commit, revision advancement, immutable history, stale revision rejection, duplicate edit idempotency, restart-safe checkpoint state, lease/fencing behavior, artifact/task durability, and tenant isolation.

## Validation evidence

Final GitHub Actions run `31959764206` passed:

- `user-core / backend-go`: GREEN;
- `user-core / user-core`: GREEN;
- real PostgreSQL generation gates: PASS, no SKIP;
- Slice D edit worker PostgreSQL coverage: PASS;
- Golden 1/2/3 and OfficeCLI: PASS;
- existing Phase 1, Phase 2, Slice A, Slice A.1, Slice B, and Slice C regressions: GREEN.

Local validation also passed `go test ./internal/app/ppt -count=1`, the full `npm run test:ppt-v2` suite (28/28), and `git diff --check`.

## Known limitations

`REGENERATE_SLIDE` and image replacement remain bounded to the current provider and asset contracts. Slice D does not provide arbitrary element patching, drag/drop editing, collaboration, full revision history UI, or new billing behavior.

## Exit Gate

All Slice D technical gates and inherited Phase 1/2/A/A.1/B/C regressions are green. No new migration, external SDK, secret, or production access was introduced.

`SLICE D STATUS: READY`

This milestone stops before Phase 4.
