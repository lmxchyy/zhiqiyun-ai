<template>
  <section class="sv-page" aria-label="AI 自动混剪工作台">
    <header class="sv-topbar">
      <div class="sv-brand">
        <strong>AI 自动混剪</strong>
        <span>{{ store.statusText }}</span>
      </div>
      <div class="sv-top-actions">
        <button v-if="store.phase !== 'list'" type="button" class="sv-btn ghost" @click="backToList">项目列表</button>
        <button type="button" class="sv-btn primary" :disabled="store.busy" @click="store.startCreate()">新建项目</button>
      </div>
    </header>

    <p v-if="store.errorMessage" class="sv-alert" role="alert">
      {{ store.errorMessage }}
      <button type="button" class="sv-link" @click="store.clearError()">关闭</button>
    </p>

    <div v-if="store.phase === 'list'" class="sv-panel">
      <div class="sv-panel-head">
        <h1>我的混剪项目</h1>
        <button type="button" class="sv-btn ghost" :disabled="store.busy" @click="store.initialize()">刷新</button>
      </div>
      <div v-if="!store.projects.length" class="sv-empty">
        <p>还没有项目。上传素材、写清需求，AI 会生成可编辑分镜并导出成片。</p>
        <button type="button" class="sv-btn primary" @click="store.startCreate()">开始创建</button>
      </div>
      <ul v-else class="sv-project-list" role="list">
        <li v-for="item in store.projects" :key="item.id" class="sv-project-card">
          <button type="button" class="sv-project-main" @click="store.openProject(item.id)">
            <strong>{{ item.title || "未命名项目" }}</strong>
            <span>{{ item.status }} · 更新于 {{ formatTime(item.updatedAt) }}</span>
          </button>
          <button type="button" class="sv-btn danger ghost" :disabled="store.busy" @click="confirmDelete(item.id)">删除</button>
        </li>
      </ul>
    </div>

    <div v-else class="sv-workspace">
      <aside class="sv-rail" aria-label="素材与版本">
        <SmartVideoUploadPanel />
        <SmartVideoVersionList />
      </aside>

      <main class="sv-main">
        <section class="sv-card">
          <h2>项目需求</h2>
          <label class="sv-field">
            <span>标题</span>
            <input v-model="store.title" type="text" maxlength="80" placeholder="例如：门店开业短视频" />
          </label>
          <label class="sv-field">
            <span>创作需求</span>
            <textarea
              v-model="store.requirement"
              rows="4"
              maxlength="2000"
              placeholder="说明时长、风格、旁白语气、必须出现的画面等"
            />
          </label>
          <label class="sv-field">
            <span>规划补充指令（可选）</span>
            <input v-model="store.instruction" type="text" maxlength="500" placeholder="例如：节奏更快，旁白口语化" />
          </label>
          <div class="sv-actions">
            <button type="button" class="sv-btn" :disabled="store.busy" @click="store.saveProjectMeta()">保存项目</button>
            <button type="button" class="sv-btn" :disabled="store.busy || !store.assets.length" @click="store.startAnalysis()">分析素材</button>
            <button type="button" class="sv-btn primary" :disabled="store.busy || !store.assets.length" @click="store.generatePlan()">生成方案</button>
          </div>
          <SmartVideoAnalysisStatus />
        </section>

        <SmartVideoStoryboard />
        <SmartVideoRenderPanel />
      </main>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted } from "vue";
import { useSmartVideoStore } from "../../stores/smartVideo";
import SmartVideoAnalysisStatus from "./SmartVideoAnalysisStatus.vue";
import SmartVideoRenderPanel from "./SmartVideoRenderPanel.vue";
import SmartVideoStoryboard from "./SmartVideoStoryboard.vue";
import SmartVideoUploadPanel from "./SmartVideoUploadPanel.vue";
import SmartVideoVersionList from "./SmartVideoVersionList.vue";

