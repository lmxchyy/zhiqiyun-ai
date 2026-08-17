import { defineConfig, devices } from "@playwright/test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const baseURL = process.env.PPT_PREVIEW_BASE_URL || "http://127.0.0.1:4175";
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
const browserChannel = process.platform === "win32" ? { channel: "msedge" as const } : {};
const repoRoot = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));

export default defineConfig({
  testDir: ".",
  testMatch: ["*.spec.ts"],
  snapshotPathTemplate: "{testDir}/snapshots/{arg}{ext}",
  timeout: 45_000,
  expect: { timeout: 10_000, toHaveScreenshot: { maxDiffPixelRatio: 0.015 } },
  use: { baseURL, trace: "retain-on-failure", screenshot: "only-on-failure" },
  webServer: {
    command: `${npmCommand} run dev -- --host 127.0.0.1 --port 4175`,
    cwd: path.join(repoRoot, "admin-vue"),
    url: `${baseURL}/admin/tests/fixtures/ppt-agent-preview.html`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  },
  projects: [{ name: "preview-chromium", use: { ...devices["Desktop Chrome"], ...browserChannel, viewport: { width: 1440, height: 1000 } } }]
});
