package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrManualGrantNotFound = errors.New("manual membership grant record not found")

type ManualGrantPlan struct {
	ID, Code, Name, PlanType, MemberLevel string
	DurationDays                          int
	Active                                bool
}

type ManualGrantUser struct {
	PlanID, MemberLevel, SubscriptionExpiresAt string
}

type ManualGrantEntitlement struct {
	ID, TenantID, UserID, MemberLevel string
	EffectiveAt, ExpiresAt            time.Time
	SourceOrderNo, IdempotencyKey     string
	Metadata                          json.RawMessage
}

func LoadManualGrantPlanTx(ctx context.Context, tx *sql.Tx, planID string) (ManualGrantPlan, error) {
	var plan ManualGrantPlan
	err := tx.QueryRowContext(ctx, `SELECT id,coalesce(code,''),coalesce(name,''),coalesce(plan_type,''),coalesce(member_level,''),coalesce(duration_days,0),coalesce(active,false) FROM xz_plans WHERE id=$1 FOR SHARE`, planID).
		Scan(&plan.ID, &plan.Code, &plan.Name, &plan.PlanType, &plan.MemberLevel, &plan.DurationDays, &plan.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return ManualGrantPlan{}, ErrManualGrantNotFound
	}
	return plan, err
}

func LoadManualGrantUserTx(ctx context.Context, tx *sql.Tx, userID string) (ManualGrantUser, error) {
	var user ManualGrantUser
	err := tx.QueryRowContext(ctx, `SELECT coalesce(plan_id,''),coalesce(member_level,''),coalesce(subscription_expires_at,'') FROM xz_users WHERE id=$1 FOR UPDATE`, userID).
		Scan(&user.PlanID, &user.MemberLevel, &user.SubscriptionExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ManualGrantUser{}, ErrManualGrantNotFound
	}
	return user, err
}

func UpdateManualGrantUserTx(ctx context.Context, tx *sql.Tx, userID, planID, memberLevel, expiresAt, updatedAt string) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_users SET plan_id=$2::text,member_level=$3::text,subscription_expires_at=$4::text,updated_at=$5::text,raw=coalesce(raw,'{}'::jsonb)||jsonb_build_object('planId',$2::text,'memberLevel',$3::text,'subscriptionExpiresAt',$4::text,'updatedAt',$5::text) WHERE id=$1::text`, userID, planID, memberLevel, expiresAt, updatedAt)
	return err
}

func InsertManualGrantEntitlementTx(ctx context.Context, tx *sql.Tx, input ManualGrantEntitlement) error {
	if tx == nil || strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.UserID) == "" || len(input.Metadata) == 0 {
		return errors.New("invalid manual membership entitlement")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_membership_entitlement_records(id,tenant_id,user_id,member_level,effective_at,expires_at,source_order_no,idempotency_key,metadata) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, input.ID, input.TenantID, input.UserID, input.MemberLevel, input.EffectiveAt, input.ExpiresAt, input.SourceOrderNo, input.IdempotencyKey, input.Metadata)
	return err
}

func InsertManualGrantOperationLogTx(ctx context.Context, tx *sql.Tx, id, actorID, userID string, beforeState, afterState []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_operation_logs(id,actor_id,operation,target,target_id,before_state,after_state) VALUES($1,$2,'MANUAL_MEMBERSHIP_GRANT','user_membership',$3,$4::jsonb,$5::jsonb)`, id, actorID, userID, beforeState, afterState)
	return err
}
