<template>
  <section class="ppt-agent-deck-preview" data-preview-workspace tabindex="0" @keydown="onKeydown">
    <div v-if="loading" class="ppt-agent-preview-state" role="status">正在加载演示文稿预览…</div>
    <div v-else-if="error" class="ppt-agent-preview-state is-error" role="alert">
      <strong>暂时无法显示预览</strong>
      <p>{{ error }}</p>
      <button type="button" data-action="retry-preview" :disabled="busy" @click="emit('retry')">重新加载预览</button>
    </div>
    <div v-else-if="projection && currentSlide && currentLayout" class="ppt-agent-preview-ready">
      <header class="ppt-agent-preview-heading">
        <div>
          <span>演示文稿预览</span>
          <h2>{{ projection.deck.deckSpec.title }}</h2>
          <p>当前预览与下载文件共享同一份排版坐标。</p>
        </div>
        <button type="button" data-action="download-pptx" :disabled="busy" @click="emit('download')">下载 PPTX</button>
      </header>

      <div class="ppt-agent-preview-grid">
        <main class="ppt-agent-current-preview">
          <div class="ppt-agent-main-canvas">
            <PptAgentSlideCanvas
              :slide="currentSlide"
              :layout="currentLayout"
              :canvas="projection.layoutResult.canvas"
              :asset-u-r-ls="assetURLs"
              :label="`第 ${currentIndex + 1} 页：${slideTitle(currentSlide)}`"
              @asset-error="handleAssetError"
            />
          </div>
          <nav class="ppt-agent-page-nav" aria-label="幻灯片翻页">
            <button type="button" data-action="previous-slide" :disabled="currentIndex === 0" aria-label="上一页" @click="selectIndex(currentIndex - 1)">上一页</button>
            <strong aria-live="polite">{{ currentIndex + 1 }} / {{ projection.deck.slides.length }}</strong>
            <button type="button" data-action="next-slide" :disabled="currentIndex === projection.deck.slides.length - 1" aria-label="下一页" @click="selectIndex(currentIndex + 1)">下一页</button>
          </nav>
        </main>

        <aside class="ppt-agent-thumbnails" aria-label="全部幻灯片">
          <button
            v-for="(slide, index) in projection.deck.slides"
            :key="slide.id"
            type="button"
            :data-preview-thumbnail="slide.id"
            :aria-label="`查看第 ${index + 1} 页：${slideTitle(slide)}`"
            :aria-current="slide.id === currentSlide.id ? 'page' : undefined"
            :class="{ 'is-current': slide.id === currentSlide.id }"
            @click="selectIndex(index)"
          >
            <PptAgentSlideCanvas
              :slide="slide"
              :layout="projection.layoutResult.slides[index]"
              :canvas="projection.layoutResult.canvas"
              :asset-u-r-ls="assetURLs"
              :label="`第 ${index + 1} 页缩略图`"
              @asset-error="handleAssetError"
            />
            <span>{{ index + 1 }}</span>
          </button>
        </aside>
      </div>

      <section class="ppt-agent-preview-sources" aria-label="当前页来源与证据">
        <header><div><span>当前页证据</span><h3>来源与事实依据</h3></div><small>第 {{ currentIndex + 1 }} 页</small></header>
        <p v-if="!currentEvidence.length" class="ppt-agent-no-evidence">本页不依赖事实型证据。</p>
        <article v-for="evidence in currentEvidence" :key="evidence.claim.id">
          <div class="ppt-agent-source-title">
            <strong>{{ evidence.source.title }}</strong>
            <span>{{ evidence.source.type }}</span>
            <span>{{ evidence.claim.verificationStatus }}</span>
          </div>
          <p>{{ evidence.claim.text }}</p>
          <p v-if="evidence.rationale" class="ppt-agent-rationale"><b>为什么支持本页：</b>{{ evidence.rationale }}</p>
          <a v-for="citation in evidence.citations" :key="citation.id" :href="citation.locator" target="_blank" rel="noopener noreferrer">{{ citation.locator }}</a>
        </article>
      </section>
    </div>
    <div v-else class="ppt-agent-preview-state is-error" role="alert">预览数据不可用，请重新加载。</div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { OutlinePlan, PptAgentPreviewProjection, PptPreviewSlide } from "../../types/pptAgent";
import PptAgentSlideCanvas from "./PptAgentSlideCanvas.vue";

const props = defineProps<{
  projection: PptAgentPreviewProjection | null;
  approvedOutline: OutlinePlan;
  loading: boolean;
  error: string;
  busy: boolean;
}>();
const emit = defineEmits<{ retry: []; download: []; "asset-expired": [assetId: string] }>();
const currentSlideID = ref("");
const reportedAssetFailures = new Set<string>();

watch(() => props.projection, projection => {
  if (!projection) return;
  if (!projection.deck.slides.some(slide => slide.id === currentSlideID.value)) currentSlideID.value = projection.deck.slides[0]?.id || "";
  reportedAssetFailures.clear();
}, { immediate: true });

const currentIndex = computed(() => Math.max(0, props.projection?.deck.slides.findIndex(slide => slide.id === currentSlideID.value) ?? 0));
const currentSlide = computed(() => props.projection?.deck.slides[currentIndex.value]);
const currentLayout = computed(() => props.projection?.layoutResult.slides[currentIndex.value]);
const assetURLs = computed(() => Object.fromEntries((props.projection?.assets || []).map(asset => [asset.assetId, asset.url])));
const currentEvidence = computed(() => {
  const projection = props.projection;
  const slide = currentSlide.value;
  if (!projection || !slide) return [];
  const claims = new Map(projection.deck.provenance.claims.map(claim => [claim.id, claim]));
  const sources = new Map(projection.deck.provenance.sources.map(source => [source.id, source]));
  const citations = new Map(projection.deck.provenance.citations.map(citation => [citation.id, citation]));
  const assignments = new Map((props.approvedOutline.slides.find(item => item.slideId === slide.id)?.evidence || []).map(item => [item.claimId, item.rationale]));
  return slide.citationRefs.flatMap(claimID => {
    const claim = claims.get(claimID);
    const source = claim ? sources.get(claim.sourceId) : undefined;
    if (!claim || !source) return [];
    return [{ claim, source, citations: claim.citationRefs.flatMap(id => citations.get(id) ? [citations.get(id)!] : []), rationale: assignments.get(claimID) || "" }];
  });
});

