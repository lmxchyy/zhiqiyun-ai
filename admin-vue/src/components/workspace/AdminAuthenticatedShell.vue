<template>
  <el-container :class="shellClass">
    <div v-if="mobileDrawerOpen" class="mobile-drawer-mask" @click="emit('update:mobileDrawerOpen', false)"></div>
    <AdminWorkspaceSidebar
      :brand-home-title="brandHomeTitle"
      :is-user-console="isUserConsole"
      :is-agent-console="isAgentConsole"
      :is-guest-user="isGuestUser"
      :desktop-sidebar-collapsed="desktopSidebarCollapsed"
      :visible-module-groups="visibleModuleGroups"
      :active-sidebar-module-id="activeSidebarModuleId"
      :user-flat-menu-items="userFlatMenuItems"
      :active-user-menu-id="activeUserMenuId"
      :sidebar-plan="sidebarPlan"
      :icon-for="iconFor"
      :is-group-active="isGroupActive"
      :on-brand-home="goBrandHome"
      :on-select-admin-module="selectAdminModule"
      :on-select-user-flat-menu="selectUserFlatMenu"
      :on-open-workspace-login="openWorkspaceLogin"
    />
    <el-container class="admin-workspace">
      <AdminWorkspaceHeader
        :title="headerTitle"
        :subtitle="activeHeaderModuleTitle"
        :active-group-icon="activeGroupIcon"
        :active-header-module-title="activeHeaderModuleTitle"
        :active-group-label="activeGroupLabel"
        :is-user-console="isUserConsole"
        :is-agent-console="isAgentConsole"
        :is-guest-user="isGuestUser"
        :api-error="apiError"
        :loading="loading"
        :search-keyword="searchKeyword"
        :global-search-keyword="globalSearchKeyword"
        :element-size-label="elementSizeLabel"
        :element-size-options="elementSizeOptions"
        :current-admin-name="currentAdminName"
        :current-admin-email="currentAdminEmail"
        :grid-icon="gridIcon"
        :search-icon="searchIcon"
        :refresh-icon="refreshIcon"
        :arrow-down-icon="arrowDownIcon"
        :user-filled-icon="userFilledIcon"
        :lock-icon="lockIcon"
        :switch-button-icon="switchButtonIcon"
        @open-mobile-drawer="emit('update:mobileDrawerOpen', true)"
        @toggle-desktop-sidebar="emit('toggle-desktop-sidebar')"
        @open-workspace-login="emit('open-workspace-login')"
        @open-command-palette="emit('open-command-palette')"
        @reload="emit('reload')"
        @set-element-size="emit('set-element-size', $event)"
        @account-command="emit('account-command', $event)"
        @update:searchKeyword="emit('update:searchKeyword', $event)"
        @update:globalSearchKeyword="emit('update:globalSearchKeyword', $event)"
      />
      <AdminWorkspaceTabs
        :is-user-console="isUserConsole"
        :active-module-id="activeModuleId"
        :open-tabs="openTabs"
        @select="emit('select-module', $event)"
        @close="emit('close-tab', $event)"
        @refresh="emit('reload')"
        @command="emit('tabs-command', $event)"
      />
      <el-main class="admin-main">
        <slot />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import AdminWorkspaceSidebar from "./AdminWorkspaceSidebar.vue";
import AdminWorkspaceHeader from "./AdminWorkspaceHeader.vue";
import AdminWorkspaceTabs from "./AdminWorkspaceTabs.vue";

interface ModuleGroup {
  id: string;
  title: string;
  icon: unknown;
  items: Array<{ id: string; title: string }>;
}

interface SidebarPlan {
  name: string;
  status: string;
  expiresAt: string;
  percent: number;
  availableText: string;
  totalText: string;
}

interface Props {
  shellClass: unknown;
  mobileDrawerOpen: boolean;
  brandHomeTitle: string;
  isUserConsole: boolean;
  isAgentConsole: boolean;
  isGuestUser: boolean;
  desktopSidebarCollapsed: boolean;
  visibleModuleGroups: ModuleGroup[];
  activeSidebarModuleId: string;
  userFlatMenuItems: Array<{ id: string; title: string; icon: unknown }>;
  activeUserMenuId: string;
  sidebarPlan: SidebarPlan;
  iconFor: (key: string) => unknown;
  isGroupActive: (group: ModuleGroup) => boolean;
  goBrandHome: () => void;
  selectAdminModule: (moduleId: string) => void;
  selectUserFlatMenu: (menuId: string) => void;
  openWorkspaceLogin: () => void;
  headerTitle: string;
  activeHeaderModuleTitle: string;
  activeGroupIcon: unknown;
  activeGroupLabel: string;
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
  activeModuleId: string;
  openTabs: Array<{ id: string; title: string }>;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  (event: 'update:mobileDrawerOpen', value: boolean): void;
  (event: 'toggle-desktop-sidebar'): void;
  (event: 'open-workspace-login'): void;
  (event: 'open-command-palette'): void;
  (event: 'reload'): void;
  (event: 'set-element-size', value: string): void;
  (event: 'account-command', value: string): void;
  (event: 'update:searchKeyword', value: string): void;
  (event: 'update:globalSearchKeyword', value: string): void;
  (event: 'select-module', value: string): void;
  (event: 'close-tab', value: string): void;
  (event: 'tabs-command', value: string): void;
}>();

void props;
</script>
