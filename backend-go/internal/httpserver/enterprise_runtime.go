package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const permissionEnterpriseAIUse = "enterprise.ai.use"

var errEnterpriseServiceUnavailable = errors.New("enterprise service is unavailable")

type modelCallAuthorization struct {
	ContextType      string
	TenantID         string
	OrganizationID   string
	UserID           string
	MemberID         string
	Role             string
	BillingScope     string
	BillingAccountID string
	ServiceState     string
}

type modelCallAuthorizer interface {
	AuthorizeModelCall(userID string, capability string) (modelCallAuthorization, error)
}

type connectorModelCallAuthorizer interface {
	AuthorizeConnectorModelCall(userID string, tenantID string, capability string) (modelCallAuthorization, error)
}

func authorizeUserModelCall(store platformStore, userID string, capability string) (modelCallAuthorization, error) {
	if authorizer, ok := store.(modelCallAuthorizer); ok {
		return authorizer.AuthorizeModelCall(userID, capability)
	}
	return modelCallAuthorization{
		ContextType: contextPersonal, TenantID: "tenant_default", UserID: userID,
		BillingScope: contextPersonal, BillingAccountID: userID, ServiceState: "ACTIVE",
	}, nil
}

func writeModelAuthorizationError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, errForbidden), errors.Is(err, errEnterpriseServiceUnavailable):
		status = http.StatusForbidden
	}
	writeError(w, status, err)
}

