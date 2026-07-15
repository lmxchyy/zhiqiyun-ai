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
const automationPort = process.env.WECHAT_AUTOMATION_PORT || "9420";
const endpoint = process.env.WECHAT_AUTOMATION_ENDPOINT || `ws://127.0.0.1:${automationPort}`;
const idePort = process.env.WECHAT_IDE_PORT || "33709";
const artifactDir = path.join(repoRoot, "artifacts", "wechat-enterprise-runtime");
const email = process.env.XIANZHI_VERIFY_USER_EMAIL || "demo@xianzhi.ai";
const password = process.env.XIANZHI_VERIFY_USER_PASSWORD || "Demo123!";
const expectedEntry = process.env.WECHAT_ENTERPRISE_EXPECT || "switcher";

fs.mkdirSync(artifactDir, { recursive: true });

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

async function wait(ms = 800) {
  await new Promise(resolve => setTimeout(resolve, ms));
}

async function connect() {
  const output = [];
  const launch = port => {
    const command = `& '${cliPath.replaceAll("'", "''")}' auto --project '${projectPath.replaceAll("'", "''")}' --port ${port} --auto-port ${automationPort} --trust-project`;
    const child = spawn("powershell.exe", ["-NoProfile", "-Command", command], { cwd: repoRoot, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] });
    child.stdout.on("data", chunk => output.push(String(chunk)));
    child.stderr.on("data", chunk => output.push(String(chunk)));
  };
  let activeIdePort = idePort;
  launch(activeIdePort);
  let lastError;
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try {
      return await automator.connect({ wsEndpoint: endpoint });
    } catch (error) {
      lastError = error;
      const match = output.join("").match(/IDE server has started on http:\/\/127\.0\.0\.1:(\d+)/i);
      if (match?.[1] && match[1] !== activeIdePort) {
        activeIdePort = match[1];
        launch(activeIdePort);
      }
      await wait(500);
    }
  }
  throw new Error(`微信自动化连接失败: ${lastError instanceof Error ? lastError.message : String(lastError)}\n${output.join("").slice(-2000)}`);
}

async function currentPage(miniProgram) {
  const page = await miniProgram.currentPage();
  assert(page, "开发者工具没有返回当前页面");
  return page;
}

async function enterpriseComponent(page) {
  const component = await page.$("enterprise-center-screen");
  assert(component, `${page.path}: 未找到企业中心组件`);
  return component;
}

async function waitForPath(miniProgram, expected, timeout = 12000) {
  const started = Date.now();
  let page;
  while (Date.now() - started < timeout) {
    page = await currentPage(miniProgram);
    if (page.path === expected) return page;
    await wait(400);
  }
  throw new Error(`页面跳转超时，期望 ${expected}，实际 ${page?.path || "未知"}`);
}

