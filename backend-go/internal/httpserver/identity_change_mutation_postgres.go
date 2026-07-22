package httpserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

func applyIdentityMutationTx(ctx context.Context, tx *sql.Tx, preview storedIdentityPreview, actorID, orderID, executionID string) (map[string]any, error) {
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, err
	}
	now = now.UTC()
	nowText := now.Format(time.RFC3339Nano)
	rolesBefore, err := commercialRoleStatusSnapshotTx(ctx, tx, preview.userID)
	if err != nil {
		return nil, err
	}
	oldIdentity, oldStatus, err := currentChannelIdentityTx(ctx, tx, preview.userID)
	if err != nil {
		return nil, err
	}
	newIdentity, newStatus := oldIdentity, oldStatus
	sourceOrderID := orderID
	if sourceOrderID == "" {
		sourceOrderID = executionID
	}
	switch preview.action {
	case "UPGRADE":
		newIdentity, newStatus = preview.targetIdentity, "ACTIVE"
		if preview.targetIdentity == "OPERATION_CENTER" {
			if err := terminateActiveBusinessIdentityTx(ctx, tx, preview.userID, "AGENT", actorID, now, "upgraded_to_operation_center"); err != nil {
				return nil, err
			}
		}
		if err := insertActiveBusinessIdentityTx(ctx, tx, preview.userID, preview.targetIdentity, preview.method, sourceOrderID, actorID, now); err != nil {
			return nil, err
		}
	case "FREEZE":
		newStatus = "FROZEN"
		if err := transitionBusinessIdentityTx(ctx, tx, preview.userID, oldIdentity, oldStatus, newStatus, actorID, now, preview.request.Reason); err != nil {
			return nil, err
		}
	case "RESTORE":
		newStatus = "ACTIVE"
		if err := transitionBusinessIdentityTx(ctx, tx, preview.userID, oldIdentity, oldStatus, newStatus, actorID, now, preview.request.Reason); err != nil {
			return nil, err
		}
	case "TERMINATE":
		newStatus = "TERMINATED"
		if err := terminateActiveBusinessIdentityTx(ctx, tx, preview.userID, oldIdentity, actorID, now, preview.request.Reason); err != nil {
			return nil, err
		}
	case "ADJUST_PARENT_AGENT", "ADJUST_OPERATION_CENTER":
	default:
		return nil, fmt.Errorf("unsupported identity mutation action %s", preview.action)
	}

	if err := updateLegacyIdentityProjectionTx(ctx, tx, preview, newIdentity, newStatus, orderID, nowText); err != nil {
		return nil, err
	}
	oldRelation := mapStringSnapshot(preview.result.RelationshipBefore)
	newRelation := mapStringSnapshot(preview.result.RelationshipAfter)
	if oldRelation["parentAgentId"] != newRelation["parentAgentId"] || oldRelation["operationCenterId"] != newRelation["operationCenterId"] || strings.HasPrefix(preview.action, "ADJUST_") {
		if err := replaceIdentityRelationshipTx(ctx, tx, preview.userID, newRelation["parentAgentId"], newRelation["operationCenterId"], actorID, preview.method, now); err != nil {
			return nil, err
		}
	}
	if err := syncCommercialRBACForUser(ctx, tx, preview.userID); err != nil {
		return nil, err
	}
	rolesAfter, err := commercialRoleStatusSnapshotTx(ctx, tx, preview.userID)
	if err != nil {
		return nil, err
	}
	oldSnapshot := map[string]any{"identityType": oldIdentity, "identityStatus": oldStatus}
	newSnapshot := map[string]any{"identityType": newIdentity, "identityStatus": newStatus}
	changeID := "identity_change_" + shortID(executionID)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_identity_change_records(
		  id,tenant_id,user_id,old_identity,new_identity,change_type,source_type,source_order_id,
		  old_parent_agent_id,new_parent_agent_id,old_operation_center_id,new_operation_center_id,
		  reason,remark,operator_id,request_id,created_at
		) VALUES($1,'tenant_default',$2,$3::jsonb,$4::jsonb,$5,$6,nullif($7,''),nullif($8,''),nullif($9,''),nullif($10,''),nullif($11,''),$12,$13,$14,$15,$16)
	`, changeID, preview.userID, jsonProjection(oldSnapshot), jsonProjection(newSnapshot), preview.action, preview.method,
		orderID, oldRelation["parentAgentId"], newRelation["parentAgentId"], oldRelation["operationCenterId"], newRelation["operationCenterId"],
		preview.request.Reason, preview.request.Remark, actorID, executionID, now)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"oldIdentity": oldSnapshot, "newIdentity": newSnapshot,
		"relationshipBefore": oldRelation, "relationshipAfter": newRelation,
		"rolesBefore": rolesBefore, "rolesAfter": rolesAfter,
		"changeRecordId": changeID, "reason": preview.request.Reason,
	}, nil
}

func commercialRoleStatusSnapshotTx(ctx context.Context, tx *sql.Tx, userID string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT role,upper(status) FROM xz_user_roles WHERE user_id=$1 AND role IN ('USER','AGENT','OPERATION') ORDER BY role`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var role, status string
		if err := rows.Scan(&role, &status); err != nil {
			return nil, err
		}
		result[role] = status
	}
	return result, rows.Err()
}

