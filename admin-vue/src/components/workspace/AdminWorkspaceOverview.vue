<template>
  <div class="workspace-overview">
    <section v-if="showSearchPanel" class="global-search-panel">
      <div class="global-search-head">
        <div><strong>搜索结果</strong><small>关键词：{{ searchKeyword.trim() }}</small></div>
        <el-button size="small" @click="clearSearch">清空</el-button>
      </div>
      <div class="global-search-grid">
        <article class="global-search-card">
          <span>模块入口</span>
          <button v-for="item in globalModuleResults" :key="item.id" type="button" @click="selectAdminModule(item.id)"><strong>{{ item.title }}</strong><small>{{ pageMeta[item.id]?.description || '进入模块查看详情' }}</small></button>
          <el-empty v-if="!globalModuleResults.length" description="没有匹配模块" :image-size="56" />
        </article>
        <article class="global-search-card">
          <span>当前模块数据</span>
          <button v-for="item in currentRecordResults" :key="item.key" type="button" @click="openCurrentRecordResult(item)"><strong>{{ item.title }}</strong><small>{{ item.desc }}</small></button>
          <el-empty v-if="!currentRecordResults.length" description="当前模块没有匹配记录" :image-size="56" />
        </article>
      </div>
    </section>

    <section v-if="showOverview" class="module-hero">
      <div>
        <el-tag effect="dark" type="primary">{{ activeModuleMeta.badge }}</el-tag>
        <h2>{{ activeModuleTitle }}</h2>
        <p>{{ activeModuleMeta.description }}</p>
      </div>
      <div class="module-hero-actions">
        <el-button v-for="action in toolbarActions" :key="action.action" type="primary" :icon="plusIcon" @click="runAction(action.action)">{{ action.label }}</el-button>
        <el-button :icon="refreshIcon" @click="reload">刷新数据</el-button>
      </div>
    </section>

    <div v-if="showOverview" class="metric-grid">
      <article v-for="metric in metrics" :key="metric.label" class="metric-card">
        <span>{{ metric.label }}</span>
        <strong>{{ metric.value }}</strong>
        <small>{{ metricHint(metric.label) }}</small>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Component } from "vue";

type SearchResult = { id: string; title: string };
type RecordResult = { key: string; title: string; desc: string; row?: Record<string, any> };
type Metric = { label: string; value: string | number };
type ToolbarAction = { action: string; label: string };
type PageMeta = Record<string, { description?: string }>;

defineProps<{
  searchKeyword: string;
  showSearchPanel: boolean;
  showOverview: boolean;
  activeModuleTitle: string;
  activeModuleMeta: { badge: string; description: string };
  globalModuleResults: SearchResult[];
  currentRecordResults: RecordResult[];
  metrics: Metric[];
  toolbarActions: ToolbarAction[];
  pageMeta: PageMeta;
  plusIcon: Component;
  refreshIcon: Component;
  metricHint: (label: string) => string;
  selectAdminModule: (moduleId: string) => void;
  openCurrentRecordResult: (item: { key: string; title: string; desc: string; row?: Record<string, any> }) => void;
  runAction: (action: string) => void;
  reload: () => void;
}>();

const emit = defineEmits<{ clearSearch: [] }>();
const clearSearch = () => emit("clearSearch");
</script>

<style scoped>
.global-search-panel{display:grid;gap:16px;padding:18px;border:1px solid #e6ebf5;border-radius:22px;background:linear-gradient(180deg,#fff,#fbfcff);box-shadow:0 14px 40px rgba(16,24,40,.06)}.global-search-head,.module-hero,.module-hero-actions{display:flex;align-items:center;gap:12px}.global-search-head,.module-hero{justify-content:space-between}.global-search-head strong{display:block;color:#101828;font-size:18px}.global-search-head small,.module-hero p,.global-search-card small,.metric-card small{color:#667085;line-height:1.55}.global-search-grid,.metric-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.global-search-card,.metric-card{display:grid;gap:10px;padding:16px;border-radius:18px;background:#fff;border:1px solid #e8edf5;box-shadow:0 10px 26px rgba(15,23,42,.04)}.global-search-card>span,.metric-card>span{color:#98a2b3;font-size:12px;font-weight:800;text-transform:uppercase;letter-spacing:.08em}.global-search-card button{display:grid;gap:4px;padding:14px;border:0;border-radius:14px;text-align:left;background:#f9fbff;cursor:pointer}.global-search-card button strong{color:#101828}.module-hero{padding:18px;border:1px solid #e6ebf5;border-radius:22px;background:linear-gradient(135deg,#f6f8ff,#fff)}.module-hero h2{margin:10px 0 8px;color:#101828;font-size:28px;line-height:1.15}.module-hero p{margin:0;max-width:58ch}.module-hero-actions{justify-content:flex-end;flex-wrap:wrap}.metric-grid{margin-top:16px}@media (max-width:960px){.global-search-grid,.metric-grid{grid-template-columns:1fr}.global-search-head,.module-hero{align-items:flex-start;flex-direction:column}.module-hero-actions{justify-content:flex-start}}
</style>
