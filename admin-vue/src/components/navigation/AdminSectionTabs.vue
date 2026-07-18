<template>
  <section v-if="tabs.length > 1" class="admin-section-navigation" :aria-label="`${sectionTitle}子页面`">
    <div class="admin-section-navigation-copy">
      <span>{{ groupTitle }}</span>
      <strong>{{ sectionTitle }}</strong>
    </div>
    <nav class="admin-section-navigation-tabs" role="tablist" :aria-label="sectionTitle">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        type="button"
        role="tab"
        :aria-selected="tab.id === activeModuleId"
        :tabindex="tab.id === activeModuleId ? 0 : -1"
        :class="{ active: tab.id === activeModuleId }"
        @click="$emit('select', tab.id)"
        @keydown.left.prevent="moveFocus(tab.id, -1)"
        @keydown.right.prevent="moveFocus(tab.id, 1)"
        @keydown.home.prevent="focusTab(0)"
        @keydown.end.prevent="focusTab(tabs.length - 1)"
      >
        {{ tab.title }}
      </button>
    </nav>
  </section>
</template>

<script setup lang="ts">
import { nextTick } from "vue";

const props = defineProps<{
  groupTitle: string;
  sectionTitle: string;
  activeModuleId: string;
  tabs: Array<{ id: string; title: string }>;
}>();

defineEmits<{ select: [moduleId: string] }>();

function tabButtons() {
  return Array.from(document.querySelectorAll<HTMLButtonElement>(".admin-section-navigation-tabs [role='tab']"));
}

function focusTab(index: number) {
  void nextTick(() => tabButtons()[index]?.focus());
}

function moveFocus(moduleId: string, direction: -1 | 1) {
  const currentIndex = props.tabs.findIndex((tab) => tab.id === moduleId);
  const nextIndex = (currentIndex + direction + props.tabs.length) % props.tabs.length;
  focusTab(nextIndex);
}
</script>

<style scoped>
.admin-section-navigation {
  display: flex;
  min-width: 0;
  padding: 12px 14px;
  align-items: center;
  gap: 18px;
  border: 1px solid var(--admin-border);
  border-radius: 10px;
  background: var(--admin-panel);
  box-shadow: 0 5px 18px rgba(30, 45, 80, 0.035);
}

.admin-section-navigation-copy {
  display: grid;
  min-width: 128px;
  gap: 2px;
}

.admin-section-navigation-copy span {
  color: var(--admin-muted);
  font-size: 11px;
}

.admin-section-navigation-copy strong {
  color: var(--admin-text);
  font-size: 14px;
}

.admin-section-navigation-tabs {
  display: flex;
  min-width: 0;
  overflow-x: auto;
  gap: 6px;
  scrollbar-width: thin;
}

.admin-section-navigation-tabs button {
  min-height: 34px;
  padding: 0 13px;
  flex: 0 0 auto;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--admin-muted);
  background: transparent;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

.admin-section-navigation-tabs button:hover,
.admin-section-navigation-tabs button:focus-visible {
  border-color: var(--color-border-active);
  color: var(--color-primary);
  outline: none;
}

.admin-section-navigation-tabs button.active {
  color: var(--color-primary);
  background: var(--color-primary-light);
}

@media (max-width: 900px) {
  .admin-section-navigation {
    align-items: stretch;
    flex-direction: column;
    gap: 10px;
  }

  .admin-section-navigation-copy {
    min-width: 0;
  }
}
</style>
