package ppt

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type semanticPlanningFixture struct {
	storylineCalls int
	outlineCalls   int
	storylineErrs  []error
	outlineErrs    []error
	mutateOutline  func(*OutlinePlanDraft)
}

func (p *semanticPlanningFixture) PlanStoryline(_ context.Context, input StorylinePlanningInput) (StorylinePlanningOutput, error) {
	p.storylineCalls++
	if len(p.storylineErrs) > 0 {
		err := p.storylineErrs[0]
		p.storylineErrs = p.storylineErrs[1:]
		if err != nil {
			return StorylinePlanningOutput{}, err
		}
	}
	claimRefs := []string{}
	if len(input.Research.Claims) > 0 {
		claimRefs = []string{input.Research.Claims[0].ID}
	}
	if input.Intent.Language == "en-US" {
		return StorylinePlanningOutput{
			Draft: StorylineDraft{
				Thesis:           "The market is changing fast enough to require a management decision.",
				AudienceTakeaway: "Management should understand the evidence, risks, and available choices.",
				NarrativeArc:     []string{"change", "evidence", "decision"},
				Sections: []StorylineSectionDraft{
					{Key: "change", Title: "What changed", Objective: "Explain the market shift", EvidenceRefs: claimRefs},
					{Key: "evidence", Title: "What the evidence shows", Objective: "Connect evidence to management implications", EvidenceRefs: claimRefs},
					{Key: "decision", Title: "What management should do", Objective: "Define a practical decision", EvidenceRefs: nil},
				},
				ClosingAction: "Choose the priority market and assign an owner.",
				Language:      "en-US",
			},
			Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "fixture-chat", Model: "fixture-model", ProviderRequestID: "storyline-request"},
		}, nil
	}
	return StorylinePlanningOutput{
		Draft: StorylineDraft{
			Thesis:           "新能源汽车市场的结构性变化已经进入管理层需要决策的阶段。",
			AudienceTakeaway: "管理层应理解事实证据、关键风险和可执行选择。",
			NarrativeArc:     []string{"change", "evidence", "decision"},
			Sections: []StorylineSectionDraft{
				{Key: "change", Title: "市场发生了什么", Objective: "说明新能源汽车市场的结构变化", EvidenceRefs: claimRefs},
				{Key: "evidence", Title: "证据意味着什么", Objective: "把事实证据转化为管理层判断", EvidenceRefs: claimRefs},
				{Key: "decision", Title: "管理层如何行动", Objective: "形成明确的决策建议"},
			},
			ClosingAction: "确定优先市场并明确下一步负责人。",
			Language:      "zh-CN",
		},
		Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "fixture-chat", Model: "fixture-model", ProviderRequestID: "storyline-request"},
	}, nil
}

func (p *semanticPlanningFixture) PlanOutline(_ context.Context, input OutlinePlanningInput) (OutlinePlanningOutput, error) {
	p.outlineCalls++
	if len(p.outlineErrs) > 0 {
		err := p.outlineErrs[0]
		p.outlineErrs = p.outlineErrs[1:]
		if err != nil {
			return OutlinePlanningOutput{}, err
		}
	}
	pageCount := input.Intent.PageCount.Preferred
	if pageCount == 0 {
		pageCount = 8
	}
	english := input.Intent.Language == "en-US"
	claimOne := ""
	claimTwo := ""
	if len(input.Research.Claims) > 0 {
		claimOne = input.Research.Claims[0].ID
		claimTwo = claimOne
	}
	if len(input.Research.Claims) > 1 {
		claimTwo = input.Research.Claims[1].ID
	}
	slides := make([]SlideObjectiveDraft, 0, pageCount)
	for index := 0; index < pageCount; index++ {
		draft := SlideObjectiveDraft{
			Title:                "管理层决策页面",
			Purpose:              "形成可执行的管理层判断",
			KeyMessage:           "管理层应基于证据决定优先市场。",
			VisualIntent:         "清晰的商务决策结构",
			ExpectedElementTypes: []string{"TEXT", "SHAPE"},
		}
		if english {
			draft.Title = "Management decision"
			draft.Purpose = "Make an actionable management decision"
			draft.KeyMessage = "Management should use evidence to choose the priority market."
			draft.VisualIntent = "A clear professional decision structure"
		}
		if index == 2 {
			draft.ExpectedElementTypes = []string{"TEXT", "SHAPE", "IMAGE"}
			draft.VisualIntent += " with one relevant market image"
		}
		if index > 0 && index < pageCount-1 && claimOne != "" {
			claimID := claimOne
			if index == 2 {
				claimID = claimTwo
			}
			draft.EvidenceRequired = true
			draft.Evidence = []EvidenceAssignment{{ClaimID: claimID, Rationale: "该事实直接支持本页对市场变化的判断。"}}
			if english {
				draft.Evidence[0].Rationale = "This fact directly supports the market conclusion on this slide."
			}
		}
		slides = append(slides, draft)
	}
	draft := OutlinePlanDraft{Language: input.Intent.Language, Slides: slides}
	if p.mutateOutline != nil {
		p.mutateOutline(&draft)
	}
	return OutlinePlanningOutput{
		Draft:      draft,
		Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "fixture-chat", Model: "fixture-model", ProviderRequestID: "outline-request"},
	}, nil
}

