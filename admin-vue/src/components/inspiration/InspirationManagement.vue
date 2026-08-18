<template>
  <section class="inspiration-admin">
    <header class="page-head">
      <div><h2>创作灵感管理</h2><p>运营案例、提示词、模型参数和发布审核统一管理。</p></div>
      <el-button type="primary" :icon="Plus" @click="openCreate">新增模板</el-button>
    </header>
    <div class="metric-row">
      <article v-for="item in metricItems" :key="item.label"><span>{{ item.label }}</span><strong>{{ item.value }}</strong></article>
    </div>
    <div class="toolbar">
      <el-input v-model="filters.q" clearable placeholder="搜索标题或描述" :prefix-icon="Search" @keyup.enter="load" />
      <el-select v-model="filters.contentType" clearable placeholder="内容类型"><el-option label="图片" value="image"/><el-option label="视频" value="video"/><el-option label="PPT" value="ppt"/></el-select>
      <el-select v-model="filters.category" clearable placeholder="分类"><el-option v-for="item in categories" :key="item.id" :label="item.name" :value="item.id"/></el-select>
      <el-select v-model="filters.status" clearable placeholder="发布状态"><el-option label="草稿" value="DRAFT"/><el-option label="已发布" value="PUBLISHED"/><el-option label="已撤回" value="WITHDRAWN"/></el-select>
      <el-button :icon="Search" @click="load">查询</el-button><el-button :icon="Refresh" @click="reset">重置</el-button>
    </div>
    <div v-if="selected.length" class="batch-bar"><span>已选 {{ selected.length }} 项</span><el-button @click="batch('approve')">审核通过</el-button><el-button type="primary" @click="batch('publish')">发布</el-button><el-button @click="batch('withdraw')">撤回</el-button></div>
    <el-table v-loading="loading" :data="items" row-key="id" @selection-change="selected=$event">
      <el-table-column type="selection" width="48"/><el-table-column label="案例" min-width="260"><template #default="scope"><div class="template-cell"><el-image :src="scope.row.thumbnailUrl || scope.row.coverUrl" fit="cover"><template #error><div class="image-error">无封面</div></template></el-image><div><strong>{{ scope.row.title }}</strong><span>AI生成示例 · {{ typeLabel(scope.row.contentType) }}</span><small>{{ scope.row.categoryName || '-' }}</small></div></div></template></el-table-column>
      <el-table-column prop="slug" label="Slug" min-width="160"/><el-table-column label="运营标记" width="150"><template #default="scope"><el-tag v-if="scope.row.featured" size="small">推荐</el-tag><el-tag v-if="scope.row.hot" size="small" type="danger">热门</el-tag><el-tag v-if="scope.row.pinned" size="small" type="warning">置顶</el-tag></template></el-table-column>
      <el-table-column label="状态" width="150"><template #default="scope"><el-tag :type="scope.row.status==='PUBLISHED'?'success':'info'">{{ scope.row.status }}</el-tag><small class="audit">{{ scope.row.auditStatus }}</small></template></el-table-column>
      <el-table-column label="数据" width="190"><template #default="scope"><span class="stats">浏览 {{ scope.row.viewCount }} · 同款 {{ scope.row.useCount }} · 生成 {{ scope.row.generateCount }}</span></template></el-table-column>
      <el-table-column label="操作" width="250" fixed="right"><template #default="scope"><el-button link type="primary" @click="openEdit(scope.row)">编辑</el-button><el-dropdown @command="runAction(scope.row,$event)"><el-button link>更多<el-icon><ArrowDown/></el-icon></el-button><template #dropdown><el-dropdown-menu><el-dropdown-item command="copy">复制</el-dropdown-item><el-dropdown-item command="approve">审核通过</el-dropdown-item><el-dropdown-item command="reject">审核驳回</el-dropdown-item><el-dropdown-item command="publish">发布</el-dropdown-item><el-dropdown-item command="withdraw">撤回</el-dropdown-item><el-dropdown-item command="preview">预览素材</el-dropdown-item><el-dropdown-item command="versions">历史版本</el-dropdown-item><el-dropdown-item divided command="delete">删除</el-dropdown-item></el-dropdown-menu></template></el-dropdown></template></el-table-column>
    </el-table>
    <el-pagination v-model:current-page="page" v-model:page-size="pageSize" layout="total, prev, pager, next" :total="total" @current-change="load"/>

    <el-drawer v-model="editorOpen" size="720px" :title="editingId ? '编辑创作灵感' : '新增创作灵感'" destroy-on-close>
      <el-form label-position="top" class="editor-form">
        <div class="form-grid"><el-form-item label="标题" required><el-input v-model="form.title" maxlength="160" show-word-limit/></el-form-item><el-form-item label="内容类型" required><el-segmented v-model="form.contentType" :options="contentTypeOptions"/></el-form-item></div>
        <div class="form-grid"><el-form-item label="Slug" required><el-input v-model="form.slug" maxlength="160" placeholder="小写字母、数字或连字符"/></el-form-item><el-form-item label="分类" required><el-select v-model="form.categoryId"><el-option v-for="item in categories" :key="item.id" :label="item.name" :value="item.id"/></el-select></el-form-item></div>
        <el-form-item label="简介"><el-input v-model="form.description" type="textarea" :rows="2" maxlength="300" show-word-limit/></el-form-item>
        <el-divider content-position="left">Template Definition</el-divider>
        <el-form-item label="Definition JSON" required><el-input v-model="definitionText" type="textarea" :rows="16" placeholder="schemaVersion / inputs / prompt / bindings / presets / handoff / capability"/></el-form-item>
        <div class="form-grid"><el-form-item label="封面地址" required><el-input v-model="form.coverUrl" placeholder="可填写素材中心或 Storage 地址"><template #append><el-upload :show-file-list="false" :http-request="uploadCover"><el-button :loading="uploading">上传</el-button></el-upload></template></el-input></el-form-item><el-form-item label="生成结果地址"><el-input v-model="form.resultUrl"><template #append><el-upload :show-file-list="false" :http-request="uploadResult"><el-button :loading="uploading">上传</el-button></el-upload></template></el-input></el-form-item></div>
        <div class="form-grid"><el-form-item label="内容标签"><el-select v-model="form.tags" multiple filterable allow-create default-first-option placeholder="输入后回车添加标签"/></el-form-item><el-form-item label="适用租户"><el-select v-model="form.applicableTenantIds" multiple filterable allow-create default-first-option placeholder="留空表示全部租户"/></el-form-item></div>
        <div class="form-grid"><el-form-item label="展示平台"><el-checkbox-group v-model="form.platforms"><el-checkbox value="miniprogram">小程序</el-checkbox><el-checkbox value="h5">H5</el-checkbox><el-checkbox value="app">App</el-checkbox><el-checkbox value="pc">PC</el-checkbox></el-checkbox-group></el-form-item><el-form-item label="运营标记"><el-checkbox v-model="form.featured">推荐</el-checkbox><el-checkbox v-model="form.hot">热门</el-checkbox><el-checkbox v-model="form.pinned">置顶</el-checkbox></el-form-item></div>
        <div class="form-grid"><el-form-item label="排序"><el-input-number v-model="form.sort" :min="0" :max="9999"/></el-form-item><el-form-item label="素材发布授权"><el-switch v-model="form.sourceAuthorized"/><span class="switch-note">用户作品转模板时必须开启并留存授权</span></el-form-item></div>
        <div class="form-grid"><el-form-item label="定时上线"><el-date-picker v-model="form.startTime" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" clearable/></el-form-item><el-form-item label="定时下线"><el-date-picker v-model="form.endTime" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" clearable/></el-form-item></div>
      </el-form>
      <template #footer><el-button @click="editorOpen=false">取消</el-button><el-button type="primary" :loading="saving" @click="save">保存草稿</el-button></template>
    </el-drawer>
    <el-drawer v-model="versionsOpen" title="历史版本" size="520px"><el-timeline><el-timeline-item v-for="item in versions" :key="item.id" :timestamp="item.createdAt"><strong>版本 {{ item.version }}</strong><p>{{ item.changeNote || '内容更新' }} · {{ item.createdBy || '-' }}</p><el-button link type="primary" @click="rollback(item.version)">回滚到此版本</el-button></el-timeline-item></el-timeline><el-empty v-if="!versions.length" description="暂无历史版本"/></el-drawer>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ArrowDown, Plus, Refresh, Search } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { UploadRequestOptions } from "element-plus";
