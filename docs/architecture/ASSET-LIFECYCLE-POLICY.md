# Asset Lifecycle Policy

<!-- markdownlint-disable MD013 -->

**Status:** Normative architecture policy
**Scope:** AI image, AI video, smart video montage, PPTX, rendered artifacts, thumbnails, and any future generated binary asset.

## 1. Durable reference rule

A Provider URL is a **temporary transport artifact**, not an asset reference.

```text
Provider Temporary Artifact
        -> Validation
        -> Internal Persistence
        -> storage://<fileId>
        -> Short-lived Signed URL
        -> Client
        -> Refresh / Re-sign
```

The only durable application reference for an owned object is `storage://<fileId>` (or the equivalent internal storage reference defined by the file center). A value such as:

```text
https://provider.example/xxx.mp4
```

may be retained as diagnostic metadata with an expiry classification, but it must not be treated as the final successful asset state in `xz_assets`, task output, or client cache.

## 2. Why this boundary exists

Provider URLs can be signed, rate-limited, compressed differently by request headers, revoked, or garbage-collected without notice. They are useful for transporting a generated result to the platform, but the provider does not own the product's availability contract. The platform owns the asset only after it has validated and persisted the bytes to its private object storage/file center.

A successful provider response therefore does **not** by itself mean that an asset is durably available. Persistence failure must be visible as failure or retryable work; it must never be represented as `storageManaged=true`, a `storage://` reference, or a completed client-visible asset.

## 3. Lifecycle states

| State | Meaning | Client/API behavior |
| --- | --- | --- |
| `PROVIDER_TEMPORARY` | Provider URL received; bytes not yet owned by the platform | Internal only; do not expose as a durable success reference |
| `VALIDATING` | URL, response, media type, size, and safety checks are running | Task remains in its processing state |
| `PERSISTING` | Bytes are being streamed to private object storage | Do not publish a completed asset before the write is committed |
| `READY` | Object exists, metadata is committed, and the durable reference is `storage://<fileId>` | API dynamically signs a short-lived URL |
| `EXPIRED` | A historical transient asset's provider URL is no longer usable and no owned object exists | Return an explicit unavailable state and recovery guidance; never show “generating” |
| `PERSISTENCE_FAILED` | Provider result was usable or attempted, but owned persistence did not complete | Surface failure/retry state; do not claim successful persistence |
| `DELETED` | Product-level logical deletion or retention removal | Do not sign or return it to active clients |

`EXPIRED` describes historical transient assets. An expired signed URL for an owned object is **not** an expired asset: re-signing the same `fileId` should restore access. The real storage anomaly is a missing, unreadable, or inconsistent object.

## 4. Canonical data invariants

1. A ready asset has a durable `storage://<fileId>` reference and metadata that identifies the owned object/file record.
2. A provider URL may be stored only as provenance/diagnostic data with an explicit transient or expiry meaning; it is never the long-term `url` success contract.
3. `storageManaged` (or an equivalent flag) is true only after the object write and metadata commit succeed.
4. `fileId`, object key, tenant/user ownership, media type, and visibility must agree. A reference from another tenant or user is an authorization failure, not a candidate for signing.
5. Object persistence is idempotent. Retries reuse the same business identity/idempotency key and do not create duplicate files or bill the user again.
6. Database updates must be guarded so a completed `fileId` is not replaced by an upstream URL or a second untracked object.
7. A persistence error is observable and retryable where safe; it must not be swallowed or converted into a false success response.
8. Client code must not interpret a missing URL as `generating`. Status, availability, and error/recovery fields are separate concerns.
9. URLs returned to browsers or mini-program clients are short-lived signed URLs. The client may request a refresh; it must not persist or promote the signed URL to a durable application reference.
10. Access to a private object is authorized from the authenticated tenant/user and the durable file record, never from a client-supplied Provider URL.

## 5. Lifecycle by artifact type

### 5.1 AI image

```text
model image URL / bytes
  -> validate content and size
  -> stream to private object storage
  -> persist file metadata and asset record
  -> store storage://<fileId>
  -> sign on read
```

Thumbnails and source/reference images follow the same rule. Inline data URLs may be accepted at an ingestion boundary, but they must not become long-lived asset references.

### 5.2 AI video

```text
provider video URL
  -> provider response validation
  -> stream/download with bounded size and consistent request semantics
  -> persist video object and cover/thumbnail
  -> record file IDs and media type
  -> store storage://<videoFileId> and storage://<coverFileId>
  -> sign on API response/download
```