type enterpriseComputeLedgerEntry struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenantId"`
	EntryType        string         `json:"entryType"`
	SourceType       string         `json:"sourceType"`
	ComputeUnitDelta int64          `json:"computeUnitDelta"`
	BalanceBefore    int64          `json:"balanceBefore"`
	BalanceAfter     int64          `json:"balanceAfter"`
	AmountCents      int64          `json:"amountCents"`
	ReferenceType    string         `json:"referenceType"`
	ReferenceID      string         `json:"referenceId"`
	RequestID        string         `json:"requestId"`
	Status           string         `json:"status"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        string         `json:"createdAt"`
}

type enterpriseComputeStore interface {
	EnterpriseComputeAccount(access enterpriseAccess, limit int) (enterpriseWalletSummary, []enterpriseComputeLedgerEntry, error)
}

type sqlQueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *postgresStore) AuthorizeModelCall(userID string, capability string) (modelCallAuthorization, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return modelCallAuthorization{}, err
	}
	return s.authorizeModelCallContext(ctx, s.db, userID, capability)
}

func (s *postgresStore) AuthorizeConnectorModelCall(userID string, tenantID string, capability string) (modelCallAuthorization, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return modelCallAuthorization{}, err
	}
	return s.authorizeConnectorModelCallContext(ctx, s.db, userID, tenantID, capability)
}

func (s *postgresStore) authorizeConnectorModelCallContext(ctx context.Context, db sqlQueryRower, userID string, tenantID string, capability string) (modelCallAuthorization, error) {
	userID = strings.TrimSpace(userID)
	tenantID = strings.TrimSpace(tenantID)
	if userID == "" || tenantID == "" || strings.EqualFold(tenantID, "tenant_default") {
		return modelCallAuthorization{}, errUnauthorized
	}
	var organizationID, currentRole string
	err := db.QueryRowContext(ctx, `
		SELECT role.organization_id, role.role
		FROM xz_user_roles role
		JOIN xz_tenant_members member
		  ON member.tenant_id=role.tenant_id AND member.user_id=role.user_id
		JOIN xz_organizations organization
		  ON organization.tenant_id=role.tenant_id AND organization.id=role.organization_id
		JOIN xz_role_permissions permission
		  ON permission.role=role.role AND permission.permission=$3
		WHERE role.user_id=$1 AND role.tenant_id=$2
		  AND upper(role.status)='ACTIVE'
		  AND upper(member.status)='ACTIVE' AND upper(member.member_status)='ACTIVE'
		  AND upper(organization.status)='ACTIVE'
		ORDER BY CASE WHEN member.primary_organization_id=role.organization_id THEN 0 ELSE 1 END,
		         role.organization_id, role.role
		LIMIT 1
	`, userID, tenantID, permissionEnterpriseAIUse).Scan(&organizationID, &currentRole)
	if errors.Is(err, sql.ErrNoRows) {
		return modelCallAuthorization{}, errForbidden
	}
	if err != nil {
		return modelCallAuthorization{}, err
	}
	return s.authorizeEnterpriseModelCallContext(ctx, db, userID, tenantID, organizationID, currentRole, capability)
}

func (s *postgresStore) authorizeModelCallContext(ctx context.Context, db sqlQueryRower, userID string, capability string) (modelCallAuthorization, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return modelCallAuthorization{}, errUnauthorized
	}
	var contextType, tenantID, organizationID, currentRole string
	err := db.QueryRowContext(ctx, `
		SELECT context_type, tenant_id, organization_id, current_role_code
		FROM xz_user_role_context
		WHERE user_id=$1
	`, userID).Scan(&contextType, &tenantID, &organizationID, &currentRole)
	if errors.Is(err, sql.ErrNoRows) || !strings.EqualFold(contextType, contextEnterprise) {
		return modelCallAuthorization{
			ContextType: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"),
			UserID: userID, Role: roleUser, BillingScope: contextPersonal, BillingAccountID: userID, ServiceState: "ACTIVE",
		}, nil
	}
	if err != nil {
		return modelCallAuthorization{}, err
	}

	return s.authorizeEnterpriseModelCallContext(ctx, db, userID, tenantID, organizationID, currentRole, capability)
}

func (s *postgresStore) authorizeEnterpriseModelCallContext(ctx context.Context, db sqlQueryRower, userID string, tenantID string, organizationID string, currentRole string, capability string) (modelCallAuthorization, error) {
	var tenantStatus, memberID, memberStatus, organizationStatus, serviceState, serviceRecordStatus, subscriptionStatus string
	var trialExpiresAt sql.NullTime
	err := db.QueryRowContext(ctx, `
		SELECT tenant.status, member.id, member.member_status, organization.status,
		       coalesce(service.lifecycle_state, CASE WHEN upper(tenant.status)='ACTIVE' THEN 'ACTIVE' ELSE 'PAUSED' END),
		       coalesce(service.status, 'ACTIVE'),
		       coalesce(subscription.status, 'MISSING'), subscription.trial_expires_at
		FROM xz_tenants tenant
		JOIN xz_tenant_members member ON member.tenant_id=tenant.id AND member.user_id=$2
		JOIN xz_organizations organization ON organization.tenant_id=tenant.id AND organization.id=$3
		LEFT JOIN xz_tenant_service_states service ON service.tenant_id=tenant.id
		LEFT JOIN LATERAL (
			SELECT item.status, item.trial_expires_at
			FROM xz_tenant_subscriptions item
			WHERE item.tenant_id=tenant.id
			ORDER BY item.updated_at DESC, item.id DESC
			LIMIT 1
		) subscription ON true
		WHERE tenant.id=$1 AND tenant.tenant_type='ENTERPRISE'
	`, tenantID, userID, organizationID).Scan(
		&tenantStatus, &memberID, &memberStatus, &organizationStatus,
		&serviceState, &serviceRecordStatus, &subscriptionStatus, &trialExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return modelCallAuthorization{}, errForbidden
	}
	if err != nil {
		return modelCallAuthorization{}, err
	}
	if !strings.EqualFold(tenantStatus, "ACTIVE") || !strings.EqualFold(memberStatus, "ACTIVE") ||
		!strings.EqualFold(organizationStatus, "ACTIVE") || !strings.EqualFold(serviceRecordStatus, "ACTIVE") ||
		!strings.EqualFold(serviceState, "ACTIVE") {
		return modelCallAuthorization{}, fmt.Errorf("%w: tenant=%s lifecycle=%s", errEnterpriseServiceUnavailable, tenantID, serviceState)
	}
	if !strings.EqualFold(subscriptionStatus, "ACTIVE") && !strings.EqualFold(subscriptionStatus, "TRIALING") {
		return modelCallAuthorization{}, fmt.Errorf("%w: tenant=%s subscription=%s", errEnterpriseServiceUnavailable, tenantID, subscriptionStatus)
	}
	if strings.EqualFold(subscriptionStatus, "TRIALING") && trialExpiresAt.Valid && !trialExpiresAt.Time.After(time.Now().UTC()) {
		return modelCallAuthorization{}, fmt.Errorf("%w: tenant=%s subscription expired", errEnterpriseServiceUnavailable, tenantID)
	}
	var roleAllowed bool
	if err := db.QueryRowContext(ctx, `
		SELECT exists(
			SELECT 1
			FROM xz_user_roles role
			JOIN xz_role_permissions permission ON permission.role=role.role
			WHERE role.user_id=$1 AND role.tenant_id=$2 AND role.role=$3
			  AND upper(role.status)='ACTIVE' AND permission.permission=$4
		)
	`, userID, tenantID, normalizeAppRole(currentRole), permissionEnterpriseAIUse).Scan(&roleAllowed); err != nil {
		return modelCallAuthorization{}, err
	}
	if !roleAllowed {
		return modelCallAuthorization{}, errForbidden
	}
	return modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: tenantID, OrganizationID: organizationID,
		UserID: userID, MemberID: memberID, Role: normalizeAppRole(currentRole), BillingScope: contextEnterprise,
		BillingAccountID: tenantID, ServiceState: serviceState,
	}, nil
}

func (s *postgresStore) authorizationForStoredTaskTx(ctx context.Context, tx *sql.Tx, task generationTask, requireActiveService bool) (modelCallAuthorization, error) {
	if !strings.EqualFold(task.BillingAccountType, contextEnterprise) &&
		!strings.EqualFold(stringValue(task.Params["billing_scope"]), contextEnterprise) {
		return modelCallAuthorization{
			ContextType: contextPersonal, TenantID: "tenant_default", OrganizationID: defaultOrganizationID("tenant_default"),
			UserID: task.UserID, Role: roleUser, BillingScope: contextPersonal, BillingAccountID: task.UserID, ServiceState: "ACTIVE",
		}, nil
	}
	tenantID := firstNonEmptyString(task.TenantID, stringValue(task.Params["tenant_id"]))
	organizationID := firstNonEmptyString(task.OrganizationID, stringValue(task.Params["organization_id"]))
	role := firstNonEmptyString(stringValue(task.Params["authorized_role"]), roleEnterpriseMember)
	var memberID, tenantStatus, memberStatus, serviceState, serviceStatus string
	err := tx.QueryRowContext(ctx, `
		SELECT member.id,tenant.status,member.member_status,
		       coalesce(service.lifecycle_state,CASE WHEN upper(tenant.status)='ACTIVE' THEN 'ACTIVE' ELSE 'PAUSED' END),
		       coalesce(service.status,'ACTIVE')
		FROM xz_tenants tenant
		JOIN xz_tenant_members member ON member.tenant_id=tenant.id AND member.user_id=$2
		LEFT JOIN xz_tenant_service_states service ON service.tenant_id=tenant.id
		WHERE tenant.id=$1 AND tenant.tenant_type='ENTERPRISE'
	`, tenantID, task.UserID).Scan(&memberID, &tenantStatus, &memberStatus, &serviceState, &serviceStatus)
	if err != nil {
		return modelCallAuthorization{}, err
	}
	if !strings.EqualFold(memberStatus, "ACTIVE") {
		return modelCallAuthorization{}, errForbidden
	}
	if requireActiveService && (!strings.EqualFold(tenantStatus, "ACTIVE") || !strings.EqualFold(serviceState, "ACTIVE") || !strings.EqualFold(serviceStatus, "ACTIVE")) {
		return modelCallAuthorization{}, fmt.Errorf("%w: tenant=%s lifecycle=%s", errEnterpriseServiceUnavailable, tenantID, serviceState)
	}
	return modelCallAuthorization{
		ContextType: contextEnterprise, TenantID: tenantID, OrganizationID: organizationID, UserID: task.UserID,
		MemberID: memberID, Role: role, BillingScope: contextEnterprise, BillingAccountID: tenantID, ServiceState: serviceState,
	}, nil
}

func (s *postgresStore) currentTenantScopeForUser(ctx context.Context, userID string) (string, string, string, error) {
	var contextType, tenantID, organizationID string
	err := s.db.QueryRowContext(ctx, `SELECT context_type,tenant_id,organization_id FROM xz_user_role_context WHERE user_id=$1`, userID).
		Scan(&contextType, &tenantID, &organizationID)
	if errors.Is(err, sql.ErrNoRows) || !strings.EqualFold(contextType, contextEnterprise) {
		return contextPersonal, "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	var allowed bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT exists(
			SELECT 1 FROM xz_tenant_members member
			JOIN xz_tenants tenant ON tenant.id=member.tenant_id
			WHERE member.user_id=$1 AND member.tenant_id=$2
			  AND upper(member.member_status)='ACTIVE' AND upper(tenant.status)='ACTIVE'
		)
	`, userID, tenantID).Scan(&allowed); err != nil {
		return "", "", "", err
	}
	if !allowed {
		return "", "", "", errForbidden
	}
	return contextEnterprise, tenantID, organizationID, nil
}

