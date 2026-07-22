<template>
  <el-dialog v-model="visible" width="min(900px, 94vw)" :close-on-click-modal="false" destroy-on-close @closed="reset">
    <template #header><div class="dialog-title"><div><span>受控身份降级</span><strong>{{ userName || userId }}</strong></div><el-tag type="danger">超级管理员 · 高风险</el-tag></div></template>
    <el-steps :active="step" finish-status="success" align-center class="steps"><el-step title="处理方案" /><el-step title="降级预检查" /><el-step title="二次确认" /></el-steps>
    <el-alert v-if="errorMessage" type="error" :title="errorMessage" show-icon :closable="false" />

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <section v-show="step === 0" class="section">
        <el-alert type="warning" :closable="false" show-icon title="代理商或运营中心不会被直接改成普通用户；本流程终止商业身份，基础账号、会员权益和 Token 保持不变。" />
        <el-form-item v-if="currentIdentity === 'OPERATION_CENTER'" label="降级结果">
          <el-radio-group v-model="form.targetIdentity"><el-radio value="AGENT">降级为代理商</el-radio><el-radio value="">终止运营中心商业身份</el-radio></el-radio-group>
        </el-form-item>
        <el-form-item v-else label="降级结果"><el-text>终止代理商商业身份；基础 USER 身份继续保留，但不是直接修改为普通用户。</el-text></el-form-item>
        <el-form-item label="下级处理方式" prop="childStrategy">
          <el-radio-group v-model="form.childStrategy" class="strategy-grid">
            <el-radio border value="TRANSFER_TO_AGENT"><strong>迁移给其他代理商</strong><small>批量结束旧关系并创建新关系</small></el-radio>
            <el-radio border value="DIRECT_OPERATION_CENTER"><strong>转运营中心直属</strong><small>解除代理层级，保留运营中心归属</small></el-radio>
            <el-radio border value="PRESERVE_HISTORY"><strong>结束当前归属，不重新分配</strong><small>保留历史，但下级将失去后续归属</small></el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="form.childStrategy === 'TRANSFER_TO_AGENT'" label="承接代理商" prop="targetAgentId"><el-select v-model="form.targetAgentId" filterable placeholder="选择具备有效身份的新上级"><el-option v-for="item in activeAgents" :key="item.id" :value="item.id" :label="optionLabel(item)" :disabled="item.userId === userId" /></el-select></el-form-item>
        <el-form-item v-if="form.childStrategy === 'DIRECT_OPERATION_CENTER'" label="承接运营中心" prop="targetOperationCenterId"><el-select v-model="form.targetOperationCenterId" filterable placeholder="选择有效运营中心"><el-option v-for="item in activeCenters" :key="item.id" :value="item.id" :label="optionLabel(item)" :disabled="item.userId === userId" /></el-select></el-form-item>
        <div class="form-grid">
          <el-form-item label="生效方式"><el-switch v-model="form.scheduled" active-text="指定时间生效" inactive-text="审核后立即生效" /></el-form-item>
          <el-form-item v-if="form.scheduled" label="指定生效时间" prop="effectiveAt"><el-date-picker v-model="form.effectiveAt" type="datetime" value-format="YYYY-MM-DD HH:mm:ss" :disabled-date="disablePastDate" placeholder="选择未来时间" /></el-form-item>
        </div>
        <el-form-item label="清算处理"><el-checkbox v-model="form.waitForSettlement">如仍有阻断项，等待结算及相关事项完成后自动降级</el-checkbox></el-form-item>
        <el-form-item label="操作原因" prop="reason"><el-input v-model.trim="form.reason" type="textarea" :rows="3" maxlength="300" show-word-limit /></el-form-item>
        <el-form-item label="内部备注"><el-input v-model.trim="form.remark" maxlength="300" /></el-form-item>
      </section>

      <section v-show="step === 1" class="section" v-loading="previewing">
        <template v-if="preview">
          <div class="preview-head"><div><span>当前身份</span><strong>{{ identityLabel(preview.currentIdentity) }}</strong></div><div><span>降级结果</span><strong>{{ preview.targetIdentity ? identityLabel(preview.targetIdentity) : '终止商业身份' }}</strong></div><el-tag :type="previewStatusType">{{ previewStatusLabel }}</el-tag></div>
          <div class="metric-grid"><article><span>下级会员</span><strong>{{ preview.downlineMembers }}</strong></article><article><span>下级代理商</span><strong>{{ preview.downlineAgents }}</strong></article><article><span>预计迁移关系</span><strong>{{ preview.migrationCount }}</strong></article><article><span>将失去归属</span><strong>{{ preview.unassignedCount }}</strong></article></div>
          <el-table :data="preview.checks" border size="small" empty-text="无阻断项"><el-table-column prop="label" label="检查项" /><el-table-column label="数量/金额"><template #default="scope">{{ scope.row.amountCents ? money(scope.row.amountCents) : scope.row.count }}</template></el-table-column><el-table-column label="结果"><template #default="scope"><el-tag :type="scope.row.blocking ? 'danger' : 'success'">{{ scope.row.blocking ? '需处理' : '通过' }}</el-tag></template></el-table-column></el-table>
          <el-alert v-if="preview.blockers.length" :type="preview.status === 'WAITING' ? 'warning' : 'error'" :title="preview.status === 'WAITING' ? '将等待阻断项清零后自动执行' : '当前不能确认降级'" :closable="false" show-icon><ul><li v-for="item in preview.blockers" :key="item">{{ item }}</li></ul></el-alert>
          <el-alert type="info" title="降级影响" :closable="false" show-icon><ul><li v-for="item in preview.riskWarnings" :key="item">{{ item }}</li></ul></el-alert>
          <el-descriptions :column="2" border><el-descriptions-item label="下级策略">{{ strategyLabel(preview.childStrategy) }}</el-descriptions-item><el-descriptions-item label="等待自动降级">{{ preview.waitForSettlement ? '是' : '否' }}</el-descriptions-item><el-descriptions-item label="新订单分润路径" :span="2">{{ preview.commissionImpact }}</el-descriptions-item><el-descriptions-item label="操作原因" :span="2">{{ form.reason }}</el-descriptions-item></el-descriptions>
        </template>
        <el-empty v-else description="尚未生成降级预检查" />
      </section>

      <section v-show="step === 2" class="section confirm">
        <el-result icon="warning" title="确认执行受控降级" sub-title="确认后将立即执行，或创建等待清算/定时生效请求。批量关系迁移与身份终止在同一事务中完成。" />
        <el-form-item v-if="needsUnassignedConfirmation" label="请输入“确认结束当前归属”"><el-input v-model.trim="confirmationText" /></el-form-item>
        <el-checkbox v-model="confirmed" size="large">我已核对下级数量、迁移目标、资金阻断项、生效时间和历史数据保留规则</el-checkbox>
      </section>
    </el-form>
    <template #footer><el-button @click="visible=false">取消</el-button><el-button v-if="step>0" :disabled="submitting" @click="step--">上一步</el-button><el-button v-if="step<2" type="primary" :loading="previewing" :disabled="step===1 && preview?.status==='BLOCKED'" @click="next">下一步</el-button><el-button v-else type="danger" :loading="submitting" :disabled="!confirmed || (needsUnassignedConfirmation && confirmationText !== '确认结束当前归属')" @click="confirmDowngrade">确认降级</el-button></template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage, type FormInstance, type FormRules } from "element-plus";
