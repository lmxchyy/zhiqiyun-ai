<template>
  <section class="enterprise-management">
    <el-result v-if="!canAccessCurrentPage" icon="warning" title="无权限" sub-title="当前角色没有访问此企业管理页面的权限。" />

    <template v-else-if="isListPage">
      <header class="enterprise-page-head">
        <div>
          <p>ENTERPRISE MANAGEMENT</p>
          <h1>企业管理</h1>
          <span>管理平台企业客户、套餐席位、算力账户及服务状态</span>
        </div>
        <div class="enterprise-page-actions">
          <el-button :icon="Download" :disabled="!can('enterprise:export')" :loading="exporting" @click="exportList">导出</el-button>
          <el-button type="primary" :icon="Plus" :disabled="!can('enterprise:create')" @click="openCreateDialog">创建企业</el-button>
        </div>
      </header>

      <div class="enterprise-stat-grid">
        <article v-for="stat in statCards" :key="stat.label">
          <span>{{ stat.label }}</span>
          <strong>{{ formatNumber(stat.value) }}</strong>
          <small>{{ stat.hint }}</small>
          <i :class="stat.tone"></i>
        </article>
      </div>

      <section class="enterprise-panel enterprise-filter-panel">
        <div class="enterprise-filter-row">
          <el-input v-model="filters.keyword" class="keyword-input" clearable :prefix-icon="Search" placeholder="搜索企业名称或企业 ID" @keyup.enter="applyFilters" />
          <el-select v-model="filters.certificationStatus" clearable placeholder="认证状态">
            <el-option label="已认证" value="APPROVED" />
            <el-option label="待审核" value="PENDING" />
            <el-option label="已驳回" value="REJECTED" />
            <el-option label="未认证" value="UNVERIFIED" />
          </el-select>
          <el-select v-model="filters.planCode" clearable placeholder="企业套餐">
            <el-option v-for="item in listResult.filters.plans" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-select v-model="filters.status" clearable placeholder="企业状态">
            <el-option label="正常" value="ACTIVE" />
            <el-option label="已暂停" value="SUSPENDED" />
            <el-option label="已禁用" value="DISABLED" />
          </el-select>
          <el-date-picker v-model="createdRange" type="daterange" value-format="YYYY-MM-DD" range-separator="至" start-placeholder="创建开始" end-placeholder="创建结束" />
          <el-button type="primary" :icon="Search" @click="applyFilters">查询</el-button>
          <el-button @click="resetFilters">重置</el-button>
        </div>
        <div class="enterprise-filter-row secondary">
          <el-select v-model="filters.sourceAgentId" clearable filterable placeholder="来源代理商">
            <el-option v-for="item in listResult.filters.sourceAgents" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <el-select v-model="filters.operationCenterId" clearable filterable placeholder="所属运营中心">
            <el-option v-for="item in listResult.filters.operationCenters" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
          <span v-if="selectedRows.length" class="selection-summary">已选择 {{ selectedRows.length }} 家企业</span>
          <el-dropdown v-if="selectedRows.length" trigger="click" @command="handleBatchCommand">
            <el-button>批量操作<el-icon class="el-icon--right"><ArrowDown /></el-icon></el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="export" :disabled="!can('enterprise:export')">导出所选</el-dropdown-item>
                <el-dropdown-item command="risk" :disabled="!can('enterprise:risk:disable')">暂停服务</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </section>

      <section class="enterprise-panel enterprise-table-panel" v-loading="loading">
        <el-alert v-if="loadError" class="enterprise-state-alert" type="error" :closable="false" show-icon :title="loadError">
          <template #default><el-button link type="primary" @click="loadList">重新加载</el-button></template>
        </el-alert>
        <el-table
          v-else
          :data="listResult.items"
          row-key="id"
          stripe
          height="480"
          empty-text="暂无企业数据"
          @selection-change="selectedRows = $event"
          @row-click="openDetail"
        >
          <el-table-column type="selection" width="44" reserve-selection />
          <el-table-column label="企业" min-width="220" fixed="left">
            <template #default="scope">
              <button class="enterprise-name-cell" type="button" @click.stop="openDetail(scope.row)">
                <span>{{ initials(scope.row.name) }}</span>
                <i><strong>{{ scope.row.name }}</strong><small>{{ scope.row.enterpriseCode }}</small></i>
              </button>
            </template>
          </el-table-column>
          <el-table-column label="认证状态" width="110"><template #default="scope"><EnterpriseStatusTag :value="scope.row.certificationStatus" kind="certification" /></template></el-table-column>
          <el-table-column label="套餐" min-width="150"><template #default="scope"><strong>{{ scope.row.plan.name }}</strong><small class="cell-subtext">{{ expiryText(scope.row.plan.expiresAt) }}</small></template></el-table-column>
          <el-table-column label="成员 / 席位" width="120"><template #default="scope">{{ scope.row.memberCount }} / {{ scope.row.seatLimit }}</template></el-table-column>
          <el-table-column label="算力余额" width="135" align="right"><template #default="scope"><strong>{{ formatPoints(scope.row.compute.balance) }}</strong><small class="cell-subtext">点</small></template></el-table-column>
          <el-table-column label="来源代理商" min-width="140"><template #default="scope">{{ scope.row.sourceAgent.name || "平台直客" }}</template></el-table-column>
          <el-table-column label="运营中心" min-width="140"><template #default="scope">{{ scope.row.operationCenter.name || "未分配" }}</template></el-table-column>
          <el-table-column label="企业状态" width="105"><template #default="scope"><EnterpriseStatusTag :value="scope.row.status" /></template></el-table-column>
          <el-table-column label="创建时间" width="170"><template #default="scope">{{ formatTime(scope.row.createdAt) }}</template></el-table-column>
          <el-table-column label="操作" width="96" fixed="right"><template #default="scope"><el-button link type="primary" @click.stop="openDetail(scope.row)">详情</el-button></template></el-table-column>
        </el-table>
        <footer class="enterprise-pagination">
          <span>共 {{ listResult.total }} 条</span>
          <el-pagination
            v-model:current-page="filters.page"
            v-model:page-size="filters.pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="listResult.total"
            layout="sizes, prev, pager, next, jumper"
            @current-change="pageChanged"
            @size-change="pageSizeChanged"
          />
        </footer>
      </section>
    </template>

    <template v-else-if="isDetailPage">
      <el-skeleton v-if="loading" :rows="10" animated />
      <el-result v-else-if="loadError" icon="error" title="企业详情加载失败" :sub-title="loadError">
        <template #extra><el-button type="primary" @click="loadDetail">重新加载</el-button></template>
      </el-result>
      <template v-else-if="detail">
        <header class="enterprise-detail-head">
          <el-button link :icon="ArrowLeft" @click="navigate('/admin/enterprises', 'enterpriseList')">返回企业列表</el-button>
          <div class="enterprise-detail-title">
            <span>{{ initials(detail.enterprise.name) }}</span>
            <div><h1>{{ detail.enterprise.name }}</h1><p>{{ detail.enterprise.enterpriseCode }} · 创建于 {{ formatTime(detail.enterprise.createdAt) }}</p></div>
            <EnterpriseStatusTag :value="detail.enterprise.certificationStatus" kind="certification" />
            <EnterpriseStatusTag :value="detail.enterprise.status" />
          </div>
          <div class="enterprise-page-actions">
            <el-button :disabled="!can('enterprise:update')" @click="openOperation('profile/update')">编辑资料</el-button>
            <el-button type="primary" :disabled="!can('enterprise:package:adjust')" @click="navigate(detailPath('/package'), 'enterprisePackage')">调整套餐</el-button>
          </div>
        </header>

        <div class="enterprise-detail-stat-grid">
          <article><span>当前套餐</span><strong>{{ detail.enterprise.plan.name }}</strong><small>{{ expiryText(detail.enterprise.plan.expiresAt) }}</small></article>
          <article><span>成员 / 席位</span><strong>{{ detail.enterprise.memberCount }} / {{ detail.enterprise.seatLimit }}</strong><small>{{ detail.organizationCount }} 个组织节点</small></article>
          <article><span>算力余额</span><strong>{{ formatPoints(detail.enterprise.compute.balance) }}</strong><small>单位：点（POINT）</small></article>
          <article><span>客户归属</span><strong>{{ detail.enterprise.sourceAgent.name || "平台直客" }}</strong><small>{{ detail.enterprise.operationCenter.name || "未分配运营中心" }}</small></article>
        </div>

        <nav class="enterprise-detail-tabs" aria-label="企业详情页面">
          <button v-for="tab in detailTabs" :key="tab.suffix" :class="{ active: isDetailTabActive(tab) }" type="button" @click="navigate(detailPath(tab.suffix), tab.moduleId)">{{ tab.label }}</button>
        </nav>

        <EnterprisePrivacyBanner :message="detail.privacy.message" />

        <div class="enterprise-detail-grid">
          <section class="enterprise-panel enterprise-profile-card">
            <header><div><h2>基本资料</h2><span>企业基础识别与认证摘要</span></div><el-button link type="primary" :disabled="!can('enterprise:update')" @click="openOperation('profile/update')">编辑</el-button></header>
            <dl>
              <div><dt>企业全称</dt><dd>{{ detail.profile.legalName || detail.enterprise.name }}</dd></div>
              <div><dt>企业 ID</dt><dd>{{ detail.enterprise.enterpriseCode }}</dd></div>
              <div><dt>统一社会信用代码</dt><dd>{{ maskCreditCode(detail.profile.unifiedSocialCreditCode) }}</dd></div>
              <div><dt>法定代表人</dt><dd>{{ detail.profile.legalRepresentativeName || "未提交" }}</dd></div>
              <div><dt>所属行业</dt><dd>{{ detail.profile.industry || "未设置" }}</dd></div>
              <div><dt>企业规模</dt><dd>{{ detail.profile.companySize || "未设置" }}</dd></div>
              <div><dt>来源代理商</dt><dd>{{ detail.enterprise.sourceAgent.name || "平台直客" }}</dd></div>
              <div><dt>所属运营中心</dt><dd>{{ detail.enterprise.operationCenter.name || "未分配" }}</dd></div>
            </dl>
          </section>
          <section class="enterprise-panel enterprise-usage-card">
            <header><div><h2>算力账户概览</h2><span>服务端返回的 POINT 单位余额</span></div><el-button link type="primary" @click="navigate(detailPath('/compute'), 'enterpriseCompute')">查看账户</el-button></header>
            <div class="enterprise-compute-balance"><span>可用算力</span><strong>{{ formatPoints(detail.enterprise.compute.balance) }}</strong><small>POINT</small></div>
            <div class="enterprise-meter"><i :style="{ width: computePercent + '%' }"></i></div>
            <div class="enterprise-compute-legend"><span>可用 {{ formatPoints(detail.enterprise.compute.balance) }}</span><span>冻结 {{ formatPoints(detail.enterprise.compute.frozen) }}</span></div>
          </section>
          <section class="enterprise-panel enterprise-audit-card">
            <header><div><h2>最近操作</h2><span>所有企业写操作必须进入审计链路</span></div><el-button link type="primary" @click="navigate(detailPath('/audit-logs'), 'enterpriseAuditLogs')">全部日志</el-button></header>
            <el-timeline v-if="detail.recentOperations.length">
              <el-timeline-item v-for="item in detail.recentOperations" :key="item.id" :timestamp="formatTime(item.createdAt)" placement="top">
                <strong>{{ item.summary || item.action }}</strong><p>{{ item.actor || "系统" }} · {{ item.action }}</p>
              </el-timeline-item>
            </el-timeline>
            <el-empty v-else description="暂无审计记录" :image-size="72" />
          </section>
        </div>
      </template>
    </template>

    <template v-else>
      <header class="enterprise-page-head">
        <div>
          <p>ENTERPRISE MANAGEMENT</p>
          <h1>{{ currentPageMeta.title }}</h1>
          <span>{{ currentPageMeta.description }}</span>
        </div>
        <div class="enterprise-page-actions">
          <el-button v-if="enterpriseId" :icon="ArrowLeft" @click="navigate(`/admin/enterprises/${enterpriseId}`, 'enterpriseDetail')">返回企业概览</el-button>
          <el-button v-if="moduleId === 'enterprisePackage'" :disabled="!can('enterprise:seat:adjust')" @click="openOperation('seats/adjust')">调整席位</el-button>
          <el-button v-if="moduleId === 'enterprisePackage'" type="primary" :disabled="!can('enterprise:package:adjust')" @click="openOperation('package/adjust')">调整套餐</el-button>
          <el-button v-if="moduleId === 'enterpriseCompute'" :disabled="!can('enterprise:compute:adjust')" @click="openOperation('compute/adjust')">调整算力</el-button>
          <el-button v-if="moduleId === 'enterpriseCompute'" type="primary" :disabled="!can('enterprise:compute:adjust')" @click="openOperation('recharge')">企业充值</el-button>
          <el-button v-if="moduleId === 'enterpriseAiCapabilities'" type="primary" :disabled="!can('enterprise:ai:configure')" @click="openOperation('ai-capabilities/configure')">配置 AI 能力</el-button>
          <el-button v-if="moduleId === 'enterpriseAttribution' || moduleId === 'enterpriseRelationships'" type="primary" :disabled="!can('enterprise:attribution:change')" @click="openOperation('attribution/change')">变更归属</el-button>
          <el-button v-if="moduleId === 'enterpriseRisk' && sectionEnterpriseStatus !== 'ACTIVE'" type="primary" :disabled="!can('enterprise:risk:restore')" @click="openOperation('risk/restore')">恢复服务</el-button>
          <el-button v-if="moduleId === 'enterpriseRisk' && sectionEnterpriseStatus === 'ACTIVE'" type="danger" :disabled="!can('enterprise:risk:disable')" @click="openOperation('risk/disable')">暂停服务</el-button>
        </div>
      </header>
      <EnterprisePrivacyBanner v-if="currentPageMeta.sensitive" />
      <el-skeleton v-if="loading" :rows="9" animated />
      <el-result v-else-if="loadError" icon="error" title="页面加载失败" :sub-title="loadError"><template #extra><el-button type="primary" @click="loadSection">重新加载</el-button></template></el-result>
      <template v-else>
        <div class="enterprise-section-summary">
          <article v-for="item in sectionSummaryCards" :key="item.key"><span>{{ item.label }}</span><strong>{{ item.value }}</strong><small>{{ item.hint }}</small></article>
        </div>
        <section class="enterprise-panel enterprise-section-panel">
          <header>
            <div><h2>{{ currentPageMeta.title }}</h2><span>{{ sectionItems.length }} 条记录 · 数据由企业范围接口返回</span></div>
            <el-button :icon="Refresh" :loading="loading" @click="loadSection">刷新</el-button>
          </header>
          <el-table v-if="sectionItems.length" :data="sectionItems" :row-key="sectionRowKey" stripe>
            <el-table-column v-for="column in sectionColumns" :key="column.key" :label="column.label" :min-width="column.width || 130">
              <template #default="scope">
                <EnterpriseStatusTag v-if="column.key === 'status'" :value="String(scope.row[column.key] || '')" />
                <span v-else :class="{ 'enterprise-json-value': isStructuredValue(scope.row[column.key]) }">{{ formatSectionCell(scope.row[column.key], column.key) }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="moduleId === 'enterpriseCertifications' && can('enterprise:certification:review')" label="操作" width="130" fixed="right">
              <template #default="scope"><el-button link type="primary" :disabled="!['PENDING','SUBMITTED'].includes(String(scope.row.status || '').toUpperCase())" @click="openCertificationReview(scope.row)">审核</el-button></template>
            </el-table-column>
          </el-table>
          <el-empty v-else description="暂无数据" :image-size="96" />
        </section>
      </template>
    </template>

    <el-dialog v-model="createDialogVisible" title="创建企业" width="600px" destroy-on-close :close-on-click-modal="false">
      <el-form label-position="top" :model="createForm">
        <div class="enterprise-form-grid">
          <el-form-item label="企业名称" required><el-input v-model.trim="createForm.name" maxlength="160" show-word-limit placeholder="请输入企业名称" /></el-form-item>
          <el-form-item label="企业 ID"><el-input v-model.trim="createForm.enterpriseCode" placeholder="留空由系统生成" /></el-form-item>
          <el-form-item label="初始套餐"><el-select v-model="createForm.planCode" placeholder="请选择"><el-option v-for="item in listResult.filters.plans" :key="item.value" :label="item.label" :value="item.value" /><el-option v-if="!listResult.filters.plans.length" label="企业试用版" value="enterprise_trial" /></el-select></el-form-item>
          <el-form-item label="成员席位" required><el-input-number v-model="createForm.seatLimit" :min="1" :max="100000" controls-position="right" /></el-form-item>
          <el-form-item label="所属行业"><el-input v-model.trim="createForm.industry" placeholder="例如：企业服务" /></el-form-item>
          <el-form-item label="企业规模"><el-select v-model="createForm.companySize" clearable placeholder="请选择"><el-option label="1-20 人" value="1-20" /><el-option label="21-100 人" value="21-100" /><el-option label="101-500 人" value="101-500" /><el-option label="500 人以上" value="500+" /></el-select></el-form-item>
          <el-form-item label="来源代理商"><el-select v-model="createForm.sourceAgentId" clearable filterable placeholder="平台直客"><el-option v-for="item in listResult.filters.sourceAgents" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
          <el-form-item label="所属运营中心"><el-select v-model="createForm.operationCenterId" clearable filterable placeholder="暂不分配"><el-option v-for="item in listResult.filters.operationCenters" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
        </div>
      </el-form>
      <el-alert type="info" :closable="false" show-icon title="创建成功后将写入企业审计日志，套餐和席位可在企业详情中继续调整。" />
      <template #footer><el-button :disabled="creating" @click="createDialogVisible = false">取消</el-button><el-button type="primary" :loading="creating" :disabled="!createForm.name" @click="submitCreate">确认创建</el-button></template>
    </el-dialog>

    <el-dialog v-model="operationDialogVisible" :title="operationTitle" width="620px" destroy-on-close :close-on-click-modal="false">
      <el-form label-position="top" :model="operationForm">
        <template v-if="operationAction === 'profile/update'">
          <el-form-item label="企业名称" required><el-input v-model.trim="operationForm.name" maxlength="160" show-word-limit /></el-form-item>
          <div class="enterprise-form-grid">
            <el-form-item label="所属行业"><el-input v-model.trim="operationForm.industry" maxlength="80" /></el-form-item>
            <el-form-item label="企业规模"><el-input v-model.trim="operationForm.companySize" maxlength="40" /></el-form-item>
          </div>
        </template>
        <el-form-item v-if="operationAction === 'certifications/review'" label="审核结果" required><el-radio-group v-model="operationForm.status"><el-radio-button value="APPROVED">通过</el-radio-button><el-radio-button value="REJECTED">驳回</el-radio-button></el-radio-group></el-form-item>
        <el-form-item v-if="operationAction === 'certifications/review'" label="审核意见"><el-input v-model.trim="operationForm.reviewComment" type="textarea" :rows="3" maxlength="500" show-word-limit /></el-form-item>
        <template v-if="operationAction === 'package/adjust'">
          <el-form-item label="套餐编码" required><el-input v-model.trim="operationForm.planCode" placeholder="例如 enterprise_pro" /></el-form-item>
          <el-form-item label="到期时间"><el-date-picker v-model="operationForm.expiresAt" type="datetime" value-format="YYYY-MM-DDTHH:mm:ssZ" placeholder="不填则保持当前到期时间" /></el-form-item>
        </template>
        <el-form-item v-if="operationAction === 'seats/adjust'" label="成员席位" required><el-input-number v-model="operationForm.seatLimit" :min="1" :max="100000" controls-position="right" /></el-form-item>
        <el-form-item v-if="operationAction === 'compute/adjust' || operationAction === 'recharge'" label="算力变更（POINT）" required><el-input-number v-model="operationForm.pointDelta" :min="operationAction === 'recharge' ? 1 : -1000000000" :max="1000000000" controls-position="right" /></el-form-item>
        <template v-if="operationAction === 'ai-capabilities/configure'">
          <div class="enterprise-form-grid">
            <el-form-item label="能力模块编码" required><el-input v-model.trim="operationForm.moduleCode" placeholder="例如 text_generation" /></el-form-item>
            <el-form-item label="模型名称"><el-input v-model.trim="operationForm.modelName" placeholder="留空表示模块默认" /></el-form-item>
          </div>
          <el-form-item label="开通状态" required><el-radio-group v-model="operationForm.status"><el-radio-button value="ACTIVE">已开通</el-radio-button><el-radio-button value="DISABLED">已停用</el-radio-button></el-radio-group></el-form-item>
          <el-form-item label="限额 JSON"><el-input v-model="operationForm.limitsText" type="textarea" :rows="4" placeholder='{"dailyRequests": 1000}' /></el-form-item>
        </template>
        <template v-if="operationAction === 'attribution/change'">
          <el-form-item label="来源代理商"><el-select v-model="operationForm.sourceAgentId" clearable filterable placeholder="平台直客"><el-option v-for="item in lookupFilters.sourceAgents" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
          <el-form-item label="所属运营中心"><el-select v-model="operationForm.operationCenterId" clearable filterable placeholder="不分配"><el-option v-for="item in lookupFilters.operationCenters" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
        </template>
        <el-form-item label="操作原因" required><el-input v-model.trim="operationForm.reason" type="textarea" :rows="3" maxlength="500" show-word-limit placeholder="必填，将写入审批和审计日志" /></el-form-item>
      </el-form>
      <div class="enterprise-change-preview">
        <article><span>修改前</span><pre>{{ JSON.stringify(operationBefore, null, 2) }}</pre></article>
        <article><span>修改后</span><pre>{{ JSON.stringify(operationAfter, null, 2) }}</pre></article>
      </div>
      <el-alert type="warning" :closable="false" show-icon title="写操作采用 requestId 防重复提交，并记录操作人、原因、修改前后内容与结果。非超级管理员的归属变更将进入待审批状态。" />
      <template #footer><el-button :disabled="operating" @click="operationDialogVisible = false">取消</el-button><el-button :type="operationAction === 'risk/disable' ? 'danger' : 'primary'" :loading="operating" :disabled="!operationForm.reason" @click="submitOperation">二次确认</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ArrowDown, ArrowLeft, Download, Plus, Refresh, Search } from "@element-plus/icons-vue";
import { createAdminEnterprise, exportAdminEnterprises, getAdminEnterprise, getAdminEnterpriseSection, listAdminEnterpriseCertifications, listAdminEnterprises, mutateAdminEnterprise, updateAdminEnterprise } from "../../api/adminEnterprise";
import type { AdminEnterpriseCreateRequest, AdminEnterpriseDetail, AdminEnterpriseFilters, AdminEnterpriseListItem, AdminEnterpriseListQuery, AdminEnterpriseListResult, AdminEnterpriseMutationRequest, AdminEnterpriseSectionResult } from "../../types/adminEnterprise";
import EnterprisePrivacyBanner from "./EnterprisePrivacyBanner.vue";
import EnterpriseStatusTag from "./EnterpriseStatusTag.vue";

const props = defineProps<{ moduleId: string; routePath: string; permissions: string[]; currentRole?: string }>();
const emit = defineEmits<{ navigate: [payload: { path: string; moduleId: string }] }>();

const emptyResult = (): AdminEnterpriseListResult => ({
  items: [], total: 0, page: 1, pageSize: 20,
  stats: { total: 0, certified: 0, createdThisMonth: 0, abnormal: 0 },
  filters: { plans: [], sourceAgents: [], operationCenters: [] }
});

const loading = ref(false);
const exporting = ref(false);
const creating = ref(false);
const loadError = ref("");
const listResult = ref<AdminEnterpriseListResult>(emptyResult());
const detail = ref<AdminEnterpriseDetail | null>(null);
const sectionResult = ref<AdminEnterpriseSectionResult>({ section: "", summary: {}, items: [], total: 0, privacy: { summaryOnly: true, message: "", restrictedFields: [] } });
const selectedRows = ref<AdminEnterpriseListItem[]>([]);
const createdRange = ref<string[]>([]);
const createDialogVisible = ref(false);
const operationDialogVisible = ref(false);
const operating = ref(false);
const operationAction = ref("");
const operationEnterpriseId = ref("");
const filters = reactive<AdminEnterpriseListQuery>({ page: 1, pageSize: 20 });
const createForm = reactive<AdminEnterpriseCreateRequest>({ name: "", enterpriseCode: "", planCode: "enterprise_trial", seatLimit: 20, industry: "", companySize: "", sourceAgentId: "", operationCenterId: "" });
type EnterpriseOperationForm = AdminEnterpriseMutationRequest & { limitsText: string };
const operationForm = reactive<EnterpriseOperationForm>({ requestId: "", reason: "", status: "APPROVED", reviewComment: "", planCode: "", expiresAt: "", seatLimit: 20, pointDelta: 0, sourceAgentId: "", operationCenterId: "", name: "", industry: "", companySize: "", moduleCode: "", modelName: "", limits: {}, limitsText: "{}" });

const modulePermissions: Record<string, string> = {
  enterpriseList: "enterprise:list", enterpriseDetail: "enterprise:detail", enterpriseCertifications: "enterprise:certification:review",
  enterpriseMembers: "enterprise:member:view", enterprisePackage: "enterprise:package:view", enterpriseCompute: "enterprise:compute:view",
  enterpriseTransactions: "enterprise:transaction:view", enterpriseOrders: "enterprise:order:view", enterpriseAiCapabilities: "enterprise:ai:view",
  enterpriseAiEmployees: "enterprise:employee:view", enterpriseKnowledgeBases: "enterprise:knowledge:view", enterpriseAttribution: "enterprise:attribution:view",
  enterpriseRelationships: "enterprise:attribution:view", enterpriseRisk: "enterprise:risk:view", enterpriseAuditLogs: "enterprise:audit:view"
};

const pageMeta: Record<string, { title: string; description: string; phase: string; permission: string; sensitive?: boolean }> = {
  enterpriseCertifications: { title: "企业认证审核", description: "审核企业主体资质，支持通过、驳回和审计追溯", phase: "第二阶段", permission: "enterprise:certification:review" },
  enterpriseMembers: { title: "企业成员与组织", description: "查看成员、席位占用和组织架构统计", phase: "第二阶段", permission: "enterprise:member:view", sensitive: true },
  enterprisePackage: { title: "企业套餐配置", description: "管理套餐、到期时间和成员席位", phase: "第二阶段", permission: "enterprise:package:view" },
  enterpriseCompute: { title: "企业算力账户", description: "查看并审计企业算力余额及调整记录", phase: "第二阶段", permission: "enterprise:compute:view" },
  enterpriseTransactions: { title: "充值与消费明细", description: "按服务端单位查看企业充值、消费与余额变化", phase: "第二阶段", permission: "enterprise:transaction:view" },
  enterpriseOrders: { title: "企业订单", description: "查看企业套餐、算力与服务订单", phase: "第二阶段", permission: "enterprise:order:view" },
  enterpriseAiCapabilities: { title: "模型与 AI 能力", description: "查看企业已开通模型和能力范围", phase: "第三阶段", permission: "enterprise:ai:view" },
  enterpriseAiEmployees: { title: "企业 AI 员工", description: "仅展示 AI 员工数量和运行状态统计", phase: "第三阶段", permission: "enterprise:employee:view", sensitive: true },
  enterpriseKnowledgeBases: { title: "知识库概览", description: "仅展示知识库数量、容量与使用统计", phase: "第三阶段", permission: "enterprise:knowledge:view", sensitive: true },
  enterpriseAttribution: { title: "客户归属", description: "查看和发起企业客户归属变更", phase: "第三阶段", permission: "enterprise:attribution:view" },
  enterpriseRelationships: { title: "代理商与运营中心关系", description: "查看企业来源代理商和运营中心关系", phase: "第三阶段", permission: "enterprise:attribution:view" },
  enterpriseRisk: { title: "企业风控与禁用", description: "查看风险记录，暂停或恢复企业服务", phase: "第四阶段", permission: "enterprise:risk:view" },
  enterpriseAuditLogs: { title: "企业审计日志", description: "追踪平台管理员对企业数据的全部写操作", phase: "第四阶段", permission: "enterprise:audit:view" }
};

const detailTabs = [
  { label: "基本资料", moduleId: "enterpriseDetail", suffix: "" }, { label: "认证资料", moduleId: "enterpriseCertifications", suffix: "/certifications" },
  { label: "成员", moduleId: "enterpriseMembers", suffix: "/members" }, { label: "组织架构", moduleId: "enterpriseMembers", suffix: "/members?view=organizations" }, { label: "套餐", moduleId: "enterprisePackage", suffix: "/package" },
  { label: "算力", moduleId: "enterpriseCompute", suffix: "/compute" }, { label: "订单", moduleId: "enterpriseOrders", suffix: "/orders" },
  { label: "AI 能力", moduleId: "enterpriseAiCapabilities", suffix: "/ai-capabilities" }, { label: "AI 员工", moduleId: "enterpriseAiEmployees", suffix: "/ai-employees" },
  { label: "知识库概览", moduleId: "enterpriseKnowledgeBases", suffix: "/knowledge-bases" }, { label: "客户归属", moduleId: "enterpriseAttribution", suffix: "/attribution" },
  { label: "风控", moduleId: "enterpriseRisk", suffix: "/risk" }, { label: "审计日志", moduleId: "enterpriseAuditLogs", suffix: "/audit-logs" }
];

const enterpriseId = computed(() => {
  const match = props.routePath.match(/^\/admin\/enterprises\/([^/?]+)/)?.[1] || "";
  return match === "certifications" ? "" : match;
});
const isOrganizationView = computed(() => props.moduleId === "enterpriseMembers" && new URLSearchParams(props.routePath.split("?")[1] || "").get("view") === "organizations");
const isListPage = computed(() => props.moduleId === "enterpriseList");
const isDetailPage = computed(() => props.moduleId === "enterpriseDetail");
const currentPageMeta = computed(() => isOrganizationView.value ? { title: "组织架构", description: "查看企业组织节点、上下级关系与成员分布", phase: "第二阶段", permission: "enterprise:member:view" } : pageMeta[props.moduleId] || { title: "企业管理", description: "企业业务管理", phase: "第一阶段", permission: modulePermissions[props.moduleId] || "enterprise:detail" });
const canAccessCurrentPage = computed(() => can(modulePermissions[props.moduleId] || "enterprise:detail"));
const statCards = computed(() => [
  { label: "企业总数", value: listResult.value.stats.total, hint: "平台企业客户", tone: "blue" },
  { label: "已认证企业", value: listResult.value.stats.certified, hint: "完成主体认证", tone: "green" },
  { label: "本月新增", value: listResult.value.stats.createdThisMonth, hint: "自然月新增", tone: "purple" },
  { label: "异常企业", value: listResult.value.stats.abnormal, hint: "暂停或禁用", tone: "red" }
]);
const computePercent = computed(() => {
  const compute = detail.value?.enterprise.compute;
  if (!compute) return 0;
  const total = compute.balance + compute.frozen;
  return total > 0 ? Math.max(8, Math.round((compute.balance / total) * 100)) : 0;
});
const lookupFilters = computed<AdminEnterpriseFilters>(() => listResult.value.filters);
const sectionEnterpriseStatus = computed(() => String(sectionResult.value.summary.enterpriseStatus || "ACTIVE").toUpperCase());
const sectionNameMap: Record<string, string> = {
  enterpriseCertifications: "certifications", enterpriseMembers: "members", enterprisePackage: "package", enterpriseCompute: "compute",
  enterpriseTransactions: "transactions", enterpriseOrders: "orders", enterpriseAiCapabilities: "ai-capabilities", enterpriseAiEmployees: "ai-employees",
  enterpriseKnowledgeBases: "knowledge-bases", enterpriseAttribution: "attribution", enterpriseRelationships: "relationships", enterpriseRisk: "risk", enterpriseAuditLogs: "audit-logs"
};
const sectionColumnMap: Record<string, Array<{ key: string; label: string; width?: number }>> = {
  enterpriseCertifications: [{ key: "enterpriseName", label: "企业", width: 180 }, { key: "legalName", label: "认证主体", width: 180 }, { key: "unifiedSocialCreditCode", label: "统一社会信用代码", width: 180 }, { key: "legalRepresentativeName", label: "法定代表人" }, { key: "status", label: "审核状态" }, { key: "createdAt", label: "提交时间", width: 170 }, { key: "reviewComment", label: "审核意见", width: 180 }],
  enterpriseMembers: [{ key: "name", label: "成员姓名" }, { key: "email", label: "邮箱", width: 180 }, { key: "organizationName", label: "组织" }, { key: "roles", label: "角色" }, { key: "dataScope", label: "数据范围" }, { key: "status", label: "状态" }, { key: "joinedAt", label: "加入时间", width: 170 }],
  enterprisePackage: [{ key: "planCode", label: "套餐编码" }, { key: "status", label: "状态" }, { key: "expiresAt", label: "到期时间", width: 180 }, { key: "entitlements", label: "权益快照", width: 260 }],
  enterpriseCompute: [{ key: "type", label: "变更类型" }, { key: "pointDelta", label: "变更点数" }, { key: "balanceAfter", label: "变更后余额" }, { key: "reason", label: "原因", width: 220 }, { key: "referenceId", label: "关联单号", width: 180 }, { key: "createdAt", label: "时间", width: 170 }],
  enterpriseTransactions: [{ key: "type", label: "流水类型" }, { key: "pointDelta", label: "变更点数" }, { key: "balanceAfter", label: "余额" }, { key: "reason", label: "原因", width: 220 }, { key: "requestId", label: "请求 ID", width: 190 }, { key: "createdAt", label: "时间", width: 170 }],
  enterpriseOrders: [{ key: "orderNo", label: "订单号", width: 180 }, { key: "orderType", label: "订单类型" }, { key: "planId", label: "套餐" }, { key: "amountCents", label: "金额" }, { key: "status", label: "状态" }, { key: "paidAt", label: "支付时间", width: 170 }, { key: "createdAt", label: "创建时间", width: 170 }],
  enterpriseAiCapabilities: [{ key: "moduleCode", label: "能力模块" }, { key: "modelName", label: "模型" }, { key: "status", label: "状态" }, { key: "limits", label: "租户限制", width: 300 }, { key: "updatedAt", label: "更新时间", width: 170 }],
  enterpriseAiEmployees: [{ key: "id", label: "AI 员工 ID", width: 210 }, { key: "status", label: "运行状态" }, { key: "version", label: "版本" }, { key: "updatedAt", label: "更新时间", width: 170 }],
  enterpriseAttribution: [{ key: "sourceAgent", label: "来源代理商", width: 220 }, { key: "operationCenter", label: "运营中心", width: 220 }, { key: "status", label: "状态" }, { key: "reason", label: "变更原因", width: 220 }, { key: "before", label: "修改前", width: 240 }, { key: "after", label: "修改后", width: 240 }, { key: "createdAt", label: "时间", width: 170 }],
  enterpriseRelationships: [{ key: "sourceAgent", label: "来源代理商", width: 220 }, { key: "operationCenter", label: "运营中心", width: 220 }, { key: "status", label: "状态" }, { key: "updatedAt", label: "更新时间", width: 170 }],
  enterpriseRisk: [{ key: "riskLevel", label: "风险等级" }, { key: "action", label: "操作" }, { key: "reason", label: "原因", width: 260 }, { key: "status", label: "状态" }, { key: "actorUserId", label: "操作人", width: 180 }, { key: "createdAt", label: "时间", width: 170 }],
  enterpriseAuditLogs: [{ key: "actorRole", label: "角色" }, { key: "actorUserId", label: "操作人", width: 180 }, { key: "action", label: "动作", width: 210 }, { key: "resourceType", label: "资源" }, { key: "status", label: "状态" }, { key: "metadata", label: "审计摘要", width: 320 }, { key: "createdAt", label: "时间", width: 170 }]
};
const organizationColumns = [{ key: "name", label: "组织名称", width: 220 }, { key: "organizationType", label: "组织类型" }, { key: "parentId", label: "上级组织", width: 220 }, { key: "memberCount", label: "成员数" }, { key: "status", label: "状态" }];
const sectionColumns = computed(() => isOrganizationView.value ? organizationColumns : sectionColumnMap[props.moduleId] || []);
const sectionItems = computed<Record<string, unknown>[]>(() => {
  if (!isOrganizationView.value) return sectionResult.value.items;
  const organizations = sectionResult.value.summary.organizations;
  return Array.isArray(organizations) ? organizations.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object") : [];
});
const summaryLabelMap: Record<string, string> = { total: "总数", pending: "待审核", approved: "已通过", rejected: "已驳回", memberCount: "成员数", activeMembers: "活跃成员", seatLimit: "成员席位", organizationCount: "组织节点", seatUsed: "已用席位", balance: "算力余额", frozen: "冻结算力", cashBalanceCents: "现金余额（分）", orderCount: "订单数", amountCents: "订单金额（分）", enabledCount: "已开通能力", configuredCount: "已配置能力", active: "运行中", knowledgeBaseCount: "知识库数", documentCount: "文档数", chunkCount: "切片数", storageBytes: "存储字节", changeRequestCount: "归属变更记录", riskRecordCount: "风险记录", enterpriseStatus: "企业状态" };
const sectionSummaryCards = computed(() => Object.entries(sectionResult.value.summary).filter(([, value]) => typeof value !== "object" && typeof value !== "boolean").slice(0, 4).map(([key, value]) => ({ key, label: summaryLabelMap[key] || key, value: formatSectionCell(value, key), hint: sectionResult.value.unit ? `单位：${sectionResult.value.unit}` : "企业范围汇总" })));
const operationTitle = computed(() => ({ "profile/update": "编辑企业资料", "certifications/review": "审核企业认证", "package/adjust": "调整企业套餐", "seats/adjust": "调整成员席位", "compute/adjust": "调整企业算力", recharge: "企业充值", "ai-capabilities/configure": "配置企业 AI 能力", "attribution/change": "变更企业归属", "risk/disable": "暂停企业服务", "risk/restore": "恢复企业服务" }[operationAction.value] || "企业操作"));
const operationBefore = computed<Record<string, unknown>>(() => {
  if (operationAction.value === "profile/update") return { name: detail.value?.enterprise.name, industry: detail.value?.profile.industry, companySize: detail.value?.profile.companySize };
  if (operationAction.value === "ai-capabilities/configure") return { configuredCapabilities: sectionResult.value.items.length };
  if (operationAction.value === "attribution/change") return { sourceAgent: sectionResult.value.summary.sourceAgent, operationCenter: sectionResult.value.summary.operationCenter };
  if (operationAction.value.startsWith("risk/")) return { status: sectionEnterpriseStatus.value };
  return { ...sectionResult.value.summary };
});
const operationAfter = computed<Record<string, unknown>>(() => {
  switch (operationAction.value) {
    case "profile/update": return { name: operationForm.name, industry: operationForm.industry, companySize: operationForm.companySize };
    case "certifications/review": return { status: operationForm.status, reviewComment: operationForm.reviewComment };
    case "package/adjust": return { planCode: operationForm.planCode, expiresAt: operationForm.expiresAt || "保持不变" };
    case "seats/adjust": return { seatLimit: operationForm.seatLimit };
    case "compute/adjust": case "recharge": return { pointDelta: operationForm.pointDelta, unit: "POINT" };
    case "ai-capabilities/configure": return { moduleCode: operationForm.moduleCode, modelName: operationForm.modelName || "模块默认", status: operationForm.status, limits: operationForm.limitsText };
    case "attribution/change": return { sourceAgentId: operationForm.sourceAgentId || "平台直客", operationCenterId: operationForm.operationCenterId || "未分配" };
    case "risk/disable": return { status: "SUSPENDED" };
    case "risk/restore": return { status: "ACTIVE" };
    default: return {};
  }
});

function can(permission: string) {
  const role = String(props.currentRole || "").toUpperCase();
  return role === "SUPER_ADMIN" || props.permissions.includes("admin.full") || props.permissions.includes(permission);
}

function isDetailTabActive(tab: { moduleId: string; suffix: string }) {
  if (tab.moduleId !== props.moduleId) return false;
  if (tab.moduleId !== "enterpriseMembers") return true;
  return tab.suffix.includes("view=organizations") === isOrganizationView.value;
}

function hydrateFiltersFromUrl() {
  const params = new URLSearchParams(props.routePath.split("?")[1] || "");
  filters.page = Math.max(1, Number(params.get("page") || 1));
  filters.pageSize = Math.max(1, Number(params.get("pageSize") || 20));
  filters.keyword = params.get("keyword") || "";
  filters.certificationStatus = params.get("certificationStatus") || "";
  filters.planCode = params.get("planCode") || "";
  filters.status = params.get("status") || "";
  filters.sourceAgentId = params.get("sourceAgentId") || "";
  filters.operationCenterId = params.get("operationCenterId") || "";
  filters.createdFrom = params.get("createdFrom") || "";
  filters.createdTo = params.get("createdTo") || "";
  createdRange.value = filters.createdFrom && filters.createdTo ? [filters.createdFrom, filters.createdTo] : [];
}

function syncListUrl() {
  const params = new URLSearchParams();
  Object.entries(filters).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") params.set(key, String(value));
  });
  navigate(`/admin/enterprises${params.size ? `?${params.toString()}` : ""}`, "enterpriseList");
}

