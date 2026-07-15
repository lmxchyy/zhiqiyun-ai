import { createRequire } from "node:module";
import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");
const repoRoot = path.resolve(import.meta.dirname, "..");
const projectPath = path.join(repoRoot, "apps", "user-uni", "dist", "build", "mp-weixin");
const cliPath = process.env.WX_CLI_PATH || "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const autoPort = Number(process.env.WX_WORKS_AUTOMATOR_PORT || 9432);
const idePort = String(process.env.WX_WORKS_IDE_PORT || 33709);
const automationEndpoint = "ws://127.0.0.1:" + autoPort;
const apiBase = (process.env.API_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const email = process.env.LOGIN_EMAIL || "demo@xianzhi.ai";
const password = process.env.LOGIN_PASSWORD || "Demo123!";
const searchMode = String(process.env.WX_WORKS_SEARCH_ONLY || "").toLowerCase();
const searchOnly = searchMode === "true" || searchMode === "main";
const librarySearchOnly = searchMode === "library";
const artifactDir = path.join(repoRoot, "artifacts", "wechat-works-clicks");
const resultPath = path.join(artifactDir, "result.json");

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function wait(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function withTimeout(label, promise, timeoutMs = 15000) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

async function waitForElement(root, selector, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const element = await root.$(selector).catch(() => null);
    if (element) return element;
    await wait(300);
  }
  return null;
}

async function assetCardTexts(root) {
  const grid = await waitForElement(root, "asset-grid", 15000);
  if (!grid) return [];
  const cards = await grid.$$("asset-card");
  return Promise.all(cards.map(card => card.text()));
}

async function login() {
  const response = await fetch(`${apiBase}/api/v1/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  const payload = await response.json();
  const auth = payload?.data || payload;
  assert(response.ok && auth?.accessToken, `登录失败: ${response.status} ${JSON.stringify(payload)}`);
  return auth;
}

async function openAssets(miniProgram) {
  const page = await withTimeout("switchTab assets", miniProgram.switchTab("/pages/user/UserAssetsPage"), 20000);
  await page.waitFor(2200);
  if (page.path !== "pages/user/UserAssetsPage") {
    const storage = await miniProgram.evaluate(() => ({
      hasToken: Boolean(wx.getStorageSync("token")),
      hasAuth: Boolean(wx.getStorageSync("auth")),
      hasLegacyAuth: Boolean(wx.getStorageSync("xianzhiMiniProgramAuth")),
    }));
    throw new Error(`作品 Tab 未打开: ${page.path}; storage=${JSON.stringify(storage)}`);
  }
  const workbench = await waitForElement(page, "mini-program-role-workbench", 15000);
  assert(workbench, "找不到 mini-program-role-workbench");
  const center = await waitForElement(workbench, "asset-center-page", 15000);
  assert(center, "找不到 asset-center-page");
  return { page, center };
}

async function elementClass(element) {
  return String(
    await element.attribute("class").catch(async () => await element.property("class").catch(() => "")) || "",
  );
}

async function verifyTabTap(center, selector, index, label) {
  let component = await center.$(selector);
  assert(component, `找不到 ${label} 组件 ${selector}`);
  let buttons = await component.$$(selector === "asset-type-tabs" ? ".tab-button" : ".status-button");
  assert(buttons.length > index, `${label} 按钮不足: ${buttons.length}`);
  if (index > 0) {
    await buttons[0].tap();
    await wait(900);
    component = await center.$(selector);
    buttons = await component.$$(selector === "asset-type-tabs" ? ".tab-button" : ".status-button");
  }
  const text = await buttons[index].text();
  const beforeClass = await elementClass(buttons[index]);
  assert(!beforeClass.includes("active"), `${label}基线未重置: ${text}; class=${beforeClass}`);
  await buttons[index].tap();
  await wait(1400);
  const refreshed = await center.$(selector);
  const refreshedButtons = await refreshed.$$(selector === "asset-type-tabs" ? ".tab-button" : ".status-button");
  const afterClass = await elementClass(refreshedButtons[index]);
  const renderData = await refreshed.data("a").catch(() => []);
  assert(afterClass.includes("active") || JSON.stringify(renderData[index] || {}).includes("active"), `${label}点击后未激活: ${text}; class=${afterClass}; data=${JSON.stringify(renderData[index] || {})}`);
  return { text, beforeClass, afterClass, active: true };
}

async function tapSectionRoute(miniProgram, center, sectionLabel, expectedPath, label) {
  const sections = await center.$$(".section-head");
  let target;
  for (const section of sections) {
    if ((await section.text()).includes(sectionLabel)) {
      target = section;
      break;
    }
  }
  assert(target, `${label}入口不存在: ${sectionLabel}`);
  const button = await target.$("button");
  assert(button, `${label}按钮不存在`);
  const text = await button.text();
  await button.tap();
  await wait(1300);
  const page = await miniProgram.currentPage();
  assert(page.path === expectedPath, `${label}未进入目标页: ${page.path}`);
  return { text, path: page.path };
}

async function connectMiniProgram() {
  const output = [];
  let activeIdePort = idePort;
  const launchCLI = port => {
    const command = `& '${cliPath.replaceAll("'", "''")}' auto --project '${projectPath.replaceAll("'", "''")}' --port ${port} --auto-port ${autoPort} --trust-project`;
    const child = spawn("powershell.exe", ["-NoProfile", "-Command", command], {
      cwd: repoRoot,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    child.stdout.on("data", chunk => output.push(String(chunk)));
    child.stderr.on("data", chunk => output.push(String(chunk)));
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
async function main() {
  fs.mkdirSync(artifactDir, { recursive: true });
  const auth = await login();
  const result = { startedAt: new Date().toISOString(), projectPath, checks: {}, consoleErrors: [] };
  let miniProgram;
  try {
    miniProgram = await withTimeout("connect WeChat DevTools", connectMiniProgram(), 75000);
    miniProgram.on("console", message => {
      if (["error", "warning"].includes(message.type)) result.consoleErrors.push(`${message.type}: ${message.args.join(" ")}`);
    });
    miniProgram.on("exception", error => result.consoleErrors.push(`exception: ${error.message || String(error)}`));

    const loginPage = await withTimeout("reLaunch login", miniProgram.reLaunch("/pages/WechatLoginPage"), 30000);
    await loginPage.waitFor(1000);
    await miniProgram.callWxMethod("setStorageSync", "token", auth.accessToken || "");
    await miniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", auth);
    const storedToken = await miniProgram.callWxMethod("getStorageSync", "token");
    const storedLegacyAuth = await miniProgram.callWxMethod("getStorageSync", "xianzhiMiniProgramAuth");
    assert(storedToken && storedLegacyAuth, `微信登录态写入失败: token=${Boolean(storedToken)} legacy=${Boolean(storedLegacyAuth)}`);

    let opened = await openAssets(miniProgram);
    result.checks.typeImage = await verifyTabTap(opened.center, "asset-type-tabs", 1, "作品类型-图片");
    result.checks.statusGenerating = await verifyTabTap(opened.center, "asset-status-tabs", 2, "状态筛选-生成中");

    opened = await openAssets(miniProgram);
    result.checks.typeAll = await verifyTabTap(opened.center, "asset-type-tabs", 0, "作品类型-全部");
    result.checks.statusRecent = await verifyTabTap(opened.center, "asset-status-tabs", 0, "状态筛选-最近");

    opened = await openAssets(miniProgram);
    let searchInput = await opened.center.$(".search-card input");
    let searchSubmit = await opened.center.$(".search-submit");
    assert(searchInput && searchSubmit, "作品首页搜索控件不完整");
    await searchInput.input("task_000208");
    await searchSubmit.tap();
    await wait(1400);
    let searchResults = await assetCardTexts(opened.center);
    assert(searchResults.length === 1 && searchResults[0].includes("task_000208"), `作品首页按钮搜索结果错误: ${JSON.stringify(searchResults)}`);
    let clearSearch = await opened.center.$(".search-clear");
    assert(clearSearch, "作品首页清空搜索按钮不存在");
    await clearSearch.tap();
    await wait(1400);
    let restoredAssets = await assetCardTexts(opened.center);
    assert(restoredAssets.length > 1, `作品首页清空搜索后未恢复: ${JSON.stringify(restoredAssets)}`);
    searchInput = await opened.center.$(".search-card input");
    assert(searchInput, "作品首页搜索输入框丢失");
    await searchInput.input("task_000208");
    await wait(1400);
    searchResults = await assetCardTexts(opened.center);
    assert(searchResults.length === 1 && searchResults[0].includes("task_000208"), `作品首页自动搜索结果错误: ${JSON.stringify(searchResults)}`);
    clearSearch = await opened.center.$(".search-clear");
    assert(clearSearch, "作品首页自动搜索后清空按钮不存在");
    await clearSearch.tap();
    await wait(1200);
    result.checks.search = { button: true, automatic: true, clear: true, matched: searchResults[0] };
    if (searchOnly) {
      result.finishedAt = new Date().toISOString();
      result.ok = true;
      fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
      console.log(JSON.stringify(result, null, 2));
      return;
    }

    opened = await openAssets(miniProgram);
    result.checks.assetsViewAll = await tapSectionRoute(miniProgram, opened.center, "最近作品", "pages/user/UserAssetsListPage", "最近作品-查看全部");
    let listPage = await miniProgram.currentPage();
    await listPage.waitFor(1800);
    const assetLibrary = await listPage.$("asset-library-page");
    assert(assetLibrary, "完整作品页未渲染 asset-library-page");
    const assetGrid = await assetLibrary.$("asset-grid");
    assert(assetGrid, "完整作品页没有加载作品网格");
    searchInput = await assetLibrary.$(".search-row input");
    searchSubmit = await assetLibrary.$(".search-button");
    assert(searchInput && searchSubmit, "完整作品页搜索控件不完整");
    await searchInput.input("task_000208");
    await searchSubmit.tap();
    await wait(1400);
    searchResults = await assetCardTexts(assetLibrary);
    assert(searchResults.length === 1 && searchResults[0].includes("task_000208"), `完整作品页搜索结果错误: ${JSON.stringify(searchResults)}`);
    clearSearch = await assetLibrary.$(".search-clear");
    assert(clearSearch, "完整作品页清空搜索按钮不存在");
    await clearSearch.tap();
    await wait(1200);
    restoredAssets = await assetCardTexts(assetLibrary);
    assert(restoredAssets.length > 1, `完整作品页清空搜索后未恢复: ${JSON.stringify(restoredAssets)}`);
    result.checks.assetsLoaded = { rendered: true, search: true, clear: true };
    if (librarySearchOnly) {
      result.finishedAt = new Date().toISOString();
      result.ok = true;
      fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
      console.log(JSON.stringify(result, null, 2));
      return;
    }

    opened = await openAssets(miniProgram);
    result.checks.tasksViewAll = await tapSectionRoute(miniProgram, opened.center, "最近任务", "pages/user/UserTasksPage", "最近任务-查看全部");
    listPage = await miniProgram.currentPage();
    await listPage.waitFor(1800);
    const taskList = await listPage.$("generation-task-list-page");
    assert(taskList, "完整任务页未渲染 generation-task-list-page");
    const renderedTasks = await taskList.$$("generation-task-item");
    assert(renderedTasks.length > 0, "完整任务页已打开，但没有渲染后端任务数据");
    result.checks.tasksLoaded = { count: renderedTasks.length };

    opened = await openAssets(miniProgram);
    let empty = await opened.center.$("asset-empty-state");
    if (!empty) {
      await verifyTabTap(opened.center, "asset-type-tabs", 9, "作品类型-模板");
      await wait(1200);
      empty = await opened.center.$("asset-empty-state");
    }
    assert(empty, "当前账号存在全部类型作品，未出现可点击的空状态");
    const start = await empty.$(".empty-action");
    assert(start, "找不到开始创作按钮");
    const startText = await start.text();
    await start.tap();
    await wait(1300);
    let creationPage;
    try {
      creationPage = await miniProgram.currentPage();
    } catch (error) {
      if (!String(error instanceof Error ? error.message : error).includes("Connection closed")) throw error;
      miniProgram = await reconnectMiniProgram();
      await wait(900);
      creationPage = await miniProgram.currentPage();
    }
    assert(creationPage.path === "pages/user/UserCreationPage", `开始创作未切换到创作 Tab: ${creationPage.path}`);
    result.checks.startCreation = { text: startText, path: creationPage.path };

    result.finishedAt = new Date().toISOString();
    result.ok = true;
    fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
    console.log(JSON.stringify(result, null, 2));
  } finally {
    try {
      if (miniProgram) {
        await miniProgram.callWxMethod("setStorageSync", "zhiqiyun:asset-center:filters", {
          type: "all",
          status: "recent",
          keyword: "",
          projectId: "",
          tagIds: [],
          model: "",
          createdFrom: "",
          createdTo: "",
        });
        await miniProgram.callWxMethod("setStorageSync", "zhiqiyun:asset-center:sort", "created_desc");
      }
      miniProgram?.disconnect();
    } catch {
      // The IDE may close its automation socket during tab navigation.
    }
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
