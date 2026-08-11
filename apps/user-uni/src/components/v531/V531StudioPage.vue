<template>
  <view class="v532-studio-page">
    <V532SceneCenter
      v-if="activeView === 'scenes'"
      :identity-label="identityLabel"
      @back="activeView = 'home'"
      @open-scene="applyScenePreset"
    />
    <template v-else>
    <view class="studio-header">
      <view class="studio-brand-mark">Z</view>
      <view class="studio-header-copy">
        <text class="studio-title">创作</text>
        <text class="studio-subtitle">{{ studioSubtitle }}</text>
      </view>
      <text class="identity-pill">{{ identityLabel }}</text>
    </view>

    <view class="balance-row">
      <text class="balance-value">{{ formattedPointBalance }} 点</text>
      <text class="balance-model">最近模型：{{ recentModel }}</text>
      <button class="recharge-button" type="button" @click="$emit('recharge')">充值</button>
    </view>

    <view :class="['prompt-composer', { focused: composerFocused, error: promptError }]">
      <text class="composer-title">一句话开始创作</text>
      <textarea
        v-model="prompt"
        class="prompt-input"
        maxlength="500"
        auto-height
        placeholder="例如：帮我生成一张适合朋友圈推广的企业级 AI SaaS 宣传海报"
        @focus="composerFocused = true"
        @blur="composerFocused = false"
        @input="promptError = ''"
      />
      <text class="prompt-count">{{ prompt.length }} / 500</text>

      <view class="composer-divider"></view>
      <view class="composer-tools">
        <button class="tool-button" type="button" data-studio-action="reference" @click="chooseReferenceImage">
          <text class="tool-glyph">▣</text>
          <text>{{ referencePaths.length ? `参考图 ${referencePaths.length}` : "参考图" }}</text>
        </button>
        <button v-if="supportsDocumentUpload" class="tool-button" type="button" data-studio-action="file" @click="chooseFile">
          <text class="tool-glyph">⇧</text>
          <text>{{ selectedFiles.length ? `文件 ${selectedFiles.length}` : "上传文件" }}</text>
        </button>
        <text class="recognition-pill">自动识别</text>
      </view>

      <view class="composer-footer">
        <view class="recommendation-copy">
          <text>推荐类型：{{ recommendation.label }}</text>
          <text>推荐模型：{{ recommendation.model }}</text>
          <text class="cost-copy">预计消耗：{{ recommendation.cost }}</text>
        </view>
        <button
          class="generate-button"
          type="button"
          :data-mode="recommendation.mode"
          :data-prompt="prompt"
          :disabled="!prompt.trim()"
          @click="startCreation"
        >
          立即生成
        </button>
      </view>
      <text v-if="promptError" class="prompt-error">{{ promptError }}</text>
    </view>

    <view class="section-heading">
      <text class="section-title">AI 核心能力</text>
      <text class="section-more">全部能力 ›</text>
    </view>
    <view class="capability-grid">
      <button
        v-for="item in coreCapabilities"
        :key="item.id"
        :data-mode="item.mode"
        :data-prompt="prompt"
        class="capability-button"
        type="button"
        @click="openMode(item.mode)"
      >
        <text :class="['capability-icon', item.tone]">{{ item.icon }}</text>
        <view class="capability-copy">
          <text class="capability-title">{{ item.title }}</text>
          <text class="capability-description">{{ item.summary }}</text>
        </view>
        <text v-if="item.free" class="free-pill">免费</text>
      </button>
    </view>

    <view class="section-heading scene-heading">
      <text class="section-title">AI 场景</text>
      <button v-if="hasExtendedSceneCenter" class="section-more section-more-button" type="button" @click="activeView = 'scenes'">全部场景 ›</button>
    </view>
    <scroll-view scroll-x class="scene-scroll" :show-scrollbar="false">
      <view class="scene-row">
        <button
          v-for="item in scenes"
          :key="item.id"
          :data-scene-id="item.id"
          :data-mode="item.mode"
          :data-prompt="item.prompt"
          :class="['scene-button', item.tone]"
          type="button"
          @click="applyScene(item)"
        >
          <text class="scene-title">{{ item.title }}</text>
          <text class="scene-summary">{{ item.summary }}</text>
        </button>
      </view>
    </scroll-view>
    </template>
  </view>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import V532SceneCenter from "./V532SceneCenter.vue";

