package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	downgradeTransferAgent = "TRANSFER_TO_AGENT"
	downgradeDirectCenter  = "DIRECT_OPERATION_CENTER"
	downgradeKeepHistory   = "PRESERVE_HISTORY"
)

type storedIdentityDowngrade struct {
	id, userID, actorID, currentIdentity, targetIdentity, status string
	request                                                      identityDowngradeRequest
	preview                                                      identityDowngradePreview
	expiresAt                                                    time.Time
}

func (s *postgresStore) PreviewAdminIdentityDowngrade(actorID, actorRole, userID string, request identityDowngradeRequest) (identityDowngradePreview, error) {
	if strings.TrimSpace(actorID) == "" || strings.ToUpper(strings.TrimSpace(actorRole)) != "SUPER_ADMIN" {
		return identityDowngradePreview{}, errIdentityDowngradePermission
	}
	request = normalizeIdentityDowngradeRequest(request)
	if strings.TrimSpace(userID) == "" || len([]rune(request.Reason)) < 4 {
		return identityDowngradePreview{}, errIdentityDowngradeInvalid
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return identityDowngradePreview{}, err
	}
	preview, err := calculateIdentityDowngradePreview(ctx, s.db, userID, request)
	if err != nil {
		return identityDowngradePreview{}, err
	}
	token, tokenHash, err := newIdentityPreviewToken()
	if err != nil {
		return identityDowngradePreview{}, err
	}
	preview.PreviewID = "identity_downgrade_preview_" + shortID(tokenHash)
	preview.PreviewToken = token
	preview.ExpiresAt = time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339Nano)
	requestJSON, _ := json.Marshal(request)
	storedPreview := preview
	storedPreview.PreviewToken = ""
	resultJSON, _ := json.Marshal(storedPreview)
	_, err = s.db.ExecContext(ctx, `INSERT INTO xz_identity_downgrade_previews(id,token_hash,user_id,actor_id,actor_role,current_identity,target_identity,request_snapshot,result_snapshot,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,nullif($7,''),$8,$9,$10,$11)`, preview.PreviewID, tokenHash, userID, actorID, actorRole, preview.CurrentIdentity, preview.TargetIdentity, requestJSON, resultJSON, preview.Status, parseDowngradeTime(preview.ExpiresAt, time.Now().UTC().Add(15*time.Minute)))
	return preview, err
}

func normalizeIdentityDowngradeRequest(request identityDowngradeRequest) identityDowngradeRequest {
	request.TargetIdentity = strings.ToUpper(strings.TrimSpace(request.TargetIdentity))
	request.ChildStrategy = strings.ToUpper(strings.TrimSpace(request.ChildStrategy))
	request.TargetAgentID = strings.TrimSpace(request.TargetAgentID)
	request.TargetOperationCenterID = strings.TrimSpace(request.TargetOperationCenterID)
	request.Reason = strings.TrimSpace(request.Reason)
	request.Remark = strings.TrimSpace(request.Remark)
	return request
}

type identityDowngradeQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func calculateIdentityDowngradePreview(ctx context.Context, q identityDowngradeQueryer, userID string, request identityDowngradeRequest) (identityDowngradePreview, error) {
	var currentIdentity, entityID string
	err := q.QueryRowContext(ctx, `SELECT identity.identity_type,CASE identity.identity_type WHEN 'AGENT' THEN coalesce((SELECT id FROM xz_channel_agents WHERE user_id=identity.user_id LIMIT 1),'') ELSE coalesce((SELECT id FROM xz_operation_centers WHERE user_id=identity.user_id LIMIT 1),'') END FROM xz_user_business_identities identity WHERE identity.user_id=$1 AND identity.identity_type IN ('AGENT','OPERATION_CENTER') AND identity.identity_status IN ('ACTIVE','FROZEN') AND identity.ended_at IS NULL ORDER BY CASE identity.identity_type WHEN 'OPERATION_CENTER' THEN 0 ELSE 1 END LIMIT 1`, userID).Scan(&currentIdentity, &entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return identityDowngradePreview{}, errIdentityDowngradeInvalid
	}
	if err != nil {
		return identityDowngradePreview{}, err
	}
	if entityID == "" {
		return identityDowngradePreview{}, errIdentityDowngradeInvalid
	}
	if currentIdentity == "AGENT" && request.TargetIdentity != "" {
		return identityDowngradePreview{}, errIdentityDowngradeInvalid
	}
	if currentIdentity == "OPERATION_CENTER" && request.TargetIdentity != "" && request.TargetIdentity != "AGENT" {
		return identityDowngradePreview{}, errIdentityDowngradeInvalid
	}
	if request.ChildStrategy != downgradeTransferAgent && request.ChildStrategy != downgradeDirectCenter && request.ChildStrategy != downgradeKeepHistory {
		return identityDowngradePreview{}, errIdentityDowngradeInvalid
	}

	condition := "relation.parent_agent_id=$2"
	if currentIdentity == "OPERATION_CENTER" {
		condition = "relation.operation_center_id=$2"
	}
	var members, agents, migrationCount int64
	countSQL := `SELECT count(*) FILTER (WHERE EXISTS(SELECT 1 FROM xz_membership_entitlement_records membership WHERE membership.user_id=relation.user_id AND membership.effective_at<=now() AND membership.expires_at>now()) AND NOT EXISTS(SELECT 1 FROM xz_user_business_identities business WHERE business.user_id=relation.user_id AND business.identity_type IN ('AGENT','OPERATION_CENTER') AND business.identity_status IN ('ACTIVE','FROZEN') AND business.ended_at IS NULL)), count(*) FILTER (WHERE EXISTS(SELECT 1 FROM xz_user_business_identities business WHERE business.user_id=relation.user_id AND business.identity_type='AGENT' AND business.identity_status IN ('ACTIVE','FROZEN') AND business.ended_at IS NULL)), count(*) FROM xz_user_relationships relation WHERE relation.user_id<>$1 AND relation.status='ACTIVE' AND relation.ended_at IS NULL AND ` + condition
	if err := q.QueryRowContext(ctx, countSQL, userID, entityID).Scan(&members, &agents, &migrationCount); err != nil {
		return identityDowngradePreview{}, err
	}

	blockers := make([]string, 0)
	warnings := []string{"历史订单、历史分润、Token 和会员权益均保持不变", "降级生效后关闭发展下级和新增分润权限，原工作台转为不可操作状态"}
	if migrationCount > 0 {
		switch request.ChildStrategy {
		case downgradeTransferAgent:
			if request.TargetAgentID == "" {
				blockers = append(blockers, "请选择承接下级的代理商")
			} else if reason, err := validateDowngradeTargetAgent(ctx, q, userID, request.TargetAgentID); err != nil {
				return identityDowngradePreview{}, err
			} else if reason != "" {
				blockers = append(blockers, reason)
			}
		case downgradeDirectCenter:
			if request.TargetOperationCenterID == "" {
				blockers = append(blockers, "请选择承接下级的运营中心")
			} else if reason, err := validateDowngradeTargetCenter(ctx, q, userID, request.TargetOperationCenterID); err != nil {
				return identityDowngradePreview{}, err
			} else if reason != "" {
				blockers = append(blockers, reason)
			}
		case downgradeKeepHistory:
			warnings = append(warnings, "将结束当前归属且不重新分配，下级将失去后续订单和分润归属；确认时必须输入指定文字")
		}
	}

	checks, err := collectIdentityDowngradeChecks(ctx, q, userID, currentIdentity, entityID)
	if err != nil {
		return identityDowngradePreview{}, err
	}
	for _, check := range checks {
		if check.Blocking {
			blockers = append(blockers, check.Label)
		}
	}
	var databaseNow time.Time
	if err := q.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
		return identityDowngradePreview{}, err
	}
	databaseNow = databaseNow.UTC()
	effectiveAt := databaseNow
	if strings.TrimSpace(request.EffectiveAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, request.EffectiveAt)
		if parseErr != nil {
			return identityDowngradePreview{}, errIdentityDowngradeInvalid
		}
		effectiveAt = parsed.UTC()
	}
	status := "READY"
	if len(blockers) > 0 {
		status = "BLOCKED"
		if request.WaitForSettlement {
			status = "WAITING"
		}
	} else if effectiveAt.After(databaseNow.Add(time.Second)) {
		status = "SCHEDULED"
	}
	unassigned := int64(0)
	if request.ChildStrategy == downgradeKeepHistory {
		unassigned = migrationCount
	}
	impact := "历史订单和历史分润不变；仅生效后的新订单按新关系计算"
	return identityDowngradePreview{UserID: userID, CurrentIdentity: currentIdentity, TargetIdentity: request.TargetIdentity, ChildStrategy: request.ChildStrategy, EffectiveAt: effectiveAt.Format(time.RFC3339Nano), WaitForSettlement: request.WaitForSettlement, Checks: checks, DownlineMembers: members, DownlineAgents: agents, MigrationCount: migrationCount, UnassignedCount: unassigned, CommissionImpact: impact, RelationshipBefore: map[string]any{"identityEntityId": entityID, "downlineCount": migrationCount}, RelationshipAfter: map[string]any{"strategy": request.ChildStrategy, "targetAgentId": request.TargetAgentID, "targetOperationCenterId": request.TargetOperationCenterID}, Blockers: blockers, RiskWarnings: warnings, Status: status}, nil
}

