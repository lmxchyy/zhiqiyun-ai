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
  "http://127.0.0.1:5174",
  "http://127.0.0.1:5173",
].filter(Boolean).map(value => String(value).replace(/\/+$/, ""));

const rootSelector = "#xianzhi-mini-click-e2e-root";
const stoppingChildren = new WeakSet();

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
    localOnly: true,
  },
  ...[
    ["capability.image", 0, "/pages/user/UserImageCreationPage", ["/api/v1/module-schema?module_code=image_generation"]],
    ["capability.video", 1, "/pages/user/UserVideoCreationPage", ["/api/v1/module-schema?module_code=video_generation"]],
    ["capability.ppt", 2, "/pages/user/UserPptCreationPage", ["/api/v1/ppt/models/text", "/api/v1/ppt/models/image"]],
    ["capability.agent", 3, "/pages/user/UserAgentCreationPage", ["/api/v1/knowledge-agents"]],
    ["capability.infographic", 4, "/pages/user/UserInfographicCreationPage", ["/api/v1/module-schema?module_code=image_generation"]],
    ["capability.review", 5, "/pages/user/UserReviewCreationPage", ["/api/v1/knowledge-agents", "/api/v1/knowledge-conversations"]],
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
    component: "scene.company-ppt",
    tab: "create",
    selector: ".scene-button",
    index: 2,
    endpoints: ["/api/v1/ppt/models/text", "/api/v1/ppt/models/image"],
    expectedRequests: ["/api/v1/ppt/models/text", "/api/v1/ppt/models/image"],
    expectedURL: "/pages/user/UserPptCreationPage",
  },
  {
    center: "works",
    component: "assets.search-toggle",
    tab: "assets",
    selector: ".icon-action",
    endpoints: ["/api/v1/assets"],
    expectedSelector: ".search-card",
  },
  {
    center: "works",
    component: "assets.filter-action",
    tab: "assets",
    selector: ".text-action",
    index: 0,
    endpoints: ["/api/v1/assets"],
    expectedSelector: ".drawer-mask",
  },
  {
    center: "works",
    component: "assets.sort-action",
    tab: "assets",
    selector: ".text-action",
    index: 1,
    endpoints: ["/api/v1/assets"],
    expectedSelector: ".sheet-mask",
  },
  ...[0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map(index => ({
    center: "works",
    component: `assets.tab.${index}`,
    tab: "assets",
    selector: ".tab-button",
    index,
    endpoints: ["/api/v1/assets"],
    expectedClass: "active",
  })),
  ...[0, 1, 2, 3, 4, 5, 6, 7].map(index => ({
    center: "works",
    component: `assets.status.${index}`,
    tab: "assets",
    selector: ".status-button",
    index,
    endpoints: ["/api/v1/assets"],
    expectedClass: "active",
  })),
  {
    center: "works",
    component: "assets.recent-view-all",
    tab: "assets",
    selector: ".section-head uni-button",
    index: 1,
    endpoints: ["/api/v1/assets"],
    expectedURL: "/pages/user/UserAssetsListPage",
  },
  {
    center: "works",
    component: "assets.tasks-view-all",
    tab: "assets",
    selector: ".section-head uni-button",
    index: 2,
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
  },  {
    center: "works",
    component: "assets.batch-manager",
    tab: "assets",
    selector: ".text-action",
    index: 2,
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
  },
  {
    center: "mine",
    component: "profile.upgrade",
    tab: "mine",
    selector: ".profile-v55-primary-cta",
    endpoints: ["/api/v1/member/profile"],
    expectedURL: "/pages/user/UserAgentUpgradePage",
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
    expectedRequests: ["/api/v1/member/profile"],
    expectedEventType: "showModal",
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
    const response = await fetch(`${url}/`, { signal: controller.signal });
    clearTimeout(timeout);
    return response.ok;
  } catch {
    return false;
  }
}

