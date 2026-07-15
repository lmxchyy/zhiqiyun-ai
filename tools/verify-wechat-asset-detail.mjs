import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");
const repoRoot = path.resolve(import.meta.dirname, "..");
const projectPath = path.join(repoRoot, "apps", "user-uni", "dist", "build", "mp-weixin");
const cliPath = process.env.WX_CLI_PATH || "C:\\Program Files (x86)\\Tencent\\微信web开发者工具\\cli.bat";
const apiBase = (process.env.API_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const email = process.env.LOGIN_EMAIL || "demo@xianzhi.ai";
const password = process.env.LOGIN_PASSWORD || "Demo123!";
const autoPort = Number(process.env.WX_ASSET_DETAIL_AUTOMATOR_PORT || 9435);
const idePort = String(process.env.WX_ASSET_DETAIL_IDE_PORT || 33709);
const verificationAction = process.env.WX_ASSET_DETAIL_ACTION || "screenshot";
const automationEndpoint = `ws://127.0.0.1:${autoPort}`;
const artifactDir = path.join(repoRoot, "artifacts", "wechat-asset-detail");
const resultPath = path.join(artifactDir, `result-${verificationAction}.json`);
const screenshotPath = path.join(artifactDir, "asset-detail-optimized.png");

fs.mkdirSync(artifactDir, { recursive: true });

function progress(message) {
  console.log(`${new Date().toISOString()} ${message}`);
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function wait(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function withTimeout(label, promise, timeoutMs = 20000) {
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

async function request(pathname, token = "", options = {}) {
  const response = await fetch(`${apiBase}${pathname}`, {
    ...options,
    headers: {
      ...(options.body ? { "content-type": "application/json" } : {}),
      ...(token ? { authorization: `Bearer ${token}` } : {}),
      ...(options.headers || {}),
    },
  });
  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("json") ? await response.json() : await response.arrayBuffer();
  if (!response.ok) throw new Error(`${pathname} returned ${response.status}: ${JSON.stringify(payload)}`);
  return { response, payload: payload?.data || payload };
}

async function authenticate() {
  const { payload } = await request("/api/v1/auth/login", "", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  assert(payload?.accessToken, "登录接口未返回 accessToken");
  return payload;
}

async function resolveAsset(token) {
  const preferredId = process.env.XIANZHI_ASSET_ID || "asset_000175";
  try {
    const { payload } = await request(`/api/v1/assets/${encodeURIComponent(preferredId)}`, token);
    const item = payload?.item || payload?.asset || payload;
    if (item?.id) return item;
  } catch {
    // Fall through to the current account's latest image.
  }
  const { payload } = await request("/api/v1/assets?paged=true&page=1&pageSize=20&limit=20&offset=0&sort=created_desc", token);
  const items = Array.isArray(payload?.items) ? payload.items : [];
  const candidate = items.find(item => String(item.mediaType || item.type || "").toLowerCase().includes("image")) || items[0];
  assert(candidate?.id, "当前测试账号没有可验证的作品");
  const { payload: detailPayload } = await request(`/api/v1/assets/${encodeURIComponent(candidate.id)}`, token);
  return detailPayload?.item || detailPayload?.asset || detailPayload;
}

async function connectMiniProgram() {
  try {
    return await automator.connect({ wsEndpoint: automationEndpoint });
  } catch {
    // Start a new automation session only when no healthy endpoint is available.
  }
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

async function waitElement(root, selector, timeoutMs = 12000) {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    const element = await root.$(selector);
    if (element) return element;
    await wait(250);
  }
  throw new Error(`找不到控件 ${selector}`);
}

async function openDetail(miniProgram, assetId) {
  const page = await withTimeout(
    "open asset detail",
    miniProgram.reLaunch(`/pages/user/UserAssetDetailPage?id=${encodeURIComponent(assetId)}`),
    30000,
  );
  await page.waitFor(1800);
  assert(page.path === "pages/user/UserAssetDetailPage", `作品详情路径错误: ${page.path}`);
  const detail = await waitElement(page, "asset-detail-center-page");
  await waitElement(detail, ".asset-title");
  return { page, detail };
}

async function returnToDetail(miniProgram, assetId) {
  await withTimeout("navigate back to asset detail", miniProgram.navigateBack(), 15000);
  await wait(800);
  const page = await miniProgram.currentPage();
  if (page.path !== "pages/user/UserAssetDetailPage") return openDetail(miniProgram, assetId);
  const detail = await waitElement(page, "asset-detail-center-page");
  await waitElement(detail, ".asset-title");
  return { page, detail };
}

async function actionTexts(actionSheet) {
  const buttons = await actionSheet.$$(".action-item");
  return Promise.all(buttons.map(button => button.text()));
}

async function verifyDetail(miniProgram, asset, result) {
  progress("detail: open initial");
  let opened = await openDetail(miniProgram, asset.id);
  const preview = await waitElement(opened.detail, ".cover-preview");
  const image = await waitElement(opened.detail, ".preview-image");
  const previewSize = await preview.size();
  const imageMode = await image.attribute("mode").catch(() => image.property("mode"));
  assert(Math.abs(previewSize.width - previewSize.height) <= 3, `作品预览不是 1:1: ${JSON.stringify(previewSize)}`);
  assert(imageMode === "aspectFit", `作品图片未使用 aspectFit: ${String(imageMode)}`);

  const title = await (await waitElement(opened.detail, ".asset-title")).text();
  const subtitle = await (await waitElement(opened.detail, ".asset-subtitle")).text();
  assert(title && !/^TEXT_TO_|^task[_-]/i.test(title), `作品标题仍为技术任务名: ${title}`);
  assert(/AI 图片|信息图|视频|PPT|文档/.test(subtitle), `作品次级信息缺失: ${subtitle}`);

  const primary = await waitElement(opened.detail, ".primary-button");
  const regenerate = await waitElement(opened.detail, ".regenerate-button");
  const utility = await opened.detail.$$(".utility-button");
  assert((await primary.text()).includes("继续编辑"), "缺少继续编辑主按钮");
  assert((await regenerate.text()).includes("再次生成"), "缺少再次生成次按钮");
  assert(utility.length === 3, `轻量操作数量错误: ${utility.length}`);
  const utilityTexts = await Promise.all(utility.map(button => button.text()));
  assert(utilityTexts.some(text => text.includes("保存到相册")), "缺少保存到相册按钮");
  assert(utilityTexts.some(text => text.includes("分享")), "缺少分享按钮");
  assert(utilityTexts.some(text => text.includes("更多")), "缺少更多按钮");

  const prompt = String(asset.prompt || asset.metadata?.prompt || "");
  let parameterRows = [];
  let allRows = [];
  let managementActions = [];
  if (verificationAction !== "screenshot") {
    const copyButton = await waitElement(opened.detail, ".copy-button");
    progress("detail: copy prompt");
    await copyButton.tap();
    await wait(1200);
    const clipboard = await miniProgram.callWxMethod("getClipboardData").catch(() => null);
    const clipboardValue = typeof clipboard === "string" ? clipboard : clipboard?.data;
    if (prompt && clipboardValue) {
      assert(String(clipboardValue).trim() === prompt.trim(), `复制后的提示词与接口数据不一致: expected=${prompt} actual=${clipboardValue}`);
    }

    const collapseTriggers = await opened.detail.$$(".collapse-trigger");
    progress("detail: expand metadata");
    assert(collapseTriggers.length === 2, `生成参数/资产信息折叠入口数量错误: ${collapseTriggers.length}`);
    await collapseTriggers[0].tap();
    await wait(250);
    parameterRows = await opened.detail.$$(".detail-row");
    assert(parameterRows.length >= 3, `生成参数未展开或数据过少: ${parameterRows.length}`);
    await collapseTriggers[1].tap();
    await wait(250);
    allRows = await opened.detail.$$(".detail-row");
    assert(allRows.length > parameterRows.length, "资产信息未展开");

    await utility[2].tap();
    progress("detail: open management sheet");
    const actionSheet = await waitElement(opened.detail, "asset-action-sheet");
    managementActions = await actionTexts(actionSheet);
    for (const expected of ["收藏", "移动项目", "重命名", "归档", "删除"]) {
      assert(managementActions.some(text => text.includes(expected)), `更多菜单缺少 ${expected}: ${managementActions.join(",")}`);
    }
    await (await waitElement(actionSheet, ".action-cancel")).tap();
    await wait(200);
  }

  progress("detail: verify authenticated download");
  const download = await request(`/api/v1/assets/${encodeURIComponent(asset.id)}/download`, result.token);
  assert(download.payload instanceof ArrayBuffer && download.payload.byteLength > 0, "作品下载接口没有返回文件内容");

  let actionDraft = null;
  let destinationPath = opened.page.path;
  if (verificationAction === "edit") {
    progress("detail: continue edit");
    await (await waitElement(opened.detail, ".primary-button")).tap();
    await wait(1200);
    const creationPage = await miniProgram.currentPage();
    destinationPath = creationPage.path;
    assert(destinationPath === "pages/user/UserImageCreationPage", `继续编辑未进入图片创作页: ${destinationPath}`);
    actionDraft = await miniProgram.callWxMethod("getStorageSync", "v532-studio-draft");
    assert(actionDraft?.intent === "edit" && actionDraft?.sourceAssetId === asset.id, `继续编辑草稿错误: ${JSON.stringify(actionDraft)}`);
    assert(String(actionDraft?.prompt || "") === prompt, "继续编辑未恢复原提示词");
    const editWorkbench = await waitElement(creationPage, "mini-program-role-workbench");
    const editPrompt = await waitElement(editWorkbench, ".v31-one-line-input");
    assert(String(await editPrompt.value()) === prompt, "创作页输入框未恢复原提示词");
  } else if (verificationAction === "regenerate") {
    progress("detail: regenerate");
    await (await waitElement(opened.detail, ".regenerate-button")).tap();
    await wait(1200);
    const creationPage = await miniProgram.currentPage();
    destinationPath = creationPage.path;
    assert(destinationPath === "pages/user/UserImageCreationPage", `再次生成未进入图片创作页: ${destinationPath}`);
    actionDraft = await miniProgram.callWxMethod("getStorageSync", "v532-studio-draft");
    assert(actionDraft?.intent === "regenerate" && actionDraft?.sourceAssetId === asset.id, `再次生成草稿错误: ${JSON.stringify(actionDraft)}`);
    assert(String(actionDraft?.model || "") === String(asset.model || asset.metadata?.model || ""), "再次生成未恢复原模型");
  }

  result.checks = {
    path: opened.page.path,
    destinationPath,
    assetId: asset.id,
    title,
    subtitle,
    previewSize,
    imageMode,
    utilityTexts,
    managementActions,
    parameterRows: parameterRows.length,
    totalExpandedRows: allRows.length,
    actionDraft: actionDraft ? {
      intent: actionDraft.intent,
      prompt: actionDraft.prompt,
      model: actionDraft.model,
      referenceCount: Array.isArray(actionDraft.referencePaths) ? actionDraft.referencePaths.length : 0,
    } : null,
    downloadBytes: download.payload.byteLength,
  };
}

async function main() {
  progress("api: login");
  const auth = await authenticate();
  progress("api: resolve asset");
  const asset = await resolveAsset(auth.accessToken);
  const result = {
    startedAt: new Date().toISOString(),
    projectPath,
    action: verificationAction,
    token: auth.accessToken,
    consoleErrors: [],
    consoleWarnings: [],
  };
  let miniProgram;
  try {
    progress("wechat: connect");
    miniProgram = await withTimeout("connect WeChat DevTools", connectMiniProgram(), 75000);
    miniProgram.on("console", message => {
      const line = `${message.type}: ${message.args.join(" ")}`;
      if (message.type === "error") result.consoleErrors.push(line);
      if (message.type === "warning") result.consoleWarnings.push(line);
    });
    miniProgram.on("exception", error => result.consoleErrors.push(`exception: ${error.message || String(error)}`));
    await miniProgram.callWxMethod("setStorageSync", "token", auth.accessToken);
    await miniProgram.callWxMethod("setStorageSync", "xianzhiMiniProgramAuth", auth);
    await miniProgram.callWxMethod("removeStorageSync", "zhiqiyun:asset-detail:preview-tip-v1");
    progress("wechat: verify detail");
    await verifyDetail(miniProgram, asset, result);
    result.token = "[redacted]";
    result.finishedAt = new Date().toISOString();
    result.ok = result.consoleErrors.length === 0;
    fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`, "utf8");
    assert(result.ok, `微信运行时存在控制台错误: ${result.consoleErrors.join(" | ")}`);
    if (verificationAction === "screenshot") {
      progress("wechat: capture screenshot");
      await withTimeout("capture asset detail screenshot", miniProgram.screenshot({ path: screenshotPath }), 15000);
    }
    console.log(JSON.stringify({
      ...result,
      screenshot: verificationAction === "screenshot" ? path.relative(repoRoot, screenshotPath).replaceAll("\\", "/") : "",
    }, null, 2));
  } finally {
    try {
      miniProgram?.disconnect();
    } catch {
      // The screenshot API may close the automation socket after writing the image.
    }
  }
}

main().catch(error => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exit(1);
});
