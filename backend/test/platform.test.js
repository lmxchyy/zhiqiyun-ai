import test from "node:test";
import assert from "node:assert/strict";
import { createStore } from "../src/store.js";
import { Platform } from "../src/platform.js";
import { buildPptx } from "../src/pptx.js";
import { buildPdf } from "../src/pdf.js";
import { ModelGateway, ProviderError } from "../src/model-gateway.js";
import { PaymentGateway } from "../src/payment-gateway.js";
import { GeoSource } from "../src/geo-source.js";

test("generation estimate returns the expected point cost", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  assert.deepEqual(platform.generationEstimate(user, "TEXT_TO_VIDEO", { count: 2, model: "mock-standard" }), {
    type: "TEXT_TO_VIDEO", model: "mock-standard", count: 2, pointCost: 160
  });
});

test("generation idempotency key prevents duplicate tasks and point charges", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store, { taskQueue: { publish: async () => {} } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const before = platform.account(user.id).available;
  const first = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "idempotent task", idempotencyKey: "same-key" });
  const duplicate = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "changed prompt is ignored", idempotencyKey: "same-key" });
  assert.equal(duplicate.id, first.id);
  assert.equal(store.state.generationTasks.length, 1);
  assert.equal(platform.account(user.id).available, before - 10);
});

test("failed tasks can be retried and preserve source task traceability", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store, { taskQueue: { publish: async () => {} } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const failed = store.insert("generationTasks", {
    id: store.id("task"), userId: user.id, type: "TEXT_TO_IMAGE", prompt: "retry source",
    params: { count: 1 }, model: "mock-standard", status: "FAILED", progress: 100,
    pointCost: 10, billingSource: "PERSONAL", resultIds: [], error: "provider error"
  });
  const retried = platform.retryGeneration(user, failed.id, { idempotencyKey: "retry-key" });
  assert.equal(retried.status, "QUEUED");
  assert.equal(retried.retryOfTaskId, failed.id);
  assert.equal(retried.prompt, failed.prompt);
  assert.throws(() => platform.retryGeneration(user, retried.id), /Only failed/);
});

test("asset favorite, regeneration and logical deletion preserve ownership rules", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store, { taskQueue: { publish: async () => {} } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const task = store.insert("generationTasks", {
    id: store.id("task"), userId: user.id, type: "TEXT_TO_IMAGE", prompt: "asset source",
    params: { count: 1 }, model: "mock-standard", status: "SUCCEEDED", progress: 100,
    pointCost: 10, billingSource: "PERSONAL", resultIds: [], error: null
  });
  const asset = store.insert("assets", {
    id: store.id("asset"), userId: user.id, taskId: task.id, name: "generated asset",
    mediaType: "image", url: "/asset.png", favorite: false, metadata: { prompt: task.prompt }
  });
  platform.setAssetFavorite(user, asset.id, true);
  assert.equal(asset.favorite, true);
  const regenerated = platform.regenerateAsset(user, asset.id, { idempotencyKey: "asset-regenerate" });
  assert.equal(regenerated.retryOfTaskId, task.id);
  platform.deleteAsset(user, asset.id);
  assert.equal(platform.listFor(user, "assets").length, 0);
  assert.throws(() => platform.assetFor(user, asset.id), /does not exist/);
});

test("free members can only view and use free models", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  assert.deepEqual(platform.availableModels(user).map((item) => item.code), ["mock-standard"]);
  assert.throws(
    () => platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "premium model permission", model: "mock-quality" }),
    /not available for the current membership plan/
  );
});

test("paid members can view and use quality models", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const order = platform.createOrder(user, "plan_month");
  platform.payOrder(user, order.id, "paid_model_access");
  assert.deepEqual(platform.availableModels(user).map((item) => item.code), ["mock-standard", "mock-quality"]);
  const task = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "quality model task", model: "mock-quality" });
  assert.equal(task.model, "mock-quality");
});

test("free members cannot exceed the generation concurrency limit", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store, { taskQueue: { publish: async () => {} } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "first queued task" });
  assert.throws(
    () => platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "second queued task" }),
    /concurrency limit has been reached/
  );
});

test("用户取消排队任务后自动退还冻结积分", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store, { taskQueue: { publish: async () => {} } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const before = platform.account(user.id).available;
  const task = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "cancel queued task" });
  assert.equal(platform.account(user.id).available, before - task.pointCost);
  platform.cancelGeneration(user, task.id);
  assert.equal(task.status, "CANCELLED");
  assert.equal(platform.account(user.id).available, before);
  assert.equal(platform.account(user.id).frozen, 0);
  assert.throws(() => platform.cancelGeneration(user, task.id), /Only queued or retrying/);
});

test("available models can be filtered by generation capability", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  assert.deepEqual(platform.availableModels(user, "TEXT_TO_VIDEO").map((item) => item.code), ["mock-standard"]);
  assert.deepEqual(platform.availableModels(user, "UNKNOWN_CAPABILITY"), []);
});

test("OpenAI 兼容供应商模型会出现在文生图模型列表", () => {
  const store = createStore({ persist: false });
  const modelGateway = new ModelGateway({ kind: "openai", openAiApiKey: "test-key", openAiImageModel: "gpt-image-2" });
  const platform = new Platform(store, { taskQueue: { publish: async () => {} }, modelGateway });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const models = platform.availableModels(user, "TEXT_TO_IMAGE").map((item) => item.code);
  assert.ok(models.includes("gpt-image-2"));
  assert.equal(platform.generationEstimate(user, "TEXT_TO_IMAGE", { model: "gpt-image-2" }).model, "gpt-image-2");
  const task = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "真实图像模型", model: "gpt-image-2" });
  assert.equal(task.model, "gpt-image-2");
});

