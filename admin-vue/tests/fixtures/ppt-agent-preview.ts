import { createApp, h } from "vue";
import PptAgentDeckPreview from "../../src/components/ppt/PptAgentDeckPreview.vue";
import type { OutlinePlan, PptAgentPreviewProjection } from "../../src/types/pptAgent";

declare global {
  interface Window {
    __PPT_AGENT_GOLDEN_2__?: { projection: PptAgentPreviewProjection; approvedOutline: OutlinePlan };
  }
}

const fixture = window.__PPT_AGENT_GOLDEN_2__;
if (!fixture) throw new Error("Golden 2 preview fixture is unavailable");

createApp({
  render: () => h(PptAgentDeckPreview, {
    projection: fixture.projection,
    approvedOutline: fixture.approvedOutline,
    loading: false,
    error: "",
    busy: false
  })
}).mount("#app");
