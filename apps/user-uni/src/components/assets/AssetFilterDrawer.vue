<template>
  <view v-if="visible" class="drawer-mask" @click="$emit('close')">
    <view class="drawer-panel" @click.stop>
      <view class="drawer-handle" />
      <view class="drawer-head"><text class="drawer-title">筛选资产</text><button class="drawer-close" @click="$emit('close')">×</button></view>
      <scroll-view scroll-y class="drawer-body">
        <text class="field-label">作品类型</text>
        <view class="option-grid"><button v-for="item in assetTypeOptions" :key="item.value" :class="['option-chip',{active:draft.type===item.value}]" @click="draft.type=item.value">{{ item.label }}</button></view>
        <text class="field-label">作品状态</text>
        <view class="option-grid"><button v-for="item in assetStatusOptions" :key="item.value" :class="['option-chip',{active:draft.status===item.value}]" @click="draft.status=item.value">{{ item.label }}</button></view>
        <text class="field-label">所属项目</text><input v-model="draft.projectId" class="field-input" placeholder="输入项目名称或编号" />
        <text class="field-label">模型</text><input v-model="draft.model" class="field-input" placeholder="输入模型名称" />
        <text class="field-label">标签</text><input v-model="tagDraft" class="field-input" placeholder="多个标签用逗号分隔" />
        <text class="field-label">创建时间</text>
        <view class="date-row"><input v-model="draft.createdFrom" class="field-input" placeholder="开始日期 YYYY-MM-DD"/><input v-model="draft.createdTo" class="field-input" placeholder="结束日期 YYYY-MM-DD"/></view>
      </scroll-view>
      <view class="drawer-actions"><button class="secondary" @click="reset">重置</button><button class="primary" @click="apply">应用筛选</button></view>
    </view>
  </view>
</template>
<script setup lang="ts">
import { reactive,ref,watch } from "vue";import { assetStatusOptions,assetTypeOptions,defaultAssetFilter,type AssetFilter } from "../../features/assets/types";
const props=defineProps<{visible:boolean;filters:AssetFilter}>();const emit=defineEmits<{close:[];apply:[filters:AssetFilter]}>();const draft=reactive<AssetFilter>(defaultAssetFilter());const tagDraft=ref("");
watch(()=>[props.visible,props.filters] as const,()=>{if(!props.visible)return;Object.assign(draft,defaultAssetFilter(),props.filters);tagDraft.value=props.filters.tagIds.join(",");},{deep:true});
function reset(){Object.assign(draft,defaultAssetFilter());tagDraft.value="";}function apply(){emit("apply",{...draft,tagIds:tagDraft.value.split(/[,，]/).map(item=>item.trim()).filter(Boolean)});emit("close");}
</script>
<style scoped>
.drawer-mask{position:fixed;z-index:180;inset:0;display:flex;align-items:flex-end;background:rgba(23,27,39,.38)}.drawer-panel{display:flex;width:100%;max-height:88vh;box-sizing:border-box;padding:10px 18px calc(16px + env(safe-area-inset-bottom));flex-direction:column;border-radius:24px 24px 0 0;background:#fff}.drawer-handle{width:42px;height:4px;margin:0 auto 8px;border-radius:4px;background:#d5d7de}.drawer-head{display:flex;height:48px;align-items:center}.drawer-title{flex:1;color:#252a39;font-size:18px;font-weight:700}.drawer-close{width:40px;height:40px;margin:0;padding:0;border:0;border-radius:14px;color:#777e91;background:#f4f5f8;font-size:22px}.drawer-close::after,.option-chip::after,.drawer-actions button::after{display:none}.drawer-body{max-height:62vh}.field-label{display:block;margin:15px 0 9px;color:#5d6478;font-size:11px;font-weight:650}.option-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}.option-chip{height:34px;margin:0;padding:0 5px;border:1px solid #e3e6ee;border-radius:12px;color:#6f7689;background:#fff;font-size:10px;line-height:32px}.option-chip.active{border-color:#7d8df6;color:#5a4db2;background:#f0efff}.field-input{width:100%;height:42px;box-sizing:border-box;padding:0 12px;border:1px solid #e3e6ee;border-radius:12px;color:#2b3040;background:#f9fafc;font-size:11px}.date-row{display:grid;grid-template-columns:1fr 1fr;gap:8px}.drawer-actions{display:grid;margin-top:16px;grid-template-columns:1fr 2fr;gap:10px}.drawer-actions button{height:44px;margin:0;border:0;border-radius:14px;font-size:12px}.drawer-actions .secondary{color:#62697c;background:#f1f2f6}.drawer-actions .primary{color:#fff;background:#7d8df6}
</style>
