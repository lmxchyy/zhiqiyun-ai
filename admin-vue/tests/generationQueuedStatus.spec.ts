import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import TaskItem from "../../apps/user-uni/src/components/assets/GenerationTaskItem.vue";
import { taskToVideoHistoryEntry, videoCardPlaceholderText } from "../src/utils/videoGeneration";
import { GENERATION_QUEUED_LABEL, generationDisplayStatus } from "@xianzhi/shared-types";

describe("Image/Video QUEUED contract", () => {
  it("renders the actual mini task card without fake progress, retaining cancel", () => {
    const wrapper = mount(TaskItem, { props: { task: {
      id: "q", name: "Image", type: "image", status: "queued", progress: 0,
      createdAt: "2026-01-01", updatedAt: "2026-01-01", resultIds: [], params: {}, prompt: "", model: "",
    } } });
    expect(wrapper.text()).toContain(GENERATION_QUEUED_LABEL);
    expect(wrapper.text()).not.toContain("生成中");
    expect(wrapper.text()).not.toContain("0%");
    expect(wrapper.find(".progress-track").exists()).toBe(false);
    expect(wrapper.text()).toContain("取消");
  });
  it("keeps web video queued through normalization, then switches to running/completed", () => {
    const task = { id: "v", type: "TEXT_TO_VIDEO", status: "PROCESSING", taskStatus: "QUEUED" };
    const entry = taskToVideoHistoryEntry(task)!;
    expect(entry.status).toBe("generating"); // Existing polling remains active.
    expect(videoCardPlaceholderText(entry)).toBe(GENERATION_QUEUED_LABEL);
    expect(videoCardPlaceholderText(taskToVideoHistoryEntry({ ...task, taskStatus: "RUNNING" }))).toBe("生成中");
    expect(videoCardPlaceholderText(taskToVideoHistoryEntry({ ...task, status: "SUCCEEDED" }))).toBe("已完成");
    expect(videoCardPlaceholderText(taskToVideoHistoryEntry({ ...task, status: "FAILED" }))).toBe("生成失败");
  });
  it("does not let stale taskStatus override terminals", () => {
    for (const status of ["SUCCEEDED", "FAILED", "CANCELLED"]) {
      expect(generationDisplayStatus({ status, taskStatus: "QUEUED" })).toBe(status);
    }
  });
});
