import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");

const wsEndpoint = process.env.WX_AUTOMATOR_WS || "ws://127.0.0.1:9441";
const generatedClientPath = path.resolve("apps", "user-uni", "dist", "build", "mp-weixin", "api", "client.js");
function detectApiBase() {
  if (process.env.API_BASE_URL) return process.env.API_BASE_URL;
  try {
    const source = fs.readFileSync(generatedClientPath, "utf8");
    const envMatch = /VITE_API_BASE_URL:"([^"]+)"/.exec(source);
    if (envMatch?.[1]) return envMatch[1];
    const urlMatch = /(https?:\/\/[^"',`]+)/.exec(source);
    if (urlMatch?.[1]) return urlMatch[1];
  } catch {
    // Fall back to local dev when the generated mini-program has not been built yet.
  }
  return "http://127.0.0.1:3100";
}
const apiBase = detectApiBase().replace(/\/+$/, "");
const email = process.env.LOGIN_EMAIL || "demo@xianzhi.ai";
const password = process.env.LOGIN_PASSWORD || "Demo123!";
const outputDir = path.resolve("artifacts", "mini-program-debug");
const resultFile = path.join(outputDir, "debug-home-card-clicks.result.json");
const componentFilter = new Set(
  String(process.env.CASE_COMPONENTS || "")
    .split(",")
    .map(item => item.trim())
    .filter(Boolean),
);

const cases = [
  { component: "home.ai-design.start", selector: ".capability-feature-card", index: 0, expected: "pages/user/UserImageCreationPage" },
  { component: "home.ai-video.start", selector: ".capability-feature-card", index: 1, expected: "pages/user/UserVideoCreationPage" },
  { component: "home.ppt-solution", selector: ".capability-secondary-card", index: 0, expected: "pages/user/UserPptCreationPage" },
  { component: "home.ai-office", selector: ".capability-secondary-card", index: 1, expected: "pages/user/UserInfographicCreationPage" },
  { component: "home.knowledge", selector: ".capability-compact-card", index: 0, expected: "pages/user/UserAgentCreationPage" },
  { component: "home.ai-employee-compact", selector: ".capability-compact-card", index: 1, expected: "pages/user/UserAgentCreationPage" },
  { component: "home.workflow", selector: ".capability-compact-card", index: 2, expected: "pages/user/UserAgentCreationPage" },
  { component: "home.more-capabilities", selector: ".capability-compact-card", index: 3, expected: "pages/user/UserAgentCreationPage" },
  { component: "home.continue-work", scrollTop: 900 },
  { component: "home.employee-section", selector: ".employee-section-action", index: 0, scrollTop: 1180, expected: "pages/user/UserAgentCreationPage" },
  { component: "home.employee-card", selector: ".employee-card", index: 0, scrollTop: 1180, expected: "pages/user/UserAgentCreationPage" },
];

