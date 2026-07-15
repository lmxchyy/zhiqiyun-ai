<template>
  <view class="scene-center">
    <view class="scene-center-header">
      <button class="scene-back-mark" type="button" aria-label="返回创作首页" @click="$emit('back')">Z</button>
      <view class="scene-header-copy">
        <text class="scene-page-title">AI 场景</text>
        <text class="scene-page-subtitle">从业务目标出发，一次完成专业创作</text>
      </view>
      <text class="scene-identity-pill">{{ identityLabel }}</text>
    </view>

    <label class="scene-search">
      <text class="scene-search-icon"></text>
      <input v-model.trim="keyword" type="text" placeholder="搜索行业、场景或内容，例如“餐饮开业”" />
      <button v-if="keyword" class="scene-search-clear" type="button" aria-label="清空搜索" @click="keyword = ''">×</button>
    </label>

    <scroll-view scroll-x class="scene-category-scroll" :show-scrollbar="false">
      <view class="scene-category-row">
        <button
          v-for="item in categories"
          :key="item.id"
          :class="['scene-category-button', { active: activeCategory === item.id }]"
          type="button"
          @click="activeCategory = item.id"
        >
          {{ item.label }}
        </button>
      </view>
    </scroll-view>

    <text class="scene-list-title">{{ activeCategoryLabel }}</text>
    <view v-if="filteredScenes.length" class="scene-card-grid">
      <button
        v-for="item in filteredScenes"
        :key="item.id"
        class="scene-card"
        type="button"
        @click="$emit('open-scene', item.mode, item.prompt)"
      >
        <view class="scene-cover-wrap">
          <RemoteCover
            class="scene-cover"
            page-code="studio"
            :slot-key="item.slotKey"
            :alt="item.title"
            mode="cover"
            width="100%"
            height="100%"
            radius="10px"
          />
          <view :class="['scene-cover-shade', item.tone]"></view>
          <text class="scene-cover-title">{{ item.title }}</text>
        </view>
        <view class="scene-card-copy">
          <text class="scene-card-title">{{ item.title }}</text>
          <text v-if="item.tag" :class="['scene-card-tag', item.tagTone]">{{ item.tag }}</text>
          <text class="scene-card-summary">{{ item.summary }}</text>
        </view>
      </button>
    </view>
    <view v-else class="scene-empty">
      <text>没有找到匹配场景</text>
      <button type="button" @click="resetFilters">查看全部场景</button>
    </view>

    <text class="scene-list-title classified-title">分类场景</text>
    <view class="classified-grid">
      <button
        v-for="item in classifiedScenes"
        :key="item.id"
        type="button"
        @click="$emit('open-scene', item.mode, item.prompt)"
      >
        {{ item.title }}
      </button>
    </view>
  </view>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import RemoteCover from "../RemoteCover.vue";

type CreationMode = "image" | "video" | "ppt" | "infographic" | "review" | "agent";
type SceneCategory = "marketing" | "ecommerce" | "office" | "store";

withDefaults(defineProps<{ identityLabel?: string }>(), {
  identityLabel: "普通用户",
});
defineEmits<{
  back: [];
  "open-scene": [mode: CreationMode, prompt: string];
}>();

const keyword = ref("");
const activeCategory = ref<SceneCategory>("marketing");
const categories = [
  { id: "marketing", label: "营销获客" },
  { id: "ecommerce", label: "电商运营" },
  { id: "office", label: "企业办公" },
  { id: "store", label: "门店经营" },
] as const;

