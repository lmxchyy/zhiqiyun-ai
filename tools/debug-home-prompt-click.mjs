import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");

const wsEndpoint = process.env.WX_AUTOMATOR_WS || "ws://127.0.0.1:33709";
const cliPath =
  process.env.WX_CLI_PATH ||
  "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const projectPath =
  process.env.WX_PROJECT_PATH ||
  path.resolve("apps", "user-uni", "dist", "build", "mp-weixin");
const autoPort = Number(process.env.WX_AUTOMATOR_PORT || 9421);
const generatedClientPath = path.resolve(projectPath, "api", "client.js");
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
const promptText =
  process.env.PROMPT_TEXT ||
  "\u751f\u6210\u4e00\u5f20\u6c34\u679c\u5e97\u5f00\u4e1a\u4fc3\u9500\u6d77\u62a5";
const screenshotsDir = path.resolve("artifacts", "mini-program-debug");
const logFile = path.join(screenshotsDir, "debug-home-prompt-click.log");
const resultFile = path.join(screenshotsDir, "debug-home-prompt-click.result.json");
const captureScreenshots = String(process.env.CAPTURE_SCREENSHOTS || "").toLowerCase() === "true";
const creationRoutes = {
  image: "pages/user/UserImageCreationPage",
  video: "pages/user/UserVideoCreationPage",
  ppt: "pages/user/UserPptCreationPage",
  infographic: "pages/user/UserInfographicCreationPage",
  agent: "pages/user/UserAgentCreationPage",
};

function inferCreationMode(value) {
  if (/视频|短片|口播|分镜/.test(value)) return "video";
  if (/ppt|演示|汇报|路演|方案/i.test(value)) return "ppt";
  if (/智能体|agent|客服|销售助手|知识库/i.test(value)) return "agent";
  if (/信息图|流程图|数据图|可视化/.test(value)) return "infographic";
  return "image";
}

function reachedRoute(snapshot, route) {
  return Boolean(snapshot && (snapshot.currentPath === route || snapshot.stack?.includes(route)));
}

function log(message) {
  const line = `[${new Date().toISOString()}] [debug-home-prompt] ${message}`;
  try {
    fs.mkdirSync(screenshotsDir, { recursive: true });
    fs.appendFileSync(logFile, `${line}\n`, "utf8");
  } catch {
    // Keep debug logging best-effort.
  }
  console.error(line);
}

async function withTimeout(label, promise, timeoutMs = 15000) {
  let timer = null;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
      }),
    ]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

async function login() {
  log(`login ${apiBase} as ${email}`);
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 12000);
  let response;
  try {
    response = await fetch(`${apiBase}/api/v1/auth/login`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ email, password }),
      signal: controller.signal,
    });
  } finally {
    clearTimeout(timer);
  }
  const text = await response.text();
  let payload = null;
  try {
    payload = text ? JSON.parse(text) : null;
  } catch {
    payload = { raw: text };
  }
  if (!response.ok) {
    throw new Error(`login failed ${response.status}: ${text.slice(0, 500)}`);
  }
  log(`login ok: ${payload.user?.name || payload.user?.email || payload.user?.id || "unknown"}`);
  return payload;
}

async function setSession(miniProgram, auth) {
  log("write wx storage auth session");
  const stored = await withTimeout(
    "write wx storage auth session",
    miniProgram.evaluate(session => {
      wx.setStorageSync("token", session.accessToken || "");
      wx.setStorageSync("refreshToken", session.refreshToken || "");
      wx.setStorageSync("auth", session);
      wx.setStorageSync("xianzhiMiniProgramAuth", session);
      wx.removeStorageSync("v531-creation-prompt");
      return {
        token: wx.getStorageSync("token"),
        refreshToken: wx.getStorageSync("refreshToken"),
        hasAuth: Boolean(wx.getStorageSync("auth")),
        hasLegacyAuth: Boolean(wx.getStorageSync("xianzhiMiniProgramAuth")),
      };
    }, auth),
    15000,
  );
  if (!stored.token || !stored.hasAuth) {
    throw new Error(`failed to persist auth storage: ${JSON.stringify(stored)}`);
  }
  log("wx storage auth session ready");
}