function request(method, requestPath, body) {
  const url = `${apiBase}${requestPath}`;
  return fetch(url, {
    method,
    headers: { "content-type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  }).then(async response => {
    const text = await response.text();
    const data = text ? JSON.parse(text) : {};
    if (!response.ok) throw new Error(`${method} ${requestPath} ${response.status}: ${text}`);
    return data;
  });
}

async function setSession(miniProgram, auth) {
  const stored = await miniProgram.evaluate(session => {
    wx.setStorageSync("token", session.accessToken || "");
    wx.setStorageSync("refreshToken", session.refreshToken || "");
    wx.setStorageSync("auth", session);
    wx.setStorageSync("xianzhiMiniProgramAuth", session);
    wx.removeStorageSync("v531-creation-prompt");
    return {
      token: wx.getStorageSync("token") || "",
      refreshToken: wx.getStorageSync("refreshToken") || "",
      hasAuth: Boolean(wx.getStorageSync("auth")),
      hasLegacyAuth: Boolean(wx.getStorageSync("xianzhiMiniProgramAuth")),
    };
  }, auth);
  if (!stored.token || !stored.hasAuth) {
    throw new Error(`failed to persist auth storage: ${JSON.stringify(stored)}`);
  }
}

async function installRequestProbe(miniProgram) {
  await miniProgram.evaluate(() => {
    if (wx.__xianzhiRequestProbeInstalled) {
      wx.setStorageSync("__xianzhi_request_probe", []);
      return;
    }
    wx.__xianzhiRequestProbeInstalled = true;
    const originalRequest = wx.request;
    wx.setStorageSync("__xianzhi_request_probe", []);
    wx.request = function patchedRequest(options) {
      const requestOptions = options || {};
      const startedAt = Date.now();
      const headers = requestOptions.header || {};
      const record = {
        url: requestOptions.url || "",
        method: requestOptions.method || "GET",
        hasAuthorization: Boolean(headers.Authorization || headers.authorization),
        startedAt,
        statusCode: 0,
        errMsg: "",
      };
      const pushRecord = next => {
        const rows = wx.getStorageSync("__xianzhi_request_probe") || [];
        rows.push(next);
        wx.setStorageSync("__xianzhi_request_probe", rows.slice(-80));
      };
      return originalRequest.call(wx, {
        ...requestOptions,
        success(response) {
          record.statusCode = Number(response?.statusCode || 0);
          requestOptions.success?.(response);
        },
        fail(error) {
          record.errMsg = error?.errMsg || String(error || "");
          requestOptions.fail?.(error);
        },
        complete(result) {
          pushRecord({ ...record, completedAt: Date.now(), durationMs: Date.now() - startedAt });
          requestOptions.complete?.(result);
        },
      });
    };
  });
}

async function stackSnapshot(miniProgram) {
  const currentPage = await miniProgram.currentPage();
  const pageStack = await miniProgram.pageStack();
  return {
    currentPath: currentPage?.path || "",
    stack: pageStack.map(page => page.path),
  };
}

async function backToStackHome(miniProgram) {
  const before = await stackSnapshot(miniProgram);
  const homeIndex = before.stack.lastIndexOf("pages/user/UserHomePage");
  const delta = homeIndex >= 0 ? before.stack.length - homeIndex - 1 : 0;
  if (before.currentPath === "pages/user/UserHomePage" || delta <= 0) return { ok: false, skipped: true, before };
  const backResult = await miniProgram.evaluate(backDelta => new Promise(resolve => {
    let settled = false;
    const finish = result => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(result);
    };
    const timer = setTimeout(() => finish({ ok: false, error: "client navigateBack timeout" }), 2500);
    wx.navigateBack({
      delta: backDelta,
      success: () => finish({ ok: true }),
      fail: error => finish({ ok: false, error: error?.errMsg || String(error) }),
    });
  }), delta);
  const page = await miniProgram.currentPage();
  await page?.waitFor(1200);
  return { ...(backResult || {}), before, after: await stackSnapshot(miniProgram) };
}

async function switchHome(miniProgram) {
  const backResult = await backToStackHome(miniProgram);
  if (backResult.after?.currentPath === "pages/user/UserHomePage") {
    return miniProgram.currentPage();
  }
  const switchResult = await miniProgram.evaluate(() => new Promise(resolve => {
    let settled = false;
    const finish = result => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(result);
    };
    const timer = setTimeout(() => finish({ ok: false, error: "client switchTab timeout" }), 3000);
    wx.switchTab({
      url: "/pages/user/UserHomePage",
      success: () => finish({ ok: true }),
      fail: error => finish({ ok: false, error: error?.errMsg || String(error) }),
    });
  }));
  let page = await miniProgram.currentPage();
  await page?.waitFor(4200);
  let snapshot = await stackSnapshot(miniProgram);
  let fallbackResult = null;
  if (snapshot.currentPath !== "pages/user/UserHomePage") {
    try {
      page = await miniProgram.reLaunch("/pages/user/UserHomePage");
      await page?.waitFor(4200);
      snapshot = await stackSnapshot(miniProgram);
      fallbackResult = { ok: true };
    } catch (error) {
      fallbackResult = { ok: false, error: error?.message || String(error) };
    }
  }
  if (snapshot.currentPath !== "pages/user/UserHomePage") {
    const storage = await miniProgram.evaluate(() => ({
      token: wx.getStorageSync("token") || "",
      hasAuth: Boolean(wx.getStorageSync("auth")),
      hasLegacyAuth: Boolean(wx.getStorageSync("xianzhiMiniProgramAuth")),
      requests: wx.getStorageSync("__xianzhi_request_probe") || [],
    }));
    throw new Error(`home not opened: ${JSON.stringify({ backResult, switchResult, fallbackResult, snapshot, storage })}`);
  }
  return page;
}

