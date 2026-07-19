package httpserver

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/connector"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type connectorRepository struct {
	db *sql.DB
}

func (r connectorRepository) connectorByKey(ctx context.Context, key string) (enterpriseConnector, error) {
	row := r.db.QueryRowContext(ctx, connectorSelect+` WHERE connector_key=$1`, strings.TrimSpace(key))
	return scanConnector(row)
}

func (r connectorRepository) connectorForEnterprise(ctx context.Context, enterpriseID string, connectorType string) (enterpriseConnector, bool, error) {
	row := r.db.QueryRowContext(ctx, connectorSelect+` WHERE enterprise_id=$1 AND connector_type=$2`, enterpriseID, connectorType)
	item, err := scanConnector(row)
	if errors.Is(err, sql.ErrNoRows) {
		return enterpriseConnector{}, false, nil
	}
	return item, err == nil, err
}

func (r connectorRepository) listConnectors(ctx context.Context, enterpriseID string) ([]enterpriseConnector, error) {
	rows, err := r.db.QueryContext(ctx, connectorSelect+` WHERE enterprise_id=$1 ORDER BY created_at,id`, enterpriseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []enterpriseConnector{}
	for rows.Next() {
		item, err := scanConnector(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r connectorRepository) createConnector(ctx context.Context, item enterpriseConnector) (enterpriseConnector, error) {
	config := connectorJSON(item.Config)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO enterprise_connectors(
			id,enterprise_id,connector_type,connector_name,connector_key,app_id,
			app_secret_encrypted,verification_token_encrypted,encrypt_key_encrypted,status,config_json
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'disabled',$10::jsonb)
	`, item.ID, item.EnterpriseID, item.ConnectorType, item.ConnectorName, item.ConnectorKey, item.AppID,
		item.AppSecretEncrypted, item.VerificationTokenEncrypted, item.EncryptKeyEncrypted, config)
	if err != nil {
		return enterpriseConnector{}, err
	}
	return r.mustConnectorForEnterprise(ctx, item.EnterpriseID, item.ConnectorType)
}

func (r connectorRepository) updateConnector(ctx context.Context, item enterpriseConnector) (enterpriseConnector, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE enterprise_connectors SET connector_name=$3,app_id=$4,app_secret_encrypted=$5,
			verification_token_encrypted=$6,encrypt_key_encrypted=$7,config_json=$8::jsonb,
			last_error_message='',updated_at=now()
		WHERE enterprise_id=$1 AND id=$2
	`, item.EnterpriseID, item.ID, item.ConnectorName, item.AppID, item.AppSecretEncrypted,
		item.VerificationTokenEncrypted, item.EncryptKeyEncrypted, connectorJSON(item.Config))
	if err != nil {
		return enterpriseConnector{}, err
	}
	return r.mustConnectorForEnterprise(ctx, item.EnterpriseID, item.ConnectorType)
}

func (r connectorRepository) mustConnectorForEnterprise(ctx context.Context, enterpriseID, connectorType string) (enterpriseConnector, error) {
	item, found, err := r.connectorForEnterprise(ctx, enterpriseID, connectorType)
	if err != nil {
		return enterpriseConnector{}, err
	}
	if !found {
		return enterpriseConnector{}, errEnterpriseNotFound
	}
	return item, nil
}

func (r connectorRepository) updateConnectorState(ctx context.Context, enterpriseID, connectorID, status, lastError string, connected bool) (enterpriseConnector, error) {
	_, err := r.db.ExecContext(ctx, `
		UPDATE enterprise_connectors SET status=$3,last_error_message=$4,
			last_connected_at=CASE WHEN $5 THEN now() ELSE last_connected_at END,updated_at=now()
		WHERE enterprise_id=$1 AND id=$2
	`, enterpriseID, connectorID, status, truncateConnectorError(lastError), connected)
	if err != nil {
		return enterpriseConnector{}, err
	}
	item, err := r.connectorByID(ctx, connectorID)
	if err != nil {
		return enterpriseConnector{}, err
	}
	if item.EnterpriseID != enterpriseID {
		return enterpriseConnector{}, errEnterpriseNotFound
	}
	return item, nil
}

func (r connectorRepository) updateConnectorBotOpenID(ctx context.Context, enterpriseID, connectorID, botOpenID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE enterprise_connectors SET bot_open_id=$3,updated_at=now()
		WHERE enterprise_id=$1 AND id=$2
	`, enterpriseID, connectorID, strings.TrimSpace(botOpenID))
	return err
}

func (r connectorRepository) updateExternalTenantKey(ctx context.Context, connectorID, tenantKey string) {
	if strings.TrimSpace(tenantKey) == "" {
		return
	}
	_, _ = r.db.ExecContext(ctx, `UPDATE enterprise_connectors SET external_tenant_key=$2,updated_at=now() WHERE id=$1`, connectorID, tenantKey)
}

func (r connectorRepository) insertIncomingMessage(ctx context.Context, item enterpriseConnector, message connector.IncomingMessage, rawPayload map[string]any) (connectorMessageRecord, bool, error) {
	record := connectorMessageRecord{
		ID: newConnectorID("connector_message"), EnterpriseID: item.EnterpriseID, ConnectorID: item.ID,
		ExternalMessageID: message.ExternalMessageID, ExternalChatID: message.ExternalChatID,
		ExternalUserID: message.ExternalUserID, ChatType: message.ChatType, MessageType: message.MessageType,
		Content:          map[string]any{"text": message.Text, "externalUnionId": message.ExternalUnionID, "externalTenantKey": message.ExternalTenantKey, "mentionedBot": message.MentionedBot},
		ProcessingStatus: "received",
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_messages(
			id,enterprise_id,connector_id,platform,external_message_id,external_chat_id,external_user_id,
			chat_type,message_type,direction,content_json,raw_payload_json,processing_status
		) VALUES($1,$2,$3,'feishu',$4,$5,$6,$7,$8,'inbound',$9::jsonb,$10::jsonb,'received')
		ON CONFLICT(platform,external_message_id) DO NOTHING
	`, record.ID, record.EnterpriseID, record.ConnectorID, record.ExternalMessageID, record.ExternalChatID,
		record.ExternalUserID, record.ChatType, record.MessageType, connectorJSON(record.Content), connectorJSON(rawPayload))
	if err != nil {
		return connectorMessageRecord{}, false, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		existing, err := r.messageByExternalID(ctx, "feishu", message.ExternalMessageID)
		return existing, false, err
	}
	return record, true, nil
}

func (r connectorRepository) messageByExternalID(ctx context.Context, platform, externalID string) (connectorMessageRecord, error) {
	var item connectorMessageRecord
	var contentRaw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id,enterprise_id,connector_id,external_message_id,external_chat_id,external_user_id,
			chat_type,message_type,content_json,processing_status
		FROM connector_messages WHERE platform=$1 AND external_message_id=$2
	`, platform, externalID).Scan(&item.ID, &item.EnterpriseID, &item.ConnectorID, &item.ExternalMessageID,
		&item.ExternalChatID, &item.ExternalUserID, &item.ChatType, &item.MessageType, &contentRaw, &item.ProcessingStatus)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal(contentRaw, &item.Content)
	return item, nil
}

func (r connectorRepository) connectorByID(ctx context.Context, connectorID string) (enterpriseConnector, error) {
	return scanConnector(r.db.QueryRowContext(ctx, connectorSelect+` WHERE id=$1`, connectorID))
}

func (r connectorRepository) markMessage(ctx context.Context, messageID, status, errorMessage string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE connector_messages SET processing_status=$2,error_message=$3,updated_at=now() WHERE id=$1`, messageID, status, truncateConnectorError(errorMessage))
	return err
}

