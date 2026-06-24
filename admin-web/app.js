const modules = [
  { id: "dashboard", label: "数据中心", icon: "数", title: "数据中心", endpoint: "/api/v1/admin/overview" },
  { id: "customers", label: "客户中心", icon: "客", title: "客户中心", endpoint: "/api/v1/admin/customers" },
  { id: "channels", label: "代理商中心", icon: "渠", title: "代理商中心", endpoint: "/api/v1/admin/channel-agents/tree" },
  { id: "products", label: "产品中心", icon: "产", title: "产品中心", endpoint: "/api/v1/admin/products" },
  { id: "plans", label: "套餐中心", icon: "套", title: "套餐中心", endpoint: "/api/v1/admin/plans" },
  { id: "orders", label: "订单中心", icon: "订", title: "订单中心", endpoint: "/api/v1/admin/orders" },
  { id: "delivery", label: "交付中心", icon: "交", title: "交付中心", endpoint: "/api/v1/admin/delivery-projects" },
  { id: "usage", label: "用量中心", icon: "用", title: "用量中心", endpoint: "/api/v1/admin/usage" },
  { id: "commissions", label: "分润中心", icon: "分", title: "分润中心", endpoint: "/api/v1/admin/commissions" },
  { id: "system", label: "系统中心", icon: "系", title: "系统中心", endpoint: "/api/v1/admin/system/settings" }
];

const state = {
  active: new URLSearchParams(location.search).get("module") || "dashboard",
  cache: new Map(),
  currentRows: []
};

const nav = document.querySelector("#nav");
const pageTitle = document.querySelector("#page-title");
const sectionKicker = document.querySelector("#section-kicker");
const sectionTitle = document.querySelector("#section-title");
const sectionMeta = document.querySelector("#section-meta");
const panel = document.querySelector("#panel");
const metrics = document.querySelector("#metrics");
const apiStatus = document.querySelector("#api-status");
const logoutButton = document.querySelector("#logout-button");
const changePasswordButton = document.querySelector("#change-password-button");
const passwordDialog = document.querySelector("#password-dialog");
const passwordForm = document.querySelector("#password-form");
const passwordStatus = document.querySelector("#password-status");

function init() {
  if (logoutButton) logoutButton.addEventListener("click", logoutAdmin);
  if (changePasswordButton) changePasswordButton.addEventListener("click", openPasswordDialog);
  document.querySelector("#password-cancel-button")?.addEventListener("click", closePasswordDialog);
  document.querySelector("#password-cancel-secondary")?.addEventListener("click", closePasswordDialog);
  if (passwordForm) passwordForm.addEventListener("submit", submitPasswordChange);
  nav.innerHTML = modules.map((item) => `
    <button type="button" data-module="${item.id}">
      <span class="icon">${item.icon}</span>
      <span>${item.label}</span>
    </button>
  `).join("");
  nav.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-module]");
    if (!button) return;
    setActive(button.dataset.module);
  });
  panel.addEventListener("click", handlePanelAction);
  setActive(state.active);
}

async function setActive(moduleId) {
  state.active = moduleId;
  const mod = modules.find((item) => item.id === moduleId) || modules[0];
  history.replaceState(null, "", `?module=${mod.id}`);
  document.querySelectorAll(".nav button").forEach((button) => {
    button.classList.toggle("active", button.dataset.module === mod.id);
  });
  pageTitle.textContent = mod.title;
  sectionTitle.textContent = mod.title;
  sectionKicker.textContent = mod.id;
  sectionMeta.textContent = "读取中";
  panel.innerHTML = `<div class="empty">正在加载 ${mod.label}</div>`;

  try {
    const data = await load(mod.endpoint);
    apiStatus.textContent = "API ONLINE";
    apiStatus.className = "status-dot ok";
    renderMetrics(mod.id, data);
    renderPanel(mod.id, data);
  } catch (error) {
    apiStatus.textContent = "API ERROR";
    apiStatus.className = "status-dot error";
    sectionMeta.textContent = "";
    panel.innerHTML = `<div class="error-box">${escapeHTML(error.message)}</div>`;
  }
}

async function load(endpoint) {
  if (state.cache.has(endpoint)) return state.cache.get(endpoint);
  const response = await fetch(endpoint, { headers: { Accept: "application/json" } });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const data = await response.json();
  state.cache.set(endpoint, data);
  return data;
}

