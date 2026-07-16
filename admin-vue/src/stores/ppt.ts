import { defineStore } from "pinia";
import {
  createPptTask,
  deletePptSlideVisual,
  deletePptTask,
  exportPdf,
  exportPpt,
  generatePptImage,
  generatePptOutline,
  getPptHistory,
  getPptImageModels,
  getPptTaskStatus,
  getPptTextModels,
  regeneratePptSlide,
  regeneratePptSlideVisual,
  restorePptSlideVisual,
  searchPptImages,
  updatePptOutline,
  upsertPptDraft
} from "../api/ppt";
import type {
  PptCreateMode,
  PptErrorInfo,
  PptGenerationAspectRatio,
  PptHistoryItem,
  PptImageOption,
  PptImageSource,
  PptLanguage,
  PptModelOption,
  PptOutline,
  PptOutlineSlide,
  PptAudience,
  PptScenario,
  PptSlide,
  PptTextContent,
  PptTheme,
  PptTone,
  PptVisualComposition,
  PptVisualPlan,
  PptWorkflowStatus
} from "../types/ppt";

interface PptState {
  prompt: string;
  createMode: PptCreateMode;
  slideCount: number;
  language: PptLanguage;
  tone: PptTone;
  textContent: PptTextContent;
  audience: PptAudience;
  scenario: PptScenario;
  generationAspectRatio: PptGenerationAspectRatio;
  theme: PptTheme;
  autoThemeEnabled: boolean;
  enableWebSearch: boolean;
  imageSource: PptImageSource;
  textModel: string;
  imageModel: string;
  imageStyle: string;
  peopleStyle: string;
  imageLighting: string;
  imageComposition: PptVisualComposition;
  outline: PptOutline | null;
  slides: PptSlide[];
  currentSlideIndex: number;
  taskId: string;
  status: PptWorkflowStatus;
  progress: number;
  currentPage: number;
  history: PptHistoryItem[];
  textModels: PptModelOption[];
  imageModels: PptModelOption[];
  selectedExample: string;
  uploadedDocumentName: string;
  imageSearchKeyword: string;
  imageSearchResults: PptImageOption[];
  imageGenerating: boolean;
  visualOperationStatus: "idle" | "loading" | "success" | "failed";
  error: PptErrorInfo | null;
  initialized: boolean;
  historyLoadedAt: number;
}

let pptInitializePromise: Promise<void> | null = null;