test("管理员可配置模型供应商、会员权限和按能力积分价格", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const provider = platform.createModelProvider(admin, { code: "image-cloud", name: "图像云", baseUrl: "https://api.example.com" });
  const model = platform.createModelDefinition(admin, {
    providerCode: provider.code, code: "image-pro", name: "图像专业版",
    capabilities: ["TEXT_TO_IMAGE"], tier: "FREE", pointCosts: { TEXT_TO_IMAGE: 23 }
  });
  assert.equal(platform.generationEstimate(user, "TEXT_TO_IMAGE", { model: model.code, count: 2 }).pointCost, 46);
  assert.ok(platform.availableModels(user, "TEXT_TO_IMAGE").some((item) => item.code === model.code));
  assert.equal(platform.adminModelConfig(admin).pricingRules[0].pointCost, 23);
  platform.updateModelDefinitionStatus(admin, model.id, "DISABLED");
  assert.ok(!platform.availableModels(user, "TEXT_TO_IMAGE").some((item) => item.code === model.code));
  assert.throws(() => platform.createModelProvider(user, { code: "no", name: "no" }), /Only platform administrators/);
});

const setup = () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const login = platform.login("demo@xianzhi.ai", "Demo123!");
  const user = platform.authenticate(login.token);
  return { store, platform, user };
};

test("登录认证并拒绝错误密码", () => {
  const { platform } = setup();
  assert.equal(platform.login("demo@xianzhi.ai", "Demo123!").user.role, "MEMBER");
  assert.throws(() => platform.login("demo@xianzhi.ai", "wrong"), /邮箱或密码错误/);
});

test("刷新令牌轮换访问令牌且退出后撤销会话", () => {
  const store = createStore({ persist: false });
  const platform = new Platform(store);
  const login = platform.login("demo@xianzhi.ai", "Demo123!");
  const refreshed = platform.refreshSession(login.refreshToken);
  assert.notEqual(refreshed.token, login.token);
  assert.notEqual(refreshed.refreshToken, login.refreshToken);
  assert.equal(platform.authenticate(login.token), null);
  const user = platform.authenticate(refreshed.token);
  assert.equal(user.email, "demo@xianzhi.ai");
  assert.throws(() => platform.refreshSession(login.refreshToken), /invalid or expired/);
  assert.deepEqual(platform.logout(user, refreshed.token), { revoked: true });
  assert.equal(platform.authenticate(refreshed.token), null);
});

test("代理商邀请码注册向新用户和邀请人发放邀请奖励", () => {
  const { platform, store } = setup();
  const agent = store.state.users.find((item) => item.email === "agent1@xianzhi.ai");
  const before = platform.account(agent.id).available;
  const created = platform.register({ email: "invited@example.com", password: "Invited123!", name: "邀请用户", inviteCode: "EAST001" });
  assert.equal(created.referredBy, agent.id);
  assert.equal(platform.account(created.id).available, 120);
  assert.equal(platform.account(agent.id).available, before + 50);
});

test("优惠券可领取、抵扣订单并在退款后恢复", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const coupon = platform.createCoupon(admin, { code: "SAVE10", name: "减十元", type: "FIXED", value: 1000, maxUses: 10 });
  platform.claimCoupon(user, coupon.code);
  const order = platform.createOrder(user, "plan_month", coupon.code);
  assert.equal(order.originalAmount, 9900);
  assert.equal(order.discountAmount, 1000);
  assert.equal(order.amount, 8900);
  platform.payOrder(user, order.id, "coupon_payment");
  assert.equal(platform.couponsFor(user)[0].status, "USED");
  assert.equal(coupon.usesCount, 1);
  platform.refundOrder(admin, order.id, "coupon_refund");
  assert.equal(platform.couponsFor(user)[0].status, "AVAILABLE");
  assert.equal(coupon.usesCount, 0);
});

test("代理商可创建积分兑换码且同一用户只能兑换一次", () => {
  const { platform, store, user } = setup();
  const agent = store.state.users.find((item) => item.email === "agent1@xianzhi.ai");
  const before = platform.account(user.id).available;
  const redemption = platform.createRedemptionCode(agent, { code: "POINTS100", type: "POINTS", points: 100, maxUses: 2 });
  platform.redeemCode(user, redemption.code);
  assert.equal(platform.account(user.id).available, before + 100);
  assert.equal(redemption.usesCount, 1);
  assert.throws(() => platform.redeemCode(user, redemption.code), /already been used/);
  assert.deepEqual(platform.redemptionCodesFor(agent).map((item) => item.id), [redemption.id]);
});

test("生成任务冻结积分，成功后结算冻结积分", async () => {
  const { platform, user } = setup();
  const before = platform.account(user.id).available;
  const task = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "未来城市", count: 1 });
  assert.equal(platform.account(user.id).available, before - 10);
  assert.equal(platform.account(user.id).frozen, 10);
  await new Promise((resolve) => setTimeout(resolve, 800));
  assert.equal(platform.taskFor(user, task.id).status, "SUCCEEDED");
  assert.equal(platform.account(user.id).frozen, 0);
});

test("生成失败自动退还积分", async () => {
  const { platform, user } = setup();
  const before = platform.account(user.id).available;
  const task = platform.createGeneration(user, "TEXT_TO_VIDEO", { prompt: "[fail] 模拟失败" });
  await new Promise((resolve) => setTimeout(resolve, 800));
  assert.equal(platform.taskFor(user, task.id).status, "FAILED");
  assert.equal(platform.account(user.id).available, before);
  assert.equal(platform.account(user.id).frozen, 0);
});

test("重复支付事件不会重复发放积分", () => {
  const { platform, user } = setup();
  const order = platform.createOrder(user, "plan_month");
  platform.payOrder(user, order.id, "evt_same");
  const afterFirst = platform.account(user.id).available;
  platform.payOrder(user, order.id, "evt_same");
  assert.equal(platform.account(user.id).available, afterFirst);
});

