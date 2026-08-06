package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/app/ppt/skills"
	"xianzhi-ai/backend-go/internal/config"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
	"xianzhi-ai/backend-go/internal/provider/parser"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestListPPTSkillsReturnsEightWithoutPrompts(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	response := harness.request(t, http.MethodGet, "/api/v1/ppt/skills", "", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list skills status = %d, body = %s", response.Code, response.Body.String())
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	wantCodes := []string{"general", "pitch_deck", "weekly_report", "sales_proposal", "training", "product_launch", "consulting", "meeting_summary"}
	if len(items) != len(wantCodes) {
		t.Fatalf("skill count = %d, want %d", len(items), len(wantCodes))
	}
	for index, item := range items {
		var code string
		if err := json.Unmarshal(item["code"], &code); err != nil {
			t.Fatal(err)
		}
		if code != wantCodes[index] {
			t.Fatalf("skill %d code = %q, want %q", index, code, wantCodes[index])
		}
		for _, forbidden := range []string{"systemPrompt", "SystemPrompt", "outlineSchema", "OutlineSchema"} {
			if _, exists := item[forbidden]; exists {
				t.Fatalf("public skill leaked %s: %s", forbidden, response.Body.String())
			}
		}
		if len(item) != 5 {
			t.Fatalf("skill keys = %v, want exactly five public fields", mapKeys(item))
		}
	}
}

func TestCreatePPTSessionIsDraftAndFree(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	before, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	var chatCalls atomic.Int32
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", `{
		"prompt":"季度经营复盘","skillCode":"weekly_report","sourceFileIds":[],"slideCount":6,"language":"zh","audience":"management"
	}`, "", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return chatprovider.Response{}, errors.New("create session must not call provider")
	})
	if response.Code != http.StatusOK {
		t.Fatalf("create session status = %d, body = %s", response.Code, response.Body.String())
	}
	var task map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if stringJSON(t, task["taskId"]) == "" || stringJSON(t, task["sessionId"]) != stringJSON(t, task["taskId"]) {
		t.Fatalf("session/task identity mismatch: %s", response.Body.String())
	}
	if got := stringJSON(t, task["stage"]); got != "DRAFT" {
		t.Fatalf("stage = %q, want DRAFT", got)
	}
	if got := stringJSON(t, task["status"]); got != "pending" {
		t.Fatalf("status = %q, want pending", got)
	}
	if chatCalls.Load() != 0 {
		t.Fatalf("create session called provider %d times", chatCalls.Load())
	}
	after, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.BillingEvents) != len(before.BillingEvents) {
		t.Fatalf("create session changed billing events: before=%d after=%d", len(before.BillingEvents), len(after.BillingEvents))
	}
	lastRequest := harness.state.lastSessionRequest()
	if lastRequest.Owner.UserID == "" || lastRequest.Owner.UserID == "attacker" {
		t.Fatalf("session did not force authenticated user: %#v", lastRequest)
	}
	if lastRequest.SkillCode != "weekly_report" || lastRequest.SlideCount <= 0 || lastRequest.SlideCount > 15 {
		t.Fatalf("session request was not skill/capability bounded: %#v", lastRequest)
	}
	assertNoPPTAgentInternalFields(t, response.Body.Bytes())
}

func TestCreatePPTSessionPersistsServerResolvedPersonalContext(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	request := harness.state.lastSessionRequest()
	if request.ContextType != contextPersonal || request.Owner.TenantID != "tenant_default" || request.OrganizationID != defaultOrganizationID("tenant_default") {
		t.Fatalf("personal tenant context = %#v", request)
	}
	if request.BillingScope != contextPersonal || request.BillingAccountID == "" || request.BillingAccountID != harness.state.owner(taskID) {
		t.Fatalf("personal billing context = %#v", request)
	}
	task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.ContextType != request.ContextType || task.TenantID != request.Owner.TenantID || task.OrganizationID != request.OrganizationID || task.BillingScope != request.BillingScope || task.BillingAccountID != request.BillingAccountID {
		t.Fatalf("persisted context task=%#v request=%#v", task, request)
	}
}

func TestPPTMessageRejectsCompletedReplayAfterEnterpriseContextSwitch(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	harness.authorizations.set(modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: "tenant_a", OrganizationID: "organization_a",
		BillingScope: contextEnterprise, BillingAccountID: "tenant_a", ServiceState: "ACTIVE",
	})
	taskID := harness.createSession(t, "general", 2)
	var chatCalls atomic.Int32
	chat := func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	}
	first := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "tenant-switch", chat)
	if first.Code != http.StatusOK {
		t.Fatalf("first enterprise message status = %d, body = %s", first.Code, first.Body.String())
	}
	beginCalls := harness.state.beginCalls
	harness.authorizations.set(modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: "tenant_b", OrganizationID: "organization_b",
		BillingScope: contextEnterprise, BillingAccountID: "tenant_b", ServiceState: "ACTIVE",
	})
	replay := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "tenant-switch", chat)
	assertPPTAgentError(t, replay, http.StatusNotFound, "PPT_TASK_NOT_FOUND")
	if chatCalls.Load() != 1 || harness.state.beginCalls != beginCalls {
		t.Fatalf("mismatched context reached idempotency/provider: begin=%d/%d provider=%d", harness.state.beginCalls, beginCalls, chatCalls.Load())
	}
}

func TestCreatePPTSessionRejectsClientTenantContext(t *testing.T) {
	for _, body := range []string{
		`{"prompt":"deck","skillCode":"general","tenantId":"attacker"}`,
		`{"prompt":"deck","skillCode":"general","organizationId":"attacker"}`,
	} {
		harness := newPPTAgentHTTPHarness(t)
		response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", body, "", nil)
		assertPPTAgentError(t, response, http.StatusBadRequest, "PPT_REQUEST_INVALID")
		if harness.state.createCalls != 0 {
			t.Fatalf("client tenant context created %d sessions", harness.state.createCalls)
		}
	}
}

func TestCreatePPTSessionAcceptsOnlyBoundedFileIDs(t *testing.T) {
	for _, body := range []string{
		`{"prompt":"deck","skillCode":"general","sourceFileIds":["https://files.example/report.md"]}`,
		`{"prompt":"deck","skillCode":"general","sourceFileIds":["../report.md"]}`,
		`{"prompt":"deck","skillCode":"general","sourceFileIds":["file_a","file_b","file_c","file_d"]}`,
	} {
		harness := newPPTAgentHTTPHarness(t)
		response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", body, "", nil)
		assertPPTAgentError(t, response, http.StatusBadRequest, "PPT_SOURCE_FILE_NOT_FOUND")
		if harness.state.createCalls != 0 {
			t.Fatalf("invalid source file IDs created %d sessions", harness.state.createCalls)
		}
	}
}

func TestCreatePPTSessionDefaultsToGeneralSkill(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", `{"prompt":"deck","slideCount":3}`, "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("default skill status = %d, body = %s", response.Code, response.Body.String())
	}
	if request := harness.state.lastSessionRequest(); request.SkillCode != "general" {
		t.Fatalf("default skill = %q, want general", request.SkillCode)
	}
}

func TestCreatePPTSessionRejectsUnknownSkill(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", `{"prompt":"deck","skillCode":"missing_skill","slideCount":3}`, "", nil)
	assertPPTAgentError(t, response, http.StatusNotFound, "PPT_SKILL_NOT_FOUND")
	if harness.state.createCalls != 0 {
		t.Fatalf("unknown skill created %d sessions", harness.state.createCalls)
	}
}

func TestPPTMessageRequiresIdempotencyKey(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	var chatCalls atomic.Int32
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成大纲"}`, "", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	})
	assertPPTAgentError(t, response, http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED")
	if chatCalls.Load() != 0 {
		t.Fatalf("missing idempotency key called provider %d times", chatCalls.Load())
	}
}

func TestPPTMessageReplayDoesNotCallProviderOrAppendTwice(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	var chatCalls atomic.Int32
	chat := func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	}
	first := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "message-key", chat)
	if first.Code != http.StatusOK {
		t.Fatalf("first message status = %d, body = %s", first.Code, first.Body.String())
	}
	second := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "message-key", chat)
	if second.Code != http.StatusOK {
		t.Fatalf("replay status = %d, body = %s", second.Code, second.Body.String())
	}
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatalf("completed replay payload changed\nfirst=%s\nreplay=%s", first.Body.String(), second.Body.String())
	}
	if chatCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", chatCalls.Load())
	}
	task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(task.AgentMessages) != 2 {
		t.Fatalf("agent messages = %d, want user+assistant once: %#v", len(task.AgentMessages), task.AgentMessages)
	}
	if task.Stage != pptapp.StageOutlineReady || task.Outline == nil || len(task.Outline.Slides) != 2 {
		t.Fatalf("unexpected replayed task: %#v", task)
	}
}

func TestPPTMessageConcurrentDuplicateReturnsInProgressWithoutSecondProviderCall(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	var chatCalls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	chat := func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		if chatCalls.Add(1) == 1 {
			close(started)
		}
		<-release
		return validPPTAgentChatResponse(), nil
	}
	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstResult <- harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "concurrent-key", chat)
	}()
	<-started

	second := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "concurrent-key", chat)
	close(release)
	first := <-firstResult
	assertPPTAgentError(t, second, http.StatusConflict, "PPT_OPERATION_IN_PROGRESS")
	if first.Code != http.StatusOK {
		t.Fatalf("first message status = %d, body = %s", first.Code, first.Body.String())
	}
	replay := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "concurrent-key", chat)
	if replay.Code != http.StatusOK {
		t.Fatalf("completed replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	if chatCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want exactly 1", chatCalls.Load())
	}
	task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(task.AgentMessages) != 2 {
		t.Fatalf("agent messages = %d, want one user/assistant pair: %#v", len(task.AgentMessages), task.AgentMessages)
	}
}

func TestPPTMessageCleanupFailureReturnsStableStorageError(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	harness.state.failErr = errors.New("database cleanup secret")
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "cleanup-failure", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		return chatprovider.Response{}, errors.New("provider failure")
	})
	assertPPTAgentError(t, response, http.StatusServiceUnavailable, "PPT_OPERATION_CLEANUP_FAILED")
	if strings.Contains(response.Body.String(), "database cleanup secret") {
		t.Fatalf("cleanup error leaked storage details: %s", response.Body.String())
	}
}

func TestPPTMessageCompletionCommitAmbiguityReturnsPersistedSnapshot(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	harness.state.completeAfterPersistErr = errors.New("commit result unknown")
	var chatCalls atomic.Int32
	chat := func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	}
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "ambiguous-complete", chat)
	if response.Code != http.StatusOK {
		t.Fatalf("ambiguous completion status = %d, body = %s", response.Code, response.Body.String())
	}
	replay := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "ambiguous-complete", chat)
	if replay.Code != http.StatusOK {
		t.Fatalf("ambiguous completion replay status = %d, body = %s", replay.Code, replay.Body.String())
	}
	if chatCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", chatCalls.Load())
	}
	task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(task.AgentMessages) != 2 {
		t.Fatalf("agent messages = %d, want one persisted pair: %#v", len(task.AgentMessages), task.AgentMessages)
	}
}