async function loadList() {
  loading.value = true;
  loadError.value = "";
  try {
    listResult.value = await listAdminEnterprises({ ...filters });
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : "企业列表加载失败";
  } finally {
    loading.value = false;
  }
}

async function loadDetail() {
  if (!enterpriseId.value) { loadError.value = "缺少企业 ID"; return; }
  loading.value = true;
  loadError.value = "";
  try {
    detail.value = await getAdminEnterprise(enterpriseId.value);
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : "企业详情加载失败";
  } finally {
    loading.value = false;
  }
}

async function loadSection() {
  loading.value = true;
  loadError.value = "";
  try {
    const section = sectionNameMap[props.moduleId];
    if (!section) throw new Error("未识别的企业管理页面");
    sectionResult.value = props.moduleId === "enterpriseCertifications" && !enterpriseId.value
      ? await listAdminEnterpriseCertifications()
      : await getAdminEnterpriseSection(enterpriseId.value, section);
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : "企业业务数据加载失败";
  } finally {
    loading.value = false;
  }
}

async function loadLookupFilters() {
  if (lookupFilters.value.sourceAgents.length || lookupFilters.value.operationCenters.length) return;
  try {
    const lookup = await listAdminEnterprises({ page: 1, pageSize: 1 });
    listResult.value = { ...listResult.value, filters: lookup.filters };
  } catch {
    // The operation can still submit an empty attribution, so lookup failure is non-blocking.
  }
}

