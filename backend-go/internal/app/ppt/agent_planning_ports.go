package ppt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	PlanningModeAI                = "AI"
	PlanningModeDeterministicTest = "DETERMINISTIC_TEST"

	PlanningProviderUnavailable      = "planning_provider_unavailable"
	PlanningTimeout                  = "planning_timeout"
	PlanningInvalidOutput            = "planning_invalid_output"
	PlanningContractValidationFailed = "planning_contract_validation_failed"
	PlanningEvidenceMappingInvalid   = "planning_evidence_mapping_invalid"
	ResearchProviderUnavailable      = "research_provider_unavailable"
	ResearchTimeout                  = "research_timeout"
	ResearchInvalidResult            = "research_invalid_result"
	ResearchContractValidationFailed = "research_contract_validation_failed"
)

var ErrInvalidEvidenceMapping = errors.New("ppt v2 planning evidence mapping is invalid")

type PlanningProvenance struct {
	Mode              string `json:"mode"`
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	ProviderRequestID string `json:"providerRequestId,omitempty"`
}

type StorylineSectionDraft struct {
	Key          string   `json:"key"`
	Title        string   `json:"title"`
	Objective    string   `json:"objective"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type StorylineDraft struct {
	Language         string                  `json:"language"`
	Thesis           string                  `json:"thesis"`
	AudienceTakeaway string                  `json:"audienceTakeaway"`
	NarrativeArc     []string                `json:"narrativeArc"`
	Sections         []StorylineSectionDraft `json:"sections"`
	ClosingAction    string                  `json:"closingAction"`
}

type SlideObjectiveDraft struct {
	Title                string               `json:"title"`
	Purpose              string               `json:"purpose"`
	KeyMessage           string               `json:"keyMessage"`
	EvidenceRequired     bool                 `json:"evidenceRequired"`
	Evidence             []EvidenceAssignment `json:"evidence"`
	VisualIntent         string               `json:"visualIntent"`
	ExpectedElementTypes []string             `json:"expectedElementTypes"`
}

type OutlinePlanDraft struct {
	Language string                `json:"language"`
	Slides   []SlideObjectiveDraft `json:"slides"`
}

type StorylinePlanningInput struct {
	Intent   IntentSpec
	Research ResearchPack
}

type StorylinePlanningOutput struct {
	Draft      StorylineDraft
	Provenance PlanningProvenance
}

type OutlinePlanningInput struct {
	Intent    IntentSpec
	Research  ResearchPack
	Storyline Storyline
}

type OutlinePlanningOutput struct {
	Draft      OutlinePlanDraft
	Provenance PlanningProvenance
}

type StorylinePlanningPort interface {
	PlanStoryline(context.Context, StorylinePlanningInput) (StorylinePlanningOutput, error)
}

type OutlinePlanningPort interface {
	PlanOutline(context.Context, OutlinePlanningInput) (OutlinePlanningOutput, error)
}

type AgentWorkflowError struct {
	Code              string
	SafeMessage       string
	Retryable         bool
	Provider          string
	ProviderRequestID string
	Cause             error
}

func NewAgentWorkflowError(code, safeMessage string, retryable bool, cause error) *AgentWorkflowError {
	return &AgentWorkflowError{Code: strings.TrimSpace(code), SafeMessage: strings.TrimSpace(safeMessage), Retryable: retryable, Cause: cause}
}

func (e *AgentWorkflowError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return e.Code
}

func (e *AgentWorkflowError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func MaterializeStoryline(intent IntentSpec, research ResearchPack, output StorylinePlanningOutput) (Storyline, error) {
	if err := ValidateResearchPack(research); err != nil {
		return Storyline{}, err
	}
	sections := make([]StorylineSection, 0, len(output.Draft.Sections))
	for index, draft := range output.Draft.Sections {
		sectionID := strings.TrimSpace(draft.Key)
		if sectionID == "" {
			sectionID = fmt.Sprintf("section_%d", index+1)
		}
		sections = append(sections, StorylineSection{
			ID: sectionID, Title: strings.TrimSpace(draft.Title), Objective: strings.TrimSpace(draft.Objective),
			EvidenceRefs: normalizedUniqueStrings(draft.EvidenceRefs),
		})
	}
	storyline := Storyline{
		ID: "storyline_" + shortStableID(intent.Topic+"\x00"+intent.Audience), Language: strings.TrimSpace(output.Draft.Language),
		Thesis: strings.TrimSpace(output.Draft.Thesis), AudienceTakeaway: strings.TrimSpace(output.Draft.AudienceTakeaway),
		NarrativeArc: normalizedUniqueStrings(output.Draft.NarrativeArc), Sections: sections,
		ClosingAction: strings.TrimSpace(output.Draft.ClosingAction), Provenance: output.Provenance,
	}
	if err := ValidateStoryline(storyline, intent, research); err != nil {
		return Storyline{}, err
	}
	return storyline, nil
}

func ValidateStoryline(storyline Storyline, intent IntentSpec, research ResearchPack) error {
	if storyline.ID == "" || storyline.Language != intent.Language || storyline.Thesis == "" || storyline.AudienceTakeaway == "" || storyline.ClosingAction == "" || len(storyline.Sections) == 0 || len(storyline.NarrativeArc) != len(storyline.Sections) || !validPlanningProvenance(storyline.Provenance) {
		return ErrInvalidStoryline
	}
	claimIDs := make(map[string]struct{}, len(research.Claims))
	for _, claim := range research.Claims {
		claimIDs[claim.ID] = struct{}{}
	}
	seenSections := make(map[string]struct{}, len(storyline.Sections))
	for index, section := range storyline.Sections {
		if section.ID == "" || section.Title == "" || section.Objective == "" || storyline.NarrativeArc[index] != section.ID {
			return ErrInvalidStoryline
		}
		if _, exists := seenSections[section.ID]; exists {
			return ErrInvalidStoryline
		}
		seenSections[section.ID] = struct{}{}
		for _, claimID := range section.EvidenceRefs {
			if _, exists := claimIDs[claimID]; !exists {
				return ErrInvalidStoryline
			}
		}
	}
	return nil
}

func MaterializeOutlinePlan(jobID string, intent IntentSpec, research ResearchPack, storyline Storyline, output OutlinePlanningOutput, now time.Time) (OutlinePlan, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" || output.Draft.Language != intent.Language || !pageCountMatchesIntent(len(output.Draft.Slides), intent.PageCount) {
		return OutlinePlan{}, ErrInvalidOutlinePlan
	}
	if err := ValidateStoryline(storyline, intent, research); err != nil {
		return OutlinePlan{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	plan := OutlinePlan{
		ID: jobID + "_outline", Revision: 1, Topic: intent.Topic, Language: intent.Language,
		PageCount: len(output.Draft.Slides), NextSlideSequence: len(output.Draft.Slides) + 1,
		CreatedAt: now, Provenance: output.Provenance,
	}
	for index, draft := range output.Draft.Slides {
		evidence := append([]EvidenceAssignment(nil), draft.Evidence...)
		refs := make([]string, 0, len(evidence))
		for evidenceIndex := range evidence {
			evidence[evidenceIndex].ClaimID = strings.TrimSpace(evidence[evidenceIndex].ClaimID)
			evidence[evidenceIndex].Rationale = strings.TrimSpace(evidence[evidenceIndex].Rationale)
			refs = append(refs, evidence[evidenceIndex].ClaimID)
		}
		plan.Slides = append(plan.Slides, SlideObjective{
			SlideID: fmt.Sprintf("%s_objective_%d", jobID, index+1), Title: strings.TrimSpace(draft.Title),
			Purpose: strings.TrimSpace(draft.Purpose), KeyMessage: strings.TrimSpace(draft.KeyMessage),
			EvidenceRequired: draft.EvidenceRequired, EvidenceRefs: normalizedUniqueStrings(refs), Evidence: evidence,
			VisualIntent: strings.TrimSpace(draft.VisualIntent), ExpectedElementTypes: normalizedUniqueStrings(draft.ExpectedElementTypes),
		})
	}
	if err := ValidateOutlinePlan(plan, research); err != nil {
		return OutlinePlan{}, err
	}
	return plan, nil
}

func validPlanningProvenance(value PlanningProvenance) bool {
	if value.Mode != PlanningModeAI && value.Mode != PlanningModeDeterministicTest {
		return false
	}
	return strings.TrimSpace(value.Provider) != "" && strings.TrimSpace(value.Model) != ""
}

func pageCountMatchesIntent(pageCount int, spec PageCountSpec) bool {
	if pageCount < AgentMinimumPageCount || pageCount > AgentMaximumPageCount {
		return false
	}
	if spec.Explicit && spec.Min == spec.Max {
		return pageCount == spec.Preferred
	}
	return pageCount >= spec.Min && pageCount <= spec.Max
}
