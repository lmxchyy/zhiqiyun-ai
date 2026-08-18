# Identity Migration Number Allocation Audit

Date: 2026-08-15  
Scope: current `main`, all local branches, all registered worktrees, cached `origin/*` and `gitee/*` refs, plus uncommitted migration files in worktrees  
Mode: read-only repository audit; this report is the only workspace write

## 1. Executive Decision

| Item | Result |
| --- | --- |
| Current branch | `main` |
| `HEAD` / `main` | `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c` |
| cached `origin/main` | `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c` |
| cached `gitee/main` | `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c` |
| Main migration head | 107, `database/migrations/107-storage-multipart-upload.sql` |
| Local branches audited | 29, including `main` |
| Registered worktrees audited | 22 |
| Cached remote heads audited | 13 origin + 12 gitee; symbolic HEAD refs excluded |
| Realtime remote verification | `REMOTE_REALTIME_UNVERIFIED`; both `git ls-remote --heads` calls exited 128 |
| Invalidated Identity range | 108–112 |
| Recommended Identity range | **114–118** |
| Allocation decision | `IDENTITY MIGRATION ALLOCATION: READY` |

Identity must not use 108–112. Migration 108 is already claimed by two active workstreams:

1. committed AI Inspiration migration `108-inspiration-template-definition-expand.sql`;
2. uncommitted but implemented/test-referenced PPT v2 WIP `108-ppt-v2-durable-generation.sql`.

Numbers 109–113 have no observed file or explicit plan, but are deliberately left as a collision-resolution buffer for the two pre-existing 108 claimants. The first continuous five-number range with reasonable separation from that conflict is 114–118.

## 2. Current Main Migration Inventory

### 2.1 Complete number set

Current `main` contains 108 migration files representing 105 distinct numbers:

- present: 002–049;
- 050 appears twice;
- present: 051–055;
- 056 appears twice;
- present: 057–077;
- 078 appears twice;
- present: 079–101;
- 102 is absent;
- present: 103–107.

Considering the expected sequence 001–107, the gaps are:

| Number | Finding | Reuse decision |
| ---: | --- | --- |
| 001 | No migration file exists | Do not reuse; it is below the established history |
| 102 | No migration file exists between 101 and 103 | Do not reuse; it is a historical hole, not a safe future allocation |

### 2.2 Duplicate numbers already in main

| Number | Filenames | Status | Collision Risk |
| ---: | --- | --- | --- |
| 050 | `050-commercial-billing-wechat.sql`; `050-wechat-virtual-custom-token-unit.sql` | `OCCUPIED_MAIN` | HIGH |
| 056 | `056-connector-qr-authorization.sql`; `056-wechat-virtual-test-token-1fen.sql` | `OCCUPIED_MAIN` | HIGH |
| 078 | `078-promotion-invite-token.sql`; `078-publish-full-legal-agreements.sql` | `OCCUPIED_MAIN` | HIGH |

These duplicates are historical governance debt. This audit does not rename or repair them.

## 3. Active Local Branch Migration Inventory

“Expected to enter main” is an audit inference from an active worktree, ahead commits, current implementation/tests, and prior governance status. It is not a merge approval.

