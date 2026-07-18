<template>
  <el-card shadow="never" class="experience-insights">
    <template #header><div class="panel-head"><span>后台体验数据</span><el-button link type="primary" :loading="loading" @click="load">刷新</el-button></div></template>
    <el-skeleton v-if="loading && !analytics" :rows="4" animated />
    <template v-else-if="analytics">
      <div class="insight-metrics"><article><span>真人体验事件</span><strong>{{ analytics.totalEvents }}</strong><small>已排除 {{ analytics.syntheticEvents }} 条自动化事件</small></article><article><span>搜索跳转</span><strong>{{ analytics.eventCounts.SEARCH_RESULT_CLICK || 0 }}</strong><small>{{ analytics.uniqueSessions }} 个会话 · {{ analytics.activeDays }} 个活跃日</small></article><article><span>任务完成率</span><strong>{{ Math.round(analytics.taskCompletionRate * 100) }}%</strong><small>{{ analytics.eventCounts.TASK_COMPLETED || 0 }} / {{ analytics.eventCounts.TASK_STARTED || 0 }}</small></article></div>
      <el-alert v-if="!analytics.sampleReady" type="info" title="真实样本仍在积累" :description="sampleProgress" show-icon :closable="false" />
      <div class="low-frequency"><strong>低频入口观察名单</strong><template v-if="analytics.sampleReady"><span v-for="item in displayCandidates" :key="item.moduleId"><b>{{ moduleTitle(item.moduleId) }}</b><small>{{ item.count }} 次访问</small></span></template><el-empty v-else description="样本成熟后生成候选名单，当前不建议合并入口" :image-size="54" /><el-empty v-if="analytics.sampleReady && !displayCandidates.length" description="暂无低频入口候选" :image-size="54" /></div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { adminWorkspaceApi, type ExperienceAnalytics } from "../../api/adminWorkspaces";
const props = defineProps<{ moduleLabels?: Record<string, string> }>();
const analytics = ref<ExperienceAnalytics | null>(null); const loading = ref(false);
const displayCandidates = computed(() => {
  if (!analytics.value) return [];
  const observed = new Set(analytics.value.moduleViews.map((item) => item.moduleId));
  const unobserved = Object.keys(props.moduleLabels || {}).filter((id) => !observed.has(id)).map((moduleId) => ({ moduleId, count: 0 }));
  return [...unobserved, ...analytics.value.lowFrequencyModules].sort((left, right) => left.count - right.count || left.moduleId.localeCompare(right.moduleId)).slice(0, 8);
});
const sampleProgress = computed(() => analytics.value
  ? `当前 ${analytics.value.totalEvents}/${analytics.value.minimumEvents} 个真人事件，覆盖 ${analytics.value.activeDays}/${analytics.value.minimumActiveDays} 个活跃日；自动化事件不会进入菜单合并判断。`
  : "");
async function load() { loading.value = true; try { analytics.value = await adminWorkspaceApi.experienceAnalytics(30); } finally { loading.value = false; } }
function moduleTitle(id: string) { return props.moduleLabels?.[id] || id; }
onMounted(load);
</script>

<style scoped>
.panel-head { display: flex; align-items: center; justify-content: space-between; }.insight-metrics { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 12px; margin-bottom: 14px; }.insight-metrics article { padding: 14px; border: 1px solid var(--admin-border); border-radius: 9px; }.insight-metrics span,.insight-metrics small { display: block; color: var(--admin-muted); }.insight-metrics strong { display: block; margin: 5px 0; font-size: 22px; }.low-frequency { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 8px; margin-top: 14px; }.low-frequency > strong { grid-column: 1 / -1; }.low-frequency span { display: grid; gap: 3px; padding: 10px; border-radius: 8px; background: var(--admin-panel); }.low-frequency small { color: var(--admin-muted); }
@media (max-width: 760px) { .insight-metrics,.low-frequency { grid-template-columns: 1fr; }.low-frequency > strong { grid-column: 1; } }
</style>
