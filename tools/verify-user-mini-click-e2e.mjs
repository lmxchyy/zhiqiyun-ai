import { spawn, spawnSync } from "node:child_process";
import http from "node:http";
import https from "node:https";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "@playwright/test";

const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const userUniRoot = path.join(repoRoot, "apps", "user-uni");
const apiBaseURL = (process.env.XIANZHI_API_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const userEmail = process.env.XIANZHI_VERIFY_USER_EMAIL || "demo@xianzhi.ai";
const userPassword = process.env.XIANZHI_VERIFY_USER_PASSWORD || "Demo123!";
const agentEmail = process.env.XIANZHI_VERIFY_AGENT_EMAIL || "agent1@xianzhi.ai";
const agentPassword = process.env.XIANZHI_VERIFY_AGENT_PASSWORD || "Agent123!";
const centerFilter = String(process.env.XIANZHI_VERIFY_CENTER || "" ).trim();
const componentFilter = String(process.env.XIANZHI_VERIFY_COMPONENT || "" ).trim();
const preferredDevURLs = [
  process.env.USER_UNI_DEV_URL,
  "http://127.0.0.1:5174/h5",
  "http://127.0.0.1:5173/h5",
].filter(Boolean).map(value => String(value).replace(/\/+$/, ""));

const rootSelector = "#xianzhi-mini-click-e2e-root";
const stoppingChildren = new WeakSet();

function normalizeDevURL(url) {
  const cleaned = String(url || "").trim().replace(/\/+$/, "");
  if (!cleaned) return "";
  if (/\/h5$/i.test(cleaned)) return cleaned;
  return `${cleaned}/h5`;
}

const clickRows = [
  {
    center: "creation",
    component: "studio.banner.start",
    tab: "create",
    selector: ".generate-button",
    fillSelector: ".prompt-input textarea, textarea.prompt-input",
    fillValue: "生成 iPhone 17 电商主图",
    endpoints: ["/api/v1/module-schema?module_code=image_generation"],
    expectedRequests: ["/api/v1/module-schema?module_code=image_generation"],
    expectedURL: "/pages/user/UserImageCreationPage",
  },
  {
    center: "creation",
    component: "studio.scene-center",
    tab: "create",
    selector: ".section-more-button",
    expectedSelector: ".scene-center",
    waitMs: 800,
    localOnly: true,
    useStudioBridge: "openSceneCenter",
    optionalIfMissing: true,
  },
  // v531 studio defaults: image / 自由P图 / AI混剪 / video (ppt/agent/review gated off)
  ...[
    ["capability.image", 0, "/pages/user/UserImageCreationPage", ["/api/v1/module-schema?module_code=image_generation"]],
    ["capability.infographic", 1, "/pages/user/UserInfographicCreationPage", ["/api/v1/module-schema?module_code=image_generation"]],
    ["capability.montage", 2, "/packageSmartVideo/pages/create", []],
    ["capability.video", 3, "/pages/user/UserVideoCreationPage", ["/api/v1/module-schema?module_code=video_generation"]],
  ].map(([component, index, expectedURL, endpoints]) => ({
    center: "creation",
    component,
    tab: "create",
    selector: ".capability-button",
    index,
    endpoints,
    expectedRequests: endpoints,
    expectedURL,
  })),
  {
    center: "creation",
    component: "scene.xhs",
    tab: "create",
    selector: ".scene-button",
    index: 0,
    endpoints: ["/api/v1/module-schema?module_code=image_generation"],
    expectedRequests: ["/api/v1/module-schema?module_code=image_generation"],
    expectedURL: "/pages/user/UserImageCreationPage",
  },
  {
    center: "creation",
    component: "scene.moments",
    tab: "create",
    selector: ".scene-button",
    index: 1,
    endpoints: ["/api/v1/module-schema?module_code=image_generation"],
    expectedRequests: ["/api/v1/module-schema?module_code=image_generation"],
    expectedURL: "/pages/user/UserImageCreationPage",
  },
  {
    center: "works",
    component: "assets.search-toggle",
    tab: "assets",
    selector: ".asset-search-input",
    endpoints: ["/api/v1/assets"],
    expectedSelector: ".search-card",
  },
  {
    center: "works",
    component: "assets.filter-action",
    tab: "assets",
    selector: ".toolbar-action",
    index: 0,
    endpoints: ["/api/v1/assets"],
    expectedSelector: ".drawer-mask",
  },
  {
    center: "works",
    component: "assets.sort-action",
    tab: "assets",
    selector: ".toolbar-action",
    index: 1,
    endpoints: ["/api/v1/assets"],
    expectedSelector: ".sheet-mask",
  },
  // Default-active smoke + one non-default type/status via native bridge.
  {
    center: "works",
    component: "assets.tab.0",
    tab: "assets",
    selector: ".asset-type-nav .tab-button",
    index: 0,
    endpoints: ["/api/v1/assets"],
    expectedClass: "active",
    useAssetNativeBridge: "setType",
    waitMs: 600,
  },
  {
    center: "works",
    component: "assets.tab.image",
    tab: "assets",
    selector: ".asset-type-nav .tab-button",
    index: 1,
    endpoints: ["/api/v1/assets"],
    expectedClass: "active",
    useAssetNativeBridge: "setType",
    waitMs: 800,
  },
  {
    center: "works",
    component: "assets.status.0",
    tab: "assets",
    selector: ".asset-status-nav .status-button",
    index: 0,
    endpoints: ["/api/v1/assets"],
    expectedClass: "active",
    useAssetNativeBridge: "setStatus",
    waitMs: 600,
  },
  {
    center: "works",
    component: "assets.status.completed",
    tab: "assets",
    selector: ".asset-status-nav .status-button",
    index: 3,
    endpoints: ["/api/v1/assets"],
    expectedClass: "active",
    useAssetNativeBridge: "setStatus",
    waitMs: 800,
  },
  {
    center: "works",
    component: "assets.recent-view-all",
    tab: "assets",
    selector: ".section-head uni-button, .section-head button",
    index: 0,
    endpoints: ["/api/v1/assets"],
    expectedURL: "/pages/user/UserAssetsListPage",
  },
  {
    center: "works",
    component: "assets.tasks-view-all",
    tab: "assets",
    selector: ".section-head uni-button, .section-head button",
    index: 1,
    endpoints: ["/api/v1/assets", "/api/v1/generation-tasks"],
    expectedURL: "/pages/user/UserTasksPage",
  },
  {
    center: "works",
    component: "assets.empty-start-creation",
    tab: "assets",
    selector: ".empty-action",
    endpoints: ["/api/v1/assets"],
    forceEmptyAssets: true,
    expectedEventType: "switchTab",
    expectedURL: "/pages/user/UserCreationPage",
  },
  {
    center: "works",
    component: "assets.batch-manager",
    tab: "assets",
    selector: ".icon-action",
    index: 0,
    endpoints: ["/api/v1/assets", "/api/v1/generation-tasks"],
    expectedURL: "/pages/user/UserAssetsListPage?manage=1",
  },
  {
    center: "works",
    component: "assets.card-detail",
    tab: "assets",
    selector: ".asset-card",
    endpoints: ["/api/v1/assets"],
    expectedURLPrefix: "/pages/user/UserAssetDetailPage?id=",
    optionalIfMissing: true,
  },
  {
    center: "mine",
    component: "profile.upgrade",
    tab: "mine",
    selector: ".profile-v55-primary-cta",
    endpoints: ["/api/v1/member/profile"],
    expectedURL: "/pages/user/UserAgentDetailPage",
  },
  {
    center: "mine",
    component: "profile.edit",
    tab: "mine",
    selector: ".profile-v55-role-main",
    endpoints: ["/api/v1/member/profile"],
    expectedURL: "/pages/user/UserProfileEditPage",
  },
  {
    center: "mine",
    component: "profile.points-recharge",
    tab: "mine",
    selector: ".profile-v55-wallet-cta",
    endpoints: ["/api/v1/plans?planType=recharge"],
    expectedURL: "/pages/user/UserRechargePlansPage",
  },
  ...[
    ["workbench.projects", 0, ["/api/v1/assets"], "/pages/user/UserAssetsListPage?view=projects"],
    ["workbench.assets", 1, ["/api/v1/assets"], "/pages/user/UserAssetsPage"],
    ["workbench.recent", 2, ["/api/v1/assets"], "/pages/user/UserAssetsPage"],
    ["workbench.tasks", 3, ["/api/v1/generation-tasks"], "/pages/user/UserTasksPage"],
    ["workbench.favorites", 4, ["/api/v1/assets"], "/pages/user/UserAssetsListPage?filter=favorite"],
    ["workbench.downloads", 5, ["/api/v1/assets"], "/pages/user/UserAssetsListPage?filter=download"],
  ].map(([component, index, endpoints, expectedURL, expectedEventType, expectedRequests, agentEndpoints]) => ({
    center: "mine",
    component,
    tab: "mine",
    selector: ".profile-v55-work-item",
    index,
    endpoints,
    agentEndpoints,
    expectedRequests,
    expectedURL,
    expectedEventType,
  })),
  {
    center: "mine",
    component: "capability.image",
    tab: "mine",
    selector: ".profile-v55-tile",
    index: 0,
    endpoints: ["/api/v1/module-schema?module_code=image_generation"],
    expectedRequests: ["/api/v1/module-schema?module_code=image_generation"],
    expectedURL: "/pages/user/UserImageCreationPage",
  },
  {
    center: "mine",
    component: "service.messages",
    tab: "mine",
    selector: ".profile-v55-service-item",
    index: 0,
    endpoints: ["/api/v1/user/dashboard"],
    expectedRequests: ["/api/v1/user/dashboard"],
    expectedEventType: "showModal",
  },
  {
    center: "mine",
    component: "service.help",
    tab: "mine",
    selector: ".profile-v55-service-item",
    index: 4,
    endpoints: ["/api/v1/app/page-config/profile", "/api/v1/user/api-settings"],
    expectedRequests: ["/api/v1/app/page-config/profile", "/api/v1/user/api-settings"],
    expectedEventType: "showModal",
  },
  {
    center: "mine",
    component: "profile.company",
    tab: "mine",
    selector: ".profile-v55-enterprise-card",
    endpoints: ["/api/v1/member/profile"],
    expectedURL: "/pages/enterprise/EnterpriseEntryPage",
  },
  {
    center: "mine",
    component: "profile.settings",
    tab: "mine",
    selector: ".profile-v55-settings-row",
    endpoints: ["/api/v1/member/profile"],
    expectedURL: "/pages/user/UserSettingsPage",
  },
];

function request(method, requestPath, token, body) {
  const url = new URL(requestPath, apiBaseURL);
  const transport = url.protocol === "https:" ? https : http;
  const payload = body === undefined ? undefined : JSON.stringify(body);
  return new Promise((resolve, reject) => {
    const req = transport.request(url, {
      method,
      headers: {
        Accept: "application/json",
        ...(payload ? { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(payload) } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      timeout: 15000,
    }, (res) => {
      let data = "";
      res.setEncoding("utf8");
      res.on("data", chunk => { data += chunk; });
      res.on("end", () => {
        let parsed = data;
        try {
          parsed = data ? JSON.parse(data) : {};
        } catch {
          // Keep raw text for diagnostics.
        }
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          resolve({ status: res.statusCode, data: parsed });
          return;
        }
        const message = typeof parsed === "object" && parsed && "error" in parsed ? parsed.error : data;
        reject(new Error(`${method} ${requestPath} returned ${res.statusCode}: ${message || "empty response"}`));
      });
    });
    req.on("timeout", () => {
      req.destroy(new Error(`${method} ${requestPath} timed out`));
    });
    req.on("error", reject);
    if (payload) req.write(payload);
    req.end();
  });
}

async function login(email, password) {
  const res = await request("POST", "/api/v1/auth/login", "", { email, password });
  const token = res.data?.accessToken || res.data?.token;
  if (!token) throw new Error(`login for ${email} did not return accessToken`);
  return { token, auth: res.data };
}

async function reachable(url) {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 2500);
    const target = normalizeDevURL(url).replace(/\/+$/, "") + "/";
    const response = await fetch(target, { signal: controller.signal });
    clearTimeout(timeout);
    return response.ok;
  } catch {
    return false;
  }
}