| Branch | Ahead / Behind main | Worktree | Migration | Introduced by | Domain | Classification | Active | Expected to enter main |
| --- | ---: | --- | --- | --- | --- | --- | --- | --- |
| `feature/ai-inspiration` | 5 / 42 | clean, `E:/code/work/先知AI-ai-inspiration` | `108-inspiration-template-definition-expand.sql` | `c22d958533ca17f9fe5a1070fbc130e405f5a295` | AI Inspiration template definition/backfill | `RESERVED_ACTIVE_BRANCH` | YES | YES, after migration collision governance |
| `codex/ppt-v2` | 2 / 0 | dirty, `E:/code/work/ppt-v2` | `108-ppt-v2-durable-generation.sql`, untracked WIP | no commit; created 2026-08-15 12:34 +08:00, modified 12:48 | PPT v2 durable jobs/checkpoints/fencing | `RESERVED_ACTIVE_BRANCH` | YES | YES, inferred from code and migration tests; must be committed under a non-conflicting number |
| `codex/ppt-agent-phase1-regression-integration` | 16 / 42 | clean, `E:/code/work/先知AI-ppt-agent-phase1-integration-20260806` | `106-ppt-agent-phase1.sql` | `22a923f4a1c898753125350fb8d128b4ada33cf4` | PPT Agent schema | `RESERVED_ACTIVE_BRANCH` but collides with main 106 | YES | YES only after renumbering/reconstruction on current main |
| `codex/ppt-agent-phase1-regression-integration` | 16 / 42 | same | `107-ppt-deepseek-v4-flash-billing.sql` | `29d2a8f6fe22c9c4d37ba3e2a32734599da6d057` | PPT billing configuration | `RESERVED_ACTIVE_BRANCH` but collides with main 107 | YES | YES only after renumbering/reconstruction on current main |
| `codex/identity-phase2-1-security` | 1 / 215 | clean historical worktree | `072-identity-phase2-1-security-patch.sql` | `dc7efceacd805efa73970fe5a3798697f413005c` | Legacy Identity security | `HISTORICAL_ONLY` | Reference only | NO |
| `codex/identity-phase2-2-deployment-gates` | 2 / 215 | no worktree | 072 above; `073-identity-release-readiness.sql` | 073: `867cf82e807a9ae8c5950171aed9e8985a224f37` | Legacy Identity release gates | `HISTORICAL_ONLY` | Superseded | NO |
| `codex/identity-phase2-2-release-readiness` | 2 / 215 | clean historical worktree | same 072/073 | same commits | Legacy Identity release gates | `HISTORICAL_ONLY` | Reference only | NO |
| `codex/protect-identity-phase2-2-deployment-gates-20260814` | 3 / 215 | clean historical worktree | same 072/073 | same commits | Protected legacy Identity evidence | `HISTORICAL_ONLY` | Reference only | NO |

The old Identity 072/073 filenames are different from the files occupying 072/073 in current `main`. Phase 3 already classified the old Identity branches as reference/superseded; their numbers remain historical collision evidence and must never be reused by Identity VNext.

### 3.1 Local branches with no branch-only migration file

The following local branches were inspected and contain no migration path absent from `main`:

| Domain | Branches | Result |
| --- | --- | --- |
| Invite / Operation Center | `codex/agent-invite-apk-production-final`, `codex/agent-invite-apk-release-blockers`, `codex/miniprogram-agent-invite-autobind-production`, `codex/operation-center-invite-promotion` | No new migration |
| Enterprise | `codex/enterprise-v1`, `codex/protect-enterprise-v1-20260814` | No new migration |
| Safe Area | `feature/cross-platform-safe-area` | No new migration; its two known untracked WIP files are not migrations |
| PPT | `codex/ppt-v2` ref itself has no committed migration; its worktree has the untracked 108 recorded above | Worktree-only claim recorded |
| Video / Seedance / SmartVideo | `codex/grok-imagine-1.5-video`, `codex/login-compliance-2.0.38`, `codex/protect-seedance-prod-minimal-20260805`, `codex/protect-seedance-prod-release-20260805`, `codex/protect-seedance-r8-compliant-20260806`, `codex/protect-seedance-video-artifact-fix-20260814`, `codex/protect-smartvideo-grok-backend-20260814`, `codex/protect-smartvideo-mini-2.0.42-20260814`, `codex/seedance-video-artifact-fix`, `codex/smartvideo-mini-2.0.42`, `codex/video-compliance-release-tools-20260731`, `codex/video-model-params-2.0.36`, `codex/video-ui-login-2.0.39` | No new migration |
| Production protection | `codex/protect-main-production-ops-20260814` | No new migration |

Every registered worktree’s `database/migrations` status was inspected. The PPT v2 untracked 108 file was the only dirty migration path.

## 4. Remote-Tracking Migration Inventory

### 4.1 Cached refs with migration claims

