<template>
  <view class="enterprise-org-node">
    <view class="enterprise-org-row" @click="expanded = !expanded">
      <text class="enterprise-org-toggle">{{ hasChildren ? (expanded ? "▾" : "›") : "·" }}</text>
      <view class="enterprise-org-icon">{{ depth ? "部" : "企" }}</view>
      <view class="enterprise-org-main">
        <text class="enterprise-org-name">{{ item.name }}</text>
        <text class="enterprise-org-meta">{{ item.memberCount }}人{{ owner ? ` · 负责人：${owner}` : "" }}</text>
      </view>
      <button v-if="editable" class="enterprise-more-button" type="button" @click.stop="$emit('manage', item)">•••</button>
    </view>
    <view v-if="expanded && hasChildren" class="enterprise-org-children">
      <EnterpriseOrganizationNode
        v-for="child in item.children"
        :key="child.id"
        :item="child"
        :depth="depth + 1"
        :editable="editable"
        @manage="$emit('manage', $event)"
      />
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import type { EnterpriseOrganization } from "../../features/enterprise/types";

const props = withDefaults(defineProps<{ item: EnterpriseOrganization; depth?: number; editable?: boolean }>(), { depth: 0, editable: false });
defineEmits<{ manage: [item: EnterpriseOrganization] }>();

const expanded = ref(true);
const hasChildren = computed(() => Boolean(props.item.children?.length));
const owner = computed(() => {
  const value = props.item.metadata?.ownerName || props.item.metadata?.leaderName;
  return typeof value === "string" ? value : "";
});
</script>