test("微信和支付宝支付回调验证签名、金额并保持幂等", () => {
  const { platform, store, user } = setup();
  const gateway = new PaymentGateway({ defaultSecret: "callback-secret" });
  const order = platform.createOrder(user, "plan_month");
  const payload = { eventId: "wechat_evt_1", orderId: order.id, amount: order.amount, status: "SUCCESS", providerTransactionId: "wx_123" };
  assert.throws(() => platform.handlePaymentCallback("wechat", payload, "bad-signature", gateway), /signature is invalid/);
  const badAmount = { ...payload, eventId: "wechat_evt_bad", amount: order.amount + 1 };
  assert.throws(() => platform.handlePaymentCallback("wechat", badAmount, gateway.sign("wechat", badAmount), gateway), /amount does not match/);
  const before = platform.account(user.id).available;
  const payment = platform.handlePaymentCallback("wechat", payload, gateway.sign("wechat", payload), gateway);
  const after = platform.account(user.id).available;
  const duplicate = platform.handlePaymentCallback("wechat", payload, gateway.sign("wechat", payload), gateway);
  assert.equal(payment.id, duplicate.id);
  assert.equal(payment.status, "SUCCEEDED");
  assert.equal(platform.account(user.id).available, after);
  assert.equal(after, before + order.snapshot.points);
  assert.equal(store.state.payments.length, 1);
});

test("已支付订单可申请发票并由管理员开具", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const order = platform.createOrder(user, "plan_month");
  assert.throws(() => platform.requestInvoice(user, { orderId: order.id, title: "先知科技" }), /Paid order/);
  platform.payOrder(user, order.id, "invoice_payment");
  const invoice = platform.requestInvoice(user, { orderId: order.id, title: "先知科技", taxNumber: "91330000TEST" });
  const duplicate = platform.requestInvoice(user, { orderId: order.id, title: "重复申请" });
  assert.equal(invoice.id, duplicate.id);
  assert.equal(platform.invoiceVisible(user).length, 1);
  assert.throws(() => platform.issueInvoice(user, invoice.id), /Only platform administrators/);
  platform.issueInvoice(admin, invoice.id);
  assert.equal(invoice.status, "ISSUED");
  assert.match(invoice.invoiceNumber, /^XZ-/);
});

test("普通用户不能读取其他用户生成任务", () => {
  const { platform, user } = setup();
  const other = platform.createUser({ email: "other@example.com", password: "Other123!", name: "其他用户" });
  const otherUser = platform.store.state.users.find((item) => item.id === other.id);
  const task = platform.createGeneration(otherUser, "TEXT_TO_IMAGE", { prompt: "私有任务" });
  assert.throws(() => platform.taskFor(user, task.id), /无权访问/);
});

test("订单退款撤回套餐积分并回退佣金", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const agent = store.state.users.find((item) => item.role === "AGENT_L1");
  platform.bindCustomer(agent, { email: user.email });
  const order = platform.createOrder(user, "plan_month");
  platform.payOrder(user, order.id, "pay_refund");
  assert.equal(store.state.commissions[0].status, "FROZEN");
  platform.refundOrder(admin, order.id, "refund_once");
  assert.equal(order.status, "REFUNDED");
  assert.equal(store.state.commissions[0].status, "REVERSED");
  assert.equal(user.planId, "plan_free");
  const available = platform.account(user.id).available;
  platform.refundOrder(admin, order.id, "refund_once");
  assert.equal(platform.account(user.id).available, available);
});

test("二级代理订单同时产生销售佣金和一级管理奖励", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const l1User = store.state.users.find((item) => item.role === "AGENT_L1");
  const l2Channel = platform.createChannelAgent(l1User, {
    email: "agent2@example.com", password: "Agent2123!", name: "二级代理"
  });
  platform.approveChannelAgent(admin, l2Channel.id);
  const l2User = store.state.users.find((item) => item.id === l2Channel.userId);
  platform.bindCustomer(l2User, { email: user.email });
  const order = platform.createOrder(user, "plan_month");
  platform.payOrder(user, order.id, "pay_l2");
  const commissions = store.state.commissions.filter((item) => item.orderId === order.id);
  assert.equal(commissions.length, 2);
  assert.deepEqual(commissions.map((item) => item.rate).sort(), [0.1, 0.2]);
});

test("代理商业绩看板汇总团队客户订单收入并生成排行榜快照", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const l1User = store.state.users.find((item) => item.role === "AGENT_L1");
  const l2Channel = platform.createChannelAgent(l1User, {
    email: "performance-l2@example.com", password: "Agent2123!", name: "业绩二级代理"
  });
  platform.approveChannelAgent(admin, l2Channel.id);
  const l2User = store.state.users.find((item) => item.id === l2Channel.userId);
  platform.bindCustomer(l2User, { email: user.email });
  const order = platform.createOrder(user, "plan_month");
  platform.payOrder(user, order.id, "performance_payment");
  const rows = platform.channelPerformance(admin);
  const l1 = rows.find((item) => item.userId === l1User.id);
  const l2 = rows.find((item) => item.userId === l2User.id);
  assert.equal(l1.customers, 1);
  assert.equal(l1.directAgents, 1);
  assert.equal(l1.revenue, order.amount);
  assert.equal(l2.customers, 1);
  assert.equal(l2.revenue, order.amount);
  assert.equal(rows[0].rank, 1);
  const snapshot = platform.createChannelPerformanceSnapshot(admin, "MONTHLY");
  assert.equal(snapshot.period, "MONTHLY");
  assert.equal(snapshot.totals.paidOrders, rows.reduce((sum, item) => sum + item.paidOrders, 0));
  assert.equal(platform.channelPerformanceSnapshots(admin).length, 1);
  assert.throws(() => platform.createChannelPerformanceSnapshot(l1User), /Only platform administrators/);
});