func collectIdentityDowngradeChecks(ctx context.Context, q identityDowngradeQueryer, userID, identityType, entityID string) ([]identityDowngradeCheck, error) {
	checks := make([]identityDowngradeCheck, 0, 6)
	var pendingCommission, frozenAmount int64
	if err := q.QueryRowContext(ctx, `SELECT coalesce(expected_cents+available_cents+settling_cents,0),coalesce(frozen_cents,0) FROM xz_commission_wallet_accounts WHERE beneficiary_type=$1 AND beneficiary_id=$2`, identityType, entityID).Scan(&pendingCommission, &frozenAmount); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var legacyFrozen int64
	if identityType == "AGENT" {
		if err := q.QueryRowContext(ctx, `SELECT coalesce(frozen_commission_cents,0) FROM xz_agent_wallets WHERE agent_id=$1`, entityID).Scan(&legacyFrozen); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		frozenAmount += legacyFrozen
	}
	checks = append(checks, identityDowngradeCheck{Code: "PENDING_COMMISSION", Label: "存在待结算分润", AmountCents: pendingCommission, Blocking: pendingCommission > 0}, identityDowngradeCheck{Code: "FROZEN_AMOUNT", Label: "存在冻结金额", AmountCents: frozenAmount, Blocking: frozenAmount > 0})
	var refundCount int64
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM xz_refund_records refund JOIN xz_orders orders ON orders.id=refund.order_id WHERE upper(refund.status) NOT IN ('SUCCEEDED','FAILED','REJECTED','CANCELLED') AND (orders.direct_agent_id=$1 OR orders.parent_agent_id=$1 OR orders.operation_center_id=$1)`, entityID).Scan(&refundCount); err != nil {
		return nil, err
	}
	checks = append(checks, identityDowngradeCheck{Code: "REFUNDING_ORDERS", Label: "存在退款中订单", Count: refundCount, Blocking: refundCount > 0})
	var reconciliationCount int64
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM xz_payout_reconciliation_differences difference LEFT JOIN xz_commission_payout_details detail ON detail.id=difference.payout_detail_id WHERE difference.status='OPEN' AND detail.beneficiary_type=$1 AND detail.beneficiary_id=$2`, identityType, entityID).Scan(&reconciliationCount); err != nil {
		return nil, err
	}
	checks = append(checks, identityDowngradeCheck{Code: "OPEN_RECONCILIATIONS", Label: "存在未完成对账单", Count: reconciliationCount, Blocking: reconciliationCount > 0})
	var agreements int64
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM xz_labor_worker_profiles WHERE user_id=$1 AND subject_type=$2 AND upper(contract_status) IN ('ACTIVE','SIGNED')`, userID, identityType).Scan(&agreements); err != nil {
		return nil, err
	}
	checks = append(checks, identityDowngradeCheck{Code: "ACTIVE_AGREEMENTS", Label: "存在有效协议", Count: agreements, Blocking: agreements > 0})
	var withdrawals int64
	if err := q.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM xz_withdrawals WHERE agent_id=$1 AND upper(coalesce(status,'')) NOT IN ('SETTLED','PAID','APPROVED','REJECTED','CANCELLED','FAILED')) + (SELECT count(*) FROM xz_settlement_applications WHERE applicant_type=$2 AND applicant_id=$1 AND status NOT IN ('COMPLETED','REJECTED'))`, entityID, identityType).Scan(&withdrawals); err != nil {
		return nil, err
	}
	checks = append(checks, identityDowngradeCheck{Code: "UNFINISHED_WITHDRAWALS", Label: "存在未完成提现", Count: withdrawals, Blocking: withdrawals > 0})
	return checks, nil
}

