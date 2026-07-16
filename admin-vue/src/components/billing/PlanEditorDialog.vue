<template>
  <el-dialog :model-value="modelValue" title="编辑套餐配置" width="860px" destroy-on-close @close="close">
    <el-alert
      v-if="isNewcomerPlan"
      type="success"
      :closable="false"
      show-icon
      title="这是新人体验套餐；发布后，新注册用户将按这里的点数、有效期和产品能力到账。"
      class="plan-editor-alert"
    />
    <el-tabs v-model="activeTab" class="plan-editor-tabs">
      <el-tab-pane label="基础配置" name="basic">
        <el-form label-position="top" class="plan-editor-form">
          <div class="plan-editor-grid">
            <el-form-item label="套餐名称">
              <el-input v-model="name" maxlength="40" show-word-limit />
            </el-form-item>
            <el-form-item label="套餐编码">
              <el-input :model-value="String(plan?.code || plan?.id || '')" disabled />
            </el-form-item>
            <el-form-item label="价格（元）">
              <el-input-number v-model="priceYuan" :min="0" :max="999999" :precision="2" :step="1" controls-position="right" />
            </el-form-item>
            <el-form-item label="到账点数">
              <el-input-number v-model="grantPoints" :min="0" :max="1000000000" :step="100" controls-position="right" />
            </el-form-item>
            <el-form-item label="有效期（天）">
              <el-input-number v-model="durationDays" :min="0" :max="36500" controls-position="right" />
              <small>填写 0 表示不设置套餐有效期。</small>
            </el-form-item>
            <el-form-item label="并发数">
              <el-input-number v-model="concurrency" :min="0" :max="10000" controls-position="right" />
            </el-form-item>
            <el-form-item label="展示价格">
              <el-input v-model="displayPrice" placeholder="例如：免费、29 元/月" />
            </el-form-item>
            <el-form-item label="有效期文案">
              <el-input v-model="validityText" placeholder="例如：7 天有效" />
            </el-form-item>
          </div>
          <el-form-item label="适用人群">
            <el-input v-model="audience" placeholder="例如：新用户体验、个人创作者" />
          </el-form-item>
          <el-form-item label="销售状态">
            <el-switch v-model="active" active-text="启用" inactive-text="停用" />
          </el-form-item>
          <el-form-item label="高级权益 JSON">
            <el-input v-model="entitlementsText" type="textarea" :rows="7" spellcheck="false" />
            <small>会保留现有扩展字段；展示价格、有效期文案和适用人群以表单值为准。</small>
          </el-form-item>
        </el-form>
      </el-tab-pane>

      <el-tab-pane label="产品能力" name="capabilities">
        <el-alert
          type="info"
          :closable="false"
          show-icon
          title="这里控制该套餐可调用的产品模块、模型和参数上限；保存后服务端会强制校验。"
          class="capability-help"
        />
        <el-skeleton v-if="capabilitiesLoading" :rows="6" animated />
        <el-empty v-else-if="!capabilityModules.length" description="暂无可配置的产品能力" />
        <div v-else class="capability-list">
          <article v-for="module in capabilityModules" :key="module.moduleCode" class="capability-card">
            <header>
              <div>
                <strong>{{ module.name || moduleLabel(module.moduleCode) }}</strong>
                <small>{{ module.description || module.moduleCode }}</small>
              </div>
              <el-switch v-model="module.enabled" active-text="套餐可用" inactive-text="套餐不可用" />
            </header>
            <div :class="['capability-body', { disabled: !module.enabled }]">
              <label>可用模型</label>
              <el-checkbox-group v-model="module.allowedModels">
                <el-checkbox v-for="model in module.availableModels" :key="model" :value="model">{{ model }}</el-checkbox>
              </el-checkbox-group>

              <div v-if="module.moduleCode === 'image_generation'" class="capability-limit-grid">
                <el-form-item label="单次最多图片数">
                  <el-input-number :model-value="limitNumber(module, 'n', 'max', 1)" :min="1" :max="100" @update:model-value="setLimitNumber(module, 'n', 'max', $event)" />
                </el-form-item>
                <el-form-item label="可用质量">
                  <el-checkbox-group :model-value="limitStrings(module, 'quality', 'allowed')" @update:model-value="setLimitStrings(module, 'quality', 'allowed', $event)">
                    <el-checkbox value="standard">标准</el-checkbox>
                    <el-checkbox value="high">高清</el-checkbox>
                  </el-checkbox-group>
                </el-form-item>
              </div>

              <div v-else-if="module.moduleCode === 'video_generation'" class="capability-limit-grid">
                <el-form-item label="最长视频（秒）">
                  <el-input-number :model-value="limitNumber(module, 'duration', 'max', 5)" :min="4" :max="300" @update:model-value="setLimitNumber(module, 'duration', 'max', $event)" />
                </el-form-item>
                <el-form-item label="可用分辨率">
                  <el-checkbox-group :model-value="limitStrings(module, 'resolution', 'allowed')" @update:model-value="setLimitStrings(module, 'resolution', 'allowed', $event)">
                    <el-checkbox v-for="resolution in videoResolutions" :key="resolution" :value="resolution">{{ resolution }}</el-checkbox>
                  </el-checkbox-group>
                </el-form-item>
              </div>

              <div v-else-if="module.moduleCode === 'ppt_generation'" class="capability-limit-grid capability-limit-grid--ppt">
                <el-form-item label="最多页数">
                  <el-input-number :model-value="limitNumber(module, 'page_count', 'max', 10)" :min="1" :max="200" @update:model-value="setLimitNumber(module, 'page_count', 'max', $event)" />
                </el-form-item>
                <el-form-item label="允许上传参考文档">
                  <el-switch :model-value="limitBoolean(module, 'uploaded_file', 'enabled', true)" @update:model-value="setLimitBoolean(module, 'uploaded_file', 'enabled', $event)" />
                </el-form-item>
                <el-form-item label="允许生成配图">
                  <el-switch :model-value="limitBoolean(module, 'with_images', 'enabled', true)" @update:model-value="setLimitBoolean(module, 'with_images', 'enabled', $event)" />
                </el-form-item>
              </div>
            </div>
          </article>
        </div>
      </el-tab-pane>
    </el-tabs>
    <template #footer>
      <el-button @click="close">取消</el-button>
      <el-button type="primary" :loading="saving || capabilitiesLoading" @click="submit">保存并立即生效</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { adminRequest } from "../../api/client";