export const usePptStore = defineStore("ppt", {
  state: (): PptState => ({
    prompt: "",
    createMode: "ai",
    slideCount: 5,
    language: "zh",
    tone: "professional",
    textContent: "concise",
    audience: "auto",
    scenario: "auto",
    generationAspectRatio: "dynamic",
    theme: "business",
    autoThemeEnabled: true,
    enableWebSearch: false,
    imageSource: "ai",
    textModel: "kimi-k2.6",
    imageModel: "default-image",
    imageStyle: "modern enterprise illustration",
    peopleStyle: "professional natural people",
    imageLighting: "soft cinematic corporate lighting",
    imageComposition: "image_right",
    outline: null,
    slides: [],
    currentSlideIndex: 0,
    taskId: "",
    status: "idle",
    progress: 0,
    currentPage: 0,
    history: [],
    textModels: [],
    imageModels: [],
    selectedExample: "",
    uploadedDocumentName: "",
    imageSearchKeyword: "",
    imageSearchResults: [],
    imageGenerating: false,
    visualOperationStatus: "idle",
    error: null,
    initialized: false,
    historyLoadedAt: 0
  }),
  getters: {
    canGenerateOutline: (state) => state.createMode === "ai" && Boolean(state.prompt.trim()) && state.status !== "outlining" && state.status !== "generating" && state.status !== "rendering",
    canGeneratePpt: (state) => Boolean(state.outline?.slides.length) && state.status !== "generating" && state.status !== "rendering",
    currentSlide: (state): PptSlide | null => state.slides[state.currentSlideIndex] || null,
    statusText: (state) => {
      const map: Record<PptWorkflowStatus, string> = {
        idle: "等待创建",
        pending: "等待生成",
        outlining: "正在生成大纲",
        outline_ready: "大纲已生成",
        generating: "正在生成PPT",
        rendering: "正在渲染页面",
        success: "生成成功",
        failed: "生成失败"
      };
      return map[state.status];
    }
  },
  actions: {
    async initialize(options: { force?: boolean } = {}) {
      if (this.initialized && !options.force) return;
      if (pptInitializePromise && !options.force) return pptInitializePromise;
      pptInitializePromise = Promise.all([this.loadModels(options), this.loadHistory(options)])
        .then(() => {
          this.initialized = true;
        })
        .finally(() => {
          pptInitializePromise = null;
        });
      return pptInitializePromise;
    },
    async loadModels(options: { force?: boolean } = {}) {
      if (!options.force && this.textModels.length && this.imageModels.length) return;
      const [textModels, imageModels] = await Promise.all([getPptTextModels(), getPptImageModels()]);
      this.textModels = textModels;
      this.imageModels = imageModels;
      if (!this.textModel && textModels[0]) this.textModel = textModels[0].value;
      if (!this.imageModel && imageModels[0]) this.imageModel = imageModels[0].value;
    },
    async loadHistory(options: { force?: boolean } = {}) {
      if (!options.force && this.history.length && Date.now() - this.historyLoadedAt < 30_000) return;
      try {
        this.history = await getPptHistory();
        this.historyLoadedAt = Date.now();
      } catch (error) {
        console.error("[ppt] load history failed", error);
        this.error = { title: "历史记录加载失败", message: error instanceof Error ? error.message : "请稍后重试" };
      }
    },
    async saveDraft(taskId = "") {
      const now = new Date().toISOString();
      const nextTaskId = taskId || (this.taskId.startsWith("draft_") ? this.taskId : `draft_${Date.now()}`);
      const previous = this.history.find(item => item.taskId === nextTaskId);
      const prompt = this.prompt.trim() || this.uploadedDocumentName || this.outline?.title || "无标题演示文稿";
      const item: PptHistoryItem = {
        taskId: nextTaskId,
        type: "ppt",
        mediaType: "ppt",
        status: "draft",
        title: draftTitleFromPrompt(prompt),
        prompt,
        slideCount: this.outline?.slides.length || this.slideCount,
        language: this.language,
        tone: this.tone,
        textContent: this.textContent,
        audience: this.audience,
        scenario: this.scenario,
        generationAspectRatio: this.generationAspectRatio,
        theme: this.theme,
        autoThemeEnabled: this.autoThemeEnabled,
        imageSource: this.imageSource,
        textModel: this.textModel,
        imageModel: this.imageModel,
        enableWebSearch: this.enableWebSearch,
        progress: this.outline?.slides.length ? 30 : 0,
        currentPage: this.outline?.slides.length || 0,
        outline: this.outline || undefined,
        slides: this.slides.length ? this.slides : this.outlineToSlides(this.outline),
        pptUrl: "",
        pdfUrl: "",
        errorMessage: "",
        createdAt: previous?.createdAt || now,
        updatedAt: now
      };
      const saved = await upsertPptDraft(item);
      this.taskId = saved.taskId;
      this.history = [saved, ...this.history.filter(record => record.taskId !== saved.taskId)];
      this.historyLoadedAt = Date.now();
      return saved;
    },
    selectCreateMode(mode: PptCreateMode) {
      this.createMode = mode;
      this.error = null;
    },
    selectExample(prompt: string) {
      this.prompt = prompt;
      this.selectedExample = prompt;
      this.error = null;
    },
    setUploadedDocument(file?: File) {
      this.uploadedDocumentName = file?.name || "";
      if (file) {
        this.createMode = "document";
        this.prompt = this.prompt || file.name.replace(/\.[^.]+$/, "");
      }
    },
    async startBlankPpt() {
      this.createMode = "blank";
      this.error = null;
      const title = "无标题演示文稿";
      this.outline = {
        title,
        updatedAt: new Date().toISOString(),
        slides: [
          { page: 1, title, summary: "空白封面页，可手动编辑。", bulletPoints: [], layout: "cover" },
          { page: 2, title: "正文页", summary: "在这里补充正文内容。", bulletPoints: ["要点一", "要点二"], layout: "content" },
          { page: 3, title: "总结页", summary: "补充结论和下一步行动。", bulletPoints: ["总结观点", "下一步行动"], layout: "summary" }
        ]
      };
      this.slides = this.outlineToSlides(this.outline);
      this.currentSlideIndex = 0;
      this.status = "success";
      this.progress = 100;
    },
    async generateOutlineFlow() {
      if (!this.prompt.trim()) {
        this.error = { title: "请输入主题", message: "填写 PPT 主题后才能生成大纲。" };
        return;
      }
      this.status = "outlining";
      this.progress = 20;
      this.currentPage = 0;
      this.error = null;
      try {
        const outline = await generatePptOutline({
          prompt: this.prompt.trim(),
          slideCount: this.slideCount,
          language: this.language,
          tone: this.tone,
          textContent: this.textContent,
          audience: this.audience,
          scenario: this.scenario,
          generationAspectRatio: this.generationAspectRatio,
          autoThemeEnabled: this.autoThemeEnabled,
          enableWebSearch: this.enableWebSearch,
          textModel: this.textModel,
          imageSource: this.imageSource,
          imageModel: this.imageModel
        });
        this.outline = normalizeOutline(outline);
        this.slides = this.outlineToSlides(this.outline);
        this.status = "outline_ready";
        this.progress = 30;
        await this.saveDraft(this.taskId);
      } catch (error) {
        console.error("[ppt] generate outline failed", error);
        this.status = "failed";
        this.error = { title: "大纲生成失败", message: error instanceof Error ? error.message : "请稍后重试" };
      }
    },
    updateOutlineSlide(index: number, patch: Partial<PptOutlineSlide>) {
      if (!this.outline?.slides[index]) return;
      this.outline.slides[index] = { ...this.outline.slides[index], ...patch };
      this.outline.slides = normalizeSlidePages(this.outline.slides);
      this.slides = this.outlineToSlides(this.outline);
    },
    addOutlineSlide() {
      if (!this.outline) return;
      const page = this.outline.slides.length + 1;
      this.outline.slides.push({
        page,
        title: `新增页面 ${page}`,
        summary: "补充这一页的核心说明。",
        bulletPoints: ["新增要点"]
      });
      this.outline.slides = normalizeSlidePages(this.outline.slides);
      this.slideCount = this.outline.slides.length;
      this.slides = this.outlineToSlides(this.outline);
    },
    deleteOutlineSlide(index: number) {
      if (!this.outline || this.outline.slides.length <= 1) return;
      this.outline.slides.splice(index, 1);
      this.outline.slides = normalizeSlidePages(this.outline.slides);
      this.slideCount = this.outline.slides.length;
      this.slides = this.outlineToSlides(this.outline);
    },
    moveOutlineSlide(index: number, direction: -1 | 1) {
      if (!this.outline) return;
      const next = index + direction;
      if (next < 0 || next >= this.outline.slides.length) return;
      const [item] = this.outline.slides.splice(index, 1);
      this.outline.slides.splice(next, 0, item);
      this.outline.slides = normalizeSlidePages(this.outline.slides);
      this.slides = this.outlineToSlides(this.outline);
    },
    async regenerateOutlineSlide(index: number) {
      if (!this.outline?.slides[index]) return;
      const slide = this.outline.slides[index];
      this.updateOutlineSlide(index, {
        summary: `${slide.summary.replace(/（已重新生成）$/, "")}（已重新生成）`,
        bulletPoints: [...slide.bulletPoints, "补充更具行动性的表达"].slice(-4)
      });
    },
    async regenerateAllOutline() {
      await this.generateOutlineFlow();
    },
    async saveOutline() {
      if (!this.outline) return;
      try {
        this.outline = await updatePptOutline(this.outline);
      } catch (error) {
        console.error("[ppt] save outline failed", error);
        this.error = { title: "大纲保存失败", message: error instanceof Error ? error.message : "请稍后重试" };
      }
    },
    async confirmOutlineAndGeneratePpt() {
      if (!this.outline) {
        this.error = { title: "缺少大纲", message: "请先生成或创建 PPT 大纲。" };
        return;
      }
      this.status = "pending";
      this.progress = 0;
      this.error = null;
      const draftTaskId = this.taskId.startsWith("draft_") ? this.taskId : "";
      try {
        const created = await createPptTask({
          prompt: this.prompt.trim() || this.outline.title,
          slideCount: this.outline.slides.length,
          language: this.language,
          tone: this.tone,
          textContent: this.textContent,
          audience: this.audience,
          scenario: this.scenario,
          generationAspectRatio: this.generationAspectRatio,
          autoThemeEnabled: this.autoThemeEnabled,
          enableWebSearch: this.enableWebSearch,
          textModel: this.textModel,
          theme: this.theme,
          imageSource: this.imageSource,
          imageModel: this.imageModel,
          imageStyle: this.imageStyle,
          peopleStyle: this.peopleStyle,
          imageLighting: this.imageLighting,
          imageComposition: this.imageComposition,
          textInImage: false,
          outline: this.outline
        });
        this.taskId = created.taskId;
        if (draftTaskId) {
          await deletePptTask(draftTaskId).catch(() => undefined);
          this.history = this.history.filter(item => item.taskId !== draftTaskId);
        }
        await this.runMockProgress(created.taskId);
      } catch (error) {
        console.error("[ppt] generate ppt failed", error);
        this.status = "failed";
        this.error = { title: "PPT生成失败", message: error instanceof Error ? error.message : "请稍后重试" };
      }
    },
    async runMockProgress(taskId: string) {
      const steps: Array<{ status: PptWorkflowStatus; progress: number; currentPage: number; delay: number }> = [
        { status: "pending", progress: 0, currentPage: 0, delay: 180 },
        { status: "generating", progress: 40, currentPage: 1, delay: 700 },
        { status: "generating", progress: 70, currentPage: Math.max(1, Math.ceil(this.slideCount * 0.55)), delay: 700 },
        { status: "rendering", progress: 90, currentPage: this.slideCount, delay: 700 }
      ];
      for (const step of steps) {
        this.status = step.status;
        this.progress = step.progress;
        this.currentPage = step.currentPage;
        await wait(step.delay);
      }
      const task = await getPptTaskStatus(taskId);
      this.slides = task.slides?.length ? task.slides : this.outlineToSlides(this.outline);
      this.currentSlideIndex = 0;
      if (this.shouldPollGeneratedImages()) {
        this.status = "rendering";
        this.progress = 92;
        await this.pollGeneratedImages(taskId, {
          timeoutMs: Math.min(180_000, Math.max(95_000, this.slides.length * 60_000))
        });
      }
      this.status = "success";
      this.progress = 100;
      this.currentPage = this.outline?.slides.length || this.slideCount;
      if (this.shouldPollGeneratedImages() && realSlideImageCount(this.slides) < this.slides.length) {
        void this.pollGeneratedImages(taskId, {
          timeoutMs: Math.min(900_000, Math.max(180_000, this.slides.length * 70_000))
        });
      }
      await this.loadHistory();
    },
    shouldPollGeneratedImages() {
      const model = this.imageModel.trim();
      return this.imageSource === "ai" && Boolean(model && model !== "default-image");
    },
    async pollGeneratedImages(taskId: string, options: { waitForFirst?: boolean; timeoutMs?: number } = {}) {
      const startedAt = Date.now();
      const timeoutMs = options.timeoutMs ?? 180_000;
      const expectedCount = Math.max(1, this.slides.length || this.slideCount);
      let lastImageCount = realSlideImageCount(this.slides);
      while (Date.now() - startedAt < timeoutMs) {
        await wait(5_000);
        const task = await getPptTaskStatus(taskId);
        if (task.slides?.length) {
          this.applyTaskImageUpdates(task.slides);
        }
        const imageCount = realSlideImageCount(this.slides);
        if (imageCount !== lastImageCount) {
          lastImageCount = imageCount;
          await this.saveDraft(taskId).catch(() => undefined);
        }
        this.currentPage = imageCount;
        this.progress = Math.min(99, 92 + Math.floor((imageCount / expectedCount) * 7));
        if (options.waitForFirst && imageCount > 0) return;
        if (imageCount >= expectedCount) return;
      }
    },
    applyTaskImageUpdates(taskSlides: PptSlide[]) {
      if (!taskSlides.length) return;
      const byId = new Map(this.slides.map((slide, index) => [slide.id, index]));
      for (const slide of taskSlides) {
        if (!isRealSlideImage(slide.imageUrl)) continue;
        const index = byId.get(slide.id);
        if (index === undefined) continue;
        this.updateSlide(index, { imageUrl: slide.imageUrl });
      }
    },
    selectSlide(index: number) {
      if (index < 0 || index >= this.slides.length) return;
      this.currentSlideIndex = index;
    },
    updateSlide(index: number, patch: Partial<PptSlide>) {
      if (!this.slides[index]) return;
      this.slides[index] = { ...this.slides[index], ...patch };
    },
    async regenerateCurrentSlide() {
      const slide = this.currentSlide;
      if (!slide) return;
      const next = await regeneratePptSlide(slide);
      this.updateSlide(this.currentSlideIndex, next);
    },
    async generateImageForCurrentSlide() {
      const slide = this.currentSlide;
      if (!slide) return;
      this.imageSource = "ai";
      this.imageGenerating = true;
      this.visualOperationStatus = "loading";
      this.error = null;
      try {
        if (this.taskId && !this.taskId.startsWith("draft_")) {
          const response = await regeneratePptSlideVisual(this.taskId, slide.id, {
            visualType: slide.visualPlan?.visualType || "illustration",
            style: slide.visualPlan?.style || this.imageStyle,
            composition: this.imageComposition,
            customInstruction: "",
            keepCurrentContent: true
          });
          this.updateSlide(this.currentSlideIndex, response.slide);
        } else {
          const image = await generatePptImage({
            slide,
            prompt: "",
            deckTitle: this.outline?.title || this.prompt.trim() || slide.title,
            theme: this.theme,
            language: this.language,
            imageSource: "ai",
            imageModel: this.imageModel,
            visualPlan: slide.visualPlan,
            imageStyle: this.imageStyle,
            peopleStyle: this.peopleStyle,
            imageLighting: this.imageLighting,
            imageComposition: this.imageComposition
          });
          this.updateSlide(this.currentSlideIndex, { imageUrl: image.url || "" });
        }
        this.visualOperationStatus = "success";
        await this.saveDraft(this.taskId).catch(() => undefined);
      } catch (error) {
        console.error("[ppt] generate slide image failed", error);
		this.visualOperationStatus = "failed";
        this.error = { title: "配图生成失败", message: error instanceof Error ? error.message : "请稍后重试" };
      } finally {
        this.imageGenerating = false;
      }
    },
    updateCurrentSlideVisualPlan(patch: Partial<PptVisualPlan>) {
      const slide = this.currentSlide;
      if (!slide) return;
      const current = slide.visualPlan || defaultVisualPlan(slide, this.imageComposition, this.imageStyle);
      this.updateSlide(this.currentSlideIndex, { visualPlan: { ...current, ...patch, textInImage: false } });
    },
    async deleteCurrentSlideVisual() {
      const slide = this.currentSlide;
      if (!slide) return;
      this.imageGenerating = true;
      this.visualOperationStatus = "loading";
      try {
        if (this.taskId && !this.taskId.startsWith("draft_")) {
          const response = await deletePptSlideVisual(this.taskId, slide.id);
          this.updateSlide(this.currentSlideIndex, response.slide);
        } else {
          this.updateSlide(this.currentSlideIndex, {
            imageUrl: "",
            visualPlan: { ...defaultVisualPlan(slide, this.imageComposition, this.imageStyle), visualType: "none", imageRequired: false }
          });
        }
        this.visualOperationStatus = "success";
      } catch (error) {
        this.visualOperationStatus = "failed";
        this.error = { title: "删除配图失败", message: error instanceof Error ? error.message : "请稍后重试" };
      } finally {
        this.imageGenerating = false;
      }
    },
    async restoreCurrentSlideVisual(asset: { createdAt: string; url: string; storageRef?: string }) {
      const slide = this.currentSlide;
      if (!slide || !this.taskId || this.taskId.startsWith("draft_") || !asset.createdAt || !asset.url || this.imageGenerating) return;
      this.imageGenerating = true;
      this.visualOperationStatus = "loading";
      try {
        const response = await restorePptSlideVisual(this.taskId, slide.id, asset.createdAt, asset.url, asset.storageRef);
        this.updateSlide(this.currentSlideIndex, response.slide);
        this.visualOperationStatus = "success";
        await this.saveDraft(this.taskId).catch(() => undefined);
      } catch (error) {
        this.visualOperationStatus = "failed";
        this.error = { title: "恢复历史配图失败", message: error instanceof Error ? error.message : "请稍后重试" };
      } finally {
        this.imageGenerating = false;
      }
    },
    async searchImages(keyword: string) {
      this.imageSearchKeyword = keyword;
      this.imageSearchResults = await searchPptImages(keyword);
    },
    applyImageToCurrentSlide(image: PptImageOption) {
      if (!this.currentSlide) return;
      this.imageSource = image.url ? (image.source === "ai" ? "ai" : "stock") : "none";
      this.updateSlide(this.currentSlideIndex, { imageUrl: image.url || "" });
    },
    async exportCurrentPpt(kind: "pptx" | "pdf") {
      if (!this.taskId) throw new Error("当前任务没有可导出的文件");
      return kind === "pptx" ? exportPpt(this.taskId) : exportPdf(this.taskId);
    },
    async removeHistoryItem(taskId: string) {
      await deletePptTask(taskId);
      this.history = this.history.filter(item => item.taskId !== taskId);
    },
    loadHistoryItem(item: PptHistoryItem) {
      this.taskId = item.taskId;
      this.prompt = item.prompt || item.title;
      this.slideCount = item.slideCount || 5;
      this.language = item.language || "zh";
      this.tone = item.tone || "professional";
      this.textContent = item.textContent || "concise";
      this.audience = item.audience || "auto";
      this.scenario = item.scenario || "auto";
      this.generationAspectRatio = item.generationAspectRatio || "dynamic";
      this.theme = item.theme || "business";
      this.autoThemeEnabled = item.autoThemeEnabled ?? true;
      this.imageSource = item.imageSource || "ai";
      this.textModel = item.textModel || this.textModel;
      this.imageModel = item.imageModel || this.imageModel;
      this.imageStyle = item.imageStyle || this.imageStyle;
      this.peopleStyle = item.peopleStyle || this.peopleStyle;
      this.imageLighting = item.imageLighting || this.imageLighting;
      this.imageComposition = item.imageComposition || this.imageComposition;
      this.enableWebSearch = Boolean(item.enableWebSearch);
      this.outline = item.outline || null;
      this.slides = item.slides?.length ? item.slides : this.outlineToSlides(this.outline);
      this.currentSlideIndex = 0;
      this.status = item.status === "success" ? "success" : item.status === "draft" ? (this.outline?.slides.length ? "outline_ready" : "idle") : "generating";
      this.progress = item.progress || (item.status === "success" ? 100 : item.status === "draft" ? 0 : 40);
      this.currentPage = item.currentPage || 0;
    },
    retry() {
      this.error = null;
      if (this.outline) {
        void this.confirmOutlineAndGeneratePpt();
      } else {
        void this.generateOutlineFlow();
      }
    },
    outlineToSlides(outline: PptOutline | null): PptSlide[] {
      if (!outline) return [];
      return outline.slides.map((slide, index) => ({
        id: `slide_${index + 1}`,
        page: index + 1,
        title: slide.title,
        content: slide.summary,
        bulletPoints: [...slide.bulletPoints],
        imageUrl: "",
        layout: slide.layout || (index === 0 ? "cover" : index === outline.slides.length - 1 ? "summary" : this.imageSource === "none" ? "content" : "imageText"),
        slideType: slide.slideType || (index === 0 ? "cover" : index === outline.slides.length - 1 ? "statement" : "text_image"),
        visualPlan: defaultVisualPlan({ id: `slide_${index + 1}`, page: index + 1, title: slide.title, content: slide.summary, bulletPoints: [...slide.bulletPoints], layout: slide.layout || "imageText", slideType: slide.slideType || (index === 0 ? "cover" : "text_image") }, this.imageComposition, this.imageStyle),
        speakerNotes: `第 ${index + 1} 页讲稿可在后续接入模型后自动生成。`
      }));
    }
  }
});

