# Video Backfill Runbook

<!-- markdownlint-disable MD013 -->

**Purpose:** Safely recover historical video assets whose database reference still points to a provider URL.
**Scope:** A targeted `xz_assets` recovery first; batch mode is allowed only after the targeted path is proven.
**Owner:** Production operator with an explicitly named approver.

This runbook is a production data-write procedure. It follows the approval contract in [Production Delivery Loop](../process/DELIVERY-LOOP.md) and the durable-reference rules in [Asset Lifecycle Policy](../architecture/ASSET-LIFECYCLE-POLICY.md).

## 0. Safety rules

- Start from the latest approved release on the production host. Do not run a dirty or locally modified checkout.
- Confirm the exact asset ID. Never infer an ID from a prefix or a browser card title.
- Default is dry run. A write requires the exact `-dry-run=false` command to be approved after the read-only evidence packet is complete.
- Process one asset before considering a batch.
- The backfill repairs `xz_assets` and owned object storage. It must not modify `xz_generation_tasks`, re-run model generation, or charge points.
- A provider URL is a temporary input. Success means the asset references `storage://<fileId>` and the owned object is verifiable.
- If any identity, task relation, HTTP behavior, object-storage result, or API result is ambiguous, stop and mark the operation `BLOCKED`.

## 1. Preconditions and evidence folder

Record these before touching production:

```bash
date -u
hostname
pwd
git rev-parse HEAD
git status --porcelain=v1
```

The status must be clean and the commit must match the approved deployment/release evidence. Create an evidence directory outside the repository if it may contain secrets; redact signed URLs and credentials before attaching evidence.

Required inputs:

- Issue and approval thread;
- exact `asset_id` and expected user/tenant;
- production database and object-storage access through the approved secret mechanism;
- `ffmpeg` available to create a cover image;
- API base URL and an authenticated browser/session for the owning user;
- operator and approver identities;
- stop/rollback contact.

## 2. Locate the asset

Prefer an exact, read-only query. Do not edit the row while locating it.

```sql
SELECT id,
       user_id,
       COALESCE(tenant_id, 'tenant_default') AS tenant_id,
       task_id,
       media_type,
       url,
       metadata->>'fileId' AS file_id,
       metadata->>'storageFileId' AS storage_file_id,
       metadata->>'storageManaged' AS storage_managed,
       deleted_at,
       created_at,
       updated_at
FROM xz_assets
WHERE id = '<EXACT_ASSET_ID>';
```

The candidate must be an active (`deleted_at IS NULL`) video with a non-empty non-`storage://` URL and no existing `fileId`/`storageFileId`. If it is already persisted, stop: the correct action is verification, not another write. If it is deleted, not a video, missing, or already storage-managed, record the safe skip and stop.

For a batch inventory only, use a bounded read-only query equivalent to the command's selection:

```sql
SELECT id, user_id, COALESCE(tenant_id, 'tenant_default'), task_id, url,
       metadata->>'fileId' AS file_id, created_at
FROM xz_assets
WHERE deleted_at IS NULL
  AND lower(media_type) = 'video'
  AND COALESCE(metadata->>'fileId', '') = ''
  AND COALESCE(url, '') <> ''
ORDER BY created_at ASC
LIMIT <SMALL_APPROVED_LIMIT>;
```

## 3. Verify the task/asset relationship

Read both records and compare ownership, tenant, task ID, media type, and source/result metadata. The task is evidence of provenance, not a row to rewrite.

```sql
SELECT id, user_id, tenant_id, type, model, status, raw, params, updated_at
FROM xz_generation_tasks
WHERE id = '<TASK_ID_FROM_ASSET>';
```

Confirm:

- the asset's `user_id` and tenant match the task and intended owner;
- the task is the source of the asset, not merely a similarly named task;
- the task is terminal/complete as expected for the product;
- no new generation, billing, or task-state transition is intended;
- the repair target is exactly one `xz_assets` row.

Before the write, capture a read-only baseline for the asset and task. Include a hash or a stable JSON snapshot where practical.

## 4. Dry run

Run the targeted command with its safe default made explicit:

```bash
cd backend-go
go run ./cmd/video-backfill -asset-id '<EXACT_ASSET_ID>' -dry-run=true
```

