package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	membershipdomain "xianzhi-ai/backend-go/internal/membership"
)

const (
	adminManualMembershipSource              = "ADMIN_MANUAL_MEMBERSHIP"
	adminManualMaxValidityDays               = membershipdomain.MaxManualGrantDays
	adminMembershipTenantID                  = "tenant_default"
	adminMembershipUserProjectionUpdateQuery = `UPDATE xz_users SET plan_id=$2::text,member_level=$3::text,subscription_expires_at=$4::text,updated_at=$5::text,raw=coalesce(raw,'{}'::jsonb)||jsonb_build_object('planId',$2::text,'memberLevel',$3::text,'subscriptionExpiresAt',$4::text,'updatedAt',$5::text) WHERE id=$1::text`
)

type adminMembershipGrantRequest struct {
	PlanID         string `json:"planId"`
	DurationDays   int    `json:"durationDays"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type adminMembershipGrantResult struct {
	UserID       string `json:"userId"`
	PlanID       string `json:"planId"`
	MemberLevel  string `json:"memberLevel"`
	EffectiveAt  string `json:"effectiveAt"`
	ExpiresAt    string `json:"expiresAt"`
	DurationDays int    `json:"durationDays"`
	Idempotent   bool   `json:"idempotent"`
}

func normalizeAdminMembershipGrantRequest(req adminMembershipGrantRequest) (adminMembershipGrantRequest, error) {
	normalized, err := membershipdomain.NormalizeManualGrant(membershipdomain.ManualGrantRequest{
		PlanID: req.PlanID, DurationDays: req.DurationDays, Reason: req.Reason, IdempotencyKey: req.IdempotencyKey,
	})
	if errors.Is(err, membershipdomain.ErrInvalidGrant) {
		return req, ErrInvalidPointCommand
	}
	req.PlanID, req.Reason, req.IdempotencyKey, req.DurationDays = normalized.PlanID, normalized.Reason, normalized.IdempotencyKey, normalized.DurationDays
	return req, err
}

func adminManualExpiry(grantedAt time.Time, days int) (time.Time, error) {
	expiresAt, err := membershipdomain.ResolveExpiry(pointNow(grantedAt), time.Time{}, days)
	if errors.Is(err, membershipdomain.ErrInvalidGrant) {
		return time.Time{}, ErrInvalidPointCommand
	}
	return expiresAt, err
}

// resolveAdminMembershipExpiry guarantees an administrative grant never shortens
// an already-longer membership. The grant means "valid for at least N days from
// now", not "replace whatever expiry already exists".
func resolveAdminMembershipExpiry(now time.Time, previousExpiry string, days int) (time.Time, error) {
	previousExpiry = strings.TrimSpace(previousExpiry)
	if previousExpiry == "" {
		return adminManualExpiry(now, days)
	}
	previous, err := time.Parse(time.RFC3339Nano, previousExpiry)
	if err != nil {
		return time.Time{}, membershipdomain.InvalidExpiryError(err)
	}
	expiresAt, err := membershipdomain.ResolveExpiry(pointNow(now), previous.UTC(), days)
	if errors.Is(err, membershipdomain.ErrInvalidGrant) {
		return time.Time{}, ErrInvalidPointCommand
	}
	return expiresAt, err
}

func findAdminMembershipPlan(plans []adminPlan, planID string) (adminPlan, error) {
	planID = strings.TrimSpace(planID)
	for _, plan := range plans {
		if plan.ID != planID {
			continue
		}
		if !plan.Active || normalizePlanTypeString(planBusinessType(plan)) != planTypeMemberPackage || strings.TrimSpace(planMemberLevel(plan)) == "" {
			return adminPlan{}, ErrInvalidPointCommand
		}
		return plan, nil
	}
	return adminPlan{}, ErrPointNotFound
}

func (a adminAPI) grantManualMembership(ctx context.Context, actorID, actorRole, userID string, request adminMembershipGrantRequest) (adminMembershipGrantResult, error) {
	request, err := normalizeAdminMembershipGrantRequest(request)
	if err != nil {
		return adminMembershipGrantResult{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(actorRole), "SUPER_ADMIN") {
		return adminMembershipGrantResult{}, errIdentityPermission
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || strings.TrimSpace(actorID) == "" {
		return adminMembershipGrantResult{}, ErrInvalidPointCommand
	}
	if pg, ok := a.store.(*postgresStore); ok && pg.db != nil {
		return grantManualMembershipPostgres(ctx, pg.db, actorID, actorRole, userID, request)
	}
	// This is a high-impact entitlement mutation. The JSON development store
	// cannot provide the transactional entitlement/audit/subscription invariants
	// required here, so fail closed instead of pretending to have equivalent
	// semantics.
	return adminMembershipGrantResult{}, errors.New("manual membership grant requires PostgreSQL-backed admin storage")
}

func grantManualMembershipPostgres(ctx context.Context, db *sql.DB, actorID, actorRole, userID string, request adminMembershipGrantRequest) (adminMembershipGrantResult, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return adminMembershipGrantResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var plan adminPlan
	var planType, memberLevel string
	if err := tx.QueryRowContext(ctx, `SELECT id,coalesce(code,''),coalesce(name,''),coalesce(plan_type,''),coalesce(member_level,''),coalesce(duration_days,0),coalesce(active,false) FROM xz_plans WHERE id=$1 FOR SHARE`, request.PlanID).
		Scan(&plan.ID, &plan.Code, &plan.Name, &planType, &memberLevel, &plan.DurationDays, &plan.Active); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminMembershipGrantResult{}, ErrPointNotFound
		}
		return adminMembershipGrantResult{}, err
	}
	plan.PlanType, plan.MemberLevel = planType, memberLevel
	if !plan.Active || normalizePlanTypeString(plan.PlanType) != planTypeMemberPackage || strings.TrimSpace(plan.MemberLevel) == "" {
		return adminMembershipGrantResult{}, ErrInvalidPointCommand
	}
	days := request.DurationDays
	if days == 0 {
		days = plan.DurationDays
	}
	if days <= 0 || days > adminManualMaxValidityDays {
		return adminMembershipGrantResult{}, ErrInvalidPointCommand
	}

	var existingUserID, existingLevel string
	var existingEffective, existingExpiry time.Time
	var existingMetadata []byte
	existingErr := tx.QueryRowContext(ctx, `SELECT user_id,member_level,effective_at,expires_at,metadata FROM xz_membership_entitlement_records WHERE idempotency_key=$1`, request.IdempotencyKey).
		Scan(&existingUserID, &existingLevel, &existingEffective, &existingExpiry, &existingMetadata)
	if existingErr == nil {
		var meta map[string]any
		_ = json.Unmarshal(existingMetadata, &meta)
		if existingUserID != userID || strings.TrimSpace(fmt.Sprint(meta["planId"])) != request.PlanID || intValue(meta["durationDays"]) != days {
			return adminMembershipGrantResult{}, ErrIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return adminMembershipGrantResult{}, err
		}
		return adminMembershipGrantResult{UserID: userID, PlanID: request.PlanID, MemberLevel: existingLevel, EffectiveAt: existingEffective.UTC().Format(time.RFC3339Nano), ExpiresAt: existingExpiry.UTC().Format(time.RFC3339Nano), DurationDays: days, Idempotent: true}, nil
	}
	if !errors.Is(existingErr, sql.ErrNoRows) {
		return adminMembershipGrantResult{}, existingErr
	}

	// xz_users is the global user projection and does not carry tenant_id.
	// Membership entitlement records still require a tenant scope; admin
	// customer grants use the canonical default tenant used by this projection.
	tenantID := adminMembershipTenantID
	var previousPlanID, previousLevel, previousExpiry string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(plan_id,''),coalesce(member_level,''),coalesce(subscription_expires_at,'') FROM xz_users WHERE id=$1 FOR UPDATE`, userID).
		Scan(&previousPlanID, &previousLevel, &previousExpiry); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminMembershipGrantResult{}, ErrPointNotFound
		}
		return adminMembershipGrantResult{}, err
	}
	now := time.Now().UTC()
	expiresAt, err := resolveAdminMembershipExpiry(now, previousExpiry, days)
	if err != nil {
		return adminMembershipGrantResult{}, err
	}
	nowText, expiryText := now.Format(time.RFC3339Nano), expiresAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, adminMembershipUserProjectionUpdateQuery, userID, plan.ID, plan.MemberLevel, expiryText, nowText); err != nil {
		return adminMembershipGrantResult{}, err
	}
	metadata := map[string]any{
		"source": adminManualMembershipSource, "planId": plan.ID, "durationDays": days,
		"reason": request.Reason, "actorId": actorID, "actorRole": actorRole,
		"previousPlanId": previousPlanID, "previousMemberLevel": previousLevel, "previousExpiresAt": previousExpiry,
		"automaticPointGrant": false, "neverShortensExistingMembership": true,
	}
	metadataJSON, _ := json.Marshal(metadata)
	grantID := "membership_admin_" + shortID(userID+":"+request.IdempotencyKey)
	sourceOrderNo := "ADMIN-MEMBERSHIP-" + strings.ToUpper(shortID(request.IdempotencyKey))
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_membership_entitlement_records(id,tenant_id,user_id,member_level,effective_at,expires_at,source_order_no,idempotency_key,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, grantID, tenantID, userID, plan.MemberLevel, now, expiresAt, sourceOrderNo, request.IdempotencyKey, metadataJSON); err != nil {
		return adminMembershipGrantResult{}, err
	}

	// xz_billing_subscriptions is the canonical admin subscription projection.
	// Manual grants have no payment order, so migration 109 allows a NULL
	// source_order_id while preserving source_order_no as the administrative
	// provenance key. One stable manual projection per user is updated in place;
	// immutable grant history remains in xz_membership_entitlement_records.
	subscriptionID := "sub_admin_" + shortID(userID)
	productCode := strings.TrimSpace(plan.Code)
	if productCode == "" {
		productCode = plan.ID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_billing_subscriptions(
			id,tenant_id,user_id,plan_id,product_code,source_order_id,source_order_no,status,
			starts_at,ends_at,entitlement_snapshot,created_at,updated_at
		) VALUES($1,$2,$3,$4,$5,NULL,$6,'ACTIVE',$7,$8,$9::jsonb,$7,$7)
		ON CONFLICT(id) DO UPDATE SET
			tenant_id=excluded.tenant_id,user_id=excluded.user_id,plan_id=excluded.plan_id,
			product_code=excluded.product_code,source_order_id=NULL,source_order_no=excluded.source_order_no,
			status='ACTIVE',starts_at=LEAST(xz_billing_subscriptions.starts_at,excluded.starts_at),
			ends_at=GREATEST(xz_billing_subscriptions.ends_at,excluded.ends_at),
			entitlement_snapshot=excluded.entitlement_snapshot,updated_at=excluded.updated_at
	`, subscriptionID, tenantID, userID, plan.ID, productCode, sourceOrderNo, now, expiresAt, metadataJSON); err != nil {
		return adminMembershipGrantResult{}, err
	}

	if err := insertAuditLog(ctx, tx, actorID, actorRole, "admin.membership.manual_grant", "user_membership", userID, http.MethodPost, "/api/v1/admin/customers/"+userID+"/point-gifts", http.StatusOK, metadata); err != nil {
		return adminMembershipGrantResult{}, err
	}
	beforeState, _ := json.Marshal(map[string]any{"planId": previousPlanID, "memberLevel": previousLevel, "expiresAt": previousExpiry})
	afterState, _ := json.Marshal(map[string]any{"planId": plan.ID, "memberLevel": plan.MemberLevel, "expiresAt": expiryText, "durationDays": days, "automaticPointGrant": false})
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_operation_logs(id,actor_id,operation,target,target_id,before_state,after_state) VALUES($1,$2,'MANUAL_MEMBERSHIP_GRANT','user_membership',$3,$4::jsonb,$5::jsonb)`, "operation_"+shortID(grantID), actorID, userID, beforeState, afterState); err != nil {
		return adminMembershipGrantResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return adminMembershipGrantResult{}, err
	}
	return adminMembershipGrantResult{UserID: userID, PlanID: plan.ID, MemberLevel: plan.MemberLevel, EffectiveAt: nowText, ExpiresAt: expiryText, DurationDays: days}, nil
}

