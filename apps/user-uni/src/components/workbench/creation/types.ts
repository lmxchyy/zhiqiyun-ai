import type { MiniProgramCreationMode } from "../../../config/miniProgramPages";

export interface GenerationNotice {
  id: string;
  title: string;
  status: string;
  tone: "pending" | "success" | "danger";
  resultId?: string;
  resultUrl?: string;
  resultType?: MiniProgramCreationMode;
  progress?: number;
}