| Cached ref | HEAD | Migration claim | Relationship to local |
| --- | --- | --- | --- |
| `origin/feature/ai-inspiration` | `7ef4a4b34e89fda8ef8f1bdf3297b86355934d90` | 108 AI Inspiration | Matches local branch |
| `gitee/feature/ai-inspiration` | same | 108 AI Inspiration | Matches local branch |
| `origin/codex/ppt-agent-phase1-regression-integration` | `df7a9808741dd829b493632bb3abd5e794fd49ac` | 106 and 107 PPT | Matches local branch |
| `gitee/codex/ppt-agent-phase1-regression-integration` | same | 106 and 107 PPT | Matches local branch |

No cached origin/gitee ref contains a migration numbered 109 or higher.

### 4.2 Cached refs with no branch-only migration

- Origin: `main`, `codex/agent-invite-apk-production-readiness`, `codex/agent-invite-apk-release-blockers`, `codex/channel-ecosystem-v132-phase3`, `codex/grok-imagine-1.5-video`, `codex/operation-center-invite-promotion`, `codex/ppt-v2`, `codex/video-contract-2.0.36`, `codex/video-model-params-2.0.36`, `codex/video-ui-login-2.0.39`, `feature/cross-platform-safe-area`.
- Gitee: `main`, `codex/agent-invite-apk-production-final`, `codex/agent-invite-apk-release-blockers`, `codex/channel-ecosystem-v132-phase3`, `codex/grok-imagine-1.5-video`, `codex/miniprogram-agent-invite-autobind-production`, `codex/ppt-v2`, `codex/video-contract-2.0.36`, `codex/video-model-params-2.0.36`, `codex/video-ui-login-2.0.39`.

### 4.3 Realtime verification

Both read-only commands were attempted:

- `git ls-remote --heads origin` → exit 128
- `git ls-remote --heads gitee` → exit 128

No fetch, pull, or prune was performed.

**REMOTE_REALTIME_UNVERIFIED**

The range recommendation therefore proves absence against current local refs, every registered worktree including untracked files, and cached remote-tracking refs. It cannot prove that an uncached server-side branch was created after the last local remote update.

## 5. Collision Table

| Number | Competing files / owners | Classification | Risk | Governance conclusion |
| ---: | --- | --- | --- | --- |
| 050 | two files in main | `OCCUPIED_MAIN` | HIGH | Historical duplicate; never reuse |
| 056 | two files in main | `OCCUPIED_MAIN` | HIGH | Historical duplicate; never reuse |
| 072 | main commercial agreement vs old Identity patch | `OCCUPIED_MAIN` + `HISTORICAL_ONLY` | HIGH | Old Identity file must not migrate forward |
| 073 | main WeChat product IDs vs old Identity readiness | `OCCUPIED_MAIN` + `HISTORICAL_ONLY` | HIGH | Old Identity file must not migrate forward |
| 078 | two files in main | `OCCUPIED_MAIN` | HIGH | Historical duplicate; never reuse |
| 106 | main AI auto montage vs active PPT Phase 1 | `OCCUPIED_MAIN` + `RESERVED_ACTIVE_BRANCH` | CRITICAL | PPT work must be renumbered/reconstructed before integration |
| 107 | main storage multipart vs active PPT billing | `OCCUPIED_MAIN` + `RESERVED_ACTIVE_BRANCH` | CRITICAL | PPT work must be renumbered/reconstructed before integration |
| 108 | active AI Inspiration vs active uncommitted PPT v2 | two `RESERVED_ACTIVE_BRANCH` claims | CRITICAL | Existing branches require separate allocation decision; Identity cannot use 108 |
| 109–113 | no observed claim | unallocated buffer | MEDIUM | Leave available for resolving pre-existing 108 collision and adjacent active-branch work |
| 114–118 | no observed claim | proposed Identity reservation | LOW locally / MEDIUM with remote uncertainty | Recommended Identity range; recheck immediately before first migration creation |

## 6. Migration Registry Near Current Head

This table covers 078–120, more than 30 numbers around the current head and the proposed Identity allocation.