func TestPPTMessageSameKeyDifferentPayloadConflicts(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	var chatCalls atomic.Int32
	chat := func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	}
	first := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成两页大纲"}`, "conflict-key", chat)
	if first.Code != http.StatusOK {
		t.Fatalf("first message status = %d, body = %s", first.Code, first.Body.String())
	}
	conflict := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"改成另一份大纲"}`, "conflict-key", chat)
	assertPPTAgentError(t, conflict, http.StatusConflict, "PPT_IDEMPOTENCY_CONFLICT")
	if chatCalls.Load() != 1 {
		t.Fatalf("conflicting replay called provider: %d", chatCalls.Load())
	}
}

func TestPPTMessageProviderFailureDoesNotReturnMockOutline(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	var chatCalls atomic.Int32
	response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成大纲"}`, "provider-failure", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		return chatprovider.Response{}, errors.New("upstream-secret provider=private apiKey=do-not-return")
	})
	assertPPTAgentError(t, response, http.StatusBadGateway, "PPT_AGENT_PROVIDER_UNAVAILABLE")
	body := response.Body.String()
	for _, forbidden := range []string{"upstream-secret", "apiKey", "mock", "slides"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("provider failure leaked or mocked %q: %s", forbidden, body)
		}
	}
	if chatCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", chatCalls.Load())
	}
	task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Outline != nil || len(task.AgentMessages) != 0 || task.Stage != pptapp.StageDraft {
		t.Fatalf("provider failure mutated outline/messages: %#v", task)
	}
}

func TestPPTMessageRejectsNonJSONAndOversizedOutlines(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		content string
	}{
		{name: "markdown wrapper", content: "```json\n{\"title\":\"Deck\",\"pages\":[{\"title\":\"One\",\"summary\":\"S\",\"bullets\":[\"A\"]}]}\n```"},
		{name: "too many pages", content: `{"title":"Deck","pages":[{"title":"One","summary":"S","bullets":["A"]},{"title":"Two","summary":"S","bullets":["B"]}]}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "meeting_summary", 1)
			response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/messages", `{"message":"生成大纲"}`, "invalid-response", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
				return chatprovider.Response{Message: chatprovider.Message{Role: "assistant", Content: testCase.content}}, nil
			})
			assertPPTAgentError(t, response, http.StatusBadGateway, "PPT_AGENT_RESPONSE_INVALID")
			task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Outline != nil || len(task.AgentMessages) != 0 {
				t.Fatalf("invalid provider response mutated task: %#v", task)
			}
		})
	}
}

func TestPPTReviseSlideChangesOnlyReadyTargetAndReplaysWithoutWorkOrCharge(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	harness.state.makeReady(taskID, []pptapp.Slide{
		pptapp.NormalizeSlideIR(pptapp.Slide{ID: "slide_1", Page: 1, Layout: "imageText", Blocks: []pptapp.SlideBlock{
			{Type: "title", Text: "Keep page"}, {Type: "paragraph", Text: "Keep body"}, {Type: "image", ImageRef: "storage://tenant_default/file_one"},
		}}),
		pptapp.NormalizeSlideIR(pptapp.Slide{ID: "slide_2", Page: 2, Layout: "imageText", Blocks: []pptapp.SlideBlock{
			{Type: "title", Text: "Old target"}, {Type: "paragraph", Text: "Old body"}, {Type: "bullets", Items: []string{"Old point"}},
			{Type: "image", ImageRef: "storage://tenant_default/file_two"}, {Type: "note", Text: "Keep notes"},
		}}),
	})
	beforeBilling, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int32
	chat := func(_ context.Context, request generation.CreateRequest) (chatprovider.Response, error) {
		providerCalls.Add(1)
		messages, ok := request.Params["messages"].([]chatprovider.Message)
		if !ok || len(messages) < 2 {
			t.Fatalf("revision provider messages = %#v", request.Params["messages"])
		}
		raw, marshalErr := json.Marshal(messages)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		prompt := string(raw)
		if !strings.Contains(prompt, "slide_2") || !strings.Contains(prompt, "Old target") || !strings.Contains(prompt, "面向投资人") {
			t.Fatalf("revision prompt omitted target/instruction: %s", prompt)
		}
		if strings.Contains(prompt, "Keep page") || strings.Contains(prompt, "storage://tenant_default/file_one") {
			t.Fatalf("revision prompt leaked another slide: %s", prompt)
		}
		return chatprovider.Response{Message: chatprovider.Message{Role: "assistant", Content: `{"blocks":[{"type":"title","text":"Investor update"},{"type":"subtitle","text":"Quarterly results"},{"type":"paragraph","text":"Revised body"},{"type":"bullets","items":["Growth","Margin"]}]}`}}, nil
	}
	body := `{"slideId":"slide_2","instruction":"面向投资人重写这一页"}`
	first := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/revise-slide", body, "revise-key", chat)
	if first.Code != http.StatusOK {
		t.Fatalf("revise slide status = %d, body = %s", first.Code, first.Body.String())
	}
	var revised pptTaskPublicResponse
	if err := json.Unmarshal(first.Body.Bytes(), &revised); err != nil {
		t.Fatal(err)
	}
	if revised.Stage != pptapp.StageReady || revised.Status != pptapp.StatusSuccess || len(revised.Slides) != 2 {
		t.Fatalf("revision response task = %#v", revised)
	}
	if revised.Slides[0].Title != "Keep page" || revised.Slides[0].ImageURL != "storage://tenant_default/file_one" {
		t.Fatalf("revision changed non-target slide: %#v", revised.Slides[0])
	}
	target := revised.Slides[1]
	if target.ID != "slide_2" || target.Page != 2 || target.Title != "Investor update" || target.Content != "Revised body" || len(target.BulletPoints) != 2 {
		t.Fatalf("revision target = %#v", target)
	}
	if target.ImageURL != "storage://tenant_default/file_two" || target.Layout != "imageText" || target.SpeakerNotes != "Keep notes" {
		t.Fatalf("revision replaced visual/layout/note state: %#v", target)
	}
	replay := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/revise-slide", body, "revise-key", chat)
	if replay.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatalf("revision replay changed response: first=%s replay=%s", first.Body.String(), replay.Body.String())
	}
	conflict := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/revise-slide", `{"slideId":"slide_2","instruction":"改成教师口吻"}`, "revise-key", chat)
	assertPPTAgentError(t, conflict, http.StatusConflict, "PPT_IDEMPOTENCY_CONFLICT")
	if providerCalls.Load() != 1 {
		t.Fatalf("revision provider calls = %d, want exactly 1", providerCalls.Load())
	}
	afterBilling, err := harness.store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterBilling.BillingEvents) != len(beforeBilling.BillingEvents) {
		t.Fatalf("revision changed billing events: before=%d after=%d", len(beforeBilling.BillingEvents), len(afterBilling.BillingEvents))
	}
}

func TestPPTReviseSlideProviderFailurePreservesReadySlideAndAllowsSameKeyRetry(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 1)
	original := pptapp.NormalizeSlideIR(pptapp.Slide{ID: "slide_1", Page: 1, Layout: "imageText", Blocks: []pptapp.SlideBlock{
		{Type: "title", Text: "Original"}, {Type: "paragraph", Text: "Original body"}, {Type: "image", ImageRef: "storage://tenant_default/file_original"},
	}})
	harness.state.makeReady(taskID, []pptapp.Slide{original})
	body := `{"slideId":"slide_1","instruction":"精简表达"}`
	failed := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/revise-slide", body, "retry-revision", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		return chatprovider.Response{}, errors.New("private provider failure")
	})
	assertPPTAgentError(t, failed, http.StatusBadGateway, "PPT_AGENT_PROVIDER_UNAVAILABLE")
	afterFailure, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Stage != pptapp.StageReady || len(afterFailure.Slides) != 1 {
		t.Fatalf("provider failure mutated READY slide: %#v", afterFailure)
	}
	afterFailureSlide := projectPPTSlideForHTTP(afterFailure.Slides[0])
	originalSlide := projectPPTSlideForHTTP(original)
	if afterFailureSlide.Title != originalSlide.Title || afterFailureSlide.ImageURL != originalSlide.ImageURL {
		t.Fatalf("provider failure mutated READY slide: %#v", afterFailure)
	}
	var retryCalls atomic.Int32
	retried := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/revise-slide", body, "retry-revision", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		retryCalls.Add(1)
		return chatprovider.Response{Message: chatprovider.Message{Role: "assistant", Content: `{"blocks":[{"type":"title","text":"Concise"},{"type":"paragraph","text":"Short body"}]}`}}, nil
	})
	if retried.Code != http.StatusOK || retryCalls.Load() != 1 {
		t.Fatalf("same-key revision retry status=%d calls=%d body=%s", retried.Code, retryCalls.Load(), retried.Body.String())
	}
}

func TestPPTReviseSlideValidatesStrictRequestAndReadyStageBeforeProvider(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		key   string
		want  string
		ready bool
	}{
		{name: "missing idempotency key", body: `{"slideId":"slide_1","instruction":"rewrite"}`, want: "PPT_IDEMPOTENCY_KEY_REQUIRED", ready: true},
		{name: "unknown request field", body: `{"slideId":"slide_1","instruction":"rewrite","tenantId":"attacker"}`, key: "strict", want: "PPT_REQUEST_INVALID", ready: true},
		{name: "blank slide id", body: `{"slideId":" ","instruction":"rewrite"}`, key: "blank-slide", want: "PPT_REQUEST_INVALID", ready: true},
		{name: "blank instruction", body: `{"slideId":"slide_1","instruction":" "}`, key: "blank-instruction", want: "PPT_REQUEST_INVALID", ready: true},
		{name: "draft task", body: `{"slideId":"slide_1","instruction":"rewrite"}`, key: "draft", want: "PPT_INVALID_STAGE"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 1)
			if testCase.ready {
				harness.state.makeReady(taskID, []pptapp.Slide{pptapp.NormalizeSlideIR(pptapp.Slide{
					ID: "slide_1", Page: 1, Blocks: []pptapp.SlideBlock{{Type: "title", Text: "Original"}},
				})})
			}
			var providerCalls atomic.Int32
			response := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/revise-slide", testCase.body, testCase.key, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
				providerCalls.Add(1)
				return chatprovider.Response{}, nil
			})
			assertPPTAgentError(t, response, map[string]int{"PPT_IDEMPOTENCY_KEY_REQUIRED": http.StatusBadRequest, "PPT_REQUEST_INVALID": http.StatusBadRequest, "PPT_INVALID_STAGE": http.StatusConflict}[testCase.want], testCase.want)
			if providerCalls.Load() != 0 {
				t.Fatalf("invalid revision called provider %d times", providerCalls.Load())
			}
		})
	}
}

func TestPPTImportOutlineRejectsInvalidRequestShapesBeforeStorage(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "missing list", body: `{}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "empty list", body: `{"sourceFileIds":[]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "empty id", body: `{"sourceFileIds":[""]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "too many", body: `{"sourceFileIds":["file_a","file_b","file_c","file_d"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "url", body: `{"sourceFileIds":["https://files.example/report.md"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "unix absolute", body: `{"sourceFileIds":["/etc/passwd"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "windows absolute", body: `{"sourceFileIds":["C:\\\\secret.md"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "relative traversal", body: `{"sourceFileIds":["../secret.md"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "separator", body: `{"sourceFileIds":["file_a/b"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "encoded separator", body: `{"sourceFileIds":["file_a%2fb"]}`, code: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "tenant override", body: `{"sourceFileIds":["file_a"],"tenantId":"attacker"}`, code: "PPT_REQUEST_INVALID"},
		{name: "user override", body: `{"sourceFileIds":["file_a"],"userId":"attacker"}`, code: "PPT_REQUEST_INVALID"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 2)
			files := newPPTAgentImportFileStore()
			var chatCalls atomic.Int32
			response := harness.requestImport(t, taskID, testCase.body, "invalid-import", files, nil, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
				chatCalls.Add(1)
				return validPPTAgentChatResponse(), nil
			})
			assertPPTAgentError(t, response, http.StatusBadRequest, testCase.code)
			if files.openCalls.Load() != 0 || chatCalls.Load() != 0 {
				t.Fatalf("invalid request reached storage/provider: opens=%d provider=%d", files.openCalls.Load(), chatCalls.Load())
			}
		})
	}
}

func TestPPTImportOutlineRequiresIdempotencyKey(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	files := newPPTAgentImportFileStore()
	files.files["file_report"] = newPPTAgentMarkdownTestFile("file_report", harness.state.owner(taskID), "report.md", "text/markdown", "# Report\nBody")
	response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_report"]}`, "", files, nil, validPPTAgentChatFunc)
	assertPPTAgentError(t, response, http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED")
	if files.openCalls.Load() != 0 {
		t.Fatalf("missing idempotency key opened %d files", files.openCalls.Load())
	}
}

func TestPPTImportOutlineMapsFileCenterDenialsAndClosesStreams(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*pptAgentImportTestFile, string)
		wantStatus int
		wantCode   string
		wantClose  int32
	}{
		{name: "not found", configure: func(file *pptAgentImportTestFile, _ string) { file.openErr = storagecenter.ErrFileNotFound }, wantStatus: http.StatusNotFound, wantCode: "PPT_SOURCE_FILE_NOT_FOUND"},
		{name: "file center forbidden", configure: func(file *pptAgentImportTestFile, _ string) { file.openErr = storagecenter.ErrFileForbidden }, wantStatus: http.StatusForbidden, wantCode: "PPT_SOURCE_FILE_FORBIDDEN"},
		{name: "deleted", configure: func(file *pptAgentImportTestFile, _ string) { file.object.Status = storagecenter.StatusDeleted }, wantStatus: http.StatusNotFound, wantCode: "PPT_SOURCE_FILE_NOT_FOUND", wantClose: 1},
		{name: "quarantined", configure: func(file *pptAgentImportTestFile, _ string) { file.object.Status = storagecenter.StatusQuarantined }, wantStatus: http.StatusNotFound, wantCode: "PPT_SOURCE_FILE_NOT_FOUND", wantClose: 1},
		{name: "cross tenant metadata", configure: func(file *pptAgentImportTestFile, _ string) { file.object.TenantID = "tenant_other" }, wantStatus: http.StatusForbidden, wantCode: "PPT_SOURCE_FILE_FORBIDDEN", wantClose: 1},
		{name: "other private owner", configure: func(file *pptAgentImportTestFile, _ string) { file.object.UserID = "user_other" }, wantStatus: http.StatusForbidden, wantCode: "PPT_SOURCE_FILE_FORBIDDEN", wantClose: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 2)
			owner := harness.state.owner(taskID)
			files := newPPTAgentImportFileStore()
			file := newPPTAgentMarkdownTestFile("file_report", owner, "report.md", "text/markdown", "# Report\nSafe body")
			testCase.configure(file, owner)
			files.files[file.object.FileID] = file
			var chatCalls atomic.Int32
			response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_report"]}`, "denied-import", files, nil, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
				chatCalls.Add(1)
				return validPPTAgentChatResponse(), nil
			})
			assertPPTAgentError(t, response, testCase.wantStatus, testCase.wantCode)
			if file.closeCalls.Load() != testCase.wantClose || chatCalls.Load() != 0 {
				t.Fatalf("denied import close/provider = %d/%d, want %d/0", file.closeCalls.Load(), chatCalls.Load(), testCase.wantClose)
			}
			if len(files.accesses) != 1 || files.accesses[0].TenantID != "tenant_default" || files.accesses[0].UserID != owner || files.accesses[0].IsAdmin {
				t.Fatalf("storage access was not server-derived: %#v", files.accesses)
			}
		})
	}
}

