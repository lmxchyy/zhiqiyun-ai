import { defineConfig, devices } from "@playwright/test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const baseURL = process.env.USER_H5_BASE_URL || "http://127.0.0.1:4173";
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
const browserChannel = process.platform === "win32" ? { channel: "msedge" as const } : {};
const repoRoot = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));
const userUniRoot = path.join(repoRoot, "apps/user-uni");

export default defineConfig({
  testDir: ".",
  testMatch: ["*.spec.ts"],
  timeout: 30_000,
  expect: {
    timeout: 10_000
  },
  use: {
    baseURL,
    trace: "retain-on-failure"
  },
  webServer: {
    command: `${npmCommand} run preview -- --host 127.0.0.1 --port 4173`,
    cwd: userUniRoot,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  },
  projects: [
    {
      name: "mobile-h5",
      use: {
        ...devices["Pixel 5"],
        browserName: "chromium",
        ...browserChannel
      }
    },
    // Desktop guest DOM still diverges from mobile; keep available via USER_H5_INCLUDE_DESKTOP=1.
    ...(process.env.USER_H5_INCLUDE_DESKTOP === "1"
      ? [{
          name: "desktop-h5",
          use: {
            ...devices["Desktop Chrome"],
            ...browserChannel
          }
        }]
      : [])
  ]
});
