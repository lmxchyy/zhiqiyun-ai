# WORKFLOW: 企业中心生命周期与上下文切换

**Version**: 0.1  
**Date**: 2026-07-15  
**Author**: Workflow Architect  
**Status**: Draft  
**Implements**: 企业创建、邀请/申请加入、成员与组织治理、认证提交、企业上下文切换和服务状态门禁

## Overview

企业中心位于“我的”页面栈内。用户先读取可用的个人/企业/智能体/运营上下文；没有企业时进入创建引导，有多个企业时选择并切换。创建企业会在一个事务内建立 tenant、根组织、Owner 成员/角色、钱包、订阅、邀请能力、当前上下文和审计。邀请接受、加入审批和成员角色变更也以事务为主；部分组织 CRUD 使用业务写入后再直接写审计，存在“业务成功但审计失败”的原子性缺口。企业服务生命周期可阻断模型调用。

## Evidence Map

| Surface | Current source |
|---|---|
| Routes | `server.go`: enterprise contexts/create/overview/members/invites/join/org/roles/billing/compute/certification/audit |
| Store | `enterprise_store.go` and related enterprise API/admin files |
| Context/RBAC | enterprise context resolver, roles, data scopes, governance middleware |
| User client/UI | `apps/user-uni/src/features/enterprise/*`, `EnterpriseEntryPage.vue` |
| Admin | `admin_enterprise_*.go`, admin enterprise pages/API |
| Schema | migrations `040`-`044` |
| Tests | enterprise lifecycle, P0 tenant isolation, concurrent debit, audit/ledger, disabled-tenant tests |

## Actors and prerequisites

| Actor | Role |
|---|---|
| Authenticated user | Creates/joins/switches enterprise and performs permitted operations. |
| Enterprise Owner/Admin | Invites, approves joins, manages members/orgs/roles. |
| Go API/store | Enforces context, RBAC, data scope and transactional mutations. |
| Admin operator | Reviews/manages enterprise service and certification state. |
| PostgreSQL | System of record for tenant, org, member, role, wallet, subscription and audit. |

Prerequisites: authenticated account; current-context middleware; migrations `040`-`044`; server-side permissions. Data scopes are `TENANT_ALL`, `ORG_AND_CHILDREN`, `ORG_SELF`, `OWNER`, `SELF`; contexts are `PERSONAL`, `ENTERPRISE`, `AGENT`, `OPERATION` and must not be conflated.

## Trigger

- Entry: get available/current contexts from the enterprise entry page.
- Onboarding: create enterprise, accept invitation, or submit join request.
- Governance: member/org/role/certification actions.
- Runtime: switch context before enterprise-scoped business calls.

## Workflow Tree

### STEP 1: Discover enterprise contexts and choose entry state

**Actor/Action**: UI loads available contexts/current context, maps membership status, then routes to onboarding, single-enterprise overview, multi-enterprise switcher or inactive-member state.  
**Timeout**: shared mini-program API timeout 600s; each PostgreSQL call uses a 5s store timeout.  
**Success**: an explicit personal/enterprise context choice -> STEP 2, or onboarding -> STEP 3/4.  
**Failures**: auth expired, DB timeout, stale local cached context, member disabled/removed, tenant unavailable.  
**Recovery**: re-login; invalidate local enterprise cache and reload from server; choose personal context; contact Owner/Admin for membership state.  
**Observable state**: current-context row/server response, Pinia/local cache, UI onboarding/switcher/status page; request logs.  
**Test cases**: zero/one/multiple enterprises; stale cache; disabled/removed member; personal vs enterprise vs agent/operation separation; auth expiry.

### STEP 2: Switch current context

**Actor/Action**: server validates that the user owns/is active in the target context, persists current context, and client resets enterprise-scoped caches before loading overview.  
**Timeout**: 5s DB timeout; client request limit 600s.  
**Success**: subsequent API calls resolve the selected tenant/org/role/data scope -> STEP 5.  
**Failures**: target missing, inactive membership, forbidden context type, tenant/service disabled, DB failure; response loss after commit can leave server and client caches different.  
**Recovery**: refetch current context; clear dependent caches; fall back to personal; never trust a client-provided tenant id without server resolution.  
**Observable state**: persisted current-context record, response context, UI header/enterprise overview, audit/request logs.  
**Test cases**: authorized/unauthorized switch; response loss; simultaneous devices; switch during another request; tenant isolation after switch; cache reset.

