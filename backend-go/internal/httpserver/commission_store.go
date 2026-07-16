package httpserver

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

type commissionRuleQuery struct {
	TenantID    string
	ProductType string
	ProductID   string
	Status      string
	Limit       int
	Offset      int
}

type commissionRuleRepository interface {
	ListCommissionRules(context.Context, commissionRuleQuery) ([]commissionapp.CommissionRule, int, error)
	CreateCommissionRule(context.Context, commissionapp.CommissionRule) (commissionapp.CommissionRule, error)
	VersionCommissionRule(context.Context, string, string, commissionapp.CommissionRule) (commissionapp.CommissionRule, error)
}

var errCommissionRuleNotFound = errors.New("commission rule not found")

func (s *postgresStore) ListCommissionRules(ctx context.Context, query commissionRuleQuery) ([]commissionapp.CommissionRule, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, 0, err
	}
	query.TenantID = firstNonEmptyString(strings.TrimSpace(query.TenantID), "tenant_default")
	query.ProductType = strings.ToUpper(strings.TrimSpace(query.ProductType))
	query.Status = strings.ToUpper(strings.TrimSpace(query.Status))
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	args := []any{query.TenantID}
	where := []string{"tenant_id = $1"}
	if query.ProductType != "" {
		args = append(args, query.ProductType)
		where = append(where, fmt.Sprintf("product_type = $%d", len(args)))
	}
	if strings.TrimSpace(query.ProductID) != "" {
		args = append(args, strings.TrimSpace(query.ProductID))
		where = append(where, fmt.Sprintf("product_id = $%d", len(args)))
	}
	if query.Status != "" {
		args = append(args, query.Status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM xz_commission_rules WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM xz_commission_rules
		WHERE `+whereSQL+`
		ORDER BY product_type, coalesce(product_id, ''), rule_code, version DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]commissionapp.CommissionRule, 0)
	for rows.Next() {
		item, err := scanCommissionRule(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *postgresStore) CreateCommissionRule(ctx context.Context, rule commissionapp.CommissionRule) (commissionapp.CommissionRule, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return commissionapp.CommissionRule{}, err
	}
	defer tx.Rollback()
	rule.TenantID = firstNonEmptyString(strings.TrimSpace(rule.TenantID), "tenant_default")
	rule.Code = strings.ToUpper(strings.TrimSpace(rule.Code))
	rule.ProductType = strings.ToUpper(strings.TrimSpace(rule.ProductType))
	rule.Status = firstNonEmptyString(strings.ToUpper(strings.TrimSpace(rule.Status)), "DRAFT")
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, rule.TenantID+"|"+rule.Code); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT coalesce(max(version), 0) + 1
		FROM xz_commission_rules WHERE tenant_id=$1 AND rule_code=$2
	`, rule.TenantID, rule.Code).Scan(&rule.Version); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	rule.ID = newCommissionRuleID(rule.Code, rule.Version)
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	if err := rule.Validate(); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if err := insertCommissionRuleTx(ctx, tx, rule); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	return rule, nil
}

func (s *postgresStore) VersionCommissionRule(ctx context.Context, tenantID, id string, next commissionapp.CommissionRule) (commissionapp.CommissionRule, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return commissionapp.CommissionRule{}, err
	}
	defer tx.Rollback()
	current, err := scanCommissionRule(tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM xz_commission_rules WHERE id=$1 AND tenant_id=$2 FOR UPDATE
	`, strings.TrimSpace(id), strings.TrimSpace(tenantID)))
	if errors.Is(err, sql.ErrNoRows) {
		return commissionapp.CommissionRule{}, errCommissionRuleNotFound
	}
	if err != nil {
		return commissionapp.CommissionRule{}, err
	}
	next.TenantID = current.TenantID
	next.Code = current.Code
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, next.TenantID+"|"+next.Code); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT coalesce(max(version), 0) + 1
		FROM xz_commission_rules WHERE tenant_id=$1 AND rule_code=$2
	`, next.TenantID, next.Code).Scan(&next.Version); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	next.ID = newCommissionRuleID(next.Code, next.Version)
	next.ProductType = strings.ToUpper(strings.TrimSpace(next.ProductType))
	next.Status = firstNonEmptyString(strings.ToUpper(strings.TrimSpace(next.Status)), "ACTIVE")
	now := time.Now().UTC()
	next.CreatedAt = now
	next.UpdatedAt = now
	if err := next.Validate(); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_commission_rules SET status='INACTIVE', updated_at=$2 WHERE id=$1`, current.ID, now); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if err := insertCommissionRuleTx(ctx, tx, next); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return commissionapp.CommissionRule{}, err
	}
	return next, nil
}

