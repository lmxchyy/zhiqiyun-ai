package ppt

import (
	"errors"
	"testing"
	"time"
)

func TestInterpretAgentIntentRespectsExplicitPageCount(t *testing.T) {
	result := InterpretAgentIntent(IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"})
	if len(result.ClarificationQuestions) != 0 {
		t.Fatalf("sufficient request asked clarification: %+v", result.ClarificationQuestions)
	}
	if result.Intent == nil {
		t.Fatal("intent is nil")
	}
	if !result.Intent.PageCount.Explicit || result.Intent.PageCount.Min != 10 || result.Intent.PageCount.Max != 10 || result.Intent.PageCount.Preferred != 10 {
		t.Fatalf("explicit page count lost: %+v", result.Intent.PageCount)
	}
	if result.Intent.Topic != "新能源汽车行业分析" || result.Intent.Audience != "公司管理层" || result.Intent.Scenario != "management-report" {
		t.Fatalf("intent extraction mismatch: %+v", result.Intent)
	}
	if result.Intent.Language != "zh-CN" || result.Intent.ProfessionalStyle == "" || !result.Intent.ResearchRequired {
		t.Fatalf("intent defaults mismatch: %+v", result.Intent)
	}
}

func TestInterpretAgentIntentPrefersExplicitNaturalLanguagePageCountOverFormDefault(t *testing.T) {
	result := InterpretAgentIntent(IntentRequest{Text: "帮我做一份8页的AI Agent行业分析。", PageCount: 10})
	if result.Intent == nil || result.Intent.PageCount.Preferred != 8 {
		t.Fatalf("natural-language page count was overridden by form default: %+v", result.Intent)
	}
}

func TestInterpretAgentIntentPreservesExplicitPageCountRange(t *testing.T) {
	result := InterpretAgentIntent(IntentRequest{Text: "帮我做一份8～10页的AI Agent行业趋势分析，给公司管理层汇报。"})
	if result.Intent == nil || !result.Intent.PageCount.Explicit || result.Intent.PageCount.Min != 8 || result.Intent.PageCount.Max != 10 || result.Intent.PageCount.Preferred != 9 {
		t.Fatalf("explicit page count range mismatch: %+v", result.Intent)
	}
}

func TestInterpretAgentIntentKeepsUnspecifiedPageCountDynamic(t *testing.T) {
	result := InterpretAgentIntent(IntentRequest{Text: "介绍我们的企业客服产品，面向潜在客户。"})
	if result.Intent == nil || result.Intent.PageCount.Explicit || result.Intent.PageCount.Min != 6 || result.Intent.PageCount.Max != 12 || result.Intent.PageCount.Preferred != 0 {
		t.Fatalf("unspecified page count was fixed: %+v", result.Intent)
	}
	if len(result.ClarificationQuestions) != 0 {
		t.Fatalf("usable request asked clarification: %+v", result.ClarificationQuestions)
	}
}

func TestInterpretAgentIntentOnlyClarifiesCriticalMissingTopic(t *testing.T) {
	result := InterpretAgentIntent(IntentRequest{Text: "帮我做一份专业PPT，10页左右。"})
	if result.Intent != nil {
		t.Fatalf("topicless request produced intent: %+v", result.Intent)
	}
	if len(result.ClarificationQuestions) != 1 {
		t.Fatalf("critical clarification mismatch: %+v", result.ClarificationQuestions)
	}
}

func TestNormalizeResearchPackDeduplicatesStableSourcesAndPreservesClaimTraceability(t *testing.T) {
	retrievedAt := time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC)
	sourceID := StableResearchSourceID("wikipedia-zh", "page:123")
	pack, err := NormalizeResearchPack(ResearchPack{
		Sources: []ResearchSource{
			{ID: sourceID, Provider: "wikipedia-zh", ProviderIdentity: "page:123", Title: "新能源汽车", Type: "encyclopedia", Locator: "https://example.test/wiki/EV", RetrievedAt: retrievedAt},
			{ID: sourceID, Provider: "wikipedia-zh", ProviderIdentity: "page:123", Title: "重复结果", Type: "encyclopedia", Locator: "https://example.test/wiki/EV", RetrievedAt: retrievedAt},
		},
		Citations:          []ResearchCitation{{ID: "citation_1", SourceID: sourceID, Locator: "https://example.test/wiki/EV", RetrievedAt: retrievedAt}},
		Claims:             []ResearchClaim{{ID: "claim_1", SourceID: sourceID, CitationRefs: []string{"citation_1"}, Text: "新能源汽车使用非常规车用燃料作为动力来源。", VerificationStatus: ResearchVerificationSourceSupported}},
		VerificationStatus: ResearchVerificationSourceSupported,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Sources) != 1 || pack.Sources[0].Title != "新能源汽车" {
		t.Fatalf("duplicate source was not normalized deterministically: %+v", pack.Sources)
	}
	if len(pack.Claims) != 1 || pack.Claims[0].SourceID != sourceID || len(pack.Claims[0].CitationRefs) != 1 {
		t.Fatalf("claim provenance lost: %+v", pack.Claims)
	}
	if err := ValidateResearchPack(pack); err != nil {
		t.Fatalf("normalized pack is invalid: %v", err)
	}
}

func TestValidateResearchPackRejectsClaimsWithoutSourceOrCitation(t *testing.T) {
	pack := ResearchPack{Claims: []ResearchClaim{{ID: "claim_orphan", SourceID: "missing", Text: "orphan"}}}
	if err := ValidateResearchPack(pack); !errors.Is(err, ErrInvalidResearchPack) {
		t.Fatalf("broken provenance error = %v", err)
	}
}

func agentResearchFixture(t *testing.T) ResearchPack {
	t.Helper()
	retrievedAt := time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC)
	sources := []ResearchSource{
		{Provider: "wikipedia-zh", ProviderIdentity: "page:1", Title: "新能源汽车", Type: "encyclopedia", Locator: "https://example.test/wiki/EV", RetrievedAt: retrievedAt},
		{Provider: "wikipedia-zh", ProviderIdentity: "page:2", Title: "新能源汽车产业", Type: "encyclopedia", Locator: "https://example.test/wiki/Industry", RetrievedAt: retrievedAt},
	}
	for index := range sources {
		sources[index].ID = StableResearchSourceID(sources[index].Provider, sources[index].ProviderIdentity)
	}
	pack, err := NormalizeResearchPack(ResearchPack{
		Sources: sources,
		Citations: []ResearchCitation{
			{ID: "citation_1", SourceID: sources[0].ID, Locator: sources[0].Locator, RetrievedAt: retrievedAt},
			{ID: "citation_2", SourceID: sources[1].ID, Locator: sources[1].Locator, RetrievedAt: retrievedAt},
		},
		Claims: []ResearchClaim{
			{ID: "claim_1", SourceID: sources[0].ID, CitationRefs: []string{"citation_1"}, Text: "新能源汽车采用非常规车用燃料或新型动力装置。", VerificationStatus: ResearchVerificationSourceSupported},
			{ID: "claim_2", SourceID: sources[1].ID, CitationRefs: []string{"citation_2"}, Text: "产业分析需要同时观察政策、技术、需求与竞争格局。", VerificationStatus: ResearchVerificationSourceSupported},
		},
		VerificationStatus: ResearchVerificationSourceSupported,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestProfessionalStorylineIsIndependentAndComplete(t *testing.T) {
	intent := InterpretAgentIntent(IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}).Intent
	storyline, err := BuildProfessionalStoryline(*intent, agentResearchFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if storyline.Thesis == "" || storyline.AudienceTakeaway == "" || storyline.ClosingAction == "" {
		t.Fatalf("storyline is incomplete: %+v", storyline)
	}
	if len(storyline.NarrativeArc) < 4 || len(storyline.Sections) < 4 {
		t.Fatalf("storyline has no ordered narrative: %+v", storyline)
	}
	for index, sectionID := range storyline.NarrativeArc {
		if sectionID != storyline.Sections[index].ID {
			t.Fatalf("narrative arc order mismatch: arc=%+v sections=%+v", storyline.NarrativeArc, storyline.Sections)
		}
	}
}

func TestDynamicOutlineRespectsExplicitCountAndValidEvidence(t *testing.T) {
	intent := InterpretAgentIntent(IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}).Intent
	pack := agentResearchFixture(t)
	storyline, _ := BuildProfessionalStoryline(*intent, pack)
	plan, err := BuildDynamicOutlinePlan("pptv2_agent_job_1", *intent, pack, storyline, time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if plan.PageCount != 10 || len(plan.Slides) != 10 {
		t.Fatalf("explicit page count ignored: %+v", plan)
	}
	claimIDs := map[string]bool{"claim_1": true, "claim_2": true}
	seen := map[string]bool{}
	for _, slide := range plan.Slides {
		if slide.SlideID == "" || seen[slide.SlideID] || slide.Purpose == "" || slide.KeyMessage == "" || slide.VisualIntent == "" || len(slide.ExpectedElementTypes) == 0 {
			t.Fatalf("invalid slide objective: %+v", slide)
		}
		seen[slide.SlideID] = true
		for _, evidenceRef := range slide.EvidenceRefs {
			if !claimIDs[evidenceRef] {
				t.Fatalf("invalid evidence ref %q in %+v", evidenceRef, slide)
			}
		}
	}
	if plan.Slides[0].SlideID != "pptv2_agent_job_1:objective:1" || plan.Slides[9].SlideID != "pptv2_agent_job_1:objective:10" {
		t.Fatalf("stable slide IDs missing: %+v", plan.Slides)
	}
}

func TestDynamicOutlineChoosesWithinRangeWhenCountIsUnspecified(t *testing.T) {
	intent := InterpretAgentIntent(IntentRequest{Text: "介绍我们的企业客服产品，面向潜在客户。"}).Intent
	storyline, _ := BuildProfessionalStoryline(*intent, ResearchPack{})
	plan, err := BuildDynamicOutlinePlan("pptv2_agent_job_dynamic", *intent, ResearchPack{}, storyline, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if plan.PageCount < AgentMinimumPageCount || plan.PageCount > AgentMaximumPageCount || plan.PageCount == 10 {
		t.Fatalf("dynamic page count is invalid or fixed to example count: %d", plan.PageCount)
	}
}

func TestOutlineCommandsAddDeleteMoveAndUpdateDeterministically(t *testing.T) {
	intent := InterpretAgentIntent(IntentRequest{Text: "做一份8页的新能源汽车行业分析，给管理层汇报。"}).Intent
	pack := agentResearchFixture(t)
	storyline, _ := BuildProfessionalStoryline(*intent, pack)
	plan, _ := BuildDynamicOutlinePlan("pptv2_agent_job_edit", *intent, pack, storyline, time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC))

	added, err := ApplyOutlineCommands(plan, []OutlineEditCommand{{Type: OutlineCommandAddSlide, AfterSlideID: plan.Slides[1].SlideID, Objective: &SlideObjective{
		Title: "补充市场证据", Purpose: "补充事实依据", KeyMessage: "市场判断必须建立在可追踪证据上。", EvidenceRefs: []string{"claim_1"}, VisualIntent: "关键证据卡片", ExpectedElementTypes: []string{"TEXT", "SHAPE"},
	}}}, pack)
	if err != nil || len(added.Slides) != 9 || added.Slides[2].SlideID != "pptv2_agent_job_edit:objective:9" {
		t.Fatalf("add slide failed: plan=%+v err=%v", added, err)
	}

	movedID := added.Slides[len(added.Slides)-1].SlideID
	moved, err := ApplyOutlineCommands(added, []OutlineEditCommand{{Type: OutlineCommandMoveSlide, SlideID: movedID, ToIndex: 2}}, pack)
	if err != nil || moved.Slides[1].SlideID != movedID {
		t.Fatalf("move slide failed: plan=%+v err=%v", moved, err)
	}

	updated, err := ApplyOutlineCommands(moved, []OutlineEditCommand{{Type: OutlineCommandUpdateSlideObjective, SlideID: movedID, Objective: &SlideObjective{
		Title: "管理层结论", Purpose: "明确管理层决策", KeyMessage: "现在需要决定优先进入的细分市场。", EvidenceRefs: []string{"claim_2"}, VisualIntent: "决策矩阵", ExpectedElementTypes: []string{"TEXT", "SHAPE"},
	}}}, pack)
	if err != nil || updated.Slides[1].KeyMessage != "现在需要决定优先进入的细分市场。" || updated.Slides[1].SlideID != movedID {
		t.Fatalf("update slide failed: plan=%+v err=%v", updated, err)
	}

	deleted, err := ApplyOutlineCommands(updated, []OutlineEditCommand{{Type: OutlineCommandDeleteSlide, SlideID: "pptv2_agent_job_edit:objective:9"}}, pack)
	if err != nil || len(deleted.Slides) != 8 {
		t.Fatalf("delete slide failed: plan=%+v err=%v", deleted, err)
	}
	if plan.Revision != 1 || added.Revision != 2 || moved.Revision != 3 || updated.Revision != 4 || deleted.Revision != 5 {
		t.Fatalf("revision sequence is not deterministic: %d %d %d %d %d", plan.Revision, added.Revision, moved.Revision, updated.Revision, deleted.Revision)
	}
}
