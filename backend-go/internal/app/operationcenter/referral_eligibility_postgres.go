package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type PostgresReferralEligibilityTrigger struct{}

func NewPostgresReferralEligibilityTrigger() PostgresReferralEligibilityTrigger {
	return PostgresReferralEligibilityTrigger{}
}

func (PostgresReferralEligibilityTrigger) MarkEligible(ctx context.Context, tx *sql.Tx, serviceOrder *OperationCenterServiceOrder) error {
	if tx == nil {
		return ErrTransactionRequired
	}
	if serviceOrder == nil || serviceOrder.Status != OperationCenterServiceActive || serviceOrder.CommercialRuleSetID == nil || serviceOrder.CommercialRuleSetVersion == nil {
		return ErrConstraintViolation
	}
	relationship, err := parseReferralRelationshipSnapshot(serviceOrder.RelationshipSnapshot)
	if errors.Is(err, ErrNoReferralRelationship) {
		return nil
	}
	if err != nil {
		return err
	}
	rules, err := loadPublishedEligibilityRules(ctx, tx, serviceOrder, relationship.ReferrerType)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return ErrNoPublishedReferralRules
	}
	beneficiaries, err := resolveReferralBeneficiaries(relationship, rules)
	if err != nil {
		return err
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return err
	}
	event := ReferralEvent{
		ID: stableWorkflowID("referral_event", serviceOrder.OrderID), TenantID: serviceOrder.TenantID,
		ReferredOperationCenterUserID: serviceOrder.ApplicantUserID,
		ReferrerType:                  relationship.ReferrerType, ReferrerUserID: relationship.ReferrerUserID,
		ReferrerOperationCenterUserID: optionalString(relationship.ReferrerOperationCenterUserID),
		SourceOrderID:                 serviceOrder.OrderID, SourceOrderNo: serviceOrder.OrderNo,
		PaymentStatusSnapshot: snapshotStringValue(serviceOrder.Metadata, "paymentStatus"),
		ReviewStatusSnapshot:  string(ReviewApproved), OperationCenterStatusSnapshot: string(OperationCenterServiceActive),
		RelationshipSnapshot: serviceOrder.RelationshipSnapshot, TriggeredAt: now, Status: "READY",
		IdempotencyKey: stableWorkflowID("referral_event_idempotency", serviceOrder.OrderID), CreatedAt: now, UpdatedAt: now,
	}
	if event.PaymentStatusSnapshot == "" {
		return ErrFrozenSnapshotMissing
	}
	if err := insertReferralEvent(ctx, tx, event); err != nil {
		return err
	}
	for _, resolved := range beneficiaries {
		eligibility := ReferralEligibility{
			ID:       stableWorkflowID("referral_eligibility", event.ID, resolved.Rule.ID, resolved.BeneficiaryUserID),
			TenantID: serviceOrder.TenantID, ReferralEventID: event.ID,
			CommercialRuleSetID:      *serviceOrder.CommercialRuleSetID,
			CommercialRuleSetVersion: *serviceOrder.CommercialRuleSetVersion,
			ReferralRuleVersionID:    resolved.Rule.ID, ReferralRuleVersion: resolved.Rule.Version,
			BeneficiaryType: resolved.Rule.BeneficiaryType, BeneficiaryUserID: resolved.BeneficiaryUserID,
			BeneficiaryRelation:  resolved.Rule.BeneficiaryRelation,
			RelationshipSnapshot: serviceOrder.RelationshipSnapshot, Status: ReferralEligibilityEligible,
			IdempotencyKey: stableWorkflowID("referral_eligibility_idempotency", event.ID, resolved.Rule.ID, resolved.BeneficiaryUserID),
			CreatedAt:      now, UpdatedAt: now,
		}
		if err := insertReferralEligibility(ctx, tx, eligibility); err != nil {
			return err
		}
	}
	return nil
}

