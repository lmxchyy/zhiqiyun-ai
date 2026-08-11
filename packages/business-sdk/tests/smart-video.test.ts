import assert from "node:assert/strict";
import test from "node:test";
import type { ApiClient, ApiRequestOptions } from "@xianzhi/api-client";
import { createSmartVideoSdk } from "../src/smart-video.ts";

function mockApi(handler: (path: string, options?: ApiRequestOptions) => Promise<unknown>): ApiClient {
  return {
    getBaseURL: () => "https://example.test",
    setBaseURL() {},
    request(path, options) {
      return handler(path, options) as Promise<never>;
    }
  };
}

test("createExport injects Idempotency-Key and posts versionId only", async () => {
  let seenPath = "";
  let seenOptions: ApiRequestOptions | undefined;
  const sdk = createSmartVideoSdk(mockApi(async (path, options) => {
    seenPath = path;
    seenOptions = options;
    return {
      id: "svrender_1",
      projectId: "vp_1",
      versionId: "svv_1",
      status: "CREATED",
      progress: 0,
      createdAt: "2026-08-11T00:00:00Z",
      updatedAt: "2026-08-11T00:00:00Z",
      tenantId: "t",
      userId: "u",
      clientRequestId: "key-1"
    };
  }));

  const task = await sdk.createExport("vp_1", { versionId: "svv_1", idempotencyKey: "key-1" });
  assert.equal(seenPath, "/api/v1/video-projects/vp_1/render-tasks");
  assert.equal(seenOptions?.method, "POST");
  assert.equal(seenOptions?.headers?.["Idempotency-Key"], "key-1");
  assert.deepEqual(seenOptions?.body, { versionId: "svv_1", idempotencyKey: "key-1" });
  assert.equal(task.id, "svrender_1");
});

test("analyze auto-generates Idempotency-Key when omitted", async () => {
  let header = "";
  const sdk = createSmartVideoSdk(mockApi(async (_path, options) => {
    header = options?.headers?.["Idempotency-Key"] || "";
    return { projectId: "vp_1", status: "ANALYZING" };
  }));
  await sdk.analyze("vp_1");
  assert.match(header, /^sv_/);
});

test("cancel and retry hit dedicated routes", async () => {
  const paths: string[] = [];
  const sdk = createSmartVideoSdk(mockApi(async path => {
    paths.push(path);
    return {
      id: "svrender_1",
      projectId: "vp_1",
      versionId: "svv_1",
      status: "CANCELLED",
      progress: 0,
      createdAt: "2026-08-11T00:00:00Z",
      updatedAt: "2026-08-11T00:00:00Z",
      tenantId: "t",
      userId: "u",
      clientRequestId: "key"
    };
  }));
  await sdk.cancelRenderTask("vp_1", "svrender_1");
  await sdk.retryRenderTask("vp_1", "svrender_1");
  assert.deepEqual(paths, [
    "/api/v1/video-projects/vp_1/render-tasks/svrender_1/cancel",
    "/api/v1/video-projects/vp_1/render-tasks/svrender_1/retry"
  ]);
});