func validateDowngradeTargetAgent(ctx context.Context, q identityDowngradeQueryer, userID, agentID string) (string, error) {
	var targetUser, status string
	if err := q.QueryRowContext(ctx, `SELECT agent.user_id,upper(coalesce(agent.status,'')) FROM xz_channel_agents agent JOIN xz_user_business_identities identity ON identity.user_id=agent.user_id AND identity.identity_type='AGENT' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL AND identity.effective_at<=clock_timestamp() AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp()) WHERE agent.id=$1`, agentID).Scan(&targetUser, &status); errors.Is(err, sql.ErrNoRows) {
		return "新上级代理商不存在", nil
	} else if err != nil {
		return "", err
	}
	if targetUser == userID {
		return "不能迁移给当前用户本人", nil
	}
	if status != "ACTIVE" {
		return "新上级代理商身份无效", nil
	}
	var cycle bool
	err := q.QueryRowContext(ctx, `WITH RECURSIVE ancestors(user_id) AS (SELECT $1::text UNION SELECT parent.user_id FROM ancestors JOIN xz_user_relationships relation ON relation.user_id=ancestors.user_id AND relation.status='ACTIVE' AND relation.ended_at IS NULL JOIN xz_channel_agents parent ON parent.id=relation.parent_agent_id) SELECT EXISTS(SELECT 1 FROM ancestors WHERE user_id=$2)`, targetUser, userID).Scan(&cycle)
	if err != nil {
		return "", err
	}
	if cycle {
		return "新上级位于当前代理层级的下游，会形成循环关系", nil
	}
	return "", nil
}

func validateDowngradeTargetCenter(ctx context.Context, q identityDowngradeQueryer, userID, centerID string) (string, error) {
	var targetUser, status string
	if err := q.QueryRowContext(ctx, `SELECT center.user_id,upper(coalesce(center.status,'')) FROM xz_operation_centers center JOIN xz_user_business_identities identity ON identity.user_id=center.user_id AND identity.identity_type='OPERATION_CENTER' AND identity.identity_status='ACTIVE' AND identity.ended_at IS NULL AND identity.effective_at<=clock_timestamp() AND (identity.expires_at IS NULL OR identity.expires_at>clock_timestamp()) WHERE center.id=$1`, centerID).Scan(&targetUser, &status); errors.Is(err, sql.ErrNoRows) {
		return "承接运营中心不存在", nil
	} else if err != nil {
		return "", err
	}
	if targetUser == userID {
		return "不能迁移给当前用户自己的运营中心", nil
	}
	if status != "ACTIVE" {
		return "承接运营中心身份无效", nil
	}
	return "", nil
}

func parseDowngradeTime(value string, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	return fallback
}

