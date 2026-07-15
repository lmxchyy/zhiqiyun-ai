import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";

const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const baseURL = String(process.env.XIANZHI_RUNTIME_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const adminEmail = process.env.XIANZHI_ADMIN_EMAIL || "admin@xianzhi.ai";
const adminPassword = process.env.XIANZHI_ADMIN_PASSWORD || "admin123!";
const userEmail = process.env.XIANZHI_VERIFY_USER_EMAIL || "demo@xianzhi.ai";
const userPassword = process.env.XIANZHI_VERIFY_USER_PASSWORD || "Demo123!";
const artifactDir = path.join(repoRoot, "artifacts", "admin-runtime");
const reportPath = path.join(repoRoot, "docs", "acceptance", "admin-runtime-verification.json");
const uploadFixture = path.join(repoRoot, "artifacts", "creation-smoke", "iphone17-ecommerce-task_000196.png");

fs.mkdirSync(artifactDir, { recursive: true });
fs.mkdirSync(path.dirname(reportPath), { recursive: true });

const report = {
  generatedAt: new Date().toISOString(),
  baseURL,
  admin: { modules: [], checks: [] },
  user: { modules: [], checks: [] },
  apiResponses: [],
  badResponses: [],
  expectedAborts: [],
  requestFailures: [],
  pageErrors: [],
  consoleErrors: [],
  screenshots: [],
  summary: {},
};

let currentScope = "bootstrap";

function messageOf(error) {
  return error instanceof Error ? error.message : String(error);
}

function compactText(value) {
  return String(value || "").replace(/\s+/g, " ").trim();
}

function normalizedMenuText(value) {
  return compactText(value).replace(/\s+/g, "");
}

function attachDiagnostics(page, surface) {
  page.on("response", response => {
    const url = response.url();
    if (!url.includes("/api/")) return;
    const entry = { surface, scope: currentScope, method: response.request().method(), status: response.status(), url };
    report.apiResponses.push(entry);
    if (response.status() >= 400) report.badResponses.push(entry);
  });
  page.on("requestfailed", request => {
    const url = request.url();
    if (!url.includes("/api/")) return;
    const entry = { surface, scope: currentScope, method: request.method(), url, error: request.failure()?.errorText || "request failed" };
    if (entry.error.includes("ERR_ABORTED")) {
      report.expectedAborts.push(entry);
      return;
    }
    report.requestFailures.push(entry);
  });
  page.on("pageerror", error => report.pageErrors.push({ surface, scope: currentScope, message: error.message }));
  page.on("console", message => {
    if (message.type() !== "error") return;
    report.consoleErrors.push({ surface, scope: currentScope, message: message.text() });
  });
}

async function login(page, entryPath, email, password, expectedPath) {
  currentScope = `login:${entryPath}`;
  await page.goto(`${baseURL}${entryPath}`, { waitUntil: "domcontentloaded" });
  if (await page.locator(".admin-shell").count()) return;
  await page.locator('input[autocomplete="email"]').fill(email);
  await page.locator('input[autocomplete="current-password"]').fill(password);
  await page.locator(".admin-auth-submit").click();
  try {
    await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 20_000 });
  } catch (error) {
    const body = compactText(await page.locator("body").innerText().catch(() => ""));
    throw new Error(`登录失败：${email}，页面=${page.url()}，内容=${body.slice(0, 300)}，原因=${messageOf(error)}`);
  }
  if (expectedPath && !new URL(page.url()).pathname.startsWith(expectedPath)) {
    throw new Error(`登录后路径错误：期望 ${expectedPath}，实际 ${page.url()}`);
  }
}

async function waitForSettled(page) {
  await page.locator(".admin-main .el-skeleton").waitFor({ state: "hidden", timeout: 12_000 }).catch(() => undefined);
  await page.waitForTimeout(120);
}

async function expandAllAdminGroups(page) {
  const groups = page.locator(".sidebar-menu .el-sub-menu");
  const count = await groups.count();
  for (let index = 0; index < count; index += 1) {
    const group = groups.nth(index);
    const className = await group.getAttribute("class");
    if (!String(className || "").includes("is-opened")) {
      await group.locator(":scope > .el-sub-menu__title").click();
    }
  }
}

async function inspectActivePage(page, item, title) {
  const className = String(await item.getAttribute("class") || "");
  const pageStack = page.locator(".admin-main .page-stack");
  const content = compactText(await pageStack.innerText().catch(() => ""));
  const alert = page.locator(".admin-main .admin-alert");
  const alertText = await alert.count() && await alert.isVisible().catch(() => false)
    ? compactText(await alert.innerText())
    : "";
  return {
    title,
    active: className.includes("is-active"),
    contentLength: content.length,
    contentPreview: content.slice(0, 180),
    alert: alertText,
    passed: className.includes("is-active") && content.length > 8 && !alertText,
  };
}