func (r connectorRepository) insertOutboundMessage(ctx context.Context, item enterpriseConnector, targetChatID, externalUserID, externalMessageID, messageType string, content map[string]any) error {
	if strings.TrimSpace(externalMessageID) == "" {
		externalMessageID = newConnectorID("feishu_out")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_messages(id,enterprise_id,connector_id,platform,external_message_id,
			external_chat_id,external_user_id,chat_type,message_type,direction,content_json,processing_status)
		VALUES($1,$2,$3,'feishu',$4,$5,$6,'',$7,'outbound',$8::jsonb,'completed')
		ON CONFLICT(platform,external_message_id) DO NOTHING
	`, newConnectorID("connector_message"), item.EnterpriseID, item.ID, externalMessageID, targetChatID, externalUserID, messageType, connectorJSON(content))
	return err
}

func (r connectorRepository) loadOrCreateBinding(ctx context.Context, connectorItem enterpriseConnector, message connectorMessageRecord) (connectorUserBinding, error) {
	if item, found, err := r.bindingByExternalUser(ctx, connectorItem.ID, message.ExternalUserID); err != nil || found {
		return item, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return connectorUserBinding{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var existingID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM connector_user_bindings WHERE connector_id=$1 AND external_user_id=$2`, connectorItem.ID, message.ExternalUserID).Scan(&existingID); err == nil {
		if err := tx.Commit(); err != nil {
			return connectorUserBinding{}, err
		}
		item, _, err := r.bindingByExternalUser(ctx, connectorItem.ID, message.ExternalUserID)
		return item, err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return connectorUserBinding{}, err
	}
	var organizationID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM xz_organizations WHERE tenant_id=$1 AND upper(status)='ACTIVE' ORDER BY parent_id NULLS FIRST,created_at,id LIMIT 1`, connectorItem.EnterpriseID).Scan(&organizationID); err != nil {
		return connectorUserBinding{}, fmt.Errorf("resolve enterprise organization: %w", err)
	}
	internalUserID := newConnectorID("feishu_user")
	memberID := newConnectorID("tenant_member")
	bindingID := newConnectorID("connector_binding")
	externalName := "飞书用户 " + shortExternalID(message.ExternalUserID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_users(id,name,role,status,created_at,updated_at,raw)
		VALUES($1,$2,'USER','ACTIVE',$3,$3,$4::jsonb)
	`, internalUserID, externalName, now, connectorJSON(map[string]any{
		"id": internalUserID, "name": externalName, "role": "USER", "status": "ACTIVE",
		"createdAt": now, "updatedAt": now, "source": "feishu",
		"externalUserId": message.ExternalUserID, "tenantId": connectorItem.EnterpriseID, "planId": "plan_free",
	})); err != nil {
		return connectorUserBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_tenant_members(id,tenant_id,user_id,role,status,primary_organization_id,member_status,
			certification_status,data_scope,joined_at,created_at,updated_at)
		VALUES($1,$2,$3,'MEMBER','ACTIVE',$4,'ACTIVE','UNVERIFIED','SELF',now(),now(),now())
	`, memberID, connectorItem.EnterpriseID, internalUserID, organizationID); err != nil {
		return connectorUserBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status,assigned_at,updated_at)
		VALUES($1,$2,$3,'ENTERPRISE_MEMBER','ACTIVE',now(),now())
		ON CONFLICT(user_id,tenant_id,organization_id,role) DO UPDATE SET status='ACTIVE',updated_at=now()
	`, internalUserID, connectorItem.EnterpriseID, organizationID); err != nil {
		return connectorUserBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_user_role_context(user_id,tenant_id,organization_id,current_role_code,context_type,updated_at)
		VALUES($1,$2,$3,'ENTERPRISE_MEMBER','ENTERPRISE',now())
		ON CONFLICT(user_id) DO UPDATE SET tenant_id=excluded.tenant_id,organization_id=excluded.organization_id,
			current_role_code=excluded.current_role_code,context_type='ENTERPRISE',updated_at=now()
	`, internalUserID, connectorItem.EnterpriseID, organizationID); err != nil {
		return connectorUserBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO connector_user_bindings(id,enterprise_id,connector_id,platform,external_user_id,
			external_union_id,external_name,internal_user_id,permission_json,status,last_active_at)
		VALUES($1,$2,$3,'feishu',$4,$5,$6,$7,'{"imageGenerate":true,"videoGenerate":true,"pptGenerate":true}'::jsonb,'active',now())
	`, bindingID, connectorItem.EnterpriseID, connectorItem.ID, message.ExternalUserID,
		stringValue(message.Content["externalUnionId"]), externalName, internalUserID); err != nil {
		return connectorUserBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return connectorUserBinding{}, err
	}
	item, _, err := r.bindingByExternalUser(ctx, connectorItem.ID, message.ExternalUserID)
	return item, err
}