const store = useSmartVideoStore();

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function backToList() {
  if (store.hasUnsavedChanges && !window.confirm("有未保存修改或上传进行中，确定返回列表？")) return;
  store.resetWorkspace();
  void store.initialize();
}

function confirmDelete(projectId: string) {
  if (!window.confirm("确定删除该项目？删除后不可恢复。")) return;
  void store.removeProject(projectId);
}

function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!store.hasUnsavedChanges) return;
  event.preventDefault();
  event.returnValue = "";
}

onMounted(() => {
  void store.initialize();
  window.addEventListener("beforeunload", onBeforeUnload);
});

onBeforeUnmount(() => {
  window.removeEventListener("beforeunload", onBeforeUnload);
  store.dispose();
});
</script>

<style scoped>
.sv-page {
  --sv-bg: #0f1218;
  --sv-panel: #171b24;
  --sv-line: rgba(255, 255, 255, 0.08);
  --sv-text: #f4f6fb;
  --sv-muted: #9aa3b5;
  --sv-accent: #ff771b;
  --sv-accent-2: #423499;
  min-height: calc(100vh - 72px);
  padding: 20px 24px 40px;
  color: var(--sv-text);
  background:
    radial-gradient(circle at top right, rgba(66, 52, 153, 0.28), transparent 40%),
    linear-gradient(180deg, #12161f 0%, #0d1016 100%);
}

.sv-topbar,
.sv-panel-head,
.sv-actions,
.sv-top-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sv-topbar,
.sv-panel-head {
  justify-content: space-between;
  margin-bottom: 16px;
}

.sv-brand {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.sv-brand strong {
  font-size: 22px;
  letter-spacing: 0.02em;
}

.sv-brand span,
.sv-field span,
.sv-empty,
.sv-project-main span {
  color: var(--sv-muted);
  font-size: 13px;
}

.sv-alert {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin: 0 0 16px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(255, 80, 80, 0.12);
  border: 1px solid rgba(255, 80, 80, 0.35);
}

.sv-panel,
.sv-card,
.sv-project-card {
  border: 1px solid var(--sv-line);
  border-radius: 16px;
  background: rgba(23, 27, 36, 0.92);
}

.sv-panel,
.sv-card {
  padding: 18px;
}

.sv-workspace {
  display: grid;
  grid-template-columns: minmax(280px, 340px) minmax(0, 1fr);
  gap: 16px;
}

.sv-rail,
.sv-main {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.sv-empty {
  padding: 28px 8px;
  text-align: center;
}

.sv-project-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 10px;
}

.sv-project-card {
  display: flex;
  align-items: center;
  padding: 8px;
}

.sv-project-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 10px 12px;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.sv-field {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 14px;
}

.sv-field input,
.sv-field textarea {
  width: 100%;
  border: 1px solid var(--sv-line);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.03);
  color: var(--sv-text);
  padding: 10px 12px;
  outline: none;
}

.sv-field input:focus,
.sv-field textarea:focus {
  border-color: rgba(255, 119, 27, 0.7);
  box-shadow: 0 0 0 3px rgba(255, 119, 27, 0.15);
}

.sv-btn,
.sv-link {
  border: 1px solid var(--sv-line);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: var(--sv-text);
  padding: 8px 14px;
  cursor: pointer;
}

.sv-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.sv-btn.primary {
  border-color: transparent;
  background: linear-gradient(135deg, var(--sv-accent), #ff9a4d);
  color: #111;
  font-weight: 600;
}

.sv-btn.ghost {
  background: transparent;
}

.sv-btn.danger {
  color: #ff8f8f;
}

.sv-link {
  border: 0;
  background: transparent;
  color: #ffb4b4;
  padding: 0;
}

.sv-actions {
  flex-wrap: wrap;
}

@media (max-width: 960px) {
  .sv-workspace {
    grid-template-columns: 1fr;
  }
}
</style>