const sceneCards = [
  { id: "xiaohongshu", title: "小红书爆款", summary: "封面 + 标题 + 正文", slotKey: "studio.scene.xiaohongshu", categories: ["marketing"], mode: "image", prompt: "创作一套小红书爆款内容，包含高点击封面、标题和正文", tone: "purple", tag: "热门", tagTone: "orange" },
  { id: "friend-poster", title: "朋友圈海报", summary: "文案 + 海报 + 二维码", slotKey: "studio.scene.friend_poster", categories: ["marketing", "store"], mode: "image", prompt: "生成一张朋友圈推广海报，包含营销文案、主视觉和二维码留白", tone: "orange", tag: "推荐", tagTone: "purple" },
  { id: "product-promo", title: "产品宣传", summary: "主视觉 + 卖点 + 视频", slotKey: "studio.scene.product_promo", categories: ["marketing", "ecommerce"], mode: "image", prompt: "制作产品宣传主视觉，突出核心卖点并适配视频封面", tone: "purple", tag: "", tagTone: "purple" },
  { id: "ecommerce-main", title: "商品主图", summary: "白底图 + 场景图", slotKey: "studio.scene.ecommerce_main", categories: ["ecommerce"], mode: "image", prompt: "生成一组电商商品主图，包含白底图和使用场景图", tone: "orange", tag: "", tagTone: "purple" },
  { id: "investment", title: "招商加盟", summary: "海报 + PPT 大纲", slotKey: "studio.scene.investment", categories: ["marketing", "office"], mode: "ppt", prompt: "制作招商加盟方案，先生成招商海报文案和完整 PPT 大纲", tone: "purple", tag: "", tagTone: "purple" },
  { id: "store-event", title: "门店活动", summary: "海报 + 朋友圈文案", slotKey: "studio.scene.store_event", categories: ["store"], mode: "image", prompt: "为门店活动生成促销海报和朋友圈推广文案", tone: "orange", tag: "", tagTone: "purple" },
] as const satisfies ReadonlyArray<{
  id: string;
  title: string;
  summary: string;
  slotKey: string;
  categories: readonly SceneCategory[];
  mode: CreationMode;
  prompt: string;
  tone: "purple" | "orange";
  tag: string;
  tagTone: "purple" | "orange";
}>;

const classifiedScenes = [
  { id: "short-video", title: "短视频营销", mode: "video", prompt: "策划一条用于营销获客的短视频，包含脚本、分镜和口播文案" },
  { id: "ppt-report", title: "PPT 汇报", mode: "ppt", prompt: "制作一份结构清晰的企业工作汇报 PPT" },
  { id: "education", title: "教育培训", mode: "ppt", prompt: "制作一套教育培训课件，包含课程大纲、案例和总结" },
  { id: "logo", title: "Logo 设计", mode: "image", prompt: "设计一个简洁专业、适合企业品牌使用的 Logo" },
] as const satisfies ReadonlyArray<{ id: string; title: string; mode: CreationMode; prompt: string }>;

const activeCategoryLabel = computed(() => {
  const label = categories.find((item) => item.id === activeCategory.value)?.label || "热门";
  return activeCategory.value === "marketing" ? "热门场景" : `${label}场景`;
});
const filteredScenes = computed(() => {
  const term = keyword.value.toLowerCase();
  return sceneCards.filter((item) => {
    const matchesCategory = activeCategory.value === "marketing" || Array.from(item.categories as readonly SceneCategory[]).includes(activeCategory.value);
    const matchesSearch = !term || `${item.title}${item.summary}${item.prompt}`.toLowerCase().includes(term);
    return matchesCategory && matchesSearch;
  });
});

function resetFilters() {
  keyword.value = "";
  activeCategory.value = "marketing";
}
</script>