func (s *postgresStore) ConfirmAdminIdentityDowngrade(actorID, actorRole, userID string, request identityDowngradeConfirmRequest) (identityDowngradeResult, error) {
	if strings.ToUpper(strings.TrimSpace(actorRole)) != "SUPER_ADMIN" {
		return identityDowngradeResult{}, errIdentityDowngradePermission
	}
	if !request.HighRiskConfirmed {
		return identityDowngradeResult{}, errIdentityHighRiskConfirm
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return identityDowngradeResult{}, err
	}
	defer tx.Rollback()
	stored, err := loadIdentityDowngradeForUpdate(ctx, tx, userID, request.PreviewToken)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if stored.status == "CONSUMED" {
		var result identityDowngradeResult
		var effectiveAt time.Time
		err = tx.QueryRowContext(ctx, `SELECT id,user_id,status,effective_at,coalesce((result_snapshot->>'migratedMembers')::bigint,0),coalesce((result_snapshot->>'migratedAgents')::bigint,0),coalesce((result_snapshot->>'migratedRelationships')::bigint,0) FROM xz_identity_downgrade_requests WHERE preview_id=$1`, stored.id).Scan(&result.RequestID, &result.UserID, &result.Status, &effectiveAt, &result.MigratedMembers, &result.MigratedAgents, &result.MigratedRelationships)
		result.EffectiveAt = effectiveAt.UTC().Format(time.RFC3339Nano)
		result.Idempotent = true
		return result, err
	}
	if time.Now().UTC().After(stored.expiresAt) {
		return identityDowngradeResult{}, errIdentityDowngradeExpired
	}
	if stored.actorID != actorID {
		return identityDowngradeResult{}, errIdentityDowngradePermission
	}
	preview, err := calculateIdentityDowngradePreview(ctx, tx, userID, stored.request)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if stored.request.ChildStrategy == downgradeKeepHistory && preview.MigrationCount > 0 && strings.TrimSpace(request.ConfirmationText) != "确认结束当前归属" {
		return identityDowngradeResult{}, fmt.Errorf("%w: type 确认结束当前归属 to continue", errIdentityHighRiskConfirm)
	}
	requestID := "identity_downgrade_" + shortID(stored.id)
	effectiveAt := parseDowngradeTime(preview.EffectiveAt, time.Now().UTC())
	status := "PROCESSING"
	if len(preview.Blockers) > 0 {
		if !stored.request.WaitForSettlement {
			return identityDowngradeResult{}, errIdentityDowngradeBlocked
		}
		status = "WAITING"
	} else if effectiveAt.After(time.Now().UTC().Add(time.Second)) {
		status = "SCHEDULED"
	}
	blockersJSON, _ := json.Marshal(preview.Blockers)
	_, err = tx.ExecContext(ctx, `INSERT INTO xz_identity_downgrade_requests(id,preview_id,user_id,actor_id,current_identity,target_identity,child_strategy,target_agent_id,target_operation_center_id,wait_for_settlement,effective_at,status,blocker_snapshot,reason,remark) VALUES($1,$2,$3,$4,$5,nullif($6,''),$7,nullif($8,''),nullif($9,''),$10,$11,$12,$13,$14,$15)`, requestID, stored.id, userID, actorID, preview.CurrentIdentity, preview.TargetIdentity, stored.request.ChildStrategy, stored.request.TargetAgentID, stored.request.TargetOperationCenterID, stored.request.WaitForSettlement, effectiveAt, status, blockersJSON, stored.request.Reason, stored.request.Remark)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	result := identityDowngradeResult{RequestID: requestID, UserID: userID, Status: status, EffectiveAt: effectiveAt.Format(time.RFC3339Nano)}
	if status == "PROCESSING" {
		result, err = executeIdentityDowngradeTx(ctx, tx, requestID, actorID, actorRole, stored.request, preview)
		if err != nil {
			return identityDowngradeResult{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE xz_identity_downgrade_previews SET status='CONSUMED',consumed_at=now(),request_id=$2,updated_at=now() WHERE id=$1`, stored.id, requestID)
	if err != nil {
		return identityDowngradeResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return identityDowngradeResult{}, err
	}
	return result, nil
}

func loadIdentityDowngradeForUpdate(ctx context.Context, tx *sql.Tx, userID, token string) (storedIdentityDowngrade, error) {
	var item storedIdentityDowngrade
	var requestJSON, resultJSON []byte
	err := tx.QueryRowContext(ctx, `SELECT id,user_id,actor_id,current_identity,coalesce(target_identity,''),status,request_snapshot,result_snapshot,expires_at FROM xz_identity_downgrade_previews WHERE token_hash=$1 AND user_id=$2 FOR UPDATE`, identityPreviewTokenHash(token), userID).Scan(&item.id, &item.userID, &item.actorID, &item.currentIdentity, &item.targetIdentity, &item.status, &requestJSON, &resultJSON, &item.expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errIdentityDowngradeNotFound
	}
	if err != nil {
		return item, err
	}
	if json.Unmarshal(requestJSON, &item.request) != nil || json.Unmarshal(resultJSON, &item.preview) != nil {
		return item, errIdentityDowngradeInvalid
	}
	return item, nil
}

func (s *postgresStore) ListAdminIdentityDowngrades(actorID, actorRole, userID string) ([]identityDowngradeResult, error) {
	if strings.ToUpper(strings.TrimSpace(actorRole)) != "SUPER_ADMIN" {
		return nil, errIdentityDowngradePermission
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,status,effective_at,coalesce((result_snapshot->>'migratedMembers')::bigint,0),coalesce((result_snapshot->>'migratedAgents')::bigint,0),coalesce((result_snapshot->>'migratedRelationships')::bigint,0),blocker_snapshot,coalesce(failure_message,''),last_checked_at,created_at FROM xz_identity_downgrade_requests WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]identityDowngradeResult, 0)
	for rows.Next() {
		var item identityDowngradeResult
		var effective, created time.Time
		var lastChecked sql.NullTime
		var blockersJSON []byte
		if err := rows.Scan(&item.RequestID, &item.UserID, &item.Status, &effective, &item.MigratedMembers, &item.MigratedAgents, &item.MigratedRelationships, &blockersJSON, &item.FailureMessage, &lastChecked, &created); err != nil {
			return nil, err
		}
		item.EffectiveAt = effective.UTC().Format(time.RFC3339Nano)
		item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		if lastChecked.Valid {
			item.LastCheckedAt = lastChecked.Time.UTC().Format(time.RFC3339Nano)
		}
		_ = json.Unmarshal(blockersJSON, &item.Blockers)
		item.TimeoutWarning = item.Status == "WAITING" && time.Since(created) > 72*time.Hour
		items = append(items, item)
	}
	_ = actorID
	return items, rows.Err()
}

func (s *postgresStore) ProcessDueIdentityDowngrades(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		processed, err := s.processOneDueIdentityDowngrade(ctx)
		if err != nil {
			return err
		}
		if !processed {
			return nil
		}
	}
	return nil
}

func (s *postgresStore) processOneDueIdentityDowngrade(ctx context.Context) (bool, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, userID, actorID, actorRole string
	var requestJSON []byte
	err = tx.QueryRowContext(ctx, `SELECT id,user_id,actor_id,(SELECT actor_role FROM xz_identity_downgrade_previews WHERE id=request.preview_id),(SELECT request_snapshot FROM xz_identity_downgrade_previews WHERE id=request.preview_id) FROM xz_identity_downgrade_requests request WHERE request.status IN ('WAITING','SCHEDULED') AND request.effective_at<=now() ORDER BY request.effective_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &userID, &actorID, &actorRole, &requestJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var request identityDowngradeRequest
	if err = json.Unmarshal(requestJSON, &request); err != nil {
		return false, err
	}
	preview, err := calculateIdentityDowngradePreview(ctx, tx, userID, request)
	if err != nil {
		return false, err
	}
	blockersJSON, _ := json.Marshal(preview.Blockers)
	if len(preview.Blockers) > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE xz_identity_downgrade_requests SET status='WAITING',blocker_snapshot=$2,last_checked_at=now(),updated_at=now() WHERE id=$1`, id, blockersJSON)
		if err == nil {
			err = tx.Commit()
		}
		return true, err
	}
	if _, err = executeIdentityDowngradeTx(ctx, tx, id, actorID, actorRole, request, preview); err != nil {
		_ = tx.Rollback()
		if _, recordErr := s.db.ExecContext(ctx, `UPDATE xz_identity_downgrade_requests SET status='FAILED',failure_message=$2,last_checked_at=now(),updated_at=now() WHERE id=$1`, id, err.Error()); recordErr != nil {
			return true, errors.Join(err, recordErr)
		}
		return true, err
	}
	return true, tx.Commit()
}

func identityDowngradeResultJSON(result identityDowngradeResult) []byte {
	value, _ := json.Marshal(result)
	return value
}

func formatDowngradeAuditReason(request identityDowngradeRequest) string {
	return fmt.Sprintf("%s; child_strategy=%s", request.Reason, request.ChildStrategy)
}
