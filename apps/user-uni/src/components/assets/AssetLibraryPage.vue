<template>
  <view class="library-page">
    <view class="safe-top" />
    <view class="library-header">
      <button class="back-button" aria-label="返回作品页" @click="backOrHome('/pages/user/UserAssetsPage')">‹</button>
      <view class="library-title"><text>全部作品</text><text>{{ store.pagination.total }} 项数字资产</text></view>
      <button class="header-button" @click="filterVisible=true">筛选</button>
      <button class="header-button" @click="sortVisible=true">排序</button>
      <button :class="['header-button',{active:store.multiSelectMode}]" @click="store.setMultiSelectMode(!store.multiSelectMode)">{{ store.multiSelectMode?'退出':'管理' }}</button>
    </view>

    <view class="search-row"><input v-model="searchDraft" class="asset-search-input" confirm-type="search" placeholder="搜索名称、Prompt、项目、标签、模型" @input="queueSearch" @confirm="search"/><button v-if="searchDraft" class="search-clear" aria-label="清空搜索" @click="clearSearch">×</button><button class="search-button" @click="search">搜索</button></view>
    <AssetTypeTabs :model-value="store.filters.type" @update:model-value="changeType"/>
    <view class="status-space"><AssetStatusTabs :model-value="store.filters.status" @update:model-value="changeStatus"/></view>

    <AssetSkeleton v-if="store.loading&&!store.assets.length" :count="6"/>
    <AssetErrorState v-else-if="store.error&&!store.assets.length" :title="store.noPermission?'暂无访问权限':store.offline?'网络不可用':'作品加载失败'" :description="store.error" @retry="refresh"/>
    <AssetGrid v-else-if="store.assets.length" :assets="store.assets" :selecting="store.multiSelectMode" :selected-ids="store.selectedIds" @open="openAsset" @select="item=>store.toggleSelect(item.id)" @action="activeAsset=$event" @favorite="toggleFavorite"/>
    <AssetEmptyState v-else :title="emptyTitle" :description="emptyDescription" :action-label="store.filters.status==='recycled'?'':'清除筛选'" @action="clearAllFilters"/>

    <view class="load-state"><text v-if="store.loadingMore">正在加载更多...</text><button v-else-if="store.pagination.hasMore" @click="store.loadMoreAssets()">加载更多</button><text v-else-if="store.assets.length">已加载全部 {{ store.pagination.total }} 项</text></view>
    <view v-if="store.filters.status==='recycled'" class="recycle-note"><text>回收站作品默认保留 30 天，请及时恢复需要的内容。</text></view>
    <view class="bottom-safe" />

    <AssetBatchBar v-if="store.multiSelectMode" :count="store.selectedIds.length" :all-selected="Boolean(store.assets.length)&&store.selectedIds.length===store.assets.length" @select-all="store.selectAll" @clear="store.clearSelection" @exit="store.setMultiSelectMode(false)" @action="handleBatch"/>
    <AssetFilterDrawer :visible="filterVisible" :filters="store.filters" @close="filterVisible=false" @apply="applyFilters"/>
    <AssetSortSheet :visible="sortVisible" :model-value="store.sort" @close="sortVisible=false" @update:model-value="changeSort"/>
    <AssetActionSheet :asset="activeAsset" @close="activeAsset=null" @action="handleAction"/>
  </view>
</template>

