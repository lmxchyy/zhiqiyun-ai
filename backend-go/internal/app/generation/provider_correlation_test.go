package generation

import (
	"context"
	"testing"
)

func TestProviderCorrelationListenerPreservesOnlyStructuredFields(t *testing.T) {
	var got ProviderCorrelationEvent
	ctx := WithProviderCorrelationListener(context.Background(), func(event ProviderCorrelationEvent) { got = event })
	NotifyProviderCorrelation(ctx, ProviderCorrelationEvent{Kind: "poll_response", ProviderCode: " channel ", Host: " provider.example ", Path: " /v1/videos/job ", JobID: " job ", JobRole: " poll ", State: " processing ", HTTPStatus: 200, ErrorCode: " generation_failed ", ErrorHash: " sha256:abc "})
	if got.Kind != "poll_response" || got.ProviderCode != "channel" || got.Host != "provider.example" || got.Path != "/v1/videos/job" || got.JobID != "job" || got.JobRole != "poll" || got.State != "processing" || got.HTTPStatus != 200 || got.ErrorCode != "generation_failed" || got.ErrorHash != "sha256:abc" {
		t.Fatalf("unexpected correlation event: %#v", got)
	}
}
