<template>
  <el-card shadow="never" class="admin-data-list">
    <template #header>
      <div class="admin-data-list__head">
        <div><strong>{{ title }}</strong><small>{{ filteredRows.length }} / {{ rows.length }} 条记录</small></div>
        <div class="admin-data-list__actions">
          <el-button v-for="action in toolbarActions" :key="action.action" size="small" type="primary" @click="$emit('run-action', action.action)">{{ action.label }}</el-button>
        </div>
      </div>
    </template>

    <div class="admin-data-list__viewbar" aria-label="列表视图工具栏">
      <el-select v-model="activeViewId" clearable placeholder="已保存视图" class="saved-view-select" @change="applySavedView">
        <el-option v-for="view in savedViews" :key="view.id" :label="view.name" :value="view.id"><span>{{ view.name }}</span><el-button link type="danger" aria-label="删除视图" @click.stop="removeSavedView(view.id)">删除</el-button></el-option>
      </el-select>
      <el-button size="small" @click="saveDialogOpen = true">保存当前视图</el-button>
      <el-popover placement="bottom-start" :width="260" trigger="click">
        <template #reference><el-button size="small">列配置 {{ visibleColumnKeys.length }}/{{ columns.length }}</el-button></template>
        <el-checkbox-group v-model="visibleColumnKeys" class="column-picker" @change="persistColumnConfig">
          <el-checkbox v-for="column in columns" :key="column" :value="column" :disabled="visibleColumnKeys.length === 1 && visibleColumnKeys.includes(column)">{{ columnLabels[column] || column }}</el-checkbox>
        </el-checkbox-group>
        <el-button link type="primary" @click="resetColumns">恢复默认列</el-button>
      </el-popover>
      <el-button size="small" @click="exportCSV">导出当前结果</el-button>
      <span class="viewbar-spacer"></span>
      <el-dropdown v-if="batchActions.length" :disabled="!selectedRows.length" @command="requestBatchAction">
        <el-button size="small" :type="selectedRows.length ? 'primary' : 'default'">批量操作（{{ selectedRows.length }}）</el-button>
        <template #dropdown><el-dropdown-menu><el-dropdown-item v-for="action in batchActions" :key="action.action" :command="action.action">{{ action.label }}</el-dropdown-item></el-dropdown-menu></template>
      </el-dropdown>
    </div>

    <div class="admin-data-list__filters" role="search">
      <el-input :model-value="searchKeyword" clearable :placeholder="searchPlaceholder" @update:model-value="$emit('update:searchKeyword', String($event || ''))" />
      <el-segmented v-if="statusFilterOptions.length > 1" :model-value="statusFilter" :options="statusFilterOptions" @update:model-value="$emit('update:statusFilter', String($event || 'ALL'))" />
      <slot name="filters" />
    </div>

    <el-table v-if="filteredRows.length" :data="filteredRows" :height="height" :row-key="rowKey" :row-class-name="rowOpenLabel ? 'is-clickable' : ''" v-loading="loading" stripe @selection-change="selectedRows = $event" @row-click="rowOpenLabel && $emit('open-row', $event)">
      <el-table-column v-if="batchActions.length" type="selection" width="48" reserve-selection />
      <el-table-column v-for="column in visibleColumnKeys" :key="column" :prop="column" :label="columnLabels[column] || column" min-width="140" show-overflow-tooltip>
        <template #default="scope">
          <el-tag v-if="isStatusColumn(column)" :type="statusType(scope.row[column])">{{ statusLabel(scope.row[column]) }}</el-tag>
          <span v-else>{{ formatCell(scope.row[column], column) }}</span>
        </template>
      </el-table-column>
      <el-table-column v-if="rowOpenLabel || rowActions.length" label="操作" fixed="right" :width="actionWidth">
        <template #default="scope">
          <el-button v-if="rowOpenLabel" link type="primary" size="small" @click.stop="$emit('open-row', scope.row)">{{ rowOpenLabel }}</el-button>
          <el-button v-for="action in visibleRowActions(scope.row)" :key="action.action" link type="primary" size="small" @click.stop="$emit('run-action', action.action, scope.row)">{{ labelForRowAction(action, scope.row) }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <el-empty v-else :description="emptyDescription" />
  </el-card>

  <el-dialog v-model="saveDialogOpen" title="保存列表视图" width="min(420px, 92vw)" append-to-body>
    <el-form label-position="top"><el-form-item label="视图名称"><el-input v-model="viewName" maxlength="30" show-word-limit placeholder="例如：待处理订单" @keyup.enter="saveCurrentView" /></el-form-item></el-form>
    <template #footer><el-button @click="saveDialogOpen = false">取消</el-button><el-button type="primary" :disabled="!viewName.trim()" @click="saveCurrentView">保存</el-button></template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { ElMessageBox } from "element-plus/es/components/message-box/index";
import { trackAdminExperience } from "../../composables/useAdminExperienceTracking";

type Row = Record<string, any>;
type Action = { action: string; label: string };
type SavedView = { id: string; name: string; keyword: string; status: string; columns: string[] };

const props = withDefaults(defineProps<{
  title: string; rows: Row[]; columns: string[]; columnLabels: Record<string, string>;
  toolbarActions?: Action[]; rowActions?: Action[]; batchActions?: Action[]; persistenceKey?: string;
  searchKeyword?: string; statusFilter?: string; statusFilterOptions?: Array<{ label: string; value: string }>;
  loading?: boolean; height?: number; actionWidth?: number; rowOpenLabel?: string; searchPlaceholder?: string; emptyDescription?: string;
  isStatusColumn: (column: string) => boolean; statusType: (value: unknown) => any; statusLabel: (value: unknown) => string;
  formatCell: (value: unknown, column: string) => unknown; visibleRowActions?: (row: Row) => Action[]; labelForRowAction?: (action: Action, row: Row) => string;
}>(), {
  toolbarActions: () => [], rowActions: () => [], batchActions: () => [], persistenceKey: "default", searchKeyword: "", statusFilter: "ALL",
  statusFilterOptions: () => [{ label: "全部", value: "ALL" }], loading: false, height: 560, actionWidth: 220, rowOpenLabel: "",
  searchPlaceholder: "按名称、邮箱、ID、状态搜索", emptyDescription: "暂无记录", visibleRowActions: () => [], labelForRowAction: (action: Action) => action.label
});

const emit = defineEmits<{ "update:searchKeyword": [value: string]; "update:statusFilter": [value: string]; "run-action": [action: string, row?: Row]; "batch-action": [action: string, rows: Row[]]; "open-row": [row: Row] }>();
const savedViews = ref<SavedView[]>([]);
const activeViewId = ref("");
const visibleColumnKeys = ref<string[]>([]);
const selectedRows = ref<Row[]>([]);
const saveDialogOpen = ref(false);
const viewName = ref("");
const rowKey = (row: Row) => String(row.id || row.orderNo || row.key || JSON.stringify(row));
const storagePrefix = computed(() => `xianzhi:admin:list:${props.persistenceKey}`);

const filteredRows = computed(() => {
  const keyword = props.searchKeyword.trim().toLowerCase();
  const status = props.statusFilter.toUpperCase();
  return props.rows.filter((row) => {
    const rowStatus = String(row.status ?? row.active ?? "").toUpperCase();
    return (status === "ALL" || rowStatus === status) && (!keyword || Object.values(row).some((value) => String(Array.isArray(value) ? value.join(" ") : value ?? "").toLowerCase().includes(keyword)));
  });
});

function loadConfiguration() {
  let storedColumns: string[] = [];
  try { storedColumns = JSON.parse(localStorage.getItem(`${storagePrefix.value}:columns`) || "[]"); } catch { storedColumns = []; }
  visibleColumnKeys.value = storedColumns.filter((column) => props.columns.includes(column));
  if (!visibleColumnKeys.value.length) visibleColumnKeys.value = [...props.columns];
  try { savedViews.value = JSON.parse(localStorage.getItem(`${storagePrefix.value}:views`) || "[]"); } catch { savedViews.value = []; }
  activeViewId.value = "";
  selectedRows.value = [];
}
watch([() => props.persistenceKey, () => props.columns.join("|")], loadConfiguration, { immediate: true });
function persistColumnConfig() { localStorage.setItem(`${storagePrefix.value}:columns`, JSON.stringify(visibleColumnKeys.value)); }
function resetColumns() { visibleColumnKeys.value = [...props.columns]; persistColumnConfig(); }
function persistViews() { localStorage.setItem(`${storagePrefix.value}:views`, JSON.stringify(savedViews.value)); }
function saveCurrentView() {
  const name = viewName.value.trim(); if (!name) return;
  const view: SavedView = { id: `view-${Date.now()}`, name, keyword: props.searchKeyword, status: props.statusFilter, columns: [...visibleColumnKeys.value] };
  savedViews.value.push(view); persistViews(); activeViewId.value = view.id; viewName.value = ""; saveDialogOpen.value = false; ElMessage.success("列表视图已保存");
}
function applySavedView(value: string) {
  const view = savedViews.value.find((item) => item.id === value); if (!view) return;
  emit("update:searchKeyword", view.keyword); emit("update:statusFilter", view.status);
  visibleColumnKeys.value = view.columns.filter((column) => props.columns.includes(column)); if (!visibleColumnKeys.value.length) visibleColumnKeys.value = [...props.columns]; persistColumnConfig();
}
function removeSavedView(id: string) { savedViews.value = savedViews.value.filter((item) => item.id !== id); if (activeViewId.value === id) activeViewId.value = ""; persistViews(); }
async function requestBatchAction(action: string) {
  const label = props.batchActions.find((item) => item.action === action)?.label || action;
  await ElMessageBox.confirm(`确认对选中的 ${selectedRows.value.length} 条记录执行“${label}”？`, "批量操作确认", { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" });
  emit("batch-action", action, [...selectedRows.value]);
  trackAdminExperience("BATCH_ACTION", props.persistenceKey, action, { count: selectedRows.value.length });
}
function csvCell(value: unknown) { return `"${String(value ?? "").replaceAll('"', '""')}"`; }
function exportCSV() {
  const headers = visibleColumnKeys.value.map((column) => props.columnLabels[column] || column);
  const lines = [headers.map(csvCell).join(","), ...filteredRows.value.map((row) => visibleColumnKeys.value.map((column) => csvCell(props.formatCell(row[column], column))).join(","))];
  const blob = new Blob(["\uFEFF", lines.join("\r\n")], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob); const link = document.createElement("a"); link.href = url; link.download = `${props.title}-${new Date().toISOString().slice(0, 10)}.csv`; link.click(); URL.revokeObjectURL(url); ElMessage.success(`已导出 ${filteredRows.value.length} 条记录`);
  trackAdminExperience("LIST_EXPORT", props.persistenceKey, "", { count: filteredRows.value.length, columns: visibleColumnKeys.value });
}
</script>

<style scoped>
.admin-data-list { border-radius: 12px; }.admin-data-list__head,.admin-data-list__filters,.admin-data-list__viewbar { display: flex; align-items: center; gap: 12px; }.admin-data-list__head { justify-content: space-between; }.admin-data-list__head > div:first-child { display: grid; gap: 3px; }.admin-data-list__head strong { color: var(--admin-text); font-size: 15px; }.admin-data-list__head small { color: var(--admin-muted); }.admin-data-list__actions,.admin-data-list__filters,.admin-data-list__viewbar { flex-wrap: wrap; }.admin-data-list__viewbar { padding-bottom: 12px; border-bottom: 1px solid var(--admin-border); }.admin-data-list__filters { margin: 12px 0 14px; }.admin-data-list__filters :deep(.el-input) { width: min(360px, 100%); }.saved-view-select { width: 180px; }.saved-view-select :deep(.el-select-dropdown__item) { display: flex; justify-content: space-between; }.viewbar-spacer { flex: 1; }.column-picker { display: grid; gap: 4px; margin-bottom: 10px; }:deep(.el-table__row.is-clickable) { cursor: pointer; }
@media (max-width: 700px) { .admin-data-list__head { align-items: flex-start; flex-direction: column; }.viewbar-spacer { display: none; }.admin-data-list__viewbar > * { flex: 1 1 140px; }.saved-view-select { width: 100%; } }
</style>
