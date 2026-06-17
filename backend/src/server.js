import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRuntimeStore } from "./runtime-store.js";
import { Platform } from "./platform.js";
import { buildPptx } from "./pptx.js";
import { buildPdf } from "./pdf.js";
import { infrastructureHealth } from "./health.js";
import { RabbitTaskQueue } from "./queue.js";
import { RedisCache } from "./redis.js";
import { ObjectStore } from "./object-store.js";
import { PaymentGateway } from "./payment-gateway.js";
import { ModelGateway } from "./model-gateway.js";
import { GeoSource } from "./geo-source.js";
import { Observability, rateLimitPolicy } from "./observability.js";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const publicDir = path.join(root, "frontend");
const store = await createRuntimeStore();
const cache = new RedisCache();
const objectStore = new ObjectStore();
const taskQueue = process.env.RABBITMQ_URL ? new RabbitTaskQueue() : null;
const modelGateway = new ModelGateway();
const geoSource = new GeoSource();
const paymentGateway = new PaymentGateway();
const observability = new Observability();
const platform = new Platform(store, { taskQueue, cache, objectStore, modelGateway, geoSource });

const securityHeaders = {
  "X-Content-Type-Options": "nosniff",
  "X-Frame-Options": "DENY",
  "Referrer-Policy": "no-referrer",
  "Content-Security-Policy": "default-src 'self'; img-src 'self' data: https://picsum.photos; style-src 'self'; script-src 'self'; connect-src 'self'"
};

async function send(res, status, data, requestId) {
  await store.flush();
  res.writeHead(status, { ...securityHeaders, "Content-Type": "application/json; charset=utf-8", "X-Request-Id": requestId });
  res.end(JSON.stringify({ code: status < 400 ? "0" : String(status), message: status < 400 ? "success" : data.message, data: status < 400 ? data : null, requestId }));
}

async function body(req) {
  const chunks = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > 7 * 1024 * 1024) throw new Error("请求内容超过 7MB 限制");
    chunks.push(chunk);
  }
  if (!chunks.length) return {};
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function serveStatic(req, res) {
  const pathname = new URL(req.url, "http://localhost").pathname;
  const target = pathname === "/" ? path.join(publicDir, "index.html") : path.join(publicDir, pathname);
  if (!target.startsWith(publicDir) || !fs.existsSync(target) || fs.statSync(target).isDirectory()) return false;
  const ext = path.extname(target);
  const types = { ".html": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".svg": "image/svg+xml" };
  res.writeHead(200, { ...securityHeaders, "Content-Type": types[ext] || "application/octet-stream" });
  fs.createReadStream(target).pipe(res);
  return true;
}

