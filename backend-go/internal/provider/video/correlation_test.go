package video

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func TestOpenAICompatibleEmitsCreatePollAndTerminalCorrelation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"submit-job","status":"processing"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"poll-job","status":"failed","error_code":"generation_failed","message":"PRIVATE_PROVIDER_REASON"}`))
		default:
			t.Fatalf("method=%s", r.Method)
		}
	}))
	defer server.Close()
	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{Code: "channel-safe", BaseURL: server.URL, APIKey: "secret", Model: "grok-imagine-1.5-video"})
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/videos", nil)
	if err != nil {
		t.Fatal(err)
	}
	var events []generation.ProviderCorrelationEvent
	ctx := generation.WithProviderCorrelationListener(context.Background(), func(event generation.ProviderCorrelationEvent) { events = append(events, event) })
	_, err = provider.finishVideoCreate(ctx, req, "grok-imagine-1.5-video", generation.CreateRequest{Params: map[string]any{}})
	if err == nil {
		t.Fatal("expected terminal provider failure")
	}
	var submit, poll, terminal *generation.ProviderCorrelationEvent
	for index := range events {
		event := &events[index]
		switch event.Kind {
		case "create_response":
			submit = event
		case "poll_response":
			poll = event
		case "terminal":
			terminal = event
		}
	}
	if submit == nil || submit.JobID != "submit-job" || submit.JobRole != "submit" {
		t.Fatalf("submit=%#v events=%#v", submit, events)
	}
	if poll == nil || poll.JobID != "poll-job" || poll.JobRole != "poll" || poll.State != "failed" || poll.ErrorCode != "generation_failed" || poll.ErrorHash == "" || poll.ErrorHash == "PRIVATE_PROVIDER_REASON" {
		t.Fatalf("poll=%#v events=%#v", poll, events)
	}
	if terminal == nil || terminal.JobID != "poll-job" || terminal.JobRole != "terminal" || terminal.ErrorHash == "" {
		t.Fatalf("terminal=%#v events=%#v", terminal, events)
	}
}

func TestOpenAICompatibleGetEmitsTerminalOnlyForProviderTerminalStates(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		terminalState string
		wantErrorHash bool
	}{
		{name: "processing", body: `{"id":"poll-job","status":"processing"}`},
		{name: "success", body: `{"id":"poll-job","status":"success"}`, terminalState: "success"},
		{name: "failed", body: `{"id":"poll-job","status":"failed","error_code":"generation_failed","message":"PRIVATE_PROVIDER_REASON"}`, terminalState: "failed", wantErrorHash: true},
		{name: "cancelled", body: `{"id":"poll-job","status":"cancelled"}`, terminalState: "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{Code: "channel-safe", BaseURL: server.URL, APIKey: "secret", Model: "grok-imagine-1.5-video"})
			var events []generation.ProviderCorrelationEvent
			ctx := generation.WithProviderCorrelationListener(context.Background(), func(event generation.ProviderCorrelationEvent) { events = append(events, event) })
			if _, err := provider.Get(ctx, "submit-job"); err != nil {
				t.Fatal(err)
			}
			var terminals []generation.ProviderCorrelationEvent
			for _, event := range events {
				if event.Kind == "terminal" {
					terminals = append(terminals, event)
				}
			}
			if tt.terminalState == "" {
				if len(terminals) != 0 {
					t.Fatalf("processing emitted terminal: %#v", terminals)
				}
				return
			}
			if len(terminals) != 1 || terminals[0].State != tt.terminalState || terminals[0].JobRole != "terminal" || terminals[0].JobID != "poll-job" {
				t.Fatalf("terminals=%#v", terminals)
			}
			if tt.wantErrorHash && (terminals[0].ErrorCode != "generation_failed" || terminals[0].ErrorHash == "" || terminals[0].ErrorHash == "PRIVATE_PROVIDER_REASON") {
				t.Fatalf("unsafe failed terminal=%#v", terminals[0])
			}
		})
	}
}

func TestOpenAICompatibleEmitsCorrelationForCreateHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"upstream_unavailable","message":"PRIVATE_CREATE_FAILURE"}`))
	}))
	defer server.Close()
	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{Code: "channel-safe", BaseURL: server.URL, APIKey: "secret", Model: "grok-imagine-1.5-video"})
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/videos", nil)
	if err != nil {
		t.Fatal(err)
	}
	var events []generation.ProviderCorrelationEvent
	ctx := generation.WithProviderCorrelationListener(context.Background(), func(event generation.ProviderCorrelationEvent) { events = append(events, event) })
	_, err = provider.finishVideoCreate(ctx, req, "grok-imagine-1.5-video", generation.CreateRequest{Params: map[string]any{}})
	if err == nil || len(events) != 2 {
		t.Fatalf("err=%v events=%#v", err, events)
	}
	terminal := events[1]
	if terminal.Kind != "terminal" || terminal.State != "failed" || terminal.HTTPStatus != http.StatusBadGateway || terminal.ErrorCode != "upstream_unavailable" || terminal.ErrorHash == "" || terminal.ErrorHash == "PRIVATE_CREATE_FAILURE" {
		t.Fatalf("terminal=%#v", terminal)
	}
}

func TestOpenAICompatibleEmitsSafeCreateAndTerminalCorrelation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"create-job","status":"failed","error_code":"generation_failed","message":"internal failure"}`))
	}))
	defer server.Close()
	provider := NewOpenAICompatibleWithOptions(OpenAICompatibleOptions{Code: "channel-safe", BaseURL: server.URL, APIKey: "secret", Model: "grok-imagine-1.5-video"})
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/videos", nil)
	if err != nil {
		t.Fatal(err)
	}
	var events []generation.ProviderCorrelationEvent
	ctx := generation.WithProviderCorrelationListener(context.Background(), func(event generation.ProviderCorrelationEvent) { events = append(events, event) })
	_, err = provider.finishVideoCreate(ctx, req, "grok-imagine-1.5-video", generation.CreateRequest{Params: map[string]any{}})
	if err == nil {
		t.Fatal("expected terminal provider failure")
	}
	if len(events) < 3 {
		t.Fatalf("events=%#v", events)
	}
	if events[0].Kind != "create_request" || events[0].Host == "" || events[0].Path != "/v1/videos" {
		t.Fatalf("create event=%#v", events[0])
	}
	if events[1].JobID != "create-job" || events[1].JobRole != "submit" {
		t.Fatalf("submit event=%#v", events[1])
	}
	last := events[len(events)-1]
	if last.Kind != "terminal" || last.JobID != "create-job" || last.State != "failed" || last.ErrorCode != "generation_failed" || last.ErrorHash == "" {
		t.Fatalf("terminal event=%#v", last)
	}
}
