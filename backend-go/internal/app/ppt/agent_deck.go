package ppt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ContentProviderUnavailable      = "content_provider_unavailable"
	ContentTimeout                  = "content_timeout"
	ContentInvalidOutput            = "content_invalid_output"
	ContentContractValidationFailed = "content_contract_validation_failed"
	ContentEvidenceMappingInvalid   = "content_evidence_mapping_invalid"
	ImageProviderUnavailable        = "image_provider_unavailable"
	ImageTimeout                    = "image_timeout"
	ImageInvalidResult              = "image_invalid_result"
	ImageStorageFailed              = "image_storage_failed"
	LayoutCompilationFailed         = "layout_compilation_failed"
	QualityGateFailed               = "quality_gate_failed"
	PPTXRenderFailed                = "pptx_render_failed"
	ArtifactStorageFailed           = "artifact_storage_failed"
	ArtifactRelationFailed          = "artifact_relation_failed"
)

var (
	ErrInvalidSlideContent         = errors.New("ppt v2 slide content is invalid")
	ErrInvalidSlideContentEvidence = errors.New("ppt v2 slide content evidence mapping is invalid")
)

type SlideBodyBlock struct {
	Heading string `json:"heading"`
	Text    string `json:"text"`
}

type SlideAssetIntent struct {
	ID       string `json:"id"`
	StableID string `json:"stableId,omitempty"`
	Kind     string `json:"kind"`
	Prompt   string `json:"prompt"`
	AltText  string `json:"altText"`
}

type SlideContentDraft struct {
	Language       string             `json:"language"`
	Title          string             `json:"title"`
	Subtitle       string             `json:"subtitle,omitempty"`
	BodyBlocks     []SlideBodyBlock   `json:"bodyBlocks"`
	Bullets        []string           `json:"bullets"`
	SupportingText string             `json:"supportingText"`
	SpeakerNotes   string             `json:"speakerNotes"`
	AssetIntents   []SlideAssetIntent `json:"assetIntents"`
	CitationRefs   []string           `json:"citationRefs"`
	LayoutHint     string             `json:"layoutHint"`
}

type SlideContent struct {
	SlideID        string             `json:"slideId"`
	Language       string             `json:"language"`
	Title          string             `json:"title"`
	Subtitle       string             `json:"subtitle,omitempty"`
	BodyBlocks     []SlideBodyBlock   `json:"bodyBlocks"`
	Bullets        []string           `json:"bullets"`
	SupportingText string             `json:"supportingText"`
	SpeakerNotes   string             `json:"speakerNotes"`
	AssetIntents   []SlideAssetIntent `json:"assetIntents"`
	CitationRefs   []string           `json:"citationRefs"`
	LayoutHint     string             `json:"layoutHint"`
	Provenance     PlanningProvenance `json:"provenance"`
}

type SlideContentPlanningInput struct {
	Intent          IntentSpec     `json:"intent"`
	Research        ResearchPack   `json:"research"`
	Storyline       Storyline      `json:"storyline"`
	ApprovedOutline OutlinePlan    `json:"approvedOutline"`
	Objective       SlideObjective `json:"objective"`
}

type SlideContentPlanningOutput struct {
	Draft      SlideContentDraft
	Provenance PlanningProvenance
}

type SlideContentPlanningPort interface {
	PlanSlideContent(context.Context, SlideContentPlanningInput) (SlideContentPlanningOutput, error)
}

type ResolvedDeckAsset struct {
	ID       string `json:"id"`
	IntentID string `json:"intentId"`
	SlideID  string `json:"slideId"`
	MIMEType string `json:"mimeType"`
	URI      string `json:"uri"`
	SHA256   string `json:"sha256"`
	FileID   string `json:"fileId"`
	AltText  string `json:"altText"`
	TenantID string `json:"tenantId"`
	UserID   string `json:"userId"`
}

type DeckCompilation struct {
	DeckID        string          `json:"deckId"`
	Revision      int             `json:"revision"`
	SlideCount    int             `json:"slideCount"`
	Deck          json.RawMessage `json:"deck"`
	LayoutResult  json.RawMessage `json:"layoutResult"`
	RenderInput   json.RawMessage `json:"renderInput"`
	QualityValid  bool            `json:"qualityValid"`
	QualityIssues []string        `json:"qualityIssues"`
}

type AgentDeckGenerationState struct {
	Contents          []SlideContent      `json:"contents"`
	Assets            []ResolvedDeckAsset `json:"assets"`
	Compilation       *DeckCompilation    `json:"compilation,omitempty"`
	ContentExecutions int                 `json:"contentExecutions"`
	AssetExecutions   int                 `json:"assetExecutions"`
	LayoutExecutions  int                 `json:"layoutExecutions"`
	RenderExecutions  int                 `json:"renderExecutions"`
}

