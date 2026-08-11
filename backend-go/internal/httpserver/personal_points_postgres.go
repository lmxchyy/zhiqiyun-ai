package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// PostgresPersonalPointStore is the authoritative lot-aware repository.  Every
// mutation uses one transaction and locks the aggregate account before lots,
// reservations, and allocations, matching the JSON adapter's lock order.
type PostgresPersonalPointStore struct{ db *sql.DB }

func personalWalletKey(accountID, kind, callerKey string) string {
	return "personal-point:" + kind + ":" + accountID + ":" + callerKey
}

func NewPostgresPersonalPointStore(db *sql.DB) *PostgresPersonalPointStore {
	return &PostgresPersonalPointStore{db: db}
}

func (s *PostgresPersonalPointStore) currentPolicy(ctx context.Context) (PointExpiryPolicy, error) {
	if s == nil || s.db == nil {
		return PointExpiryPolicy{}, ErrInvalidPointCommand
	}
	var policy PointExpiryPolicy
	var sourceTypes []byte
	var effectiveTo sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT id,version,revision,enabled,duration_value,duration_unit,time_zone,source_types,effective_from,effective_to,status,created_by,change_reason FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' AND effective_from <= now() AND (effective_to IS NULL OR effective_to > now()) ORDER BY version DESC LIMIT 1`).Scan(
		&policy.ID, &policy.Version, &policy.Revision, &policy.Enabled, &policy.DurationValue, &policy.DurationUnit, &policy.TimeZone, &sourceTypes, &policy.EffectiveFrom, &effectiveTo, &policy.Status, &policy.CreatedBy, &policy.ChangeReason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PointExpiryPolicy{}, ErrPointNotFound
	}
	if err != nil {
		return PointExpiryPolicy{}, err
	}
	if effectiveTo.Valid {
		policy.EffectiveTo = effectiveTo.Time.UTC()
	}
	if err := json.Unmarshal(sourceTypes, &policy.SourceTypes); err != nil {
		return PointExpiryPolicy{}, err
	}
	return policy, validatePointExpiryPolicy(policy)
}

func (s *PostgresPersonalPointStore) publishPolicy(ctx context.Context, cmd PersonalPointPolicyPublishCommand) (PointExpiryPolicy, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return PointExpiryPolicy{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT transaction_timestamp()`).Scan(&now); err != nil {
		return PointExpiryPolicy{}, err
	}
	now = now.UTC()
	var current PointExpiryPolicy
	err = tx.QueryRowContext(ctx, `SELECT id,version,revision FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' ORDER BY version DESC LIMIT 1 FOR UPDATE`).Scan(&current.ID, &current.Version, &current.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		// A concurrent publisher can archive the tuple selected by this statement
		// while we wait for its row lock. Re-read in the next READ COMMITTED
		// statement so that replacement is reported as a revision conflict.
		var latestRevision int64
		rereadErr := tx.QueryRowContext(ctx, `SELECT revision FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' ORDER BY version DESC LIMIT 1`).Scan(&latestRevision)
		if rereadErr == nil {
			return PointExpiryPolicy{}, ErrPointPolicyRevisionConflict
		}
		if errors.Is(rereadErr, sql.ErrNoRows) {
			return PointExpiryPolicy{}, ErrPointNotFound
		}
		return PointExpiryPolicy{}, rereadErr
	}
	if err != nil {
		return PointExpiryPolicy{}, err
	}
	if current.Revision != cmd.ExpectedRevision {
		return PointExpiryPolicy{}, ErrPointPolicyRevisionConflict
	}
	published := PointExpiryPolicy{
		ID: fmt.Sprintf("point_expiry_policy_v%d", current.Version+1), Version: current.Version + 1, Revision: current.Revision + 1,
		Enabled: cmd.Enabled, DurationValue: cmd.DurationValue, DurationUnit: "CALENDAR_MONTH", TimeZone: "Asia/Shanghai",
		SourceTypes:   []string{string(PointSourceRegistrationGift), string(PointSourceActivityGift), string(PointSourceAdminGift)},
		EffectiveFrom: now, Status: "PUBLISHED", CreatedBy: strings.TrimSpace(cmd.ActorID), ChangeReason: strings.TrimSpace(cmd.ChangeReason),
	}
	if err := validatePointExpiryPolicy(published); err != nil {
		return PointExpiryPolicy{}, err
	}
	sourceTypes, _ := json.Marshal(published.SourceTypes)
	if _, err := tx.ExecContext(ctx, `UPDATE xz_point_expiry_policy_versions SET status='ARCHIVED',effective_to=$2,updated_at=$2 WHERE id=$1`, current.ID, now); err != nil {
		return PointExpiryPolicy{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_point_expiry_policy_versions(id,version,revision,enabled,duration_value,duration_unit,time_zone,source_types,effective_from,status,created_by,change_reason,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,'PUBLISHED',$10,$11,$9,$9)`, published.ID, published.Version, published.Revision, published.Enabled, published.DurationValue, published.DurationUnit, published.TimeZone, sourceTypes, now, published.CreatedBy, published.ChangeReason); err != nil {
		return PointExpiryPolicy{}, err
	}
	if err := tx.Commit(); err != nil {
		return PointExpiryPolicy{}, err
	}
	return published, nil
}

func (s *PostgresPersonalPointStore) begin(ctx context.Context) (*sql.Tx, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidPointCommand
	}
	return s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

type pgPointAccount struct {
	ID            string
	UserID        string
	Available     int64
	Frozen        int64
	TotalGranted  int64
	TotalConsumed int64
	TotalExpired  int64
	TotalReversed int64
}

func pgLoadAccount(ctx context.Context, tx *sql.Tx, accountID, userID string, forUpdate bool) (pgPointAccount, bool, error) {
	if accountID == "" || userID == "" {
		return pgPointAccount{}, false, ErrInvalidPointCommand
	}
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	var account pgPointAccount
	err := tx.QueryRowContext(ctx, `SELECT id, COALESCE(user_id,''), available, frozen, COALESCE(NULLIF(raw->>'totalGranted','')::bigint,0), COALESCE(NULLIF(raw->>'totalUsed','')::bigint,0), COALESCE(NULLIF(raw->>'totalExpired','')::bigint,0), COALESCE(NULLIF(raw->>'totalReversed','')::bigint,0) FROM xz_point_accounts WHERE id=$1`+lock, accountID).Scan(&account.ID, &account.UserID, &account.Available, &account.Frozen, &account.TotalGranted, &account.TotalConsumed, &account.TotalExpired, &account.TotalReversed)
	if errors.Is(err, sql.ErrNoRows) {
		return pgPointAccount{}, false, nil
	}
	if err != nil {
		return pgPointAccount{}, false, err
	}
	if account.UserID != userID {
		return pgPointAccount{}, false, ErrPointOwnership
	}
	return account, true, nil
}

func pgLoadPersonalAccountForUserTx(ctx context.Context, tx *sql.Tx, userID string) (pgPointAccount, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM xz_point_accounts WHERE user_id=$1 ORDER BY id FOR UPDATE`, userID)
	if err != nil {
		return pgPointAccount{}, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return pgPointAccount{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return pgPointAccount{}, err
	}
	if len(ids) == 0 {
		return pgPointAccount{}, ErrInsufficientPoints
	}
	if len(ids) != 1 {
		return pgPointAccount{}, ErrPointOwnership
	}
	account, ok, err := pgLoadAccount(ctx, tx, ids[0], userID, true)
	if err != nil {
		return pgPointAccount{}, err
	}
	if !ok {
		return pgPointAccount{}, ErrPointNotFound
	}
	return account, nil
}

func pgEnsureAccount(ctx context.Context, tx *sql.Tx, accountID, userID string) (pgPointAccount, error) {
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw) VALUES($1,$2,0,0,'{}'::jsonb) ON CONFLICT (id) DO NOTHING`, accountID, userID); err != nil {
		return pgPointAccount{}, err
	}
	account, ok, err := pgLoadAccount(ctx, tx, accountID, userID, true)
	if err != nil {
		return pgPointAccount{}, err
	}
	if !ok {
		return pgPointAccount{}, ErrPointNotFound
	}
	return account, nil
}

func pgUpdateAccount(ctx context.Context, tx *sql.Tx, account pgPointAccount) error {
	var lotAvailable, lotReserved int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(available_points),0),COALESCE(sum(reserved_points),0) FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2`, account.ID, account.UserID).Scan(&lotAvailable, &lotReserved); err != nil {
		return err
	}
	if lotAvailable != account.Available || lotReserved != account.Frozen {
		return fmt.Errorf("personal point projection mismatch for account %s: account=(%d,%d) lots=(%d,%d)", account.ID, account.Available, account.Frozen, lotAvailable, lotReserved)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_point_accounts SET available=$2::bigint, frozen=$3::bigint, raw=COALESCE(raw,'{}'::jsonb)||jsonb_build_object('id',$1::text,'userId',$4::text,'available',$2::bigint,'frozen',$3::bigint,'totalGranted',$5::bigint,'totalUsed',$6::bigint,'totalExpired',$7::bigint,'totalReversed',$8::bigint) WHERE id=$1::text AND user_id=$4::text`, account.ID, account.Available, account.Frozen, account.UserID, account.TotalGranted, account.TotalConsumed, account.TotalExpired, account.TotalReversed); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_user_wallets(user_id,token_balance,cash_balance_cents,frozen_token,total_token_granted,total_token_used,updated_at,raw) VALUES($1::text,$2::bigint,0,$3::bigint,$4::bigint,$5::bigint,now(),jsonb_build_object('userId',$1::text,'tokenBalance',$2::bigint,'frozenToken',$3::bigint,'totalTokenGranted',$4::bigint,'totalTokenUsed',$5::bigint)) ON CONFLICT(user_id) DO UPDATE SET token_balance=excluded.token_balance,frozen_token=excluded.frozen_token,total_token_granted=excluded.total_token_granted,total_token_used=excluded.total_token_used,updated_at=now(),raw=COALESCE(xz_user_wallets.raw,'{}'::jsonb)||excluded.raw`, account.UserID, account.Available, account.Frozen, account.TotalGranted, account.TotalConsumed)
	return err
}