### STEP 3: Create an enterprise

**Actor/Action**: transaction creates tenant, root organization, Owner member and role assignment, wallet, trial/subscription defaults, invitation capability, current context and audit.  
**Timeout**: 5s DB transaction timeout.  
**Success**: active Owner context with initialized enterprise resources -> STEP 5.  
**Failures**: invalid/duplicate name or identifier, quota/policy rejection, any insert/constraint/audit error, deadlock/timeout.  
**Recovery**: whole transaction rolls back; correct input and retry. If response is lost, reload contexts before creating again.  
**Observable state**: tenant/root-org/member/role/wallet/subscription/current-context/audit rows appear atomically; UI opens enterprise overview.  
**Test cases**: complete happy path; failure at each insert; response loss/duplicate retry; owner permissions; wallet/subscription defaults; audit presence.

### STEP 4: Join through invitation or request approval

**Actor/Action**: invitation acceptance validates/locks pending invitation and transactionally creates/activates member, roles, accepted state, current context and audit. Alternatively user creates a pending join request; authorized reviewer approves/rejects, with approval creating member/roles.  
**Timeout**: 5s DB transaction timeout; invitations have persisted expiry but no continuous scheduler was found.  
**Success**: invite=`ACCEPTED` or request=`APPROVED`, member=`ACTIVE` -> STEP 2/5; rejection is terminal without membership.  
**Failures**: invalid/expired/already-used token, duplicate pending request/member, reviewer forbidden, concurrent approve/reject, role/org invalid, DB rollback.  
**Recovery**: request a new invite; reload latest request; repeat idempotently where terminal guards allow; Owner corrects org/role and re-invites.  
**Observable state**: invitation/join-request/member/role/current-context/audit rows and status pages; no queue/worker state.  
**Test cases**: accept/expire/reuse invite; concurrent accept; approve/reject race; duplicate join request; cross-tenant reviewer denial; atomic member+role+audit.

### STEP 5: Load overview and enforce service/RBAC/data scope

**Actor/Action**: middleware/store resolves active tenant, member, roles, organization and data scope; overview/billing/compute/model calls enforce permissions and enterprise service lifecycle.  
**Timeout**: 5s per store operation; downstream model/generation timeout belongs to the invoked workflow.  
**Success**: only permitted tenant/org/owner/self data is returned or mutated -> STEP 6/7/8.  
**Failures**: missing permission, out-of-scope org/resource, disabled member, service `PAUSED/DISABLED/TERMINATED`, inconsistent current context.  
**Recovery**: switch context; Owner updates role/member; admin restores service where policy permits; return 403 rather than hiding missing server capability with client state.  
**Observable state**: resolved context and request result, service lifecycle, permission/data-scope denial logs, audit records for mutations.  
**Test cases**: every data scope; horizontal/cross-tenant access; disabled member; disabled service blocks model call; personal resources remain isolated.

### STEP 6: Manage members and roles

**Actor/Action**: authorized Owner/Admin updates member profile/status/org and transactionally disables/reinserts role assignments with audit; disable/remove paths update member and role records.  
**Timeout**: 5s DB transaction timeout.  
**Success**: effective permissions change consistently; removed/disabled user loses enterprise access.  
**Failures**: removing last Owner/self-lockout, invalid role/org, concurrent updates, forbidden data scope, audit/DB error.  
**Recovery**: transaction rollback; reload and retry; enforce last-Owner invariant and break-glass admin policy before production.  
**Observable state**: member `ACTIVE/DISABLED/REMOVED`, role assignments/enabled flags and audit; affected client learns on next context/permission request.  
**Test cases**: role replacement atomicity; disable/remove; last Owner; concurrent edits; access revoked immediately; audit immutability.

### STEP 7: Manage organization hierarchy

