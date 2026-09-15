import test from "node:test";
import assert from "node:assert/strict";
import {
  videoCardPlaceholderText,
  isSameVideoHistoryEntry,
  mergeVideoHistoryEntry,
  mergeVideoHistoryList,
  normalizeVideoHistoryEntry,
  taskToVideoHistoryEntry
} from "../admin-vue/src/utils/videoGeneration.ts";

test("AC-1: videoCardPlaceholderText never shows '生成中' for success videos", () => {
  // 1. success + url => "悬停预览"
  assert.equal(
    videoCardPlaceholderText({ status: "success", url: "https://example.com/v.mp4" }),
    "悬停预览"
  );

  // 2. success + no url (compacted payload) => "已完成"
  assert.equal(
    videoCardPlaceholderText({ status: "success", url: "" }),
    "已完成"
  );
  assert.notEqual(
    videoCardPlaceholderText({ status: "success", url: "" }),
    "生成中"
  );

  // 3. success + EXPIRED => "已完成 · 视频源已过期"
  assert.equal(
    videoCardPlaceholderText({ status: "success", url: "", availability: "EXPIRED" }),
    "已完成 · 视频源已过期"
  );
  assert.notEqual(
    videoCardPlaceholderText({ status: "success", url: "", availability: "EXPIRED" }),
    "生成中"
  );

  // 4. failed => "生成失败"
  assert.equal(
    videoCardPlaceholderText({ status: "failed", url: "" }),
    "生成失败"
  );

  // 5. generating => "生成中"
  assert.equal(
    videoCardPlaceholderText({ status: "generating", url: "" }),
    "生成中"
  );
});

test("AC-2: mergeVideoHistoryEntry protects existing valid URLs from destructive empty overwrite", () => {
  const existing = {
    id: "task_001",
    taskId: "task_001",
    backendTaskId: "task_001",
    url: "https://storage.example.com/video_signed.mp4",
    posterUrl: "https://storage.example.com/poster.jpg",
    thumbnailUrl: "https://storage.example.com/poster.jpg",
    downloadUrl: "https://storage.example.com/download.mp4",
    prompt: "a majestic mountain",
    model: "grok-imagine-1.5-video",
    mode: "text-to-video",
    aspect_ratio: "16:9",
    duration: 6,
    resolution: "720p",
    inputImageUrls: ["https://example.com/ref.png"],
    inputVideoUrl: "",
    createdAt: "2026-09-15T10:00:00Z",
    timestamp: 1789454400000,
    status: "success",
    assetId: "asset_001",
    resultIds: ["asset_001"]
  };

  // Backend returns compacted task where url/posterUrl/downloadUrl are empty
  const incoming = {
    id: "task_001",
    backendTaskId: "task_001",
    url: "",
    posterUrl: "",
    thumbnailUrl: "",
    downloadUrl: "",
    status: "success"
  };

  const merged = mergeVideoHistoryEntry(existing, incoming);

  // Assert local valid URLs are preserved
  assert.equal(merged.url, "https://storage.example.com/video_signed.mp4");
  assert.equal(merged.posterUrl, "https://storage.example.com/poster.jpg");
  assert.equal(merged.thumbnailUrl, "https://storage.example.com/poster.jpg");
  assert.equal(merged.downloadUrl, "https://storage.example.com/download.mp4");
  assert.equal(merged.assetId, "asset_001");
  assert.deepEqual(merged.resultIds, ["asset_001"]);
  assert.deepEqual(merged.inputImageUrls, ["https://example.com/ref.png"]);
  assert.equal(merged.status, "success");
});

test("AC-2: mergeVideoHistoryEntry prevents success status from regressing to generating", () => {
  const existing = {
    id: "task_002",
    taskId: "task_002",
    backendTaskId: "task_002",
    url: "https://storage.example.com/video.mp4",
    prompt: "sunset harbor",
    model: "grok-imagine-1.5-video",
    mode: "text-to-video",
    aspect_ratio: "16:9",
    duration: 6,
    resolution: "720p",
    inputImageUrls: [],
    inputVideoUrl: "",
    createdAt: "2026-09-15T10:00:00Z",
    timestamp: 1789454400000,
    status: "success"
  };

  // Late poll or stale message returns generating
  const staleIncoming = {
    id: "task_002",
    status: "generating"
  };

  const merged = mergeVideoHistoryEntry(existing, staleIncoming);
  assert.equal(merged.status, "success", "confirmed success status must not be overridden by generating");
});

