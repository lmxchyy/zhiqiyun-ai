export type InspirationContentType = "image" | "video" | "ppt" | "text" | "agent" | "workflow";

export interface InspirationCategory {
  id: string;
  code: string;
  name: string;
  sort: number;
}

export type TemplateInputType =
  | "TEXT"
  | "TEXTAREA"
  | "NUMBER"
  | "SELECT"
  | "MULTI_SELECT"
  | "BOOLEAN"
  | "IMAGE"
  | "VIDEO"
  | "FILE";

export type TemplateInputControl =
  | "TEXT"
  | "TEXTAREA"
  | "SELECT"
  | "MULTI_SELECT"
  | "SEGMENTED"
  | "BOOLEAN"
  | "NUMBER"
  | "SLIDER"
  | "ASSET_UPLOAD";

export type TemplateFormSection = "materials" | "requirements" | "preferences" | "advanced";

export interface PublicTemplateInputOption {
  label: string;
  value: unknown;
}

export interface PublicTemplateInputValidation {
  minLength?: number;
  maxLength?: number;
  min?: number;
  max?: number;
  minItems?: number;
  maxItems?: number;
  pattern?: string;
  accept?: string[];
}

export interface PublicTemplateVisibilityCondition {
  inputKey: string;
  operator: string;
  value?: unknown;
}

export interface PublicTemplateInput {
  key: string;
  type: TemplateInputType;
  control?: TemplateInputControl;
  label: string;
  required?: boolean;
  helpText?: string;
  placeholder?: string;
  default?: unknown;
  options?: PublicTemplateInputOption[];
  validation?: PublicTemplateInputValidation;
  visibleWhen?: PublicTemplateVisibilityCondition;
  section?: TemplateFormSection;
  order?: number;
  advanced?: boolean;
}

export interface PublicTemplateDefinition {
  inputs: PublicTemplateInput[];
  presentation: Record<string, unknown>;
  presets: { inputDefaults?: Record<string, unknown> };
  handoff: { targetType: string };
}

export interface InspirationTemplate {
  id: string;
  slug: string;
  title: string;
  description: string;
  contentType: InspirationContentType;
  categoryId: string;
  categoryCode?: string;
  categoryName?: string;
  coverUrl: string;
  thumbnailUrl?: string;
  resultUrl?: string;
  platforms?: string[];
  tags?: string[];
  featured?: boolean;
  hot?: boolean;
  pinned?: boolean;
  sort?: number;
  templateVersion: number;
  favorite: boolean;
  viewCount: number;
  copyCount?: number;
  favoriteCount: number;
  useCount: number;
  generateCount: number;
}

export interface PublicTemplateDetail extends InspirationTemplate {
  schema: PublicTemplateDefinition;
}

export interface InspirationDetailResponse {
  item: PublicTemplateDetail;
  aiGenerated: boolean;
}

export interface TemplateUploadedAsset {
  assetId: string;
  name?: string;
  previewUrl?: string;
  localPath?: string;
  mimeType?: string;
  status: "uploading" | "uploaded" | "failed";
  error?: string;
}

export type TemplateAssetValues = Record<string, TemplateUploadedAsset[]>;

export interface CreationDraftTemplateRef {
  id: string;
  slug: string;
  version: number;
}

export interface CreationDraftMaterial {
  inputKey: string;
  assetId: string;
}

export interface InspirationCreationDraft {
  contractVersion: number;
  templateRef: CreationDraftTemplateRef;
  contentType: InspirationContentType;
  handoff: { targetType: string; targetKey?: string; intentKey?: string };
  values: Record<string, unknown>;
  materials: CreationDraftMaterial[];
  basePrompt: string;
  negativePrompt?: string;
  parameters: Record<string, unknown>;
  capabilityKey: string;
  modelHint?: string;
  integrityToken: string;
  createdAt: string;
  expiresAt: string;
}

export interface InspirationComposeRequest {
  templateVersion: number;
  values: Record<string, unknown>;
  materials: CreationDraftMaterial[];
}

export interface InspirationComposeResponse {
  draft: InspirationCreationDraft;
}
