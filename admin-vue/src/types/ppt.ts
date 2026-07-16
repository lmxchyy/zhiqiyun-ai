import type { Component } from "vue";

export type PptCreateMode = "ai" | "blank" | "document";
export type PptLanguage = "zh" | "en";
export type PptTone = "professional" | "simple" | "marketing" | "education" | "pitch";
export type PptTextContent = "minimal" | "concise" | "detailed" | "extensive";
export type PptAudience = "auto" | "general" | "business" | "investor" | "teacher" | "student";
export type PptScenario = "auto" | "general" | "analysis-report" | "teaching-training" | "promotional-materials" | "public-speeches";
export type PptImageSource = "ai" | "stock" | "none";
export type PptGenerationAspectRatio = "dynamic" | "16:9";
export type PptTheme =
  | "business"
  | "techBlue"
  | "education"
  | "marketing"
  | "pitch"
  | "medical"
  | "government"
  | "blackGold"
  | "freshGreen"
  | "noir"
  | "indigo"
  | "orbit"
  | "cosmos"
  | "piano"
  | "ebony"
  | "mystique"
  | "phantom"
  | "ember"
  | "sunset"
  | "dusk"
  | "canopy"
  | "aurora"
  | "borealis"
  | "sakura"
  | "midnight"
  | "abyss"
  | "mint"
  | "jade"
  | "rose"
  | "wine"
  | "arctic"
  | "glacier"
  | "honey"
  | "amber"
  | "coral"
  | "magma"
  | "lavender"
  | "velvet";
export type PptWorkflowStatus =
  | "idle"
  | "pending"
  | "outlining"
  | "outline_ready"
  | "generating"
  | "rendering"
  | "success"
  | "failed";
export type PptTaskStatus = Exclude<PptWorkflowStatus, "idle" | "outline_ready" | "outlining"> | "processing" | "draft";
export type PptSlideLayout = "cover" | "section" | "content" | "imageText" | "summary";
export type PptSlideType = "cover" | "section" | "statement" | "text_image" | "case_study" | "product_showcase" | "industry_scene" | "agenda" | "feature_grid" | "process" | "timeline" | "comparison" | "data_chart" | "swot" | "matrix" | "organization" | "table";
export type PptVisualType = "scene" | "illustration" | "product" | "office" | "icon" | "chart" | "diagram" | "none";
export type PptVisualComposition = "image_left" | "image_right" | "full_width" | "background" | "card";

export interface PptVisualPlan {
  visualType: PptVisualType | string;
  imageRequired: boolean;
  chartRequired: boolean;
  diagramRequired: boolean;
  textInImage: false;
  subject: string;
  scene: string;
  action: string;
  objects: string[];
  mood: string;
  composition: string;
  style: string;
  prompt: string;
  negativePrompt: string;
}

export interface PptVisualAsset {
  url: string;
  storageRef?: string;
  taskId?: string;
  modelName?: string;
  createdAt: string;
}

export interface PptModelOption {
  label: string;
  value: string;
  provider?: string;
  providerType?: "openai" | "ollama" | "lmstudio" | "system" | "newapi" | "comfyui" | "other";
  group?: string;
  description?: string;
  downloadable?: boolean;
  disabled?: boolean;
}

export interface PptThemeOption {
  label: string;
  value: PptTheme;
  description: string;
  colors: string[];
}

export interface PptOutlineSlide {
  page: number;
  title: string;
  summary: string;
  bulletPoints: string[];
  layout?: PptSlideLayout;
  slideType?: PptSlideType;
}

export interface PptOutline {
  title: string;
  slides: PptOutlineSlide[];
  updatedAt?: string;
}

export interface PptSlide {
  id: string;
  page: number;
  title: string;
  content: string;
  bulletPoints: string[];
  imageUrl?: string;
  visualStorageRef?: string;
  layout: PptSlideLayout;
  speakerNotes?: string;
  slideType?: PptSlideType;
  visualPlan?: PptVisualPlan;
  visualHistory?: PptVisualAsset[];
  visualTaskId?: string;
  visualModelName?: string;
  visualCreatedAt?: string;
  visualStatus?: "planned" | "processing" | "success" | "failed" | string;
  visualError?: string;
}

export interface PptGenerateOutlineRequest {
  prompt: string;
  slideCount: number;
  language: PptLanguage;
  tone: PptTone;
  textContent: PptTextContent;
  audience: PptAudience;
  scenario: PptScenario;
  generationAspectRatio: PptGenerationAspectRatio;
  autoThemeEnabled: boolean;
  enableWebSearch: boolean;
  textModel: string;
  imageSource: PptImageSource;
  imageModel: string;
}

export interface PptGenerateRequest extends PptGenerateOutlineRequest {
  theme: PptTheme;
  outline?: PptOutline;
  imageStyle?: string;
  peopleStyle?: string;
  imageLighting?: string;
  imageComposition?: PptVisualComposition;
  textInImage?: false;
}

export interface PptGenerateImageRequest {
  slide: PptSlide;
  prompt: string;
  deckTitle?: string;
  theme: PptTheme;
  language: PptLanguage;
  imageSource: PptImageSource;
  imageModel: string;
  visualPlan?: PptVisualPlan;
  imageStyle?: string;
  peopleStyle?: string;
  imageLighting?: string;
  imageComposition?: PptVisualComposition;
}

export interface PptRegenerateVisualRequest {
  visualType: PptVisualType | string;
  style: string;
  composition: PptVisualComposition;
  customInstruction: string;
  keepCurrentContent: true;
}

export interface PptRegenerateVisualResponse {
  taskId?: string;
  status: string;
  slide: PptSlide;
}

export interface PptGenerateResponse {
  taskId: string;
  status: PptTaskStatus;
}

export interface PptTaskResponse {
  taskId: string;
  type?: "ppt";
  mediaType?: "ppt";
  status: PptTaskStatus;
  title: string;
  prompt?: string;
  slideCount?: number;
  language?: PptLanguage;
  tone?: PptTone;
  textContent?: PptTextContent;
  audience?: PptAudience;
  scenario?: PptScenario;
  theme?: PptTheme;
  generationAspectRatio?: PptGenerationAspectRatio;
  autoThemeEnabled?: boolean;
  imageSource?: PptImageSource;
  textModel?: string;
  imageModel?: string;
  imageStyle?: string;
  peopleStyle?: string;
  imageLighting?: string;
  imageComposition?: PptVisualComposition;
  textInImage?: false;
  enableWebSearch?: boolean;
  progress?: number;
  currentPage?: number;
  outline?: PptOutline;
  slides?: PptSlide[];
  pptUrl: string;
  pdfUrl: string;
  errorMessage: string;
  createdAt?: string;
  updatedAt?: string;
}

export type PptHistoryItem = PptTaskResponse;

export interface PptImageOption {
  id: string;
  url: string;
  title: string;
  source: "ai" | "stock";
}

export interface PptCreateModeOption {
  value: PptCreateMode;
  title: string;
  description: string;
  badge: string;
  icon: Component;
}

export interface PptErrorInfo {
  title: string;
  message: string;
}