type enterpriseComputeReservation struct {
	Authorization modelCallAuthorization
	LedgerID      string
	BalanceBefore int64
	BalanceAfter  int64
	Units         int64
}

func (s *postgresStore) reserveEnterpriseComputeTx(ctx context.Context, tx *sql.Tx, authorization modelCallAuthorization, units int64, referenceType string, referenceID string) (enterpriseComputeReservation, error) {
	reservation := enterpriseComputeReservation{Authorization: authorization, Units: units}
	if authorization.ContextType != contextEnterprise || units <= 0 {
		return reservation, nil
	}
	var balance int64
	var walletStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT point_balance,status FROM xz_tenant_wallets WHERE tenant_id=$1 FOR UPDATE
	`, authorization.TenantID).Scan(&balance, &walletStatus); err != nil {
		return reservation, err
	}
	if !strings.EqualFold(walletStatus, "ACTIVE") {
		return reservation, fmt.Errorf("%w: enterprise compute account is %s", errEnterpriseServiceUnavailable, walletStatus)
	}
	if balance < units {
		return reservation, fmt.Errorf("insufficient enterprise compute units: available %d, required %d", balance, units)
	}
	allocations, err := consumeEnterpriseCreditLotsTx(ctx, tx, authorization.TenantID, units)
	if err != nil {
		return reservation, err
	}
	after := balance - units
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_tenant_wallets
		SET point_balance=$2,version=version+1,updated_at=now()
		WHERE tenant_id=$1
	`, authorization.TenantID, after); err != nil {
		return reservation, err
	}
	ledgerID := newEnterpriseResourceID("compute_ledger")
	idempotencyKey := strings.ToLower(referenceType) + ":" + referenceID + ":debit"
	beforeValue := map[string]any{"balance": balance, "unit": "COMPUTE_UNIT"}
	afterValue := map[string]any{"balance": after, "unit": "COMPUTE_UNIT"}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_compute_ledger_entries(
			id,tenant_id,account_id,entry_type,source_type,compute_unit_delta,
			balance_before,balance_after,reference_type,reference_id,idempotency_key,
			actor_user_id,request_id,lot_allocations,before_value,after_value,status,metadata
		) VALUES($1,$2,$2,'DEBIT','MODEL_USAGE',$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb,$13::jsonb,'POSTED',$14::jsonb)
	`, ledgerID, authorization.TenantID, -units, balance, after, referenceType, referenceID, idempotencyKey,
		authorization.UserID, referenceID, jsonProjection(allocations), jsonProjection(beforeValue), jsonProjection(afterValue),
		jsonProjection(map[string]any{"organizationId": authorization.OrganizationID, "role": authorization.Role})); err != nil {
		return reservation, err
	}
	if err := insertTenantSafetyAuditTx(ctx, tx, authorization, "enterprise.compute.debit", "compute_ledger", ledgerID, referenceID, beforeValue, afterValue); err != nil {
		return reservation, err
	}
	reservation.LedgerID, reservation.BalanceBefore, reservation.BalanceAfter = ledgerID, balance, after
	return reservation, nil
}

func consumeEnterpriseCreditLotsTx(ctx context.Context, tx *sql.Tx, tenantID string, units int64) ([]map[string]any, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id,remaining_units
		FROM xz_compute_credit_lots
		WHERE tenant_id=$1 AND status='ACTIVE' AND remaining_units>0
		  AND (expires_at IS NULL OR expires_at>now())
		ORDER BY expires_at ASC NULLS LAST, created_at ASC, id ASC
		FOR UPDATE
	`, tenantID)
	if err != nil {
		return nil, err
	}
	type lotBalance struct {
		id        string
		remaining int64
	}
	lots := []lotBalance{}
	for rows.Next() {
		var lot lotBalance
		if err := rows.Scan(&lot.id, &lot.remaining); err != nil {
			rows.Close()
			return nil, err
		}
		lots = append(lots, lot)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	remaining := units
	allocations := []map[string]any{}
	for _, lot := range lots {
		if remaining == 0 {
			break
		}
		used := lot.remaining
		if used > remaining {
			used = remaining
		}
		next := lot.remaining - used
		status := "ACTIVE"
		if next == 0 {
			status = "EXHAUSTED"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_compute_credit_lots SET remaining_units=$2,status=$3,updated_at=now() WHERE tenant_id=$1 AND id=$4`, tenantID, next, status, lot.id); err != nil {
			return nil, err
		}
		allocations = append(allocations, map[string]any{"lotId": lot.id, "units": used})
		remaining -= used
	}
	if remaining != 0 {
		return nil, fmt.Errorf("enterprise compute credit lots are inconsistent: missing %d units", remaining)
	}
	return allocations, nil
}

func (s *postgresStore) reverseEnterpriseComputeTx(ctx context.Context, tx *sql.Tx, authorization modelCallAuthorization, units int64, referenceType string, referenceID string) error {
	if authorization.ContextType != contextEnterprise || units <= 0 {
		return nil
	}
	var balance int64
	if err := tx.QueryRowContext(ctx, `SELECT point_balance FROM xz_tenant_wallets WHERE tenant_id=$1 FOR UPDATE`, authorization.TenantID).Scan(&balance); err != nil {
		return err
	}
	after := balance + units
	if _, err := tx.ExecContext(ctx, `UPDATE xz_tenant_wallets SET point_balance=$2,version=version+1,updated_at=now() WHERE tenant_id=$1`, authorization.TenantID, after); err != nil {
		return err
	}
	lotID := newEnterpriseResourceID("compute_credit")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_compute_credit_lots(
			id,tenant_id,account_id,source_type,original_units,remaining_units,reference_type,reference_id,idempotency_key,status,metadata
		) VALUES($1,$2,$2,'REVERSAL',$3,$3,$4,$5,$6,'ACTIVE',$7::jsonb)
	`, lotID, authorization.TenantID, units, referenceType, referenceID,
		strings.ToLower(referenceType)+":"+referenceID+":reversal-lot", jsonProjection(map[string]any{"reason": "generation failed"})); err != nil {
		return err
	}
	ledgerID := newEnterpriseResourceID("compute_ledger")
	beforeValue := map[string]any{"balance": balance, "unit": "COMPUTE_UNIT"}
	afterValue := map[string]any{"balance": after, "unit": "COMPUTE_UNIT"}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_compute_ledger_entries(
			id,tenant_id,account_id,entry_type,source_type,compute_unit_delta,balance_before,balance_after,
			reference_type,reference_id,idempotency_key,actor_user_id,request_id,before_value,after_value,status,metadata
		) VALUES($1,$2,$2,'REVERSAL','REVERSAL',$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb,'POSTED',$13::jsonb)
	`, ledgerID, authorization.TenantID, units, balance, after, referenceType, referenceID,
		strings.ToLower(referenceType)+":"+referenceID+":reversal", authorization.UserID, referenceID,
		jsonProjection(beforeValue), jsonProjection(afterValue), jsonProjection(map[string]any{"creditLotId": lotID})); err != nil {
		return err
	}
	return insertTenantSafetyAuditTx(ctx, tx, authorization, "enterprise.compute.reverse", "compute_ledger", ledgerID, referenceID, beforeValue, afterValue)
}

func insertTenantSafetyAuditTx(ctx context.Context, tx *sql.Tx, authorization modelCallAuthorization, action string, resourceType string, resourceID string, requestID string, beforeValue map[string]any, afterValue map[string]any) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_tenant_audit_logs(
			id,tenant_id,actor_user_id,actor_role,organization_id,action,resource_type,resource_id,
			request_id,status,metadata,before_value,after_value,idempotency_key
		) VALUES($1,$2,nullif($3,''),$4,nullif($5,''),$6,$7,nullif($8,''),nullif($9,''),'SUCCEEDED',$10::jsonb,$11::jsonb,$12::jsonb,$13)
	`, newEnterpriseResourceID("tenant_audit"), authorization.TenantID, authorization.UserID, authorization.Role,
		authorization.OrganizationID, action, resourceType, resourceID, requestID,
		jsonProjection(map[string]any{"requestId": requestID}), jsonProjection(beforeValue), jsonProjection(afterValue),
		action+":"+requestID)
	return err
}

