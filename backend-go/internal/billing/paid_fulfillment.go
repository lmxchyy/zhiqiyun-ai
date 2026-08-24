package billing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var ErrPointGrantHookUnavailable = errors.New("personal point grant hook is unavailable")

type PointGrantRequest struct {
	UserID, TenantID, Source, ReferenceType, ReferenceID, IdempotencyKey string
	Points                                                               int64
}

type PointGrantResult struct {
	AccountID, UserID string
	AvailableBefore   int64
	AvailableAfter    int64
}

type PointGrantHook func(context.Context, *sql.Tx, PointGrantRequest) (PointGrantResult, error)

type PaidPointFulfillment struct {
	OrderNo, UserID, TenantID string
	Payload                   json.RawMessage
}

func ApplyPaidPointFulfillment(ctx context.Context, tx *sql.Tx, input PaidPointFulfillment, grant PointGrantHook) error {
	if tx == nil {
		return errors.New("paid fulfillment transaction is unavailable")
	}
	entitlement, err := ParsePaidPointEntitlement(input.Payload)
	if err != nil {
		return err
	}
	if grant == nil {
		return ErrPointGrantHookUnavailable
	}
	key := "unified-payment:" + input.OrderNo + ":grant_token"
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_token_records WHERE idempotency_key=$1)`, key).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	result, err := grant(ctx, tx, PointGrantRequest{UserID: input.UserID, TenantID: input.TenantID, Source: "UNIFIED_PAYMENT_GRANT", Points: entitlement.Points, ReferenceType: "UNIFIED_PAYMENT_ORDER", ReferenceID: input.OrderNo, IdempotencyKey: key})
	if err != nil {
		return err
	}
	if result.UserID != input.UserID || result.AccountID == "" || result.AvailableAfter-result.AvailableBefore != entitlement.Points {
		return errors.New("personal point grant hook returned an invalid result")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO xz_token_records(id,user_id,order_id,change_type,amount,balance_before,balance_after,remark,created_at,tenant_id,idempotency_key,source_order_no,raw) VALUES($1,$2,$3,'UNIFIED_PAYMENT_GRANT',$4,$5,$6,'unified_payment_grant_token',now(),$7,$8,$3,$9::jsonb)`, "token_"+hex.EncodeToString(nonce), input.UserID, input.OrderNo, entitlement.Points, result.AvailableBefore, result.AvailableAfter, input.TenantID, key, `{}`)
	return err
}
