package operationcenter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type PostgresReferralRewardGrantService struct {
	store *PostgresStore
}

func NewPostgresReferralRewardGrantService(store *PostgresStore) *PostgresReferralRewardGrantService {
	return &PostgresReferralRewardGrantService{store: store}
}

func (service *PostgresReferralRewardGrantService) GrantForServiceOrder(ctx context.Context, tx *sql.Tx, item *OperationCenterServiceOrder) ([]ReferralReward, error) {
	if tx == nil {
		return nil, ErrTransactionRequired
	}
	if item == nil || strings.TrimSpace(item.OrderID) == "" {
		return nil, ErrConstraintViolation
	}
	var eventID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM xz_referral_events WHERE source_order_id=$1`, item.OrderID).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return service.CreateRewardsForReferralEvent(ctx, tx, eventID)
}

func (service *PostgresReferralRewardGrantService) CreateRewardsForReferralEvent(ctx context.Context, tx *sql.Tx, eventID string) ([]ReferralReward, error) {
	if tx == nil {
		return nil, ErrTransactionRequired
	}
	if service == nil || service.store == nil || strings.TrimSpace(eventID) == "" {
		return nil, ErrConstraintViolation
	}
	var event ReferralEvent
	err := tx.QueryRowContext(ctx, `
		SELECT id,tenant_id,referred_operation_center_user_id,referrer_type,referrer_user_id,
		       referrer_operation_center_user_id,source_order_id,source_order_no,payment_status_snapshot,
		       review_status_snapshot,operation_center_status_snapshot,relationship_snapshot,
		       triggered_at,status,idempotency_key,created_at,updated_at
		FROM xz_referral_events WHERE id=$1 FOR UPDATE
	`, eventID).Scan(&event.ID, &event.TenantID, &event.ReferredOperationCenterUserID, &event.ReferrerType,
		&event.ReferrerUserID, &event.ReferrerOperationCenterUserID, &event.SourceOrderID, &event.SourceOrderNo,
		&event.PaymentStatusSnapshot, &event.ReviewStatusSnapshot, &event.OperationCenterStatusSnapshot,
		&event.RelationshipSnapshot, &event.TriggeredAt, &event.Status, &event.IdempotencyKey,
		&event.CreatedAt, &event.UpdatedAt)
	if err != nil {
		return nil, mapPostgresStoreError("lock referral event", err)
	}
	if event.Status == ReferralEventRewarded {
		return loadGrantedReferralRewards(ctx, tx, event.ID)
	}
	if event.Status != ReferralEventReady {
		return nil, fmt.Errorf("%w: %s", ErrReferralEventNotReady, event.Status)
	}
	eligibilities, err := lockReferralEligibilities(ctx, tx, event.ID)
	if err != nil {
		return nil, err
	}
	if len(eligibilities) == 0 {
		return nil, ErrReferralRuleSnapshotMissing
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	plans := make([]referralRewardGrantPlan, 0, len(eligibilities))
	for _, eligibility := range eligibilities {
		if eligibility.Status == ReferralEligibilityCancelled {
			continue
		}
		if eligibility.Status != ReferralEligibilityEligible {
			return nil, ErrEligibilityAlreadyConsumed
		}
		rule, loadErr := loadPinnedReferralRewardRule(ctx, tx, eligibility)
		if loadErr != nil {
			return nil, loadErr
		}
		plan, planErr := buildReferralRewardGrantPlan(event, eligibility, rule, now)
		if planErr != nil {
			return nil, planErr
		}
		plans = append(plans, plan)
	}
	if len(plans) == 0 {
		return nil, ErrReferralRuleSnapshotMissing
	}
	wallets, err := lockReferralWallets(ctx, tx, plans, now)
	if err != nil {
		return nil, err
	}
	txStoreValue, err := service.store.BindTx(tx)
	if err != nil {
		return nil, err
	}
	txStore := txStoreValue.(Store)
	transactionGroupID := stableWorkflowID("referral_reward_grant_tx", event.ID)
	result := make([]ReferralReward, 0, len(plans))
	for _, plan := range plans {
		reward := plan.Reward
		if err := insertReferralReward(ctx, tx, reward); err != nil {
			return nil, err
		}
		walletKey := referralWalletKey(reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID)
		wallet := wallets[walletKey]
		ledgerID, creditErr := creditFrozenReferralWallet(ctx, tx, wallet, reward, now)
		if creditErr != nil {
			return nil, creditErr
		}
		wallet.Frozen += reward.AmountCents
		wallets[walletKey] = wallet
		releaseKey := "referral-reward-release:" + reward.ID
		releaseTask := ReferralRewardReleaseTask{
			ID: stableWorkflowID("referral_reward_release", reward.ID), TenantID: reward.TenantID,
			ReferralRewardID: reward.ID, IdempotencyKey: releaseKey,
			Status: ReferralRewardReleasePending, ExecuteAt: reward.FreezeUntil,
			FailureDetail: JSONSnapshot{}, CreatedAt: now, UpdatedAt: now,
		}
		if createErr := txStore.CreateRewardReleaseTask(ctx, &releaseTask); createErr != nil {
			if errors.Is(createErr, ErrIdempotencyConflict) || errors.Is(createErr, ErrUniqueConflict) {
				return nil, fmt.Errorf("%w: %v", ErrRewardReleaseTaskConflict, createErr)
			}
			return nil, createErr
		}
		reward.GrantWalletLedgerID = ledgerID
		reward.CurrentReleaseTaskID = releaseTask.ID
		if _, updateErr := tx.ExecContext(ctx, `
			UPDATE xz_referral_rewards
			SET grant_wallet_ledger_id=$2,current_release_task_id=$3,updated_at=$4
			WHERE id=$1
		`, reward.ID, ledgerID, releaseTask.ID, now); updateErr != nil {
			return nil, mapPostgresStoreError("link referral reward grant", updateErr)
		}
		updated, updateErr := tx.ExecContext(ctx, `
			UPDATE xz_referral_eligibilities
			SET eligibility_status='CONSUMED',consumed_at=$2,reward_id=$3,updated_at=$2
			WHERE id=$1 AND eligibility_status='ELIGIBLE'
		`, plan.Eligibility.ID, now, reward.ID)
		if updateErr != nil {
			return nil, mapPostgresStoreError("consume referral eligibility", updateErr)
		}
		if rows, rowsErr := updated.RowsAffected(); rowsErr != nil || rows != 1 {
			return nil, ErrEligibilityAlreadyConsumed
		}
		if auditErr := appendReferralGrantAudits(ctx, txStore, reward, releaseTask, transactionGroupID, now); auditErr != nil {
			return nil, auditErr
		}
		result = append(result, reward)
	}
	updated, err := tx.ExecContext(ctx, `UPDATE xz_referral_events SET status='REWARDED',updated_at=$2 WHERE id=$1 AND status='READY'`, event.ID, now)
	if err != nil {
		return nil, mapPostgresStoreError("complete referral event", err)
	}
	if rows, rowsErr := updated.RowsAffected(); rowsErr != nil || rows != 1 {
		return nil, ErrRewardGrantConflict
	}
	return result, nil
}

func lockReferralEligibilities(ctx context.Context, tx *sql.Tx, eventID string) ([]ReferralEligibility, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id,tenant_id,referral_event_id,commercial_rule_set_id,commercial_rule_set_version,
		       referral_rule_version_id,referral_rule_version,beneficiary_type,beneficiary_user_id,
		       beneficiary_relation,relationship_snapshot,eligibility_status,idempotency_key,
		       reward_id,consumed_at,created_at,updated_at
		FROM xz_referral_eligibilities WHERE referral_event_id=$1 ORDER BY id FOR UPDATE
	`, eventID)
	if err != nil {
		return nil, mapPostgresStoreError("lock referral eligibilities", err)
	}
	defer rows.Close()
	var result []ReferralEligibility
	for rows.Next() {
		var item ReferralEligibility
		var rewardID sql.NullString
		var consumedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ReferralEventID, &item.CommercialRuleSetID,
			&item.CommercialRuleSetVersion, &item.ReferralRuleVersionID, &item.ReferralRuleVersion,
			&item.BeneficiaryType, &item.BeneficiaryUserID, &item.BeneficiaryRelation,
			&item.RelationshipSnapshot, &item.Status, &item.IdempotencyKey, &rewardID, &consumedAt,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, mapPostgresStoreError("scan referral eligibility", err)
		}
		item.RewardID = nullableStringPointer(rewardID)
		item.ConsumedAt = nullableTimePointer(consumedAt)
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadPinnedReferralRewardRule(ctx context.Context, tx *sql.Tx, eligibility ReferralEligibility) (ReferralRewardRuleSnapshot, error) {
	var rule ReferralRewardRuleSnapshot
	err := tx.QueryRowContext(ctx, `
		SELECT rule.id,rule.tenant_id,rule.rule_set_id,rule_set.version,rule.version,
		       rule.beneficiary_type,rule.beneficiary_relation,rule.amount_cents,rule.freeze_days
		FROM xz_referral_reward_rule_versions rule
		JOIN xz_commercial_rule_sets rule_set ON rule_set.id=rule.rule_set_id
		WHERE rule.id=$1 AND rule.tenant_id=$2 AND rule.rule_set_id=$3
		  AND rule.version=$4 AND rule_set.version=$5
	`, eligibility.ReferralRuleVersionID, eligibility.TenantID, eligibility.CommercialRuleSetID,
		eligibility.ReferralRuleVersion, eligibility.CommercialRuleSetVersion).Scan(
		&rule.ID, &rule.TenantID, &rule.RuleSetID, &rule.RuleSetVersion, &rule.Version,
		&rule.BeneficiaryType, &rule.BeneficiaryRelation, &rule.AmountCents, &rule.FreezeDays)
	if errors.Is(err, sql.ErrNoRows) {
		return rule, ErrReferralRuleSnapshotMissing
	}
	if err != nil {
		return rule, mapPostgresStoreError("load pinned referral reward rule", err)
	}
	return rule, nil
}

