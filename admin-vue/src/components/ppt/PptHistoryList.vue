<template>
  <section class="ppt-history-list" :class="`is-${viewMode}`" @click="closeActionMenu" @keyup.esc="closeActionMenu">
    <article
      v-for="item in history"
      :key="item.taskId"
      class="ppt-history-card"
      :class="{ favorited: isFavorite(item) }"
    >
      <button type="button" class="ppt-history-open" :title="`预览 ${item.title}`" :aria-label="`预览 ${item.title}`" @click="openHistoryItem(item)" />

      <div class="ppt-history-preview" aria-hidden="true">
        <svg class="ppt-folder-icon" viewBox="0 0 24 24">
          <path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z" />
        </svg>
      </div>

      <div class="ppt-card-actions" @click.stop @keydown.stop>
        <button
          type="button"
          class="ppt-card-icon-button favorite"
          :class="{ active: isFavorite(item) }"
          :aria-label="isFavorite(item) ? `取消收藏 ${item.title}` : `收藏 ${item.title}`"
          :aria-pressed="isFavorite(item)"
          :title="isFavorite(item) ? '取消收藏' : '加入收藏夹'"
          @click="emitCardAction('toggle-favorite', item)"
        >
          <svg class="ppt-card-action-icon" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z" />
          </svg>
        </button>

        <div class="ppt-card-menu-wrap">
          <button
            type="button"
            class="ppt-card-icon-button"
            :aria-label="`打开 ${item.title} 的操作菜单`"
            :aria-expanded="openActionMenuId === item.taskId"
            aria-haspopup="menu"
            title="更多操作"
            @click="toggleActionMenu(item.taskId)"
          >
            <svg class="ppt-card-action-icon" viewBox="0 0 24 24" aria-hidden="true">
              <circle cx="12" cy="12" r="1" />
              <circle cx="12" cy="5" r="1" />
              <circle cx="12" cy="19" r="1" />
            </svg>
          </button>

          <div v-if="openActionMenuId === item.taskId" class="ppt-card-action-menu" role="menu">
            <button type="button" role="menuitem" :title="`继续编辑 ${item.title}`" :aria-label="`继续编辑 ${item.title}`" @click="emitCardAction('edit', item)">
              <svg class="ppt-card-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z" />
                <path d="m15 5 4 4" />
              </svg>
              继续编辑
            </button>
            <button type="button" role="menuitem" :title="`重新生成 ${item.title}`" :aria-label="`重新生成 ${item.title}`" @click="emitCardAction('regenerate', item)">
              <svg class="ppt-card-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                <rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
                <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
              </svg>
              重新生成
            </button>
            <button
              type="button"
              role="menuitem"
              :title="isFavorite(item) ? `取消收藏 ${item.title}` : `加入收藏 ${item.title}`"
              :aria-label="isFavorite(item) ? `取消收藏 ${item.title}` : `加入收藏 ${item.title}`"
              @click="emitCardAction('toggle-favorite', item)"
            >
              <svg class="ppt-card-menu-icon star" :class="{ active: isFavorite(item) }" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z" />
              </svg>
              {{ isFavorite(item) ? "取消收藏" : "加入收藏夹" }}
            </button>
            <div class="ppt-menu-separator" />
            <button
              type="button"
              role="menuitem"
              :disabled="!item.pptUrl"
              :title="item.pptUrl ? `下载 ${item.title} 的 PPT` : 'PPT下载文件尚未生成'"
              :aria-label="item.pptUrl ? `下载 ${item.title} 的 PPT` : 'PPT下载文件尚未生成'"
              @click="emitCardAction('download-ppt', item)"
            >
              <svg class="ppt-card-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M12 15V3" />
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                <path d="m7 10 5 5 5-5" />
              </svg>
              下载PPT
            </button>
            <button
              type="button"
              role="menuitem"
              :disabled="!item.pdfUrl"
              :title="item.pdfUrl ? `下载 ${item.title} 的 PDF` : 'PDF下载文件尚未生成'"
              :aria-label="item.pdfUrl ? `下载 ${item.title} 的 PDF` : 'PDF下载文件尚未生成'"
              @click="emitCardAction('download-pdf', item)"
            >
              <svg class="ppt-card-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" />
                <path d="M14 2v4a2 2 0 0 0 2 2h4" />
                <path d="M12 18v-6" />
                <path d="m9 15 3 3 3-3" />
              </svg>
              下载PDF
            </button>
            <div class="ppt-menu-separator" />
            <button type="button" role="menuitem" class="danger" :title="`删除 ${item.title}`" :aria-label="`删除 ${item.title}`" @click="emitCardAction('delete', item)">
              <svg class="ppt-card-menu-icon" viewBox="0 0 24 24" aria-hidden="true">
                <path d="M3 6h18" />
                <path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6" />
                <path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" />
                <line x1="10" x2="10" y1="11" y2="17" />
                <line x1="14" x2="14" y1="11" y2="17" />
              </svg>
              删除
            </button>
          </div>
        </div>
      </div>

      <main class="ppt-history-card-body">
        <strong>{{ item.title }}</strong>
        <span>{{ formatTime(item.updatedAt || item.createdAt) }}</span>
        <small>{{ item.slideCount || 5 }}页 · {{ languageLabel(item.language) }} · {{ themeLabel(item.theme || "business") }}</small>
        <el-tag class="ppt-history-status" :type="statusType(item.status)" size="small">{{ statusLabel(item.status) }}</el-tag>
      </main>
    </article>
    <PptEmptyState v-if="!history.length" :title="emptyTitle || '暂无演示文稿'" :description="emptyDescription || '生成完成后会出现在最近生成记录中。'" />
  </section>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { pptThemeLabel } from "../../config/pptThemes";