async function waitForStartedServer(child, fallbackURL) {
  const seen = [];
  let detectedURL = fallbackURL;
  const remember = chunk => {
    const text = String(chunk);
    seen.push(text);
    const plainText = text.replace(/\x1b\[[0-9;]*m/g, "");
    const match = plainText.match(/http:\/\/(?:localhost|127\.0\.0\.1):(\d+)\//);
    if (match) detectedURL = `http://127.0.0.1:${match[1]}`;
    const vitePortMatch = plainText.match(/:(\d+)\/\s*(?:\r?\n|$)/);
    if (vitePortMatch) detectedURL = `http://127.0.0.1:${vitePortMatch[1]}`;
  };
  child.stdout?.on("data", remember);
  child.stderr?.on("data", remember);

  const deadline = Date.now() + 120000;
  while (Date.now() < deadline) {
    if (detectedURL && await reachable(detectedURL)) return detectedURL;
    await new Promise(resolve => setTimeout(resolve, 1000));
  }
  throw new Error(`user-uni dev server did not become reachable:\n${seen.join("").slice(-4000)}`);
}

async function resolveDevServer() {
  if (process.env.USER_UNI_FORCE_DEV_SERVER !== "1") {
    for (const url of preferredDevURLs) {
      if (await reachable(url)) return { url, child: null };
    }
  }

  const npmCommand = process.platform === "win32" ? "npm.cmd" : "npm";
  const port = process.env.USER_UNI_DEV_PORT || "5187";
  const fallbackURL = `http://127.0.0.1:${port}`;
  const command = process.platform === "win32" ? (process.env.ComSpec || "cmd.exe") : npmCommand;
  const args = process.platform === "win32"
    ? ["/d", "/s", "/c", `${npmCommand} run dev -- --host 127.0.0.1 --port ${port}`]
    : ["run", "dev", "--", "--host", "127.0.0.1", "--port", port];
  const child = spawn(command, args, {
    cwd: userUniRoot,
    env: { ...process.env, BROWSER: "none", VITE_API_BASE_URL: apiBaseURL },
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
  await request("GET", "/api/v1/health", "");
  const { token: userToken, auth: userAuth } = await login(userEmail, userPassword);
  let agentToken = userToken;
  try {
    agentToken = (await login(agentEmail, agentPassword)).token;
  } catch {
    // Agent-only endpoints remain optional for the click path, but will still
    // be reported by the static verifier if the dedicated account is absent.
  }

  const { url: devURL, child: serverChild } = await resolveDevServer();
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
    await page.goto(devURL, { waitUntil: "domcontentloaded" });

    for (const row of clickRows.filter(row => (!centerFilter || row.center === centerFilter) && (!componentFilter || row.component === componentFilter))) {
      const index = row.index || 0;
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
          const vue = await import("/node_modules/@dcloudio/uni-h5-vue/dist/vue.runtime.esm.js");
          const pinia = await import("/@id/pinia");
          const uniH5 = await import("/node_modules/@dcloudio/uni-h5/dist/uni-h5.es.js");
          const activeUniRuntime = global.uni || uniRuntime;
          localStorage.setItem("token", JSON.stringify(token));
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
          const workbench = await import("/src/components/MiniProgramRoleWorkbench.vue");

          document.querySelector("#app")?.setAttribute("style", "display:none");
          const oldRoot = document.querySelector(root);
          oldRoot?.__xianzhiVerifyApp?.unmount?.();
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
            const assetsModule = await import("/src/stores/assets.ts");
            const assetStore = assetsModule.useAssetStore(piniaInstance);
            await assetStore.refreshAssets(4);
            await assetStore.fetchRecentTasks().catch(() => null);
            await new Promise(resolve => setTimeout(resolve, 1800));
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
        await target.click({ timeout: 10000 });
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
        if (row.expectedSelector) {
          const visibleCount = await page.locator(`${rootSelector} ${row.expectedSelector}`).count();
          if (!visibleCount) throw new Error(`expected selector ${row.expectedSelector} after click`);
        }
        if (row.expectedClass) {
          const hasClass = await locator.nth(index).evaluate((el, className) => el.classList.contains(className), row.expectedClass);
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
      } catch (error) {
        failures.push(`${row.center}/${row.component}: ${error instanceof Error ? error.message : String(error)}`);
        const summary = byCenter.get(row.center) || { ok: 0, failed: 0 };
        summary.failed += 1;
        byCenter.set(row.center, summary);
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