func pgPolicy(ctx context.Context, tx *sql.Tx, source PointSource, now time.Time) (PointExpiryPolicy, error) {
	var policy PointExpiryPolicy
	var sourceTypes []byte
	var effectiveTo sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT id,version,revision,enabled,duration_value,duration_unit,time_zone,source_types,effective_from,effective_to,status FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' AND effective_from <= $2 AND (effective_to IS NULL OR effective_to > $2) AND source_types @> jsonb_build_array($1::text) ORDER BY version DESC LIMIT 1`, string(source), pointNow(now)).Scan(&policy.ID, &policy.Version, &policy.Revision, &policy.Enabled, &policy.DurationValue, &policy.DurationUnit, &policy.TimeZone, &sourceTypes, &policy.EffectiveFrom, &effectiveTo, &policy.Status)
	if err != nil {
		return policy, err
	}
	if err := json.Unmarshal(sourceTypes, &policy.SourceTypes); err != nil {
		return policy, err
	}
	if effectiveTo.Valid {
		policy.EffectiveTo = effectiveTo.Time.UTC()
	}
	if err := validatePointExpiryPolicy(policy); err != nil {
		return policy, err
	}
	return policy, nil
}

func pgScanLot(scanner interface{ Scan(...any) error }) (PersonalPointLot, error) {
	var lot PersonalPointLot
	var source, policyID, status string
	var expires sql.NullTime
	var snapshot []byte
	err := scanner.Scan(&lot.ID, &lot.AccountID, &lot.UserID, &source, &lot.ReferenceType, &lot.ReferenceID, &lot.OriginalPoints, &lot.AvailablePoints, &lot.ReservedPoints, &lot.ConsumedPoints, &lot.ExpiredPoints, &lot.ReversedPoints, &lot.GrantedAt, &expires, &policyID, &snapshot, &lot.IdempotencyKey, &status)
	if err != nil {
		return lot, err
	}
	lot.SourceType, lot.PolicyVersionID, lot.Status = PointSource(source), policyID, status
	if expires.Valid {
		lot.ExpiresAt = expires.Time.UTC()
	}
	if len(snapshot) > 0 {
		_ = json.Unmarshal(snapshot, &lot.PolicySnapshot)
	}
	lot.GrantedAt = lot.GrantedAt.UTC()
	return lot, nil
}

const pgLotColumns = `id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,COALESCE(policy_version_id,''),policy_snapshot,idempotency_key,status`

func pgReadLot(ctx context.Context, tx *sql.Tx, id, accountID, userID string, forUpdate bool) (PersonalPointLot, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	lot, err := pgScanLot(tx.QueryRowContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE id=$1 AND account_id=$2 AND user_id=$3`+lock, id, accountID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return PersonalPointLot{}, ErrPointNotFound
	}
	return lot, err
}

func pgStatusLot(lot *PersonalPointLot) string {
	if lot.SourceType == PointSourceLegacy {
		return "LEGACY"
	}
	if lot.AvailablePoints > 0 || lot.ReservedPoints > 0 {
		return "ACTIVE"
	}
	if lot.ExpiredPoints > 0 && lot.ConsumedPoints == 0 && lot.ReversedPoints == 0 {
		return "EXPIRED"
	}
	if lot.ReversedPoints > 0 && lot.ConsumedPoints == 0 {
		return "REVERSED"
	}
	return "EXHAUSTED"
}

func pgStatusReservation(r *PersonalPointReservation) string {
	if r.ReservedPoints > 0 {
		if r.CapturedPoints > 0 || r.ReleasedPoints > 0 || r.ExpiredPoints > 0 {
			return "PARTIAL"
		}
		return "RESERVED"
	}
	if r.CapturedPoints == r.RequestedPoints {
		return "CAPTURED"
	}
	if r.ReleasedPoints+r.ExpiredPoints == r.RequestedPoints {
		return "RELEASED"
	}
	if r.CapturedPoints > 0 {
		return "PARTIAL"
	}
	return "RELEASED"
}

func pgStatusAllocation(a *PersonalPointAllocation) string {
	if a.ReservedPoints > 0 {
		if a.CapturedPoints > 0 || a.ReleasedPoints > 0 || a.ExpiredPoints > 0 {
			return "PARTIAL"
		}
		return "RESERVED"
	}
	if a.CapturedPoints == a.AllocatedPoints {
		return "CAPTURED"
	}
	if a.ReleasedPoints+a.ExpiredPoints == a.AllocatedPoints {
		return "RELEASED"
	}
	if a.CapturedPoints > 0 {
		return "PARTIAL"
	}
	return "RELEASED"
}

func pgSnapshot(policy PointExpiryPolicy) []byte {
	raw, _ := json.Marshal(PointPolicySnapshot{Version: policy.Version, Enabled: policy.Enabled, DurationValue: policy.DurationValue, DurationUnit: policy.DurationUnit, TimeZone: policy.TimeZone})
	return raw
}

func pgInsertMovement(ctx context.Context, tx *sql.Tx, lot PersonalPointLot, movementType string, points int64, before PersonalPointLot, reservationID, key string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lot_movements(id,lot_id,account_id,user_id,movement_type,points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after,reference_type,reference_id,reservation_id,idempotency_key,metadata,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,NULLIF($19,'')::text,$20,'{}'::jsonb,$21) ON CONFLICT (lot_id,idempotency_key) DO NOTHING`, stablePointID("movement", lot.AccountID, key), lot.ID, lot.AccountID, lot.UserID, movementType, points, before.AvailablePoints, lot.AvailablePoints, before.ReservedPoints, lot.ReservedPoints, before.ConsumedPoints, lot.ConsumedPoints, before.ExpiredPoints, lot.ExpiredPoints, before.ReversedPoints, lot.ReversedPoints, lot.ReferenceType, lot.ReferenceID, reservationID, key, now)
	return err
}

