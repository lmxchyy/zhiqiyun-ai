package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *postgresStore) GetAdminIdentityProfile(userID string) (adminIdentityProfile, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminIdentityProfile{}, err
	}
	var profile adminIdentityProfile
	profile.UserID = userID
	if err := s.db.QueryRowContext(ctx, `
		SELECT status,coalesce(role,'') FROM xz_users WHERE id=$1
	`, userID).Scan(&profile.AccountStatus, &profile.LegacyRole); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminIdentityProfile{}, errIdentityUserNotFound
		}
		return adminIdentityProfile{}, err
	}
	roles, err := s.adminIdentityAccountRoles(ctx, userID)
	if err != nil {
		return adminIdentityProfile{}, err
	}
	identities, err := s.adminBusinessIdentityRows(ctx, userID, true)
	if err != nil {
		return adminIdentityProfile{}, err
	}
	profile.AccountRoles = roles
	profile.Identities = identities
	profile.PrimaryIdentity = primaryBusinessIdentity(identities)
	_ = s.db.QueryRowContext(ctx, `SELECT coalesce(current_role_code,'USER') FROM xz_user_role_context WHERE user_id=$1`, userID).Scan(&profile.CurrentRole)
	_ = s.db.QueryRowContext(ctx, `SELECT coalesce((SELECT upper(status) FROM xz_channel_agents WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1),'')`, userID).Scan(&profile.AgentProfileStatus)
	_ = s.db.QueryRowContext(ctx, `SELECT coalesce((SELECT upper(status) FROM xz_operation_centers WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1),'')`, userID).Scan(&profile.OperationCenterProfileStatus)
	return profile, nil
}