async function stackSnapshot(miniProgram) {
  const currentPage = await withTimeout("currentPage", miniProgram.currentPage(), 10000);
  const pageStack = await withTimeout("pageStack", miniProgram.pageStack(), 10000);
  const storedPrompt = await withTimeout(
    "read v531-creation-prompt",
    miniProgram.evaluate(() => wx.getStorageSync("v531-creation-prompt")),
    10000,
  );
  return {
    currentPath: currentPage?.path || "",
    stack: pageStack.map(page => page.path),
    storedPrompt,
  };
}

async function screenshot(miniProgram, name) {
  fs.mkdirSync(screenshotsDir, { recursive: true });
  const file = path.join(screenshotsDir, name);
  await miniProgram.screenshot({ path: file });
  return file;
}

async function switchHome(miniProgram) {
  log("switchTab /pages/user/UserHomePage");
  await withTimeout(
    "switchTab home",
    miniProgram.evaluate(() => new Promise(resolve => {
      wx.switchTab({
        url: "/pages/user/UserHomePage",
        success: () => resolve({ ok: true }),
        fail: error => resolve({ ok: false, error: error?.errMsg || String(error) }),
      });
    })),
    15000,
  );
  const home = await withTimeout("current home page", miniProgram.currentPage(), 10000);
  await home?.waitFor(4500);
  const snapshot = await stackSnapshot(miniProgram);
  if (snapshot.currentPath !== "pages/user/UserHomePage") {
    const storage = await withTimeout(
      "read auth storage after switchTab",
      miniProgram.evaluate(() => ({
        token: wx.getStorageSync("token") || "",
        hasAuth: Boolean(wx.getStorageSync("auth")),
        hasLegacyAuth: Boolean(wx.getStorageSync("xianzhiMiniProgramAuth")),
      })),
      10000,
    );
    throw new Error(`home not opened: ${JSON.stringify({ snapshot, storage })}`);
  }
  return home;
}

async function findElement(page, selectors) {
  for (const selector of selectors) {
    const element = await page.$(selector);
    if (element) return { selector, element };
  }
  return { selector: "", element: null };
}

async function readElementValue(page, selectors) {
  const match = await findElement(page, selectors);
  if (!match.element) return { selector: "", value: "" };
  const value = await match.element.value().catch(() => "");
  return { selector: match.selector, value };
}

async function tryTap(miniProgram, selector, element) {
  const before = await stackSnapshot(miniProgram);
  const size = await element.size().catch(error => ({ error: error.message }));
  const offset = await element.offset().catch(error => ({ error: error.message }));
  await element.tap();
  const current = await miniProgram.currentPage();
  await current?.waitFor(1800);
  const after = await stackSnapshot(miniProgram);
  return { selector, size, offset, before, after };
}

async function tryTrigger(miniProgram, selector, element) {
  const before = await stackSnapshot(miniProgram);
  await element.trigger("tap");
  const current = await miniProgram.currentPage();
  await current?.waitFor(1800);
  const after = await stackSnapshot(miniProgram);
  return { selector, event: "trigger.tap", before, after };
}

async function tryComponentMethod(miniProgram, page, promptText) {
  const before = await stackSnapshot(miniProgram);
  const component = await page.$("v531-home-page");
  if (!component || typeof component.callMethod !== "function") {
    return { selector: "v531-home-page", event: "callMethod", error: "component not found", before, after: before };
  }
  const outerWxml = await component.outerWxml().catch(error => `outerWxml failed: ${error.message}`);
  await component.callMethod("nativeHomePromptInput", { detail: { value: promptText } });
  await component.callMethod("nativeHomePromptSubmit");
  const current = await miniProgram.currentPage();
  await current?.waitFor(1800);
  const after = await stackSnapshot(miniProgram);
  return { selector: "v531-home-page", event: "callMethod.nativeHomePromptSubmit", outerWxml, before, after };
}

