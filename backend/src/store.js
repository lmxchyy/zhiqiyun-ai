import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const dataDir = path.join(root, "data");
const storeFile = path.join(dataDir, "store.json");

const now = () => new Date().toISOString();

function initialState() {
  return {
    users: [],
    sessions: [],
    enterprises: [],
    enterpriseMembers: [],
    enterpriseQuotaTransactions: [],
    enterpriseAssetShares: [],
    enterpriseAgentShares: [],
    plans: [
      { id: "plan_free", name: "免费会员", price: 0, points: 100, durationDays: 36500, concurrency: 1 },
      { id: "plan_month", name: "月度会员", price: 9900, points: 3000, durationDays: 30, concurrency: 3 },
      { id: "plan_year", name: "年度会员", price: 89900, points: 50000, durationDays: 365, concurrency: 8 }
    ],
    pointAccounts: [],
    pointTransactions: [],
    generationTasks: [],
    generationAttempts: [],
    modelCallLogs: [],
    modelProviders: [],
    modelDefinitions: [],
    modelPricingRules: [],
    assets: [],
    moderationLogs: [],
    orders: [],
    payments: [],
    invoices: [],
    coupons: [],
    userCoupons: [],
    redemptionCodes: [],
    redemptionUses: [],
    paymentEvents: [],
    refunds: [],
    channelAgents: [],
    commissions: [],
    channelPerformanceSnapshots: [],
    withdrawals: [],
    settlementStatements: [],
    presentations: [],
    agents: [],
    agentVersions: [],
    agentCalls: [],
    agentShares: [],
    agentFeedback: [],
    knowledgeBases: [],
    knowledgeDocuments: [],
    geoBrands: [],
    geoTasks: [],
    geoSchedules: [],
    geoReports: [],
    geoContents: [],
    geoContentPublications: [],
    auditLogs: [],
    sensitiveTerms: ["违法", "暴力犯罪", "色情内容", "仇恨言论"],
    counters: {}
  };
}

export class Store {
  constructor({ persist = true } = {}) {
    this.persist = persist;
    this.state = initialState();
    if (persist) {
      fs.mkdirSync(dataDir, { recursive: true });
      this.refresh();
    }
  }

  refresh() {
    if (!this.persist || !fs.existsSync(storeFile)) return this.state;
    try {
      const loaded = JSON.parse(fs.readFileSync(storeFile, "utf8"));
      this.state = { ...initialState(), ...loaded };
    } catch (error) {
      if (!(error instanceof SyntaxError)) throw error;
    }
    return this.state;
  }

  save() {
    if (!this.persist) return;
    const temp = `${storeFile}.tmp`;
    fs.writeFileSync(temp, JSON.stringify(this.state, null, 2), "utf8");
    fs.renameSync(temp, storeFile);
  }

  id(prefix) {
    this.state.counters[prefix] = (this.state.counters[prefix] || 0) + 1;
    return `${prefix}_${String(this.state.counters[prefix]).padStart(6, "0")}`;
  }

  insert(collection, value) {
    const item = { ...value, createdAt: value.createdAt || now(), updatedAt: now() };
    this.state[collection].push(item);
    this.save();
    return item;
  }

  update(collection, id, changes) {
    const item = this.state[collection].find((entry) => entry.id === id);
    if (!item) return null;
    Object.assign(item, changes, { updatedAt: now() });
    this.save();
    return item;
  }

  audit(actorId, action, targetType, targetId, detail = {}) {
    return this.insert("auditLogs", {
      id: this.id("audit"), actorId, action, targetType, targetId, detail
    });
  }
}

export const createStore = (options) => new Store(options);