import { inspirationAdminAPI, type InspirationCategory, type InspirationTemplate, type InspirationTemplateDefinition } from "../../api/inspirations";
import { uploadMediaAssets } from "../../api/media";

const loading=ref(false),saving=ref(false),uploading=ref(false),editorOpen=ref(false),versionsOpen=ref(false),editingId=ref("");
const items=ref<InspirationTemplate[]>([]),categories=ref<InspirationCategory[]>([]),selected=ref<InspirationTemplate[]>([]),versions=ref<Array<{id:string;version:number;changeNote:string;createdBy:string;createdAt:string}>>([]),stats=ref<Record<string,number>>({});
const page=ref(1),pageSize=ref(20),total=ref(0),filters=reactive({q:"",contentType:"",category:"",status:""});
const contentTypeOptions=[{label:"图片",value:"image"},{label:"视频",value:"video"},{label:"PPT",value:"ppt"}];
const emptyDefinition=(contentType:"image"|"video"|"ppt"):InspirationTemplateDefinition=>{
  const contracts={image:{targetType:"IMAGE_CREATION",targetKey:"image.create",capabilityKey:"image_generation",modelHint:"gpt-image-2"},video:{targetType:"VIDEO_CREATION",targetKey:"video.create",capabilityKey:"video_generation",modelHint:""},ppt:{targetType:"PPT_CREATION",targetKey:"ppt.create",capabilityKey:"ppt_generation",modelHint:"kimi-k2.6"}} as const;
  const contract=contracts[contentType];
  return {schemaVersion:1,inputs:[],prompt:{template:"",negativeTemplate:"",composer:{key:"deterministic-template",version:1}},bindings:[],presets:{inputDefaults:{},generationDefaults:contentType==="image"?{ratio:"1:1",quality:"high",count:1}:{},materials:[]},presentation:{},handoff:{targetType:contract.targetType,targetKey:contract.targetKey},capability:{capabilityKey:contract.capabilityKey,modelHint:contract.modelHint}};
};
const emptyForm=()=>({slug:"",title:"",description:"",contentType:"image" as const,categoryId:"",coverUrl:"",thumbnailUrl:"",resultUrl:"",definition:emptyDefinition("image"),platforms:["miniprogram"],tags:[] as string[],applicableTenantIds:[] as string[],featured:false,hot:false,pinned:false,sort:0,status:"DRAFT",auditStatus:"PENDING",sourceAssetId:"",sourceAuthorized:false,startTime:"",endTime:""});
const form=reactive(emptyForm());const definitionText=ref(JSON.stringify(emptyDefinition("image"),null,2));
const metricItems=computed(()=>[{label:"模板总数",value:stats.value.templates||0},{label:"已发布",value:stats.value.published||0},{label:"待审核",value:stats.value.pendingAudit||0},{label:"生成同款",value:stats.value.generated||0},{label:"累计浏览",value:stats.value.views||0}]);
function typeLabel(type:string){return ({image:"图片",video:"视频",ppt:"PPT"} as Record<string,string>)[type]||type}
function slugFromTitle(title:string){return title.toLowerCase().replace(/[^a-z0-9]+/g,"-").replace(/^-+|-+$/g,"").slice(0,80)}
async function load(){loading.value=true;try{const [list,summary]=await Promise.all([inspirationAdminAPI.list({...filters,page:page.value,pageSize:pageSize.value}),inspirationAdminAPI.statistics()]);items.value=list.items;total.value=list.total;stats.value=summary}catch(e){ElMessage.error((e as Error).message)}finally{loading.value=false}}
function reset(){Object.assign(filters,{q:"",contentType:"",category:"",status:""});page.value=1;void load()}
function openCreate(){editingId.value="";Object.assign(form,emptyForm());form.categoryId=categories.value[0]?.id||"";definitionText.value=JSON.stringify(emptyDefinition("image"),null,2);editorOpen.value=true}
function openEdit(item:InspirationTemplate){editingId.value=item.id;Object.assign(form,{...emptyForm(),...item,definition:item.definition||emptyDefinition(item.contentType==="video"||item.contentType==="ppt"?item.contentType:"image")});definitionText.value=JSON.stringify(form.definition||emptyDefinition("image"),null,2);editorOpen.value=true}
async function uploadAsset(options:UploadRequestOptions,target:"cover"|"result"){uploading.value=true;try{const response=await uploadMediaAssets([options.file],{sourceType:"inspiration_template"});const asset=response.item||response.items?.[0];if(!asset)throw new Error("素材接口未返回文件信息");const url=asset.cdnUrl||asset.originalUrl||asset.thumbnailUrl||"";if(target==="cover"){form.coverUrl=url;form.thumbnailUrl=asset.thumbnailUrl||url;form.sourceAssetId=asset.id;form.sourceAuthorized=false}else form.resultUrl=url;ElMessage.success("素材上传成功");options.onSuccess?.(response)}catch(error){ElMessage.error(error instanceof Error?error.message:"素材上传失败");throw error}finally{uploading.value=false}}
function uploadCover(options:UploadRequestOptions){return uploadAsset(options,"cover")}
function uploadResult(options:UploadRequestOptions){return uploadAsset(options,"result")}
async function save(){if(!form.title.trim()||!form.categoryId||!form.coverUrl.trim()){ElMessage.warning("请完整填写标题、分类和封面");return}if(!form.slug.trim())form.slug=slugFromTitle(form.title);let definition:InspirationTemplateDefinition;try{definition=JSON.parse(definitionText.value||"{}")}catch{ElMessage.error("Definition JSON 无效");return}saving.value=true;try{const payload={slug:form.slug,title:form.title,description:form.description,contentType:form.contentType,categoryId:form.categoryId,coverUrl:form.coverUrl,thumbnailUrl:form.thumbnailUrl,resultUrl:form.resultUrl,definition,platforms:form.platforms,tags:form.tags,applicableTenantIds:form.applicableTenantIds,featured:form.featured,hot:form.hot,pinned:form.pinned,sort:form.sort,sourceAssetId:form.sourceAssetId,sourceAuthorized:form.sourceAuthorized,startTime:form.startTime,endTime:form.endTime};editingId.value?await inspirationAdminAPI.update(editingId.value,payload):await inspirationAdminAPI.create(payload);ElMessage.success("模板已保存");editorOpen.value=false;await load()}catch(e){ElMessage.error((e as Error).message)}finally{saving.value=false}}
async function runAction(item:InspirationTemplate,command:string){if(command==="delete"){await ElMessageBox.confirm(`确认删除“${item.title}”？`,`删除模板`,{type:"warning"});await inspirationAdminAPI.remove(item.id)}else if(command==="versions"){versions.value=(await inspirationAdminAPI.versions(item.id)).items;editingId.value=item.id;versionsOpen.value=true;return}else if(command==="preview"){window.open(item.resultUrl||item.coverUrl,"_blank","noopener,noreferrer");return}else{const action=command==="approve"?"audit/approve":command==="reject"?"audit/reject":command;await inspirationAdminAPI.action(item.id,action as "copy"|"publish"|"withdraw"|"audit/approve"|"audit/reject")}ElMessage.success("操作成功");await load()}
async function batch(action:string){try{await inspirationAdminAPI.batch(selected.value.map(i=>i.id),action);ElMessage.success("批量操作完成");await load()}catch(e){ElMessage.error((e as Error).message)}}
async function rollback(version:number){await ElMessageBox.confirm(`确认回滚到版本 ${version}？`,`版本回滚`,{type:"warning"});await inspirationAdminAPI.rollback(editingId.value,version);versionsOpen.value=false;ElMessage.success("已回滚");await load()}
onMounted(async()=>{try{categories.value=(await inspirationAdminAPI.categories()).items}finally{await load()}});
</script>

