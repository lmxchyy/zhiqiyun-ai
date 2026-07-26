package httpserver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type v132CommissionWalletBalances struct {
	Expected    int64 `json:"expectedCents"`
	Frozen      int64 `json:"frozenCents"`
	Available   int64 `json:"availableCents"`
	Settling    int64 `json:"settlingCents"`
	Settled     int64 `json:"settledCents"`
	Recoverable int64 `json:"recoverableCents"`
}

func postV132CommissionRecordToWalletTx(ctx context.Context, tx *sql.Tx, record commissionapp.CommissionRecord) error {
	if record.RecordType != commissionapp.RecordEarning || record.AmountCents <= 0 {
		return errors.New("V1.3.2 wallet credit requires a positive earning record")
	}
	accountID, before, err := lockV132CommissionWalletAccountTx(ctx, tx, record.TenantID, string(record.BeneficiaryType), record.BeneficiaryID, record.CreatedAt)
	if err != nil {
		return err
	}
	idempotencyKey := "v132-wallet-credit:" + record.IdempotencyKey
	exists, err := v132CommissionWalletLedgerExistsTx(ctx, tx, idempotencyKey)
	if err != nil || exists {
		return err
	}
	after := before
	amount := int64(record.AmountCents)
	expectedDelta, frozenDelta, availableDelta := int64(0), int64(0), int64(0)
	switch record.Status {
	case commissionapp.CommissionExpected:
		expectedDelta, after.Expected = amount, after.Expected+amount
	case commissionapp.CommissionFrozen:
		frozenDelta, after.Frozen = amount, after.Frozen+amount
	case commissionapp.CommissionAvailable:
		availableDelta, after.Available = amount, after.Available+amount
	default:
		return fmt.Errorf("unsupported V1.3.2 earning wallet status %q", record.Status)
	}
	if err := updateV132CommissionWalletAccountTx(ctx, tx, accountID, after, record.UpdatedAt); err != nil {
		return err
	}
	return insertV132CommissionWalletLedgerTx(ctx, tx, v132CommissionWalletLedgerEntry{
		ID: v132WalletStableID("commission_wallet_ledger_", idempotencyKey), TenantID: record.TenantID, AccountID: accountID,
		BeneficiaryType: string(record.BeneficiaryType), BeneficiaryID: record.BeneficiaryID,
		BusinessType: "COMMERCE_ORDER", BusinessID: record.OrderID, Direction: "CREDIT",
		ExpectedDelta: expectedDelta, FrozenDelta: frozenDelta, AvailableDelta: availableDelta,
		Before: before, After: after, CommissionRecordID: record.ID, IdempotencyKey: idempotencyKey,
		Metadata:  map[string]any{"settlementEngine": "V132", "orderNo": record.OrderNo, "ruleId": record.RuleID, "ruleVersion": record.RuleVersion},
		CreatedAt: record.CreatedAt,
	})
}

func reverseV132CommissionWalletTx(ctx context.Context, tx *sql.Tx, originalID, reversalID, tenantID, beneficiaryType, beneficiaryID, orderID, orderNo, ruleID string, ruleVersion int, originalStatus string, amount int64, now time.Time, commercialSnapshot map[string]any) error {
	accountID, before, err := lockV132CommissionWalletAccountTx(ctx, tx, tenantID, beneficiaryType, beneficiaryID, now)
	if err != nil {
		return err
	}
	idempotencyKey := "v132-wallet-refund:" + originalID
	exists, err := v132CommissionWalletLedgerExistsTx(ctx, tx, idempotencyKey)
	if err != nil || exists {
		return err
	}
	after := before
	expectedDelta, frozenDelta, availableDelta := int64(0), int64(0), int64(0)
	settlingDelta, settledDelta, recoverableDelta := int64(0), int64(0), int64(0)
	switch commissionapp.CommissionStatus(strings.ToUpper(strings.TrimSpace(originalStatus))) {
	case commissionapp.CommissionExpected:
		if before.Expected < amount {
			return errors.New("insufficient expected commission for V1.3.2 refund")
		}
		expectedDelta, after.Expected = -amount, before.Expected-amount
	case commissionapp.CommissionFrozen:
		if before.Frozen < amount {
			return errors.New("insufficient frozen commission for V1.3.2 refund")
		}
		frozenDelta, after.Frozen = -amount, before.Frozen-amount
	case commissionapp.CommissionAvailable:
		if before.Available < amount {
			return errors.New("insufficient available commission for V1.3.2 refund")
		}
		availableDelta, after.Available = -amount, before.Available-amount
	case commissionapp.CommissionSettling:
		if before.Settling < amount {
			return errors.New("insufficient settling commission for V1.3.2 refund")
		}
		settlingDelta, after.Settling = -amount, before.Settling-amount
	case commissionapp.CommissionSettled:
		recoverableDelta, after.Recoverable = amount, before.Recoverable+amount
	default:
		return fmt.Errorf("unsupported V1.3.2 refund wallet status %q", originalStatus)
	}
	if err := updateV132CommissionWalletAccountTx(ctx, tx, accountID, after, now); err != nil {
		return err
	}
	return insertV132CommissionWalletLedgerTx(ctx, tx, v132CommissionWalletLedgerEntry{
		ID: v132WalletStableID("commission_wallet_ledger_", idempotencyKey), TenantID: tenantID, AccountID: accountID,
		BeneficiaryType: beneficiaryType, BeneficiaryID: beneficiaryID,
		BusinessType: "COMMERCE_ORDER_REFUND", BusinessID: orderID, Direction: "DEBIT",
		ExpectedDelta: expectedDelta, FrozenDelta: frozenDelta, AvailableDelta: availableDelta,
		SettlingDelta: settlingDelta, SettledDelta: settledDelta, RecoverableDelta: recoverableDelta,
		Before: before, After: after, CommissionRecordID: reversalID, IdempotencyKey: idempotencyKey,
		Metadata:  map[string]any{"settlementEngine": "V132", "orderNo": orderNo, "originalCommissionRecordId": originalID, "ruleId": ruleID, "ruleVersion": ruleVersion, "commercialSnapshot": commercialSnapshot},
		CreatedAt: now,
	})
}

