import assert from "node:assert/strict";
import test from "node:test";
import type { ApiClient, ApiRequestOptions } from "@xianzhi/api-client";
import { createMultipartUploadController } from "../src/files.ts";

function mockApi(handler: (path: string, options?: ApiRequestOptions) => Promise<unknown>): ApiClient {
  return {
    getBaseURL: () => "https://example.test",
    setBaseURL() {},
    request(path, options) {
      return handler(path, options) as Promise<never>;
    }
  };
}

test("multipart controller uploads parts with concurrency and completes", async () => {
  const calls: string[] = [];
  const partsPut: number[] = [];
  const api = mockApi(async (path, options) => {
    calls.push(`${options?.method || "GET"} ${path}`);
    if (path.endsWith("/multipart/init")) {
      return {
        uploadId: "up_1",
        fileId: "file_1",
        objectKey: "obj/1",
        partSize: 4,
        totalParts: 3
      };
    }
    if (path.includes("/parts/")) {
      const partNumber = Number(path.split("/").pop());
      return {
        partNumber,
        uploadUrl: `https://upload.test/part/${partNumber}`,
        headers: { "x-amz-meta-part": String(partNumber) }
      };
    }
    if (path.endsWith("/complete")) {
      const body = options?.body as { parts: Array<{ partNumber: number; etag: string }> };
      assert.equal(body.parts.length, 3);
      assert.deepEqual(body.parts.map(p => p.partNumber), [1, 2, 3]);
      return {
        file: {
          fileId: "file_1",
          tenantId: "t",
          userId: "u",
          storageConfigId: "s",
          provider: "s3",
          bucket: "b",
          objectKey: "obj/1",
          originalName: "clip.mp4",
          fileSize: 10,
          businessType: "smart_video_source",
          visibility: "PRIVATE",
          status: "READY",
          isTemporary: false,
          createdAt: "2026-08-11T00:00:00Z",
          updatedAt: "2026-08-11T00:00:00Z"
        }
      };
    }
    throw new Error(`unexpected path ${path}`);
  });

  const controller = createMultipartUploadController(api);
  const blob = new Blob([new Uint8Array(10)]);
  const statuses: string[] = [];
  const handle = controller.uploadBlob(
    { name: "clip.mp4", type: "video/mp4", blob },
    { businessType: "smart_video_source" },
    {
      concurrency: 2,
      maxRetries: 1,
      onProgress: progress => statuses.push(progress.status),
      fetcher: async (url, init) => {
        const partNumber = Number(String(url).split("/").pop());
        partsPut.push(partNumber);
        return new Response(null, {
          status: 200,
          headers: { etag: `"etag-${partNumber}"` }
        });
      }
    }
  );

  const result = await handle.promise;
  assert.equal(result.file.fileId, "file_1");
  assert.equal(partsPut.sort((a, b) => a - b).join(","), "1,2,3");
  assert.ok(statuses.includes("uploading"));
  assert.ok(statuses.includes("completing"));
  assert.ok(statuses.includes("completed"));
  assert.ok(calls.some(c => c.includes("/multipart/init")));
  assert.ok(calls.some(c => c.includes("/complete")));
});

test("multipart controller aborts in-flight session", async () => {
  let aborted = false;
  const api = mockApi(async (path, options) => {
    if (path.endsWith("/multipart/init")) {
      return { uploadId: "up_abort", fileId: "file_a", objectKey: "o", partSize: 8, totalParts: 2 };
    }
    if (path.includes("/parts/")) {
      return { partNumber: 1, uploadUrl: "https://upload.test/part/1", headers: {} };
    }
    if (path.endsWith("/abort")) {
      aborted = true;
      return undefined;
    }
    throw new Error(`unexpected ${options?.method} ${path}`);
  });
  const controller = createMultipartUploadController(api);
  const handle = controller.uploadBlob(
    { name: "a.mp4", blob: new Blob([new Uint8Array(9)]) },
    {},
    {
      fetcher: async () => {
        await handle.abort();
        throw new Error("forced abort");
      }
    }
  );
  await assert.rejects(() => handle.promise);
  assert.equal(aborted, true);
});
