package channelrules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

var ErrOrderRuleSnapshotConflict = errors.New("channel order rule snapshot conflict")

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, errors.New("channel rule postgres store requires a database")
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) LoadEffectiveRuleBundle(ctx context.Context, query RuleBundleQuery) (RuleBundle, error) {
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.PlanID = strings.TrimSpace(query.PlanID)
	if query.TenantID == "" || query.PlanID == "" || query.BusinessTime.IsZero() {
		return RuleBundle{}, fmt.Errorf("%s: invalid rule bundle query", ErrCodeRuleValidationFailed)
	}

	var bundle RuleBundle
	err := s.db.QueryRowContext(ctx, `
		SELECT rs.id, rs.tenant_id, rs.rule_code, rs.version, rs.name, rs.description,
		       rs.status, rs.effective_start_at, rs.effective_end_at, rs.created_at, rs.updated_at,
		       pv.id, pv.tenant_id, pv.rule_set_id, pv.plan_id, pv.version,
		       pv.price_cents, pv.currency, pv.token_rights_value_cents,
		       pv.token_grant_amount, pv.duration_days, pv.identity_type
		FROM xz_commercial_rule_sets rs
		JOIN xz_commercial_plan_versions pv ON pv.rule_set_id = rs.id
		WHERE rs.tenant_id = $1 AND pv.plan_id = $2 AND rs.status = 'PUBLISHED'
		  AND rs.effective_start_at <= $3
		  AND (rs.effective_end_at IS NULL OR rs.effective_end_at > $3)
		ORDER BY rs.version DESC
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
	bundle.Plans = []PlanConfigVersion{bundle.Plan}

	scenario, err := scenarioForIdentity(bundle.Plan.IdentityType)
	if err != nil {
		return RuleBundle{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM xz_commission_rules
		WHERE commercial_rule_set_id = $1 AND commercial_scenario_code = $2 AND status = 'ACTIVE'
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
	return bundle, nil
}

func (s *PostgresStore) LoadRuleBundleByID(ctx context.Context, tenantID, ruleSetID string) (RuleBundle, error) {
	var bundle RuleBundle
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, rule_code, version, name, description, status,
		       effective_start_at, effective_end_at, created_at, updated_at
		FROM xz_commercial_rule_sets WHERE tenant_id = $1 AND id = $2
	`, strings.TrimSpace(tenantID), strings.TrimSpace(ruleSetID)).Scan(
		&bundle.RuleSet.ID, &bundle.RuleSet.TenantID, &bundle.RuleSet.Code, &bundle.RuleSet.Version,
		&bundle.RuleSet.Name, &bundle.RuleSet.Description, &bundle.RuleSet.Status,
		&bundle.RuleSet.EffectiveStartAt, &bundle.RuleSet.EffectiveEndAt,
		&bundle.RuleSet.CreatedAt, &bundle.RuleSet.UpdatedAt,
	)
	if err != nil {
		return RuleBundle{}, err
	}

	planRows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, rule_set_id, plan_id, version, price_cents, currency,
		       token_rights_value_cents, token_grant_amount, duration_days, identity_type
		FROM xz_commercial_plan_versions WHERE rule_set_id = $1 ORDER BY plan_id
	`, bundle.RuleSet.ID)
	if err != nil {
		return RuleBundle{}, err
	}
	for planRows.Next() {
		var plan PlanConfigVersion
		if err := planRows.Scan(&plan.ID, &plan.TenantID, &plan.RuleSetID, &plan.PlanID, &plan.Version,
			&plan.PriceCents, &plan.Currency, &plan.TokenRightsValueCents, &plan.TokenGrantAmount,
			&plan.DurationDays, &plan.IdentityType); err != nil {
			planRows.Close()
			return RuleBundle{}, err
		}
		bundle.Plans = append(bundle.Plans, plan)
	}
	if err := planRows.Close(); err != nil {
		return RuleBundle{}, err
	}
	if err := planRows.Err(); err != nil {
		return RuleBundle{}, err
	}

	ruleRows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM xz_commission_rules WHERE commercial_rule_set_id = $1
		ORDER BY commercial_scenario_code, priority, rule_code
	`, bundle.RuleSet.ID)
	if err != nil {
		return RuleBundle{}, err
	}
	for ruleRows.Next() {
		rule, scanErr := scanCommissionRule(ruleRows)
		if scanErr != nil {
			ruleRows.Close()
			return RuleBundle{}, scanErr
		}
		bundle.Rules = append(bundle.Rules, rule)
	}
	if err := ruleRows.Close(); err != nil {
		return RuleBundle{}, err
	}
	if err := ruleRows.Err(); err != nil {
		return RuleBundle{}, err
	}

	referralRows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, rule_set_id, rule_code, version, referrer_type,
		       beneficiary_type, beneficiary_relation, amount_cents, freeze_days, status
		FROM xz_referral_reward_rule_versions WHERE rule_set_id = $1 ORDER BY rule_code
	`, bundle.RuleSet.ID)
	if err != nil {
		return RuleBundle{}, err
	}
	defer referralRows.Close()
	for referralRows.Next() {
		var rule ReferralRewardRule
		if err := referralRows.Scan(&rule.ID, &rule.TenantID, &rule.RuleSetID, &rule.Code, &rule.Version,
			&rule.ReferrerType, &rule.BeneficiaryType, &rule.BeneficiaryRelation,
			&rule.AmountCents, &rule.FreezeDays, &rule.Status); err != nil {
			return RuleBundle{}, err
		}
		bundle.ReferralRules = append(bundle.ReferralRules, rule)
	}
	return bundle, referralRows.Err()
}

func (s *PostgresStore) PublishRuleBundle(ctx context.Context, request PublishRuleSetRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var tenantID, ruleCode string
	var effectiveStart time.Time
	var status RuleSetStatus
	if err := tx.QueryRowContext(ctx, `
		SELECT tenant_id, rule_code, effective_start_at, status
		FROM xz_commercial_rule_sets WHERE id = $1 AND tenant_id = $2 FOR UPDATE
	`, request.RuleSetID, request.TenantID).Scan(&tenantID, &ruleCode, &effectiveStart, &status); err != nil {
		return err
	}
	if status != RuleSetDraft {
		return fmt.Errorf("%s: only draft rule sets can be published", ErrCodeRuleValidationFailed)
	}
	var conflictingFuture int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM xz_commercial_rule_sets
		WHERE tenant_id = $1 AND rule_code = $2 AND id <> $3 AND status = 'PUBLISHED'
		  AND effective_start_at >= $4
	`, tenantID, ruleCode, request.RuleSetID, effectiveStart).Scan(&conflictingFuture); err != nil {
		return err
	}
	if conflictingFuture > 0 {
		return fmt.Errorf("%s: a published rule set already starts at or after this version", ErrCodeRuleValidationFailed)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commercial_rule_sets
		SET effective_end_at = $4, updated_at = $5
		WHERE tenant_id = $1 AND rule_code = $2 AND id <> $3 AND status = 'PUBLISHED'
		  AND effective_start_at < $4 AND (effective_end_at IS NULL OR effective_end_at > $4)
	`, tenantID, ruleCode, request.RuleSetID, effectiveStart, request.PublishedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commercial_rule_sets
		SET status = 'PUBLISHED', published_by = $2, published_at = $3, updated_at = $3
		WHERE id = $1
	`, request.RuleSetID, request.OperatorID, request.PublishedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commission_rules SET status = 'ACTIVE', updated_at = $2
		WHERE commercial_rule_set_id = $1 AND status = 'DRAFT'
	`, request.RuleSetID, request.PublishedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_referral_reward_rule_versions SET status = 'PUBLISHED', updated_at = $2
		WHERE rule_set_id = $1 AND status = 'DRAFT'
	`, request.RuleSetID, request.PublishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) ResolveRelationshipSnapshot(ctx context.Context, query RelationshipQuery) (RelationshipSnapshot, error) {
	snapshot := RelationshipSnapshot{
		SourceUserID: strings.TrimSpace(query.SourceUserID), EffectiveAt: query.BusinessTime, SourceType: "NONE",
	}
	var effectiveFrom time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT coalesce(direct_agent_user_id, ''), coalesce(operation_center_user_id, ''),
		       effective_from, source_type, source_id
		FROM xz_channel_relationship_history
		WHERE tenant_id = $1 AND subject_user_id = $2 AND status = 'ACTIVE'
		  AND effective_from <= $3 AND (effective_to IS NULL OR effective_to > $3)
		ORDER BY effective_from DESC
		LIMIT 1
	`, strings.TrimSpace(query.TenantID), snapshot.SourceUserID, query.BusinessTime).Scan(
		&snapshot.DirectAgentID, &snapshot.OperationCenterID, &effectiveFrom, &snapshot.SourceType, &snapshot.SourceID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, nil
	}
	if err != nil {
		return RelationshipSnapshot{}, err
	}
	snapshot.EffectiveAt = effectiveFrom
	return snapshot, nil
}

func (s *PostgresStore) SaveOrderRuleSnapshot(ctx context.Context, snapshot OrderRuleSnapshot) error {
	relationshipJSON, err := json.Marshal(snapshot.Relationship)
	if err != nil {
		return err
	}
	rulesJSON, err := json.Marshal(snapshot.CommissionRules)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO xz_commercial_order_rule_snapshots (
		  id, tenant_id, order_id, order_no, source_user_id, plan_id, plan_version_id,
		  rule_set_id, rule_set_version, scenario_code, paid_amount_cents,
		  token_rights_value_cents, token_grant_amount, direct_agent_user_id,
		  operation_center_user_id, business_time, relationship_snapshot, commission_rule_snapshot
		) VALUES (
		  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,nullif($14,''),nullif($15,''),$16,$17::jsonb,$18::jsonb
		)
		ON CONFLICT (order_id) DO NOTHING
	`, "channel_order_snapshot_"+snapshot.OrderID, snapshot.TenantID, snapshot.OrderID, snapshot.OrderNo,
		snapshot.SourceUserID, snapshot.PlanID, snapshot.PlanVersionID, snapshot.RuleSetID,
		snapshot.RuleSetVersion, snapshot.Scenario, snapshot.PaidAmountCents,
		snapshot.TokenRightsValueCents, snapshot.TokenGrantAmount, snapshot.DirectAgentID,
		snapshot.OperationCenterID, snapshot.BusinessTime, relationshipJSON, rulesJSON)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 1 {
		return err
	}

	var existingRuleSetID, existingPlanVersionID string
	var existingRuleSetVersion int
	if err := s.db.QueryRowContext(ctx, `
		SELECT rule_set_id, rule_set_version, plan_version_id
		FROM xz_commercial_order_rule_snapshots WHERE order_id = $1
	`, snapshot.OrderID).Scan(&existingRuleSetID, &existingRuleSetVersion, &existingPlanVersionID); err != nil {
		return err
	}
	if existingRuleSetID != snapshot.RuleSetID || existingRuleSetVersion != snapshot.RuleSetVersion || existingPlanVersionID != snapshot.PlanVersionID {
		return ErrOrderRuleSnapshotConflict
	}
	return nil
}

type commissionRuleScanner interface {
	Scan(...any) error
}

func scanCommissionRule(scanner commissionRuleScanner) (commissionapp.CommissionRule, error) {
	var rule commissionapp.CommissionRule
	var config []byte
	var effectiveEnd sql.NullTime
	err := scanner.Scan(
		&rule.ID, &rule.TenantID, &rule.Code, &rule.Name, &rule.ProductType, &rule.ProductID,
		&rule.BeneficiaryRole, &rule.RelationshipLevel, &rule.CalculationType,
		&rule.FixedAmountCents, &rule.PercentageBPS, &config, &rule.Priority,
		&rule.FreezeDays, &rule.RefundPolicy, &rule.EffectiveStartAt, &effectiveEnd,
		&rule.Version, &rule.Status, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return commissionapp.CommissionRule{}, err
	}
	rule.CalculationConfig = append(json.RawMessage(nil), config...)
	if effectiveEnd.Valid {
		rule.EffectiveEndAt = &effectiveEnd.Time
	}
	return rule, nil
}
