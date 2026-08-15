import { ref, watch } from "vue";
import type { PptCreateForm } from "../api/ppt";

const DRAFT_KEY = "xianzhi:ppt:v2:draft";

const defaults: PptCreateForm = {
  topic: "",
  description: "",
  pageCount: 10,
  language: "zh",
  scenario: "education",
  style: "business",
  templateId: "tech-blue",
  referenceFileIds: [],
  knowledgeBaseIds: [],
  generateSpeakerNotes: true,
  generateVisuals: true
};

export function usePptDraft() {
  let stored: Partial<PptCreateForm> = {};
  try { stored = uni.getStorageSync(DRAFT_KEY) || {}; } catch { stored = {}; }
  const form = ref<PptCreateForm>({ ...defaults, ...stored });
  watch(form, value => {
    try { uni.setStorageSync(DRAFT_KEY, value); } catch { /* storage may be unavailable in preview */ }
  }, { deep: true });
  function clearDraft() {
    form.value = { ...defaults, referenceFileIds: [], knowledgeBaseIds: [] };
    try { uni.removeStorageSync(DRAFT_KEY); } catch { /* noop */ }
  }
  return { form, clearDraft };
}