async function main() {
  const auth = await login();
  let miniProgram = null;
  let connectionMode = "connect";
  try {
    log(`connect ${wsEndpoint}`);
    miniProgram = await withTimeout("connect", automator.connect({ wsEndpoint }), 12000);
    log("connect ok");
  } catch (error) {
    log(`connect failed: ${error.message}`);
    connectionMode = "launch";
    log(`launch devtools auto port ${autoPort}`);
    miniProgram = await withTimeout(
      "launch",
      automator.launch({
        cliPath,
        projectPath,
        port: autoPort,
        trustProject: true,
        timeout: 60000,
      }),
      75000,
    );
    log("launch ok");
  }
  try {
    log("open /pages/WechatLoginPage to initialize wx context");
    const loginPage = await withTimeout("reLaunch login", miniProgram.reLaunch("/pages/WechatLoginPage"), 45000);
    await loginPage?.waitFor(2500);
    await setSession(miniProgram, auth);
    log("open /pages/user/UserHomePage");
    const home = await switchHome(miniProgram);
    if (!home) throw new Error("failed to open /pages/user/UserHomePage");
    log("home loaded");

    const beforeScreenshot = captureScreenshots ? await screenshot(miniProgram, "home-before-prompt.png") : "";
    log("read initial page snapshot");
    const before = await stackSnapshot(miniProgram);
    const expectedMode = inferCreationMode(promptText);
    const expectedRoute = creationRoutes[expectedMode];

    log("find and fill hero input");
    const inputMatch = await findElement(home, [".hero-text-input", ".hero-input input", "input"]);
    if (!inputMatch.element) {
      throw new Error("hero input not found");
    }
    await inputMatch.element.input(promptText);
    await home.waitFor(500);
    const inputValue = await inputMatch.element.value().catch(() => "");
    log(`input value: ${inputValue || "(empty)"}`);

    const attempts = [];
    log("find submit element");
    const submitHit = await findElement(home, [
      ".hero-input-action-hit.submit",
      ".hero-input-action.submit",
      ".hero-input-actions",
    ]);
    if (!submitHit.element) {
      throw new Error("hero submit element not found");
    }
    log(`tap ${submitHit.selector}`);
    attempts.push(await tryTap(miniProgram, submitHit.selector, submitHit.element));

    let latest = attempts[attempts.length - 1]?.after;
    if (!reachedRoute(latest, expectedRoute)) {
      log("first tap did not complete navigation, try visible submit");
      const visibleButton = await findElement(home, [".hero-input-action.submit"]);
      if (visibleButton.element && visibleButton.selector !== submitHit.selector) {
        attempts.push(await tryTap(miniProgram, visibleButton.selector, visibleButton.element));
      }
    }
    latest = attempts[attempts.length - 1]?.after;
    if (!reachedRoute(latest, expectedRoute)) {
      log(`coordinate tap did not complete navigation, trigger tap on ${submitHit.selector}`);
      attempts.push(await tryTrigger(miniProgram, submitHit.selector, submitHit.element));
    }
    latest = attempts[attempts.length - 1]?.after;
    if (!reachedRoute(latest, expectedRoute)) {
      log("trigger tap did not complete navigation, call V531HomePage native method");
      attempts.push(await tryComponentMethod(miniProgram, home, promptText));
    }

    latest = await stackSnapshot(miniProgram);
    const targetPage = await withTimeout("current target page", miniProgram.currentPage(), 10000);
    const targetPrompt = await readElementValue(targetPage, [".v31-one-line-input", ".v31-ppt-input", "textarea", "input"]);
    const afterScreenshot = captureScreenshots ? await screenshot(miniProgram, "home-after-prompt-click.png") : "";
    const navigated = reachedRoute(latest, expectedRoute);
    const targetPromptFilled = targetPrompt.value === promptText;
    const promptStored = latest.storedPrompt === promptText;
    const result = {
      ok: navigated && targetPromptFilled,
      successReason:
        navigated && targetPromptFilled
          ? "navigatedToExpectedCreationPageWithPrompt"
          : navigated
            ? "navigatedWithoutPrompt"
            : promptStored
              ? "promptStoredOnly"
              : "",
      expectedMode,
      expectedRoute,
      connectionMode,
      wsEndpoint,
      autoPort,
      projectPath,
      apiBase,
      user: {
        id: auth.user?.id || "",
        name: auth.user?.name || "",
        currentRole: auth.currentRole || "",
      },
      inputSelector: inputMatch.selector,
      inputValue,
      before,
      attempts,
      after: latest,
      targetPrompt,
      screenshots: { before: beforeScreenshot, after: afterScreenshot },
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
