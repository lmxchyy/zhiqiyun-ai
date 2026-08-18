<template>
  <el-aside width="200px" class="admin-sidebar">
    <div class="brand">
      <button class="brand-home-button" type="button" :title="brandHomeTitle" :aria-label="brandHomeTitle" @click="onBrandHome">
        <img class="brand-logo" :src="logo" alt="知启云 AI" />
        <span class="brand-copy">
          <strong>知启云 AI</strong>
          <small>{{ isUserConsole ? "User Console" : isAgentConsole ? "Agent Console" : "Master SaaS Console" }}</small>
        </span>
      </button>
    </div>
    <div class="sidebar-section-label">{{ isUserConsole ? "用户导航" : isAgentConsole ? "代理导航" : "平台导航" }}</div>
    <nav v-if="!isUserConsole" class="collapsed-icon-menu" aria-label="折叠模块导航">
      <div v-for="group in visibleModuleGroups" :key="group.id" :class="['collapsed-icon-group', { 'is-active': isGroupActive(group) }]">
        <button class="collapsed-icon-button" type="button" :aria-label="group.title" @click="onSelectAdminModule(group.items[0]?.id || activeSidebarModuleId)">
          <el-icon><component :is="group.icon" /></el-icon>
        </button>
        <div class="collapsed-flyout" role="menu">
          <strong>{{ group.title }}</strong>
          <button v-for="item in group.items" :key="item.id" :class="{ 'is-active': item.id === activeSidebarModuleId }" type="button" role="menuitem" @click.stop="onSelectAdminModule(item.id)">
            <el-icon><component :is="iconFor(item.id)" /></el-icon>
            <span>{{ item.title }}</span>
          </button>
        </div>
      </div>
    </nav>
    <el-menu v-if="isUserConsole" class="sidebar-menu user-flat-sidebar-menu" :default-active="activeUserMenuId" @select="onSelectUserFlatMenu">
      <el-menu-item v-for="item in userFlatMenuItems" :key="item.id" :index="item.id" :aria-label="item.title" :title="desktopSidebarCollapsed ? item.title : undefined">
        <el-tooltip :content="item.title" placement="right" :disabled="!desktopSidebarCollapsed" :show-after="120" :hide-after="0" popper-class="user-sidebar-tooltip">
          <span class="user-sidebar-tooltip-target">
            <el-icon><component :is="item.icon" /></el-icon>
          </span>
        </el-tooltip>
        <span class="user-sidebar-menu-title">{{ item.title }}</span>
      </el-menu-item>
    </el-menu>
    <el-menu v-else class="sidebar-menu" :default-active="activeSidebarModuleId" @select="onSelectAdminModule">
      <el-sub-menu v-for="group in visibleModuleGroups" :key="group.id" :index="group.id">
        <template #title>
          <el-icon><component :is="group.icon" /></el-icon>
          <span>{{ group.title }}</span>
        </template>
        <el-menu-item v-for="item in group.items" :key="item.id" :index="item.id">
          <el-icon><component :is="iconFor(item.id)" /></el-icon>
          <span>{{ item.title }}</span>
        </el-menu-item>
      </el-sub-menu>
    </el-menu>

    <aside v-if="isUserConsole" class="sidebar-plan-card">
      <template v-if="isGuestUser">
        <span>当前身份</span>
        <div class="sidebar-plan-title"><strong>游客</strong><em>体验中</em></div>
        <small>登录后查看会员、额度和创作记录</small>
        <button type="button" @click="onOpenWorkspaceLogin">登录后继续</button>
      </template>
      <template v-else>
        <span>当前套餐</span>
        <div class="sidebar-plan-title">
          <strong>{{ sidebarPlan.name }}</strong>
          <em>{{ sidebarPlan.status }}</em>
        </div>
        <small>有效期至：{{ sidebarPlan.expiresAt }}</small>
        <div class="sidebar-plan-progress"><i :style="{ width: sidebarPlan.percent + '%' }"></i></div>
        <span>可用点数</span>
        <strong class="sidebar-plan-points">{{ sidebarPlan.availableText }} <small>/ {{ sidebarPlan.totalText }}</small></strong>
        <button type="button" @click="onSelectAdminModule('userMembership')">去充值</button>
      </template>
    </aside>
  </el-aside>
</template>

<script setup lang="ts">
import logo from "../../assets/xianzhi-ai-logo.webp";

type ModuleGroup = {
  id: string;
  title: string;
  icon: unknown;
  items: Array<{ id: string; title: string }>;
};

type SidebarPlan = {
  name: string;
  status: string;
  expiresAt: string;
  percent: number;
  availableText: string;
  totalText: string;
};

withDefaults(defineProps<{
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
  iconFor: (moduleId: string) => unknown;
  isGroupActive: (group: ModuleGroup) => boolean;
  onBrandHome: () => void;
  onSelectAdminModule: (moduleId: string) => void;
  onSelectUserFlatMenu: (moduleId: string) => void;
  onOpenWorkspaceLogin: () => void;
}>(), {
  brandHomeTitle: "返回首页"
});
</script>
