package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// resolveIdentityRelationshipTargetsTx validates the canonical relationship
// against business identities and profiles. When a parent agent is selected,
// its operation center is authoritative and cannot be overridden manually.
func resolveIdentityRelationshipTargetsTx(ctx context.Context, tx *sql.Tx, userID, parentAgentID, operationCenterID string) (string, error) {
	if parentAgentID != "" {
		var parentUserID, derivedCenterID string
		err := tx.QueryRowContext(ctx, `
			SELECT agent.user_id,
			       coalesce(relation.operation_center_id, agent.operation_center_id, '')
			FROM xz_channel_agents agent
			JOIN xz_users account ON account.id=agent.user_id AND upper(coalesce(account.status,''))='ACTIVE'
			JOIN xz_user_business_identities identity
			  ON identity.user_id=agent.user_id
			 AND identity.identity_type='AGENT'
			 AND identity.identity_status='ACTIVE'
			 AND identity.ended_at IS NULL
			 AND identity.effective_at<=clock_timestamp()
			 AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
			LEFT JOIN xz_user_relationships relation
			  ON relation.user_id=agent.user_id AND relation.status='ACTIVE' AND relation.ended_at IS NULL
			WHERE agent.id=$1 AND upper(coalesce(agent.status,''))='ACTIVE'
			FOR SHARE OF agent, account, identity
		`, parentAgentID).Scan(&parentUserID, &derivedCenterID)
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("parent agent does not have an active business identity and profile")
		}
		if err != nil {
			return "", err
		}
		if parentUserID == userID {
			return "", errors.New("user cannot be their own parent agent")
		}
		if operationCenterID != "" && operationCenterID != derivedCenterID {
			return "", errors.New("operation center must be derived from the selected parent agent")
		}
		operationCenterID = derivedCenterID

		var cyclic bool
		if err := tx.QueryRowContext(ctx, `
			WITH RECURSIVE ancestors(user_id) AS (
			  SELECT $1::text
			  UNION
			  SELECT parent.user_id
			  FROM ancestors child
			  JOIN xz_user_relationships relation
			    ON relation.user_id=child.user_id AND relation.status='ACTIVE' AND relation.ended_at IS NULL
			  JOIN xz_channel_agents parent ON parent.id=relation.parent_agent_id
			)
			SELECT EXISTS(SELECT 1 FROM ancestors WHERE user_id=$2)
		`, parentUserID, userID).Scan(&cyclic); err != nil {
			return "", err
		}
		if cyclic {
			return "", errors.New("cyclic agent relationship is forbidden")
		}
	}

	if operationCenterID != "" {
		var centerUserID string
		err := tx.QueryRowContext(ctx, `
			SELECT center.user_id
			FROM xz_operation_centers center
			JOIN xz_users account ON account.id=center.user_id AND upper(coalesce(account.status,''))='ACTIVE'
			JOIN xz_user_business_identities identity
			  ON identity.user_id=center.user_id
			 AND identity.identity_type='OPERATION_CENTER'
			 AND identity.identity_status='ACTIVE'
			 AND identity.ended_at IS NULL
			 AND identity.effective_at<=clock_timestamp()
			 AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp())
			WHERE center.id=$1 AND upper(coalesce(center.status,''))='ACTIVE'
			FOR SHARE OF center, account, identity
		`, operationCenterID).Scan(&centerUserID)
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("operation center does not have an active business identity and profile")
		}
		if err != nil {
			return "", err
		}
		if centerUserID == userID {
			return "", errors.New("user cannot belong to their own operation center")
		}
	}
	return operationCenterID, nil
}

func lockIdentityCommandUserTx(ctx context.Context, tx *sql.Tx, userID string) error {
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "identity-command:"+userID); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT true FROM xz_users WHERE id=$1 FOR UPDATE`, userID).Scan(&exists); err != nil {
		return err
	}
	return nil
}
