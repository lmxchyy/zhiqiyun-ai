package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rabbitmq/amqp091-go"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
)

const asyncCanaryStaleAfter = 15 * time.Minute

const (
	canaryReasonSelected         = "async_canary_selected"
	canaryReasonRejectedUser     = "async_canary_rejected_user"
	canaryReasonRejectedType     = "async_canary_rejected_type"
	canaryReasonRejectedProvider = "async_canary_rejected_provider"
	canaryReasonRejectedModel    = "async_canary_rejected_model"
	canaryReasonDisabled         = "async_canary_disabled"
)

// asyncCanaryProcessMetrics contains process-local counters. Durable state is
// sampled from PostgreSQL and RabbitMQ by the existing /metrics collector.
// Counters deliberately carry no prompt, user, token, or credential labels.
type asyncCanaryProcessMetrics struct {
	mu                         sync.Mutex
	decisions                  map[string]uint64
	submitted                  atomic.Uint64
	completed                  atomic.Uint64
	failed                     atomic.Uint64
	recovered                  atomic.Uint64
	providerSubmissionAttempts atomic.Uint64
	preventedDuplicates        atomic.Uint64
	unknownTransitions         atomic.Uint64
	fallbackAttempts           atomic.Uint64
	providerRecoveries         atomic.Uint64
	pointsCaptureFailures      atomic.Uint64
	pointsReleaseFailures      atomic.Uint64
	pointsSettlementConflicts  atomic.Uint64
	artifactRecoveryAttempts   atomic.Uint64
	artifactRecoveryFailures   atomic.Uint64
}

var generationCanaryMetrics = &asyncCanaryProcessMetrics{decisions: map[string]uint64{}}

func recordAsyncCanaryDecision(reason string) {
	generationCanaryMetrics.mu.Lock()
	generationCanaryMetrics.decisions[reason]++
	generationCanaryMetrics.mu.Unlock()
}

func asyncCanaryDecisionSnapshot() map[string]uint64 {
	generationCanaryMetrics.mu.Lock()
	defer generationCanaryMetrics.mu.Unlock()
	result := make(map[string]uint64, len(generationCanaryMetrics.decisions))
	for key, value := range generationCanaryMetrics.decisions {
		result[key] = value
	}
	return result
}

func resetAsyncCanaryProcessMetricsForTest() {
	generationCanaryMetrics = &asyncCanaryProcessMetrics{decisions: map[string]uint64{}}
}

type asyncCanaryOperationalCollector struct {
	db          *sql.DB
	cfg         config.Config
	readyStatus func() string
}

type asyncCanaryOperationalSnapshot struct {
	dbScrapeOK, rabbitScrapeOK                                             float64
	outboxPending, outboxFailed, outboxOldestSeconds, outboxPublishRetries float64
	rabbitQueueDepth, rabbitRetryDepth, rabbitDLQDepth, rabbitConsumers    float64
	providerCount                                                          map[string]float64
	providerAge                                                            map[string]float64
	generationStuckCount, generationStuckAge                               float64
	pointsUnsettledCount, pointsUnsettledAge                               float64
	artifactStaleClaims                                                    float64
	consumerReady                                                          float64
}

func (c *asyncCanaryOperationalCollector) snapshot() asyncCanaryOperationalSnapshot {
	s := asyncCanaryOperationalSnapshot{
		providerCount: map[string]float64{}, providerAge: map[string]float64{},
		rabbitQueueDepth: -1, rabbitRetryDepth: -1, rabbitDLQDepth: -1,
	}
	for _, status := range []string{"submitting", "submitted", "processing", "unknown", "failed"} {
		s.providerCount[status], s.providerAge[status] = 0, 0
	}
	if c == nil {
		return s
	}
	if c.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.collectDatabase(ctx, &s); err == nil {
			s.dbScrapeOK = 1
		} else {
			_ = err
		}
	}
	if strings.TrimSpace(c.cfg.RabbitMQURL) != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if c.collectRabbit(ctx, &s) == nil {
			s.rabbitScrapeOK = 1
		}
	}
	if c.readyStatus != nil && strings.EqualFold(c.readyStatus(), "READY") {
		s.consumerReady = 1
	}
	return s
}

