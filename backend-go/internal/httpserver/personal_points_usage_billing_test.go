package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestJSONPPTAndRAGUsageConsumePersonalLotsAndReplayIsIdempotent(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		record func(*jsonStore, string) error
	}{
		{
			name: "ppt",
			record: func(store *jsonStore, userID string) error {
				_, err := store.RecordPPTGenerationUsage(pptapp.Task{TaskID: "ppt-usage-one", UserID: userID, SlideCount: 1, TextModel: "deepseek-v4-flash"})
				return err
			},
		},
		{
			name: "rag",
			record: func(store *jsonStore, userID string) error {
				return store.RecordRAGUsage(context.Background(), knowledgeapp.RAGBillingUsage{RunID: "rag-usage-one", UserID: userID, Model: "rag-model", InputTokens: 500, OutputTokens: 500, PointCost: 1})
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store, accountID, userID := seedJSONPersonalUsageBalance(t, 10)
			if testCase.name == "ppt" {
				seedDeepSeekPPTBillingRuleForTest(t, store)
			}
			if err := testCase.record(store, userID); err != nil {
				t.Fatal(err)
			}
			assertJSONPersonalUsageState(t, store, accountID, userID, 9, 1, 1)

			if err := testCase.record(store, userID); err != nil {
				t.Fatal(err)
			}
			assertJSONPersonalUsageState(t, store, accountID, userID, 9, 1, 1)

			if _, err := store.PersonalPointService().Reserve(context.Background(), PersonalPointReserveCommand{
				AccountID: accountID, UserID: userID, BusinessType: "AFTER_USAGE", BusinessID: "after-usage",
				RequestedPoints: 9, IdempotencyKey: "after-usage:reserve",
			}); err != nil {
				t.Fatalf("remaining lot balance cannot be reserved: %v", err)
			}
		})
	}
}

func TestJSONPPTAndRAGUsageRequireStableBusinessID(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		record func(*jsonStore, string) error
	}{
		{
			name: "ppt",
			record: func(store *jsonStore, userID string) error {
				_, err := store.RecordPPTGenerationUsage(pptapp.Task{UserID: userID, SlideCount: 1})
				return err
			},
		},
		{
			name: "rag",
			record: func(store *jsonStore, userID string) error {
				return store.RecordRAGUsage(context.Background(), knowledgeapp.RAGBillingUsage{UserID: userID, PointCost: 1})
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store, accountID, userID := seedJSONPersonalUsageBalance(t, 10)
			err := testCase.record(store, userID)
			if !errors.Is(err, ErrInvalidPointCommand) {
				t.Fatalf("missing stable business id error = %v", err)
			}
			assertJSONPersonalUsageState(t, store, accountID, userID, 10, 0, 0)
		})
	}
}

func seedJSONPersonalUsageBalance(t *testing.T, points int64) (*jsonStore, string, string) {
	t.Helper()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	user, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Usage Billing", Email: "usage-billing@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.PointAccount(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: account.ID, UserID: user.ID, Source: PointSourceRecharge, Points: points,
		ReferenceType: "TEST", ReferenceID: "usage-seed", IdempotencyKey: "usage-seed",
	}); err != nil {
		t.Fatal(err)
	}
	return store, account.ID, user.ID
}