func (s *postgresStore) GetAdminIdentityHistory(userID string) (adminIdentityHistory, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminIdentityHistory{}, err
	}
	if err := ensureIdentityQueryUser(ctx, s.db, userID); err != nil {
		return adminIdentityHistory{}, err
	}
	identities, err := s.adminBusinessIdentityRows(ctx, userID, false)
	if err != nil {
		return adminIdentityHistory{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,tenant_id,user_id,old_identity,new_identity,change_type,source_type,
		       coalesce(source_order_id,''),coalesce(old_parent_agent_id,''),coalesce(new_parent_agent_id,''),
		       coalesce(old_operation_center_id,''),coalesce(new_operation_center_id,''),
		       reason,remark,operator_id,request_id,created_at
		FROM xz_identity_change_records
		WHERE user_id=$1
		ORDER BY created_at DESC,id DESC
	`, userID)
	if err != nil {
		return adminIdentityHistory{}, err
	}
	defer rows.Close()
	changes := make([]adminIdentityChangeRecord, 0)
	for rows.Next() {
		var item adminIdentityChangeRecord
		var oldRaw, newRaw []byte
		var createdAt time.Time
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.UserID, &oldRaw, &newRaw, &item.ChangeType, &item.SourceType,
			&item.SourceOrderID, &item.OldParentAgentID, &item.NewParentAgentID,
			&item.OldOperationCenterID, &item.NewOperationCenterID,
			&item.Reason, &item.Remark, &item.OperatorID, &item.RequestID, &createdAt,
		); err != nil {
			return adminIdentityHistory{}, err
		}
		item.OldIdentity = decodeIdentitySnapshot(oldRaw)
		item.NewIdentity = decodeIdentitySnapshot(newRaw)
		item.CreatedAt = formatIdentityQueryTime(createdAt)
		changes = append(changes, item)
	}
	if err := rows.Err(); err != nil {
		return adminIdentityHistory{}, err
	}
	return adminIdentityHistory{UserID: userID, Identities: identities, ChangeRecords: changes}, nil
}

func (s *postgresStore) GetAdminCurrentRelationship(userID string) (*adminUserRelationship, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	if err := ensureIdentityQueryUser(ctx, s.db, userID); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, adminRelationshipSelect+`
		WHERE relation.user_id=$1 AND relation.status='ACTIVE' AND relation.ended_at IS NULL
		ORDER BY relation.effective_at DESC,relation.created_at DESC LIMIT 1
	`, userID)
	item, err := scanAdminRelationship(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *postgresStore) GetAdminRelationshipHistory(userID string) ([]adminUserRelationship, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return nil, err
	}
	if err := ensureIdentityQueryUser(ctx, s.db, userID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, adminRelationshipSelect+`
		WHERE relation.user_id=$1
		ORDER BY relation.effective_at DESC,relation.created_at DESC,relation.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]adminUserRelationship, 0)
	for rows.Next() {
		item, err := scanAdminRelationship(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) GetAdminIdentityFinancialOverview(userID string) (adminIdentityFinancialOverview, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return adminIdentityFinancialOverview{}, err
	}
	var planID, memberLevel, subscriptionExpiresAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT coalesce(plan_id,''),coalesce(member_level,''),coalesce(subscription_expires_at,'')
		FROM xz_users WHERE id=$1
	`, userID).Scan(&planID, &memberLevel, &subscriptionExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminIdentityFinancialOverview{}, errIdentityUserNotFound
		}
		return adminIdentityFinancialOverview{}, err
	}
	overview := adminIdentityFinancialOverview{
		UserID:     userID,
		Membership: map[string]any{"level": memberLevel, "planId": planID, "expiresAt": subscriptionExpiresAt, "entitlementRecordCount": int64(0)},
		Wallet:     map[string]any{"pointsAvailable": int64(0), "pointsFrozen": int64(0), "tokenBalance": int64(0), "cashBalanceCents": int64(0), "frozenToken": int64(0)},
		Token:      map[string]any{"recordCount": int64(0), "granted": int64(0), "consumed": int64(0)},
		Commission: map[string]any{"recordCount": int64(0), "totalCents": int64(0), "expectedCents": int64(0), "frozenCents": int64(0), "availableCents": int64(0), "settlingCents": int64(0), "settledCents": int64(0), "legacyRecordCount": int64(0), "legacyTotalCents": int64(0)},
	}

	var entitlementCount int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_membership_entitlement_records WHERE user_id=$1`, userID).Scan(&entitlementCount); err != nil {
		return adminIdentityFinancialOverview{}, err
	}
	overview.Membership["entitlementRecordCount"] = entitlementCount
	var entitlementLevel, entitlementOrder string
	var entitlementEffective, entitlementExpires time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT member_level,source_order_no,effective_at,expires_at
		FROM xz_membership_entitlement_records WHERE user_id=$1
		ORDER BY effective_at DESC,created_at DESC LIMIT 1
	`, userID).Scan(&entitlementLevel, &entitlementOrder, &entitlementEffective, &entitlementExpires)
	if err == nil {
		overview.Membership["latestEntitlement"] = map[string]any{
			"level": entitlementLevel, "sourceOrderNo": entitlementOrder,
			"effectiveAt": formatIdentityQueryTime(entitlementEffective), "expiresAt": formatIdentityQueryTime(entitlementExpires),
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return adminIdentityFinancialOverview{}, err
	}

	var pointsAvailable, pointsFrozen int64
	err = s.db.QueryRowContext(ctx, `SELECT available,frozen FROM xz_point_accounts WHERE user_id=$1`, userID).Scan(&pointsAvailable, &pointsFrozen)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return adminIdentityFinancialOverview{}, err
	}
	overview.Wallet["pointsAvailable"], overview.Wallet["pointsFrozen"] = pointsAvailable, pointsFrozen
	var tokenBalance, cashBalance, frozenToken, totalGranted, totalUsed int64
	err = s.db.QueryRowContext(ctx, `
		SELECT token_balance,cash_balance_cents,frozen_token,total_token_granted,total_token_used
		FROM xz_user_wallets WHERE user_id=$1
	`, userID).Scan(&tokenBalance, &cashBalance, &frozenToken, &totalGranted, &totalUsed)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return adminIdentityFinancialOverview{}, err
	}
	overview.Wallet["tokenBalance"], overview.Wallet["cashBalanceCents"], overview.Wallet["frozenToken"] = tokenBalance, cashBalance, frozenToken
	overview.Wallet["totalTokenGranted"], overview.Wallet["totalTokenUsed"] = totalGranted, totalUsed

	var tokenRecords, granted, consumed int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*),coalesce(sum(amount) FILTER (WHERE amount > 0),0),coalesce(-sum(amount) FILTER (WHERE amount < 0),0)
		FROM xz_token_records WHERE user_id=$1
	`, userID).Scan(&tokenRecords, &granted, &consumed); err != nil {
		return adminIdentityFinancialOverview{}, err
	}
	overview.Token["recordCount"], overview.Token["granted"], overview.Token["consumed"] = tokenRecords, granted, consumed

	var commissionRecords, commissionTotal, expected, frozen, available, settling, settled int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*),coalesce(sum(record.amount_cents),0),
		       coalesce(sum(record.amount_cents) FILTER (WHERE record.status='EXPECTED'),0),
		       coalesce(sum(record.amount_cents) FILTER (WHERE record.status='FROZEN'),0),
		       coalesce(sum(record.amount_cents) FILTER (WHERE record.status='AVAILABLE'),0),
		       coalesce(sum(record.amount_cents) FILTER (WHERE record.status='SETTLING'),0),
		       coalesce(sum(record.amount_cents) FILTER (WHERE record.status='SETTLED'),0)
		FROM xz_commission_records record
		WHERE record.beneficiary_id IN (
		  SELECT id FROM xz_channel_agents WHERE user_id=$1
		  UNION SELECT id FROM xz_operation_centers WHERE user_id=$1
		)
	`, userID).Scan(&commissionRecords, &commissionTotal, &expected, &frozen, &available, &settling, &settled); err != nil {
		return adminIdentityFinancialOverview{}, err
	}
	overview.Commission["recordCount"], overview.Commission["totalCents"] = commissionRecords, commissionTotal
	overview.Commission["expectedCents"], overview.Commission["frozenCents"] = expected, frozen
	overview.Commission["availableCents"], overview.Commission["settlingCents"], overview.Commission["settledCents"] = available, settling, settled
	var legacyCount, legacyTotal int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*),coalesce(sum(commission.amount_cents),0)
		FROM xz_commissions commission
		WHERE commission.agent_id IN (SELECT id FROM xz_channel_agents WHERE user_id=$1)
		   OR commission.receiver_id IN (
		     SELECT id FROM xz_channel_agents WHERE user_id=$1
		     UNION SELECT id FROM xz_operation_centers WHERE user_id=$1
		   )
	`, userID).Scan(&legacyCount, &legacyTotal); err != nil {
		return adminIdentityFinancialOverview{}, err
	}
	overview.Commission["legacyRecordCount"], overview.Commission["legacyTotalCents"] = legacyCount, legacyTotal
	return overview, nil
}