func (r connectorRepository) bindingByExternalUser(ctx context.Context, connectorID, externalUserID string) (connectorUserBinding, bool, error) {
	var item connectorUserBinding
	var permissionRaw []byte
	var lastActive sql.NullTime
	var createdAt, updatedAt time.Time
	err := r.db.QueryRowContext(ctx, `
		SELECT binding.id,binding.enterprise_id,binding.connector_id,binding.platform,binding.external_user_id,
			binding.external_union_id,binding.external_name,binding.external_avatar,coalesce(binding.internal_user_id,''),
			coalesce(user_account.name,''),coalesce(organization.name,''),binding.permission_json,binding.status,binding.last_active_at,
			binding.created_at,binding.updated_at
		FROM connector_user_bindings binding
		LEFT JOIN xz_users user_account ON user_account.id=binding.internal_user_id
		LEFT JOIN xz_tenant_members member ON member.tenant_id=binding.enterprise_id AND member.user_id=binding.internal_user_id
		LEFT JOIN xz_organizations organization ON organization.tenant_id=binding.enterprise_id AND organization.id=member.primary_organization_id
		WHERE binding.connector_id=$1 AND binding.external_user_id=$2
	`, connectorID, externalUserID).Scan(&item.ID, &item.EnterpriseID, &item.ConnectorID, &item.Platform,
		&item.ExternalUserID, &item.ExternalUnionID, &item.ExternalName, &item.ExternalAvatar, &item.InternalUserID,
		&item.InternalUserName, &item.OrganizationName, &permissionRaw, &item.Status, &lastActive, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return item, false, nil
	}
	if err != nil {
		return item, false, err
	}
	_ = json.Unmarshal(permissionRaw, &item.Permission)
	item.LastActiveAt = nullableConnectorTime(lastActive)
	item.CreatedAt, item.UpdatedAt = connectorTime(createdAt), connectorTime(updatedAt)
	return item, true, nil
}