func TestPlanningWorkerClassifiesInvalidEvidenceMapping(t *testing.T) {
	store := NewMemoryGenerationJobStore()
	research := &countingResearchProvider{pack: agentResearchFixture(t)}
	planning := &semanticPlanningFixture{mutateOutline: func(draft *OutlinePlanDraft) {
		draft.Slides[1].Evidence = []EvidenceAssignment{{ClaimID: "missing_claim", Rationale: "Unsupported evidence."}}
	}}
	now := time.Date(2026, 8, 16, 6, 30, 0, 0, time.UTC)
	service, err := NewAgentPlanningService(store, research, planning, planning, AgentPlanningOptions{WorkerID: "invalid_evidence", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	guided, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "guide_invalid_evidence",
		Request: IntentRequest{Text: "Create an 8-page electric vehicle market analysis for management."}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	state, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Job.Error == nil || state.Job.Error.Code != PlanningEvidenceMappingInvalid || state.Outline.ID != "" {
		t.Fatalf("invalid evidence mapping did not fail closed: %+v", state)
	}
}

func newAgentPlanningA1Fixture(t *testing.T) (*MemoryGenerationJobStore, *AgentPlanningService, *countingResearchProvider, *semanticPlanningFixture, time.Time) {
	t.Helper()
	store := NewMemoryGenerationJobStore()
	research := &countingResearchProvider{pack: agentResearchFixture(t)}
	planning := &semanticPlanningFixture{}
	now := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	service, err := NewAgentPlanningService(store, research, planning, planning, AgentPlanningOptions{WorkerID: "agent_planner_a1_test", LeaseDuration: time.Minute, RetryDelay: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return store, service, research, planning, now
}

func TestAgentGuideReturnsCreatedJobBeforeBackgroundPlanning(t *testing.T) {
	store, service, research, planning, now := newAgentPlanningA1Fixture(t)
	result, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", OrganizationID: "org_a", IdempotencyKey: "guide_async",
		Request: IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State == nil || result.State.Job.Status != GenerationJobQueued || result.State.Job.Stage != GenerationStageCreated {
		t.Fatalf("guide did not return durable CREATED state: %+v", result)
	}
	if research.calls != 0 || planning.storylineCalls != 0 || planning.outlineCalls != 0 {
		t.Fatalf("guide performed background work synchronously: research=%d storyline=%d outline=%d", research.calls, planning.storylineCalls, planning.outlineCalls)
	}
	bundle, err := store.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, result.State.Job.ID)
	if err != nil || len(bundle.Job.InputSnapshot) == 0 {
		t.Fatalf("guide did not persist input snapshot: job=%+v err=%v", bundle.Job, err)
	}
}

func TestPlanningWorkerBuildsSemanticOutlineAndStopsAtApproval(t *testing.T) {
	store, service, research, planning, now := newAgentPlanningA1Fixture(t)
	guided, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "guide_worker",
		Request: IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	state, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Job.Status != GenerationJobWaitingForOutlineApproval || state.Job.Stage != GenerationStageOutlinePlanned || len(state.Outline.Slides) != 10 {
		t.Fatalf("worker did not reach approval gate: %+v", state)
	}
	if research.calls != 1 || planning.storylineCalls != 1 || planning.outlineCalls != 1 {
		t.Fatalf("planning call counts mismatch: research=%d storyline=%d outline=%d", research.calls, planning.storylineCalls, planning.outlineCalls)
	}
	if state.Storyline.Provenance.Mode != PlanningModeAI || state.Outline.Provenance.Mode != PlanningModeAI {
		t.Fatalf("production planning provenance is missing: storyline=%+v outline=%+v", state.Storyline.Provenance, state.Outline.Provenance)
	}
	if state.Outline.Slides[1].Evidence[0].ClaimID == state.Outline.Slides[2].Evidence[0].ClaimID {
		t.Fatalf("semantic fixture mapping was replaced by positional allocation: %+v", state.Outline.Slides[:3])
	}
	if err := ValidateOutlinePlan(state.Outline, state.Research); err != nil {
		t.Fatalf("semantic outline is invalid: %v", err)
	}
	bundle, err := store.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, state.Job.ID)
	if err != nil || bundle.Deck.ID != "" || len(bundle.Slides) != 0 {
		t.Fatalf("Slice A.1 crossed into Slice B: bundle=%+v err=%v", bundle, err)
	}
}

func TestPlanningWorkerResumesFromResearchAndStorylineCheckpoints(t *testing.T) {
	_, service, research, planning, now := newAgentPlanningA1Fixture(t)
	planning.storylineErrs = []error{NewAgentWorkflowError(PlanningProviderUnavailable, "规划服务暂时不可用", true, errors.New("provider down")), nil}
	guided, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "guide_resume",
		Request: IntentRequest{Text: "帮我做一份8页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Job.Stage != GenerationStageResearched || failed.Job.Status != GenerationJobRetryWait || failed.Job.Error == nil || failed.Job.Error.Code != PlanningProviderUnavailable {
		t.Fatalf("storyline failure was not durable: %+v", failed.Job)
	}
	if err := service.ProcessReady(t.Context(), now.Add(3*time.Second), 10); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Job.Status != GenerationJobWaitingForOutlineApproval || research.calls != 1 || resumed.ResearchExecutionCount != 1 || planning.storylineCalls != 2 || planning.outlineCalls != 1 {
		t.Fatalf("research checkpoint was repeated or resume failed: state=%+v research=%d storyline=%d outline=%d", resumed, research.calls, planning.storylineCalls, planning.outlineCalls)
	}

	_, outlineService, outlineResearch, outlinePlanning, outlineNow := newAgentPlanningA1Fixture(t)
	outlinePlanning.outlineErrs = []error{NewAgentWorkflowError(PlanningInvalidOutput, "规划结果无法解析", true, errors.New("bad json")), nil}
	outlineGuided, err := outlineService.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "guide_outline_resume",
		Request: IntentRequest{Text: "帮我做一份8页的新能源汽车行业分析，给公司管理层汇报。"}, Now: outlineNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := outlineService.ProcessReady(t.Context(), outlineNow.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	if err := outlineService.ProcessReady(t.Context(), outlineNow.Add(3*time.Second), 10); err != nil {
		t.Fatal(err)
	}
	outlineResumed, err := outlineService.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, outlineGuided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if outlineResumed.Job.Status != GenerationJobWaitingForOutlineApproval || outlineResearch.calls != 1 || outlinePlanning.storylineCalls != 1 || outlinePlanning.outlineCalls != 2 {
		t.Fatalf("storyline checkpoint was repeated or outline resume failed: state=%+v research=%d storyline=%d outline=%d", outlineResumed, outlineResearch.calls, outlinePlanning.storylineCalls, outlinePlanning.outlineCalls)
	}
}

func TestPlanningFailureIsFailClosedAndRetryIsIdempotent(t *testing.T) {
	store := NewMemoryGenerationJobStore()
	research := &countingResearchProvider{pack: agentResearchFixture(t)}
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	service, err := NewAgentPlanningService(store, research, nil, nil, AgentPlanningOptions{WorkerID: "fail_closed", LeaseDuration: time.Minute, RetryDelay: 0})
	if err != nil {
		t.Fatal(err)
	}
	guided, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "guide_fail_closed",
		Request: IntentRequest{Text: "帮我做一份8页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	state, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Job.Error == nil || state.Job.Error.Code != PlanningProviderUnavailable || state.Outline.ID != "" || state.Storyline.ID != "" {
		t.Fatalf("missing planning provider silently produced a plan: %+v", state)
	}
	firstRetry, err := service.Retry(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	secondRetry, err := service.Retry(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstRetry, secondRetry) {
		t.Fatalf("duplicate retry changed durable state: first=%+v second=%+v", firstRetry, secondRetry)
	}
}

func TestStalePlanningLeaseCannotPersistProviderOutput(t *testing.T) {
	store := NewMemoryGenerationJobStore()
	now := time.Date(2026, 8, 16, 6, 0, 0, 0, time.UTC)
	intent := *InterpretAgentIntent(IntentRequest{Text: "帮我做一份8页的新能源汽车行业分析，给公司管理层汇报。"}).Intent
	job, _, err := store.CreateAgentPlanning(t.Context(), CreateGenerationJobInput{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "stale_planner", ClientRequestID: "stale_planner_request",
		SlideCount: 8, WorkflowType: GenerationWorkflowAgentOutline, InputSnapshot: []byte(`{"text":"request"}`), Now: now,
	}, intent)
	if err != nil {
		t.Fatal(err)
	}
	scope := GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}
	stale, err := store.Claim(t.Context(), scope, job.ID, "worker_old", now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.Claim(t.Context(), scope, job.ID, "worker_new", now.Add(2*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAgentIntent(t.Context(), current, intent, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAgentIntent(t.Context(), stale, intent, now.Add(3*time.Second)); !errors.Is(err, ErrGenerationJobLeaseLost) {
		t.Fatalf("stale worker output was accepted: %v", err)
	}
}