function renderMetrics(moduleId, data) {
  const items = moduleId === "dashboard" ? data.metrics || [] : summaryMetrics(moduleId, data);
  metrics.innerHTML = items.slice(0, 8).map((item) => `
    <article class="metric">
      <label>${escapeHTML(item.label)}</label>
      <strong>${formatValue(item.value, item.unit)}</strong>
    </article>
  `).join("");
}

function summaryMetrics(moduleId, data) {
  const count = Array.isArray(data.items) ? data.items.length : 0;
  const map = {
    customers: [{ label: "客户数", value: count }, { label: "活跃客户", value: countByStatus(data.items, "ACTIVE") }],
    channels: [{ label: "一级渠道", value: count }, { label: "渠道层级", value: "2 级" }],
    products: [{ label: "产品线", value: count }, { label: "API 模型", value: "3 个" }],
    plans: [{ label: "套餐数", value: count }, { label: "可售套餐", value: count }],
    orders: [{ label: "订单数", value: count }, { label: "收款金额", value: sum(data.items, "amountCents"), unit: "cents" }],
    delivery: [{ label: "项目数", value: count }, { label: "平均进度", value: average(data.items, "progress") + "%" }],
    usage: [
      { label: "API 调用", value: data.summary?.apiCalls || 0 },
      { label: "Agent 对话", value: data.summary?.agentChats || 0 },
      { label: "GEO 任务", value: data.summary?.geoTasks || 0 }
    ],
    commissions: [
      { label: "代理数", value: data.summary?.agents || 0 },
      { label: "代理收益", value: data.summary?.totalCents || 0, unit: "cents" }
    ],
    system: [
      { label: "权限角色", value: data.permissions?.length || 0 },
      { label: "支付通道", value: data.payments?.length || 0 },
      { label: "上游渠道", value: data.apiChannels?.length || 0 },
      { label: "API Key", value: data.apiKeys?.length || 0 }
    ]
  };
  return map[moduleId] || [{ label: "记录数", value: count }];
}

function renderPanel(moduleId, data) {
  if (moduleId === "dashboard") return renderDashboard(data);
  if (moduleId === "system") return renderSystem(data);
  if (moduleId === "usage") return renderTable(data.items || [], ["product", "metric", "usage", "costCents"], ["产品", "指标", "用量", "成本"], toolbarFor("usage"));
  if (moduleId === "commissions") return renderCommissions(data);

  const configs = {
    customers: [["name", "email", "role", "plan", "pointsAvailable", "status", "_actions"], ["客户", "邮箱", "角色", "套餐", "余额", "状态", "操作"]],
    channels: [["name", "email", "level", "inviteCode", "status", "_actions"], ["渠道", "邮箱", "等级", "邀请码", "状态", "操作"]],
    products: [["name", "type", "status", "usage", "entitlements", "_actions"], ["产品", "类型", "状态", "用量", "权益", "操作"]],
    plans: [["name", "priceCents", "grantPoints", "durationDays", "concurrency", "_actions"], ["套餐", "价格", "权益点数", "有效期", "并发", "操作"]],
    orders: [["id", "customer", "plan", "amountCents", "status", "createdAt", "_actions"], ["订单", "客户", "套餐", "金额", "状态", "创建时间", "操作"]],
    delivery: [["name", "type", "customer", "status", "progress", "owner", "_actions"], ["项目", "类型", "客户", "状态", "进度", "负责人", "操作"]]
  };
  const [keys, labels] = configs[moduleId] || [Object.keys((data.items || [])[0] || {}), []];
  renderTable(flattenTree(data.items || []), keys, labels, toolbarFor(moduleId));
}

function renderDashboard(data) {
  sectionMeta.textContent = "收入、成本、利润和核心用量";
  const profit = data.profit || {};
  const usage = data.usage || {};
  panel.innerHTML = `
    <div class="cards">
      <article class="mini-card"><h3>经营结果</h3><p>收入 ${formatMoney(profit.revenueCents || 0)}，成本 ${formatMoney((profit.modelCostCents || 0) + (profit.commissionCents || 0))}，利润 ${formatMoney(profit.estimatedProfitCents || 0)}。</p></article>
      <article class="mini-card"><h3>产品用量</h3><p>API/生成 ${usage.apiCalls || 0} 次，Agent 对话 ${usage.agentChats || 0} 次，GEO 任务 ${usage.geoTasks || 0} 个。</p></article>
      <article class="mini-card"><h3>API 中转</h3><p>已规划模型目录、上游渠道、API Key、分组倍率、配额和 OpenAI 兼容用量查询。</p></article>
    </div>
  `;
}