func pgInsertWallet(ctx context.Context, tx *sql.Tx, account pgPointAccount, entryType string, points int64, beforeAvailable, beforeFrozen int64, key, refType, refID string, now time.Time, metadata any) error {
	raw, _ := json.Marshal(metadata)
	taskID := ""
	if strings.EqualFold(strings.TrimSpace(refType), "GENERATION_TASK") {
		taskID = strings.TrimSpace(refID)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO xz_wallet_ledger(id,account_id,user_id,task_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key,reference_type,reference_id,metadata,created_at) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT (idempotency_key) DO NOTHING`, stablePointID("wallet", account.ID, key), account.ID, account.UserID, taskID, entryType, points, beforeAvailable, account.Available, beforeFrozen, account.Frozen, key, refType, refID, raw, now)
	return err
}

func pgInsertPersonalPointAudit(ctx context.Context, tx *sql.Tx, audit PersonalPointAudit, accountID, userID, idempotencyKey, reason string, signedPoints int64, source PointSource) error {
	if strings.TrimSpace(audit.Action) == "" {
		return nil
	}
	if tx == nil || strings.TrimSpace(audit.ActorID) == "" || strings.TrimSpace(audit.ActorRole) == "" || strings.TrimSpace(audit.RequestID) == "" || strings.TrimSpace(reason) == "" {
		return ErrInvalidPointCommand
	}
	metadata := jsonProjection(map[string]any{"requestId": audit.RequestID, "reason": reason, "idempotencyKey": idempotencyKey, "signedPoints": signedPoints, "sourceType": source, "userId": userID})
	result, err := tx.ExecContext(ctx, `INSERT INTO xz_audit_logs(id,actor_id,actor_role,action,resource,resource_id,method,path,status,metadata) VALUES($1,$2,$3,$4,'personal_point_account',$5,$6,$7,200,$8::jsonb) ON CONFLICT(id) DO NOTHING`, stablePointID("audit", accountID, audit.Action+":"+idempotencyKey), audit.ActorID, audit.ActorRole, audit.Action, accountID, audit.Method, audit.Path, metadata)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrIdempotencyConflict
	}
	return nil
}

func (s *PostgresPersonalPointStore) grant(ctx context.Context, cmd PersonalPointGrantCommand) (result PersonalPointGrantResult, err error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err = s.grantTx(ctx, tx, cmd)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return PersonalPointGrantResult{}, err
	}
	return result, nil
}

func (s *PostgresPersonalPointStore) grantTx(ctx context.Context, tx *sql.Tx, cmd PersonalPointGrantCommand) (result PersonalPointGrantResult, err error) {
	if tx == nil {
		return result, ErrInvalidPointCommand
	}
	if err := normalizePointCommand(cmd); err != nil {
		return result, err
	}
	fingerprint := personalPointGrantFingerprint(cmd)
	grantedAtProvided := !cmd.GrantedAt.IsZero()
	cmd.GrantedAt = pointNow(cmd.GrantedAt)
	account, err := pgEnsureAccount(ctx, tx, cmd.AccountID, cmd.UserID)
	if err != nil {
		return result, err
	}
	walletKey := personalWalletKey(cmd.AccountID, "grant", cmd.IdempotencyKey)
	if idem, idemErr := pgWalletIdempotent(ctx, tx, cmd.AccountID, walletKey, fingerprint); idemErr != nil {
		return result, idemErr
	} else if idem {
		existing, existingErr := pgScanLot(tx.QueryRowContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND idempotency_key=$2`, cmd.AccountID, cmd.IdempotencyKey))
		if existingErr != nil {
			if errors.Is(existingErr, sql.ErrNoRows) {
				return result, ErrPointNotFound
			}
			return result, existingErr
		}
		return PersonalPointGrantResult{Lot: existing, Idempotent: true}, nil
	}
	var existing PersonalPointLot
	existing, scanErr := pgScanLot(tx.QueryRowContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND idempotency_key=$2`, cmd.AccountID, cmd.IdempotencyKey))
	if scanErr == nil {
		if existing.UserID != cmd.UserID || existing.SourceType != cmd.Source || existing.OriginalPoints != cmd.Points || existing.ReferenceType != cmd.ReferenceType || existing.ReferenceID != cmd.ReferenceID || (grantedAtProvided && !existing.GrantedAt.Equal(cmd.GrantedAt)) {
			return result, ErrIdempotencyConflict
		}
		return PersonalPointGrantResult{Lot: existing, Idempotent: true}, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return result, scanErr
	}
	lot := PersonalPointLot{ID: stablePointID("lot", cmd.AccountID, cmd.IdempotencyKey), AccountID: cmd.AccountID, UserID: cmd.UserID, SourceType: cmd.Source, ReferenceType: cmd.ReferenceType, ReferenceID: cmd.ReferenceID, OriginalPoints: cmd.Points, AvailablePoints: cmd.Points, GrantedAt: cmd.GrantedAt, IdempotencyKey: cmd.IdempotencyKey, Status: "ACTIVE"}
	if isGiftPointSource(cmd.Source) {
		policy, policyErr := pgPolicy(ctx, tx, cmd.Source, time.Now().UTC())
		if policyErr != nil {
			return result, policyErr
		}
		lot.PolicyVersionID, lot.PolicySnapshot = policy.ID, PointPolicySnapshot{Version: policy.Version, Enabled: policy.Enabled, DurationValue: policy.DurationValue, DurationUnit: policy.DurationUnit, TimeZone: policy.TimeZone}
		if policy.Enabled {
			lot.ExpiresAt, err = addCalendarMonthsClamp(cmd.GrantedAt, policy.DurationValue, policy.TimeZone)
			if err != nil {
				return result, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,policy_version_id,policy_snapshot,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,$7,0,0,0,0,$8,$9,NULLIF($10,'')::text,$11,$12,$13)`, lot.ID, lot.AccountID, lot.UserID, lot.SourceType, lot.ReferenceType, lot.ReferenceID, lot.OriginalPoints, lot.GrantedAt, nullTime(lot.ExpiresAt), lot.PolicyVersionID, pgSnapshot(lotPolicy(lot)), lot.IdempotencyKey, lot.Status); err != nil {
		return result, err
	}
	beforeAvailable, beforeFrozen := account.Available, account.Frozen
	account.Available += cmd.Points
	account.TotalGranted += cmd.Points
	if err = pgUpdateAccount(ctx, tx, account); err != nil {
		return result, err
	}
	if err = pgInsertMovement(ctx, tx, lot, "OPENING", cmd.Points, PersonalPointLot{}, "", "grant:"+cmd.IdempotencyKey, cmd.GrantedAt); err != nil {
		return result, err
	}
	entryType := "GRANT"
	if cmd.Source == PointSourceRecharge {
		entryType = "RECHARGE"
	}
	if err = pgInsertWallet(ctx, tx, account, entryType, cmd.Points, beforeAvailable, beforeFrozen, walletKey, cmd.ReferenceType, cmd.ReferenceID, cmd.GrantedAt, map[string]any{"fingerprint": fingerprint, "source_type": cmd.Source}); err != nil {
		return result, err
	}
	if err = pgInsertPersonalPointAudit(ctx, tx, cmd.Audit, cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, strings.TrimSpace(cmd.Reason), cmd.Points, cmd.Source); err != nil {
		return result, err
	}
	return PersonalPointGrantResult{Lot: lot}, nil
}

