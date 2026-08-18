<template><MiniProgramRoleWorkbench initial-role="user" initial-tab="assets" /></template>
<script setup lang="ts">
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";
import { useUserAssetsPage } from "../../composables/useUserAssetsPage";

useUserAssetsPage(2);
</script>
<style>page { background: #f7f8fc; }</style>
