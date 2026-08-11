package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	channelrules "xianzhi-ai/backend-go/internal/app/channelrules"
)

type settlementEngine string

const (
	settlementEngineLegacy settlementEngine = "LEGACY"
	settlementEngineV132   settlementEngine = "V132"
)

type orderSettlementDecision struct {
	OrderID          string
	TenantID         string
	UserID           string
	PlanID           string
	SettlementEngine settlementEngine
	RuleSetID        string
	RuleSetVersion   int
	ConfigVersion    int
	RolloutMode      string
	Bucket           int
	Reason           string
	DecidedAt        time.Time
}

func preserveExistingSettlementDecision(existing, _ orderSettlementDecision) (orderSettlementDecision, error) {
	if strings.TrimSpace(existing.OrderID) == "" || (existing.SettlementEngine != settlementEngineLegacy && existing.SettlementEngine != settlementEngineV132) {
		return orderSettlementDecision{}, errors.New("invalid existing settlement decision")
	}
	return existing, nil
}

func validateSettlementWriteSource(expected, attempted settlementEngine) error {
	if expected != attempted {
		return fmt.Errorf("settlement write source conflict: order is pinned to %s, attempted %s", expected, attempted)
	}
	return nil
}

func validateV132SettlementConservation(paid, token, directAgent, operationCenter, platform int64) error {
	if paid <= 0 || token < 0 || directAgent < 0 || operationCenter < 0 || platform < 0 {
		return errors.New("invalid V1.3.2 settlement amount")
	}
	if token+directAgent+operationCenter+platform != paid {
		return fmt.Errorf("V1.3.2 settlement amount is not conserved: paid=%d allocated=%d", paid, token+directAgent+operationCenter+platform)
	}
	return nil
}

