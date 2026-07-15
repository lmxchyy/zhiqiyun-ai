<template>
  <view class="promotion-page has-fixed-action">
    <PromotionPageHeader title="推广模板" subtitle="选择适合当前推广场景的视觉模板" />
    <view class="promotion-content">
      <view class="promotion-filter-row">
        <button v-for="category in categories" :key="category.id" :class="['promotion-filter-pill', { active: activeCategory === category.id }]" @click="activeCategory = category.id"><text>{{ category.name }}</text></button>
      </view>
      <view class="promotion-template-grid">
        <PromotionTemplateThumb v-for="template in filteredTemplates" :key="template.id" :template="template" :selected="selectedTemplateId === template.id" @select="selectTemplate" />
      </view>
      <PromotionStatePanel v-if="!filteredTemplates.length" tone="empty" title="当前分类暂无可用模板" description="可切换“全部”查看当前角色支持的模板" />
    </view>
    <view class="promotion-fixed-action"><view><text>已选 {{ selectedTemplate.name }}</text><text>{{ selectedTemplate.categoryLabel }}</text></view><button class="promotion-primary-button" @click="preview"><text>预览并生成</text></button></view>
  </view>
</template>
<script setup lang="ts">
import { computed, ref } from "vue";
import { onLoad, onShow } from "@dcloudio/uni-app";
import PromotionPageHeader from "../../components/promotion/PromotionPageHeader.vue";
import PromotionStatePanel from "../../components/promotion/PromotionStatePanel.vue";
import PromotionTemplateThumb from "../../components/promotion/PromotionTemplateThumb.vue";
import { trackPromotion } from "../../features/promotion/analytics";
import { promotionTemplateById, promotionTemplatesForRole } from "../../features/promotion/templates";
import type { PromotionTemplateId } from "../../features/promotion/types";
import { useUserStore } from "../../stores/user";
const userStore = useUserStore(); const selectedTemplateId = ref<PromotionTemplateId>("poster.brand.simple"); const activeCategory = ref("all");
const categories = [{ id: "all", name: "全部" }, { id: "brand", name: "品牌" }, { id: "product", name: "产品" }, { id: "invite", name: "邀新" }, { id: "industry", name: "行业" }, { id: "campaign", name: "活动" }];
const roleTemplates = computed(() => promotionTemplatesForRole(userStore.currentRole));
const filteredTemplates = computed(() => activeCategory.value === "all" ? roleTemplates.value : roleTemplates.value.filter(item => item.category === activeCategory.value));
const selectedTemplate = computed(() => promotionTemplateById(selectedTemplateId.value));
onLoad(options => { selectedTemplateId.value = promotionTemplateById(String(options?.templateId || "")).id; });
onShow(() => void userStore.loadProfile());
function selectTemplate(id: PromotionTemplateId) { selectedTemplateId.value = id; trackPromotion("promotion_template_select", { templateId: id, source: "template_center" }); }
function preview() { uni.navigateTo({ url: `/pages/promotion/PromotionPosterPreviewPage?templateId=${encodeURIComponent(selectedTemplateId.value)}` }); }
</script>
<style>@import "../../styles/promotion-center.css";</style>
