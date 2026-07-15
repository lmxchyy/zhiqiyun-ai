<template>
  <MiniProgramRoleWorkbench
    initial-role="user"
    initial-tab="create"
    initial-creation-mode="image"
    :initial-creation-asset-id="assetId"
    :initial-creation-intent="intent"
  />
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";

const assetId = ref("");
const intent = ref<"edit" | "regenerate">("edit");

onLoad(options => {
  assetId.value = String(options?.assetId || "").trim();
  intent.value = String(options?.intent || "").trim() === "regenerate" ? "regenerate" : "edit";
});
</script>

<style>page { background: #f7f8fc; }</style>