test("AC-2: isSameVideoHistoryEntry aligns across id, backendTaskId, and taskId", () => {
  assert.equal(isSameVideoHistoryEntry({ id: "t1" }, { id: "t1" }), true);
  assert.equal(isSameVideoHistoryEntry({ id: "video-123", backendTaskId: "t1" }, { id: "t1" }), true);
  assert.equal(isSameVideoHistoryEntry({ id: "t1" }, { id: "video-123", backendTaskId: "t1" }), true);
  assert.equal(isSameVideoHistoryEntry({ id: "video-123", taskId: "t1" }, { id: "t1" }), true);
  assert.equal(isSameVideoHistoryEntry({ id: "t1" }, { id: "t2" }), false);
});

test("AC-2: mergeVideoHistoryList merges incoming updates without deleting existing un-matched entries", () => {
  const current = [
    {
      id: "task_old",
      taskId: "task_old",
      url: "https://storage.example.com/old.mp4",
      prompt: "old",
      model: "grok",
      mode: "text-to-video",
      aspect_ratio: "16:9",
      duration: 6,
      resolution: "720p",
      inputImageUrls: [],
      inputVideoUrl: "",
      createdAt: "2026-09-14T10:00:00Z",
      timestamp: 1789400000000,
      status: "success"
    }
  ];

  const incoming = [
    {
      id: "task_old",
      url: "", // compacted empty url
      status: "success"
    },
    {
      id: "task_new",
      taskId: "task_new",
      url: "https://storage.example.com/new.mp4",
      prompt: "new",
      model: "grok",
      mode: "text-to-video",
      aspect_ratio: "16:9",
      duration: 6,
      resolution: "720p",
      inputImageUrls: [],
      inputVideoUrl: "",
      createdAt: "2026-09-15T10:00:00Z",
      timestamp: 1789454400000,
      status: "success"
    }
  ];

  const merged = mergeVideoHistoryList(current, incoming, []);
  assert.equal(merged.length, 2);
  const oldItem = merged.find((i) => i.id === "task_old");
  assert.ok(oldItem);
  assert.equal(oldItem.url, "https://storage.example.com/old.mp4", "existing URL must be preserved");

  const newItem = merged.find((i) => i.id === "task_new");
  assert.ok(newItem);
  assert.equal(newItem.url, "https://storage.example.com/new.mp4");
});

test("AC-3 & AC-4: taskToVideoHistoryEntry keeps success status and extracts assetId/resultIds when URL is compacted", () => {
  const compactedTask = {
    id: "task_compact_01",
    userId: "user_1",
    type: "TEXT_TO_VIDEO",
    status: "SUCCEEDED",
    model: "grok-imagine-1.5-video",
    prompt: "cinematic sunset",
    resultIds: ["asset_v01"],
    outputUrl: "", // compacted to empty
    resultUrl: "",
    imageUrl: "",
    thumbnailUrl: "",
    params: {
      duration: 6,
      aspect_ratio: "16:9",
      resolution: "720p"
    },
    createdAt: "2026-09-15T10:53:00Z"
  };

  const entry = taskToVideoHistoryEntry(compactedTask);
  assert.ok(entry);
  assert.equal(entry.status, "success", "SUCCEEDED task must map to success");
  assert.equal(entry.url, "", "compacted URL is empty");
  assert.equal(entry.assetId, "asset_v01", "assetId must be extracted as signing clue");
  assert.deepEqual(entry.resultIds, ["asset_v01"], "resultIds must be extracted");
  assert.equal(videoCardPlaceholderText(entry), "已完成", "must show 已完成, never 生成中");
});

test("AC-1: taskToVideoHistoryEntry propagates EXPIRED availability", () => {
  const expiredTask = {
    id: "task_expired_01",
    userId: "user_1",
    type: "TEXT_TO_VIDEO",
    status: "SUCCEEDED",
    model: "grok-imagine-1.5-video",
    prompt: "old video",
    resultIds: ["asset_exp01"],
    outputUrl: "",
    availability: "EXPIRED",
    availabilityReason: "视频源已失效，请重新生成",
    params: {
      duration: 6
    },
    createdAt: "2026-08-01T10:00:00Z"
  };

  const entry = taskToVideoHistoryEntry(expiredTask);
  assert.ok(entry);
  assert.equal(entry.status, "success");
  assert.equal(entry.availability, "EXPIRED");
  assert.equal(entry.availabilityReason, "视频源已失效，请重新生成");
  assert.equal(videoCardPlaceholderText(entry), "已完成 · 视频源已过期");
  assert.notEqual(videoCardPlaceholderText(entry), "生成中");
});