type PlanRecord = Record<string, unknown>;
type CapabilityModule = {
  moduleCode: string;
  name: string;
  description: string;
  enabled: boolean;
  allowedModels: string[];
  availableModels: string[];
  limits: Record<string, unknown>;
};
type PlanSavePayload = {
  plan: PlanRecord;
  capabilities: { modules: CapabilityModule[] };
};

const props = defineProps<{
  modelValue: boolean;
  plan: PlanRecord | null;
  saving?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  save: [payload: PlanSavePayload];
}>();

const name = ref("");
const priceYuan = ref(0);
const grantPoints = ref(0);
const durationDays = ref(0);
const concurrency = ref(0);
const active = ref(true);
const displayPrice = ref("");
const validityText = ref("");
const audience = ref("");
const entitlementsText = ref("{}");
const activeTab = ref("basic");
const capabilitiesLoading = ref(false);
const capabilityModules = ref<CapabilityModule[]>([]);
const videoResolutions = ["480p", "720p", "1080p", "4k"];

const isNewcomerPlan = computed(() => props.plan?.id === "plan_free" || String(props.plan?.code || "").toLowerCase() === "trial");

watch(
  () => [props.modelValue, props.plan] as const,
  ([visible, plan]) => {
    if (!visible || !plan) return;
    const entitlements = plan.entitlements && typeof plan.entitlements === "object"
      ? { ...(plan.entitlements as Record<string, unknown>) }
      : {};
    activeTab.value = "basic";
    name.value = String(plan.name || "");
    priceYuan.value = Number(plan.priceCents || 0) / 100;
    grantPoints.value = Number(plan.grantPoints || 0);
    durationDays.value = Number(plan.durationDays || 0);
    concurrency.value = Number(plan.concurrency || 0);
    active.value = Boolean(plan.active);
    displayPrice.value = String(entitlements.displayPrice || "");
    validityText.value = String(entitlements.validityText || "");
    audience.value = String(entitlements.audience || "");
    entitlementsText.value = JSON.stringify(entitlements, null, 2);
    void loadCapabilities(String(plan.id || ""));
  },
  { immediate: true }
);

async function loadCapabilities(planId: string) {
  capabilityModules.value = [];
  if (!planId) return;
  capabilitiesLoading.value = true;
  try {
    const response = await adminRequest<{ items?: CapabilityModule[] }>({ url: `/admin/plans/${planId}/capabilities` });
    capabilityModules.value = Array.isArray(response.items)
      ? response.items.map((item) => ({
        ...item,
        allowedModels: Array.isArray(item.allowedModels) ? [...item.allowedModels] : [],
        availableModels: Array.isArray(item.availableModels) ? [...item.availableModels] : [],
        limits: item.limits && typeof item.limits === "object" ? structuredClone(item.limits) : {}
      }))
      : [];
  } catch (error) {
    ElMessage.error(error instanceof Error ? `产品能力加载失败：${error.message}` : "产品能力加载失败");
  } finally {
    capabilitiesLoading.value = false;
  }
}

function moduleLabel(code: string) {
  return ({ image_generation: "AI 生图", video_generation: "视频生成", ppt_generation: "PPT 文档生成" } as Record<string, string>)[code] || code;
}

function limitRule(module: CapabilityModule, key: string) {
  const current = module.limits[key];
  if (current && typeof current === "object" && !Array.isArray(current)) return current as Record<string, unknown>;
  const created: Record<string, unknown> = {};
  module.limits[key] = created;
  return created;
}