func TestPPTImportOutlineRequiresMatchingMarkdownExtensionAndMIME(t *testing.T) {
	tests := []struct {
		name string
		file *pptAgentImportTestFile
	}{
		{name: "unsupported extension", file: newPPTAgentMarkdownTestFile("file_txt", "", "report.txt", "text/markdown", "# Report\nBody")},
		{name: "unsupported mime", file: newPPTAgentMarkdownTestFile("file_plain", "", "report.md", "text/plain", "# Report\nBody")},
		{name: "double extension", file: newPPTAgentMarkdownTestFile("file_double", "", "report.pdf.md", "text/markdown", "# Report\nBody")},
		{name: "extension metadata mismatch", file: newPPTAgentMarkdownTestFile("file_mismatch", "", "report.md", "text/markdown", "# Report\nBody")},
	}
	tests[3].file.object.Extension = "markdown"
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 2)
			testCase.file.object.UserID = harness.state.owner(taskID)
			files := newPPTAgentImportFileStore()
			files.files[testCase.file.object.FileID] = testCase.file
			response := harness.requestImport(t, taskID, `{"sourceFileIds":["`+testCase.file.object.FileID+`"]}`, "type-import", files, nil, validPPTAgentChatFunc)
			assertPPTAgentError(t, response, http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED")
			if testCase.file.closeCalls.Load() != 1 {
				t.Fatalf("unsupported file stream close calls = %d, want 1", testCase.file.closeCalls.Load())
			}
		})
	}
}

func TestPPTImportOutlineBoundsStreamAndMergedRuneCount(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*pptAgentImportTestFile)
		wantStatus int
		wantCode   string
	}{
		{name: "declared over ten MiB", configure: func(file *pptAgentImportTestFile) { file.object.FileSize = pptAgentMaxSourceFileBytes + 1 }, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "PPT_SOURCE_FILE_TOO_LARGE"},
		{name: "stream over ten MiB", configure: func(file *pptAgentImportTestFile) {
			file.content = bytes.Repeat([]byte("a"), int(pptAgentMaxSourceFileBytes+1))
			file.object.FileSize = 1
		}, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "PPT_SOURCE_TEXT_TOO_LARGE"},
		{name: "read failure", configure: func(file *pptAgentImportTestFile) { file.readErr = errors.New("object-key-secret") }, wantStatus: http.StatusServiceUnavailable, wantCode: "PPT_SESSION_STORAGE_UNAVAILABLE"},
		{name: "close failure", configure: func(file *pptAgentImportTestFile) { file.closeErr = errors.New("close-internal-secret") }, wantStatus: http.StatusServiceUnavailable, wantCode: "PPT_SESSION_STORAGE_UNAVAILABLE"},
		{name: "merged text over rune limit", configure: func(file *pptAgentImportTestFile) {
			file.content = []byte(strings.Repeat("界", pptAgentMaxSourceTextRunes+1))
			file.object.FileSize = int64(len(file.content))
		}, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "PPT_SOURCE_TEXT_TOO_LARGE"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 2)
			file := newPPTAgentMarkdownTestFile("file_bounded", harness.state.owner(taskID), "report.md", "text/markdown; charset=utf-8", "# Report\nBody")
			testCase.configure(file)
			files := newPPTAgentImportFileStore()
			files.files[file.object.FileID] = file
			response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_bounded"]}`, "bounded-import", files, nil, validPPTAgentChatFunc)
			assertPPTAgentError(t, response, testCase.wantStatus, testCase.wantCode)
			if file.closeCalls.Load() != 1 {
				t.Fatalf("bounded stream close calls = %d, want 1", file.closeCalls.Load())
			}
		})
	}
}

func TestPPTImportOutlineStopsBeforeParserWhenSingleFileRawBudgetIsExceeded(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	owner := harness.state.owner(taskID)
	files := newPPTAgentImportFileStore()
	content := strings.Repeat("# h\nbody\n", 25001)
	file := newPPTAgentMarkdownTestFile("file_many_headings", owner, "report.md", "text/markdown", content)
	files.files[file.object.FileID] = file
	var parserCalls atomic.Int32
	var providerCalls atomic.Int32
	parse := func(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		parserCalls.Add(1)
		return (parser.Markdown{}).Parse(ctx, source)
	}
	response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_many_headings"]}`, "many-headings", files, parse, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		providerCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	})
	assertPPTAgentError(t, response, http.StatusRequestEntityTooLarge, "PPT_SOURCE_TEXT_TOO_LARGE")
	if parserCalls.Load() != 0 || providerCalls.Load() != 0 || file.closeCalls.Load() != 1 {
		t.Fatalf("pre-parse single-file budget parser/provider/close = %d/%d/%d, want 0/0/1", parserCalls.Load(), providerCalls.Load(), file.closeCalls.Load())
	}
}

func TestPPTImportOutlineStopsBeforeAnyParserWhenThreeFileRawBudgetIsExceeded(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	owner := harness.state.owner(taskID)
	files := newPPTAgentImportFileStore()
	fileIDs := []string{"file_part_a", "file_part_b", "file_part_c"}
	for _, fileID := range fileIDs {
		files.files[fileID] = newPPTAgentMarkdownTestFile(fileID, owner, fileID+".md", "text/markdown", strings.Repeat("界", 70000))
	}
	var parserCalls atomic.Int32
	var providerCalls atomic.Int32
	parse := func(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		parserCalls.Add(1)
		return (parser.Markdown{}).Parse(ctx, source)
	}
	response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_part_a","file_part_b","file_part_c"]}`, "aggregate-raw", files, parse, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		providerCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	})
	assertPPTAgentError(t, response, http.StatusRequestEntityTooLarge, "PPT_SOURCE_TEXT_TOO_LARGE")
	if parserCalls.Load() != 0 || providerCalls.Load() != 0 {
		t.Fatalf("aggregate raw budget reached parser/provider = %d/%d", parserCalls.Load(), providerCalls.Load())
	}
	for _, fileID := range fileIDs {
		if got := files.files[fileID].closeCalls.Load(); got != 1 {
			t.Fatalf("aggregate raw budget close %s = %d, want 1", fileID, got)
		}
	}
}

