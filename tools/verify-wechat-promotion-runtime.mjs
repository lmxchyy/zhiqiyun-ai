import fs from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");
const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const projectPath = path.join(repoRoot, "apps", "user-uni", "dist", "build", "mp-weixin");
const cliPath = "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const automationPort = process.env.WECHAT_PROMOTION_AUTOMATION_PORT || "9431";
const endpoint = `ws://127.0.0.1:${automationPort}`;
const idePort = process.env.WECHAT_IDE_PORT || "33709";
const artifactDir = path.join(repoRoot, "artifacts", "wechat-promotion-runtime");
fs.mkdirSync(artifactDir, { recursive: true });
const progressPath = path.join(artifactDir, "progress.log");
fs.writeFileSync(progressPath, "", "utf8");

const accounts = {
  agent: { email: "agent1@xianzhi.ai", password: "Agent123!", role: "AGENT", roleLabel: "推广伙伴" },
  operation: { email: "operation@xianzhi.ai", password: "Demo123!", role: "OPERATION", roleLabel: "运营中心" },
};

function assert(condition, message) { if (!condition) throw new Error(message); }
function wait(ms = 500) { return new Promise(resolve => setTimeout(resolve, ms)); }
function progress(message) { const line = `${new Date().toISOString()} ${message}`; fs.appendFileSync(progressPath, `${line}\n`, "utf8"); console.log(line); }
function timeout(label, promise, milliseconds = 15000) { return Promise.race([promise, wait(milliseconds).then(() => { throw new Error(`${label} 超时`); })]); }

async function connect() {
  const output = [];
  const launch = port => {
    const command = `& '${cliPath.replaceAll("'", "''")}' auto --project '${projectPath.replaceAll("'", "''")}' --port ${port} --auto-port ${automationPort} --trust-project`;
    const child = spawn("powershell.exe", ["-NoProfile", "-Command", command], { cwd: repoRoot, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] });
    child.stdout.on("data", chunk => output.push(String(chunk)));
    child.stderr.on("data", chunk => output.push(String(chunk)));
  };
  launch(idePort);
  let activeIdePort = idePort;
  let lastError;
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try { return await automator.connect({ wsEndpoint: endpoint }); }
    catch (error) {
      lastError = error;
      const match = output.join("").match(/IDE server has started on http:\/\/127\.0\.0\.1:(\d+)/i);
      if (match?.[1] && match[1] !== activeIdePort) { activeIdePort = match[1]; launch(activeIdePort); }
      await wait(500);
    }
  }
  throw new Error(`微信自动化连接失败: ${lastError instanceof Error ? lastError.message : String(lastError)}\n${output.join("").slice(-1500)}`);
}