function renderSystem(data) {
  sectionMeta.textContent = "权限、域名、支付、品牌";
  const rows = [
    { item: "品牌", value: data.brand?.name || "先知 AI", status: "ACTIVE" },
    { item: "域名", value: data.brand?.domain || "localhost:3100", status: "ACTIVE" },
    { item: "权限角色", value: (data.permissions || []).join(" / "), status: "ACTIVE" },
    { item: "支付通道", value: (data.payments || []).map((item) => `${item.channel}:${item.status}`).join(" / "), status: "CONFIGURABLE" },
    { item: "API 网关", value: data.apiGateway?.openAICompatible ? "OpenAI 兼容" : "未启用", status: "ACTIVE" }
  ];
  const channels = (data.apiChannels || []).map((item) => ({ ...item, item: `上游渠道：${item.name}`, value: `${item.baseUrl} / ${item.models?.join(", ")}`, status: item.status, _kind: "channel" }));
  const models = (data.apiModels || []).map((item) => ({ ...item, item: `模型：${item.name}`, value: `${item.model} / ${item.billingMode} / ${item.fixedQuota || item.modelRatio}`, status: item.status, _kind: "model" }));
  const keys = (data.apiKeys || []).map((item) => ({ ...item, item: `API Key：${item.customer}`, value: `${item.prefix} / ${item.models?.join(", ")} / ${item.quotaLimit}`, status: item.status, _kind: "key" }));
  const groups = (data.customerGroups || []).map((item) => ({ ...item, item: `客户分组：${item.name}`, value: `倍率 ${item.ratio} / ${item.models?.join(", ")}`, status: "ACTIVE", _kind: "group" }));
  renderTable([...rows, ...channels, ...models, ...keys, ...groups], ["item", "value", "status", "_actions"], ["配置项", "值", "状态", "操作"], toolbarFor("system"));
}

function renderTable(items, keys, labels, toolbar = "") {
  state.currentRows = items;
  sectionMeta.textContent = `${items.length} 条记录`;
  if (!items.length) {
    panel.innerHTML = `${toolbar}<div class="empty">暂无记录</div>`;
    return;
  }
  panel.innerHTML = `
    ${toolbar}
    <div class="table-wrap">
      <table>
        <thead><tr>${keys.map((key, index) => `<th>${escapeHTML(labels[index] || key)}</th>`).join("")}</tr></thead>
        <tbody>
          ${items.map((item) => `<tr>${keys.map((key) => `<td>${formatCell(item[key], key, item)}</td>`).join("")}</tr>`).join("")}
        </tbody>
      </table>
    </div>
  `;
}

function flattenTree(items) {
  return items.flatMap((item) => [item, ...(item.children || []).map((child) => ({ ...child, name: `二级 - ${child.name || child.id}` }))]);
}

function formatCell(value, key, item) {
  if (key === "_actions") return rowActions(state.active, item);
  if (Array.isArray(value)) return value.map(escapeHTML).join("、");
  if (key.toLowerCase().includes("status")) return `<span class="status ${String(value || "").toLowerCase()}">${escapeHTML(value || "-")}</span>`;
  if (key.toLowerCase().includes("cents")) return formatMoney(Number(value || 0));
  if (typeof value === "object" && value) return escapeHTML(JSON.stringify(value));
  return escapeHTML(value ?? "-");
}

function toolbarFor(moduleId) {
  const buttons = {
    customers: [["create-customer", "新建客户"]],
    channels: [["create-channel", "新增代理商"]],
    orders: [["create-order", "新建订单"]],
    usage: [["filter-usage", "筛选产品"], ["export-usage", "导出 CSV"]],
    commissions: [["create-commission", "登记分润"], ["create-withdrawal", "申请提现"]],
    system: [["edit-system", "品牌域名"], ["create-api-channel", "新增上游"], ["create-api-key", "新增 API Key"]],
  }[moduleId] || [];
  if (!buttons.length) return "";
  return `<div class="toolbar">${buttons.map(([action, label]) => `<button class="action-btn" data-action="${action}">${label}</button>`).join("")}</div>`;
}