test("佣金释放后代理商可申请完整佣金提现并由管理员审核", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  const agent = store.state.users.find((item) => item.role === "AGENT_L1");
  platform.bindCustomer(agent, { email: user.email });
  const order = platform.createOrder(user, "plan_month");
  platform.payOrder(user, order.id, "pay_withdraw");
  platform.releaseCommissions(admin, order.id);
  const commission = store.state.commissions.find((item) => item.orderId === order.id);
  const withdrawal = platform.createWithdrawal(agent, commission.amount);
  platform.approveWithdrawal(admin, withdrawal.id);
  assert.equal(withdrawal.status, "APPROVED");
  assert.equal(commission.status, "SETTLED");
  assert.equal(store.state.settlementStatements.length, 1);
  assert.equal(withdrawal.settlementStatementId, store.state.settlementStatements[0].id);
  assert.deepEqual(store.state.settlementStatements[0].commissionIds, [commission.id]);
  assert.equal(platform.settlementStatementsVisible(agent).length, 1);
});

test("智能体必须发布后才能调用并记录成本", () => {
  const { platform, user } = setup();
  const agent = platform.createAgent(user, { name: "测试助手" });
  assert.throws(() => platform.callAgent(user, agent.id, "你好"), /未发布/);
  platform.publishAgent(user, agent.id);
  const call = platform.callAgent(user, agent.id, "你好");
  assert.match(call.output, /测试助手/);
  assert.equal(call.cost, 1);
});

test("已发布智能体可通过分享链接公开调用并汇总反馈统计", () => {
  const { platform, user } = setup();
  const agent = platform.createAgent(user, { name: "分享助手" });
  platform.publishAgent(user, agent.id);
  const share = platform.createAgentShare(user, agent.id);
  const duplicate = platform.createAgentShare(user, agent.id);
  assert.equal(share.id, duplicate.id);
  const publicResult = platform.publicCallAgent(share.token, "公开问题");
  assert.match(publicResult.output, /公开问题/);
  const authenticatedCall = platform.callAgent(user, agent.id, "登录用户问题");
  platform.submitAgentFeedback(user, authenticatedCall.id, { rating: 5, comment: "很好" });
  const stats = platform.agentStats(user, agent.id);
  assert.equal(stats.calls, 2);
  assert.equal(stats.publicCalls, 1);
  assert.equal(stats.feedbackCount, 1);
  assert.equal(stats.averageRating, 5);
  assert.equal(stats.share.id, share.id);
  assert.throws(() => platform.publicCallAgent("invalid-token", "问题"), /invalid or unavailable/);
});

test("PPT 项目可导出为包含演示文稿结构的 PPTX", () => {
  const { platform, user } = setup();
  const presentation = platform.createPresentation(user, { topic: "先知 AI 商业计划" });
  const pptx = buildPptx(presentation);
  assert.equal(pptx.subarray(0, 2).toString(), "PK");
  assert.ok(pptx.includes(Buffer.from("ppt/presentation.xml")));
  assert.ok(pptx.includes(Buffer.from("先知 AI 商业计划")));
});

test("PPT 页面可编辑排序、重新生成大纲并导出 PDF", () => {
  const { platform, user } = setup();
  const presentation = platform.createPresentation(user, { topic: "可编辑商业方案" });
  platform.updatePresentation(user, presentation.id, {
    slides: [
      { title: "第二页", content: "内容二", notes: "备注二" },
      { title: "第一页", content: "内容一", notes: "备注一" }
    ]
  });
  assert.deepEqual(presentation.slides.map((slide) => slide.index), [1, 2]);
  assert.equal(presentation.slides[0].notes, "备注二");
  platform.regeneratePresentationOutline(user, presentation.id, { outline: ["封面", "结论"] });
  assert.deepEqual(presentation.slides.map((slide) => slide.title), ["封面", "结论"]);
  const pdf = buildPdf(presentation);
  assert.equal(pdf.subarray(0, 8).toString(), "%PDF-1.4");
  assert.ok(pdf.includes(Buffer.from("/Type /Page")));
});

