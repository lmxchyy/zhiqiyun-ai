package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type downgradeChildRelationship struct {
	id, userID, parentAgentID, operationCenterID string
	isAgent                                      bool
}

func executeIdentityDowngradeTx(ctx context.Context, tx *sql.Tx, requestID, actorID, actorRole string, request identityDowngradeRequest, preview identityDowngradePreview) (identityDowngradeResult, error) {
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return identityDowngradeResult{}, err
	}
	now = now.UTC()
	if err := lockIdentityCommandUserTx(ctx, tx, preview.UserID); err != nil {
		return identityDowngradeResult{}, err
	}
	rolesBefore, err := commercialRoleStatusSnapshotTx(ctx, tx, preview.UserID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	entityID, err := lockDowngradeIdentityTx(ctx, tx, preview.UserID, preview.CurrentIdentity)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	children, err := downgradeChildrenForUpdateTx(ctx, tx, preview.UserID, preview.CurrentIdentity, entityID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	result := identityDowngradeResult{RequestID: requestID, UserID: preview.UserID, Status: "SUCCEEDED", EffectiveAt: now.Format(time.RFC3339Nano)}
	for _, child := range children {
		if _, err := tx.ExecContext(ctx, `UPDATE xz_user_relationships SET status='ENDED',ended_at=$2,updated_at=$2 WHERE id=$1`, child.id, now); err != nil {
			return identityDowngradeResult{}, err
		}
		newParent, newCenter := "", ""
		switch request.ChildStrategy {
		case downgradeTransferAgent:
			newParent = request.TargetAgentID
			if err := tx.QueryRowContext(ctx, `SELECT coalesce(operation_center_id,'') FROM xz_channel_agents WHERE id=$1 AND upper(coalesce(status,''))='ACTIVE'`, newParent).Scan(&newCenter); err != nil {
				return identityDowngradeResult{}, err
			}
		case downgradeDirectCenter:
			newCenter = request.TargetOperationCenterID
		case downgradeKeepHistory:
			newCenter = child.operationCenterID
			if preview.CurrentIdentity == "OPERATION_CENTER" && newCenter == entityID {
				newCenter = ""
			}
		}
		if newParent != "" || newCenter != "" {
			if err := replaceIdentityRelationshipTx(ctx, tx, child.userID, newParent, newCenter, actorID, "CONTROLLED_DOWNGRADE", now); err != nil {
				return identityDowngradeResult{}, err
			}
		}
		result.MigratedRelationships++
		if child.isAgent {
			result.MigratedAgents++
		} else {
			result.MigratedMembers++
		}
	}

	if err := terminateActiveBusinessIdentityTx(ctx, tx, preview.UserID, preview.CurrentIdentity, actorID, now, "controlled_downgrade"); err != nil {
		return identityDowngradeResult{}, err
	}
	user, err := userByIDForUpdateTx(ctx, tx, preview.UserID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if preview.CurrentIdentity == "AGENT" {
		user.AgentStatus = "TERMINATED"
		if _, err := tx.ExecContext(ctx, `UPDATE xz_channel_agents SET status='TERMINATED',updated_at=$2 WHERE id=$1`, entityID, now.Format(time.RFC3339Nano)); err != nil {
			return identityDowngradeResult{}, err
		}
	} else {
		user.OperationCenterStatus = "TERMINATED"
		if _, err := tx.ExecContext(ctx, `UPDATE xz_operation_centers SET status='TERMINATED',updated_at=$2 WHERE id=$1`, entityID, now.Format(time.RFC3339Nano)); err != nil {
			return identityDowngradeResult{}, err
		}
		if preview.TargetIdentity == "AGENT" {
			if err := insertActiveBusinessIdentityTx(ctx, tx, preview.UserID, "AGENT", "CONTROLLED_DOWNGRADE", requestID, actorID, now); err != nil {
				return identityDowngradeResult{}, err
			}
			order := adminOrder{ID: requestID, UserID: preview.UserID}
			if err := ensureAgentForUserTx(ctx, tx, user, &order, commissionSettlementResult{}, now.Format(time.RFC3339Nano)); err != nil {
				return identityDowngradeResult{}, err
			}
			user.AgentStatus = "ACTIVE"
		}
	}
	user.UpdatedAt = now.Format(time.RFC3339Nano)
	if err := insertUser(ctx, tx, user); err != nil {
		return identityDowngradeResult{}, err
	}
	if err := syncCommercialRBACForUser(ctx, tx, preview.UserID); err != nil {
		return identityDowngradeResult{}, err
	}
	rolesAfter, err := commercialRoleStatusSnapshotTx(ctx, tx, preview.UserID)
	if err != nil {
		return identityDowngradeResult{}, err
	}

	newIdentity := map[string]any{"identityType": preview.TargetIdentity, "identityStatus": "ACTIVE"}
	if preview.TargetIdentity == "" {
		newIdentity = map[string]any{"identityType": preview.CurrentIdentity, "identityStatus": "TERMINATED"}
	}
	oldIdentity := map[string]any{"identityType": preview.CurrentIdentity, "identityStatus": "ACTIVE"}
	oldJSON, _ := json.Marshal(oldIdentity)
	newJSON, _ := json.Marshal(newIdentity)
	changeID := "identity_change_" + shortID(requestID)
	_, err = tx.ExecContext(ctx, `INSERT INTO xz_identity_change_records(id,tenant_id,user_id,old_identity,new_identity,change_type,source_type,reason,remark,operator_id,request_id,idempotency_key,created_at) VALUES($1,'tenant_default',$2,$3,$4,'DOWNGRADE','CONTROLLED_DOWNGRADE',$5,$6,$7,$8,$8,$9)`, changeID, preview.UserID, oldJSON, newJSON, request.Reason, request.Remark, actorID, requestID, now)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	resultJSON := identityDowngradeResultJSON(result)
	_, err = tx.ExecContext(ctx, `UPDATE xz_identity_downgrade_requests SET status='SUCCEEDED',result_snapshot=$2,last_checked_at=$3,completed_at=$3,updated_at=$3 WHERE id=$1`, requestID, resultJSON, now)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	beforeJSON, _ := json.Marshal(map[string]any{"identity": oldIdentity, "roles": rolesBefore, "downlineCount": len(children)})
	afterJSON, _ := json.Marshal(map[string]any{"identity": newIdentity, "roles": rolesAfter, "migration": result, "reason": formatDowngradeAuditReason(request)})
	_, err = tx.ExecContext(ctx, `INSERT INTO xz_operation_logs(id,actor_id,operation,target,target_id,before_state,after_state) VALUES($1,$2,'IDENTITY_DOWNGRADE','user_identity',$3,$4,$5)`, "operation_"+shortID(requestID), actorID, preview.UserID, beforeJSON, afterJSON)
	if err == nil {
		err = insertAuditLog(ctx, tx, actorID, actorRole, "admin.identity_downgrade.confirm", "user_identity", preview.UserID, "POST", "/api/v1/admin/users/"+preview.UserID+"/identity-downgrade/confirm", 200, map[string]any{"requestId": requestID, "reason": request.Reason, "rolesBefore": rolesBefore, "rolesAfter": rolesAfter})
	}
	return result, err
}

func lockDowngradeIdentityTx(ctx context.Context, tx *sql.Tx, userID, identityType string) (string, error) {
	var entityID string
	err := tx.QueryRowContext(ctx, `SELECT CASE $2 WHEN 'AGENT' THEN coalesce((SELECT id FROM xz_channel_agents WHERE user_id=$1 LIMIT 1),'') ELSE coalesce((SELECT id FROM xz_operation_centers WHERE user_id=$1 LIMIT 1),'') END FROM xz_user_business_identities WHERE user_id=$1 AND identity_type=$2 AND identity_status IN ('ACTIVE','FROZEN') AND ended_at IS NULL FOR UPDATE`, userID, identityType).Scan(&entityID)
	if err != nil {
		return "", err
	}
	if entityID == "" {
		return "", fmt.Errorf("downgrade identity projection is missing")
	}
	return entityID, nil
}

func downgradeChildrenForUpdateTx(ctx context.Context, tx *sql.Tx, userID, identityType, entityID string) ([]downgradeChildRelationship, error) {
	condition := "relation.parent_agent_id=$2"
	if identityType == "OPERATION_CENTER" {
		condition = "relation.operation_center_id=$2"
	}
	rows, err := tx.QueryContext(ctx, `SELECT relation.id,relation.user_id,coalesce(relation.parent_agent_id,''),coalesce(relation.operation_center_id,''),EXISTS(SELECT 1 FROM xz_user_business_identities business WHERE business.user_id=relation.user_id AND business.identity_type='AGENT' AND business.identity_status IN ('ACTIVE','FROZEN') AND business.ended_at IS NULL) FROM xz_user_relationships relation WHERE relation.user_id<>$1 AND relation.status='ACTIVE' AND relation.ended_at IS NULL AND `+condition+` ORDER BY relation.id FOR UPDATE`, userID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]downgradeChildRelationship, 0)
	for rows.Next() {
		var item downgradeChildRelationship
		if err := rows.Scan(&item.id, &item.userID, &item.parentAgentID, &item.operationCenterID, &item.isAgent); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