type CreationMode = "image" | "video" | "ppt" | "infographic" | "review" | "agent" | "montage";
type NativeStudioBridge = typeof globalThis & {
  __xianzhiV531StudioGenerate?: () => void;
  __xianzhiV531StudioOpenMode?: (mode: string) => void;
  __xianzhiV531StudioOpenScene?: (sceneId: string) => void;
  __xianzhiV531StudioOpenSceneCenter?: () => void;
  __xianzhiV531StudioChooseReference?: () => void;
  __xianzhiV531StudioChooseFile?: () => void;
};

const props = withDefaults(defineProps<{
  pointBalance?: number;
  planName?: string;
  recentModel?: string;
  allowedCreationModes?: CreationMode[];
}>(), {
  pointBalance: 0,
  planName: "普通用户",
  recentModel: "豆包·通用 Pro",
  allowedCreationModes: () => ["image", "infographic", "video", "montage"],
});

const emit = defineEmits<{
  "open-mode": [mode: CreationMode];
  recharge: [];
}>();

const prompt = ref("");
const activeView = ref<"home" | "scenes">("home");
const composerFocused = ref(false);
const promptError = ref("");
const referencePaths = ref<string[]>([]);
const selectedFiles = ref<Array<{ name: string; path: string }>>([]);

const identityLabel = computed(() => {
  const label = String(props.planName || "普通用户").replace(/[（(].*?[）)]/g, "").trim();
  return label.length <= 6 ? label : "普通用户";
});
const formattedPointBalance = computed(() => Math.max(0, Number(props.pointBalance || 0)).toLocaleString("zh-CN"));
const isModeAllowed = (mode: CreationMode) => props.allowedCreationModes.includes(mode);
const supportsDocumentUpload = computed(() =>
  ["ppt", "agent", "review"].some((mode) => isModeAllowed(mode as CreationMode)),
);
const studioSubtitle = computed(() =>
  isModeAllowed("video") || isModeAllowed("ppt") || isModeAllowed("agent")
    ? "一句话生成图片、视频、PPT 和智能内容"
    : "一句话生成合规图片内容",
);
const hasExtendedSceneCenter = computed(() =>
  ["video", "ppt", "agent", "review"].some((mode) => isModeAllowed(mode as CreationMode)),
);

const recommendation = computed(() => {
  const value = prompt.value.toLowerCase();
  if (isModeAllowed("video") && /视频|短片|口播|分镜|video/.test(value)) return { mode: "video" as const, label: "AI 视频", model: "即梦视频 Pro", cost: "40～80 点" };
  if (isModeAllowed("ppt") && /ppt|演示|汇报|路演|方案/.test(value)) return { mode: "ppt" as const, label: "PPT 文档", model: "Kimi K2.6", cost: "60～100 点" };
  if (/信息图|自由P图|流程图|数据图|可视化|修图|P图/.test(value)) return { mode: "infographic" as const, label: "自由P图", model: "豆包·通用 Pro", cost: "20～40 点" };
  if (isModeAllowed("review") && /质检|审稿|校对|合规|审核/.test(value)) return { mode: "review" as const, label: "AI 质检", model: "豆包·通用 Pro", cost: "10～30 点" };
  if (isModeAllowed("agent") && /agent|智能体|销售助手|客服|数字员工/.test(value)) return { mode: "agent" as const, label: "AI Agent", model: "平台智能体", cost: "免费试用" };
  return { mode: "image" as const, label: "AI 生图", model: props.recentModel, cost: "20～40 点" };
});