test("配置任务队列后 API 只发布任务，Worker 独立执行", async () => {
  const store = createStore({ persist: false });
  const published = [];
  const platform = new Platform(store, { taskQueue: { publish: async (taskId) => published.push(taskId) } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const reference = store.insert("assets", { id: store.id("asset"), userId: user.id, taskId: null, name: "参考图", mediaType: "image", url: "/reference.png", favorite: false, metadata: {} });
  const task = platform.createGeneration(user, "IMAGE_TO_VIDEO", { prompt: "队列视频任务", referenceAssetId: reference.id });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(published, [task.id]);
  assert.equal(task.status, "QUEUED");
  await platform.executeGeneration(task.id);
  assert.equal(task.status, "SUCCEEDED");
  assert.equal(store.state.assets.find((item) => item.taskId === task.id).metadata.processedBy, "worker");
});

test("Worker 将生成资产写入对象存储并保存对象键", async () => {
  const store = createStore({ persist: false });
  const uploads = [];
  const objectStore = { enabled: true, put: async (key, content, contentType) => uploads.push({ key, content, contentType }) };
  const platform = new Platform(store, { objectStore });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const task = store.insert("generationTasks", {
    id: store.id("task"), userId: user.id, type: "TEXT_TO_IMAGE", prompt: "对象存储测试",
    params: {}, model: "mock-standard", status: "QUEUED", progress: 0, pointCost: 10, resultIds: [], error: null
  });
  platform.pointTransaction(user.id, "FREEZE", -10, 10, "GENERATION_TASK", task.id);
  await platform.executeGeneration(task.id);
  assert.equal(uploads.length, 1);
  assert.equal(store.state.assets[0].metadata.storage, "minio");
  assert.match(store.state.assets[0].storageKey, /\.svg$/);
});

test("参考图生成必须使用当前用户可访问的图片资产", () => {
  const { platform, store, user } = setup();
  assert.throws(() => platform.createGeneration(user, "IMAGE_TO_IMAGE", { prompt: "缺少参考图" }), /Reference image asset is required/);
  const other = platform.createUser({ email: "reference-owner@example.com", password: "Owner123!", name: "参考图所有者" });
  const otherAsset = store.insert("assets", { id: store.id("asset"), userId: other.id, taskId: null, name: "他人图片", mediaType: "image", url: "/private.png", favorite: false, metadata: {} });
  assert.throws(() => platform.createGeneration(user, "IMAGE_TO_VIDEO", { prompt: "越权参考图", referenceAssetId: otherAsset.id }), /must be accessible/);
  const ownAsset = store.insert("assets", { id: store.id("asset"), userId: user.id, taskId: null, name: "我的图片", mediaType: "image", url: "/mine.png", favorite: false, metadata: {} });
  const task = platform.createGeneration(user, "IMAGE_TO_IMAGE", { prompt: "基于我的图片生成", referenceAssetId: ownAsset.id });
  assert.equal(task.params.referenceAssetId, ownAsset.id);
});

test("模型网关记录供应商调用、成本和可重试失败", async () => {
  const store = createStore({ persist: false });
  const uploads = [];
  let calls = 0;
  const modelGateway = {
    providerCode: "test-provider",
    generate: async () => {
      calls += 1;
      if (calls === 1) throw new ProviderError("temporary limit", { retryable: true, status: 429 });
      return {
        providerCode: "test-provider", providerRequestId: "provider_123",
        content: Buffer.from("image"), contentType: "image/png", extension: "png",
        costCents: 12, responseSnapshot: { bytes: 5 }
      };
    }
  };
  const platform = new Platform(store, {
    taskQueue: { publish: async () => {} }, modelGateway,
    objectStore: { enabled: true, put: async (...args) => uploads.push(args) }
  });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const task = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "供应商重试测试", maxAttempts: 3 });
  await platform.executeGatewayGeneration(task.id);
  assert.equal(task.status, "SUCCEEDED");
  assert.equal(task.attemptCount, 2);
  assert.equal(store.state.generationAttempts.length, 2);
  assert.deepEqual(store.state.modelCallLogs.map((item) => item.status), ["RETRYING", "SUCCEEDED"]);
  assert.equal(store.state.modelCallLogs.at(-1).costCents, 12);
  assert.equal(uploads.length, 1);
});

test("模型网关最终失败后才退款", async () => {
  const store = createStore({ persist: false });
  const modelGateway = { providerCode: "failed-provider", generate: async () => { throw new ProviderError("provider down", { retryable: true }); } };
  const platform = new Platform(store, { taskQueue: { publish: async () => {} }, modelGateway });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const before = platform.account(user.id).available;
  const task = platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "供应商失败测试", maxAttempts: 2 });
  await platform.executeGatewayGeneration(task.id);
  assert.equal(task.status, "FAILED");
  assert.equal(task.attemptCount, 2);
  assert.equal(platform.account(user.id).available, before);
  assert.equal(store.state.modelCallLogs.length, 2);
});

test("HTTP 模型供应商适配器转换标准响应", async () => {
  const server = (await import("node:http")).createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "application/json", "X-Request-Id": "http_provider_1" });
    res.end(JSON.stringify({ dataBase64: Buffer.from("provider-image").toString("base64"), contentType: "image/png", extension: "png", costCents: 9 }));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    const gateway = new ModelGateway({ url: `http://127.0.0.1:${address.port}`, apiKey: "secret", timeoutMs: 2000 });
    const result = await gateway.generate({ id: "task_http", type: "TEXT_TO_IMAGE", model: "provider-model", prompt: "test", params: {} });
    assert.equal(result.providerRequestId, "http_provider_1");
    assert.equal(result.content.toString(), "provider-image");
    assert.equal(result.costCents, 9);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test("模型网关按能力和权重路由并在可重试故障时降级", async () => {
  const http = await import("node:http");
  const primary = http.createServer((req, res) => {
    res.writeHead(503, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ message: "temporary unavailable" }));
  });
  const fallback = http.createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({
      providerCode: "fallback-provider", providerRequestId: "fallback_1",
      dataBase64: Buffer.from("fallback-image").toString("base64"), contentType: "image/png", extension: "png", costCents: 7
    }));
  });
  await new Promise((resolve) => primary.listen(0, "127.0.0.1", resolve));
  await new Promise((resolve) => fallback.listen(0, "127.0.0.1", resolve));
  try {
    const primaryAddress = primary.address();
    const fallbackAddress = fallback.address();
    const gateway = new ModelGateway({
      providers: [
        { code: "primary-provider", url: `http://127.0.0.1:${primaryAddress.port}`, capabilities: ["TEXT_TO_IMAGE"], weight: 100 },
        { code: "fallback-provider", url: `http://127.0.0.1:${fallbackAddress.port}`, capabilities: ["TEXT_TO_IMAGE"], weight: 10 }
      ],
      timeoutMs: 2000
    });
    const result = await gateway.generate({ id: "route_task", type: "TEXT_TO_IMAGE", model: "mock-standard", prompt: "route", params: {} });
    assert.equal(result.providerCode, "fallback-provider");
    assert.equal(result.content.toString(), "fallback-image");
    await assert.rejects(() => gateway.generate({ id: "video_task", type: "TEXT_TO_VIDEO", model: "mock-standard", prompt: "route", params: {} }), /No model provider supports/);
  } finally {
    await new Promise((resolve) => primary.close(resolve));
    await new Promise((resolve) => fallback.close(resolve));
  }
});