Expected outcomes include:

- `<asset>|upstream=206` (or another successful 2xx status), followed by `<asset>|dry_run_recoverable`;
- `<asset>|skipped_upstream_status=403` for an expired/unrecoverable provider URL;
- `<asset>|already_persisted`, `skipped_deleted`, `skipped_not_video`, `skipped_invalid_url`, or `not_found` for safe non-write exits.

A dry run must not call persistence, create an object, update `xz_assets`, update `xz_generation_tasks`, or change billing. Preserve stdout/stderr and the database baseline.

Do not approve a write from a dry-run status alone. Continue to the persist-equivalent read-only probe.

## 5. Persist-equivalent read-only probe

The probe must answer the question that matters: would the real persistence fetch receive usable binary bytes under the same request behavior? Probe and persistence must share the same HTTP semantics:

- same `User-Agent`;
- same `Accept-Encoding` behavior;
- same redirect policy;
- same timeout;
- same proxy and transport configuration;
- same compression behavior and response-body handling;
- same provider URL and relevant authentication/query parameters.

The probe may use a bounded read to avoid storing bytes, but it must not use a different client or silently different headers that could make the dry run pass while the real persistence download fails. If a `Range` header or a HEAD request is used for efficiency, document that it is only an additional bounded check and perform a full-response-compatible read before approval.

For the current binary-video compatibility case, the required semantics are:

```text
Accept-Encoding: identity
Transport.DisableCompression: true
```

These settings explain the observed provider binary download issue, but they are not a permanent hostname exception. The invariant is that probe and persistence use consistent binary-download request semantics.

Record:

- response status and redirect chain/policy;
- final content type, content length when present, and a bounded byte count;
- timeout/transport configuration;
- whether the body is non-empty and decodes as the expected media;
- any provider response error.

If the probe cannot be made persist-equivalent, stop at `BLOCKED`; do not use a weaker probe to authorize a stronger write.

## 6. Human approval gate

Prepare an approval packet containing:

- exact asset/task/user/tenant IDs;
- original source URL redacted or safely referenced;
- dry-run output;
- persist-equivalent probe output and HTTP semantics;
- expected object/file IDs and `storage://` references;
- expected `xz_assets` fields to change;
- explicit invariant that `xz_generation_tasks` and billing must remain unchanged;
- stop/rollback criteria;
- operator, command, release SHA, and evidence path.

Set the operational state to:

```text
READY_FOR_HUMAN_APPROVAL
```

The named approver must explicitly authorize the exact single-asset command. A passing probe, an Issue comment from automation, or an operator's assumption is not approval.

## 7. Execute one asset

After approval, run the smallest write:

```bash
cd backend-go
go run ./cmd/video-backfill -asset-id '<EXACT_ASSET_ID>' -dry-run=false
```

The current command downloads the binary, stores the private video object idempotently, extracts and stores a cover, then updates the asset to durable references such as:

```text
url            = storage://<videoFileId>
thumbnail_url  = storage://<coverFileId>
metadata.fileId / storageFileId
metadata.storageManaged = true
metadata.contentType = video/mp4
```

The update is guarded so an already persisted asset is not overwritten. If persistence fails, treat the result as a failed/partial operation: do not claim success, do not manually replace the provider URL with a `storage://` value, and inspect object/database evidence before retrying.

## 8. Verify `xz_assets`

Immediately run a read-only query for the exact asset:

```sql
SELECT id,
       task_id,
       url,
       thumbnail_url,
       media_type,
       metadata->>'fileId' AS file_id,
       metadata->>'storageFileId' AS storage_file_id,
       metadata->>'storageManaged' AS storage_managed,
       metadata->>'storageBucket' AS storage_bucket,
       metadata->>'storageObjectKey' AS storage_object_key,
       metadata->>'coverFileId' AS cover_file_id,
       metadata->>'contentType' AS content_type,
       updated_at
FROM xz_assets
WHERE id = '<EXACT_ASSET_ID>';
```

Pass criteria:

- `url` is `storage://<videoFileId>`, not the provider URL;
- `thumbnail_url` is an owned `storage://` cover reference;
- file IDs and object metadata are present and internally consistent;
- `media_type` remains video and `contentType` is appropriate;
- the row is active and belongs to the expected owner/tenant.