function applyFilters() {
  filters.page = 1;
  filters.createdFrom = createdRange.value?.[0] || "";
  filters.createdTo = createdRange.value?.[1] || "";
  syncListUrl();
  void loadList();
}

function resetFilters() {
  Object.assign(filters, { page: 1, pageSize: 20, keyword: "", certificationStatus: "", planCode: "", status: "", sourceAgentId: "", operationCenterId: "", createdFrom: "", createdTo: "" });
  createdRange.value = [];
  syncListUrl();
  void loadList();
}

function pageChanged() { syncListUrl(); void loadList(); }
function pageSizeChanged() { filters.page = 1; syncListUrl(); void loadList(); }
function navigate(path: string, moduleId: string) { emit("navigate", { path, moduleId }); }
function openDetail(row: AdminEnterpriseListItem) { navigate(`/admin/enterprises/${row.id}`, "enterpriseDetail"); }
function detailPath(suffix: string) { return `/admin/enterprises/${enterpriseId.value}${suffix}`; }

function openCreateDialog() {
  Object.assign(createForm, { name: "", enterpriseCode: "", planCode: "enterprise_trial", seatLimit: 20, industry: "", companySize: "", sourceAgentId: "", operationCenterId: "" });
  createDialogVisible.value = true;
}

async function submitCreate() {
  if (!createForm.name || creating.value) return;
  creating.value = true;
  try {
    const confirmed = await ElMessageBox.confirm(`确认创建企业“${createForm.name}”并初始化试用套餐与算力账户？`, "二次确认", { type: "warning", confirmButtonText: "确认创建", cancelButtonText: "返回检查" });
    if (confirmed !== "confirm") return;
    const result = await createAdminEnterprise({ ...createForm });
    createDialogVisible.value = false;
    ElMessage.success("企业创建成功，审计记录已写入");
    navigate(`/admin/enterprises/${result.enterprise.id}`, "enterpriseDetail");
  } catch (error) {
    if (error === "cancel" || error === "close") return;
    ElMessage.error(error instanceof Error ? error.message : "创建企业失败");
  } finally {
    creating.value = false;
  }
}

