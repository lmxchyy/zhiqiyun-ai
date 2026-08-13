<template>
  <view v-if="visible && roles.length > 1" class="role-switcher">
    <button
      v-for="role in roles"
      :key="role.id"
      type="button"
      :class="['role-pill', { active: activeRole === role.id }]"
      @click="emit('change', role.id)"
    >
      <text>{{ role.label }}</text>
    </button>
  </view>
</template>

<script setup lang="ts">
import type { MiniProgramRoleId } from "../../../config/miniProgramPages";

export interface WorkbenchRoleOption {
  id: MiniProgramRoleId;
  label: string;
}

interface RoleSwitcherProps {
  roles: WorkbenchRoleOption[];
  activeRole: MiniProgramRoleId;
  visible?: boolean;
}

const props = withDefaults(defineProps<RoleSwitcherProps>(), {
  visible: true,
});

const emit = defineEmits<{
  (e: 'change', roleId: MiniProgramRoleId): void;
}>();
</script>

<style scoped>
.role-switcher {
  display: flex;
  gap: 4px;
  margin-top: 8px;
  padding: 4px;
  border: 1px solid #e3e7f4;
  border-radius: 14px;
  background: #eef1f8;
  box-shadow: 0 6px 18px rgba(31, 41, 55, 0.05);
}

.role-switcher .role-pill {
  min-width: 0;
  height: 44px;
  margin: 0;
  padding: 0 12px;
  border: 0;
  border-radius: 10px;
  color: #697386;
  background: transparent;
  font-size: 13px;
  font-weight: 600;
  line-height: 44px;
}

.role-switcher .role-pill::after {
  border: 0;
}

.role-switcher .role-pill.active {
  color: #ffffff;
  background: linear-gradient(135deg, #7d8df6 0%, #6f68d9 100%);
  box-shadow: 0 5px 14px rgba(90, 77, 178, 0.22);
}
</style>
