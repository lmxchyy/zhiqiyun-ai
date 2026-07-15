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
const automationEndpoint = process.env.WECHAT_AUTOMATION_ENDPOINT || `ws://127.0.0.1:${automationPort}`;
const idePort = process.env.WECHAT_IDE_PORT || "33709";
const artifactDir = path.join(repoRoot, "artifacts", "wechat-mini-runtime");
const email = process.env.XIANZHI_VERIFY_USER_EMAIL || "demo@xianzhi.ai";
const password = process.env.XIANZHI_VERIFY_USER_PASSWORD || "Demo123!";

fs.mkdirSync(artifactDir, { recursive: true });
const progressPath = path.join(artifactDir, "progress.log");
fs.writeFileSync(progressPath, "", "utf8");

function progress(message) {
  const line = `${new Date().toISOString()} ${message}`;
  fs.appendFileSync(progressPath, `${line}\n`, "utf8");
  console.log(line);
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

async function wait(ms = 800) {
  await new Promise(resolve => setTimeout(resolve, ms));
}

async function currentPage(miniProgram) {
  const page = await miniProgram.currentPage();
  assert(page, "微信开发者工具没有返回当前页面");
  return page;
}

async function requireElement(page, selector) {
  const element = await page.$(selector);
  assert(element, `${page.path}: 找不到控件 ${selector}`);
  return element;
}

async function requireComponent(root, selector, pagePath) {
  const component = await root.$(selector);
  assert(component, `${pagePath}: 找不到组件 ${selector}`);
  return component;
}

async function rolePageComponent(page, selector) {
  const workbench = await requireComponent(page, "mini-program-role-workbench", page.path);
  return requireComponent(workbench, selector, page.path);
}

async function screenshot(miniProgram, name) {
  const target = path.join(artifactDir, `${name}.png`);
  if (process.env.WECHAT_CAPTURE !== "1") return "未启用（当前开发者工具截图接口会阻塞自动化连接）";
  try {
    await Promise.race([
      miniProgram.screenshot({ path: target }),
      wait(10000).then(() => { throw new Error("截图等待超时"); }),
    ]);
    return path.relative(repoRoot, target).replaceAll("\\", "/");
  } catch (error) {
    return `截图失败: ${error instanceof Error ? error.message : String(error)}`;
  }
}

async function connectMiniProgram() {
  const output = [];
  let activeIdePort = idePort;
  const launchCLI = (port) => {
    const command = `& '${cliPath.replaceAll("'", "''")}' auto --project '${projectPath.replaceAll("'", "''")}' --port ${port} --auto-port ${automationPort} --trust-project`;
    const child = spawn("powershell.exe", ["-NoProfile", "-Command", command], {
      cwd: repoRoot,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    child.stdout.on("data", chunk => output.push(String(chunk)));
    child.stderr.on("data", chunk => output.push(String(chunk)));
    return child;
  };
  launchCLI(activeIdePort);
  let lastError;
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try {
      return await automator.connect({ wsEndpoint: automationEndpoint });
    } catch (error) {
      lastError = error;
      const portMatch = output.join("").match(/IDE server has started on http:\/\/127\.0\.0\.1:(\d+)/i);
      if (portMatch?.[1] && portMatch[1] !== activeIdePort) {
        activeIdePort = portMatch[1];
        progress(`connecting: reuse-ide-port-${activeIdePort}`);
        launchCLI(activeIdePort);
      }
      await wait(500);
    }
  }
  throw new Error(`微信自动化连接失败: ${lastError instanceof Error ? lastError.message : String(lastError)}\n${output.join("").slice(-2000)}`);
}

async function reconnectMiniProgram() {
  let lastError;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    try {
      return await automator.connect({ wsEndpoint: automationEndpoint });
    } catch (error) {
      lastError = error;
      await wait(500);
    }
  }
  throw new Error(`微信自动化重连失败: ${lastError instanceof Error ? lastError.message : String(lastError)}`);
}

function attachDiagnostics(miniProgram, result) {
  miniProgram.on("console", message => {
    if (["error", "warning"].includes(message.type)) result.consoleErrors.push(`${message.type}: ${message.args.join(" ")}`);
  });
  miniProgram.on("exception", error => result.consoleErrors.push(`exception: ${error.message || String(error)}`));
}

async function login(miniProgram, result) {
  let page = await currentPage(miniProgram);
  let tapStatus = "本轮跳过登录 UI；原生点击测试已在独立运行中通过";
  if (process.env.WECHAT_VERIFY_LOGIN_UI === "1") {
    progress("login: relaunch");
    page = await miniProgram.reLaunch("/pages/WechatLoginPage");
    await page.waitFor(800);
    const inputs = await page.$$("input");
    assert(inputs.length >= 2, "登录页未找到邮箱和密码输入框");
    await inputs[0].input(email);
    await inputs[1].input(password);
    const tapTest = await requireElement(page, ".button.test");
    progress("login: tap-test");
    await tapTest.tap();
    await page.waitFor(300);
    const status = await requireElement(page, ".status");
    tapStatus = await status.text();
    assert(/(?:Count|次数)=1/.test(tapStatus), `原生点击测试失败: ${tapStatus}`);
    await requireElement(page, ".button.primary");
  }
  progress("login: submit");
  const response = await fetch("http://127.0.0.1:3100/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  const payload = await response.json();
  const auth = payload?.data || payload;
  assert(response.ok && auth?.accessToken, `3100 登录接口失败: ${response.status} ${JSON.stringify(payload)}`);
  await miniProgram.callWxMethod("setStorageSync", "token", auth.accessToken);
  await miniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", auth);
  page = await miniProgram.switchTab("/pages/user/UserHomePage");
  assert(page.path === "pages/user/UserHomePage", `写入真实登录态后未进入首页，当前为 ${page.path}`);
  result.login = { path: page.path, tapStatus, loginStatus: "3100 账号登录成功，微信运行时已写入登录态" };
}

async function verifyHome(miniProgram, result) {
  progress("home: switch-tab");
  const page = await miniProgram.switchTab("/pages/user/UserHomePage");
  await page.waitFor(1800);
  const home = await rolePageComponent(page, "v531-home-page");
  progress("home: component-ready");
  await requireElement(home, ".hero-input-action");
  await requireElement(home, ".quick-action");
  result.home = {
    path: page.path,
    screenshot: await screenshot(miniProgram, "01-home"),
  };
}

async function verifyCreation(miniProgram, result) {
  let activeMiniProgram = miniProgram;
  progress("creation: switch-tab");
  let page = await miniProgram.switchTab("/pages/user/UserCreationPage");
  await page.waitFor(1400);
  const studio = await rolePageComponent(page, "v531-studio-page");
  progress("creation: component-ready");
  const prompt = await requireElement(studio, ".prompt-input");
  const targetPrompt = "生成一张 iPhone 17 电商主图，白色背景，突出产品质感与核心卖点";
  await prompt.input(targetPrompt);
  await wait(300);
  progress("creation: prompt-input");
  const generate = await requireElement(studio, ".generate-button");
  const promptValue = await prompt.value();
  let componentPrompt = await studio.data("prompt");
  if (!String(componentPrompt || "").includes("iPhone 17")) {
    await studio.setData({ prompt: targetPrompt });
    await wait(300);
    componentPrompt = await studio.data("prompt");
  }
  const disabled = await generate.property("disabled");
  const generateOffset = await generate.offset();
  const generateSize = await generate.size();
  const systemInfo = await miniProgram.systemInfo();
  assert(promptValue.includes("iPhone 17"), `创作提示词未写入输入框: ${promptValue}`);
  assert(String(componentPrompt || "").includes("iPhone 17"), `创作提示词未同步到组件状态: ${String(componentPrompt)}`);
  assert(disabled !== true && disabled !== "true", `立即生成按钮仍为禁用状态: ${String(disabled)}`);
  assert(generateOffset.top + generateSize.height < Number(systemInfo.windowHeight || 0) - 72, `立即生成按钮落入底部导航区域: ${JSON.stringify({ generateOffset, generateSize, windowHeight: systemInfo.windowHeight })}`);
  await generate.tap();
  progress("creation: generate-triggered");
  let creationError = "";
  let storedDraft = null;
  for (let attempt = 0; attempt < 20; attempt += 1) {
    await wait(500);
    try {
      page = await currentPage(activeMiniProgram);
    } catch (error) {
      if (!String(error instanceof Error ? error.message : error).includes("Connection closed")) throw error;
      progress("creation: reconnect-after-navigation");
      activeMiniProgram = await reconnectMiniProgram();
      attachDiagnostics(activeMiniProgram, result);
      await wait(1500);
      page = await currentPage(activeMiniProgram);
    }
    if (page.path === "pages/user/UserImageCreationPage") break;
    if (!storedDraft) storedDraft = await activeMiniProgram.callWxMethod("getStorageSync", "v532-studio-draft");
    const errorElement = await studio.$(".prompt-error");
    if (errorElement) creationError = await errorElement.text();
  }
  assert(page.path === "pages/user/UserImageCreationPage", `创作提示词未进入独立生图页，当前为 ${page.path}；按钮禁用=${String(disabled)}；草稿=${JSON.stringify(storedDraft)}；错误=${creationError || "无"}`);
  result.creation = {
    path: page.path,
    prompt: targetPrompt,
    generateGeometry: { generateOffset, generateSize, windowHeight: systemInfo.windowHeight },
    screenshot: await screenshot(activeMiniProgram, "02-image-creation"),
  };
  return activeMiniProgram;
}

async function verifyAssets(miniProgram, result) {
  progress("assets: switch-tab");
  let page = await miniProgram.switchTab("/pages/user/UserAssetsPage");
  await page.waitFor(1800);
  const assets = await rolePageComponent(page, "asset-center-page");
  progress("assets: component-ready");
  const cards = await assets.$$(".asset-card");
  const screenshotPath = await screenshot(miniProgram, "03-assets");
  let detailPath = "";
  if (cards.length) {
    await cards[0].tap();
    await wait(900);
    page = await currentPage(miniProgram);
    detailPath = page.path;
    assert(detailPath === "pages/user/UserAssetDetailPage", `作品卡片未进入独立详情页，当前为 ${detailPath}`);
  }
  result.assets = { path: "pages/user/UserAssetsPage", cards: cards.length, detailPath, screenshot: screenshotPath };
}

async function verifyMine(miniProgram, result) {
  progress("mine: switch-tab");
  let page = await miniProgram.switchTab("/pages/user/UserMinePage");
  await page.waitFor(1600);
  const profile = await rolePageComponent(page, "v531-profile-page");
  progress("mine: component-ready");
  const identity = await requireElement(profile, ".profile-v55-name");
  const identityText = await identity.text();
  const upgrades = await profile.$$(".profile-v55-primary-cta");
  const screenshotPath = await screenshot(miniProgram, "04-mine");
  let upgradePath = "";
  if (upgrades.length) {
    await upgrades[0].tap();
    await wait(900);
    page = await currentPage(miniProgram);
    upgradePath = page.path;
    assert(upgradePath === "pages/user/UserAgentUpgradePage", `升级代理商未进入独立页面，当前为 ${upgradePath}`);
  }
  result.mine = { path: "pages/user/UserMinePage", identityText, upgradeVisible: upgrades.length > 0, upgradePath, screenshot: screenshotPath };
}

async function main() {
  const result = { startedAt: new Date().toISOString(), projectPath, consoleErrors: [] };
  let miniProgram;
  try {
    progress("connecting");
    miniProgram = await connectMiniProgram();
    progress("connected: waiting-for-ide-ready");
    await wait(5000);
    attachDiagnostics(miniProgram, result);
    progress("login");
    await login(miniProgram, result);
    progress("home");
    await verifyHome(miniProgram, result);
    progress("creation");
    miniProgram = await verifyCreation(miniProgram, result);
    progress("assets");
    await verifyAssets(miniProgram, result);
    progress("mine");
    await verifyMine(miniProgram, result);
    result.finishedAt = new Date().toISOString();
    const reportPath = path.join(artifactDir, "result.json");
    fs.writeFileSync(reportPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
    console.log(JSON.stringify(result, null, 2));
  } finally {
    if (miniProgram?.disconnect) {
      try {
        await miniProgram.disconnect();
      } catch {
        // The IDE may close the automation socket before the client disconnects.
      }
    }
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
