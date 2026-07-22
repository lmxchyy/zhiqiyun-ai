<template><MiniProgramRoleWorkbench initial-role="user" initial-tab="create" initial-creation-mode="video" :initial-inspiration-template-id="templateId" /></template>
<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";
const templateId = ref("");
onLoad(options => { templateId.value = String(options?.templateId || "").trim(); });
</script>
<style>page { background: #f7f8fc; }</style>
