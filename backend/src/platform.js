import crypto from "node:crypto";
import { GeoSource } from "./geo-source.js";

const now = () => new Date().toISOString();
const addDays = (days) => new Date(Date.now() + days * 86400000).toISOString();
const hashToken = (token) => crypto.createHash("sha256").update(String(token)).digest("hex");

function hashPassword(password, salt = crypto.randomBytes(16).toString("hex")) {
  return `${salt}:${crypto.scryptSync(password, salt, 64).toString("hex")}`;
}

function verifyPassword(password, encoded) {
  const [salt, hash] = encoded.split(":");
  return crypto.timingSafeEqual(Buffer.from(hash, "hex"), crypto.scryptSync(password, salt, 64));
}

export class Platform {
  constructor(store, { taskQueue = null, cache = null, objectStore = null, modelGateway = null, geoSource = null, workerMode = false } = {}) {
    this.store = store;
    this.taskQueue = taskQueue;
    this.cache = cache;
    this.objectStore = objectStore;
    this.modelGateway = modelGateway;
    this.geoSource = geoSource || new GeoSource();
    this.workerMode = workerMode;
    this.seed();
  }

  seed() {
    if (this.store.state.users.length) return;
    const admin = this.createUser({ email: "admin@xianzhi.ai", password: "Admin123!", name: "平台管理员", role: "SUPER_ADMIN", points: 100000 });
    const demo = this.createUser({ email: "demo@xianzhi.ai", password: "Demo123!", name: "演示用户", role: "MEMBER", points: 1000 });
    const agent = this.createUser({ email: "agent1@xianzhi.ai", password: "Agent123!", name: "华东一级代理", role: "AGENT_L1", points: 5000 });
    this.store.insert("channelAgents", { id: this.store.id("channel"), userId: agent.id, level: 1, status: "ACTIVE", parentId: null, inviteCode: "EAST001" });
    this.store.insert("geoBrands", { id: this.store.id("brand"), ownerId: demo.id, name: "先知 AI", competitors: ["示例竞品"], keywords: ["AI 内容生产", "智能体平台"] });
    this.store.audit(admin.id, "SYSTEM_SEEDED", "system", "platform");
  }

  createUser({ email, password, name, role = "MEMBER", points = 100, referredBy = null }) {
    if (!email || !password || !name) throw new Error("姓名、邮箱和密码为必填项");
    if (this.store.state.users.some((user) => user.email === email)) throw new Error("邮箱已注册");
    const user = this.store.insert("users", {
      id: this.store.id("user"), email: email.toLowerCase(), name, role,
      passwordHash: hashPassword(password), status: "ACTIVE", planId: "plan_free",
      subscriptionExpiresAt: addDays(36500), referredBy
    });
    this.store.insert("pointAccounts", { id: this.store.id("points"), userId: user.id, available: points, frozen: 0 });
    this.store.insert("pointTransactions", {
      id: this.store.id("ptx"), userId: user.id, type: "GRANT", amount: points,
      availableAfter: points, frozenAfter: 0, referenceType: "USER", referenceId: user.id
    });
    return this.publicUser(user);
  }

  publicUser(user) {
    const { passwordHash, ...safe } = user;
    return safe;
  }

  register(body) {
    const channel = body.inviteCode ? this.store.state.channelAgents.find((item) => item.inviteCode === body.inviteCode && item.status === "ACTIVE") : null;
    if (body.inviteCode && !channel) throw new Error("邀请码无效");
    const created = this.createUser({ ...body, referredBy: channel?.userId || null });
    if (channel) {
      this.pointTransaction(created.id, "INVITE_REWARD", 20, 0, "CHANNEL_AGENT", channel.id);
      this.pointTransaction(channel.userId, "INVITE_REWARD", 50, 0, "USER", created.id);
      this.store.audit(created.id, "REGISTER_WITH_INVITE", "channelAgent", channel.id, { inviterUserId: channel.userId });
    }
    return created;
  }

  login(email, password) {
    const user = this.store.state.users.find((item) => item.email === String(email).toLowerCase());
    if (user && user.status !== "ACTIVE") throw new Error("User account is not active");
    if (!user || !verifyPassword(password || "", user.passwordHash)) throw new Error("邮箱或密码错误");
    const token = crypto.randomBytes(24).toString("hex");
    const refreshToken = crypto.randomBytes(32).toString("hex");
    this.store.insert("sessions", {
      id: hashToken(token), userId: user.id, expiresAt: addDays(1), refreshTokenHash: hashToken(refreshToken),
      refreshExpiresAt: addDays(30), revokedAt: null
    });
    this.cache?.setJson(`session:${hashToken(token)}`, { userId: user.id, expiresAt: addDays(1) }, 86400).catch(() => {});
    this.store.audit(user.id, "LOGIN", "user", user.id);
    return { token, refreshToken, expiresAt: addDays(1), user: this.publicUser(user) };
  }

  authenticate(token) {
    const session = this.store.state.sessions.find((item) => item.id === hashToken(token || "") && !item.revokedAt && item.expiresAt > now());
    if (!session) return null;
    return this.store.state.users.find((user) => user.id === session.userId && user.status === "ACTIVE") || null;
  }

  refreshSession(refreshToken) {
    const refreshTokenHash = hashToken(refreshToken || "");
    const session = this.store.state.sessions.find((item) => item.refreshTokenHash === refreshTokenHash && !item.revokedAt && item.refreshExpiresAt > now());
    if (!session) throw new Error("Refresh token is invalid or expired");
    const user = this.store.state.users.find((item) => item.id === session.userId && item.status === "ACTIVE");
    if (!user) throw new Error("User account is not active");
    const previousTokenHash = session.id;
    const token = crypto.randomBytes(24).toString("hex");
    const nextRefreshToken = crypto.randomBytes(32).toString("hex");
    session.id = hashToken(token);
    session.expiresAt = addDays(1);
    session.refreshTokenHash = hashToken(nextRefreshToken);
    session.refreshExpiresAt = addDays(30);
    session.updatedAt = now();
    this.store.save();
    this.cache?.setJson(`session:${hashToken(token)}`, { userId: user.id, expiresAt: session.expiresAt }, 86400).catch(() => {});
    this.store.audit(user.id, "REFRESH_SESSION", "session", session.id, { previousTokenHash });
    return { token, refreshToken: nextRefreshToken, expiresAt: session.expiresAt, user: this.publicUser(user) };
  }

  logout(user, token) {
    const accessTokenHash = hashToken(token || "");
    const session = this.store.state.sessions.find((item) => item.id === accessTokenHash && item.userId === user.id && !item.revokedAt);
    if (!session) return { revoked: false };
    session.revokedAt = now();
    session.updatedAt = now();
    this.store.save();
    this.store.audit(user.id, "LOGOUT", "session", accessTokenHash);
    return { revoked: true };
  }

  account(userId) {
    return this.store.state.pointAccounts.find((item) => item.userId === userId);
  }

  pointTransaction(userId, type, availableDelta, frozenDelta, referenceType, referenceId) {
    const account = this.account(userId);
    if (!account) throw new Error("积分账户不存在");
    if (account.available + availableDelta < 0 || account.frozen + frozenDelta < 0) throw new Error("积分余额不足");
    account.available += availableDelta;
    account.frozen += frozenDelta;
    account.updatedAt = now();
    const tx = this.store.insert("pointTransactions", {
      id: this.store.id("ptx"), userId, type, amount: availableDelta || frozenDelta,
      availableAfter: account.available, frozenAfter: account.frozen, referenceType, referenceId
    });
    this.store.save();
    return tx;
  }

  generationCost(type, params = {}) {
    const base = { TEXT_TO_IMAGE: 10, IMAGE_TO_IMAGE: 15, TEXT_TO_VIDEO: 80, IMAGE_TO_VIDEO: 100 };
    const model = this.modelCatalog().find((item) => item.code === (params.model || "mock-standard"));
    const configured = model?.pointCosts?.[type];
    return (Number(configured) || base[type] || 10) * Math.max(1, Number(params.count || 1));
  }

  generationEstimate(user, type, params = {}) {
    const allowed = ["TEXT_TO_IMAGE", "IMAGE_TO_IMAGE", "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO"];
    if (!allowed.includes(type)) throw new Error("Unsupported generation type");
    const model = params.model || "mock-standard";
    if (!this.availableModels(user, type).some((item) => item.code === model)) throw new Error("Selected model is not available for the current membership plan");
    return {
      type,
      model: params.model || "mock-standard",
      count: Math.max(1, Number(params.count || 1)),
      pointCost: this.generationCost(type, params)
    };
  }

  modelCatalog() {
    const defaults = [
      { code: "mock-standard", name: "标准模型", capabilities: ["TEXT_TO_IMAGE", "IMAGE_TO_IMAGE", "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO"], tier: "FREE", pointCosts: {} },
      { code: "mock-quality", name: "高质量模型", capabilities: ["TEXT_TO_IMAGE", "IMAGE_TO_IMAGE", "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO"], tier: "PAID", pointCosts: {} }
    ];
    const providerModels = this.providerCatalogModels();
    const baseCatalog = [...defaults];
    for (const model of providerModels) {
      if (!baseCatalog.some((item) => item.code === model.code)) baseCatalog.push(model);
    }
    if (!this.store.state.modelDefinitions.length) return baseCatalog;
    const configuredCodes = new Set(this.store.state.modelDefinitions.map((item) => item.code));
    const configured = this.store.state.modelDefinitions.filter((item) => item.status === "ACTIVE").map((item) => ({
      code: item.code, name: item.name, capabilities: item.capabilities, tier: item.tier,
      pointCosts: item.pointCosts || {}, providerCode: item.providerCode
    }));
    return [...baseCatalog.filter((item) => !configuredCodes.has(item.code)), ...configured];
  }

  providerCatalogModels() {
    const models = [];
    const add = (model) => {
      if (model?.code && !models.some((item) => item.code === model.code)) models.push(model);
    };
    if (this.modelGateway?.kind === "openai") {
      add({
        code: this.modelGateway.openAiImageModel,
        name: this.modelGateway.openAiImageModel,
        capabilities: ["TEXT_TO_IMAGE"],
        tier: "FREE",
        pointCosts: { TEXT_TO_IMAGE: 10 },
        providerCode: "openai"
      });
      add({
        code: this.modelGateway.openAiVideoModel,
        name: this.modelGateway.openAiVideoModel,
        capabilities: ["TEXT_TO_VIDEO"],
        tier: "PAID",
        pointCosts: { TEXT_TO_VIDEO: 80 },
        providerCode: "openai"
      });
    }
    for (const provider of this.modelGateway?.providers || []) {
      if (provider.kind !== "openai") continue;
      add({
        code: provider.imageModel || this.modelGateway.openAiImageModel,
        name: provider.imageModel || this.modelGateway.openAiImageModel,
        capabilities: ["TEXT_TO_IMAGE"],
        tier: provider.imageTier || "FREE",
        pointCosts: { TEXT_TO_IMAGE: Number(provider.imagePointCost || 10) },
        providerCode: provider.code || "openai"
      });
      add({
        code: provider.videoModel || this.modelGateway.openAiVideoModel,
        name: provider.videoModel || this.modelGateway.openAiVideoModel,
        capabilities: ["TEXT_TO_VIDEO"],
        tier: provider.videoTier || "PAID",
        pointCosts: { TEXT_TO_VIDEO: Number(provider.videoPointCost || 80) },
        providerCode: provider.code || "openai"
      });
    }
    return models.filter((item) => item.code);
  }

