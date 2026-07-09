import { computed, ref } from "vue";
import { ElMessage } from "element-plus/es/components/message/index";
import { adminRequest } from "../api/client";

type OfficeCLICommand = { label: string; command: string };

type OfficeCLIStatusResponse = {
  available?: boolean;
  binaryPath?: string;
  version?: string;
  error?: string;
  runnerMode?: string;
  installCommands?: OfficeCLICommand[];
  mcpCommands?: OfficeCLICommand[];
  capabilities?: Array<{ code: string; label: string }>;
  formats?: string[];
};

type OfficeCLIFormat = "docx" | "xlsx" | "pptx";

type OfficeCLIDocumentResponse = {
  id: string;
  fileName: string;
  format: OfficeCLIFormat;
  title: string;
  downloadUrl: string;
  size: number;
  commands?: OfficeCLICommand[];
};

type UseOfficeCLIOptions = {
  hasAuthToken: () => boolean;
  downloadUrl: (url: string, fileName: string) => Promise<void>;
};

export const officeCLIFormatOptions: Array<{ label: string; value: OfficeCLIFormat; desc: string }> = [
  { label: "Word", value: "docx", desc: "报告 / 方案" },
  { label: "Excel", value: "xlsx", desc: "表格 / 清单" },
  { label: "PPT", value: "pptx", desc: "演示 / 汇报" }
];

export function useOfficeCLI(options: UseOfficeCLIOptions) {
  const officeCLIStatus = ref<OfficeCLIStatusResponse | null>(null);
  const officeCLIStatusLoading = ref(false);
  const officeCLIForm = ref<{ format: OfficeCLIFormat; title: string; prompt: string }>({
    format: "pptx",
    title: "OfficeCLI 文档智能体演示",
    prompt: "生成一份面向客户的 OfficeCLI 能力介绍，包含产品价值、适用场景和下一步计划。"
  });
  const officeCLIWorkspaceOpen = ref(false);
  const officeCLIDocumentGenerating = ref(false);
  const officeCLIDocumentResult = ref<OfficeCLIDocumentResponse | null>(null);

  const officeCLIStatusLabel = computed(() => {
    if (officeCLIStatusLoading.value) return "检测中";
    if (officeCLIStatus.value?.available) return "已安装";
    if (officeCLIStatus.value) return "待安装";
    return "未检测";
  });
  const officeCLIStatusTone = computed(() => officeCLIStatus.value?.available ? "ready" : officeCLIStatus.value ? "pending" : "idle");
  const officeCLIDocumentSizeText = computed(() => {
    const size = officeCLIDocumentResult.value?.size || 0;
    if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
    if (size >= 1024) return `${Math.max(1, Math.round(size / 1024))} KB`;
    return `${size} B`;
  });

  async function loadOfficeCLIStatus() {
    if (!options.hasAuthToken()) return;
    officeCLIStatusLoading.value = true;
    try {
      officeCLIStatus.value = await adminRequest<OfficeCLIStatusResponse>({ method: "GET", url: "/officecli/status" });
    } catch (error) {
      officeCLIStatus.value = { available: false, error: error instanceof Error ? error.message : "OfficeCLI 状态检测失败" };
    } finally {
      officeCLIStatusLoading.value = false;
    }
  }

  async function submitOfficeCLIDocument() {
    const prompt = officeCLIForm.value.prompt.trim();
    if (!prompt) {
      ElMessage.warning("请先填写生成需求");
      return;
    }
    officeCLIDocumentGenerating.value = true;
    try {
      if (!officeCLIStatus.value?.available) await loadOfficeCLIStatus();
      if (!officeCLIStatus.value?.available) throw new Error("OfficeCLI 尚未可用，请先确认服务端安装状态");
      officeCLIDocumentResult.value = await adminRequest<OfficeCLIDocumentResponse>({
        method: "POST",
        url: "/officecli/documents",
        data: {
          format: officeCLIForm.value.format,
          title: officeCLIForm.value.title.trim() || "OfficeCLI 文档",
          prompt
        }
      });
      ElMessage.success("Office 文档已生成");
    } catch (error) {
      ElMessage.error(error instanceof Error ? error.message : "文档生成失败");
    } finally {
      officeCLIDocumentGenerating.value = false;
    }
  }

  async function downloadOfficeCLIDocument() {
    const result = officeCLIDocumentResult.value;
    if (!result) return;
    try {
      await options.downloadUrl(result.downloadUrl, result.fileName);
    } catch (error) {
      ElMessage.error(error instanceof Error ? error.message : "文件下载失败");
    }
  }

  return {
    officeCLIFormatOptions,
    officeCLIStatus,
    officeCLIStatusLoading,
    officeCLIForm,
    officeCLIWorkspaceOpen,
    officeCLIDocumentGenerating,
    officeCLIDocumentResult,
    officeCLIStatusLabel,
    officeCLIStatusTone,
    officeCLIDocumentSizeText,
    loadOfficeCLIStatus,
    submitOfficeCLIDocument,
    downloadOfficeCLIDocument
  };
}
