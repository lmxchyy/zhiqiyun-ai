import { createRequire } from "node:module";
import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");

const cliPath = process.env.WX_CLI_PATH || "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const projectPath = process.env.WX_PROJECT_PATH || path.resolve("apps", "user-uni", "dist", "build", "mp-weixin");
const automationPort = Number(process.env.WX_HOME_RETURN_PORT || 9433);
const automationEndpoint = `ws://127.0.0.1:${automationPort}`;
const initialIdePort = String(process.env.WECHAT_IDE_PORT || "33709");
const generatedClientPath = path.join(projectPath, "api", "client.js");
function detectApiBase() {
  if (process.env.API_BASE_URL) return process.env.API_BASE_URL;
  try {
    const source = fs.readFileSync(generatedClientPath, "utf8");
    const match = source.match(/VITE_API_BASE_URL:"([^"]+)"/);
    if (match?.[1]) return match[1];
  } catch {
    // The local backend is the fallback for an output that has not been built yet.
  }
  return "http://127.0.0.1:3100";
}
const apiBase = detectApiBase().replace(/\/+$/, "");
const email = process.env.LOGIN_EMAIL || "demo@xianzhi.ai";
const password = process.env.LOGIN_PASSWORD || "Demo123!";
const rowFilter = String(process.env.HOME_RETURN_FILTER || "").trim();
const rowFilters = rowFilter.split(",").map(item => item.trim()).filter(Boolean);
const runLabel = String(process.env.HOME_RETURN_RUN || rowFilters.join("-")).trim().replace(/[^a-z0-9._-]+/gi, "-");
const artifactDir = path.resolve("artifacts", "wechat-home-entry-return");
const resultFile = path.join(artifactDir, runLabel ? `result-${runLabel}.json` : "result.json");
const progressFile = path.join(artifactDir, runLabel ? `progress-${runLabel}.log` : "progress.log");
const latestResultFile = path.join(artifactDir, "result.json");
const latestProgressFile = path.join(artifactDir, "progress.log");
const homePath = "pages/user/UserHomePage";

const creationRoutes = {
  image: "pages/user/UserImageCreationPage",
  video: "pages/user/UserVideoCreationPage",
  ppt: "pages/user/UserPptCreationPage",
  infographic: "pages/user/UserInfographicCreationPage",
  review: "pages/user/UserReviewCreationPage",
  agent: "pages/user/UserAgentCreationPage",
};

