<template><MiniProgramRoleWorkbench initial-role="user" initial-tab="home" /></template>
<script setup lang="ts">
import { onShow } from "@dcloudio/uni-app";
import MiniProgramRoleWorkbench from "../../components/MiniProgramRoleWorkbench.vue";
import { syncCustomTabBar } from "../../utils/customTabBar";

onShow(() => syncCustomTabBar(0));
</script>
<style>page { background: #f7f8fc; }</style>