function rowActions(moduleId, item) {
  const id = escapeHTML(item.id || "");
  const status = String(item.status || "").toUpperCase();
  const buttons = {
    customers: [["edit-customer", "编辑"]],
    channels: [["toggle-channel", status === "ACTIVE" ? "停用" : "启用"]],
    products: [["edit-product", "编辑"]],
    plans: [["edit-plan", "保存价格"]],
    orders: [["mark-paid", "标记收款"], ["renew-order", "续费"]],
    delivery: [["update-delivery", "更新进度"]],
    commissions: item._kind === "withdrawal" && status === "PENDING" ? [["approve-withdrawal", "通过"], ["reject-withdrawal", "驳回"]] : [],
    system: systemRowActions(item)
  }[moduleId] || [];
  return `<div class="action-cell">${buttons.map(([action, label]) => `<button class="action-btn" data-action="${action}" data-id="${id}">${label}</button>`).join("")}</div>`;
}

function systemRowActions(item) {
  if (item._kind === "channel") return [["test-api-channel", "测试"], ["toggle-api-channel", "启停"]];
  if (item._kind === "model") return [["edit-api-model", "调价"]];
  if (item._kind === "key") return [["toggle-api-key", "启停"]];
  if (item._kind === "group") return [["edit-customer-group", "倍率"]];
  return [];
}

async function handlePanelAction(event) {
  const button = event.target.closest("button[data-action]");
  if (!button) return;
  const action = button.dataset.action;
  const id = button.dataset.id;
  const item = state.currentRows.find((row) => row.id === id) || {};
  try {
    await runAction(action, item);
    state.cache.clear();
    await setActive(state.active);
  } catch (error) {
    alert(error.message);
  }
}