func adminGiftReferenceID(key string, validityDays int) string {
	if validityDays <= 0 {
		return key
	}
	return key + ":validity-days:" + strconv.Itoa(validityDays)
}

func grantAdminPointGiftWithValidity(ctx context.Context, service *PersonalPointService, cmd PersonalPointGrantCommand, validityDays int) (PersonalPointGrantResult, error) {
	if validityDays <= 0 {
		return service.Grant(ctx, cmd)
	}
	if validityDays > adminManualMaxValidityDays || service == nil || service.repo == nil || cmd.Source != PointSourceAdminGift {
		return PersonalPointGrantResult{}, ErrInvalidPointCommand
	}
	cmd.ReferenceID = adminGiftReferenceID(cmd.IdempotencyKey, validityDays)
	switch repo := service.repo.(type) {
	case *PostgresPersonalPointStore:
		tx, err := repo.begin(ctx)
		if err != nil {
			return PersonalPointGrantResult{}, adminPointGiftStageError("begin", err)
		}
		defer func() { _ = tx.Rollback() }()
		result, err := repo.grantTx(ctx, tx, cmd)
		if err != nil {
			return PersonalPointGrantResult{}, adminPointGiftStageError("grant_tx", err)
		}
		expiresAt, err := adminManualExpiry(result.Lot.GrantedAt, validityDays)
		if err != nil {
			return PersonalPointGrantResult{}, adminPointGiftStageError("resolve_expiry", err)
		}
		policySnapshot := pgSnapshot(lotPolicy(result.Lot))
		if err := repo.UpdateLotExpiryTx(ctx, tx, result.Lot.ID, expiresAt, policySnapshot); err != nil {
			return PersonalPointGrantResult{}, adminPointGiftStageError("update_expiry", err)
		}
		result.Lot.ExpiresAt = expiresAt
		if err := tx.Commit(); err != nil {
			return PersonalPointGrantResult{}, adminPointGiftStageError("commit", err)
		}
		return result, nil
	case *JSONPersonalPointStore:
		result, err := repo.grant(ctx, cmd)
		if err != nil {
			return PersonalPointGrantResult{}, err
		}
		expiresAt, err := adminManualExpiry(result.Lot.GrantedAt, validityDays)
		if err != nil {
			return PersonalPointGrantResult{}, err
		}
		err = repo.withState(ctx, func(state *personalPointState) error {
			for i := range state.Lots {
				if state.Lots[i].ID != result.Lot.ID {
					continue
				}
				state.Lots[i].ExpiresAt = expiresAt
				result.Lot = state.Lots[i]
				return nil
			}
			return ErrPointNotFound
		})
		return result, err
	default:
		return PersonalPointGrantResult{}, ErrInvalidPointCommand
	}
}

func adminPointGiftStageError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("admin point gift stage=%s: %w", stage, err)
}