func (s *PostgresPersonalPointStore) correct(ctx context.Context, cmd PersonalPointCorrectionCommand) (result PersonalPointCorrectionResult, err error) {
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.Points == 0 || cmd.Points == math.MinInt64 || strings.TrimSpace(cmd.Reason) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return result, ErrInvalidPointCommand
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	fingerprint := personalPointCorrectionFingerprint(cmd)
	cmd.CorrectedAt = pointNow(cmd.CorrectedAt)
	account, err := pgEnsureAccount(ctx, tx, cmd.AccountID, cmd.UserID)
	if err != nil {
		return result, err
	}
	walletKey := personalWalletKey(cmd.AccountID, "correction", cmd.IdempotencyKey)
	if idem, idemErr := pgWalletIdempotent(ctx, tx, cmd.AccountID, walletKey, fingerprint); idemErr != nil {
		return result, idemErr
	} else if idem {
		result = PersonalPointCorrectionResult{Balance: PersonalPointBalance{AccountID: account.ID, UserID: account.UserID, Available: account.Available, Frozen: account.Frozen, Total: account.Available + account.Frozen}, Points: cmd.Points, Idempotent: true}
		if cmd.Points > 0 {
			lot, lotErr := pgReadLot(ctx, tx, stablePointID("lot", cmd.AccountID, "correction:"+cmd.IdempotencyKey), cmd.AccountID, cmd.UserID, false)
			if lotErr != nil {
				return PersonalPointCorrectionResult{}, lotErr
			}
			result.Lot = &lot
		}
		return result, tx.Commit()
	}
	if err := pgExpireDueTx(ctx, tx, &account, cmd.AccountID, cmd.UserID, cmd.CorrectedAt); err != nil {
		return result, err
	}
	beforeAvailable, beforeFrozen := account.Available, account.Frozen
	amount := cmd.Points
	if cmd.Points > 0 {
		if account.Available > math.MaxInt64-cmd.Points {
			return result, ErrInvalidPointCommand
		}
		lot := PersonalPointLot{ID: stablePointID("lot", cmd.AccountID, "correction:"+cmd.IdempotencyKey), AccountID: cmd.AccountID, UserID: cmd.UserID, SourceType: PointSourceAdminCorrection, ReferenceType: "ADMIN_CORRECTION", ReferenceID: cmd.IdempotencyKey, OriginalPoints: cmd.Points, AvailablePoints: cmd.Points, GrantedAt: cmd.CorrectedAt, IdempotencyKey: "correction:" + cmd.IdempotencyKey, Status: "ACTIVE"}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,policy_version_id,policy_snapshot,idempotency_key,status) VALUES($1,$2,$3,$4,$5,$6,$7,$7,0,0,0,0,$8,NULL,NULL,'{}'::jsonb,$9,'ACTIVE')`, lot.ID, lot.AccountID, lot.UserID, lot.SourceType, lot.ReferenceType, lot.ReferenceID, lot.OriginalPoints, lot.GrantedAt, lot.IdempotencyKey); err != nil {
			return result, err
		}
		account.Available += cmd.Points
		account.TotalGranted += cmd.Points
		if err = pgInsertMovement(ctx, tx, lot, "OPENING", cmd.Points, PersonalPointLot{}, "", "correction:opening:"+cmd.IdempotencyKey, cmd.CorrectedAt); err != nil {
			return result, err
		}
		result.Lot = &lot
	} else {
		amount = -cmd.Points
		if account.Available < amount {
			return result, ErrInsufficientPoints
		}
		rows, queryErr := tx.QueryContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND available_points>0 ORDER BY expires_at ASC NULLS LAST,granted_at,id FOR UPDATE`, cmd.AccountID, cmd.UserID)
		if queryErr != nil {
			return result, queryErr
		}
		lots := []PersonalPointLot{}
		for rows.Next() {
			lot, scanErr := pgScanLot(rows)
			if scanErr != nil {
				_ = rows.Close()
				return result, scanErr
			}
			lots = append(lots, lot)
		}
		if err := rows.Close(); err != nil {
			return result, err
		}
		remaining := amount
		for _, lot := range lots {
			if remaining == 0 {
				break
			}
			debit := lot.AvailablePoints
			if debit > remaining {
				debit = remaining
			}
			before := lot
			lot.AvailablePoints -= debit
			lot.ReversedPoints += debit
			lot.Status = pgStatusLot(&lot)
			if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=$2,reversed_points=$3,status=$4,updated_at=$5 WHERE id=$1 AND account_id=$6 AND user_id=$7`, lot.ID, lot.AvailablePoints, lot.ReversedPoints, lot.Status, cmd.CorrectedAt, lot.AccountID, lot.UserID); err != nil {
				return result, err
			}
			if err = pgInsertMovement(ctx, tx, lot, "REVERSE", debit, before, "", "correction:reverse:"+cmd.IdempotencyKey+":"+lot.ID, cmd.CorrectedAt); err != nil {
				return result, err
			}
			remaining -= debit
		}
		if remaining != 0 {
			return result, ErrInsufficientPoints
		}
		account.Available -= amount
		account.TotalReversed += amount
	}
	if err = pgUpdateAccount(ctx, tx, account); err != nil {
		return result, err
	}
	if err = pgInsertWallet(ctx, tx, account, "ADJUSTMENT", amount, beforeAvailable, beforeFrozen, walletKey, "ADMIN_CORRECTION", cmd.IdempotencyKey, cmd.CorrectedAt, map[string]any{"fingerprint": fingerprint, "signed_points": cmd.Points, "reason": strings.TrimSpace(cmd.Reason), "actor_id": cmd.Audit.ActorID, "actor_role": cmd.Audit.ActorRole, "request_id": cmd.Audit.RequestID}); err != nil {
		return result, err
	}
	if err = pgInsertPersonalPointAudit(ctx, tx, cmd.Audit, cmd.AccountID, cmd.UserID, cmd.IdempotencyKey, strings.TrimSpace(cmd.Reason), cmd.Points, PointSourceAdminCorrection); err != nil {
		return result, err
	}
	result.Balance = PersonalPointBalance{AccountID: account.ID, UserID: account.UserID, Available: account.Available, Frozen: account.Frozen, Total: account.Available + account.Frozen}
	result.Points = cmd.Points
	if err = tx.Commit(); err != nil {
		return PersonalPointCorrectionResult{}, err
	}
	return result, nil
}

func lotPolicy(lot PersonalPointLot) PointExpiryPolicy {
	return PointExpiryPolicy{ID: lot.PolicyVersionID, Version: lot.PolicySnapshot.Version, Enabled: lot.PolicySnapshot.Enabled, DurationValue: lot.PolicySnapshot.DurationValue, DurationUnit: lot.PolicySnapshot.DurationUnit, TimeZone: lot.PolicySnapshot.TimeZone}
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func (s *PostgresPersonalPointStore) mergeTx(ctx context.Context, tx *sql.Tx, targetUserID, sourceUserID, mergeID string, now time.Time) (personalPointMergeResult, error) {
	result := personalPointMergeResult{}
	targetUserID = strings.TrimSpace(targetUserID)
	sourceUserID = strings.TrimSpace(sourceUserID)
	mergeID = strings.TrimSpace(mergeID)
	if tx == nil || targetUserID == "" || sourceUserID == "" || targetUserID == sourceUserID || mergeID == "" {
		return result, ErrInvalidPointCommand
	}
	now = pointNow(now)
	rows, err := tx.QueryContext(ctx, `SELECT id,COALESCE(user_id,''),available,frozen,COALESCE(NULLIF(raw->>'totalGranted','')::bigint,0),COALESCE(NULLIF(raw->>'totalUsed','')::bigint,0),COALESCE(NULLIF(raw->>'totalExpired','')::bigint,0),COALESCE(NULLIF(raw->>'totalReversed','')::bigint,0) FROM xz_point_accounts WHERE user_id IN($1,$2) ORDER BY id FOR UPDATE`, targetUserID, sourceUserID)
	if err != nil {
		return result, err
	}
	accounts := map[string]pgPointAccount{}
	for rows.Next() {
		var account pgPointAccount
		if err := rows.Scan(&account.ID, &account.UserID, &account.Available, &account.Frozen, &account.TotalGranted, &account.TotalConsumed, &account.TotalExpired, &account.TotalReversed); err != nil {
			_ = rows.Close()
			return result, err
		}
		if _, exists := accounts[account.UserID]; exists {
			_ = rows.Close()
			return result, ErrPointOwnership
		}
		accounts[account.UserID] = account
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	source, sourceExists := accounts[sourceUserID]
	target, targetExists := accounts[targetUserID]
	if (sourceExists && source.Frozen > 0) || (targetExists && target.Frozen > 0) {
		return result, ErrPersonalPointMergeActiveReservation
	}
	var activeReservations bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_personal_point_reservations WHERE user_id IN($1,$2) AND reserved_points>0)`, targetUserID, sourceUserID).Scan(&activeReservations); err != nil {
		return result, err
	}
	if activeReservations {
		return result, ErrPersonalPointMergeActiveReservation
	}
	if !sourceExists {
		return result, nil
	}
	if !targetExists {
		target, err = pgEnsureAccount(ctx, tx, stablePointID("account", targetUserID, "auth-merge:"+mergeID), targetUserID)
		if err != nil {
			return result, err
		}
	}
	ordered := []*pgPointAccount{&target, &source}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, account := range ordered {
		if err := pgExpireDueTx(ctx, tx, account, account.ID, account.UserID, now); err != nil {
			return result, err
		}
	}

	lotRows, err := tx.QueryContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND available_points>0 ORDER BY id FOR UPDATE`, source.ID, source.UserID)
	if err != nil {
		return result, err
	}
	lots := []PersonalPointLot{}
	for lotRows.Next() {
		lot, scanErr := pgScanLot(lotRows)
		if scanErr != nil {
			_ = lotRows.Close()
			return result, scanErr
		}
		lots = append(lots, lot)
	}
	if err := lotRows.Close(); err != nil {
		return result, err
	}
	targetBefore, sourceBefore := target.Available, source.Available
	for _, lot := range lots {
		amount := lot.AvailablePoints
		before := lot
		lot.AvailablePoints = 0
		lot.ReversedPoints += amount
		lot.Status = pgStatusLot(&lot)
		if _, err := tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=0,reversed_points=$2,status=$3,updated_at=$4 WHERE id=$1 AND account_id=$5 AND user_id=$6`, lot.ID, lot.ReversedPoints, lot.Status, now, source.ID, source.UserID); err != nil {
			return result, err
		}
		if err := pgInsertMovement(ctx, tx, lot, "REVERSE", amount, before, "", "auth-merge:reverse:"+mergeID+":"+lot.ID, now); err != nil {
			return result, err
		}
		transferKey := "auth-merge:" + mergeID + ":" + lot.ID
		transferred := before
		transferred.ID = stablePointID("lot", target.ID, transferKey)
		transferred.AccountID = target.ID
		transferred.UserID = target.UserID
		transferred.OriginalPoints = amount
		transferred.AvailablePoints = amount
		transferred.ReservedPoints = 0
		transferred.ConsumedPoints = 0
		transferred.ExpiredPoints = 0
		transferred.ReversedPoints = 0
		transferred.IdempotencyKey = transferKey
		transferred.Status = pgStatusLot(&transferred)
		policySnapshot, err := json.Marshal(transferred.PolicySnapshot)
		if err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,granted_at,expires_at,policy_version_id,policy_snapshot,idempotency_key,status,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7,0,0,0,0,$8,$9,NULLIF($10,'')::text,$11,$12,$13,$14)`, transferred.ID, transferred.AccountID, transferred.UserID, transferred.SourceType, transferred.ReferenceType, transferred.ReferenceID, amount, transferred.GrantedAt, nullTime(transferred.ExpiresAt), transferred.PolicyVersionID, policySnapshot, transferred.IdempotencyKey, transferred.Status, now); err != nil {
			return result, err
		}
		if err := pgInsertMovement(ctx, tx, transferred, "OPENING", amount, PersonalPointLot{}, "", "auth-merge:opening:"+mergeID+":"+lot.ID, now); err != nil {
			return result, err
		}
		result.PointsMoved += amount
	}
	source.Available -= result.PointsMoved
	source.TotalReversed += result.PointsMoved
	target.Available += result.PointsMoved
	target.TotalGranted += result.PointsMoved
	ordered = []*pgPointAccount{&target, &source}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, account := range ordered {
		if err := pgUpdateAccount(ctx, tx, *account); err != nil {
			return result, err
		}
	}
	if result.PointsMoved > 0 {
		if err := pgInsertWallet(ctx, tx, source, "ADJUSTMENT", result.PointsMoved, sourceBefore, source.Frozen, personalWalletKey(source.ID, "auth-merge-out", mergeID), "AUTH_MERGE", mergeID, now, map[string]any{"direction": "OUT", "target_user_id": targetUserID}); err != nil {
			return result, err
		}
		if err := pgInsertWallet(ctx, tx, target, "ADJUSTMENT", result.PointsMoved, targetBefore, target.Frozen, personalWalletKey(target.ID, "auth-merge-in", mergeID), "AUTH_MERGE", mergeID, now, map[string]any{"direction": "IN", "source_user_id": sourceUserID}); err != nil {
			return result, err
		}
	}
	result.AccountsMoved = 1
	return result, nil
}

func (s *PostgresPersonalPointStore) grantRegistration(ctx context.Context, cmd PersonalPointRegistrationGrantCommand) (PersonalPointGrantResult, error) {
	if cmd.PlanGrantPoints <= 0 {
		return PersonalPointGrantResult{}, ErrInvalidPointCommand
	}
	return s.grant(ctx, PersonalPointGrantCommand{AccountID: cmd.AccountID, UserID: cmd.UserID, Source: PointSourceRegistrationGift, Points: cmd.PlanGrantPoints, ReferenceType: "PLAN", ReferenceID: cmd.PlanID, IdempotencyKey: cmd.IdempotencyKey, GrantedAt: cmd.GrantedAt})
}

func (s *PostgresPersonalPointStore) getBalance(ctx context.Context, accountID, userID string) (PersonalPointBalance, error) {
	if accountID == "" || userID == "" {
		return PersonalPointBalance{}, ErrInvalidPointCommand
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PersonalPointBalance{}, err
	}
	defer func() { _ = tx.Rollback() }()
	account, ok, err := pgLoadAccount(ctx, tx, accountID, userID, true)
	if err != nil {
		return PersonalPointBalance{}, err
	}
	if !ok {
		if err := tx.Commit(); err != nil {
			return PersonalPointBalance{}, err
		}
		return PersonalPointBalance{AccountID: accountID, UserID: userID}, nil
	}
	if err := pgExpireDueTx(ctx, tx, &account, accountID, userID, time.Now().UTC()); err != nil {
		return PersonalPointBalance{}, err
	}
	if err := tx.Commit(); err != nil {
		return PersonalPointBalance{}, err
	}
	return PersonalPointBalance{AccountID: accountID, UserID: userID, Available: account.Available, Frozen: account.Frozen, Total: account.Available + account.Frozen}, nil
}

type pgReservationRow struct{ PersonalPointReservation }

func pgScanReservation(scanner interface{ Scan(...any) error }) (PersonalPointReservation, error) {
	var r PersonalPointReservation
	err := scanner.Scan(&r.ID, &r.AccountID, &r.UserID, &r.BusinessType, &r.BusinessID, &r.RequestedPoints, &r.ReservedPoints, &r.CapturedPoints, &r.ReleasedPoints, &r.ExpiredPoints, &r.IdempotencyKey, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err == nil {
		r.CreatedAt, r.UpdatedAt = r.CreatedAt.UTC(), r.UpdatedAt.UTC()
	}
	return r, err
}

const pgReservationColumns = `id,account_id,user_id,business_type,business_id,requested_points,reserved_points,captured_points,released_points,expired_points,idempotency_key,status,created_at,updated_at`

func pgReadReservation(ctx context.Context, tx *sql.Tx, id, accountID, userID string, forUpdate bool) (PersonalPointReservation, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	r, err := pgScanReservation(tx.QueryRowContext(ctx, `SELECT `+pgReservationColumns+` FROM xz_personal_point_reservations WHERE id=$1 AND account_id=$2 AND user_id=$3`+lock, id, accountID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrPointNotFound
	}
	return r, err
}

func pgScanAllocation(scanner interface{ Scan(...any) error }) (PersonalPointAllocation, error) {
	var a PersonalPointAllocation
	var source string
	err := scanner.Scan(&a.ID, &a.ReservationID, &a.LotID, &a.AccountID, &a.UserID, &a.AllocatedPoints, &a.ReservedPoints, &a.CapturedPoints, &a.ReleasedPoints, &a.ExpiredPoints, &a.Status, &source)
	a.SourceType = PointSource(source)
	return a, err
}

func pgLoadAllocations(ctx context.Context, tx *sql.Tx, reservationID, accountID, userID string, forUpdate bool) ([]PersonalPointAllocation, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE OF a"
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.reservation_id,a.lot_id,a.account_id,a.user_id,a.allocated_points,a.reserved_points,a.captured_points,a.released_points,a.expired_points,a.status,l.source_type FROM xz_personal_point_reservation_allocations a JOIN xz_personal_point_lots l ON l.id=a.lot_id AND l.account_id=a.account_id AND l.user_id=a.user_id WHERE a.reservation_id=$1 AND a.account_id=$2 AND a.user_id=$3 ORDER BY a.created_at,a.id`+lock, reservationID, accountID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PersonalPointAllocation
	for rows.Next() {
		item, scanErr := pgScanAllocation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func pgLockAllocationLots(ctx context.Context, tx *sql.Tx, allocations []PersonalPointAllocation, accountID, userID string) (map[string]PersonalPointLot, error) {
	lotIDs := make([]string, 0, len(allocations))
	seen := make(map[string]struct{}, len(allocations))
	for _, allocation := range allocations {
		if _, ok := seen[allocation.LotID]; ok {
			continue
		}
		seen[allocation.LotID] = struct{}{}
		lotIDs = append(lotIDs, allocation.LotID)
	}
	sort.Strings(lotIDs)
	locked := make(map[string]PersonalPointLot, len(lotIDs))
	for _, lotID := range lotIDs {
		lot, err := pgReadLot(ctx, tx, lotID, accountID, userID, true)
		if err != nil {
			return nil, err
		}
		locked[lotID] = lot
	}
	return locked, nil
}

func (s *PostgresPersonalPointStore) reserve(ctx context.Context, cmd PersonalPointReserveCommand) (result PersonalPointReserveResult, err error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err = s.reserveTx(ctx, tx, cmd)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return PersonalPointReserveResult{}, err
	}
	return result, nil
}