| Number | Filename | Branch | Domain | Status | Collision Risk |
| ---: | --- | --- | --- | --- | --- |
| 078 | `078-promotion-invite-token.sql` | main | Promotion | `OCCUPIED_MAIN` | HIGH |
| 078 | `078-publish-full-legal-agreements.sql` | main | Legal | `OCCUPIED_MAIN` | HIGH |
| 079 | `079-channel-commercial-rule-versions.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 080 | `080-channel-relationships-and-order-snapshots.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 081 | `081-channel-referral-rewards.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 082 | `082-channel-service-fee-and-adjustments.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 083 | `083-channel-v132-default-draft.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 084 | `084-channel-shadow-differences.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 085 | `085-channel-rollout-config.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 086 | `086-channel-settlement-engine-canary.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 087 | `087-channel-rollout-tenant-whitelist.sql` | main | Channel | `OCCUPIED_MAIN` | LOW |
| 088 | `088-runtime-projection-baseline-completion.sql` | main | Runtime projection | `OCCUPIED_MAIN` | LOW |
| 089 | `089-operation-center-lifecycle-refund-saga.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 090 | `090-operation-center-referral-eligibility.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 091 | `091-operation-center-referral-reward-grant.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 092 | `092-operation-center-referral-reward-release.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 093 | `093-operation-center-referral-reward-reversal.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 094 | `094-operation-center-refund-saga-orchestrator.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 095 | `095-payment-refund-adapter-query-verification.sql` | main | Payment | `OCCUPIED_MAIN` | LOW |
| 096 | `096-operation-center-refund-management.sql` | main | Operation Center | `OCCUPIED_MAIN` | LOW |
| 097 | `097-member-agent-price-plan-v2.sql` | main | Pricing | `OCCUPIED_MAIN` | LOW |
| 098 | `098-price-plan-admin-governance.sql` | main | Pricing | `OCCUPIED_MAIN` | LOW |
| 099 | `099-price-plan-default-switch.sql` | main | Pricing | `OCCUPIED_MAIN` | LOW |
| 100 | `100-price-plan-test-whitelist-audit.sql` | main | Pricing | `OCCUPIED_MAIN` | LOW |
| 101 | `101-inspiration-template-experience-config.sql` | main | AI Inspiration | `OCCUPIED_MAIN` | LOW |
| 102 | none | none | historical gap | UNALLOCATED_HISTORICAL_GAP | HIGH if reused |
| 103 | `103-personal-gift-point-expiry.sql` | main | Points | `OCCUPIED_MAIN` | LOW |
| 104 | `104-personal-gift-policy-versioning.sql` | main | Points | `OCCUPIED_MAIN` | LOW |
| 105 | `105-personal-point-legacy-reservation-attribution.sql` | main | Points | `OCCUPIED_MAIN` | LOW |
| 106 | `106-ai-auto-montage-v1.sql` | main | Video montage | `OCCUPIED_MAIN` | CRITICAL with PPT claim |
| 106 | `106-ppt-agent-phase1.sql` | PPT Phase 1 branch | PPT | `RESERVED_ACTIVE_BRANCH` | CRITICAL |
| 107 | `107-storage-multipart-upload.sql` | main | Storage | `OCCUPIED_MAIN` | CRITICAL with PPT claim |
| 107 | `107-ppt-deepseek-v4-flash-billing.sql` | PPT Phase 1 branch | PPT billing | `RESERVED_ACTIVE_BRANCH` | CRITICAL |
| 108 | `108-inspiration-template-definition-expand.sql` | AI Inspiration branch + cached remotes | AI Inspiration | `RESERVED_ACTIVE_BRANCH` | CRITICAL |
| 108 | `108-ppt-v2-durable-generation.sql` | PPT v2 worktree, untracked | PPT v2 | `RESERVED_ACTIVE_BRANCH` | CRITICAL |
| 109 | none observed | none | collision buffer | UNALLOCATED_BUFFER | MEDIUM |
| 110 | none observed | none | collision buffer | UNALLOCATED_BUFFER | MEDIUM |
| 111 | none observed | none | collision buffer | UNALLOCATED_BUFFER | MEDIUM |
| 112 | none observed | none | collision buffer | UNALLOCATED_BUFFER | MEDIUM |
| 113 | none observed | none | collision buffer | UNALLOCATED_BUFFER | MEDIUM |
| 114 | proposed `114-identity-login-lookup-indexes.sql` | future Identity rebuild | Identity login | PROPOSED_IDENTITY_RESERVATION | LOW/MEDIUM |
| 115 | proposed `115-operation-center-sensitive-profiles.sql` | future Identity rebuild | Sensitive profiles | PROPOSED_IDENTITY_RESERVATION | LOW/MEDIUM |
| 116 | proposed `116-operation-center-sensitive-plaintext-contract.sql` | future Identity rebuild | Sensitive profile contract | PROPOSED_IDENTITY_RESERVATION | HIGH operation risk, no number collision observed |
| 117 | proposed `117-commercial-account-merge-assessments.sql` | future Identity rebuild | Account merge audit | PROPOSED_IDENTITY_RESERVATION | LOW/MEDIUM |
| 118 | proposed `118-identity-worker-readiness.sql` | future Identity rebuild | Identity worker | PROPOSED_IDENTITY_RESERVATION | LOW/MEDIUM |
| 119 | none observed | none | future | FREE_OBSERVED | MEDIUM remote uncertainty |
| 120 | none observed | none | future | FREE_OBSERVED | MEDIUM remote uncertainty |