func TestPPTImportOutlineRejectsInvalidUTF8BeforeParser(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	file := newPPTAgentMarkdownTestFile("file_invalid_utf8", harness.state.owner(taskID), "report.md", "text/markdown", "valid")
	file.content = []byte{0xff, 0xfe, 0xfd}
	file.object.FileSize = int64(len(file.content))
	files := newPPTAgentImportFileStore()
	files.files[file.object.FileID] = file
	var parserCalls atomic.Int32
	var providerCalls atomic.Int32
	parse := func(context.Context, knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		parserCalls.Add(1)
		return nil, nil
	}
	response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_invalid_utf8"]}`, "invalid-utf8", files, parse, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		providerCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	})
	assertPPTAgentError(t, response, http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED")
	if parserCalls.Load() != 0 || providerCalls.Load() != 0 || file.closeCalls.Load() != 1 {
		t.Fatalf("invalid UTF-8 parser/provider/close = %d/%d/%d", parserCalls.Load(), providerCalls.Load(), file.closeCalls.Load())
	}
}

func TestPPTImportOutlineRejectsExpiredActiveFileAndAllowsSafeRetry(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	file := newPPTAgentMarkdownTestFile("file_expired", harness.state.owner(taskID), "report.md", "text/markdown", "# Report\nBody")
	expiredAt := time.Now().UTC().Add(-time.Minute)
	file.object.ExpiresAt = &expiredAt
	files := newPPTAgentImportFileStore()
	files.files[file.object.FileID] = file
	var parserCalls atomic.Int32
	var providerCalls atomic.Int32
	parse := func(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		parserCalls.Add(1)
		return (parser.Markdown{}).Parse(ctx, source)
	}
	chat := func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		providerCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	}
	first := harness.requestImport(t, taskID, `{"sourceFileIds":["file_expired"]}`, "expired-retry", files, parse, chat)
	assertPPTAgentError(t, first, http.StatusNotFound, "PPT_SOURCE_FILE_NOT_FOUND")
	if parserCalls.Load() != 0 || providerCalls.Load() != 0 || file.closeCalls.Load() != 1 {
		t.Fatalf("expired file parser/provider/close = %d/%d/%d", parserCalls.Load(), providerCalls.Load(), file.closeCalls.Load())
	}
	file.object.ExpiresAt = nil
	second := harness.requestImport(t, taskID, `{"sourceFileIds":["file_expired"]}`, "expired-retry", files, parse, chat)
	if second.Code != http.StatusOK {
		t.Fatalf("safe retry status = %d, body = %s", second.Code, second.Body.String())
	}
	if parserCalls.Load() != 1 || providerCalls.Load() != 1 || file.closeCalls.Load() != 2 {
		t.Fatalf("safe retry parser/provider/close = %d/%d/%d", parserCalls.Load(), providerCalls.Load(), file.closeCalls.Load())
	}
}

func TestPPTImportOutlineRejectsMismatchedIDAndUnknownVisibility(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		configure func(*pptAgentImportTestFile)
	}{
		{name: "returned file id mismatch", configure: func(file *pptAgentImportTestFile) { file.object.FileID = "file_other" }},
		{name: "unknown visibility", configure: func(file *pptAgentImportTestFile) { file.object.Visibility = "MYSTERY" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 2)
			file := newPPTAgentMarkdownTestFile("file_requested", harness.state.owner(taskID), "report.md", "text/markdown", "# Report\nBody")
			testCase.configure(file)
			files := newPPTAgentImportFileStore()
			files.files["file_requested"] = file
			var parserCalls atomic.Int32
			var providerCalls atomic.Int32
			response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_requested"]}`, "metadata-defense", files, func(context.Context, knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
				parserCalls.Add(1)
				return nil, nil
			}, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
				providerCalls.Add(1)
				return validPPTAgentChatResponse(), nil
			})
			assertPPTAgentError(t, response, http.StatusForbidden, "PPT_SOURCE_FILE_FORBIDDEN")
			if parserCalls.Load() != 0 || providerCalls.Load() != 0 || file.closeCalls.Load() != 1 {
				t.Fatalf("metadata defense parser/provider/close = %d/%d/%d", parserCalls.Load(), providerCalls.Load(), file.closeCalls.Load())
			}
		})
	}
}

func TestPPTImportOutlineMetadataRejectionCloseFailureTakesStoragePriority(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	file := newPPTAgentMarkdownTestFile("file_bad_type_close", harness.state.owner(taskID), "report.txt", "text/plain", "Body")
	file.closeErr = errors.New("close-storage-secret")
	files := newPPTAgentImportFileStore()
	files.files[file.object.FileID] = file
	var parserCalls atomic.Int32
	var providerCalls atomic.Int32
	response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_bad_type_close"]}`, "metadata-close", files, func(context.Context, knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		parserCalls.Add(1)
		return nil, nil
	}, func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		providerCalls.Add(1)
		return validPPTAgentChatResponse(), nil
	})
	assertPPTAgentError(t, response, http.StatusServiceUnavailable, "PPT_SESSION_STORAGE_UNAVAILABLE")
	if strings.Contains(response.Body.String(), "close-storage-secret") || parserCalls.Load() != 0 || providerCalls.Load() != 0 || file.closeCalls.Load() != 1 {
		t.Fatalf("metadata close priority leaked/reached work: parser=%d provider=%d close=%d body=%s", parserCalls.Load(), providerCalls.Load(), file.closeCalls.Load(), response.Body.String())
	}
}

func TestPPTImportOutlineAllowsExactlyTwoHundredThousandNormalizedRunes(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	file := newPPTAgentMarkdownTestFile("file_boundary", harness.state.owner(taskID), "report.md", "text/markdown", "source")
	files := newPPTAgentImportFileStore()
	files.files[file.object.FileID] = file
	parse := func(context.Context, knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		return []knowledgeapp.DocumentUnit{{Content: strings.Repeat("界", pptAgentMaxSourceTextRunes)}}, nil
	}
	var providerRunes int
	response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_boundary"]}`, "rune-boundary", files, parse, func(_ context.Context, req generation.CreateRequest) (chatprovider.Response, error) {
		messages := req.Params["messages"].([]chatprovider.Message)
		providerRunes = len([]rune(messages[len(messages)-1].Content))
		return validPPTAgentChatResponse(), nil
	})
	if response.Code != http.StatusOK {
		t.Fatalf("exact rune boundary status = %d, body = %s", response.Code, response.Body.String())
	}
	if providerRunes != pptAgentMaxSourceTextRunes || file.closeCalls.Load() != 1 {
		t.Fatalf("exact rune boundary provider/close = %d/%d", providerRunes, file.closeCalls.Load())
	}
}

func TestPPTImportOutlineParserFailureAndEmptyInputFailClosed(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		body   string
		parser pptAgentMarkdownParseFunc
	}{
		{name: "empty input", body: " \n\t"},
		{name: "parser failure", body: "# Report\nBody", parser: func(context.Context, knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
			return nil, errors.New("parser-internal-secret")
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newPPTAgentHTTPHarness(t)
			taskID := harness.createSession(t, "general", 2)
			file := newPPTAgentMarkdownTestFile("file_parse", harness.state.owner(taskID), "report.md", "text/markdown", testCase.body)
			files := newPPTAgentImportFileStore()
			files.files[file.object.FileID] = file
			response := harness.requestImport(t, taskID, `{"sourceFileIds":["file_parse"]}`, "parse-import", files, testCase.parser, validPPTAgentChatFunc)
			assertPPTAgentError(t, response, http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED")
			if strings.Contains(response.Body.String(), "parser-internal-secret") || file.closeCalls.Load() != 1 {
				t.Fatalf("parser failure leaked or left stream open: closes=%d body=%s", file.closeCalls.Load(), response.Body.String())
			}
		})
	}
}

func TestPPTImportOutlineValidMarkdownPersistsFilesAndReplaysWithoutWork(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := harness.createSession(t, "general", 2)
	owner := harness.state.owner(taskID)
	files := newPPTAgentImportFileStore()
	firstFile := newPPTAgentMarkdownTestFile("file_first", owner, "REPORT.MD", "TEXT/MARKDOWN; CHARSET=UTF-8", "# Alpha\nFIRST_IMPORT_SECRET")
	secondFile := newPPTAgentMarkdownTestFile("file_second", owner, "notes.markdown", "text/x-markdown", "# Beta\nSECOND_IMPORT_SECRET")
	files.files[firstFile.object.FileID] = firstFile
	files.files[secondFile.object.FileID] = secondFile
	var chatCalls atomic.Int32
	chat := func(_ context.Context, req generation.CreateRequest) (chatprovider.Response, error) {
		chatCalls.Add(1)
		messages, ok := req.Params["messages"].([]chatprovider.Message)
		if !ok || len(messages) == 0 {
			t.Fatalf("provider messages = %#v", req.Params["messages"])
		}
		content := messages[len(messages)-1].Content
		if !strings.Contains(content, "FIRST_IMPORT_SECRET") || !strings.Contains(content, "SECOND_IMPORT_SECRET") || len([]rune(content)) > pptAgentMaxSourceTextRunes {
			t.Fatalf("provider import content was incomplete or unbounded: %d runes", len([]rune(content)))
		}
		return validPPTAgentChatResponse(), nil
	}
	body := `{"sourceFileIds":[" file_first ","file_second","file_first"]}`
	first := harness.requestImport(t, taskID, body, "valid-import", files, nil, chat)
	if first.Code != http.StatusOK {
		t.Fatalf("valid import status = %d, body = %s", first.Code, first.Body.String())
	}
	var response pptTaskPublicResponse
	if err := json.Unmarshal(first.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Stage != pptapp.StageOutlineReady || len(response.SourceFileIDs) != 2 || response.SourceFileIDs[0] != "file_first" || response.SourceFileIDs[1] != "file_second" {
		t.Fatalf("valid import response = %#v", response)
	}
	if firstFile.closeCalls.Load() != 1 || secondFile.closeCalls.Load() != 1 || chatCalls.Load() != 1 {
		t.Fatalf("valid import work = close %d/%d provider %d", firstFile.closeCalls.Load(), secondFile.closeCalls.Load(), chatCalls.Load())
	}
	replay := harness.requestImport(t, taskID, body, "valid-import", files, nil, chat)
	if replay.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatalf("completed import replay changed: first=%s replay=%s", first.Body.String(), replay.Body.String())
	}
	if firstFile.closeCalls.Load() != 1 || secondFile.closeCalls.Load() != 1 || files.openCalls.Load() != 2 || chatCalls.Load() != 1 {
		t.Fatalf("replay repeated work: opens=%d close=%d/%d provider=%d", files.openCalls.Load(), firstFile.closeCalls.Load(), secondFile.closeCalls.Load(), chatCalls.Load())
	}
	task, err := harness.state.GetTask(context.Background(), harness.state.ownerScope(taskID), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(task.SourceFileIDs) != 2 || task.SourceFileIDs[0] != "file_first" || task.SourceFileIDs[1] != "file_second" || len(task.AgentMessages) != 2 {
		t.Fatalf("valid import task = %#v", task)
	}
	for _, message := range task.AgentMessages {
		if strings.Contains(message.Content, "FIRST_IMPORT_SECRET") || strings.Contains(message.Content, "SECOND_IMPORT_SECRET") {
			t.Fatalf("raw imported file content was persisted in agent messages: %#v", task.AgentMessages)
		}
	}
	conflict := harness.requestImport(t, taskID, `{"sourceFileIds":["file_second","file_first"]}`, "valid-import", files, nil, chat)
	assertPPTAgentError(t, conflict, http.StatusConflict, "PPT_IDEMPOTENCY_CONFLICT")
	if files.openCalls.Load() != 2 || chatCalls.Load() != 1 {
		t.Fatalf("ordered hash conflict repeated work: opens=%d provider=%d", files.openCalls.Load(), chatCalls.Load())
	}
}

func TestPPTImportOutlineRejectsCompletedReplayAfterTenantSwitch(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	harness.authorizations.set(modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: "tenant_a", OrganizationID: "organization_a",
		BillingScope: contextEnterprise, BillingAccountID: "tenant_a", ServiceState: "ACTIVE",
	})
	taskID := harness.createSession(t, "general", 2)
	owner := harness.state.owner(taskID)
	files := newPPTAgentImportFileStore()
	file := newPPTAgentMarkdownTestFile("file_enterprise", owner, "report.md", "text/markdown", "# Report\nBody")
	file.object.TenantID = "tenant_a"
	files.files[file.object.FileID] = file
	first := harness.requestImport(t, taskID, `{"sourceFileIds":["file_enterprise"]}`, "tenant-import", files, nil, validPPTAgentChatFunc)
	if first.Code != http.StatusOK {
		t.Fatalf("enterprise import status = %d, body = %s", first.Code, first.Body.String())
	}
	beginCalls := harness.state.beginCalls
	harness.authorizations.set(modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: "tenant_b", OrganizationID: "organization_b",
		BillingScope: contextEnterprise, BillingAccountID: "tenant_b", ServiceState: "ACTIVE",
	})
	replay := harness.requestImport(t, taskID, `{"sourceFileIds":["file_enterprise"]}`, "tenant-import", files, nil, validPPTAgentChatFunc)
	assertPPTAgentError(t, replay, http.StatusNotFound, "PPT_TASK_NOT_FOUND")
	if harness.state.beginCalls != beginCalls || files.openCalls.Load() != 1 {
		t.Fatalf("tenant switch reached idempotency/storage: begin=%d/%d opens=%d", harness.state.beginCalls, beginCalls, files.openCalls.Load())
	}
}

