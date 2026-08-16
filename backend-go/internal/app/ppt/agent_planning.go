package ppt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	AgentMinimumPageCount = 6
	AgentMaximumPageCount = 12

	ResearchVerificationUnverified      = "UNVERIFIED"
	ResearchVerificationSourceSupported = "SOURCE_SUPPORTED"
	ResearchVerificationMixed           = "MIXED"
)

var ErrInvalidResearchPack = errors.New("ppt v2 research pack is invalid")

var (
	ErrInvalidStoryline     = errors.New("ppt v2 storyline is invalid")
	ErrInvalidOutlinePlan   = errors.New("ppt v2 outline plan is invalid")
	ErrOutlinePlanApproved  = errors.New("ppt v2 outline plan is already approved")
	ErrOutlineSlideNotFound = errors.New("ppt v2 outline slide is not found")
)

const (
	OutlineCommandAddSlide             = "ADD_SLIDE"
	OutlineCommandDeleteSlide          = "DELETE_SLIDE"
	OutlineCommandMoveSlide            = "MOVE_SLIDE"
	OutlineCommandUpdateSlideObjective = "UPDATE_SLIDE_OBJECTIVE"
)

type IntentRequest struct {
	Text              string
	Audience          string
	Scenario          string
	Language          string
	ProfessionalStyle string
	PageCount         int
	ResearchRequired  *bool
}

type PageCountSpec struct {
	Min       int  `json:"min"`
	Max       int  `json:"max"`
	Preferred int  `json:"preferred,omitempty"`
	Explicit  bool `json:"explicit"`
}

type IntentSpec struct {
	Topic             string        `json:"topic"`
	Goal              string        `json:"goal"`
	Audience          string        `json:"audience"`
	Scenario          string        `json:"scenario"`
	Language          string        `json:"language"`
	PageCount         PageCountSpec `json:"pageCount"`
	ProfessionalStyle string        `json:"professionalStyle"`
	ResearchRequired  bool          `json:"researchRequired"`
}

type IntentResolution struct {
	Intent                 *IntentSpec `json:"intent,omitempty"`
	ClarificationQuestions []string    `json:"clarificationQuestions,omitempty"`
}

type ResearchSource struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider"`
	ProviderIdentity string    `json:"providerIdentity"`
	Title            string    `json:"title"`
	Type             string    `json:"type"`
	Locator          string    `json:"locator"`
	RetrievedAt      time.Time `json:"retrievedAt"`
}

type ResearchCitation struct {
	ID          string    `json:"id"`
	SourceID    string    `json:"sourceId"`
	Locator     string    `json:"locator"`
	RetrievedAt time.Time `json:"retrievedAt"`
}

type ResearchClaim struct {
	ID                 string   `json:"id"`
	SourceID           string   `json:"sourceId"`
	CitationRefs       []string `json:"citationRefs"`
	Text               string   `json:"text"`
	VerificationStatus string   `json:"verificationStatus"`
}

type ResearchDataset struct {
	ID           string   `json:"id"`
	SourceID     string   `json:"sourceId"`
	Title        string   `json:"title"`
	Locator      string   `json:"locator"`
	CitationRefs []string `json:"citationRefs"`
}

type ResearchPack struct {
	Sources            []ResearchSource   `json:"sources"`
	Claims             []ResearchClaim    `json:"claims"`
	Citations          []ResearchCitation `json:"citations"`
	Datasets           []ResearchDataset  `json:"datasets"`
	VerificationStatus string             `json:"verificationStatus"`
}

