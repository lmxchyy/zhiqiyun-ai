package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrInvalidSubscriptionProjection = errors.New("invalid membership subscription projection")

type SubscriptionProjection struct {
	ID, TenantID, UserID, PlanID, ProductCode, SourceOrderNo string
	StartsAt, EndsAt, SnapshotJSON                           string
}

// UpsertSubscriptionTx persists the canonical membership projection inside the
// caller-owned transaction. Manual grants intentionally have no payment order.
func UpsertSubscriptionTx(ctx context.Context, tx *sql.Tx, input SubscriptionProjection) error {
	if tx == nil || strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.PlanID) == "" || strings.TrimSpace(input.SnapshotJSON) == "" {
		return ErrInvalidSubscriptionProjection
	}
	_, err := tx.ExecContext(ctx, `
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
	`, input.ID, input.TenantID, input.UserID, input.PlanID, input.ProductCode, input.SourceOrderNo, input.StartsAt, input.EndsAt, input.SnapshotJSON)
	return err
}
