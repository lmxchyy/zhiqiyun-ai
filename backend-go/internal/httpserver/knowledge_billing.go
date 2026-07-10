package httpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

const billingMetricKnowledgeRAG = "knowledge.rag.tokens"

func (s *jsonStore) RecordRAGUsage(_ context.Context, usage knowledgeapp.RAGBillingUsage) error {
	if usage.PointCost <= 0 {
		return nil
	}
	return s.updateAdmin(func(data *adminPlatformData) error {
		for _, item := range data.BillingEvents {
			if item.TaskID == usage.RunID && strings.EqualFold(item.MetricCode, billingMetricKnowledgeRAG) {
				return nil
			}
		}
		available := pointsAvailableForAdminUser(*data, usage.UserID)
		pointCost := int(usage.PointCost)
		if available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", available, pointCost)
		}
		after := available - pointCost
		data.PointsAvailable = &after
		setPointAccount(data, usage.UserID, after)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		data.BillingEvents = append(data.BillingEvents, ragBillingEvent(usage, available, after, now, uniqueAdminID("evt", billingEventIDs(data.BillingEvents))))
		return nil
	})
}

func (s *postgresStore) RecordRAGUsage(ctx context.Context, usage knowledgeapp.RAGBillingUsage) error {
	if usage.PointCost <= 0 {
		return nil
	}
	if err := s.ensureReady(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, exists, err := billingEventForTaskMetricTx(ctx, tx, usage.RunID, billingMetricKnowledgeRAG); err != nil || exists {
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	account, err := pointAccountForUpdate(ctx, tx, usage.UserID)
	if err != nil {
		return err
	}
	pointCost := int(usage.PointCost)
	if account.Available < pointCost {
		return fmt.Errorf("insufficient remaining points: available %d, required %d", account.Available, pointCost)
	}
	after := account.Available - pointCost
	if _, err := tx.ExecContext(ctx, `
		update xz_point_accounts
		set available=available-$1, raw=jsonb_set(raw, '{available}', to_jsonb((available-$1)::int), true)
		where id=$2
	`, pointCost, account.ID); err != nil {
		return err
	}
	eventID, err := nextTableID(ctx, tx, "xz_billing_events", "evt")
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := ragBillingEvent(usage, account.Available, after, now, eventID)
	if err := insertBillingEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := insertAuditLog(ctx, tx, usage.UserID, "MEMBER", "knowledge.rag.complete", "rag_run", usage.RunID, "", "", 200, map[string]any{
		"inputTokens": usage.InputTokens, "outputTokens": usage.OutputTokens, "pointCost": usage.PointCost,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func ragBillingEvent(usage knowledgeapp.RAGBillingUsage, before int, after int, now string, id string) adminBillingEvent {
	return adminBillingEvent{
		ID: id, UserID: usage.UserID, AgentID: usage.AgentID, TenantID: usage.TenantID,
		ModuleCode: "knowledge_agent", TaskID: usage.RunID, MetricCode: billingMetricKnowledgeRAG,
		Quantity: usage.InputTokens + usage.OutputTokens, UnitAmountCents: pointUnitAmountCents,
		AmountCents: int(usage.PointCost) * pointUnitAmountCents, PointCost: int(usage.PointCost),
		BalanceBefore: before, BalanceAfter: after, Model: usage.Model, Status: "SUCCEEDED", OccurredAt: now,
		Metadata: map[string]any{"inputTokens": usage.InputTokens, "outputTokens": usage.OutputTokens, "billingUnit": "1000_tokens"},
	}
}

var _ knowledgeapp.RAGBillingRecorder = (*jsonStore)(nil)
var _ knowledgeapp.RAGBillingRecorder = (*postgresStore)(nil)