<style scoped>
.scene-center { min-height: calc(100vh - 138px); color: #111827; }
.scene-center-header { display: flex; min-height: 54px; align-items: center; gap: 12px; }
.scene-back-mark { display: grid; width: 36px; height: 36px; min-height: 36px; flex: 0 0 36px; margin: 0; padding: 0; place-items: center; border: 0; border-radius: 10px; color: #fff; background: #5a4db2; font-size: 13px; font-weight: 700; line-height: 36px; }
.scene-header-copy { display: grid; min-width: 0; flex: 1; gap: 2px; }
.scene-page-title { font-size: 18px; font-weight: 700; line-height: 24px; }
.scene-page-subtitle { overflow: hidden; color: #6b7280; font-size: 11px; line-height: 16px; text-overflow: ellipsis; white-space: nowrap; }
.scene-identity-pill { flex: 0 0 auto; border-radius: 13px; color: #5a4db2; background: #eef0ff; padding: 6px 11px; font-size: 12px; font-weight: 600; }
.scene-search { display: flex; height: 44px; margin-top: 14px; padding: 0 13px; align-items: center; gap: 9px; border: 1px solid #e5e7eb; border-radius: 14px; background: #fff; }
.scene-search-icon { position: relative; width: 14px; height: 14px; flex: 0 0 14px; border: 2px solid #9ca3af; border-radius: 50%; }
.scene-search-icon::after { position: absolute; right: -5px; bottom: -3px; width: 6px; height: 2px; border-radius: 2px; background: #9ca3af; transform: rotate(45deg); content: ""; }
.scene-search input { min-width: 0; height: 42px; flex: 1; color: #111827; font-size: 12px; }
.scene-search-clear { width: 28px; height: 28px; min-height: 28px; margin: 0; padding: 0; border: 0; border-radius: 50%; color: #667085; background: #f2f4f7; font-size: 18px; line-height: 28px; }
.scene-category-scroll { margin: 14px -15px 0; padding-left: 15px; }
.scene-category-row { display: flex; width: max-content; gap: 7px; padding-right: 15px; }
.scene-category-button { width: 80px; height: 28px; min-height: 28px; margin: 0; padding: 0; border: 0; border-radius: 13px; color: #6b7280; background: #fff; font-size: 12px; font-weight: 600; line-height: 28px; }
.scene-category-button.active { color: #fff; background: #5a4db2; box-shadow: 0 6px 14px rgba(90,77,178,.18); }
.scene-list-title { display: block; margin: 17px 0 10px; font-size: 16px; font-weight: 600; line-height: 22px; }
.scene-card-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 12px 10px; }
.scene-card { display: block; width: 100%; min-height: 116px; margin: 0; padding: 9px; overflow: hidden; border: 1px solid #e5e7eb; border-radius: 16px; background: #fff; text-align: left; box-shadow: 0 6px 16px rgba(31,46,89,.035); }
.scene-cover-wrap { position: relative; height: 54px; overflow: hidden; border-radius: 10px; background: #eef2ff; }
.scene-cover { position: absolute; inset: 0; width: 100% !important; height: 100% !important; }
.scene-cover-shade { position: absolute; inset: 0; background: rgba(79,70,229,.5); }
.scene-cover-shade.orange { background: rgba(255,119,27,.48); }
.scene-cover-title { position: absolute; z-index: 2; right: 8px; bottom: 8px; left: 8px; overflow: hidden; color: #fff; font-size: 13px; font-weight: 700; text-align: center; text-overflow: ellipsis; white-space: nowrap; }
.scene-card-copy { position: relative; display: grid; margin-top: 6px; gap: 2px; }
.scene-card-title { padding-right: 44px; color: #111827; font-size: 13px; font-weight: 600; line-height: 18px; }
.scene-card-summary { overflow: hidden; color: #6b7280; font-size: 10px; line-height: 14px; text-overflow: ellipsis; white-space: nowrap; }
.scene-card-tag { position: absolute; top: -2px; right: 0; border-radius: 10px; padding: 3px 7px; font-size: 9px; font-weight: 600; }
.scene-card-tag.orange { color: #ff771b; background: #fff1e8; }
.scene-card-tag.purple { color: #5a4db2; background: #eef0ff; }
.scene-empty { display: grid; min-height: 180px; place-items: center; align-content: center; gap: 12px; border: 1px dashed #d8deed; border-radius: 16px; color: #667085; background: #fff; font-size: 13px; }
.scene-empty button { width: auto; min-height: 34px; margin: 0; padding: 0 14px; border: 0; border-radius: 12px; color: #5a4db2; background: #eef0ff; font-size: 12px; }
.classified-title { margin-top: 20px; }
.classified-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 8px 14px; }
.classified-grid button { width: 100%; height: 30px; min-height: 30px; margin: 0; padding: 0; border: 0; border-radius: 13px; color: #5a4db2; background: #fff; font-size: 12px; font-weight: 600; line-height: 30px; }
.scene-back-mark::after,.scene-search-clear::after,.scene-category-button::after,.scene-card::after,.scene-empty button::after,.classified-grid button::after { display: none; }
@media (max-width: 360px) {
  .scene-card-grid { gap: 10px 8px; }
  .scene-card { padding: 8px; }
  .scene-category-button { width: 76px; }
}
</style>