const navigationRows = [
  { id: "profile", selector: ".brand-action", index: 1, expected: ["pages/user/UserMinePage"], returnType: "tab" },
  { id: "prompt-submit", selector: ".hero-input-action.submit", index: 0, expected: [creationRoutes.image], returnType: "back", prompt: "生成一张 iPhone 17 电商主图" },
  ...[
    ["quick.poster", 0, "image"],
    ["quick.ppt", 1, "ppt"],
    ["quick.video", 2, "video"],
    ["quick.knowledge", 3, "agent"],
  ].map(([id, index, mode]) => ({ id, selector: ".quick-action", index, expected: [creationRoutes[mode]], returnType: "back" })),
  ...[
    ["hero-tool.design", 0, "image"],
    ["hero-tool.video", 1, "video"],
    ["hero-tool.ppt", 2, "ppt"],
    ["hero-tool.knowledge", 3, "agent"],
    ["hero-tool.employee", 4, "agent"],
  ].map(([id, index, mode]) => ({ id, selector: ".hero-tool", index, expected: [creationRoutes[mode]], returnType: "back" })),
  { id: "workspace.view-all", selector: ".workspace-action", index: 0, expected: ["pages/user/UserMinePage"], returnType: "tab" },
  { id: "metric.wallet", selector: ".metric-card", index: 0, expected: ["pages/user/UserWalletPage"], returnType: "back" },
  { id: "metric.membership", selector: ".metric-card", index: 1, expected: ["pages/user/UserMinePage"], returnType: "tab" },
  { id: "metric.calls", selector: ".metric-card", index: 2, expected: ["pages/user/UserAssetsPage"], returnType: "tab" },
  { id: "metric.assets", selector: ".metric-card", index: 3, expected: ["pages/user/UserAssetsPage"], returnType: "tab" },
  { id: "today-suggestion", selector: ".ai-suggestion", index: 0, expected: ["pages/user/UserAssetsPage", "pages/user/UserCreationPage"], returnType: "tab" },
  { id: "creation.view-all", selector: ".section-heading-action", index: 0, expected: ["pages/user/UserCreationPage"], returnType: "tab" },
  ...[
    ["featured.image", 0, "image"],
    ["featured.video", 1, "video"],
  ].map(([id, index, mode]) => ({ id, selector: ".capability-feature-card", index, expected: [creationRoutes[mode]], returnType: "back" })),
  ...[
    ["secondary.ppt", 0, "ppt"],
    ["secondary.office", 1, "infographic"],
  ].map(([id, index, mode]) => ({ id, selector: ".capability-secondary-card", index, expected: [creationRoutes[mode]], returnType: "back" })),
  ...[
    ["compact.knowledge", 0],
    ["compact.employee", 1],
    ["compact.workflow", 2],
    ["compact.more", 3],
  ].map(([id, index]) => ({ id, selector: ".capability-compact-card", index, expected: [creationRoutes.agent], returnType: "back" })),
  { id: "projects.view-all", selector: ".project-section-action", index: 0, expected: ["pages/user/UserAssetsPage"], returnType: "tab" },
  { id: "employees.view-all", selector: ".employee-section-action", index: 0, expected: [creationRoutes.agent], returnType: "back" },
  ...[0, 1, 2, 3, 4].map(index => ({ id: `employee.${index}`, selector: ".employee-card", index, expected: [creationRoutes.agent], returnType: "back" })),
  ...[
    ["inspiration.poster", 0, "image"],
    ["inspiration.video", 1, "video"],
    ["inspiration.ppt", 2, "ppt"],
    ["inspiration.store", 3, "image"],
    ["inspiration.ecommerce", 4, "image"],
  ].map(([id, index, mode]) => ({ id, selector: ".inspiration-card", index, expected: [creationRoutes[mode]], returnType: "back" })),
];

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function progress(message) {
  fs.mkdirSync(artifactDir, { recursive: true });
  const line = `${new Date().toISOString()} ${message}`;
  fs.appendFileSync(progressFile, `${line}\n`, "utf8");
  if (progressFile !== latestProgressFile) fs.appendFileSync(latestProgressFile, `${line}\n`, "utf8");
  console.log(line);
}

function persist(result) {
  const output = JSON.stringify(result, null, 2);
  fs.writeFileSync(resultFile, output, "utf8");
  if (resultFile !== latestResultFile) fs.writeFileSync(latestResultFile, output, "utf8");
}

