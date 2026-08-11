import { expect, test } from "@playwright/test";

test("user H5 shell renders usable content", async ({ page }) => {
  const pageErrors: string[] = [];
  page.on("pageerror", error => pageErrors.push(error.message));

  await page.goto("/");
  const app = page.locator("#app");
  await expect(app).toBeVisible();
  await expect.poll(async () => (await app.innerText()).trim().length).toBeGreaterThan(5);
  expect(pageErrors).toEqual([]);
});

test("guest sees the product before login and can carry an idea into creation", async ({ page }) => {
  const idea = "为东方茶饮制作一支夏日产品短片";
  await page.goto("/");
  await expect(page.locator(".guest-site")).toBeVisible();
  await expect(page.locator(".guest-hero-title")).toContainText("把一个想法，变成");
  await expect(page.locator(".login-shell")).toHaveCount(0);
  await expect(page.getByText("知启云 · 普通用户", { exact: true })).toHaveCount(0);

  await page.locator(".guest-prompt-card textarea").fill(idea);
  await page.locator(".guest-primary-action").click();
  await expect(page).toHaveURL(/\/app\/video-generation(?:#\/)?$/);
  await expect(page.locator(".web-video-form textarea")).toHaveValue(idea);
  await expect(page.locator(".login-shell")).toHaveCount(0);
});

test("guest can open and refresh the public workspace without login redirect", async ({ page }) => {
  const requestedPaths: string[] = [];
  page.on("request", request => requestedPaths.push(new URL(request.url()).pathname));
  await page.goto("/app");
  await expect(page.locator(".app-shell")).toBeVisible();
  await expect(page.locator(".login-shell")).toHaveCount(0);
  await expect(page).toHaveURL(/\/app(?:#\/)?$/);
  await page.reload();
  await expect(page.locator(".app-shell")).toBeVisible();
  await expect(page).toHaveURL(/\/app(?:#\/)?$/);
  expect(requestedPaths.some(path => path === "/api/v1/models")).toBeTruthy();
  expect(requestedPaths.filter(path => /^\/api\/v1\/(?:member|assets|generation-tasks|points|user)(?:\/|$)/.test(path))).toEqual([]);
});

test("guest works center shows official cases without requesting private works", async ({ page }) => {
  const requestedPaths: string[] = [];
  page.on("request", request => requestedPaths.push(new URL(request.url()).pathname));
  await page.goto("/app/works");
  await expect(page.getByText("官方精选", { exact: true })).toBeVisible();
  await expect(page.getByText("登录后查看我的作品", { exact: true })).toBeVisible();
  expect(requestedPaths.filter(path => /^\/api\/v1\/(?:assets|generation-tasks)(?:\/|$)/.test(path))).toEqual([]);
});

test("guest PPT draft survives a cancelled login and page refresh", async ({ page }) => {
  const prompt = "网页游客登录恢复测试方案";
  await page.goto("/app/ppt-generation");
  const input = page.locator(".ppt-prompt-textarea textarea");
  await input.fill(prompt);
  await page.locator(".ppt-generate-button").click();
  await expect(page.getByText("登录后继续使用", { exact: true })).toBeVisible();
  await page.getByText("暂不登录", { exact: true }).click();
  await expect(input).toHaveValue(prompt);
  await expect.poll(() => page.evaluate(() => window.localStorage.getItem("zhiqiyun:web:ppt-guest-draft"))).toContain(prompt);
  await expect(page).toHaveURL(/\/app\/ppt-generation(?:#\/)?$/);
  await page.reload();
  await expect.poll(() => page.evaluate(() => window.localStorage.getItem("zhiqiyun:web:ppt-guest-draft"))).toContain(prompt);
  await expect(page.locator(".ppt-prompt-textarea textarea")).toHaveValue(prompt);
});

test("guest video draft survives cancelled login without creating a task", async ({ page }) => {
  const prompt = "A product film with a slow camera orbit and soft studio lighting";
  const taskRequests: string[] = [];
  page.on("request", request => {
    if (new URL(request.url()).pathname === "/api/v1/generation-tasks") taskRequests.push(request.method());
  });

  await page.goto("/app/video-generation");
  const input = page.locator(".web-video-form textarea");
  await input.fill(prompt);
  // Prefer an aspect-ratio chip (e.g. 16:9 / 9:16); fall back to any visible option after models load.
  const ratioOption = page.locator(".web-video-form .web-video-option").filter({ hasText: /\d+\s*:\s*\d+/ }).first();
  await expect(ratioOption).toBeVisible({ timeout: 20000 });
  await ratioOption.click();
  await page.locator(".web-video-submit").click();
  await expect(page.getByText("登录后继续使用", { exact: true })).toBeVisible();
  await page.getByText("暂不登录", { exact: true }).click();

  await expect(input).toHaveValue(prompt);
  expect(taskRequests).toEqual([]);
  await expect.poll(() => page.evaluate(() => window.localStorage.getItem("zhiqiyun:web:video-guest-draft"))).toContain(prompt);
  await page.reload();
  await expect(page.locator(".web-video-form textarea")).toHaveValue(prompt);
  await expect(page.locator(".web-video-form .web-video-option.active").filter({ hasText: /\d+\s*:\s*\d+/ })).toHaveCount(1);
});

test("wireless canvas uses one parent login gate and cancels the protected fetch in place", async ({ page }) => {
  await page.goto("/app/wireless-canvas");
  await expect(page.locator(".wireless-canvas-frame")).toBeVisible();
  const frame = await expect.poll(() => page.frames().find(item => item.url().includes("smart-canvas.html"))).not.toBeUndefined().then(() =>
    page.frames().find(item => item.url().includes("smart-canvas.html"))!,
  );

  await expect.poll(() => frame.evaluate(() => window.fetch.name)).toBe("canvasAuthenticatedFetch");
  await frame.evaluate(() => {
    window.localStorage.removeItem("token");
    (window as typeof window & { __canvasAuthProbe?: Promise<string> }).__canvasAuthProbe = window
      .fetch("/api/canvas-video", { method: "POST", body: "{}" })
      .then(() => "unexpected-success")
      .catch(error => error instanceof Error ? error.message : String(error));
  });
  await expect(page.getByText("登录后继续使用", { exact: true })).toBeVisible();
  await expect(page.getByText("登录后继续使用", { exact: true })).toHaveCount(1);
  await page.getByText("暂不登录", { exact: true }).click();
  await expect.poll(() => frame.evaluate(() =>
    (window as typeof window & { __canvasAuthProbe?: Promise<string> }).__canvasAuthProbe,
  )).toBe("LOGIN_CANCELLED");
  await expect(page).toHaveURL(/\/app\/wireless-canvas(?:#\/)?$/);
});