  adminModelConfig(user) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can manage model configuration");
    return {
      providers: this.store.state.modelProviders,
      models: this.store.state.modelDefinitions,
      pricingRules: this.store.state.modelPricingRules,
      effectiveCatalog: this.modelCatalog()
    };
  }

  createModelProvider(user, body) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can manage model providers");
    const code = String(body.code || "").trim().toLowerCase();
    if (!code || this.store.state.modelProviders.some((item) => item.code === code)) throw new Error("Provider code is required and must be unique");
    const provider = this.store.insert("modelProviders", {
      id: this.store.id("modelprovider"), code, name: body.name || code,
      baseUrl: body.baseUrl || null, status: "ACTIVE", config: body.config || {}
    });
    this.store.audit(user.id, "CREATE_MODEL_PROVIDER", "modelProvider", provider.id, { code });
    return provider;
  }

  createModelDefinition(user, body) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can manage model definitions");
    const provider = this.store.state.modelProviders.find((item) => item.code === body.providerCode && item.status === "ACTIVE");
    const code = String(body.code || "").trim();
    const capabilities = [...new Set(body.capabilities || [])];
    const allowed = ["TEXT_TO_IMAGE", "IMAGE_TO_IMAGE", "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO"];
    if (!provider || !code || this.store.state.modelDefinitions.some((item) => item.code === code) || !capabilities.length || capabilities.some((item) => !allowed.includes(item))) {
      throw new Error("Model provider, code and capabilities must be valid and unique");
    }
    const tier = String(body.tier || "PAID").toUpperCase();
    if (!["FREE", "PAID"].includes(tier)) throw new Error("Model membership tier is invalid");
    const pointCosts = Object.fromEntries(capabilities.map((capability) => {
      const cost = Number(body.pointCosts?.[capability]);
      if (!Number.isInteger(cost) || cost <= 0) throw new Error("Model point costs must be positive integers");
      return [capability, cost];
    }));
    const model = this.store.insert("modelDefinitions", {
      id: this.store.id("modeldef"), providerCode: provider.code, code, name: body.name || code,
      capabilities, tier, pointCosts, status: "ACTIVE"
    });
    capabilities.forEach((capability) => this.store.insert("modelPricingRules", {
      id: this.store.id("modelprice"), modelCode: code, capability, pointCost: pointCosts[capability], status: "ACTIVE"
    }));
    this.store.audit(user.id, "CREATE_MODEL_DEFINITION", "modelDefinition", model.id, { code, providerCode: provider.code });
    return model;
  }

  updateModelDefinitionStatus(user, id, status) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can manage model definitions");
    const normalized = String(status || "").toUpperCase();
    if (!["ACTIVE", "DISABLED"].includes(normalized)) throw new Error("Model status is invalid");
    const model = this.store.update("modelDefinitions", id, { status: normalized });
    if (!model) throw new Error("Model definition does not exist");
    this.store.state.modelPricingRules.filter((item) => item.modelCode === model.code).forEach((item) => this.store.update("modelPricingRules", item.id, { status: normalized }));
    this.store.audit(user.id, "UPDATE_MODEL_STATUS", "modelDefinition", model.id, { status: normalized });
    return model;
  }

  availableModels(user, capability = null) {
    const plan = this.store.state.plans.find((item) => item.id === user.planId) || this.store.state.plans[0];
    return this.modelCatalog().filter((model) => (!capability || model.capabilities.includes(capability)) && (model.tier === "FREE" || plan.price > 0));
  }

  enforceGenerationEntitlement(user, type, body) {
    const model = body.model || "mock-standard";
    if (!this.availableModels(user, type).some((item) => item.code === model)) throw new Error("Selected model is not available for the current membership plan");
    const plan = this.store.state.plans.find((item) => item.id === user.planId) || this.store.state.plans[0];
    const running = this.store.state.generationTasks.filter((item) => item.userId === user.id && ["QUEUED", "PROCESSING", "RETRYING"].includes(item.status)).length;
    if (running >= plan.concurrency) throw new Error("Generation concurrency limit has been reached");
  }

  activeEnterpriseMembership(userId) {
    return this.store.state.enterpriseMembers.find((item) => item.userId === userId && item.status === "ACTIVE") || null;
  }

  chargeGeneration(user, task, requestedSource = "AUTO") {
    const source = String(requestedSource || "AUTO").toUpperCase();
    if (!["AUTO", "PERSONAL", "ENTERPRISE"].includes(source)) throw new Error("Invalid generation billing source");
    const membership = this.activeEnterpriseMembership(user.id);
    const enterpriseRemaining = membership ? membership.quotaLimit - membership.quotaUsed : 0;
    const useEnterprise = source === "ENTERPRISE" || (source === "AUTO" && enterpriseRemaining >= task.pointCost);
    if (useEnterprise) {
      if (!membership || enterpriseRemaining < task.pointCost) throw new Error("Enterprise member quota is insufficient");
      membership.quotaUsed += task.pointCost;
      membership.updatedAt = now();
      const tx = this.store.insert("enterpriseQuotaTransactions", {
        id: this.store.id("eqtx"), enterpriseId: membership.enterpriseId, memberId: membership.id,
        type: "CONSUME", amount: task.pointCost, availableAfter: membership.quotaLimit - membership.quotaUsed,
        actorId: user.id, referenceType: "GENERATION_TASK", referenceId: task.id
      });
      this.store.update("generationTasks", task.id, {
        billingSource: "ENTERPRISE", enterpriseMemberId: membership.id, billingTransactionId: tx.id
      });
      this.store.save();
      return;
    }
    this.pointTransaction(user.id, "FREEZE", -task.pointCost, task.pointCost, "GENERATION_TASK", task.id);
    this.store.update("generationTasks", task.id, { billingSource: "PERSONAL" });
  }

  settleGeneration(task) {
    if (task.billingSource === "ENTERPRISE") return;
    this.pointTransaction(task.userId, "SETTLE", 0, -task.pointCost, "GENERATION_TASK", task.id);
  }

  refundGeneration(task) {
    if (task.billingSource !== "ENTERPRISE") {
      this.pointTransaction(task.userId, "REFUND", task.pointCost, -task.pointCost, "GENERATION_TASK", task.id);
      return;
    }
    const membership = this.store.state.enterpriseMembers.find((item) => item.id === task.enterpriseMemberId);
    if (!membership) throw new Error("Enterprise generation billing record is missing");
    membership.quotaUsed = Math.max(0, membership.quotaUsed - task.pointCost);
    membership.updatedAt = now();
    this.store.insert("enterpriseQuotaTransactions", {
      id: this.store.id("eqtx"), enterpriseId: membership.enterpriseId, memberId: membership.id,
      type: "REFUND", amount: -task.pointCost, availableAfter: membership.quotaLimit - membership.quotaUsed,
      actorId: task.userId, referenceType: "GENERATION_TASK", referenceId: task.id
    });
    this.store.save();
  }

  moderate(user, content, contentType = "TEXT") {
    const text = String(content || "").toLowerCase();
    const matchedTerms = this.store.state.sensitiveTerms.filter((term) => text.includes(term.toLowerCase()));
    const result = this.store.insert("moderationLogs", {
      id: this.store.id("moderation"), userId: user.id, contentType,
      status: matchedTerms.length ? "REJECTED" : "PASSED", matchedTerms
    });
    if (matchedTerms.length) throw new Error(`内容安全审核未通过：${matchedTerms.join("、")}`);
    return result;
  }

  createGeneration(user, type, body) {
    const allowed = ["TEXT_TO_IMAGE", "IMAGE_TO_IMAGE", "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO"];
    if (!allowed.includes(type)) throw new Error("Unsupported generation type");
    const idempotencyKey = String(body.idempotencyKey || "").trim();
    if (idempotencyKey) {
      const existing = this.store.state.generationTasks.find((item) => item.userId === user.id && item.idempotencyKey === idempotencyKey);
      if (existing) return existing;
    }
    this.enforceGenerationEntitlement(user, type, body);
    if (type.startsWith("IMAGE_TO_")) {
      if (!body.referenceAssetId) throw new Error("Reference image asset is required and must be accessible");
      let reference;
      try {
        reference = this.usableAssetFor(user, body.referenceAssetId);
      } catch {
        throw new Error("Reference image asset is required and must be accessible");
      }
      if (reference.mediaType !== "image") throw new Error("Reference image asset is required and must be accessible");
    }
    if (!body.prompt?.trim()) throw new Error("提示词不能为空");
    this.moderate(user, body.prompt, "PROMPT");
    const cost = this.generationCost(type, body);
    const task = this.store.insert("generationTasks", {
      id: this.store.id("task"), userId: user.id, type, prompt: body.prompt.trim(),
      params: body, model: body.model || "mock-standard", status: "QUEUED", progress: 0,
      pointCost: cost, billingSource: null, enterpriseMemberId: null, resultIds: [], error: null,
      idempotencyKey: idempotencyKey || null, retryOfTaskId: body.retryOfTaskId || null
    });
    try {
      this.chargeGeneration(user, task, body.billingSource);
    } catch (error) {
      this.store.update("generationTasks", task.id, { status: "FAILED", progress: 100, error: error.message });
      throw error;
    }
    this.store.audit(user.id, "CREATE_GENERATION", "generationTask", task.id, { type, cost, billingSource: task.billingSource });
    if (this.taskQueue) {
      this.taskQueue.publish(task.id).catch((error) => {
        this.store.update("generationTasks", task.id, { status: "FAILED", progress: 100, error: `队列发布失败: ${error.message}` });
        this.refundGeneration(task);
      });
    } else {
      this.processGeneration(task.id);
    }
    this.cacheTask(task);
    return task;
  }

  cacheTask(task) {
    this.cache?.setJson(`generation-task:${task.id}`, task, 86400).catch(() => {});
  }

  processGeneration(taskId) {
    setTimeout(() => {
      const task = this.store.update("generationTasks", taskId, { status: "PROCESSING", progress: 35 });
      if (!task) return;
      this.cacheTask(task);
      setTimeout(() => {
        if (task.prompt.includes("[fail]")) {
          this.store.update("generationTasks", taskId, { status: "FAILED", progress: 100, error: "模拟供应商生成失败" });
          this.refundGeneration(task);
          this.cacheTask(task);
          return;
        }
        const isVideo = task.type.includes("VIDEO");
        const asset = this.store.insert("assets", {
          id: this.store.id("asset"), userId: task.userId, taskId: task.id,
          name: `${task.type}-${task.id}`, mediaType: isVideo ? "video" : "image",
          url: isVideo ? "/assets/demo-video-placeholder" : `https://picsum.photos/seed/${task.id}/960/540`,
          favorite: false, metadata: { prompt: task.prompt, model: task.model }
        });
        task.resultIds = [asset.id];
        this.store.update("generationTasks", taskId, { status: "SUCCEEDED", progress: 100, resultIds: [asset.id] });
        this.settleGeneration(task);
        this.cacheTask(task);
      }, 450);
    }, 150);
  }

  async executeGeneration(taskId) {
    const task = this.store.state.generationTasks.find((item) => item.id === taskId);
    if (!task || !["QUEUED", "RETRYING"].includes(task.status)) return task;
    this.store.update("generationTasks", taskId, { status: "PROCESSING", progress: 35, workerStartedAt: now() });
    this.cacheTask(task);
    await new Promise((resolve) => setTimeout(resolve, 350));
    if (task.prompt.includes("[fail]")) {
      this.store.update("generationTasks", taskId, { status: "FAILED", progress: 100, error: "模拟供应商生成失败", workerFinishedAt: now() });
      this.refundGeneration(task);
      this.cacheTask(task);
      return task;
    }
    try {
      const isVideo = task.type.includes("VIDEO");
      const assetId = this.store.id("asset");
      let storageKey = null;
      let url = isVideo ? "/assets/demo-video-placeholder" : `https://picsum.photos/seed/${task.id}/960/540`;
      if (this.objectStore?.enabled) {
        storageKey = `generated/${task.userId}/${assetId}.${isVideo ? "txt" : "svg"}`;
        const content = isVideo
          ? `先知 AI 视频生成占位资源\n任务：${task.id}\n提示词：${task.prompt}`
          : `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540"><defs><linearGradient id="g"><stop stop-color="#181b3a"/><stop offset="1" stop-color="#5b5cf0"/></linearGradient></defs><rect width="960" height="540" fill="url(#g)"/><text x="60" y="240" fill="white" font-size="42" font-family="Arial">先知 AI 生成作品</text><text x="60" y="310" fill="#d8ddff" font-size="22" font-family="Arial">${task.prompt.replace(/[<>&]/g, "")}</text></svg>`;
        await this.objectStore.put(storageKey, content, isVideo ? "text/plain; charset=utf-8" : "image/svg+xml");
        url = `/assets/content/${assetId}`;
      }
      const asset = this.store.insert("assets", {
        id: assetId, userId: task.userId, taskId: task.id,
        name: `${task.type}-${task.id}`, mediaType: isVideo ? "video" : "image", url, storageKey,
        favorite: false, metadata: { prompt: task.prompt, model: task.model, processedBy: "worker", storage: storageKey ? "minio" : "external" }
      });
      this.store.update("generationTasks", taskId, { status: "SUCCEEDED", progress: 100, resultIds: [asset.id], workerFinishedAt: now() });
      this.settleGeneration(task);
      this.cacheTask(task);
      return task;
    } catch (error) {
      this.store.update("generationTasks", taskId, { status: "FAILED", progress: 100, error: `Worker 执行失败: ${error.message}`, workerFinishedAt: now() });
      this.refundGeneration(task);
      this.cacheTask(task);
      throw error;
    }
  }

  async executeGatewayGeneration(taskId) {
    const task = this.store.state.generationTasks.find((item) => item.id === taskId);
    if (!task || !["QUEUED", "RETRYING"].includes(task.status)) return task;
    const maxAttempts = Math.max(1, Number(task.params?.maxAttempts || 3));
    this.store.update("generationTasks", taskId, { status: "PROCESSING", progress: 20, workerStartedAt: task.workerStartedAt || now() });
    this.cacheTask(task);
    for (let attemptNumber = 1; attemptNumber <= maxAttempts; attemptNumber += 1) {
      const started = Date.now();
      const attempt = this.store.insert("generationAttempts", {
        id: this.store.id("attempt"), taskId: task.id, attempt: attemptNumber, status: "PROCESSING",
        providerRequestId: null, requestSnapshot: { type: task.type, model: task.model, params: task.params },
        responseSnapshot: {}, costCents: 0, error: null, startedAt: now(), finishedAt: null
      });
      try {
        if (!this.modelGateway) throw Object.assign(new Error("Model gateway is not configured"), { retryable: false });
        const result = await this.modelGateway.generate(task);
        const assetId = this.store.id("asset");
        const isVideo = task.type.includes("VIDEO");
        let storageKey = null;
        let url = isVideo ? "/assets/demo-video-placeholder" : `https://picsum.photos/seed/${task.id}/960/540`;
        if (this.objectStore?.enabled) {
          storageKey = `generated/${task.userId}/${assetId}.${result.extension}`;
          await this.objectStore.put(storageKey, result.content, result.contentType);
          url = `/assets/content/${assetId}`;
        }
        const asset = this.store.insert("assets", {
          id: assetId, userId: task.userId, taskId: task.id,
          name: `${task.type}-${task.id}`, mediaType: isVideo ? "video" : "image", url, storageKey,
          favorite: false, metadata: {
            prompt: task.prompt, model: task.model, provider: result.providerCode,
            providerRequestId: result.providerRequestId, processedBy: "worker",
            storage: storageKey ? "minio" : "external", contentType: result.contentType
          }
        });
        this.store.update("generationAttempts", attempt.id, {
          status: "SUCCEEDED", providerRequestId: result.providerRequestId,
          responseSnapshot: result.responseSnapshot, costCents: result.costCents, finishedAt: now()
        });
        this.store.insert("modelCallLogs", {
          id: this.store.id("modelcall"), taskId: task.id, providerCode: result.providerCode,
          modelCode: task.model, status: "SUCCEEDED", latencyMs: Date.now() - started,
          costCents: result.costCents, providerRequestId: result.providerRequestId, error: null
        });
        this.store.update("generationTasks", taskId, {
          status: "SUCCEEDED", progress: 100, resultIds: [asset.id], attemptCount: attemptNumber, workerFinishedAt: now(), error: null
        });
        this.settleGeneration(task);
        this.cacheTask(task);
        return task;
      } catch (error) {
        const retryable = Boolean(error.retryable) && attemptNumber < maxAttempts;
        this.store.update("generationAttempts", attempt.id, { status: retryable ? "RETRYING" : "FAILED", error: error.message, finishedAt: now() });
        this.store.insert("modelCallLogs", {
          id: this.store.id("modelcall"), taskId: task.id, providerCode: this.modelGateway?.providerCode || "unconfigured",
          modelCode: task.model, status: retryable ? "RETRYING" : "FAILED", latencyMs: Date.now() - started,
          costCents: 0, providerRequestId: null, error: error.message
        });
        if (retryable) {
          this.store.update("generationTasks", taskId, { status: "RETRYING", progress: Math.min(80, 20 + attemptNumber * 20), attemptCount: attemptNumber, error: error.message });
          this.cacheTask(task);
          await new Promise((resolve) => setTimeout(resolve, Math.min(1000, attemptNumber * 200)));
          continue;
        }
        this.store.update("generationTasks", taskId, { status: "FAILED", progress: 100, attemptCount: attemptNumber, error: error.message, workerFinishedAt: now() });
        this.refundGeneration(task);
        this.cacheTask(task);
        return task;
      }
    }
    return task;
  }

  async uploadAsset(user, body) {
    if (!this.objectStore?.enabled) throw new Error("对象存储未配置");
    if (!body.name || !body.dataBase64 || !body.contentType) throw new Error("文件名、类型和内容不能为空");
    const allowed = ["image/png", "image/jpeg", "image/webp", "image/svg+xml", "text/plain", "application/pdf"];
    if (!allowed.includes(body.contentType)) throw new Error("不支持的文件类型");
    const content = Buffer.from(body.dataBase64, "base64");
    if (!content.length || content.length > 5 * 1024 * 1024) throw new Error("文件大小必须在 1B 至 5MB 之间");
    if (body.contentType.startsWith("text/") || body.contentType === "image/svg+xml") this.moderate(user, content.toString("utf8"), "FILE");
    const assetId = this.store.id("asset");
    const extension = String(body.name).split(".").pop()?.replace(/[^a-zA-Z0-9]/g, "") || "bin";
    const storageKey = `uploads/${user.id}/${assetId}.${extension}`;
    await this.objectStore.put(storageKey, content, body.contentType);
    const asset = this.store.insert("assets", {
      id: assetId, userId: user.id, taskId: null, name: body.name, mediaType: body.contentType.startsWith("image/") ? "image" : "document",
      url: `/assets/content/${assetId}`, storageKey, favorite: false,
      metadata: { storage: "minio", contentType: body.contentType, size: content.length, source: "upload" }
    });
    this.store.audit(user.id, "UPLOAD_ASSET", "asset", asset.id, { size: content.length, contentType: body.contentType });
    return asset;
  }

  taskFor(user, id) {
    const task = this.store.state.generationTasks.find((item) => item.id === id);
    if (!task || (task.userId !== user.id && user.role !== "SUPER_ADMIN")) throw new Error("任务不存在或无权访问");
    return task;
  }

  retryGeneration(user, id, body = {}) {
    const task = this.taskFor(user, id);
    if (task.status !== "FAILED") throw new Error("Only failed generation tasks can be retried");
    return this.createGeneration(user, task.type, {
      ...task.params,
      ...body,
      prompt: body.prompt || task.prompt,
      model: body.model || task.model,
      retryOfTaskId: task.id
    });
  }

  cancelGeneration(user, id) {
    const task = this.taskFor(user, id);
    if (!["QUEUED", "RETRYING"].includes(task.status)) throw new Error("Only queued or retrying generation tasks can be cancelled");
    this.store.update("generationTasks", task.id, { status: "CANCELLED", progress: 100, error: "Cancelled by user", cancelledAt: now() });
    this.refundGeneration(task);
    this.cacheTask(task);
    this.store.audit(user.id, "CANCEL_GENERATION", "generationTask", task.id, { refundedPoints: task.pointCost, billingSource: task.billingSource });
    return task;
  }

  assetFor(user, id, { includeDeleted = false } = {}) {
    const asset = this.store.state.assets.find((item) => item.id === id);
    if (!asset || (!includeDeleted && asset.deletedAt) || (asset.userId !== user.id && user.role !== "SUPER_ADMIN")) {
      throw new Error("Asset does not exist or is not accessible");
    }
    return asset;
  }

  enterpriseMembership(user) {
    return this.store.state.enterpriseMembers.find((item) => item.userId === user.id && item.status === "ACTIVE") || null;
  }

  isEnterpriseAdmin(user, enterpriseId) {
    return this.store.state.enterpriseMembers.some((item) => item.enterpriseId === enterpriseId && item.userId === user.id && item.role === "ENTERPRISE_ADMIN" && item.status === "ACTIVE");
  }

  isEnterpriseAssetSharedWith(user, assetId) {
    const membership = this.enterpriseMembership(user);
    return Boolean(membership && this.store.state.enterpriseAssetShares.some((item) => item.enterpriseId === membership.enterpriseId && item.assetId === assetId && item.status === "ACTIVE"));
  }

  usableAssetFor(user, id, { includeDeleted = false } = {}) {
    const asset = this.store.state.assets.find((item) => item.id === id);
    if (!asset || (!includeDeleted && asset.deletedAt)) throw new Error("Asset does not exist or is not accessible");
    if (asset.userId === user.id || user.role === "SUPER_ADMIN" || this.isEnterpriseAssetSharedWith(user, asset.id)) return asset;
    throw new Error("Asset does not exist or is not accessible");
  }

  shareAssetToEnterprise(user, id) {
    const asset = this.assetFor(user, id);
    const membership = this.enterpriseMembership(user);
    if (!membership || !this.isEnterpriseAdmin(user, membership.enterpriseId)) throw new Error("Only enterprise administrators can share assets");
    const existing = this.store.state.enterpriseAssetShares.find((item) => item.enterpriseId === membership.enterpriseId && item.assetId === asset.id && item.status === "ACTIVE");
    if (existing) return existing;
    const share = this.store.insert("enterpriseAssetShares", {
      id: this.store.id("eassetshare"), enterpriseId: membership.enterpriseId, assetId: asset.id,
      ownerId: asset.userId, sharedBy: user.id, status: "ACTIVE"
    });
    this.store.audit(user.id, "SHARE_ENTERPRISE_ASSET", "asset", asset.id, { enterpriseId: membership.enterpriseId });
    return share;
  }

  setAssetFavorite(user, id, favorite) {
    const asset = this.assetFor(user, id);
    this.store.update("assets", asset.id, { favorite: Boolean(favorite) });
    this.store.audit(user.id, "UPDATE_ASSET_FAVORITE", "asset", asset.id, { favorite: asset.favorite });
    return asset;
  }

  deleteAsset(user, id) {
    const asset = this.assetFor(user, id);
    this.store.update("assets", asset.id, { deletedAt: now() });
    this.store.audit(user.id, "DELETE_ASSET", "asset", asset.id);
    return asset;
  }

  regenerateAsset(user, id, body = {}) {
    const asset = this.assetFor(user, id);
    if (!asset.taskId) throw new Error("Only generated assets can be regenerated");
    const task = this.taskFor(user, asset.taskId);
    return this.createGeneration(user, task.type, {
      ...task.params,
      ...body,
      prompt: body.prompt || task.prompt,
      model: body.model || task.model,
      retryOfTaskId: task.id
    });
  }

  generationAttemptsFor(user, taskId) {
    this.taskFor(user, taskId);
    return this.store.state.generationAttempts.filter((item) => item.taskId === taskId);
  }

  createCoupon(user, body) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can create coupons");
    const code = String(body.code || "").trim().toUpperCase();
    const type = String(body.type || "FIXED").toUpperCase();
    if (!code || this.store.state.coupons.some((item) => item.code === code)) throw new Error("Coupon code is required and must be unique");
    if (!["FIXED", "PERCENT"].includes(type)) throw new Error("Coupon type is invalid");
    const value = Number(body.value);
    if (!Number.isFinite(value) || value <= 0 || (type === "PERCENT" && value > 100)) throw new Error("Coupon value is invalid");
    const coupon = this.store.insert("coupons", {
      id: this.store.id("coupon"), code, name: body.name || code, type, value,
      minAmount: Math.max(0, Number(body.minAmount || 0)), maxUses: Math.max(1, Number(body.maxUses || 1)),
      usesCount: 0, status: "ACTIVE", expiresAt: body.expiresAt || addDays(30), createdBy: user.id
    });
    this.store.audit(user.id, "CREATE_COUPON", "coupon", coupon.id);
    return coupon;
  }

  claimCoupon(user, code) {
    const coupon = this.store.state.coupons.find((item) => item.code === String(code || "").trim().toUpperCase());
    if (!coupon || coupon.status !== "ACTIVE" || coupon.expiresAt <= now() || coupon.usesCount >= coupon.maxUses) throw new Error("Coupon is invalid or unavailable");
    const existing = this.store.state.userCoupons.find((item) => item.userId === user.id && item.couponId === coupon.id);
    if (existing) return existing;
    const claimed = this.store.insert("userCoupons", {
      id: this.store.id("usercoupon"), userId: user.id, couponId: coupon.id, status: "AVAILABLE", orderId: null
    });
    this.store.audit(user.id, "CLAIM_COUPON", "coupon", coupon.id);
    return { ...claimed, coupon };
  }

  couponsFor(user) {
    return this.store.state.userCoupons.filter((item) => item.userId === user.id).map((item) => ({
      ...item, coupon: this.store.state.coupons.find((coupon) => coupon.id === item.couponId)
    }));
  }

  createRedemptionCode(user, body) {
    const channel = this.store.state.channelAgents.find((item) => item.userId === user.id && item.status === "ACTIVE");
    if (user.role !== "SUPER_ADMIN" && !channel) throw new Error("Only administrators and active agents can create redemption codes");
    const type = String(body.type || "POINTS").toUpperCase();
    if (!["POINTS", "MEMBERSHIP"].includes(type)) throw new Error("Redemption code type is invalid");
    const code = String(body.code || crypto.randomBytes(5).toString("hex")).trim().toUpperCase();
    if (this.store.state.redemptionCodes.some((item) => item.code === code)) throw new Error("Redemption code must be unique");
    const points = type === "POINTS" ? Number(body.points || 0) : 0;
    const plan = type === "MEMBERSHIP" ? this.store.state.plans.find((item) => item.id === body.planId && item.price > 0) : null;
    if ((type === "POINTS" && (!Number.isInteger(points) || points <= 0 || (channel && points > 10000))) || (type === "MEMBERSHIP" && !plan)) {
      throw new Error("Redemption code benefit is invalid");
    }
    const redemption = this.store.insert("redemptionCodes", {
      id: this.store.id("redeem"), code, type, points, planId: plan?.id || null,
      maxUses: Math.max(1, Number(body.maxUses || 1)), usesCount: 0, status: "ACTIVE",
      expiresAt: body.expiresAt || addDays(30), createdBy: user.id, channelAgentId: channel?.id || null
    });
    this.store.audit(user.id, "CREATE_REDEMPTION_CODE", "redemptionCode", redemption.id, { type });
    return redemption;
  }

  redemptionCodesFor(user) {
    return user.role === "SUPER_ADMIN" ? this.store.state.redemptionCodes : this.store.state.redemptionCodes.filter((item) => item.createdBy === user.id);
  }

  redeemCode(user, code) {
    const redemption = this.store.state.redemptionCodes.find((item) => item.code === String(code || "").trim().toUpperCase());
    if (!redemption || redemption.status !== "ACTIVE" || redemption.expiresAt <= now() || redemption.usesCount >= redemption.maxUses) {
      throw new Error("Redemption code is invalid or unavailable");
    }
    if (this.store.state.redemptionUses.some((item) => item.codeId === redemption.id && item.userId === user.id)) throw new Error("Redemption code has already been used");
    if (redemption.type === "POINTS") {
      this.pointTransaction(user.id, "REDEMPTION", redemption.points, 0, "REDEMPTION_CODE", redemption.id);
    } else {
      const plan = this.store.state.plans.find((item) => item.id === redemption.planId);
      user.planId = plan.id;
      user.subscriptionExpiresAt = addDays(plan.durationDays);
    }
    redemption.usesCount += 1;
    redemption.updatedAt = now();
    const use = this.store.insert("redemptionUses", { id: this.store.id("redeemuse"), codeId: redemption.id, userId: user.id, benefitSnapshot: { type: redemption.type, points: redemption.points, planId: redemption.planId } });
    this.store.save();
    this.store.audit(user.id, "REDEEM_CODE", "redemptionCode", redemption.id);
    return { use, benefit: use.benefitSnapshot };
  }

  createOrder(user, planId, couponCode = null) {
    const plan = this.store.state.plans.find((item) => item.id === planId);
    if (!plan || plan.price <= 0) throw new Error("套餐不存在或无需购买");
    let userCoupon = null;
    let discount = 0;
    if (couponCode) {
      const normalized = String(couponCode).trim().toUpperCase();
      userCoupon = this.couponsFor(user).find((item) => item.coupon?.code === normalized && item.status === "AVAILABLE");
      const coupon = userCoupon?.coupon;
      if (!coupon || coupon.status !== "ACTIVE" || coupon.expiresAt <= now() || plan.price < coupon.minAmount || coupon.usesCount >= coupon.maxUses) throw new Error("Coupon is invalid for this order");
      discount = coupon.type === "PERCENT" ? Math.floor(plan.price * coupon.value / 100) : coupon.value;
      discount = Math.min(plan.price, discount);
    }
    const order = this.store.insert("orders", {
      id: this.store.id("order"), userId: user.id, planId, amount: plan.price - discount,
      originalAmount: plan.price, discountAmount: discount, couponId: userCoupon?.couponId || null,
      status: "PENDING", snapshot: { ...plan, coupon: userCoupon?.coupon || null }
    });
    if (userCoupon) this.store.update("userCoupons", userCoupon.id, { status: "RESERVED", orderId: order.id });
    return order;
  }

  payOrder(user, orderId, eventId) {
    if (!eventId) throw new Error("支付事件编号不能为空");
    if (this.store.state.paymentEvents.some((item) => item.eventId === eventId)) {
      return this.store.state.orders.find((item) => item.id === orderId);
    }
    const order = this.store.state.orders.find((item) => item.id === orderId);
    if (!order || (order.userId !== user.id && user.role !== "SUPER_ADMIN")) throw new Error("订单不存在");
    if (order.status !== "PENDING") throw new Error("订单状态不允许支付");
    this.store.insert("paymentEvents", { id: this.store.id("pay"), eventId, orderId });
    this.store.update("orders", orderId, { status: "PAID", paidAt: now() });
    if (order.couponId) {
      const userCoupon = this.store.state.userCoupons.find((item) => item.userId === order.userId && item.couponId === order.couponId && item.orderId === order.id);
      const coupon = this.store.state.coupons.find((item) => item.id === order.couponId);
      if (userCoupon) this.store.update("userCoupons", userCoupon.id, { status: "USED" });
      if (coupon) this.store.update("coupons", coupon.id, { usesCount: coupon.usesCount + 1 });
    }
    const owner = this.store.state.users.find((item) => item.id === order.userId);
    owner.planId = order.planId;
    owner.subscriptionExpiresAt = addDays(order.snapshot.durationDays);
    this.pointTransaction(owner.id, "PURCHASE_GRANT", order.snapshot.points, 0, "ORDER", order.id);
    this.createCommissions(order);
    this.store.audit(user.id, "PAY_ORDER", "order", order.id, { eventId });
    return order;
  }

  handlePaymentCallback(channel, payload, signature, gateway) {
    gateway.verify(channel, payload, signature);
    if (!payload.eventId || !payload.orderId || payload.status !== "SUCCESS") throw new Error("Payment callback payload is invalid");
    const existing = this.store.state.payments.find((item) => item.channel === channel && item.eventId === payload.eventId);
    if (existing) return existing;
    const order = this.store.state.orders.find((item) => item.id === payload.orderId);
    if (!order || Number(payload.amount) !== order.amount) throw new Error("Payment callback order or amount does not match");
    const systemAdmin = this.store.state.users.find((item) => item.role === "SUPER_ADMIN");
    this.payOrder(systemAdmin, order.id, payload.eventId);
    const payment = this.store.insert("payments", {
      id: this.store.id("payment"), channel, eventId: payload.eventId, orderId: order.id,
      amount: Number(payload.amount), status: "SUCCEEDED", providerTransactionId: payload.providerTransactionId || null
    });
    this.store.audit(systemAdmin.id, "PAYMENT_CALLBACK_SUCCEEDED", "order", order.id, { channel, eventId: payload.eventId });
    return payment;
  }

  requestInvoice(user, body) {
    const order = this.store.state.orders.find((item) => item.id === body.orderId && item.userId === user.id && item.status === "PAID");
    if (!order) throw new Error("Paid order does not exist or is not accessible");
    if (!body.title?.trim()) throw new Error("Invoice title is required");
    const existing = this.store.state.invoices.find((item) => item.orderId === order.id && item.status !== "REJECTED");
    if (existing) return existing;
    const invoice = this.store.insert("invoices", {
      id: this.store.id("invoice"), userId: user.id, orderId: order.id, amount: order.amount,
      title: body.title.trim(), taxNumber: body.taxNumber || null, email: body.email || user.email,
      status: "PENDING", invoiceNumber: null, issuedAt: null
    });
    this.store.audit(user.id, "REQUEST_INVOICE", "invoice", invoice.id, { orderId: order.id });
    return invoice;
  }

  invoiceVisible(user) {
    return user.role === "SUPER_ADMIN" ? this.store.state.invoices : this.store.state.invoices.filter((item) => item.userId === user.id);
  }

  issueInvoice(user, invoiceId) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can issue invoices");
    const invoice = this.store.state.invoices.find((item) => item.id === invoiceId && item.status === "PENDING");
    if (!invoice) throw new Error("Pending invoice does not exist");
    const updated = this.store.update("invoices", invoice.id, {
      status: "ISSUED", invoiceNumber: `XZ-${Date.now()}-${invoice.id.split("_").pop()}`, issuedAt: now()
    });
    this.store.audit(user.id, "ISSUE_INVOICE", "invoice", invoice.id, { invoiceNumber: updated.invoiceNumber });
    return updated;
  }

  refundOrder(user, orderId, eventId) {
    if (user.role !== "SUPER_ADMIN") throw new Error("仅平台管理员可执行退款");
    if (!eventId) throw new Error("退款事件编号不能为空");
    const existing = this.store.state.refunds.find((item) => item.eventId === eventId);
    if (existing) return this.store.state.orders.find((item) => item.id === existing.orderId);
    const order = this.store.state.orders.find((item) => item.id === orderId);
    if (!order || order.status !== "PAID") throw new Error("订单不存在或状态不允许退款");
    const owner = this.store.state.users.find((item) => item.id === order.userId);
    if (this.account(owner.id).available < order.snapshot.points) throw new Error("用户已消费套餐积分，暂不能自动退款");
    this.pointTransaction(owner.id, "ORDER_REFUND", -order.snapshot.points, 0, "ORDER", order.id);
    owner.planId = "plan_free";
    owner.subscriptionExpiresAt = addDays(36500);
    this.store.update("orders", order.id, { status: "REFUNDED", refundedAt: now() });
    if (order.couponId) {
      const userCoupon = this.store.state.userCoupons.find((item) => item.userId === order.userId && item.couponId === order.couponId && item.orderId === order.id);
      const coupon = this.store.state.coupons.find((item) => item.id === order.couponId);
      if (userCoupon) this.store.update("userCoupons", userCoupon.id, { status: "AVAILABLE", orderId: null });
      if (coupon) this.store.update("coupons", coupon.id, { usesCount: Math.max(0, coupon.usesCount - 1) });
    }
    this.store.state.commissions.filter((item) => item.orderId === order.id).forEach((commission) => {
      this.store.update("commissions", commission.id, { status: "REVERSED", reversedAt: now() });
    });
    this.store.insert("refunds", { id: this.store.id("refund"), eventId, orderId, amount: order.amount, status: "SUCCEEDED" });
    this.store.audit(user.id, "REFUND_ORDER", "order", order.id, { eventId });
    return order;
  }

  createCommissions(order) {
    const buyer = this.store.state.users.find((item) => item.id === order.userId);
    const channel = this.store.state.channelAgents.find((item) => item.userId === buyer.referredBy);
    if (!channel) return;
    const rate = channel.level === 1 ? 0.3 : 0.2;
    this.store.insert("commissions", {
      id: this.store.id("commission"), orderId: order.id, agentId: channel.id,
      amount: Math.floor(order.amount * rate), rate, status: "FROZEN", snapshot: { level: channel.level, rate }
    });
    if (channel.level === 2 && channel.parentId) {
      this.store.insert("commissions", {
        id: this.store.id("commission"), orderId: order.id, agentId: channel.parentId,
        amount: Math.floor(order.amount * 0.1), rate: 0.1, status: "FROZEN",
        snapshot: { level: 1, rate: 0.1, type: "MANAGEMENT_REWARD" }
      });
    }
  }

  createChannelAgent(user, body) {
    const actorChannel = this.store.state.channelAgents.find((item) => item.userId === user.id);
    const isAdmin = user.role === "SUPER_ADMIN";
    if (!isAdmin && (!actorChannel || actorChannel.level !== 1)) throw new Error("仅平台管理员或一级代理商可创建代理商");
    const level = isAdmin ? Number(body.level || 1) : 2;
    if (![1, 2].includes(level)) throw new Error("代理商等级无效");
    const parentId = level === 2 ? (actorChannel?.id || body.parentId) : null;
    if (level === 2 && !parentId) throw new Error("二级代理商必须指定上级");
    const created = this.createUser({ email: body.email, password: body.password, name: body.name, role: level === 1 ? "AGENT_L1" : "AGENT_L2", points: 1000 });
    return this.store.insert("channelAgents", {
      id: this.store.id("channel"), userId: created.id, level, status: isAdmin ? "ACTIVE" : "PENDING",
      parentId, inviteCode: body.inviteCode || crypto.randomBytes(4).toString("hex").toUpperCase()
    });
  }

  approveChannelAgent(user, channelId) {
    if (user.role !== "SUPER_ADMIN") throw new Error("仅平台管理员可审核代理商");
    const channel = this.store.update("channelAgents", channelId, { status: "ACTIVE", approvedAt: now() });
    if (!channel) throw new Error("代理商不存在");
    this.store.audit(user.id, "APPROVE_CHANNEL_AGENT", "channelAgent", channel.id);
    return channel;
  }

  bindCustomer(user, body) {
    const channel = this.store.state.channelAgents.find((item) => item.userId === user.id && item.status === "ACTIVE");
    if (!channel) throw new Error("仅已启用代理商可绑定客户");
    const customer = this.store.state.users.find((item) => item.email === String(body.email).toLowerCase());
    if (!customer || customer.role !== "MEMBER") throw new Error("客户不存在或角色不允许绑定");
    if (customer.referredBy && customer.referredBy !== user.id) throw new Error("客户已绑定其他代理商");
    customer.referredBy = user.id;
    customer.updatedAt = now();
    this.store.save();
    this.store.audit(user.id, "BIND_CUSTOMER", "user", customer.id);
    return this.publicUser(customer);
  }

  channelVisible(user) {
    if (user.role === "SUPER_ADMIN") return this.store.state.channelAgents;
    const own = this.store.state.channelAgents.find((item) => item.userId === user.id);
    if (!own) return [];
    return this.store.state.channelAgents.filter((item) => item.id === own.id || item.parentId === own.id);
  }

  commissionVisible(user) {
    if (user.role === "SUPER_ADMIN") return this.store.state.commissions;
    const ids = new Set(this.channelVisible(user).map((item) => item.id));
    return this.store.state.commissions.filter((item) => ids.has(item.agentId));
  }

  channelPerformance(user) {
    const visible = this.channelVisible(user);
    if (!visible.length && user.role !== "SUPER_ADMIN") throw new Error("Current user is not an agent");
    const rows = visible.map((channel) => {
      const agentUser = this.store.state.users.find((item) => item.id === channel.userId);
      const teamChannels = [channel, ...this.store.state.channelAgents.filter((item) => item.parentId === channel.id)];
      const teamUserIds = new Set(teamChannels.map((item) => item.userId));
      const customers = this.store.state.users.filter((item) => teamUserIds.has(item.referredBy));
      const customerIds = new Set(customers.map((item) => item.id));
      const orders = this.store.state.orders.filter((item) => customerIds.has(item.userId));
      const paidOrders = orders.filter((item) => item.status === "PAID");
      const refundedOrders = orders.filter((item) => item.status === "REFUNDED");
      const commissions = this.store.state.commissions.filter((item) => teamChannels.some((entry) => entry.id === item.agentId));
      const commissionByStatus = (status) => commissions.filter((item) => item.status === status).reduce((sum, item) => sum + item.amount, 0);
      return {
        channelId: channel.id, userId: channel.userId, name: agentUser?.name || channel.id,
        level: channel.level, status: channel.status, inviteCode: channel.inviteCode,
        directAgents: teamChannels.length - 1, customers: customers.length,
        orders: orders.length, paidOrders: paidOrders.length, refundedOrders: refundedOrders.length,
        revenue: paidOrders.reduce((sum, item) => sum + item.amount, 0),
        refundedRevenue: refundedOrders.reduce((sum, item) => sum + item.amount, 0),
        commissionFrozen: commissionByStatus("FROZEN"), commissionAvailable: commissionByStatus("AVAILABLE"),
        commissionSettled: commissionByStatus("SETTLED"), commissionReversed: commissionByStatus("REVERSED")
      };
    });
    rows.sort((a, b) => b.revenue - a.revenue || b.paidOrders - a.paidOrders || a.channelId.localeCompare(b.channelId));
    return rows.map((item, index) => ({ ...item, rank: index + 1 }));
  }

  createChannelPerformanceSnapshot(user, period = "ALL") {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can create channel performance snapshots");
    const rows = this.channelPerformance(user);
    const snapshot = this.store.insert("channelPerformanceSnapshots", {
      id: this.store.id("channelperf"), period: String(period || "ALL").toUpperCase(),
      totals: {
        agents: rows.length, customers: rows.reduce((sum, item) => sum + item.customers, 0),
        paidOrders: rows.reduce((sum, item) => sum + item.paidOrders, 0),
        revenue: rows.reduce((sum, item) => sum + item.revenue, 0)
      },
      rankings: rows, createdBy: user.id
    });
    this.store.audit(user.id, "CREATE_CHANNEL_PERFORMANCE_SNAPSHOT", "channelPerformanceSnapshot", snapshot.id, { period: snapshot.period });
    return snapshot;
  }

  channelPerformanceSnapshots(user) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can view all channel performance snapshots");
    return this.store.state.channelPerformanceSnapshots;
  }

  createPresentation(user, body) {
    if (!body.topic?.trim()) throw new Error("PPT 主题不能为空");
    const topics = ["背景与目标", "核心方案", "实施路径", "价值与展望"];
    return this.store.insert("presentations", {
      id: this.store.id("ppt"), userId: user.id, topic: body.topic.trim(), status: "DRAFT",
      theme: body.theme || "科技蓝", slides: topics.map((title, index) => ({
        index: index + 1, title, content: `${body.topic}：${title}的关键内容与执行建议。`
      }))
    });
  }

  presentationFor(user, id) {
    const item = this.store.state.presentations.find((entry) => entry.id === id);
    if (!item || (item.userId !== user.id && user.role !== "SUPER_ADMIN")) throw new Error("PPT 项目不存在或无权访问");
    return item;
  }

  updatePresentation(user, id, body) {
    const presentation = this.presentationFor(user, id);
    const slides = body.slides || presentation.slides;
    if (!Array.isArray(slides) || !slides.length || slides.length > 50) throw new Error("Presentation slides must contain between 1 and 50 pages");
    const normalized = slides.map((slide, index) => {
      if (!slide.title?.trim()) throw new Error("Every slide requires a title");
      return { index: index + 1, title: slide.title.trim(), content: String(slide.content || "").trim(), notes: String(slide.notes || "").trim() };
    });
    const updated = this.store.update("presentations", id, {
      topic: body.topic?.trim() || presentation.topic, theme: body.theme || presentation.theme,
      slides: normalized, status: "DRAFT"
    });
    this.store.audit(user.id, "UPDATE_PRESENTATION", "presentation", id, { slides: normalized.length });
    return updated;
  }

  regeneratePresentationOutline(user, id, body = {}) {
    const presentation = this.presentationFor(user, id);
    const outline = Array.isArray(body.outline) && body.outline.length
      ? body.outline
      : ["问题与机会", "目标与策略", "核心方案", "落地计划", "指标与展望"];
    return this.updatePresentation(user, id, {
      slides: outline.map((title) => ({ title, content: `${presentation.topic}：${title}的关键内容、证据与行动建议。` }))
    });
  }

  createAgent(user, body) {
    if (!body.name?.trim()) throw new Error("智能体名称不能为空");
    const knowledgeBaseIds = this.validateKnowledgeBaseIds(user, body.knowledgeBaseIds || []);
    this.validateAgentWorkflow(body.workflow || [{ id: "start", type: "START" }, { id: "llm", type: "LLM" }, { id: "end", type: "END" }]);
    const agent = this.store.insert("agents", {
      id: this.store.id("agent"), ownerId: user.id, name: body.name.trim(),
      description: body.description || "", status: "DRAFT", version: 1,
      workflow: body.workflow || [{ id: "start", type: "START" }, { id: "llm", type: "LLM" }, { id: "end", type: "END" }],
      knowledgeBaseIds,
      callCount: 0
    });
    this.saveAgentVersion(agent, user.id, "INITIAL");
    return agent;
  }

  agentFor(user, id) {
    const item = this.store.state.agents.find((entry) => entry.id === id);
    if (!item || (item.ownerId !== user.id && user.role !== "SUPER_ADMIN")) throw new Error("智能体不存在或无权访问");
    return item;
  }

  isEnterpriseAgentSharedWith(user, agentId) {
    const membership = this.enterpriseMembership(user);
    return Boolean(membership && this.store.state.enterpriseAgentShares.some((item) => item.enterpriseId === membership.enterpriseId && item.agentId === agentId && item.status === "ACTIVE"));
  }

  usableAgentFor(user, id) {
    const agent = this.usableAgentFor(user, id);
    if (!agent || agent.status !== "PUBLISHED") throw new Error("Agent does not exist or is not accessible");
    if (agent.ownerId === user.id || user.role === "SUPER_ADMIN" || this.isEnterpriseAgentSharedWith(user, agent.id)) return agent;
    throw new Error("Agent does not exist or is not accessible");
  }

  shareAgentToEnterprise(user, id) {
    const agent = this.agentFor(user, id);
    if (agent.status !== "PUBLISHED") throw new Error("Only published agents can be shared to an enterprise");
    const membership = this.enterpriseMembership(user);
    if (!membership || !this.isEnterpriseAdmin(user, membership.enterpriseId)) throw new Error("Only enterprise administrators can share agents");
    const existing = this.store.state.enterpriseAgentShares.find((item) => item.enterpriseId === membership.enterpriseId && item.agentId === agent.id && item.status === "ACTIVE");
    if (existing) return existing;
    const share = this.store.insert("enterpriseAgentShares", {
      id: this.store.id("eagentshare"), enterpriseId: membership.enterpriseId, agentId: agent.id,
      ownerId: agent.ownerId, sharedBy: user.id, status: "ACTIVE"
    });
    this.store.audit(user.id, "SHARE_ENTERPRISE_AGENT", "agent", agent.id, { enterpriseId: membership.enterpriseId });
    return share;
  }

  publishAgent(user, id) {
    const agent = this.agentFor(user, id);
    this.validateAgentWorkflow(agent.workflow);
    if (!Array.isArray(agent.workflow) || agent.workflow.length < 2) throw new Error("工作流不完整");
    const updated = this.store.update("agents", id, { status: "PUBLISHED", publishedAt: now(), version: agent.version + 1 });
    this.saveAgentVersion(updated, user.id, "PUBLISH");
    return updated;
  }

  saveAgentVersion(agent, actorId, reason) {
    return this.store.insert("agentVersions", {
      id: this.store.id("agentversion"), agentId: agent.id, version: agent.version,
      actorId, reason, snapshot: { name: agent.name, description: agent.description, workflow: structuredClone(agent.workflow), knowledgeBaseIds: [...(agent.knowledgeBaseIds || [])] }
    });
  }

  updateAgentWorkflow(user, id, body) {
    const agent = this.agentFor(user, id);
    this.validateAgentWorkflow(body.workflow);
    if (!Array.isArray(body.workflow) || body.workflow.length < 2) throw new Error("工作流至少包含两个节点");
    const ids = new Set();
    for (const node of body.workflow) {
      if (!node.id || !node.type || ids.has(node.id)) throw new Error("工作流节点 ID 和类型必须有效且唯一");
      ids.add(node.id);
    }
    const knowledgeBaseIds = this.validateKnowledgeBaseIds(user, body.knowledgeBaseIds || agent.knowledgeBaseIds || []);
    const updated = this.store.update("agents", id, {
      workflow: body.workflow, knowledgeBaseIds,
      status: "DRAFT", version: agent.version + 1
    });
    this.saveAgentVersion(updated, user.id, "UPDATE_WORKFLOW");
    return updated;
  }

  validateAgentWorkflow(workflow) {
    if (!Array.isArray(workflow) || workflow.length < 2) throw new Error("Agent workflow must contain at least two nodes");
    const allowed = new Set(["START", "LLM", "KNOWLEDGE", "TOOL", "CONDITION", "OUTPUT", "END"]);
    const ids = new Set();
    for (const node of workflow) {
      if (!node.id || !node.type || ids.has(node.id) || !allowed.has(node.type)) throw new Error("Agent workflow node ID and type must be valid and unique");
      ids.add(node.id);
    }
    if (workflow[0].type !== "START" || workflow.at(-1).type !== "END") throw new Error("Agent workflow must start with START and end with END");
    if (workflow.filter((node) => node.type === "START").length !== 1 || workflow.filter((node) => node.type === "END").length !== 1) {
      throw new Error("Agent workflow must contain exactly one START and one END");
    }
    return workflow;
  }

  rollbackAgent(user, id, version) {
    const agent = this.agentFor(user, id);
    const target = this.store.state.agentVersions.find((item) => item.agentId === id && item.version === Number(version));
    if (!target) throw new Error("目标版本不存在");
    const updated = this.store.update("agents", id, {
      ...structuredClone(target.snapshot), status: "DRAFT", version: agent.version + 1
    });
    this.saveAgentVersion(updated, user.id, `ROLLBACK_TO_${version}`);
    return updated;
  }

  callAgent(user, id, input) {
    const agent = this.store.state.agents.find((entry) => entry.id === id);
    if (!agent || agent.status !== "PUBLISHED") throw new Error("智能体未发布或不存在");
    return this.executeAgentCall(agent, user.id, input, "AUTHENTICATED");
  }

  executeAgentCall(agent, userId, input, channel) {
    const started = Date.now();
    const query = String(input || "").trim() || "空输入";
    const references = this.vectorSearchKnowledge(agent.knowledgeBaseIds || [], query);
    const referenceText = references.length ? ` 参考知识：${references.map((item) => item.chunk).join("；")}` : "";
    const output = `${agent.name} 已处理：${query}。这是本地模型网关生成的演示响应。${referenceText}`;
    const call = this.store.insert("agentCalls", {
      id: this.store.id("agentcall"), agentId: agent.id, userId, input, output, channel,
      tokenUsage: Math.max(1, Math.ceil((String(input).length + output.length) / 4)),
      cost: 1, latencyMs: Date.now() - started, references: [...new Set(references.map((item) => item.documentId))]
    });
    this.store.update("agents", agent.id, { callCount: agent.callCount + 1 });
    return call;
  }

  createAgentShare(user, id) {
    const agent = this.agentFor(user, id);
    if (agent.status !== "PUBLISHED") throw new Error("Only published agents can be shared");
    const existing = this.store.state.agentShares.find((item) => item.agentId === agent.id && item.status === "ACTIVE");
    if (existing) return existing;
    const share = this.store.insert("agentShares", {
      id: this.store.id("agentshare"), agentId: agent.id, ownerId: user.id,
      token: crypto.randomBytes(16).toString("hex"), status: "ACTIVE", calls: 0
    });
    this.store.audit(user.id, "CREATE_AGENT_SHARE", "agent", agent.id, { shareId: share.id });
    return share;
  }

  publicCallAgent(token, input) {
    const share = this.store.state.agentShares.find((item) => item.token === token && item.status === "ACTIVE");
    const agent = share && this.store.state.agents.find((item) => item.id === share.agentId && item.status === "PUBLISHED");
    if (!agent) throw new Error("Agent share link is invalid or unavailable");
    const call = this.executeAgentCall(agent, null, input, "PUBLIC_SHARE");
    this.store.update("agentShares", share.id, { calls: share.calls + 1, lastCalledAt: now() });
    return { callId: call.id, agent: { id: agent.id, name: agent.name, description: agent.description }, output: call.output, references: call.references };
  }

  submitAgentFeedback(user, callId, body) {
    const call = this.store.state.agentCalls.find((item) => item.id === callId && (item.userId === user.id || user.role === "SUPER_ADMIN"));
    if (!call) throw new Error("Agent call does not exist or is not accessible");
    const rating = Number(body.rating);
    if (![1, 2, 3, 4, 5].includes(rating)) throw new Error("Feedback rating must be between 1 and 5");
    const existing = this.store.state.agentFeedback.find((item) => item.callId === call.id && item.userId === user.id);
    if (existing) return this.store.update("agentFeedback", existing.id, { rating, comment: body.comment || "" });
    return this.store.insert("agentFeedback", {
      id: this.store.id("agentfeedback"), agentId: call.agentId, callId: call.id, userId: user.id,
      rating, comment: body.comment || ""
    });
  }

  agentStats(user, id) {
    const agent = this.agentFor(user, id);
    const calls = this.store.state.agentCalls.filter((item) => item.agentId === agent.id);
    const feedback = this.store.state.agentFeedback.filter((item) => item.agentId === agent.id);
    const share = this.store.state.agentShares.find((item) => item.agentId === agent.id && item.status === "ACTIVE") || null;
    return {
      agentId: agent.id, calls: calls.length,
      publicCalls: calls.filter((item) => item.channel === "PUBLIC_SHARE").length,
      averageLatencyMs: calls.length ? Math.round(calls.reduce((sum, item) => sum + item.latencyMs, 0) / calls.length) : 0,
      tokenUsage: calls.reduce((sum, item) => sum + item.tokenUsage, 0),
      cost: calls.reduce((sum, item) => sum + item.cost, 0),
      feedbackCount: feedback.length,
      averageRating: feedback.length ? Number((feedback.reduce((sum, item) => sum + item.rating, 0) / feedback.length).toFixed(2)) : null,
      share
    };
  }

  createKnowledgeBase(user, body) {
    if (!body.name?.trim()) throw new Error("知识库名称不能为空");
    return this.store.insert("knowledgeBases", {
      id: this.store.id("kb"), ownerId: user.id, name: body.name.trim(), description: body.description || "", documentCount: 0
    });
  }

  validateKnowledgeBaseIds(user, ids) {
    if (!Array.isArray(ids)) throw new Error("知识库列表格式无效");
    const unique = [...new Set(ids)];
    const allowed = this.store.state.knowledgeBases.filter((item) => unique.includes(item.id) && (item.ownerId === user.id || user.role === "SUPER_ADMIN"));
    if (allowed.length !== unique.length) throw new Error("知识库不存在或无权绑定");
    return unique;
  }

  addKnowledgeDocument(user, knowledgeBaseId, body) {
    const kb = this.store.state.knowledgeBases.find((item) => item.id === knowledgeBaseId && (item.ownerId === user.id || user.role === "SUPER_ADMIN"));
    if (!kb) throw new Error("知识库不存在或无权访问");
    if (!body.name || !body.content?.trim()) throw new Error("文档名称和内容不能为空");
    this.moderate(user, body.content, "KNOWLEDGE_DOCUMENT");
    const chunks = String(body.content).split(/\n{2,}|(?<=[。！？])/).map((item) => item.trim()).filter(Boolean).slice(0, 100);
    const document = this.store.insert("knowledgeDocuments", {
      id: this.store.id("kbdoc"), knowledgeBaseId: kb.id, ownerId: user.id, name: body.name, content: body.content, chunks,
      embeddings: chunks.map((chunk) => this.embedText(chunk))
    });
    this.store.update("knowledgeBases", kb.id, { documentCount: kb.documentCount + 1 });
    return document;
  }

  searchKnowledge(knowledgeBaseIds, query) {
    const terms = String(query).toLowerCase().split(/\s+|[，。！？、]/).filter((item) => item.length > 1);
    return this.store.state.knowledgeDocuments
      .filter((doc) => knowledgeBaseIds.includes(doc.knowledgeBaseId))
      .flatMap((doc) => doc.chunks.map((chunk) => ({ documentId: doc.id, chunk, score: terms.reduce((score, term) => score + (chunk.toLowerCase().includes(term) ? 1 : 0), 0) })))
      .filter((item) => item.score > 0).sort((a, b) => b.score - a.score).slice(0, 3);
  }

  embedText(text, dimensions = 32) {
    const normalized = String(text || "").toLowerCase().replace(/\s+/g, " ").trim();
    const tokens = [...normalized].map((char, index) => `${char}${normalized[index + 1] || ""}`);
    const vector = Array(dimensions).fill(0);
    for (const token of tokens) {
      const digest = crypto.createHash("sha256").update(token).digest();
      vector[digest[0] % dimensions] += digest[1] % 2 ? 1 : -1;
    }
    const magnitude = Math.sqrt(vector.reduce((sum, value) => sum + value * value, 0)) || 1;
    return vector.map((value) => Number((value / magnitude).toFixed(6)));
  }

  cosineSimilarity(left, right) {
    if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return 0;
    return Number(left.reduce((sum, value, index) => sum + value * right[index], 0).toFixed(6));
  }

  vectorSearchKnowledge(knowledgeBaseIds, query) {
    const queryEmbedding = this.embedText(query);
    const terms = String(query).toLowerCase().split(/\s+|[,.;:!?，。；：！？]/).filter((item) => item.length > 1);
    return this.store.state.knowledgeDocuments
      .filter((doc) => knowledgeBaseIds.includes(doc.knowledgeBaseId))
      .flatMap((doc) => doc.chunks.map((chunk, index) => {
        const keywordScore = terms.reduce((score, term) => score + (chunk.toLowerCase().includes(term) ? 1 : 0), 0);
        const vectorScore = this.cosineSimilarity(queryEmbedding, doc.embeddings?.[index] || this.embedText(chunk));
        return { documentId: doc.id, chunk, keywordScore, vectorScore, score: keywordScore + vectorScore };
      }))
      .filter((item) => item.keywordScore > 0 || item.vectorScore >= 0.15)
      .sort((a, b) => b.score - a.score).slice(0, 3);
  }

  createGeoBrand(user, body) {
    if (!body.name?.trim()) throw new Error("品牌名称不能为空");
    return this.store.insert("geoBrands", {
      id: this.store.id("brand"), ownerId: user.id, name: body.name.trim(),
      competitors: body.competitors || [], keywords: body.keywords || []
    });
  }

  geoBrandFor(user, brandId) {
    const brand = this.store.state.geoBrands.find((item) => item.id === brandId);
    if (!brand || (brand.ownerId !== user.id && user.role !== "SUPER_ADMIN")) throw new Error("GEO brand does not exist or is not accessible");
    return brand;
  }

  async geoResult(brand, question, platform) {
    return this.geoSource.collect({ brand, question, platform });
  }

  async createGeoTask(user, body) {
    const brand = this.geoBrandFor(user, body.brandId);
    const question = body.question || `推荐与 ${brand.name} 相关的服务`;
    const platform = body.platform || this.geoSource.sourceCode || "mock-ai-search";
    const result = await this.geoResult(brand, question, platform);
    const task = this.store.insert("geoTasks", {
      id: this.store.id("geo"), ownerId: user.id, brandId: brand.id, question,
      platform, scheduleId: body.scheduleId || null, status: "COMPLETED",
      result
    });
    this.store.audit(user.id, "RUN_GEO_MONITOR", "geoTask", task.id, { brandId: brand.id, scheduleId: task.scheduleId });
    return task;
  }

  createGeoSchedule(user, body) {
    const brand = this.geoBrandFor(user, body.brandId);
    const frequency = String(body.frequency || "DAILY").toUpperCase();
    if (!["DAILY", "WEEKLY", "MONTHLY"].includes(frequency)) throw new Error("GEO schedule frequency is invalid");
    const schedule = this.store.insert("geoSchedules", {
      id: this.store.id("geoschedule"), ownerId: user.id, brandId: brand.id,
      question: body.question || `推荐与 ${brand.name} 相关的服务`, platform: body.platform || "mock-ai-search",
      frequency, status: "ACTIVE", nextRunAt: body.nextRunAt || now(), lastRunAt: null
    });
    this.store.audit(user.id, "CREATE_GEO_SCHEDULE", "geoSchedule", schedule.id, { frequency });
    return schedule;
  }

  nextGeoRun(frequency, from = Date.now()) {
    return new Date(from + ({ DAILY: 1, WEEKLY: 7, MONTHLY: 30 })[frequency] * 86400000).toISOString();
  }

  async runGeoSchedule(scheduleId, actor = null) {
    const schedule = this.store.state.geoSchedules.find((item) => item.id === scheduleId && item.status === "ACTIVE");
    if (!schedule) throw new Error("Active GEO schedule does not exist");
    if (actor && schedule.ownerId !== actor.id && actor.role !== "SUPER_ADMIN") throw new Error("GEO schedule does not exist or is not accessible");
    const owner = this.store.state.users.find((item) => item.id === schedule.ownerId);
    if (!owner) throw new Error("GEO schedule owner does not exist");
    const task = await this.createGeoTask(owner, { ...schedule, scheduleId: schedule.id });
    this.store.update("geoSchedules", schedule.id, { lastRunAt: now(), nextRunAt: this.nextGeoRun(schedule.frequency) });
    return task;
  }

  async runDueGeoSchedules(at = now()) {
    return Promise.all(this.store.state.geoSchedules
      .filter((item) => item.status === "ACTIVE" && item.nextRunAt <= at)
      .map((item) => this.runGeoSchedule(item.id)));
  }

  createGeoReport(user, body) {
    const brand = this.geoBrandFor(user, body.brandId);
    const tasks = this.store.state.geoTasks.filter((item) => item.brandId === brand.id && item.status === "COMPLETED").slice(-30);
    if (!tasks.length) throw new Error("No completed GEO monitor data is available");
    const average = (key) => Number((tasks.reduce((sum, item) => sum + item.result[key], 0) / tasks.length).toFixed(2));
    const latest = tasks.at(-1);
    const previous = tasks.at(-2);
    const recommendations = [
      latest.result.mentionRate < 0.6 ? `围绕 ${brand.keywords?.[0] || brand.name} 增加权威问答内容` : `保持 ${brand.name} 的高频品牌露出`,
      latest.result.citationRate < 0.5 ? "补充可引用的数据、案例和来源链接" : "持续维护已有高引用内容",
      "针对主要竞品差距生成对比型文章"
    ];
    return this.store.insert("geoReports", {
      id: this.store.id("georeport"), ownerId: user.id, brandId: brand.id,
      period: body.period || "WEEKLY", taskCount: tasks.length,
      metrics: {
        mentionRate: average("mentionRate"), citationRate: average("citationRate"), rank: average("rank"),
        mentionTrend: previous ? Number((latest.result.mentionRate - previous.result.mentionRate).toFixed(2)) : 0
      },
      recommendations
    });
  }

  createGeoContent(user, body) {
    const brand = this.geoBrandFor(user, body.brandId);
    const topic = body.topic || brand.keywords?.[0] || brand.name;
    return this.store.insert("geoContents", {
      id: this.store.id("geocontent"), ownerId: user.id, brandId: brand.id, type: body.type || "ARTICLE",
      title: `${brand.name}：${topic} 完整指南`,
      content: `围绕“${topic}”介绍 ${brand.name} 的能力、适用场景、真实案例与常见问题，并提供可验证的数据来源。`,
      status: "DRAFT", publications: 0
    });
  }

  geoContentFor(user, id) {
    const content = this.store.state.geoContents.find((item) => item.id === id);
    if (!content || (content.ownerId !== user.id && user.role !== "SUPER_ADMIN")) throw new Error("GEO content does not exist or is not accessible");
    return content;
  }

  publishGeoContent(user, id, body) {
    const content = this.geoContentFor(user, id);
    const platform = String(body.platform || "").trim();
    const url = String(body.url || "").trim();
    if (!platform || !/^https?:\/\//i.test(url)) throw new Error("Publication platform and valid URL are required");
    const existing = this.store.state.geoContentPublications.find((item) => item.contentId === content.id && item.url === url);
    if (existing) return existing;
    const publication = this.store.insert("geoContentPublications", {
      id: this.store.id("geopub"), ownerId: user.id, brandId: content.brandId, contentId: content.id,
      platform, url, status: "PUBLISHED", publishedAt: body.publishedAt || now(), metricsHistory: []
    });
    this.store.update("geoContents", content.id, { status: "PUBLISHED", publications: content.publications + 1, lastPublishedAt: publication.publishedAt });
    this.store.audit(user.id, "PUBLISH_GEO_CONTENT", "geoContentPublication", publication.id, { platform, url });
    return publication;
  }

  recordGeoContentMetrics(user, publicationId, body) {
    const publication = this.store.state.geoContentPublications.find((item) => item.id === publicationId && (item.ownerId === user.id || user.role === "SUPER_ADMIN"));
    if (!publication) throw new Error("GEO content publication does not exist or is not accessible");
    const metrics = {
      impressions: Math.max(0, Number(body.impressions || 0)),
      citations: Math.max(0, Number(body.citations || 0)),
      brandMentions: Math.max(0, Number(body.brandMentions || 0)),
      clicks: Math.max(0, Number(body.clicks || 0)),
      collectedAt: body.collectedAt || now()
    };
    publication.metricsHistory.push(metrics);
    publication.updatedAt = now();
    this.store.save();
    this.store.audit(user.id, "RECORD_GEO_CONTENT_METRICS", "geoContentPublication", publication.id, metrics);
    return { publicationId: publication.id, metrics, effect: this.geoPublicationEffect(publication) };
  }

  geoPublicationEffect(publication) {
    const history = publication.metricsHistory || [];
    const latest = history.at(-1) || { impressions: 0, citations: 0, brandMentions: 0, clicks: 0 };
    const baseline = history[0] || latest;
    return {
      latest,
      citationRate: latest.impressions ? Number((latest.citations / latest.impressions).toFixed(4)) : 0,
      mentionRate: latest.impressions ? Number((latest.brandMentions / latest.impressions).toFixed(4)) : 0,
      clickRate: latest.impressions ? Number((latest.clicks / latest.impressions).toFixed(4)) : 0,
      citationGrowth: latest.citations - baseline.citations,
      mentionGrowth: latest.brandMentions - baseline.brandMentions
    };
  }

  geoOverview(user) {
    const brands = this.store.state.geoBrands.filter((item) => item.ownerId === user.id || user.role === "SUPER_ADMIN");
    const brandIds = new Set(brands.map((item) => item.id));
    return {
      brands,
      tasks: this.store.state.geoTasks.filter((item) => brandIds.has(item.brandId)),
      schedules: this.store.state.geoSchedules.filter((item) => brandIds.has(item.brandId)),
      reports: this.store.state.geoReports.filter((item) => brandIds.has(item.brandId)),
      contents: this.store.state.geoContents.filter((item) => brandIds.has(item.brandId)),
      publications: this.store.state.geoContentPublications.filter((item) => brandIds.has(item.brandId)).map((item) => ({ ...item, effect: this.geoPublicationEffect(item) }))
    };
  }

  createEnterprise(user, body) {
    if (!body.name?.trim()) throw new Error("企业名称不能为空");
    if (this.store.state.enterpriseMembers.some((item) => item.userId === user.id && item.status === "ACTIVE")) throw new Error("当前用户已加入企业");
    const enterprise = this.store.insert("enterprises", {
      id: this.store.id("enterprise"), name: body.name.trim(), ownerId: user.id,
      status: "ACTIVE", totalQuota: Number(body.totalQuota || 10000), availableQuota: Number(body.totalQuota || 10000)
    });
    this.store.insert("enterpriseMembers", {
      id: this.store.id("emember"), enterpriseId: enterprise.id, userId: user.id,
      role: "ENTERPRISE_ADMIN", status: "ACTIVE", quotaLimit: 0, quotaUsed: 0
    });
    user.role = user.role === "MEMBER" ? "ENTERPRISE_ADMIN" : user.role;
    this.store.save();
    this.store.audit(user.id, "CREATE_ENTERPRISE", "enterprise", enterprise.id);
    return enterprise;
  }

  enterpriseFor(user) {
    const membership = this.store.state.enterpriseMembers.find((item) => item.userId === user.id && item.status === "ACTIVE");
    if (!membership) return null;
    const enterprise = this.store.state.enterprises.find((item) => item.id === membership.enterpriseId);
    return enterprise ? {
      ...enterprise,
      membership,
      members: this.enterpriseMembers(user, enterprise.id),
      quotaTransactions: this.store.state.enterpriseQuotaTransactions.filter((item) => item.enterpriseId === enterprise.id)
    } : null;
  }

  enterpriseMembers(user, enterpriseId) {
    const actor = this.store.state.enterpriseMembers.find((item) => item.enterpriseId === enterpriseId && item.userId === user.id && item.status === "ACTIVE");
    if (!actor) throw new Error("无权访问企业成员");
    return this.store.state.enterpriseMembers.filter((item) => item.enterpriseId === enterpriseId).map((item) => ({
      ...item, user: this.publicUser(this.store.state.users.find((entry) => entry.id === item.userId))
    }));
  }

  addEnterpriseMember(user, body) {
    const view = this.enterpriseFor(user);
    if (!view || view.membership.role !== "ENTERPRISE_ADMIN") throw new Error("仅企业管理员可添加成员");
    const enterprise = this.store.state.enterprises.find((item) => item.id === view.id);
    const target = this.store.state.users.find((item) => item.email === String(body.email).toLowerCase());
    if (!target) throw new Error("用户不存在");
    if (this.store.state.enterpriseMembers.some((item) => item.userId === target.id && item.status === "ACTIVE")) throw new Error("用户已加入企业");
    const quotaLimit = Math.max(0, Number(body.quotaLimit || 0));
    if (quotaLimit > enterprise.availableQuota) throw new Error("企业可分配额度不足");
    enterprise.availableQuota -= quotaLimit;
    const membership = this.store.insert("enterpriseMembers", {
      id: this.store.id("emember"), enterpriseId: enterprise.id, userId: target.id,
      role: "ENTERPRISE_MEMBER", status: "ACTIVE", quotaLimit, quotaUsed: 0
    });
    target.role = target.role === "MEMBER" ? "ENTERPRISE_MEMBER" : target.role;
    this.store.save();
    this.store.audit(user.id, "ADD_ENTERPRISE_MEMBER", "enterpriseMember", membership.id, { quotaLimit });
    return membership;
  }

  allocateEnterpriseQuota(user, memberId, amount) {
    const view = this.enterpriseFor(user);
    if (!view || view.membership.role !== "ENTERPRISE_ADMIN") throw new Error("仅企业管理员可分配额度");
    const enterprise = this.store.state.enterprises.find((item) => item.id === view.id);
    const member = this.store.state.enterpriseMembers.find((item) => item.id === memberId && item.enterpriseId === enterprise.id);
    const quota = Number(amount);
    if (!member || !Number.isInteger(quota) || quota <= 0 || quota > enterprise.availableQuota) throw new Error("额度分配参数无效或企业额度不足");
    member.quotaLimit += quota;
    enterprise.availableQuota -= quota;
    const tx = this.store.insert("enterpriseQuotaTransactions", {
      id: this.store.id("eqtx"), enterpriseId: enterprise.id, memberId, type: "ALLOCATE",
      amount: quota, availableAfter: enterprise.availableQuota, actorId: user.id
    });
    this.store.save();
    return tx;
  }

  createWithdrawal(user, amount) {
    const channel = this.store.state.channelAgents.find((item) => item.userId === user.id);
    if (!channel) throw new Error("当前用户不是代理商");
    const available = this.store.state.commissions.filter((item) => item.agentId === channel.id && item.status === "AVAILABLE").reduce((sum, item) => sum + item.amount, 0);
    if (!Number.isInteger(amount) || amount <= 0 || amount > available) throw new Error("可提现佣金不足");
    return this.store.insert("withdrawals", { id: this.store.id("withdraw"), agentId: channel.id, amount, status: "PENDING" });
  }

  releaseCommissions(user, orderId) {
    if (user.role !== "SUPER_ADMIN") throw new Error("仅平台管理员可释放佣金");
    const order = this.store.state.orders.find((item) => item.id === orderId && item.status === "PAID");
    if (!order) throw new Error("订单不存在或状态不允许释放佣金");
    const commissions = this.store.state.commissions.filter((item) => item.orderId === order.id && item.status === "FROZEN");
    commissions.forEach((item) => this.store.update("commissions", item.id, { status: "AVAILABLE", availableAt: now() }));
    return commissions;
  }

  approveWithdrawal(user, id) {
    if (user.role !== "SUPER_ADMIN") throw new Error("仅平台管理员可审核提现");
    const withdrawal = this.store.state.withdrawals.find((item) => item.id === id && item.status === "PENDING");
    if (!withdrawal) throw new Error("提现申请不存在或状态不允许审核");
    const available = this.store.state.commissions
      .filter((item) => item.agentId === withdrawal.agentId && item.status === "AVAILABLE")
      .sort((a, b) => a.createdAt.localeCompare(b.createdAt));
    let remaining = withdrawal.amount;
    for (const commission of available) {
      if (remaining <= 0) break;
      if (commission.amount > remaining) throw new Error("当前演示版本仅支持按完整佣金明细提现");
      remaining -= commission.amount;
    }
    if (remaining !== 0) throw new Error("可提现佣金不足或金额未匹配完整佣金明细");
    let paid = withdrawal.amount;
    const settledCommissionIds = [];
    for (const commission of available) {
      if (paid <= 0) break;
      if (commission.amount <= paid) {
        paid -= commission.amount;
        settledCommissionIds.push(commission.id);
        this.store.update("commissions", commission.id, { status: "SETTLED", settledAt: now() });
      }
    }
    this.store.update("withdrawals", withdrawal.id, { status: "APPROVED", reviewedBy: user.id, reviewedAt: now() });
    const statement = this.store.insert("settlementStatements", {
      id: this.store.id("settlement"), statementNumber: `ST-${Date.now()}-${withdrawal.id.split("_").pop()}`,
      agentId: withdrawal.agentId, withdrawalId: withdrawal.id, amount: withdrawal.amount,
      commissionIds: settledCommissionIds, status: "PAID", periodStart: available[0]?.createdAt || withdrawal.createdAt,
      periodEnd: now(), reviewedBy: user.id, paidAt: now()
    });
    this.store.update("withdrawals", withdrawal.id, { settlementStatementId: statement.id });
    this.store.audit(user.id, "APPROVE_WITHDRAWAL", "withdrawal", withdrawal.id, { settlementStatementId: statement.id });
    return withdrawal;
  }

  settlementStatementsVisible(user) {
    if (user.role === "SUPER_ADMIN") return this.store.state.settlementStatements;
    const ids = new Set(this.channelVisible(user).map((item) => item.id));
    return this.store.state.settlementStatements.filter((item) => ids.has(item.agentId));
  }

  adminOverview(user) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can access operations data");
    const completedTasks = this.store.state.generationTasks.filter((item) => ["SUCCEEDED", "FAILED"].includes(item.status));
    const modelUsage = Object.values(this.store.state.generationTasks.reduce((result, item) => {
      const key = item.model || "unknown";
      result[key] ||= { model: key, tasks: 0, points: 0, succeeded: 0 };
      result[key].tasks += 1;
      result[key].points += item.pointCost || 0;
      if (item.status === "SUCCEEDED") result[key].succeeded += 1;
      return result;
    }, {}));
    return {
      metrics: {
        users: this.store.state.users.length,
        activeUsers: this.store.state.users.filter((item) => item.status === "ACTIVE").length,
        paidRevenue: this.store.state.orders.filter((item) => item.status === "PAID").reduce((sum, item) => sum + item.amount, 0),
        generationTasks: this.store.state.generationTasks.length,
        generationSuccessRate: completedTasks.length ? Number((completedTasks.filter((item) => item.status === "SUCCEEDED").length / completedTasks.length).toFixed(2)) : 0,
        pendingWithdrawals: this.store.state.withdrawals.filter((item) => item.status === "PENDING").length,
        rejectedModeration: this.store.state.moderationLogs.filter((item) => item.status === "REJECTED").length
      },
      users: this.store.state.users.map((item) => this.publicUser(item)),
      modelUsage,
      providerUsage: Object.values(this.store.state.modelCallLogs.reduce((result, item) => {
        result[item.providerCode] ||= { providerCode: item.providerCode, calls: 0, succeeded: 0, costCents: 0, latencyMs: 0 };
        result[item.providerCode].calls += 1;
        result[item.providerCode].costCents += item.costCents || 0;
        result[item.providerCode].latencyMs += item.latencyMs || 0;
        if (item.status === "SUCCEEDED") result[item.providerCode].succeeded += 1;
        return result;
      }, {})).map((item) => ({ ...item, averageLatencyMs: item.calls ? Math.round(item.latencyMs / item.calls) : 0 })),
      recentAuditLogs: this.store.state.auditLogs.slice(-20).reverse(),
      moderationLogs: this.store.state.moderationLogs.slice(-20).reverse()
    };
  }

  updateUserStatus(user, userId, status) {
    if (user.role !== "SUPER_ADMIN") throw new Error("Only platform administrators can manage users");
    const normalized = String(status || "").toUpperCase();
    if (!["ACTIVE", "SUSPENDED"].includes(normalized)) throw new Error("User status is invalid");
    if (user.id === userId && normalized === "SUSPENDED") throw new Error("Administrator cannot suspend the current account");
    const target = this.store.update("users", userId, { status: normalized });
    if (!target) throw new Error("User does not exist");
    this.store.audit(user.id, "UPDATE_USER_STATUS", "user", target.id, { status: normalized });
    return this.publicUser(target);
  }

  dashboard(user) {
    const own = (item) => user.role === "SUPER_ADMIN" || item.userId === user.id || item.ownerId === user.id;
    return {
      user: this.publicUser(user),
      account: this.account(user.id),
      plan: this.store.state.plans.find((item) => item.id === user.planId),
      metrics: {
        tasks: this.store.state.generationTasks.filter(own).length,
        assets: this.store.state.assets.filter(own).length,
        presentations: this.store.state.presentations.filter(own).length,
        agents: this.store.state.agents.filter(own).length,
        geoTasks: this.store.state.geoTasks.filter(own).length,
        orders: this.store.state.orders.filter(own).length,
        enterpriseMembers: this.enterpriseFor(user)?.members.length || 0
      }
    };
  }

  listFor(user, collection) {
    const admin = user.role === "SUPER_ADMIN";
    const ownerFields = { generationTasks: "userId", assets: "userId", orders: "userId", presentations: "userId", agents: "ownerId", knowledgeBases: "ownerId", knowledgeDocuments: "ownerId", geoBrands: "ownerId", geoTasks: "ownerId", geoSchedules: "ownerId", geoReports: "ownerId", geoContents: "ownerId", geoContentPublications: "ownerId" };
    const field = ownerFields[collection];
    const items = (!field || admin) ? this.store.state[collection] : this.store.state[collection].filter((item) => item[field] === user.id);
    return collection === "assets" ? items.filter((item) => !item.deletedAt) : items;
  }
}
