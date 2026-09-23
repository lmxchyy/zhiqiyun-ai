package httpserver

import (
	"testing"
	"time"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func TestProviderCorrelationDiagnosticsExposeOnlyRedactedEvidence(t *testing.T) {
	items := providerCorrelationDiagnostics([]pe.CorrelationEvent{{
		Kind: "poll_response", ProviderCode: "resolved-channel", Host: "provider.example",
		Path: "/v1/videos/poll-job", JobID: "poll-job", JobRole: "poll", State: "failed",
		HTTPStatus: 200, ErrorCode: "generation_failed",
		ErrorHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CreatedAt: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC),
	}})
	if len(items) != 1 || items[0]["jobId"] != "poll-job" || items[0]["host"] != "provider.example" || items[0]["errorCode"] != "generation_failed" {
		t.Fatalf("items=%#v", items)
	}
	for _, unsafeKey := range []string{"prompt", "providerPrompt", "apiKey", "errorMessage", "payload", "url"} {
		if _, exists := items[0][unsafeKey]; exists {
			t.Fatalf("diagnostic exposed %s: %#v", unsafeKey, items)
		}
	}
}
