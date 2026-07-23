import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { spawn } from "node:child_process";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");
const repoRoot = path.resolve(import.meta.dirname, "..");
const projectPath = path.join(repoRoot, "apps", "user-uni", "dist", "build", "mp-weixin");
const cliPath = process.env.WX_CLI_PATH || "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const automationPort = Number(process.env.WX_WORKS_PERF_AUTOMATOR_PORT || 9450);
const idePort = String(process.env.WX_WORKS_PERF_IDE_PORT || 33709);
const backendBase = (process.env.WORKS_PERF_BACKEND || "http://127.0.0.1:3199").replace(/\/+$/, "");
const proxyPort = Number(process.env.WORKS_PERF_PROXY_PORT || 3200);
const email = process.env.LOGIN_EMAIL || "demo@xianzhi.ai";
const password = process.env.LOGIN_PASSWORD || "Demo123!";
const artifactDir = path.join(repoRoot, "artifacts", "wechat-works-performance");
const resultPath = path.join(artifactDir, "result.json");
const filterKey = "zhiqiyun:asset-center:filters";
const sortKey = "zhiqiyun:asset-center:sort";
let activeMiniProgram;

const proxyState = {
  overviewDelayMs: 15000,
  recentDelayMs: 550,
  offlineRecent: false,
  counts: {
    recentStarted: 0,
    recentCompleted: 0,
    overviewStarted: 0,
    overviewCompleted: 0,
    taskStarted: 0,
    taskCompleted: 0,
  },
};

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function wait(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function progress(message) {
  const line = `${new Date().toISOString()} ${message}`;
  fs.mkdirSync(artifactDir, { recursive: true });
  fs.appendFileSync(path.join(artifactDir, "progress.log"), `${line}\n`, "utf8");
  console.log(line);
}

async function withTimeout(label, promise, timeoutMs) {
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

function startDelayProxy() {
  const target = new URL(backendBase);
  const server = http.createServer((request, response) => {
    const requestPath = request.url || "/";
    const isRecent = requestPath.startsWith("/api/v1/works/recent");
    const isOverview = requestPath.startsWith("/api/v1/assets/overview");
    const isTasks = requestPath.startsWith("/api/v1/generation-tasks");
    if (isRecent) proxyState.counts.recentStarted += 1;
    if (isOverview) proxyState.counts.overviewStarted += 1;
    if (isTasks) proxyState.counts.taskStarted += 1;
    if (isRecent && proxyState.offlineRecent) {
      request.socket.destroy();
      return;
    }
    const delayMs = isOverview ? proxyState.overviewDelayMs : isRecent ? proxyState.recentDelayMs : 0;
    setTimeout(() => {
      const upstream = http.request({
        hostname: target.hostname,
        port: Number(target.port || 80),
        path: requestPath,
        method: request.method,
        headers: { ...request.headers, host: target.host },
      }, upstreamResponse => {
        response.writeHead(upstreamResponse.statusCode || 502, upstreamResponse.headers);
        upstreamResponse.pipe(response);
        upstreamResponse.on("end", () => {
          if (isRecent) proxyState.counts.recentCompleted += 1;
          if (isOverview) proxyState.counts.overviewCompleted += 1;
          if (isTasks) proxyState.counts.taskCompleted += 1;
        });
      });
      upstream.on("error", error => {
        if (!response.headersSent) response.writeHead(502, { "content-type": "application/json" });
        response.end(JSON.stringify({ error: error.message }));
      });
      request.pipe(upstream);
    }, delayMs);
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(proxyPort, "127.0.0.1", () => resolve(server));
  });
}

async function login() {
  let last;
  for (let attempt = 0; attempt < 5; attempt += 1) {
    const response = await fetch(`${backendBase}/api/v1/auth/login`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    const payload = await response.json();
    if (response.ok && payload?.accessToken) return payload;
    last = `${response.status} ${JSON.stringify(payload)}`;
  }
  throw new Error(`登录失败: ${last}`);
}

async function connectMiniProgram() {
  const output = [];
  let activeIdePort = idePort;
  const launch = port => {
    const command = `& '${cliPath.replaceAll("'", "''")}' auto --project '${projectPath.replaceAll("'", "''")}' --port ${port} --auto-port ${automationPort} --trust-project`;
    const child = spawn("powershell.exe", ["-NoProfile", "-Command", command], {
      cwd: repoRoot,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    child.stdout.on("data", chunk => output.push(String(chunk)));
    child.stderr.on("data", chunk => output.push(String(chunk)));
  };
  launch(activeIdePort);
  let lastError;
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try {
      return await automator.connect({ wsEndpoint: `ws://127.0.0.1:${automationPort}` });
    } catch (error) {
      lastError = error;
      const portMatch = output.join("").match(/IDE server has started on http:\/\/127\.0\.0\.1:(\d+)/i);
      if (portMatch?.[1] && portMatch[1] !== activeIdePort) {
        activeIdePort = portMatch[1];
        launch(activeIdePort);
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
      return await automator.connect({ wsEndpoint: `ws://127.0.0.1:${automationPort}` });
    } catch (error) {
      lastError = error;
      await wait(500);
    }
  }
  throw new Error(`微信自动化重连失败: ${lastError instanceof Error ? lastError.message : String(lastError)}`);
}

async function safeSwitchTab(pathname) {
  try {
    return await activeMiniProgram.switchTab(pathname);
  } catch (error) {
    if (!/Connection closed|timeout/i.test(String(error instanceof Error ? error.message : error))) throw error;
    activeMiniProgram = await reconnectMiniProgram();
    const current = await activeMiniProgram.currentPage();
    if (current?.path === pathname.replace(/^\/+/, "")) return current;
    return activeMiniProgram.switchTab(pathname);
  }
}

async function waitForElement(root, selector, timeoutMs = 5000, intervalMs = 20) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const element = await root.$(selector).catch(() => null);
    if (element) return element;
    await wait(intervalMs);
  }
  return null;
}

async function worksCenter(page) {
  const workbench = await waitForElement(page, "mini-program-role-workbench", 5000);
  assert(workbench, "作品页未渲染工作台");
  const center = await waitForElement(workbench, "asset-center-page", 5000);
  assert(center, "作品页未渲染资产中心");
  return center;
}

async function resetPerformance() {
  await activeMiniProgram.evaluate(() => {
    globalThis.__XIANZHI_WORKS_PERF__ = true;
    globalThis.__XIANZHI_WORKS_PERF_EVENTS__ = [];
    globalThis.__XIANZHI_WORKS_TAB_CLICK_AT__ = Date.now();
  });
}

async function performanceEvents() {
  return activeMiniProgram.evaluate(() => globalThis.__XIANZHI_WORKS_PERF_EVENTS__ || []);
}

async function openWorksAndMeasure() {
  await resetPerformance();
  const wallStartedAt = Date.now();
  const page = await withTimeout("switchTab works", safeSwitchTab("/pages/user/UserAssetsPage"), 20000);
  const center = await worksCenter(page);
  const card = await waitForElement(center, ".asset-card", 5000, 10);
  const visibleAt = Date.now();
  assert(card, "作品卡片未显示");
  await wait(20);
  const events = await performanceEvents();
  const renderEvent = [...events].reverse().find(item => item.step === "first_screen_render");
  return {
    page,
    center,
    wallMs: visibleAt - wallStartedAt,
    renderMs: renderEvent?.durationMs ?? null,
    events,
  };
}

async function main() {
  fs.mkdirSync(artifactDir, { recursive: true });
  fs.writeFileSync(path.join(artifactDir, "progress.log"), "", "utf8");
  const proxy = await startDelayProxy();
  const result = {
    startedAt: new Date().toISOString(),
    backendBase,
    proxyBase: `http://127.0.0.1:${proxyPort}`,
    checks: {},
    proxyCounts: proxyState.counts,
    consoleErrors: [],
  };
  try {
    progress("login backend");
    const auth = await login();
    progress("connect WeChat DevTools");
    activeMiniProgram = await withTimeout("connect WeChat DevTools", connectMiniProgram(), 75000);
    await wait(2000);
    activeMiniProgram.on("console", message => {
      const text = message.args.join(" ");
      if (text.includes("[works-perf]")) return;
      if (["error", "warning"].includes(message.type)) result.consoleErrors.push(`${message.type}: ${text}`);
    });
    activeMiniProgram.on("exception", error => result.consoleErrors.push(`exception: ${error.message || String(error)}`));
    progress("prepare login storage");
    await activeMiniProgram.callWxMethod("setStorageSync", "token", auth.accessToken);
    progress("stored token");
    await activeMiniProgram.callWxMethod("setStorageSync", "refreshToken", auth.refreshToken || "");
    progress("stored refresh token");
    await activeMiniProgram.callWxMethod("setStorageSync", "auth", auth);
    progress("stored auth");
    await activeMiniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", auth);
    progress("stored legacy auth");
    progress("login storage ready");
    await activeMiniProgram.callWxMethod("setStorageSync", filterKey, {
      type: "all",
      status: "recent",
      keyword: "",
      projectId: "",
      tagIds: [],
      model: "",
      createdFrom: "",
      createdTo: "",
    });
    await activeMiniProgram.callWxMethod("setStorageSync", sortKey, "created_desc");
    const scope = `${auth.user.id}:${auth.tenantId || auth.user.tenantId || "tenant_default"}`;
    const cacheKey = `recent_works_cache:${encodeURIComponent(scope)}`;
    await activeMiniProgram.callWxMethod("removeStorageSync", cacheKey);
    await activeMiniProgram.evaluate(() => {
      globalThis.__XIANZHI_WORKS_PERF__ = true;
      globalThis.__XIANZHI_WORKS_PERF_EVENTS__ = [];
    });

    progress("first load without cache");
    const firstPagePromise = safeSwitchTab("/pages/user/UserAssetsPage");
    const firstPage = await firstPagePromise;
    const firstCenter = await worksCenter(firstPage);
    const skeleton = await waitForElement(firstCenter, "asset-skeleton", 300, 10);
    const blockingRefresh = await firstCenter.$(".refresh-indicator");
    const firstStartedAt = Date.now();
    const firstCard = await waitForElement(firstCenter, ".asset-card", 3000, 10);
    const firstCardMs = Date.now() - firstStartedAt;
    assert(skeleton, "无缓存首屏未显示作品骨架屏");
    assert(!blockingRefresh, "无缓存首屏出现阻塞式资产中心刷新提示");
    assert(firstCard, "独立最近作品接口未在目标时间内返回卡片");
    const overviewWasPending = proxyState.counts.overviewStarted > proxyState.counts.overviewCompleted;
    assert(overviewWasPending, "15秒资产概览延迟未生效，无法验证解耦");
    result.checks.firstLoad = {
      skeleton: true,
      blockingRefresh: false,
      cardVisibleAfterWaitMs: firstCardMs,
      overviewDelayMs: proxyState.overviewDelayMs,
      overviewWasPending,
    };
    proxyState.recentDelayMs = 0;
    await wait(200);
    const cache = await activeMiniProgram.callWxMethod("getStorageSync", cacheKey);
    assert(cache?.assets?.length > 0, "真实最近作品未写入同步缓存");
    result.checks.cacheWrite = {
      key: cacheKey,
      itemCount: cache.assets.length,
      storedAt: cache.storedAt,
    };

    progress("warm cache ten entries");
    const warmEntries = [];
    for (let index = 0; index < 10; index += 1) {
      await safeSwitchTab("/pages/user/UserHomePage");
      await wait(30);
      const measurement = await openWorksAndMeasure();
      warmEntries.push({
        index: index + 1,
        renderMs: measurement.renderMs,
        wallMs: measurement.wallMs,
        requestEvents: measurement.events.filter(item => item.step === "recent_works_request"),
      });
    }
    const renderDurations = warmEntries.map(item => item.renderMs).filter(Number.isFinite);
    assert(renderDurations.length === 10, `有缓存10次进入缺少首屏渲染日志: ${JSON.stringify(warmEntries)}`);
    assert(renderDurations.every(value => value <= 100), `有缓存首屏超过100ms: ${JSON.stringify(renderDurations)}`);
    result.checks.warmCacheTenEntries = {
      pass: true,
      renderDurationsMs: renderDurations,
      maxRenderMs: Math.max(...renderDurations),
      wallDurationsMs: warmEntries.map(item => item.wallMs),
    };

    progress("offline cache");
    await safeSwitchTab("/pages/user/UserHomePage");
    await wait(3200);
    proxyState.offlineRecent = true;
    const offlineMeasurement = await openWorksAndMeasure();
    await wait(300);
    const offlineNote = await offlineMeasurement.center.$(".cached-content-note");
    const offlineText = offlineNote ? await offlineNote.text() : "";
    assert(offlineMeasurement.renderMs <= 100, `断网缓存首屏超过100ms: ${offlineMeasurement.renderMs}`);
    assert(offlineText.includes("当前展示缓存内容"), `断网缓存提示缺失: ${offlineText}`);
    result.checks.offlineCache = {
      cardRenderMs: offlineMeasurement.renderMs,
      message: offlineText,
    };
    proxyState.offlineRecent = false;

    progress("rapid five switches");
    await safeSwitchTab("/pages/user/UserHomePage");
    await wait(3200);
    const recentBefore = proxyState.counts.recentStarted;
    for (let index = 0; index < 5; index += 1) {
      await safeSwitchTab("/pages/user/UserAssetsPage");
      await wait(25);
      await safeSwitchTab("/pages/user/UserHomePage");
      await wait(25);
    }
    await wait(300);
    const rapidRequests = proxyState.counts.recentStarted - recentBefore;
    assert(rapidRequests <= 1, `快速切换5次发起了${rapidRequests}次最近作品请求`);
    result.checks.rapidFiveSwitches = {
      switches: 5,
      recentRequests: rapidRequests,
      pass: true,
    };

    result.finishedAt = new Date().toISOString();
    result.proxyCounts = { ...proxyState.counts };
    result.ok = true;
    fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
    console.log(JSON.stringify(result, null, 2));
  } finally {
    try {
      activeMiniProgram?.disconnect();
    } catch {
      // The IDE may already have closed its automation socket.
    }
    await new Promise(resolve => proxy.close(resolve));
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