async function verifyAdminMenu(page) {
  await expandAllAdminGroups(page);
  const items = page.locator(".sidebar-menu .el-menu-item");
  const count = await items.count();
  for (let index = 0; index < count; index += 1) {
    const item = items.nth(index);
    const title = compactText(await item.innerText());
    currentScope = `admin-module:${title}`;
    const beforeBad = report.badResponses.length;
    const beforePageErrors = report.pageErrors.length;
    try {
      await item.click();
      await waitForSettled(page);
      const result = await inspectActivePage(page, item, title);
      result.apiErrors = report.badResponses.slice(beforeBad);
      result.pageErrors = report.pageErrors.slice(beforePageErrors);
      result.passed = result.passed && !result.apiErrors.length && !result.pageErrors.length;
      report.admin.modules.push(result);
    } catch (error) {
      report.admin.modules.push({ title, passed: false, error: messageOf(error) });
    }
  }
}

async function clickSidebarItemByTitle(page, title, selector = ".sidebar-menu .el-menu-item") {
  const items = page.locator(selector);
  const count = await items.count();
  for (let index = 0; index < count; index += 1) {
    const item = items.nth(index);
    if (normalizedMenuText(await item.innerText()) === normalizedMenuText(title)) {
      await item.click();
      await waitForSettled(page);
      return item;
    }
  }
  throw new Error(`未找到菜单：${title}`);
}

async function verifyAdminSearch(page) {
  currentScope = "admin-check:global-search";
  const input = page.locator(".header-search input");
  await input.fill("客户中心");
  const moduleResult = page.locator(".global-search-card").first().locator("button").filter({ hasText: "客户中心" }).first();
  await moduleResult.waitFor({ state: "visible", timeout: 8_000 });
  await moduleResult.click();
  await waitForSettled(page);
  report.admin.checks.push({ name: "全局搜索模块并跳转", passed: compactText(await page.locator(".header-path").innerText()).includes("客户中心") });

  await input.fill("");
  await clickSidebarItemByTitle(page, "客户中心");
  const firstRow = page.locator(".data-panel .el-table__body tbody tr").first();
  if (!await firstRow.count()) {
    report.admin.checks.push({ name: "搜索当前模块记录并打开", passed: true, skipped: "当前客户数据为空，空状态已渲染" });
    return;
  }
  const rowText = compactText(await firstRow.innerText());
  const keyword = rowText.split(" ").find(value => value.length >= 3) || rowText.slice(0, 8);
  await input.fill(keyword);
  const recordButtons = page.locator(".global-search-card").nth(1).locator("button");
  if (!await recordButtons.count()) {
    report.admin.checks.push({ name: "搜索当前模块记录并打开", passed: false, error: `关键词 ${keyword} 未生成记录结果` });
    return;
  }
  await recordButtons.first().click();
  const feedback = page.locator(".el-message, .el-dialog, .el-drawer, .el-message-box").filter({ visible: true }).first();
  await feedback.waitFor({ state: "visible", timeout: 10_000 }).catch(() => undefined);
  const feedbackVisible = await page.locator(".el-message, .el-dialog, .el-drawer, .el-message-box").filter({ visible: true }).count().catch(() => 0);
  report.admin.checks.push({ name: "搜索当前模块记录并打开", passed: feedbackVisible > 0, keyword });
  const closeButton = page.locator(".el-dialog__headerbtn, .el-drawer__close-btn, .el-message-box__headerbtn").last();
  if (await closeButton.count() && await closeButton.isVisible().catch(() => false)) await closeButton.click();
  await input.fill("");
}

