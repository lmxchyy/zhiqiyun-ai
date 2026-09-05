package httpserver

import (
	"strings"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

func stage0PPTCanaryConfig() config.Config {
	return config.Config{
		AsyncMessagingEnabled:           true,
		PPTAsyncCanaryEnabled:           false, // default fail-closed
		ProviderExecutionSafetyEnabled:  true,
		PPTAsyncCanaryUsers:             "user-ppt-canary",
		PPTAsyncCanaryProviderAllowlist: "configured",
		PPTAsyncCanaryModelAllowlist:    "kimi-k2.6",
	}
}

func TestPPTCanary_DefaultDisabledFailsClosed(t *testing.T) {
	cfg := stage0PPTCanaryConfig()
	if cfg.PPTAsyncCanaryEnabled {
		t.Fatalf("PPTAsyncCanaryEnabled must be false by default")
	}

	// In PR #1 (foundation), pptAsyncCanaryDecision must always fail-closed
	// ensuring zero live traffic reaches RabbitMQ
	a := api{cfg: cfg}
	req := pptapp.GenerateRequest{
		UserID: "user-ppt-canary",
		Prompt: "test prompt",
	}
	eligible := a.pptAsyncCanaryEligible(req)
	if eligible {
		t.Fatalf("expected pptAsyncCanaryEligible to be false in foundation PR, got true")
	}
}

func TestPPTCanary_AllowlistsFailClosedWhenDisabled(t *testing.T) {
	// Empty users
	cfg := stage0PPTCanaryConfig()
	cfg.PPTAsyncCanaryUsers = ""
	a := api{cfg: cfg}
	req := pptapp.GenerateRequest{UserID: "user-ppt-canary", Prompt: "test"}
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("empty users allowlist must fail-closed")
	}

	// Empty provider
	cfg = stage0PPTCanaryConfig()
	cfg.PPTAsyncCanaryProviderAllowlist = ""
	a = api{cfg: cfg}
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("empty provider allowlist must fail-closed")
	}

	// Empty model
	cfg = stage0PPTCanaryConfig()
	cfg.PPTAsyncCanaryModelAllowlist = ""
	a = api{cfg: cfg}
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("empty model allowlist must fail-closed")
	}
}

func TestPPTCanary_TopologyConstantsAndRouting(t *testing.T) {
	// A. PPT queues correctly declared
	if messaging.GenerationPPTCanaryQueue != "x.ai.generation.ppt.canary" {
		t.Errorf("unexpected queue name: %s", messaging.GenerationPPTCanaryQueue)
	}
	if messaging.GenerationPPTCanaryRetryQueue != "x.ai.generation.ppt.canary.retry" {
		t.Errorf("unexpected retry queue name: %s", messaging.GenerationPPTCanaryRetryQueue)
	}
	if messaging.GenerationPPTCanaryDLQ != "x.ai.generation.ppt.canary.dlq" {
		t.Errorf("unexpected dlq name: %s", messaging.GenerationPPTCanaryDLQ)
	}

	// B. retry -> main routing correct
	if err := messaging.ValidateRoutingKey(messaging.GenerationPPTCanaryRoutingKey); err != nil {
		t.Errorf("GenerationPPTCanaryRoutingKey %q must be valid: %v", messaging.GenerationPPTCanaryRoutingKey, err)
	}
	if err := messaging.ValidateRoutingKey(messaging.GenerationPPTCanaryRetryKey); err != nil {
		t.Errorf("GenerationPPTCanaryRetryKey %q must be valid: %v", messaging.GenerationPPTCanaryRetryKey, err)
	}

	// C. DLQ routing correct
	if messaging.GenerationPPTCanaryDeadKey != "x.ai.generation.ppt.canary.dead" {
		t.Errorf("unexpected dead key: %s", messaging.GenerationPPTCanaryDeadKey)
	}
}

func TestPPTCanary_MetricsRenderingIncludesLowCardinalityOnly(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{
		rabbitPPTQueueDepth:     10,
		rabbitPPTRetryDepth:     3,
		rabbitPPTDLQDepth:       0,
		rabbitPPTConsumers:      2,
		pptGenerationStuckCount: 1,
		pptGenerationStuckAge:   300,
		pptPointsUnsettledCount: 2,
		pptPointsUnsettledAge:   200,
	})

	expectedMetrics := []string{
		"xianzhi_async_canary_ppt_rabbitmq_queue_depth 10",
		"xianzhi_async_canary_ppt_rabbitmq_retry_queue_depth 3",
		"xianzhi_async_canary_ppt_rabbitmq_dlq_depth 0",
		"xianzhi_async_canary_ppt_rabbitmq_consumers 2",
		"xianzhi_async_canary_ppt_generation_stuck 1",
		"xianzhi_async_canary_ppt_generation_oldest_stuck_age_seconds 300",
		"xianzhi_async_canary_ppt_points_reserved_unsettled 2",
		"xianzhi_async_canary_ppt_points_oldest_unsettled_age_seconds 200",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(output, m) {
			t.Errorf("metrics output missing expected sample: %s", m)
		}
	}

	// Verify NO sensitive or high-cardinality labels exist
	for _, prohibited := range []string{"user_id", "task_id", "prompt", "provider_request_id", "tenant_id"} {
		if strings.Contains(output, prohibited+"=") {
			t.Errorf("metrics output must NOT contain prohibited label: %s", prohibited)
		}
	}
}

func TestPPTCanary_ImageAndVideoMetricsDoNotRegress(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{
		rabbitQueueDepth:          1,
		rabbitRetryDepth:          0,
		rabbitDLQDepth:            0,
		rabbitConsumers:           1,
		rabbitVideoQueueDepth:     2,
		rabbitVideoRetryDepth:     0,
		rabbitVideoDLQDepth:       0,
		rabbitVideoConsumers:      1,
		rabbitPPTQueueDepth:       3,
		rabbitPPTRetryDepth:       0,
		rabbitPPTDLQDepth:         0,
		rabbitPPTConsumers:        1,
		generationStuckCount:      0,
		videoGenerationStuckCount: 0,
		pptGenerationStuckCount:   0,
	})

	// Check image metrics
	if !strings.Contains(output, "xianzhi_async_canary_rabbitmq_queue_depth 1") {
		t.Errorf("missing image queue depth")
	}
	// Check video metrics
	if !strings.Contains(output, "xianzhi_async_canary_video_rabbitmq_queue_depth 2") {
		t.Errorf("missing video queue depth")
	}
	// Check ppt metrics
	if !strings.Contains(output, "xianzhi_async_canary_ppt_rabbitmq_queue_depth 3") {
		t.Errorf("missing ppt queue depth")
	}
}

func TestPPTCanary_UnmatchedConfigKeepsExistingPPTPath(t *testing.T) {
	// Verify that with default config (PPT_ASYNC_CANARY_ENABLED=false),
	// decision helper returns false so existing synchronous path is preserved
	cfg := stage0PPTCanaryConfig()
	a := api{cfg: cfg}

	req := pptapp.GenerateRequest{
		UserID:     "test_user",
		Prompt:     "Quarterly business review",
		SlideCount: 3,
		TextModel:  "kimi-k2.6",
	}

	// Decision helper must return false in PR #1
	if a.pptAsyncCanaryEligible(req) {
		t.Fatalf("expected pptAsyncCanaryEligible to be false, got true")
	}
}
