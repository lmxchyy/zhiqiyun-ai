<template>
  <el-alert
    class="enterprise-privacy-banner"
    type="warning"
    :closable="false"
    show-icon
    title="敏感数据访问边界"
    :description="message"
  />
</template>

<script setup lang="ts">
withDefaults(defineProps<{ message?: string }>(), {
  message: "本页只展示企业治理、计费和使用统计。知识库正文、原始文件、作品、对话、提示词及 AI 员工任务输入输出默认不可见。"
});
</script>