async function exportList() {
  if (exporting.value) return;
  exporting.value = true;
  try {
    const blob = await exportAdminEnterprises({ ...filters });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `企业列表-${new Date().toISOString().slice(0, 10)}.csv`;
    anchor.click();
    URL.revokeObjectURL(url);
    ElMessage.success("企业列表导出成功");
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : "导出失败");
  } finally {
    exporting.value = false;
  }
}

async function handleBatchCommand(command: string) {
  if (command === "export") { void exportList(); return; }
  if (command !== "risk" || !selectedRows.value.length) return;
  try {
    const { value } = await ElMessageBox.prompt("请填写批量暂停服务的原因；该原因会写入每家企业的审计日志。", `批量暂停 ${selectedRows.value.length} 家企业`, {
      inputPattern: /\S{2,}/,
      inputErrorMessage: "请填写不少于 2 个字的原因",
      confirmButtonText: "下一步",
      cancelButtonText: "取消",
    });
    await ElMessageBox.confirm(`确认暂停所选 ${selectedRows.value.length} 家企业的服务？每家企业都会单独执行权限校验、幂等保护和审计记录。`, "危险操作二次确认", {
      type: "error",
      confirmButtonText: "确认批量暂停",
      cancelButtonText: "返回检查",
    });
    operating.value = true;
    for (const enterprise of selectedRows.value) {
      const requestId = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `enterprise-${Date.now()}-${enterprise.id}`;
      await mutateAdminEnterprise(enterprise.id, "risk/disable", { requestId, reason: value });
    }
    ElMessage.success(`已暂停 ${selectedRows.value.length} 家企业，审计日志已写入`);
    selectedRows.value = [];
    await loadList();
  } catch (error) {
    if (error === "cancel" || error === "close") return;
    ElMessage.error(error instanceof Error ? error.message : "批量暂停失败");
  } finally {
    operating.value = false;
  }
}

