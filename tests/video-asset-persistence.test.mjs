import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const configSource = await readFile(
  new URL("../backend-go/internal/config/config.go", import.meta.url),
  "utf8",
);

const storageSource = await readFile(
  new URL("../backend-go/internal/httpserver/generation_storage.go", import.meta.url),
  "utf8",
);

const storeSource = await readFile(
  new URL("../backend-go/internal/httpserver/postgres_store.go", import.meta.url),
  "utf8",
);

const apiSource = await readFile(
  new URL("../backend-go/internal/httpserver/api.go", import.meta.url),
  "utf8",
);

test("config declares VideoStoragePersistenceEnabled with fail-closed default", () => {
  assert.match(configSource, /VideoStoragePersistenceEnabled\s+bool/, "Config must declare VideoStoragePersistenceEnabled");
  assert.match(configSource, /VideoStoragePersistenceEnabled:\s*boolEnv\(os\.Getenv\("VIDEO_STORAGE_PERSISTENCE_ENABLED"\)\)/, "Load must read env VIDEO_STORAGE_PERSISTENCE_ENABLED");
});

test("generation_storage implements persistGeneratedVideos with streaming and size bounds", () => {
  assert.match(storageSource, /func \(a api\) persistGeneratedVideos/, "must declare persistGeneratedVideos");
  assert.match(storageSource, /func streamGeneratedVideoArtifact/, "must declare streamGeneratedVideoArtifact");
  assert.match(storageSource, /maxGeneratedVideoBytes/, "must enforce maxGeneratedVideoBytes");
  assert.match(storageSource, /os\.CreateTemp/, "must buffer to temp file instead of full in-memory read");
  assert.match(storageSource, /StoreObjectIdempotent/, "must store via fileService.StoreObjectIdempotent");
});

test("postgres_store binds video to storage URL and prevents saving temp provider URL", () => {
  assert.match(storeSource, /item\.URL = "storage:\/\/" \+ stringValue\(item\.Metadata\["fileId"\]\)/, "must bind URL to storage://fileId for managed video");
  assert.match(storeSource, /item\.Metadata\["storageKey"\] = stringValue\(stored\["objectKey"\]\)/, "must record storageKey in metadata");
});

test("api guards video generation settlement against persistence failure", () => {
  assert.match(apiSource, /if a\.cfg\.VideoStoragePersistenceEnabled/, "runVideoGenerationTask must branch on VideoStoragePersistenceEnabled");
  assert.match(apiSource, /prepared, storedFiles, persistErr = a\.persistGeneratedVideos/, "must call persistGeneratedVideos");
  assert.match(
    apiSource,
    /(?:FailGenerationTask(?:Durable)?\(|failGenerationTaskDurableWithFencing\(a\.store,\s*)taskID,\s*"视频资产归档失败，已取消并退回积分"/,
    "must fail task and release points on persistence failure",
  );
});
