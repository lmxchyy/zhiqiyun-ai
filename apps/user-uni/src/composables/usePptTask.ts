import { computed, onUnmounted, ref } from "vue";
import { getPptGenerationTask, type PptTaskResponse } from "../api/ppt";

const terminal = new Set(["success", "failed", "cancelled"]);

export function usePptTask() {
  const task = ref<PptTaskResponse | null>(null);
  const loading = ref(false);
  const error = ref("");
  let timer: ReturnType<typeof setTimeout> | null = null;
  let sequence = 0;
  let currentTaskId = "";
  let visible = true;

  function clearTimer() { if (timer) clearTimeout(timer); timer = null; }
  async function refresh() {
    if (!currentTaskId) return;
    const requestSequence = ++sequence;
    loading.value = !task.value;
    try {
      const next = await getPptGenerationTask(currentTaskId);
      if (requestSequence !== sequence) return;
      task.value = next;
      error.value = "";
      if (!terminal.has(next.status)) schedule();
    } catch (cause) {
      if (requestSequence !== sequence) return;
      error.value = cause instanceof Error ? cause.message : "任务状态加载失败";
      schedule(visible ? 5000 : 15000);
    } finally { if (requestSequence === sequence) loading.value = false; }
  }
  function schedule(delay = visible ? 3000 : 15000) {
    clearTimer();
    if (!currentTaskId || terminal.has(task.value?.status || "")) return;
    timer = setTimeout(refresh, delay);
  }
  function start(taskId: string) {
    if (currentTaskId === taskId && timer) return;
    stop(); currentTaskId = taskId; void refresh();
  }
  function setVisible(value: boolean) {
    visible = value;
    if (value) void refresh(); else schedule(15000);
  }
  function stop() { clearTimer(); sequence += 1; currentTaskId = ""; }
  onUnmounted(stop);
  return { task, loading, error, progress: computed(() => task.value?.progress || (task.value?.status === "success" ? 100 : 0)), start, stop, refresh, setVisible };
}