function openCertificationReview(row: Record<string, unknown>) {
  const targetId = String(row.enterpriseId || enterpriseId.value || "");
  if (!targetId) { ElMessage.error("缺少企业 ID"); return; }
  operationEnterpriseId.value = targetId;
  openOperation("certifications/review");
}

function openOperation(action: string) {
  operationAction.value = action;
  if (action !== "certifications/review") operationEnterpriseId.value = enterpriseId.value;
  else if (!operationEnterpriseId.value) operationEnterpriseId.value = enterpriseId.value;
  Object.assign(operationForm, {
    requestId: "", reason: "", status: action === "ai-capabilities/configure" ? "ACTIVE" : "APPROVED", reviewComment: "",
    planCode: String(sectionResult.value.summary.planCode || ""), expiresAt: "",
    seatLimit: Number(sectionResult.value.summary.seatLimit || 20), pointDelta: action === "recharge" ? 1000 : 0,
    sourceAgentId: String((sectionResult.value.summary.sourceAgent as { id?: string } | undefined)?.id || ""),
    operationCenterId: String((sectionResult.value.summary.operationCenter as { id?: string } | undefined)?.id || ""),
    name: detail.value?.enterprise.name || "", industry: detail.value?.profile.industry || "", companySize: detail.value?.profile.companySize || "",
    moduleCode: "", modelName: "", limits: {}, limitsText: "{}"
  });
  if (action === "attribution/change") void loadLookupFilters();
  operationDialogVisible.value = true;
}

