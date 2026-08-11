package httpserver

import (
	"context"
	"database/sql"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

const billingMetricKnowledgeRAG = "knowledge.rag.tokens"

func (s *jsonStore) RecordRAGUsage(_ context.Context, usage knowledgeapp.RAGBillingUsage) error {
	if usage.PointCost <= 0 {
		return nil
	}
	usage.UserID = strings.TrimSpace(usage.UserID)
	usage.RunID = strings.TrimSpace(usage.RunID)
	if usage.UserID == "" || usage.RunID == "" {
		return ErrInvalidPointCommand
	}
	return s.updateWithPersonalPoints(context.Background(), func(data *platformData, points *JSONPersonalPointStore) error {
		for _, item := range data.BillingEvents {
			if item.TaskID == usage.RunID && strings.EqualFold(item.MetricCode, billingMetricKnowledgeRAG) {
				return nil
			}
		}
		available, after, err := chargeJSONPersonalPointUsage(context.Background(), points, personalPointUsageChargeCommand{
			UserID: usage.UserID, BusinessType: "RAG_RUN", BusinessID: usage.RunID, Points: usage.PointCost, IdempotencyPrefix: "rag:" + usage.RunID,
		})
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		data.BillingEvents = append(data.BillingEvents, ragBillingEvent(usage, available, after, now, uniqueAdminID("evt", billingEventIDs(data.BillingEvents))))
		return nil
	})
}

type personalPointUsageChargeCommand struct {
	UserID, BusinessType, BusinessID, IdempotencyPrefix string
	Points                                              int64
}

func chargeJSONPersonalPointUsage(ctx context.Context, points *JSONPersonalPointStore, cmd personalPointUsageChargeCommand) (int, int, error) {
	if points == nil || strings.TrimSpace(cmd.UserID) == "" || strings.TrimSpace(cmd.BusinessType) == "" || strings.TrimSpace(cmd.BusinessID) == "" || strings.TrimSpace(cmd.IdempotencyPrefix) == "" || cmd.Points <= 0 {
		return 0, 0, ErrInvalidPointCommand
	}
	account, err := personalPointAccountForUserState(points.memory, cmd.UserID)
	if err != nil {
		return 0, 0, err
	}
	before := account.AvailablePoints
	reserved, err := points.reserve(ctx, PersonalPointReserveCommand{
		AccountID: account.ID, UserID: cmd.UserID, BusinessType: cmd.BusinessType, BusinessID: cmd.BusinessID,
		RequestedPoints: cmd.Points, IdempotencyKey: cmd.IdempotencyPrefix + ":reserve",
	})
	if err != nil {
		return 0, 0, err
	}
	if _, err := points.capture(ctx, PersonalPointCaptureCommand{
		AccountID: account.ID, UserID: cmd.UserID, ReservationID: reserved.Reservation.ID,
		Points: cmd.Points, IdempotencyKey: cmd.IdempotencyPrefix + ":capture",
	}); err != nil {
		return 0, 0, err
	}
	account, err = personalPointAccountForUserState(points.memory, cmd.UserID)
	if err != nil {
		return 0, 0, err
	}
	return int(before), int(account.AvailablePoints), nil
}

func chargePostgresPersonalPointUsage(ctx context.Context, db *sql.DB, tx *sql.Tx, cmd personalPointUsageChargeCommand) (int, int, error) {
	if db == nil || tx == nil || strings.TrimSpace(cmd.UserID) == "" || strings.TrimSpace(cmd.BusinessType) == "" || strings.TrimSpace(cmd.BusinessID) == "" || strings.TrimSpace(cmd.IdempotencyPrefix) == "" || cmd.Points <= 0 {
		return 0, 0, ErrInvalidPointCommand
	}
	account, err := pgLoadPersonalAccountForUserTx(ctx, tx, cmd.UserID)
	if err != nil {
		return 0, 0, err
	}
	before := account.Available
	points := NewPostgresPersonalPointStore(db)
	reserved, err := points.reserveTx(ctx, tx, PersonalPointReserveCommand{
		AccountID: account.ID, UserID: cmd.UserID, BusinessType: cmd.BusinessType, BusinessID: cmd.BusinessID,
		RequestedPoints: cmd.Points, IdempotencyKey: cmd.IdempotencyPrefix + ":reserve",
	})
	if err != nil {
		return 0, 0, err
	}
	if _, err := points.captureTx(ctx, tx, PersonalPointCaptureCommand{
		AccountID: account.ID, UserID: cmd.UserID, ReservationID: reserved.Reservation.ID,
		Points: cmd.Points, IdempotencyKey: cmd.IdempotencyPrefix + ":capture",
	}); err != nil {
		return 0, 0, err
	}
	account, err = pgLoadPersonalAccountForUserTx(ctx, tx, cmd.UserID)
	if err != nil {
		return 0, 0, err
	}
	return int(before), int(account.Available), nil
}

func (s *postgresStore) RecordRAGUsage(ctx context.Context, usage knowledgeapp.RAGBillingUsage) error {
	if usage.PointCost <= 0 {
		return nil
	}
	usage.UserID = strings.TrimSpace(usage.UserID)
	usage.RunID = strings.TrimSpace(usage.RunID)
	if usage.UserID == "" || usage.RunID == "" {
		return ErrInvalidPointCommand
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
	before, after := 0, 0
	if authorization.ContextType == contextEnterprise {
		reservation, err := s.reserveEnterpriseComputeTx(ctx, tx, authorization, int64(pointCost), "RAG_RUN", usage.RunID)
		if err != nil {
			return err
		}
		before, after = int(reservation.BalanceBefore), int(reservation.BalanceAfter)
		usage.TenantID = authorization.TenantID
	} else {
		before, after, err = chargePostgresPersonalPointUsage(ctx, s.db, tx, personalPointUsageChargeCommand{
			UserID: usage.UserID, BusinessType: "RAG_RUN", BusinessID: usage.RunID,
			Points: usage.PointCost, IdempotencyPrefix: "rag:" + usage.RunID,
		})
		if err != nil {
			return err
		}
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
