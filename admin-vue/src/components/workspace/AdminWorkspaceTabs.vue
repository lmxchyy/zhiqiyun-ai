<template>
  <nav v-if="!isUserConsole" class="admin-page-tabs" aria-label="已打开页面标签">
    <button class="tabs-rail-button" type="button" aria-label="向左滚动标签" @click="scrollOpenTabs(-1)">«</button>
    <div ref="tabsScrollRef" class="tabs-scroll">
      <button v-for="tab in openTabs" :key="tab.id" :class="['page-tab', { 'is-active': tab.id === activeModuleId }]" type="button" @click="$emit('select', tab.id)">
        <span>{{ tab.title }}</span>
        <i v-if="openTabs.length > 1" role="button" aria-label="关闭标签" @click.stop="$emit('close', tab.id)">×</i>
      </button>
    </div>
    <button class="tabs-rail-button" type="button" aria-label="向右滚动标签" @click="scrollOpenTabs(1)">»</button>
    <button class="tabs-tool-button" type="button" aria-label="刷新当前页" @click="$emit('refresh')"><el-icon><Refresh /></el-icon></button>
    <el-dropdown trigger="click" @command="handleCommand">
      <button class="tabs-tool-button" type="button" aria-label="标签页更多操作"><el-icon><Setting /></el-icon></button>
      <template #dropdown>
        <el-dropdown-menu>
          <el-dropdown-item command="refresh"><el-icon><Refresh /></el-icon><span>刷新当前</span></el-dropdown-item>
          <el-dropdown-item command="closeOthers"><span>关闭其它</span></el-dropdown-item>
          <el-dropdown-item command="closeLeft"><span>关闭左侧</span></el-dropdown-item>
          <el-dropdown-item command="closeRight"><span>关闭右侧</span></el-dropdown-item>
          <el-dropdown-item command="closeAll" divided><span>关闭全部</span></el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </nav>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { Refresh, Setting } from "@element-plus/icons-vue";

withDefaults(defineProps<{
  isUserConsole: boolean;
  activeModuleId: string;
  openTabs: Array<{ id: string; title: string }>;
}>(), {});

const emit = defineEmits<{
  select: [moduleId: string];
  close: [moduleId: string];
  refresh: [];
  command: [value: string];
}>();

const tabsScrollRef = ref<HTMLElement | null>(null);
function scrollOpenTabs(direction: -1 | 1) {
  tabsScrollRef.value?.scrollBy({ left: direction * 260, behavior: "smooth" });
}
function handleCommand(command: string | number | object) {
  emit("command", String(command));
}
</script>