<script setup lang="ts">
import { computed,onBeforeUnmount,ref } from "vue";
import { useAssetStore } from "../../stores/assets";import type { AssetFilter,AssetItem,AssetSort,AssetStatus,AssetType,BatchAssetAction } from "../../features/assets/types";import { downloadAssetFile,shareAsset } from "../../features/assets/platform";import { miniProgramFeaturePages } from "../../config/miniProgramPages";import { backOrHome } from "../../utils/miniProgramBusiness";import { registerAssetNativeBridge } from "../../features/assets/nativeBridge";
import AssetActionSheet from "./AssetActionSheet.vue";import AssetBatchBar from "./AssetBatchBar.vue";import AssetEmptyState from "./AssetEmptyState.vue";import AssetErrorState from "./AssetErrorState.vue";import AssetFilterDrawer from "./AssetFilterDrawer.vue";import AssetGrid from "./AssetGrid.vue";import AssetSkeleton from "./AssetSkeleton.vue";import AssetSortSheet from "./AssetSortSheet.vue";import AssetStatusTabs from "./AssetStatusTabs.vue";import AssetTypeTabs from "./AssetTypeTabs.vue";
const store=useAssetStore();const searchDraft=ref(store.filters.keyword);const filterVisible=ref(false);const sortVisible=ref(false);const activeAsset=ref<AssetItem|null>(null);
const emptyTitle=computed(()=>store.filters.status==="recycled"?"回收站为空":store.isSearchResult?"没有匹配的作品":"暂无作品");const emptyDescription=computed(()=>store.filters.status==="recycled"?"删除的作品会暂存在这里。":"调整搜索或筛选条件后再试。");
let searchTimer:ReturnType<typeof setTimeout>|null=null;
function clearSearchTimer(){if(searchTimer)clearTimeout(searchTimer);searchTimer=null;}
function updateSearch(value:string){searchDraft.value=value;clearSearchTimer();searchTimer=setTimeout(search,400);}
function refresh(){return store.refreshAssets(20);}function search(){clearSearchTimer();void store.setFilters({keyword:searchDraft.value.trim()},20);}function queueSearch(event?:unknown){const value=(event as {detail?:{value?:unknown}}|undefined)?.detail?.value;updateSearch(typeof value==="string"?value:searchDraft.value);}function clearSearch(){clearSearchTimer();searchDraft.value="";void store.setFilters({keyword:""},20);}function changeType(value:AssetType){void store.setType(value,20);}function changeStatus(value:AssetStatus){void store.setStatus(value,20);}function changeSort(value:AssetSort){void store.setSort(value,20);}function applyFilters(value:AssetFilter){void store.setFilters(value,20);}function clearAllFilters(){clearSearchTimer();searchDraft.value="";void store.clearFilters(20);}
function assetById(id:string){return store.assets.find(item=>item.id===id);}
const disposeNativeBridge=registerAssetNativeBridge({
  setType:changeType,
  setStatus:changeStatus,
  emptyAction:clearAllFilters,
  updateSearch,
  submitSearch:search,
  clearSearch,
  openAsset:id=>{if(store.multiSelectMode){store.toggleSelect(id);return;}const asset=assetById(id);if(asset)openAsset(asset);},
  favoriteAsset:id=>{const asset=assetById(id);if(asset)void toggleFavorite(asset);},
  openAssetActions:id=>{const asset=assetById(id);if(asset)activeAsset.value=asset;},
});onBeforeUnmount(()=>{clearSearchTimer();disposeNativeBridge();});
function openAsset(asset:AssetItem){openDetail(asset);}function openDetail(asset:AssetItem){uni.navigateTo({url:`${miniProgramFeaturePages.userAssetDetail}?id=${encodeURIComponent(asset.id)}`});}function continueEditing(asset:AssetItem){const page=asset.type==="video"?"UserVideoCreationPage":asset.type==="ppt"?"UserPptCreationPage":asset.type==="agent"?"UserAgentCreationPage":"UserImageCreationPage";uni.navigateTo({url:`/pages/user/${page}?assetId=${encodeURIComponent(asset.id)}`});}async function toggleFavorite(asset:AssetItem){try{await store.toggleFavorite(asset.id);}catch{/* rollback handled in store */}}
function confirmDelete(asset:AssetItem){uni.showModal({title:"移到回收站",content:"作品将在回收站保留 30 天。",confirmColor:"#ff771b",success:result=>{if(result.confirm)void store.deleteAsset(asset.id);}});}function confirmPermanent(asset:AssetItem){uni.showModal({title:"彻底删除",content:"此操作不可恢复，是否继续？",confirmText:"彻底删除",confirmColor:"#e05435",success:result=>{if(result.confirm)void store.permanentlyDeleteAsset(asset.id);}});}
function rename(asset:AssetItem){uni.showModal({title:"重命名作品",editable:true,content:asset.name,placeholderText:asset.name,success:result=>{const name=String(result.content||"").trim();if(result.confirm&&name)void store.renameAsset(asset.id,name);}});}async function moveOne(asset:AssetItem){await store.loadProjects();uni.showActionSheet({itemList:["新建项目并加入","从项目移出",...store.projects.map(item=>item.name)],success:result=>{if(result.tapIndex===0){createProjectForOne(asset);return;}const project=result.tapIndex===1?null:store.projects[result.tapIndex-2];void store.moveToProject(asset.id,project?.id||"",project?.name||"");}});}function createProjectForOne(asset:AssetItem){uni.showModal({title:"新建项目",editable:true,placeholderText:"输入项目名称",success:result=>{const name=String(result.content||"").trim();if(result.confirm&&name)void store.moveToProject(asset.id,`project_${Date.now()}`,name);}});}
async function handleAction(action:string,asset:AssetItem){try{if(action==="detail")openDetail(asset);else if(action==="edit")continueEditing(asset);else if(action==="download")await downloadAssetFile(asset);else if(action==="share")shareAsset(asset);else if(action==="favorite")await store.toggleFavorite(asset.id);else if(action==="move")await moveOne(asset);else if(action==="rename")rename(asset);else if(action==="archive")await store.archiveAsset(asset.id);else if(action==="delete")confirmDelete(asset);else if(action==="restore")await store.restoreAsset(asset.id);else if(action==="permanent")confirmPermanent(asset);else if(action==="retry"&&asset.taskId)await store.retryTask(asset.taskId);}catch(error){uni.showToast({title:error instanceof Error?error.message:"操作失败",icon:"none"});}}
function handleBatch(action:BatchAssetAction){if(!store.selectedIds.length){uni.showToast({title:"请先选择作品",icon:"none"});return;}if(action==="download"){void batchDownload();return;}if(action==="move"){void batchMove();return;}if(action==="delete"){uni.showModal({title:"批量移到回收站",content:`确定删除已选择的 ${store.selectedIds.length} 项作品吗？`,confirmColor:"#ff771b",success:result=>{if(result.confirm)void store.applyBatch({action:"delete"});}});return;}void store.applyBatch({action});}
async function batchDownload(){for(const item of store.selectedAssets){try{await downloadAssetFile(item);}catch(error){uni.showToast({title:error instanceof Error?error.message:"下载失败",icon:"none"});break;}}}
async function batchMove(){await store.loadProjects();uni.showActionSheet({itemList:["新建项目并加入","从项目移出",...store.projects.map(item=>item.name)],success:result=>{if(result.tapIndex===0){uni.showModal({title:"新建项目",editable:true,placeholderText:"输入项目名称",success:modal=>{const name=String(modal.content||"").trim();if(modal.confirm&&name)void store.applyBatch({action:"move",projectId:`project_${Date.now()}`,projectName:name});}});return;}const project=result.tapIndex===1?null:store.projects[result.tapIndex-2];void store.applyBatch({action:"move",projectId:project?.id||"",projectName:project?.name||""});}});}
</script>

