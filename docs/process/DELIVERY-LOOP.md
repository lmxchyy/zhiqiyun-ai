# Production Delivery Loop

<!-- markdownlint-disable MD013 -->

**Status:** Normative engineering standard
**Scope:** Every production Issue, including code changes, configuration changes, operational scripts, and production data write operations.

This document turns the lessons from [Issue #133](https://github.com/lmxchyy/zhiqiyun-ai/issues/133) and [Issue #135](https://github.com/lmxchyy/zhiqiyun-ai/issues/135) into a repeatable delivery protocol. It is not a project diary and it does not replace the repository's release scripts or CI policy.

## 1. Non-negotiable rules

1. **GitHub and production are the facts of record.** Pull requests, CI checks, merge commits, release manifests, deployment output, database evidence, and production observations are authoritative. A local `state.json`, task note, terminal transcript, or agent claim is only supporting evidence.
2. **`main` is protected.** Work starts from the latest `origin/main` in a dedicated branch/worktree. Direct edits, direct pushes, and local commits on `main` are not an accepted delivery path.
3. **CI cannot be bypassed.** A merge requires the required checks to pass on the PR commit. A green local command does not substitute for GitHub CI.
4. **Production deploys use the supported entry point.** The production path remains `./deploy.sh`; a dirty tree, untracked release input, or an image that cannot be traced to the reviewed commit must stop the release.
5. **Immutable images are traceable.** Where immutable release mode is enabled, record the exact commit, registry, image digest, release manifest, and running container image identity. A tag alone is not sufficient.
6. **Deploy success is not business success.** Process health, HTTP health, smoke checks, and business verification are separate gates. The final gate is production business verification.
7. **Production data writes require a human gate.** No write follows a dry run automatically. The operator must show a persist-equivalent read-only probe, obtain explicit approval, execute the smallest safe write, and record post-write evidence.

## 2. Default delivery paths

### 2.1 Ordinary code or configuration change

```text
ISSUE_OPEN
  -> WORKTREE_READY
  -> IMPLEMENTING
  -> LOCAL_VALIDATED
  -> COMMITTED
  -> PUSHED
  -> PR_OPEN
  -> CI_RUNNING
  -> CI_PASSED
  -> REVIEWED
  -> MERGED
  -> DEPLOYING
  -> DEPLOYED
  -> SMOKE_PASSED
  -> PROD_VERIFIED
  -> ISSUE_CLOSED
```

### 2.2 Production data write

A data write uses the same issue/branch/evidence discipline, but adds a mandatory approval sequence before the write:

```text
...
-> CI_PASSED
-> REVIEWED
-> DRY_RUN
-> PERSIST_EQUIVALENT_READONLY_PROBE
-> READY_FOR_HUMAN_APPROVAL
-> PRODUCTION_WRITE
-> DATA_VERIFIED
-> PROD_VERIFIED
-> ISSUE_CLOSED
```

`READY_FOR_HUMAN_APPROVAL` is a hard stop. Automation may prepare evidence and pause; it may not infer approval from a green dry run, a local flag, or a comment written by the same automation.

## 3. State contract

Every state has four obligations: entry conditions, allowed actions, required evidence, and an exit gate. A failed gate moves the work to `BLOCKED` rather than silently advancing it.

### `ISSUE_OPEN`

- **Entry conditions:** The Issue states the problem, scope, acceptance criteria, risk, and whether it can write production data.
- **Allowed actions:** Clarify scope; link protected surfaces; identify owners and required reviewers; choose validation commands.
- **Required evidence:** Issue URL, acceptance criteria, affected systems, rollback/stop conditions, and requested production window if applicable.
- **Exit gate:** Scope is actionable and an owner is assigned.
- **Failure behavior:** Keep open and mark `BLOCKED` if requirements, access, or ownership are missing.

### `WORKTREE_READY`

- **Entry conditions:** Latest `origin/main` has been fetched and the worktree is clean.
- **Allowed actions:** Create a dedicated branch/worktree; inspect repository instructions and relevant protected surfaces.
- **Required evidence:** Branch name, worktree path, base commit, and clean `git status --short` output.
- **Exit gate:** The branch is based on the recorded main commit and no unrelated changes are present.
- **Failure behavior:** Stop; do not edit or deploy from a dirty or incorrectly based worktree.

### `IMPLEMENTING`

- **Entry conditions:** Scope and design are approved for implementation.
- **Allowed actions:** Make the smallest scoped change; add or update tests and documentation required by the Issue.
- **Required evidence:** Changed-file list, design notes where needed, and links to the acceptance criteria addressed.
- **Exit gate:** The requested change exists without unrelated refactoring or protected-surface regression.
- **Failure behavior:** Revert out-of-scope edits or return to `BLOCKED` for a design decision.

### `LOCAL_VALIDATED`

- **Entry conditions:** Implementation is complete in the worktree.
- **Allowed actions:** Run targeted tests, static checks, Markdown/link/spec checks, format checks, and `git diff --check`.
- **Required evidence:** Exact commands, exit status, relevant output, and known limitations.
- **Exit gate:** Required local checks pass and the diff is reviewable.
- **Failure behavior:** Fix and rerun; do not commit a known failing change.

### `COMMITTED`

- **Entry conditions:** Local validation passed and the diff contains only scoped changes.
- **Allowed actions:** Create a descriptive commit; inspect the commit and its parent.
- **Required evidence:** Commit SHA, subject, `git show --stat`, and clean/expected worktree status.
- **Exit gate:** The commit is reproducible from the intended branch and contains no secrets or generated release residue.
- **Failure behavior:** Amend or create a corrective commit before pushing.

### `PUSHED`

- **Entry conditions:** A valid commit exists on the dedicated branch.
- **Allowed actions:** Push the branch to GitHub; do not force-update shared branches without an approved recovery procedure.
- **Required evidence:** Remote branch URL and pushed SHA.
- **Exit gate:** GitHub shows the pushed commit and the PR can be opened from it.
- **Failure behavior:** Resolve divergence explicitly; never pretend a local push succeeded.

### `PR_OPEN`

- **Entry conditions:** The pushed branch is visible on GitHub.
- **Allowed actions:** Open a PR linked to the Issue; fill scope, validation, risk, rollback, and deployment notes.
- **Required evidence:** PR URL, base/head branches, linked Issue, changed-file summary, and test commands.
- **Exit gate:** Required reviewers/checks are configured and the PR is not draft unless intentionally so.
- **Failure behavior:** Keep the PR open and correct metadata; do not merge an unlinked or unexplained change.

### `CI_RUNNING`

- **Entry conditions:** GitHub has accepted the PR commit and started required checks.
- **Allowed actions:** Monitor checks; investigate logs; push fixes only through the branch.
- **Required evidence:** Check run names, commit SHA, start/completion status, and any retry reason.
- **Exit gate:** All required checks have reached a terminal result.
- **Failure behavior:** A failed or cancelled required check returns to `IMPLEMENTING`/`LOCAL_VALIDATED`; infrastructure failure is `BLOCKED` until rerun or acknowledged by the owner.

### `CI_PASSED`

- **Entry conditions:** Required CI checks are green on the exact PR head.
- **Allowed actions:** Prepare review and, if relevant, the release/deploy plan.
- **Required evidence:** GitHub check URLs and exact green head SHA.
- **Exit gate:** No required check is pending, skipped unexpectedly, or green on a different commit.
- **Failure behavior:** Any new push invalidates this state and returns to `CI_RUNNING`.

### `REVIEWED`

- **Entry conditions:** CI is green and the requested reviewers have inspected the diff.
- **Allowed actions:** Resolve review comments; update evidence; request re-review after material changes.
- **Required evidence:** Approved review(s), resolved comments, protected-surface check, and residual-risk statement.
- **Exit gate:** Reviewers explicitly accept the current head and the merge policy permits merge.
- **Failure behavior:** Address findings and rerun affected validation; unresolved blocking feedback is `BLOCKED`.

### `MERGED`

- **Entry conditions:** The PR is approved and required checks are green.
- **Allowed actions:** Merge through GitHub's configured method; record the merge commit.
- **Required evidence:** PR merge URL, merge SHA, merged-at time, and resulting `main` SHA.
- **Exit gate:** The merge commit is present on the intended protected branch.
- **Failure behavior:** If merge state is uncertain, verify GitHub before any deployment. Do not deploy a local equivalent commit.

### `DEPLOYING`

- **Entry conditions:** The intended merge SHA is on `main`, the production window is open, and the release input is clean and traceable.
- **Allowed actions:** Follow the supported deployment procedure; apply migrations only through the approved release path; observe logs and health checks.
- **Required evidence:** Deploy command, commit/release manifest, image reference and digest, migration result, and deployment log location.
- **Exit gate:** The deployment process reports success and the expected image/runtime is running.
- **Failure behavior:** Stop rollout or execute the documented rollback. Never hide a partial migration or replace the image manually without recording it.

### `DEPLOYED`

- **Entry conditions:** Deployment completed and runtime identity matches the reviewed release.
- **Allowed actions:** Run smoke checks and inspect runtime metrics/logs.
- **Required evidence:** Running service IDs, image `Config.Image`/digest evidence where applicable, health endpoints, and migration completion.
- **Exit gate:** Infrastructure and service health are stable for the agreed observation window.
- **Failure behavior:** `BLOCKED` or rollback; a healthy process with failed business behavior is not a successful delivery.

### `SMOKE_PASSED`

- **Entry conditions:** Deployment health is stable.
- **Allowed actions:** Run the smallest safe endpoint/UI smoke path, including auth/tenant context when relevant.
- **Required evidence:** Smoke commands, timestamps, response/assertion summary, and screenshots or logs when UI behavior is involved.
- **Exit gate:** The changed surface responds correctly without an immediate error or regression.
- **Failure behavior:** Stop and investigate; do not close the Issue based only on `/health`.

### `PROD_VERIFIED`

- **Entry conditions:** Smoke passed and the production business path is available for verification.
- **Allowed actions:** Verify the acceptance criteria against production facts; for data writes, verify both the changed row/object and untouched invariants.
- **Required evidence:** Business-level request/response or UI evidence, database/object checks, and comparison to the pre-change baseline.
- **Exit gate:** Every acceptance criterion is evidenced in production or explicitly waived by the owner.
- **Failure behavior:** Keep the Issue open; mark `BLOCKED` if external access or a safe verification window is unavailable.

### `ISSUE_CLOSED`

- **Entry conditions:** Production verification passed, evidence is attached, and follow-up risks have owners.
- **Allowed actions:** Close the Issue; link the PR, merge SHA, deploy evidence, and runbook/architecture changes.
- **Required evidence:** Final checklist, links to all facts of record, and any rollback/follow-up ticket.
- **Exit gate:** The Issue is closed on GitHub only after the final evidence is published.
- **Failure behavior:** Reopen or keep open; do not use local notes as a substitute for closure evidence.

## 4. `BLOCKED` state

`BLOCKED` is a first-class state, not a failure hidden in prose. Use it for missing approval, unavailable credentials/access, unclear requirements, failed CI, unsafe production conditions, provider/object-storage uncertainty, or an unverified deployment identity.

A blocked record must contain:

- the state and exact gate that failed;
- the last known good commit/release/data observation;
- the evidence already collected;
- the single next action and its owner;
- whether production writes/deploys are prohibited while blocked.

Automation may retry an idempotent observation or notify an owner. It must not broaden scope, bypass a gate, write production data, or close the Issue while blocked.

## 5. Automation versus human gates

| Step | Automation may do | Human gate required |
| --- | --- | --- |
| Issue/worktree | Fetch, branch, status, static inspection | Scope, risk, owner, production impact |
| Implementation | Run formatters and targeted checks | Review design and protected-surface impact |
| PR/CI | Open PR, run checks, report failures | Review and merge approval |
| Deploy | Validate clean tree, manifest, digest, health | Authorize production window and rollback decision |
| Smoke | Run deterministic health/endpoint checks | Interpret business risk when checks are ambiguous |
| Data write | Dry run, persist-equivalent read-only probe, generate evidence | Explicit `READY_FOR_HUMAN_APPROVAL` decision |
| Post-write | Verify rows, objects, APIs, and idempotency | Accept business result or stop/rollback |
| Close | Assemble links/checklist | Confirm `PROD_VERIFIED` and close Issue |

## 6. Data-write approval contract

A production data write is not ready merely because `--dry-run` found a candidate. The evidence packet must include:

1. exact asset/row/task identifiers and relationship checks;
2. dry-run output showing the candidate and why it is recoverable;
3. a persist-equivalent read-only probe using the same request semantics and timeout/transport behavior as the real write;
4. expected database, object-storage, API, and client effects;
5. explicit no-touch invariants, including tables/tasks that must remain unchanged;
6. rollback or stop criteria;
7. approver identity, timestamp, and the exact command/flags authorized.

The write must be the smallest possible scope, preferably one asset first. Afterward verify the write, the object, the business API, browser behavior, and a second idempotency run before declaring success.

## 7. Reusable lessons from Issues #133 and #135

- **#133 — history URL loss:** A compact payload and a destructive client merge can turn a successful video with a missing URL into a false “generating” state. Transport compaction must never erase a valid asset reference, and a successful task must be verified at the business/UI level rather than inferred from HTTP success.
- **#135 — targeted backfill:** Historical recovery must be narrow, dry-run first, explicitly related to its task, guarded by an idempotent asset update, and proven not to mutate generation-task rows. A batch tool that supports a precise `asset-id` target is safer than an unbounded production scan.
- **Combined policy:** Provider responses are input evidence, not durable asset state. Persist to owned storage, keep the database reference durable, issue short-lived access URLs, and verify the whole business path after deployment or repair.

## 8. Delivery close checklist

- [ ] Issue, PR, merge SHA, deploy evidence, and production verification are linked.
- [ ] `main` and production facts were checked on GitHub/production, not inferred from local state.
- [ ] Required CI passed on the exact merged head.
- [ ] Immutable image digest and release manifest are recorded when applicable.
- [ ] Smoke and business verification both passed.
- [ ] For a data write: dry run, persist-equivalent read-only probe, explicit human approval, smallest write, post-write verification, and second idempotency run are attached.
- [ ] Residual risk and follow-up work have owners.
- [ ] Only then is the Issue closed.

## Related standards

- [Immutable Image Release](../architecture/immutable-image-release.md)
- [Production Contract CI](../architecture/production-contract-ci.md)
- [Asset Lifecycle Policy](../architecture/ASSET-LIFECYCLE-POLICY.md)
- [Video Backfill Runbook](../runbooks/VIDEO-BACKFILL-RUNBOOK.md)
