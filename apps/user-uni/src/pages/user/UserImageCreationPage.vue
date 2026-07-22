<template>
  <MiniProgramRoleWorkbench
    initial-role="user"
    initial-tab="create"
    initial-creation-mode="image"
    :initial-creation-asset-id="assetId"
    :initial-creation-intent="intent"
    :initial-inspiration-template-id="templateId"
  />
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";

const assetId = ref("");
const intent = ref<"edit" | "regenerate">("edit");
const templateId = ref("");

onLoad(options => {
  assetId.value = String(options?.assetId || "").trim();
  intent.value = String(options?.intent || "").trim() === "regenerate" ? "regenerate" : "edit";
  templateId.value = String(options?.templateId || "").trim();
});
</script>

<style>page { background: #f7f8fc; }</style>
