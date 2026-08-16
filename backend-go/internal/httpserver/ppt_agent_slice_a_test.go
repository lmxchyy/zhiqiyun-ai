package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

type pptAgentHTTPResearchProvider struct {
	pack  pptapp.ResearchPack
	calls int
}

func (p *pptAgentHTTPResearchProvider) Research(context.Context, pptapp.IntentSpec) (pptapp.ResearchPack, error) {
	p.calls++
	return p.pack, nil
}

type pptAgentHTTPPlanningPort struct{}

func (pptAgentHTTPPlanningPort) PlanStoryline(_ context.Context, input pptapp.StorylinePlanningInput) (pptapp.StorylinePlanningOutput, error) {
	claimID := input.Research.Claims[0].ID
	return pptapp.StorylinePlanningOutput{
		Draft: pptapp.StorylineDraft{
			Language: input.Intent.Language, Thesis: "The market shift requires a focused management response.",
			AudienceTakeaway: "Management should prioritize the highest-confidence opportunities.",
			NarrativeArc:     []string{"context", "evidence", "action"},
			Sections: []pptapp.StorylineSectionDraft{
				{Key: "context", Title: "Market context", Objective: "Frame the decision."},
				{Key: "evidence", Title: "Evidence", Objective: "Assess the verified market signal.", EvidenceRefs: []string{claimID}},
				{Key: "action", Title: "Action", Objective: "Define management priorities."},
			},
			ClosingAction: "Confirm the priority actions and owners.",
		},
		Provenance: pptapp.PlanningProvenance{Mode: pptapp.PlanningModeDeterministicTest, Provider: "http-test", Model: "semantic-fixture"},
	}, nil
}

func (pptAgentHTTPPlanningPort) PlanOutline(_ context.Context, input pptapp.OutlinePlanningInput) (pptapp.OutlinePlanningOutput, error) {
	claimID := input.Research.Claims[0].ID
	slides := make([]pptapp.SlideObjectiveDraft, input.Intent.PageCount.Preferred)
	for index := range slides {
		slides[index] = pptapp.SlideObjectiveDraft{
			Title: "Management decision " + strconv.Itoa(index+1), Purpose: "Advance the storyline for management.",
			KeyMessage: "A focused response is required at this point in the narrative.", VisualIntent: "Professional evidence-led layout", ExpectedElementTypes: []string{"TEXT"},
		}
		if index > 0 && index < len(slides)-1 {
			slides[index].EvidenceRequired = true
			slides[index].Evidence = []pptapp.EvidenceAssignment{{ClaimID: claimID, Rationale: "This verified market claim supports the management decision on this page."}}
		}
	}
	return pptapp.OutlinePlanningOutput{
		Draft:      pptapp.OutlinePlanDraft{Language: input.Intent.Language, Slides: slides},
		Provenance: pptapp.PlanningProvenance{Mode: pptapp.PlanningModeDeterministicTest, Provider: "http-test", Model: "semantic-fixture"},
	}, nil
}