async function api(pathname, init = {}) {
  const response = await fetch(`http://127.0.0.1:3100${pathname}`, { ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = await response.json();
  if (!response.ok) throw new Error(`${pathname}: ${response.status} ${JSON.stringify(payload)}`);
  return payload?.data || payload;
}

async function useAccount(miniProgram, account, role = account.role) {
  progress(`account:${account.email}:${role}`);
  const auth = await timeout("账号登录", api("/api/v1/auth/login", { method: "POST", body: JSON.stringify({ email: account.email, password: account.password }) }));
  progress(`account:${account.email}:login-ready`);
  const profile = await timeout("角色切换", api("/api/v1/user/current-role", { method: "POST", headers: { Authorization: `Bearer ${auth.accessToken}` }, body: JSON.stringify({ role }) }));
  progress(`account:${account.email}:role-ready`);
  const stored = { ...auth, tenantId: profile.tenantId, organizationId: profile.organizationId, roles: profile.roles, currentRole: profile.currentRole, permissions: profile.permissions };
  await timeout("写入 token", miniProgram.callWxMethod("setStorageSync", "token", auth.accessToken));
  progress(`account:${account.email}:token-ready`);
  await timeout("写入 auth", miniProgram.callWxMethod("setStorageSync", "auth", stored));
  await timeout("写入兼容登录态", miniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", stored));
  progress(`account:${account.email}:storage-ready`);
  return stored;
}

async function waitForPage(miniProgram, expected, timeout = 15000) {
  const started = Date.now();
  let page;
  while (Date.now() - started < timeout) {
    page = await miniProgram.currentPage();
    if (page?.path === expected) return page;
    await wait(300);
  }
  throw new Error(`等待页面 ${expected} 超时，当前 ${page?.path || "unknown"}`);
}

async function waitForElement(page, selector, timeout = 15000) {
  const started = Date.now();
  let element;
  while (Date.now() - started < timeout) {
    element = await page.$(selector);
    if (element) return element;
    await wait(300);
  }
  throw new Error(`${page.path}: 找不到控件 ${selector}`);
}

async function centered(button) {
  const label = await button.$("text");
  assert(label, "按钮没有 text 标签");
  const [buttonOffset, buttonSize, labelOffset, labelSize] = await Promise.all([button.offset(), button.size(), label.offset(), label.size()]);
  const deltaX = Math.abs((buttonOffset.left + buttonSize.width / 2) - (labelOffset.left + labelSize.width / 2));
  const deltaY = Math.abs((buttonOffset.top + buttonSize.height / 2) - (labelOffset.top + labelSize.height / 2));
  return { deltaX, deltaY, buttonOffset, buttonSize, labelOffset, labelSize };
}

async function openPromotionCenter(miniProgram, result, name, account, role) {
  progress(`promotion:${name}:open`);
  const auth = await useAccount(miniProgram, account, role);
  let page = await miniProgram.reLaunch("/pages/promotion/PromotionCenterPage");
  page = await waitForPage(miniProgram, "pages/promotion/PromotionCenterPage");
  const code = await waitForElement(page, ".promotion-code-image", 20000);
  progress(`promotion:${name}:code-ready`);
  const identity = await waitForElement(page, ".promotion-role-pill");
  const identityText = await identity.text();
  const codeSrc = await code.attribute("src");
  assert(codeSrc, `${name}: 小程序码图片为空`);
  const primary = await waitForElement(page, ".promotion-primary-button");
  const primaryGeometry = await centered(primary);
  assert(primaryGeometry.deltaX <= 3 && primaryGeometry.deltaY <= 3, `${name}: 主按钮文字未居中 ${JSON.stringify(primaryGeometry)}`);
  const secondary = await waitForElement(page, ".promotion-secondary-button");
  const secondaryGeometry = await centered(secondary);
  assert(secondaryGeometry.deltaX <= 3 && secondaryGeometry.deltaY <= 3, `${name}: 次按钮文字未居中 ${JSON.stringify(secondaryGeometry)}`);
  result.roles[name] = { currentRole: auth.currentRole, identityText, codeReady: true, primaryGeometry, secondaryGeometry };
  return page;
}

async function verifyNavigation(miniProgram, result) {
  let page = await openPromotionCenter(miniProgram, result, "agent", accounts.agent, "AGENT");
  const linkRows = await page.$$(".promotion-link-row");
  assert(linkRows.length === 2, `推广中心功能入口数量=${linkRows.length}`);
  await miniProgram.callWxMethod("pageScrollTo", { scrollTop: 1400, duration: 0 });
  await wait(500);
  await linkRows[0].tap();
  progress("navigation:records:tap");
  page = await waitForPage(miniProgram, "pages/promotion/PromotionRecordsPage");
  assert((await page.$$(".promotion-status-tabs button")).length === 4, "推广记录状态 Tab 不完整");
  result.navigation.records = page.path;
  progress("navigation:records:ready");

  page = await miniProgram.reLaunch("/pages/promotion/PromotionCenterPage");
  await waitForElement(page, ".promotion-code-image", 20000);
  const linksAgain = await page.$$(".promotion-link-row");
  await miniProgram.callWxMethod("pageScrollTo", { scrollTop: 1400, duration: 0 });
  await wait(500);
  await linksAgain[1].tap();
  progress("navigation:stats:tap");
  page = await waitForPage(miniProgram, "pages/promotion/PromotionStatsPage");
  assert((await page.$$(".promotion-period-tabs button")).length === 3, "推广数据周期 Tab 不完整");
  result.navigation.stats = page.path;
  progress("navigation:stats:ready");

  page = await miniProgram.reLaunch("/pages/promotion/PromotionTemplateCenterPage");
  const templates = await page.$$("promotion-template-thumb");
  progress(`templates:ready:${templates.length}`);
  assert(templates.length === 10, `代理商可用模板数量=${templates.length}`);
  const targetButton = await templates[2].$(".promotion-template-thumb");
  assert(targetButton, "第三套模板按钮不存在");
  await targetButton.tap();
  const previewButton = await waitForElement(page, ".promotion-fixed-action .promotion-primary-button");
  await previewButton.tap();
  progress("navigation:preview:tap");
  page = await waitForPage(miniProgram, "pages/promotion/PromotionPosterPreviewPage", 20000);
  await waitForElement(page, ".promotion-poster-canvas", 20000);
  const previewButtons = await page.$$(".promotion-fixed-action button");
  assert(previewButtons.length === 2, "海报预览底部操作按钮不完整");
  result.navigation.preview = page.path;
  result.templates = { count: templates.length, selected: "poster.invite.reward", canvasReady: true };
  progress("navigation:preview:ready");
}

async function main() {
  const result = { startedAt: new Date().toISOString(), projectPath, roles: {}, navigation: {}, templates: {}, consoleErrors: [] };
  progress("connect:start");
  const miniProgram = await connect();
  progress("connect:ready");
  miniProgram.on("console", message => { if (["error", "warning"].includes(message.type)) result.consoleErrors.push(`${message.type}: ${message.args.join(" ")}`); });
  miniProgram.on("exception", error => result.consoleErrors.push(`exception: ${error.message || String(error)}`));
  try {
    progress("runtime:initialize-login-page");
    await timeout("初始化登录页", miniProgram.reLaunch("/pages/WechatLoginPage"), 30000);
    await wait(1500);
    progress("runtime:ready");
    progress("verify:navigation");
    await verifyNavigation(miniProgram, result);
    progress("verify:operation");
    await openPromotionCenter(miniProgram, result, "operation", accounts.operation, "OPERATION");
    progress("verify:user-role");
    await openPromotionCenter(miniProgram, result, "agentAsUser", accounts.agent, "USER");
    assert(result.roles.agent.identityText !== result.roles.agentAsUser.identityText, "切换 USER 后身份标签未刷新");
    result.finishedAt = new Date().toISOString();
    fs.writeFileSync(path.join(artifactDir, "result.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");
    console.log(JSON.stringify(result, null, 2));
  } finally {
    try { await miniProgram.disconnect(); } catch { /* IDE can close automation socket during relaunch */ }
  }
}

main().catch(error => { const message = error instanceof Error ? error.stack || error.message : String(error); try { progress(`failed:${message.replaceAll("\n", " | ")}`); } catch { /* ignore progress write failure */ } console.error(message); process.exit(1); });