func (r connectorRepository) touchBinding(ctx context.Context, id string) {
	_, _ = r.db.ExecContext(ctx, `UPDATE connector_user_bindings SET last_active_at=now(),updated_at=now() WHERE id=$1`, id)
}

func (r connectorRepository) dailyBindingUsage(ctx context.Context, enterpriseID, bindingID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT count(*) FROM connector_ai_tasks
		WHERE enterprise_id=$1 AND binding_id=$2 AND created_at>=date_trunc('day',now())
			AND status IN ('pending','processing','succeeded')
	`, enterpriseID, bindingID).Scan(&count)
	return count, err
}

func (r connectorRepository) bindingPointUsage(ctx context.Context, enterpriseID, bindingID, capability string, since time.Time) (int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, `
		SELECT coalesce(sum(greatest(points_cost,estimated_points)),0)
		FROM connector_ai_tasks
		WHERE enterprise_id=$1 AND binding_id=$2 AND task_type=$3 AND created_at >= $4
		  AND unified_status NOT IN ('failed','cancelled','refunded')
	`, enterpriseID, bindingID, capability, since.UTC()).Scan(&total)
	return total, err
}

func (r connectorRepository) latestSuccessfulReferenceImage(ctx context.Context, enterpriseID, connectorID, bindingID, externalChatID string) (connectorReferenceImage, bool, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT asset.id,task.platform_task_id,asset.name,asset.url,task.task_type,task.original_text,task.result_json
		FROM connector_ai_tasks task
		JOIN xz_assets asset ON asset.task_id=task.platform_task_id AND asset.deleted_at IS NULL
		WHERE task.enterprise_id=$1 AND task.connector_id=$2 AND task.binding_id=$3
		  AND task.external_chat_id=$4 AND task.status IN ('succeeded','delivery_failed')
		  AND task.unified_status IN ('completed','delivery_failed')
		  AND asset.media_type='image' AND coalesce(asset.url,'')<>''
		  AND asset.url NOT LIKE 'data:image/svg+xml%'
		ORDER BY task.completed_at DESC NULLS LAST,task.created_at DESC,asset.created_at DESC
		LIMIT 20
	`, enterpriseID, connectorID, bindingID, externalChatID)
	if err != nil {
		return connectorReferenceImage{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var item connectorReferenceImage
		var taskType, originalText string
		var resultRaw []byte
		if err := rows.Scan(&item.AssetID, &item.GenerationTaskID, &item.Name, &item.URL, &taskType, &originalText, &resultRaw); err != nil {
			return connectorReferenceImage{}, false, err
		}
		result := map[string]any{}
		_ = json.Unmarshal(resultRaw, &result)
		legacyMisclassifiedEdit := taskType == connector.IntentImageGenerate && isConnectorImageEditRequest(originalText) && !boolValue(result["editMode"])
		if legacyMisclassifiedEdit {
			continue
		}
		return item, true, nil
	}
	if err := rows.Err(); err != nil {
		return connectorReferenceImage{}, false, err
	}
	return connectorReferenceImage{}, false, nil
}