const coreCapabilitiesSource = [
  { id: "image", icon: "图", title: "AI 生图", summary: "海报·商品图", tone: "blue", mode: "image", free: false },
  { id: "infographic", icon: "图", title: "自由P图", summary: "杂志·修图", tone: "blue", mode: "infographic", free: false },
  { id: "montage", icon: "剪", title: "AI混剪", summary: "素材自动成片", tone: "blue", mode: "montage", free: false },
  { id: "video", icon: "视", title: "AI 视频", summary: "宣传片·短视频", tone: "blue", mode: "video", free: false },
  { id: "ppt", icon: "P", title: "PPT 文档", summary: "方案·路演", tone: "blue", mode: "ppt", free: false },
  { id: "agent", icon: "AI", title: "AI Agent", summary: "营销·销售", tone: "blue", mode: "agent", free: true },
  { id: "review", icon: "检", title: "AI 质检", summary: "审稿·合规", tone: "orange", mode: "review", free: false },
] as const satisfies ReadonlyArray<{
  id: string;
  icon: string;
  title: string;
  summary: string;
  tone: "blue" | "orange";
  mode: CreationMode;
  free?: boolean;
}>;
const coreCapabilities = computed(() =>
  coreCapabilitiesSource.filter((item) => isModeAllowed(item.mode)),
);

const scenesSource = [
  { id: "xhs", title: "小红书爆款", summary: "封面 + 文案", tone: "pink", mode: "image", prompt: "帮我创作一套小红书爆款封面和配套文案" },
  { id: "moments", title: "朋友圈海报", summary: "自动匹配", tone: "white", mode: "image", prompt: "帮我生成一张适合朋友圈推广的企业宣传海报" },
  { id: "company", title: "企业宣传", summary: "自动匹配", tone: "white", mode: "ppt", prompt: "帮我制作一份企业品牌宣传与业务介绍方案" },
] as const satisfies ReadonlyArray<{
  id: string;
  title: string;
  summary: string;
  tone: "pink" | "white";
  mode: CreationMode;
  prompt: string;
}>;
const scenes = computed(() =>
  scenesSource.filter((item) => isModeAllowed(item.mode)),
);

function persistDraft(mode: CreationMode) {
  uni.setStorageSync("v531-creation-prompt", prompt.value.trim());
  uni.setStorageSync("v532-studio-draft", {
    mode,
    prompt: prompt.value.trim(),
    referencePaths: referencePaths.value,
    files: selectedFiles.value,
  });
}

function startCreation() {
  if (!prompt.value.trim()) {
    promptError.value = "请先输入你的创作需求";
    return;
  }
  const mode = recommendation.value.mode;
  if (!isModeAllowed(mode)) return showModeUnavailable();
  persistDraft(mode);
  emit("open-mode", mode);
}

function openMode(mode: CreationMode) {
  if (!isModeAllowed(mode)) return showModeUnavailable();
  if (prompt.value.trim()) persistDraft(mode);
  emit("open-mode", mode);
}

function applyScene(scene: (typeof scenesSource)[number]) {
  if (!isModeAllowed(scene.mode)) return showModeUnavailable();
  prompt.value = scene.prompt;
  persistDraft(scene.mode);
  emit("open-mode", scene.mode);
}

function applyScenePreset(mode: CreationMode, scenePrompt: string) {
  if (!isModeAllowed(mode)) return showModeUnavailable();
  prompt.value = scenePrompt;
  persistDraft(mode);
  emit("open-mode", mode);
}

onMounted(() => {
  const bridge = globalThis as NativeStudioBridge;
  bridge.__xianzhiV531StudioGenerate = startCreation;
  bridge.__xianzhiV531StudioOpenMode = (mode) => {
    if (["image", "video", "ppt", "infographic", "review", "agent", "montage"].includes(mode)) {
      openMode(mode as CreationMode);
    }
  };
  bridge.__xianzhiV531StudioOpenScene = (sceneId) => {
    const scene = scenes.value.find(item => item.id === sceneId);
    if (scene) applyScene(scene);
  };
  bridge.__xianzhiV531StudioOpenSceneCenter = () => {
    activeView.value = "scenes";
  };
  bridge.__xianzhiV531StudioChooseReference = chooseReferenceImage;
  bridge.__xianzhiV531StudioChooseFile = chooseFile;
});

function showModeUnavailable() {
  uni.showToast({ title: "当前小程序审核版本暂未开放该能力", icon: "none" });
}

onBeforeUnmount(() => {
  const bridge = globalThis as NativeStudioBridge;
  bridge.__xianzhiV531StudioGenerate = undefined;
  bridge.__xianzhiV531StudioOpenMode = undefined;
  bridge.__xianzhiV531StudioOpenScene = undefined;
  bridge.__xianzhiV531StudioOpenSceneCenter = undefined;
  bridge.__xianzhiV531StudioChooseReference = undefined;
  bridge.__xianzhiV531StudioChooseFile = undefined;
});

