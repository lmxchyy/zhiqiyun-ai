<template>
  <section class="sv-card" aria-label="导出成片">
    <div class="sv-head">
      <div>
        <h2>导出</h2>
        <p class="sv-muted">确认方案后查看报价并提交导出，完成后可到作品中心查看。</p>
      </div>
      <div class="sv-inline">
        <button type="button" class="sv-btn" :disabled="store.busy || !store.currentVersion" @click="store.loadQuote()">估算积分</button>
        <button
          type="button"
          class="sv-btn primary"
          :disabled="store.busy || (store.phase === 'rendering' && !isRenderFailed)"
          @click="store.startExport()"
        >
          {{ store.phase === 'rendering' && !isRenderFailed ? '导出中…' : '提交导出' }}
        </button>
        <button v-if="store.phase === 'rendering'" type="button" class="sv-btn ghost" :disabled="store.busy" @click="store.cancelExport()">取消</button>
        <button v-if="isRenderFailed" type="button" class="sv-btn" @click="store.retryExport()">重试</button>
      </div>
    </div>

    <p v-if="store.phase === 'rendering' && !isRenderFailed" class="sv-muted">
      正在导出成片，按钮暂时不可点属正常；可点「取消」后重新提交。
    </p>

    <div v-if="store.quote" class="sv-quote">
      <strong>{{ store.quote.points }} 积分</strong>
      <span>报价有效至 {{ formatTime(store.quote.expiresAt) }}</span>
    </div>

    <div v-if="store.renderTask" class="sv-render" aria-live="polite">
      <div class="sv-row">
        <strong>任务 {{ store.renderTask.status }}</strong>
        <span>{{ store.renderTask.stage || store.renderTask.step || "处理中" }} · {{ store.renderTask.progress || 0 }}%</span>
      </div>
      <div class="sv-progress" role="progressbar" :aria-valuenow="store.renderTask.progress || 0" aria-valuemin="0" aria-valuemax="100">
        <i :style="{ width: `${store.renderTask.progress || 0}%` }" />
      </div>
      <p v-if="store.phase === 'completed'" class="sv-success">
        导出完成。
        <button type="button" class="sv-link" @click="goWorks">前往作品中心</button>
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useAdminStore } from "../../stores/admin";
import { useSmartVideoStore } from "../../stores/smartVideo";

const store = useSmartVideoStore();
const admin = useAdminStore();

const isRenderFailed = computed(() => ["FAILED"].includes(String(store.renderTask?.status || "").toUpperCase()));

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function goWorks() {
  void admin.openWorksMine();
}
</script>

<style scoped>
.sv-card {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  background: rgba(23, 27, 36, 0.92);
  padding: 18px;
}

.sv-head,
.sv-inline,
.sv-row,
.sv-quote {
  display: flex;
  gap: 12px;
}

.sv-head {
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
}

.sv-inline,
.sv-row,
.sv-quote {
  align-items: center;
  flex-wrap: wrap;
}

.sv-muted,
.sv-quote span,
.sv-row span {
  color: #9aa3b5;
  font-size: 12px;
}

.sv-quote,
.sv-render {
  margin-top: 10px;
  padding: 12px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.03);
}

.sv-progress {
  margin-top: 10px;
  height: 8px;
  border-radius: 999px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.08);
}

.sv-progress i {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, #423499, #ff771b);
}

.sv-success {
  margin: 12px 0 0;
  color: #9dffb0;
}

.sv-btn,
.sv-link {
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.04);
  color: #f4f6fb;
  padding: 8px 14px;
  cursor: pointer;
}

.sv-btn.primary {
  border: 0;
  background: #ff771b;
  color: #111;
  font-weight: 600;
}

.sv-btn.ghost,
.sv-link {
  background: transparent;
}

.sv-link {
  border: 0;
  color: #9dffb0;
  padding: 0;
}

.sv-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

@media (max-width: 720px) {
  .sv-head {
    flex-direction: column;
  }
}
</style>
