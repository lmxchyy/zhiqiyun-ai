export type TaskStatus = "QUEUED" | "PROCESSING" | "RETRYING" | "SUCCEEDED" | "FAILED" | "CANCELLED";

export interface GenerationTask {
  id: string;
  type: "TEXT_TO_IMAGE" | "IMAGE_TO_IMAGE" | "TEXT_TO_VIDEO" | "IMAGE_TO_VIDEO";
  status: TaskStatus;
  prompt: string;
  model: string;
  pointCost: number;
  resultIds?: string[];
  params?: Record<string, unknown>;
  createdAt?: string;
  updatedAt?: string;
  workerFinishedAt?: string;
}

export interface Asset {
  id: string;
  name: string;
  url: string;
  mediaType: "image" | "video" | "document";
  taskId?: string;
  metadata?: Record<string, unknown>;
}

export interface ReferenceImage {
  path: string;
  name: string;
  sourceAssetId?: string;
}

export interface ModelInfo {
  code: string;
  name: string;
  capabilities?: string[];
}

export interface PointAccount {
  id: string;
  userId: string;
  available: number;
  frozen: number;
}

export interface PointAccountResponse {
  account: PointAccount;
  transactions?: unknown[];
}