<style scoped>
.library-page{min-height:100vh;box-sizing:border-box;padding:0 16px calc(24px + env(safe-area-inset-bottom));color:#1a1f2e;background:#f7f8fc}.safe-top{height:calc(10px + env(safe-area-inset-top))}.library-header{display:flex;min-height:58px;align-items:center;gap:6px}.back-button,.header-button,.search-row button,.load-state button{margin:0;border:0}.back-button::after,.header-button::after,.search-row button::after,.load-state button::after{display:none}.back-button{width:36px;height:36px;padding:0;border-radius:12px;color:#5a4db2;background:#fff;font-size:28px;line-height:34px}.library-title{display:flex;min-width:0;flex:1;flex-direction:column}.library-title text:first-child{font-size:20px;font-weight:720}.library-title text:last-child{margin-top:2px;color:#8a90a1;font-size:9px}.header-button{width:auto;height:32px;padding:0 8px;border-radius:11px;color:#626a7d;background:#fff;font-size:9px}.header-button.active{color:#fff;background:#7d8df6}.search-row{display:flex;margin:8px 0 14px;padding:7px;align-items:center;gap:5px;border:1px solid #e3e6ef;border-radius:16px;background:#fff}.search-row input{min-width:0;height:34px;flex:1;padding-left:7px;font-size:11px}.search-row button{width:32px;height:32px;padding:0;border-radius:10px;color:#737a8d;background:#f3f4f7}.search-row .search-button{width:auto;padding:0 12px;color:#fff;background:#7d8df6;font-size:10px}.status-space{margin:12px 0 16px}.load-state{display:flex;min-height:72px;align-items:center;justify-content:center;color:#8b91a5;font-size:10px}.load-state button{width:auto;height:34px;padding:0 17px;border-radius:17px;color:#5a4db2;background:#fff;font-size:10px}.recycle-note{padding:12px;border-radius:13px;color:#b65a61;background:#fff0f1;font-size:10px;line-height:1.6}.bottom-safe{height:84px}
</style>