export function createServer() {
  return http.createServer(async (req, res) => {
    const startedAt = Date.now();
    const requestId = req.headers["x-request-id"] || crypto.randomUUID();
    const url = new URL(req.url, "http://localhost");
    const method = req.method;
    res.on("finish", () => observability.record({ method, path: url.pathname, status: res.statusCode, latencyMs: Date.now() - startedAt }));
    try {
      await store.refresh();
      const policy = rateLimitPolicy(url.pathname);
      const clientKey = String(req.socket.remoteAddress || "unknown");
      const rate = observability.checkRateLimit(`${policy.group}:${clientKey}`, policy.limit, policy.windowMs);
      res.setHeader("X-RateLimit-Remaining", String(rate.remaining));
      res.setHeader("X-RateLimit-Reset", String(rate.resetAt));
      if (!rate.allowed) return send(res, 429, { message: "请求过于频繁，请稍后重试" }, requestId);
      if (url.pathname.startsWith("/assets/content/")) {
        const asset = store.state.assets.find((item) => item.id === url.pathname.split("/").pop() && item.storageKey);
        if (!asset) { res.writeHead(404); res.end("Not found"); return; }
        const stream = await objectStore.get(asset.storageKey);
        res.writeHead(200, { ...securityHeaders, "Content-Type": asset.metadata?.contentType || (asset.mediaType === "image" ? "image/svg+xml" : "text/plain; charset=utf-8") });
        stream.pipe(res);
        return;
      }
      if (method === "GET" && url.pathname === "/metrics") {
        const expected = process.env.METRICS_TOKEN;
        const token = String(req.headers.authorization || "").replace(/^Bearer\s+/i, "");
        if (expected && token !== expected) {
          res.writeHead(401, { ...securityHeaders, "Content-Type": "text/plain; charset=utf-8", "X-Request-Id": requestId });
          res.end("Unauthorized\n");
          return;
        }
        res.writeHead(200, { ...securityHeaders, "Content-Type": "text/plain; version=0.0.4; charset=utf-8", "X-Request-Id": requestId });
        res.end(observability.prometheus());
        return;
      }
      if (!url.pathname.startsWith("/api/")) {
        if (serveStatic(req, res)) return;
        res.writeHead(404); res.end("Not found"); return;
      }
      if (method === "GET" && url.pathname === "/api/v1/health") return send(res, 200, await infrastructureHealth(), requestId);
      if (method === "GET" && url.pathname === "/api/v1/runtime") return send(res, 200, { storeBackend: store.backend, storeVersion: store.version || null, workerMode: false }, requestId);
      if (method === "POST" && url.pathname === "/api/v1/auth/register") return send(res, 201, platform.register(await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/auth/login") {
        const input = await body(req); return send(res, 200, platform.login(input.email, input.password), requestId);
      }
      if (method === "POST" && url.pathname === "/api/v1/auth/refresh") return send(res, 200, platform.refreshSession((await body(req)).refreshToken), requestId);
      if (method === "POST" && /^\/api\/v1\/payments\/callback\/[^/]+$/.test(url.pathname)) {
        const channel = url.pathname.split("/").pop();
        return send(res, 200, platform.handlePaymentCallback(channel, await body(req), req.headers["x-payment-signature"], paymentGateway), requestId);
      }
      if (method === "POST" && /^\/api\/v1\/public\/agents\/[^/]+\/call$/.test(url.pathname)) return send(res, 200, platform.publicCallAgent(url.pathname.split("/")[5], (await body(req)).input), requestId);
      const token = String(req.headers.authorization || "").replace(/^Bearer\s+/i, "");
      const user = platform.authenticate(token);
      if (!user) return send(res, 401, { message: "请先登录" }, requestId);
      if (method === "POST" && url.pathname === "/api/v1/auth/logout") return send(res, 200, platform.logout(user, token), requestId);

      if (method === "GET" && url.pathname === "/api/v1/dashboard") return send(res, 200, platform.dashboard(user), requestId);
      if (method === "GET" && url.pathname === "/api/v1/plans") return send(res, 200, store.state.plans, requestId);
      if (method === "GET" && url.pathname === "/api/v1/models") return send(res, 200, platform.availableModels(user, url.searchParams.get("capability")), requestId);
      if (method === "GET" && url.pathname === "/api/v1/points/account") return send(res, 200, { account: platform.account(user.id), transactions: platform.listFor(user, "pointTransactions").filter((item) => item.userId === user.id) }, requestId);
      if (method === "GET" && url.pathname === "/api/v1/coupons") return send(res, 200, platform.couponsFor(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/coupons") return send(res, 201, platform.createCoupon(user, await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/coupons/claim") return send(res, 201, platform.claimCoupon(user, (await body(req)).code), requestId);
      if (method === "GET" && url.pathname === "/api/v1/redemption-codes") return send(res, 200, platform.redemptionCodesFor(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/redemption-codes") return send(res, 201, platform.createRedemptionCode(user, await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/redemption-codes/redeem") return send(res, 200, platform.redeemCode(user, (await body(req)).code), requestId);

      const generationMap = {
        "/api/v1/generations/images/text-to-image": "TEXT_TO_IMAGE",
        "/api/v1/generations/images/image-to-image": "IMAGE_TO_IMAGE",
        "/api/v1/generations/videos/text-to-video": "TEXT_TO_VIDEO",
        "/api/v1/generations/videos/image-to-video": "IMAGE_TO_VIDEO"
      };
      if (method === "POST" && url.pathname === "/api/v1/generations/estimate") {
        const input = await body(req);
        return send(res, 200, platform.generationEstimate(user, input.type, input), requestId);
      }
      if (method === "POST" && generationMap[url.pathname]) {
        const input = await body(req);
        input.idempotencyKey ||= req.headers["idempotency-key"];
        return send(res, 202, platform.createGeneration(user, generationMap[url.pathname], input), requestId);
      }
      if (method === "GET" && url.pathname === "/api/v1/generation-tasks") return send(res, 200, platform.listFor(user, "generationTasks"), requestId);
      if (method === "GET" && /^\/api\/v1\/generation-tasks\/[^/]+\/attempts$/.test(url.pathname)) return send(res, 200, platform.generationAttemptsFor(user, url.pathname.split("/")[4]), requestId);
      if (method === "POST" && /^\/api\/v1\/generation-tasks\/[^/]+\/retry$/.test(url.pathname)) return send(res, 202, platform.retryGeneration(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/generation-tasks\/[^/]+\/cancel$/.test(url.pathname)) return send(res, 200, platform.cancelGeneration(user, url.pathname.split("/")[4]), requestId);
      if (method === "GET" && url.pathname.startsWith("/api/v1/generation-tasks/")) return send(res, 200, platform.taskFor(user, url.pathname.split("/").pop()), requestId);
      if (method === "GET" && url.pathname === "/api/v1/assets") return send(res, 200, platform.listFor(user, "assets"), requestId);
      if (method === "POST" && url.pathname === "/api/v1/assets/upload") return send(res, 201, await platform.uploadAsset(user, await body(req)), requestId);
      if (method === "GET" && /^\/api\/v1\/assets\/[^/]+\/download$/.test(url.pathname)) {
        const asset = platform.assetFor(user, url.pathname.split("/")[4]);
        if (!asset.storageKey) throw new Error("Asset content is not stored locally");
        const stream = await objectStore.get(asset.storageKey);
        res.writeHead(200, {
          ...securityHeaders,
          "Content-Type": asset.metadata?.contentType || "application/octet-stream",
          "Content-Disposition": `attachment; filename="${encodeURIComponent(asset.name)}"`,
          "X-Request-Id": requestId
        });
        stream.pipe(res);
        return;
      }
      if (method === "POST" && /^\/api\/v1\/assets\/[^/]+\/favorite$/.test(url.pathname)) return send(res, 200, platform.setAssetFavorite(user, url.pathname.split("/")[4], (await body(req)).favorite), requestId);
      if (method === "POST" && /^\/api\/v1\/assets\/[^/]+\/regenerate$/.test(url.pathname)) return send(res, 202, platform.regenerateAsset(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "DELETE" && /^\/api\/v1\/assets\/[^/]+$/.test(url.pathname)) return send(res, 200, platform.deleteAsset(user, url.pathname.split("/")[4]), requestId);
      if (method === "GET" && url.pathname === "/api/v1/moderation-logs" && user.role === "SUPER_ADMIN") return send(res, 200, store.state.moderationLogs, requestId);

      if (method === "POST" && url.pathname === "/api/v1/orders") {
        const input = await body(req);
        return send(res, 201, platform.createOrder(user, input.planId, input.couponCode), requestId);
      }
      if (method === "POST" && /^\/api\/v1\/orders\/[^/]+\/pay$/.test(url.pathname)) return send(res, 200, platform.payOrder(user, url.pathname.split("/")[4], (await body(req)).eventId), requestId);
      if (method === "POST" && /^\/api\/v1\/orders\/[^/]+\/refund$/.test(url.pathname)) return send(res, 200, platform.refundOrder(user, url.pathname.split("/")[4], (await body(req)).eventId), requestId);
      if (method === "POST" && /^\/api\/v1\/orders\/[^/]+\/release-commissions$/.test(url.pathname)) return send(res, 200, platform.releaseCommissions(user, url.pathname.split("/")[4]), requestId);
      if (method === "GET" && url.pathname === "/api/v1/orders") return send(res, 200, platform.listFor(user, "orders"), requestId);
      if (method === "GET" && url.pathname === "/api/v1/invoices") return send(res, 200, platform.invoiceVisible(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/invoices") return send(res, 201, platform.requestInvoice(user, await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/invoices\/[^/]+\/issue$/.test(url.pathname)) return send(res, 200, platform.issueInvoice(user, url.pathname.split("/")[4]), requestId);

      if (method === "POST" && url.pathname === "/api/v1/presentations") return send(res, 201, platform.createPresentation(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/presentations") return send(res, 200, platform.listFor(user, "presentations"), requestId);
      if (method === "PUT" && /^\/api\/v1\/presentations\/[^/]+$/.test(url.pathname)) return send(res, 200, platform.updatePresentation(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/presentations\/[^/]+\/regenerate-outline$/.test(url.pathname)) return send(res, 200, platform.regeneratePresentationOutline(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "GET" && /^\/api\/v1\/presentations\/[^/]+\/export-pptx$/.test(url.pathname)) {
        const presentation = platform.presentationFor(user, url.pathname.split("/")[4]);
        const file = buildPptx(presentation);
        res.writeHead(200, {
          "Content-Type": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
          "Content-Disposition": `attachment; filename="${presentation.id}.pptx"`,
          "Content-Length": file.length,
          "X-Request-Id": requestId
        });
        res.end(file);
        return;
      }
      if (method === "GET" && /^\/api\/v1\/presentations\/[^/]+\/export-pdf$/.test(url.pathname)) {
        const presentation = platform.presentationFor(user, url.pathname.split("/")[4]);
        const file = buildPdf(presentation);
        res.writeHead(200, {
          "Content-Type": "application/pdf",
          "Content-Disposition": `attachment; filename="${presentation.id}.pdf"`,
          "Content-Length": file.length,
          "X-Request-Id": requestId
        });
        res.end(file);
        return;
      }
      if (method === "POST" && url.pathname === "/api/v1/agents") return send(res, 201, platform.createAgent(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/agents") return send(res, 200, platform.listFor(user, "agents"), requestId);
      if (method === "POST" && /^\/api\/v1\/agents\/[^/]+\/publish$/.test(url.pathname)) return send(res, 200, platform.publishAgent(user, url.pathname.split("/")[4]), requestId);
      if (method === "POST" && /^\/api\/v1\/agents\/[^/]+\/call$/.test(url.pathname)) return send(res, 200, platform.callAgent(user, url.pathname.split("/")[4], (await body(req)).input), requestId);
      if (method === "POST" && /^\/api\/v1\/agents\/[^/]+\/share$/.test(url.pathname)) return send(res, 201, platform.createAgentShare(user, url.pathname.split("/")[4]), requestId);
      if (method === "GET" && /^\/api\/v1\/agents\/[^/]+\/stats$/.test(url.pathname)) return send(res, 200, platform.agentStats(user, url.pathname.split("/")[4]), requestId);
      if (method === "POST" && /^\/api\/v1\/agent-calls\/[^/]+\/feedback$/.test(url.pathname)) return send(res, 201, platform.submitAgentFeedback(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "PUT" && /^\/api\/v1\/agents\/[^/]+\/workflow$/.test(url.pathname)) return send(res, 200, platform.updateAgentWorkflow(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/agents\/[^/]+\/rollback$/.test(url.pathname)) return send(res, 200, platform.rollbackAgent(user, url.pathname.split("/")[4], (await body(req)).version), requestId);
      if (method === "GET" && /^\/api\/v1\/agents\/[^/]+\/versions$/.test(url.pathname)) {
        const agent = platform.agentFor(user, url.pathname.split("/")[4]);
        return send(res, 200, store.state.agentVersions.filter((item) => item.agentId === agent.id), requestId);
      }
      if (method === "POST" && url.pathname === "/api/v1/knowledge-bases") return send(res, 201, platform.createKnowledgeBase(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/knowledge-bases") return send(res, 200, platform.listFor(user, "knowledgeBases"), requestId);
      if (method === "POST" && /^\/api\/v1\/knowledge-bases\/[^/]+\/documents$/.test(url.pathname)) return send(res, 201, platform.addKnowledgeDocument(user, url.pathname.split("/")[4], await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/geo/brands") return send(res, 201, platform.createGeoBrand(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/geo/brands") return send(res, 200, platform.listFor(user, "geoBrands"), requestId);
      if (method === "POST" && url.pathname === "/api/v1/geo/monitor-tasks") return send(res, 201, await platform.createGeoTask(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/geo/monitor-tasks") return send(res, 200, platform.listFor(user, "geoTasks"), requestId);
      if (method === "GET" && url.pathname === "/api/v1/geo/overview") return send(res, 200, platform.geoOverview(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/geo/schedules") return send(res, 201, platform.createGeoSchedule(user, await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/geo\/schedules\/[^/]+\/run$/.test(url.pathname)) return send(res, 201, await platform.runGeoSchedule(url.pathname.split("/")[5], user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/geo/reports") return send(res, 201, platform.createGeoReport(user, await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/geo/contents") return send(res, 201, platform.createGeoContent(user, await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/geo\/contents\/[^/]+\/publish$/.test(url.pathname)) return send(res, 201, platform.publishGeoContent(user, url.pathname.split("/")[5], await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/geo\/publications\/[^/]+\/metrics$/.test(url.pathname)) return send(res, 201, platform.recordGeoContentMetrics(user, url.pathname.split("/")[5], await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/enterprises") return send(res, 201, platform.createEnterprise(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/enterprises/current") return send(res, 200, platform.enterpriseFor(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/enterprise-members") return send(res, 201, platform.addEnterpriseMember(user, await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/enterprise-members\/[^/]+\/allocate-quota$/.test(url.pathname)) return send(res, 200, platform.allocateEnterpriseQuota(user, url.pathname.split("/")[4], (await body(req)).amount), requestId);
      if (method === "GET" && url.pathname === "/api/v1/channel-agents") return send(res, 200, platform.channelVisible(user), requestId);
      if (method === "GET" && url.pathname === "/api/v1/channel-agents/performance") return send(res, 200, platform.channelPerformance(user), requestId);
      if (method === "GET" && url.pathname === "/api/v1/channel-agents/performance-snapshots") return send(res, 200, platform.channelPerformanceSnapshots(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/channel-agents/performance-snapshots") return send(res, 201, platform.createChannelPerformanceSnapshot(user, (await body(req)).period), requestId);
      if (method === "POST" && url.pathname === "/api/v1/channel-agents") return send(res, 201, platform.createChannelAgent(user, await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/channel-agents\/[^/]+\/approve$/.test(url.pathname)) return send(res, 200, platform.approveChannelAgent(user, url.pathname.split("/")[4]), requestId);
      if (method === "POST" && url.pathname === "/api/v1/channel-customers/bind") return send(res, 200, platform.bindCustomer(user, await body(req)), requestId);
      if (method === "GET" && url.pathname === "/api/v1/commissions") return send(res, 200, platform.commissionVisible(user), requestId);
      if (method === "GET" && url.pathname === "/api/v1/settlement-statements") return send(res, 200, platform.settlementStatementsVisible(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/withdrawals") return send(res, 201, platform.createWithdrawal(user, (await body(req)).amount), requestId);
      if (method === "GET" && url.pathname === "/api/v1/withdrawals") {
        const visibleIds = new Set(platform.channelVisible(user).map((item) => item.id));
        return send(res, 200, user.role === "SUPER_ADMIN" ? store.state.withdrawals : store.state.withdrawals.filter((item) => visibleIds.has(item.agentId)), requestId);
      }
      if (method === "POST" && /^\/api\/v1\/withdrawals\/[^/]+\/approve$/.test(url.pathname)) return send(res, 200, platform.approveWithdrawal(user, url.pathname.split("/")[4]), requestId);
      if (method === "GET" && url.pathname === "/api/v1/audit-logs" && user.role === "SUPER_ADMIN") return send(res, 200, store.state.auditLogs, requestId);
      if (method === "GET" && url.pathname === "/api/v1/admin/overview") return send(res, 200, platform.adminOverview(user), requestId);
      if (method === "GET" && url.pathname === "/api/v1/admin/metrics" && user.role === "SUPER_ADMIN") return send(res, 200, observability.snapshot(), requestId);
      if (method === "GET" && url.pathname === "/api/v1/admin/model-config") return send(res, 200, platform.adminModelConfig(user), requestId);
      if (method === "POST" && url.pathname === "/api/v1/admin/model-providers") return send(res, 201, platform.createModelProvider(user, await body(req)), requestId);
      if (method === "POST" && url.pathname === "/api/v1/admin/model-definitions") return send(res, 201, platform.createModelDefinition(user, await body(req)), requestId);
      if (method === "POST" && /^\/api\/v1\/admin\/model-definitions\/[^/]+\/status$/.test(url.pathname)) return send(res, 200, platform.updateModelDefinitionStatus(user, url.pathname.split("/")[5], (await body(req)).status), requestId);
      if (method === "POST" && /^\/api\/v1\/admin\/users\/[^/]+\/status$/.test(url.pathname)) return send(res, 200, platform.updateUserStatus(user, url.pathname.split("/")[4], (await body(req)).status), requestId);
      return send(res, 404, { message: "接口不存在" }, requestId);
    } catch (error) {
      return send(res, error instanceof SyntaxError ? 400 : 422, { message: error.message || "请求处理失败" }, requestId);
    }
  });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.PORT || 3100);
  createServer().listen(port, () => console.log(`先知 AI 平台已启动：http://localhost:${port}`));
}
