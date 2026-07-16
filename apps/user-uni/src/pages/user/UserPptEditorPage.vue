<template>
  <PptDocumentGeneration v-if="pageReady" :initial-task-id="taskId" />
</template>

<script setup lang="ts">
import { ref } from "vue";
import { onLoad } from "@dcloudio/uni-app";
import PptDocumentGeneration from "../../components/PptDocumentGeneration.vue";

const taskId = ref("");
const pageReady = ref(false);

onLoad((query) => {
  taskId.value = typeof query?.taskId === "string" ? query.taskId : "";
  pageReady.value = true;
});
</script>

<style>
page {
  min-height: 100%;
  background: #050505;
}
</style>