import { identityManagementApi, type BusinessIdentityType, type IdentityDowngradePreview, type IdentityDowngradeRequest, type IdentityDowngradeStrategy, type IdentityOption } from "../../api/identityManagement";

const props=defineProps<{userId:string;userName?:string;currentIdentity:BusinessIdentityType;agents:IdentityOption[];centers:IdentityOption[]}>();
const emit=defineEmits<{success:[]}>();const visible=defineModel<boolean>({required:true});const formRef=ref<FormInstance>();const step=ref(0);const preview=ref<IdentityDowngradePreview|null>(null);const previewing=ref(false);const submitting=ref(false);const confirmed=ref(false);const confirmationText=ref("");const errorMessage=ref("");
const form=reactive({targetIdentity:"" as ""|"AGENT",childStrategy:"" as ""|IdentityDowngradeStrategy,targetAgentId:"",targetOperationCenterId:"",scheduled:false,effectiveAt:"",waitForSettlement:false,reason:"",remark:""});
const rules:FormRules={childStrategy:[{required:true,message:"请选择下级处理方式",trigger:"change"}],targetAgentId:[{validator:(_r,v,cb)=>form.childStrategy==="TRANSFER_TO_AGENT"&&!v?cb(new Error("请选择承接代理商")):cb(),trigger:"change"}],targetOperationCenterId:[{validator:(_r,v,cb)=>form.childStrategy==="DIRECT_OPERATION_CENTER"&&!v?cb(new Error("请选择承接运营中心")):cb(),trigger:"change"}],effectiveAt:[{validator:(_r,v,cb)=>form.scheduled&&!v?cb(new Error("请选择生效时间")):cb(),trigger:"change"}],reason:[{required:true,message:"请填写操作原因",trigger:"blur"},{min:4,message:"操作原因至少 4 个字符",trigger:"blur"}]};
const activeAgents=computed(()=>props.agents.filter(item=>!item.status||String(item.status).toUpperCase()==="ACTIVE"));const activeCenters=computed(()=>props.centers.filter(item=>!item.status||String(item.status).toUpperCase()==="ACTIVE"));
const previewStatusType=computed(()=>preview.value?.status==="BLOCKED"?"danger":preview.value?.status==="WAITING"||preview.value?.status==="SCHEDULED"?"warning":"success");const previewStatusLabel=computed(()=>({READY:"可立即执行",BLOCKED:"存在阻断",WAITING:"等待清算",SCHEDULED:"定时生效",CONSUMED:"已确认",EXPIRED:"已过期"}[preview.value?.status||"READY"]));
const needsUnassignedConfirmation=computed(()=>preview.value?.childStrategy==="PRESERVE_HISTORY"&&Number(preview.value?.unassignedCount||0)>0);
watch(visible,open=>{if(open)reset()});
function reset(){step.value=0;preview.value=null;previewing.value=false;submitting.value=false;confirmed.value=false;confirmationText.value="";errorMessage.value="";Object.assign(form,{targetIdentity:props.currentIdentity==="OPERATION_CENTER"?"AGENT":"",childStrategy:"",targetAgentId:"",targetOperationCenterId:"",scheduled:false,effectiveAt:"",waitForSettlement:false,reason:"",remark:""})}
async function next(){errorMessage.value="";if(step.value===0){const valid=await formRef.value?.validate().catch(()=>false);if(!valid)return;previewing.value=true;try{preview.value=await identityManagementApi.downgradePreview(props.userId,payload());step.value=1}catch(error){errorMessage.value=error instanceof Error?error.message:"降级预检查失败"}finally{previewing.value=false}return}if(step.value===1&&preview.value?.status!=="BLOCKED")step.value=2}
function payload():IdentityDowngradeRequest{return{targetIdentity:form.targetIdentity||undefined,childStrategy:form.childStrategy as IdentityDowngradeStrategy,targetAgentId:form.childStrategy==="TRANSFER_TO_AGENT"?form.targetAgentId:undefined,targetOperationCenterId:form.childStrategy==="DIRECT_OPERATION_CENTER"?form.targetOperationCenterId:undefined,waitForSettlement:form.waitForSettlement,effectiveAt:form.scheduled&&form.effectiveAt?new Date(form.effectiveAt.replace(" ","T")).toISOString():undefined,reason:form.reason,remark:form.remark}}
async function confirmDowngrade(){if(!preview.value||submitting.value)return;submitting.value=true;errorMessage.value="";try{const result=await identityManagementApi.downgradeConfirm(props.userId,preview.value.previewToken,confirmationText.value);ElMessage.success(result.status==="SUCCEEDED"?`降级完成，迁移 ${result.migratedRelationships} 条关系`:`降级请求已创建：${result.status}`);visible.value=false;emit("success")}catch(error){errorMessage.value=error instanceof Error?error.message:"降级确认失败"}finally{submitting.value=false}}
function disablePastDate(date:Date){return date.getTime()<Date.now()-86400000}function optionLabel(item:IdentityOption){return item.name||item.owner||item.userId||item.id}function identityLabel(value:string){return value==="OPERATION_CENTER"?"运营中心":value==="AGENT"?"代理商":value}function strategyLabel(value:string){return({TRANSFER_TO_AGENT:"迁移给其他代理商",DIRECT_OPERATION_CENTER:"转运营中心直属",PRESERVE_HISTORY:"结束当前归属，不重新分配"}as Record<string,string>)[value]||value}function money(cents:number){return`¥${(Number(cents||0)/100).toLocaleString("zh-CN",{minimumFractionDigits:2})}`}function dateTime(value:string){return new Date(value).toLocaleString("zh-CN",{hour12:false})}
</script>

