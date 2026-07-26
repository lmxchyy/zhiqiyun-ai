package operationcenter

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
)

type ReferralRewardReversalService struct {
	store *PostgresStore
}

func NewReferralRewardReversalService(store *PostgresStore) *ReferralRewardReversalService {
	return &ReferralRewardReversalService{store: store}
}

type reversalLockedReward struct {
	reward          ReferralReward
	releaseTask     *ReferralRewardReleaseTask
	grantLedger     referralGrantLedgerSnapshot
	releaseLedgerID string
}

func (service *ReferralRewardReversalService) ReverseReferralRewardsForRefund(ctx context.Context, tx *sql.Tx, command ReferralRewardReversalCommand) (ReferralRewardReversalResult, error) {
	var result ReferralRewardReversalResult
	if tx == nil {
		return result, ErrTransactionRequired
	}
	if service == nil || service.store == nil || strings.TrimSpace(command.RefundTaskID) == "" ||
		strings.TrimSpace(command.OperationCenterServiceOrderID) == "" || strings.TrimSpace(command.ReferralEventID) == "" ||
		command.RefundAmountCents <= 0 || strings.TrimSpace(command.ReversalReason) == "" ||
		strings.TrimSpace(command.OperatorID) == "" || strings.TrimSpace(command.TransactionGroupID) == "" {
		return result, ErrConstraintViolation
	}
	var refundTenantID, serviceOrderID, refundStatus, sourceOrderID string
	var refundAmount int64
	err := tx.QueryRowContext(ctx, `
		SELECT refund.tenant_id,refund.service_order_id,refund.amount_cents,refund.refund_status,service_order.order_id
		FROM xz_operation_center_refund_tasks refund
		JOIN xz_operation_center_service_orders service_order ON service_order.id=refund.service_order_id
		WHERE refund.id=$1 FOR UPDATE OF refund
	`, command.RefundTaskID).Scan(&refundTenantID, &serviceOrderID, &refundAmount, &refundStatus, &sourceOrderID)
	if err != nil {
		return result, mapPostgresStoreError("lock referral reversal refund task", err)
	}
	if serviceOrderID != command.OperationCenterServiceOrderID || refundAmount != command.RefundAmountCents {
		return result, ErrRewardReversalInvariantViolation
	}
	var eventTenantID, eventOrderID, eventStatus string
	err = tx.QueryRowContext(ctx, `SELECT tenant_id,source_order_id,status FROM xz_referral_events WHERE id=$1 FOR UPDATE`, command.ReferralEventID).Scan(&eventTenantID, &eventOrderID, &eventStatus)
	if err != nil {
		return result, mapPostgresStoreError("lock referral reversal event", err)
	}
	if eventTenantID != refundTenantID || eventOrderID != sourceOrderID {
		return result, ErrRewardReversalInvariantViolation
	}
	rewards, err := lockOriginalReferralRewardsForReversal(ctx, tx, command.ReferralEventID)
	if err != nil {
		return result, err
	}
	if len(rewards) == 0 {
		return result, ErrRewardReversalInvariantViolation
	}
	if eventStatus == "REVERSED" {
		return loadReferralReversalReplay(ctx, tx, command, rewards)
	}
	if eventStatus != "REWARDED" {
		return result, ErrRewardStateNotReversible
	}
	for _, reward := range rewards {
		if reward.Status == ReferralRewardStatus("REVERSED") {
			if reward.RefundTaskID == command.RefundTaskID {
				return result, ErrRewardAlreadyReversed
			}
			return result, ErrRewardReversalConflict
		}
	}
	locked := make([]reversalLockedReward, len(rewards))
	for index, reward := range rewards {
		locked[index].reward = reward
		if reward.CurrentReleaseTaskID != "" {
			task, taskErr := scanRewardReleaseTask(tx.QueryRowContext(ctx, `SELECT `+rewardReleaseColumns+` FROM xz_referral_reward_release_tasks WHERE id=$1 FOR UPDATE`, reward.CurrentReleaseTaskID))
			if taskErr != nil {
				return result, taskErr
			}
			locked[index].releaseTask = task
		}
	}
	wallets, err := lockReversalWallets(ctx, tx, rewards)
	if err != nil {
		return result, err
	}
	for index := range locked {
		grant, grantErr := lockReferralGrantLedger(ctx, tx, locked[index].reward.GrantWalletLedgerID)
		if grantErr != nil {
			return result, grantErr
		}
		locked[index].grantLedger = grant
		if locked[index].reward.ReleaseWalletLedgerID != "" {
			var releaseRewardID string
			if err := tx.QueryRowContext(ctx, `SELECT coalesce(referral_reward_id,'') FROM xz_commission_wallet_ledger WHERE id=$1 FOR UPDATE`, locked[index].reward.ReleaseWalletLedgerID).Scan(&releaseRewardID); err != nil || releaseRewardID != locked[index].reward.ID {
				return result, ErrRewardReversalInvariantViolation
			}
			locked[index].releaseLedgerID = locked[index].reward.ReleaseWalletLedgerID
		}
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	storeValue, err := service.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	result.RefundTaskID = command.RefundTaskID
	result.ReferralEventID = command.ReferralEventID
	for _, item := range locked {
		reward := item.reward
		walletKey := referralWalletKey(reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID)
		wallet := wallets[walletKey]
		if reward.Status == ReferralRewardFrozen {
			if item.releaseTask == nil || item.releaseTask.Status == ReferralRewardReleaseSucceeded || item.releaseTask.Status == ReferralRewardReleaseCancelled {
				return result, ErrReleaseTaskStillExecutable
			}
		} else if reward.Status == ReferralRewardAvailable || reward.Status == ReferralRewardStatus("SETTLED") {
			if item.releaseTask == nil || item.releaseTask.Status != ReferralRewardReleaseSucceeded || item.releaseLedgerID == "" {
				return result, ErrRewardReversalInvariantViolation
			}
		}
		plan, planErr := buildReferralRewardReversalPlan(reward, wallet)
		if planErr != nil {
			return result, planErr
		}
		after, balanceErr := applyReferralRewardReversalBalances(wallet.referralWalletBalances, plan)
		if balanceErr != nil {
			return result, balanceErr
		}
		reversalRewardID := stableWorkflowID("referral_reward_reversal", command.RefundTaskID, reward.ID)
		reversalKey := referralRewardReversalKey(command.RefundTaskID, reward.ID)
		metadata, _ := json.Marshal(JSONSnapshot{"reason": command.ReversalReason, "operatorId": command.OperatorID, "sourceRewardStatus": string(reward.Status), "reversalType": string(plan.ReversalType), "transactionGroupId": command.TransactionGroupID})
		_, err := tx.ExecContext(ctx, `
			INSERT INTO xz_referral_rewards(
			  id,tenant_id,referral_event_id,referral_eligibility_id,reward_rule_id,reward_rule_version,
			  commercial_rule_set_id,beneficiary_type,beneficiary_user_id,beneficiary_relation,
			  amount_cents,record_type,status,freeze_until,reversal_of_id,idempotency_key,metadata,
			  refund_task_id,relationship_snapshot,recoverable_cents,reversal_amount_cents,
			  source_reward_status,reversal_type,transaction_group_id,created_at,updated_at
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'REVERSAL','REVERSED',$12,$13,$14,$15,$16,$17,0,$18,$19,$20,$21,$22,$22)
		`, reversalRewardID, reward.TenantID, reward.ReferralEventID, reward.ReferralEligibilityID,
			reward.ReferralRuleID, reward.ReferralRuleVersion, reward.CommercialRuleSetID,
			reward.BeneficiaryType, reward.BeneficiaryUserID, reward.BeneficiaryRelation,
			-reward.AmountCents, reward.FreezeUntil, reward.ID, reversalKey, metadata,
			command.RefundTaskID, reward.RelationshipSnapshot, reward.AmountCents,
			reward.Status, plan.ReversalType, command.TransactionGroupID, now)
		if err != nil {
			return result, rewardGrantDBError("create referral reward reversal", err, ErrRewardReversalConflict)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE xz_commission_wallet_accounts
			SET frozen_cents=$2,available_cents=$3,recoverable_cents=$4,version=version+1,updated_at=$5
			WHERE id=$1
		`, wallet.ID, after.Frozen, after.Available, after.Recoverable, now); err != nil {
			return result, mapPostgresStoreError("apply referral reward reversal wallet", err)
		}
		ledgerKey := referralRewardReversalLedgerKey(command.RefundTaskID, reward.ID)
		ledgerID := stableWorkflowID("referral_reversal_wallet_ledger", ledgerKey)
		beforeJSON, _ := json.Marshal(wallet.referralWalletBalances)
		afterJSON, _ := json.Marshal(after)
		ledgerMetadata, _ := json.Marshal(JSONSnapshot{"reason": command.ReversalReason, "operatorId": command.OperatorID, "sourceRewardStatus": string(reward.Status), "reversalType": string(plan.ReversalType)})
		var releaseTaskID any
		if item.releaseTask != nil {
			releaseTaskID = item.releaseTask.ID
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO xz_commission_wallet_ledger(
			  id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,direction,
			  frozen_delta_cents,available_delta_cents,recoverable_delta_cents,recoverable_cents_delta,
			  balances_before,balances_after,idempotency_key,metadata,referral_reward_id,
			  original_referral_reward_id,referral_event_id,referral_eligibility_id,original_ledger_id,
			  original_grant_ledger_id,original_release_ledger_id,refund_task_id,commercial_rule_set_id,
			  referral_release_task_id,transaction_group_id,created_at
			) VALUES($1,$2,$3,$4,$5,'REFERRAL_REWARD_REVERSAL',$6,'DEBIT',$7,$8,$9,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		`, ledgerID, reward.TenantID, wallet.ID, reward.BeneficiaryType, reward.BeneficiaryUserID,
			reversalRewardID, -plan.FrozenDebitCents, -plan.AvailableDebitCents, plan.RecoverableCents,
			beforeJSON, afterJSON, ledgerKey, ledgerMetadata, reversalRewardID, reward.ID,
			reward.ReferralEventID, reward.ReferralEligibilityID, item.grantLedger.ID,
			item.grantLedger.ID, nullableSQLString(item.releaseLedgerID), command.RefundTaskID,
			reward.CommercialRuleSetID, releaseTaskID, command.TransactionGroupID, now)
		if err != nil {
			return result, rewardGrantDBError("create referral reversal wallet ledger", err, ErrReversalLedgerConflict)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE xz_referral_rewards SET status='REVERSED',refund_task_id=$2,reversal_wallet_ledger_id=$3,updated_at=$4 WHERE id=$1 AND status=$5`, reward.ID, command.RefundTaskID, ledgerID, now, reward.Status); err != nil {
			return result, mapPostgresStoreError("reverse original referral reward", err)
		}
		if reward.Status == ReferralRewardFrozen {
			task := item.releaseTask
			previous := task.Status
			task.Status = ReferralRewardReleaseCancelled
			task.LeaseOwner = nil
			task.LeaseExpiresAt = nil
			task.NextRetryAt = nil
			task.FailureClass = nil
			task.FailureDetail = JSONSnapshot{"cancellationReason": command.ReversalReason, "refundTaskId": command.RefundTaskID}
			task.CancellationReason = optionalString("REFERRAL_REWARD_REVERSED")
			task.CancelledAt = timePointer(now)
			task.CompletedAt = timePointer(now)
			task.UpdatedAt = now
			transition := OperationCenterStateTransition{
				ID: stableWorkflowID("referral_release_cancelled_transition", task.ID, command.RefundTaskID), TenantID: task.TenantID,
				EntityType: StateEntityRewardReleaseTask, EntityID: task.ID,
				FromStatus: optionalString(string(previous)), ToStatus: string(task.Status),
				TransitionReason: "referral_reward_reversed", TransactionGroupID: command.TransactionGroupID,
				OperatorID: optionalString(command.OperatorID), IdempotencyKey: "referral-release-cancelled:" + command.RefundTaskID + ":" + task.ID,
				Metadata: JSONSnapshot{"refundTaskId": command.RefundTaskID, "originalRewardId": reward.ID}, CreatedAt: now,
			}
			if err := storeValue.UpdateRewardReleaseTask(ctx, task, &transition); err != nil {
				return result, err
			}
		}
		rewardTransition := OperationCenterStateTransition{
			ID: stableWorkflowID("referral_reward_reversed_transition", reward.ID, command.RefundTaskID), TenantID: reward.TenantID,
			EntityType: StateEntityReferralReward, EntityID: reward.ID,
			FromStatus: optionalString(string(reward.Status)), ToStatus: "REVERSED",
			TransitionReason: "operation_center_refund_reversal", TransactionGroupID: command.TransactionGroupID,
			OperatorID: optionalString(command.OperatorID), IdempotencyKey: "referral-reward-reversed:" + command.RefundTaskID + ":" + reward.ID,
			Metadata: JSONSnapshot{"refundTaskId": command.RefundTaskID, "reversalRewardId": reversalRewardID, "walletLedgerId": ledgerID}, CreatedAt: now,
		}
		if err := storeValue.AppendStateTransition(ctx, &rewardTransition); err != nil {
			return result, err
		}
		wallet.referralWalletBalances = after
		wallets[walletKey] = wallet
		itemResult := ReferralRewardReversalItem{
			OriginalRewardID: reward.ID, ReversalRewardID: reversalRewardID, WalletLedgerID: ledgerID,
			SourceStatus: reward.Status, AmountCents: reward.AmountCents,
			DirectDebitCents: plan.FrozenDebitCents + plan.AvailableDebitCents, RecoverableCents: plan.RecoverableCents,
		}
		result.Items = append(result.Items, itemResult)
		result.OriginalAmountCents += reward.AmountCents
		result.DirectDebitCents += itemResult.DirectDebitCents
		result.RecoverableCents += itemResult.RecoverableCents
	}
	if result.OriginalAmountCents != result.DirectDebitCents+result.RecoverableCents {
		return result, ErrRewardReversalInvariantViolation
	}
	updated, err := tx.ExecContext(ctx, `UPDATE xz_referral_events SET status='REVERSED',updated_at=$2 WHERE id=$1 AND status='REWARDED'`, command.ReferralEventID, now)
	if err != nil {
		return result, err
	}
	if rows, rowsErr := updated.RowsAffected(); rowsErr != nil || rows != 1 {
		return result, ErrRewardReversalInvariantViolation
	}
	return result, nil
}

func lockOriginalReferralRewardsForReversal(ctx context.Context, tx *sql.Tx, eventID string) ([]ReferralReward, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id,tenant_id,referral_event_id,referral_eligibility_id,reward_rule_id,reward_rule_version,
		       commercial_rule_set_id,beneficiary_type,beneficiary_user_id,beneficiary_relation,
		       amount_cents,status,freeze_until,relationship_snapshot,idempotency_key,
		       grant_wallet_ledger_id,release_wallet_ledger_id,current_release_task_id,
		       refund_task_id,reversal_wallet_ledger_id,created_at,updated_at
		FROM xz_referral_rewards WHERE referral_event_id=$1 AND record_type='REWARD' ORDER BY id FOR UPDATE
	`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReferralReward
	for rows.Next() {
		var reward ReferralReward
		var eligibilityID, grantLedgerID, releaseLedgerID, releaseTaskID, refundTaskID, reversalLedgerID sql.NullString
		if err := rows.Scan(&reward.ID, &reward.TenantID, &reward.ReferralEventID, &eligibilityID,
			&reward.ReferralRuleID, &reward.ReferralRuleVersion, &reward.CommercialRuleSetID,
			&reward.BeneficiaryType, &reward.BeneficiaryUserID, &reward.BeneficiaryRelation,
			&reward.AmountCents, &reward.Status, &reward.FreezeUntil, &reward.RelationshipSnapshot,
			&reward.IdempotencyKey, &grantLedgerID, &releaseLedgerID, &releaseTaskID,
			&refundTaskID, &reversalLedgerID, &reward.CreatedAt, &reward.UpdatedAt); err != nil {
			return nil, err
		}
		reward.ReferralEligibilityID = eligibilityID.String
		reward.GrantWalletLedgerID = grantLedgerID.String
		reward.ReleaseWalletLedgerID = releaseLedgerID.String
		reward.CurrentReleaseTaskID = releaseTaskID.String
		reward.RefundTaskID = refundTaskID.String
		reward.ReversalWalletLedgerID = reversalLedgerID.String
		result = append(result, reward)
	}
	return result, rows.Err()
}

func lockReversalWallets(ctx context.Context, tx *sql.Tx, rewards []ReferralReward) (map[string]referralWalletState, error) {
	keys := make([]string, 0, len(rewards))
	byKey := make(map[string]ReferralReward)
	for _, reward := range rewards {
		key := referralWalletKey(reward.TenantID, reward.BeneficiaryType, reward.BeneficiaryUserID)
		if _, exists := byKey[key]; !exists {
			keys = append(keys, key)
			byKey[key] = reward
		}
	}
	sort.Strings(keys)
	result := make(map[string]referralWalletState, len(keys))
	for _, key := range keys {
		wallet, err := lockReferralReleaseWallet(ctx, tx, byKey[key])
		if err != nil {
			return nil, err
		}
		result[key] = wallet
	}
	return result, nil
}

func loadReferralReversalReplay(ctx context.Context, tx *sql.Tx, command ReferralRewardReversalCommand, rewards []ReferralReward) (ReferralRewardReversalResult, error) {
	result := ReferralRewardReversalResult{RefundTaskID: command.RefundTaskID, ReferralEventID: command.ReferralEventID, IdempotentReplay: true}
	for _, reward := range rewards {
		if reward.Status != ReferralRewardStatus("REVERSED") {
			return result, ErrRewardReversalInvariantViolation
		}
		if reward.RefundTaskID != command.RefundTaskID {
			return result, ErrRewardReversalConflict
		}
		var reversalID, ledgerID, sourceStatus string
		var reversalAmount, frozenDelta, availableDelta, recoverableDelta int64
		err := tx.QueryRowContext(ctx, `
			SELECT reversal.id,ledger.id,reversal.source_reward_status,reversal.reversal_amount_cents,
			       ledger.frozen_delta_cents,ledger.available_delta_cents,ledger.recoverable_cents_delta
			FROM xz_referral_rewards reversal
			JOIN xz_commission_wallet_ledger ledger ON ledger.referral_reward_id=reversal.id
			WHERE reversal.reversal_of_id=$1 AND reversal.refund_task_id=$2 AND reversal.record_type='REVERSAL'
		`, reward.ID, command.RefundTaskID).Scan(&reversalID, &ledgerID, &sourceStatus, &reversalAmount, &frozenDelta, &availableDelta, &recoverableDelta)
		if err != nil || reversalAmount != reward.AmountCents || -frozenDelta-availableDelta+recoverableDelta != reward.AmountCents {
			return result, ErrRewardReversalInvariantViolation
		}
		item := ReferralRewardReversalItem{OriginalRewardID: reward.ID, ReversalRewardID: reversalID, WalletLedgerID: ledgerID, SourceStatus: ReferralRewardStatus(sourceStatus), AmountCents: reward.AmountCents, DirectDebitCents: -frozenDelta - availableDelta, RecoverableCents: recoverableDelta}
		result.Items = append(result.Items, item)
		result.OriginalAmountCents += item.AmountCents
		result.DirectDebitCents += item.DirectDebitCents
		result.RecoverableCents += item.RecoverableCents
	}
	if result.OriginalAmountCents != result.DirectDebitCents+result.RecoverableCents {
		return result, ErrRewardReversalInvariantViolation
	}
	return result, nil
}

func nullableSQLString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