**Actor/Action**: authorized user creates, renames, moves or deletes an organization after tenant/data-scope/hierarchy checks.  
**Timeout**: 5s DB operations; no separate hierarchy-operation SLA.  
**Success**: org tree and member scopes reflect the mutation.  
**Failures**: cycle/root move, non-empty delete, cross-tenant parent, stale hierarchy, DB write failure, or audit insert failure after business mutation.  
**Recovery**: refetch tree and correct target; retry failed mutation; if audit alone fails, operator must reconcile because some paths do not wrap mutation and direct audit insert in one transaction.  
**Observable state**: organization active/deleted fields and tree response; direct audit record may be missing in the partial-failure case.  
**Test cases**: create/update/move/delete; cycle/root/non-empty cases; cross-tenant parent; mutation-success/audit-failure injection; concurrent moves.

### STEP 8: Submit and review enterprise certification

**Actor/Action**: enterprise user submits certification; transaction creates one pending record under a unique pending constraint. Admin-side APIs expose review/state actions.  
**Timeout**: submit/review DB operations use 5s timeout; no review SLA/escalation scheduler found.  
**Success**: certification moves from `PENDING` to an admin-reviewed terminal state according to implementation/policy.  
**Failures**: duplicate pending submission, invalid evidence, forbidden reviewer, storage/reference problem, DB/audit failure, indefinitely pending review.  
**Recovery**: return existing pending item; correct evidence after rejection; admin queue/manual follow-up. User-side end-to-end review notification is not implemented as a distinct workflow.  
**Observable state**: certification status and admin list/detail; unique pending DB constraint; audit/request logs; no SLA metric.  
**Test cases**: first/duplicate submit; tenant isolation; approve/reject permissions; audit; pending aging; user-visible refreshed status.

## State Transitions

| Entity | Transition |
|---|---|
| Context | personal/current -> authorized enterprise/agent/operation context |
| Member | invitation/approval -> `ACTIVE -> DISABLED/REMOVED` |
| Invitation | `PENDING -> ACCEPTED/EXPIRED` |
| Join request | `PENDING -> APPROVED/REJECTED` |
| Organization | active hierarchy -> updated/moved/soft-deleted per constraints |
| Certification | `PENDING -> reviewed terminal state` |
| Service lifecycle | `PROVISIONING -> ACTIVE -> PAUSED/DISABLED/TERMINATED` as administered |

## Handoffs and Cleanup

- Context switch must invalidate tenant/org/role/billing/compute caches before any new request.
- Membership disable/remove must invalidate access even if a client retains cached UI data.
- Invitation expiry is checked from persisted state; no durable expiry worker was found.
- Removing a member disables role assignments but must preserve audit and financial ledgers.
- Tenant termination/data deletion has no complete workflow in this scope and must not be inferred from service status alone.

## Reality Checker

- Enterprise is integrated into the existing user page stack and server RBAC; it is not a separate mock admin.
- Context types and data scopes are explicit, but cached client state can lag server authority.
- Create/invite/join/member paths are largely transactional; some organization mutations and audit writes are not atomic.
- Certification submission exists, while review SLA, notification and full user-visible lifecycle are incomplete.
- Service lifecycle can block model calls; every other enterprise-scoped resource path still requires isolation tests.
- No durable invitation expiry, certification escalation or tenant deprovisioning worker was found.

## Test Cases

1. Zero/one/multiple-enterprise entry routing and cache invalidation.
2. Atomic enterprise bootstrap with Owner/root org/wallet/subscription/audit.
3. Invitation accept/expiry/reuse/concurrency.
4. Join request approve/reject race and tenant isolation.
5. Context switch authorization and simultaneous-device behavior.
6. All RBAC data scopes against member/org/resources.
7. Member role update/disable/remove and last-Owner safety.
8. Org cycle/root/non-empty/cross-tenant checks.
9. Inject audit failure after organization mutation and detect inconsistency.
10. Certification duplicate pending and review permissions.
11. Disabled service/member blocks downstream model/generation calls.
12. Immutable audit and financial ledgers across membership removal.

## Assumptions and Open Questions

- What is the authoritative last-Owner/break-glass policy?
- Which organization mutations must be refactored into one business+audit transaction?
- What are certification review states, SLA, notification and resubmission rules?
- What is the required tenant termination, export, retention and data-erasure workflow?
