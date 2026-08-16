package ppt

import (
	"context"
	"errors"
	"testing"
	"time"
)

type countingResearchProvider struct {
	pack  ResearchPack
	calls int
}

func (p *countingResearchProvider) Research(_ context.Context, _ IntentSpec) (ResearchPack, error) {
	p.calls++
	return p.pack, nil
}

func newAgentPlanningServiceFixture(t *testing.T) (*MemoryGenerationJobStore, *AgentPlanningService, *countingResearchProvider, time.Time) {
	t.Helper()
	store := NewMemoryGenerationJobStore()
	provider := &countingResearchProvider{pack: agentResearchFixture(t)}
	planning := &semanticPlanningFixture{}
	now := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	service, err := NewAgentPlanningService(store, provider, planning, planning, AgentPlanningOptions{WorkerID: "agent_planner_test", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return store, service, provider, now
}

func TestAgentPlanningServiceStopsAtDurableOutlineApprovalGate(t *testing.T) {
	store, service, provider, now := newAgentPlanningServiceFixture(t)
	result, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", OrganizationID: "org_a", IdempotencyKey: "guide_1",
		Request: IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ClarificationQuestions) != 0 || result.State == nil {
		t.Fatalf("unexpected guide result: %+v", result)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	stateValue, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, result.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	state := &stateValue
	if state.Job.WorkflowType != GenerationWorkflowAgentOutline || state.Job.Status != GenerationJobWaitingForOutlineApproval || state.Job.Stage != GenerationStageOutlinePlanned {
		t.Fatalf("job did not stop at approval: %+v", state.Job)
	}
	if state.Job.Progress() != 100 || state.Outline.Revision != 1 || len(state.Outline.Slides) != 10 || state.ApprovedOutline != nil {
		t.Fatalf("planning state mismatch: %+v", state)
	}
	if provider.calls != 1 || state.ResearchExecutionCount != 1 {
		t.Fatalf("research execution mismatch: provider=%d state=%d", provider.calls, state.ResearchExecutionCount)
	}
	bundle, err := store.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, state.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Deck.ID != "" || len(bundle.Slides) != 0 {
		t.Fatalf("Slice A created DeckJob/SlideJob before approval: %+v", bundle)
	}
	if _, err := store.Claim(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, state.Job.ID, "slide_worker", now.Add(time.Second), time.Minute); !errors.Is(err, ErrGenerationJobAwaitingOutlineApproval) {
		t.Fatalf("generation continued before approval: %v", err)
	}
}

func TestAgentPlanningGuideReplayRestoresSameOutlineWithoutRepeatingResearch(t *testing.T) {
	_, service, provider, now := newAgentPlanningServiceFixture(t)
	request := GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", OrganizationID: "org_a", IdempotencyKey: "guide_replay",
		Request: IntentRequest{Text: "帮我做一份8页的 AI Agent 行业趋势分析，给公司管理层汇报。"}, Now: now,
	}
	first, err := service.Guide(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	firstState, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, first.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	first.State = &firstState
	request.Now = now.Add(time.Hour)
	replayed, err := service.Guide(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.State.Job.ID != replayed.State.Job.ID || first.State.Outline.ID != replayed.State.Outline.ID || first.State.Outline.Revision != replayed.State.Outline.Revision {
		t.Fatalf("replay changed durable outline: first=%+v replay=%+v", first.State, replayed.State)
	}
	if provider.calls != 1 || replayed.State.ResearchExecutionCount != 1 {
		t.Fatalf("replay repeated research: provider=%d state=%d", provider.calls, replayed.State.ResearchExecutionCount)
	}
}

func TestAgentPlanningOutlineApprovalIsOptimisticImmutableAndIdempotent(t *testing.T) {
	_, service, _, now := newAgentPlanningServiceFixture(t)
	guided, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", OrganizationID: "org_a", IdempotencyKey: "guide_approve",
		Request: IntentRequest{Text: "做一份8页的新能源汽车行业分析，给管理层汇报。"}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	guidedState, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	guided.State = &guidedState
	scope := GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}
	jobID := guided.State.Job.ID
	updated, err := service.UpdateOutline(t.Context(), scope, jobID, 1, []OutlineEditCommand{{
		Type: OutlineCommandUpdateSlideObjective, SlideID: guided.State.Outline.Slides[1].SlideID,
		Objective: &SlideObjective{Title: "市场变化", Purpose: "说明市场变化", KeyMessage: "需求、技术和政策共同改变竞争格局。", EvidenceRequired: true, EvidenceRefs: []string{"claim_1"}, Evidence: []EvidenceAssignment{{ClaimID: "claim_1", Rationale: "该事实支持市场变化判断。"}}, VisualIntent: "三因素结构", ExpectedElementTypes: []string{"TEXT", "SHAPE"}},
	}}, now.Add(time.Minute))
	if err != nil || updated.Outline.Revision != 2 {
		t.Fatalf("outline update failed: state=%+v err=%v", updated, err)
	}
	if _, err := service.ApproveOutline(t.Context(), scope, jobID, 1, now.Add(2*time.Minute)); !errors.Is(err, ErrStaleOutlineRevision) {
		t.Fatalf("stale approval error = %v", err)
	}
	approved, err := service.ApproveOutline(t.Context(), scope, jobID, 2, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if approved.Job.Status != GenerationJobQueued || approved.Job.Stage != GenerationStageOutlineApproved || approved.ApprovedOutline == nil || approved.ApprovedOutline.Revision != 2 || approved.ApprovedOutline.ApprovedAt.IsZero() {
		t.Fatalf("approval state mismatch: %+v", approved)
	}
	replayed, err := service.ApproveOutline(t.Context(), scope, jobID, 2, now.Add(4*time.Minute))
	if err != nil || replayed.ApprovedOutline == nil || !replayed.ApprovedOutline.ApprovedAt.Equal(approved.ApprovedOutline.ApprovedAt) {
		t.Fatalf("duplicate approve is not idempotent: state=%+v err=%v", replayed, err)
	}
	if _, err := service.UpdateOutline(t.Context(), scope, jobID, 2, []OutlineEditCommand{{Type: OutlineCommandDeleteSlide, SlideID: approved.Outline.Slides[1].SlideID}}, now.Add(5*time.Minute)); !errors.Is(err, ErrOutlinePlanApproved) {
		t.Fatalf("approved revision was mutable: %v", err)
	}
}

func TestAgentPlanningTenantIsolationAndCriticalClarification(t *testing.T) {
	_, service, provider, now := newAgentPlanningServiceFixture(t)
	clarification, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", IdempotencyKey: "guide_missing", Request: IntentRequest{Text: "帮我做一份专业PPT。"}, Now: now,
	})
	if err != nil || clarification.State != nil || len(clarification.ClarificationQuestions) != 1 || provider.calls != 0 {
		t.Fatalf("critical clarification crossed planning boundary: result=%+v calls=%d err=%v", clarification, provider.calls, err)
	}
	guided, err := service.Guide(t.Context(), GuideAgentRequest{
		TenantID: "tenant_a", UserID: "user_a", OrganizationID: "org_a", IdempotencyKey: "guide_isolation",
		Request: IntentRequest{Text: "做一份8页的新能源汽车行业分析，给管理层汇报。"}, Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(2*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	guidedState, err := service.Get(t.Context(), GenerationJobScope{TenantID: "tenant_a", UserID: "user_a"}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	guided.State = &guidedState
	wrongScope := GenerationJobScope{TenantID: "tenant_b", UserID: "user_a"}
	if _, err := service.Get(t.Context(), wrongScope, guided.State.Job.ID); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant read error = %v", err)
	}
	if _, err := service.UpdateOutline(t.Context(), wrongScope, guided.State.Job.ID, 1, []OutlineEditCommand{{
		Type: OutlineCommandMoveSlide, SlideID: guided.State.Outline.Slides[1].SlideID, ToIndex: 1,
	}}, now.Add(2*time.Minute)); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant update error = %v", err)
	}
	if _, err := service.ApproveOutline(t.Context(), wrongScope, guided.State.Job.ID, 1, now.Add(2*time.Minute)); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant approve error = %v", err)
	}
}