type DeckBuildInput struct {
	GenerationJobID string              `json:"generationJobId"`
	Revision        int                 `json:"revision"`
	Intent          IntentSpec          `json:"intent"`
	Research        ResearchPack        `json:"research"`
	Storyline       Storyline           `json:"storyline"`
	ApprovedOutline OutlinePlan         `json:"approvedOutline"`
	SlideContents   []SlideContent      `json:"slideContents"`
	Assets          []ResolvedDeckAsset `json:"assets"`
}

type DeckRenderOutput struct {
	DeckID     string
	Revision   int
	SlideCount int
	PPTX       []byte
}

type AgentDeckAssetPort interface {
	ResolveImage(context.Context, GenerationJobScope, string, string, SlideAssetIntent) (ResolvedDeckAsset, error)
}

type AgentDeckCompilerPort interface {
	Compile(context.Context, DeckBuildInput) (DeckCompilation, error)
	Render(context.Context, DeckCompilation, []ResolvedDeckAsset) (DeckRenderOutput, error)
}

type AgentDeckArtifactPort interface {
	EnsureTask(context.Context, GenerationJobScope, string, IntentSpec, OutlinePlan, []SlideContent) (string, error)
	StorePPTX(context.Context, GenerationJobScope, string, string, []byte) (string, error)
	EnsureArtifact(context.Context, GenerationLease, string, string, string, int) (string, GenerationJob, error)
	RelateTask(context.Context, GenerationLease, string, V2ArtifactRelation) (GenerationJob, error)
}

type AgentDeckCheckpoint struct {
	ExpectedStage      string
	NextStage          string
	State              AgentDeckGenerationState
	CompletedWorkUnits int
	DeckID             string
	Revision           int
	RenderSHA256       string
	RenderBytes        []byte
	ExistingTaskID     string
	FileID             string
	AssetID            string
	Now                time.Time
}

var supportedProfessionalLayouts = map[string]struct{}{
	"cover": {}, "section": {}, "title-body": {}, "title-bullets": {}, "two-column": {},
	"text-image": {}, "image-text": {}, "key-metric": {}, "closing-action": {},
}

func MaterializeSlideContent(input SlideContentPlanningInput, output SlideContentPlanningOutput) (SlideContent, error) {
	if input.ApprovedOutline.ApprovedAt.IsZero() || input.Objective.SlideID == "" || input.Intent.Language == "" || output.Draft.Language != input.Intent.Language || !validPlanningProvenance(output.Provenance) {
		return SlideContent{}, ErrInvalidSlideContent
	}
	if err := ValidateResearchPack(input.Research); err != nil {
		return SlideContent{}, err
	}
	if err := ValidateStoryline(input.Storyline, input.Intent, input.Research); err != nil {
		return SlideContent{}, err
	}
	if err := ValidateOutlinePlan(input.ApprovedOutline, input.Research); err != nil {
		return SlideContent{}, err
	}
	if !outlineContainsObjective(input.ApprovedOutline, input.Objective) {
		return SlideContent{}, ErrInvalidSlideContent
	}

	draft := output.Draft
	content := SlideContent{
		SlideID: input.Objective.SlideID, Language: strings.TrimSpace(draft.Language),
		Title: strings.TrimSpace(draft.Title), Subtitle: strings.TrimSpace(draft.Subtitle),
		SupportingText: strings.TrimSpace(draft.SupportingText), SpeakerNotes: strings.TrimSpace(draft.SpeakerNotes),
		CitationRefs: normalizedUniqueStrings(draft.CitationRefs), LayoutHint: strings.ToLower(strings.TrimSpace(draft.LayoutHint)),
		Provenance: output.Provenance,
	}
	for _, block := range draft.BodyBlocks {
		content.BodyBlocks = append(content.BodyBlocks, SlideBodyBlock{Heading: strings.TrimSpace(block.Heading), Text: strings.TrimSpace(block.Text)})
	}
	content.Bullets = normalizedUniqueStrings(draft.Bullets)
	for index, intent := range draft.AssetIntents {
		intent.ID = strings.TrimSpace(intent.ID)
		intent.Kind = strings.ToLower(strings.TrimSpace(intent.Kind))
		intent.Prompt = strings.TrimSpace(intent.Prompt)
		intent.AltText = strings.TrimSpace(intent.AltText)
		intent.StableID = fmt.Sprintf("asset_intent_%s_%s", shortStableID(input.Objective.SlideID), shortStableID(intent.ID+"\x00"+intent.Prompt))
		if intent.ID == "" {
			intent.StableID = fmt.Sprintf("asset_intent_%s_%d", shortStableID(input.Objective.SlideID), index+1)
		}
		content.AssetIntents = append(content.AssetIntents, intent)
	}
	if err := ValidateSlideContent(content, input.Objective, input.Research); err != nil {
		return SlideContent{}, err
	}
	return content, nil
}

