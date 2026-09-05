package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func renderOperationalForTest(snapshot asyncCanaryOperationalSnapshot) string {
	if snapshot.providerCount == nil {
		snapshot.providerCount = map[string]float64{}
	}
	if snapshot.providerAge == nil {
		snapshot.providerAge = map[string]float64{}
	}
	var rendered strings.Builder
	renderAsyncCanaryMetrics(&rendered, snapshot)
	return rendered.String()
}

func TestTEST_J_OutboxObservability(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{outboxPending: 3, outboxFailed: 1, outboxOldestSeconds: 42, outboxPublishRetries: 5})
	for _, sample := range []string{"xianzhi_async_canary_outbox_pending 3", "xianzhi_async_canary_outbox_failed 1", "xianzhi_async_canary_outbox_oldest_pending_age_seconds 42", "xianzhi_async_canary_outbox_publish_retries_total 5"} {
		if !strings.Contains(output, sample) {
			t.Errorf("missing updated metric %q", sample)
		}
	}
}

func TestTEST_K_ProviderExecutionUnknownStaleObservability(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{providerCount: map[string]float64{"unknown": 2, "submitting": 1}, providerAge: map[string]float64{"unknown": 60, "submitting": 901}})
	for _, sample := range []string{`xianzhi_async_canary_provider_execution_count{status="unknown"} 2`, `xianzhi_async_canary_provider_execution_oldest_age_seconds{status="submitting"} 901`} {
		if !strings.Contains(output, sample) {
			t.Errorf("missing provider safety metric %q", sample)
		}
	}
}

func TestTEST_L_PointsUnsettledObservability(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{pointsUnsettledCount: 4, pointsUnsettledAge: 1200})
	if !strings.Contains(output, "xianzhi_async_canary_points_reserved_unsettled 4") || !strings.Contains(output, "xianzhi_async_canary_points_oldest_unsettled_age_seconds 1200") {
		t.Fatalf("points metrics not rendered: %s", output)
	}
}

func TestTEST_M_ArtifactRecoveryFailureObservability(t *testing.T) {
	resetAsyncCanaryProcessMetricsForTest()
	generationCanaryMetrics.artifactRecoveryAttempts.Add(1)
	generationCanaryMetrics.artifactRecoveryFailures.Add(1)
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{artifactStaleClaims: 2})
	for _, sample := range []string{"xianzhi_async_canary_artifact_recovery_attempts_total 1", "xianzhi_async_canary_artifact_recovery_failures_total 1", "xianzhi_async_canary_artifact_stale_claims 2"} {
		if !strings.Contains(output, sample) {
			t.Errorf("missing artifact metric %q", sample)
		}
	}
}

func TestTEST_I_MetricsRegisteredAndExposed(t *testing.T) {
	resetAsyncCanaryProcessMetricsForTest()
	recordAsyncCanaryDecision(canaryReasonSelected)
	generationCanaryMetrics.submitted.Add(1)
	collector := newHTTPMetricsCollector().withAsyncCanaryOperations(&asyncCanaryOperationalCollector{cfg: config.Config{MetricsEnabled: true}})
	recorder := httptest.NewRecorder()
	collector.handler(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("metrics endpoint response=%d content-type=%s", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	body := recorder.Body.String()
	for _, sample := range []string{`xianzhi_async_canary_decisions_total{reason="async_canary_selected"} 1`, "xianzhi_async_canary_submitted_total 1", "xianzhi_async_canary_rabbitmq_dlq_depth"} {
		if !strings.Contains(body, sample) {
			t.Errorf("/metrics missing %q", sample)
		}
	}
}

func TestVideoCanaryObservabilityMetrics(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{
		rabbitVideoQueueDepth:     5,
		rabbitVideoRetryDepth:     2,
		rabbitVideoDLQDepth:       1,
		rabbitVideoConsumers:      1,
		videoGenerationStuckCount: 3,
		videoGenerationStuckAge:   600,
		videoPointsUnsettledCount: 2,
		videoPointsUnsettledAge:   900,
	})
	for _, sample := range []string{
		"xianzhi_async_canary_video_rabbitmq_queue_depth 5",
		"xianzhi_async_canary_video_rabbitmq_retry_queue_depth 2",
		"xianzhi_async_canary_video_rabbitmq_dlq_depth 1",
		"xianzhi_async_canary_video_rabbitmq_consumers 1",
		"xianzhi_async_canary_video_generation_stuck 3",
		"xianzhi_async_canary_video_generation_oldest_stuck_age_seconds 600",
		"xianzhi_async_canary_video_points_reserved_unsettled 2",
		"xianzhi_async_canary_video_points_oldest_unsettled_age_seconds 900",
	} {
		if !strings.Contains(output, sample) {
			t.Errorf("missing video canary metric %q", sample)
		}
	}
}

func TestOperationalSnapshotRendersPPTCanaryMetrics(t *testing.T) {
	output := renderOperationalForTest(asyncCanaryOperationalSnapshot{
		rabbitPPTQueueDepth:     4,
		rabbitPPTRetryDepth:     1,
		rabbitPPTDLQDepth:       0,
		rabbitPPTConsumers:      1,
		pptGenerationStuckCount: 2,
		pptGenerationStuckAge:   450,
		pptPointsUnsettledCount: 1,
		pptPointsUnsettledAge:   300,
	})
	for _, sample := range []string{
		"xianzhi_async_canary_ppt_rabbitmq_queue_depth 4",
		"xianzhi_async_canary_ppt_rabbitmq_retry_queue_depth 1",
		"xianzhi_async_canary_ppt_rabbitmq_dlq_depth 0",
		"xianzhi_async_canary_ppt_rabbitmq_consumers 1",
		"xianzhi_async_canary_ppt_generation_stuck 2",
		"xianzhi_async_canary_ppt_generation_oldest_stuck_age_seconds 450",
		"xianzhi_async_canary_ppt_points_reserved_unsettled 1",
		"xianzhi_async_canary_ppt_points_oldest_unsettled_age_seconds 300",
	} {
		if !strings.Contains(output, sample) {
			t.Errorf("missing ppt canary metric %q", sample)
		}
	}
}