import type { PptHistoryItem, PptLanguage, PptTaskStatus } from "../../types/ppt";
import PptEmptyState from "./PptEmptyState.vue";

const props = defineProps<{
  history: PptHistoryItem[];
  viewMode: "grid" | "list";
  favoriteTaskIds?: string[];
  emptyTitle?: string;
  emptyDescription?: string;
}>();

type CardAction = "edit" | "download-ppt" | "download-pdf" | "regenerate" | "toggle-favorite" | "delete";

const emit = defineEmits<{
  preview: [item: PptHistoryItem];
  edit: [item: PptHistoryItem];
  "download-ppt": [item: PptHistoryItem];
  "download-pdf": [item: PptHistoryItem];
  regenerate: [item: PptHistoryItem];
  "toggle-favorite": [item: PptHistoryItem];
  delete: [item: PptHistoryItem];
}>();

const openActionMenuId = ref("");

function isFavorite(item: PptHistoryItem) {
  return props.favoriteTaskIds?.includes(item.taskId) || false;
}

function toggleActionMenu(taskId: string) {
  openActionMenuId.value = openActionMenuId.value === taskId ? "" : taskId;
}

function closeActionMenu() {
  openActionMenuId.value = "";
}

function openHistoryItem(item: PptHistoryItem) {
  emit("preview", item);
  closeActionMenu();
}

function emitCardAction(eventName: CardAction, item: PptHistoryItem) {
  if (eventName === "edit") emit("edit", item);
  if (eventName === "download-ppt") emit("download-ppt", item);
  if (eventName === "download-pdf") emit("download-pdf", item);
  if (eventName === "regenerate") emit("regenerate", item);
  if (eventName === "toggle-favorite") emit("toggle-favorite", item);
  if (eventName === "delete") emit("delete", item);
  closeActionMenu();
}

function languageLabel(language?: PptLanguage) {
  return language === "en" ? "英文" : "中文";
}

function themeLabel(theme: string) {
  return pptThemeLabel(theme);
}

function statusLabel(status: PptTaskStatus) {
  if (status === "draft") return "草稿";
  if (status === "success") return "成功";
  if (status === "failed") return "失败";
  return "生成中";
}

function statusType(status: PptTaskStatus) {
  if (status === "draft") return "warning";
  if (status === "success") return "success";
  if (status === "failed") return "danger";
  return "info";
}

function formatTime(value?: string) {
  if (!value) return "刚刚";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "刚刚";
  return date.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" });
}
</script>

<style scoped>
.ppt-history-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(220px, 1fr));
  gap: 14px;
}

.ppt-history-list.is-list {
  grid-template-columns: 1fr;
}

.ppt-history-list.is-list .ppt-history-card {
  display: grid;
  grid-template-columns: 96px minmax(0, 1fr) auto;
  align-items: center;
  min-height: 76px;
}

.ppt-history-list.is-list .ppt-history-preview {
  grid-column: 1;
  grid-row: 1;
  width: 96px;
  height: 58px;
  min-height: 58px;
  margin: 10px 0 10px 12px;
  border-radius: 7px;
}

.ppt-history-list.is-list .ppt-history-card-body {
  grid-column: 2;
  grid-row: 1;
  padding: 12px 18px;
}

.ppt-history-list.is-list .ppt-card-actions {
  grid-column: 3;
  grid-row: 1;
  position: relative;
  top: auto;
  right: auto;
  justify-self: end;
  padding-right: 12px;
  opacity: 1;
}