function chooseReferenceImage() {
  uni.chooseImage({
    count: Math.max(1, 3 - referencePaths.value.length),
    sizeType: ["compressed"],
    success: (result) => {
      referencePaths.value = [...referencePaths.value, ...result.tempFilePaths].slice(0, 3);
      uni.showToast({ title: `已选择 ${referencePaths.value.length} 张参考图`, icon: "success" });
    },
    fail: error => {
      if (!String(error.errMsg || "").toLowerCase().includes("cancel")) {
        uni.showToast({ title: "参考图选择失败", icon: "none" });
      }
    },
  });
}

function chooseFile() {
  const chooser = (uni as unknown as {
    chooseMessageFile?: (options: {
      count: number;
      type: "file";
      success: (result: { tempFiles: Array<{ name: string; path: string }> }) => void;
      fail: () => void;
    }) => void;
  }).chooseMessageFile;
  if (!chooser) {
    uni.showToast({ title: "当前环境请进入具体创作类型后上传文件", icon: "none" });
    return;
  }
  chooser({
    count: 3,
    type: "file",
    success: (result) => {
      selectedFiles.value = result.tempFiles.slice(0, 3);
      uni.showToast({ title: `已选择 ${selectedFiles.value.length} 个文件`, icon: "success" });
    },
    fail: () => uni.showToast({ title: "文件选择已取消或失败", icon: "none" }),
  });
}
</script>