The product must normalize the client/download contract as required by the target platform (for example, a shareable MP4 filename and content type). A provider `.m4v` name is not a reason to leak the provider URL or an unnormalized download.

### 5.3 Smart video montage

The ExportService/worker path must publish the completed render through the existing work/asset/file center and billing/outbox lifecycle. The rendered video, cover, and any manifest are owned artifacts; Provider/planner/speech/render URLs are transport inputs. The final work references `storage://<fileId>` and the client receives a fresh signed URL.

A render task is not complete merely because FFmpeg or a worker produced a local file. Object persistence, asset publication, point settlement, and idempotent retry behavior must all satisfy the task's completion contract.

### 5.4 PPTX and rendered artifacts

PPTX, PDF, preview images, and rendered slide/video outputs enter the private file center before being exposed through the works/task API:

```text
render/export output
  -> file object persistence
  -> private visibility + ownership metadata
  -> storage://<fileId> in task/asset state
  -> short-lived signed download/preview URL
```

A task field such as `pptUrl`, `downloadUrl`, or `previewUrl` may contain a temporary signed URL only at the response boundary. Stored task/raw state must retain the durable reference instead.

## 6. Signed URL contract

- Signing occurs on the server when an authorized client reads an asset or requests a download.
- The URL has a bounded TTL and the response may expose its expiry if the client needs proactive refresh.
- `401/403` caused by URL expiry should trigger a refresh/re-sign path when the durable object still exists.
- A signed URL disappearing from a cached payload is not evidence that generation is still running.
- `EXPIRED` is appropriate when no owned object exists and the historical transient source is no longer recoverable.
- Object-not-found, checksum/size mismatch, ownership mismatch, or failed signing is a storage/authorization incident and must be observed separately from provider URL expiry.

## 7. Persistence and retry contract

### Before persistence

- Confirm the provider response is an expected media type and within size/time limits.
- Use the same HTTP request semantics for any probe and the actual persistence fetch: User-Agent, `Accept-Encoding`, redirect policy, timeout, proxy/transport behavior, and compression handling.
- Keep the provider URL out of success state until the owned object is committed.

### During persistence

- Stream bytes where supported; avoid unbounded buffering.
- Use a deterministic business identity/idempotency key.
- Write private object metadata with tenant, user, media type, business type, and source task identifiers.
- Do not mark the asset ready before the object and database/file-center commit are both successful.

### After persistence

- Verify the returned file ID/object key and non-zero object size.
- Commit `storage://<fileId>` and associated metadata atomically or with a recoverable outbox/repair record.
- On retry, detect the existing object/file record and reuse it; do not re-charge or publish duplicates.
- If the database update affects zero rows because another worker already persisted the asset, verify and reuse the existing durable reference rather than overwriting it.

## 8. Client contract

Clients consume an asset view, not storage internals. A response should distinguish:

- task state (`PENDING`, `PROCESSING`, `SUCCEEDED`, `FAILED`);
- asset availability (`READY`, `EXPIRED`, `PERSISTENCE_FAILED`, `DELETED`);
- a short-lived `url`/signed URL when authorized;
- a refresh/recovery action when the URL is absent or expired.

Rules:

- `SUCCEEDED` with a missing signed URL is not `PENDING` or “generating”.
- A URL load error must render an explicit unavailable/retry state rather than a black player or `00:00` placeholder.
- Clients must not call a Provider URL directly as a fallback for a persisted asset.
- Clients must not write or promote `storage://` references, file IDs, prices, or ownership fields.
- Refresh/re-sign is safe; re-generation is a separate business action and must not be triggered by an expired signed URL alone.

## 9. Review and operational checklist

For any new image/video/montage/PPT artifact path, reviewers must answer:

- [ ] Where is the provider result validated?
- [ ] Where is it persisted to private owned storage?
- [ ] What is the durable `storage://<fileId>` field and how is it protected from overwrite?
- [ ] Is persistence idempotent across worker retry, HTTP retry, and client retry?
- [ ] How are object/file ownership and tenant boundaries enforced?
- [ ] Where is the signed URL generated, what is its TTL, and how is it refreshed?
- [ ] What response represents `EXPIRED`, missing object, or persistence failure?
- [ ] Does the client distinguish unavailable from generating?
- [ ] Are download content type/filename/platform compatibility normalized?
- [ ] Are object, database, API, and browser playback/download checks present?

## Related documents

- [Production Delivery Loop](../process/DELIVERY-LOOP.md)
- [Video Backfill Runbook](../runbooks/VIDEO-BACKFILL-RUNBOOK.md)
- [Immutable Image Release](immutable-image-release.md)
