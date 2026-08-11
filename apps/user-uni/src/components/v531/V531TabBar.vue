<template>
  <view :class="['v531-tabbar', { 'works-variant': role === 'user' && active === 'assets' }]">
    <button
      v-for="item in items"
      :key="item.id"
      :class="['v531-tab', { active: active === item.id }]"
      :style="{ width: `${100 / items.length}%` }"
      @click="selectTab(item.id)"
    >
      <image
        v-if="item.icon"
        class="v531-tab-icon"
        :src="active === item.id ? item.activeIcon : item.icon"
        mode="aspectFit"
      />
      <text v-else class="v531-tab-glyph">{{ item.glyph }}</text>
      <text class="v531-tab-label">{{ item.label }}</text>
    </button>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { rolePage } from "../../config/miniProgramPages";
import type { MiniProgramRoleId, MiniProgramTabId } from "../../config/miniProgramPages";

interface NavigationItem {
  id: MiniProgramTabId;
  label: string;
  icon?: string;
  activeIcon?: string;
  glyph?: string;
}

const props = withDefaults(defineProps<{ role?: MiniProgramRoleId; active: MiniProgramTabId }>(), {
  role: "user",
});
const emit = defineEmits<{ change: [tab: MiniProgramTabId] }>();
const navigating = ref(false);
const root = "/static/icons";
const navigationItems: Record<MiniProgramRoleId, NavigationItem[]> = {
  user: [
    { id: "home", label: "首页", icon: `${root}/home.svg`, activeIcon: `${root}/home-active.svg` },
    { id: "create", label: "创作", icon: `${root}/create.svg`, activeIcon: `${root}/create-active.svg` },
    { id: "assets", label: "作品", icon: `${root}/assets.svg`, activeIcon: `${root}/assets-active.svg` },
    { id: "mine", label: "我的", icon: `${root}/profile.svg`, activeIcon: `${root}/profile-active.svg` },
  ],
  agent: [
    { id: "overview", label: "概览", icon: `${root}/home.svg`, activeIcon: `${root}/home-active.svg` },
    { id: "promotion", label: "推广", glyph: "推" },
    { id: "customers", label: "客户", glyph: "客" },
    { id: "commission", label: "分润", glyph: "润" },
    { id: "mine", label: "我的", icon: `${root}/profile.svg`, activeIcon: `${root}/profile-active.svg` },
  ],
  operation: [
    { id: "overview", label: "概览", icon: `${root}/home.svg`, activeIcon: `${root}/home-active.svg` },
    { id: "agents", label: "代理", glyph: "代" },
    { id: "orders", label: "订单", glyph: "单" },
    { id: "commission", label: "分润", glyph: "润" },
    { id: "mine", label: "我的", icon: `${root}/profile.svg`, activeIcon: `${root}/profile-active.svg` },
  ],
};
const items = computed(() => navigationItems[props.role]);

function selectTab(tab: MiniProgramTabId) {
  if (tab === props.active || navigating.value) return;
  // #ifdef MP-WEIXIN
  // User native tabs must switchTab. Agent/operation share one workbench instance —
  // emit to parent so overview/customers/commission can switch in-place without reLaunch.
  if (props.role === "user") {
    navigating.value = true;
    uni.switchTab({
      url: rolePage(props.role, tab),
      fail: () => uni.showToast({ title: "页面切换失败，请重试", icon: "none" }),
      complete: () => { navigating.value = false; },
    });
    return;
  }
  emit("change", tab);
  // #endif
  // #ifndef MP-WEIXIN
  emit("change", tab);
  // #endif
}
</script>

<style scoped>
.v531-tabbar {
  position: fixed;
  z-index: 70;
  right: 15px;
  bottom: calc(12px + env(safe-area-inset-bottom));
  left: 15px;
  display: flex;
  height: 69px;
  box-sizing: border-box;
  padding: 8px 10px 15px;
  border: 1px solid #e7eaf0;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 10px 28px rgba(15, 23, 42, 0.1);
}
.v531-tab {
  display: flex;
  width: 100%;
  height: 46px;
  margin: 0;
  padding: 4px 0 3px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  border: 0;
  border-radius: 14px;
  color: #7b8498;
  background: transparent;
  font-size: 10px;
  line-height: 14px;
}
.v531-tab::after {
  display: none;
}
.v531-tab.active {
  color: #4a6cff;
  background: #f0edff;
}
.v531-tabbar.works-variant {
  right: 16px;
  bottom: calc(12px + env(safe-area-inset-bottom));
  left: 16px;
  height: 68px;
  padding: 6px;
  border-color: #e3e5f0;
  border-radius: 18px;
  box-shadow: none;
}
.v531-tabbar.works-variant .v531-tab {
  height: 56px;
  padding: 4px;
  color: #6e758c;
  border-radius: 12px;
}
.v531-tabbar.works-variant .v531-tab.active {
  color: #7d8cf5;
  background: transparent;
}
.v531-tab-icon {
  width: 21px;
  height: 21px;
  pointer-events: none;
}
.v531-tab-glyph {
  display: flex;
  width: 21px;
  height: 21px;
  align-items: center;
  justify-content: center;
  border-radius: 7px;
  color: currentColor;
  background: #f3f5f9;
  font-size: 11px;
  font-weight: 600;
  line-height: 21px;
  pointer-events: none;
}
.v531-tab.active .v531-tab-glyph {
  background: #ffffff;
}
.v531-tab-label {
  font-size: 10px;
  font-weight: 500;
  line-height: 14px;
  pointer-events: none;
}
.v531-tab.active .v531-tab-label {
  font-weight: 700;
}
</style>
