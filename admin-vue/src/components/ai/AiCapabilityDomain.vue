<template>
  <section class="ai-capability-page">
    <section class="ai-capability-head">
      <div>
        <el-tag effect="dark" type="success">AI Capability Center</el-tag>
        <h2>AI 能力中心</h2>
        <p>统一配置 image_generation、video_generation、ppt_generation 的模块开关、模型绑定、参数 Schema、租户限制和调用链路。</p>
      </div>
      <div class="ai-capability-actions">
        <el-button :icon="Refresh" :loading="model.loading" @click="model.refresh">刷新配置</el-button>
        <el-button :icon="Setting" @click="model.navigate('apiSettings')">上游 API</el-button>
      </div>
    </section>

    <div class="ai-capability-kpis">
      <article v-for="metric in model.metrics" :key="metric.label">
        <span>{{ metric.label }}</span><strong>{{ metric.value }}</strong><small>{{ metric.hint }}</small>
      </article>
    </div>

    <section v-if="model.activeModuleId === 'aiCapabilities'" class="ai-capability-section">
      <div v-if="model.modules.length" class="ai-capability-module-grid">
        <article v-for="module in model.modules" :key="String(module.id || model.text(module, 'module_code', 'moduleCode'))" class="ai-capability-module-card">
          <header>
            <div><el-tag :type="model.statusType(module.status)" size="small">{{ model.statusLabel(module.status) }}</el-tag><h3>{{ module.name || module.id }}</h3></div>
            <el-switch :model-value="model.isActiveStatus(module.status)" :loading="model.saving" active-text="启用" inactive-text="停用" @change="model.toggleModule(module)" />
          </header>
          <p>{{ module.description || '-' }}</p>
          <dl>
            <div><dt>module_code</dt><dd><code>{{ model.text(module, 'module_code', 'moduleCode') }}</code></dd></div>
            <div><dt>默认 Schema</dt><dd>{{ model.text(module, 'default_schema_id', 'defaultSchemaId') || '-' }}</dd></div>
            <div><dt>开放套餐</dt><dd>{{ model.list(module, 'open_package_ids', 'openPackageIds').join('、') || '-' }}</dd></div>
            <div><dt>开放对象</dt><dd>{{ model.audienceLabel(module) }}</dd></div>
          </dl>
          <div class="ai-capability-chip-list"><el-tag v-for="item in model.list(module, 'bound_models', 'boundModels')" :key="item" size="small" effect="plain">{{ item }}</el-tag></div>
          <footer><el-button size="small" @click="model.editModulePackages(module)">编辑套餐</el-button><el-button size="small" type="primary" plain @click="model.editModuleModels(module)">绑定模型</el-button></footer>
        </article>
      </div>
      <el-empty v-else description="暂无 AI 能力模块" />
    </section>

    <section v-if="model.activeModuleId === 'aiCapabilityModels'" class="ai-capability-section">
      <div class="ai-capability-section-head"><div><h3>模型管理</h3><p>新增或编辑 AI 生图、视频生成、PPT 文档生成可调用模型。</p></div><el-button type="primary" :icon="Plus" @click="model.createModel">新增模型</el-button></div>
      <el-table v-if="model.models.length" :data="model.models" height="430" stripe>
        <el-table-column label="模型" min-width="180"><template #default="scope"><div class="ai-capability-main-cell"><strong>{{ model.text(scope.row, 'model_name', 'modelName') }}</strong><small>{{ scope.row.id }}</small></div></template></el-table-column>
        <el-table-column label="模块" min-width="150"><template #default="scope"><el-tag effect="plain">{{ model.moduleLabel(model.text(scope.row, 'module_code', 'moduleCode')) }}</el-tag></template></el-table-column>
        <el-table-column prop="provider" label="上游" min-width="120" />
        <el-table-column label="小程序合规" min-width="160"><template #default="scope"><div class="ai-capability-main-cell"><el-tag :type="scope.row.miniprogram_enabled && scope.row.compliance_status === 'approved' ? 'success' : 'info'">{{ scope.row.miniprogram_enabled && scope.row.compliance_status === 'approved' ? '可用' : '未开放' }}</el-tag><small>{{ scope.row.algorithm_filing_no || '备案号待配置' }}</small></div></template></el-table-column>
        <el-table-column label="类型" width="110"><template #default="scope">{{ model.text(scope.row, 'model_type', 'modelType') || '-' }}</template></el-table-column>
        <el-table-column label="能力" min-width="260"><template #default="scope"><div class="ai-capability-chip-list"><el-tag v-for="item in model.list(scope.row, 'capability_code', 'capabilityCode')" :key="item" size="small">{{ item }}</el-tag></div></template></el-table-column>
        <el-table-column label="Fallback" min-width="140"><template #default="scope">{{ model.text(scope.row, 'fallback_model', 'fallbackModel') || '-' }}</template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="scope"><el-tag :type="model.statusType(scope.row.status)">{{ model.statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="操作" fixed="right" width="210"><template #default="scope"><el-button link type="primary" @click="model.editModel(scope.row)">编辑</el-button><el-button link type="primary" @click="model.editModelCapabilities(scope.row)">能力</el-button><el-button link :type="model.isActiveStatus(scope.row.status) ? 'danger' : 'success'" @click="model.toggleModel(scope.row)">{{ model.isActiveStatus(scope.row.status) ? '停用' : '启用' }}</el-button></template></el-table-column>
      </el-table>
      <el-empty v-else description="暂无模型配置" />
    </section>

    <section v-if="model.activeModuleId === 'aiCapabilitySchemas'" class="ai-capability-section">
      <el-table v-if="model.schemas.length" :data="model.schemas" height="430" stripe>
        <el-table-column label="Schema" min-width="220"><template #default="scope"><div class="ai-capability-main-cell"><strong>{{ scope.row.id }}</strong><small>{{ model.text(scope.row, 'model_name', 'modelName') || '默认模型' }}</small></div></template></el-table-column>
        <el-table-column label="模块" min-width="150"><template #default="scope">{{ model.moduleLabel(model.text(scope.row, 'module_code', 'moduleCode')) }}</template></el-table-column>
        <el-table-column label="字段与选项" min-width="520"><template #default="scope"><div class="ai-schema-field-list"><span v-for="field in model.schemaFields(scope.row).slice(0, 8)" :key="String(field.key || field.label)" class="ai-schema-field-chip"><strong>{{ model.schemaFieldLabel(field) }}</strong><small v-if="model.schemaFieldOptionsText(field)">{{ model.schemaFieldOptionsText(field) }}</small></span><span v-if="model.schemaFields(scope.row).length > 8" class="ai-capability-more">+{{ model.schemaFields(scope.row).length - 8 }}</span></div></template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="scope"><el-tag :type="model.statusType(scope.row.status)">{{ model.statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="操作" fixed="right" width="170"><template #default="scope"><el-button link type="primary" @click="model.editSchema(scope.row)">编辑</el-button><el-button link :type="model.isActiveStatus(scope.row.status) ? 'danger' : 'success'" @click="model.toggleSchema(scope.row)">{{ model.isActiveStatus(scope.row.status) ? '停用' : '启用' }}</el-button></template></el-table-column>
      </el-table>
      <el-empty v-else description="暂无参数 Schema" />
    </section>

    <section v-if="model.activeModuleId === 'aiCapabilityLimits'" class="ai-capability-section">
      <el-table v-if="model.limits.length" :data="model.limits" height="430" stripe>
        <el-table-column label="适用范围" min-width="190"><template #default="scope"><div class="ai-capability-main-cell"><strong>{{ model.limitScope(scope.row) }}</strong><small>{{ scope.row.id }}</small></div></template></el-table-column>
        <el-table-column label="模块" min-width="150"><template #default="scope">{{ model.moduleLabel(model.text(scope.row, 'module_code', 'moduleCode')) }}</template></el-table-column>
        <el-table-column label="模型" min-width="150"><template #default="scope">{{ model.text(scope.row, 'model_name', 'modelName') || '模块默认' }}</template></el-table-column>
        <el-table-column label="限制项" min-width="300"><template #default="scope"><code class="ai-capability-json-preview">{{ model.jsonPreview(model.object(scope.row, 'limit_json', 'limitJson')) }}</code></template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="scope"><el-tag :type="model.statusType(scope.row.status)">{{ model.statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="操作" fixed="right" width="170"><template #default="scope"><el-button link type="primary" @click="model.editLimit(scope.row)">JSON</el-button><el-button link :type="model.isActiveStatus(scope.row.status) ? 'danger' : 'success'" @click="model.toggleLimit(scope.row)">{{ model.isActiveStatus(scope.row.status) ? '停用' : '启用' }}</el-button></template></el-table-column>
      </el-table>
      <el-empty v-else description="暂无租户限制" />
    </section>

    <section v-if="model.activeModuleId === 'aiCapabilityChannels'" class="ai-capability-section">
      <el-table v-if="model.channels.length" :data="model.channels" height="380" stripe>
        <el-table-column prop="name" label="通道" min-width="170" /><el-table-column prop="protocol" label="协议" width="120" /><el-table-column prop="baseUrl" label="Base URL" min-width="260" show-overflow-tooltip />
        <el-table-column label="模型" min-width="260"><template #default="scope"><div class="ai-capability-chip-list"><el-tag v-for="item in model.list(scope.row, 'models')" :key="item" size="small" effect="plain">{{ item }}</el-tag></div></template></el-table-column>
        <el-table-column label="Key" width="110"><template #default="scope"><el-tag :type="scope.row.apiKeyConfigured ? 'success' : 'warning'">{{ scope.row.apiKeyConfigured ? '已配置' : '待配置' }}</el-tag></template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="scope"><el-tag :type="model.statusType(scope.row.status)">{{ model.statusLabel(scope.row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="操作" fixed="right" width="110"><template #default><el-button link type="primary" @click="model.navigate('apiSettings')">配置</el-button></template></el-table-column>
      </el-table>
      <el-empty v-else description="暂无上游通道" />
    </section>

    <section v-if="model.activeModuleId === 'aiCapabilityLogs'" class="ai-capability-section">
      <el-table v-if="model.logs.length" :data="model.logs" height="430" stripe>
        <el-table-column prop="id" label="任务" min-width="150" show-overflow-tooltip />
        <el-table-column label="模块" min-width="140"><template #default="scope">{{ model.moduleLabel(model.text(scope.row, 'module_code', 'moduleCode')) }}</template></el-table-column>
        <el-table-column prop="model_name" label="模型" min-width="150" /><el-table-column prop="user" label="用户" min-width="130" />
        <el-table-column label="扣费" width="110"><template #default="scope">{{ model.moneyYuan(scope.row.user_charge_amount || scope.row.amountCents || 0) }}</template></el-table-column>
        <el-table-column label="成本" width="110"><template #default="scope">{{ model.moneyYuan(scope.row.upstream_cost || 0) }}</template></el-table-column>
        <el-table-column label="利润" width="110"><template #default="scope">{{ model.moneyYuan(scope.row.platform_profit || 0) }}</template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="scope"><el-tag :type="model.statusType(scope.row.task_status)">{{ model.statusLabel(scope.row.task_status) }}</el-tag></template></el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="190" show-overflow-tooltip />
      </el-table>
      <el-empty v-else description="暂无调用日志" />
    </section>
  </section>
