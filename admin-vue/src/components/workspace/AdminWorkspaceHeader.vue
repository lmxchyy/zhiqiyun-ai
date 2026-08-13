<template>
  <div>
    <div class="mobile-admin-bar">
      <el-button class="mobile-collapse-button" :icon="gridIcon" aria-label="打开模块导航" @click="emit('openMobileDrawer')" />
      <div class="mobile-admin-title">
        <strong>{{ title }}</strong>
        <small>{{ subtitle }}</small>
      </div>
      <div class="mobile-admin-actions">
        <el-tag :type="apiError ? 'danger' : 'success'" effect="light">{{ apiError ? 'API ERROR' : 'API ONLINE' }}</el-tag>
        <el-button v-if="isGuestUser" class="mobile-account-button" :icon="userFilledIcon" circle aria-label="登录" @click="emit('openWorkspaceLogin')" />
        <el-dropdown v-else trigger="click" @command="emitAccountCommand">
          <el-button class="mobile-account-button" :icon="userFilledIcon" circle aria-label="账号操作" />
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="profile"><el-icon><component :is="userFilledIcon" /></el-icon><span>账号信息</span></el-dropdown-item>
              <el-dropdown-item command="password"><el-icon><component :is="lockIcon" /></el-icon><span>修改密码</span></el-dropdown-item>
              <el-dropdown-item class="logout-dropdown-item" command="logout" divided><el-icon><component :is="switchButtonIcon" /></el-icon><span>退出登录</span></el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>

    <el-header class="admin-header">
      <el-button class="admin-collapse-button" :icon="gridIcon" circle @click="emit('toggleDesktopSidebar')" />
      <div class="header-title">
        <div class="header-path">
          <el-icon><component :is="activeGroupIcon" /></el-icon>
          <span v-if="isUserConsole" class="header-single-title">{{ activeHeaderModuleTitle }}</span>
          <el-breadcrumb v-else separator="/">
            <el-breadcrumb-item>{{ activeGroupLabel }}</el-breadcrumb-item>
            <el-breadcrumb-item>{{ activeHeaderModuleTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
      </div>
      <div class="header-actions">
        <el-input
          v-if="!isUserConsole && !isAgentConsole"
          :model-value="searchKeyword"
          class="header-search"
          :prefix-icon="searchIcon"
          clearable
          placeholder="搜索当前模块"
          @update:model-value="emitSearchKeyword"
        />
        <el-input
          v-else
          :model-value="globalSearchKeyword"
          class="header-search"
          :prefix-icon="searchIcon"
          clearable
          placeholder="全局搜索菜单与业务记录（Ctrl K）"
          @focus="emit('openCommandPalette')"
          @keydown.enter="emit('openCommandPalette')"
          @update:model-value="emitGlobalSearchKeyword"
        />
        <el-button :icon="refreshIcon" circle :loading="loading" @click="emit('reload')" />
        <el-dropdown trigger="click" @command="emitSetElementSize">
          <el-button class="size-button">
            <span>{{ elementSizeLabel }}</span>
            <el-icon><component :is="arrowDownIcon" /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="item in elementSizeOptions" :key="item.value" :command="item.value">{{ item.label }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-tag :type="apiError ? 'danger' : 'success'" effect="light">{{ apiError ? 'API ERROR' : 'API ONLINE' }}</el-tag>
        <el-button v-if="isGuestUser" class="account-button" @click="emit('openWorkspaceLogin')">
          <el-icon><component :is="userFilledIcon" /></el-icon><span class="account-button-copy"><strong>游客</strong><small>点击登录</small></span>
        </el-button>
        <el-dropdown v-else trigger="click" @command="emitAccountCommand">
          <el-button class="account-button">
            <el-icon><component :is="userFilledIcon" /></el-icon>
            <span class="account-button-copy">
              <strong>{{ currentAdminName }}</strong>
              <small v-if="currentAdminEmail">{{ currentAdminEmail }}</small>
            </span>
            <el-icon><component :is="arrowDownIcon" /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="profile"><el-icon><component :is="userFilledIcon" /></el-icon><span>账号信息</span></el-dropdown-item>
              <el-dropdown-item command="password"><el-icon><component :is="lockIcon" /></el-icon><span>修改密码</span></el-dropdown-item>
              <el-dropdown-item class="logout-dropdown-item" command="logout" divided><el-icon><component :is="switchButtonIcon" /></el-icon><span>退出登录</span></el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </el-header>
  </div>
</template>

<script setup lang="ts">
interface Props {
  title: string;
  subtitle: string;
  activeGroupIcon: unknown;
  activeHeaderModuleTitle: string;
  activeGroupLabel: string;
  isUserConsole: boolean;
  isAgentConsole: boolean;
  isGuestUser: boolean;
  apiError: boolean;
  loading: boolean;
  searchKeyword: string;
  globalSearchKeyword: string;
  elementSizeLabel: string;
  elementSizeOptions: Array<{ label: string; value: string }>;
  currentAdminName: string;
  currentAdminEmail: string;
  gridIcon: unknown;
  searchIcon: unknown;
  refreshIcon: unknown;
  arrowDownIcon: unknown;
  userFilledIcon: unknown;
  lockIcon: unknown;
  switchButtonIcon: unknown;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (event: 'update:searchKeyword', value: string): void;
  (event: 'update:globalSearchKeyword', value: string): void;
  (event: 'openMobileDrawer'): void;
  (event: 'toggleDesktopSidebar'): void;
  (event: 'openWorkspaceLogin'): void;
  (event: 'openCommandPalette'): void;
  (event: 'reload'): void;
  (event: 'setElementSize', value: string): void;
  (event: 'accountCommand', value: string): void;
}>();

function emitSearchKeyword(value: string) {
  emit('update:searchKeyword', value);
}

function emitGlobalSearchKeyword(value: string) {
  emit('update:globalSearchKeyword', value);
}

function emitSetElementSize(value: string) {
  emit('setElementSize', value);
}

function emitAccountCommand(value: string | number | Record<string, unknown>) {
  emit('accountCommand', String(value));
}

void props;
</script>
