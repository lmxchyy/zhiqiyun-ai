import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.USER_H5_BASE_URL || "http://127.0.0.1:4173";
const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
const browserChannel = process.platform === "win32" ? { channel: "msedge" as const } : {};

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
    command: `${npmCommand} --prefix apps/user-uni run preview -- --host 127.0.0.1 --port 4173`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000
  },
  projects: [
    {
      name: "desktop-h5",
      use: {
        ...devices["Desktop Chrome"],
        ...browserChannel
      }
    },
    {
      name: "mobile-h5",
      use: {
        ...devices["Pixel 5"],
        browserName: "chromium",
        ...browserChannel
      }
    }
  ]
});
