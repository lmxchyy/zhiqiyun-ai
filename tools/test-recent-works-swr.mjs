import assert from "node:assert/strict";
import {
  isCurrentRecentRequest,
  mergeRecentWorks,
  shouldDedupeRecentRequest,
} from "../apps/user-uni/src/features/assets/recent.ts";

function asset(id, updatedAt, thumbnailUrl = "") {
  return {
    id,
    taskId: `task_${id}`,
    name: id,
    type: "image",
    status: "completed",
    remoteUrl: "",
    fallbackUrl: "/fallback.jpg",
    thumbnailUrl,
    projectId: "",
    projectName: "",
    favorite: false,
    archived: false,
    deletedAt: "",
    createdAt: updatedAt,
    updatedAt,
    tags: [],
    model: "",
    prompt: "",
    negativePrompt: "",
    fileSize: 0,
    aspectRatio: "",
    metadata: {},
  };
}

const cached = [
  asset("work_2", "2026-07-23T01:00:00Z", "data:image/jpeg;base64,CACHE"),
  asset("work_1", "2026-07-22T01:00:00Z", "data:image/jpeg;base64,OLD"),
];
const refreshed = [
  asset("work_3", "2026-07-23T02:00:00Z", "data:image/jpeg;base64,NEW"),
  asset("work_2", "2026-07-23T01:00:00Z", ""),
];
const merged = mergeRecentWorks(cached, refreshed, 20);
assert.deepEqual(merged.map(item => item.id), ["work_3", "work_2", "work_1"]);
assert.equal(merged[1].thumbnailUrl, "data:image/jpeg;base64,CACHE");
assert.deepEqual(mergeRecentWorks(cached, [], 20).map(item => item.id), ["work_2", "work_1"]);
assert.equal(new Set(mergeRecentWorks(cached, [cached[0], cached[0]], 20).map(item => item.id)).size, 2);
assert.equal(shouldDedupeRecentRequest(1000, 1100, false), true);
assert.equal(shouldDedupeRecentRequest(1000, 1100, true), false);
assert.equal(shouldDedupeRecentRequest(0, 1000, false), false);
assert.equal(
  Array.from({ length: 5 }, (_, index) => shouldDedupeRecentRequest(1000, 1000 + index * 25, false))
    .filter(Boolean).length,
  5,
);
assert.equal(isCurrentRecentRequest(8, 9, "user-a", "user-a"), false);
assert.equal(isCurrentRecentRequest(9, 9, "user-a", "user-b"), false);
assert.equal(isCurrentRecentRequest(9, 9, "user-a", "user-a"), true);

console.log(JSON.stringify({
  ok: true,
  checks: [
    "new_work_inserted_at_top",
    "cached_cover_preserved",
    "empty_refresh_does_not_flash_clear",
    "duplicate_ids_removed",
    "rapid_switch_requests_deduplicated",
    "stale_response_rejected",
  ],
}));