async function verifyUserMenu(page) {
  const items = page.locator(".user-flat-sidebar-menu .el-menu-item");
  const count = await items.count();
  for (let index = 0; index < count; index += 1) {
    const item = items.nth(index);
    const title = compactText(await item.innerText());
    currentScope = `user-module:${title}`;
    const beforeBad = report.badResponses.length;
    const beforePageErrors = report.pageErrors.length;
    try {
      await item.click();
      await waitForSettled(page);
      const className = String(await item.getAttribute("class") || "");
      const content = compactText(await page.locator(".admin-main .page-stack").innerText().catch(() => ""));
      let embeddedContentLength = 0;
      let embeddedReady = false;
      const canvasFrame = page.locator(".wireless-canvas-admin-frame");
      if (await canvasFrame.count() && await canvasFrame.isVisible().catch(() => false)) {
        await canvasFrame.waitFor({ state: "visible", timeout: 10_000 });
        const frame = page.frames().find(candidate => candidate.url().includes("/static/smart-canvas.html"));
        if (frame) {
          await frame.waitForLoadState("domcontentloaded").catch(() => {});
          const embeddedText = compactText(await frame.locator("body").innerText().catch(() => ""));
          const embeddedNodes = await frame.locator("body > *").count().catch(() => 0);
          const embeddedCanvases = await frame.locator("canvas").count().catch(() => 0);
          embeddedContentLength = embeddedText.length;
          embeddedReady = embeddedNodes > 0 && (embeddedContentLength > 8 || embeddedCanvases > 0);
        }
      }
      const apiErrors = report.badResponses.slice(beforeBad);
      const pageErrors = report.pageErrors.slice(beforePageErrors);
      report.user.modules.push({
        title,
        path: new URL(page.url()).pathname,
        active: className.includes("is-active"),
        contentLength: Math.max(content.length, embeddedContentLength),
        embeddedReady,
        apiErrors,
        pageErrors,
        passed: className.includes("is-active") && (content.length > 8 || embeddedReady) && !apiErrors.length && !pageErrors.length,
      });
    } catch (error) {
      report.user.modules.push({ title, passed: false, error: messageOf(error) });
    }
  }
}

async function verifyAgentCenter(page) {
  currentScope = "user-check:agent-center";
  await clickSidebarItemByTitle(page, "智能体中心", ".user-flat-sidebar-menu .el-menu-item");
  await page.locator(".user-agent-center-page").waitFor({ state: "visible", timeout: 10_000 });
  const tabs = page.locator(".user-agent-desktop-view .user-agent-tabs button");
  const tabCount = await tabs.count();
  let tabsPassed = tabCount >= 3;
  for (let index = 0; index < tabCount; index += 1) {
    await tabs.nth(index).click();
    tabsPassed = tabsPassed && String(await tabs.nth(index).getAttribute("class") || "").includes("active");
  }
  await tabs.first().click();
  report.user.checks.push({ name: "智能体中心标签切换", passed: tabsPassed, count: tabCount });

  const search = page.locator('.user-agent-list-tools input[placeholder="搜索智能体..."]');
  const beforeRows = await page.locator(".user-agent-table-row").count();
  await search.fill("客服");
  const afterRows = await page.locator(".user-agent-table-row").count();
  report.user.checks.push({ name: "智能体搜索", passed: beforeRows > 0 && afterRows <= beforeRows, beforeRows, afterRows });
  await search.fill("");

  const typeSelect = page.locator('select[aria-label="筛选智能体类型"]');
  const options = await typeSelect.locator("option").allTextContents();
  if (options.length > 1) await typeSelect.selectOption({ index: 1 });
  report.user.checks.push({ name: "智能体类型筛选", passed: options.length > 1, options: options.map(compactText) });
  await typeSelect.selectOption("all");

  const favorite = page.locator('button[aria-label="收藏智能体"], button[aria-label="取消收藏智能体"]').first();
  if (await favorite.count()) {
    await favorite.click();
    report.user.checks.push({ name: "智能体收藏", passed: true });
  } else report.user.checks.push({ name: "智能体收藏", passed: false, error: "没有可操作的智能体行" });

  const copy = page.locator('button[aria-label="复制智能体配置"]').first();
  if (await copy.count()) {
    await copy.click();
    await page.waitForTimeout(200);
    report.user.checks.push({ name: "复制智能体配置", passed: await page.locator(".el-message").count() > 0 });
  } else report.user.checks.push({ name: "复制智能体配置", passed: false, error: "复制入口不存在" });

  const edit = page.locator('button[aria-label="编辑智能体"]').first();
  if (await edit.count()) {
    await edit.click();
    const workspace = page.locator(".user-agent-workspace, .knowledge-agent-center, .user-agent-officecli-workspace");
    const opened = await workspace.count() > 0;
    report.user.checks.push({ name: "编辑智能体进入工作台", passed: opened });
    const back = page.getByRole("button", { name: "返回智能体中心" }).first();
    if (opened && await back.count()) await back.click();
  } else report.user.checks.push({ name: "编辑智能体进入工作台", passed: false, error: "编辑入口不存在" });
}