func loadPublishedEligibilityRules(ctx context.Context, tx *sql.Tx, serviceOrder *OperationCenterServiceOrder, referrerType ReferralReferrerType) ([]ReferralEligibilityRule, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT rule.id,rule.rule_set_id,rules.version,rule.version,
		       rule.referrer_type,rule.beneficiary_type,rule.beneficiary_relation
		FROM xz_referral_reward_rule_versions rule
		JOIN xz_commercial_rule_sets rules ON rules.id=rule.rule_set_id
		WHERE rule.rule_set_id=$1 AND rules.version=$2
		  AND rules.status='PUBLISHED' AND rule.status='PUBLISHED'
		  AND rule.referrer_type=$3
		ORDER BY rule.rule_code,rule.id
	`, *serviceOrder.CommercialRuleSetID, *serviceOrder.CommercialRuleSetVersion, referrerType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReferralEligibilityRule
	for rows.Next() {
		var rule ReferralEligibilityRule
		if err := rows.Scan(&rule.ID, &rule.RuleSetID, &rule.RuleSetVersion, &rule.Version, &rule.ReferrerType, &rule.BeneficiaryType, &rule.BeneficiaryRelation); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func insertReferralEvent(ctx context.Context, tx *sql.Tx, event ReferralEvent) error {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO xz_referral_events(
		  id,tenant_id,referred_operation_center_user_id,referrer_type,referrer_user_id,
		  referrer_operation_center_user_id,source_order_id,source_order_no,
		  payment_status_snapshot,review_status_snapshot,operation_center_status_snapshot,
		  relationship_snapshot,triggered_at,status,idempotency_key,created_at,updated_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT(source_order_id) DO NOTHING
	`, event.ID, event.TenantID, event.ReferredOperationCenterUserID, event.ReferrerType,
		event.ReferrerUserID, event.ReferrerOperationCenterUserID, event.SourceOrderID, event.SourceOrderNo,
		event.PaymentStatusSnapshot, event.ReviewStatusSnapshot, event.OperationCenterStatusSnapshot,
		event.RelationshipSnapshot, event.TriggeredAt, event.Status, event.IdempotencyKey, event.CreatedAt, event.UpdatedAt)
	if err != nil {
		return mapPostgresStoreError("create referral event", err)
	}
	if count, countErr := result.RowsAffected(); countErr != nil {
		return countErr
	} else if count == 0 {
		var existingID, existingReferrerID string
		if err := tx.QueryRowContext(ctx, `SELECT id,referrer_user_id FROM xz_referral_events WHERE source_order_id=$1 FOR UPDATE`, event.SourceOrderID).Scan(&existingID, &existingReferrerID); err != nil {
			return err
		}
		if existingID != event.ID || existingReferrerID != event.ReferrerUserID {
			return fmt.Errorf("referral event idempotency identity mismatch: %w", ErrIdempotencyConflict)
		}
	}
	return nil
}

func insertReferralEligibility(ctx context.Context, tx *sql.Tx, eligibility ReferralEligibility) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_referral_eligibilities(
		  id,tenant_id,referral_event_id,commercial_rule_set_id,commercial_rule_set_version,
		  referral_rule_version_id,referral_rule_version,beneficiary_type,beneficiary_user_id,
		  beneficiary_relation,relationship_snapshot,eligibility_status,idempotency_key,created_at,updated_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT(idempotency_key) DO NOTHING
	`, eligibility.ID, eligibility.TenantID, eligibility.ReferralEventID,
		eligibility.CommercialRuleSetID, eligibility.CommercialRuleSetVersion,
		eligibility.ReferralRuleVersionID, eligibility.ReferralRuleVersion,
		eligibility.BeneficiaryType, eligibility.BeneficiaryUserID, eligibility.BeneficiaryRelation,
		eligibility.RelationshipSnapshot, eligibility.Status, eligibility.IdempotencyKey,
		eligibility.CreatedAt, eligibility.UpdatedAt)
	return mapPostgresStoreError("create referral eligibility", err)
}
