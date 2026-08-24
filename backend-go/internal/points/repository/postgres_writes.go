package repository

import (
	"context"
	"database/sql"
)

func ArchivePolicy(ctx context.Context, tx *sql.Tx, policyID string, effectiveTo any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_point_expiry_policy_versions SET status='ARCHIVED',effective_to=$2,updated_at=$2 WHERE id=$1`, policyID, effectiveTo)
	return err
}

func EnsureAccount(ctx context.Context, tx *sql.Tx, accountID, userID string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw) VALUES($1,$2,0,0,'{}'::jsonb) ON CONFLICT (id) DO NOTHING`, accountID, userID)
	return err
}

// The write-side SQL for Points lives here. Callers provide the transaction;
// these functions never begin or commit one themselves.
func UpdateAccount(ctx context.Context, tx *sql.Tx, accountID string, available, frozen int64, userID string, totalGranted, totalUsed, totalExpired, totalReversed int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_point_accounts SET available=$2::bigint, frozen=$3::bigint, raw=COALESCE(raw,'{}'::jsonb)||jsonb_build_object('id',$1::text,'userId',$4::text,'available',$2::bigint,'frozen',$3::bigint,'totalGranted',$5::bigint,'totalUsed',$6::bigint,'totalExpired',$7::bigint,'totalReversed',$8::bigint) WHERE id=$1::text AND user_id=$4::text`, accountID, available, frozen, userID, totalGranted, totalUsed, totalExpired, totalReversed)
	return err
}

func InsertMovement(ctx context.Context, tx *sql.Tx, id, lotID, accountID, userID, movementType string, points, availableBefore, availableAfter, reservedBefore, reservedAfter, consumedBefore, consumedAfter, expiredBefore, expiredAfter, reversedBefore, reversedAfter int64, referenceType, referenceID, reservationID, idempotencyKey string, createdAt any) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lot_movements(id,lot_id,account_id,user_id,movement_type,points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after,reference_type,reference_id,reservation_id,idempotency_key,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,NULLIF($19,'')::text,$20,'{}'::jsonb,$21) ON CONFLICT (lot_id,idempotency_key) DO NOTHING`, id, lotID, accountID, userID, movementType, points, availableBefore, availableAfter, reservedBefore, reservedAfter, consumedBefore, consumedAfter, expiredBefore, expiredAfter, reversedBefore, reversedAfter, referenceType, referenceID, reservationID, idempotencyKey, createdAt)
	return err
}

func InsertWallet(ctx context.Context, tx *sql.Tx, id, accountID, userID, taskID, entryType string, points, availableBefore, availableAfter, frozenBefore, frozenAfter int64, idempotencyKey, referenceType, referenceID string, metadata any, createdAt any) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_wallet_ledger(id,account_id,user_id,task_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key,reference_type,reference_id,metadata,created_at) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT (idempotency_key) DO NOTHING`, id, accountID, userID, taskID, entryType, points, availableBefore, availableAfter, frozenBefore, frozenAfter, idempotencyKey, referenceType, referenceID, metadata, createdAt)
	return err
}

func UpdateLotExpiry(ctx context.Context, tx *sql.Tx, lotID string, expiresAt, policySnapshot any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET expires_at=$2,policy_snapshot=$3::jsonb WHERE id=$1`, lotID, expiresAt, policySnapshot)
	return err
}

func InsertLot(ctx context.Context, tx *sql.Tx, id, accountID, userID, sourceType, referenceType, referenceID string, points int64, grantedAt, expiresAt, policyVersionID, policySnapshot, idempotencyKey, status any) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,policy_version_id,policy_snapshot,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,$7,0,0,0,0,$8,$9,NULLIF($10,'')::text,$11,$12,$13)`, id, accountID, userID, sourceType, referenceType, referenceID, points, grantedAt, expiresAt, policyVersionID, policySnapshot, idempotencyKey, status)
	return err
}

func InsertPermanentLot(ctx context.Context, tx *sql.Tx, id, accountID, userID, sourceType, referenceType, referenceID string, points int64, grantedAt, idempotencyKey any) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,policy_version_id,policy_snapshot,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,$7,0,0,0,0,$8,NULL,NULL,'{}'::jsonb,$9,'ACTIVE')`, id, accountID, userID, sourceType, referenceType, referenceID, points, grantedAt, idempotencyKey)
	return err
}

