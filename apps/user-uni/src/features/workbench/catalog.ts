import type { MiniProgramCreationMode, MiniProgramRoleId, MiniProgramTabId } from "../../config/miniProgramPages";

export type WorkbenchAssetFilter = "all" | "image" | "video" | "document" | "favorite";

export interface WorkbenchCreationModule {
  id: MiniProgramCreationMode;
  icon: string;
  name: string;
  homeName: string;
  description: string;
  model: string;
  cost: string;
  tone: "orange" | "purple" | "green" | "blue";
}

export const roleTabs: Record<MiniProgramRoleId, Array<{ id: MiniProgramTabId; label: string; icon: string }>> = {
  user: [
    { id: "home", label: "首页", icon: "⌂" },
    { id: "create", label: "创作", icon: "＋" },
    { id: "assets", label: "作品", icon: "▣" },
    { id: "mine", label: "我的", icon: "○" },
  ],
  agent: [
    { id: "overview", label: "概览", icon: "总" },
    { id: "promotion", label: "推广", icon: "推" },
    { id: "customers", label: "客户", icon: "客" },
    { id: "commission", label: "分润", icon: "润" },
    { id: "mine", label: "我的", icon: "我" },
  ],
  operation: [
    { id: "overview", label: "概览", icon: "总" },
    { id: "agents", label: "代理", icon: "代" },
    { id: "orders", label: "订单", icon: "单" },
    { id: "commission", label: "分润", icon: "润" },
    { id: "mine", label: "我的", icon: "我" },
  ],
};

export const roleNames: Record<MiniProgramRoleId, string> = {
  user: "普通用户",
  agent: "代理商",
  operation: "运营中心",
};

export const creationModules: WorkbenchCreationModule[] = [
  { id: "image", icon: "图", name: "AI生图", homeName: "轻易海报", description: "主图/海报/配图", model: "gpt-image-2", cost: "约 10 点/张", tone: "orange" },
  { id: "ppt", icon: "P", name: "PPT文档", homeName: "PPT文档", description: "方案/培训/路演", model: "ppt-generator", cost: "约 30 点/份", tone: "purple" },
  { id: "video", icon: "视", name: "视频生成", homeName: "视频生成", description: "广告/口播/图生视频", model: "grok-imagine-1.5-video", cost: "约 15 积分/秒", tone: "green" },
  { id: "agent", icon: "星", name: "AI Agent", homeName: "LOGO", description: "经营助手与知识库", model: "agent-workflow", cost: "按任务计费", tone: "blue" },
  { id: "infographic", icon: "表", name: "自由P图", homeName: "自由P图", description: "杂志封面/去除路人/精致补妆", model: "infographic", cost: "约 20 点/份", tone: "orange" },
  { id: "review", icon: "查", name: "易找茬", homeName: "易共识", description: "多模型判断与风险", model: "multi-model", cost: "按模型计费", tone: "purple" },
];

export const pptTopics = ["企业营销增长", "数字员工方案", "GEO品牌曝光", "短视频矩阵", "项目路演计划", "糖尿病患教"] as const;

export const assetFilters: Array<{ id: WorkbenchAssetFilter; label: string }> = [
  { id: "all", label: "全部" },
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "document", label: "PPT" },
  { id: "favorite", label: "收藏" },
];

export const agentWorkbenchTabs = new Set<MiniProgramTabId>(["overview", "promotion", "customers", "commission", "mine"]);
export const operationWorkbenchTabs = new Set<MiniProgramTabId>(["overview", "agents", "orders", "commission", "mine"]);

export function homeModuleSlot(mode: MiniProgramCreationMode) {
  return ({
    image: "home.quick.poster",
    ppt: "home.quick.ppt",
    video: "home.quick.video",
    agent: "home.quick.knowledge",
    infographic: "home.capability.office",
    review: "home.capability.employee",
    montage: "home.capability.montage",
  } as Record<MiniProgramCreationMode, string>)[mode];
}

export function studioModuleSlot(mode: MiniProgramCreationMode) {
  return ({
    image: "studio.template.poster",
    ppt: "studio.template.ppt",
    video: "studio.template.video",
    agent: "studio.template.knowledge",
    infographic: "studio.template.office",
    review: "studio.template.employee",
    montage: "home.capability.montage",
  } as Record<MiniProgramCreationMode, string>)[mode];
}

export function assetDefaultSlot(mediaType: string) {
  if (mediaType === "image") return "assets.default.image";
  if (mediaType === "video") return "assets.default.video";
  if (mediaType === "document") return "assets.default.document";
  return "assets.default.other";
}