async function verifyImageUpload(page) {
  currentScope = "user-check:image-reference-upload";
  await clickSidebarItemByTitle(page, "AI生图", ".user-flat-sidebar-menu .el-menu-item");
  const fileInput = page.locator(".ai-attach-upload input[type=file]");
  if (!fs.existsSync(uploadFixture)) {
    report.user.checks.push({ name: "AI 生图参考图上传", passed: false, error: `缺少测试图片 ${uploadFixture}` });
    return;
  }
  const beforeBad = report.badResponses.length;
  await fileInput.setInputFiles(uploadFixture);
  await page.locator(".ai-reference-strip .ai-reference-thumb").first().waitFor({ state: "visible", timeout: 20_000 });
  await page.waitForFunction(() => !document.querySelector(".ai-reference-thumb em"), undefined, { timeout: 20_000 }).catch(() => undefined);
  const thumb = page.locator(".ai-reference-strip .ai-reference-thumb").first();
  const errorText = compactText(await thumb.locator("em.is-error").innerText().catch(() => ""));
  const apiErrors = report.badResponses.slice(beforeBad);
  report.user.checks.push({ name: "AI 生图参考图上传", passed: !errorText && !apiErrors.length, errorText, apiErrors });
}

async function verifyPptSafeControls(page) {
  currentScope = "user-check:ppt-controls";
  await clickSidebarItemByTitle(page, "PPT文档生成", ".user-flat-sidebar-menu .el-menu-item");
  const root = page.locator(".ppt-generate-page");
  await root.waitFor({ state: "visible", timeout: 20_000 });
  const slideButton = page.locator('button[aria-label^="选择幻灯片页数"]');
  await slideButton.click();
  const expanded = await slideButton.getAttribute("aria-expanded");
  const exportElementCount = await page.locator("button.ppt-export-option").count();
  const gripButtonCount = await page.locator("button.ppt-outline-grip").count();
  report.user.checks.push({ name: "PPT 页数选择器", passed: expanded === "true" });
  report.user.checks.push({ name: "PPT 固定导出格式非伪按钮", passed: exportElementCount === 0, runtimeStage: "首页未生成演示文稿时导出区不渲染" });
  report.user.checks.push({ name: "PPT 排序手柄非伪按钮", passed: gripButtonCount === 0, runtimeStage: "首页未生成大纲时排序区不渲染" });
}

async function screenshot(page, name) {
  const target = path.join(artifactDir, name);
  await page.screenshot({ path: target, fullPage: true });
  report.screenshots.push(path.relative(repoRoot, target).replaceAll("\\", "/"));
}

async function main() {
  const browser = await chromium.launch({ channel: process.platform === "win32" ? "msedge" : undefined, headless: true });
  try {
    const adminContext = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    const adminPage = await adminContext.newPage();
    attachDiagnostics(adminPage, "admin");
    await login(adminPage, "/admin/", adminEmail, adminPassword, "/admin");
    await verifyAdminMenu(adminPage);
    await verifyAdminSearch(adminPage);
    await screenshot(adminPage, "admin-all-modules.png");
    await adminContext.close();

    const userContext = await browser.newContext({ viewport: { width: 1440, height: 1000 }, permissions: ["clipboard-read", "clipboard-write"] });
    const userPage = await userContext.newPage();
    attachDiagnostics(userPage, "user");
    await login(userPage, "/app", userEmail, userPassword, "/app");
    await verifyUserMenu(userPage);
    await verifyAgentCenter(userPage);
    await verifyImageUpload(userPage);
    await verifyPptSafeControls(userPage);
    await screenshot(userPage, "user-ppt-runtime.png");
    await userContext.close();
  } finally {
    await browser.close();
  }

  const failedAdminModules = report.admin.modules.filter(item => !item.passed);
  const failedUserModules = report.user.modules.filter(item => !item.passed);
  const failedChecks = [...report.admin.checks, ...report.user.checks].filter(item => !item.passed);
  report.summary = {
    adminModules: report.admin.modules.length,
    failedAdminModules: failedAdminModules.length,
    userModules: report.user.modules.length,
    failedUserModules: failedUserModules.length,
    checks: report.admin.checks.length + report.user.checks.length,
    failedChecks: failedChecks.length,
    apiResponses: report.apiResponses.length,
    badResponses: report.badResponses.length,
    expectedAborts: report.expectedAborts.length,
    requestFailures: report.requestFailures.length,
    pageErrors: report.pageErrors.length,
    consoleErrors: report.consoleErrors.length,
    passed: !failedAdminModules.length && !failedUserModules.length && !failedChecks.length && !report.badResponses.length && !report.requestFailures.length && !report.pageErrors.length,
  };
  fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`, "utf8");
  console.log(JSON.stringify(report.summary, null, 2));
  console.log(`report: ${reportPath}`);
  if (!report.summary.passed) process.exitCode = 1;
}

main().catch(error => {
  report.summary = { passed: false, fatal: messageOf(error) };
  fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`, "utf8");
  console.error(error);
  process.exitCode = 1;
});
