package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	channelrules "xianzhi-ai/backend-go/internal/app/channelrules"
	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

const commerceShadowVersion = "V1.3.2"

type commerceShadowDifference struct {
	Match                      bool  `json:"match"`
	DirectAgentDeltaCents      int64 `json:"directAgentDeltaCents"`
	ParentAgentDeltaCents      int64 `json:"parentAgentDeltaCents"`
	OperationCenterDeltaCents  int64 `json:"operationCenterDeltaCents"`
	TokenGrantDelta            int64 `json:"tokenGrantDelta"`
	TokenRightsValueDeltaCents int64 `json:"tokenRightsValueDeltaCents"`
	PlatformDeltaCents         int64 `json:"platformDeltaCents"`
	LegacyConservationCents    int64 `json:"legacyConservationCents"`
	V132ConservationCents      int64 `json:"v132ConservationCents"`
}

type commerceShadowRecord struct {
	Status       string
	RuleSetID    string
	RuleVersion  int
	Scenario     string
	Legacy       commissionSettlementResult
	V132         *channelrules.OrderCalculation
	Difference   *commerceShadowDifference
	Relationship channelrules.RelationshipSnapshot
	ErrorCode    string
	ErrorMessage string
}

func compareCommerceShadow(legacy commissionSettlementResult, v132 channelrules.OrderCalculation) commerceShadowDifference {
	difference := commerceShadowDifference{
		DirectAgentDeltaCents:      v132.DirectAgentAmountCents - int64(legacy.DirectAgentRewardCents),
		ParentAgentDeltaCents:      -int64(legacy.ParentAgentRewardCents),
		OperationCenterDeltaCents:  v132.OperationCenterAmountCents - int64(legacy.OperationCenterRewardCents),
		TokenGrantDelta:            v132.TokenGrantAmount - int64(legacy.TokenGrantAmount),
		TokenRightsValueDeltaCents: v132.TokenRightsValueCents - int64(legacy.TokenGrantValueCents),
		PlatformDeltaCents:         v132.PlatformAmountCents - int64(legacy.PlatformIncomeCents),
		LegacyConservationCents: int64(legacy.TokenGrantValueCents + legacy.DirectAgentRewardCents +
			legacy.ParentAgentRewardCents + legacy.OperationCenterRewardCents + legacy.PlatformIncomeCents),
		V132ConservationCents: v132.TokenRightsValueCents + v132.DirectAgentAmountCents +
			v132.OperationCenterAmountCents + v132.PlatformAmountCents,
	}
	difference.Match = difference.DirectAgentDeltaCents == 0 && difference.ParentAgentDeltaCents == 0 &&
		difference.OperationCenterDeltaCents == 0 && difference.TokenGrantDelta == 0 &&
		difference.TokenRightsValueDeltaCents == 0 && difference.PlatformDeltaCents == 0 &&
		difference.LegacyConservationCents == difference.V132ConservationCents
	return difference
}

func recordCommerceShadowDifferenceTx(ctx context.Context, tx *sql.Tx, order adminOrder, plan adminPlan, commerceCtx commissionOrderContext, legacy commissionSettlementResult) {
	if tx == nil {
		return
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT channel_commerce_shadow"); err != nil {
		return
	}
	record, err := calculateCommerceShadow(ctx, tx, order, plan, commerceCtx, legacy)
	if err != nil {
		_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT channel_commerce_shadow")
		record = commerceShadowRecord{
			Status: "ERROR", Legacy: legacy,
			Relationship: channelrules.RelationshipSnapshot{
				SourceUserID: commerceCtx.BuyerUserID, DirectAgentID: commerceCtx.DirectAgentID,
				OperationCenterID: commerceCtx.OperationCenterID, SourceType: "LEGACY_FULFILLMENT_CONTEXT", SourceID: order.ID,
			},
			ErrorCode: "SHADOW_CALCULATION_ERROR", ErrorMessage: err.Error(),
		}
	}
	if err := insertCommerceShadowRecordTx(ctx, tx, order, record); err != nil {
		_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT channel_commerce_shadow")
	}
	_, _ = tx.ExecContext(ctx, "RELEASE SAVEPOINT channel_commerce_shadow")
}

