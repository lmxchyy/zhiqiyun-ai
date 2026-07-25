import type { AppRole } from "../../types";

export type PromotionTemplateId =
  | "poster.brand.simple"
  | "poster.product.features"
  | "poster.invite.reward"
  | "poster.enterprise.brand"
  | "poster.scene.marketing"
  | "poster.industry.solution"
  | "poster.case.study"
  | "poster.trial.limited"
  | "poster.partner.recruit"
  | "poster.festival.campaign";

export type PromotionLayout =
  | "brand-focus"
  | "feature-grid"
  | "reward-card"
  | "enterprise-split"
  | "scene-stack"
  | "industry-panel"
  | "case-quote"
  | "campaign-countdown"
  | "partner-steps"
  | "festival-frame";

export interface PromotionTemplate {
  id: PromotionTemplateId;
  name: string;
  category: string;
  categoryLabel: string;
  allowedRoles: AppRole[];
  background: string;
  primaryColor: string;
  secondaryColor: string;
  title: string;
  subtitle: string;
  badge: string;
  description: string;
  featureItems: string[];
  layout: PromotionLayout;
  qrPosition: { x: number; y: number; size: number };
  inviterPosition: { x: number; y: number };
}

export interface PromotionSummary {
  visitCount: number;
  registerCount: number;
  paidCount: number;
  rewardAmountCents: number;
  registerRate: number;
  paidRate: number;
}

export interface PromotionProfile {
  userId: string;
  tenantId: string;
  organizationId: string;
  name: string;
  avatarUrl: string;
  companyName: string;
  currentRole: AppRole;
  roleLabel: string;
  roles: AppRole[];
  inviteCode: string;
  summary: PromotionSummary;
}

export interface PromotionActivity {
  id: string;
  name: string;
  badge: string;
  description: string;
  status: string;
  startsAt?: string;
  endsAt?: string;
}

export interface PromotionOverview {
  profile: PromotionProfile;
  summary: PromotionSummary;
  featuredTemplates: PromotionTemplate[];
  defaultTemplateId: PromotionTemplateId;
  activity: PromotionActivity;
}

export interface PromotionCode {
  imageDataUrl: string;
  scene: string;
  page: string;
  isPlaceholder: boolean;
  cacheKey: string;
  expiresAt: string;
}

export interface PromotionRecord {
  id: string;
  tenantId: string;
  inviterUserId: string;
  inviteeUserId?: string;
  visitorId?: string;
  visitorName?: string;
  maskedMobile?: string;
  inviteCode: string;
  status: "visited" | "registered" | "paid" | "invalid";
  source: string;
  templateId: PromotionTemplateId;
  activityId?: string;
  visitTime?: string;
  registerTime?: string;
  paidTime?: string;
  rewardAmountCents: number;
  rewardStatus?: string;
  createdAt: string;
  updatedAt: string;
}

export interface PromotionRecordsResponse {
  items: PromotionRecord[];
  total: number;
  page: number;
  pageSize: number;
  hasMore: boolean;
}

export interface PromotionTrendItem {
  date: string;
  visitCount: number;
  registerCount: number;
  paidCount: number;
}

export interface PromotionChannelItem {
  source: string;
  label: string;
  count: number;
}

export interface PromotionAnalytics {
  summary: PromotionSummary;
  trend: PromotionTrendItem[];
  channels: PromotionChannelItem[];
  days: number;
}

export interface PromotionShareCopy {
  title: string;
  description: string;
  text: string;
  path: string;
}

export interface AgentInviteFunnel {
  pageViews: number;
  registered: number;
  downloads: number;
  activations: number;
}

export interface AgentInviteProfile {
  inviteCode: string;
  inviteLink: string;
  agentDisplayName: string;
  agentStatus: string;
  funnel: AgentInviteFunnel;
}

export interface AgentInvitePoster {
  inviteCode: string;
  inviteLink: string;
  qrCodeDataUrl: string;
  poster: {
    title: string;
    subtitle: string;
    width: number;
    height: number;
    format: "png";
  };
  funnel: AgentInviteFunnel;
}

export interface PromotionReferral {
  inviteCode: string;
  templateId: PromotionTemplateId;
  activityId: string;
  source: string;
  capturedAt: number;
}