func lockV132CommissionWalletAccountTx(ctx context.Context, tx *sql.Tx, tenantID, beneficiaryType, beneficiaryID string, now time.Time) (string, v132CommissionWalletBalances, error) {
	accountID := v132WalletStableID("commission_wallet_", tenantID+"|"+beneficiaryType+"|"+beneficiaryID)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_commission_wallet_accounts(id,tenant_id,beneficiary_type,beneficiary_id,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$5)
		ON CONFLICT(tenant_id,beneficiary_type,beneficiary_id) DO NOTHING
	`, accountID, tenantID, beneficiaryType, beneficiaryID, now)
	if err != nil {
		return "", v132CommissionWalletBalances{}, err
	}
	var balances v132CommissionWalletBalances
	err = tx.QueryRowContext(ctx, `
		SELECT id,expected_cents,frozen_cents,available_cents,settling_cents,settled_cents,recoverable_cents
		FROM xz_commission_wallet_accounts
		WHERE tenant_id=$1 AND beneficiary_type=$2 AND beneficiary_id=$3
		FOR UPDATE
	`, tenantID, beneficiaryType, beneficiaryID).Scan(&accountID, &balances.Expected, &balances.Frozen, &balances.Available, &balances.Settling, &balances.Settled, &balances.Recoverable)
	return accountID, balances, err
}

func updateV132CommissionWalletAccountTx(ctx context.Context, tx *sql.Tx, accountID string, balances v132CommissionWalletBalances, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE xz_commission_wallet_accounts
		SET expected_cents=$2,frozen_cents=$3,available_cents=$4,settling_cents=$5,settled_cents=$6,recoverable_cents=$7,version=version+1,updated_at=$8
		WHERE id=$1
	`, accountID, balances.Expected, balances.Frozen, balances.Available, balances.Settling, balances.Settled, balances.Recoverable, now)
	return err
}

func v132CommissionWalletLedgerExistsTx(ctx context.Context, tx *sql.Tx, idempotencyKey string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_commission_wallet_ledger WHERE idempotency_key=$1)`, idempotencyKey).Scan(&exists)
	return exists, err
}

type v132CommissionWalletLedgerEntry struct {
	ID, TenantID, AccountID, BeneficiaryType, BeneficiaryID string
	BusinessType, BusinessID, Direction                     string
	ExpectedDelta, FrozenDelta, AvailableDelta              int64
	SettlingDelta, SettledDelta, RecoverableDelta           int64
	Before, After                                           v132CommissionWalletBalances
	CommissionRecordID, IdempotencyKey                      string
	Metadata                                                map[string]any
	CreatedAt                                               time.Time
}

func insertV132CommissionWalletLedgerTx(ctx context.Context, tx *sql.Tx, item v132CommissionWalletLedgerEntry) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO xz_commission_wallet_ledger(
		  id,tenant_id,account_id,beneficiary_type,beneficiary_id,business_type,business_id,direction,
		  expected_delta_cents,frozen_delta_cents,available_delta_cents,settling_delta_cents,settled_delta_cents,recoverable_delta_cents,
		  balances_before,balances_after,commission_record_id,idempotency_key,metadata,created_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16::jsonb,$17,$18,$19::jsonb,$20)
		ON CONFLICT(idempotency_key) DO NOTHING
	`, item.ID, item.TenantID, item.AccountID, item.BeneficiaryType, item.BeneficiaryID, item.BusinessType, item.BusinessID, item.Direction,
		item.ExpectedDelta, item.FrozenDelta, item.AvailableDelta, item.SettlingDelta, item.SettledDelta, item.RecoverableDelta,
		jsonProjection(item.Before), jsonProjection(item.After), item.CommissionRecordID, item.IdempotencyKey, jsonProjection(item.Metadata), item.CreatedAt)
	return err
}

func loadV132OrderCommercialSnapshotTx(ctx context.Context, tx *sql.Tx, orderID string) (map[string]any, error) {
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT price_snapshot FROM xz_orders WHERE id=$1`, orderID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func isV132SettlementOrderTx(ctx context.Context, tx *sql.Tx, orderID string) (bool, error) {
	var engine string
	err := tx.QueryRowContext(ctx, `SELECT settlement_engine FROM xz_order_settlement_engine_decisions WHERE order_id=$1`, orderID).Scan(&engine)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(engine, string(settlementEngineV132)), nil
}

func v132WalletStableID(prefix, key string) string {
	digest := sha256.Sum256([]byte(key))
	return prefix + fmt.Sprintf("%x", digest[:16])
}