func insertActiveBusinessIdentityTx(ctx context.Context, tx *sql.Tx, userID, identityType, sourceType, sourceOrderID, actorID string, now time.Time) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_user_business_identities WHERE user_id=$1 AND identity_type=$2 AND identity_status IN ('ACTIVE','FROZEN') AND ended_at IS NULL)`, userID, identityType).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errors.New("target business identity is already active")
	}
	var version int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(identity_version),0)+1 FROM xz_user_business_identities WHERE user_id=$1 AND identity_type=$2`, userID, identityType).Scan(&version); err != nil {
		return err
	}
	id := identityChangeStableID("business_identity", userID, identityType, fmt.Sprint(version))
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_user_business_identities(id,tenant_id,user_id,identity_type,identity_status,commission_enabled,source_type,source_order_id,effective_at,identity_version,created_by,created_at,updated_at)
		VALUES($1,'tenant_default',$2,$3,'ACTIVE',true,$4,nullif($5,''),$6,$7,$8,$6,$6)
	`, id, userID, identityType, sourceType, sourceOrderID, now, version, actorID)
	return err
}

func transitionBusinessIdentityTx(ctx context.Context, tx *sql.Tx, userID, identityType, expectedStatus, nextStatus, actorID string, now time.Time, reason string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE xz_user_business_identities SET identity_status=$4,commission_enabled=CASE WHEN $4='ACTIVE' THEN true ELSE false END,status_reason=$5,updated_at=$6,
		  ended_at=CASE WHEN $4='TERMINATED' THEN $6 ELSE ended_at END
		WHERE user_id=$1 AND identity_type=$2 AND identity_status=$3 AND ended_at IS NULL
	`, userID, identityType, expectedStatus, nextStatus, reason, now)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("business identity status changed after preview")
	}
	_ = actorID
	return nil
}