func (s *postgresStore) adminIdentityAccountRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT role FROM xz_user_roles
		WHERE user_id=$1 AND upper(status)='ACTIVE'
		ORDER BY role
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]string, 0)
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *postgresStore) adminBusinessIdentityRows(ctx context.Context, userID string, currentOnly bool) ([]adminBusinessIdentity, error) {
	query := `
		SELECT id,tenant_id,user_id,identity_type,identity_status,commission_enabled,
		       source_type,coalesce(source_order_id,''),effective_at,expires_at,ended_at,
		       status_reason,identity_version,created_by,created_at,updated_at
		FROM xz_user_business_identities WHERE user_id=$1`
	if currentOnly {
		query += ` AND identity_status IN ('PENDING','ACTIVE','FROZEN') AND ended_at IS NULL`
	}
	query += ` ORDER BY effective_at DESC,created_at DESC,id DESC`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]adminBusinessIdentity, 0)
	for rows.Next() {
		var item adminBusinessIdentity
		var effectiveAt, createdAt, updatedAt time.Time
		var expiresAt, endedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.TenantID, &item.UserID, &item.IdentityType, &item.IdentityStatus, &item.CommissionEnabled,
			&item.SourceType, &item.SourceOrderID, &effectiveAt, &expiresAt, &endedAt,
			&item.StatusReason, &item.IdentityVersion, &item.CreatedBy, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		item.EffectiveAt, item.CreatedAt, item.UpdatedAt = formatIdentityQueryTime(effectiveAt), formatIdentityQueryTime(createdAt), formatIdentityQueryTime(updatedAt)
		item.ExpiresAt, item.EndedAt = formatIdentityQueryNullTime(expiresAt), formatIdentityQueryNullTime(endedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

const adminRelationshipSelect = `
	SELECT relation.id,relation.tenant_id,relation.user_id,
	       coalesce(relation.parent_agent_id,''),coalesce(parent_agent.user_id,''),coalesce(parent_user.name,''),
	       coalesce(relation.operation_center_id,''),coalesce(center.name,''),
	       relation.effective_at,relation.ended_at,relation.status,relation.source_type,relation.created_by,
	       relation.created_at,relation.updated_at
	FROM xz_user_relationships relation
	LEFT JOIN xz_channel_agents parent_agent ON parent_agent.id=relation.parent_agent_id
	LEFT JOIN xz_users parent_user ON parent_user.id=parent_agent.user_id
	LEFT JOIN xz_operation_centers center ON center.id=relation.operation_center_id
`

type identityRowScanner interface {
	Scan(dest ...any) error
}

func scanAdminRelationship(scanner identityRowScanner) (adminUserRelationship, error) {
	var item adminUserRelationship
	var effectiveAt, createdAt, updatedAt time.Time
	var endedAt sql.NullTime
	err := scanner.Scan(
		&item.ID, &item.TenantID, &item.UserID,
		&item.ParentAgentID, &item.ParentAgentUserID, &item.ParentAgentName,
		&item.OperationCenterID, &item.OperationCenterName,
		&effectiveAt, &endedAt, &item.Status, &item.SourceType, &item.CreatedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		return adminUserRelationship{}, err
	}
	item.EffectiveAt, item.EndedAt = formatIdentityQueryTime(effectiveAt), formatIdentityQueryNullTime(endedAt)
	item.CreatedAt, item.UpdatedAt = formatIdentityQueryTime(createdAt), formatIdentityQueryTime(updatedAt)
	return item, nil
}

func ensureIdentityQueryUser(ctx context.Context, db *sql.DB, userID string) error {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT exists(SELECT 1 FROM xz_users WHERE id=$1)`, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errIdentityUserNotFound
	}
	return nil
}

func formatIdentityQueryTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatIdentityQueryNullTime(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return formatIdentityQueryTime(value.Time)
}
