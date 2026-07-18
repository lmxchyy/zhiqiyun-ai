<template>
  <view class="asset-center-page">
    <template v-if="isGuest">
      <view class="asset-center-header">
        <text class="page-title">{{ guestCopy.title }}</text>
      </view>
      <view class="guest-work-tabs">
        <text class="guest-work-tab active">{{ guestCopy.official }}</text>
        <button class="guest-work-tab locked" @click="emit('login')">{{ guestCopy.mine }}</button>
      </view>
      <view class="guest-case-grid">
        <view v-for="item in guestCases" :key="item.title" class="guest-case-card">
          <image :src="item.image" mode="aspectFill" />
          <text class="guest-case-title">{{ item.title }}</text>
          <text class="guest-case-description">{{ item.description }}</text>
        </view>
      </view>
      <view class="guest-login-card">
        <text class="guest-login-title">{{ guestCopy.loginTitle }}</text>
        <text class="guest-login-description">{{ guestCopy.loginDescription }}</text>
        <button @click="emit('login')">{{ guestCopy.loginAction }}</button>
      </view>
    </template>
    <template v-else>
    <view class="asset-center-header">
      <text class="page-title">作品</text>
      <view class="header-actions">
        <button class="icon-action" aria-label="批量管理作品" @click="openManage"><image src="/static/icons/edit-square.svg" mode="aspectFit"/></button>
        <button class="icon-action" aria-label="更多作品操作" @click="openHeaderMore"><image src="/static/icons/more-vertical.svg" mode="aspectFit"/></button>
      </view>
    </view>

    <view class="asset-toolbar">
      <view class="search-card">
        <button class="search-submit" aria-label="搜索作品" @click="applySearch"><image class="search-icon" src="/static/icons/search.svg" mode="aspectFit"/></button>
        <input v-model="searchDraft" class="asset-search-input" :focus="searchFocused" confirm-type="search" placeholder="搜索作品、提示词、项目..." @blur="searchFocused=false" @input="queueSearch" @confirm="applySearch" />
        <button v-if="searchDraft" class="search-clear" aria-label="清空搜索" @click="clearSearch">×</button>
      </view>
      <button class="toolbar-action" @click="openFilter"><image src="/static/icons/funnel.svg" mode="aspectFit"/><text>筛选</text></button>
      <button class="toolbar-action" @click="openSort"><image src="/static/icons/sort.svg" mode="aspectFit"/><text>排序</text></button>
    </view>

    <AssetErrorState v-if="store.overviewError" compact title="资产概览加载失败" :description="store.overviewError" @retry="store.fetchOverview" />
    <AssetOverviewCard v-else :overview="store.overview" :loading="store.overviewLoading" />

    <AssetTypeTabs class="asset-type-nav" :model-value="store.filters.type" @update:model-value="changeType" />
    <AssetStatusTabs class="asset-status-nav" :model-value="store.filters.status" @update:model-value="changeStatus" />

    <view class="section-head asset-section"><text>最近作品</text><button @click="openAllAssets">查看全部 ›</button></view>
    <AssetSkeleton v-if="store.loading&&!store.assets.length" class="asset-result-block" :count="4" />
    <AssetErrorState v-else-if="store.error&&!store.assets.length" class="asset-result-block" :title="store.noPermission?'暂无访问权限':store.offline?'网络不可用':'作品加载失败'" :description="store.error" @retry="refresh" />
    <AssetGrid v-else-if="store.assets.length" class="asset-result-block" :assets="store.assets.slice(0,4)" @open="openAsset" @action="activeAsset=$event" @favorite="toggleFavorite" />
    <AssetEmptyState v-else class="asset-result-block" :title="store.isSearchResult?'没有匹配的作品':'暂无数字资产'" :description="store.isSearchResult?'换个关键词或清除筛选条件后再试。':'完成一次 AI 创作后，成果会自动保存到这里。'" :action-label="store.isSearchResult?'清除筛选':'开始创作'" @action="handleEmptyAction" />

    <view class="section-head task-section"><text>最近任务</text><button @click="openAllTasks">查看全部 ›</button></view>
    <view v-if="store.recentTasks.length" class="task-list">
      <GenerationTaskItem v-for="task in store.recentTasks.slice(0,5)" :key="task.id" :task="task" @open="openTask" @cancel="confirmCancelTask" @retry="retryTask" @result="openTaskResult" />
    </view>
    <view v-else-if="store.tasksLoading" class="task-state"><text>正在同步生成任务...</text></view>
    <AssetErrorState v-else-if="store.taskError" compact title="任务加载失败" :description="store.taskError" @retry="store.fetchRecentTasks" />
    <view v-else class="task-state"><text>暂无生成任务</text></view>

    <view v-if="store.refreshing" class="refresh-indicator"><text>正在刷新资产中心...</text></view>
    <view class="bottom-safe" />

    <AssetFilterDrawer :visible="filterVisible" :filters="store.filters" @close="filterVisible=false" @apply="applyFilters" />
    <AssetSortSheet :visible="sortVisible" :model-value="store.sort" @close="sortVisible=false" @update:model-value="changeSort" />
    <AssetActionSheet :asset="activeAsset" @close="activeAsset=null" @action="handleAssetAction" />
    </template>
  </view>