func (c *asyncCanaryOperationalCollector) collectDatabase(ctx context.Context, s *asyncCanaryOperationalSnapshot) error {
	queries := []struct {
		query string
		dest  []any
	}{
		{`SELECT count(*) FILTER (WHERE status='pending'), count(*) FILTER (WHERE status='failed'), COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)) FILTER (WHERE status='pending'),0), COALESCE(sum(attempt_count),0) FROM outbox_events`, []any{&s.outboxPending, &s.outboxFailed, &s.outboxOldestSeconds, &s.outboxPublishRetries}},
		{`SELECT count(*), COALESCE(EXTRACT(EPOCH FROM now()-min(NULLIF(updated_at,'')::timestamptz)),0) FROM xz_generation_tasks WHERE status IN ('PENDING','PROCESSING','RUNNING','QUEUED') AND COALESCE((params->>'generation_async_canary')::boolean,false) AND COALESCE(NULLIF(updated_at,'')::timestamptz,now()) < now()-$1::interval`, []any{&s.generationStuckCount, &s.generationStuckAge}},
		{`SELECT count(*), COALESCE(EXTRACT(EPOCH FROM now()-min(NULLIF(created_at,'')::timestamptz)),0) FROM xz_generation_tasks WHERE billing_status='RESERVED' AND status IN ('PENDING','PROCESSING','RUNNING','QUEUED') AND COALESCE((params->>'generation_async_canary')::boolean,false)`, []any{&s.pointsUnsettledCount, &s.pointsUnsettledAge}},
		{`SELECT count(*) FROM xz_file_objects WHERE business_type='generation_result' AND status='PENDING_UPLOAD' AND created_at < now()-$1::interval`, []any{&s.artifactStaleClaims}},
	}
	for index, item := range queries {
		args := []any{}
		if index > 0 {
			args = append(args, fmt.Sprintf("%f seconds", asyncCanaryStaleAfter.Seconds()))
		}
		if err := c.db.QueryRowContext(ctx, item.query, args...).Scan(item.dest...); err != nil {
			return err
		}
	}
	for _, status := range []string{"submitting", "submitted", "processing", "unknown", "failed"} {
		var count, age float64
		if err := c.db.QueryRowContext(ctx, `SELECT count(*), COALESCE(EXTRACT(EPOCH FROM now()-min(updated_at)),0) FROM provider_executions WHERE status=$1`, status).Scan(&count, &age); err != nil {
			return err
		}
		s.providerCount[status], s.providerAge[status] = count, age
	}
	return nil
}