func (s *PostgresPersonalPointStore) reserveTx(ctx context.Context, tx *sql.Tx, cmd PersonalPointReserveCommand) (result PersonalPointReserveResult, err error) {
	if tx == nil {
		return result, ErrInvalidPointCommand
	}
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.BusinessType == "" || cmd.BusinessID == "" || cmd.IdempotencyKey == "" || cmd.RequestedPoints <= 0 {
		return result, ErrInvalidPointCommand
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.ReservedAt = pointNow(cmd.ReservedAt)
	account, ok, err := pgLoadAccount(ctx, tx, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrInsufficientPoints
	}
	walletKey := personalWalletKey(cmd.AccountID, "reserve", cmd.IdempotencyKey)
	if idem, idemErr := pgWalletIdempotent(ctx, tx, cmd.AccountID, walletKey, fingerprint); idemErr != nil {
		return result, idemErr
	} else if idem {
		reservation, rErr := pgReadReservation(ctx, tx, stablePointID("reservation", cmd.AccountID, cmd.IdempotencyKey), cmd.AccountID, cmd.UserID, false)
		if rErr != nil {
			return result, rErr
		}
		allocations, aErr := pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, false)
		if aErr != nil {
			return result, aErr
		}
		return PersonalPointReserveResult{Reservation: reservation, Allocations: allocations, Idempotent: true}, nil
	}
	var existing PersonalPointReservation
	existing, scanErr := pgScanReservation(tx.QueryRowContext(ctx, `SELECT `+pgReservationColumns+` FROM xz_personal_point_reservations WHERE account_id=$1 AND idempotency_key=$2`, cmd.AccountID, cmd.IdempotencyKey))
	if scanErr == nil {
		if existing.UserID != cmd.UserID || existing.BusinessType != cmd.BusinessType || existing.BusinessID != cmd.BusinessID || existing.RequestedPoints != cmd.RequestedPoints {
			return result, ErrIdempotencyConflict
		}
		allocations, allocErr := pgLoadAllocations(ctx, tx, existing.ID, cmd.AccountID, cmd.UserID, false)
		if allocErr != nil {
			return result, allocErr
		}
		return PersonalPointReserveResult{Reservation: existing, Allocations: allocations, Idempotent: true}, nil
	}
	if !errors.Is(scanErr, sql.ErrNoRows) {
		return result, scanErr
	}
	businessExisting, businessErr := pgScanReservation(tx.QueryRowContext(ctx, `SELECT `+pgReservationColumns+` FROM xz_personal_point_reservations WHERE account_id=$1 AND business_type=$2 AND business_id=$3`, cmd.AccountID, cmd.BusinessType, cmd.BusinessID))
	if businessErr == nil {
		if businessExisting.UserID != cmd.UserID || businessExisting.IdempotencyKey != cmd.IdempotencyKey {
			return result, ErrIdempotencyConflict
		}
	}
	if businessErr != nil && !errors.Is(businessErr, sql.ErrNoRows) {
		return result, businessErr
	}
	if err = pgExpireDueTx(ctx, tx, &account, cmd.AccountID, cmd.UserID, cmd.ReservedAt); err != nil {
		return result, err
	}
	if account.Available < cmd.RequestedPoints {
		return result, ErrInsufficientPoints
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND available_points>0 AND status IN ('ACTIVE','LEGACY') ORDER BY expires_at ASC NULLS LAST, granted_at ASC, id FOR UPDATE`, cmd.AccountID, cmd.UserID)
	if err != nil {
		return result, err
	}
	type selectedLot struct {
		lot    PersonalPointLot
		amount int64
	}
	var selected []selectedLot
	remaining := cmd.RequestedPoints
	for rows.Next() {
		lot, scanErr := pgScanLot(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		amount := lot.AvailablePoints
		if amount > remaining {
			amount = remaining
		}
		selected = append(selected, selectedLot{lot: lot, amount: amount})
		remaining -= amount
		if remaining == 0 {
			break
		}
	}
	rows.Close()
	if remaining != 0 {
		return result, ErrInsufficientPoints
	}
	reservation := PersonalPointReservation{ID: stablePointID("reservation", cmd.AccountID, cmd.IdempotencyKey), AccountID: cmd.AccountID, UserID: cmd.UserID, BusinessType: cmd.BusinessType, BusinessID: cmd.BusinessID, RequestedPoints: cmd.RequestedPoints, ReservedPoints: cmd.RequestedPoints, IdempotencyKey: cmd.IdempotencyKey, Status: "RESERVED", CreatedAt: cmd.ReservedAt, UpdatedAt: cmd.ReservedAt}
	if _, err = tx.ExecContext(ctx, `INSERT INTO xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,captured_points,released_points,expired_points,idempotency_key,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$6,0,0,0,$7,'RESERVED',$8,$8)`, reservation.ID, reservation.AccountID, reservation.UserID, reservation.BusinessType, reservation.BusinessID, reservation.RequestedPoints, reservation.IdempotencyKey, reservation.CreatedAt); err != nil {
		return result, err
	}
	beforeAvailable, beforeFrozen := account.Available, account.Frozen
	for _, selected := range selected {
		lot := selected.lot
		amount := selected.amount
		before := lot
		lot.AvailablePoints -= amount
		lot.ReservedPoints += amount
		lot.Status = pgStatusLot(&lot)
		if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=$2,reserved_points=$3,status=$4,updated_at=$5 WHERE id=$1 AND account_id=$6 AND user_id=$7`, lot.ID, lot.AvailablePoints, lot.ReservedPoints, lot.Status, cmd.ReservedAt, lot.AccountID, lot.UserID); err != nil {
			return result, err
		}
		allocation := PersonalPointAllocation{ID: stablePointID("allocation", reservation.ID, lot.ID), ReservationID: reservation.ID, LotID: lot.ID, AccountID: cmd.AccountID, UserID: cmd.UserID, SourceType: lot.SourceType, AllocatedPoints: amount, ReservedPoints: amount, Status: "RESERVED"}
		if _, err = tx.ExecContext(ctx, `INSERT INTO xz_personal_point_reservation_allocations(id,reservation_id,lot_id,account_id,user_id,allocated_points,reserved_points,captured_points,released_points,expired_points,status,metadata,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$6,0,0,0,'RESERVED','{}'::jsonb,$7,$7)`, allocation.ID, allocation.ReservationID, allocation.LotID, allocation.AccountID, allocation.UserID, amount, cmd.ReservedAt); err != nil {
			return result, err
		}
		if err = pgInsertMovement(ctx, tx, lot, "RESERVE", amount, before, reservation.ID, "reserve:"+cmd.IdempotencyKey+":"+lot.ID, cmd.ReservedAt); err != nil {
			return result, err
		}
		result.Allocations = append(result.Allocations, allocation)
	}
	account.Available -= cmd.RequestedPoints
	account.Frozen += cmd.RequestedPoints
	if err = pgUpdateAccount(ctx, tx, account); err != nil {
		return result, err
	}
	if err = pgInsertWallet(ctx, tx, account, "RESERVE", cmd.RequestedPoints, beforeAvailable, beforeFrozen, walletKey, cmd.BusinessType, cmd.BusinessID, cmd.ReservedAt, map[string]any{"fingerprint": fingerprint}); err != nil {
		return result, err
	}
	result.Reservation = reservation
	return result, nil
}

