import type { AppRole } from "../../types";
import type { PromotionTemplate, PromotionTemplateId } from "./types";

const commonRoles: AppRole[] = ["USER", "AGENT", "OPERATION", "ENTERPRISE_ADMIN", "AI_ADMIN", "FINANCE", "CUSTOMER_SERVICE", "ENTERPRISE_MEMBER"];
const qrPosition = { x: 708, y: 1112, size: 244 };
const inviterPosition = { x: 120, y: 1160 };

function item(input: Omit<PromotionTemplate, "qrPosition" | "inviterPosition" | "allowedRoles"> & { allowedRoles?: AppRole[] }): PromotionTemplate {
  return { ...input, qrPosition: { ...qrPosition }, inviterPosition: { ...inviterPosition }, allowedRoles: [...(input.allowedRoles || commonRoles)] };
}

export const promotionTemplates: PromotionTemplate[] = [
  item({ id: "poster.brand.simple", name: "品牌极简", category: "brand", categoryLabel: "品牌推广", background: "#F3F6FF", primaryColor: "#7D8DF6", secondaryColor: "#5A4DB2", title: "知启云AI，让创意更高效", subtitle: "AI创作、知识与团队协作一站完成", badge: "品牌推荐", description: "高级留白与品牌聚焦", featureItems: ["AI智能创作", "企业级安全", "多端协同"], layout: "brand-focus" }),
  item({ id: "poster.product.features", name: "产品能力", category: "product", categoryLabel: "产品推广", background: "#EEF2FF", primaryColor: "#5D5FEF", secondaryColor: "#132A77", title: "一套平台，释放团队AI生产力", subtitle: "从灵感到作品，流程更清晰", badge: "核心能力", description: "能力卡片与产品亮点", featureItems: ["图片与视频", "PPT与信息图", "AI员工"], layout: "feature-grid" }),
  item({ id: "poster.invite.reward", name: "邀请有礼", category: "invite", categoryLabel: "邀新活动", background: "#FFF5EC", primaryColor: "#FF771B", secondaryColor: "#B84A08", title: "邀请好友，一起开启AI创作", subtitle: "完成注册即可体验丰富AI能力", badge: "邀请有礼", description: "橙色活动视觉与奖励提示", featureItems: ["扫码即达", "专属邀请码", "活动奖励"], layout: "reward-card" }),
  item({ id: "poster.enterprise.brand", name: "企业品牌", category: "brand", categoryLabel: "品牌推广", background: "#F0F3FA", primaryColor: "#5A4DB2", secondaryColor: "#132A77", title: "企业级AI内容生产平台", subtitle: "统一品牌资产，沉淀组织知识", badge: "企业方案", description: "深色品牌面板与企业信息", featureItems: ["组织权限", "品牌一致", "数据治理"], layout: "enterprise-split" }),
  item({ id: "poster.scene.marketing", name: "营销场景", category: "product", categoryLabel: "产品推广", background: "#F2F7FF", primaryColor: "#7D8DF6", secondaryColor: "#325AA8", title: "让每一次营销都有AI助力", subtitle: "海报、视频、朋友圈与PPT快速交付", badge: "营销提效", description: "多场景内容生产流程", featureItems: ["新品发布", "社媒传播", "销售物料"], layout: "scene-stack" }),
  item({ id: "poster.industry.solution", name: "行业方案", category: "industry", categoryLabel: "行业方案", background: "#ECF7F6", primaryColor: "#3F9E91", secondaryColor: "#185E55", title: "面向行业的AI解决方案", subtitle: "连接知识、内容与业务场景", badge: "行业精选", description: "行业数据面板与方案表达", featureItems: ["场景适配", "知识增强", "安全可控"], layout: "industry-panel" }),
  item({ id: "poster.case.study", name: "客户案例", category: "industry", categoryLabel: "行业方案", background: "#F8F4EE", primaryColor: "#B88A58", secondaryColor: "#694523", title: "真实案例，见证AI业务价值", subtitle: "用可复用的方法加速团队落地", badge: "客户案例", description: "案例引语与成果指标", featureItems: ["效率提升", "成本优化", "持续增长"], layout: "case-quote" }),
  item({ id: "poster.trial.limited", name: "限时体验", category: "campaign", categoryLabel: "活动推广", background: "#FFF0E8", primaryColor: "#FF771B", secondaryColor: "#9A3500", title: "限时开放，立即体验知启云AI", subtitle: "把握体验机会，快速完成第一份作品", badge: "限时体验", description: "活动倒计时视觉", featureItems: ["快速上手", "多种模板", "随时创作"], layout: "campaign-countdown" }),
  item({ id: "poster.partner.recruit", name: "伙伴招募", category: "invite", categoryLabel: "伙伴招募", background: "#F3F0FF", primaryColor: "#7D8DF6", secondaryColor: "#5A4DB2", title: "携手知启云AI，共创增长", subtitle: "加入推广伙伴，拓展企业AI市场", badge: "伙伴招募", description: "伙伴权益与合作步骤", featureItems: ["推广支持", "客户管理", "分润透明"], layout: "partner-steps", allowedRoles: ["AGENT", "OPERATION", "ENTERPRISE_ADMIN"] }),
  item({ id: "poster.festival.campaign", name: "节日活动", category: "campaign", categoryLabel: "活动推广", background: "#FFF3F4", primaryColor: "#E75D70", secondaryColor: "#8F2436", title: "节日灵感，由AI即刻点亮", subtitle: "定制节日内容，传递品牌心意", badge: "节日限定", description: "节日边框与社媒氛围", featureItems: ["节日海报", "品牌祝福", "社媒传播"], layout: "festival-frame" }),
];

export const promotionTemplateIds = promotionTemplates.map(item => item.id);

export function promotionTemplateById(id?: string): PromotionTemplate {
  return promotionTemplates.find(item => item.id === id) || promotionTemplates[0];
}

export function promotionTemplateIdBySceneIndex(index: string | number): PromotionTemplateId {
  const value = Math.max(1, Number(index) || 1) - 1;
  return promotionTemplates[value]?.id || "poster.brand.simple";
}

export function promotionTemplatesForRole(role: AppRole) {
  return promotionTemplates.filter(item => item.allowedRoles.includes(role));
}
