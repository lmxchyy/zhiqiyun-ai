package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (r connectorRepository) createAuthorizationSession(ctx context.Context, item connectorAuthorizationSession) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_authorization_sessions(
			id,enterprise_id,platform,state_hash,status,created_by_user_id,created_by_role,
			organization_id,result_json,expires_at
		) VALUES($1,$2,$3,$4,'PENDING',$5,$6,$7,'{}'::jsonb,$8)
	`, item.ID, item.EnterpriseID, item.Platform, item.StateHash, item.CreatedByUserID,
		item.CreatedByRole, item.OrganizationID, item.ExpiresAt)
	return err
}

func (r connectorRepository) authorizationSessionByID(ctx context.Context, enterpriseID, id string) (connectorAuthorizationSession, error) {
	_, _ = r.db.ExecContext(ctx, `
		UPDATE connector_authorization_sessions SET status='EXPIRED',updated_at=now()
		WHERE enterprise_id=$1 AND id=$2 AND status IN ('PENDING','AUTHORIZING') AND expires_at<=now()
	`, enterpriseID, id)
	return scanConnectorAuthorizationSession(r.db.QueryRowContext(ctx, connectorAuthorizationSessionSelect+` WHERE enterprise_id=$1 AND id=$2`, enterpriseID, id))
}

func (r connectorRepository) authorizationSessionByStateHash(ctx context.Context, stateHash string) (connectorAuthorizationSession, error) {
	_, _ = r.db.ExecContext(ctx, `
		UPDATE connector_authorization_sessions SET status='EXPIRED',updated_at=now()
		WHERE state_hash=$1 AND status IN ('PENDING','AUTHORIZING') AND expires_at<=now()
	`, stateHash)
	return scanConnectorAuthorizationSession(r.db.QueryRowContext(ctx, connectorAuthorizationSessionSelect+` WHERE state_hash=$1`, stateHash))
}

func (r connectorRepository) beginAuthorizationSession(ctx context.Context, stateHash, platform string) (connectorAuthorizationSession, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE connector_authorization_sessions
		SET platform=$2,status='AUTHORIZING',error_code='',error_message='',updated_at=now()
		WHERE state_hash=$1 AND expires_at>now() AND status IN ('PENDING','AUTHORIZING')
		  AND (platform='universal' OR platform=$2)
	`, stateHash, platform)
	if err != nil {
		return connectorAuthorizationSession{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return connectorAuthorizationSession{}, errEnterpriseConflict
	}
	return r.authorizationSessionByStateHash(ctx, stateHash)
}