<style scoped>
.v532-studio-page {
  min-height: 100vh;
  box-sizing: border-box;
  padding: 18px 15px 120px;
  color: #111827;
  background: #f7f8fc;
  font-family: Inter, "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif;
}
.studio-header { display: flex; min-height: 54px; align-items: center; gap: 12px; }
.studio-brand-mark { display: grid; width: 36px; height: 36px; flex: 0 0 36px; place-items: center; border-radius: 10px; color: #fff; background: #5a4db2; font-size: 13px; font-weight: 700; }
.studio-header-copy { display: grid; min-width: 0; flex: 1; gap: 2px; }
.studio-title { font-size: 18px; font-weight: 700; line-height: 24px; }
.studio-subtitle { overflow: hidden; color: #6b7280; font-size: 11px; line-height: 16px; text-overflow: ellipsis; white-space: nowrap; }
.identity-pill { flex: 0 0 auto; border-radius: 13px; color: #5a4db2; background: #eef0ff; padding: 6px 11px; font-size: 12px; font-weight: 600; }
.balance-row { display: grid; min-height: 50px; margin-top: 12px; padding: 9px 12px; grid-template-columns:auto minmax(0,1fr) auto; align-items: center; gap: 12px; border: 1px solid #e5e7eb; border-radius: 14px; background: #fff; }
.balance-value { font-size: 16px; font-weight: 700; white-space: nowrap; }
.balance-model { overflow: hidden; color: #6b7280; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.recharge-button { width: 58px; height: 28px; margin: 0; padding: 0; border: 0; border-radius: 13px; color: #ff771b; background: #fff1e8; font-size: 12px; font-weight: 600; line-height: 28px; }
.prompt-composer { margin-top: 12px; padding: 13px 15px 11px; border: 1px solid #7d8df6; border-radius: 18px; background: #fff; box-shadow: 0 8px 22px rgba(74,108,255,.06); transition: border-color .18s, box-shadow .18s; }
.prompt-composer.focused { border-color: #4a6cff; box-shadow: 0 0 0 3px rgba(74,108,255,.11), 0 10px 24px rgba(74,108,255,.08); }
.prompt-composer.error { border-color: #f04438; }
.composer-title { display: block; font-size: 16px; font-weight: 600; line-height: 22px; }
.prompt-input { width: 100%; min-height: 62px; margin-top: 9px; padding: 0; color: #111827; background: transparent; font-size: 14px; line-height: 20px; }
.prompt-count { display: block; color: #9ca3af; font-size: 11px; text-align: right; }
.composer-divider { height: 1px; margin-top: 8px; background: #e5e7eb; }
.composer-tools { display: flex; min-height: 38px; align-items: center; gap: 14px; }
.tool-button { display: flex; width: auto; min-height: 32px; margin: 0; padding: 0; align-items: center; gap: 4px; border: 0; color: #6b7280; background: transparent; font-size: 12px; line-height: 32px; }
.tool-glyph { color: #7b8498; font-size: 13px; }
.recognition-pill { margin-left: auto; border-radius: 13px; color: #5a4db2; background: #eef0ff; padding: 6px 12px; font-size: 12px; font-weight: 600; white-space: nowrap; }
.composer-footer { display: grid; grid-template-columns: minmax(0,1fr) 146px; align-items: end; gap: 10px; }
.recommendation-copy { display: flex; min-width: 0; flex-wrap: wrap; gap: 4px 9px; color: #6b7280; font-size: 11px; line-height: 16px; }
.cost-copy { width: 100%; }
.generate-button { width: 146px; height: 38px; margin: 0; padding: 0; border: 0; border-radius: 14px; color: #fff; background: #ff771b; font-size: 15px; font-weight: 700; line-height: 38px; box-shadow: 0 8px 16px rgba(255,119,27,.2); }
.generate-button[disabled] { opacity: .45; }
.prompt-error { display: block; margin-top: 7px; color: #f04438; font-size: 11px; }
.section-heading { display: flex; margin: 19px 0 10px; align-items: center; justify-content: space-between; gap: 12px; }
.section-title { font-size: 16px; font-weight: 600; line-height: 22px; }
.section-more { color: #7d8df6; font-size: 12px; font-weight: 600; }
.section-more-button { width: auto; min-height: 28px; margin: 0; padding: 0; border: 0; background: transparent; line-height: 28px; }
.capability-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 10px; }
.capability-button { position: relative; display: grid; width: 100%; min-height: 64px; margin: 0; padding: 9px; grid-template-columns: 36px minmax(0,1fr); align-items: center; gap: 8px; border: 1px solid #e5e7eb; border-radius: 14px; background: #fff; text-align: left; box-shadow: 0 6px 14px rgba(31,46,89,.035); }
.capability-icon { display: grid; width: 36px; height: 36px; place-items: center; border-radius: 10px; font-size: 13px; font-weight: 700; }
.capability-icon.blue { color: #7d8df6; background: #eef0ff; }
.capability-icon.orange { color: #ff771b; background: #fff1e8; }
.capability-copy { display: grid; min-width: 0; gap: 3px; }
.capability-title { overflow: hidden; color: #111827; font-size: 14px; font-weight: 600; line-height: 20px; text-overflow: ellipsis; white-space: nowrap; }
.capability-description { overflow: hidden; color: #6b7280; font-size: 11px; line-height: 16px; text-overflow: ellipsis; white-space: nowrap; }
.free-pill { position: absolute; top: 7px; right: 7px; border-radius: 10px; color: #7d8df6; background: #eef0ff; padding: 3px 6px; font-size: 9px; font-weight: 600; }
.scene-heading { margin-top: 20px; }
.scene-scroll { margin: 0 -15px; padding-left: 15px; }
.scene-row { display: flex; width: max-content; gap: 10px; padding-right: 15px; }
.scene-button { display: grid; width: 106px; height: 54px; margin: 0; padding: 8px 8px 6px; place-items: center; gap: 1px; border: 1px solid #e5e7eb; border-radius: 14px; background: #fff; text-align: center; }
.scene-button.pink { background: #fff1f5; }
.scene-title { color: #111827; font-size: 12px; font-weight: 600; line-height: 17px; }
.scene-summary { color: #6b7280; font-size: 10px; line-height: 14px; }
.recharge-button::after,.tool-button::after,.generate-button::after,.capability-button::after,.scene-button::after,.section-more-button::after { display: none; }
@media (max-width: 360px) {
  .v532-studio-page { padding-right: 12px; padding-left: 12px; }
  .balance-row { gap: 8px; }
  .balance-model { display: none; }
  .composer-footer { grid-template-columns: minmax(0,1fr) 126px; }
  .generate-button { width: 126px; }
  .capability-button { padding: 8px; gap: 7px; }
}
</style>