func UpdateCorrectionLot(ctx context.Context, tx *sql.Tx, id string, available, reversed int64, status string, updatedAt any, accountID, userID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=$2,reversed_points=$3,status=$4,updated_at=$5 WHERE id=$1 AND account_id=$6 AND user_id=$7`, id, available, reversed, status, updatedAt, accountID, userID)
	return err
}

func UpdateReversedLot(ctx context.Context, tx *sql.Tx, id string, reversed int64, status string, updatedAt any, accountID, userID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=0,reversed_points=$2,status=$3,updated_at=$4 WHERE id=$1 AND account_id=$5 AND user_id=$6`, id, reversed, status, updatedAt, accountID, userID)
	return err
}

func InsertTransferredLot(ctx context.Context, tx *sql.Tx, id, accountID, userID, sourceType, referenceType, referenceID string, points int64, grantedAt, expiresAt, policyVersionID, policySnapshot, idempotencyKey, status, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,policy_version_id,policy_snapshot,idempotency_key,status,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7,0,0,0,0,$8,$9,NULLIF($10,'')::text,$11,$12,$13,$14)`, id, accountID, userID, sourceType, referenceType, referenceID, points, grantedAt, expiresAt, policyVersionID, policySnapshot, idempotencyKey, status, updatedAt)
	return err
}

func UpdateReservedLot(ctx context.Context, tx *sql.Tx, id string, available, reserved int64, status string, updatedAt any, accountID, userID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=$2,reserved_points=$3,status=$4,updated_at=$5 WHERE id=$1 AND account_id=$6 AND user_id=$7`, id, available, reserved, status, updatedAt, accountID, userID)
	return err
}

func UpdateCapturedLot(ctx context.Context, tx *sql.Tx, id string, reserved, consumed int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET reserved_points=$2,consumed_points=$3,status=$4,updated_at=$5 WHERE id=$1`, id, reserved, consumed, status, updatedAt)
	return err
}

func UpdateReleasedLot(ctx context.Context, tx *sql.Tx, id string, available, reserved, expired int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=$2,reserved_points=$3,expired_points=$4,status=$5,updated_at=$6 WHERE id=$1`, id, available, reserved, expired, status, updatedAt)
	return err
}

func UpdateExpiredLot(ctx context.Context, tx *sql.Tx, id string, expired int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=0,expired_points=$2,status=$3,updated_at=$4 WHERE id=$1`, id, expired, status, updatedAt)
	return err
}

func UpdateCapturedAllocation(ctx context.Context, tx *sql.Tx, id string, reserved, captured int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_reservation_allocations SET reserved_points=$2,captured_points=$3,status=$4,updated_at=$5 WHERE id=$1`, id, reserved, captured, status, updatedAt)
	return err
}

func UpdateCapturedReservation(ctx context.Context, tx *sql.Tx, id string, reserved, captured int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_reservations SET reserved_points=$2,captured_points=$3,status=$4,updated_at=$5 WHERE id=$1`, id, reserved, captured, status, updatedAt)
	return err
}

func UpdateReleasedAllocation(ctx context.Context, tx *sql.Tx, id string, reserved, released int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_reservation_allocations SET reserved_points=$2,released_points=$3,status=$4,updated_at=$5 WHERE id=$1`, id, reserved, released, status, updatedAt)
	return err
}

func UpdateReleasedReservation(ctx context.Context, tx *sql.Tx, id string, reserved, released int64, status string, updatedAt any) error {
	_, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_reservations SET reserved_points=$2,released_points=$3,status=$4,updated_at=$5 WHERE id=$1`, id, reserved, released, status, updatedAt)
	return err
}