func TestPPTTaskResponseAllowlistOmitsPersistenceAndProviderInternals(t *testing.T) {
	task := pptapp.Task{
		TaskID: "ppt_1", SessionID: "ppt_1", UserID: "user_secret", ClientRequestID: "client_secret",
		SkillCode: "general", Stage: pptapp.StageGenerating, Status: pptapp.StatusProcessing,
		Prompt: "visible user prompt", TextModel: "private-model", ImageModel: "private-image-model",
		BillingTaskID: "billing_secret", GenerationLease: &pptapp.GenerationLease{RunToken: "run_secret", LeaseUntil: "tomorrow"},
		IdempotencyRecords: []pptapp.IdempotencyRecord{{RequestHash: "hash_secret", OperationToken: "op_secret", ResponseJSON: "snapshot_secret"}},
		AgentMessages: []pptapp.AgentMessage{
			{Role: "user", Content: "visible user message"},
			{Role: "assistant", Content: `{"title":"provider_raw_secret","pages":[]}`},
		},
		Slides: []pptapp.Slide{{
			ID: "slide_1", Page: 1, Layout: "content", VisualTaskID: "provider_task_secret", VisualModelName: "provider_model_secret",
			Blocks: []pptapp.SlideBlock{{Type: "title", Text: "Visible"}, {Type: "paragraph", Text: "Visible body"}},
		}},
	}
	raw, err := json.Marshal(pptTaskResponse(task))
	if err != nil {
		t.Fatal(err)
	}
	assertNoPPTAgentInternalFields(t, raw)
	for _, secret := range []string{"user_secret", "client_secret", "private-model", "billing_secret", "run_secret", "hash_secret", "op_secret", "snapshot_secret", "provider_task_secret", "provider_model_secret", "provider_raw_secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("allowlist response leaked %q: %s", secret, raw)
		}
	}
	if !strings.Contains(string(raw), `"title":"Visible"`) {
		t.Fatalf("allowlist response removed public slide fields: %s", raw)
	}
	if !strings.Contains(string(raw), `"prompt":"visible user prompt"`) {
		t.Fatalf("allowlist response removed the legacy public prompt field: %s", raw)
	}
	if !strings.Contains(string(raw), `"content":"visible user message"`) {
		t.Fatalf("allowlist response removed the public user conversation: %s", raw)
	}
}

func TestPPTTaskResponsePreservesOnlyApprovedStableErrorCodes(t *testing.T) {
	approved := []string{
		"PPT_SKILL_NOT_FOUND", "PPT_INVALID_STAGE", "PPT_IDEMPOTENCY_KEY_REQUIRED", "PPT_IDEMPOTENCY_CONFLICT",
		"PPT_OUTLINE_REQUIRED", "PPT_SESSION_CANCELLED", "PPT_GENERATION_ALREADY_RUNNING",
		"PPT_AGENT_PROVIDER_UNAVAILABLE", "PPT_AGENT_RESPONSE_INVALID", "PPT_SOURCE_FILE_NOT_FOUND",
		"PPT_SOURCE_FILE_FORBIDDEN", "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "PPT_SOURCE_FILE_TOO_LARGE",
		"PPT_SOURCE_TEXT_TOO_LARGE", "PPT_BILLING_RESERVATION_FAILED", "PPT_BILLING_FINALIZE_FAILED",
		"PPT_IDEMPOTENCY_KEY_INVALID", "PPT_OPERATION_IN_PROGRESS", "PPT_OPERATION_CLEANUP_FAILED",
		"PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT_TENANT_CONTEXT_MISMATCH", "PPT_TASK_NOT_FOUND",
		"PPT_POSTGRES_UNAVAILABLE", "PPT_OPERATION_TOKEN_MISMATCH", "PPT_SESSION_STORAGE_UNAVAILABLE",
		"PPT_GENERATION_RUN_MISMATCH", "PPT_GENERATION_INCOMPLETE", "PPT_SLIDE_COORDINATE_INVALID",
		"PPT_SLIDE_COORDINATE_CONFLICT", "PPT_BILLING_TASK_REQUIRED", "PPT_BILLING_BINDING_MISSING",
		"PPT_BILLING_BINDING_MISMATCH",
	}
	for _, code := range approved {
		raw, err := json.Marshal(pptTaskResponse(pptapp.Task{TaskID: "ppt_error", Stage: pptapp.StageFailed, ErrorCode: code, ErrorMessage: "internal details"}))
		if err != nil {
			t.Fatal(err)
		}
		var response map[string]json.RawMessage
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		if got := stringJSON(t, response["errorCode"]); got != code {
			t.Fatalf("approved error code %q mapped to %q: %s", code, got, raw)
		}
		if strings.Contains(string(raw), "internal details") {
			t.Fatalf("approved code leaked independent internal message: %s", raw)
		}
	}
	raw, err := json.Marshal(pptTaskResponse(pptapp.Task{TaskID: "ppt_internal", Stage: pptapp.StageFailed, ErrorCode: "PPT_INTERNAL_SECRET", ErrorMessage: "internal-database-secret"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PPT_INTERNAL_SECRET") || strings.Contains(string(raw), "internal-database-secret") {
		t.Fatalf("unknown error leaked through public mapper: %s", raw)
	}
}

func TestPPTAgentRoutesUsePublicDTOForCreateAndCompletedMessage(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	harness.authorizations.set(modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: "tenant_secret", OrganizationID: "organization_secret",
		BillingScope: contextEnterprise, BillingAccountID: "billing_secret", ServiceState: "ACTIVE",
	})
	harness.state.secretRich = true
	unknownCreated := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", `{"prompt":"safe prompt","skillCode":"general","slideCount":2}`, "", nil)
	if unknownCreated.Code != http.StatusOK {
		t.Fatalf("unknown-code create status = %d, body = %s", unknownCreated.Code, unknownCreated.Body.String())
	}
	assertNoPPTAgentInternalFields(t, unknownCreated.Body.Bytes())
	assertNoPPTAgentSecretValues(t, unknownCreated.Body.Bytes())
	assertPPTTaskPublicErrorCode(t, unknownCreated.Body.Bytes(), "")
	var payload struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(unknownCreated.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	unknownCompleted := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+payload.TaskID+"/messages", `{"message":"生成两页大纲"}`, "dto-message", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		return validPPTAgentChatResponse(), nil
	})
	if unknownCompleted.Code != http.StatusOK {
		t.Fatalf("unknown-code completed message status = %d, body = %s", unknownCompleted.Code, unknownCompleted.Body.String())
	}
	assertNoPPTAgentInternalFields(t, unknownCompleted.Body.Bytes())
	assertNoPPTAgentSecretValues(t, unknownCompleted.Body.Bytes())
	assertPPTTaskPublicErrorCode(t, unknownCompleted.Body.Bytes(), "")

	harness.state.secretErrorCode = "PPT_AGENT_PROVIDER_UNAVAILABLE"
	approvedCreated := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions", `{"prompt":"approved error","skillCode":"general","slideCount":2}`, "", nil)
	if approvedCreated.Code != http.StatusOK {
		t.Fatalf("approved-code create status = %d, body = %s", approvedCreated.Code, approvedCreated.Body.String())
	}
	assertNoPPTAgentInternalFields(t, approvedCreated.Body.Bytes())
	assertNoPPTAgentSecretValues(t, approvedCreated.Body.Bytes())
	assertPPTTaskPublicErrorCode(t, approvedCreated.Body.Bytes(), "PPT_AGENT_PROVIDER_UNAVAILABLE")
	if err := json.Unmarshal(approvedCreated.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	approvedCompleted := harness.request(t, http.MethodPost, "/api/v1/ppt/sessions/"+payload.TaskID+"/messages", `{"message":"生成两页大纲"}`, "dto-message-approved", func(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
		return validPPTAgentChatResponse(), nil
	})
	if approvedCompleted.Code != http.StatusOK {
		t.Fatalf("approved-code completed message status = %d, body = %s", approvedCompleted.Code, approvedCompleted.Body.String())
	}
	assertNoPPTAgentInternalFields(t, approvedCompleted.Body.Bytes())
	assertNoPPTAgentSecretValues(t, approvedCompleted.Body.Bytes())
	assertPPTTaskPublicErrorCode(t, approvedCompleted.Body.Bytes(), "PPT_AGENT_PROVIDER_UNAVAILABLE")
}

func TestLegacyPPTDetailAndHistoryRoutesUsePublicDTO(t *testing.T) {
	harness, unknownTaskID, approvedTaskID := newPPTAgentHTTPHarnessWithPersistedSecretTasks(t)
	for _, testCase := range []struct {
		path          string
		wantErrorCode string
	}{
		{path: "/api/v1/ppt/tasks/" + unknownTaskID},
		{path: "/api/v1/ppt/tasks/" + approvedTaskID, wantErrorCode: "PPT_AGENT_PROVIDER_UNAVAILABLE"},
		{path: "/api/v1/ppt/history", wantErrorCode: "PPT_AGENT_PROVIDER_UNAVAILABLE"},
	} {
		response := harness.request(t, http.MethodGet, testCase.path, "", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", testCase.path, response.Code, response.Body.String())
		}
		assertNoPPTAgentInternalFields(t, response.Body.Bytes())
		assertNoPPTAgentSecretValues(t, response.Body.Bytes())
		assertPPTTaskPublicErrorCode(t, response.Body.Bytes(), testCase.wantErrorCode)
	}
}

func TestPPTAgentRoutesRequireAuthentication(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	for _, requestCase := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/ppt/skills"},
		{method: http.MethodPost, path: "/api/v1/ppt/sessions", body: `{"prompt":"deck","skillCode":"general"}`},
		{method: http.MethodPost, path: "/api/v1/ppt/sessions/ppt_unknown/messages", body: `{"message":"outline"}`},
		{method: http.MethodPost, path: "/api/v1/ppt/sessions/ppt_unknown/import-outline", body: `{"sourceFileIds":["file_unknown"]}`},
		{method: http.MethodPost, path: "/api/v1/ppt/sessions/ppt_unknown/revise-slide", body: `{"slideId":"slide_1","instruction":"rewrite"}`},
	} {
		request := httptest.NewRequest(requestCase.method, requestCase.path, strings.NewReader(requestCase.body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		harness.handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401, body = %s", requestCase.method, requestCase.path, response.Code, response.Body.String())
		}
	}
}

func TestPPTAgentPromptContainsOnlySelectedSkillConversationOutlineAndCurrentMessage(t *testing.T) {
	selected, ok := skills.Resolve("weekly_report")
	if !ok {
		t.Fatal("weekly_report skill missing")
	}
	other, _ := skills.Resolve("pitch_deck")
	task := pptapp.Task{
		Prompt: "INITIAL_PRIVATE_PROMPT_MUST_NOT_BE_SENT", BillingTaskID: "BILLING_MUST_NOT_BE_SENT",
		AgentMessages: []pptapp.AgentMessage{
			{Role: "system", Content: "UNTRUSTED_SYSTEM_MESSAGE"},
			{Role: " user ", Content: "  上一条   用户消息  "},
			{Role: "assistant", Content: "  上一条助手消息  "},
		},
		Outline: &pptapp.Outline{Title: "当前大纲", Slides: []pptapp.OutlineSlide{{Page: 1, Title: "现状", Summary: "摘要", BulletPoints: []string{"要点"}}}},
	}
	request := buildPPTAgentChatRequest(selected, task, "  当前   用户消息  ", "model_1")
	messages, ok := request.Params["messages"].([]chatprovider.Message)
	if !ok || len(messages) < 5 {
		t.Fatalf("unexpected prompt messages: %#v", request.Params["messages"])
	}
	combinedRaw, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	combined := string(combinedRaw)
	if !strings.Contains(combined, selected.SystemPrompt) || strings.Contains(combined, other.SystemPrompt) {
		t.Fatalf("selected skill prompt isolation failed: %s", combined)
	}
	for _, forbidden := range []string{"INITIAL_PRIVATE_PROMPT_MUST_NOT_BE_SENT", "BILLING_MUST_NOT_BE_SENT", "UNTRUSTED_SYSTEM_MESSAGE"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("provider prompt leaked %q: %s", forbidden, combined)
		}
	}
	if messages[len(messages)-1].Role != "user" || messages[len(messages)-1].Content != "当前 用户消息" {
		t.Fatalf("current message was not normalized/last: %#v", messages[len(messages)-1])
	}
	if !strings.Contains(combined, "当前大纲") || !strings.Contains(combined, "上一条 用户消息") || !strings.Contains(combined, "上一条助手消息") {
		t.Fatalf("prompt omitted allowed conversation/outline context: %s", combined)
	}
}

