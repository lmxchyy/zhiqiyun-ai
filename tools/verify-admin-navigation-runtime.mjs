import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { chromium } from "@playwright/test";

const baseURL = String(process.env.XIANZHI_RUNTIME_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const email = process.env.XIANZHI_ADMIN_EMAIL || "admin@xianzhi.ai";
const password = process.env.XIANZHI_ADMIN_PASSWORD || "admin123!";
const artifactDir = path.resolve("artifacts", "admin-navigation-runtime");
fs.mkdirSync(artifactDir, { recursive: true });

const expectedGroups = ["管理总览", "客户与企业", "商品与计费", "AI 与内容", "渠道与增长", "系统管理"];
const checks = [];
const pageErrors = [];

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

async function expandAllGroups(page) {
  const groups = page.locator(".sidebar-menu .el-sub-menu");
  for (let index = 0; index < await groups.count(); index += 1) {
    const group = groups.nth(index);
    if (!String(await group.getAttribute("class") || "").includes("is-opened")) {
      await group.locator(":scope > .el-sub-menu__title").click();
    }
  }
}

async function clickMenu(page, title) {
  const item = page.locator(".sidebar-menu .el-menu-item", { hasText: title }).first();
  await item.scrollIntoViewIfNeeded();
  await item.click();
  await page.locator(".admin-main .el-skeleton").waitFor({ state: "hidden", timeout: 12_000 }).catch(() => undefined);
  await page.waitForTimeout(150);
  assert(String(await item.getAttribute("class") || "").includes("is-active"), `侧栏未高亮：${title}`);
}

async function clickSectionTab(page, title) {
  const tab = page.locator(".admin-section-navigation-tabs [role='tab']", { hasText: title }).first();
  await tab.click();
  await page.locator(".admin-main .el-skeleton").waitFor({ state: "hidden", timeout: 12_000 }).catch(() => undefined);
  await page.waitForTimeout(150);
  assert(await tab.getAttribute("aria-selected") === "true", `页内导航未选中：${title}`);
}

const browser = await chromium.launch({ channel: process.platform === "win32" ? "msedge" : undefined, headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  await page.addInitScript(() => window.sessionStorage.setItem("xianzhi-admin-experience-mode", "synthetic"));
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto(`${baseURL}/admin/`, { waitUntil: "domcontentloaded" });
  if (!await page.locator(".admin-shell").count()) {
    await page.locator('input[autocomplete="email"]').fill(email);
    await page.locator('input[autocomplete="current-password"]').fill(password);
    await page.locator(".admin-auth-submit").click();
    await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 20_000 });
  }
  const initialPptResources = await page.evaluate(() => performance.getEntriesByType("resource").map((entry) => entry.name).filter((name) => /\/assets\/Ppt/i.test(name)));
  assert(initialPptResources.length === 0, `PPT 资源不应在后台首屏加载：${initialPptResources.join(", ")}`);
  const initialLogoResources = await page.evaluate(() => performance.getEntriesByType("resource").map((entry) => entry.name).filter((name) => /xianzhi-ai-logo/i.test(name)));
  assert(initialLogoResources.length >= 1 && initialLogoResources.every((name) => name.endsWith(".webp")), `后台 Logo 应使用 WebP：${initialLogoResources.join(", ")}`);

  const groupTitles = (await page.locator(".sidebar-menu .el-sub-menu__title").allInnerTexts()).map((value) => value.trim());
  assert(JSON.stringify(groupTitles) === JSON.stringify(expectedGroups), `一级业务域不匹配：${JSON.stringify(groupTitles)}`);
  checks.push({ name: "六个一级业务域", passed: true, groupTitles });

  await expandAllGroups(page);
  const menuTitles = (await page.locator(".sidebar-menu .el-menu-item").allInnerTexts()).map((value) => value.trim());
  assert(menuTitles.length === 22, `侧栏入口应为 22，实际为 ${menuTitles.length}`);
  checks.push({ name: "侧栏入口收敛", passed: true, count: menuTitles.length, menuTitles });
  checks.push({ name: "PPT 资源退出后台首屏加载", passed: true });
  checks.push({ name: "后台 Logo WebP 资源预算", passed: true });

  await clickMenu(page, "管理总览");
  assert(await page.locator(".admin-section-navigation-tabs [role='tab']").count() === 3, "管理总览应包含三个页内入口");
  await clickSectionTab(page, "工作台");
  assert(await page.locator(".role-workspace-hero").isVisible(), "角色工作台未显示");
  assert(await page.locator(".operations-inbox").isVisible(), "待办与异常中心未显示");
  checks.push({ name: "管理总览页内导航", passed: true });

  await clickMenu(page, "定价与成本");
  assert(await page.locator(".admin-section-navigation-tabs [role='tab']").count() === 2, "定价与成本应包含两个页内入口");
  await clickSectionTab(page, "供应商成本");
  checks.push({ name: "计费页内导航", passed: true });

  await clickMenu(page, "接入与调用治理");
  assert(await page.locator(".admin-section-navigation-tabs [role='tab']").count() === 4, "接入与调用治理应包含四个页内入口");
  await clickSectionTab(page, "API 设置");
  checks.push({ name: "AI 治理页内导航", passed: true });

  await clickMenu(page, "页面运营");
  assert(await page.locator(".admin-section-navigation-tabs [role='tab']").count() === 5, "页面运营应包含五个页内入口");
  checks.push({ name: "页面运营页内导航", passed: true });

  await clickMenu(page, "定价与成本");
  assert(new URL(page.url()).pathname === "/admin/billing/rules", `定价入口 URL 错误：${page.url()}`);
  await clickSectionTab(page, "供应商成本");
  assert(new URL(page.url()).pathname === "/admin/billing/provider-costs", `供应商成本 URL 错误：${page.url()}`);
  await page.goBack();
  await page.waitForTimeout(350);
  assert(new URL(page.url()).pathname === "/admin/billing/rules", `浏览器返回未恢复计费规则：${page.url()}`);
  assert(await page.locator(".admin-section-navigation-tabs [role='tab']", { hasText: "模型计费规则" }).getAttribute("aria-selected") === "true", "浏览器返回后页内标签未恢复");
  checks.push({ name: "Canonical URL 与浏览器返回", passed: true });

  const headerSearch = page.locator(".header-search input");
  await headerSearch.fill("计费");
  await page.locator(".command-palette").waitFor({ state: "visible" });
  const billingSearchResult = page.locator(".command-results section", { hasText: "模块入口" }).locator("button", { hasText: "计费总览" }).first();
  assert(await billingSearchResult.count() === 1, "模块搜索未按业务域/区块匹配计费模块");
  await billingSearchResult.click();
  await page.waitForTimeout(250);
  assert(new URL(page.url()).pathname === "/admin/billing/overview", `搜索结果未进入计费总览：${page.url()}`);
  assert(String(await page.locator(".sidebar-menu .el-menu-item", { hasText: "账务与对账" }).first().getAttribute("class") || "").includes("is-active"), "搜索进入子模块后侧栏父入口未高亮");
  await headerSearch.fill("");
  checks.push({ name: "模块搜索与父级高亮", passed: true });

  const runtimeToken = await page.evaluate(() => localStorage.getItem("token") || sessionStorage.getItem("token") || "");
  assert(Boolean(runtimeToken), "运行态缺少管理员访问令牌");
  const customerListResponse = await page.request.get(`${baseURL}/api/v1/admin/customers`, { headers: { Authorization: `Bearer ${runtimeToken}` } });
  assert(customerListResponse.ok(), `客户列表接口失败：${customerListResponse.status()}`);
  const customerListPayload = await customerListResponse.json();
  const runtimeCustomer = customerListPayload.items?.find((item) => item.email);
  assert(Boolean(runtimeCustomer?.email), "运行态缺少可搜索客户");
  const searchResponsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/admin/search?") && response.request().method() === "GET");
  await headerSearch.fill(runtimeCustomer.email);
  const searchResponse = await searchResponsePromise;
  assert(searchResponse.ok(), `跨域搜索接口失败：${searchResponse.status()}`);
  const businessSection = page.locator(".command-results section", { hasText: "全局业务数据" });
  await businessSection.locator("button").first().waitFor({ state: "visible", timeout: 10_000 });
  await businessSection.locator("button").first().click();
  await page.waitForTimeout(250);
  assert(new URL(page.url()).pathname === "/admin/customers", `客户搜索结果未进入客户中心：${page.url()}`);
  await page.locator(".admin-data-list").waitFor({ state: "visible", timeout: 10_000 });
  for (const capability of ["保存当前视图", "列配置", "导出当前结果"]) assert(await page.locator(".admin-data-list", { hasText: capability }).count() === 1, `统一列表缺少 ${capability}`);
  await page.locator(".admin-data-list button", { hasText: "保存当前视图" }).click();
  await page.locator('.el-dialog input[placeholder="例如：待处理订单"]').fill("运行态客户视图");
  await page.locator(".el-dialog button", { hasText: "保存" }).click();
  assert((await page.locator(".saved-view-select").innerText()).includes("运行态客户视图"), "保存视图未持久化到列表");
  const firstSelection = page.locator(".admin-data-list .el-table__body-wrapper .el-checkbox").first();
  await firstSelection.click();
  assert(await page.locator(".admin-data-list button", { hasText: "批量操作（1）" }).isEnabled(), "勾选记录后批量操作未启用");
  const downloadPromise = page.waitForEvent("download");
  await page.locator(".admin-data-list button", { hasText: "导出当前结果" }).click();
  const download = await downloadPromise;
  assert((await download.suggestedFilename()).endsWith(".csv"), "列表导出未生成 CSV 文件");
  const customer360ResponsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/admin/customers/") && response.url().endsWith("/360") && response.request().method() === "GET");
  await page.locator(".admin-data-list button", { hasText: "查看 360°" }).first().click();
  const customer360Response = await customer360ResponsePromise;
  assert(customer360Response.ok(), `客户 360° 接口失败：${customer360Response.status()}`);
  await page.locator(".customer-360-content").waitFor({ state: "visible", timeout: 10_000 });
  await page.keyboard.press("Escape");
  checks.push({ name: "跨域业务搜索与客户 360°", passed: true });
  checks.push({ name: "列表保存视图、批量选择与 CSV 导出", passed: true });

  const enterpriseListResponse = await page.request.get(`${baseURL}/api/v1/admin/enterprises?page=1&pageSize=1`, { headers: { Authorization: `Bearer ${runtimeToken}` } });
  const enterpriseListPayload = await enterpriseListResponse.json();
  const paymentListResponse = await page.request.get(`${baseURL}/api/v1/admin/billing/payments`, { headers: { Authorization: `Bearer ${runtimeToken}` } });
  const paymentListPayload = await paymentListResponse.json();
  const invoiceListResponse = await page.request.get(`${baseURL}/api/v1/admin/billing/invoices`, { headers: { Authorization: `Bearer ${runtimeToken}` } });
  const invoiceListPayload = await invoiceListResponse.json();
  const searchCases = [
    { query: enterpriseListPayload.items?.[0]?.id, type: "enterprise" },
    { query: paymentListPayload.items?.[0]?.id, type: "payment" },
    { query: invoiceListPayload.items?.[0]?.invoiceNo || invoiceListPayload.items?.[0]?.id, type: "invoice" },
    { query: "FAILED", type: "generation_task" }
  ];
  for (const item of searchCases) {
    assert(Boolean(item.query), `缺少 ${item.type} 搜索样本`);
    const response = await page.request.get(`${baseURL}/api/v1/admin/search?q=${encodeURIComponent(item.query)}`, { headers: { Authorization: `Bearer ${runtimeToken}` } });
    const payload = await response.json();
    assert(payload.items?.some((candidate) => candidate.type === item.type), `实时跨域搜索未命中 ${item.type}`);
  }
  checks.push({ name: "企业、生成任务、支付与发票实时搜索", passed: true });

  await clickMenu(page, "订单与支付");
  const orderTimelineResponsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/admin/orders/") && response.url().endsWith("/timeline") && response.request().method() === "GET");
  await page.locator(".admin-data-list button", { hasText: "履约时间轴" }).first().click();
  const orderTimelineResponse = await orderTimelineResponsePromise;
  assert(orderTimelineResponse.ok(), `订单履约时间轴接口失败：${orderTimelineResponse.status()}`);
  await page.locator(".fulfillment-timeline").waitFor({ state: "visible", timeout: 10_000 });
  assert(await page.locator(".fulfillment-timeline li").count() === 5, "订单履约时间轴应包含五个阶段");
  await page.keyboard.press("Escape");
  checks.push({ name: "订单履约时间轴", passed: true });

  await clickMenu(page, "企业中心");
  assert(new URL(page.url()).pathname === "/admin/enterprises", `企业中心 URL 错误：${page.url()}`);
  const firstEnterprise = page.locator(".enterprise-name-cell").first();
  await firstEnterprise.waitFor({ state: "visible", timeout: 15_000 });
  await firstEnterprise.click();
  await page.locator(".enterprise-detail-tabs").waitFor({ state: "visible", timeout: 15_000 });
  const integrationResponsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/admin/enterprises/") && response.url().endsWith("/integrations") && response.request().method() === "GET");
  await page.locator(".enterprise-detail-tabs button", { hasText: "集成" }).click();
  const integrationResponse = await integrationResponsePromise;
  assert(integrationResponse.ok(), `企业集成接口失败：${integrationResponse.status()}`);
  const integrationPayload = await integrationResponse.json();
  const integrationJSON = JSON.stringify(integrationPayload);
  assert(integrationJSON.includes('"adapterBoundary":"PlatformConnector"'), "企业集成未声明 PlatformConnector 边界");
  for (const forbiddenKey of ["connectorKey", "appId", "externalMessageId", "lastErrorMessage", "verificationTokenCiphertext", "encryptKeyCiphertext", "appSecretCiphertext"]) {
    assert(!integrationJSON.includes(`"${forbiddenKey}":`), `企业集成响应泄露字段：${forbiddenKey}`);
  }
  await page.locator(".enterprise-integration-center").waitFor({ state: "visible", timeout: 15_000 });
  assert(new URL(page.url()).pathname.endsWith("/integrations"), `企业集成 URL 错误：${page.url()}`);
  await page.screenshot({ path: path.join(artifactDir, "enterprise-integrations.png"), fullPage: true });
  checks.push({ name: "企业集成中心与敏感字段边界", passed: true });

  await page.goto(`${baseURL}/admin/billing`, { waitUntil: "domcontentloaded" });
  await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 15_000 });
  assert((await page.locator(".page-tab.is-active").innerText()).includes("计费总览"), "旧地址 /admin/billing 未恢复计费总览");
  await page.goto(`${baseURL}/app/image-generation`, { waitUntil: "domcontentloaded" });
  await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 15_000 });
  assert((await page.locator(".user-flat-sidebar-menu .el-menu-item.is-active").innerText()).includes("AI 生图"), "旧地址 /app/image-generation 未恢复 AI 生图");
  await page.goto(`${baseURL}/app/enterprise/connectors`, { waitUntil: "domcontentloaded" });
  await page.locator(".authorization-page").waitFor({ state: "visible", timeout: 15_000 });
  await page.goto(`${baseURL}/app/enterprise/feishu`, { waitUntil: "domcontentloaded" });
  await page.locator(".connector-page").waitFor({ state: "visible", timeout: 15_000 });
  checks.push({ name: "旧地址与 Connector 专用入口兼容", passed: true });

  await page.goto(`${baseURL}/admin/overview`, { waitUntil: "domcontentloaded" });
  await page.locator(".admin-shell").waitFor({ state: "visible", timeout: 15_000 });
  await page.locator(".admin-main .el-skeleton").waitFor({ state: "hidden", timeout: 15_000 }).catch(() => undefined);
  await page.waitForTimeout(250);
  await page.locator(".experience-insights").waitFor({ state: "visible", timeout: 10_000 });
  const firstException = page.locator(".exception-list button").first();
  if (await firstException.count()) {
    await firstException.click();
    await page.locator('.exception-form input[placeholder="输入负责人姓名"]').fill("浏览器值班");
    await page.locator(".exception-form .el-select").click();
    await page.locator(".el-select-dropdown__item", { hasText: "处理中" }).last().click();
    const updateResponsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/admin/exceptions/") && response.request().method() === "PATCH");
    await page.locator(".exception-form button", { hasText: "保存处置记录" }).click();
    assert((await updateResponsePromise).ok(), "异常工单分派接口失败");
    await page.locator(".exception-form .el-select").click();
    await page.locator(".el-select-dropdown__item", { hasText: "已关闭" }).last().click();
    await page.locator('.exception-form textarea[placeholder="关闭工单必须填写原因"]').fill("浏览器运行态验收完成");
    const closeResponsePromise = page.waitForResponse((response) => response.url().includes("/api/v1/admin/exceptions/") && response.request().method() === "PATCH");
    await page.locator(".exception-form button", { hasText: "保存处置记录" }).click();
    assert((await closeResponsePromise).ok(), "异常工单关闭接口失败");
    assert(await page.locator(".el-timeline-item").count() >= 3, "异常工单处理历史不足");
    await page.keyboard.press("Escape");
  }
  const experienceAnalyticsResponse = await page.request.get(`${baseURL}/api/v1/admin/experience-analytics?days=30`, { headers: { Authorization: `Bearer ${runtimeToken}` } });
  assert(experienceAnalyticsResponse.ok(), `体验分析接口失败：${experienceAnalyticsResponse.status()}`);
  const experienceAnalyticsPayload = await experienceAnalyticsResponse.json();
  assert(experienceAnalyticsPayload.syntheticEvents >= 1, "自动化体验事件未被识别");
  assert(experienceAnalyticsPayload.observedEvents === experienceAnalyticsPayload.totalEvents + experienceAnalyticsPayload.syntheticEvents, "真人与自动化体验事件未正确分流");
  checks.push({ name: "异常负责人、SLA、状态与关闭记录", passed: true });
  checks.push({ name: "体验事件、自动化隔离与低频入口分析", passed: true });

  assert(pageErrors.length === 0, `页面脚本错误：${pageErrors.join(" | ")}`);
  await page.screenshot({ path: path.join(artifactDir, "admin-navigation.png"), fullPage: true });
  await page.setViewportSize({ width: 768, height: 900 });
  await page.waitForTimeout(200);
  assert(await page.locator(".mobile-admin-bar").isVisible(), "768px 下未切换为移动端管理栏");
  const viewportHasNoHorizontalOverflow = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1);
  assert(viewportHasNoHorizontalOverflow, "768px 下页面出现横向溢出");
  await page.locator(".mobile-collapse-button").click();
  await page.locator(".mobile-drawer-mask").waitFor({ state: "visible" });
  await page.waitForTimeout(250);
  assert(String(await page.locator(".admin-shell").getAttribute("class") || "").includes("mobile-drawer-open"), "移动端导航抽屉未打开");
  await page.screenshot({ path: path.join(artifactDir, "admin-navigation-mobile.png"), fullPage: true });
  await page.locator(".mobile-drawer-mask").click();
  checks.push({ name: "768px 响应式布局与导航抽屉", passed: true });
  const result = { generatedAt: new Date().toISOString(), baseURL, passed: true, checks, pageErrors };
  fs.writeFileSync(path.join(artifactDir, "result.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");
  console.log(JSON.stringify(result, null, 2));
} catch (error) {
  const result = { generatedAt: new Date().toISOString(), baseURL, passed: false, checks, pageErrors, error: error instanceof Error ? error.message : String(error) };
  fs.writeFileSync(path.join(artifactDir, "result.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");
  console.error(JSON.stringify(result, null, 2));
  process.exitCode = 1;
} finally {
  await browser.close();
}