async function runAction(action, item) {
  if (action === "create-channel") {
    const name = prompt("代理商名称");
    if (!name) return;
    const email = prompt("代理商登录邮箱", `agent${Date.now()}@example.com`);
    if (!email) return;
    const level = Number(prompt("代理等级：1 或 2", "1"));
    if (![1, 2].includes(level)) throw new Error("代理等级只能是 1 或 2");
    const parentId = level === 2 ? prompt("上级代理 ID", "channel_000001") : "";
    if (level === 2 && !parentId) return;
    const inviteCode = prompt("邀请码（可留空自动生成）", "");
    await apiRequest("POST", "/api/v1/admin/channel-agents", { name, email, level, parentId, inviteCode, status: "ACTIVE", available: 0 });
    alert("代理商已新增，默认登录密码：Agent123!");
    return;
  }
  if (action === "create-customer") {
    const name = prompt("客户名称");
    if (!name) return;
    const email = prompt("客户邮箱", `${Date.now()}@example.com`);
    if (!email) return;
    await apiRequest("POST", "/api/v1/admin/customers", { name, email, role: "MEMBER", status: "ACTIVE", planId: "plan_free", available: 1000 });
    return;
  }
  if (action === "edit-customer") {
    const status = prompt("客户状态", item.status || "ACTIVE");
    if (!status) return;
    const planId = prompt("套餐 ID", item.planId || "plan_free");
    if (!planId) return;
    const available = Number(prompt("客户余额", item.pointsAvailable ?? 0));
    await apiRequest("PATCH", `/api/v1/admin/customers/${item.id}`, { name: item.name, email: item.email, role: item.role, status, planId, available });
    return;
  }
  if (action === "toggle-channel") {
    const next = String(item.status || "").toUpperCase() === "ACTIVE" ? "DISABLED" : "ACTIVE";
    await apiRequest("PATCH", `/api/v1/admin/channel-agents/${item.id}`, { status: next });
    return;
  }
  if (action === "edit-product") {
    const status = prompt("产品状态", item.status || "ACTIVE");
    if (!status) return;
    const name = prompt("产品名称", item.name || "");
    if (!name) return;
    await apiRequest("PATCH", `/api/v1/admin/products/${item.id}`, { name, type: item.type, status, entitlements: Array.isArray(item.entitlements) ? item.entitlements : [] });
    return;
  }
  if (action === "edit-plan") {
    const priceCents = Number(prompt("价格（分）", item.priceCents || 0));
    const grantPoints = Number(prompt("权益点数", item.grantPoints || 0));
    await apiRequest("PATCH", `/api/v1/admin/plans/${item.id}`, {
      name: item.name,
      priceCents,
      grantPoints,
      durationDays: Number(item.durationDays || 30),
      concurrency: Number(item.concurrency || 1),
      active: true
    });
    return;
  }
  if (action === "create-order") {
    const userId = prompt("客户 userId", "user_000002");
    if (!userId) return;
    const planId = prompt("套餐 ID", "plan_month");
    if (!planId) return;
    const amountCents = Number(prompt("订单金额（分）", "9900"));
    await apiRequest("POST", "/api/v1/admin/orders", { userId, planId, amountCents, status: "PENDING" });
    return;
  }
  if (action === "mark-paid") {
    await apiRequest("POST", `/api/v1/admin/orders/${item.id}/mark-paid`, {});
    return;
  }
  if (action === "renew-order") {
    await apiRequest("POST", `/api/v1/admin/orders/${item.id}/renew`, {});
    return;
  }
  if (action === "update-delivery") {
    const progress = Number(prompt("进度 0-100", item.progress || 50));
    const status = prompt("交付状态", item.status || "IN_PROGRESS");
    if (!status) return;
    await apiRequest("PATCH", `/api/v1/admin/delivery-projects/${item.id}`, { status, progress });
    return;
  }
  if (action === "filter-usage") {
    const product = prompt("产品筛选（API / Agent / GEO）", "");
    const endpoint = product ? `/api/v1/admin/usage?product=${encodeURIComponent(product)}` : "/api/v1/admin/usage";
    state.cache.delete(endpoint);
    const data = await load(endpoint);
    renderMetrics("usage", data);
    renderPanel("usage", data);
    return;
  }
  if (action === "export-usage") {
    const product = prompt("导出筛选（留空导出全部）", "");
    const query = product ? `?product=${encodeURIComponent(product)}` : "";
    location.href = `/api/v1/admin/usage/export${query}`;
    return;
  }
  if (action === "create-commission") {
    const orderId = prompt("订单 ID", "order_000001");
    if (!orderId) return;
    const agentId = prompt("代理 ID", "channel_000001");
    if (!agentId) return;
    const amountCents = Number(prompt("分润金额（分）", "1000"));
    const rate = Number(prompt("分润比例", "0.1"));
    await apiRequest("POST", "/api/v1/admin/commissions", { orderId, agentId, amountCents, rate, status: "PENDING" });
    return;
  }
  if (action === "create-withdrawal") {
    const agentId = prompt("代理 ID", item.agentId || "channel_000001");
    if (!agentId) return;
    const amountCents = Number(prompt("提现金额（分）", "1000"));
    await apiRequest("POST", "/api/v1/admin/withdrawals", { agentId, amountCents });
    return;
  }
  if (action === "approve-withdrawal") {
    await apiRequest("POST", `/api/v1/admin/withdrawals/${item.id}/approve`, {});
    return;
  }
  if (action === "reject-withdrawal") {
    await apiRequest("POST", `/api/v1/admin/withdrawals/${item.id}/reject`, {});
    return;
  }
  if (action === "edit-system") {
    const name = prompt("品牌名称", "先知 AI");
    if (!name) return;
    const domain = prompt("绑定域名", "localhost:3100");
    if (!domain) return;
    await apiRequest("PATCH", "/api/v1/admin/system/settings", {
      brand: { name, domain, logo: name.slice(0, 1) },
      payments: [
        { channel: "wechat", status: "CONFIGURABLE" },
        { channel: "alipay", status: "CONFIGURABLE" },
        { channel: "manual", status: "ACTIVE" }
      ],
      permissions: ["SUPER_ADMIN", "ADMIN", "FINANCE", "CHANNEL_MANAGER", "DELIVERY_MANAGER"]
    });
    return;
  }
  if (action === "create-api-channel") {
    const name = prompt("上游渠道名称", "OpenAI 兼容上游");
    if (!name) return;
    const baseUrl = prompt("Base URL", "https://example.com/v1");
    if (!baseUrl) return;
    await apiRequest("POST", "/api/v1/admin/api/provider-channels", { name, baseUrl, status: "CONFIGURABLE", priority: 50, models: ["gpt-image-2", "mock-standard"] });
    return;
  }
  if (action === "create-api-key") {
    const customer = prompt("客户名称", "演示用户");
    if (!customer) return;
    await apiRequest("POST", "/api/v1/admin/api/keys", { customer, status: "ACTIVE", quotaLimit: 100000, models: ["mock-standard", "gpt-image-2"] });
    return;
  }
  if (action === "test-api-channel") {
    const result = await apiRequest("POST", `/api/v1/admin/api/provider-channels/${item.id}/test`, {});
    alert(`渠道测试：${result.item.status}，延迟 ${result.item.latencyMs}ms`);
    return;
  }
  if (action === "toggle-api-channel") {
    const next = String(item.status || "").toUpperCase() === "ACTIVE" ? "DISABLED" : "ACTIVE";
    await apiRequest("PATCH", `/api/v1/admin/api/provider-channels/${item.id}`, { name: item.name, baseUrl: item.baseUrl, status: next, priority: item.priority, models: item.models || [] });
    return;
  }
  if (action === "edit-api-model") {
    const fixedQuota = Number(prompt("固定配额/按次价格", item.fixedQuota || 1));
    await apiRequest("PATCH", `/api/v1/admin/api/models/${item.id}`, { name: item.name, capability: item.capability, billingMode: item.billingMode, fixedQuota, modelRatio: item.modelRatio || 1, completionRatio: item.completionRatio || 1, status: item.status });
    return;
  }
  if (action === "toggle-api-key") {
    const next = String(item.status || "").toUpperCase() === "ACTIVE" ? "DISABLED" : "ACTIVE";
    await apiRequest("PATCH", `/api/v1/admin/api/keys/${item.id}`, { customer: item.customer, status: next, models: item.models || [], quotaLimit: item.quotaLimit || 100000 });
    return;
  }
  if (action === "edit-customer-group") {
    const ratio = Number(prompt("分组倍率", item.ratio || 1));
    await apiRequest("PATCH", `/api/v1/admin/customer-groups/${item.id}`, { name: item.name, ratio, models: item.models || [], description: item.description || "" });
  }
}

