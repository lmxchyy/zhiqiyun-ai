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
)

const (
	adminManualMembershipSource = "ADMIN_MANUAL_MEMBERSHIP"
	adminManualMaxValidityDays   = 3650
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
	req.PlanID = strings.TrimSpace(req.PlanID)
	req.Reason = strings.TrimSpace(req.Reason)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	if req.PlanID == "" || req.Reason == "" || req.IdempotencyKey == "" {
		return req, ErrInvalidPointCommand
	}
	if req.DurationDays < 0 || req.DurationDays > adminManualMaxValidityDays {
		return req, ErrInvalidPointCommand
	}
	return req, nil
}

func adminManualExpiry(grantedAt time.Time, days int) (time.Time, error) {
	if days <= 0 || days > adminManualMaxValidityDays {
		return time.Time{}, ErrInvalidPointCommand
	}
	return pointNow(grantedAt).AddDate(0, 0, days), nil
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
	js, ok := a.store.(*jsonStore)
	if !ok {
		return adminMembershipGrantResult{}, errors.New("manual membership grant requires a writable admin store")
	}
	return grantManualMembershipJSON(js, actorID, actorRole, userID, request)
}

func grantManualMembershipJSON(store *jsonStore, actorID, actorRole, userID string, request adminMembershipGrantRequest) (adminMembershipGrantResult, error) {
	var result adminMembershipGrantResult
	err := store.updateAdmin(func(data *adminPlatformData) error {
		plan, err := findAdminMembershipPlan(data.Plans, request.PlanID)
		if err != nil {
			return err
		}
		days := request.DurationDays
		if days == 0 {
			days = plan.DurationDays
		}
		expiresAt, err := adminManualExpiry(time.Now().UTC(), days)
		if err != nil {
			return err
		}
		for i := range data.Users {
			if data.Users[i].ID != userID {
				continue
			}
			data.Users[i].PlanID = plan.ID
			data.Users[i].MemberLevel = planMemberLevel(plan)
			data.Users[i].SubscriptionExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
			data.Users[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			result = adminMembershipGrantResult{
				UserID: userID, PlanID: plan.ID, MemberLevel: data.Users[i].MemberLevel,
				EffectiveAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: data.Users[i].SubscriptionExpiresAt,
				DurationDays: days,
			}
			return nil
		}
		return ErrPointNotFound
	})
	return result, err
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

	var previousPlanID, previousLevel, previousExpiry string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(plan_id,''),coalesce(member_level,''),coalesce(subscription_expires_at,'') FROM xz_users WHERE id=$1 FOR UPDATE`, userID).
		Scan(&previousPlanID, &previousLevel, &previousExpiry); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adminMembershipGrantResult{}, ErrPointNotFound
		}
		return adminMembershipGrantResult{}, err
	}
	now := time.Now().UTC()
	expiresAt, err := adminManualExpiry(now, days)
	if err != nil {
		return adminMembershipGrantResult{}, err
	}
	nowText, expiryText := now.Format(time.RFC3339Nano), expiresAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE xz_users SET plan_id=$2,member_level=$3,subscription_expires_at=$4,updated_at=$5,raw=coalesce(raw,'{}'::jsonb)||jsonb_build_object('planId',$2,'memberLevel',$3,'subscriptionExpiresAt',$4,'updatedAt',$5) WHERE id=$1`, userID, plan.ID, plan.MemberLevel, expiryText, nowText); err != nil {
		return adminMembershipGrantResult{}, err
	}
	metadata := map[string]any{
		"source": adminManualMembershipSource, "planId": plan.ID, "durationDays": days,
		"reason": request.Reason, "actorId": actorID, "actorRole": actorRole,
		"previousPlanId": previousPlanID, "previousMemberLevel": previousLevel, "previousExpiresAt": previousExpiry,
		"automaticPointGrant": false,
	}
	metadataJSON, _ := json.Marshal(metadata)
	grantID := "membership_admin_" + shortID(userID+":"+request.IdempotencyKey)
	sourceOrderNo := "ADMIN-MEMBERSHIP-" + strings.ToUpper(shortID(request.IdempotencyKey))
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_membership_entitlement_records(id,tenant_id,user_id,member_level,effective_at,expires_at,source_order_no,idempotency_key,metadata) VALUES($1,'tenant_default',$2,$3,$4,$5,$6,$7,$8::jsonb)`, grantID, userID, plan.MemberLevel, now, expiresAt, sourceOrderNo, request.IdempotencyKey, metadataJSON); err != nil {
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
			return PersonalPointGrantResult{}, err
		}
		defer func() { _ = tx.Rollback() }()
		result, err := repo.grantTx(ctx, tx, cmd)
		if err != nil {
			return PersonalPointGrantResult{}, err
		}
		expiresAt, err := adminManualExpiry(result.Lot.GrantedAt, validityDays)
		if err != nil {
			return PersonalPointGrantResult{}, err
		}
		policySnapshot, _ := json.Marshal(map[string]any{"version": 0, "enabled": true, "durationValue": validityDays, "durationUnit": "DAY", "timeZone": "Asia/Shanghai", "adminOverride": true})
		if _, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET expires_at=$2,policy_version_id=NULL,policy_snapshot=$3::jsonb WHERE id=$1`, result.Lot.ID, expiresAt, policySnapshot); err != nil {
			return PersonalPointGrantResult{}, err
		}
		result.Lot.ExpiresAt = expiresAt
		result.Lot.PolicyVersionID = ""
		result.Lot.PolicySnapshot = PointPolicySnapshot{Enabled: true, DurationValue: validityDays, DurationUnit: "DAY", TimeZone: "Asia/Shanghai"}
		if err := tx.Commit(); err != nil {
			return PersonalPointGrantResult{}, err
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
				state.Lots[i].PolicyVersionID = ""
				state.Lots[i].PolicySnapshot = PointPolicySnapshot{Enabled: true, DurationValue: validityDays, DurationUnit: "DAY", TimeZone: "Asia/Shanghai"}
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