func (r connectorRepository) createConnectorTask(ctx context.Context, task connectorTaskRecord) (connectorTaskRecord, bool, error) {
	if task.ID == "" {
		task.ID = newConnectorID("connector_task")
	}
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_ai_tasks(id,enterprise_id,connector_id,binding_id,platform,external_chat_id,
			external_message_id,task_type,intent,original_text,optimized_prompt,model_id,status,unified_status,
			progress,estimated_points,started_at)
		VALUES($1,$2,$3,nullif($4,''),'feishu',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now())
		ON CONFLICT(platform,external_message_id) DO NOTHING
	`, task.ID, task.EnterpriseID, task.ConnectorID, task.BindingID, task.ExternalChatID, task.ExternalMessageID,
		task.TaskType, task.Intent, task.OriginalText, task.OptimizedPrompt, task.ModelID, task.Status,
		firstNonEmptyString(task.UnifiedStatus, "created"), task.Progress, task.EstimatedPoints)
	if err != nil {
		return connectorTaskRecord{}, false, err
	}
	affected, _ := result.RowsAffected()
	item, err := r.taskByExternalMessage(ctx, "feishu", task.ExternalMessageID)
	return item, affected > 0, err
}

func (r connectorRepository) taskByExternalMessage(ctx context.Context, platform, externalMessageID string) (connectorTaskRecord, error) {
	return scanConnectorTask(r.db.QueryRowContext(ctx, connectorTaskSelect+` WHERE task.platform=$1 AND task.external_message_id=$2`, platform, externalMessageID))
}

func (r connectorRepository) taskByIDForBinding(ctx context.Context, enterpriseID, connectorID, bindingID, taskID string) (connectorTaskRecord, error) {
	return scanConnectorTask(r.db.QueryRowContext(ctx, connectorTaskSelect+`
		WHERE task.enterprise_id=$1 AND task.connector_id=$2 AND task.binding_id=$3 AND task.id=$4`, enterpriseID, connectorID, bindingID, taskID))
}

func (r connectorRepository) taskByIDForEnterprise(ctx context.Context, enterpriseID, connectorID, taskID string) (connectorTaskRecord, error) {
	return scanConnectorTask(r.db.QueryRowContext(ctx, connectorTaskSelect+`
		WHERE task.enterprise_id=$1 AND task.connector_id=$2 AND task.id=$3`, enterpriseID, connectorID, taskID))
}

func (r connectorRepository) assetForInternalTask(ctx context.Context, userID, taskID string) (asset, bool, error) {
	var item asset
	var metadataRaw []byte
	var deletedAt sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT id,user_id,coalesce(tenant_id,''),coalesce(organization_id,''),task_id,name,media_type,url,
			coalesce(thumbnail_url,''),favorite,metadata,deleted_at::text,created_at::text,updated_at::text
		FROM xz_assets WHERE user_id=$1 AND task_id=$2 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1
	`, userID, taskID).Scan(&item.ID, &item.UserID, &item.TenantID, &item.OrganizationID, &item.TaskID, &item.Name, &item.MediaType,
		&item.URL, &item.ThumbnailURL, &item.Favorite, &metadataRaw, &deletedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return asset{}, false, nil
	}
	if err != nil {
		return asset{}, false, err
	}
	_ = json.Unmarshal(metadataRaw, &item.Metadata)
	return item, true, nil
}

func (r connectorRepository) deleteConnectorAsset(ctx context.Context, assetID, userID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM xz_assets WHERE id=$1 AND user_id=$2`, assetID, userID)
	return err
}

func (r connectorRepository) updateConnectorTask(ctx context.Context, id, status string, progress int, platformTaskID string, pointsCost int64, result map[string]any, errorCode, errorMessage string) error {
	return r.updateConnectorTaskState(ctx, id, status, connectorUnifiedStatus(status), "", progress, platformTaskID, pointsCost, result, errorCode, errorMessage)
}

func (r connectorRepository) updateConnectorTaskState(ctx context.Context, id, status, unifiedStatus, internalStage string, progress int, platformTaskID string, pointsCost int64, result map[string]any, errorCode, errorMessage string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE connector_ai_tasks SET status=$2,progress=$3,platform_task_id=coalesce(nullif($4,''),platform_task_id),
			points_cost=$5,result_json=$6::jsonb,error_code=$7,error_message=$8,
			unified_status=coalesce(nullif($9,''),unified_status),internal_stage=coalesce(nullif($10,''),internal_stage),
			completed_at=CASE WHEN $9 IN ('completed','delivery_failed','failed','cancelled','refunded') THEN now() ELSE completed_at END,updated_at=now()
		WHERE id=$1
	`, id, status, progress, platformTaskID, pointsCost, connectorJSON(result), errorCode, truncateConnectorError(errorMessage), unifiedStatus, internalStage)
	return err
}

func (r connectorRepository) setConnectorTaskEstimate(ctx context.Context, id string, points int64, modelID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE connector_ai_tasks SET estimated_points=$2,model_id=coalesce(nullif($3,''),model_id),updated_at=now() WHERE id=$1`, id, points, modelID)
	return err
}

func connectorUnifiedStatus(status string) string {
	switch status {
	case "pending":
		return "created"
	case "processing":
		return "processing"
	case "succeeded", "completed", "ignored":
		return "completed"
	case "delivery_failed":
		return "delivery_failed"
	case "cancelled":
		return "cancelled"
	case "refunded":
		return "refunded"
	default:
		return "failed"
	}
}

func (r connectorRepository) markConnectorDelivery(ctx context.Context, id, status string, delivered bool, errorMessage string) error {
	unified := "completed"
	legacy := "succeeded"
	if !delivered {
		unified, legacy = "delivery_failed", "delivery_failed"
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE connector_ai_tasks SET delivery_status=$2,delivery_attempts=delivery_attempts+1,
			unified_status=$3,status=$4,error_code=CASE WHEN $2='failed' THEN 'DELIVERY_FAILED' ELSE '' END,
			error_message=$5,delivered_at=CASE WHEN $2='delivered' THEN now() ELSE delivered_at END,
			completed_at=coalesce(completed_at,now()),updated_at=now() WHERE id=$1
	`, id, status, unified, legacy, truncateConnectorError(errorMessage))
	return err
}