type StorylineSection struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Objective    string   `json:"objective"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type Storyline struct {
	ID               string             `json:"id"`
	Language         string             `json:"language"`
	Thesis           string             `json:"thesis"`
	AudienceTakeaway string             `json:"audienceTakeaway"`
	NarrativeArc     []string           `json:"narrativeArc"`
	Sections         []StorylineSection `json:"sections"`
	ClosingAction    string             `json:"closingAction"`
	Provenance       PlanningProvenance `json:"provenance"`
}

type EvidenceAssignment struct {
	ClaimID   string `json:"claimId"`
	Rationale string `json:"rationale"`
}

type SlideObjective struct {
	SlideID              string               `json:"slideId"`
	Title                string               `json:"title"`
	Purpose              string               `json:"purpose"`
	KeyMessage           string               `json:"keyMessage"`
	EvidenceRequired     bool                 `json:"evidenceRequired"`
	EvidenceRefs         []string             `json:"evidenceRefs"`
	Evidence             []EvidenceAssignment `json:"evidence"`
	VisualIntent         string               `json:"visualIntent"`
	ExpectedElementTypes []string             `json:"expectedElementTypes"`
}

type OutlinePlan struct {
	ID                string             `json:"id"`
	Revision          int                `json:"revision"`
	Topic             string             `json:"topic"`
	Language          string             `json:"language"`
	PageCount         int                `json:"pageCount"`
	NextSlideSequence int                `json:"nextSlideSequence"`
	Slides            []SlideObjective   `json:"slides"`
	CreatedAt         time.Time          `json:"createdAt"`
	ApprovedAt        time.Time          `json:"approvedAt,omitempty"`
	Provenance        PlanningProvenance `json:"provenance"`
}

type OutlineEditCommand struct {
	Type         string          `json:"type"`
	SlideID      string          `json:"slideId,omitempty"`
	AfterSlideID string          `json:"afterSlideId,omitempty"`
	ToIndex      int             `json:"toIndex,omitempty"`
	Objective    *SlideObjective `json:"objective,omitempty"`
}

func ApplyOutlineCommands(current OutlinePlan, commands []OutlineEditCommand, research ResearchPack) (OutlinePlan, error) {
	if !current.ApprovedAt.IsZero() {
		return OutlinePlan{}, ErrOutlinePlanApproved
	}
	if err := ValidateOutlinePlan(current, research); err != nil {
		return OutlinePlan{}, err
	}
	updated := cloneOutlinePlan(current)
	for _, command := range commands {
		switch strings.TrimSpace(command.Type) {
		case OutlineCommandAddSlide:
			if command.Objective == nil {
				return OutlinePlan{}, ErrInvalidOutlinePlan
			}
			objective := cloneSlideObjective(*command.Objective)
			objective.SlideID = outlineSlideID(updated.ID, updated.NextSlideSequence)
			updated.NextSlideSequence++
			position := len(updated.Slides)
			if strings.TrimSpace(command.AfterSlideID) != "" {
				index := outlineSlideIndex(updated.Slides, command.AfterSlideID)
				if index < 0 {
					return OutlinePlan{}, ErrOutlineSlideNotFound
				}
				position = index + 1
			}
			updated.Slides = append(updated.Slides, SlideObjective{})
			copy(updated.Slides[position+1:], updated.Slides[position:])
			updated.Slides[position] = objective
		case OutlineCommandDeleteSlide:
			index := outlineSlideIndex(updated.Slides, command.SlideID)
			if index < 0 {
				return OutlinePlan{}, ErrOutlineSlideNotFound
			}
			updated.Slides = append(updated.Slides[:index], updated.Slides[index+1:]...)
		case OutlineCommandMoveSlide:
			index := outlineSlideIndex(updated.Slides, command.SlideID)
			if index < 0 {
				return OutlinePlan{}, ErrOutlineSlideNotFound
			}
			if command.ToIndex < 1 || command.ToIndex > len(updated.Slides) {
				return OutlinePlan{}, ErrInvalidOutlinePlan
			}
			objective := updated.Slides[index]
			updated.Slides = append(updated.Slides[:index], updated.Slides[index+1:]...)
			target := command.ToIndex - 1
			updated.Slides = append(updated.Slides, SlideObjective{})
			copy(updated.Slides[target+1:], updated.Slides[target:])
			updated.Slides[target] = objective
		case OutlineCommandUpdateSlideObjective:
			index := outlineSlideIndex(updated.Slides, command.SlideID)
			if index < 0 {
				return OutlinePlan{}, ErrOutlineSlideNotFound
			}
			if command.Objective == nil {
				return OutlinePlan{}, ErrInvalidOutlinePlan
			}
			objective := cloneSlideObjective(*command.Objective)
			objective.SlideID = updated.Slides[index].SlideID
			updated.Slides[index] = objective
		default:
			return OutlinePlan{}, ErrInvalidOutlinePlan
		}
	}
	updated.PageCount = len(updated.Slides)
	updated.Revision++
	if err := ValidateOutlinePlan(updated, research); err != nil {
		return OutlinePlan{}, err
	}
	return updated, nil
}

func ValidateOutlinePlan(plan OutlinePlan, research ResearchPack) error {
	if plan.ID == "" || plan.Revision <= 0 || plan.Topic == "" || plan.Language == "" || plan.PageCount != len(plan.Slides) || plan.PageCount < AgentMinimumPageCount || plan.PageCount > AgentMaximumPageCount || plan.NextSlideSequence <= plan.PageCount || plan.CreatedAt.IsZero() || !validPlanningProvenance(plan.Provenance) {
		return ErrInvalidOutlinePlan
	}
	claimIDs := map[string]struct{}{}
	for _, claim := range research.Claims {
		claimIDs[claim.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, slide := range plan.Slides {
		if slide.SlideID == "" || slide.Title == "" || slide.Purpose == "" || slide.KeyMessage == "" || slide.VisualIntent == "" || len(slide.ExpectedElementTypes) == 0 {
			return ErrInvalidOutlinePlan
		}
		if _, exists := seen[slide.SlideID]; exists {
			return ErrInvalidOutlinePlan
		}
		seen[slide.SlideID] = struct{}{}
		if slide.EvidenceRequired && len(slide.Evidence) == 0 {
			return ErrInvalidEvidenceMapping
		}
		assigned := make(map[string]string, len(slide.Evidence))
		for _, evidence := range slide.Evidence {
			if strings.TrimSpace(evidence.ClaimID) == "" || strings.TrimSpace(evidence.Rationale) == "" {
				return ErrInvalidEvidenceMapping
			}
			if _, exists := claimIDs[evidence.ClaimID]; !exists {
				return ErrInvalidEvidenceMapping
			}
			assigned[evidence.ClaimID] = evidence.Rationale
		}
		for _, evidenceRef := range slide.EvidenceRefs {
			if _, exists := claimIDs[evidenceRef]; !exists {
				return ErrInvalidEvidenceMapping
			}
			if _, exists := assigned[evidenceRef]; !exists {
				return ErrInvalidEvidenceMapping
			}
		}
		if len(assigned) != len(slide.EvidenceRefs) {
			return ErrInvalidEvidenceMapping
		}
	}
	return nil
}

func cloneOutlinePlan(input OutlinePlan) OutlinePlan {
	input.Slides = append([]SlideObjective(nil), input.Slides...)
	for index := range input.Slides {
		input.Slides[index] = cloneSlideObjective(input.Slides[index])
	}
	return input
}

func cloneSlideObjective(input SlideObjective) SlideObjective {
	input.EvidenceRefs = append([]string(nil), input.EvidenceRefs...)
	input.Evidence = append([]EvidenceAssignment(nil), input.Evidence...)
	input.ExpectedElementTypes = append([]string(nil), input.ExpectedElementTypes...)
	return input
}

func outlineSlideID(outlineID string, sequence int) string {
	jobID := strings.TrimSuffix(outlineID, ":outline")
	return jobID + ":objective:" + strconv.Itoa(sequence)
}

func outlineSlideIndex(slides []SlideObjective, slideID string) int {
	slideID = strings.TrimSpace(slideID)
	for index := range slides {
		if slides[index].SlideID == slideID {
			return index
		}
	}
	return -1
}

func shortStableID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func StableResearchSourceID(provider, providerIdentity string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(providerIdentity)))
	return "source_" + hex.EncodeToString(digest[:8])
}

func NormalizeResearchPack(input ResearchPack) (ResearchPack, error) {
	pack := ResearchPack{VerificationStatus: strings.TrimSpace(input.VerificationStatus)}
	sourceIDs := map[string]struct{}{}
	for _, source := range input.Sources {
		source.ID = strings.TrimSpace(source.ID)
		if _, exists := sourceIDs[source.ID]; exists {
			continue
		}
		source.Provider = strings.TrimSpace(source.Provider)
		source.ProviderIdentity = strings.TrimSpace(source.ProviderIdentity)
		source.Title = strings.TrimSpace(source.Title)
		source.Type = strings.TrimSpace(source.Type)
		source.Locator = strings.TrimSpace(source.Locator)
		sourceIDs[source.ID] = struct{}{}
		pack.Sources = append(pack.Sources, source)
	}
	citationIDs := map[string]struct{}{}
	for _, citation := range input.Citations {
		citation.ID = strings.TrimSpace(citation.ID)
		if _, exists := citationIDs[citation.ID]; exists {
			continue
		}
		citation.SourceID = strings.TrimSpace(citation.SourceID)
		citation.Locator = strings.TrimSpace(citation.Locator)
		citationIDs[citation.ID] = struct{}{}
		pack.Citations = append(pack.Citations, citation)
	}
	claimIDs := map[string]struct{}{}
	for _, claim := range input.Claims {
		claim.ID = strings.TrimSpace(claim.ID)
		if _, exists := claimIDs[claim.ID]; exists {
			continue
		}
		claim.SourceID = strings.TrimSpace(claim.SourceID)
		claim.Text = strings.TrimSpace(claim.Text)
		claim.VerificationStatus = strings.TrimSpace(claim.VerificationStatus)
		claim.CitationRefs = normalizedUniqueStrings(claim.CitationRefs)
		claimIDs[claim.ID] = struct{}{}
		pack.Claims = append(pack.Claims, claim)
	}
	datasetIDs := map[string]struct{}{}
	for _, dataset := range input.Datasets {
		dataset.ID = strings.TrimSpace(dataset.ID)
		if _, exists := datasetIDs[dataset.ID]; exists {
			continue
		}
		dataset.SourceID = strings.TrimSpace(dataset.SourceID)
		dataset.Title = strings.TrimSpace(dataset.Title)
		dataset.Locator = strings.TrimSpace(dataset.Locator)
		dataset.CitationRefs = normalizedUniqueStrings(dataset.CitationRefs)
		datasetIDs[dataset.ID] = struct{}{}
		pack.Datasets = append(pack.Datasets, dataset)
	}
	if err := ValidateResearchPack(pack); err != nil {
		return ResearchPack{}, err
	}
	return pack, nil
}

func ValidateResearchPack(pack ResearchPack) error {
	sources := map[string]ResearchSource{}
	for _, source := range pack.Sources {
		if source.ID == "" || source.ID != StableResearchSourceID(source.Provider, source.ProviderIdentity) || source.Title == "" || source.Type == "" || source.Locator == "" || source.RetrievedAt.IsZero() {
			return ErrInvalidResearchPack
		}
		if _, exists := sources[source.ID]; exists {
			return ErrInvalidResearchPack
		}
		sources[source.ID] = source
	}
	citations := map[string]ResearchCitation{}
	for _, citation := range pack.Citations {
		if citation.ID == "" || citation.SourceID == "" || citation.Locator == "" || citation.RetrievedAt.IsZero() {
			return ErrInvalidResearchPack
		}
		if _, exists := sources[citation.SourceID]; !exists {
			return ErrInvalidResearchPack
		}
		if _, exists := citations[citation.ID]; exists {
			return ErrInvalidResearchPack
		}
		citations[citation.ID] = citation
	}
	claims := map[string]struct{}{}
	for _, claim := range pack.Claims {
		if claim.ID == "" || claim.SourceID == "" || claim.Text == "" || !validResearchVerification(claim.VerificationStatus) || len(claim.CitationRefs) == 0 {
			return ErrInvalidResearchPack
		}
		if _, exists := sources[claim.SourceID]; !exists {
			return ErrInvalidResearchPack
		}
		if _, exists := claims[claim.ID]; exists {
			return ErrInvalidResearchPack
		}
		claims[claim.ID] = struct{}{}
		for _, citationID := range claim.CitationRefs {
			citation, exists := citations[citationID]
			if !exists || citation.SourceID != claim.SourceID {
				return ErrInvalidResearchPack
			}
		}
	}
	for _, dataset := range pack.Datasets {
		if dataset.ID == "" || dataset.SourceID == "" || dataset.Title == "" || dataset.Locator == "" || len(dataset.CitationRefs) == 0 {
			return ErrInvalidResearchPack
		}
		if _, exists := sources[dataset.SourceID]; !exists {
			return ErrInvalidResearchPack
		}
		for _, citationID := range dataset.CitationRefs {
			citation, exists := citations[citationID]
			if !exists || citation.SourceID != dataset.SourceID {
				return ErrInvalidResearchPack
			}
		}
	}
	if len(pack.Claims) > 0 && !validResearchVerification(pack.VerificationStatus) {
		return ErrInvalidResearchPack
	}
	return nil
}

func validResearchVerification(value string) bool {
	return value == ResearchVerificationUnverified || value == ResearchVerificationSourceSupported || value == ResearchVerificationMixed
}

func normalizedUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

var (
	agentPageCountPattern = regexp.MustCompile(`(?i)(\d{1,2})\s*(?:[-~～—到至]\s*(\d{1,2}))?\s*页(?:左右|上下|以内)?`)
	agentAudiencePattern  = regexp.MustCompile(`(?:给|面向)([^，。,；;]+?)(?:看|汇报|使用|阅读|$|[，。,；;])`)
)

func InterpretAgentIntent(request IntentRequest) IntentResolution {
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return criticalTopicClarification()
	}
	pageCount, pageQuestion := agentPageCount(text, request.PageCount)
	if pageQuestion != "" {
		return IntentResolution{ClarificationQuestions: []string{pageQuestion}}
	}
	topic := agentTopic(text)
	if topic == "" || genericPPTTopic(topic) {
		return criticalTopicClarification()
	}
	audience := strings.TrimSpace(request.Audience)
	if audience == "" {
		audience = agentAudience(text)
	}
	if audience == "" {
		audience = "专业决策者"
	}
	scenario := strings.TrimSpace(request.Scenario)
	if scenario == "" {
		scenario = agentScenario(text, audience)
	}
	language := strings.TrimSpace(request.Language)
	if language == "" {
		language = agentLanguage(text)
	} else {
		language = canonicalAgentLanguage(language, text)
	}
	style := strings.TrimSpace(request.ProfessionalStyle)
	if style == "" {
		style = agentProfessionalStyle(text)
	}
	researchRequired := agentResearchRequired(text)
	if request.ResearchRequired != nil {
		researchRequired = *request.ResearchRequired
	}
	intent := IntentSpec{
		Topic: topic, Goal: agentGoal(text), Audience: audience, Scenario: scenario,
		Language: language, PageCount: pageCount, ProfessionalStyle: style,
		ResearchRequired: researchRequired,
	}
	return IntentResolution{Intent: &intent}
}

func canonicalAgentLanguage(language, text string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	switch {
	case normalized == "zh" || strings.HasPrefix(normalized, "zh-"):
		return "zh-CN"
	case normalized == "en" || strings.HasPrefix(normalized, "en-"):
		return "en-US"
	default:
		return agentLanguage(text)
	}
}

func criticalTopicClarification() IntentResolution {
	return IntentResolution{ClarificationQuestions: []string{"这份演示文稿需要围绕什么明确主题？"}}
}

func agentPageCount(text string, override int) (PageCountSpec, string) {
	match := agentPageCountPattern.FindStringSubmatch(text)
	if len(match) == 0 {
		if override > 0 {
			if override < AgentMinimumPageCount || override > AgentMaximumPageCount {
				return PageCountSpec{}, "Professional Deck 当前支持 6～12 页，请确认页数。"
			}
			return PageCountSpec{Min: override, Max: override, Preferred: override, Explicit: true}, ""
		}
		return PageCountSpec{Min: AgentMinimumPageCount, Max: AgentMaximumPageCount}, ""
	}
	minimum, _ := strconv.Atoi(match[1])
	maximum := minimum
	if len(match) > 2 && match[2] != "" {
		maximum, _ = strconv.Atoi(match[2])
	}
	if minimum > maximum {
		minimum, maximum = maximum, minimum
	}
	if minimum < AgentMinimumPageCount || maximum > AgentMaximumPageCount {
		return PageCountSpec{}, "Professional Deck 当前支持 6～12 页，请确认页数。"
	}
	return PageCountSpec{Min: minimum, Max: maximum, Preferred: (minimum + maximum + 1) / 2, Explicit: true}, ""
}

func agentTopic(text string) string {
	value := agentPageCountPattern.ReplaceAllString(text, "")
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"请帮我做一份", "帮我做一份", "请做一份", "做一份", "请制作一份", "制作一份", "请介绍", "介绍"} {
		value = strings.TrimPrefix(value, prefix)
	}
	value = strings.TrimPrefix(strings.TrimSpace(value), "的")
	if index := strings.IndexAny(value, "，。,；;"); index >= 0 {
		value = value[:index]
	}
	if index := strings.Index(value, "给"); index > 0 {
		value = value[:index]
	}
	value = strings.TrimSpace(strings.Trim(value, "-_—：:,.，。"))
	return value
}

func genericPPTTopic(topic string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(topic), " ", ""), "演示文稿", "ppt"))
	for _, value := range []string{"ppt", "专业ppt", "商务ppt", "一个ppt", "一份ppt"} {
		if normalized == value {
			return true
		}
	}
	return false
}

func agentAudience(text string) string {
	match := agentAudiencePattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func agentScenario(text, audience string) string {
	value := strings.ToLower(text + " " + audience)
	switch {
	case strings.Contains(value, "投资") || strings.Contains(value, "路演"):
		return "investment-pitch"
	case strings.Contains(value, "管理层") || strings.Contains(value, "汇报"):
		return "management-report"
	case strings.Contains(value, "客户") || strings.Contains(value, "产品"):
		return "customer-presentation"
	case strings.Contains(value, "总结"):
		return "work-summary"
	default:
		return "professional-presentation"
	}
}

func agentGoal(text string) string {
	value := strings.ToLower(text)
	switch {
	case strings.Contains(value, "行业") || strings.Contains(value, "市场") || strings.Contains(value, "趋势"):
		return "industry-analysis"
	case strings.Contains(value, "投资") || strings.Contains(value, "战略"):
		return "decision-support"
	case strings.Contains(value, "产品") || strings.Contains(value, "介绍"):
		return "product-introduction"
	case strings.Contains(value, "方案") || strings.Contains(value, "项目"):
		return "proposal"
	case strings.Contains(value, "总结"):
		return "work-summary"
	default:
		return "professional-overview"
	}
}

func agentLanguage(text string) string {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return "zh-CN"
		}
	}
	return "en-US"
}

func agentProfessionalStyle(text string) string {
	value := strings.ToLower(text)
	switch {
	case strings.Contains(value, "科技") || strings.Contains(value, "ai"):
		return "professional-technology"
	case strings.Contains(value, "简约"):
		return "professional-minimal"
	default:
		return "professional-business"
	}
}

func agentResearchRequired(text string) bool {
	value := strings.ToLower(text)
	for _, marker := range []string{"行业", "市场", "趋势", "分析", "数据", "投资", "研究", "research", "market", "trend", "data"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