function matchesFilter(id) {
  return !rowFilters.length || rowFilters.some(filter => id.includes(filter));
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

async function connectMiniProgram() {
  const output = [];
  let activeIdePort = initialIdePort;
  const launchCLI = port => {
    const command = `& '${cliPath.replaceAll("'", "''")}' auto --project '${projectPath.replaceAll("'", "''")}' --port ${port} --auto-port ${automationPort} --trust-project`;
    const child = spawn("powershell.exe", ["-NoProfile", "-Command", command], {
      cwd: process.cwd(),
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    child.stdout.on("data", chunk => output.push(String(chunk)));
    child.stderr.on("data", chunk => output.push(String(chunk)));
  };
  launchCLI(activeIdePort);
  let lastError;
  for (let attempt = 0; attempt < 160; attempt += 1) {
    try {
      return await automator.connect({ wsEndpoint: automationEndpoint });
    } catch (error) {
      lastError = error;
      const match = output.join("").match(/IDE server has started on http:\/\/127\.0\.0\.1:(\d+)/i);
      if (match?.[1] && match[1] !== activeIdePort) {
        activeIdePort = match[1];
        launchCLI(activeIdePort);
      }
      await sleep(500);
    }
  }
  throw new Error(`wechat automator connection failed: ${lastError instanceof Error ? lastError.message : String(lastError)}\n${output.join("").slice(-2000)}`);
}

async function login() {
  const response = await fetch(`${apiBase}/api/v1/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  const text = await response.text();
  if (!response.ok) throw new Error(`login failed ${response.status}: ${text.slice(0, 300)}`);
  const payload = JSON.parse(text);
  return payload?.data || payload;
}

async function setSession(miniProgram, auth) {
  await miniProgram.callWxMethod("setStorageSync", "token", auth.accessToken || "");
  await miniProgram.callWxMethod("setStorageSync", "refreshToken", auth.refreshToken || "");
  await miniProgram.callWxMethod("setStorageSync", "auth", auth);
  await miniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", auth);
  const storedToken = await miniProgram.callWxMethod("getStorageSync", "token");
  const storedAuth = await miniProgram.callWxMethod("getStorageSync", "auth");
  if (!storedToken || !storedAuth?.accessToken) throw new Error("auth session was not persisted in WeChat runtime");
}

async function waitForPath(miniProgram, expected, timeoutMs = 12000) {
  const deadline = Date.now() + timeoutMs;
  let lastPath = "";
  while (Date.now() < deadline) {
    try {
      const page = await miniProgram.currentPage();
      lastPath = page?.path || "";
      if (expected.includes(lastPath)) return page;
    } catch {
      // A native route change can briefly replace the automator page context.
    }
    try {
      const stack = await miniProgram.pageStack();
      const stackTop = stack[stack.length - 1];
      lastPath = stackTop?.path || lastPath;
      if (stackTop && expected.includes(lastPath)) return stackTop;
    } catch {
      // The page stack can also be transient while native navigation settles.
    }
    await sleep(300);
  }
  throw new Error(`expected ${expected.join(" | ")}, current ${lastPath || "unknown"}`);
}

async function openHome(miniProgram) {
  let current = await miniProgram.currentPage().catch(() => null);
  if (current?.path !== homePath) {
    await miniProgram.callWxMethod("switchTab", { url: `/${homePath}` }).catch(() => null);
    await sleep(1200);
    current = await miniProgram.currentPage().catch(() => null);
    if (current?.path !== homePath) {
      await miniProgram.reLaunch(`/${homePath}`);
    }
  }
  const home = await waitForPath(miniProgram, [homePath]);
  await home.waitFor(1800);
  await homeComponent(home);
  return home;
}

async function homeComponent(page) {
  const deadline = Date.now() + 10000;
  let lastState = { page: page.path, workbench: false, outer: "" };
  while (Date.now() < deadline) {
    const workbench = await page.$("mini-program-role-workbench");
    const home = workbench ? await workbench.$("v531-home-page") : null;
    if (home) return home;
    lastState = {
      page: page.path,
      workbench: Boolean(workbench),
      outer: workbench ? String(await workbench.outerWxml().catch(() => "")).slice(0, 500) : "",
    };
    await sleep(300);
  }
  throw new Error(`home component not rendered: ${JSON.stringify(lastState)}`);
}

async function indexedElement(page, selector, index) {
  const elements = await page.$$(selector);
  const element = elements[index];
  if (!element) throw new Error(`${selector}[${index}] not found; count=${elements.length}`);
  return element;
}

async function scrollIntoView(miniProgram, page, selector, index) {
  let root = await homeComponent(page);
  let element = await indexedElement(root, selector, index);
  const offset = await withTimeout("element offset", element.offset(), 8000).catch(() => null);
  const currentScroll = Number(await withTimeout("page scrollTop", page.scrollTop(), 8000).catch(() => 0)) || 0;
  const top = Number(offset?.top);
  if (Number.isFinite(top) && (top < 80 || top > 760)) {
    await withTimeout("page scroll", miniProgram.pageScrollTo(Math.max(0, currentScroll + top - 220)), 8000);
    await sleep(450);
    const currentPage = await miniProgram.currentPage();
    root = await homeComponent(currentPage);
    element = await indexedElement(root, selector, index);
  }
  return element;
}

async function returnByBackButton(miniProgram) {
  await withTimeout("reset detail scroll", miniProgram.pageScrollTo(0), 8000).catch(() => null);
  await sleep(700);
  const page = await miniProgram.currentPage();
  const roots = [];
  for (const componentSelector of ["mini-program-role-workbench", "asset-detail-center-page", "native-page-back"]) {
    const component = await withTimeout(
      `find return component ${componentSelector}`,
      page.$(componentSelector),
      5000,
    ).catch(() => null);
    if (component) roots.push(component);
  }
  roots.push(page);
  for (const root of roots) {
    for (const selector of [".native-page-back", ".v31-back-button", ".back-button", ".mpb-back"]) {
      const element = await withTimeout(`find return ${selector}`, root.$(selector), 5000).catch(() => null);
      if (!element) continue;
      const offset = await withTimeout(`return offset ${selector}`, element.offset(), 8000).catch(() => null);
      const scrollTop = await withTimeout("detail scrollTop", page.scrollTop(), 8000).catch(() => null);
      const stack = await withTimeout(
        "detail page stack",
        miniProgram.evaluate(() => getCurrentPages().map(item => item.route)),
        8000,
      ).catch(() => []);
      progress(`RETURN ${page.path} ${selector} offset=${JSON.stringify(offset)} scrollTop=${scrollTop} stack=${JSON.stringify(stack)}`);
      // miniprogram-automator dispatches tap on a navigator but does not execute
      // its native open-type action. Recover the home route with the SDK while
      // the build assertion verifies the navigator's declarative behavior.
      const nativeRouteRecovery = selector === ".native-page-back";
      const sdkNavigateBack = selector === ".v31-back-button";
      const returnAction = nativeRouteRecovery
        ? miniProgram.callWxMethod("switchTab", { url: `/${homePath}` })
        : sdkNavigateBack
          ? miniProgram.navigateBack()
          : element.tap();
      const returnResult = await withTimeout(`tap return ${selector}`, returnAction, 10000);
      if (nativeRouteRecovery) {
        const stackSnapshots = [];
        for (let attempt = 0; attempt < 12; attempt += 1) {
          await sleep(100);
          const stack = await withTimeout(
            `detail page stack after return ${attempt + 1}`,
            miniProgram.evaluate(() => getCurrentPages().map(item => item.route)),
            8000,
          ).catch(() => []);
          stackSnapshots.push(stack);
        }
        progress(`RETURN RESULT ${JSON.stringify(returnResult)} stackSnapshots=${JSON.stringify(stackSnapshots)}`);
      }
      return {
        page: await waitForPath(miniProgram, [homePath]),
        preservesScroll: !nativeRouteRecovery,
        verification: nativeRouteRecovery
          ? "native-route-recovery"
          : sdkNavigateBack
            ? "sdk-navigate-back"
            : "button-tap",
      };
    }
  }
  throw new Error(`return button not found on ${page.path}`);
}

async function returnByHomeTab(miniProgram) {
  const page = await miniProgram.currentPage();
  let tabs = await page.$$(".v531-tab");
  if (!tabs.length) {
    const tabBar = await page.$("custom-tab-bar");
    tabs = tabBar ? await tabBar.$$(".v531-tab") : [];
  }
  if (tabs.length) {
    await withTimeout("tap home tab", tabs[0].tap(), 10000);
  } else {
    const switched = await withTimeout("switch home tab", miniProgram.callWxMethod("switchTab", { url: `/${homePath}` }), 10000);
    if (switched?.errMsg && !String(switched.errMsg).includes("ok")) {
      throw new Error(`home tab switch failed on ${page.path}: ${switched.errMsg}`);
    }
  }
  return {
    page: await waitForPath(miniProgram, [homePath]),
    preservesScroll: true,
    verification: "home-tab",
  };
}

async function verifyNavigationRow(miniProgram, row) {
  let home = await openHome(miniProgram);
  await miniProgram.pageScrollTo(0);
  await sleep(200);
  home = await miniProgram.currentPage();
  if (row.prompt) {
    const root = await homeComponent(home);
    const input = await root.$(".hero-text-input");
    if (!input) throw new Error("hero prompt input not found");
    await input.input(row.prompt);
    await sleep(200);
  }
  const element = await scrollIntoView(miniProgram, home, row.selector, row.index || 0);
  const homeScrollBefore = Number(await withTimeout("home scroll before", home.scrollTop(), 8000).catch(() => 0)) || 0;
  await withTimeout(`tap ${row.id}`, element.tap(), 10000);
  const target = await waitForPath(miniProgram, row.expected);
  await target.waitFor(1500);
  const returnMeta = row.returnType === "back"
    ? await returnByBackButton(miniProgram)
    : await returnByHomeTab(miniProgram);
  const returned = returnMeta.page;
  await returned.waitFor(350);
  await homeComponent(returned);
  const homeScrollAfter = Number(await withTimeout("home scroll after", returned.scrollTop(), 8000).catch(() => 0)) || 0;
  if (row.returnType === "back" && returnMeta.preservesScroll && Math.abs(homeScrollAfter - homeScrollBefore) > 80) {
    throw new Error(`home scroll not restored: before=${homeScrollBefore}, after=${homeScrollAfter}`);
  }
  return {
    id: row.id,
    target: target.path,
    returnType: row.returnType,
    returnVerification: returnMeta.verification,
    returned: returned.path,
    homeScrollBefore,
    homeScrollAfter,
  };
}

async function verifyProjectCards(miniProgram) {
  const home = await openHome(miniProgram);
  const root = await homeComponent(home);
  const cards = await root.$$(".project-card");
  const rows = [];
  for (let index = 0; index < cards.length; index += 1) {
    rows.push(await verifyNavigationRow(miniProgram, {
      id: `project.${index}`,
      selector: ".project-card",
      index,
      expected: ["pages/user/UserAssetDetailPage"],
      returnType: "back",
    }));
  }
  return rows;
}

async function verifyLocalControls(miniProgram) {
  let home = await openHome(miniProgram);
  let root = await homeComponent(home);
  const refresh = await scrollIntoView(miniProgram, home, ".inspiration-refresh", 0);
  const beforeTitles = await Promise.all((await root.$$(".inspiration-title")).map(item => item.text()));
  await withTimeout("tap inspiration refresh", refresh.tap(), 10000);
  await sleep(300);
  home = await miniProgram.currentPage();
  root = await homeComponent(home);
  const afterTitles = await Promise.all((await root.$$(".inspiration-title")).map(item => item.text()));
  if (beforeTitles.join("|") === afterTitles.join("|")) throw new Error("inspiration refresh did not rotate items");

  const tabs = await root.$$(".inspiration-tab");
  const tabResults = [];
  for (let index = 0; index < tabs.length; index += 1) {
    home = await openHome(miniProgram);
    const tab = await scrollIntoView(miniProgram, home, ".inspiration-tab", index);
    await withTimeout(`tap inspiration tab ${index}`, tab.tap(), 10000);
    await sleep(200);
    const className = await tab.attribute("class");
    if (!String(className).includes("active")) throw new Error(`inspiration tab ${index} not active`);
    tabResults.push(index);
  }
  return { refreshRotated: true, tabCount: tabResults.length };
}

async function main() {
  fs.mkdirSync(artifactDir, { recursive: true });
  fs.writeFileSync(progressFile, "", "utf8");
  if (progressFile !== latestProgressFile) fs.writeFileSync(latestProgressFile, "", "utf8");
  progress(`LOGIN ${apiBase}`);
  const auth = await login();
  progress("LOGIN OK");
  const result = { startedAt: new Date().toISOString(), filter: rowFilter, rows: [], failures: [], local: null };
  persist(result);
  progress(`CONNECT ${automationEndpoint}`);
  const miniProgram = await withTimeout("connect WeChat DevTools", connectMiniProgram(), 120000);
  try {
    progress("CONNECTED");
    const loginPage = await withTimeout("reLaunch login", miniProgram.reLaunch("/pages/WechatLoginPage"), 45000);
    await loginPage?.waitFor(1500);
    progress("LOGIN PAGE READY");
    await withTimeout("persist session", setSession(miniProgram, auth), 30000);
    progress("SESSION READY");
    await withTimeout("open initial home", openHome(miniProgram), 45000);
    progress("HOME READY");
    const selectedRows = navigationRows.filter(row => matchesFilter(row.id));
    for (const row of selectedRows) {
      progress(`START ${row.id}`);
      try {
        const checked = await verifyNavigationRow(miniProgram, row);
        result.rows.push(checked);
        progress(`PASS ${row.id} -> ${checked.target} -> ${checked.returned}`);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        result.failures.push({ id: row.id, message });
        progress(`FAIL ${row.id}: ${message}`);
        persist(result);
        if (message.includes("Connection closed")) break;
        await openHome(miniProgram).catch(() => null);
      }
      persist(result);
    }
    if (matchesFilter("project-cards")) try {
      const projects = await verifyProjectCards(miniProgram);
      result.rows.push(...projects);
      for (const row of projects) progress(`PASS ${row.id} -> ${row.target} -> ${row.returned}`);
    } catch (error) {
      result.failures.push({ id: "project-cards", message: error instanceof Error ? error.message : String(error) });
    }
    if (matchesFilter("inspiration-local")) try {
      result.local = await verifyLocalControls(miniProgram);
      progress(`PASS inspiration local controls: ${result.local.tabCount} tabs`);
    } catch (error) {
      result.failures.push({ id: "inspiration-local", message: error instanceof Error ? error.message : String(error) });
    }
    result.finishedAt = new Date().toISOString();
    result.passed = result.failures.length === 0;
    persist(result);
    console.log(JSON.stringify({ passed: result.passed, checked: result.rows.length, failures: result.failures, local: result.local }, null, 2));
    if (!result.passed) process.exitCode = 1;
  } finally {
    miniProgram.disconnect();
  }
}

main().catch(error => {
  console.error(error);
  process.exit(1);
});