func ValidateSlideContent(content SlideContent, objective SlideObjective, research ResearchPack) error {
	if content.SlideID != objective.SlideID || content.Language == "" || content.Title == "" || content.SupportingText == "" || content.SpeakerNotes == "" || !validPlanningProvenance(content.Provenance) {
		return ErrInvalidSlideContent
	}
	if _, exists := supportedProfessionalLayouts[content.LayoutHint]; !exists {
		return ErrInvalidSlideContent
	}
	if len(content.BodyBlocks) == 0 && len(content.Bullets) == 0 && content.LayoutHint != "cover" {
		return ErrInvalidSlideContent
	}
	if !sameStringSet(content.CitationRefs, objective.EvidenceRefs) {
		return ErrInvalidSlideContentEvidence
	}
	claimIDs := make(map[string]struct{}, len(research.Claims))
	for _, claim := range research.Claims {
		claimIDs[claim.ID] = struct{}{}
	}
	for _, ref := range content.CitationRefs {
		if _, ok := claimIDs[ref]; !ok {
			return ErrInvalidSlideContentEvidence
		}
	}
	if objective.EvidenceRequired && len(content.CitationRefs) == 0 {
		return ErrInvalidSlideContentEvidence
	}
	requiresImage := containsString(objective.ExpectedElementTypes, "IMAGE")
	usesImageLayout := content.LayoutHint == "text-image" || content.LayoutHint == "image-text"
	if requiresImage != usesImageLayout {
		return ErrInvalidSlideContent
	}
	if requiresImage && len(content.AssetIntents) != 1 {
		return ErrInvalidSlideContent
	}
	if !requiresImage && len(content.AssetIntents) > 0 {
		return ErrInvalidSlideContent
	}
	seen := map[string]struct{}{}
	for _, intent := range content.AssetIntents {
		if intent.StableID == "" || intent.Kind != "image" || intent.Prompt == "" || intent.AltText == "" {
			return ErrInvalidSlideContent
		}
		if _, exists := seen[intent.StableID]; exists {
			return ErrInvalidSlideContent
		}
		seen[intent.StableID] = struct{}{}
	}
	return nil
}

func outlineContainsObjective(outline OutlinePlan, objective SlideObjective) bool {
	for _, candidate := range outline.Slides {
		if candidate.SlideID == objective.SlideID {
			return candidate.Title == objective.Title && candidate.Purpose == objective.Purpose && candidate.KeyMessage == objective.KeyMessage && sameStringSet(candidate.EvidenceRefs, objective.EvidenceRefs)
		}
	}
	return false
}

func sameStringSet(left, right []string) bool {
	left = normalizedUniqueStrings(left)
	right = normalizedUniqueStrings(right)
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, item := range left {
		values[item] = struct{}{}
	}
	for _, item := range right {
		if _, ok := values[item]; !ok {
			return false
		}
	}
	return true
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), wanted) {
			return true
		}
	}
	return false
}

func agentDeckImageCount(outline OutlinePlan) int {
	count := 0
	for _, objective := range outline.Slides {
		if containsString(objective.ExpectedElementTypes, "IMAGE") {
			count++
		}
	}
	return count
}

func agentDeckTotalWorkUnits(outline OutlinePlan) int {
	return 8 + 2*len(outline.Slides) + agentDeckImageCount(outline)
}

func cloneAgentDeckGenerationState(input AgentDeckGenerationState) AgentDeckGenerationState {
	input.Contents = append([]SlideContent(nil), input.Contents...)
	for index := range input.Contents {
		input.Contents[index].BodyBlocks = append([]SlideBodyBlock(nil), input.Contents[index].BodyBlocks...)
		input.Contents[index].Bullets = append([]string(nil), input.Contents[index].Bullets...)
		input.Contents[index].AssetIntents = append([]SlideAssetIntent(nil), input.Contents[index].AssetIntents...)
		input.Contents[index].CitationRefs = append([]string(nil), input.Contents[index].CitationRefs...)
	}
	input.Assets = append([]ResolvedDeckAsset(nil), input.Assets...)
	if input.Compilation != nil {
		copyValue := *input.Compilation
		copyValue.Deck = append(json.RawMessage(nil), copyValue.Deck...)
		copyValue.LayoutResult = append(json.RawMessage(nil), copyValue.LayoutResult...)
		copyValue.RenderInput = append(json.RawMessage(nil), copyValue.RenderInput...)
		copyValue.QualityIssues = append([]string(nil), copyValue.QualityIssues...)
		input.Compilation = &copyValue
	}
	return input
}