async function findIndexed(page, selector, index = 0) {
  const elements = typeof page.$$ === "function" ? await page.$$(selector) : [];
  if (elements[index]) return elements[index];
  const single = await page.$(selector);
  if (single && index === 0) return single;
  throw new Error(`element not found: ${selector}[${index}]`);
}

function reached(snapshot, testCase) {
  if (testCase.expectedPrefix) {
    return snapshot.currentPath.startsWith(testCase.expectedPrefix) || snapshot.stack.some(item => item.startsWith(testCase.expectedPrefix));
  }
  return snapshot.currentPath === testCase.expected || snapshot.stack.includes(testCase.expected);
}

async function runCase(miniProgram, testCase) {
  const home = await switchHome(miniProgram);
  const root = await home.$("v531-home-page");
  if (!root) {
    const stateCard = await home.$(".state-card");
    const stateWxml = stateCard ? await stateCard.outerWxml().catch(error => error.message) : "";
    throw new Error(`home root not rendered before ${testCase.component}: ${stateWxml}`);
  }
  if (typeof testCase.scrollTop === "number") {
    await miniProgram.evaluate(scrollTop => {
      wx.pageScrollTo({ scrollTop, duration: 0 });
    }, testCase.scrollTop);
    await home.waitFor(700);
  }
  let activeCase = testCase;
  let element;
  if (testCase.component === "home.continue-work") {
    const emptyStart = await home.$(".project-empty button");
    if (emptyStart) {
      activeCase = { ...testCase, selector: ".project-empty button", index: 0, expected: "pages/user/UserCreationPage" };
      element = emptyStart;
    } else {
      const projectCard = await home.$(".project-card");
      if (projectCard) {
        activeCase = { ...testCase, selector: ".project-card", index: 0, expectedPrefix: "pages/user/UserAssetDetailPage" };
        element = projectCard;
      } else {
        activeCase = { ...testCase, selector: ".project-section-action", index: 0, expected: "pages/user/UserAssetsPage" };
        element = await findIndexed(home, ".project-section-action", 0);
      }
    }
  } else {
    element = await findIndexed(home, testCase.selector, testCase.index);
  }
  const before = await stackSnapshot(miniProgram);
  await element.tap();
  const current = await miniProgram.currentPage();
  await current?.waitFor(1800);
  const after = await stackSnapshot(miniProgram);
  return {
    component: activeCase.component,
    selector: activeCase.selector,
    index: activeCase.index,
    expected: activeCase.expected || activeCase.expectedPrefix,
    before,
    after,
    ok: reached(after, activeCase),
  };
}

async function main() {
  fs.mkdirSync(outputDir, { recursive: true });
  const auth = await request("POST", "/api/v1/auth/login", { email, password });
  const miniProgram = await automator.connect({ wsEndpoint });
  try {
    await miniProgram.reLaunch("/pages/WechatLoginPage");
    await setSession(miniProgram, auth);
    await installRequestProbe(miniProgram);
    const rows = [];
    const runCases = componentFilter.size ? cases.filter(item => componentFilter.has(item.component)) : cases;
    for (const testCase of runCases) {
      console.error(`[debug-home-card] run ${testCase.component}`);
      const row = await runCase(miniProgram, testCase);
      rows.push(row);
      fs.writeFileSync(resultFile, JSON.stringify({ ok: rows.every(item => item.ok), partial: true, rows }, null, 2), "utf8");
      console.error(`[debug-home-card] ${row.ok ? "ok" : "fail"} ${testCase.component} -> ${row.after.currentPath}`);
    }
    const result = {
      ok: rows.every(row => row.ok),
      partial: false,
      rows,
    };
    fs.writeFileSync(resultFile, JSON.stringify(result, null, 2), "utf8");
    console.log(JSON.stringify(result, null, 2));
    if (!result.ok) process.exitCode = 2;
  } finally {
    miniProgram.disconnect();
  }
}

main().catch(error => {
  console.error(error);
  process.exit(1);
});