async function apiRequest(method, endpoint, body) {
  const response = await fetch(endpoint, {
    method,
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: JSON.stringify(body || {})
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
  return data;
}

function formatValue(value, unit) {
  if (unit === "cents") return formatMoney(Number(value || 0));
  return escapeHTML(value ?? 0);
}

function formatMoney(cents) {
  return `￥${(Number(cents || 0) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function countByStatus(items = [], status) {
  return items.filter((item) => String(item.status).toUpperCase() === status).length;
}

function sum(items = [], key) {
  return items.reduce((total, item) => total + Number(item[key] || 0), 0);
}

function average(items = [], key) {
  if (!items.length) return 0;
  return Math.round(sum(items, key) / items.length);
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

init();



function openPasswordDialog() {
  passwordForm?.reset();
  if (passwordStatus) passwordStatus.textContent = "";
  if (passwordDialog?.showModal) {
    passwordDialog.showModal();
  } else {
    passwordDialog?.setAttribute("open", "");
  }
}

function closePasswordDialog() {
  if (passwordDialog?.close) {
    passwordDialog.close();
  } else {
    passwordDialog?.removeAttribute("open");
  }
}

async function submitPasswordChange(event) {
  event.preventDefault();
  const token = localStorage.getItem("token") || sessionStorage.getItem("token") || "";
  if (!token) {
    setPasswordStatus("登录状态已失效，请重新登录。", true);
    return;
  }
  const currentPassword = document.querySelector("#current-password")?.value || "";
  const newPassword = document.querySelector("#new-password")?.value || "";
  const confirmPassword = document.querySelector("#confirm-password")?.value || "";
  if (newPassword.length < 8) {
    setPasswordStatus("新密码至少 8 位。", true);
    return;
  }
  if (newPassword !== confirmPassword) {
    setPasswordStatus("两次输入的新密码不一致。", true);
    return;
  }
  setPasswordStatus("正在保存...", false);
  try {
    const response = await fetch("/api/v1/auth/change-password", {
      method: "POST",
      headers: {
        "Accept": "application/json",
        "Authorization": `Bearer ${token}`,
        "Content-Type": "application/json"
      },
      body: JSON.stringify({ currentPassword, newPassword })
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`);
    setPasswordStatus("密码已修改，请使用新密码重新登录。", false);
    setTimeout(() => logoutAdmin(), 600);
  } catch (error) {
    setPasswordStatus(error.message || "修改失败", true);
  }
}

function setPasswordStatus(message, isError) {
  if (!passwordStatus) return;
  passwordStatus.textContent = message;
  passwordStatus.classList.toggle("error", Boolean(isError));
}
function logoutAdmin() {
  localStorage.removeItem("token");
  sessionStorage.clear();
  location.href = "/login";
}
