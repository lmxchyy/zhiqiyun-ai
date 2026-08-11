<template>
  <view class="sv-page" :style="navigationStyle">
    <view class="sv-nav">
      <button class="nav-back" aria-label="返回" @click="goBack">‹</button>
      <text class="nav-title">AI混剪</text>
      <view class="nav-spacer" />
    </view>

    <scroll-view class="sv-scroll" scroll-y>
      <view class="sv-body">
        <text class="lead">上传视频或图片素材，填写需求后开始智能分析与混剪方案。</text>

        <label class="field">
          <text class="label">项目标题</text>
          <input v-model="title" maxlength="80" placeholder="例如：门店开业宣传混剪" />
        </label>

        <label class="field">
          <text class="label">混剪需求</text>
          <textarea
            v-model="requirement"
            maxlength="800"
            placeholder="说明风格、节奏、旁白重点，例如：节奏明快，突出优惠信息，旁白简洁有力"
          />
        </label>

        <view class="field">
          <view class="label-row">
            <text class="label">素材</text>
            <button class="text-btn" :disabled="busy" @click="chooseMedia">选择素材</button>
          </view>
          <view v-if="!picked.length" class="empty-box">
            <text>还没有素材</text>
            <text class="hint">支持视频与图片，可多选</text>
          </view>
          <view v-else class="media-list">
            <view v-for="(item, index) in picked" :key="item.path + index" class="media-row">
              <text class="media-tag">{{ item.assetType === "IMAGE" ? "图片" : "视频" }}</text>
              <text class="media-name">{{ item.name }}</text>
              <button class="text-btn danger" :disabled="busy" @click="removePick(index)">移除</button>
            </view>
          </view>
        </view>

        <view v-if="uploads.length" class="upload-box">
          <view v-for="item in uploads" :key="item.id" class="upload-row">
            <text class="upload-name">{{ item.name }}</text>
            <text class="upload-status">
              {{ item.status === "completed" ? "已上传" : item.status === "failed" ? item.error || "失败" : `${item.progress}%` }}
            </text>
          </view>
        </view>

        <text v-if="errorMessage" class="error">{{ errorMessage }}</text>
      </view>
    </scroll-view>

    <view class="sv-footer">
      <button class="primary" :disabled="busy || !canStart" @click="startFlow">
        {{ busy ? busyLabel : "上传并开始分析" }}
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { onLoad, onHide, onShow } from "@dcloudio/uni-app";
import { assetTypeFromMime, type LocalMediaPick } from "../../api/smart-video";
import { useMiniProgramNavigation } from "../../composables/useMiniProgramNavigation";
import { useSmartVideoProject } from "../../composables/useSmartVideoProject";

const { navigationStyle } = useMiniProgramNavigation();
const flow = useSmartVideoProject();

const title = ref("未命名混剪项目");
const requirement = ref("");
const picked = ref<LocalMediaPick[]>([]);
const projectId = ref("");

const busy = computed(() => flow.busy.value);
const errorMessage = computed(() => flow.errorMessage.value);
const uploads = computed(() => flow.state.uploads);
const canStart = computed(() => Boolean(requirement.value.trim() && picked.value.length));
const busyLabel = computed(() => {
  if (flow.state.phase === "uploading") return "正在上传…";
  if (flow.state.phase === "analyzing") return "正在分析…";
  if (flow.state.phase === "planning") return "正在生成方案…";
  return "处理中…";
});

watch(title, (value) => {
  flow.state.title = value;
});
watch(requirement, (value) => {
  flow.state.requirement = value;
});

onLoad((query) => {
  const id = String(query?.projectId || "").trim();
  if (id) {
    projectId.value = id;
    void flow.openProject(id).then(() => {
      title.value = flow.state.title || title.value;
      requirement.value = flow.state.requirement || requirement.value;
    });
  } else {
    flow.reset();
    flow.state.title = title.value;
  }
});

onShow(() => flow.setForeground(true));
onHide(() => flow.setForeground(false));

function goBack() {
  if (getCurrentPages().length > 1) uni.navigateBack();
  else uni.switchTab({ url: "/pages/user/UserHomePage" });
}

function removePick(index: number) {
  picked.value = picked.value.filter((_, i) => i !== index);
}

