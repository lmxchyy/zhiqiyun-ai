export type AgentPlanningStage =
  | "CREATED"
  | "INTENT_RESOLVED"
  | "RESEARCHED"
  | "STORYLINE_PLANNED"
  | "OUTLINE_PLANNED"
  | "OUTLINE_APPROVED"
  | "CONTENT_READY"
  | "ASSETS_READY"
  | "LAYOUT_COMPILED"
  | "QUALITY_CHECKED"
  | "RENDERED"
  | "FILE_STORED"
  | "ASSET_CREATED"
  | "TASK_RELATED"
  | "COMPLETED";

export type AgentPlanningStatus = "QUEUED" | "RUNNING" | "RETRY_WAIT" | "FAILED" | "CANCELLED" | "WAITING_FOR_OUTLINE_APPROVAL" | "SUCCEEDED";

export interface AgentPlanningError {
  code: string;
  message: string;
  stage: string;
  retryable: boolean;
  provider?: string;
  providerRequestId?: string;
  occurredAt?: string;
}

export interface AgentPlanningJob {
  id: string;
  workflowType: "AGENT_OUTLINE";
  tenantId: string;
  userId: string;
  organizationId: string;
  status: AgentPlanningStatus;
  stage: AgentPlanningStage;
  completedWorkUnits: number;
  totalWorkUnits: number;
  slideCount: number;
  runAfter: string;
  error?: AgentPlanningError;
  updatedAt: string;
  deckId?: string;
  revision?: number;
  fileId?: string;
  assetId?: string;
  taskId?: string;
}

export interface AgentIntentSpec {
  topic: string;
  goal: string;
  audience: string;
  scenario: string;
  language: string;
  pageCount: { min: number; max: number; preferred?: number; explicit: boolean };
  professionalStyle: string;
  researchRequired: boolean;
}

export interface ResearchSource {
  id: string;
  provider: string;
  providerIdentity: string;
  title: string;
  type: string;
  locator: string;
  retrievedAt: string;
}

export interface ResearchCitation {
  id: string;
  sourceId: string;
  locator: string;
  retrievedAt: string;
}

export interface ResearchClaim {
  id: string;
  sourceId: string;
  citationRefs: string[];
  text: string;
  verificationStatus: string;
}

export interface ResearchPack {
  sources: ResearchSource[];
  claims: ResearchClaim[];
  citations: ResearchCitation[];
  datasets: Array<{ id: string; sourceId: string; title: string; locator: string; citationRefs: string[] }>;
  verificationStatus: string;
}

export interface PlanningProvenance {
  mode: "AI" | "DETERMINISTIC_TEST";
  provider: string;
  model: string;
  providerRequestId?: string;
}

export interface Storyline {
  id: string;
  language: string;
  thesis: string;
  audienceTakeaway: string;
  narrativeArc: string[];
  sections: Array<{ id: string; title: string; objective: string; evidenceRefs: string[] }>;
  closingAction: string;
  provenance: PlanningProvenance;
}

export interface EvidenceAssignment {
  claimId: string;
  rationale: string;
}

export interface SlideObjective {
  slideId: string;
  title: string;
  purpose: string;
  keyMessage: string;
  evidenceRequired: boolean;
  evidenceRefs: string[];
  evidence: EvidenceAssignment[];
  visualIntent: string;
  expectedElementTypes: string[];
}

export interface OutlinePlan {
  id: string;
  revision: number;
  topic: string;
  language: string;
  pageCount: number;
  nextSlideSequence: number;
  slides: SlideObjective[];
  createdAt: string;
  approvedAt?: string;
  provenance: PlanningProvenance;
}

export interface AgentPlanningState {
  job: AgentPlanningJob;
  intent: AgentIntentSpec;
  research: ResearchPack;
  storyline: Storyline;
  outline: OutlinePlan;
  approvedOutline?: OutlinePlan;
  researchExecutionCount: number;
}