func (c *asyncCanaryOperationalCollector) collectRabbit(ctx context.Context, s *asyncCanaryOperationalSnapshot) error {
	cfg := amqp091.Config{Heartbeat: 5 * time.Second, Dial: func(network, addr string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext(ctx, network, addr)
	}}
	conn, err := amqp091.DialConfig(c.cfg.RabbitMQURL, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	business, err := ch.QueueInspect(messaging.GenerationCanaryQueue)
	if err != nil {
		return err
	}
	retry, err := ch.QueueInspect(messaging.GenerationCanaryRetryQueue)
	if err != nil {
		return err
	}
	dlq, err := ch.QueueInspect(messaging.GenerationCanaryDLQ)
	if err != nil {
		return err
	}
	s.rabbitQueueDepth = float64(business.Messages)
	s.rabbitConsumers = float64(business.Consumers)
	s.rabbitRetryDepth = float64(retry.Messages)
	s.rabbitDLQDepth = float64(dlq.Messages)
	return nil
}

func renderAsyncCanaryMetrics(rendered *strings.Builder, snapshot asyncCanaryOperationalSnapshot) {
	writeGauge := func(name, help string, value float64) {
		writeMetricFamily(rendered, name, help, "gauge", func() { fmt.Fprintf(rendered, "%s %s\n", name, metricsFormatFloat(value)) })
	}
	writeCounter := func(name, help string, value uint64) {
		writeMetricFamily(rendered, name, help, "counter", func() { fmt.Fprintf(rendered, "%s %d\n", name, value) })
	}
	writeGauge("xianzhi_async_canary_database_scrape_success", "Whether the latest async canary PostgreSQL metrics sample succeeded.", snapshot.dbScrapeOK)
	writeGauge("xianzhi_async_canary_rabbitmq_scrape_success", "Whether the latest async canary RabbitMQ passive inspection succeeded.", snapshot.rabbitScrapeOK)
	writeGauge("xianzhi_async_canary_outbox_pending", "Pending async messaging outbox events.", snapshot.outboxPending)
	writeGauge("xianzhi_async_canary_outbox_failed", "Failed async messaging outbox events.", snapshot.outboxFailed)
	writeGauge("xianzhi_async_canary_outbox_oldest_pending_age_seconds", "Age of the oldest pending outbox event.", snapshot.outboxOldestSeconds)
	writeGauge("xianzhi_async_canary_outbox_publish_retries_total", "Cumulative outbox publish retries recorded durably.", snapshot.outboxPublishRetries)
	writeGauge("xianzhi_async_canary_rabbitmq_queue_depth", "Generation canary business queue depth.", snapshot.rabbitQueueDepth)
	writeGauge("xianzhi_async_canary_rabbitmq_retry_queue_depth", "Generation canary retry queue depth.", snapshot.rabbitRetryDepth)
	writeGauge("xianzhi_async_canary_rabbitmq_dlq_depth", "Generation canary dead-letter queue depth.", snapshot.rabbitDLQDepth)
	writeGauge("xianzhi_async_canary_rabbitmq_consumers", "RabbitMQ consumers on the generation canary queue.", snapshot.rabbitConsumers)
	writeGauge("xianzhi_async_canary_consumer_ready", "Whether the embedded async runtime reports READY.", snapshot.consumerReady)
	writeMetricFamily(rendered, "xianzhi_async_canary_provider_execution_count", "Provider executions by safety state.", "gauge", func() {
		for _, status := range []string{"submitting", "submitted", "processing", "unknown", "failed"} {
			fmt.Fprintf(rendered, "xianzhi_async_canary_provider_execution_count{status=%q} %s\n", status, metricsFormatFloat(snapshot.providerCount[status]))
		}
	})
	writeMetricFamily(rendered, "xianzhi_async_canary_provider_execution_oldest_age_seconds", "Oldest provider execution age by safety state.", "gauge", func() {
		for _, status := range []string{"submitting", "submitted", "processing", "unknown", "failed"} {
			fmt.Fprintf(rendered, "xianzhi_async_canary_provider_execution_oldest_age_seconds{status=%q} %s\n", status, metricsFormatFloat(snapshot.providerAge[status]))
		}
	})
	writeGauge("xianzhi_async_canary_generation_stuck", "Canary generation tasks processing beyond the Stage 0 stale threshold.", snapshot.generationStuckCount)
	writeGauge("xianzhi_async_canary_generation_oldest_stuck_age_seconds", "Age of the oldest stuck canary generation task.", snapshot.generationStuckAge)
	writeGauge("xianzhi_async_canary_points_reserved_unsettled", "Canary point reservations not yet captured or released.", snapshot.pointsUnsettledCount)
	writeGauge("xianzhi_async_canary_points_oldest_unsettled_age_seconds", "Age of the oldest unsettled canary point reservation.", snapshot.pointsUnsettledAge)
	writeGauge("xianzhi_async_canary_artifact_stale_claims", "Stale generated-artifact upload claims.", snapshot.artifactStaleClaims)
	writeMetricFamily(rendered, "xianzhi_async_canary_decisions_total", "Server-side async canary decisions by non-sensitive reason.", "counter", func() {
		decisions := asyncCanaryDecisionSnapshot()
		for _, reason := range []string{canaryReasonSelected, canaryReasonRejectedUser, canaryReasonRejectedType, canaryReasonRejectedProvider, canaryReasonRejectedModel, canaryReasonDisabled} {
			fmt.Fprintf(rendered, "xianzhi_async_canary_decisions_total{reason=%q} %d\n", reason, decisions[reason])
		}
	})
	writeCounter("xianzhi_async_canary_submitted_total", "Async canary tasks transactionally submitted.", generationCanaryMetrics.submitted.Load())
	writeCounter("xianzhi_async_canary_completed_total", "Async canary tasks completed.", generationCanaryMetrics.completed.Load())
	writeCounter("xianzhi_async_canary_failed_total", "Async canary tasks definitively failed.", generationCanaryMetrics.failed.Load())
	writeCounter("xianzhi_async_canary_recovered_total", "Async canary tasks completed through recovery.", generationCanaryMetrics.recovered.Load())
	writeCounter("xianzhi_async_canary_provider_submission_attempts_total", "Provider submission attempts from guarded canary execution.", generationCanaryMetrics.providerSubmissionAttempts.Load())
	writeCounter("xianzhi_async_canary_provider_duplicate_submissions_prevented_total", "Provider submissions prevented by durable execution state.", generationCanaryMetrics.preventedDuplicates.Load())
	writeCounter("xianzhi_async_canary_provider_unknown_transitions_total", "Provider executions transitioned to UNKNOWN by guarded execution.", generationCanaryMetrics.unknownTransitions.Load())
	writeCounter("xianzhi_async_canary_provider_fallback_attempts_total", "Fallback provider attempts after proven pre-submit failure.", generationCanaryMetrics.fallbackAttempts.Load())
	writeCounter("xianzhi_async_canary_provider_recoveries_total", "Provider execution recovery attempts.", generationCanaryMetrics.providerRecoveries.Load())
	writeCounter("xianzhi_async_canary_points_capture_failures_total", "Point capture failures during canary completion.", generationCanaryMetrics.pointsCaptureFailures.Load())
	writeCounter("xianzhi_async_canary_points_release_failures_total", "Point release failures during canary terminal settlement.", generationCanaryMetrics.pointsReleaseFailures.Load())
	writeCounter("xianzhi_async_canary_points_settlement_conflicts_total", "Detected point capture/release settlement conflicts.", generationCanaryMetrics.pointsSettlementConflicts.Load())
	writeCounter("xianzhi_async_canary_artifact_recovery_attempts_total", "Generated artifact recovery attempts.", generationCanaryMetrics.artifactRecoveryAttempts.Load())
	writeCounter("xianzhi_async_canary_artifact_recovery_failures_total", "Generated artifact recovery failures.", generationCanaryMetrics.artifactRecoveryFailures.Load())
}