test("OpenAI 图像供应商适配器转换 GPT Image 响应", async () => {
  const http = await import("node:http");
  let requestBody = null;
  const server = http.createServer(async (req, res) => {
    requestBody = JSON.parse(await new Promise((resolve) => {
      const chunks = [];
      req.on("data", (chunk) => chunks.push(chunk));
      req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    }));
    assert.equal(req.url, "/v1/images/generations");
    assert.equal(req.headers.authorization, "Bearer test-openai-key");
    res.writeHead(200, { "Content-Type": "application/json", "X-Request-Id": "openai_img_1" });
    res.end(JSON.stringify({ data: [{ b64_json: Buffer.from("openai-image").toString("base64") }] }));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    const gateway = new ModelGateway({
      kind: "openai",
      openAiApiKey: "test-openai-key",
      openAiBaseUrl: `http://127.0.0.1:${address.port}`,
      openAiImageModel: "newapi-image-model",
      timeoutMs: 2000
    });
    const result = await gateway.generate({ id: "openai_img_task", type: "TEXT_TO_IMAGE", model: "mock-standard", prompt: "test image", params: { size: "1024x1024" } });
    assert.equal(requestBody.model, "newapi-image-model");
    assert.equal(requestBody.response_format, "b64_json");
    assert.equal(result.providerCode, "openai");
    assert.equal(result.providerRequestId, "openai_img_1");
    assert.equal(result.content.toString(), "openai-image");
    assert.equal(result.contentType, "image/png");
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test("OpenAI 视频供应商适配器创建、轮询并下载 Sora 内容", async () => {
  const http = await import("node:http");
  const paths = [];
  const server = http.createServer(async (req, res) => {
    paths.push(`${req.method} ${req.url}`);
    assert.equal(req.headers.authorization, "Bearer test-openai-key");
    if (req.method === "POST" && req.url === "/v1/videos") {
      res.writeHead(200, { "Content-Type": "application/json", "X-Request-Id": "openai_video_req" });
      res.end(JSON.stringify({ id: "video_123", status: "queued" }));
      return;
    }
    if (req.method === "GET" && req.url === "/v1/videos/video_123") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ id: "video_123", status: "completed" }));
      return;
    }
    if (req.method === "GET" && req.url === "/v1/videos/video_123/content") {
      res.writeHead(200, { "Content-Type": "video/mp4" });
      res.end(Buffer.from("mp4-bytes"));
      return;
    }
    res.writeHead(404).end();
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    const gateway = new ModelGateway({
      kind: "openai",
      openAiApiKey: "test-openai-key",
      openAiBaseUrl: `http://127.0.0.1:${address.port}`,
      timeoutMs: 2000
    });
    const result = await gateway.generate({
      id: "openai_video_task",
      type: "TEXT_TO_VIDEO",
      model: "sora-2",
      prompt: "test video",
      params: { seconds: 4, pollIntervalMs: 100, maxPolls: 2 }
    });
    assert.deepEqual(paths, ["POST /v1/videos", "GET /v1/videos/video_123", "GET /v1/videos/video_123/content"]);
    assert.equal(result.providerCode, "openai");
    assert.equal(result.providerRequestId, "openai_video_req");
    assert.equal(result.content.toString(), "mp4-bytes");
    assert.equal(result.contentType, "video/mp4");
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test("企业管理员可添加成员并分配额度，普通成员不能跨企业管理", () => {
  const { platform, store, user } = setup();
  const other = platform.createUser({ email: "member@example.com", password: "Member123!", name: "企业成员" });
  const otherUser = store.state.users.find((item) => item.id === other.id);
  const enterprise = platform.createEnterprise(user, { name: "先知科技", totalQuota: 20000 });
  const membership = platform.addEnterpriseMember(user, { email: other.email, quotaLimit: 3000 });
  assert.equal(platform.enterpriseFor(otherUser).id, enterprise.id);
  assert.equal(enterprise.availableQuota, 17000);
  platform.allocateEnterpriseQuota(user, membership.id, 2000);
  assert.equal(membership.quotaLimit, 5000);
  assert.equal(enterprise.availableQuota, 15000);
  assert.throws(() => platform.addEnterpriseMember(otherUser, { email: "demo@xianzhi.ai" }), /仅企业管理员/);
});

test("企业成员生成任务可使用企业额度并在成功后保留消耗", async () => {
  const { platform, store, user } = setup();
  const other = platform.createUser({ email: "quota-member@example.com", password: "Member123!", name: "额度成员" });
  const otherUser = store.state.users.find((item) => item.id === other.id);
  platform.createEnterprise(user, { name: "企业计费测试", totalQuota: 1000 });
  const membership = platform.addEnterpriseMember(user, { email: other.email, quotaLimit: 100 });
  const personalBefore = platform.account(otherUser.id).available;
  const task = platform.createGeneration(otherUser, "TEXT_TO_IMAGE", { prompt: "企业额度生成", billingSource: "ENTERPRISE" });
  assert.equal(task.billingSource, "ENTERPRISE");
  assert.equal(membership.quotaUsed, 10);
  assert.equal(platform.account(otherUser.id).available, personalBefore);
  await new Promise((resolve) => setTimeout(resolve, 800));
  assert.equal(task.status, "SUCCEEDED");
  assert.equal(membership.quotaUsed, 10);
  assert.equal(store.state.enterpriseQuotaTransactions.at(-1).type, "CONSUME");
});

test("企业额度生成失败会自动返还额度", async () => {
  const { platform, store, user } = setup();
  const other = platform.createUser({ email: "quota-refund@example.com", password: "Member123!", name: "退款成员" });
  const otherUser = store.state.users.find((item) => item.id === other.id);
  platform.createEnterprise(user, { name: "企业退款测试", totalQuota: 1000 });
  const membership = platform.addEnterpriseMember(user, { email: other.email, quotaLimit: 100 });
  const task = platform.createGeneration(otherUser, "TEXT_TO_IMAGE", { prompt: "[fail] 企业额度生成", billingSource: "ENTERPRISE" });
  await new Promise((resolve) => setTimeout(resolve, 800));
  assert.equal(task.status, "FAILED");
  assert.equal(membership.quotaUsed, 0);
  assert.equal(store.state.enterpriseQuotaTransactions.at(-1).type, "REFUND");
});

test("企业额度不足时拒绝生成并将任务标记为失败", () => {
  const { platform, store, user } = setup();
  const other = platform.createUser({ email: "quota-low@example.com", password: "Member123!", name: "低额度成员" });
  const otherUser = store.state.users.find((item) => item.id === other.id);
  platform.createEnterprise(user, { name: "企业低额度测试", totalQuota: 1000 });
  platform.addEnterpriseMember(user, { email: other.email, quotaLimit: 5 });
  assert.throws(() => platform.createGeneration(otherUser, "TEXT_TO_IMAGE", { prompt: "额度不足", billingSource: "ENTERPRISE" }), /quota is insufficient/);
  assert.equal(store.state.generationTasks.at(-1).status, "FAILED");
});

test("GEO 到期计划可自动执行并推进下一次运行时间", async () => {
  const { platform, store, user } = setup();
  const brand = platform.createGeoBrand(user, { name: "GEO 测试品牌", keywords: ["AI 平台"], competitors: ["竞品 A"] });
  const schedule = platform.createGeoSchedule(user, {
    brandId: brand.id, question: "推荐 AI 内容平台", frequency: "DAILY", nextRunAt: "2020-01-01T00:00:00.000Z"
  });
  const tasks = await platform.runDueGeoSchedules("2026-06-14T00:00:00.000Z");
  assert.equal(tasks.length, 1);
  assert.equal(tasks[0].scheduleId, schedule.id);
  assert.equal(tasks[0].result.source, "mock-ai-search");
  assert.ok(schedule.nextRunAt > "2026-06-14T00:00:00.000Z");
  assert.equal(store.state.auditLogs.at(-1).action, "RUN_GEO_MONITOR");
});

test("GEO 监测数据可生成趋势报告和优化内容", async () => {
  const { platform, user } = setup();
  const brand = platform.createGeoBrand(user, { name: "报告品牌", keywords: ["企业智能体"], competitors: ["竞品 B"] });
  await platform.createGeoTask(user, { brandId: brand.id, question: "企业智能体推荐" });
  await platform.createGeoTask(user, { brandId: brand.id, question: "企业智能体方案" });
  const report = platform.createGeoReport(user, { brandId: brand.id, period: "WEEKLY" });
  const content = platform.createGeoContent(user, { brandId: brand.id, topic: "企业智能体落地" });
  const overview = platform.geoOverview(user);
  assert.equal(report.taskCount, 2);
  assert.equal(report.recommendations.length, 3);
  assert.match(content.title, /企业智能体落地/);
  assert.equal(overview.reports.length, 1);
  assert.equal(overview.contents.length, 1);
});

test("用户不能手动执行其他用户的 GEO 计划", async () => {
  const { platform, store, user } = setup();
  const other = platform.createUser({ email: "geo-private@example.com", password: "Private123!", name: "GEO 私有用户" });
  const otherUser = store.state.users.find((item) => item.id === other.id);
  const brand = platform.createGeoBrand(otherUser, { name: "私有 GEO 品牌" });
  const schedule = platform.createGeoSchedule(otherUser, { brandId: brand.id });
  await assert.rejects(() => platform.runGeoSchedule(schedule.id, user), /not accessible/);
});

test("HTTP GEO 数据源结果会标准化并写入监测任务", async () => {
  const server = (await import("node:http")).createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({
      mentionRate: 1.4, citationRate: 0.36, sentiment: "POSITIVE", rank: 2,
      competitorRates: { "竞品 C": 0.52 }, confidence: 0.91, source: "doubao-search"
    }));
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    const store = createStore({ persist: false });
    const address = server.address();
    const platform = new Platform(store, { geoSource: new GeoSource({ url: `http://127.0.0.1:${address.port}`, timeoutMs: 2000 }) });
    const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
    const brand = platform.createGeoBrand(user, { name: "真实数据品牌", competitors: ["竞品 C"] });
    const task = await platform.createGeoTask(user, { brandId: brand.id, question: "推荐内容生产平台", platform: "doubao" });
    assert.equal(task.result.source, "doubao-search");
    assert.equal(task.result.mentionRate, 1);
    assert.equal(task.result.citationRate, 0.36);
    assert.equal(task.result.competitorRates["竞品 C"], 0.52);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test("GEO 优化内容可记录发布并跟踪引用和品牌提及效果", () => {
  const { platform, user } = setup();
  const brand = platform.createGeoBrand(user, { name: "效果品牌", keywords: ["智能体"] });
  const content = platform.createGeoContent(user, { brandId: brand.id, topic: "智能体选型" });
  const publication = platform.publishGeoContent(user, content.id, { platform: "官网", url: "https://example.com/agent-guide" });
  const duplicate = platform.publishGeoContent(user, content.id, { platform: "官网", url: "https://example.com/agent-guide" });
  assert.equal(publication.id, duplicate.id);
  platform.recordGeoContentMetrics(user, publication.id, { impressions: 1000, citations: 20, brandMentions: 40, clicks: 80 });
  const tracked = platform.recordGeoContentMetrics(user, publication.id, { impressions: 1600, citations: 35, brandMentions: 70, clicks: 150 });
  assert.equal(tracked.effect.citationRate, 0.0219);
  assert.equal(tracked.effect.mentionGrowth, 30);
  assert.equal(content.status, "PUBLISHED");
  assert.equal(platform.geoOverview(user).publications[0].effect.latest.clicks, 150);
});

test("运营后台仅管理员可访问并可冻结或恢复用户", () => {
  const { platform, store, user } = setup();
  const admin = store.state.users.find((item) => item.role === "SUPER_ADMIN");
  assert.throws(() => platform.adminOverview(user), /Only platform administrators/);
  const overview = platform.adminOverview(admin);
  assert.ok(overview.metrics.users >= 3);
  const session = platform.login(user.email, "Demo123!");
  platform.updateUserStatus(admin, user.id, "SUSPENDED");
  assert.equal(user.status, "SUSPENDED");
  assert.equal(platform.authenticate(session.token), null);
  assert.throws(() => platform.login(user.email, "Demo123!"), /not active/);
  platform.updateUserStatus(admin, user.id, "ACTIVE");
  assert.equal(user.status, "ACTIVE");
  assert.throws(() => platform.updateUserStatus(admin, admin.id, "SUSPENDED"), /cannot suspend/);
});

test("内容安全审核拒绝敏感提示词并记录审核日志", () => {
  const { platform, store, user } = setup();
  assert.throws(() => platform.createGeneration(user, "TEXT_TO_IMAGE", { prompt: "包含暴力犯罪的内容" }), /内容安全审核未通过/);
  assert.equal(store.state.moderationLogs.at(-1).status, "REJECTED");
  assert.equal(platform.account(user.id).frozen, 0);
});

test("文件上传校验类型和大小并写入对象存储", async () => {
  const store = createStore({ persist: false });
  const uploads = [];
  const platform = new Platform(store, { objectStore: { enabled: true, put: async (...args) => uploads.push(args) } });
  const user = store.state.users.find((item) => item.email === "demo@xianzhi.ai");
  const asset = await platform.uploadAsset(user, {
    name: "brief.txt", contentType: "text/plain", dataBase64: Buffer.from("先知 AI 项目简报").toString("base64")
  });
  assert.equal(asset.metadata.source, "upload");
  assert.equal(uploads.length, 1);
  await assert.rejects(() => platform.uploadAsset(user, {
    name: "bad.exe", contentType: "application/octet-stream", dataBase64: Buffer.from("bad").toString("base64")
  }), /不支持的文件类型/);
});

test("智能体工作流支持版本保存和回滚", () => {
  const { platform, store, user } = setup();
  const agent = platform.createAgent(user, { name: "版本助手" });
  platform.updateAgentWorkflow(user, agent.id, {
    workflow: [{ id: "start", type: "START" }, { id: "knowledge", type: "KNOWLEDGE" }, { id: "end", type: "END" }]
  });
  assert.equal(agent.version, 2);
  platform.rollbackAgent(user, agent.id, 1);
  assert.equal(agent.version, 3);
  assert.equal(agent.workflow.length, 3);
  assert.equal(store.state.agentVersions.filter((item) => item.agentId === agent.id).length, 3);
});

test("智能体工作流拒绝无效首尾节点、重复节点和未知类型", () => {
  const { platform, user } = setup();
  const agent = platform.createAgent(user, { name: "工作流校验助手" });
  assert.throws(() => platform.updateAgentWorkflow(user, agent.id, {
    workflow: [{ id: "llm", type: "LLM" }, { id: "end", type: "END" }]
  }), /start with START/);
  assert.throws(() => platform.updateAgentWorkflow(user, agent.id, {
    workflow: [{ id: "start", type: "START" }, { id: "same", type: "LLM" }, { id: "same", type: "END" }]
  }), /valid and unique/);
  assert.throws(() => platform.updateAgentWorkflow(user, agent.id, {
    workflow: [{ id: "start", type: "START" }, { id: "bad", type: "UNKNOWN" }, { id: "end", type: "END" }]
  }), /valid and unique/);
});

test("知识库文档切片可为已发布智能体提供检索引用", () => {
  const { platform, user } = setup();
  const kb = platform.createKnowledgeBase(user, { name: "产品知识库" });
  const doc = platform.addKnowledgeDocument(user, kb.id, {
    name: "先知介绍", content: "先知 AI 提供文生图和文生视频能力。\n\n企业可以通过代理商体系销售会员套餐。"
  });
  const agent = platform.createAgent(user, { name: "知识助手", knowledgeBaseIds: [kb.id] });
  platform.publishAgent(user, agent.id);
  const call = platform.callAgent(user, agent.id, "先知 AI 提供什么能力");
  assert.match(call.output, /文生图和文生视频/);
  assert.deepEqual(call.references, [doc.id]);
  assert.equal(doc.embeddings.length, doc.chunks.length);
  assert.equal(doc.embeddings[0].length, 32);
});

test("知识库混合检索返回向量相似度并保持结果排序", () => {
  const { platform, user } = setup();
  const kb = platform.createKnowledgeBase(user, { name: "向量知识库" });
  platform.addKnowledgeDocument(user, kb.id, { name: "生成能力", content: "平台支持图像生成和视频生成。\n\n代理商可以销售会员套餐。" });
  const results = platform.vectorSearchKnowledge([kb.id], "图像视频生成");
  assert.ok(results.length >= 1);
  assert.ok(results[0].vectorScore > 0);
  assert.match(results[0].chunk, /生成/);
});

test("智能体不能绑定其他用户的私有知识库", () => {
  const { platform, store, user } = setup();
  const other = platform.createUser({ email: "private@example.com", password: "Private123!", name: "私有用户" });
  const otherUser = store.state.users.find((item) => item.id === other.id);
  const privateKb = platform.createKnowledgeBase(otherUser, { name: "私有知识库" });
  assert.throws(() => platform.createAgent(user, { name: "越权助手", knowledgeBaseIds: [privateKb.id] }), /无权绑定/);
});
