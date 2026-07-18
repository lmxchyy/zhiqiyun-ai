<template>
  <AdminDataTable :title="moduleTitle" :persistence-key="persistenceKey" :rows="rows" :loading="saving" :toolbar-actions="toolbarActions" :columns="columns" :column-labels="columnLabels" :row-actions="rowActions" :batch-actions="rowActions" :search-keyword="searchKeyword" :status-filter="statusFilter" :status-filter-options="normalizedStatusOptions" :is-status-column="isStatusColumn" :status-type="statusType" :status-label="statusLabel" :format-cell="formatCell" :visible-row-actions="visibleRowActions" :label-for-row-action="labelForRowAction" @update:search-keyword="$emit('update:searchKeyword', $event)" @update:status-filter="$emit('update:statusFilter', $event)" @run-action="(action, row) => $emit('runAction', action, row)" @batch-action="(action, rows) => $emit('batchAction', action, rows)" />
</template>

<script setup lang="ts">
import { computed } from "vue";
import AdminDataTable from "../admin/AdminDataTable.vue";

type RecordValue = Record<string, any>;
interface Action { action: string; label: string }

const props = defineProps<{
  moduleTitle: string; persistenceKey: string; rows: RecordValue[]; saving: boolean; toolbarActions: Action[]; columns: string[]; columnLabels: Record<string, string>; rowActions: Action[];
  searchKeyword: string; statusFilter: string; statusFilterOptions: Array<string | { label: string; value: string }>;
  isStatusColumn: (column: string) => boolean; statusType: (value: unknown) => any; statusLabel: (value: unknown) => string; formatCell: (value: unknown, column: string) => unknown;
  visibleRowActions: (row: RecordValue) => Action[]; labelForRowAction: (action: Action, row: RecordValue) => string;
}>();

const normalizedStatusOptions = computed(() => props.statusFilterOptions.map((item) => typeof item === "string" ? { label: item, value: item } : item));

defineEmits<{
  "update:searchKeyword": [value: string];
  "update:statusFilter": [value: string];
  runAction: [action: string, row?: RecordValue];
  batchAction: [action: string, rows: RecordValue[]];
}>();
</script>
