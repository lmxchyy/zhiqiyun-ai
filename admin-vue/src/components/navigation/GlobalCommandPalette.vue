<template>
  <el-dialog :model-value="open" width="min(920px, 92vw)" class="command-palette" :show-close="false" append-to-body @close="$emit('update:open', false)">
    <template #header>
      <div class="command-search">
        <el-icon><Search /></el-icon>
        <input ref="searchInput" :value="query" type="search" placeholder="搜索菜单、客户、订单或当前页面记录…" aria-label="全局搜索" @input="$emit('update:query', ($event.target as HTMLInputElement).value)" @keydown.esc="$emit('update:open', false)" />
        <kbd>ESC</kbd>
      </div>
    </template>
    <div class="command-results">
      <section>
        <header><span>模块入口</span><small>{{ moduleResults.length }}</small></header>
        <button v-for="item in moduleResults" :key="item.id" type="button" @click="$emit('select-module', item.id)"><strong>{{ item.title }}</strong><small>{{ item.groupTitle }} / {{ item.sectionTitle }}</small><kbd>跳转</kbd></button>
        <el-empty v-if="query && !moduleResults.length" description="没有匹配的模块" :image-size="48" />
      </section>
      <section>
        <header><span>当前页面记录</span><small>{{ recordResults.length }}</small></header>
        <button v-for="item in recordResults" :key="item.key" type="button" @click="$emit('select-record', item)"><strong>{{ item.title }}</strong><small>{{ item.desc }}</small><kbd>打开</kbd></button>
        <el-empty v-if="query && !recordResults.length" description="当前页面没有匹配记录" :image-size="48" />
      </section>
      <section>
        <header><span>全局业务数据</span><small>{{ businessResults.length }}</small></header>
        <button v-for="item in businessResults" :key="`${item.type}-${item.recordId}`" type="button" @click="$emit('select-business', item)"><strong>{{ item.title }}</strong><small>{{ item.description }}</small><kbd>{{ resultTypeLabel(item.type) }}</kbd></button>
        <el-empty v-if="query && !businessResults.length" description="没有匹配的业务数据" :image-size="48" />
      </section>
      <div v-if="!query" class="command-hint"><span>输入名称、邮箱、订单号或模块名称</span><small>支持菜单与当前业务数据联合检索</small></div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from "vue";
import { Search } from "@element-plus/icons-vue";

const props = defineProps<{ open: boolean; query: string; moduleResults: Array<{ id: string; title: string; groupTitle?: string; sectionTitle?: string }>; recordResults: Array<{ key: string; title: string; desc: string; row: Record<string, any> }>; businessResults: Array<{ type: "customer" | "order" | "enterprise" | "generation_task" | "payment" | "invoice"; recordId: string; title: string; description: string; module: string }> }>();
defineEmits<{ "update:open": [value: boolean]; "update:query": [value: string]; "select-module": [moduleId: string]; "select-record": [item: any]; "select-business": [item: any] }>();
const searchInput = ref<HTMLInputElement>();
watch(() => props.open, (open) => { if (open) void nextTick(() => searchInput.value?.focus()); });
function resultTypeLabel(type: string) { return ({ customer: "客户", order: "订单", enterprise: "企业", generation_task: "生成", payment: "支付", invoice: "发票" } as Record<string, string>)[type] || "业务"; }
</script>

<style scoped>
.command-search { display: grid; grid-template-columns: 24px 1fr auto; align-items: center; gap: 10px; padding: 4px 0 12px; border-bottom: 1px solid var(--admin-border); }.command-search input { min-width: 0; border: 0; outline: 0; background: transparent; color: var(--admin-text); font: inherit; font-size: 17px; }.command-search kbd,.command-results kbd { padding: 2px 6px; border: 1px solid var(--admin-border); border-radius: 5px; background: var(--admin-panel); color: var(--admin-muted); font-size: 10px; }.command-results { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; min-height: 260px; }.command-results section { display: grid; align-content: start; gap: 7px; min-width: 0; }.command-results header { display: flex; justify-content: space-between; padding: 2px 4px 6px; color: var(--admin-muted); font-size: 12px; }.command-results button { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 3px 8px; padding: 10px; border: 1px solid transparent; border-radius: 8px; background: transparent; color: var(--admin-text); cursor: pointer; text-align: left; }.command-results button:hover,.command-results button:focus-visible { border-color: var(--color-border-active); background: var(--color-primary-light); outline: none; }.command-results button strong { grid-column: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.command-results button small { grid-column: 1; overflow: hidden; color: var(--admin-muted); text-overflow: ellipsis; white-space: nowrap; }.command-results button kbd { grid-column: 2; grid-row: 1 / span 2; align-self: center; }.command-hint { grid-column: 1 / -1; display: grid; place-content: center; gap: 5px; min-height: 220px; color: var(--admin-muted); text-align: center; }.command-hint span { color: var(--admin-text); font-weight: 600; }
@media (max-width: 860px) { .command-results { grid-template-columns: 1fr 1fr; } }
@media (max-width: 620px) { .command-results { grid-template-columns: 1fr; }.command-hint { grid-column: 1; } }
</style>
