<template>
  <section v-if="shouldRender" class="ppt-agent-activity-inline">
    <button
      type="button"
      class="ppt-agent-activity-trigger"
      :title="isExpanded ? '收起 Agent 活动' : '展开 Agent 活动'"
      :aria-label="isExpanded ? '收起 Agent 活动' : '展开 Agent 活动'"
      :aria-expanded="isExpanded"
      @click="isExpanded = !isExpanded"
    >
      <span class="ppt-agent-activity-title">
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <path d="m21 21-4.34-4.34" />
          <circle cx="11" cy="11" r="8" />
        </svg>
        <strong>Agent 活动</strong>
      </span>
      <span class="ppt-agent-activity-side">
        <i v-if="isRunning" class="ppt-agent-spinner" aria-hidden="true"></i>
        <small>{{ isExpanded ? "收起" : "展开" }}</small>
      </span>
    </button>

    <div v-if="isExpanded" class="ppt-agent-activity-content">
      <article v-if="uploadedDocumentName" class="ppt-agent-activity-card is-document">
        <span>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z" />
            <path d="M14 2v6h6" />
            <path d="M8 13h8" />
            <path d="M8 17h5" />
          </svg>
        </span>
        <div>
          <strong>{{ uploadedDocumentName }}</strong>
          <small>{{ isRunning ? "处理中" : "已处理" }}</small>
        </div>
        <b>{{ isRunning ? "处理中" : "已处理" }}</b>
      </article>

      <article v-if="enableWebSearch" class="ppt-agent-activity-card" :class="searchStateClass">
        <span>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path d="m21 21-4.34-4.34" />
            <circle cx="11" cy="11" r="8" />
          </svg>
        </span>
        <div>
          <strong>{{ isRunning ? "正在搜索" : "已完成搜索" }}</strong>
          <small>{{ prompt || "演示主题" }}</small>
        </div>
        <b>{{ isRunning ? "运行中" : "已完成" }}</b>
      </article>

      <article v-for="item in activityItems" :key="item.key" class="ppt-agent-activity-card" :class="`is-${item.state}`">
        <span>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path v-if="item.state === 'done'" d="m5 13 4 4L19 7" />
            <path v-else-if="item.state === 'failed'" d="M18 6 6 18M6 6l12 12" />
            <path v-else-if="item.state === 'active'" d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
            <path v-else d="M12 6v6l4 2" />
            <circle v-if="item.state === 'pending'" cx="12" cy="12" r="9" />
          </svg>
        </span>
        <div>
          <strong>{{ item.label }}</strong>
          <small>{{ item.description }}</small>
        </div>
        <b>{{ stateLabel(item.state) }}</b>
      </article>

      <div v-if="!activityItems.length && !enableWebSearch && !uploadedDocumentName" class="ppt-agent-empty">
        暂无活动。
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";

type ActivityItem = {
  key: string;
  label: string;
  description: string;
  state: string;
};

const props = withDefaults(defineProps<{
  activityItems: ActivityItem[];
  isRunning?: boolean;
  enableWebSearch?: boolean;
  prompt?: string;
  uploadedDocumentName?: string;
  defaultExpanded?: boolean;
}>(), {
  isRunning: false,
  enableWebSearch: false,
  prompt: "",
  uploadedDocumentName: "",
  defaultExpanded: false
});

const isExpanded = ref(props.defaultExpanded);
const hasDisplayContent = computed(() => props.activityItems.length > 0 || props.enableWebSearch || Boolean(props.uploadedDocumentName));
const shouldRender = computed(() => hasDisplayContent.value || props.isRunning);
const searchStateClass = computed(() => props.isRunning ? "is-active" : "is-done");

watch(
  () => props.isRunning,
  (value) => {
    if (value) isExpanded.value = true;
  }
);

function stateLabel(state: string) {
  const map: Record<string, string> = {
    pending: "等待",
    active: "运行中",
    done: "已完成",
    failed: "失败"
  };
  return map[state];
}
</script>

<style scoped>
.ppt-agent-activity-inline {
  display: grid;
  gap: 8px;
}

.ppt-agent-activity-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  min-height: 48px;
  padding: 0 13px;
  border: 1px solid #202020;
  border-radius: 10px;
  color: #f4f4f5;
  background: rgba(24, 24, 27, 0.72);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.16s ease, background-color 0.16s ease;
}

.ppt-agent-activity-trigger:hover {
  border-color: #3f3f46;
  background: rgba(39, 39, 42, 0.86);
}

.ppt-agent-activity-trigger:focus-visible {
  outline: 2px solid rgba(34, 211, 238, 0.72);
  outline-offset: 2px;
}

.ppt-agent-activity-title,
.ppt-agent-activity-side {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.ppt-agent-activity-title strong {
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-agent-activity-side small {
  color: #a1a1aa;
  font-size: 12px;
}

.ppt-agent-activity-content {
  display: grid;
  gap: 8px;
  padding: 0 16px 2px;
}

.ppt-agent-activity-card {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  min-height: 58px;
  padding: 10px 12px;
  border: 1px solid rgba(96, 165, 250, 0.2);
  border-radius: 10px;
  background: #0d0d0d;
}

.ppt-agent-activity-card > span {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  color: #93c5fd;
  background: rgba(37, 99, 235, 0.14);
}

.ppt-agent-activity-card div {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.ppt-agent-activity-card strong {
  overflow: hidden;
  color: #f4f4f5;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-agent-activity-card small {
  overflow: hidden;
  color: #a1a1aa;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-agent-activity-card b {
  color: #8f8f95;
  font-size: 11px;
  white-space: nowrap;
}

.ppt-agent-activity-card.is-active > span {
  color: #bfdbfe;
  background: rgba(37, 99, 235, 0.24);
}

.ppt-agent-activity-card.is-active > span svg {
  animation: ppt-agent-spin 1.1s linear infinite;
}

.ppt-agent-activity-card.is-done > span,
.ppt-agent-activity-card.is-document > span {
  color: #bbf7d0;
  background: rgba(22, 101, 52, 0.24);
}

.ppt-agent-activity-card.is-failed > span {
  color: #fecaca;
  background: rgba(127, 29, 29, 0.28);
}

.ppt-agent-empty {
  padding: 12px;
  border: 1px solid rgba(96, 165, 250, 0.18);
  border-radius: 10px;
  color: #8f8f95;
  background: #0d0d0d;
  font-size: 12px;
}

.ppt-agent-spinner {
  width: 15px;
  height: 15px;
  border: 2px solid rgba(96, 165, 250, 0.22);
  border-top-color: #60a5fa;
  border-radius: 999px;
  animation: ppt-agent-spin 0.8s linear infinite;
}

.ppt-agent-activity-inline svg {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

@keyframes ppt-agent-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 640px) {
  .ppt-agent-activity-content {
    padding: 0;
  }

  .ppt-agent-activity-card {
    grid-template-columns: 30px minmax(0, 1fr);
  }

  .ppt-agent-activity-card b {
    grid-column: 2;
  }
}
</style>