async function submitOperation() {
  const targetId = operationEnterpriseId.value || enterpriseId.value;
  if (!targetId || operating.value || !operationForm.reason) return;
  if (operationAction.value === "certifications/review" && operationForm.status === "REJECTED" && !operationForm.reviewComment) {
    ElMessage.warning("驳回认证时必须填写审核意见");
    return;
  }
  if (operationAction.value === "profile/update" && !operationForm.name?.trim()) {
    ElMessage.warning("企业名称不能为空");
    return;
  }
  if (operationAction.value === "ai-capabilities/configure") {
    if (!operationForm.moduleCode?.trim()) {
      ElMessage.warning("请填写能力模块编码");
      return;
    }
    try {
      const parsed = JSON.parse(operationForm.limitsText || "{}");
      if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") throw new Error("限额必须是 JSON 对象");
      operationForm.limits = parsed as Record<string, unknown>;
    } catch (error) {
      ElMessage.warning(error instanceof Error ? error.message : "限额 JSON 格式不正确");
      return;
    }
  }
  operating.value = true;
  try {
    await ElMessageBox.confirm(`确认执行“${operationTitle.value}”？操作原因和修改前后内容将进入审计日志。`, "二次确认", { type: operationAction.value === "risk/disable" ? "error" : "warning", confirmButtonText: "确认执行", cancelButtonText: "返回检查" });
    const requestId = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `enterprise-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const payload: AdminEnterpriseMutationRequest = { ...operationForm, requestId };
    const result = operationAction.value === "profile/update" ? await updateAdminEnterprise(targetId, payload) : await mutateAdminEnterprise(targetId, operationAction.value, payload);
    operationDialogVisible.value = false;
    operationEnterpriseId.value = "";
    if (result.status === "PENDING_APPROVAL") ElMessage.warning(result.message);
    else ElMessage.success(result.message || "操作成功");
    if (operationAction.value === "profile/update") await loadDetail();
    else await loadSection();
  } catch (error) {
    if (error === "cancel" || error === "close") return;
    ElMessage.error(error instanceof Error ? error.message : "企业操作失败");
  } finally {
    operating.value = false;
  }
}

function formatNumber(value: number) { return new Intl.NumberFormat("zh-CN").format(value || 0); }
function formatPoints(value: number) { return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value || 0); }
function formatTime(value?: string) { if (!value) return "-"; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(date); }
function expiryText(value?: string) { return value ? `有效期至 ${formatTime(value).slice(0, 10)}` : "长期有效"; }
function initials(name: string) { return String(name || "企").trim().slice(0, 2); }
function maskCreditCode(value?: string) { if (!value) return "未提交"; return value.length > 8 ? `${value.slice(0, 4)}********${value.slice(-4)}` : value; }
function sectionRowKey(row: Record<string, unknown>) { return String(row.id || row.requestId || row.enterpriseId || JSON.stringify(row)); }
function isStructuredValue(value: unknown) { return Array.isArray(value) || Boolean(value && typeof value === "object"); }
function formatSectionCell(value: unknown, key: string) {
  if (value === undefined || value === null || value === "") return "-";
  if (key === "amountCents" || key === "cashBalanceCents") return `¥${(Number(value) / 100).toLocaleString("zh-CN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  if (key === "pointDelta") { const amount = Number(value); return `${amount > 0 ? "+" : ""}${formatPoints(amount)} POINT`; }
  if (/At$/.test(key) || key === "expiresAt") return formatTime(String(value));
  if (Array.isArray(value)) return value.map(String).join("、") || "-";
  if (typeof value === "object") return JSON.stringify(value);
  if (typeof value === "number") return formatNumber(value);
  return String(value);
}

watch(() => [props.moduleId, props.routePath, props.permissions.join("|")], () => {
  loadError.value = "";
  if (!canAccessCurrentPage.value) return;
  if (isListPage.value) { hydrateFiltersFromUrl(); void loadList(); return; }
  if (isDetailPage.value) void loadDetail();
  else void loadSection();
}, { immediate: true });
</script>

<style scoped>
.enterprise-management {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  max-width: 100%;
}
.enterprise-management{min-height:calc(100vh - 160px);color:#181c28;font-family:"Noto Sans SC","Microsoft YaHei",sans-serif}.enterprise-page-head,.enterprise-detail-head{display:flex;align-items:flex-end;justify-content:space-between;gap:24px;margin-bottom:20px}.enterprise-page-head p{margin:0 0 6px;color:#4a6cff;font-size:11px;font-weight:800;letter-spacing:.14em}.enterprise-page-head h1,.enterprise-detail-head h1{margin:0;font-size:28px;line-height:1.3}.enterprise-page-head span,.enterprise-detail-head p{display:block;margin:7px 0 0;color:#737b8c;font-size:14px}.enterprise-page-actions{display:flex;gap:10px}.enterprise-stat-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:16px;margin-bottom:16px}.enterprise-stat-grid article{position:relative;overflow:hidden;padding:20px 22px;background:#fff;border:1px solid #e7eaf0;border-radius:10px;box-shadow:0 5px 18px rgba(30,45,80,.04)}.enterprise-stat-grid span,.enterprise-detail-stat-grid span{display:block;color:#737b8c;font-size:13px}.enterprise-stat-grid strong{display:block;margin:10px 0 5px;font-size:29px}.enterprise-stat-grid small{color:#9aa1af}.enterprise-stat-grid i{position:absolute;right:0;bottom:0;width:54px;height:4px;border-radius:4px 0 0 0}.enterprise-stat-grid .blue{background:#4a6cff}.enterprise-stat-grid .green{background:#20b26b}.enterprise-stat-grid .purple{background:#8b5cf6}.enterprise-stat-grid .red{background:#f04438}.enterprise-panel{background:#fff;border:1px solid #e7eaf0;border-radius:10px;box-shadow:0 5px 18px rgba(30,45,80,.035)}.enterprise-filter-panel{padding:16px 18px;margin-bottom:16px}.enterprise-filter-row{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.enterprise-filter-row+.enterprise-filter-row{margin-top:10px;padding-top:10px;border-top:1px dashed #e7eaf0}.enterprise-filter-row .keyword-input{width:250px}.enterprise-filter-row :deep(.el-select){width:145px}.enterprise-filter-row :deep(.el-date-editor){width:250px}.enterprise-filter-row.secondary .selection-summary{margin-left:auto;color:#4a6cff;font-weight:700}.enterprise-table-panel{overflow:hidden}.enterprise-state-alert{margin:18px}.enterprise-table-panel :deep(.el-table__row){cursor:pointer}.enterprise-name-cell{display:flex;align-items:center;gap:10px;padding:0;border:0;background:transparent;text-align:left;cursor:pointer}.enterprise-name-cell>span,.enterprise-detail-title>span{display:grid;place-items:center;flex:0 0 38px;height:38px;border-radius:9px;background:#eaf0ff;color:#4a6cff;font-size:13px;font-weight:800}.enterprise-name-cell i{display:grid;font-style:normal}.enterprise-name-cell small,.cell-subtext{display:block;margin-top:3px;color:#929aaa;font-size:12px}.enterprise-pagination{display:flex;align-items:center;justify-content:space-between;padding:15px 18px;border-top:1px solid #edf0f5;color:#737b8c;font-size:13px}.enterprise-detail-head{align-items:center}.enterprise-detail-head>.el-button{align-self:flex-start}.enterprise-detail-title{display:flex;align-items:center;gap:12px;flex:1}.enterprise-detail-title>span{flex-basis:48px;height:48px;font-size:15px}.enterprise-detail-title h1{font-size:24px}.enterprise-detail-title p{font-size:12px}.enterprise-detail-stat-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin:0 0 16px}.enterprise-detail-stat-grid article{padding:17px 19px;background:#fff;border:1px solid #e7eaf0;border-radius:10px}.enterprise-detail-stat-grid strong{display:block;margin:8px 0 4px;font-size:21px}.enterprise-detail-stat-grid small{color:#929aaa}.enterprise-detail-tabs{display:flex;gap:2px;margin-bottom:14px;padding:0 10px;background:#fff;border:1px solid #e7eaf0;border-radius:9px;overflow-x:auto}.enterprise-detail-tabs button{padding:14px 12px;border:0;border-bottom:2px solid transparent;background:transparent;color:#667085;white-space:nowrap;cursor:pointer}.enterprise-detail-tabs button.active{border-bottom-color:#4a6cff;color:#4a6cff;font-weight:700}.enterprise-privacy-banner{margin-bottom:16px}.enterprise-detail-grid{display:grid;grid-template-columns:minmax(0,1.4fr) minmax(300px,.8fr);gap:16px}.enterprise-detail-grid>.enterprise-panel{padding:20px}.enterprise-detail-grid section>header{display:flex;align-items:center;justify-content:space-between;padding-bottom:16px;border-bottom:1px solid #edf0f5}.enterprise-detail-grid h2{margin:0;font-size:17px}.enterprise-detail-grid header span{display:block;margin-top:5px;color:#929aaa;font-size:12px}.enterprise-profile-card{grid-row:span 2}.enterprise-profile-card dl{display:grid;grid-template-columns:1fr 1fr;gap:0;margin:8px 0 0}.enterprise-profile-card dl>div{padding:16px 0;border-bottom:1px solid #f0f2f5}.enterprise-profile-card dt{color:#8b93a2;font-size:12px}.enterprise-profile-card dd{margin:6px 0 0;font-weight:600}.enterprise-compute-balance{display:flex;align-items:baseline;gap:7px;margin:26px 0 14px}.enterprise-compute-balance span{margin-right:auto;color:#737b8c}.enterprise-compute-balance strong{font-size:28px}.enterprise-compute-balance small{color:#737b8c}.enterprise-meter{height:9px;background:#edf0f7;border-radius:6px;overflow:hidden}.enterprise-meter i{display:block;height:100%;background:linear-gradient(90deg,#4a6cff,#6f8cff);border-radius:6px}.enterprise-compute-legend{display:flex;justify-content:space-between;margin-top:9px;color:#737b8c;font-size:12px}.enterprise-audit-card :deep(.el-timeline){margin:20px 0 0;padding-left:5px}.enterprise-audit-card p{margin:5px 0;color:#8b93a2;font-size:12px}.enterprise-phase-card{padding:44px;text-align:center}.phase-badge{display:inline-flex;padding:6px 11px;border-radius:99px;background:#edf2ff;color:#4a6cff;font-size:12px;font-weight:800}.enterprise-phase-card h2{margin:16px 0 8px}.enterprise-phase-card>p{max-width:720px;margin:0 auto;color:#737b8c;line-height:1.8}.phase-state-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;max-width:760px;margin:30px auto 0;text-align:left}.phase-state-grid article{padding:16px;background:#f7f8fb;border:1px solid #e7eaf0;border-radius:8px}.phase-state-grid span{display:block;margin-bottom:7px;color:#8b93a2;font-size:12px}.phase-state-grid strong{font-size:13px}.enterprise-form-grid{display:grid;grid-template-columns:1fr 1fr;gap:0 16px}.enterprise-form-grid :deep(.el-select),.enterprise-form-grid :deep(.el-input-number){width:100%}@media(max-width:1100px){.enterprise-stat-grid,.enterprise-detail-stat-grid{grid-template-columns:repeat(2,1fr)}.enterprise-detail-grid{grid-template-columns:1fr}.enterprise-profile-card{grid-row:auto}}@media(max-width:700px){.enterprise-page-head,.enterprise-detail-head{align-items:flex-start;flex-direction:column}.enterprise-stat-grid,.enterprise-detail-stat-grid,.enterprise-form-grid,.phase-state-grid{grid-template-columns:1fr}.enterprise-filter-row>*{width:100%!important}.enterprise-detail-title{flex-wrap:wrap}.enterprise-profile-card dl{grid-template-columns:1fr}.enterprise-pagination{align-items:flex-start;flex-direction:column;gap:12px}}
.enterprise-section-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin-bottom:16px}.enterprise-section-summary article{padding:18px 20px;background:#fff;border:1px solid #e7eaf0;border-radius:10px}.enterprise-section-summary span{display:block;color:#737b8c;font-size:12px}.enterprise-section-summary strong{display:block;margin:8px 0 4px;font-size:22px}.enterprise-section-summary small{color:#9aa1af}.enterprise-section-panel{overflow:hidden}.enterprise-section-panel>header{display:flex;align-items:center;justify-content:space-between;padding:17px 20px;border-bottom:1px solid #edf0f5}.enterprise-section-panel h2{margin:0;font-size:17px}.enterprise-section-panel header span{display:block;margin-top:4px;color:#929aaa;font-size:12px}.enterprise-section-panel :deep(.el-empty){padding:50px 0}.enterprise-json-value{display:block;max-width:320px;overflow:hidden;color:#596174;font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:12px;text-overflow:ellipsis;white-space:nowrap}.enterprise-change-preview{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin:4px 0 16px}.enterprise-change-preview article{min-width:0;padding:13px;background:#f7f8fb;border:1px solid #e7eaf0;border-radius:8px}.enterprise-change-preview span{color:#737b8c;font-size:12px}.enterprise-change-preview pre{max-height:150px;margin:8px 0 0;overflow:auto;color:#333b4d;font-size:12px;white-space:pre-wrap;word-break:break-word}.enterprise-page-actions{flex-wrap:wrap}.enterprise-management :deep(.el-dialog .el-select),.enterprise-management :deep(.el-dialog .el-input-number),.enterprise-management :deep(.el-dialog .el-date-editor){width:100%}@media(max-width:1100px){.enterprise-section-summary{grid-template-columns:repeat(2,1fr)}}@media(max-width:700px){.enterprise-section-summary,.enterprise-change-preview{grid-template-columns:1fr}}
</style>