type referralWalletBalances struct {
	Expected, Frozen, Available, Settling, Settled, Recoverable int64
}

type referralWalletState struct {
	ID, TenantID, BeneficiaryType, BeneficiaryID, Status string
	referralWalletBalances
}

func referralWalletKey(tenantID string, beneficiaryType ReferralBeneficiaryType, beneficiaryID string) string {
	return tenantID + "|" + string(beneficiaryType) + "|" + beneficiaryID
}

func referralWalletAccountID(tenantID string, beneficiaryType ReferralBeneficiaryType, beneficiaryID string) string {
	return stableWorkflowID("referral_commission_wallet", referralWalletKey(tenantID, beneficiaryType, beneficiaryID))
}

func lockReferralWallets(ctx context.Context, tx *sql.Tx, plans []referralRewardGrantPlan, now time.Time) (map[string]referralWalletState, error) {
	keys := make([]string, 0, len(plans))
	byKey := make(map[string]ReferralReward)
	for _, plan := range plans {
		key := referralWalletKey(plan.Reward.TenantID, plan.Reward.BeneficiaryType, plan.Reward.BeneficiaryUserID)
		if _, exists := byKey[key]; !exists {
			keys = append(keys, key)
			byKey[key] = plan.Reward
		}
	}
	sort.Strings(keys)
	result := make(map[string]referralWalletState, len(keys))
	for _, key := range keys {
		reward := byKey[key]
		accountID := referralWalletAccountID(reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_commission_wallet_accounts(id,tenant_id,beneficiary_type,beneficiary_id,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$5)
			ON CONFLICT(tenant_id,beneficiary_type,beneficiary_id) DO NOTHING
		`, accountID, reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID, now)
		if err != nil {
			return nil, rewardGrantDBError("create referral wallet", err, ErrWalletCreditConflict)
		}
		var wallet referralWalletState
		err = tx.QueryRowContext(ctx, `
			SELECT id,tenant_id,beneficiary_type,beneficiary_id,status,
			       expected_cents,frozen_cents,available_cents,settling_cents,settled_cents,recoverable_cents
			FROM xz_commission_wallet_accounts
			WHERE tenant_id=$1 AND beneficiary_type=$2 AND beneficiary_id=$3 FOR UPDATE
		`, reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID).Scan(
			&wallet.ID, &wallet.TenantID, &wallet.BeneficiaryType, &wallet.BeneficiaryID, &wallet.Status,
			&wallet.Expected, &wallet.Frozen, &wallet.Available, &wallet.Settling, &wallet.Settled, &wallet.Recoverable)
		if err != nil {
			return nil, rewardGrantDBError("lock referral wallet", err, ErrWalletCreditConflict)
		}
		if wallet.TenantID != reward.TenantID || wallet.BeneficiaryType != string(reward.BeneficiaryType) || wallet.BeneficiaryID != reward.BeneficiaryUserID {
			return nil, ErrReferralWalletTenantInvalid
		}
		if wallet.Status != "ACTIVE" {
			return nil, ErrWalletCreditConflict
		}
		result[key] = wallet
	}
	return result, nil
}

func insertReferralReward(ctx context.Context, tx *sql.Tx, reward ReferralReward) error {
	metadata, _ := json.Marshal(JSONSnapshot{"source": "OPERATION_CENTER_REFERRAL_ELIGIBILITY"})
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_referral_rewards(
		  id,tenant_id,referral_event_id,referral_eligibility_id,reward_rule_id,reward_rule_version,
		  commercial_rule_set_id,beneficiary_type,beneficiary_user_id,beneficiary_relation,
		  amount_cents,record_type,status,freeze_until,relationship_snapshot,idempotency_key,metadata,
		  recoverable_cents,created_at,updated_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'REWARD','FROZEN',$12,$13,$14,$15,0,$16,$16)
	`, reward.ID, reward.TenantID, reward.ReferralEventID, reward.ReferralEligibilityID,
		reward.ReferralRuleID, reward.ReferralRuleVersion, reward.CommercialRuleSetID,
		reward.BeneficiaryType, reward.BeneficiaryUserID, reward.BeneficiaryRelation,
		reward.AmountCents, reward.FreezeUntil, reward.RelationshipSnapshot, reward.IdempotencyKey, metadata, reward.CreatedAt)
	if err != nil {
		return rewardGrantDBError("create referral reward", err, ErrRewardGrantConflict)
	}
	return nil
}

func creditFrozenReferralWallet(ctx context.Context, tx *sql.Tx, wallet referralWalletState, reward ReferralReward, now time.Time) (string, error) {
	before := wallet.referralWalletBalances
	after := before
	after.Frozen += reward.AmountCents
	result, err := tx.ExecContext(ctx, `
		UPDATE xz_commission_wallet_accounts
		SET frozen_cents=$2,version=version+1,updated_at=$3
		WHERE id=$1 AND tenant_id=$4 AND beneficiary_type=$5 AND beneficiary_id=$6
	`, wallet.ID, after.Frozen, now, reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID)
	if err != nil {
		return "", rewardGrantDBError("credit referral wallet", err, ErrWalletCreditConflict)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return "", ErrWalletCreditConflict
	}
	ledgerKey := "referral-wallet-frozen:" + reward.IdempotencyKey
	ledgerID := stableWorkflowID("referral_wallet_ledger", ledgerKey)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	metadata, _ := json.Marshal(JSONSnapshot{"rewardIdempotencyKey": reward.IdempotencyKey, "source": "OPERATION_CENTER_REFERRAL"})
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_commission_wallet_ledger(
		  id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,direction,
		  frozen_delta_cents,balances_before,balances_after,idempotency_key,metadata,
		  referral_reward_id,referral_event_id,referral_eligibility_id,commercial_rule_set_id,created_at
		) VALUES($1,$2,$3,$4,$5,'REFERRAL_REWARD_GRANT',$6,'CREDIT',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, ledgerID, reward.TenantID, wallet.ID, reward.BeneficiaryType, reward.BeneficiaryUserID,
		reward.ID, reward.AmountCents, beforeJSON, afterJSON, ledgerKey, metadata,
		reward.ID, reward.ReferralEventID, reward.ReferralEligibilityID, reward.CommercialRuleSetID, now)
	if err != nil {
		return "", rewardGrantDBError("create referral wallet credit", err, ErrWalletCreditConflict)
	}
	return ledgerID, nil
}

func appendReferralGrantAudits(ctx context.Context, store Store, reward ReferralReward, task ReferralRewardReleaseTask, groupID string, now time.Time) error {
	rewardTransition := OperationCenterStateTransition{
		ID: stableWorkflowID("referral_reward_transition", reward.ID), TenantID: reward.TenantID,
		EntityType: StateEntityReferralReward, EntityID: reward.ID, ToStatus: string(ReferralRewardFrozen),
		TransitionReason: "referral_eligibility_consumed", TransactionGroupID: groupID,
		IdempotencyKey: "referral-reward-granted:" + reward.ReferralEligibilityID,
		Metadata:       JSONSnapshot{"referralEventId": reward.ReferralEventID, "referralEligibilityId": reward.ReferralEligibilityID}, CreatedAt: now,
	}
	if err := store.AppendStateTransition(ctx, &rewardTransition); err != nil {
		return err
	}
	taskTransition := OperationCenterStateTransition{
		ID: stableWorkflowID("referral_release_transition", task.ID), TenantID: task.TenantID,
		EntityType: StateEntityRewardReleaseTask, EntityID: task.ID, ToStatus: string(ReferralRewardReleasePending),
		TransitionReason: "referral_reward_frozen", TransactionGroupID: groupID,
		IdempotencyKey: "referral-release-created:" + reward.ID,
		Metadata:       JSONSnapshot{"referralRewardId": reward.ID, "executeAt": task.ExecuteAt}, CreatedAt: now,
	}
	return store.AppendStateTransition(ctx, &taskTransition)
}

func loadGrantedReferralRewards(ctx context.Context, tx *sql.Tx, eventID string) ([]ReferralReward, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id,tenant_id,referral_event_id,referral_eligibility_id,reward_rule_id,reward_rule_version,
		       commercial_rule_set_id,beneficiary_type,beneficiary_user_id,beneficiary_relation,
		       amount_cents,status,freeze_until,relationship_snapshot,idempotency_key,
		       grant_wallet_ledger_id,current_release_task_id,created_at,updated_at
		FROM xz_referral_rewards
		WHERE referral_event_id=$1 AND record_type='REWARD' ORDER BY referral_eligibility_id,id
	`, eventID)
	if err != nil {
		return nil, mapPostgresStoreError("load granted referral rewards", err)
	}
	defer rows.Close()
	var result []ReferralReward
	for rows.Next() {
		var reward ReferralReward
		if err := rows.Scan(&reward.ID, &reward.TenantID, &reward.ReferralEventID, &reward.ReferralEligibilityID,
			&reward.ReferralRuleID, &reward.ReferralRuleVersion, &reward.CommercialRuleSetID,
			&reward.BeneficiaryType, &reward.BeneficiaryUserID, &reward.BeneficiaryRelation,
			&reward.AmountCents, &reward.Status, &reward.FreezeUntil, &reward.RelationshipSnapshot,
			&reward.IdempotencyKey, &reward.GrantWalletLedgerID, &reward.CurrentReleaseTaskID,
			&reward.CreatedAt, &reward.UpdatedAt); err != nil {
			return nil, mapPostgresStoreError("scan granted referral reward", err)
		}
		result = append(result, reward)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, ErrRewardGrantConflict
	}
	return result, nil
}

func rewardGrantDBError(operation string, err error, conflict error) error {
	mapped := mapPostgresStoreError(operation, err)
	if errors.Is(mapped, ErrUniqueConflict) || errors.Is(mapped, ErrIdempotencyConflict) {
		return fmt.Errorf("%w: %v", conflict, mapped)
	}
	return mapped
}
