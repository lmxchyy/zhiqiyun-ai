import { miniProgramAssetCreationPages } from "../../config/miniProgramPages";
import type { GenerationTask } from "./types";

const CREATION_PROMPT_STORAGE_KEY = "v531-creation-prompt";

function creationModeForTask(task: GenerationTask): keyof typeof miniProgramAssetCreationPages {
  if (task.type === "video") return "video";
  if (task.type === "ppt" || task.type === "document") return "ppt";
  if (task.type === "infographic") return "infographic";
  if (task.type === "agent") return "agent";
  return "image";
}

export function navigateToEditFailedTaskPrompt(task: GenerationTask) {
  const prompt = String(task.prompt || "").trim();
  if (prompt) {
    uni.setStorageSync(CREATION_PROMPT_STORAGE_KEY, prompt);
  }
  const mode = creationModeForTask(task);
  uni.navigateTo({
    url: miniProgramAssetCreationPages[mode],
    fail: () => {
      uni.showToast({ title: "无法打开创作页", icon: "none" });
    },
  });
}

export function confirmRetryFailedTask(task: GenerationTask, runRetry: (task: GenerationTask) => void | Promise<void>) {
  uni.showModal({
    title: "重新生成",
    content: "将使用原提示词重新生成，并按规则重新计费。",
    confirmText: "确认重试",
    cancelText: "取消",
    confirmColor: "#ff771b",
    success: (result) => {
      if (result.confirm) void Promise.resolve(runRetry(task));
    },
  });
}

export function confirmDeleteFailedTask(task: GenerationTask, runDelete: (task: GenerationTask) => void | Promise<void>) {
  uni.showModal({
    title: "删除任务记录",
    content: "删除后仅从任务列表移除，不影响已生成作品与账单记录。",
    confirmText: "删除",
    cancelText: "取消",
    confirmColor: "#e05435",
    success: (result) => {
      if (result.confirm) void Promise.resolve(runDelete(task));
    },
  });
}

/** Failed-task row: show reason, then choose retry / edit / delete. */
export function openFailedTaskActions(
  task: GenerationTask,
  runRetry: (task: GenerationTask) => void | Promise<void>,
  runDelete?: (task: GenerationTask) => void | Promise<void>,
) {
  const reason = String(task.failureReason || "").trim() || "生成失败";
  uni.showModal({
    title: task.name || "生成失败",
    content: reason,
    confirmText: "处理",
    cancelText: "关闭",
    confirmColor: "#ff771b",
    success: (result) => {
      if (!result.confirm) return;
      const itemList = runDelete
        ? ["重试（重新计费）", "去修改提示词", "删除记录"]
        : ["重试（重新计费）", "去修改提示词"];
      setTimeout(() => {
        uni.showActionSheet({
          itemList,
          success: (sheet) => {
            if (sheet.tapIndex === 0) {
              confirmRetryFailedTask(task, runRetry);
              return;
            }
            if (sheet.tapIndex === 1) {
              navigateToEditFailedTaskPrompt(task);
              return;
            }
            if (sheet.tapIndex === 2 && runDelete) {
              confirmDeleteFailedTask(task, runDelete);
            }
          },
        });
      }, 80);
    },
  });
}