func (r connectorRepository) upsertSessionContext(ctx context.Context, item connectorSessionContext) error {
	if item.ExpiresAt.IsZero() {
		item.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO connector_session_contexts(enterprise_id,connector_id,external_chat_id,external_user_id,
			last_intent,last_task_type,last_task_id,last_asset_ids,last_topic,last_parameters,last_prompt,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10::jsonb,$11,$12)
		ON CONFLICT(connector_id,external_chat_id,external_user_id) DO UPDATE SET
			enterprise_id=excluded.enterprise_id,last_intent=excluded.last_intent,last_task_type=excluded.last_task_type,
			last_task_id=excluded.last_task_id,last_asset_ids=excluded.last_asset_ids,last_topic=excluded.last_topic,
			last_parameters=excluded.last_parameters,last_prompt=excluded.last_prompt,expires_at=excluded.expires_at,updated_at=now()
	`, item.EnterpriseID, item.ConnectorID, item.ExternalChatID, item.ExternalUserID, item.LastIntent, item.LastTaskType,
		item.LastTaskID, connectorJSON(item.LastAssetIDs), item.LastTopic, connectorJSON(item.LastParameters), item.LastPrompt, item.ExpiresAt.UTC())
	return err
}

func (r connectorRepository) sessionContext(ctx context.Context, enterpriseID, connectorID, chatID, externalUserID string) (connectorSessionContext, bool, error) {
	var item connectorSessionContext
	var assetRaw, parameterRaw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT enterprise_id,connector_id,external_chat_id,external_user_id,last_intent,last_task_type,last_task_id,
			last_asset_ids,last_topic,last_parameters,last_prompt,expires_at
		FROM connector_session_contexts
		WHERE enterprise_id=$1 AND connector_id=$2 AND external_chat_id=$3 AND external_user_id=$4 AND expires_at>now()
	`, enterpriseID, connectorID, chatID, externalUserID).Scan(&item.EnterpriseID, &item.ConnectorID, &item.ExternalChatID,
		&item.ExternalUserID, &item.LastIntent, &item.LastTaskType, &item.LastTaskID, &assetRaw, &item.LastTopic,
		&parameterRaw, &item.LastPrompt, &item.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return item, false, nil
	}
	if err != nil {
		return item, false, err
	}
	_ = json.Unmarshal(assetRaw, &item.LastAssetIDs)
	_ = json.Unmarshal(parameterRaw, &item.LastParameters)
	return item, true, nil
}

func (r connectorRepository) latestTaskForContext(ctx context.Context, enterpriseID, connectorID, bindingID, chatID string) (connectorTaskRecord, bool, error) {
	item, err := scanConnectorTask(r.db.QueryRowContext(ctx, connectorTaskSelect+`
		WHERE task.enterprise_id=$1 AND task.connector_id=$2 AND task.binding_id=$3 AND task.external_chat_id=$4
		ORDER BY task.created_at DESC LIMIT 1`, enterpriseID, connectorID, bindingID, chatID))
	if errors.Is(err, sql.ErrNoRows) {
		return connectorTaskRecord{}, false, nil
	}
	return item, err == nil, err
}