export type PptAgentEditCommandType = "UPDATE_TEXT" | "REGENERATE_SLIDE" | "CHANGE_LAYOUT" | "REPLACE_IMAGE" | "MOVE_SLIDE" | "ADD_SLIDE" | "DELETE_SLIDE";

export interface PptAgentEditCommand {
  commandId: string;
  commandType: PptAgentEditCommandType;
  deckId: string;
  baseRevision: number;
  targetSlideId: string;
  targetElementId?: string;
  payload: Record<string, string>;
  userIntentSummary: string;
}

export interface PptPreviewTextContent {
  kind: "plain" | "bullets";
  text?: string;
  items?: string[];
}

export interface PptPreviewSlideElement {
  id: string;
  type: "text" | "shape" | "image";
  slot: string;
  content?: PptPreviewTextContent;
  styleRole?: string;
  shapeType?: "rect" | "roundRect" | "ellipse";
  assetRef?: string;
  fit?: "cover" | "contain";
  altText?: string;
  citationRefs?: string[];
}

export interface PptPreviewSlide {
  id: string;
  sequence: number;
  role: string;
  layoutId: string;
  backgroundToken: string;
  speakerNotes: string;
  objectiveId: string;
  keyMessage: string;
  evidenceRequired: boolean;
  citationRefs: string[];
  elements: PptPreviewSlideElement[];
}

export interface PptPreviewResolvedStyle {
  kind: "text" | "shape" | "image";
  fontFace?: string;
  fontSizePt?: number;
  color?: string;
  bold?: boolean;
  italic?: boolean;
  align?: "left" | "center" | "right" | "justify";
  verticalAlign?: "top" | "middle" | "bottom";
  marginPt?: number;
  shapeType?: "rect" | "roundRect" | "ellipse";
  fillColor?: string;
  lineColor?: string;
  lineWidthPt?: number;
  transparency?: number;
  fit?: "cover" | "contain";
}

export interface PptPreviewLayoutElement {
  elementId: string;
  x: number;
  y: number;
  width: number;
  height: number;
  zIndex: number;
  resolvedStyle: PptPreviewResolvedStyle;
}

export interface PptPreviewLayoutSlide {
  slideId: string;
  layoutId: string;
  backgroundColor: string;
  elements: PptPreviewLayoutElement[];
}

export interface PptAgentPreviewProjection {
  deckId: string;
  revision: number;
  deck: {
    contractVersion: string;
    deckId: string;
    revision: number;
    deckSpec: { title: string; language: string; author?: string; audience?: string; scenario?: string };
    assetManifest: Array<{ id: string; type: string; mimeType: string; uri: string; sha256: string }>;
    provenance: {
      sources: Array<{ id: string; title: string; type: string; locator: string }>;
      citations: Array<{ id: string; sourceId: string; locator: string }>;
      claims: Array<{ id: string; sourceId: string; citationRefs: string[]; text: string; verificationStatus: string }>;
    };
    slides: PptPreviewSlide[];
  };
  layoutResult: {
    contractVersion: string;
    deckId: string;
    revision: number;
    canvas: { unit: "pt"; width: number; height: number };
    slides: PptPreviewLayoutSlide[];
  };
  assets: Array<{ assetId: string; url: string; expiresIn: number; mimeType: string; altText: string }>;
}

export interface AgentGuideRequest {
  idempotencyKey: string;
  text: string;
  audience?: string;
  scenario?: string;
  language?: string;
  professionalStyle?: string;
  pageCount?: number;
  researchRequired?: boolean;
}

export interface AgentGuideResult {
  clarificationQuestions?: string[];
  state?: AgentPlanningState;
}

export type OutlineEditCommand =
  | { type: "ADD_SLIDE"; afterSlideId?: string; objective: SlideObjective }
  | { type: "DELETE_SLIDE"; slideId: string }
  | { type: "MOVE_SLIDE"; slideId: string; toIndex: number }
  | { type: "UPDATE_SLIDE_OBJECTIVE"; slideId: string; objective: SlideObjective };
