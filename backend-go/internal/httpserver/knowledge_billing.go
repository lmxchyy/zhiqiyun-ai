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
		walletTask := generationTask{ID: usage.RunID, UserID: usage.UserID, TenantID: usage.TenantID, ModuleCode: "knowledge_agent", Model: usage.Model}
		if _, err := applyAdminJSONWalletEntryV1(data, walletTask, "RESERVE", pointCost, "RAG usage reserve"); err != nil {
			return err
		}
		if _, err := applyAdminJSONWalletEntryV1(data, walletTask, "CAPTURE", pointCost, "RAG usage capture"); err != nil {
			return err
		}
		after := available - pointCost
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
	pointCost := int(usage.PointCost)
	authorization, err := s.authorizeModelCallContext(ctx, tx, usage.UserID, "knowledge_agent")
	if err != nil {
		return err
	}
	if authorization.ContextType == contextEnterprise && usage.TenantID != "" && usage.TenantID != authorization.TenantID {
		return errForbidden
	}
	var account adminPointAccount
	before, after := 0, 0
	if authorization.ContextType == contextEnterprise {
		reservation, err := s.reserveEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "RAG_RUN", usage.RunID)
		if err != nil {
			return err
		}
		before, after = int(reservation.BalanceBefore), int(reservation.BalanceAfter)
		usage.TenantID = authorization.TenantID
	} else {
		account, err = pointAccountForUpdate(ctx, tx, usage.UserID)
		if err != nil {
			return err
		}
		if account.Available < pointCost {
			return fmt.Errorf("insufficient remaining points: available %d, required %d", account.Available, pointCost)
		}
		before = account.Available
		walletTask := generationTask{ID: usage.RunID, UserID: usage.UserID, TenantID: usage.TenantID, ModuleCode: "knowledge_agent", Model: usage.Model}
		reserved, _, err := applyPersonalWalletEntryV1(ctx, tx, walletTask, account, "RESERVE", pointCost, "RAG usage reserve")
		if err != nil {
			return err
		}
		captured, _, err := applyPersonalWalletEntryV1(ctx, tx, walletTask, reserved, "CAPTURE", pointCost, "RAG usage capture")
		if err != nil {
			return err
		}
		after = captured.Available
	}
	eventID, err := nextTableID(ctx, tx, "xz_billing_events", "evt")
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := ragBillingEvent(usage, before, after, now, eventID)
	if err := insertBillingEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := insertAuditLog(ctx, tx, usage.UserID, "MEMBER", "knowledge.rag.complete", "rag_run", usage.RunID, "", "", 200, map[string]any{
		"inputTokens": usage.InputTokens, "outputTokens": usage.OutputTokens, "pointCost": usage.PointCost,
	}); err != nil {
		return err
	}
	usageTask := generationTask{ID: usage.RunID, UserID: usage.UserID, TenantID: authorization.TenantID, OrganizationID: authorization.OrganizationID, BillingAccountType: authorization.BillingScope, BillingAccountID: authorization.BillingAccountID, ModuleCode: "knowledge_agent", Model: usage.Model, PointCost: pointCost}
	if err := s.recordModelUsageTx(ctx, tx, authorization, usageTask, map[string]any{"inputTokens": usage.InputTokens, "outputTokens": usage.OutputTokens}); err != nil {
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