func TestPostgresPPTAPIAssemblyDoesNotImportOrMirrorLegacyTaskFile(t *testing.T) {
	db, fixture := newPPTHTTPTestFixture(t)
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	legacyPath := filepath.Join(filepath.Dir(dataPath), "ppt-tasks.json")
	legacyTaskID := "ppt_legacy_api_only"
	legacyContents := []byte(`{"tasks":[{"taskId":"ppt_legacy_api_only","userId":"` + fixture.owner.ID + `","status":"success"}]}`)
	if err := os.WriteFile(legacyPath, legacyContents, 0o600); err != nil {
		t.Fatal(err)
	}

	server := NewWithDatabase(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir(), PPTAutoImageMode: "disabled"}, db)
	handler := server.Handler
	token := fixture.loginToken(t, handler, fixture.owner)

	legacyResponse := fixture.authedRequest(t, handler, http.MethodGet, "/api/v1/ppt/tasks/"+legacyTaskID, nil, token)
	if legacyResponse.Code != http.StatusNotFound {
		t.Fatalf("legacy JSON task was visible through Postgres API: status=%d body=%s", legacyResponse.Code, legacyResponse.Body.String())
	}

	createResponse := fixture.authedRequest(t, handler, http.MethodPost, "/api/v1/ppt/generate", bytes.NewBufferString(`{
		"prompt":"Postgres-only source of truth",
		"slideCount":1,
		"imageSource":"none",
			"outline":{"title":"Postgres","slides":[{"page":1,"title":"Only","summary":"Database task","bulletPoints":["Persisted in PostgreSQL"],"layout":"cover","slideType":"cover"}]}
	}`), token)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("Postgres PPT create status=%d body=%s", createResponse.Code, createResponse.Body.String())
	}

	actualLegacyContents, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actualLegacyContents, legacyContents) {
		t.Fatalf("Postgres API mutated legacy file\nwant=%s\ngot=%s", legacyContents, actualLegacyContents)
	}
}