</template>

<script setup lang="ts">
import { Plus, Refresh, Setting } from "@element-plus/icons-vue";

type RecordValue = Record<string, any>;
type Action = (record: RecordValue) => unknown;

defineProps<{ model: {
  activeModuleId: string; loading: boolean; saving: boolean;
  metrics: Array<{ label: string; value: string; hint: string }>;
  modules: RecordValue[]; models: RecordValue[]; schemas: RecordValue[]; limits: RecordValue[]; channels: RecordValue[]; logs: RecordValue[];
  refresh: () => unknown; navigate: (moduleId: string) => unknown;
  text: (record: RecordValue, ...keys: string[]) => string; list: (record: RecordValue, ...keys: string[]) => string[]; object: (record: RecordValue, ...keys: string[]) => RecordValue;
  audienceLabel: (record: RecordValue) => string; moduleLabel: (value: string) => string; limitScope: (record: RecordValue) => string; jsonPreview: (value: RecordValue) => string;
  schemaFields: (record: RecordValue) => RecordValue[]; schemaFieldLabel: (field: RecordValue) => string; schemaFieldOptionsText: (field: RecordValue) => string;
  statusType: (value: unknown) => any; statusLabel: (value: unknown) => string; isActiveStatus: (value: unknown) => boolean; moneyYuan: (value: unknown) => string;
  toggleModule: Action; editModulePackages: Action; editModuleModels: Action; createModel: () => unknown; editModel: Action; editModelCapabilities: Action; toggleModel: Action;
  editSchema: Action; toggleSchema: Action; editLimit: Action; toggleLimit: Action;
} }>();
</script>