function normalizeOutline(outline: PptOutline): PptOutline {
  return {
    ...outline,
    title: outline.title || "无标题演示文稿",
    slides: normalizeSlidePages(outline.slides || [])
  };
}

function normalizeSlidePages(slides: PptOutlineSlide[]) {
  return slides.map((slide, index) => ({ ...slide, page: index + 1 }));
}

function draftTitleFromPrompt(prompt: string) {
  const title = prompt.trim().replace(/\s+/g, " ");
  return title.length > 50 ? title.slice(0, 50) : title || "无标题演示文稿";
}

function wait(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms));
}

function realSlideImageCount(slides: PptSlide[]) {
  return slides.filter(slide => isRealSlideImage(slide.imageUrl)).length;
}

function isRealSlideImage(url = "") {
  const normalized = url.trim().toLowerCase();
  if (!normalized) return false;
  if (normalized.startsWith("http://") || normalized.startsWith("https://")) return true;
  return normalized.startsWith("data:image/png") || normalized.startsWith("data:image/jpeg") || normalized.startsWith("data:image/jpg") || normalized.startsWith("data:image/webp");
}

function defaultVisualPlan(slide: PptSlide, composition: PptVisualComposition, style: string): PptVisualPlan {
  const noImageTypes = new Set(["agenda", "feature_grid", "process", "timeline", "comparison", "data_chart", "swot", "matrix", "organization", "table"]);
  const imageRequired = !noImageTypes.has(slide.slideType || "text_image");
  return {
    visualType: imageRequired ? "illustration" : slide.slideType === "data_chart" ? "chart" : slide.slideType === "process" ? "diagram" : "icon",
    imageRequired,
    chartRequired: slide.slideType === "data_chart",
    diagramRequired: ["process", "timeline", "organization"].includes(slide.slideType || ""),
    textInImage: false,
    subject: slide.title,
    scene: "modern enterprise environment",
    action: "people and intelligent systems collaborate",
    objects: ["workspace", "abstract data connections"],
    mood: "professional, efficient, trustworthy",
    composition,
    style,
    prompt: "",
    negativePrompt: "text, letters, words, typography, numbers, logo, watermark, captions, subtitles, garbled text"
  };
}