.ppt-history-card {
  position: relative;
  overflow: visible;
  border: 1px solid #262626;
  border-radius: 8px;
  background: #0d0d0d;
  transition: border-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.ppt-history-card:hover,
.ppt-history-card:focus-within {
  border-color: #3a3a3a;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.32);
}

.ppt-history-open {
  position: absolute;
  inset: 0;
  z-index: 1;
  border: 0;
  border-radius: 8px;
  background: transparent;
  cursor: pointer;
}

.ppt-history-open:focus-visible {
  outline: 2px solid rgba(32, 212, 191, 0.7);
  outline-offset: 2px;
}

.ppt-history-preview {
  position: relative;
  z-index: 2;
  display: grid;
  place-items: center;
  height: 142px;
  color: #666;
  background: #171717;
  font-size: 42px;
  overflow: hidden;
  pointer-events: none;
}

.ppt-history-preview::after {
  content: "";
  position: absolute;
  inset: 0;
  opacity: 0;
  background: linear-gradient(180deg, transparent, rgba(0, 0, 0, 0.16));
  transition: opacity 0.18s ease;
}

.ppt-history-card:hover .ppt-history-preview::after {
  opacity: 1;
}

.ppt-folder-icon {
  width: 46px;
  height: 46px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  opacity: 0.72;
}

.ppt-history-card-body {
  position: relative;
  z-index: 2;
  display: grid;
  gap: 7px;
  padding: 14px;
  pointer-events: none;
}

.ppt-history-card strong {
  overflow: hidden;
  color: #f4f4f5;
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-history-card:hover strong {
  color: #ffffff;
}

.ppt-history-card span,
.ppt-history-card small {
  overflow: hidden;
  color: #a1a1aa;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ppt-history-status {
  width: fit-content;
}

.ppt-card-actions {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 5;
  display: flex;
  align-items: center;
  gap: 6px;
  opacity: 0;
  transition: opacity 0.18s ease;
}

.ppt-history-card:hover .ppt-card-actions,
.ppt-history-card:focus-within .ppt-card-actions,
.ppt-history-card.favorited .ppt-card-actions {
  opacity: 1;
}

.ppt-card-icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 1px solid rgba(64, 64, 64, 0.78);
  border-radius: 999px;
  color: #a1a1aa;
  background: rgba(12, 12, 12, 0.88);
  box-shadow: 0 8px 22px rgba(0, 0, 0, 0.28);
  backdrop-filter: blur(10px);
  cursor: pointer;
  touch-action: manipulation;
  transition: background-color 0.16s ease, border-color 0.16s ease, color 0.16s ease, transform 0.16s ease;
}

.ppt-card-icon-button:hover,
.ppt-card-icon-button:focus-visible {
  color: #f4f4f5;
  border-color: #4a4a4a;
  background: #181818;
  outline: 0;
  transform: translateY(-1px);
}

.ppt-card-icon-button.favorite.active {
  color: #facc15;
  border-color: rgba(250, 204, 21, 0.52);
  background: rgba(250, 204, 21, 0.12);
}

.ppt-card-icon-button.favorite.active .ppt-card-action-icon {
  fill: currentColor;
}

.ppt-card-action-icon,
.ppt-card-menu-icon {
  width: 16px;
  height: 16px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.ppt-card-menu-wrap {
  position: relative;
}

.ppt-card-action-menu {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  z-index: 12;
  display: grid;
  gap: 4px;
  width: min(172px, calc(100vw - 24px));
  max-height: min(460px, calc(100vh - 140px));
  overflow: auto;
  padding: 7px;
  border: 1px solid #2b2b2b;
  border-radius: 10px;
  background: #101010;
  box-shadow: 0 18px 56px rgba(0, 0, 0, 0.54);
  overscroll-behavior: contain;
}

.ppt-card-action-menu button {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  min-height: 34px;
  padding: 0 9px;
  border: 0;
  border-radius: 7px;
  color: #e5e5e5;
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  text-align: left;
  touch-action: manipulation;
}

.ppt-card-action-menu button:hover,
.ppt-card-action-menu button:focus-visible {
  background: #1c1c1c;
  outline: 0;
}

.ppt-card-action-menu button:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.ppt-card-action-menu button:disabled:hover,
.ppt-card-action-menu button:disabled:focus-visible {
  background: transparent;
}

.ppt-card-action-menu button.danger {
  color: #fecaca;
}

.ppt-card-menu-icon.star.active {
  color: #facc15;
  fill: currentColor;
}

.ppt-menu-separator {
  height: 1px;
  margin: 4px 2px;
  background: #262626;
}

@media (max-width: 1100px) {
  .ppt-history-list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 680px) {
  .ppt-history-list {
    grid-template-columns: 1fr;
  }

  .ppt-history-list.is-list .ppt-history-card {
    display: grid;
    grid-template-columns: 76px minmax(0, 1fr) auto;
  }

  .ppt-history-list.is-list .ppt-history-preview {
    width: 64px;
    height: 46px;
    min-height: 46px;
    margin-left: 10px;
  }

  .ppt-history-list.is-list .ppt-history-card-body {
    padding: 10px 12px;
  }

  .ppt-history-list.is-list .ppt-card-actions {
    gap: 4px;
    padding-right: 8px;
  }

  .ppt-card-icon-button {
    width: 30px;
    height: 30px;
  }

  .ppt-card-action-menu {
    right: -4px;
  }
}
</style>
