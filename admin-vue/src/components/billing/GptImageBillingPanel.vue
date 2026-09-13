<template>
  <div class="gpt-image-panel">
    <!-- 当前正式版看板 -->
    <el-card shadow="never" class="gpt-image-panel__board">
      <template #header>
        <div class="board-header">
          <div class="board-header__info">
            <div class="board-header__title">
              <span class="board-header__badge">专属配置</span>
              <h3>GPT Image 计费规则 (gpt-image-2)</h3>
              <el-tag :type="publishedStatusTagType" effect="dark" size="small">
                {{ currentPublishedRule ? `v${currentPublishedRule.version} 正式版` : "未发布正式版" }}
              </el-tag>
              <el-tag v-if="hasNegativeMarginRisk" type="warning" effect="plain" size="small">
                存在负毛利商业风险
              </el-tag>
              <el-tag v-if="activeDraftRule" type="info" effect="plain" size="small">
                待发布草稿 v{{ activeDraftRule.version }}
              </el-tag>
            </div>
            <p class="board-header__desc">
              规范尺寸阶梯 (1K/2K/4K) 与画质倍率 (low/normal/high) 结构化定价，避免原始 JSON 误配导致的计费故障与商业损失。
            </p>
          </div>
          <div class="board-header__actions">
            <el-button :icon="Clock" @click="historyDrawerVisible = true">版本历史 ({{ gptImageVersions.length }})</el-button>
            <el-button type="primary" :icon="Edit" @click="openEditor">
              {{ activeDraftRule ? `继续编辑草稿 v${activeDraftRule.version}` : "配置价格 (新建草稿)" }}
            </el-button>
          </div>
        </div>
      </template>

      <!-- 核心指标栅格 -->
      <div class="board-metrics">
        <div class="metric-card">
          <span class="metric-card__label">基础售价 (Base Price)</span>
          <strong class="metric-card__value">{{ currentPublishedRule ? currentPublishedRule.basePrice : "-" }} <small>积分/张</small></strong>
          <span class="metric-card__hint">折算约 {{ formatRevenue(currentPublishedRule?.basePrice) }} CNY / 张</span>
        </div>

        <div class="metric-card">
          <span class="metric-card__label">供应商成本 (Provider Cost)</span>
          <strong class="metric-card__value">{{ formatCost(matchingProviderCost?.unitCost) }} <small>CNY/张</small></strong>
          <span class="metric-card__hint">通道: {{ matchingProviderCost?.channel || "channel_openai" }}</span>
        </div>

        <div class="metric-card">
          <span class="metric-card__label">单张基础毛利 (Margin)</span>
          <strong :class="['metric-card__value', marginDiff < 0 ? 'text-danger' : 'text-success']">
            {{ formatMargin(currentPublishedRule?.basePrice, matchingProviderCost?.unitCost || 0.60) }}
          </strong>
          <span class="metric-card__hint">{{ marginDiff < 0 ? "售价低于成本 (需确认放行)" : "正常正向毛利" }}</span>
        </div>

        <div class="metric-card metric-card--multipliers">
          <span class="metric-card__label">尺寸阶梯倍率 (Size Tiers)</span>
          <div class="multipliers-row">
            <span><strong>1K:</strong> {{ currentMultipliers.tier1k }}x</span>
            <span><strong>2K:</strong> {{ currentMultipliers.tier2k }}x</span>
            <span><strong>4K:</strong> {{ currentMultipliers.tier4k }}x</span>
          </div>
          <span class="metric-card__hint">权威尺寸计费阶梯</span>
        </div>

        <div class="metric-card metric-card--multipliers">
          <span class="metric-card__label">画质等级倍率 (Quality Tiers)</span>
          <div class="multipliers-row">
            <span><strong>low:</strong> {{ currentMultipliers.qualityLow }}x</span>
            <span><strong>normal:</strong> {{ currentMultipliers.qualityNormal }}x</span>
            <span><strong>high:</strong> {{ currentMultipliers.qualityHigh }}x</span>
          </div>
          <span class="metric-card__hint">medium 自动对齐 normal</span>
        </div>

        <div class="metric-card">
          <span class="metric-card__label">生效时间 / 校验状态</span>
          <div class="metric-status-row">
            <span class="metric-status-row__date">{{ dateTime(currentPublishedRule?.publishedAt || currentPublishedRule?.effectiveFrom) }}</span>
            <el-tag :type="publishedValidationTagType" size="small">
              {{ publishedValidationText }}
            </el-tag>
          </div>
          <span class="metric-card__hint">更新时间: {{ dateTime(currentPublishedRule?.updatedAt) }}</span>
        </div>
      </div>
    </el-card>

    <!-- 结构化配置弹窗 -->
    <el-dialog
      v-model="editorVisible"
      :title="`配置 GPT Image 计费规则 (${activeDraftRule ? `编辑草稿 v${activeDraftRule.version}` : '新建草稿'})`"
      width="920px"
      destroy-on-close
      class="gpt-image-dialog"
    >
      <div class="editor-layout">
        <!-- 左侧编辑表单 -->
        <div class="editor-form-pane">
          <h4>参数配置</h4>
          <el-form label-width="120px" label-position="left">
            <el-form-item label="计费模型">
              <el-input value="GPT Image 2 (gpt-image-2)" disabled />
            </el-form-item>

            <el-form-item label="基础售价">
              <el-input-number
                v-model="editorForm.basePrice"
                :min="1"
                :step="1"
                :precision="0"
                style="width: 220px;"
              />
              <span class="unit-suffix">积分 / 张 (1积分 = 0.01 CNY)</span>
            </el-form-item>

            <el-form-item label="最低扣费">
              <el-input-number
                v-model="editorForm.minimumCharge"
                :min="1"
                :step="1"
                :precision="0"
                style="width: 220px;"
              />
              <span class="unit-suffix">积分</span>
            </el-form-item>

            <el-divider content-position="left">尺寸阶梯倍率 (Size Multipliers)</el-divider>

            <el-form-item label="1K 尺寸 (tier_1k)">
              <el-input-number
                v-model="editorForm.tier1k"
                :min="0.1"
                :step="0.1"
                :precision="2"
                style="width: 220px;"
              />
              <span class="unit-suffix">倍 (基准 1.0x, 1024x1024等)</span>
            </el-form-item>

            <el-form-item label="2K 尺寸 (tier_2k)">
              <el-input-number
                v-model="editorForm.tier2k"
                :min="0.1"
                :step="0.1"
                :precision="2"
                style="width: 220px;"
              />
              <span class="unit-suffix">倍 (如 1.5x, 2048x2048等)</span>
            </el-form-item>

            <el-form-item label="4K 尺寸 (tier_4k)">
              <el-input-number
                v-model="editorForm.tier4k"
                :min="0.1"
                :step="0.1"
                :precision="2"
                style="width: 220px;"
              />
              <span class="unit-suffix">倍 (如 2.0x, 3840x2160等)</span>
            </el-form-item>

            <el-divider content-position="left">画质等级倍率 (Quality Multipliers)</el-divider>

            <el-form-item label="低画质 (low)">
              <el-input-number
                v-model="editorForm.qualityLow"
                :min="0.1"
                :step="0.1"
                :precision="2"
                style="width: 220px;"
              />
              <span class="unit-suffix">倍 (基准 1.0x)</span>
            </el-form-item>

            <el-form-item label="标准画质 (normal)">
              <el-input-number
                v-model="editorForm.qualityNormal"
                :min="0.1"
                :step="0.1"
                :precision="2"
                style="width: 220px;"
              />
              <span class="unit-suffix">倍 (自动对齐 medium)</span>
            </el-form-item>

            <el-form-item label="高画质 (high)">
              <el-input-number
                v-model="editorForm.qualityHigh"
                :min="0.1"
                :step="0.1"
                :precision="2"
                style="width: 220px;"
              />
              <span class="unit-suffix">倍 (如 1.5x)</span>
            </el-form-item>
          </el-form>

          <el-collapse v-model="jsonPreviewActive" style="margin-top: 12px;">
            <el-collapse-item title="查看提交参数 JSON 结构 (高级视图)" name="jsonPreview">
              <pre class="json-preview-box">{{ generatedJsonPreview }}</pre>
            </el-collapse-item>
          </el-collapse>
        </div>

        <!-- 右侧实时试算预览卡片 -->
        <div class="editor-preview-pane">
          <div class="preview-card">
            <h4>实时试算价格预览</h4>
            <p class="preview-card__desc">基于当前设定的售价与倍率即时换算各规格所需点数：</p>

            <el-table :data="quotePreviewRows" size="small" border class="preview-table">
              <el-table-column prop="tierName" label="规格阶梯" min-width="140" />
              <el-table-column label="Low (低画质)" align="right" min-width="90">
                <template #default="s"><span class="price-val">{{ s?.row?.lowPoints ?? "-" }} 积分</span></template>
              </el-table-column>
              <el-table-column label="Normal (普通)" align="right" min-width="90">
                <template #default="s"><span class="price-val">{{ s?.row?.normalPoints ?? "-" }} 积分</span></template>
              </el-table-column>
              <el-table-column label="High (高画质)" align="right" min-width="90">
                <template #default="s"><span class="price-val">{{ s?.row?.highPoints ?? "-" }} 积分</span></template>
              </el-table-column>
            </el-table>

            <div class="preview-cost-summary">
              <div class="cost-item">
                <span>基础折算收入:</span>
                <strong>{{ formatRevenue(editorForm.basePrice) }} CNY / 张</strong>
              </div>
              <div class="cost-item">
                <span>供应商成本:</span>
                <strong>{{ formatCost(matchingProviderCost?.unitCost) }} CNY / 张</strong>
              </div>
              <div class="cost-item">
                <span>基础毛利预估:</span>
                <strong :class="previewMarginDiff < 0 ? 'text-danger' : 'text-success'">
                  {{ formatMargin(editorForm.basePrice, matchingProviderCost?.unitCost || 0.60) }}
                </strong>
              </div>
            </div>

            <el-alert
              v-if="previewMarginDiff < 0"
              type="warning"
              :closable="false"
              show-icon
              title="当前价格属于负毛利商业风险"
              description="基础售价折算低于供应商成本 0.60 CNY。发布上线时将触发商业风险提示，需要您二次明确确认后才可放行。"
              style="margin-top: 14px;"
            />

            <div class="preview-note">
              <el-icon><InfoFilled /></el-icon>
              <span>注意：此表格为 UI 实时计算预览，最终扣费点数以规则发布后服务端的官方 Quote 接口为准。</span>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="dialog-footer">
          <el-button @click="editorVisible = false">取消</el-button>
          <el-button :loading="savingDraft" @click="handleSaveDraft">保存为草稿</el-button>
          <el-button type="primary" :loading="publishing" @click="handlePublish">发布上线</el-button>
        </div>
      </template>
    </el-dialog>

    <!-- 负毛利二次确认弹窗 -->
    <el-dialog
      v-model="negativeMarginDialogVisible"
      title="⚠️ 负毛利商业风险确认"
      width="540px"
      destroy-on-close
    >
      <div class="negative-margin-content">
        <el-alert
          type="error"
          :closable="false"
          show-icon
          title="检测到当前发布版本存在负毛利风险"
          description="该规则基础售价折算金额低于供应商上游成本。上线后该模型的每次生成将对平台产生亏损补贴。"
        />

        <div class="negative-margin-facts">
          <div class="fact-row">
            <span>规则版本:</span>
            <strong>v{{ targetDraftVersion }} ({{ targetDraftId }})</strong>
          </div>
          <div class="fact-row">
            <span>基础售价折算收入:</span>
            <strong class="text-danger">{{ formatRevenue(targetDraftBasePrice) }} CNY / 张</strong>
          </div>
          <div class="fact-row">
            <span>上游供应商成本:</span>
            <strong>{{ formatCost(matchingProviderCost?.unitCost) }} CNY / 张</strong>
          </div>
          <div class="fact-row">
            <span>单张毛利亏损:</span>
            <strong class="text-danger">{{ formatMargin(targetDraftBasePrice, matchingProviderCost?.unitCost || 0.60) }}</strong>
          </div>
        </div>

        <p class="negative-margin-tip">
          如果这是您明确认可的平台运营补贴策略，请点击下方<strong>“确认负毛利并发布”</strong>，系统将记录管理员确认审计并正式发布上线；否则请取消并在草稿中调整售价。
        </p>
      </div>

      <template #footer>
        <el-button @click="negativeMarginDialogVisible = false">取消发布</el-button>
        <el-button
          type="danger"
          :loading="publishingWithOverride"
          @click="confirmAndPublishNegativeMargin"
        >
          确认负毛利并发布上线
        </el-button>
      </template>
    </el-dialog>

    <!-- 版本历史抽屉 -->
    <el-drawer
      v-model="historyDrawerVisible"
      title="GPT Image 计费规则版本历史"
      size="760px"
      destroy-on-close
    >
      <div class="history-container">
        <el-table :data="gptImageVersions" stripe empty-text="暂无历史版本">
          <el-table-column prop="version" label="版本" width="80">
            <template #default="s"><strong>v{{ s?.row?.version }}</strong></template>
          </el-table-column>
          <el-table-column prop="status" label="状态" width="100">
            <template #default="s">
              <el-tag :type="statusTagType(s?.row?.status)" size="small">
                {{ s?.row?.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="basePrice" label="基础售价" width="110" align="right">
            <template #default="s">{{ s?.row?.basePrice }} 积分</template>
          </el-table-column>
          <el-table-column label="关键倍率" min-width="200">
            <template #default="s">
              <div class="history-multipliers">
                <span>1K/2K/4K: {{ getTierMultipliersSummary(s?.row?.parameterRules) }}</span>
                <span>low/normal/high: {{ getQualityMultipliersSummary(s?.row?.parameterRules) }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column prop="effectiveFrom" label="生效时间" min-width="150">
            <template #default="s">{{ dateTime(s?.row?.effectiveFrom || s?.row?.publishedAt) }}</template>
          </el-table-column>
          <el-table-column label="校验结果" width="110">
            <template #default="s">
              <el-tag
                :type="s?.row?.validationResult?.valid ? 'success' : hasIssueCode(s?.row, 'NEGATIVE_MARGIN') ? 'warning' : 'danger'"
                size="small"
              >
                {{ s?.row?.validationResult?.valid ? '正常' : hasIssueCode(s?.row, 'NEGATIVE_MARGIN') ? '负毛利' : '有错误' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="s">
              <el-button link type="primary" size="small" @click="viewVersionDetail(s?.row)">详情</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>

    <!-- 版本快照详情弹窗 -->
    <el-dialog
      v-model="detailDialogVisible"
      :title="`版本快照详情: ${selectedVersionItem?.id || ''} (v${selectedVersionItem?.version || ''})`"
      width="640px"
      destroy-on-close
    >
      <div v-if="selectedVersionItem" class="version-detail-content">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="版本号">v{{ selectedVersionItem.version }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ selectedVersionItem.status }}</el-descriptions-item>
          <el-descriptions-item label="基础售价">{{ selectedVersionItem.basePrice }} 积分/张</el-descriptions-item>
          <el-descriptions-item label="最低扣费">{{ selectedVersionItem.minimumCharge }} 积分</el-descriptions-item>
          <el-descriptions-item label="生效时间">{{ dateTime(selectedVersionItem.effectiveFrom) }}</el-descriptions-item>
          <el-descriptions-item label="发布时间">{{ dateTime(selectedVersionItem.publishedAt) }}</el-descriptions-item>
        </el-descriptions>

        <h5 style="margin: 16px 0 8px;">参数规则 (Parameter Rules)</h5>
        <pre class="json-preview-box">{{ JSON.stringify(selectedVersionItem.parameterRules, null, 2) }}</pre>

        <h5 style="margin: 16px 0 8px;">校验结果 (Validation Result)</h5>
        <pre class="json-preview-box">{{ JSON.stringify(selectedVersionItem.validationResult, null, 2) }}</pre>
      </div>
      <template #footer>
        <el-button @click="detailDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Clock, Edit, InfoFilled } from "@element-plus/icons-vue";
import {
  billingApi,
  type BillingRuleVersion,
  type ProviderCost,
} from "../../api/billing";
import {
  buildGptImageParameterRules,
  calculateGptImageQuotePreview,
  classifyValidationIssues,
  extractGptImageMultipliers,
  formatBillingErrorMessage,
  isGPTImageRule,
} from "../../domain/gptImageBilling";

const props = defineProps<{
  rules: BillingRuleVersion[];
  costs: ProviderCost[];
}>();

const emit = defineEmits<{
  (e: "reload"): void;
}>();

// 筛选所有 gpt-image 相关的版本
const gptImageVersions = computed(() => {
  return props.rules
    .filter((r) => isGPTImageRule(r))
    .sort((a, b) => b.version - a.version);
});

// 当前生效中的正式版本 (PUBLISHED)
const currentPublishedRule = computed(() => {
  return gptImageVersions.value.find((r) => String(r.status || "").toUpperCase() === "PUBLISHED");
});

// 当前草稿版本 (DRAFT)
const activeDraftRule = computed(() => {
  return gptImageVersions.value.find((r) => String(r.status || "").toUpperCase() === "DRAFT");
});

// 匹配的供应商成本 (UnitCost CNY)
const matchingProviderCost = computed(() => {
  return props.costs.find(
    (c) =>
      c.platformModelCode === "gpt-image-2" &&
      String(c.status || "").toUpperCase() === "ACTIVE",
  );
});

// 当前正式版倍率与毛利数据
const currentMultipliers = computed(() => extractGptImageMultipliers(currentPublishedRule.value));

const marginDiff = computed(() => {
  const revenue = ((currentPublishedRule.value?.basePrice || 0) * 1) / 100;
  const cost = matchingProviderCost.value?.unitCost || 0.60;
  return revenue - cost;
});

const hasNegativeMarginRisk = computed(() => {
  const issues = currentPublishedRule.value?.validationResult?.issues || [];
  return issues.some((i) => i.code === "NEGATIVE_MARGIN") || marginDiff.value < 0;
});

const publishedStatusTagType = computed(() => {
  if (!currentPublishedRule.value) return "info";
  return "success";
});

const publishedValidationTagType = computed(() => {
  if (!currentPublishedRule.value) return "info";
  if (currentPublishedRule.value.validationResult?.valid) return "success";
  if (hasNegativeMarginRisk.value) return "warning";
  return "danger";
});

const publishedValidationText = computed(() => {
  if (!currentPublishedRule.value) return "无正式版";
  if (currentPublishedRule.value.validationResult?.valid) return "校验通过";
  if (hasNegativeMarginRisk.value) return "负毛利风险";
  return "存在阻断错误";
});

// 格式化辅助
function formatRevenue(points?: number) {
  const p = Number(points || 0);
  return (p * 0.01).toFixed(2);
}

function formatCost(cost?: number) {
  const c = Number(cost ?? 0.60);
  return Number.isFinite(c) ? c.toFixed(2) : "0.60";
}

function formatMargin(points?: number, unitCost = 0.60) {
  const revenue = Number(points || 0) * 0.01;
  const diff = revenue - unitCost;
  return `${diff >= 0 ? "+" : ""}${diff.toFixed(2)} CNY / 张`;
}

function dateTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

function statusTagType(status?: string) {
  const upper = String(status || "").toUpperCase();
  if (upper === "PUBLISHED") return "success";
  if (upper === "DRAFT") return "warning";
  if (upper === "ARCHIVED") return "info";
  return "info";
}

function getMultiplier(paramRules: unknown, cat: string, key: string, fallback: number) {
  if (!paramRules || typeof paramRules !== "object") return fallback;
  const obj = (paramRules as Record<string, unknown>)[cat];
  if (!obj || typeof obj !== "object") return fallback;
  const val = (obj as Record<string, unknown>)[key];
  return typeof val === "number" ? val : fallback;
}

function getTierMultipliersSummary(paramRules: unknown) {
  const t1 = getMultiplier(paramRules, "size", "tier_1k", 1.0);
  const t2 = getMultiplier(paramRules, "size", "tier_2k", 1.5);
  const t4 = getMultiplier(paramRules, "size", "tier_4k", 2.0);
  return `${t1}x / ${t2}x / ${t4}x`;
}

function getQualityMultipliersSummary(paramRules: unknown) {
  const qL = getMultiplier(paramRules, "quality", "low", 1.0);
  const qN = getMultiplier(paramRules, "quality", "normal", 1.2);
  const qH = getMultiplier(paramRules, "quality", "high", 1.5);
  return `${qL}x / ${qN}x / ${qH}x`;
}

function hasIssueCode(row: BillingRuleVersion, code: string) {
  const issues = row.validationResult?.issues || [];
  return issues.some((i) => i.code === code);
}

// ----------------------------------------------------
// 结构化编辑状态与逻辑
// ----------------------------------------------------
const editorVisible = ref(false);
const savingDraft = ref(false);
const publishing = ref(false);
const jsonPreviewActive = ref<string[]>([]);

const editorForm = reactive({
  basePrice: 10,
  minimumCharge: 1,
  tier1k: 1.0,
  tier2k: 1.5,
  tier4k: 2.0,
  qualityLow: 1.0,
  qualityNormal: 1.2,
  qualityHigh: 1.5,
});

// 实时试算预览数据
const quotePreviewRows = computed(() => {
  return calculateGptImageQuotePreview(editorForm.basePrice, {
    tier1k: editorForm.tier1k,
    tier2k: editorForm.tier2k,
    tier4k: editorForm.tier4k,
    qualityLow: editorForm.qualityLow,
    qualityNormal: editorForm.qualityNormal,
    qualityHigh: editorForm.qualityHigh,
  });
});

const previewMarginDiff = computed(() => {
  const rev = (editorForm.basePrice * 1) / 100;
  const cost = matchingProviderCost.value?.unitCost || 0.60;
  return rev - cost;
});

const generatedJsonPreview = computed(() => {
  const payload = {
    billing_type: "per_image",
    base_price: editorForm.basePrice,
    minimum_charge: editorForm.minimumCharge,
    parameter_multiplier: buildGptImageParameterRules(editorForm),
    status: "DRAFT",
  };
  return JSON.stringify(payload, null, 2);
});

function openEditor() {
  const targetRule = activeDraftRule.value || currentPublishedRule.value;
  const initial = extractGptImageMultipliers(targetRule);
  Object.assign(editorForm, initial);
  editorVisible.value = true;
}

// 保存草稿
async function handleSaveDraft(): Promise<BillingRuleVersion | null> {
  if (editorForm.basePrice <= 0) {
    ElMessage.error("基础售价必须大于 0");
    return null;
  }
  if (editorForm.tier1k <= 0 || editorForm.tier2k <= 0 || editorForm.tier4k <= 0) {
    ElMessage.error("尺寸阶梯倍率必须大于 0");
    return null;
  }
  if (editorForm.qualityLow <= 0 || editorForm.qualityNormal <= 0 || editorForm.qualityHigh <= 0) {
    ElMessage.error("画质等级倍率必须大于 0");
    return null;
  }

  const targetId = activeDraftRule.value?.id || currentPublishedRule.value?.id || "billing_rule_image_gpt";
  const payload = {
    billing_type: "per_image",
    base_price: editorForm.basePrice,
    minimum_charge: editorForm.minimumCharge,
    parameter_multiplier: buildGptImageParameterRules(editorForm),
    status: "DRAFT",
  };

  try {
    savingDraft.value = true;
    const res = await billingApi.createRuleDraft(targetId, payload);
    ElMessage.success(`草稿已保存 (v${res.item.version})，当前正式版不受影响`);
    emit("reload");
    return res.item;
  } catch (error) {
    ElMessage.error(formatBillingErrorMessage(error));
    return null;
  } finally {
    savingDraft.value = false;
  }
}

// ----------------------------------------------------
// 发布上线流程与负毛利确认弹窗
// ----------------------------------------------------
const negativeMarginDialogVisible = ref(false);
const publishingWithOverride = ref(false);
const targetDraftId = ref("");
const targetDraftVersion = ref<number | string>("");
const targetDraftBasePrice = ref<number>(10);

async function handlePublish() {
  // 1. 确保草稿已保存
  const draft = await handleSaveDraft();
  if (!draft) return;

  targetDraftId.value = draft.id;
  targetDraftVersion.value = draft.version;
  targetDraftBasePrice.value = draft.basePrice;

  try {
    publishing.value = true;
    // 2. 先调用 validate 进行安全校验
    const valRes = await billingApi.validateRule(draft.id);
    const issues = valRes.validation?.issues || [];
    const classification = classifyValidationIssues(issues);

    // 3. 如果存在 Hard Blocker，严格禁止发布
    if (classification.hasHardBlockers) {
      const blockerMsgs = classification.hardBlockers
        .map((b) => `• [${b.code}] ${b.message}`)
        .join("\n");
      await ElMessageBox.alert(
        `该版本存在 ${classification.hardBlockers.length} 项硬性结构阻断错误，必须修正后才可发布：\n\n${blockerMsgs}`,
        "校验阻断：禁止发布上线",
        { type: "error", customStyle: { whiteSpace: "pre-wrap" } },
      );
      return;
    }

    // 4. 如果没有 Hard Blocker，但存在负毛利商业风险，弹出二次确认
    if (classification.hasNegativeMargin) {
      negativeMarginDialogVisible.value = true;
      return;
    }

    // 5. 无任何问题，常规确认并直接发布
    await ElMessageBox.confirm(
      `确认将草稿 v${draft.version} 发布为全站正式生效的计费规则？`,
      "发布计费版本确认",
      { type: "warning" },
    );
    await executePublish(draft.id, false);
  } catch (error) {
    if (error !== "cancel") {
      ElMessage.error(formatBillingErrorMessage(error));
    }
  } finally {
    publishing.value = false;
  }
}

async function confirmAndPublishNegativeMargin() {
  if (!targetDraftId.value) return;
  try {
    publishingWithOverride.value = true;
    await executePublish(targetDraftId.value, true);
    negativeMarginDialogVisible.value = false;
  } catch (error) {
    ElMessage.error(formatBillingErrorMessage(error));
  } finally {
    publishingWithOverride.value = false;
  }
}

async function executePublish(draftId: string, confirmNegativeMargin: boolean) {
  const res = await billingApi.publishRule(draftId, { confirmNegativeMargin });
  ElMessage.success(`GPT Image 规则 v${res.item.version} 已正式发布并生效！`);
  editorVisible.value = false;
  emit("reload");
}

// ----------------------------------------------------
// 历史版本详情弹窗
// ----------------------------------------------------
const historyDrawerVisible = ref(false);
const detailDialogVisible = ref(false);
const selectedVersionItem = ref<BillingRuleVersion | null>(null);

function viewVersionDetail(item: BillingRuleVersion) {
  selectedVersionItem.value = item;
  detailDialogVisible.value = true;
}

defineExpose({
  openEditor,
});
</script>

<style scoped>
.gpt-image-panel {
  margin-bottom: 20px;
}

.gpt-image-panel__board {
  border-radius: 14px;
  background: #ffffff;
  border: 1px solid #e3e8ef;
}

.board-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
}

.board-header__info {
  flex: 1;
}

.board-header__title {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.board-header__badge {
  background: #6366f1;
  color: #ffffff;
  font-size: 11px;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 6px;
  letter-spacing: 0.05em;
}

.board-header__title h3 {
  margin: 0;
  font-size: 18px;
  color: #0f172a;
}

.board-header__desc {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 13px;
}

.board-header__actions {
  display: flex;
  gap: 10px;
}

.board-metrics {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
}

.metric-card {
  padding: 14px;
  border: 1px solid #eef2f6;
  border-radius: 10px;
  background: #f8fafc;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 98px;
}

.metric-card__label {
  font-size: 12px;
  color: #64748b;
}

.metric-card__value {
  font-size: 20px;
  color: #0f172a;
  margin: 6px 0 4px;
}

.metric-card__value small {
  font-size: 12px;
  font-weight: normal;
  color: #64748b;
}

.metric-card__hint {
  font-size: 11px;
  color: #94a3b8;
}

.metric-card--multipliers .multipliers-row {
  display: flex;
  gap: 8px;
  font-size: 13px;
  color: #1e293b;
  margin: 8px 0 4px;
  flex-wrap: wrap;
}

.metric-status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 6px 0 4px;
  gap: 6px;
}

.metric-status-row__date {
  font-size: 12px;
  color: #334155;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.text-danger {
  color: #ef4444 !important;
}

.text-success {
  color: #10b981 !important;
}

/* 编辑布局 */
.editor-layout {
  display: grid;
  grid-template-columns: 1.15fr 1fr;
  gap: 20px;
}

.editor-form-pane h4,
.editor-preview-pane h4 {
  margin: 0 0 14px;
  font-size: 15px;
  color: #0f172a;
}

.unit-suffix {
  margin-left: 10px;
  font-size: 12px;
  color: #64748b;
}

.json-preview-box {
  background: #0f172a;
  color: #38bdf8;
  padding: 12px;
  border-radius: 8px;
  font-size: 11px;
  line-height: 1.5;
  max-height: 240px;
  overflow-y: auto;
  margin: 0;
}

/* 试算预览 */
.preview-card {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 16px;
  background: #fdfdfd;
}

.preview-card__desc {
  font-size: 12px;
  color: #64748b;
  margin: 0 0 12px;
}

.preview-table {
  width: 100%;
  margin-bottom: 14px;
}

.price-val {
  font-weight: 700;
  color: #4338ca;
}

.preview-cost-summary {
  background: #f8fafc;
  border: 1px dashed #cbd5e1;
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cost-item {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: #334155;
}

.preview-note {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: 14px;
  font-size: 11px;
  color: #94a3b8;
  line-height: 1.4;
}

.preview-note .el-icon {
  margin-top: 2px;
}

/* 负毛利确认弹窗 */
.negative-margin-content {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.negative-margin-facts {
  background: #fef2f2;
  border: 1px solid #fee2e2;
  border-radius: 8px;
  padding: 12px 16px;
}

.fact-row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  padding: 4px 0;
  color: #374151;
}

.negative-margin-tip {
  font-size: 12px;
  color: #6b7280;
  line-height: 1.5;
  margin: 0;
}

.history-multipliers {
  display: flex;
  flex-direction: column;
  font-size: 12px;
  color: #475569;
}

@media (max-width: 1280px) {
  .board-metrics {
    grid-template-columns: repeat(3, 1fr);
  }
  .editor-layout {
    grid-template-columns: 1fr;
  }
}
</style>
