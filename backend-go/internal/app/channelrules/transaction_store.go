package channelrules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type TransactionStore struct {
	tx *sql.Tx
}

func NewTransactionStore(tx *sql.Tx) (*TransactionStore, error) {
	if tx == nil {
		return nil, errors.New("channel rule transaction store requires a transaction")
	}
	return &TransactionStore{tx: tx}, nil
}

func (s *TransactionStore) LoadEffectiveRuleBundle(context.Context, RuleBundleQuery) (RuleBundle, error) {
	return RuleBundle{}, errors.New("transaction store only supports shadow rule loading")
}

func (s *TransactionStore) ResolveRelationshipSnapshot(context.Context, RelationshipQuery) (RelationshipSnapshot, error) {
	return RelationshipSnapshot{}, errors.New("shadow relationship must be supplied by the fulfillment context")
}

func (s *TransactionStore) SaveOrderRuleSnapshot(context.Context, OrderRuleSnapshot) error {
	return errors.New("shadow mode cannot save formal order rule snapshots")
}

func (s *TransactionStore) LoadShadowRuleBundle(ctx context.Context, query RuleBundleQuery) (RuleBundle, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.PlanID = strings.TrimSpace(query.PlanID)
	if query.TenantID == "" || query.PlanID == "" || query.BusinessTime.IsZero() {
		return RuleBundle{}, fmt.Errorf("%s: invalid shadow rule bundle query", ErrCodeRuleValidationFailed)
	}
	var bundle RuleBundle
	err := s.tx.QueryRowContext(ctx, `
		SELECT rs.id, rs.tenant_id, rs.rule_code, rs.version, rs.name, rs.description,
		       rs.status, rs.effective_start_at, rs.effective_end_at, rs.created_at, rs.updated_at,
		       pv.id, pv.tenant_id, pv.rule_set_id, pv.plan_id, pv.version,
		       pv.price_cents, pv.currency, pv.token_rights_value_cents,
		       pv.token_grant_amount, pv.duration_days, pv.identity_type
		FROM xz_channel_rollout_configs rc
		JOIN xz_commercial_rule_sets rs
		  ON rs.id = rc.pinned_rule_set_id AND rs.version = rc.pinned_rule_set_version
		JOIN xz_commercial_plan_versions pv ON pv.rule_set_id = rs.id
		WHERE rc.tenant_id = $1 AND rc.enabled = TRUE AND rc.mode IN ('SHADOW', 'CANARY', 'V132')
		  AND rs.tenant_id = rc.tenant_id AND pv.plan_id = $2 AND rs.status IN ('DRAFT', 'PUBLISHED')
		  AND rs.effective_start_at <= $3
		  AND (rs.effective_end_at IS NULL OR rs.effective_end_at > $3)
		ORDER BY rc.config_version DESC
		LIMIT 1
	`, query.TenantID, query.PlanID, query.BusinessTime).Scan(
		&bundle.RuleSet.ID, &bundle.RuleSet.TenantID, &bundle.RuleSet.Code, &bundle.RuleSet.Version,
		&bundle.RuleSet.Name, &bundle.RuleSet.Description, &bundle.RuleSet.Status,
		&bundle.RuleSet.EffectiveStartAt, &bundle.RuleSet.EffectiveEndAt,
		&bundle.RuleSet.CreatedAt, &bundle.RuleSet.UpdatedAt,
		&bundle.Plan.ID, &bundle.Plan.TenantID, &bundle.Plan.RuleSetID, &bundle.Plan.PlanID,
		&bundle.Plan.Version, &bundle.Plan.PriceCents, &bundle.Plan.Currency,
		&bundle.Plan.TokenRightsValueCents, &bundle.Plan.TokenGrantAmount,
		&bundle.Plan.DurationDays, &bundle.Plan.IdentityType,
	)
	if err != nil {
		return RuleBundle{}, err
	}
	scenario, err := scenarioForIdentity(bundle.Plan.IdentityType)
	if err != nil {
		return RuleBundle{}, err
	}
	rows, err := s.tx.QueryContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM xz_commission_rules
		WHERE commercial_rule_set_id = $1 AND commercial_scenario_code = $2
		  AND status IN ('DRAFT', 'ACTIVE')
		ORDER BY priority, rule_code, version DESC
	`, bundle.RuleSet.ID, scenario)
	if err != nil {
		return RuleBundle{}, err
	}
	defer rows.Close()
	for rows.Next() {
		rule, scanErr := scanCommissionRule(rows)
		if scanErr != nil {
			return RuleBundle{}, scanErr
		}
		bundle.Rules = append(bundle.Rules, rule)
	}
	if err := rows.Err(); err != nil {
		return RuleBundle{}, err
	}
	bundle.Plans = []PlanConfigVersion{bundle.Plan}
	return bundle, nil
}

func (s *TransactionStore) LoadPinnedRuleBundle(ctx context.Context, query RuleBundleQuery, ruleSetID string, ruleSetVersion int) (RuleBundle, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.PlanID = strings.TrimSpace(query.PlanID)
	ruleSetID = strings.TrimSpace(ruleSetID)
	if query.TenantID == "" || query.PlanID == "" || query.BusinessTime.IsZero() || ruleSetID == "" || ruleSetVersion <= 0 {
		return RuleBundle{}, fmt.Errorf("%s: invalid pinned rule bundle query", ErrCodeRuleValidationFailed)
	}
	var bundle RuleBundle
	err := s.tx.QueryRowContext(ctx, `
		SELECT rs.id, rs.tenant_id, rs.rule_code, rs.version, rs.name, rs.description,
		       rs.status, rs.effective_start_at, rs.effective_end_at, rs.created_at, rs.updated_at,
		       pv.id, pv.tenant_id, pv.rule_set_id, pv.plan_id, pv.version,
		       pv.price_cents, pv.currency, pv.token_rights_value_cents,
		       pv.token_grant_amount, pv.duration_days, pv.identity_type
		FROM xz_commercial_rule_sets rs
		JOIN xz_commercial_plan_versions pv ON pv.rule_set_id = rs.id
		WHERE rs.tenant_id=$1 AND rs.id=$2 AND rs.version=$3 AND pv.plan_id=$4
		  AND rs.status IN ('DRAFT','PUBLISHED')
		  AND rs.effective_start_at <= $5
		  AND (rs.effective_end_at IS NULL OR rs.effective_end_at > $5)
		LIMIT 1
	`, query.TenantID, ruleSetID, ruleSetVersion, query.PlanID, query.BusinessTime).Scan(
		&bundle.RuleSet.ID, &bundle.RuleSet.TenantID, &bundle.RuleSet.Code, &bundle.RuleSet.Version,
		&bundle.RuleSet.Name, &bundle.RuleSet.Description, &bundle.RuleSet.Status,
		&bundle.RuleSet.EffectiveStartAt, &bundle.RuleSet.EffectiveEndAt,
		&bundle.RuleSet.CreatedAt, &bundle.RuleSet.UpdatedAt,
		&bundle.Plan.ID, &bundle.Plan.TenantID, &bundle.Plan.RuleSetID, &bundle.Plan.PlanID,
		&bundle.Plan.Version, &bundle.Plan.PriceCents, &bundle.Plan.Currency,
		&bundle.Plan.TokenRightsValueCents, &bundle.Plan.TokenGrantAmount,
		&bundle.Plan.DurationDays, &bundle.Plan.IdentityType,
	)
	if err != nil {
		return RuleBundle{}, err
	}
	scenario, err := scenarioForIdentity(bundle.Plan.IdentityType)
	if err != nil {
		return RuleBundle{}, err
	}
	rows, err := s.tx.QueryContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM xz_commission_rules
		WHERE commercial_rule_set_id=$1 AND commercial_scenario_code=$2
		  AND status IN ('DRAFT','ACTIVE')
		ORDER BY priority,rule_code,version DESC
	`, bundle.RuleSet.ID, scenario)
	if err != nil {
		return RuleBundle{}, err
	}
	defer rows.Close()
	for rows.Next() {
		rule, scanErr := scanCommissionRule(rows)
		if scanErr != nil {
			return RuleBundle{}, scanErr
		}
		bundle.Rules = append(bundle.Rules, rule)
	}
	if err := rows.Err(); err != nil {
		return RuleBundle{}, err
	}
	bundle.Plans = []PlanConfigVersion{bundle.Plan}
	return bundle, nil
}