func (r connectorRepository) ensureConnectorPPTAsset(ctx context.Context, userID, tenantID, organizationID, taskID, title string, file storagecenter.FileObject) (asset, error) {
	var existing asset
	var metadataRaw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT id,user_id,coalesce(tenant_id,''),coalesce(organization_id,''),task_id,name,media_type,url,
			coalesce(thumbnail_url,''),favorite,metadata,coalesce(deleted_at::text,''),created_at::text,updated_at::text
		FROM xz_assets WHERE task_id=$1 AND user_id=$2 AND media_type='ppt' AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 1
	`, taskID, userID).Scan(&existing.ID, &existing.UserID, &existing.TenantID, &existing.OrganizationID, &existing.TaskID,
		&existing.Name, &existing.MediaType, &existing.URL, &existing.ThumbnailURL, &existing.Favorite, &metadataRaw,
		&existing.DeletedAt, &existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		_ = json.Unmarshal(metadataRaw, &existing.Metadata)
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return asset{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := asset{ID: newConnectorID("asset"), UserID: userID, TenantID: tenantID, OrganizationID: organizationID,
		TaskID: taskID, Name: firstNonEmptyString(strings.TrimSpace(title), "AI演示文稿") + ".pptx", MediaType: "ppt",
		URL: pptStorageReference(file), Metadata: map[string]any{
			"fileId": file.FileID, "storageFileId": file.FileID, "storageTenantId": file.TenantID,
			"storageProvider": file.Provider, "storageBucket": file.Bucket, "storageObjectKey": file.ObjectKey,
			"fileSize": file.FileSize, "contentType": file.MIMEType, "source": "feishu", "type": "PPT_GENERATION",
		}, CreatedAt: now, UpdatedAt: now}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return asset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertAsset(ctx, tx, item); err != nil {
		return asset{}, err
	}
	if err := tx.Commit(); err != nil {
		return asset{}, err
	}
	return item, nil
}

func (r connectorRepository) listBindings(ctx context.Context, enterpriseID, connectorID string, limit int) ([]connectorUserBinding, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT binding.id,binding.enterprise_id,binding.connector_id,binding.platform,binding.external_user_id,
			binding.external_union_id,binding.external_name,binding.external_avatar,coalesce(binding.internal_user_id,''),
			coalesce(user_account.name,''),coalesce(organization.name,''),binding.permission_json,binding.status,binding.last_active_at,
			binding.created_at,binding.updated_at,
			(SELECT count(*) FROM connector_ai_tasks task WHERE task.enterprise_id=binding.enterprise_id
				 AND task.binding_id=binding.id AND task.created_at>=date_trunc('day',now())
				 AND task.status IN ('pending','processing','succeeded'))
		FROM connector_user_bindings binding
		LEFT JOIN xz_users user_account ON user_account.id=binding.internal_user_id
		LEFT JOIN xz_tenant_members member ON member.tenant_id=binding.enterprise_id AND member.user_id=binding.internal_user_id
		LEFT JOIN xz_organizations organization ON organization.tenant_id=binding.enterprise_id AND organization.id=member.primary_organization_id
		WHERE binding.enterprise_id=$1 AND binding.connector_id=$2
		ORDER BY binding.last_active_at DESC NULLS LAST,binding.created_at DESC LIMIT $3
	`, enterpriseID, connectorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []connectorUserBinding{}
	for rows.Next() {
		var item connectorUserBinding
		var permissionRaw []byte
		var lastActive sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.EnterpriseID, &item.ConnectorID, &item.Platform, &item.ExternalUserID,
			&item.ExternalUnionID, &item.ExternalName, &item.ExternalAvatar, &item.InternalUserID, &item.InternalUserName,
			&item.OrganizationName, &permissionRaw, &item.Status, &lastActive, &createdAt, &updatedAt, &item.DailyUsage); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(permissionRaw, &item.Permission)
		item.LastActiveAt, item.CreatedAt, item.UpdatedAt = nullableConnectorTime(lastActive), connectorTime(createdAt), connectorTime(updatedAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r connectorRepository) updateBinding(ctx context.Context, enterpriseID, connectorID, bindingID string, request connectorBindingUpdateRequest) (connectorUserBinding, error) {
	if request.Permission == nil {
		request.Permission = map[string]any{"imageGenerate": true}
	}
	if request.Status != "active" && request.Status != "disabled" {
		return connectorUserBinding{}, errEnterpriseInvalid
	}
	var currentInternalUserID string
	if err := r.db.QueryRowContext(ctx, `
		SELECT coalesce(internal_user_id,'') FROM connector_user_bindings
		WHERE enterprise_id=$1 AND connector_id=$2 AND id=$3
	`, enterpriseID, connectorID, bindingID).Scan(&currentInternalUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectorUserBinding{}, errEnterpriseNotFound
		}
		return connectorUserBinding{}, err
	}
	// Phase 1 creates an enterprise-scoped shadow user. Controlled account
	// merging needs identity proof and is intentionally deferred to phase 2.
	if requested := strings.TrimSpace(request.InternalUserID); requested != "" && requested != currentInternalUserID {
		return connectorUserBinding{}, errEnterpriseInvalid
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE connector_user_bindings SET permission_json=$4::jsonb,status=$5,updated_at=now()
		WHERE enterprise_id=$1 AND connector_id=$2 AND id=$3
	`, enterpriseID, connectorID, bindingID, connectorJSON(request.Permission), request.Status)
	if err != nil {
		return connectorUserBinding{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return connectorUserBinding{}, errEnterpriseNotFound
	}
	var externalUserID string
	if err := r.db.QueryRowContext(ctx, `SELECT external_user_id FROM connector_user_bindings WHERE id=$1`, bindingID).Scan(&externalUserID); err != nil {
		return connectorUserBinding{}, err
	}
	item, _, err := r.bindingByExternalUser(ctx, connectorID, externalUserID)
	return item, err
}

func (r connectorRepository) listMessages(ctx context.Context, enterpriseID, connectorID string, limit int) ([]connectorMessageView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id,platform,external_message_id,external_chat_id,external_user_id,chat_type,message_type,
			direction,content_json,processing_status,error_message,created_at
		FROM connector_messages WHERE enterprise_id=$1 AND connector_id=$2 ORDER BY created_at DESC LIMIT $3
	`, enterpriseID, connectorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []connectorMessageView{}
	for rows.Next() {
		var item connectorMessageView
		var contentRaw []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.Platform, &item.ExternalMessageID, &item.ExternalChatID, &item.ExternalUserID,
			&item.ChatType, &item.MessageType, &item.Direction, &contentRaw, &item.ProcessingStatus, &item.ErrorMessage, &createdAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(contentRaw, &item.Content)
		item.CreatedAt = connectorTime(createdAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r connectorRepository) listTasks(ctx context.Context, enterpriseID, connectorID, capability, status string, limit int) ([]connectorTaskRecord, error) {
	query := connectorTaskSelect + ` WHERE task.enterprise_id=$1 AND task.connector_id=$2`
	args := []any{enterpriseID, connectorID}
	if capability = strings.TrimSpace(capability); capability != "" {
		args = append(args, capability)
		query += fmt.Sprintf(" AND task.task_type=$%d", len(args))
	}
	if status = strings.TrimSpace(status); status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND (task.unified_status=$%d OR task.status=$%d)", len(args), len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY task.created_at DESC LIMIT $%d", len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []connectorTaskRecord{}
	for rows.Next() {
		item, err := scanConnectorTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const connectorSelect = `SELECT id,enterprise_id,connector_type,connector_name,connector_key,app_id,
	app_secret_encrypted,verification_token_encrypted,encrypt_key_encrypted,external_tenant_key,bot_open_id,
	status,config_json,last_connected_at,last_error_message,created_at,updated_at FROM enterprise_connectors`

type connectorScanner interface{ Scan(...any) error }

func scanConnector(scanner connectorScanner) (enterpriseConnector, error) {
	var item enterpriseConnector
	var configRaw []byte
	err := scanner.Scan(&item.ID, &item.EnterpriseID, &item.ConnectorType, &item.ConnectorName, &item.ConnectorKey,
		&item.AppID, &item.AppSecretEncrypted, &item.VerificationTokenEncrypted, &item.EncryptKeyEncrypted,
		&item.ExternalTenantKey, &item.BotOpenID, &item.Status, &configRaw, &item.LastConnectedAt,
		&item.LastErrorMessage, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.Config = defaultConnectorConfig()
	_ = json.Unmarshal(configRaw, &item.Config)
	item.Config = normalizeConnectorConfig(item.Config)
	return item, nil
}

const connectorTaskSelect = `SELECT task.id,task.enterprise_id,task.connector_id,coalesce(task.binding_id,''),
	coalesce(binding.external_user_id,''),coalesce(binding.external_name,''),task.external_chat_id,
	task.external_message_id,task.task_type,task.intent,task.original_text,task.optimized_prompt,task.model_id,
	coalesce(task.platform_task_id,''),task.status,task.unified_status,task.internal_stage,task.delivery_status,
	task.delivery_attempts,task.progress,task.result_json,task.token_cost,task.points_cost,task.estimated_points,
	task.error_code,task.error_message,task.started_at,task.completed_at,task.created_at,task.updated_at
	FROM connector_ai_tasks task
	LEFT JOIN connector_user_bindings binding ON binding.id=task.binding_id AND binding.enterprise_id=task.enterprise_id`

func scanConnectorTask(scanner connectorScanner) (connectorTaskRecord, error) {
	var item connectorTaskRecord
	var resultRaw []byte
	var startedAt, completedAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := scanner.Scan(&item.ID, &item.EnterpriseID, &item.ConnectorID, &item.BindingID, &item.ExternalUserID,
		&item.ExternalUserName, &item.ExternalChatID,
		&item.ExternalMessageID, &item.TaskType, &item.Intent, &item.OriginalText, &item.OptimizedPrompt, &item.ModelID,
		&item.PlatformTaskID, &item.Status, &item.UnifiedStatus, &item.InternalStage, &item.DeliveryStatus,
		&item.DeliveryAttempts, &item.Progress, &resultRaw, &item.TokenCost, &item.PointsCost, &item.EstimatedPoints,
		&item.ErrorCode, &item.ErrorMessage, &startedAt, &completedAt, &createdAt, &updatedAt)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal(resultRaw, &item.Result)
	item.StartedAt, item.CompletedAt = nullableConnectorTime(startedAt), nullableConnectorTime(completedAt)
	item.CreatedAt, item.UpdatedAt = connectorTime(createdAt), connectorTime(updatedAt)
	return item, nil
}

func newConnectorID(prefix string) string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(raw[:])
}

func shortExternalID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}

func truncateConnectorError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

func connectorTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
func nullableConnectorTime(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return connectorTime(value.Time)
}