<style scoped>
.inspiration-admin{display:grid;gap:18px}.page-head{display:flex;align-items:center;justify-content:space-between}.page-head h2{margin:0 0 6px;font-size:24px}.page-head p{margin:0;color:#697386}.metric-row{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:12px}.metric-row article{padding:18px;background:#fff;border:1px solid #e7eaf0;border-radius:8px}.metric-row span{display:block;color:#7a8498;font-size:13px}.metric-row strong{display:block;margin-top:8px;font-size:26px}.toolbar{display:grid;grid-template-columns:minmax(220px,1fr) 150px 160px 150px auto auto;gap:10px}.batch-bar{display:flex;align-items:center;gap:10px;padding:10px 14px;background:#f3f6ff;border:1px solid #dfe6ff;border-radius:6px}.template-cell{display:flex;align-items:center;gap:12px}.template-cell .el-image{width:82px;height:58px;border-radius:6px;background:#f1f3f7}.image-error{display:grid;place-items:center;height:100%;font-size:12px;color:#929bad}.template-cell strong,.template-cell span,.template-cell small{display:block}.template-cell span,.template-cell small,.stats,.audit{color:#768096;font-size:12px}.audit{display:block;margin-top:5px}.editor-form{padding-right:12px}.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}.requirement-count{display:flex;align-items:center;gap:8px}.switch-note{margin-left:10px;color:#7b8498;font-size:12px}.el-pagination{justify-content:flex-end}@media(max-width:960px){.metric-row{grid-template-columns:repeat(2,1fr)}.toolbar,.form-grid{grid-template-columns:1fr}}
</style>
