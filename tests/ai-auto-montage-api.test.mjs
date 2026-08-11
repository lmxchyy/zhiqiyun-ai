import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

async function read(rel) {
  return readFile(path.join(root, rel), "utf8");
}

test("smart-video API routes stay registered and auth-gated in Go tests", async () => {
  const source = await read("backend-go/internal/httpserver/smart_video_api_test.go");
  assert.match(source, /TestSmartVideoRoutesAreRegisteredAndProtected/);
  assert.match(source, /\/api\/v1\/video-projects/);
  assert.match(source, /StatusUnauthorized/);
  assert.match(source, /TestSmartVideoAssetPayloadRejectsRemoteURLAndLocalPath/);
});

test("export path uses ExportService and smoke direct enqueue is closed", async () => {
  const service = await read("backend-go/internal/app/smartvideo/service.go");
  assert.match(service, /ErrExportNotReady/);
  assert.match(service, /use ExportService\.CreateExport/);
  assert.doesNotMatch(service, /renderQueue\.Enqueue/);

  const api = await read("backend-go/internal/httpserver/smart_video_api.go");
  assert.match(api, /exports\.CreateExport/);
  assert.match(api, /exports\.CancelExport/);
  assert.match(api, /exports\.RetryExport/);
});

test("points lifecycle uses SMART_VIDEO_RENDER and idempotent keys", async () => {
  const lifecycle = await read("backend-go/internal/app/smartvideo/personal_points_lifecycle.go");
  assert.match(lifecycle, /SMART_VIDEO_RENDER/);
  assert.match(lifecycle, /sv_render_/);
  assert.match(lifecycle, /reserve|capture|release/i);
  assert.match(lifecycle, /PointsLifecycle/);

  const bridge = await read("backend-go/internal/httpserver/smartvideo_points_bridge.go");
  assert.match(bridge, /NewSmartVideoPointsLifecycleFromDB|PersonalPoint/);
});

test("analysis and render enqueue through outbox", async () => {
  const analysis = await read("backend-go/internal/app/smartvideo/analysis_service.go");
  assert.match(analysis, /EnqueueAnalysisTaskWithOutbox|CreateAnalysis.*Outbox|outbox/i);
  assert.doesNotMatch(analysis, /queue\.Enqueue\(/);

  const postgres = await read("backend-go/internal/app/smartvideo/postgres_analysis.go");
  assert.match(postgres, /EnqueueAnalysisTaskWithOutbox/);
  assert.match(postgres, /video_task_outbox/);
});

test("ffmpeg argv stays non-shell and rejects arbitrary URL payloads", async () => {
  const rendererTest = await read("backend-go/internal/provider/media/manifest_renderer_test.go");
  assert.match(rendererTest, /shell invocation is forbidden|output path must remain one argv element/);

  const assetReject = await read("backend-go/internal/httpserver/smart_video_api_test.go");
  assert.match(assetReject, /RejectsRemoteURLAndLocalPath/);
});