function limitNumber(module: CapabilityModule, key: string, field: string, fallback: number) {
  const value = Number(limitRule(module, key)[field]);
  return Number.isFinite(value) && value > 0 ? value : fallback;
}

function setLimitNumber(module: CapabilityModule, key: string, field: string, value: number | undefined) {
  limitRule(module, key)[field] = Number(value || 0);
}

function limitStrings(module: CapabilityModule, key: string, field: string) {
  const value = limitRule(module, key)[field];
  return Array.isArray(value) ? value.map((item) => String(item)) : [];
}

function setLimitStrings(module: CapabilityModule, key: string, field: string, value: unknown) {
  limitRule(module, key)[field] = Array.isArray(value) ? value.map((item) => String(item)) : [];
}

function limitBoolean(module: CapabilityModule, key: string, field: string, fallback: boolean) {
  const value = limitRule(module, key)[field];
  return typeof value === "boolean" ? value : fallback;
}

function setLimitBoolean(module: CapabilityModule, key: string, field: string, value: boolean) {
  limitRule(module, key)[field] = value;
}

function close() {
  if (!props.saving && !capabilitiesLoading.value) emit("update:modelValue", false);
}

function submit() {
  if (!name.value.trim()) {
    activeTab.value = "basic";
    ElMessage.warning("请填写套餐名称");
    return;
  }
  const invalidModule = capabilityModules.value.find((module) => module.enabled && module.availableModels.length > 0 && module.allowedModels.length === 0);
  if (invalidModule) {
    activeTab.value = "capabilities";
    ElMessage.warning(`${invalidModule.name || moduleLabel(invalidModule.moduleCode)}至少选择一个可用模型`);
    return;
  }
  const emptyAllowedLimit = capabilityModules.value.find((module) => {
    if (!module.enabled) return false;
    if (module.moduleCode === "image_generation") return limitStrings(module, "quality", "allowed").length === 0;
    if (module.moduleCode === "video_generation") return limitStrings(module, "resolution", "allowed").length === 0;
    return false;
  });
  if (emptyAllowedLimit) {
    activeTab.value = "capabilities";
    const label = emptyAllowedLimit.moduleCode === "image_generation" ? "图片质量" : "视频分辨率";
    ElMessage.warning(`${label}至少选择一项；如需停用，请关闭整个产品能力`);
    return;
  }
  let entitlements: Record<string, unknown>;
  try {
    const parsed = JSON.parse(entitlementsText.value || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("权益配置必须是 JSON 对象");
    entitlements = parsed as Record<string, unknown>;
  } catch (error) {
    activeTab.value = "basic";
    ElMessage.error(error instanceof Error ? error.message : "高级权益 JSON 格式不正确");
    return;
  }
  entitlements.displayPrice = displayPrice.value.trim();
  entitlements.validityText = validityText.value.trim();
  entitlements.audience = audience.value.trim();
  emit("save", {
    plan: {
      name: name.value.trim(),
      priceCents: Math.round(priceYuan.value * 100),
      grantPoints: Math.round(grantPoints.value),
      durationDays: Math.round(durationDays.value),
      concurrency: Math.round(concurrency.value),
      active: active.value,
      entitlements
    },
    capabilities: {
      modules: capabilityModules.value.map((module) => ({
        ...module,
        allowedModels: [...module.allowedModels],
        availableModels: [...module.availableModels],
        limits: structuredClone(module.limits)
      }))
    }
  });
}
</script>

<style scoped>
.plan-editor-alert { margin-bottom: 12px; }
.plan-editor-tabs { min-height: 520px; }
.plan-editor-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 18px; }
.plan-editor-form :deep(.el-input-number), .capability-card :deep(.el-input-number) { width: 100%; }
.plan-editor-form small { display: block; margin-top: 6px; color: var(--el-text-color-secondary); line-height: 1.5; }
.capability-help { margin-bottom: 14px; }
.capability-list { display: grid; gap: 14px; max-height: 470px; padding-right: 4px; overflow: auto; }
.capability-card { padding: 16px; border: 1px solid var(--el-border-color-lighter); border-radius: 10px; background: var(--el-fill-color-blank); }
.capability-card > header { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; }
.capability-card > header div { display: grid; gap: 4px; }
.capability-card > header strong { font-size: 16px; }
.capability-card > header small { color: var(--el-text-color-secondary); line-height: 1.45; }
.capability-body { margin-top: 14px; transition: opacity .2s; }
.capability-body.disabled { opacity: .52; }
.capability-body > label { display: block; margin-bottom: 8px; color: var(--el-text-color-regular); font-size: 13px; }
.capability-limit-grid { display: grid; grid-template-columns: 200px minmax(0, 1fr); gap: 0 22px; margin-top: 14px; }
.capability-limit-grid--ppt { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.capability-limit-grid :deep(.el-form-item) { margin-bottom: 0; }
@media (max-width: 720px) {
  .plan-editor-grid, .capability-limit-grid, .capability-limit-grid--ppt { grid-template-columns: 1fr; }
}
</style>
