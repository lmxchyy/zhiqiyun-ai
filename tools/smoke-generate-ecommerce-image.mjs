import fs from "node:fs";
import http from "node:http";
import https from "node:https";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const apiBaseURL = (process.env.XIANZHI_API_BASE_URL || "http://127.0.0.1:3100").replace(/\/+$/, "");
const email = process.env.XIANZHI_VERIFY_USER_EMAIL || "demo@xianzhi.ai";
const password = process.env.XIANZHI_VERIFY_USER_PASSWORD || "Demo123!";
const requestedModel = process.env.XIANZHI_IMAGE_MODEL || "gpt-image-2";
const existingTaskId = String(process.env.XIANZHI_TASK_ID || "").trim();
const skipDownload = ["1", "true", "yes"].includes(String(process.env.XIANZHI_SKIP_DOWNLOAD || "").toLowerCase());
const prompt = process.env.XIANZHI_IMAGE_PROMPT || [
  "生成一张 iPhone 17 电商平台商品主图，1:1 方图，纯白到浅灰渐变背景，",
  "手机正面与背面组合陈列，金属边框和玻璃质感清晰，柔和棚拍光，",
  "画面上方保留品牌标题空间，右侧用简洁中文展示三条卖点：旗舰影像、全天续航、轻薄机身，",
  "蓝色与橙色作为少量视觉点缀，构图干净高级，适合电商首页，不要水印，不要无关文字。",
].join("");
const pollIntervalMs = Number(process.env.XIANZHI_POLL_INTERVAL_MS || 5000);
const pollTimeoutMs = Number(process.env.XIANZHI_POLL_TIMEOUT_MS || 600000);