function slideTitle(slide: PptPreviewSlide) {
  const title = slide.elements.find(element => element.type === "text" && element.slot === "title");
  return title?.content?.text || slide.keyMessage;
}

function selectIndex(index: number) {
  const slides = props.projection?.deck.slides || [];
  if (index >= 0 && index < slides.length) currentSlideID.value = slides[index].id;
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === "ArrowLeft") {
    event.preventDefault();
    selectIndex(currentIndex.value - 1);
  }
  if (event.key === "ArrowRight") {
    event.preventDefault();
    selectIndex(currentIndex.value + 1);
  }
}

function handleAssetError(assetId: string) {
  if (!assetId || reportedAssetFailures.has(assetId)) return;
  reportedAssetFailures.add(assetId);
  emit("asset-expired", assetId);
}
</script>

<style scoped>
.ppt-agent-deck-preview { outline: none; }
.ppt-agent-preview-state { min-height: 260px; display: grid; place-content: center; gap: 10px; padding: 28px; text-align: center; border: 1px solid #e1e6ef; border-radius: 18px; background: #fff; color: #526071; }
.ppt-agent-preview-state.is-error { color: #8f3030; border-color: #eccaca; background: #fff8f8; }
.ppt-agent-preview-state p { margin: 0; }
.ppt-agent-preview-state button, .ppt-agent-preview-heading button, .ppt-agent-page-nav button { border: 0; border-radius: 9px; padding: 9px 14px; cursor: pointer; }
.ppt-agent-preview-state button { justify-self: center; background: #172033; color: #fff; }
.ppt-agent-preview-ready { display: grid; gap: 18px; }
.ppt-agent-preview-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 18px 20px; border: 1px solid #e1e6ef; border-radius: 16px; background: #fff; }
.ppt-agent-preview-heading span, .ppt-agent-preview-sources span { color: #697386; font-size: 12px; text-transform: uppercase; letter-spacing: .08em; }
.ppt-agent-preview-heading h2, .ppt-agent-preview-sources h3 { margin: 3px 0; }
.ppt-agent-preview-heading p { margin: 0; color: #697386; }
.ppt-agent-preview-heading button { background: #ff6b00; color: #fff; font-weight: 700; }
.ppt-agent-preview-grid { display: grid; grid-template-columns: minmax(0, 1fr) 190px; gap: 18px; min-height: 520px; }
.ppt-agent-current-preview { min-width: 0; display: grid; align-content: start; gap: 14px; padding: 22px; border: 1px solid #e1e6ef; border-radius: 18px; background: #e9edf4; }
.ppt-agent-main-canvas { width: 100%; display: flex; justify-content: center; min-width: 0; }
.ppt-agent-page-nav { display: flex; justify-content: center; align-items: center; gap: 16px; }
.ppt-agent-page-nav button { background: #fff; color: #172033; }
.ppt-agent-page-nav button:disabled { opacity: .42; cursor: not-allowed; }
.ppt-agent-thumbnails { display: grid; align-content: start; gap: 10px; max-height: 620px; overflow-y: auto; padding: 10px; border: 1px solid #e1e6ef; border-radius: 18px; background: #fff; }
.ppt-agent-thumbnails > button { display: grid; gap: 5px; width: 100%; padding: 6px; border: 2px solid transparent; border-radius: 10px; background: #f2f4f8; color: #526071; cursor: pointer; overflow: hidden; }
.ppt-agent-thumbnails > button.is-current { border-color: #ff6b00; color: #9e4200; }
.ppt-agent-thumbnails :deep(.ppt-agent-slide-viewport) { width: 160px !important; height: 90px !important; }
.ppt-agent-thumbnails :deep(.ppt-agent-slide-canvas) { transform: scale(.1666666667) !important; transform-origin: top left !important; }
.ppt-agent-preview-sources { display: grid; gap: 12px; padding: 18px 20px; border: 1px solid #e1e6ef; border-radius: 16px; background: #fff; }
.ppt-agent-preview-sources > header, .ppt-agent-source-title { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.ppt-agent-preview-sources article { display: grid; gap: 7px; padding-top: 12px; border-top: 1px solid #e1e6ef; }
.ppt-agent-preview-sources article p { margin: 0; }
.ppt-agent-source-title { justify-content: flex-start; }
.ppt-agent-source-title span { padding: 3px 7px; border-radius: 999px; background: #eef4ef; color: #376047; letter-spacing: 0; text-transform: none; }
.ppt-agent-rationale, .ppt-agent-no-evidence { color: #697386; }
.ppt-agent-preview-sources a { color: #315eaa; overflow-wrap: anywhere; }
@media (max-width: 900px) {
  .ppt-agent-preview-grid { grid-template-columns: 1fr; }
  .ppt-agent-thumbnails { grid-template-columns: repeat(4, minmax(0, 1fr)); max-height: none; }
  .ppt-agent-thumbnails > button { min-width: 0; }
  .ppt-agent-preview-heading { align-items: stretch; flex-direction: column; }
}
@media (max-width: 560px) {
  .ppt-agent-thumbnails { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