func resolveOrderSettlementDecisionTx(ctx context.Context, tx *sql.Tx, order *adminOrder, plan adminPlan) (orderSettlementDecision, error) {
	if order == nil || tx == nil {
		return orderSettlementDecision{}, errors.New("order and transaction are required for settlement decision")
	}
	if existing, found, err := loadOrderSettlementDecisionTx(ctx, tx, order.ID); err != nil {
		return orderSettlementDecision{}, err
	} else if found {
		applySettlementDecisionSnapshot(order, existing)
		return existing, nil
	}

	tenantID := firstNonEmptyString(order.TenantID, "tenant_default")
	config, found, err := loadChannelRolloutConfigTx(ctx, tx, tenantID)
	if err != nil {
		return orderSettlementDecision{}, err
	}
	decision := orderSettlementDecision{
		OrderID: order.ID, TenantID: tenantID, UserID: order.UserID, PlanID: plan.ID,
		SettlementEngine: settlementEngineLegacy, RolloutMode: string(channelrules.RolloutModeLegacy),
		Bucket: -1, Reason: "NO_ROLLOUT_CONFIG", DecidedAt: time.Now().UTC(),
	}
	if found {
		rolloutDecision, evaluateErr := channelrules.EvaluateRollout(config, channelrules.RolloutSubject{
			TenantID: tenantID, OrderID: order.ID, UserID: order.UserID, PlanID: plan.ID,
			OperationCenterPackage: planBusinessType(plan) == planTypeOperationCenterPackage,
		})
		if evaluateErr != nil {
			return orderSettlementDecision{}, evaluateErr
		}
		decision.ConfigVersion = config.ConfigVersion
		decision.RolloutMode = string(config.Mode)
		decision.Bucket = rolloutDecision.Bucket
		decision.Reason = rolloutDecision.Reason
		if rolloutDecision.UseV132Settlement {
			decision.SettlementEngine = settlementEngineV132
			decision.RuleSetID = config.PinnedRuleSetID
			decision.RuleSetVersion = config.PinnedRuleSetVersion
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_order_settlement_engine_decisions (
		  order_id,tenant_id,user_id,plan_id,settlement_engine,rule_set_id,rule_set_version,
		  rollout_config_version,rollout_mode,hash_bucket,decision_reason,decided_at
		) VALUES ($1,$2,$3,$4,$5,nullif($6,''),nullif($7,0),nullif($8,0),$9,$10,$11,$12)
		ON CONFLICT (order_id) DO NOTHING
	`, decision.OrderID, decision.TenantID, decision.UserID, decision.PlanID, decision.SettlementEngine,
		decision.RuleSetID, decision.RuleSetVersion, decision.ConfigVersion, decision.RolloutMode,
		decision.Bucket, decision.Reason, decision.DecidedAt); err != nil {
		return orderSettlementDecision{}, err
	}
	stored, found, err := loadOrderSettlementDecisionTx(ctx, tx, order.ID)
	if err != nil || !found {
		return orderSettlementDecision{}, firstNonNilError(err, errors.New("settlement decision was not persisted"))
	}
	applySettlementDecisionSnapshot(order, stored)
	return stored, nil
}

func loadOrderSettlementDecisionTx(ctx context.Context, tx *sql.Tx, orderID string) (orderSettlementDecision, bool, error) {
	var item orderSettlementDecision
	var ruleSetID sql.NullString
	var ruleSetVersion, configVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `
		SELECT order_id,tenant_id,user_id,plan_id,settlement_engine,rule_set_id,rule_set_version,
		       rollout_config_version,rollout_mode,hash_bucket,decision_reason,decided_at
		FROM xz_order_settlement_engine_decisions WHERE order_id=$1
	`, orderID).Scan(&item.OrderID, &item.TenantID, &item.UserID, &item.PlanID, &item.SettlementEngine,
		&ruleSetID, &ruleSetVersion, &configVersion, &item.RolloutMode, &item.Bucket, &item.Reason, &item.DecidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return orderSettlementDecision{}, false, nil
	}
	if err != nil {
		return orderSettlementDecision{}, false, err
	}
	item.RuleSetID = ruleSetID.String
	item.RuleSetVersion = int(ruleSetVersion.Int64)
	item.ConfigVersion = int(configVersion.Int64)
	return item, true, nil
}

func claimSettlementWriteSourceTx(ctx context.Context, tx *sql.Tx, decision orderSettlementDecision) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_order_settlement_write_sources (order_id,tenant_id,settlement_engine,created_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT (order_id) DO NOTHING
	`, decision.OrderID, decision.TenantID, decision.SettlementEngine, decision.DecidedAt); err != nil {
		return err
	}
	var existing settlementEngine
	if err := tx.QueryRowContext(ctx, `SELECT settlement_engine FROM xz_order_settlement_write_sources WHERE order_id=$1`, decision.OrderID).Scan(&existing); err != nil {
		return err
	}
	return validateSettlementWriteSource(existing, decision.SettlementEngine)
}

func loadChannelRolloutConfigTx(ctx context.Context, tx *sql.Tx, tenantID string) (channelrules.RolloutConfig, bool, error) {
	var config channelrules.RolloutConfig
	var allowOrders, allowUsers, allowPlans, allowTenants, denyOrders, denyUsers []byte
	err := tx.QueryRowContext(ctx, `
		SELECT tenant_id,config_version,mode,enabled,pinned_rule_set_id,pinned_rule_set_version,
		       canary_basis_points,hash_salt,allow_order_ids,allow_user_ids,allow_plan_ids,allow_tenant_ids,
		       deny_order_ids,deny_user_ids,percentage_rollout_enabled,real_switch_enabled
		FROM xz_channel_rollout_configs WHERE tenant_id=$1
	`, tenantID).Scan(&config.TenantID, &config.ConfigVersion, &config.Mode, &config.Enabled,
		&config.PinnedRuleSetID, &config.PinnedRuleSetVersion, &config.CanaryBasisPoints,
		&config.HashSalt, &allowOrders, &allowUsers, &allowPlans, &allowTenants, &denyOrders, &denyUsers,
		&config.PercentageRolloutEnabled, &config.RealSwitchEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return channelrules.RolloutConfig{}, false, nil
	}
	if err != nil {
		return channelrules.RolloutConfig{}, false, err
	}
	for payload, target := range map[*[]string][]byte{
		&config.AllowOrderIDs: allowOrders, &config.AllowUserIDs: allowUsers, &config.AllowPlanIDs: allowPlans,
		&config.AllowTenantIDs: allowTenants,
		&config.DenyOrderIDs:   denyOrders, &config.DenyUserIDs: denyUsers,
	} {
		if err := json.Unmarshal(target, payload); err != nil {
			return channelrules.RolloutConfig{}, false, err
		}
	}
	return config, true, nil
}

func applySettlementDecisionSnapshot(order *adminOrder, decision orderSettlementDecision) {
	if order.PriceSnapshot == nil {
		order.PriceSnapshot = map[string]any{}
	}
	order.PriceSnapshot["settlementEngine"] = string(decision.SettlementEngine)
	order.PriceSnapshot["settlementRuleSetId"] = decision.RuleSetID
	order.PriceSnapshot["settlementRuleSetVersion"] = decision.RuleSetVersion
	order.PriceSnapshot["settlementRolloutConfigVersion"] = decision.ConfigVersion
	order.PriceSnapshot["settlementRolloutMode"] = decision.RolloutMode
	order.PriceSnapshot["settlementDecisionReason"] = decision.Reason
	order.PriceSnapshot["settlementDecidedAt"] = decision.DecidedAt.Format(time.RFC3339Nano)
}

func firstNonNilError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}
