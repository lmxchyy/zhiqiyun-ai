package pptplanning

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/provider/chat"
)

type planningChatFixture struct {
	responses []chat.Response
	err       error
	requests  []generation.CreateRequest
	wait      bool
}

func (f *planningChatFixture) Chat(ctx context.Context, request generation.CreateRequest) (chat.Response, error) {
	f.requests = append(f.requests, request)
	if f.wait {
		<-ctx.Done()
		return chat.Response{}, ctx.Err()
	}
	if f.err != nil {
		return chat.Response{}, f.err
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func planningResearchFixture(t *testing.T) pptapp.ResearchPack {
	t.Helper()
	now := time.Date(2026, 8, 16, 7, 0, 0, 0, time.UTC)
	sourceID := pptapp.StableResearchSourceID("fixture", "ev-market")
	pack, err := pptapp.NormalizeResearchPack(pptapp.ResearchPack{
		Sources:            []pptapp.ResearchSource{{ID: sourceID, Provider: "fixture", ProviderIdentity: "ev-market", Title: "EV market report", Type: "report", Locator: "https://example.test/ev", RetrievedAt: now}},
		Citations:          []pptapp.ResearchCitation{{ID: "citation_ev", SourceID: sourceID, Locator: "https://example.test/ev#sales", RetrievedAt: now}},
		Claims:             []pptapp.ResearchClaim{{ID: "claim_ev_sales", SourceID: sourceID, CitationRefs: []string{"citation_ev"}, Text: "Electric vehicle sales continued to grow.", VerificationStatus: pptapp.ResearchVerificationSourceSupported}},
		VerificationStatus: pptapp.ResearchVerificationSourceSupported,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestClientPlansStorylineAndOutlineFromIntentAndClaims(t *testing.T) {
	chatFixture := &planningChatFixture{responses: []chat.Response{
		{
			ProviderCode: "openai-compatible-chat", Model: "planning-model",
			Message:  chat.Message{Role: "assistant", Content: `{"language":"en-US","thesis":"EV adoption requires a management decision.","audienceTakeaway":"Management should act on verified market evidence.","narrativeArc":["change","decision"],"sections":[{"key":"change","title":"What changed","objective":"Explain the market shift","evidenceRefs":["claim_ev_sales"]},{"key":"decision","title":"What to do","objective":"Choose the next action","evidenceRefs":[]}],"closingAction":"Assign an owner and validate the priority market."}`},
			Metadata: map[string]any{"id": "storyline-provider-request"},
		},
		{
			ProviderCode: "openai-compatible-chat", Model: "planning-model",
			Message:  chat.Message{Role: "assistant", Content: `{"language":"en-US","slides":[{"title":"EV market decision","purpose":"Frame the management question","keyMessage":"Management must choose where to compete.","evidenceRequired":false,"evidence":[],"visualIntent":"Professional cover","expectedElementTypes":["TEXT","SHAPE"]},{"title":"Sales momentum","purpose":"Explain verified demand momentum","keyMessage":"EV sales growth supports continued market attention.","evidenceRequired":true,"evidence":[{"claimId":"claim_ev_sales","rationale":"The verified sales claim directly supports the demand momentum conclusion."}],"visualIntent":"Evidence-led trend summary","expectedElementTypes":["TEXT","SHAPE"]},{"title":"Management action","purpose":"Close with a decision","keyMessage":"Assign an owner and validate the priority market.","evidenceRequired":false,"evidence":[],"visualIntent":"Action checklist","expectedElementTypes":["TEXT","SHAPE"]},{"title":"Decision criteria","purpose":"Define criteria","keyMessage":"Use evidence and risk to rank options.","evidenceRequired":true,"evidence":[{"claimId":"claim_ev_sales","rationale":"Demand momentum is one explicit decision criterion."}],"visualIntent":"Decision matrix","expectedElementTypes":["TEXT","SHAPE"]},{"title":"Risk","purpose":"Describe risk","keyMessage":"Growth does not remove execution risk.","evidenceRequired":true,"evidence":[{"claimId":"claim_ev_sales","rationale":"The growth claim provides the market context against which execution risk is assessed."}],"visualIntent":"Risk cards","expectedElementTypes":["TEXT","SHAPE"]},{"title":"Next step","purpose":"Commit action","keyMessage":"Start a focused validation sprint.","evidenceRequired":false,"evidence":[],"visualIntent":"Action roadmap","expectedElementTypes":["TEXT","SHAPE"]}]}`},
			Metadata: map[string]any{"id": "outline-provider-request"},
		},
	}}
	client := NewClient(chatFixture, Options{Model: "planning-model", Timeout: time.Second})
	intent := pptapp.IntentSpec{
		Topic: "Electric vehicle market", Goal: "industry-analysis", Audience: "company management", Scenario: "management-report", Language: "en-US",
		PageCount: pptapp.PageCountSpec{Min: 6, Max: 6, Preferred: 6, Explicit: true}, ProfessionalStyle: "professional-business", ResearchRequired: true,
	}
	research := planningResearchFixture(t)
	storylineOutput, err := client.PlanStoryline(t.Context(), pptapp.StorylinePlanningInput{Intent: intent, Research: research})
	if err != nil {
		t.Fatal(err)
	}
	storyline, err := pptapp.MaterializeStoryline(intent, research, storylineOutput)
	if err != nil {
		t.Fatal(err)
	}
	outlineOutput, err := client.PlanOutline(t.Context(), pptapp.OutlinePlanningInput{Intent: intent, Research: research, Storyline: storyline})
	if err != nil {
		t.Fatal(err)
	}
	outline, err := pptapp.MaterializeOutlinePlan("job_provider", intent, research, storyline, outlineOutput, time.Date(2026, 8, 16, 7, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if storyline.Language != "en-US" || outline.Language != "en-US" || outline.Provenance.ProviderRequestID != "outline-provider-request" {
		t.Fatalf("language or provider provenance lost: storyline=%+v outline=%+v", storyline, outline.Provenance)
	}
	if outline.Slides[1].Evidence[0].ClaimID != "claim_ev_sales" || outline.Slides[1].Evidence[0].Rationale == "" {
		t.Fatalf("semantic evidence mapping lost: %+v", outline.Slides[1])
	}
	if len(chatFixture.requests) != 2 || !strings.Contains(chatFixture.requests[0].Prompt, intent.Topic) || !strings.Contains(chatFixture.requests[0].Prompt, "claim_ev_sales") || !strings.Contains(chatFixture.requests[1].Prompt, storyline.Thesis) {
		t.Fatalf("provider prompts did not receive authoritative inputs: %+v", chatFixture.requests)
	}
}

func TestClientClassifiesInvalidOutputTimeoutAndUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		fixture  *planningChatFixture
		wantCode string
	}{
		{name: "invalid output", fixture: &planningChatFixture{responses: []chat.Response{{Message: chat.Message{Content: "not-json"}}}}, wantCode: pptapp.PlanningInvalidOutput},
		{name: "timeout", fixture: &planningChatFixture{wait: true}, wantCode: pptapp.PlanningTimeout},
		{name: "provider unavailable", fixture: &planningChatFixture{err: errors.New("provider unavailable")}, wantCode: pptapp.PlanningProviderUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient(test.fixture, Options{Model: "planning-model", Timeout: 5 * time.Millisecond})
			_, err := client.PlanStoryline(t.Context(), pptapp.StorylinePlanningInput{Intent: pptapp.IntentSpec{Language: "en-US"}})
			var workflowErr *pptapp.AgentWorkflowError
			if !errors.As(err, &workflowErr) || workflowErr.Code != test.wantCode {
				t.Fatalf("error=%v code=%v want=%s", err, workflowErr, test.wantCode)
			}
		})
	}
}

func TestClientRejectsUnknownJSONFields(t *testing.T) {
	client := NewClient(&planningChatFixture{responses: []chat.Response{{Message: chat.Message{Content: `{"language":"en-US","thesis":"x","unknown":true}`}}}}, Options{Model: "planning-model"})
	_, err := client.PlanStoryline(t.Context(), pptapp.StorylinePlanningInput{Intent: pptapp.IntentSpec{Language: "en-US"}})
	var workflowErr *pptapp.AgentWorkflowError
	if !errors.As(err, &workflowErr) || workflowErr.Code != pptapp.PlanningInvalidOutput {
		t.Fatalf("unknown field was accepted: %v", err)
	}
}

func TestClientKeepsChinesePlanningLanguage(t *testing.T) {
	fixture := &planningChatFixture{responses: []chat.Response{{
		ProviderCode: "openai-compatible-chat", Model: "planning-model",
		Message: chat.Message{Role: "assistant", Content: `{"language":"zh-CN","thesis":"市场变化已经进入管理层需要决策的阶段。","audienceTakeaway":"管理层应基于已验证证据选择优先市场。","narrativeArc":["市场判断"],"sections":[{"key":"市场判断","title":"市场发生了什么","objective":"用事实建立决策背景","evidenceRefs":["claim_ev_sales"]}],"closingAction":"明确优先市场和负责人。"}`},
	}}}
	client := NewClient(fixture, Options{Model: "planning-model"})
	intent := pptapp.IntentSpec{
		Topic: "新能源汽车行业", Goal: "industry-analysis", Audience: "公司管理层", Scenario: "management-report", Language: "zh-CN",
		PageCount: pptapp.PageCountSpec{Min: 6, Max: 12}, ProfessionalStyle: "professional-business", ResearchRequired: true,
	}
	research := planningResearchFixture(t)
	output, err := client.PlanStoryline(t.Context(), pptapp.StorylinePlanningInput{Intent: intent, Research: research})
	if err != nil {
		t.Fatal(err)
	}
	storyline, err := pptapp.MaterializeStoryline(intent, research, output)
	if err != nil {
		t.Fatal(err)
	}
	if storyline.Language != "zh-CN" || !strings.Contains(storyline.Thesis, "管理层") || !strings.Contains(fixture.requests[0].Prompt, `"language":"zh-CN"`) {
		t.Fatalf("Chinese planning language was not preserved: storyline=%+v prompt=%s", storyline, fixture.requests[0].Prompt)
	}
}
