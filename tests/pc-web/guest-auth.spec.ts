import { expect, test } from "@playwright/test";

test.beforeEach(async ({ context }) => {
  await context.clearCookies();
});

test("the single PC entry opens the real workspace in guest state", async ({ page }) => {
  const privateRequests: string[] = [];
  page.on("request", request => {
    const path = new URL(request.url()).pathname;
    if (/^\/api\/v1\/(?:user|member|generation-tasks|assets)(?:\/|$)/.test(path)) privateRequests.push(path);
  });

  await page.goto("/");

  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator(".admin-shell.user-console-shell")).toBeVisible();
  await expect(page.locator(".user-home-page")).toBeVisible();
  await expect(page.locator(".auth-modal")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /游客.*点击登录/ })).toBeVisible();
  expect(privateRequests).toEqual([]);
});

test("cancelled image login keeps the prompt and never submits generation", async ({ page }) => {
  const prompt = "一张青紫色企业 AI 产品发布海报，主体居中，留白干净";
  const generationRequests: string[] = [];
  page.on("request", request => {
    if (new URL(request.url()).pathname === "/api/v1/generation-tasks" && request.method() === "POST") {
      generationRequests.push(request.method());
    }
  });

  await page.goto("/");
  await page.getByRole("menuitem", { name: "AI 生图" }).click();
  const promptEditor = page.locator('.prompt-editable-input[role="textbox"]');
  await promptEditor.fill(prompt);
  await page.getByRole("button", { name: "生成图像" }).click();

  await expect(page.locator(".auth-modal")).toBeVisible();
  await expect(page.getByRole("heading", { name: "登录后继续使用" })).toBeVisible();
  await page.getByRole("button", { name: /暂不登录/ }).last().click();

  await expect(page.locator(".auth-modal")).toHaveCount(0);
  await expect(promptEditor).toContainText(prompt);
  await expect(page).toHaveURL(/\/$/);
  expect(generationRequests).toEqual([]);
});

test("guest works center uses public cases and gates only My Works", async ({ page }) => {
  const privateRequests: string[] = [];
  page.on("request", request => {
    const path = new URL(request.url()).pathname;
    if (/^\/api\/v1\/(?:user\/online-image|generation-tasks|assets)(?:\/|$)/.test(path)) privateRequests.push(path);
  });

  await page.goto("/");
  await page.getByRole("menuitem", { name: "作品中心" }).click();

  await expect(page.getByRole("button", { name: "官方精选" })).toBeVisible();
  await expect(page.getByText("先浏览官方精选案例", { exact: true })).toBeVisible();
  await expect(page.locator(".user-work-card")).toHaveCount(4);
  expect(privateRequests).toEqual([]);

  await page.getByRole("button", { name: "登录后查看我的作品" }).click();
  await expect(page.locator(".auth-modal")).toBeVisible();
  expect(privateRequests).toEqual([]);
});

test("login resumes the image draft and submits the protected action exactly once", async ({ page }) => {
  const prompt = "登录恢复测试：高端白色空间中的智能终端产品，青紫色轮廓光";
  const generationBodies: Array<Record<string, unknown>> = [];
  await page.route("**/api/v1/generation-tasks", async route => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    generationBodies.push(route.request().postDataJSON() as Record<string, unknown>);
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "pc-e2e-generation-1",
        type: "TEXT_TO_IMAGE",
        prompt,
        model: "mock-standard",
        status: "SUCCEEDED",
        resultIds: [],
        createdAt: new Date().toISOString()
      })
    });
  });

  await page.goto("/");
  await page.getByRole("menuitem", { name: "AI 生图" }).click();
  const promptEditor = page.locator('.prompt-editable-input[role="textbox"]');
  await promptEditor.fill(prompt);
  await page.getByRole("button", { name: "生成图像" }).click();

  const loginModal = page.locator(".auth-modal");
  await expect(loginModal).toBeVisible();
  await loginModal.getByRole("tab", { name: "密码登录" }).click();
  await loginModal.locator('input[autocomplete="username"]').fill("demo@xianzhi.ai");
  await loginModal.locator('input[autocomplete="current-password"]').fill("Demo123!");
  await loginModal.locator(".web-login-agreement input").check();
  await loginModal.getByRole("button", { name: "登录并继续" }).click();

  const resumeDialog = page.locator(".el-message-box");
  await expect(resumeDialog.getByText("继续刚才的创作", { exact: true })).toBeVisible();
  await expect(promptEditor).toContainText(prompt);
  expect(generationBodies).toHaveLength(0);
  await resumeDialog.getByRole("button", { name: "确认生成" }).click();

  await expect.poll(() => generationBodies.length).toBe(1);
  expect(generationBodies[0]?.prompt).toBe(prompt);
  expect(String(generationBodies[0]?.clientRequestId || "")).toMatch(/^web-ai-image-/);
  await expect(page).toHaveURL(/\/$/);
  await expect(loginModal).toHaveCount(0);
});

test("logout returns the same PC entry to guest browsing", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /游客.*点击登录/ }).click();
  const loginModal = page.locator(".auth-modal");
  await loginModal.getByRole("tab", { name: "密码登录" }).click();
  await loginModal.locator('input[autocomplete="username"]').fill("demo@xianzhi.ai");
  await loginModal.locator('input[autocomplete="current-password"]').fill("Demo123!");
  await loginModal.locator(".web-login-agreement input").check();
  await loginModal.getByRole("button", { name: "登录并继续" }).click();

  await expect(loginModal).toHaveCount(0);
  await expect(page.locator(".admin-header .account-button")).toBeVisible();
  await page.locator(".admin-header .account-button").click();
  await page.getByRole("menuitem", { name: "退出登录" }).click();

  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator(".user-home-page")).toBeVisible();
  await expect(page.getByRole("button", { name: /游客.*点击登录/ })).toBeVisible();
  await expect(loginModal).toHaveCount(0);
});