function request(method, requestPath, token, body, responseType = "json", redirectsRemaining = 5) {
  const url = new URL(requestPath, apiBaseURL);
  const transport = url.protocol === "https:" ? https : http;
  const payload = body === undefined ? undefined : JSON.stringify(body);
  return new Promise((resolve, reject) => {
    const req = transport.request(url, {
      method,
      headers: {
        Accept: responseType === "json" ? "application/json" : "*/*",
        ...(payload ? { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(payload) } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      timeout: 30000,
    }, res => {
      if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        if (redirectsRemaining <= 0) {
          reject(new Error(`${method} ${requestPath} exceeded redirect limit`));
          return;
        }
        const redirectURL = new URL(res.headers.location, url).toString();
        resolve(request(method, redirectURL, token, body, responseType, redirectsRemaining - 1));
        return;
      }
      const chunks = [];
      res.on("data", chunk => chunks.push(chunk));
      res.on("end", () => {
        const buffer = Buffer.concat(chunks);
        let data = buffer;
        if (responseType === "json") {
          const text = buffer.toString("utf8");
          try {
            data = text ? JSON.parse(text) : {};
          } catch {
            data = text;
          }
        }
        if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300) {
          resolve({ status: res.statusCode, headers: res.headers, data });
          return;
        }
        const message = typeof data === "object" && data && "error" in data ? data.error : String(data || "empty response");
        reject(new Error(`${method} ${requestPath} returned ${res.statusCode}: ${message}`));
      });
    });
    req.on("timeout", () => req.destroy(new Error(`${method} ${requestPath} timed out`)));
    req.on("error", reject);
    if (payload) req.write(payload);
    req.end();
  });
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function firstString(value, keys) {
  if (!value || typeof value !== "object") return "";
  for (const key of keys) {
    const candidate = value[key];
    if (typeof candidate === "string" && candidate.trim()) return candidate.trim();
  }
  return "";
}

function listOf(value) {
  if (Array.isArray(value)) return value;
  if (!value || typeof value !== "object") return [];
  for (const key of ["items", "rows", "data", "assets"]) {
    if (Array.isArray(value[key])) return value[key];
  }
  return [];
}

function findImageURL(value, seen = new Set()) {
  if (!value || typeof value !== "object" || seen.has(value)) return "";
  seen.add(value);
  for (const key of ["url", "imageUrl", "imageURL", "outputUrl", "thumbnailUrl"]) {
    const candidate = value[key];
    if (typeof candidate === "string" && candidate.trim()) return candidate.trim();
  }
  for (const key of ["images", "outputs", "results", "assets", "item", "data", "metadata"]) {
    const nested = value[key];
    if (Array.isArray(nested)) {
      for (const item of nested) {
        const found = findImageURL(item, seen);
        if (found) return found;
      }
    } else {
      const found = findImageURL(nested, seen);
      if (found) return found;
    }
  }
  return "";
}

function absoluteURL(value) {
  return /^https?:\/\//i.test(value) ? value : new URL(value, `${apiBaseURL}/`).toString();
}

function pointSnapshot(payload, taskId) {
  if (!payload || typeof payload !== "object") return null;
  const account = payload.account && typeof payload.account === "object" ? payload.account : payload;
  const transactions = Array.isArray(payload.transactions) ? payload.transactions : [];
  const transaction = transactions.find(item => firstString(item, ["taskId"]) === taskId) || null;
  return {
    available: Number(account.available || 0),
    frozen: Number(account.frozen || 0),
    total: Number(account.total || 0),
    transaction: transaction ? {
      id: firstString(transaction, ["transactionId", "id"]),
      pointCost: Number(transaction.pointCost || 0),
      status: firstString(transaction, ["status"]),
    } : null,
  };
}

async function optionalGet(requestPath, token) {
  try {
    return (await request("GET", requestPath, token)).data;
  } catch {
    return null;
  }
}

async function main() {
  console.log(`[1/7] 登录测试账号：${email}`);
  const login = await request("POST", "/api/v1/auth/login", "", { email, password });
  const token = login.data?.accessToken || login.data?.token;
  if (!token) throw new Error("登录成功，但响应中没有 accessToken");

  const pointsBefore = await optionalGet("/api/v1/points/account", token);
  console.log(`[2/7] 查询生图模型：${requestedModel}`);
  const schema = await request(
    "GET",
    `/api/v1/module-schema?module_code=image_generation&model_name=${encodeURIComponent(requestedModel)}`,
    token,
  );
  const model = firstString(schema.data, ["model_name", "modelName", "model"]) || requestedModel;

  let task;
  let taskId = existingTaskId;
  if (taskId) {
    console.log(`[3/7] 继续追踪已有任务：${taskId}`);
    task = (await request("GET", `/api/v1/generation-tasks/${encodeURIComponent(taskId)}`, token)).data;
  } else {
    console.log(`[3/7] 提交真实生成任务：${model}`);
    const created = await request("POST", "/api/v1/generation-tasks", token, {
      type: "TEXT_TO_IMAGE",
      moduleCode: "image_generation",
      prompt,
      model,
      params: {
        size: "1024x1024",
        quality: "standard",
        n: 1,
      },
    });
    task = created.data;
    taskId = firstString(task, ["id", "taskId"]);
    if (!taskId) throw new Error(`创建任务成功，但响应中没有任务 ID：${JSON.stringify(task)}`);
  }
  console.log(`[4/7] 开始轮询任务：${taskId}`);

  const startedAt = Date.now();
  while (Date.now() - startedAt < pollTimeoutMs) {
    const status = firstString(task, ["status"]).toUpperCase() || "PENDING";
    console.log(`      ${status} (${Math.round((Date.now() - startedAt) / 1000)}s)`);
    if (["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(status)) break;
    if (["FAILED", "ERROR", "CANCELED", "CANCELLED"].includes(status)) {
      throw new Error(`任务 ${taskId} 生成失败：${firstString(task, ["error", "errorMessage", "message"]) || status}`);
    }
    await sleep(pollIntervalMs);
    task = (await request("GET", `/api/v1/generation-tasks/${encodeURIComponent(taskId)}`, token)).data;
  }
  const finalStatus = firstString(task, ["status"]).toUpperCase();
  if (!["SUCCEEDED", "SUCCESS", "COMPLETED"].includes(finalStatus)) {
    throw new Error(`任务 ${taskId} 在 ${Math.round(pollTimeoutMs / 1000)} 秒内未完成，当前状态：${finalStatus || "UNKNOWN"}`);
  }

  console.log("[5/7] 核对作品资产");
  const assetsPayload = await request("GET", "/api/v1/assets?paged=true&limit=50&offset=0", token);
  const assets = listOf(assetsPayload.data);
  let asset = assets.find(item => {
    const taskValue = firstString(item, ["taskId", "generationTaskId"])
      || firstString(item?.metadata, ["taskId", "generationTaskId"]);
    return taskValue === taskId;
  }) || assets.find(item => firstString(item?.metadata, ["prompt"]) === prompt) || null;
  if (!asset && Array.isArray(task.resultIds) && task.resultIds.length) {
    const detail = await optionalGet(`/api/v1/assets/${encodeURIComponent(String(task.resultIds[0]))}`, token);
    asset = detail?.item && typeof detail.item === "object" ? detail.item : detail;
  }
  const imageURL = findImageURL(task) || findImageURL(asset);
  if (!imageURL) throw new Error(`任务 ${taskId} 已成功，但没有找到可下载的图片 URL`);
  const assetId = firstString(asset, ["id", "assetId"]) || null;
  console.log(`      资产：${assetId || "已生成，未匹配到 ID"}`);
  console.log(`      图片：${absoluteURL(imageURL)}`);

  let outputPath = null;
  if (skipDownload) {
    console.log("[6/7] 已按参数跳过 CDN 下载");
  } else {
    console.log("[6/7] 下载生成图片");
    const imageResponse = await request("GET", absoluteURL(imageURL), "", undefined, "buffer");
    const outputDir = path.join(repoRoot, "artifacts", "creation-smoke");
    fs.mkdirSync(outputDir, { recursive: true });
    const contentType = String(imageResponse.headers["content-type"] || "");
    const extension = contentType.includes("jpeg") ? ".jpg" : contentType.includes("webp") ? ".webp" : ".png";
    outputPath = path.join(outputDir, `iphone17-ecommerce-${taskId}${extension}`);
    fs.writeFileSync(outputPath, imageResponse.data);
  }

  const pointsAfter = await optionalGet("/api/v1/points/account", token);
  console.log("[7/7] 全链路完成");
  console.log(JSON.stringify({
    taskId,
    status: finalStatus,
    model,
    assetId,
    imageURL: absoluteURL(imageURL),
    outputPath,
    pointsBefore: pointSnapshot(pointsBefore, taskId),
    pointsAfter: pointSnapshot(pointsAfter, taskId),
  }, null, 2));
}

main().catch(error => {
  console.error(`[FAIL] ${error instanceof Error ? error.message : String(error)}`);
  process.exitCode = 1;
});
