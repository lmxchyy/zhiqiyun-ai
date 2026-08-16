export type AgentPlanningStage =
  | "CREATED"
  | "INTENT_RESOLVED"
  | "RESEARCHED"
  | "STORYLINE_PLANNED"
  | "OUTLINE_PLANNED"
  | "OUTLINE_APPROVED";

export type AgentPlanningStatus = "QUEUED" | "RUNNING" | "RETRY_WAIT" | "FAILED" | "CANCELLED" | "WAITING_FOR_OUTLINE_APPROVAL";

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