async function login(miniProgram) {
  const response = await fetch("http://127.0.0.1:3100/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  const payload = await response.json();
  const auth = payload?.data || payload;
  assert(response.ok && auth?.accessToken, `登录失败: ${response.status} ${JSON.stringify(payload)}`);
  await miniProgram.callWxMethod("setStorageSync", "token", auth.accessToken);
  await miniProgram.callWxMethod("setStorageSync", "auth", auth);
  await miniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", auth);
  const homePage = await miniProgram.switchTab("/pages/user/UserHomePage");
  assert(homePage.path === "pages/user/UserHomePage", `登录态写入后未进入首页: ${homePage.path}`);
  await homePage.waitFor(1800);
  return auth;
}

async function main() {
  const result = { startedAt: new Date().toISOString(), projectPath, consoleErrors: [] };
  let miniProgram;
  try {
    miniProgram = await connect();
    miniProgram.on("console", message => {
      if (["error", "warning"].includes(message.type)) result.consoleErrors.push(`${message.type}: ${message.args.join(" ")}`);
    });
    miniProgram.on("exception", error => result.consoleErrors.push(`exception: ${error.message || String(error)}`));
    await wait(4000);
    const auth = await login(miniProgram);

    await miniProgram.reLaunch("/pages/enterprise/EnterpriseEntryPage");
    if (expectedEntry === "onboarding") {
      const onboardingPage = await waitForPath(miniProgram, "pages/enterprise/EnterpriseOnboardingPage");
      await onboardingPage.waitFor(800);
      const onboarding = await enterpriseComponent(onboardingPage);
      const createButton = await onboarding.$(".enterprise-primary-button");
      const joinButton = await onboarding.$(".enterprise-secondary-button");
      assert(createButton && joinButton, "未加入企业页缺少创建或加入入口");
      result.authUser = auth.user?.id || "";
      result.onboarding = { createVisible: true, joinVisible: true };
      result.finishedAt = new Date().toISOString();
      fs.writeFileSync(path.join(artifactDir, "onboarding-result.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");
      console.log(JSON.stringify(result, null, 2));
      return;
    }
    const switcherPage = await waitForPath(miniProgram, "pages/enterprise/EnterpriseSwitcherPage");
    await switcherPage.waitFor(1200);
    const switcher = await enterpriseComponent(switcherPage);
    const workspaceCards = await switcher.$$(".enterprise-workspace-card");
    assert(workspaceCards.length >= 3, `多工作空间卡片不足: ${workspaceCards.length}`);
    let targetCard = null;
    let targetText = "";
    for (const card of workspaceCards) {
      const text = await card.text();
      if (!text.includes("个人数据") && text.includes("○")) {
        targetCard = card;
        targetText = text;
        break;
      }
    }
    assert(targetCard, "未找到可切换的另一企业工作空间");
    await targetCard.tap();

    const overviewPage = await waitForPath(miniProgram, "pages/enterprise/EnterpriseOverviewPage");
    await overviewPage.waitFor(1500);
    const overview = await enterpriseComponent(overviewPage);
    const metrics = await overview.$$("enterprise-metric-card");
    const managementRows = await overview.$$(".enterprise-setting-row");
    assert(metrics.length === 4, `企业概览统计卡数量异常: ${metrics.length}`);
    assert(managementRows.length >= 6, `企业管理入口未完整显示: ${managementRows.length}`);

    let membersPage = await miniProgram.reLaunch("/pages/enterprise/EnterpriseMembersPage");
    await membersPage.waitFor(1500);
    let members = await enterpriseComponent(membersPage);
    const memberCards = await members.$$(".enterprise-list-card");
    assert(memberCards.length > 0, "真实成员接口未渲染成员列表");
    const inviteButton = await members.$(".enterprise-primary-button");
    assert(inviteButton, "管理员的底部邀请成员按钮未显示");
    const buttonOffset = await inviteButton.offset();
    const buttonSize = await inviteButton.size();
    const systemInfo = await miniProgram.systemInfo();
    assert(buttonOffset.top >= 0 && buttonOffset.top + buttonSize.height <= Number(systemInfo.windowHeight || 0), `底部按钮被安全区遮挡: ${JSON.stringify({ buttonOffset, buttonSize, windowHeight: systemInfo.windowHeight })}`);

    const usagePage = await miniProgram.reLaunch("/pages/enterprise/EnterpriseUsagePage");
    await usagePage.waitFor(1200);
    const usage = await enterpriseComponent(usagePage);
    const usageState = await usage.$("enterprise-state-panel");
    assert(usageState, "消费明细空状态未渲染");

    const forbiddenPage = await miniProgram.reLaunch("/pages/enterprise/EnterpriseStatusPage?state=forbidden");
    await forbiddenPage.waitFor(500);
    const forbidden = await enterpriseComponent(forbiddenPage);
    const forbiddenState = await forbidden.$("enterprise-state-panel");
    assert(forbiddenState, "无权限状态未渲染");

    result.authUser = auth.user?.id || "";
    result.switcher = { workspaceCards: workspaceCards.length, selected: targetText };
    result.overview = { metrics: metrics.length, managementRows: managementRows.length };
    result.members = { memberCards: memberCards.length, buttonOffset, buttonSize, windowHeight: systemInfo.windowHeight };
    result.states = { usageEmpty: true, forbidden: true };
    result.finishedAt = new Date().toISOString();
    fs.writeFileSync(path.join(artifactDir, "result.json"), `${JSON.stringify(result, null, 2)}\n`, "utf8");
    console.log(JSON.stringify(result, null, 2));
  } finally {
    if (miniProgram?.disconnect) {
      try { await miniProgram.disconnect(); } catch { /* IDE can close the socket first. */ }
    }
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