func pptAgentHTTPFixtureWithPlanning(t *testing.T, planning pptapp.StorylinePlanningPort, outline pptapp.OutlinePlanningPort) (api, string, *pptapp.MemoryGenerationJobStore, *pptAgentHTTPResearchProvider) {
	t.Helper()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	user, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "PPT Agent User", Email: "ppt-agent@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(t.Context(), "ppt-agent-token", user.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	retrievedAt := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	sourceID := pptapp.StableResearchSourceID("test-research", "source:1")
	pack, err := pptapp.NormalizeResearchPack(pptapp.ResearchPack{
		Sources:            []pptapp.ResearchSource{{ID: sourceID, Provider: "test-research", ProviderIdentity: "source:1", Title: "新能源汽车", Type: "test", Locator: "https://example.test/ev", RetrievedAt: retrievedAt}},
		Citations:          []pptapp.ResearchCitation{{ID: "citation_1", SourceID: sourceID, Locator: "https://example.test/ev", RetrievedAt: retrievedAt}},
		Claims:             []pptapp.ResearchClaim{{ID: "claim_1", SourceID: sourceID, CitationRefs: []string{"citation_1"}, Text: "新能源汽车市场正在发生结构性变化。", VerificationStatus: pptapp.ResearchVerificationSourceSupported}},
		VerificationStatus: pptapp.ResearchVerificationSourceSupported,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &pptAgentHTTPResearchProvider{pack: pack}
	jobs := pptapp.NewMemoryGenerationJobStore()
	service, err := pptapp.NewAgentPlanningService(jobs, provider, planning, outline, pptapp.AgentPlanningOptions{WorkerID: "ppt_agent_http_test", LeaseDuration: time.Minute, RetryDelay: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return api{store: store, sessions: sessions, pptAgentService: service}, "ppt-agent-token", jobs, provider
}

func pptAgentHTTPFixture(t *testing.T) (api, string, *pptapp.MemoryGenerationJobStore, *pptAgentHTTPResearchProvider) {
	return pptAgentHTTPFixtureWithPlanning(t, pptAgentHTTPPlanningPort{}, pptAgentHTTPPlanningPort{})
}

func pptAgentHTTPRequest(t *testing.T, token, method, path string, body []byte) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestPPTAgentSliceAHTTPStopsAtApprovedOutlinePlan(t *testing.T) {
	a, token, jobs, provider := pptAgentHTTPFixture(t)
	guideResponse := httptest.NewRecorder()
	a.guidePPTAgent(guideResponse, pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/guide", []byte(`{"idempotencyKey":"http-guide-1","text":"帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}`)))
	if guideResponse.Code != http.StatusOK {
		t.Fatalf("guide status=%d body=%s", guideResponse.Code, guideResponse.Body.String())
	}
	var guided pptapp.AgentGuideResult
	if err := json.Unmarshal(guideResponse.Body.Bytes(), &guided); err != nil {
		t.Fatal(err)
	}
	if guided.State == nil || guided.State.Job.Status != pptapp.GenerationJobQueued || guided.State.Job.Stage != pptapp.GenerationStageCreated || provider.calls != 0 {
		t.Fatalf("guide result mismatch: result=%+v calls=%d", guided, provider.calls)
	}
	if err := a.pptAgentService.ProcessReady(t.Context(), time.Now().UTC().Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	state, err := a.pptAgentService.Get(t.Context(), pptapp.GenerationJobScope{TenantID: guided.State.Job.TenantID, UserID: guided.State.Job.UserID}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	guided.State = &state
	if guided.State.Job.Status != pptapp.GenerationJobWaitingForOutlineApproval || guided.State.Outline.Revision != 1 || len(guided.State.Outline.Slides) != 10 || provider.calls != 1 {
		t.Fatalf("planning result mismatch: result=%+v calls=%d", guided, provider.calls)
	}
	bundle, err := jobs.Get(t.Context(), pptapp.GenerationJobScope{TenantID: guided.State.Job.TenantID, UserID: guided.State.Job.UserID}, guided.State.Job.ID)
	if err != nil || bundle.Deck.ID != "" || len(bundle.Slides) != 0 {
		t.Fatalf("Slice A crossed into slide generation: bundle=%+v err=%v", bundle, err)
	}

	jobID := guided.State.Job.ID
	moveBody, _ := json.Marshal(map[string]any{
		"expectedRevision": 1,
		"commands":         []pptapp.OutlineEditCommand{{Type: pptapp.OutlineCommandMoveSlide, SlideID: guided.State.Outline.Slides[2].SlideID, ToIndex: 2}},
	})
	updateRequest := pptAgentHTTPRequest(t, token, http.MethodPatch, "/api/v1/ppt/agent/jobs/"+jobID+"/outline", moveBody)
	updateRequest.SetPathValue("jobId", jobID)
	updateResponse := httptest.NewRecorder()
	a.updatePPTAgentOutline(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	var updated pptapp.AgentPlanningState
	if err := json.Unmarshal(updateResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Outline.Revision != 2 {
		t.Fatalf("updated revision=%d", updated.Outline.Revision)
	}

	staleRequest := pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/jobs/"+jobID+"/outline/approve", []byte(`{"expectedRevision":1}`))
	staleRequest.SetPathValue("jobId", jobID)
	staleResponse := httptest.NewRecorder()
	a.approvePPTAgentOutline(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale approval status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}

	approveRequest := pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/jobs/"+jobID+"/outline/approve", []byte(`{"expectedRevision":2}`))
	approveRequest.SetPathValue("jobId", jobID)
	approveResponse := httptest.NewRecorder()
	a.approvePPTAgentOutline(approveResponse, approveRequest)
	if approveResponse.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", approveResponse.Code, approveResponse.Body.String())
	}
	var approved pptapp.AgentPlanningState
	if err := json.Unmarshal(approveResponse.Body.Bytes(), &approved); err != nil {
		t.Fatal(err)
	}
	if approved.Job.Stage != pptapp.GenerationStageOutlineApproved || approved.ApprovedOutline == nil || approved.ApprovedOutline.Revision != 2 {
		t.Fatalf("approved state mismatch: %+v", approved)
	}

	getRequest := pptAgentHTTPRequest(t, token, http.MethodGet, "/api/v1/ppt/agent/jobs/"+jobID, nil)
	getRequest.SetPathValue("jobId", jobID)
	getResponse := httptest.NewRecorder()
	a.getPPTAgentState(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !bytes.Contains(getResponse.Body.Bytes(), []byte(`"approvedOutline"`)) {
		t.Fatalf("get approved state=%d body=%s", getResponse.Code, getResponse.Body.String())
	}
}

func TestPPTAgentSliceA1HTTPRetriesDurablePlanningFailure(t *testing.T) {
	a, token, _, _ := pptAgentHTTPFixtureWithPlanning(t, nil, nil)
	guideResponse := httptest.NewRecorder()
	a.guidePPTAgent(guideResponse, pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/guide", []byte(`{"idempotencyKey":"http-guide-retry","text":"Create a 10-page market analysis for management.","pageCount":10,"language":"en"}`)))
	var guided pptapp.AgentGuideResult
	if err := json.Unmarshal(guideResponse.Body.Bytes(), &guided); err != nil {
		t.Fatal(err)
	}
	if err := a.pptAgentService.ProcessReady(t.Context(), time.Now().UTC().Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	jobID := guided.State.Job.ID
	retryRequest := pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/jobs/"+jobID+"/retry", nil)
	retryRequest.SetPathValue("jobId", jobID)
	retryResponse := httptest.NewRecorder()
	a.retryPPTAgentPlanning(retryResponse, retryRequest)
	if retryResponse.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retryResponse.Code, retryResponse.Body.String())
	}
	var retried pptapp.AgentPlanningState
	if err := json.Unmarshal(retryResponse.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	if retried.Job.Status != pptapp.GenerationJobQueued || retried.Job.Stage != pptapp.GenerationStageResearched || retried.Job.Error != nil {
		t.Fatalf("retry did not resume failed stage: %+v", retried.Job)
	}
}

func TestPPTAgentSliceAHTTPReturnsCriticalClarificationWithoutCreatingJob(t *testing.T) {
	a, token, _, provider := pptAgentHTTPFixture(t)
	response := httptest.NewRecorder()
	a.guidePPTAgent(response, pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/guide", []byte(`{"idempotencyKey":"http-guide-missing","text":"帮我做一份专业PPT。"}`)))
	if response.Code != http.StatusOK || provider.calls != 0 || !bytes.Contains(response.Body.Bytes(), []byte(`"clarificationQuestions"`)) {
		t.Fatalf("clarification response=%d calls=%d body=%s", response.Code, provider.calls, response.Body.String())
	}
}