## 7. Recommended Identity Allocation

### 7.1 Range

**IDENTITY RESERVED MIGRATIONS: 114–118**

The original Phase 4 mapping should be renumbered as follows without changing its production design:

| New number | Replaces old proposal | Purpose |
| ---: | ---: | --- |
| 114 | 108 | Bounded password-login lookup indexes and duplicate-account preflight |
| 115 | 109 | Encrypted operation-center sensitive profile expand schema |
| 116 | 110 | Irreversible plaintext contract gate after expand → backfill → verify → cutover |
| 117 | 111 | Append-only commercial account merge assessment/audit evidence |
| 118 | 112 | Identity worker retry/readiness metadata |

No migration file is created by this audit. Migration 116 remains a separately approved irreversible production gate; allocating its number does not authorize its execution.

### 7.2 Why not 109–113

Although 109–113 are currently empty, using them immediately would force the unresolved AI Inspiration/PPT v2 108 collision to compete with Identity for the next number. Reserving 114–118 gives existing active branches five numbers to reconcile their already-created work without moving Identity again.

## 8. Residual Uncertainty and Controls

1. **REMOTE_REALTIME_UNVERIFIED:** cached refs may not include a newly created server-side branch. Before creating migration 114, rerun `git ls-remote` or obtain an owner-confirmed remote registry snapshot.
2. PPT v2’s 108 file is untracked. It may be renamed before commit, but its implemented schema and tests make it an explicit active reservation, not hypothetical text.
3. AI Inspiration and PPT v2 already collide at 108. This audit does not decide which branch keeps 108; that requires a separate branch migration renumbering action.
4. PPT Phase 1 already collides with main at 106/107 and must not be merged with those filenames.
5. No non-Identity active branch committed/uncommitted planning reference for 109–120 was found. The invalidated Phase 4 Identity plan references 109–112 and is intentionally replaced by this 114–118 recommendation.
6. A number allocation has no Git-native lock. Task 0 must freeze 114–118 in its contract/report and add a test that rejects unexpected files using those numbers; the check must be rerun immediately before each future migration is created.

## 9. Can Phase 5A Resume?

YES, for Task 0 contract freeze only, with these conditions:

- replace every Identity VNext 108–112 allocation reference with 114–118 in the Task 0 contract/report;
- do not create migration files during Task 0;
- keep old Identity 072/073 as reference-only evidence;
- recheck main, all worktrees, local refs, cached remotes, and preferably live remotes before the first migration file is created;
- stop again if any non-Identity claim appears in 114–118.

The next approved action is to return to Phase 5A Task 0 using 114–118. This report does not authorize Task 1 or creation of any Identity migration.

## 10. Safety Evidence

- No branch or worktree was created, changed, switched, merged, rebased, cherry-picked, deleted, pushed, fetched, or pruned.
- No migration, production code, configuration, or Phase 4 plan was modified.
- Existing Safe Area WIP was read-only and unchanged.
- The PPT v2 untracked migration was read only and remains in its original worktree.
- This Markdown audit report is the only file written by this phase.

**IDENTITY MIGRATION ALLOCATION: READY**