## 9. Verify `xz_generation_tasks` was not modified

Compare the pre-write snapshot with a post-write read. At minimum:

```sql
SELECT id, status, billing_status, points, raw, params, updated_at
FROM xz_generation_tasks
WHERE id = '<TASK_ID_FROM_ASSET>';
```

The task row must have zero changes attributable to the backfill. No task status transition, provider retry, point reservation/capture/release, or new billing event is allowed. If the task changed unexpectedly, stop and escalate before any retry.

## 10. Verify object storage

Using the approved storage/admin tooling, verify both the video and cover object:

- object exists in the expected private bucket/tenant scope;
- object key and file ID match `xz_assets.metadata`;
- object size is non-zero and within the configured limit;
- media type is `video/mp4` for the video and `image/jpeg` for the cover;
- owner/tenant and visibility are correct;
- an authorized server-side read/sign operation succeeds;
- a missing object is treated as a real storage incident, not as an expired signed URL.

Do not attach long-lived signed URLs to the Issue. Redact or record only short-lived test evidence.

## 11. Verify asset/task APIs

With the owning authenticated context:

1. Fetch the asset/task detail API and confirm a completed/available asset with a fresh signed URL.
2. Confirm the response does not expose the old provider URL as the durable reference.
3. Confirm the works/list API includes the asset and the detail API resolves the same task/asset relation.
4. Request a fresh signed URL/download and verify the expected content type and filename contract.
5. Confirm an unauthorized tenant/user cannot read or sign the object.

An HTTP 200 from an endpoint is not enough; inspect the asset availability and durable-reference fields.

## 12. Browser playback and F5 recovery

In a real browser session for the owning user:

1. Open the works list and asset detail.
2. Start playback and confirm duration, first frame, and seeking are functional.
3. Refresh with F5 and repeat the detail/playback check.
4. Navigate away and back to confirm the signed URL is re-issued rather than relying on a stale cached URL.
5. Test download/share behavior where applicable; video downloads must follow the platform's MP4/shareable contract.
6. Capture a redacted screenshot or short observation log.

A successful database update without browser playback is not `PROD_VERIFIED`.

## 13. Idempotency: second run

Run the exact same read-only or approved command again, depending on the operation policy:

```bash
cd backend-go
go run ./cmd/video-backfill -asset-id '<EXACT_ASSET_ID>' -dry-run=true
```

Expected result is `already_persisted` without a provider probe or new object. If policy requires a non-dry second-run test in a controlled window, it must still produce no duplicate object and no second database mutation. Compare object counts/file IDs and the `xz_assets.updated_at`/content before and after.

## 14. Close evidence and Issue

Attach a concise evidence index:

- Issue and approval URL;
- release/commit SHA and clean-tree proof;
- exact target asset/task/user/tenant;
- dry-run output;
- persist-equivalent probe semantics and result;
- approval identity/time/command;
- write output;
- `xz_assets` before/after evidence;
- unchanged `xz_generation_tasks` and billing evidence;
- object-storage checks;
- API response checks;
- browser playback and F5 evidence;
- second-run idempotency evidence;
- residual risks or assets classified `EXPIRED`.

Only after all checks pass may the operation be recorded as `PROD_VERIFIED` and the Issue closed. Expired historical provider URLs should be classified explicitly as `EXPIRED` and offered a product recovery path; they must not be silently reported as generating.

## 15. Stop conditions

Stop immediately and mark `BLOCKED` if:

- the asset/task/tenant relationship is ambiguous;
- the source returns non-2xx, redirects unexpectedly, or cannot be probed with persistence-equivalent semantics;
- downloaded bytes are empty, malformed, over the configured bound, or the cover cannot be produced;
- persistence returns an error, zero/ambiguous rows affected, or inconsistent file IDs;
- object storage is missing or ownership/visibility is wrong;
- `xz_generation_tasks`, billing, or points changed;
- the API or browser still shows a stale/black/`00:00` player after refresh;
- a second run creates a duplicate or calls the provider again unexpectedly;
- approval, credentials, release identity, or evidence is missing.

Never “fix” a stop condition by writing the Provider URL back as a successful durable asset.