func terminateActiveBusinessIdentityTx(ctx context.Context, tx *sql.Tx, userID, identityType, actorID string, now time.Time, reason string) error {
	if identityType == "USER" || identityType == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE xz_user_business_identities SET identity_status='TERMINATED',commission_enabled=false,status_reason=$3,ended_at=$4,updated_at=$4 WHERE user_id=$1 AND identity_type=$2 AND identity_status IN ('ACTIVE','FROZEN') AND ended_at IS NULL`, userID, identityType, reason, now)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count > 1 {
		return errors.New("multiple current business identities found")
	}
	_ = actorID
	return nil
}

func updateLegacyIdentityProjectionTx(ctx context.Context, tx *sql.Tx, preview storedIdentityPreview, identityType, identityStatus, orderID, now string) error {
	user, err := userByIDForUpdateTx(ctx, tx, preview.userID)
	if err != nil {
		return err
	}
	order := adminOrder{ID: firstNonEmptyString(orderID, "identity_"+shortID(preview.id)), UserID: preview.userID, DirectAgentID: stringValue(preview.result.RelationshipAfter["parentAgentId"]), OperationCenterID: stringValue(preview.result.RelationshipAfter["operationCenterId"]), AmountCents: int(preview.result.PaidAmountCents), Amount: int(preview.result.PaidAmountCents)}
	switch identityType {
	case "AGENT":
		user.AgentStatus = identityStatus
		if preview.action == "UPGRADE" {
			result := commissionSettlementResult{TokenGrantAmount: int(preview.result.TokenDelta)}
			if err := ensureAgentForUserTx(ctx, tx, user, &order, result, now); err != nil {
				return err
			}
		} else if agent, exists, err := channelAgentByUserIDForUpdateTx(ctx, tx, user.ID); err != nil {
			return err
		} else if exists {
			agent.Status = identityStatus
			agent.ParentID = stringValue(preview.result.RelationshipAfter["parentAgentId"])
			agent.OperationCenterID = stringValue(preview.result.RelationshipAfter["operationCenterId"])
			agent.UpdatedAt = now
			if err := insertChannelAgent(ctx, tx, agent); err != nil {
				return err
			}
		}
	case "OPERATION_CENTER":
		user.OperationCenterStatus = identityStatus
		if preview.action == "UPGRADE" {
			user.AgentStatus = "TERMINATED"
			if agent, exists, lookupErr := channelAgentByUserIDForUpdateTx(ctx, tx, user.ID); lookupErr != nil {
				return lookupErr
			} else if exists {
				agent.Status = "TERMINATED"
				agent.UpdatedAt = now
				if err := insertChannelAgent(ctx, tx, agent); err != nil {
					return err
				}
			}
			if err := ensureOperationCenterForUserTx(ctx, tx, user, &order, now); err != nil {
				return err
			}
		} else if center, exists, err := operationCenterByUserIDForUpdateTx(ctx, tx, user.ID); err != nil {
			return err
		} else if exists {
			center.Status = identityStatus
			center.UpdatedAt = now
			if err := insertOperationCenter(ctx, tx, center); err != nil {
				return err
			}
		}
	}
	user.UpdatedAt = now
	return insertUser(ctx, tx, user)
}

func replaceIdentityRelationshipTx(ctx context.Context, tx *sql.Tx, userID, parentAgentID, operationCenterID, actorID, sourceType string, now time.Time) error {
	resolvedCenterID, err := resolveIdentityRelationshipTargetsTx(ctx, tx, userID, parentAgentID, operationCenterID)
	if err != nil {
		return err
	}
	operationCenterID = resolvedCenterID
	if _, err := tx.ExecContext(ctx, `UPDATE xz_user_relationships SET status='ENDED',ended_at=$2,updated_at=$2 WHERE user_id=$1 AND status='ACTIVE' AND ended_at IS NULL`, userID, now); err != nil {
		return err
	}
	if parentAgentID == "" && operationCenterID == "" {
		return nil
	}
	id := identityChangeStableID("user_relationship", userID, now.Format(time.RFC3339Nano))
	_, err = tx.ExecContext(ctx, `INSERT INTO xz_user_relationships(id,tenant_id,user_id,parent_agent_id,operation_center_id,effective_at,status,source_type,created_by,created_at,updated_at) VALUES($1,'tenant_default',$2,nullif($3,''),nullif($4,''),$5,'ACTIVE',$6,$7,$5,$5)`, id, userID, parentAgentID, operationCenterID, now, sourceType, actorID)
	return err
}

func mapStringSnapshot(value map[string]any) map[string]string {
	return map[string]string{"parentAgentId": stringValue(value["parentAgentId"]), "operationCenterId": stringValue(value["operationCenterId"])}
}

func identityChangeStableID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + "_" + hex.EncodeToString(digest[:12])
}
