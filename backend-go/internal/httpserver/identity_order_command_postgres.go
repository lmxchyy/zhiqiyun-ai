package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// fulfillCommercialIdentityForOrderTx is the order-facing identity command.
// The caller owns the transaction, so order, token, commission, profile,
// relationship, identity, RBAC and audit either all commit or all roll back.
func fulfillCommercialIdentityForOrderTx(ctx context.Context, tx *sql.Tx, order *adminOrder, plan adminPlan, result commissionSettlementResult, nowText string) error {
	identityType := ""
	switch planBusinessType(plan) {
	case planTypeAgentJoinPackage:
		identityType = "AGENT"
	case planTypeOperationCenterPackage:
		identityType = "OPERATION_CENTER"
	default:
		return nil
	}
	if err := lockIdentityCommandUserTx(ctx, tx, order.UserID); err != nil {
		return err
	}
	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	now = now.UTC()
	nowText = now.Format(time.RFC3339Nano)
	user, err := userByIDForUpdateTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	oldType, oldStatus, err := currentChannelIdentityTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	if oldType != "USER" && oldType != identityType {
		if identityType != "OPERATION_CENTER" || oldType != "AGENT" {
			return fmt.Errorf("commercial identity %s cannot be fulfilled while %s is current", identityType, oldType)
		}
		if err := terminateActiveBusinessIdentityTx(ctx, tx, order.UserID, "AGENT", "system:commerce", now, "upgraded_to_operation_center_by_order"); err != nil {
			return err
		}
		if agent, exists, lookupErr := channelAgentByUserIDForUpdateTx(ctx, tx, order.UserID); lookupErr != nil {
			return lookupErr
		} else if exists {
			agent.Status = "TERMINATED"
			agent.UpdatedAt = nowText
			if err := insertChannelAgent(ctx, tx, agent); err != nil {
				return err
			}
		}
	}

	relation, err := currentIdentityRelationshipTx(ctx, tx, order.UserID)
	if err != nil {
		return err
	}
	parentID, centerID := relation.ParentAgentID, relation.OperationCenterID
	if relation.ID == "" {
		parentID, centerID = order.DirectAgentID, order.OperationCenterID
		resolvedCenter, resolveErr := resolveIdentityRelationshipTargetsTx(ctx, tx, order.UserID, parentID, centerID)
		if resolveErr != nil {
			return resolveErr
		}
		centerID = resolvedCenter
		if parentID != "" || centerID != "" {
			if err := replaceIdentityRelationshipTx(ctx, tx, order.UserID, parentID, centerID, "system:commerce", "COMMERCE_ORDER", now); err != nil {
				return err
			}
		}
	} else {
		resolvedCenter, resolveErr := resolveIdentityRelationshipTargetsTx(ctx, tx, order.UserID, parentID, centerID)
		if resolveErr != nil {
			return resolveErr
		}
		centerID = resolvedCenter
	}
	profileOrder := *order
	profileOrder.DirectAgentID = parentID
	profileOrder.OperationCenterID = centerID

	var currentExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_user_business_identities WHERE user_id=$1 AND identity_type=$2 AND identity_status IN ('ACTIVE','FROZEN') AND ended_at IS NULL)`, order.UserID, identityType).Scan(&currentExists); err != nil {
		return err
	}
	if currentExists && oldStatus != "ACTIVE" {
		return fmt.Errorf("existing %s identity is not active", identityType)
	}
	if !currentExists {
		if err := insertActiveBusinessIdentityTx(ctx, tx, order.UserID, identityType, "COMMERCE_ORDER", order.ID, "system:commerce", now); err != nil {
			return err
		}
	}
	if identityType == "AGENT" {
		user.AgentStatus = "ACTIVE"
		if user.MemberLevel == "" {
			user.MemberLevel = memberLevelFree
		}
		if user.Role == "" {
			user.Role = "MEMBER"
		}
		if err := ensureAgentForUserTx(ctx, tx, user, &profileOrder, result, nowText); err != nil {
			return err
		}
	} else {
		user.OperationCenterStatus = "ACTIVE"
		user.AgentStatus = "TERMINATED"
		if user.MemberLevel == "" {
			user.MemberLevel = memberLevelFree
		}
		if err := ensureOperationCenterForUserTx(ctx, tx, user, &profileOrder, nowText); err != nil {
			return err
		}
	}
	user.UpdatedAt = nowText
	if err := insertUser(ctx, tx, user); err != nil {
		return err
	}
	if err := syncCommercialRBACForUser(ctx, tx, order.UserID); err != nil {
		return err
	}

	changeID := identityChangeStableID("identity_change_order", order.ID, identityType)
	oldJSON, _ := json.Marshal(map[string]any{"identityType": oldType, "identityStatus": oldStatus})
	newJSON, _ := json.Marshal(map[string]any{"identityType": identityType, "identityStatus": "ACTIVE"})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_identity_change_records(
		 id,tenant_id,user_id,old_identity,new_identity,change_type,source_type,source_order_id,
		 new_parent_agent_id,new_operation_center_id,reason,operator_id,request_id,idempotency_key,created_at)
		VALUES($1,'tenant_default',$2,$3,$4,'UPGRADE','COMMERCE_ORDER',$5,nullif($6,''),nullif($7,''),
		 'commerce order fulfillment','system:commerce',$5,$8,$9)
		ON CONFLICT DO NOTHING
	`, changeID, order.UserID, oldJSON, newJSON, order.ID, parentID, centerID, "commerce-order:"+order.ID+":"+identityType, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_operation_logs(id,actor_id,operation,target,target_id,before_state,after_state) VALUES($1,'system:commerce','IDENTITY_ORDER_FULFILLMENT','user_identity',$2,$3,$4) ON CONFLICT(id) DO NOTHING`, "operation_"+shortID(changeID), order.UserID, oldJSON, newJSON); err != nil {
		return err
	}
	return insertAuditLog(ctx, tx, "system:commerce", "SYSTEM", "identity.order_fulfillment", "user_identity", order.UserID, "SYSTEM", "commerce-order:"+order.ID, 200, map[string]any{"orderId": order.ID, "identityType": identityType})
}