func (r connectorRepository) cancelAuthorizationSession(ctx context.Context, enterpriseID, id string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE connector_authorization_sessions SET status='CANCELLED',consumed_at=now(),updated_at=now()
		WHERE enterprise_id=$1 AND id=$2 AND status IN ('PENDING','AUTHORIZING')
	`, enterpriseID, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errEnterpriseConflict
	}
	return nil
}

func (r connectorRepository) failAuthorizationSession(ctx context.Context, stateHash, code, message string) {
	_, _ = r.db.ExecContext(ctx, `
		UPDATE connector_authorization_sessions
		SET status='FAILED',error_code=$2,error_message=$3,consumed_at=now(),updated_at=now()
		WHERE state_hash=$1 AND status IN ('PENDING','AUTHORIZING')
	`, stateHash, code, truncateConnectorError(message))
}

func (r connectorRepository) completeAuthorizationSession(ctx context.Context, session connectorAuthorizationSession, connectorItem enterpriseConnector, identity connectorExternalIdentity) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM connector_authorization_sessions WHERE id=$1 FOR UPDATE`, session.ID).Scan(&status); err != nil {
		return err
	}
	if status != "AUTHORIZING" || time.Now().UTC().After(session.ExpiresAt) {
		return errEnterpriseConflict
	}

	var bindingID, currentUserID, source string
	err = tx.QueryRowContext(ctx, `
		SELECT binding.id,coalesce(binding.internal_user_id,''),coalesce(account.raw->>'source','')
		FROM connector_user_bindings binding
		LEFT JOIN xz_users account ON account.id=binding.internal_user_id
		WHERE binding.connector_id=$1 AND binding.external_user_id=$2 FOR UPDATE
	`, connectorItem.ID, identity.ExternalUserID).Scan(&bindingID, &currentUserID, &source)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && currentUserID != "" && currentUserID != session.CreatedByUserID && source != identity.Platform {
		return fmt.Errorf("external account is already bound to another user")
	}
	if errors.Is(err, sql.ErrNoRows) {
		bindingID = newConnectorID("connector_binding")
		_, err = tx.ExecContext(ctx, `
			INSERT INTO connector_user_bindings(
				id,enterprise_id,connector_id,platform,external_user_id,external_union_id,
				external_name,external_avatar,internal_user_id,permission_json,status,last_active_at
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'{"imageGenerate":true}'::jsonb,'active',now())
		`, bindingID, session.EnterpriseID, connectorItem.ID, identity.Platform, identity.ExternalUserID,
			identity.ExternalUnionID, identity.ExternalName, identity.ExternalAvatar, session.CreatedByUserID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE connector_user_bindings SET internal_user_id=$2,external_union_id=$3,
				external_name=$4,external_avatar=$5,status='active',last_active_at=now(),updated_at=now()
			WHERE id=$1
		`, bindingID, session.CreatedByUserID, identity.ExternalUnionID, identity.ExternalName, identity.ExternalAvatar)
	}
	if err != nil {
		return err
	}

	result := map[string]any{"bindingId": bindingID, "platform": identity.Platform, "externalUserName": identity.ExternalName}
	_, err = tx.ExecContext(ctx, `
		UPDATE connector_authorization_sessions SET status='AUTHORIZED',connector_id=$2,
			external_tenant_key=$3,external_user_id=$4,external_user_name=$5,result_json=$6::jsonb,
			consumed_at=now(),updated_at=now()
		WHERE id=$1 AND status='AUTHORIZING'
	`, session.ID, connectorItem.ID, identity.ExternalTenantKey, identity.ExternalUserID,
		identity.ExternalName, connectorJSON(result))
	if err != nil {
		return err
	}
	if err := insertTenantAuditTx(ctx, tx, enterpriseAccess{UserID: session.CreatedByUserID, TenantID: session.EnterpriseID, OrganizationID: session.OrganizationID, Role: session.CreatedByRole},
		"enterprise.connector.oauth.bind", "connector_user_binding", bindingID, session.CreatedByUserID,
		map[string]any{"platform": identity.Platform, "connectorId": connectorItem.ID}); err != nil {
		return err
	}
	return tx.Commit()
}

const connectorAuthorizationSessionSelect = `
	SELECT id,enterprise_id,platform,state_hash,status,created_by_user_id,created_by_role,
		organization_id,coalesce(connector_id,''),external_tenant_key,external_user_id,external_user_name,
		result_json,error_code,error_message,expires_at,consumed_at,created_at,updated_at
	FROM connector_authorization_sessions`

func scanConnectorAuthorizationSession(row interface{ Scan(...any) error }) (connectorAuthorizationSession, error) {
	var item connectorAuthorizationSession
	var resultRaw []byte
	var consumedAt sql.NullTime
	err := row.Scan(&item.ID, &item.EnterpriseID, &item.Platform, &item.StateHash, &item.Status,
		&item.CreatedByUserID, &item.CreatedByRole, &item.OrganizationID, &item.ConnectorID,
		&item.ExternalTenantKey, &item.ExternalUserID, &item.ExternalUserName, &resultRaw,
		&item.ErrorCode, &item.ErrorMessage, &item.ExpiresAt, &consumedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.Result = map[string]any{}
	_ = json.Unmarshal(resultRaw, &item.Result)
	if consumedAt.Valid {
		item.ConsumedAt = &consumedAt.Time
	}
	return item, nil
}