function chooseMedia() {
  uni.chooseMedia({
    count: 9,
    mediaType: ["video", "image"],
    sourceType: ["album", "camera"],
    success: (result) => {
      const next: LocalMediaPick[] = (result.tempFiles || []).map((file, index) => {
        const path = String(file.tempFilePath || "");
        const fileType = String(file.fileType || "").toLowerCase();
        const assetType = fileType === "image" ? "IMAGE" : assetTypeFromMime(fileType, "VIDEO");
        const ext = assetType === "IMAGE" ? "jpg" : "mp4";
        return {
          path,
          name: `素材${picked.value.length + index + 1}.${ext}`,
          mimeType: assetType === "IMAGE" ? "image/jpeg" : "video/mp4",
          size: Number(file.size) || undefined,
          assetType,
        };
      }).filter((item) => item.path);
      picked.value = [...picked.value, ...next].slice(0, 12);
    },
    fail: (error) => {
      if (/cancel/i.test(String(error.errMsg || ""))) return;
      uni.showToast({ title: "选择素材失败", icon: "none" });
    },
  });
}

async function startFlow() {
  if (!canStart.value || busy.value) return;
  flow.clearError();
  flow.state.title = title.value.trim() || "未命名混剪项目";
  flow.state.requirement = requirement.value.trim();
  try {
    await flow.uploadMedia(picked.value);
    await flow.startAnalysis();
    const id = flow.state.project?.id;
    if (!id) throw new Error("项目创建失败");
    uni.redirectTo({
      url: `/packageSmartVideo/pages/plan?projectId=${encodeURIComponent(id)}`,
    });
  } catch (error) {
    uni.showToast({
      title: error instanceof Error ? error.message : "启动失败",
      icon: "none",
    });
  }
}
</script>

<style scoped>
.sv-page {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f7fb;
  color: #181c28;
  font-family: -apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif;
}
.sv-nav {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--header-padding-top, 20px) var(--capsule-right-space, 16px) 10px 12px;
  background: #fff;
}
.nav-back,
.text-btn,
.primary {
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  line-height: 1.2;
}
.nav-back::after,
.text-btn::after,
.primary::after {
  display: none;
}
.nav-back {
  width: 36px;
  height: 36px;
  font-size: 28px;
  color: #181c28;
}
.nav-title {
  font-size: 17px;
  font-weight: 700;
}
.nav-spacer {
  width: 36px;
}
.sv-scroll {
  flex: 1;
  height: 0;
}
.sv-body {
  padding: 16px 16px 120px;
}
.lead {
  display: block;
  margin-bottom: 16px;
  color: #697084;
  font-size: 13px;
  line-height: 20px;
}
.field {
  display: block;
  margin-bottom: 14px;
  padding: 14px;
  border-radius: 14px;
  background: #fff;
}
.label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}
.label {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  font-weight: 600;
}
input,
textarea {
  width: 100%;
  font-size: 14px;
  color: #181c28;
}
textarea {
  min-height: 96px;
}
.empty-box,
.upload-box,
.media-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.empty-box {
  padding: 18px 0 8px;
  color: #697084;
  font-size: 13px;
}
.hint {
  font-size: 12px;
  color: #9aa3b5;
}
.media-row,
.upload-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.media-tag {
  flex: none;
  padding: 2px 6px;
  border-radius: 6px;
  background: #eef2ff;
  color: #4a6bff;
  font-size: 11px;
}
.media-name,
.upload-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}
.upload-status {
  flex: none;
  color: #697084;
  font-size: 12px;
}
.text-btn {
  color: #4a6bff;
  font-size: 13px;
}
.text-btn.danger {
  color: #d94848;
}
.error {
  display: block;
  margin-top: 8px;
  color: #d94848;
  font-size: 13px;
}
.sv-footer {
  position: sticky;
  bottom: 0;
  padding: 12px 16px calc(12px + env(safe-area-inset-bottom));
  background: rgba(245, 247, 251, 0.96);
}
.primary {
  width: 100%;
  height: 48px;
  border-radius: 14px;
  background: #4a6bff;
  color: #fff;
  font-size: 16px;
  font-weight: 700;
}
.primary[disabled] {
  opacity: 0.45;
}
</style>