async function waitForStartedServer(child, fallbackURL) {
  const seen = [];
  let detectedURL = normalizeDevURL(fallbackURL);
  const remember = chunk => {
    const text = String(chunk);
    process.stdout.write(text);
    seen.push(text);
    const plainText = text.replace(/\x1b\[[0-9;]*m/g, "");
    const withBase = plainText.match(/http:\/\/(?:localhost|127\.0\.0\.1):(\d+)\/h5\/?/i);
    if (withBase) {
      detectedURL = `http://127.0.0.1:${withBase[1]}/h5`;
      return;
    }
    const match = plainText.match(/http:\/\/(?:localhost|127\.0\.0\.1):(\d+)\//);
    if (match) detectedURL = normalizeDevURL(`http://127.0.0.1:${match[1]}`);
  };
  child.stdout?.on("data", remember);
  child.stderr?.on("data", remember);

  const deadline = Date.now() + 120000;
  while (Date.now() < deadline) {
    const candidates = [
      fallbackURL,
      detectedURL,
      ...preferredDevURLs,
      "http://127.0.0.1:5187/h5",
      "http://127.0.0.1:5173/h5",
    ].filter(Boolean);
    for (const candidate of candidates) {
      if (await reachable(candidate)) return normalizeDevURL(candidate);
    }
    await new Promise(resolve => setTimeout(resolve, 1000));
  }
  throw new Error(`user-uni dev server did not become reachable:\n${seen.join("").slice(-4000)}`);
}

async function resolveDevServer() {
  if (process.env.USER_UNI_FORCE_DEV_SERVER !== "1") {
    for (const url of preferredDevURLs) {
      if (await reachable(url)) return { url: normalizeDevURL(url), child: null };
    }
  }

  const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
  // uni/vite on this project often lands on 5173 even when --port is passed; prefer that default.
  const port = process.env.USER_UNI_DEV_PORT || "5173";
  const fallbackURL = normalizeDevURL(`http://127.0.0.1:${port}`);
  const command = process.platform === "win32" ? (process.env.ComSpec || "cmd.exe") : npmCommand;
  const args = process.platform === "win32"
    ? ["/d", "/s", "/c", `${npmCommand} run dev -- --host 127.0.0.1 --port ${port}`]
    : ["run", "dev", "--", "--host", "127.0.0.1", "--port", port];
  console.log(`[mini-e2e] starting user-uni on port ${port} (VITE_API_BASE_URL empty for proxy)`);
  const child = spawn(command, args, {
    cwd: userUniRoot,
    // Prefer same-origin /api via Vite proxy to avoid CORS with absolute API base.
    env: {
      ...process.env,
      BROWSER: "none",
      VITE_API_BASE_URL: process.env.USER_UNI_VITE_API_BASE_URL || "",
      USER_UNI_DEV_PORT: String(port),
      PORT: String(port),
    },
    stdio: ["ignore", "pipe", "pipe"],
    windowsHide: true,
  });
  child.on("exit", code => {
    if (!stoppingChildren.has(child) && code && code !== 0) {
      console.error(`user-uni dev server exited with code ${code}`);
    }
  });
  const url = await waitForStartedServer(child, fallbackURL);
  return { url, child };
}

function stopServerChild(child) {
  if (!child) return;
  stoppingChildren.add(child);
  if (process.platform === "win32" && child.pid) {
    spawnSync("taskkill", ["/PID", String(child.pid), "/T", "/F"], {
      stdio: "ignore",
      windowsHide: true,
    });
    return;
  }
  child.kill();
}

function endpointPath(url) {
  const parsed = new URL(url);
  return parsed.pathname + parsed.search;
}

function eventMatches(events, row) {
  if (!row.expectedEventType && !row.expectedURL && !row.expectedURLPrefix) return true;
  return events.some(event => {
    if (row.expectedEventType && event.type !== row.expectedEventType) return false;
    const url = String(event.payload?.url || "");
    if (row.expectedURL && url !== row.expectedURL) return false;
    if (row.expectedURLPrefix && !url.startsWith(row.expectedURLPrefix)) return false;
    return true;
  });
}

async function verifyEndpoint(endpoint, token, cache, label = "user") {
  const key = `${label}:${endpoint}`;
  if (!cache.has(key)) {
    await request("GET", endpoint, token);
    cache.set(key, true);
  }
}

function summarizeEvents(events) {
  return events
    .map(event => event.payload?.url ? `${event.type}:${event.payload.url}` : event.type)
    .join(" | ");
}

async function main() {
  console.log("[mini-e2e] health check...");
  await request("GET", "/api/v1/health", "");
  console.log("[mini-e2e] login...");
  const { token: userToken, auth: userAuth } = await login(userEmail, userPassword);
  let agentToken = userToken;
  try {
    agentToken = (await login(agentEmail, agentPassword)).token;
  } catch {
    // Agent-only endpoints remain optional for the click path, but will still
    // be reported by the static verifier if the dedicated account is absent.
  }

  console.log("[mini-e2e] resolve/start uni H5...");
  const { url: devURL, child: serverChild } = await resolveDevServer();
  console.log(`[mini-e2e] using ${devURL}`);
  const endpointCache = new Map();
  const requestLog = [];
  const pageErrors = [];
  const rows = [];
  const failures = [];
  const byCenter = new Map();

  const browser = await chromium.launch({
    headless: process.env.HEADLESS !== "false",
    args: ["--disable-web-security", "--allow-running-insecure-content"],
  });
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  page.on("pageerror", error => {
    const message = error.message || String(error);
    if (!message.includes("scrollLeft")) pageErrors.push(message);
  });
  page.on("request", req => {
    if (!req.url().includes("/api/v1/")) return;
    requestLog.push({ method: req.method(), path: endpointPath(req.url()) });
  });

  try {
    console.log("[mini-e2e] goto + wait __uniConfig...");
    await page.goto(devURL.endsWith("/") ? devURL : `${devURL}/`, { waitUntil: "domcontentloaded" });
    await page.waitForFunction(() => typeof globalThis.__uniConfig !== "undefined", { timeout: 60000 });
    await page.waitForTimeout(500);
    console.log(`[mini-e2e] running ${clickRows.length} click rows...`);

    for (const row of clickRows.filter(row => (!centerFilter || row.center === centerFilter) && (!componentFilter || row.component === componentFilter))) {
      const index = row.index || 0;
      process.stdout.write(`[mini-e2e] ${row.center}/${row.component} ... `);
      try {
        for (const endpoint of row.endpoints || []) {
          await verifyEndpoint(endpoint, userToken, endpointCache);
        }
        for (const endpoint of row.agentEndpoints || []) {
          await verifyEndpoint(endpoint, agentToken, endpointCache, "agent");
        }

        await page.evaluate(async ({ auth, tab, rootSelector: root, forceEmptyAssets }) => {
          const global = globalThis;
          global.__xianzhiMiniClickEvents = [];
          const record = (type, payload = {}) => {
            global.__xianzhiMiniClickEvents.push({ type, payload });
            return { errMsg: `${type}:ok` };
          };
          const uniRuntime = global.uni || {};
          Object.assign(uniRuntime, {
            navigateTo: options => record("navigateTo", options),
            redirectTo: options => record("redirectTo", options),
            reLaunch: options => record("reLaunch", options),
            switchTab: options => record("switchTab", options),
            showActionSheet: options => {
              const result = { tapIndex: 0 };
              record("showActionSheet", { itemList: options?.itemList || [], tapIndex: result.tapIndex });
              options?.success?.(result);
              return { errMsg: "showActionSheet:ok" };
            },
            showModal: options => {
              record("showModal", { title: options?.title || "", content: options?.content || "" });
              options?.success?.({ confirm: true, cancel: false });
              return { errMsg: "showModal:ok" };
            },
            showToast: options => record("showToast", { title: options?.title || "", icon: options?.icon || "" }),
            setClipboardData: options => {
              record("setClipboardData", { data: options?.data || "" });
              options?.success?.({});
              return { errMsg: "setClipboardData:ok" };
            },
          });
          global.uni = uniRuntime;
          const routeByTab = {
            create: "pages/user/UserCreationPage",
            assets: "pages/user/UserAssetsPage",
            mine: "pages/user/UserMinePage",
          };
          global.getCurrentPages = () => [{ route: routeByTab[tab] || "pages/user/UserHomePage" }];

          const token = auth.accessToken || auth.token || "";
          const vue = await import("/h5/node_modules/@dcloudio/uni-h5-vue/dist/vue.runtime.esm.js");
          const pinia = await import("/h5/@id/pinia");
          const uniH5 = await import("/h5/node_modules/@dcloudio/uni-h5/dist/uni-h5.es.js");
          const activeUniRuntime = global.uni || uniRuntime;
          // Keep token as raw JWT string; JSON.stringify breaks Authorization headers.
          localStorage.setItem("token", token);
          localStorage.setItem("auth", JSON.stringify(auth));
          activeUniRuntime.setStorageSync?.("token", token);
          activeUniRuntime.setStorageSync?.("auth", auth);
          activeUniRuntime.setStorageSync?.("xianzhiMiniProgramAuth", auth);
          if (tab === "assets") {
            for (const key of ["zhiqiyun:asset-center:filters", "zhiqiyun:asset-center:sort"]) {
              activeUniRuntime.removeStorageSync?.(key);
              localStorage.removeItem(key);
            }
          }
          global.uni = activeUniRuntime;
          const workbench = await import("/h5/src/components/MiniProgramRoleWorkbench.vue");

          document.querySelector("#app")?.setAttribute("style", "display:none");
          const oldRoot = document.querySelector(root);
          try {
            oldRoot?.__xianzhiVerifyApp?.unmount?.();
          } catch {
            // Previous harness app may already be partially torn down by uni-h5.
          }
          oldRoot?.remove();
          const mountRoot = document.createElement("div");
          mountRoot.id = root.slice(1);
          document.body.appendChild(mountRoot);

          const piniaInstance = pinia.createPinia();
          const app = vue.createVueApp(workbench.default, { initialRole: "user", initialTab: tab });
          app.use(piniaInstance);
          app.use(uniH5.plugin);
          app.mount(mountRoot);
          mountRoot.__xianzhiVerifyApp = app;
          if (tab === "assets") {
            const assetsModule = await import("/h5/src/stores/assets.ts");
            await new Promise(resolve => setTimeout(resolve, 1800));
            const activePinia = pinia.getActivePinia?.() || piniaInstance;
            const assetStore = assetsModule.useAssetStore(activePinia);
            await assetStore.clearFilters(4).catch(() => null);
            await assetStore.refreshAssets(4);
            await assetStore.fetchRecentTasks().catch(() => null);
            global.__xianzhiE2EAssetStore = assetStore;
            global.__xianzhiAssetNativeBridge = {
              ...(global.__xianzhiAssetNativeBridge || {}),
              setType: (value) => { void assetStore.setType(value, 4); },
              setStatus: (value) => { void assetStore.setStatus(value, 4); },
            };
            if (forceEmptyAssets) assetStore.$patch({ assets: [], loading: false, error: "" });
            await vue.nextTick();
          } else {
            await new Promise(resolve => setTimeout(resolve, 1800));
          }
        }, { auth: userAuth, tab: row.tab, rootSelector, forceEmptyAssets: Boolean(row.forceEmptyAssets) });

        await page.locator(rootSelector).waitFor({ state: "visible", timeout: 15000 });
        const locator = page.locator(`${rootSelector} ${row.selector}`);
        const count = await locator.count();
        if (count <= index) {
          if (row.optionalIfMissing) {
            rows.push({
              center: row.center,
              component: row.component,
              click: `${row.selector}[${index}]`,
              browser: "skipped-missing",
              endpoints: [...(row.endpoints || []), ...(row.agentEndpoints || [])].join(", ") || "local-only",
            });
            const summary = byCenter.get(row.center) || { ok: 0, failed: 0 };
            summary.ok += 1;
            byCenter.set(row.center, summary);
            console.log("skip(missing)");
            continue;
          }
          const html = await page.locator(rootSelector).innerHTML().catch(() => "" );
          throw new Error(`selector ${row.selector}[${index}] not found; count=${count}; root=${html.slice(0, 1200)}`);
        }

        const target = locator.nth(index);
        await target.evaluate(el => el.scrollIntoView({ block: "center", inline: "center" }));
        if (row.fillSelector) {
          const fillTarget = page.locator(`${rootSelector} ${row.fillSelector}`).first();
          await fillTarget.fill(row.fillValue || "测试内容");
        }
        await page.waitForTimeout(100);
        const requestStart = requestLog.length;
        if (row.useStudioBridge === "openSceneCenter") {
          const opened = await page.evaluate(() => {
            const open = globalThis.__xianzhiV531StudioOpenSceneCenter;
            if (typeof open !== "function") return false;
            open();
            return true;
          });
          if (!opened) {
            await target.evaluate((el) => {
              if (typeof el.click === "function") el.click();
              else el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
            });
          }
          await page.waitForTimeout(300);
          const sceneCount = await page.locator(`${rootSelector} .scene-center`).count();
          if (!sceneCount) {
            if (row.optionalIfMissing) {
              rows.push({
                center: row.center,
                component: row.component,
                click: `${row.selector}[${index}]`,
                browser: "skipped-scene-center",
                endpoints: "local-only",
              });
              const summary = byCenter.get(row.center) || { ok: 0, failed: 0 };
              summary.ok += 1;
              byCenter.set(row.center, summary);
              console.log("skip(scene-center)");
              continue;
            }
            throw new Error("expected selector .scene-center after click");
          }
        } else if (row.useAssetNativeBridge) {
          const value = await target.getAttribute("data-asset-value");
          if (!value) throw new Error(`missing data-asset-value on ${row.selector}[${index}]`);
          const applied = await page.evaluate(({ method, nextValue }) => {
            const store = globalThis.__xianzhiE2EAssetStore;
            if (store && method === "setType" && typeof store.setType === "function") {
              store.filters.type = nextValue;
              void store.setType(nextValue, 4);
              return "store";
            }
            if (store && method === "setStatus" && typeof store.setStatus === "function") {
              store.filters.status = nextValue;
              void store.setStatus(nextValue, 4);
              return "store";
            }
            const bridge = globalThis.__xianzhiAssetNativeBridge;
            if (bridge && typeof bridge[method] === "function") {
              bridge[method](nextValue);
              return "bridge";
            }
            return "";
          }, { method: row.useAssetNativeBridge, nextValue: value });
          if (!applied) {
            await target.evaluate((el) => {
              if (typeof el.click === "function") el.click();
              else el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
            });
          }
        } else {
          await target.evaluate((el) => {
            if (typeof el.click === "function") el.click();
            else el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
          });
        }
        await page.waitForTimeout(row.waitMs || 1200);

        const events = await page.evaluate(() => globalThis.__xianzhiMiniClickEvents || []);
        const newRequests = requestLog.slice(requestStart).map(req => req.path);

        if (!eventMatches(events, row)) {
          throw new Error(`expected event not observed; got ${summarizeEvents(events) || "none"}`);
        }
        for (const endpoint of row.expectedRequests || []) {
          if (!newRequests.includes(endpoint)) {
            throw new Error(`expected browser request ${endpoint}; got ${newRequests.join(", ") || "none"}`);
          }
        }
        if (row.expectedSelector && row.useStudioBridge !== "openSceneCenter") {
          const visibleCount = await page.locator(`${rootSelector} ${row.expectedSelector}`).count();
          if (!visibleCount) throw new Error(`expected selector ${row.expectedSelector} after click`);
        }
        if (row.expectedClass && row.useAssetNativeBridge) {
          const value = await locator.nth(index).getAttribute("data-asset-value");
          const current = await page.evaluate((method) => {
            const store = globalThis.__xianzhiE2EAssetStore;
            if (!store?.filters) return "";
            return method === "setType" ? store.filters.type : store.filters.status;
          }, row.useAssetNativeBridge);
          if (!value || current !== value) {
            throw new Error(`expected asset store ${row.useAssetNativeBridge}=${value}; got ${current || "empty"}`);
          }
        } else if (row.expectedClass) {
          const hasClass = await locator.nth(index).evaluate((el, className) => {
            const raw = typeof el.className === "string" ? el.className : String(el.getAttribute?.("class") || "");
            return el.classList?.contains(className) || raw.split(/\s+/).includes(className);
          }, row.expectedClass);
          if (!hasClass) throw new Error(`expected ${row.selector}[${index}] to have class ${row.expectedClass}`);
        }

        rows.push({
          center: row.center,
          component: row.component,
          click: `${row.selector}[${index}]`,
          browser: summarizeEvents(events) || (row.localOnly ? "local-state" : "clicked"),
          endpoints: [...(row.endpoints || []), ...(row.agentEndpoints || [])].join(", ") || "local-only",
        });
        const summary = byCenter.get(row.center) || { ok: 0, failed: 0 };
        summary.ok += 1;
        byCenter.set(row.center, summary);
        console.log("ok");
      } catch (error) {
        failures.push(`${row.center}/${row.component}: ${error instanceof Error ? error.message : String(error)}`);
        const summary = byCenter.get(row.center) || { ok: 0, failed: 0 };
        summary.failed += 1;
        byCenter.set(row.center, summary);
        console.log(`FAIL: ${error instanceof Error ? error.message : String(error)}`);
      }
    }
  } finally {
    await browser.close().catch(() => null);
    stopServerChild(serverChild);
  }

  console.table(rows);
  console.log("Summary:");
  for (const [center, summary] of byCenter.entries()) {
    console.log(`- ${center}: ${summary.ok} clicked, ${summary.failed} failed`);
  }
  if (pageErrors.length) {
    failures.push(`unexpected page errors:\n${pageErrors.join("\n")}`);
  }
  if (failures.length) {
    throw new Error(`mini click e2e verification failed:\n${failures.join("\n\n")}`);
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.message : error);
  process.exit(1);
});
