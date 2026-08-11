import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");
const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const projectPath = path.join(repoRoot, "apps", "user-uni", "dist", "build", "mp-weixin");
const cliPath = "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const automationPort = process.env.WECHAT_ROLE_AUTOMATION_PORT || "9425";
const automationEndpoint = `ws://127.0.0.1:${automationPort}`;
const idePort = process.env.WECHAT_IDE_PORT || "33709";

const allCases = [
  {
    name: "demo",
    email: process.env.XIANZHI_VERIFY_DEMO_EMAIL || "demo@xianzhi.ai",
    password: process.env.XIANZHI_VERIFY_DEMO_PASSWORD || "Demo123!",
    role: "USER",
    landingPage: "pages/user/UserHomePage",
  },
  {
    name: "agent",
    email: process.env.XIANZHI_VERIFY_AGENT_EMAIL || "agent1@xianzhi.ai",
    password: process.env.XIANZHI_VERIFY_AGENT_PASSWORD || "Agent123!",
    role: "AGENT",
    // Multi-role accounts default to consumer home; agent workbench is entered via role switch.
    landingPage: "pages/user/UserHomePage",
  },
  {
    name: "operation",
    email: process.env.XIANZHI_VERIFY_OPERATION_EMAIL || "operation@xianzhi.ai",
    password: process.env.XIANZHI_VERIFY_OPERATION_PASSWORD || "Demo123!",
    role: "OPERATION",
    landingPage: "pages/user/UserHomePage",
  },
];
const requestedCase = String(process.env.XIANZHI_VERIFY_LOGIN_CASE || "").trim().toLowerCase();
const cases = requestedCase ? allCases.filter(item => item.name === requestedCase) : allCases;

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function wait(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function connect() {
  const output = [];
  try {
    return await automator.connect({ wsEndpoint: automationEndpoint });
  } catch {
    // No active automation session yet; launch one below.
  }
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
  launch(idePort);
  let lastError;
  let activeIdePort = idePort;
  for (let attempt = 0; attempt < 120; attempt += 1) {
    try {
      return await automator.connect({ wsEndpoint: automationEndpoint });
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
  throw new Error(`微信自动化连接失败: ${lastError instanceof Error ? lastError.message : String(lastError)}\n${output.join("").slice(-1200)}`);
}

async function reconnect() {
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

async function currentPage(miniProgram) {
  const page = await miniProgram.currentPage();
  assert(page, "微信开发者工具没有返回当前页面");
  return page;
}

async function waitForRoute(miniProgram, expectedRoute, timeoutMs = 12000) {
  const startedAt = Date.now();
  let activeMiniProgram = miniProgram;
  let page;
  while (Date.now() - startedAt < timeoutMs) {
    try {
      page = await currentPage(activeMiniProgram);
    } catch (error) {
      if (!String(error instanceof Error ? error.message : error).includes("Connection closed")) throw error;
      activeMiniProgram = await reconnect();
      await wait(800);
      continue;
    }
    if (page.path === expectedRoute) return { miniProgram: activeMiniProgram, page };
    await wait(400);
  }
  throw new Error(`等待页面超时，期望 ${expectedRoute}，当前 ${page?.path || "unknown"}`);
}

async function verifyCase(miniProgram, testCase) {
  await miniProgram.reLaunch("/pages/WechatLoginPage");
  await wait(900);
  let page = await currentPage(miniProgram);
  assert(page.path === "pages/WechatLoginFormPage", `${testCase.name}: 冷启动登录门页未进入正式登录表单，当前为 ${page.path}`);

  const passwordEntryButton = await page.$(".auth-login-mode-password");
  assert(passwordEntryButton, `${testCase.name}: 未找到账号密码登录原生按钮`);
  await passwordEntryButton.tap();
  await page.waitFor(300);

  const accountInput = await page.$("#auth-account-input");
  const passwordInput = await page.$("#auth-password-input");
  assert(accountInput && passwordInput, `${testCase.name}: 点击账号密码登录后未显示输入框`);
  if (process.env.XIANZHI_VERIFY_LOGIN_SWITCH_ONLY === "true") {
    return {
      miniProgram,
      result: {
        account: testCase.email,
        accountPasswordSwitch: true,
        accountInputVisible: true,
        passwordInputVisible: true,
      },
    };
  }
  await accountInput.input(testCase.email);
  await passwordInput.input(testCase.password);

  const safeAreaComponent = await page.$("safe-area-container");
  const loginScrollView = safeAreaComponent ? await safeAreaComponent.$("scroll-view") : null;
  assert(loginScrollView && typeof loginScrollView.scrollTo === "function", `${testCase.name}: 未找到登录页滚动容器`);

  const agreementControl = await page.$(".auth-password-agreement-toggle");
  assert(agreementControl, `${testCase.name}: 未找到原生协议标签`);
  console.log(JSON.stringify({
    case: testCase.name,
    stage: "before-consent-login-tap",
    scrollTop: await loginScrollView.property("scrollTop"),
    agreementOffset: await agreementControl.offset(),
  }));
  await miniProgram.screenshot({ path: path.join(repoRoot, "artifacts", `wechat-login-${testCase.name}-agreement.png`) });
  if (process.env.XIANZHI_VERIFY_LOGIN_LAYOUT_ONLY === "true") {
    return {
      miniProgram,
      result: {
        account: testCase.email,
        accountPasswordSwitch: true,
        screenshot: `artifacts/wechat-login-${testCase.name}-agreement.png`,
      },
    };
  }
  const submit = await page.$(".auth-password-submit");
  assert(submit, `${testCase.name}: 未找到账号密码登录按钮`);
  console.log(JSON.stringify({
    case: testCase.name,
    stage: "before-login-tap",
    submitOffset: await submit.offset(),
    submitText: await submit.text(),
    submitClass: await submit.attribute("class"),
  }));
  await submit.tap();
  console.log(JSON.stringify({ case: testCase.name, stage: "login-tapped" }));
  await page.waitFor(1800);

  const pageAfterLogin = await currentPage(miniProgram);
  if (pageAfterLogin.path === "pages/WechatLoginPage") {
    const token = await miniProgram.callWxMethod("getStorageSync", "token");
    const storedAuth = await miniProgram.callWxMethod("getStorageSync", "auth");
    const toastComponent = await pageAfterLogin.$("toast");
    const toastState = toastComponent ? await toastComponent.data() : {};
    const fieldErrors = [];
    for (const element of await pageAfterLogin.$$(".auth-field-error")) fieldErrors.push(await element.text());
    const accountValue = await accountInput.value();
    const passwordValue = await passwordInput.value();
    console.log(JSON.stringify({
      case: testCase.name,
      stage: "login-diagnostics",
      accountValuePresent: Boolean(accountValue),
      passwordLength: String(passwordValue || "").length,
      tokenPresent: Boolean(token),
      authPresent: Boolean(storedAuth),
      fieldErrors,
      toastVisible: Boolean(toastState?.visible),
      toastMessage: String(toastState?.message || ""),
      toastTone: String(toastState?.tone || ""),
    }));
  }

  let routed = await waitForRoute(miniProgram, testCase.landingPage);
  miniProgram = routed.miniProgram;
  page = routed.page;
  const auth = await miniProgram.callWxMethod("getStorageSync", "auth");
  const expectedCurrentRole = Array.isArray(auth?.roles) && auth.roles.includes("USER") ? "USER" : testCase.role;
  assert(auth?.currentRole === expectedCurrentRole, `${testCase.name}: currentRole=${auth?.currentRole || "missing"}, want ${expectedCurrentRole}`);
  assert(Array.isArray(auth?.roles) && auth.roles.includes("USER") && auth.roles.includes(testCase.role), `${testCase.name}: roles=${JSON.stringify(auth?.roles)}`);

  if (testCase.role === "USER") {
    return {
      miniProgram,
      result: {
        account: testCase.email,
        defaultRole: expectedCurrentRole,
        landingPage: testCase.landingPage,
        roles: auth.roles,
      },
    };
  }

  await miniProgram.reLaunch(testCase.role === "AGENT" ? "/pages/agent/AgentOverviewPage" : "/pages/operation/OperationOverviewPage");
  routed = await waitForRoute(
    miniProgram,
    testCase.role === "AGENT" ? "pages/agent/AgentOverviewPage" : "pages/operation/OperationOverviewPage",
  );
  miniProgram = routed.miniProgram;
  page = routed.page;
  const workbench = await page.$("mini-program-role-workbench");
  assert(workbench, `${testCase.name}: 未找到角色工作台`);
  const roleButtons = await workbench.$$(".role-pill");
  assert(roleButtons.length >= 2, `${testCase.name}: 未显示角色切换器`);
  await roleButtons[0].tap();
  routed = await waitForRoute(miniProgram, "pages/user/UserMinePage");
  miniProgram = routed.miniProgram;
  const switchedAuth = await miniProgram.callWxMethod("getStorageSync", "auth");
  assert(switchedAuth?.currentRole === "USER", `${testCase.name}: 未切回 USER，currentRole=${switchedAuth?.currentRole || "missing"}`);

  return {
    miniProgram,
    result: {
      account: testCase.email,
      defaultRole: expectedCurrentRole,
      landingPage: testCase.landingPage,
      roles: auth.roles,
      switchedToUserPage: "pages/user/UserMinePage",
    },
  };
}

async function main() {
  let miniProgram = await connect();
  const results = [];
  try {
    if (process.env.XIANZHI_VERIFY_COLD_START === "true") {
      await miniProgram.reLaunch("/pages/index/index");
      await wait(1500);
      const legacyEntryPage = await currentPage(miniProgram);
      assert(
        legacyEntryPage.path === "pages/WechatLoginFormPage",
        `体验版旧入口未进入正式登录表单，当前为 ${legacyEntryPage.path}`,
      );
      console.log(JSON.stringify({ stage: "legacy-experience-entry", landingPage: legacyEntryPage.path }));
      await miniProgram.reLaunch("/pages/WechatLoginPage");
      await wait(1500);
      const coldStartPage = await currentPage(miniProgram);
      assert(
        coldStartPage.path === "pages/WechatLoginFormPage",
        `冷启动页未进入正式登录页，当前为 ${coldStartPage.path}`,
      );
      console.log(JSON.stringify({ stage: "cold-start", landingPage: coldStartPage.path }));
    }
    for (const testCase of cases) {
      const verified = await verifyCase(miniProgram, testCase);
      miniProgram = verified.miniProgram;
      results.push(verified.result);
    }
    console.log(JSON.stringify({ passed: results.length, results }, null, 2));
  } finally {
    try {
      await miniProgram.disconnect();
    } catch {
      // The IDE may close the automation socket while a role switch is relaunching the app.
    }
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
