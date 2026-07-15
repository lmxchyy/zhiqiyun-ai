export type AssetType =
  | "all"
  | "image"
  | "video"
  | "ppt"
  | "document"
  | "agent"
  | "infographic"
  | "knowledge"
  | "prompt"
  | "template";

export type AssetStatus =
  | "recent"
  | "queued"
  | "generating"
  | "completed"
  | "failed"
  | "favorite"
  | "archived"
  | "recycled";

export type AssetSort =
  | "created_desc"
  | "created_asc"
  | "updated_desc"
  | "name_asc"
  | "size_desc"
  | "usage_desc";

export interface AssetFilter {
  type: AssetType;
  status: AssetStatus;
  keyword: string;
  projectId: string;
  tagIds: string[];
  model: string;
  createdFrom: string;
  createdTo: string;
  favorite?: boolean;
}

export interface AssetOverview {
  total: number;
  monthTotal: number;
  favoriteTotal: number;
  storageBytes: number;
  storageQuotaBytes: number;
  storageUsagePercent: number;
}

export interface AssetItem {
  id: string;
  taskId?: string;
  name: string;
  type: Exclude<AssetType, "all">;
  status: Exclude<AssetStatus, "recent" | "favorite">;
  remoteUrl: string;
  fallbackUrl: string;
  thumbnailUrl: string;
  projectId: string;
  projectName: string;
  favorite: boolean;
  archived: boolean;
  deletedAt: string;
  createdAt: string;
  updatedAt: string;
  tags: string[];
  model: string;
  prompt: string;
  negativePrompt: string;
  fileSize: number;
  width?: number;
  height?: number;
  duration?: number;
  pageCount?: number;
  documentCount?: number;
  aspectRatio: string;
  seed?: string | number;
  tokenCost?: number;
  pointCost?: number;
  generationDurationMs?: number;
  usageCount?: number;
  metadata: Record<string, unknown>;
}

export interface AssetDetail extends AssetItem {
  downloadUrl: string;
  shareUrl: string;
  variables: Record<string, unknown>;
}

export interface AssetPagination {
  page: number;
  pageSize: number;
  total: number;
  hasMore: boolean;
}

export type GenerationTaskStatus = "queued" | "generating" | "completed" | "failed" | "cancelled";

export interface GenerationTask {
  id: string;
  name: string;
  type: Exclude<AssetType, "all">;
  status: GenerationTaskStatus;
  progress: number;
  createdAt: string;
  updatedAt: string;
  failureReason: string;
  resultIds: string[];
  prompt: string;
  model: string;
  params: Record<string, unknown>;
}

export type BatchAssetAction = "download" | "favorite" | "unfavorite" | "move" | "archive" | "delete" | "restore" | "permanent";

export interface ProjectOption {
  id: string;
  name: string;
  assetCount?: number;
}

export interface AssetPageResponse {
  items: AssetItem[];
  total: number;
  page: number;
  pageSize: number;
  hasMore: boolean;
  overview?: AssetOverview;
}

export interface TaskPageResponse {
  items: GenerationTask[];
  total: number;
  page: number;
  pageSize: number;
  hasMore: boolean;
}

export interface AssetBatchPayload {
  action: BatchAssetAction;
  ids: string[];
  projectId?: string;
  projectName?: string;
}

export const assetTypeOptions: Array<{ value: AssetType; label: string }> = [
  { value: "all", label: "全部" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "ppt", label: "PPT" },
  { value: "document", label: "文档" },
  { value: "agent", label: "Agent" },
  { value: "infographic", label: "信息图" },
  { value: "knowledge", label: "知识库" },
  { value: "prompt", label: "Prompt" },
  { value: "template", label: "模板" },
];

export const assetStatusOptions: Array<{ value: AssetStatus; label: string }> = [
  { value: "recent", label: "最近" },
  { value: "queued", label: "排队中" },
  { value: "generating", label: "生成中" },
  { value: "completed", label: "已完成" },
  { value: "failed", label: "失败" },
  { value: "favorite", label: "收藏" },
  { value: "archived", label: "已归档" },
  { value: "recycled", label: "回收站" },
];

export const assetSortOptions: Array<{ value: AssetSort; label: string }> = [
  { value: "created_desc", label: "最新创建" },
  { value: "created_asc", label: "最早创建" },
  { value: "updated_desc", label: "最近更新" },
  { value: "name_asc", label: "名称排序" },
  { value: "size_desc", label: "文件大小" },
  { value: "usage_desc", label: "使用次数" },
];

export function defaultAssetFilter(): AssetFilter {
  return {
    type: "all",
    status: "recent",
    keyword: "",
    projectId: "",
    tagIds: [],
    model: "",
    createdFrom: "",
    createdTo: "",
  };
}