func (s *PostgresPersonalPointStore) capture(ctx context.Context, cmd PersonalPointCaptureCommand) (result PersonalPointMutationResult, err error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err = s.captureTx(ctx, tx, cmd)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return PersonalPointMutationResult{}, err
	}
	return result, nil
}

func (s *PostgresPersonalPointStore) captureTx(ctx context.Context, tx *sql.Tx, cmd PersonalPointCaptureCommand) (result PersonalPointMutationResult, err error) {
	if tx == nil {
		return result, ErrInvalidPointCommand
	}
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.ReservationID == "" || cmd.IdempotencyKey == "" || cmd.Points <= 0 {
		return result, ErrInvalidPointCommand
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.CapturedAt = pointNow(cmd.CapturedAt)
	account, ok, err := pgLoadAccount(ctx, tx, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrPointNotFound
	}
	walletKey := personalWalletKey(cmd.AccountID, "capture", cmd.IdempotencyKey)
	if idem, idemErr := pgWalletIdempotent(ctx, tx, cmd.AccountID, walletKey, fingerprint); idemErr != nil {
		return result, idemErr
	} else if idem {
		reservation, rErr := pgReadReservation(ctx, tx, cmd.ReservationID, cmd.AccountID, cmd.UserID, false)
		if rErr != nil {
			return result, rErr
		}
		allocations, aErr := pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, false)
		if aErr != nil {
			return result, aErr
		}
		return PersonalPointMutationResult{Reservation: reservation, Allocations: allocations, Idempotent: true}, nil
	}
	reservation, err := pgReadReservation(ctx, tx, cmd.ReservationID, cmd.AccountID, cmd.UserID, false)
	if err != nil {
		return result, err
	}
	if reservation.ReservedPoints < cmd.Points {
		return result, ErrInsufficientPoints
	}
	allocations, err := pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, false)
	if err != nil {
		return result, err
	}
	lockedLots, err := pgLockAllocationLots(ctx, tx, allocations, cmd.AccountID, cmd.UserID)
	if err != nil {
		return result, err
	}
	reservation, err = pgReadReservation(ctx, tx, cmd.ReservationID, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	allocations, err = pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	remaining := cmd.Points
	for i := range allocations {
		a := &allocations[i]
		if a.ReservedPoints <= 0 || remaining == 0 {
			continue
		}
		amount := a.ReservedPoints
		if amount > remaining {
			amount = remaining
		}
		lot, lotOK := lockedLots[a.LotID]
		if !lotOK {
			return result, ErrPointNotFound
		}
		before := lot
		lot.ReservedPoints -= amount
		lot.ConsumedPoints += amount
		lot.Status = pgStatusLot(&lot)
		if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET reserved_points=$2,consumed_points=$3,status=$4,updated_at=$5 WHERE id=$1`, lot.ID, lot.ReservedPoints, lot.ConsumedPoints, lot.Status, cmd.CapturedAt); err != nil {
			return result, err
		}
		a.ReservedPoints -= amount
		a.CapturedPoints += amount
		a.Status = pgStatusAllocation(a)
		if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_reservation_allocations SET reserved_points=$2,captured_points=$3,status=$4,updated_at=$5 WHERE id=$1`, a.ID, a.ReservedPoints, a.CapturedPoints, a.Status, cmd.CapturedAt); err != nil {
			return result, err
		}
		if err = pgInsertMovement(ctx, tx, lot, "CAPTURE", amount, before, reservation.ID, "capture:"+cmd.IdempotencyKey+":"+lot.ID, cmd.CapturedAt); err != nil {
			return result, err
		}
		remaining -= amount
	}
	if remaining != 0 {
		return result, ErrInsufficientPoints
	}
	beforeAvailable, beforeFrozen := account.Available, account.Frozen
	reservation.ReservedPoints -= cmd.Points
	reservation.CapturedPoints += cmd.Points
	reservation.UpdatedAt = cmd.CapturedAt
	reservation.Status = pgStatusReservation(&reservation)
	if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_reservations SET reserved_points=$2,captured_points=$3,status=$4,updated_at=$5 WHERE id=$1`, reservation.ID, reservation.ReservedPoints, reservation.CapturedPoints, reservation.Status, reservation.UpdatedAt); err != nil {
		return result, err
	}
	account.Frozen -= cmd.Points
	account.TotalConsumed += cmd.Points
	if err = pgUpdateAccount(ctx, tx, account); err != nil {
		return result, err
	}
	if err = pgInsertWallet(ctx, tx, account, "CAPTURE", cmd.Points, beforeAvailable, beforeFrozen, walletKey, reservation.BusinessType, reservation.BusinessID, cmd.CapturedAt, map[string]any{"fingerprint": fingerprint}); err != nil {
		return result, err
	}
	result.Reservation, result.Allocations = reservation, allocations
	return result, nil
}

func (s *PostgresPersonalPointStore) release(ctx context.Context, cmd PersonalPointReleaseCommand) (result PersonalPointMutationResult, err error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err = s.releaseTx(ctx, tx, cmd)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(); err != nil {
		return PersonalPointMutationResult{}, err
	}
	return result, nil
}

func (s *PostgresPersonalPointStore) releaseTx(ctx context.Context, tx *sql.Tx, cmd PersonalPointReleaseCommand) (result PersonalPointMutationResult, err error) {
	if tx == nil {
		return result, ErrInvalidPointCommand
	}
	if cmd.AccountID == "" || cmd.UserID == "" || cmd.ReservationID == "" || cmd.IdempotencyKey == "" {
		return result, ErrInvalidPointCommand
	}
	fingerprint := pointCommandFingerprint(cmd)
	cmd.ReleasedAt = pointNow(cmd.ReleasedAt)
	account, ok, err := pgLoadAccount(ctx, tx, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	if !ok {
		return result, ErrPointNotFound
	}
	walletKey := personalWalletKey(cmd.AccountID, "release", cmd.IdempotencyKey)
	if idem, idemErr := pgWalletIdempotent(ctx, tx, cmd.AccountID, walletKey, fingerprint); idemErr != nil {
		return result, idemErr
	} else if idem {
		reservation, rErr := pgReadReservation(ctx, tx, cmd.ReservationID, cmd.AccountID, cmd.UserID, false)
		if rErr != nil {
			return result, rErr
		}
		allocations, aErr := pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, false)
		if aErr != nil {
			return result, aErr
		}
		return PersonalPointMutationResult{Reservation: reservation, Allocations: allocations, Idempotent: true}, nil
	}
	reservation, err := pgReadReservation(ctx, tx, cmd.ReservationID, cmd.AccountID, cmd.UserID, false)
	if err != nil {
		return result, err
	}
	amountTotal := cmd.Points
	if amountTotal == 0 {
		amountTotal = reservation.ReservedPoints
	}
	if amountTotal <= 0 || reservation.ReservedPoints < amountTotal {
		return result, ErrInvalidPointCommand
	}
	allocations, err := pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, false)
	if err != nil {
		return result, err
	}
	lockedLots, err := pgLockAllocationLots(ctx, tx, allocations, cmd.AccountID, cmd.UserID)
	if err != nil {
		return result, err
	}
	reservation, err = pgReadReservation(ctx, tx, cmd.ReservationID, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	allocations, err = pgLoadAllocations(ctx, tx, reservation.ID, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return result, err
	}
	remaining := amountTotal
	type expiredLot struct {
		lot    PersonalPointLot
		amount int64
	}
	var expiredLots []expiredLot
	for i := range allocations {
		a := &allocations[i]
		if a.ReservedPoints <= 0 || remaining == 0 {
			continue
		}
		amount := a.ReservedPoints
		if amount > remaining {
			amount = remaining
		}
		lot, lotOK := lockedLots[a.LotID]
		if !lotOK {
			return result, ErrPointNotFound
		}
		before := lot
		lot.ReservedPoints -= amount
		lot.AvailablePoints += amount
		lot.Status = pgStatusLot(&lot)
		releaseLot := lot
		expireAmount := int64(0)
		expireBefore := PersonalPointLot{}
		if !lot.ExpiresAt.IsZero() && !lot.ExpiresAt.After(cmd.ReleasedAt) && lot.AvailablePoints > 0 {
			expireAmount = lot.AvailablePoints
			expireBefore = lot
			lot.AvailablePoints = 0
			lot.ExpiredPoints += expireAmount
			lot.Status = pgStatusLot(&lot)
			expiredLots = append(expiredLots, expiredLot{lot: lot, amount: expireAmount})
		}
		if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=$2,reserved_points=$3,expired_points=$4,status=$5,updated_at=$6 WHERE id=$1`, lot.ID, lot.AvailablePoints, lot.ReservedPoints, lot.ExpiredPoints, lot.Status, cmd.ReleasedAt); err != nil {
			return result, err
		}
		a.ReservedPoints -= amount
		a.ReleasedPoints += amount
		a.Status = pgStatusAllocation(a)
		if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_reservation_allocations SET reserved_points=$2,released_points=$3,status=$4,updated_at=$5 WHERE id=$1`, a.ID, a.ReservedPoints, a.ReleasedPoints, a.Status, cmd.ReleasedAt); err != nil {
			return result, err
		}
		if err = pgInsertMovement(ctx, tx, releaseLot, "RELEASE", amount, before, reservation.ID, "release:"+cmd.IdempotencyKey+":"+lot.ID, cmd.ReleasedAt); err != nil {
			return result, err
		}
		if expireAmount > 0 {
			if err = pgInsertMovement(ctx, tx, lot, "EXPIRE", expireAmount, expireBefore, reservation.ID, "expire:"+lot.ID+":"+reservation.ID+":"+cmd.IdempotencyKey, cmd.ReleasedAt); err != nil {
				return result, err
			}
		}
		remaining -= amount
	}
	if remaining != 0 {
		return result, ErrInvalidPointCommand
	}
	beforeAvailable, beforeFrozen := account.Available, account.Frozen
	reservation.ReservedPoints -= amountTotal
	reservation.ReleasedPoints += amountTotal
	reservation.UpdatedAt = cmd.ReleasedAt
	reservation.Status = pgStatusReservation(&reservation)
	if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_reservations SET reserved_points=$2,released_points=$3,status=$4,updated_at=$5 WHERE id=$1`, reservation.ID, reservation.ReservedPoints, reservation.ReleasedPoints, reservation.Status, reservation.UpdatedAt); err != nil {
		return result, err
	}
	account.Frozen -= amountTotal
	account.Available += amountTotal
	if err = pgInsertWallet(ctx, tx, account, "RELEASE", amountTotal, beforeAvailable, beforeFrozen, walletKey, reservation.BusinessType, reservation.BusinessID, cmd.ReleasedAt, map[string]any{"fingerprint": fingerprint}); err != nil {
		return result, err
	}
	for _, expired := range expiredLots {
		beforeExpireAvailable := account.Available
		account.Available -= expired.amount
		account.TotalExpired += expired.amount
		if err = pgInsertWallet(ctx, tx, account, "EXPIRE", expired.amount, beforeExpireAvailable, account.Frozen, personalWalletKey(cmd.AccountID, "expire", expired.lot.ID+":"+reservation.ID+":"+cmd.IdempotencyKey), expired.lot.ReferenceType, expired.lot.ReferenceID, cmd.ReleasedAt, map[string]any{"source": "release_after_expiry"}); err != nil {
			return result, err
		}
	}
	if err = pgUpdateAccount(ctx, tx, account); err != nil {
		return result, err
	}
	result.Reservation, result.Allocations = reservation, allocations
	return result, nil
}

