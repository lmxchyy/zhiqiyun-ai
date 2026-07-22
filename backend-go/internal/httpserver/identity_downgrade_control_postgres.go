package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type downgradeControlRow struct {
	id, userID, status, actorID, actorRole string
	effectiveAt, createdAt                 time.Time
	request                                identityDowngradeRequest
}

func loadDowngradeControlForUpdate(ctx context.Context, tx *sql.Tx, userID, requestID string) (downgradeControlRow, error) {
	var item downgradeControlRow
	var requestJSON []byte
	err := tx.QueryRowContext(ctx, `
		SELECT request.id,request.user_id,request.status,request.actor_id,preview.actor_role,
		       request.effective_at,request.created_at,preview.request_snapshot
		FROM xz_identity_downgrade_requests request
		JOIN xz_identity_downgrade_previews preview ON preview.id=request.preview_id
		WHERE request.id=$1 AND request.user_id=$2 FOR UPDATE OF request
	`, requestID, userID).Scan(&item.id, &item.userID, &item.status, &item.actorID, &item.actorRole, &item.effectiveAt, &item.createdAt, &requestJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errIdentityDowngradeNotFound
	}
	if err != nil {
		return item, err
	}
	if err := json.Unmarshal(requestJSON, &item.request); err != nil {
		return item, err
	}
	return item, nil
}

func authorizeDowngradeControl(actorID, actorRole string) error {
	if strings.TrimSpace(actorID) == "" || !strings.EqualFold(strings.TrimSpace(actorRole), "SUPER_ADMIN") {
		return errIdentityDowngradePermission
	}
	return nil
}

func (s *postgresStore) RecheckAdminIdentityDowngrade(actorID, actorRole, userID, requestID string) (identityDowngradeResult, error) {
	if err := authorizeDowngradeControl(actorID, actorRole); err != nil {
		return identityDowngradeResult{}, err
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return identityDowngradeResult{}, err
	}
	defer tx.Rollback()
	item, err := loadDowngradeControlForUpdate(ctx, tx, userID, requestID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if item.status != "WAITING" && item.status != "SCHEDULED" && item.status != "FAILED" {
		return identityDowngradeResult{RequestID: item.id, UserID: item.userID, Status: item.status, EffectiveAt: item.effectiveAt.UTC().Format(time.RFC3339Nano), Idempotent: true}, tx.Commit()
	}
	preview, err := calculateIdentityDowngradePreview(ctx, tx, userID, item.request)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	status := "WAITING"
	if len(preview.Blockers) == 0 {
		status = "SCHEDULED"
	}
	blockersJSON, _ := json.Marshal(preview.Blockers)
	if _, err = tx.ExecContext(ctx, `UPDATE xz_identity_downgrade_requests SET status=$2,blocker_snapshot=$3,failure_message='',last_checked_at=now(),updated_at=now() WHERE id=$1`, requestID, status, blockersJSON); err != nil {
		return identityDowngradeResult{}, err
	}
	if err = insertAuditLog(ctx, tx, actorID, actorRole, "admin.identity_downgrade.recheck", "identity_downgrade", requestID, http.MethodPost, "/api/v1/admin/users/"+userID+"/identity-downgrade/requests/"+requestID+"/recheck", 200, map[string]any{"blockers": preview.Blockers}); err != nil {
		return identityDowngradeResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return identityDowngradeResult{}, err
	}
	return identityDowngradeResult{RequestID: requestID, UserID: userID, Status: status, EffectiveAt: item.effectiveAt.UTC().Format(time.RFC3339Nano), Blockers: preview.Blockers, LastCheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func (s *postgresStore) CancelAdminIdentityDowngrade(actorID, actorRole, userID, requestID string) (identityDowngradeResult, error) {
	if err := authorizeDowngradeControl(actorID, actorRole); err != nil {
		return identityDowngradeResult{}, err
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	defer tx.Rollback()
	item, err := loadDowngradeControlForUpdate(ctx, tx, userID, requestID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if item.status == "CANCELLED" {
		return identityDowngradeResult{RequestID: item.id, UserID: userID, Status: item.status, EffectiveAt: item.effectiveAt.UTC().Format(time.RFC3339Nano), Idempotent: true}, tx.Commit()
	}
	if item.status != "WAITING" && item.status != "SCHEDULED" {
		return identityDowngradeResult{}, errIdentityDowngradeBlocked
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xz_identity_downgrade_requests SET status='CANCELLED',failure_message='cancelled by administrator',updated_at=now() WHERE id=$1`, requestID); err != nil {
		return identityDowngradeResult{}, err
	}
	if err = insertAuditLog(ctx, tx, actorID, actorRole, "admin.identity_downgrade.cancel", "identity_downgrade", requestID, http.MethodPost, "/api/v1/admin/users/"+userID+"/identity-downgrade/requests/"+requestID+"/cancel", 200, map[string]any{"previousStatus": item.status}); err != nil {
		return identityDowngradeResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return identityDowngradeResult{}, err
	}
	return identityDowngradeResult{RequestID: requestID, UserID: userID, Status: "CANCELLED", EffectiveAt: item.effectiveAt.UTC().Format(time.RFC3339Nano)}, nil
}

func (s *postgresStore) RescheduleAdminIdentityDowngrade(actorID, actorRole, userID, requestID string, request identityDowngradeRescheduleRequest) (identityDowngradeResult, error) {
	if err := authorizeDowngradeControl(actorID, actorRole); err != nil {
		return identityDowngradeResult{}, err
	}
	effectiveAt, err := time.Parse(time.RFC3339, strings.TrimSpace(request.EffectiveAt))
	if err != nil || !effectiveAt.After(time.Now().UTC()) {
		return identityDowngradeResult{}, errIdentityDowngradeInvalid
	}
	if len([]rune(strings.TrimSpace(request.Reason))) < 4 {
		return identityDowngradeResult{}, errIdentityDowngradeInvalid
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	defer tx.Rollback()
	item, err := loadDowngradeControlForUpdate(ctx, tx, userID, requestID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if item.status != "WAITING" && item.status != "SCHEDULED" {
		return identityDowngradeResult{}, errIdentityDowngradeBlocked
	}
	if _, err = tx.ExecContext(ctx, `UPDATE xz_identity_downgrade_requests SET effective_at=$2,updated_at=now() WHERE id=$1`, requestID, effectiveAt.UTC()); err != nil {
		return identityDowngradeResult{}, err
	}
	if err = insertAuditLog(ctx, tx, actorID, actorRole, "admin.identity_downgrade.reschedule", "identity_downgrade", requestID, http.MethodPost, "/api/v1/admin/users/"+userID+"/identity-downgrade/requests/"+requestID+"/reschedule", 200, map[string]any{"before": item.effectiveAt, "after": effectiveAt.UTC(), "reason": strings.TrimSpace(request.Reason)}); err != nil {
		return identityDowngradeResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return identityDowngradeResult{}, err
	}
	return identityDowngradeResult{RequestID: requestID, UserID: userID, Status: item.status, EffectiveAt: effectiveAt.UTC().Format(time.RFC3339Nano)}, nil
}
