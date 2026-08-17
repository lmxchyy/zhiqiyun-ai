package ppt

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMaterializeSlideContentPreservesLanguageEvidenceAndImageIntent(t *testing.T) {
	intent, research, storyline, outline := agentDeckFixture(t)
	objective := outline.Slides[2]
	content, err := MaterializeSlideContent(SlideContentPlanningInput{
		Intent: intent, Research: research, Storyline: storyline, ApprovedOutline: outline, Objective: objective,
	}, SlideContentPlanningOutput{Draft: SlideContentDraft{
		Language: intent.Language, Title: objective.Title, BodyBlocks: []SlideBodyBlock{{Heading: "市场证据", Text: objective.KeyMessage}},
		Bullets: []string{"需求持续增长", "需要明确进入顺序"}, SupportingText: objective.KeyMessage,
		SpeakerNotes: "来源见研究报告。", CitationRefs: append([]string(nil), objective.EvidenceRefs...), LayoutHint: "text-image",
		AssetIntents: []SlideAssetIntent{{ID: "hero", Kind: "image", Prompt: "新能源汽车市场专业摄影", AltText: "新能源汽车市场"}},
	}, Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "openai-compatible", Model: "test-model"}})
	if err != nil {
		t.Fatal(err)
	}
	if content.SlideID != objective.SlideID || content.Language != "zh-CN" || len(content.CitationRefs) != 1 || content.CitationRefs[0] != objective.EvidenceRefs[0] {
		t.Fatalf("content lost approved semantics: %+v", content)
	}
	if len(content.AssetIntents) != 1 || content.AssetIntents[0].StableID == "" {
		t.Fatalf("image intent was not materialized: %+v", content.AssetIntents)
	}
}

func TestMaterializeSlideContentFailsClosedOnEvidenceOrProviderContractMismatch(t *testing.T) {
	intent, research, storyline, outline := agentDeckFixture(t)
	objective := outline.Slides[1]
	base := SlideContentPlanningOutput{Draft: SlideContentDraft{
		Language: intent.Language, Title: objective.Title, BodyBlocks: []SlideBodyBlock{{Heading: "判断", Text: objective.KeyMessage}},
		Bullets: []string{"证据"}, SupportingText: objective.KeyMessage, SpeakerNotes: "来源说明", CitationRefs: append([]string(nil), objective.EvidenceRefs...), LayoutHint: "title-bullets",
	}, Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "openai-compatible", Model: "test-model"}}

	missingEvidence := base
	missingEvidence.Draft.CitationRefs = nil
	if _, err := MaterializeSlideContent(SlideContentPlanningInput{Intent: intent, Research: research, Storyline: storyline, ApprovedOutline: outline, Objective: objective}, missingEvidence); !errors.Is(err, ErrInvalidSlideContentEvidence) {
		t.Fatalf("expected evidence failure, got %v", err)
	}
	wrongLanguage := base
	wrongLanguage.Draft.Language = "en-US"
	if _, err := MaterializeSlideContent(SlideContentPlanningInput{Intent: intent, Research: research, Storyline: storyline, ApprovedOutline: outline, Objective: objective}, wrongLanguage); !errors.Is(err, ErrInvalidSlideContent) {
		t.Fatalf("expected language contract failure, got %v", err)
	}
	imageLayoutWithoutApprovedImage := base
	imageLayoutWithoutApprovedImage.Draft.LayoutHint = "text-image"
	if _, err := MaterializeSlideContent(SlideContentPlanningInput{Intent: intent, Research: research, Storyline: storyline, ApprovedOutline: outline, Objective: objective}, imageLayoutWithoutApprovedImage); !errors.Is(err, ErrInvalidSlideContent) {
		t.Fatalf("expected unapproved image layout contract failure, got %v", err)
	}

	imageObjective := outline.Slides[2]
	imageObjectiveWithoutImageLayout := SlideContentPlanningOutput{Draft: SlideContentDraft{
		Language: intent.Language, Title: imageObjective.Title, BodyBlocks: []SlideBodyBlock{{Heading: "证据", Text: imageObjective.KeyMessage}},
		Bullets: []string{"证据"}, SupportingText: imageObjective.KeyMessage, SpeakerNotes: "来源说明",
		CitationRefs: append([]string(nil), imageObjective.EvidenceRefs...), LayoutHint: "title-body",
		AssetIntents: []SlideAssetIntent{{ID: "hero", Kind: "image", Prompt: "新能源汽车市场", AltText: "新能源汽车市场"}},
	}, Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "openai-compatible", Model: "test-model"}}
	if _, err := MaterializeSlideContent(SlideContentPlanningInput{Intent: intent, Research: research, Storyline: storyline, ApprovedOutline: outline, Objective: imageObjective}, imageObjectiveWithoutImageLayout); !errors.Is(err, ErrInvalidSlideContent) {
		t.Fatalf("expected approved image objective to require an image layout, got %v", err)
	}
}

func TestSlideContentPlanningPortContract(t *testing.T) {
	var _ SlideContentPlanningPort = slideContentPlanningFixture{}
	_, err := slideContentPlanningFixture{}.PlanSlideContent(context.Background(), SlideContentPlanningInput{})
	if err == nil {
		t.Fatal("fixture must reject an empty input")
	}
}

type slideContentPlanningFixture struct{}

func (slideContentPlanningFixture) PlanSlideContent(_ context.Context, input SlideContentPlanningInput) (SlideContentPlanningOutput, error) {
	if input.Objective.SlideID == "" {
		return SlideContentPlanningOutput{}, ErrInvalidSlideContent
	}
	return SlideContentPlanningOutput{}, nil
}

func agentDeckFixture(t *testing.T) (IntentSpec, ResearchPack, Storyline, OutlinePlan) {
	t.Helper()
	intent := IntentSpec{Topic: "新能源汽车行业", Goal: "industry-analysis", Audience: "公司管理层", Scenario: "management-report", Language: "zh-CN", PageCount: PageCountSpec{Min: 8, Max: 8, Preferred: 8, Explicit: true}, ProfessionalStyle: "professional-business", ResearchRequired: true}
	research := agentResearchFixture(t)
	storyline := agentStorylineFixture(t, intent, research)
	outline := agentOutlineFixture(t, "pptv2_job_deck", intent, research, storyline, time.Now().UTC())
	outline.ApprovedAt = outline.CreatedAt.Add(1)
	outline.Slides[2].ExpectedElementTypes = append(outline.Slides[2].ExpectedElementTypes, "IMAGE")
	return intent, research, storyline, outline
}
