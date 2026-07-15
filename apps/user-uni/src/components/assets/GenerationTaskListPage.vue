<template>
  <view class="task-page">
    <view class="safe-top"/><view class="task-header"><button @click="backOrHome('/pages/user/UserAssetsPage')">‹</button><view><text>生成任务</text><text>运行中、排队中和失败任务优先</text></view><text>{{ store.taskPagination.total }}</text></view>
    <AssetErrorState v-if="store.taskError&&!store.tasks.length" title="任务加载失败" :description="store.taskError" @retry="refresh"/>
    <view v-else-if="store.taskListLoading&&!store.tasks.length" class="task-loading"><text>正在同步任务状态...</text></view>
    <view v-else-if="store.tasks.length" class="task-list"><GenerationTaskItem v-for="task in store.tasks" :key="task.id" :task="task" @open="openTask" @cancel="cancel" @retry="retry" @result="result"/></view>
    <AssetEmptyState v-else title="暂无生成任务" description="新的图片、视频、PPT 和文档生成任务会显示在这里。" action-label="返回作品" @action="backOrHome('/pages/user/UserAssetsPage')"/>
    <view class="load-state"><text v-if="store.taskListLoadingMore">正在加载更多...</text><button v-else-if="store.taskPagination.hasMore" @click="store.loadMoreTasks()">加载更多</button><text v-else-if="store.tasks.length">已加载全部 {{ store.taskPagination.total }} 条任务</text></view><view class="bottom-safe"/>
  </view>
</template>
<script setup lang="ts">
import { onBeforeUnmount } from "vue";
import { useAssetStore } from "../../stores/assets";
import type { GenerationTask } from "../../features/assets/types";
import { registerAssetNativeBridge } from "../../features/assets/nativeBridge";
import { miniProgramFeaturePages } from "../../config/miniProgramPages";
import { backOrHome } from "../../utils/miniProgramBusiness";
import AssetEmptyState from "./AssetEmptyState.vue";
import AssetErrorState from "./AssetErrorState.vue";
import GenerationTaskItem from "./GenerationTaskItem.vue";

const store=useAssetStore();
function refresh(){return store.fetchTasks(true);}
function openTask(task:GenerationTask){if(task.status==="completed")result(task);else uni.showModal({title:task.name,content:task.status==="failed"?(task.failureReason||"生成失败"):task.status==="cancelled"?"任务已取消":`当前进度 ${task.progress}%`,showCancel:false});}
function result(task:GenerationTask){const id=task.resultIds[0];if(id)uni.navigateTo({url:`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(id)}`});else uni.showToast({title:"暂未生成结果",icon:"none"});}
function cancel(task:GenerationTask){uni.showModal({title:"取消任务",content:"确定停止当前生成任务吗？",confirmColor:"#ff771b",success:res=>{if(res.confirm)void store.cancelTask(task.id);}});}
async function retry(task:GenerationTask){uni.showLoading({title:"正在重新提交",mask:true});try{await store.retryTask(task.id);uni.hideLoading();uni.showToast({title:"已重新提交",icon:"success"});}catch(error){uni.hideLoading();uni.showToast({title:error instanceof Error?error.message:"重试失败",icon:"none"});}}
function taskById(id:string){return store.tasks.find(item=>item.id===id);}
const disposeNativeBridge=registerAssetNativeBridge({
  openTask:id=>{const task=taskById(id);if(task)openTask(task);},
  cancelTask:id=>{const task=taskById(id);if(task)cancel(task);},
  retryTask:id=>{const task=taskById(id);if(task)retry(task);},
  openTaskResult:id=>{const task=taskById(id);if(task)result(task);},
});
onBeforeUnmount(disposeNativeBridge);
</script>
<style scoped>.task-page{min-height:100vh;box-sizing:border-box;padding:0 16px calc(24px + env(safe-area-inset-bottom));color:#1a1f2e;background:#f7f8fc}.safe-top{height:calc(10px + env(safe-area-inset-top))}.task-header{display:flex;min-height:60px;align-items:center;gap:11px}.task-header button,.load-state button{margin:0;border:0}.task-header button::after,.load-state button::after{display:none}.task-header button{width:36px;height:36px;padding:0;border-radius:12px;color:#5a4db2;background:#fff;font-size:28px;line-height:34px}.task-header>view{display:flex;min-width:0;flex:1;flex-direction:column}.task-header>view text:first-child{font-size:20px;font-weight:720}.task-header>view text:last-child{margin-top:2px;color:#8a90a1;font-size:9px}.task-header>text{padding:5px 9px;border-radius:12px;color:#5a4db2;background:#fff;font-size:10px}.task-list{display:flex;margin-top:12px;flex-direction:column;gap:9px}.task-loading{display:flex;min-height:280px;align-items:center;justify-content:center;color:#8a90a1;font-size:11px}.load-state{display:flex;min-height:72px;align-items:center;justify-content:center;color:#8b91a5;font-size:10px}.load-state button{width:auto;height:34px;padding:0 17px;border-radius:17px;color:#5a4db2;background:#fff;font-size:10px}.bottom-safe{height:16px}</style>