func insertCommissionRuleTx(ctx context.Context, tx *sql.Tx, rule commissionapp.CommissionRule) error {
	config := rule.CalculationConfig
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_commission_rules (
		  id,tenant_id,rule_code,rule_name,product_type,product_id,beneficiary_role,
		  relationship_level,calculation_type,fixed_amount_cents,percentage_bps,
		  calculation_config,priority,freeze_days,refund_policy,effective_start_at,
		  effective_end_at,version,status,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,nullif($6,''),$7,$8,$9,$10,$11,$12::jsonb,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, rule.ID, rule.TenantID, rule.Code, rule.Name, rule.ProductType, rule.ProductID,
		rule.BeneficiaryRole, rule.RelationshipLevel, rule.CalculationType, rule.FixedAmountCents,
		rule.PercentageBPS, []byte(config), rule.Priority, rule.FreezeDays, rule.RefundPolicy,
		rule.EffectiveStartAt, rule.EffectiveEndAt, rule.Version, rule.Status, rule.CreatedAt, rule.UpdatedAt)
	return err
}

type commissionRuleScanner interface {
	Scan(...any) error
}

func scanCommissionRule(scanner commissionRuleScanner) (commissionapp.CommissionRule, error) {
	var item commissionapp.CommissionRule
	var config []byte
	var effectiveEnd sql.NullTime
	err := scanner.Scan(
		&item.ID, &item.TenantID, &item.Code, &item.Name, &item.ProductType, &item.ProductID,
		&item.BeneficiaryRole, &item.RelationshipLevel, &item.CalculationType, &item.FixedAmountCents,
		&item.PercentageBPS, &config, &item.Priority, &item.FreezeDays, &item.RefundPolicy,
		&item.EffectiveStartAt, &effectiveEnd, &item.Version, &item.Status, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return commissionapp.CommissionRule{}, err
	}
	item.CalculationConfig = append(json.RawMessage(nil), config...)
	if effectiveEnd.Valid {
		item.EffectiveEndAt = &effectiveEnd.Time
	}
	return item, nil
}