</template>

<script setup lang="ts">
import { onBeforeUnmount,ref } from "vue";
import { useAssetStore } from "../../stores/assets";
import type { AssetFilter,AssetItem,AssetSort,AssetStatus,AssetType,GenerationTask } from "../../features/assets/types";
import { downloadAssetFile,shareAsset } from "../../features/assets/platform";
import { miniProgramFeaturePages,miniProgramRolePages } from "../../config/miniProgramPages";
import { registerAssetNativeBridge } from "../../features/assets/nativeBridge";
import AssetActionSheet from "./AssetActionSheet.vue";
import AssetEmptyState from "./AssetEmptyState.vue";
import AssetErrorState from "./AssetErrorState.vue";
import AssetFilterDrawer from "./AssetFilterDrawer.vue";
import AssetGrid from "./AssetGrid.vue";
import AssetOverviewCard from "./AssetOverviewCard.vue";
import AssetSkeleton from "./AssetSkeleton.vue";
import AssetSortSheet from "./AssetSortSheet.vue";
import AssetStatusTabs from "./AssetStatusTabs.vue";
import AssetTypeTabs from "./AssetTypeTabs.vue";
import GenerationTaskItem from "./GenerationTaskItem.vue";

defineProps<{isGuest?:boolean}>();
const emit=defineEmits<{create:[];login:[]}>();
const guestCopy={title:"\u4f5c\u54c1\u4e2d\u5fc3",official:"\u5b98\u65b9\u7cbe\u9009",mine:"\u6211\u7684\u4f5c\u54c1 \ud83d\udd12",loginTitle:"\u767b\u5f55\u540e\u67e5\u770b\u6211\u7684\u4f5c\u54c1",loginDescription:"\u540c\u6b65\u521b\u4f5c\u8bb0\u5f55\u3001\u4fdd\u5b58\u5386\u53f2\u4f5c\u54c1\u5e76\u67e5\u770b\u751f\u6210\u8fdb\u5ea6\u3002",loginAction:"\u767b\u5f55\u540e\u7ee7\u7eed"};
const guestCases=[
  {title:"\u54c1\u724c\u5ba3\u4f20\u6d77\u62a5",description:"AI \u5546\u4e1a\u89c6\u89c9\u6848\u4f8b",image:"/static/fallbacks/inspiration-poster.jpg"},
  {title:"\u77ed\u89c6\u9891\u521b\u610f",description:"\u4ea7\u54c1\u5c55\u793a\u4e0e\u8fd0\u8425\u7d20\u6750",image:"/static/fallbacks/inspiration-video.jpg"},
  {title:"\u5546\u4e1a PPT",description:"\u62db\u5546\u4e0e\u65b9\u6848\u6c47\u62a5\u6848\u4f8b",image:"/static/fallbacks/inspiration-ppt.jpg"},
  {title:"\u7535\u5546\u89c6\u89c9",description:"\u5546\u54c1\u4e3b\u56fe\u4e0e\u6d3b\u52a8\u7d20\u6750",image:"/static/fallbacks/inspiration-ecommerce.jpg"},
];
const store=useAssetStore();
const searchFocused=ref(false);const searchDraft=ref(store.filters.keyword);const filterVisible=ref(false);const sortVisible=ref(false);const activeAsset=ref<AssetItem|null>(null);
let searchTimer:ReturnType<typeof setTimeout>|null=null;
function refresh(){return store.refreshAssets(4);}function changeType(value:AssetType){void store.setType(value,4);}function changeStatus(value:AssetStatus){void store.setStatus(value,4);}function changeSort(value:AssetSort){void store.setSort(value,4);}function applyFilters(value:AssetFilter){void store.setFilters(value,4);}
function clearSearchTimer(){if(searchTimer)clearTimeout(searchTimer);searchTimer=null;}
function applySearch(){clearSearchTimer();const keyword=searchDraft.value.trim();if(!keyword&&!store.filters.keyword){searchFocused.value=true;return;}searchFocused.value=false;void store.setFilters({keyword},4);}
function updateSearch(value:string){searchDraft.value=value;clearSearchTimer();searchTimer=setTimeout(applySearch,400);}
function queueSearch(event?:unknown){const value=(event as {detail?:{value?:unknown}}|undefined)?.detail?.value;updateSearch(typeof value==="string"?value:searchDraft.value);}
function clearSearch(){clearSearchTimer();searchDraft.value="";searchFocused.value=true;void store.setFilters({keyword:""},4);}function handleEmptyAction(){if(store.isSearchResult){void store.clearFilters(4);searchDraft.value="";}else emit("create");}
function openAllAssets(){uni.navigateTo({url:miniProgramFeaturePages.userAssetsList});}function openManage(){uni.navigateTo({url:`${miniProgramFeaturePages.userAssetsList}?manage=1`});}function openAllTasks(){uni.navigateTo({url:miniProgramFeaturePages.userTasksList});}
  function toggleSearch(){searchFocused.value=true;}function openFilter(){filterVisible.value=true;}function openSort(){sortVisible.value=true;}function openHeaderMore(){uni.showActionSheet({itemList:["批量管理","查看全部作品","查看全部任务"],success:result=>{if(result.tapIndex===0)openManage();else if(result.tapIndex===1)openAllAssets();else if(result.tapIndex===2)openAllTasks();}});}function handleNativeEmptyAction(){if(store.isSearchResult){handleEmptyAction();return;}const url=miniProgramRolePages.user.create as string;uni.switchTab({url,fail:()=>uni.reLaunch({url})});}
