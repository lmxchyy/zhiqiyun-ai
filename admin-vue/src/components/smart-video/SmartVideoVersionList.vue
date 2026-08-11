<template>
  <section class="sv-card" aria-label="版本历史">
    <h2>版本</h2>
    <p v-if="!store.versions.length" class="sv-muted">生成方案后会出现版本记录。</p>
    <ul v-else class="sv-list" role="list">
      <li v-for="version in store.versions" :key="version.id">
        <button type="button" class="sv-version" :class="{ active: store.currentVersion?.id === version.id }" @click="store.selectVersion(version.id)">
          <strong>v{{ version.versionNumber }} · {{ version.source }}</strong>
          <span>
            {{ version.changeNote || "方案快照" }}
            <template v-if="store.project?.confirmedVersionId === version.id"> · 已确认</template>
            <template v-if="store.project?.currentVersionId === version.id"> · 当前</template>
          </span>
        </button>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { useSmartVideoStore } from "../../stores/smartVideo";

const store = useSmartVideoStore();
</script>

<style scoped>
.sv-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  background: rgba(23, 27, 36, 0.92);
  padding: 16px;
}

.sv-muted {
  color: #9aa3b5;
  font-size: 13px;
}

.sv-list {
  list-style: none;
  margin: 12px 0 0;
  padding: 0;
  display: grid;
  gap: 8px;
}

.sv-version {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 10px 12px;
  border-radius: 12px;
  border: 1px solid transparent;
  background: rgba(255, 255, 255, 0.03);
  color: #f4f6fb;
  text-align: left;
  cursor: pointer;
}

.sv-version.active {
  border-color: rgba(255, 119, 27, 0.55);
  background: rgba(255, 119, 27, 0.08);
}

.sv-version span {
  color: #9aa3b5;
  font-size: 12px;
}
</style>
