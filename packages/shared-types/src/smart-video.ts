export type SmartVideoProjectStatus =
  | "DRAFT"
  | "ANALYZING"
  | "MATERIAL_READY"
  | "PLANNING"
  | "STORYBOARD_READY"
  | "CONFIRMED"
  | "RENDERING"
  | "COMPLETED"
  | "FAILED";

export type SmartVideoRenderStatus =
  | "CREATED"
  | "QUEUED"
  | "PROCESSING"
  | "SYNTHESIZING"
  | "RENDERING"
  | "UPLOADING"
  | "PUBLISHING"
  | "SUCCEEDED"
  | "FAILED"
  | "CANCELLED";

export type SmartVideoPlanTaskState =
  | "CREATED"
  | "QUEUED"
  | "PROCESSING"
  | "SUCCEEDED"
  | "FAILED";

export type SmartVideoAssetType = "VIDEO" | "IMAGE";

export interface SmartVideoTargetSpec {
  aspectRatio: "9:16" | "16:9" | string;
  resolution: "720p" | "1080p" | string;
  durationMs: number;
}

export interface SmartVideoVoiceConfig {
  enabled: boolean;
  modelKey: string;
  voiceKey: string;
  speed: number;
}

export interface SmartVideoSubtitleConfig {
  enabled: boolean;
  preset: "clean" | "emphasis" | string;
  position: "bottom" | "center" | string;
}

export interface SmartVideoAudioConfig {
  sourceGain: number;
  voiceGain: number;
}

export interface SmartVideoClipV1 {
  assetId: string;
  assetType: SmartVideoAssetType | string;
  sourceInMs: number;
  sourceOutMs: number;
  displayDurationMs: number;
  fitMode: "cover" | "contain" | string;
  motion: "static" | "push" | "pull" | "pan_left" | "pan_right" | string;
  originalAudioGain: number;
}

export interface SmartVideoSceneTransitionV1 {
  type: string;
  durationMs: number;
}

export interface SmartVideoSceneV1 {
  id: string;
  index: number;
  title: string;
  durationMs: number;
  narration: string;
  clips: SmartVideoClipV1[];
  transition: SmartVideoSceneTransitionV1;
}

export interface SmartVideoEditPlanV1 {
  schemaVersion: number;
  title: string;
  summary: string;
  language: "zh-CN" | "en-US" | string;
  target: SmartVideoTargetSpec;
  voice: SmartVideoVoiceConfig;
  subtitles: SmartVideoSubtitleConfig;
  audio: SmartVideoAudioConfig;
  scenes: SmartVideoSceneV1[];
}

export interface SmartVideoAssetMetadata {
  originalName?: string;
  mimeType?: string;
  fileSize?: number;
  width?: number;
  height?: number;
  durationMs?: number;
  fileHash?: string;
}

export interface SmartVideoProject {
  id: string;
  tenantId: string;
  userId: string;
  title: string;
  requirement: string;
  status: SmartVideoProjectStatus | string;
  targetSpec?: SmartVideoTargetSpec;
  currentVersion: number;
  currentVersionId?: string;
  confirmedVersionId?: string;
  activeAnalysisTaskId?: string;
  activePlanTaskId?: string;
  outputAssetId?: string;
  activeRenderTaskId?: string;
  errorStage?: string;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface SmartVideoProjectAsset {
  id: string;
  projectId: string;
  tenantId: string;
  userId: string;
  fileId: string;
  storageKey?: string;
  assetType: SmartVideoAssetType | string;
  sortOrder: number;
  orderIndex?: number;
  durationMs?: number;
  metadata?: SmartVideoAssetMetadata;
  contentAuditStatus?: string;
  analysisStatus?: string;
  thumbnailFileId?: string;
  proxyFileId?: string;
  attemptCount?: number;
  errorCode?: string;
  errorMessage?: string;
  analyzerVersion?: string;
  analysisStartedAt?: string;
  analysisFinishedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SmartVideoProjectVersion {
  id: string;
  projectId: string;
  tenantId: string;
  versionNumber: number;
  source: "ai" | "user" | string;
  parentVersionId?: string;
  planSchemaVersion: number;
  planSnapshot: SmartVideoEditPlanV1;
  renderManifest?: Record<string, unknown>;
  manifestHash?: string;
  plannerModelKey?: string;
  plannerRequestId?: string;
  changeNote?: string;
  status?: string;
  createdBy: string;
  createdAt: string;
}

export interface SmartVideoPlanTask {
  id: string;
  tenantId: string;
  projectId: string;
  userId: string;
  state: SmartVideoPlanTaskState | string;
  instruction?: string;
  sourceVersionId?: string;
  outputVersionId?: string;
  modelKey?: string;
  providerRequestId?: string;
  attempt: number;
  progress: number;
  planSnapshot?: SmartVideoEditPlanV1;
  errorCode?: string;
  errorMessage?: string;
  idempotencyKey?: string;
  createdAt: string;
  updatedAt?: string;
}

export interface SmartVideoRenderSpecification {
  width: number;
  height: number;
  frameRate: number;
  format: string;
  videoCodec: string;
  audioCodec: string;
  durationMs: number;
}

export interface SmartVideoRenderOutput {
  videoFileId?: string;
  coverFileId?: string;
  videoUrl?: string;
  coverUrl?: string;
  durationMs?: number;
  width?: number;
  height?: number;
  frameRate?: number;
  fileSize?: number;
  videoCodec?: string;
  audioCodec?: string;
  pixelFormat?: string;
}

export interface SmartVideoRenderTask {
  id: string;
  projectId: string;
  versionId: string;
  tenantId: string;
  userId: string;
  clientRequestId: string;
  status: SmartVideoRenderStatus | string;
  progress: number;
  step?: string;
  stage?: string;
  attempt?: number;
  attemptCount?: number;
  maxAttempts?: number;
  runAfter?: string;
  specification?: SmartVideoRenderSpecification;
  quotedPoints?: number;
  reservedPoints?: number;
  capturedPoints?: number;
  releasedPoints?: number;
  outputFileId?: string;
  coverFileId?: string;
  outputAssetId?: string;
  voiceFileId?: string;
  captionFileId?: string;
  workId?: string;
  manifestHash?: string;
  retryOfTaskId?: string;
  billingTransactionId?: string;
  output?: SmartVideoRenderOutput;
  errorCode?: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface SmartVideoRenderQuote {
  points: number;
  expiresAt: string;
}

export interface SmartVideoAnalysisSummary {
  projectId: string;
  status: string;
  overallStatus?: string;
  items?: Array<Record<string, unknown>>;
  readyCount?: number;
  failedCount?: number;
  totalCount?: number;
  succeededCount?: number;
  totalAssets?: number;
}

export interface CreateSmartVideoProjectInput {
  title: string;
  requirement: string;
}

export interface UpdateSmartVideoProjectInput {
  title?: string;
  requirement?: string;
}

export interface CreateSmartVideoAssetInput {
  fileId: string;
  assetType: SmartVideoAssetType | string;
  sortOrder?: number;
}

export interface ReorderSmartVideoAssetsInput {
  assetIds: string[];
}

export interface CreateSmartVideoPlanTaskInput {
  instruction?: string;
  regenerateFromVersionId?: string;
  idempotencyKey?: string;
  modelKey?: string;
}

export interface ReviseSmartVideoPlanInput {
  plan: SmartVideoEditPlanV1;
  changeNote?: string;
}

export interface CreateSmartVideoExportInput {
  versionId: string;
  idempotencyKey?: string;
}