func TestNewPPTServiceForPostgresSelectsFailClosedPostgresService(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	legacyPath := filepath.Join(filepath.Dir(dataPath), "ppt-tasks.json")
	if err := os.WriteFile(legacyPath, []byte(`{"tasks":[{"taskId":"ppt_legacy_startup_read","userId":"user_legacy","status":"success"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	service := newPPTService(&postgresStore{})
	if _, err := service.HistoryWithError(pptapp.OwnerScope{TenantID: "tenant_default", UserID: "user_legacy"}); !errors.Is(err, pptapp.ErrPostgresUnavailable) {
		t.Fatalf("Postgres store must select the fail-closed Postgres service without constructing persistent JSON state: %v", err)
	}
}

func TestPPTExportRoutesRejectNonReadyOrIncompleteCanonicalTasks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*pptapp.Task)
	}{
		{name: "draft", mutate: func(task *pptapp.Task) { task.Stage, task.Status = pptapp.StageDraft, pptapp.StatusPending }},
		{name: "outline_ready", mutate: func(task *pptapp.Task) { task.Stage, task.Status = pptapp.StageOutlineReady, pptapp.StatusPending }},
		{name: "generating", mutate: func(task *pptapp.Task) { task.Stage, task.Status = pptapp.StageGenerating, pptapp.StatusProcessing }},
		{name: "failed", mutate: func(task *pptapp.Task) { task.Stage, task.Status = pptapp.StageFailed, pptapp.StatusFailed }},
		{name: "cancelled", mutate: func(task *pptapp.Task) { task.Stage, task.Status = pptapp.StageCancelled, pptapp.StatusCancelled }},
		{name: "ready_without_billing_binding", mutate: func(task *pptapp.Task) { task.BillingTaskID = "" }},
		{name: "ready_without_slides", mutate: func(task *pptapp.Task) { task.Slides = nil }},
		{name: "ready_with_missing_page", mutate: func(task *pptapp.Task) { task.Slides = task.Slides[:1] }},
		{name: "ready_with_duplicate_page", mutate: func(task *pptapp.Task) { task.Slides[1].Page = 1 }},
		{name: "ready_with_duplicate_id", mutate: func(task *pptapp.Task) { task.Slides[1].ID = task.Slides[0].ID }},
		{name: "ready_with_empty_id", mutate: func(task *pptapp.Task) { task.Slides[0].ID = "" }},
		{name: "ready_with_empty_blocks", mutate: func(task *pptapp.Task) { task.Slides[0].Blocks = nil }},
	}
	routes := []struct {
		name   string
		method string
		path   func(string) string
		body   func(string) string
	}{
		{
			name: "post_export", method: http.MethodPost,
			path: func(string) string { return "/api/v1/ppt/export/pptx" },
			body: func(taskID string) string { return fmt.Sprintf(`{"taskId":%q}`, taskID) },
		},
		{
			name: "get_export", method: http.MethodGet,
			path: func(taskID string) string { return "/api/v1/ppt/tasks/" + taskID + "/export/pptx" },
			body: func(string) string { return "" },
		},
	}

	for _, test := range tests {
		for _, route := range routes {
			t.Run(test.name+"/"+route.name, func(t *testing.T) {
				harness := newPPTAgentHTTPHarness(t)
				taskID := seedPPTAgentExportTask(t, harness, test.mutate)
				response := harness.request(t, route.method, route.path(taskID), route.body(taskID), "", nil)
				if response.Code == http.StatusOK {
					t.Fatalf("non-exportable task returned 200 with %d bytes", response.Body.Len())
				}
				if response.Header().Get("Content-Type") == "application/vnd.openxmlformats-officedocument.presentationml.presentation" || strings.HasPrefix(response.Body.String(), "PK") {
					t.Fatalf("non-exportable task returned PPTX payload: status=%d headers=%v", response.Code, response.Header())
				}
			})
		}
	}
}

func TestPPTExportRoutesAllowCompleteReadySameOwnerTask(t *testing.T) {
	harness := newPPTAgentHTTPHarness(t)
	taskID := seedPPTAgentExportTask(t, harness, nil)
	routes := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/ppt/export/pptx", body: fmt.Sprintf(`{"taskId":%q}`, taskID)},
		{method: http.MethodGet, path: "/api/v1/ppt/tasks/" + taskID + "/export/pptx"},
	}
	for _, route := range routes {
		response := harness.request(t, route.method, route.path, route.body, "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("export %s %s status=%d body=%s", route.method, route.path, response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.presentationml.presentation" || !strings.HasPrefix(response.Body.String(), "PK") {
			t.Fatalf("export %s %s did not return PPTX: headers=%v bytes=%d", route.method, route.path, response.Header(), response.Body.Len())
		}
	}
}

func seedPPTAgentExportTask(t *testing.T, harness pptAgentHTTPHarness, mutate func(*pptapp.Task)) string {
	t.Helper()
	taskID := harness.createSession(t, "general", 2)
	harness.state.mu.Lock()
	defer harness.state.mu.Unlock()
	task := harness.state.tasks[taskID]
	task.Stage = pptapp.StageReady
	task.Status = pptapp.StatusSuccess
	task.BillingTaskID = "billing-export-ready"
	task.SlideCount = 2
	task.Slides = []pptapp.Slide{
		{ID: "slide_1", Page: 1, Layout: "cover", Blocks: []pptapp.SlideBlock{{Type: "title", Text: "Ready cover"}}},
		{ID: "slide_2", Page: 2, Layout: "content", Blocks: []pptapp.SlideBlock{{Type: "title", Text: "Ready content"}, {Type: "paragraph", Text: "Complete body"}}},
	}
	if mutate != nil {
		mutate(&task)
	}
	harness.state.tasks[taskID] = task
	return taskID
}

type pptAgentHTTPHarness struct {
	handler        http.Handler
	token          string
	store          *jsonStore
	authorizations *pptAgentAuthorizationStore
	state          *pptAgentTestState
}

func newPPTAgentHTTPHarness(t *testing.T) pptAgentHTTPHarness {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(dataPath)
	authorizations := &pptAgentAuthorizationStore{
		jsonStore: store,
		authorization: modelCallAuthorization{
			ContextType: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"),
			BillingScope: contextPersonal, ServiceState: "ACTIVE",
		},
	}
	handler := newWithStore(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, authorizations).Handler
	return pptAgentHTTPHarness{
		handler:        handler,
		token:          loginToken(t, handler, "demo@xianzhi.ai", "Demo123!"),
		store:          store,
		authorizations: authorizations,
		state:          newPPTAgentTestState(),
	}
}

func newPPTAgentHTTPHarnessWithPersistedSecretTasks(t *testing.T) (pptAgentHTTPHarness, string, string) {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(dataPath)
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	userID := ""
	for _, user := range data.Users {
		if strings.EqualFold(user.Email, "demo@xianzhi.ai") {
			userID = user.ID
			break
		}
	}
	if userID == "" {
		t.Fatal("demo user missing")
	}
	unknownTask := secretRichPPTAgentTask(pptapp.Task{
		TaskID: "ppt_secret_route", SessionID: "ppt_secret_route", UserID: userID,
		SkillCode: "general", Stage: pptapp.StageDraft, Status: pptapp.StatusPending,
		Title: "Visible title", Prompt: "Visible prompt", SlideCount: 2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	unknownTask.TenantID = "tenant_default"
	approvedTask := unknownTask
	approvedTask.TaskID = "ppt_approved_error_route"
	approvedTask.SessionID = approvedTask.TaskID
	approvedTask.ErrorCode = "PPT_AGENT_PROVIDER_UNAVAILABLE"
	authorizations := &pptAgentAuthorizationStore{
		jsonStore: store,
		authorization: modelCallAuthorization{
			ContextType: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"),
			BillingScope: contextPersonal, ServiceState: "ACTIVE",
		},
	}
	handler := newWithStore(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()}, authorizations).Handler
	state := newPPTAgentTestState()
	state.tasks[unknownTask.TaskID] = unknownTask
	state.tasks[approvedTask.TaskID] = approvedTask
	return pptAgentHTTPHarness{
		handler: handler, token: loginToken(t, handler, "demo@xianzhi.ai", "Demo123!"), store: store,
		authorizations: authorizations, state: state,
	}, unknownTask.TaskID, approvedTask.TaskID
}

type pptAgentAuthorizationStore struct {
	*jsonStore
	mu            sync.Mutex
	authorization modelCallAuthorization
}

func (s *pptAgentAuthorizationStore) AuthorizeModelCall(userID string, _ string) (modelCallAuthorization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	authorization := s.authorization
	authorization.UserID = userID
	if strings.TrimSpace(authorization.BillingAccountID) == "" {
		authorization.BillingAccountID = userID
	}
	return authorization, nil
}

func (s *pptAgentAuthorizationStore) set(authorization modelCallAuthorization) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorization = authorization
}

func (h pptAgentHTTPHarness) request(t *testing.T, method, path, body, idempotencyKey string, chat pptAgentChatFunc) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+h.token)
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	ctx := context.WithValue(request.Context(), pptAgentStateContextKey{}, pptAgentStateStore(h.state))
	if chat != nil {
		ctx = context.WithValue(ctx, pptAgentChatContextKey{}, chat)
	}
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, request)
	return response
}

func (h pptAgentHTTPHarness) requestImport(t *testing.T, taskID, body, idempotencyKey string, files *pptAgentImportFileStore, parse pptAgentMarkdownParseFunc, chat pptAgentChatFunc) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ppt/sessions/"+taskID+"/import-outline", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+h.token)
	request.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	ctx := context.WithValue(request.Context(), pptAgentStateContextKey{}, pptAgentStateStore(h.state))
	ctx = context.WithValue(ctx, pptAgentFileStoreContextKey{}, pptAgentFileStore(files))
	if parse != nil {
		ctx = context.WithValue(ctx, pptAgentMarkdownParserContextKey{}, parse)
	}
	if chat != nil {
		ctx = context.WithValue(ctx, pptAgentChatContextKey{}, chat)
	}
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, request)
	return response
}

func (h pptAgentHTTPHarness) createSession(t *testing.T, skillCode string, slideCount int) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"prompt": "测试演示", "skillCode": skillCode, "slideCount": slideCount, "language": "zh", "audience": "management"})
	if err != nil {
		t.Fatal(err)
	}
	response := h.request(t, http.MethodPost, "/api/v1/ppt/sessions", string(body), "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("create session status = %d, body = %s", response.Code, response.Body.String())
	}
	var task struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	return task.TaskID
}

type pptAgentTestOperation struct {
	hash     string
	claim    pptapp.OperationClaim
	state    string
	response pptapp.Task
}

type pptAgentTestState struct {
	mu                      sync.Mutex
	tasks                   map[string]pptapp.Task
	operations              map[string]pptAgentTestOperation
	nextID                  int
	createCalls             int
	beginCalls              int
	sessionInputs           []pptapp.SessionRequest
	failErr                 error
	completeAfterPersistErr error
	secretRich              bool
	secretErrorCode         string
}

type pptAgentImportTestFile struct {
	object     storagecenter.FileObject
	content    []byte
	openErr    error
	readErr    error
	closeErr   error
	closeCalls atomic.Int32
}

type pptAgentImportFileStore struct {
	mu        sync.Mutex
	files     map[string]*pptAgentImportTestFile
	accesses  []storagecenter.AccessContext
	openCalls atomic.Int32
}

func newPPTAgentImportFileStore() *pptAgentImportFileStore {
	return &pptAgentImportFileStore{files: map[string]*pptAgentImportTestFile{}}
}

func newPPTAgentMarkdownTestFile(fileID, userID, name, mimeType, content string) *pptAgentImportTestFile {
	extension := "md"
	if strings.HasSuffix(strings.ToLower(name), ".markdown") {
		extension = "markdown"
	}
	return &pptAgentImportTestFile{
		object: storagecenter.FileObject{
			FileID: fileID, TenantID: "tenant_default", UserID: userID, OriginalName: name, Extension: extension,
			MIMEType: mimeType, FileSize: int64(len([]byte(content))), Visibility: "PRIVATE", Status: storagecenter.StatusActive,
		},
		content: []byte(content),
	}
}

func (s *pptAgentImportFileStore) OpenObject(_ context.Context, access storagecenter.AccessContext, fileID string) (storagecenter.FileObject, io.ReadCloser, error) {
	s.mu.Lock()
	s.accesses = append(s.accesses, access)
	file := s.files[fileID]
	s.mu.Unlock()
	s.openCalls.Add(1)
	if file == nil {
		return storagecenter.FileObject{}, nil, storagecenter.ErrFileNotFound
	}
	if file.openErr != nil {
		return storagecenter.FileObject{}, nil, file.openErr
	}
	return file.object, &pptAgentImportReadCloser{reader: bytes.NewReader(file.content), file: file}, nil
}

type pptAgentImportReadCloser struct {
	reader   *bytes.Reader
	file     *pptAgentImportTestFile
	readOnce bool
}

func (r *pptAgentImportReadCloser) Read(buffer []byte) (int, error) {
	if r.file.readErr != nil && !r.readOnce {
		r.readOnce = true
		return 0, r.file.readErr
	}
	return r.reader.Read(buffer)
}

func (r *pptAgentImportReadCloser) Close() error {
	r.file.closeCalls.Add(1)
	return r.file.closeErr
}

func validPPTAgentChatFunc(context.Context, generation.CreateRequest) (chatprovider.Response, error) {
	return validPPTAgentChatResponse(), nil
}

func newPPTAgentTestState() *pptAgentTestState {
	return &pptAgentTestState{tasks: map[string]pptapp.Task{}, operations: map[string]pptAgentTestOperation{}}
}

func (s *pptAgentTestState) CreateSession(_ context.Context, request pptapp.SessionRequest) (pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	s.createCalls++
	s.sessionInputs = append(s.sessionInputs, request)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	taskID := "ppt_test_" + string(rune('0'+s.nextID))
	task := pptapp.NormalizeTask(pptapp.Task{
		TaskID: taskID, SessionID: taskID, UserID: request.Owner.UserID, ClientRequestID: request.ClientRequestID,
		TenantID: request.Owner.TenantID, OrganizationID: request.OrganizationID, ContextType: request.ContextType,
		BillingScope: request.BillingScope, BillingAccountID: request.BillingAccountID,
		Type: "ppt", MediaType: "ppt", SkillCode: request.SkillCode, Stage: pptapp.StageDraft, Status: pptapp.StatusPending,
		Title: request.Prompt, Prompt: request.Prompt, SlideCount: request.SlideCount, Language: request.Language, Audience: request.Audience,
		Tone: request.DeckSpec.Tone, TextContent: request.DeckSpec.TextContent, Scenario: request.DeckSpec.Scenario,
		GenerationAspectRatio: request.DeckSpec.GenerationAspectRatio, Theme: request.DeckSpec.Theme,
		AutoThemeEnabled: request.DeckSpec.AutoThemeEnabled, EnableWebSearch: request.DeckSpec.EnableWebSearch,
		ImageSource: request.DeckSpec.ImageSource, TextModel: request.DeckSpec.TextModel, ImageModel: request.DeckSpec.ImageModel,
		ImageStyle: request.DeckSpec.ImageStyle, PeopleStyle: request.DeckSpec.PeopleStyle,
		ImageLighting: request.DeckSpec.ImageLighting, ImageComposition: request.DeckSpec.ImageComposition,
		TextInImage:   request.DeckSpec.TextInImage,
		SourceFileIDs: append([]string(nil), request.SourceFileIDs...), CreatedAt: now, UpdatedAt: now,
	})
	if s.secretRich {
		task = secretRichPPTAgentTask(task)
		if s.secretErrorCode != "" {
			task.ErrorCode = s.secretErrorCode
		}
	}
	s.tasks[taskID] = task
	return task, nil
}

func (s *pptAgentTestState) GetTask(_ context.Context, owner pptapp.OwnerScope, taskID string) (pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	return pptapp.NormalizeTask(task), nil
}

func (s *pptAgentTestState) History(_ context.Context, owner pptapp.OwnerScope) ([]pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]pptapp.Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if pptAgentTestOwnerMatches(task, owner) {
			items = append(items, pptapp.NormalizeTask(task))
		}
	}
	return items, nil
}

func (s *pptAgentTestState) BeginOperation(_ context.Context, owner pptapp.OwnerScope, taskID, scope, key, requestHash string) (pptapp.OperationClaim, pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(key) == "" {
		return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrIdempotencyKeyRequired
	}
	task, ok := s.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	s.beginCalls++
	operationKey := owner.TenantID + ":" + owner.UserID + ":" + taskID + ":" + scope + ":" + key
	if existing, exists := s.operations[operationKey]; exists {
		if existing.hash != requestHash {
			return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrIdempotencyConflict
		}
		if existing.state == "completed" {
			existing.claim.Replay = true
			existing.claim.CompletedReplay = true
			return existing.claim, existing.response, nil
		}
		if existing.state == "processing" {
			existing.claim.InFlight = true
			return existing.claim, task, pptapp.ErrOperationInProgress
		}
	}
	if strings.HasPrefix(scope, "revise-slide") {
		if task.Stage != pptapp.StageReady {
			return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
		}
	} else if task.Stage != pptapp.StageDraft && task.Stage != pptapp.StageOutlineReady {
		return pptapp.OperationClaim{}, pptapp.Task{}, pptapp.ErrInvalidStage
	}
	task.ErrorCode = ""
	claim := pptapp.OperationClaim{Scope: scope, Key: key, RequestHash: requestHash, OperationToken: "op_test"}
	s.operations[operationKey] = pptAgentTestOperation{hash: requestHash, claim: claim, state: "processing"}
	task.IdempotencyRecords = append(task.IdempotencyRecords, pptapp.IdempotencyRecord{
		Scope: scope, Key: key, RequestHash: requestHash, State: "processing", OperationToken: claim.OperationToken,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	s.tasks[taskID] = task
	return claim, task, nil
}

func (s *pptAgentTestState) CompleteSlideRevision(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, slideID string, revision pptapp.Slide) (pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	operationKey := owner.TenantID + ":" + owner.UserID + ":" + taskID + ":" + claim.Scope + ":" + claim.Key
	operation, exists := s.operations[operationKey]
	if !exists || operation.claim.OperationToken != claim.OperationToken {
		return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
	}
	if task.Stage != pptapp.StageReady {
		return pptapp.Task{}, pptapp.ErrInvalidStage
	}
	target := -1
	for index := range task.Slides {
		if task.Slides[index].ID == strings.TrimSpace(slideID) {
			target = index
			break
		}
	}
	if target < 0 {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	original := pptapp.NormalizeSlideIR(task.Slides[target])
	revision = pptapp.NormalizeSlideIR(revision)
	blocks := make([]pptapp.SlideBlock, 0, len(revision.Blocks)+2)
	for _, block := range revision.Blocks {
		if block.Type != "image" && block.Type != "note" {
			blocks = append(blocks, block)
		}
	}
	for _, block := range original.Blocks {
		if block.Type == "image" || block.Type == "note" {
			blocks = append(blocks, block)
		}
	}
	revised := original
	revised.Blocks = blocks
	revised = pptapp.NormalizeSlideIR(revised)
	revised.ID, revised.Page, revised.Layout = original.ID, original.Page, original.Layout
	task.Slides[target] = revised
	task.ErrorCode = ""
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task = pptapp.NormalizeTask(task)
	operation.state = "completed"
	operation.response = task
	for index := range task.IdempotencyRecords {
		record := &task.IdempotencyRecords[index]
		if record.Scope == claim.Scope && record.Key == claim.Key && record.OperationToken == claim.OperationToken {
			record.State = "completed"
			record.UpdatedAt = task.UpdatedAt
			snapshot := task
			snapshot.IdempotencyRecords = nil
			if raw, err := json.Marshal(snapshot); err == nil {
				record.ResponseJSON = string(raw)
			}
		}
	}
	s.tasks[taskID] = task
	operation.response = task
	s.operations[operationKey] = operation
	return task, nil
}

func (s *pptAgentTestState) CompleteOutlineOperation(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, messages []pptapp.AgentMessage, outline pptapp.Outline) (pptapp.Task, error) {
	return s.completeOutlineOperation(owner, taskID, claim, messages, outline, nil, false)
}

func (s *pptAgentTestState) CompleteImportOutlineOperation(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, messages []pptapp.AgentMessage, outline pptapp.Outline, sourceFileIDs []string) (pptapp.Task, error) {
	return s.completeOutlineOperation(owner, taskID, claim, messages, outline, sourceFileIDs, true)
}

func (s *pptAgentTestState) completeOutlineOperation(owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, messages []pptapp.AgentMessage, outline pptapp.Outline, sourceFileIDs []string, replaceSourceFiles bool) (pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	operationKey := owner.TenantID + ":" + owner.UserID + ":" + taskID + ":" + claim.Scope + ":" + claim.Key
	operation, exists := s.operations[operationKey]
	if !exists || operation.claim.OperationToken != claim.OperationToken {
		return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
	}
	task.AgentMessages = append(task.AgentMessages, messages...)
	if replaceSourceFiles {
		task.SourceFileIDs = nil
		seen := map[string]struct{}{}
		for _, sourceFileID := range sourceFileIDs {
			sourceFileID = strings.TrimSpace(sourceFileID)
			if sourceFileID == "" {
				continue
			}
			if _, exists := seen[sourceFileID]; exists {
				continue
			}
			seen[sourceFileID] = struct{}{}
			task.SourceFileIDs = append(task.SourceFileIDs, sourceFileID)
		}
	}
	outlineCopy := outline
	task.Outline = &outlineCopy
	task.SlideCount = len(outline.Slides)
	task.Stage = pptapp.StageOutlineReady
	task.Status = pptapp.StatusPending
	task.ErrorCode = ""
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task = pptapp.NormalizeTask(task)
	if s.secretRich {
		task = secretRichPPTAgentTask(task)
		if s.secretErrorCode != "" {
			task.ErrorCode = s.secretErrorCode
		}
	}
	operation.state = "completed"
	operation.response = task
	for index := range task.IdempotencyRecords {
		record := &task.IdempotencyRecords[index]
		if record.Scope == claim.Scope && record.Key == claim.Key && record.OperationToken == claim.OperationToken {
			record.State = "completed"
			snapshot := task
			snapshot.IdempotencyRecords = nil
			if raw, err := json.Marshal(snapshot); err == nil {
				record.ResponseJSON = string(raw)
			}
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	s.tasks[taskID] = task
	operation.response = task
	s.operations[operationKey] = operation
	if s.completeAfterPersistErr != nil {
		return task, s.completeAfterPersistErr
	}
	return task, nil
}

func (s *pptAgentTestState) FailOperation(_ context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, errorCode string) (pptapp.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || !pptAgentTestOwnerMatches(task, owner) {
		return pptapp.Task{}, pptapp.ErrTaskNotFound
	}
	operationKey := owner.TenantID + ":" + owner.UserID + ":" + taskID + ":" + claim.Scope + ":" + claim.Key
	operation, exists := s.operations[operationKey]
	if !exists || operation.claim.OperationToken != claim.OperationToken {
		return pptapp.Task{}, pptapp.ErrOperationTokenMismatch
	}
	if s.failErr != nil {
		return pptapp.Task{}, s.failErr
	}
	operation.state = "failed"
	s.operations[operationKey] = operation
	task.ErrorCode = errorCode
	for index := range task.IdempotencyRecords {
		record := &task.IdempotencyRecords[index]
		if record.Scope == claim.Scope && record.Key == claim.Key && record.OperationToken == claim.OperationToken {
			record.State = "failed"
			record.ErrorCode = errorCode
			record.ResponseJSON = ""
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	s.tasks[taskID] = task
	return task, nil
}

func (s *pptAgentTestState) lastSessionRequest() pptapp.SessionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessionInputs) == 0 {
		return pptapp.SessionRequest{}
	}
	return s.sessionInputs[len(s.sessionInputs)-1]
}

func (s *pptAgentTestState) owner(taskID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tasks[taskID].UserID
}

func (s *pptAgentTestState) ownerScope(taskID string) pptapp.OwnerScope {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.tasks[taskID]
	return pptapp.OwnerScope{TenantID: task.TenantID, UserID: task.UserID}
}

func pptAgentTestOwnerMatches(task pptapp.Task, owner pptapp.OwnerScope) bool {
	return task.TenantID == owner.TenantID && task.UserID == owner.UserID
}

func (s *pptAgentTestState) makeReady(taskID string, slides []pptapp.Slide) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.tasks[taskID]
	task.Stage = pptapp.StageReady
	task.Status = pptapp.StatusSuccess
	task.Progress = 100
	task.CurrentPage = len(slides)
	task.SlideCount = len(slides)
	task.Slides = append([]pptapp.Slide(nil), slides...)
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.tasks[taskID] = pptapp.NormalizeTask(task)
}

func validPPTAgentChatResponse() chatprovider.Response {
	return chatprovider.Response{Message: chatprovider.Message{Role: "assistant", Content: `{"title":"测试演示","pages":[{"title":"现状","summary":"说明现状","bullets":["要点一"]},{"title":"行动","summary":"说明行动","bullets":["要点二"]}]}`}}
}

func assertPPTAgentError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, status, response.Body.String())
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != code {
		t.Fatalf("code = %q, want %q, body = %s", payload.Code, code, response.Body.String())
	}
}

func assertNoPPTAgentInternalFields(t *testing.T, raw []byte) {
	t.Helper()
	text := string(raw)
	for _, forbidden := range []string{
		"userId", "tenantId", "organizationId", "contextType", "billingScope", "billingAccountId", "clientRequestId", "textModel", "imageModel", "billingTaskId", "generationLease", "idempotencyRecords",
		"runToken", "leaseUntil", "operationToken", "requestHash", "responseJson", "visualTaskId", "visualModelName", "systemPrompt", "outlineSchema",
	} {
		if strings.Contains(text, `"`+forbidden+`"`) {
			t.Fatalf("response leaked forbidden field %s: %s", forbidden, text)
		}
	}
}

func assertNoPPTAgentSecretValues(t *testing.T, raw []byte) {
	t.Helper()
	for _, secret := range []string{
		"tenant_secret", "organization_secret", "billing_secret", "private-model", "private-image-model",
		"run_secret", "hash_secret", "op_secret", "snapshot_secret", "provider_task_secret", "provider_model_secret", "internal-database-secret",
	} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("response leaked secret %q: %s", secret, raw)
		}
	}
}

func assertPPTTaskPublicErrorCode(t *testing.T, raw []byte, want string) {
	t.Helper()
	if want == "" {
		if strings.Contains(string(raw), `"errorCode"`) {
			t.Fatalf("unknown/internal errorCode must be omitted: %s", raw)
		}
		return
	}
	if !strings.Contains(string(raw), `"errorCode":"`+want+`"`) {
		t.Fatalf("approved errorCode %q missing: %s", want, raw)
	}
}

func secretRichPPTAgentTask(task pptapp.Task) pptapp.Task {
	task.TenantID = "tenant_secret"
	task.OrganizationID = "organization_secret"
	task.ContextType = contextEnterprise
	task.BillingScope = contextEnterprise
	task.BillingAccountID = "billing_secret"
	task.TextModel = "private-model"
	task.ImageModel = "private-image-model"
	task.BillingTaskID = "billing_secret"
	task.GenerationLease = &pptapp.GenerationLease{RunToken: "run_secret", LeaseUntil: "tomorrow"}
	task.ErrorCode = "PPT_INTERNAL_SECRET"
	task.ErrorMessage = "internal-database-secret"
	task.IdempotencyRecords = append(task.IdempotencyRecords, pptapp.IdempotencyRecord{
		Scope: "secret", Key: "secret", RequestHash: "hash_secret", State: "completed",
		ResponseJSON: "snapshot_secret", OperationToken: "op_secret",
	})
	task.Slides = append(task.Slides, pptapp.Slide{
		ID: "slide_secret", Page: 1, Title: "Visible slide", Content: "Visible content", Layout: "content",
		VisualTaskID: "provider_task_secret", VisualModelName: "provider_model_secret",
	})
	return task
}

func stringJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func mapKeys(value map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	return keys
}
