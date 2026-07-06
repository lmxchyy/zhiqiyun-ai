<template>
  <section class="ppt-example-prompts">
    <button
      v-for="example in examples"
      :key="example"
      type="button"
      :class="{ active: selected === example }"
      :aria-pressed="selected === example"
      :title="`使用示例主题：${example}`"
      :aria-label="`使用示例主题：${example}`"
      @click="$emit('select', example)"
    >
      {{ example }}
    </button>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  selected?: string;
}>();

defineEmits<{
  select: [value: string];
}>();

const examples = [
  "AI赋能企业营销增长方案",
  "灵活用工平台介绍",
  "企业数字员工解决方案",
  "GEO品牌曝光方案",
  "短视频矩阵运营方案",
  "糖尿病饮食运动患教"
];
</script>

<style scoped>
.ppt-example-prompts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.ppt-example-prompts button {
  min-height: 42px;
  padding: 10px 12px;
  border: 1px solid #282828;
  border-radius: 8px;
  color: #f4f4f5;
  background: #0d0d0d;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.16s ease, transform 0.16s ease, background-color 0.16s ease;
}

.ppt-example-prompts button:hover,
.ppt-example-prompts button.active {
  border-color: #666;
  background: #171717;
  transform: translateY(-1px);
}

@media (max-width: 720px) {
  .ppt-example-prompts {
    grid-template-columns: 1fr;
  }
}
</style>
