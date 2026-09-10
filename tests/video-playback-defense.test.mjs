import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const pageSource = await readFile(
  new URL("../apps/user-uni/src/components/assets/AssetDetailCenterPage.vue", import.meta.url),
  "utf8",
);

const apiSource = await readFile(
  new URL("../apps/user-uni/src/features/assets/api.ts", import.meta.url),
  "utf8",
);

const typesSource = await readFile(
  new URL("../apps/user-uni/src/features/assets/types.ts", import.meta.url),
  "utf8",
);

const goApiSource = await readFile(
  new URL("../backend-go/internal/httpserver/api.go", import.meta.url),
  "utf8",
);

test("AssetDetailCenterPage binds video error handler and renders fallback error card", () => {
  assert.match(pageSource, /@error="handleVideoError"/, "video tag must bind @error");
  assert.match(pageSource, /class="video-error-container"/, "must have fallback error card for video");
  assert.match(pageSource, /isVideoExpired/, "must evaluate isVideoExpired");
  assert.match(pageSource, /videoErrorTitle/, "must display friendly error title");
  assert.match(pageSource, /videoErrorCopy/, "must display friendly error description");
  assert.match(pageSource, /regenerate/, "must provide regenerate action button on failure");
});

test("AssetDetailCenterPage guards video download when expired", () => {
  assert.match(pageSource, /if\s*\(isVideoExpired\.value/, "download must guard expired video");
  assert.match(pageSource, /视频源已失效无法下载，请重新生成/, "must show toast when downloading expired video");
});

test("frontend asset types and normalizer expose availability contracts", () => {
  assert.match(typesSource, /availability\?:/, "AssetItem must define availability");
  assert.match(typesSource, /videoStatus\?:/, "AssetItem must define videoStatus");
  assert.match(apiSource, /availability:\s*stringValue\(raw\.availability/, "normalizeAsset must parse availability");
  assert.match(apiSource, /videoStatus:\s*stringValue\(raw\.videoStatus/, "normalizeAsset must parse videoStatus");
});

test("Go backend guards asset availability and download for expired video", () => {
  assert.match(goApiSource, /AssetAvailabilityExpired\s*=\s*"EXPIRED"/, "Go backend must define AssetAvailabilityExpired");
  assert.match(goApiSource, /func enrichAssetAvailability/, "Go backend must implement enrichAssetAvailability");
  assert.match(goApiSource, /writeError\(w,\s*http\.StatusGone,\s*errors\.New\("视频源已失效，无法下载，请重新生成"\)\)/, "download must return 410 Gone for expired video");
});