func calculateCommerceShadow(ctx context.Context, tx *sql.Tx, order adminOrder, plan adminPlan, commerceCtx commissionOrderContext, legacy commissionSettlementResult) (commerceShadowRecord, error) {
	businessTime, err := time.Parse(time.RFC3339Nano, nowForOrder(order))
	if err != nil {
		return commerceShadowRecord{}, err
	}
	store, err := channelrules.NewTransactionStore(tx)
	if err != nil {
		return commerceShadowRecord{}, err
	}
	request := channelrules.ResolveOrderRequest{
		TenantID: firstNonEmptyString(order.TenantID, "tenant_default"), OrderID: order.ID,
		OrderNo: firstNonEmptyString(order.OrderNo, order.ID), PlanID: plan.ID,
		SourceUserID:    firstNonEmptyString(commerceCtx.BuyerUserID, order.UserID),
		PaidAmountCents: int64(commerceCtx.AmountCents), BusinessTime: businessTime,
	}
	relationship := channelrules.RelationshipSnapshot{
		SourceUserID: request.SourceUserID, DirectAgentID: commerceCtx.DirectAgentID,
		OperationCenterID: commerceCtx.OperationCenterID, EffectiveAt: businessTime,
		SourceType: "LEGACY_FULFILLMENT_CONTEXT", SourceID: order.ID,
	}
	resolved, err := channelrules.NewChannelRuleService(store).ResolveShadowOrder(ctx, request, relationship)
	if err != nil {
		return commerceShadowRecord{}, err
	}
	calculation, err := channelrules.NewCommissionEngineAdapter(commissionapp.NewEngine()).Calculate(request, resolved)
	if err != nil {
		return commerceShadowRecord{}, err
	}
	difference := compareCommerceShadow(legacy, calculation)
	status := "DIFFERENT"
	if difference.Match {
		status = "MATCH"
	}
	return commerceShadowRecord{
		Status: status, RuleSetID: resolved.RuleSet.ID, RuleVersion: resolved.RuleSet.Version,
		Scenario: string(resolved.Scenario), Legacy: legacy, V132: &calculation,
		Difference: &difference, Relationship: relationship,
	}, nil
}

func insertCommerceShadowRecordTx(ctx context.Context, tx *sql.Tx, order adminOrder, record commerceShadowRecord) error {
	legacyJSON, err := json.Marshal(record.Legacy)
	if err != nil {
		return err
	}
	v132JSON, err := nullableShadowJSON(record.V132)
	if err != nil {
		return err
	}
	differenceJSON, err := nullableShadowJSON(record.Difference)
	if err != nil {
		return err
	}
	relationshipJSON, err := json.Marshal(record.Relationship)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_commercial_shadow_differences (
		  id, tenant_id, order_id, order_no, plan_id, scenario_code,
		  shadow_rule_set_id, shadow_rule_set_version, shadow_version,
		  comparison_status, legacy_result, v132_result, difference,
		  error_code, error_message, relationship_snapshot
		) VALUES (
		  $1,$2,$3,$4,$5,nullif($6,''),nullif($7,''),nullif($8,0),$9,
		  $10,$11::jsonb,$12::jsonb,$13::jsonb,$14,$15,$16::jsonb
		)
		ON CONFLICT (order_id, shadow_version, shadow_rule_set_id) DO NOTHING
	`, "shadow_"+shortID(order.ID+"_"+record.RuleSetID+"_"+commerceShadowVersion),
		firstNonEmptyString(order.TenantID, "tenant_default"), order.ID,
		firstNonEmptyString(order.OrderNo, order.ID), order.PlanID, record.Scenario,
		record.RuleSetID, record.RuleVersion, commerceShadowVersion, record.Status,
		legacyJSON, v132JSON, differenceJSON, record.ErrorCode, record.ErrorMessage, relationshipJSON)
	return err
}

func nullableShadowJSON(value any) ([]byte, error) {
	if value == nil {
		return []byte("null"), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal shadow payload: %w", err)
	}
	return data, nil
}