func loadEffectiveCommissionRulesTx(ctx context.Context, tx *sql.Tx, tenantID, productType, productID string, paidAt time.Time) ([]commissionapp.CommissionRule, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, tenant_id, rule_code, rule_name, product_type, coalesce(product_id, ''),
		       beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
		       percentage_bps, calculation_config, priority, freeze_days, refund_policy,
		       effective_start_at, effective_end_at, version, status, created_at, updated_at
		FROM (
		  SELECT DISTINCT ON (rule_code) *
		  FROM xz_commission_rules
		  WHERE tenant_id IN ($1, 'tenant_default') AND product_type=$2
		    AND (product_id=$3 OR product_id IS NULL OR product_id='')
		    AND status='ACTIVE' AND effective_start_at <= $4
		    AND (effective_end_at IS NULL OR effective_end_at > $4)
		  ORDER BY rule_code, CASE WHEN tenant_id=$1 THEN 0 ELSE 1 END,
		           CASE WHEN product_id=$3 THEN 0 ELSE 1 END, version DESC
		) effective_rules
		ORDER BY priority, rule_code, version DESC
	`, tenantID, productType, productID, paidAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]commissionapp.CommissionRule, 0)
	for rows.Next() {
		item, err := scanCommissionRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func generateCommissionRecordsForCommerceOrderTx(ctx context.Context, tx *sql.Tx, order adminOrder, plan adminPlan, commerceCtx commissionOrderContext) (commissionapp.CalculationResult, error) {
	paidAt, err := parseCommissionPaidAt(nowForOrder(order))
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	tenantID := firstNonEmptyString(strings.TrimSpace(order.TenantID), "tenant_default")
	productType := planBusinessType(plan)
	rules, err := loadEffectiveCommissionRulesTx(ctx, tx, tenantID, productType, plan.ID, paidAt)
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	agentIDs := map[int]string{}
	if commerceCtx.DirectAgentID != "" {
		agentIDs[1] = commerceCtx.DirectAgentID
	}
	if commerceCtx.ParentAgentID != "" {
		agentIDs[2] = commerceCtx.ParentAgentID
	}
	quantity := int64(intValue(order.PriceSnapshot["quantity"]))
	if quantity <= 0 {
		quantity = 1
	}
	result, err := commissionapp.NewEngine().Calculate(commissionapp.CalculationInput{
		TenantID: tenantID, OrderID: order.ID, OrderNo: firstNonEmptyString(order.OrderNo, order.ID),
		ProductType: productType, ProductID: plan.ID, SourceUserID: order.UserID,
		OrderAmountCents: commissionapp.AmountCents(orderAmount(order)), PaidAmountCents: commissionapp.AmountCents(orderAmount(order)),
		Quantity: quantity, PaidAt: paidAt, Rules: rules,
		Relationships: commissionapp.RelationshipSnapshot{
			AgentIDsByLevel: agentIDs, OperationCenterID: commerceCtx.OperationCenterID, PlatformID: "platform:" + tenantID,
		},
	})
	if err != nil {
		return commissionapp.CalculationResult{}, err
	}
	rulesByID := make(map[string]commissionapp.CommissionRule, len(rules))
	for _, rule := range rules {
		rulesByID[rule.ID] = rule
	}
	for _, record := range result.Records {
		if err := insertImmutableCommissionRecordTx(ctx, tx, record, rulesByID[record.RuleID], plan); err != nil {
			return commissionapp.CalculationResult{}, err
		}
	}
	return result, nil
}

func insertImmutableCommissionRecordTx(ctx context.Context, tx *sql.Tx, record commissionapp.CommissionRecord, rule commissionapp.CommissionRule, plan adminPlan) error {
	if err := record.Validate(); err != nil {
		return err
	}
	metadata := map[string]any{
		"cashOnly": true, "ruleCode": rule.Code, "calculationType": rule.CalculationType,
		"productType": rule.ProductType, "productId": plan.ID, "refundPolicy": rule.RefundPolicy,
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO xz_commission_records (
		  id,tenant_id,order_id,order_no,beneficiary_type,beneficiary_id,source_user_id,
		  rule_id,rule_version,amount_cents,currency,record_type,status,freeze_until,
		  available_at,reversal_of_id,idempotency_key,metadata,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,nullif($16,''),$17,$18::jsonb,$19,$20)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, record.ID, record.TenantID, record.OrderID, record.OrderNo, record.BeneficiaryType,
		record.BeneficiaryID, record.SourceUserID, record.RuleID, record.RuleVersion,
		record.AmountCents, record.Currency, record.RecordType, record.Status, record.FreezeUntil,
		record.AvailableAt, record.ReversalOfID, record.IdempotencyKey, jsonProjection(metadata),
		record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	var existing commissionapp.CommissionRecord
	err = tx.QueryRowContext(ctx, `
		SELECT order_id,rule_id,rule_version,beneficiary_type,beneficiary_id,amount_cents,record_type
		FROM xz_commission_records WHERE idempotency_key=$1
	`, record.IdempotencyKey).Scan(&existing.OrderID, &existing.RuleID, &existing.RuleVersion, &existing.BeneficiaryType,
		&existing.BeneficiaryID, &existing.AmountCents, &existing.RecordType)
	if err != nil {
		return err
	}
	if existing.OrderID != record.OrderID || existing.RuleID != record.RuleID || existing.RuleVersion != record.RuleVersion ||
		existing.BeneficiaryType != record.BeneficiaryType || existing.BeneficiaryID != record.BeneficiaryID ||
		existing.AmountCents != record.AmountCents || existing.RecordType != record.RecordType {
		return errors.New("commission idempotency key conflicts with different immutable data")
	}
	return nil
}

func compatibilitySettlementResult(ctx commissionOrderContext, result commissionapp.CalculationResult) (commissionSettlementResult, error) {
	legacy := commissionSettlementResult{
		OrderType: ctx.OrderType, TokenGrantAmount: ctx.TokenGrantAmount,
		TokenGrantValueCents: ctx.TokenGrantValueCents,
	}
	for _, record := range result.Records {
		amount, err := commissionAmountToInt(record.AmountCents)
		if err != nil {
			return commissionSettlementResult{}, err
		}
		switch record.BeneficiaryType {
		case commissionapp.BeneficiaryAgent:
			if record.BeneficiaryID == ctx.DirectAgentID {
				legacy.DirectAgentRewardCents += amount
			} else if record.BeneficiaryID == ctx.ParentAgentID {
				legacy.ParentAgentRewardCents += amount
			}
		case commissionapp.BeneficiaryOperationCenter:
			legacy.OperationCenterRewardCents += amount
		case commissionapp.BeneficiaryPlatform:
			legacy.PlatformIncomeCents += amount
		}
	}
	return legacy, validateSettlementAmount(ctx.AmountCents, legacy)
}

func commissionAmountToInt(amount commissionapp.AmountCents) (int, error) {
	value := int(amount)
	if commissionapp.AmountCents(value) != amount {
		return 0, errors.New("commission amount exceeds platform integer range")
	}
	return value, nil
}

func parseCommissionPaidAt(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid commission paid time: %w", err)
	}
	return parsed.UTC(), nil
}

func newCommissionRuleID(code string, version int) string {
	seed := fmt.Sprintf("%s|%d|%d", code, version, time.Now().UTC().UnixNano())
	return "commission_rule_" + shortID(seed)
}