function recentTaskById(id:string){return store.recentTasks.find(item=>item.id===id);}
const disposeNativeBridge=registerAssetNativeBridge({
  setType:changeType,
  setStatus:changeStatus,
  emptyAction:handleNativeEmptyAction,
  toggleSearch,
  updateSearch,
  submitSearch:applySearch,
  clearSearch,
  openFilter,
  openSort,
  openManage,
  openAllAssets,
  openAllTasks,
  openTask:id=>{const task=recentTaskById(id);if(task)openTask(task);},
  cancelTask:id=>confirmCancelTask(recentTaskById(id)),
  retryTask:id=>{const task=recentTaskById(id);if(task)retryTask(task);},
  openTaskResult:id=>{const task=recentTaskById(id);if(task)openTaskResult(task);},
  openAsset:id=>{const asset=store.assets.find(item=>item.id===id);if(asset)openAsset(asset);},
  favoriteAsset:id=>{const asset=store.assets.find(item=>item.id===id);if(asset)void toggleFavorite(asset);},
  openAssetActions:id=>{const asset=store.assets.find(item=>item.id===id);if(asset)activeAsset.value=asset;},
});onBeforeUnmount(()=>{clearSearchTimer();disposeNativeBridge();});
function openAsset(asset:AssetItem){if(asset.status==="generating"||asset.status==="queued"){const task=store.recentTasks.find(item=>item.id===asset.taskId);if(task)openTask(task);return;}openDetail(asset);}
function openDetail(asset:AssetItem){uni.navigateTo({url:`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(asset.id)}`});}
async function toggleFavorite(asset:AssetItem){try{await store.toggleFavorite(asset.id);}catch{/* store rolls back */}}
function continueEditing(asset:AssetItem){const page=asset.type==="video"?"UserVideoCreationPage":asset.type==="ppt"?"UserPptCreationPage":"UserImageCreationPage";uni.navigateTo({url:`/pages/user/${page}?assetId=${encodeURIComponent(asset.id)}`});}
function confirmDelete(asset:AssetItem){uni.showModal({title:"移到回收站",content:"删除后作品将进入回收站，可在保留期内恢复。",confirmColor:"#ff771b",success:result=>{if(result.confirm)void store.deleteAsset(asset.id).then(()=>uni.showToast({title:"已移到回收站",icon:"success"}));}});}
function confirmPermanent(asset:AssetItem){uni.showModal({title:"彻底删除",content:"此操作不可恢复，确定永久删除该作品吗？",confirmText:"彻底删除",confirmColor:"#e05435",success:result=>{if(result.confirm)void store.permanentlyDeleteAsset(asset.id).then(()=>uni.showToast({title:"已彻底删除",icon:"success"}));}});}
function rename(asset:AssetItem){uni.showModal({title:"重命名作品",editable:true,placeholderText:asset.name,content:asset.name,success:result=>{const name=String(result.content||"").trim();if(result.confirm&&name)void store.renameAsset(asset.id,name);}});}
async function move(asset:AssetItem){await store.loadProjects();uni.showActionSheet({itemList:["新建项目并加入","从项目移出",...store.projects.map(item=>item.name)],success:result=>{if(result.tapIndex===0){createProjectForAsset(asset);return;}const project=result.tapIndex===1?null:store.projects[result.tapIndex-2];void store.moveToProject(asset.id,project?.id||"",project?.name||"");}});}
function createProjectForAsset(asset:AssetItem){uni.showModal({title:"新建项目",editable:true,placeholderText:"输入项目名称",success:result=>{const name=String(result.content||"").trim();if(result.confirm&&name)void store.moveToProject(asset.id,`project_${Date.now()}`,name);}});}
async function handleAssetAction(action:string,asset:AssetItem){try{if(action==="detail")openDetail(asset);else if(action==="edit")continueEditing(asset);else if(action==="download")await downloadAssetFile(asset);else if(action==="share")shareAsset(asset);else if(action==="favorite")await store.toggleFavorite(asset.id);else if(action==="move")await move(asset);else if(action==="rename")rename(asset);else if(action==="archive")await store.archiveAsset(asset.id);else if(action==="delete")confirmDelete(asset);else if(action==="restore")await store.restoreAsset(asset.id);else if(action==="permanent")confirmPermanent(asset);else if(action==="retry"&&asset.taskId)await store.retryTask(asset.taskId);else if(action==="cancel"&&asset.taskId)confirmCancelTask(store.recentTasks.find(item=>item.id===asset.taskId));}catch(error){uni.showToast({title:error instanceof Error?error.message:"操作失败",icon:"none"});}}
function openTask(task:GenerationTask){if(task.status==="completed")openTaskResult(task);else uni.showModal({title:task.name,content:task.status==="failed"?(task.failureReason||"生成失败"):task.status==="cancelled"?"任务已取消":`当前进度 ${task.progress}%`,showCancel:false});}
function openTaskResult(task:GenerationTask){const id=task.resultIds[0];if(id)uni.navigateTo({url:`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(id)}`});else uni.showToast({title:"任务暂未生成结果",icon:"none"});}
async function retryTask(task:GenerationTask){uni.showLoading({title:"正在重新提交",mask:true});try{await store.retryTask(task.id);uni.hideLoading();uni.showToast({title:"已重新提交",icon:"success"});}catch(error){uni.hideLoading();uni.showToast({title:error instanceof Error?error.message:"重试失败",icon:"none"});}}
function confirmCancelTask(task?:GenerationTask){if(!task)return;uni.showModal({title:"取消生成任务",content:"取消后将停止生成，已预扣点数按后端规则退回。",confirmColor:"#ff771b",success:result=>{if(result.confirm)void store.cancelTask(task.id);}});}

