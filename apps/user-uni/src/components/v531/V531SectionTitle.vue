<template>
  <view class="section-heading"
    ><view class="section-heading-copy"
      ><text class="section-heading-title">{{ title }}</text
      ><text class="section-heading-subtitle">{{ subtitle }}</text></view
    ><button
      v-if="action"
      class="section-heading-action"
      @click="$emit('action')"
    >
      {{ action }}
    </button></view
  >
</template>
<script setup lang="ts">
withDefaults(
  defineProps<{ title: string; subtitle?: string; action?: string }>(),
  { subtitle: "", action: "" },
);
defineEmits<{ action: [] }>();
</script>
<style scoped>
.section-heading {
  display: flex;
  margin: 22px 0 12px;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.section-heading-title,
.section-heading-subtitle {
  display: block;
}
.section-heading-title {
  color: #181c28;
  font-size: 18px;
  font-weight: 700;
  line-height: 25px;
}
.section-heading-subtitle {
  color: #697084;
  font-size: 10px;
  line-height: 16px;
}
.section-heading-action {
  width: auto;
  height: 25px;
  margin: 0;
  padding: 0 2px;
  border: 0;
  color: #4a6cff;
  background: transparent;
  font-size: 10px;
  line-height: 25px;
}
.section-heading-action::after {
  display: none;
}
</style>