<style scoped>
.dialog-title,.preview-head{display:flex;align-items:center;justify-content:space-between;gap:16px}.dialog-title>div{display:grid}.dialog-title span,.preview-head span{color:var(--admin-muted);font-size:12px}.dialog-title strong{font-size:19px}.steps{margin:4px 0 24px}.section{display:grid;gap:16px;min-height:410px}.strategy-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;width:100%}.strategy-grid :deep(.el-radio){height:auto;min-height:82px;margin:0;display:grid;gap:4px}.strategy-grid small{display:block;color:var(--admin-muted)}.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:14px}.form-grid :deep(.el-date-editor),.section :deep(.el-select){width:100%}.preview-head>div{display:grid}.metric-grid{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}.metric-grid article{padding:13px;border:1px solid var(--admin-border);border-radius:9px}.metric-grid span{display:block;color:var(--admin-muted);font-size:12px}.metric-grid strong{display:block;margin-top:6px;font-size:21px}.metric-grid .date-value{font-size:13px}.section :deep(.el-alert__content) ul{margin:6px 0 0;padding-left:18px}.confirm{justify-items:center}.confirm .el-result{padding-bottom:8px}@media(max-width:760px){.strategy-grid,.form-grid,.metric-grid{grid-template-columns:1fr}.section{min-height:520px}}
</style>
