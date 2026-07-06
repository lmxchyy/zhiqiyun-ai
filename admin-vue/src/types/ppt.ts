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
  layout: PptSlideLayout;
  speakerNotes?: string;
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
}

export interface PptGenerateImageRequest {
  slide: PptSlide;
  prompt: string;
  deckTitle?: string;
  theme: PptTheme;
  language: PptLanguage;
  imageSource: PptImageSource;
  imageModel: string;
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