func (s *postgresStore) recordModelUsageTx(ctx context.Context, tx *sql.Tx, authorization modelCallAuthorization, task generationTask, params map[string]any) error {
	if authorization.ContextType != contextEnterprise {
		return nil
	}
	inputTokens := rawTokenCount(params, "input_tokens", "inputTokens")
	outputTokens := rawTokenCount(params, "output_tokens", "outputTokens")
	cachedTokens := rawTokenCount(params, "cached_input_tokens", "cachedInputTokens")
	reasoningTokens := rawTokenCount(params, "reasoning_tokens", "reasoningTokens")
	rawUsage := map[string]any{
		"inputTokens": inputTokens, "outputTokens": outputTokens,
		"cachedInputTokens": cachedTokens, "reasoningTokens": reasoningTokens,
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_model_usage_records(
			id,tenant_id,organization_id,user_id,task_id,provider_code,provider_request_id,model_code,capability,
			input_tokens,output_tokens,cached_input_tokens,reasoning_tokens,total_tokens,compute_units_charged,
			amount_cents_charged,idempotency_key,raw_usage,status,occurred_at
		) VALUES($1,$2,nullif($3,''),$4,$5,$6,nullif($7,''),$8,$9,$10,$11,$12,$13,$14,$15,0,$16,$17::jsonb,'SETTLED',now())
		ON CONFLICT (tenant_id,idempotency_key) DO NOTHING
	`, newEnterpriseResourceID("model_usage"), authorization.TenantID, authorization.OrganizationID, authorization.UserID,
		task.ID, task.UpstreamProvider, task.UpstreamRequestID, task.Model, task.ModuleCode,
		inputTokens, outputTokens, cachedTokens, reasoningTokens, inputTokens+outputTokens+cachedTokens+reasoningTokens,
		int64(task.PointCost), "model-usage:"+task.ID, jsonProjection(rawUsage))
	return err
}

func rawTokenCount(values map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			switch typed := value.(type) {
			case int:
				return int64(typed)
			case int64:
				return typed
			case float64:
				return int64(typed)
			case json.Number:
				parsed, _ := typed.Int64()
				return parsed
			}
		}
	}
	return 0
}

func maxInt64(value int64, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func (s *postgresStore) EnterpriseComputeAccount(access enterpriseAccess, limit int) (enterpriseWalletSummary, []enterpriseComputeLedgerEntry, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var wallet enterpriseWalletSummary
	if err := s.db.QueryRowContext(ctx, `
		SELECT point_balance,frozen_points,cash_balance_cents,status
		FROM xz_tenant_wallets WHERE tenant_id=$1
	`, access.TenantID).Scan(&wallet.PointBalance, &wallet.FrozenPoints, &wallet.CashBalanceCents, &wallet.Status); err != nil {
		return wallet, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,tenant_id,entry_type,source_type,compute_unit_delta,balance_before,balance_after,
		       amount_cents,reference_type,reference_id,request_id,status,metadata,created_at
		FROM xz_compute_ledger_entries
		WHERE tenant_id=$1
		ORDER BY created_at DESC,id DESC
		LIMIT $2
	`, access.TenantID, limit)
	if err != nil {
		return wallet, nil, err
	}
	defer rows.Close()
	items := []enterpriseComputeLedgerEntry{}
	for rows.Next() {
		var item enterpriseComputeLedgerEntry
		var metadataRaw []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.EntryType, &item.SourceType, &item.ComputeUnitDelta,
			&item.BalanceBefore, &item.BalanceAfter, &item.AmountCents, &item.ReferenceType, &item.ReferenceID,
			&item.RequestID, &item.Status, &metadataRaw, &createdAt); err != nil {
			return wallet, nil, err
		}
		item.Metadata = decodeJSONMap(metadataRaw)
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return wallet, items, rows.Err()
}
