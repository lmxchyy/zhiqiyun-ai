# Task 5 - Admin PPT Agent workflow report

## Conclusion

Implemented the Admin PPT Agent Phase 1 workflow on top of the existing PPT document generation page without changing routes or replacing the page. The Admin client now uses the shared Axios client for Agent skills, sessions, messages, outline confirmation, task polling, and single-slide revision. Agent stages are treated as authoritative, including a distinct `CANCELLED` terminal state.

## Changed files

- `admin-vue/src/api/ppt.ts`
- `admin-vue/src/types/ppt.ts`
- `admin-vue/src/stores/ppt.ts`
- `admin-vue/src/components/ppt/PptDocumentGeneratePage.vue`
- `admin-vue/tests/pptAgent.spec.ts`
- `.superpowers/sdd/ppt-agent-phase1-regression-integration-20260806/task-5-report.md`

No backend, user-uni, Connector, migration, deployment, or production files were changed.

## Call-chain changes

1. The Admin generation workspace loads `GET /ppt/skills` through the existing shared Admin Axios client.
2. Starting a new Agent flow creates `POST /ppt/sessions` with a generated client request ID, then sends the user's request through `POST /ppt/sessions/:taskId/messages` with an idempotency key.
3. The returned task snapshot updates the store through one explicit stage mapper:
   - `DRAFT -> idle`
   - `OUTLINE_READY -> outline_ready`
   - `GENERATING -> generating`
   - `READY -> success`
   - `FAILED -> failed`
   - `CANCELLED -> cancelled`
4. Outline confirmation calls `POST /ppt/sessions/:taskId/confirm-outline`. The store polls `GET /ppt/tasks/:taskId` every 1500 ms only while the authoritative stage is `GENERATING`; it stops on every other stage and never promotes a cancelled task to success.
5. The existing presentation-side PPT assistant now calls `POST /ppt/sessions/:taskId/revise-slide`. Only the requested slide is replaced from the returned snapshot, so other slide object identities and content are preserved.
6. The previous fixed-delay fake generation progress and local fake Agent suggestion/application path were removed from the active source.

## API and database impact

- Frontend API client additions only; no backend API contract was changed.
- All Agent mutating requests use generated idempotency keys where the backend contract requires them.
- Agent API failures propagate through the existing sanitized Admin API error boundary; there is no Agent mock fallback.
- No database or migration changes.

## Backend contract gap and safe UI behavior

The current legacy `POST /ppt/outline/save` endpoint only echoes an outline and timestamp. It has no Agent task identity, owner-bound persistence, or Agent stage transition. There is no Agent outline-update endpoint in the approved backend contract.

Therefore, local outline edits in an Agent session set `outlineDirty`. Agent save and confirm are blocked with the visible message `请通过 Agent 消息修订大纲或恢复服务端版本` and make zero network requests. The Admin client does not call the echo-only endpoint for Agent sessions and does not confirm a stale server outline. A backend-owned, tenant-safe Agent outline persistence API is a follow-up requirement.

## TDD evidence

- Initial test start was blocked because the isolated worktree had no dependencies: `vitest` was not recognized.
- Installed the lockfile-defined Admin dependencies with `npm.cmd --prefix admin-vue ci --ignore-scripts --no-audit --no-fund`.
- RED: the initial Agent test suite failed 11/11 tests because the Agent API functions, stage mapper, real polling, and slide revision action did not exist. A twelfth dirty-outline contract test was added before its implementation.
- Intermediate run: 10/12 passed; the two remaining failures were an incorrect test expectation for the existing sanitized API error and fake-timer scheduling in the polling test. The production behavior was unchanged; the tests were corrected to the established Admin error boundary and deterministic timer draining.
- GREEN: `npm.cmd --prefix admin-vue run test -- tests/pptAgent.spec.ts` passed 12/12 tests.

The narrow tests cover exact routes and request bodies, idempotency headers, no Agent mock fallback, all six stages, session/message/confirm/poll flow, cancellation, dirty-outline blocking with zero requests, and target-slide-only revision.

## Verification

- `npm.cmd --prefix admin-vue run typecheck` - PASS.
  - First run found one TypeScript narrowing error for legacy `draft` task status; the status mapper was made explicit and the rerun passed.
- `npm.cmd --prefix admin-vue run test -- tests/pptAgent.spec.ts` - PASS, 1 file / 12 tests.
- `npm.cmd --prefix admin-vue run test` - PASS, 2 files / 26 tests.
- `npm.cmd run build:admin` - PASS, 2019 modules transformed.
  - Non-blocking build warnings: Rollup removed two misplaced `PURE` annotations from `@vueuse/core`; the existing main chunk remains larger than 500 kB.
- `npm.cmd run verify:api-boundaries` - PASS.
- `git diff --check` - PASS.

## Protected surfaces

Confirmed by source diff and successful Admin build that the existing PPT route, home composer, generation configuration, history/library, outline editor, theme controls, slide preview/editor, visual controls, presentation workspace, export, share, and help entry points remain present. The change adds a small Agent session card and rewires the already-existing right-side assistant; it does not rewrite the page.

Manual browser interaction and real backend end-to-end execution were not performed in this task, so visual layout and live runtime acceptance remain unverified.

## Risks and follow-up

- Agent outline edits cannot be persisted directly until the backend provides an owner-bound Agent outline update contract. The UI fails closed rather than confirming stale data.
- The Admin flow is verified against mocked HTTP task snapshots and production compilation, not a live provider/task runner.
- Existing Rollup chunk-size warnings remain unchanged and are outside this task.

## Rollback

Revert the single Task 5 commit. No database rollback, migration, configuration change, or production action is required.