func assertJSONPersonalUsageState(t *testing.T, store *jsonStore, accountID, userID string, available, consumed int64, reservations int) {
	t.Helper()
	balance, err := store.PersonalPointService().GetBalance(context.Background(), accountID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != available || balance.Frozen != 0 {
		t.Fatalf("balance = %+v, want available/frozen %d/0", balance, available)
	}
	lots, err := store.PersonalPointService().ListLots(context.Background(), accountID, userID, PersonalPointLotFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var lotAvailable, lotConsumed int64
	for _, lot := range lots {
		lotAvailable += lot.AvailablePoints
		lotConsumed += lot.ConsumedPoints
	}
	if lotAvailable != available || lotConsumed != consumed {
		t.Fatalf("lot available/consumed = %d/%d, want %d/%d; lots=%+v", lotAvailable, lotConsumed, available, consumed, lots)
	}
	state, err := store.PersonalPointService().repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Reservations) != reservations {
		t.Fatalf("reservations = %+v, want %d", state.Reservations, reservations)
	}
	if reservations == 1 && (state.Reservations[0].CapturedPoints != consumed || state.Reservations[0].ReservedPoints != 0) {
		t.Fatalf("captured reservation = %+v, want captured/reserved %d/0", state.Reservations[0], consumed)
	}
}

func TestPostgresPersonalUsageChargeUsesCallerTransactionAndIsIdempotent(t *testing.T) {
	db, pointStore, ctx := openPersonalPointFixRound1Postgres(t)
	ensurePersonalPointUserWalletTestSchema(t, db, ctx)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID, businessID := "usage-account-"+suffix, "usage-user-"+suffix, "usage-business-"+suffix
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge, Points: 10,
		ReferenceType: "TEST", ReferenceID: businessID, IdempotencyKey: "usage-seed-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, after, err := chargePostgresPersonalPointUsage(ctx, db, tx, personalPointUsageChargeCommand{
		UserID: userID, BusinessType: "RAG_RUN", BusinessID: businessID, Points: 1, IdempotencyPrefix: "rag:" + businessID,
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if before != 10 || after != 9 {
		_ = tx.Rollback()
		t.Fatalf("rollback charge before/after = %d/%d, want 10/9", before, after)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertPostgresPersonalUsageState(t, db, ctx, accountID, 10, 0, 0)

	for attempt := 0; attempt < 2; attempt++ {
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		before, after, err = chargePostgresPersonalPointUsage(ctx, db, tx, personalPointUsageChargeCommand{
			UserID: userID, BusinessType: "RAG_RUN", BusinessID: businessID, Points: 1, IdempotencyPrefix: "rag:" + businessID,
		})
		if err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if before != 9 || after != 9 {
		t.Fatalf("idempotent replay before/after = %d/%d, want 9/9", before, after)
	}
	assertPostgresPersonalUsageState(t, db, ctx, accountID, 9, 1, 1)
}

func TestPostgresPPTAndRAGUsageConsumePersonalLotsAndRollbackAfterCharge(t *testing.T) {
	db, pointStore, ctx := openPersonalPointFixRound1Postgres(t)
	ensurePersonalPointUserWalletTestSchema(t, db, ctx)
	for _, testCase := range []struct {
		name   string
		record func(*postgresStore, string, string) error
	}{
		{
			name: "ppt",
			record: func(store *postgresStore, userID, businessID string) error {
				_, err := store.RecordPPTGenerationUsage(pptapp.Task{TaskID: businessID, UserID: userID, SlideCount: 1, TextModel: "ppt-text-model"})
				return err
			},
		},
		{
			name: "rag",
			record: func(store *postgresStore, userID, businessID string) error {
				return store.RecordRAGUsage(ctx, knowledgeapp.RAGBillingUsage{RunID: businessID, UserID: userID, Model: "rag-model", InputTokens: 500, OutputTokens: 500, PointCost: 1})
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			suffix := testCase.name + strconv.FormatInt(time.Now().UnixNano(), 36)
			store := &postgresStore{db: db, ready: true}
			accountID, userID, businessID := "usage-method-account-"+suffix, "usage-method-user-"+suffix, "usage-method-business-"+suffix
			seedPostgresPersonalUsageUser(t, db, pointStore, ctx, accountID, userID, suffix)

			if err := testCase.record(store, userID, businessID); err != nil {
				t.Fatal(err)
			}
			var pointCost int64
			if err := db.QueryRowContext(ctx, `SELECT point_cost FROM xz_billing_events WHERE task_id=$1`, businessID).Scan(&pointCost); err != nil {
				t.Fatal(err)
			}
			if pointCost <= 0 {
				t.Fatalf("billing point cost=%d, want positive", pointCost)
			}
			assertPostgresPersonalUsageState(t, db, ctx, accountID, 10-pointCost, pointCost, 1)
			if err := testCase.record(store, userID, businessID); err != nil {
				t.Fatal(err)
			}
			assertPostgresPersonalUsageState(t, db, ctx, accountID, 10-pointCost, pointCost, 1)
			var eventCount int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_billing_events WHERE task_id=$1`, businessID).Scan(&eventCount); err != nil {
				t.Fatal(err)
			}
			if eventCount != 1 {
				t.Fatalf("billing event count=%d, want 1", eventCount)
			}

			rollbackAccountID, rollbackUserID, rollbackBusinessID := "usage-rollback-account-"+suffix, "usage-rollback-user-"+suffix, "usage-rollback-business-"+suffix
			seedPostgresPersonalUsageUser(t, db, pointStore, ctx, rollbackAccountID, rollbackUserID, "rollback-"+suffix)
			triggerName := "trg_usage_fail_" + suffix
			functionName := "fn_usage_fail_" + suffix
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced billing failure'; END $$`, functionName)); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = db.ExecContext(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON xz_billing_events`, triggerName))
				_, _ = db.ExecContext(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
			})
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON xz_billing_events FOR EACH ROW WHEN (NEW.task_id = %s) EXECUTE FUNCTION %s()`, triggerName, quotePostgresTestLiteral(rollbackBusinessID), functionName)); err != nil {
				t.Fatal(err)
			}
			if err := testCase.record(store, rollbackUserID, rollbackBusinessID); err == nil {
				t.Fatal("forced downstream billing failure unexpectedly succeeded")
			}
			assertPostgresPersonalUsageState(t, db, ctx, rollbackAccountID, 10, 0, 0)
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP TRIGGER %s ON xz_billing_events`, triggerName)); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`DROP FUNCTION %s()`, functionName)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresPPTAndRAGUsageRequireStableBusinessIDBeforeDatabaseAccess(t *testing.T) {
	store := &postgresStore{}
	if _, err := store.RecordPPTGenerationUsage(pptapp.Task{UserID: "user"}); !errors.Is(err, ErrInvalidPointCommand) {
		t.Fatalf("PPT missing task ID error=%v", err)
	}
	if err := store.RecordRAGUsage(context.Background(), knowledgeapp.RAGBillingUsage{UserID: "user", PointCost: 1}); !errors.Is(err, ErrInvalidPointCommand) {
		t.Fatalf("RAG missing run ID error=%v", err)
	}
}

func seedPostgresPersonalUsageUser(t *testing.T, db *sql.DB, pointStore *PostgresPersonalPointStore, ctx context.Context, accountID, userID, suffix string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_users(id,email,name,role,status,created_at,updated_at,raw) VALUES($1,$2,'Usage Billing Test','MEMBER','ACTIVE',now()::text,now()::text,'{}'::jsonb)`, userID, suffix+"@usage.example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge, Points: 10,
		ReferenceType: "TEST", ReferenceID: suffix, IdempotencyKey: "usage-method-seed-" + suffix,
	}); err != nil {
		t.Fatal(err)
	}
}

func quotePostgresTestLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func assertPostgresPersonalUsageState(t *testing.T, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, ctx context.Context, accountID string, available, consumed int64, reservations int) {
	t.Helper()
	var accountAvailable, frozen, lotAvailable, lotReserved, lotConsumed int64
	if err := db.QueryRowContext(ctx, `SELECT available,frozen FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&accountAvailable, &frozen); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(sum(available_points),0),COALESCE(sum(reserved_points),0),COALESCE(sum(consumed_points),0) FROM xz_personal_point_lots WHERE account_id=$1`, accountID).Scan(&lotAvailable, &lotReserved, &lotConsumed); err != nil {
		t.Fatal(err)
	}
	var reservationCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_reservations WHERE account_id=$1`, accountID).Scan(&reservationCount); err != nil {
		t.Fatal(err)
	}
	if accountAvailable != available || frozen != 0 || lotAvailable != available || lotReserved != 0 || lotConsumed != consumed || reservationCount != reservations {
		t.Fatalf("postgres personal usage state account=%d/%d lots=%d/%d/%d reservations=%d, want available/consumed/reservations %d/%d/%d", accountAvailable, frozen, lotAvailable, lotReserved, lotConsumed, reservationCount, available, consumed, reservations)
	}
}