func pgWalletIdempotent(ctx context.Context, tx *sql.Tx, accountID, key, fingerprint string) (bool, error) {
	var metadata []byte
	err := tx.QueryRowContext(ctx, `SELECT metadata FROM xz_wallet_ledger WHERE account_id=$1 AND idempotency_key=$2`, accountID, key).Scan(&metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var data map[string]any
	_ = json.Unmarshal(metadata, &data)
	if prior, ok := data["fingerprint"].(string); ok && prior != fingerprint {
		return false, ErrIdempotencyConflict
	}
	return true, nil
}

func pgExpireDueTx(ctx context.Context, tx *sql.Tx, account *pgPointAccount, accountID, userID string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND expires_at IS NOT NULL AND expires_at <= $3 AND available_points>0 ORDER BY expires_at,granted_at,id FOR UPDATE`, accountID, userID, now)
	if err != nil {
		return err
	}
	defer rows.Close()
	var lots []PersonalPointLot
	for rows.Next() {
		lot, scanErr := pgScanLot(rows)
		if scanErr != nil {
			return scanErr
		}
		lots = append(lots, lot)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range lots {
		lot, err := pgReadLot(ctx, tx, item.ID, accountID, userID, true)
		if err != nil {
			return err
		}
		if lot.AvailablePoints > 0 {
			amount := lot.AvailablePoints
			before := lot
			lot.AvailablePoints = 0
			lot.ExpiredPoints += amount
			lot.Status = pgStatusLot(&lot)
			if _, err = tx.ExecContext(ctx, `UPDATE xz_personal_point_lots SET available_points=0,expired_points=$2,status=$3,updated_at=$4 WHERE id=$1`, lot.ID, lot.ExpiredPoints, lot.Status, now); err != nil {
				return err
			}
			account.Available -= amount
			account.TotalExpired += amount
			if err = pgInsertMovement(ctx, tx, lot, "EXPIRE", amount, before, "", "expire:"+lot.ID, now); err != nil {
				return err
			}
			if err = pgInsertWallet(ctx, tx, *account, "EXPIRE", amount, account.Available+amount, account.Frozen, personalWalletKey(accountID, "expire", lot.ID), lot.ReferenceType, lot.ReferenceID, now, nil); err != nil {
				return err
			}
		}
	}
	return pgUpdateAccount(ctx, tx, *account)
}

func (s *PostgresPersonalPointStore) expire(ctx context.Context, cmd PersonalPointExpiryCommand) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.expireTx(ctx, tx, cmd); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresPersonalPointStore) expireTx(ctx context.Context, tx *sql.Tx, cmd PersonalPointExpiryCommand) error {
	if tx == nil {
		return ErrInvalidPointCommand
	}
	if cmd.AccountID == "" || cmd.UserID == "" {
		return ErrInvalidPointCommand
	}
	now := pointNow(cmd.Now)
	account, ok, err := pgLoadAccount(ctx, tx, cmd.AccountID, cmd.UserID, true)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err = pgExpireDueTx(ctx, tx, &account, cmd.AccountID, cmd.UserID, now); err != nil {
		return err
	}
	return nil
}

func (s *PostgresPersonalPointStore) movementCount(ctx context.Context, accountID, userID, movementType string) int {
	if s == nil || s.db == nil {
		return 0
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id=$1 AND movement_type=$2`, accountID, movementType).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

func (s *PostgresPersonalPointStore) listLots(ctx context.Context, accountID, userID string, filter PersonalPointLotFilter) ([]PersonalPointLot, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidPointCommand
	}
	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(user_id,'') FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return []PersonalPointLot{}, nil
	}
	if err != nil {
		return nil, err
	}
	if owner != userID {
		return nil, ErrPointOwnership
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+pgLotColumns+` FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND ($3='' OR source_type=$3) AND ($4='' OR status=$4) ORDER BY expires_at ASC NULLS LAST,granted_at ASC,id LIMIT $5 OFFSET $6`, accountID, userID, string(filter.Source), strings.ToUpper(strings.TrimSpace(filter.Status)), limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PersonalPointLot, 0)
	for rows.Next() {
		lot, err := pgScanLot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, lot)
	}
	return items, rows.Err()
}

