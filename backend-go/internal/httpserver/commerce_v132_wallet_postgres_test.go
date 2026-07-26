package httpserver

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

func TestV132FormalWalletCreditRefundAndIdempotencyPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := shortID(now.Format(time.RFC3339Nano))
	orderID := "wallet_canary_order_" + suffix
	orderNo := "WALLET-CANARY-" + suffix
	beneficiaryID := "platform_wallet_" + suffix
	sourceUserID := "user_wallet_" + suffix
	var ruleID string
	var ruleVersion int
	if err := tx.QueryRowContext(ctx, `
		SELECT id,version FROM xz_commission_rules
		WHERE commercial_rule_set_id='channel_rules_v132_default_v1'
		ORDER BY priority,id LIMIT 1
	`).Scan(&ruleID, &ruleVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_users(id,email,name,role,status,created_at,updated_at,raw)
		VALUES($1,$2,'Canary Wallet Test','MEMBER','ACTIVE',$3,$3,'{}'::jsonb)
	`, sourceUserID, sourceUserID+"@example.invalid", now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_orders(id,order_no,tenant_id,user_id,plan_id,amount_cents,status,paid_at,created_at,price_snapshot,raw)
		VALUES($1,$2,'tenant_default',$3,'plan_ai_creator_996',99600,'PAID',$4,$4,$5::jsonb,'{}'::jsonb)
	`, orderID, orderNo, sourceUserID, now.Format(time.RFC3339Nano), jsonProjection(map[string]any{
		"ruleSetId": "channel_rules_v132_default_v1", "ruleSetVersion": 1, "settlementEngine": "V132",
	})); err != nil {
		t.Fatal(err)
	}
	record := commissionapp.CommissionRecord{
		ID: "commission_wallet_test_" + suffix, TenantID: "tenant_default", OrderID: orderID, OrderNo: orderNo,
		BeneficiaryType: commissionapp.BeneficiaryPlatform, BeneficiaryID: beneficiaryID, SourceUserID: sourceUserID,
		RuleID: ruleID, RuleVersion: ruleVersion, AmountCents: 59600, Currency: "CNY",
		RecordType: commissionapp.RecordEarning, Status: commissionapp.CommissionFrozen,
		FreezeUntil: canaryTimePointer(now.Add(7 * 24 * time.Hour)), AvailableAt: canaryTimePointer(now.Add(7 * 24 * time.Hour)),
		IdempotencyKey: "wallet-canary-credit:" + orderID, CreatedAt: now, UpdatedAt: now,
	}
	if err := insertImmutableCommissionRecordTx(ctx, tx, record, commissionapp.CommissionRule{Code: "CANARY_TEST"}, adminPlan{}); err != nil {
		t.Fatal(err)
	}
	if err := postV132CommissionRecordToWalletTx(ctx, tx, record); err != nil {
		t.Fatal(err)
	}
	if err := postV132CommissionRecordToWalletTx(ctx, tx, record); err != nil {
		t.Fatal(err)
	}

	var frozen int64
	var creditCount int
	if err := tx.QueryRowContext(ctx, `SELECT frozen_cents FROM xz_commission_wallet_accounts WHERE tenant_id=$1 AND beneficiary_type='PLATFORM' AND beneficiary_id=$2`, "tenant_default", beneficiaryID).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM xz_commission_wallet_ledger WHERE business_id=$1 AND direction='CREDIT'`, orderID).Scan(&creditCount); err != nil {
		t.Fatal(err)
	}
	if frozen != 59600 || creditCount != 1 {
		t.Fatalf("wallet credit must be idempotent frozen=%d credits=%d", frozen, creditCount)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO xz_order_settlement_engine_decisions(
		  order_id,tenant_id,user_id,plan_id,settlement_engine,rule_set_id,rule_set_version,
		  rollout_config_version,rollout_mode,hash_bucket,decision_reason,decided_at
		) VALUES($1,'tenant_default',$2,'plan_ai_creator_996','V132','channel_rules_v132_default_v1',1,1,'CANARY',-1,'ORDER_WHITELIST',$3)
	`, orderID, sourceUserID, now); err != nil {
		t.Fatal(err)
	}
	if err := reverseCommissionRecordsForOrderTx(ctx, tx, orderID, orderNo, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := reverseCommissionRecordsForOrderTx(ctx, tx, orderID, orderNo, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	var debitCount int
	var snapshotRuleSet string
	var reversalRuleID string
	var reversalRuleVersion int
	if err := tx.QueryRowContext(ctx, `SELECT frozen_cents FROM xz_commission_wallet_accounts WHERE tenant_id=$1 AND beneficiary_type='PLATFORM' AND beneficiary_id=$2`, "tenant_default", beneficiaryID).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*),max(metadata->'commercialSnapshot'->>'ruleSetId')
		FROM xz_commission_wallet_ledger WHERE business_id=$1 AND direction='DEBIT'
	`, orderID).Scan(&debitCount, &snapshotRuleSet); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT rule_id,rule_version FROM xz_commission_records WHERE reversal_of_id=$1`, record.ID).Scan(&reversalRuleID, &reversalRuleVersion); err != nil {
		t.Fatal(err)
	}
	if frozen != 0 || debitCount != 1 || snapshotRuleSet != "channel_rules_v132_default_v1" || reversalRuleID != ruleID || reversalRuleVersion != ruleVersion {
		t.Fatalf("refund must use the historical snapshot frozen=%d debits=%d ruleSet=%s rule=%s/%d", frozen, debitCount, snapshotRuleSet, reversalRuleID, reversalRuleVersion)
	}
}

func canaryTimePointer(value time.Time) *time.Time { return &value }
