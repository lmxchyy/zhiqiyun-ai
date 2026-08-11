package operationcenter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ReferralRewardReleaseService struct {
	db      *sql.DB
	store   *PostgresStore
	options ReferralRewardReleaseOptions
}

func NewReferralRewardReleaseService(db *sql.DB, options ReferralRewardReleaseOptions) (*ReferralRewardReleaseService, error) {
	store, err := NewPostgresStore(db)
	if err != nil {
		return nil, err
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 2 * time.Minute
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = time.Minute
	}
	return &ReferralRewardReleaseService{db: db, store: store, options: options}, nil
}

func (service *ReferralRewardReleaseService) ClaimDueRewards(ctx context.Context, owner string, limit int) ([]ReferralRewardReleaseTask, error) {
	if service == nil || service.db == nil || strings.TrimSpace(owner) == "" || limit <= 0 {
		return nil, ErrConstraintViolation
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	storeValue, err := service.store.BindTx(tx)
	if err != nil {
		return nil, err
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	tasks, err := storeValue.ClaimDueRewardReleaseTasks(ctx, now, owner, now.Add(service.options.LeaseDuration), limit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (service *ReferralRewardReleaseService) ClaimAndReleaseDueRewards(ctx context.Context, owner string, limit int) (ReferralRewardReleaseBatchResult, error) {
	var result ReferralRewardReleaseBatchResult
	tasks, err := service.ClaimDueRewards(ctx, owner, limit)
	if err != nil {
		return result, err
	}
	result.Claimed = len(tasks)
	for _, task := range tasks {
		released, releaseErr := service.ReleaseReferralReward(ctx, task.ID, owner)
		if releaseErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, releaseErr)
			continue
		}
		result.Succeeded++
		result.Results = append(result.Results, released)
	}
	return result, nil
}

func (service *ReferralRewardReleaseService) ReleaseReferralReward(ctx context.Context, taskID, owner string) (ReferralRewardReleaseResult, error) {
	result, err := service.releaseReferralRewardTx(ctx, taskID, owner)
	if err == nil {
		return result, nil
	}
	if recordErr := service.recordReleaseFailure(ctx, taskID, owner, err); recordErr != nil && !errors.Is(recordErr, ErrReleaseTaskNotExecutable) {
		return result, errors.Join(err, recordErr)
	}
	return result, err
}

func (service *ReferralRewardReleaseService) releaseReferralRewardTx(ctx context.Context, taskID, owner string) (result ReferralRewardReleaseResult, err error) {
	if service == nil || service.db == nil || strings.TrimSpace(taskID) == "" || strings.TrimSpace(owner) == "" {
		return result, ErrConstraintViolation
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	task, err := scanRewardReleaseTask(tx.QueryRowContext(ctx, `SELECT `+rewardReleaseColumns+` FROM xz_referral_reward_release_tasks WHERE id=$1 FOR UPDATE`, taskID))
	if err != nil {
		return result, err
	}
	reward, err := lockReferralRewardForRelease(ctx, tx, task.ReferralRewardID)
	if err != nil {
		return result, err
	}
	if task.Status == ReferralRewardReleaseSucceeded {
		result, err = validateReferralReleaseReplay(ctx, tx, *task, reward)
		if err != nil {
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	if err := validateReferralReleaseLease(*task, owner, now); err != nil {
		return result, err
	}
	if reward.Status == ReferralRewardAvailable {
		return result, ErrRewardReleaseInvariantViolation
	}
	wallet, err := lockReferralReleaseWallet(ctx, tx, reward)
	if err != nil {
		return result, err
	}
	grantLedger, err := lockReferralGrantLedger(ctx, tx, reward.GrantWalletLedgerID)
	if err != nil {
		return result, err
	}
	if err := validateReferralRewardRelease(*task, reward, wallet, grantLedger, now); err != nil {
		return result, err
	}
	ledgerKey := referralRewardReleaseLedgerKey(task.ID, reward.ID)
	var existingLedgerID string
	existingErr := tx.QueryRowContext(ctx, `SELECT id FROM xz_commission_wallet_ledger WHERE idempotency_key=$1`, ledgerKey).Scan(&existingLedgerID)
	if existingErr == nil {
		return result, ErrReleaseLedgerConflict
	}
	if !errors.Is(existingErr, sql.ErrNoRows) {
		return result, existingErr
	}
	after, err := applyReferralRewardReleaseBalances(wallet.referralWalletBalances, reward.AmountCents)
	if err != nil {
		return result, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE xz_commission_wallet_accounts
		SET frozen_cents=$2,available_cents=$3,version=version+1,updated_at=$4
		WHERE id=$1
	`, wallet.ID, after.Frozen, after.Available, now); err != nil {
		return result, mapPostgresStoreError("transfer referral reward wallet buckets", err)
	}
	ledgerID := stableWorkflowID("referral_release_wallet_ledger", ledgerKey)
	beforeJSON, _ := json.Marshal(wallet.referralWalletBalances)
	afterJSON, _ := json.Marshal(after)
	metadata, _ := json.Marshal(JSONSnapshot{"releaseTaskId": task.ID, "source": "OPERATION_CENTER_REFERRAL_RELEASE"})
	_, err = tx.ExecContext(ctx, `
		INSERT INTO xz_commission_wallet_ledger(
		  id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,direction,
		  frozen_delta_cents,available_delta_cents,balances_before,balances_after,idempotency_key,metadata,
		  referral_reward_id,referral_event_id,referral_eligibility_id,original_ledger_id,
		  commercial_rule_set_id,referral_release_task_id,created_at
		) VALUES($1,$2,$3,$4,$5,'REFERRAL_REWARD_RELEASE',$6,'TRANSFER',$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`, ledgerID, reward.TenantID, wallet.ID, reward.BeneficiaryType, reward.BeneficiaryUserID,
		reward.ID, -reward.AmountCents, reward.AmountCents, beforeJSON, afterJSON, ledgerKey, metadata,
		reward.ID, reward.ReferralEventID, reward.ReferralEligibilityID, grantLedger.ID,
		reward.CommercialRuleSetID, task.ID, now)
	if err != nil {
		return result, rewardGrantDBError("create referral reward release ledger", err, ErrReleaseLedgerConflict)
	}
	updated, err := tx.ExecContext(ctx, `
		UPDATE xz_referral_rewards
		SET status='AVAILABLE',release_wallet_ledger_id=$2,updated_at=$3
		WHERE id=$1 AND status='FROZEN'
	`, reward.ID, ledgerID, now)
	if err != nil {
		return result, mapPostgresStoreError("make referral reward available", err)
	}
	if rows, rowsErr := updated.RowsAffected(); rowsErr != nil || rows != 1 {
		return result, ErrRewardReleaseInvariantViolation
	}
	storeValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	groupID := stableWorkflowID("referral_reward_release_tx", task.ID)
	rewardTransition := OperationCenterStateTransition{
		ID: stableWorkflowID("referral_reward_available_transition", reward.ID), TenantID: reward.TenantID,
		EntityType: StateEntityReferralReward, EntityID: reward.ID,
		FromStatus: optionalString(string(ReferralRewardFrozen)), ToStatus: string(ReferralRewardAvailable),
		TransitionReason: "referral_reward_released", TransactionGroupID: groupID,
		IdempotencyKey: "referral-reward-available:" + reward.ID,
		Metadata:       JSONSnapshot{"releaseTaskId": task.ID, "walletLedgerId": ledgerID}, CreatedAt: now,
	}
	if err := storeValue.AppendStateTransition(ctx, &rewardTransition); err != nil {
		return result, err
	}
	previousStatus := task.Status
	task.Status = ReferralRewardReleaseSucceeded
	task.CompletedAt = timePointer(now)
	task.LeaseOwner = nil
	task.LeaseExpiresAt = nil
	task.NextRetryAt = nil
	task.FailureClass = nil
	task.FailureDetail = JSONSnapshot{}
	task.UpdatedAt = now
	taskTransition := OperationCenterStateTransition{
		ID: stableWorkflowID("referral_release_succeeded_transition", task.ID), TenantID: task.TenantID,
		EntityType: StateEntityRewardReleaseTask, EntityID: task.ID,
		FromStatus: optionalString(string(previousStatus)), ToStatus: string(task.Status),
		TransitionReason: "referral_reward_released", TransactionGroupID: groupID,
		IdempotencyKey: "referral-release-succeeded:" + task.ID,
		Metadata:       JSONSnapshot{"referralRewardId": reward.ID, "walletLedgerId": ledgerID}, CreatedAt: now,
	}
	if err := storeValue.UpdateRewardReleaseTask(ctx, task, &taskTransition); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return ReferralRewardReleaseResult{TaskID: task.ID, RewardID: reward.ID, WalletLedgerID: ledgerID, AmountCents: reward.AmountCents}, nil
}

func (service *ReferralRewardReleaseService) recordReleaseFailure(ctx context.Context, taskID, owner string, releaseErr error) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	storeValue, err := service.store.BindTx(tx)
	if err != nil {
		return err
	}
	task, err := scanRewardReleaseTask(tx.QueryRowContext(ctx, `SELECT `+rewardReleaseColumns+` FROM xz_referral_reward_release_tasks WHERE id=$1 FOR UPDATE`, taskID))
	if err != nil {
		return err
	}
	if task.Status == ReferralRewardReleaseSucceeded {
		return nil
	}
	if task.Status != ReferralRewardReleaseProcessing || task.LeaseOwner == nil || *task.LeaseOwner != owner {
		return ErrReleaseTaskNotExecutable
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return err
	}
	failureClass, retryable := classifyReferralRewardReleaseFailure(releaseErr)
	previousStatus := task.Status
	task.Status = ReferralRewardReleaseFailed
	task.AttemptCount++
	task.LeaseOwner = nil
	task.LeaseExpiresAt = nil
	task.FailureClass = &failureClass
	task.FailureDetail = JSONSnapshot{"error": releaseErr.Error(), "retryable": retryable}
	task.UpdatedAt = now
	if retryable {
		next := now.Add(service.options.RetryDelay)
		task.NextRetryAt = &next
	} else {
		task.NextRetryAt = nil
	}
	transition := OperationCenterStateTransition{
		ID: stableWorkflowID("referral_release_failed_transition", task.ID, fmt.Sprint(task.AttemptCount)), TenantID: task.TenantID,
		EntityType: StateEntityRewardReleaseTask, EntityID: task.ID,
		FromStatus: optionalString(string(previousStatus)), ToStatus: string(task.Status),
		TransitionReason: "referral_reward_release_failed", TransactionGroupID: stableWorkflowID("referral_release_failure_tx", task.ID, fmt.Sprint(task.AttemptCount)),
		IdempotencyKey: fmt.Sprintf("referral-release-failed:%s:%d", task.ID, task.AttemptCount),
		Metadata:       JSONSnapshot{"failureClass": string(failureClass), "retryable": retryable}, CreatedAt: now,
	}
	if err := storeValue.UpdateRewardReleaseTask(ctx, task, &transition); err != nil {
		return err
	}
	return tx.Commit()
}

func lockReferralRewardForRelease(ctx context.Context, tx *sql.Tx, rewardID string) (ReferralReward, error) {
	var reward ReferralReward
	var eligibilityID, grantWalletLedgerID, currentReleaseTaskID sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT id,tenant_id,referral_event_id,referral_eligibility_id,reward_rule_id,reward_rule_version,
		       commercial_rule_set_id,beneficiary_type,beneficiary_user_id,beneficiary_relation,
		       amount_cents,status,freeze_until,relationship_snapshot,idempotency_key,
		       grant_wallet_ledger_id,current_release_task_id,created_at,updated_at
		FROM xz_referral_rewards WHERE id=$1 AND record_type='REWARD' FOR UPDATE
	`, rewardID).Scan(&reward.ID, &reward.TenantID, &reward.ReferralEventID, &eligibilityID,
		&reward.ReferralRuleID, &reward.ReferralRuleVersion, &reward.CommercialRuleSetID,
		&reward.BeneficiaryType, &reward.BeneficiaryUserID, &reward.BeneficiaryRelation,
		&reward.AmountCents, &reward.Status, &reward.FreezeUntil, &reward.RelationshipSnapshot,
		&reward.IdempotencyKey, &grantWalletLedgerID, &currentReleaseTaskID,
		&reward.CreatedAt, &reward.UpdatedAt)
	reward.ReferralEligibilityID = eligibilityID.String
	reward.GrantWalletLedgerID = grantWalletLedgerID.String
	reward.CurrentReleaseTaskID = currentReleaseTaskID.String
	return reward, mapPostgresStoreError("lock referral reward for release", err)
}

func lockReferralReleaseWallet(ctx context.Context, tx *sql.Tx, reward ReferralReward) (referralWalletState, error) {
	var wallet referralWalletState
	err := tx.QueryRowContext(ctx, `
		SELECT id,tenant_id,beneficiary_type,beneficiary_id,status,
		       expected_cents,frozen_cents,available_cents,settling_cents,settled_cents,recoverable_cents
		FROM xz_commission_wallet_accounts
		WHERE tenant_id=$1 AND beneficiary_type=$2 AND beneficiary_id=$3 FOR UPDATE
	`, reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID).Scan(
		&wallet.ID, &wallet.TenantID, &wallet.BeneficiaryType, &wallet.BeneficiaryID, &wallet.Status,
		&wallet.Expected, &wallet.Frozen, &wallet.Available, &wallet.Settling, &wallet.Settled, &wallet.Recoverable)
	return wallet, mapPostgresStoreError("lock referral reward wallet for release", err)
}

func lockReferralGrantLedger(ctx context.Context, tx *sql.Tx, ledgerID string) (referralGrantLedgerSnapshot, error) {
	var ledger referralGrantLedgerSnapshot
	err := tx.QueryRowContext(ctx, `
		SELECT id,tenant_id,account_id,coalesce(referral_reward_id,''),business_type,frozen_delta_cents
		FROM xz_commission_wallet_ledger WHERE id=$1 FOR UPDATE
	`, ledgerID).Scan(&ledger.ID, &ledger.TenantID, &ledger.AccountID, &ledger.ReferralRewardID, &ledger.BusinessType, &ledger.FrozenDelta)
	return ledger, mapPostgresStoreError("lock referral reward grant ledger", err)
}

func validateReferralReleaseReplay(ctx context.Context, tx *sql.Tx, task ReferralRewardReleaseTask, reward ReferralReward) (ReferralRewardReleaseResult, error) {
	if reward.Status != ReferralRewardAvailable || reward.CurrentReleaseTaskID != task.ID || reward.GrantWalletLedgerID == "" {
		return ReferralRewardReleaseResult{}, ErrRewardReleaseInvariantViolation
	}
	var ledgerID, rewardID, taskID string
	var frozenDelta, availableDelta int64
	err := tx.QueryRowContext(ctx, `
		SELECT id,coalesce(referral_reward_id,''),coalesce(referral_release_task_id,''),frozen_delta_cents,available_delta_cents
		FROM xz_commission_wallet_ledger
		WHERE id=(SELECT release_wallet_ledger_id FROM xz_referral_rewards WHERE id=$1)
	`, reward.ID).Scan(&ledgerID, &rewardID, &taskID, &frozenDelta, &availableDelta)
	if err != nil || rewardID != reward.ID || taskID != task.ID || frozenDelta != -reward.AmountCents || availableDelta != reward.AmountCents {
		return ReferralRewardReleaseResult{}, ErrRewardReleaseInvariantViolation
	}
	return ReferralRewardReleaseResult{TaskID: task.ID, RewardID: reward.ID, WalletLedgerID: ledgerID, AmountCents: reward.AmountCents, IdempotentReplay: true}, nil
}