func (s *PostgresPersonalPointStore) summary(ctx context.Context, accountID, userID string, now time.Time) (PersonalPointBalanceSummary, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return PersonalPointBalanceSummary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	account, found, err := pgLoadAccount(ctx, tx, accountID, userID, true)
	if err != nil {
		return PersonalPointBalanceSummary{}, err
	}
	summary := PersonalPointBalanceSummary{PersonalPointBalance: PersonalPointBalance{AccountID: accountID, UserID: userID}}
	if !found {
		return summary, tx.Commit()
	}
	if err := pgExpireDueTx(ctx, tx, &account, accountID, userID, now); err != nil {
		return PersonalPointBalanceSummary{}, err
	}
	summary.Available, summary.Frozen = account.Available, account.Frozen
	summary.Total = summary.Available + summary.Frozen
	var nextExpiry sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(available_points) FILTER (WHERE expires_at IS NULL),0),COALESCE(sum(available_points) FILTER (WHERE expires_at IS NOT NULL),0),min(expires_at) FILTER (WHERE available_points>0) FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND available_points>0`, accountID, userID).Scan(&summary.PermanentAvailable, &summary.ExpiringAvailable, &nextExpiry); err != nil {
		return PersonalPointBalanceSummary{}, err
	}
	if nextExpiry.Valid {
		summary.NextExpiryAt = nextExpiry.Time.UTC()
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(available_points),0) FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND available_points>0 AND expires_at=$3`, accountID, userID, nextExpiry.Time).Scan(&summary.NextExpiryPoints); err != nil {
			return PersonalPointBalanceSummary{}, err
		}
	}
	return summary, tx.Commit()
}

func (s *PostgresPersonalPointStore) expireDue(ctx context.Context, now time.Time, limit int) (PersonalPointExpiryBatchResult, error) {
	result := PersonalPointExpiryBatchResult{}
	if s == nil || s.db == nil {
		return result, ErrInvalidPointCommand
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT account_id,user_id FROM xz_personal_point_lots WHERE expires_at IS NOT NULL AND expires_at <= $1 AND available_points>0 AND status='ACTIVE' ORDER BY account_id LIMIT $2`, pointNow(now), limit)
	if err != nil {
		return result, err
	}
	type owner struct{ accountID, userID string }
	owners := make([]owner, 0, limit)
	for rows.Next() {
		var item owner
		if err := rows.Scan(&item.accountID, &item.userID); err != nil {
			_ = rows.Close()
			return result, err
		}
		owners = append(owners, item)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	for _, item := range owners {
		var before int64
		if err := s.db.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1 AND user_id=$2`, item.accountID, item.userID).Scan(&before); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return result, err
		}
		if err := s.expire(ctx, PersonalPointExpiryCommand{AccountID: item.accountID, UserID: item.userID, Now: now}); err != nil {
			return result, err
		}
		var after int64
		if err := s.db.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1 AND user_id=$2`, item.accountID, item.userID).Scan(&after); err != nil {
			return result, err
		}
		if after < before {
			result.AccountsProcessed++
			result.PointsExpired += before - after
		}
	}
	return result, nil
}