</script>

<style scoped>
.asset-center-page{min-height:100vh;box-sizing:border-box;padding:34px 18px calc(104px + env(safe-area-inset-bottom));overflow-x:hidden;color:#171b2d;background:#f7f8fc}.asset-center-header{display:flex;height:48px;align-items:center;justify-content:space-between}.page-title{display:block;font-size:27px;font-weight:780;line-height:34px;letter-spacing:-.8px}.header-actions{display:flex;align-items:center;gap:15px}.icon-action,.toolbar-action,.search-card button,.section-head button{margin:0;padding:0;border:0}.icon-action::after,.toolbar-action::after,.search-card button::after,.section-head button::after{display:none}.icon-action{display:flex;width:28px;height:32px;align-items:center;justify-content:center;background:transparent}.icon-action image{width:23px;height:23px}.asset-toolbar{display:flex;height:46px;margin-top:5px;align-items:center;gap:13px}.search-card{display:flex;min-width:0;height:39px;box-sizing:border-box;padding:0 11px;flex:1;align-items:center;gap:8px;border:1px solid #e8eaf2;border-radius:20px;background:#f1f3f8}.search-icon{width:17px;height:17px}.search-card input{min-width:0;height:38px;flex:1;color:#242a3a;font-size:12px}.search-card button{display:flex;width:22px;height:28px;align-items:center;justify-content:center;color:#8b91a2;background:transparent;font-size:18px;line-height:28px}.search-card .search-submit{flex:0 0 22px}.search-card .search-clear{flex:0 0 22px}.toolbar-action{display:flex;width:auto;height:39px;align-items:center;gap:5px;color:#202537;background:transparent;font-size:12px;font-weight:600;line-height:39px;white-space:nowrap}.toolbar-action image{width:19px;height:19px}.asset-type-nav{display:block;margin-top:17px}.asset-status-nav{display:block;margin-top:13px}.asset-result-block{margin-top:13px}.section-head{display:flex;margin:20px 0 11px;align-items:center;justify-content:space-between}.section-head>text:first-child{font-size:16px;font-weight:700}.section-head button{width:auto;height:30px;color:#4a6cff;background:transparent;font-size:10px}.asset-section{margin-bottom:0}.task-section{margin-top:24px}.task-list{display:flex;flex-direction:column;gap:9px}.task-state{display:flex;min-height:86px;align-items:center;justify-content:center;border:1px solid #e7e9f1;border-radius:16px;color:#8c92a3;background:#fff;font-size:10px}.refresh-indicator{position:fixed;z-index:100;top:calc(10px + env(safe-area-inset-top));left:50%;padding:7px 12px;transform:translateX(-50%);border-radius:14px;color:#4a6cff;background:#fff;box-shadow:0 6px 18px rgba(30,36,58,.12);font-size:9px}.bottom-safe{height:22px}
.guest-work-tabs{display:flex;margin:12px 0 18px;gap:10px}.guest-work-tab{margin:0;padding:9px 14px;border:0;border-radius:18px;background:#eef1f7;color:#737b90;font-size:13px}.guest-work-tab::after{display:none}.guest-work-tab.active{color:#fff;background:#4a6cff}.guest-case-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.guest-case-card{overflow:hidden;padding-bottom:12px;border-radius:16px;background:#fff;box-shadow:0 5px 18px rgba(40,50,85,.07)}.guest-case-card image{display:block;width:100%;height:132px}.guest-case-title,.guest-case-description{display:block;padding:0 11px}.guest-case-title{margin-top:10px;font-size:14px;font-weight:700}.guest-case-description{margin-top:4px;color:#8a91a3;font-size:10px}.guest-login-card{display:flex;margin-top:20px;padding:20px;align-items:center;flex-direction:column;border-radius:18px;text-align:center;background:#fff}.guest-login-title{font-size:16px;font-weight:700}.guest-login-description{margin-top:7px;color:#858c9e;font-size:11px;line-height:1.6}.guest-login-card button{margin-top:15px;padding:0 22px;border:0;border-radius:20px;color:#fff;background:#4a6cff;font-size:13px}.guest-login-card button::after{display:none}
@media(max-width:350px){.asset-center-page{padding-right:14px;padding-left:14px}.asset-toolbar{gap:8px}.toolbar-action{gap:3px;font-size:11px}.header-actions{gap:10px}}
</style>
