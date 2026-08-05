package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type failWriteBackend struct {
	delegate stateBackend
	err      error
}

func (b failWriteBackend) Read() ([]byte, error) { return b.delegate.Read() }
func (b failWriteBackend) Write([]byte) error    { return b.err }

func TestPersonalPointsPostgresCallerRollbackRemovesAllProjections(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	ensurePersonalPointUserWalletTestSchema(t, db, ctx)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID := "tx-rollback-account-"+suffix, "tx-rollback-user-"+suffix
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.grantTx(ctx, tx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge, Points: 11,
		IdempotencyKey: "caller-rollback", GrantedAt: time.Now().UTC(),
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{
		`SELECT count(*) FROM xz_point_accounts WHERE id=$1`,
		`SELECT count(*) FROM xz_personal_point_lots WHERE account_id=$1`,
		`SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1`,
		`SELECT count(*) FROM xz_user_wallets WHERE user_id=$1`,
	} {
		var count int
		if err := db.QueryRowContext(ctx, query, map[bool]string{true: userID, false: accountID}[query == `SELECT count(*) FROM xz_user_wallets WHERE user_id=$1`]).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback left %d row(s) for query %q", count, query)
		}
	}
}

func TestPersonalPointsPostgresCommitReconcilesAllProjections(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	ensurePersonalPointUserWalletTestSchema(t, db, ctx)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID := "tx-commit-account-"+suffix, "tx-commit-user-"+suffix
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.grantTx(ctx, tx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge, Points: 13,
		IdempotencyKey: "caller-commit", GrantedAt: time.Now().UTC(),
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var available, frozen, rawAvailable, rawFrozen, lotAvailable, lotReserved int64
	if err := db.QueryRowContext(ctx, `SELECT available,frozen,COALESCE((raw->>'available')::bigint,-1),COALESCE((raw->>'frozen')::bigint,-1) FROM xz_point_accounts WHERE id=$1`, accountID).Scan(&available, &frozen, &rawAvailable, &rawFrozen); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(sum(available_points),0),COALESCE(sum(reserved_points),0) FROM xz_personal_point_lots WHERE account_id=$1`, accountID).Scan(&lotAvailable, &lotReserved); err != nil {
		t.Fatal(err)
	}
	var walletAvailable, walletFrozen int64
	if err := db.QueryRowContext(ctx, `SELECT token_balance,frozen_token FROM xz_user_wallets WHERE user_id=$1`, userID).Scan(&walletAvailable, &walletFrozen); err != nil {
		t.Fatal(err)
	}
	if available != 13 || frozen != 0 || rawAvailable != available || rawFrozen != frozen || lotAvailable != available || lotReserved != frozen || walletAvailable != available || walletFrozen != frozen {
		t.Fatalf("projection mismatch account=(%d,%d) raw=(%d,%d) lots=(%d,%d) wallet=(%d,%d)", available, frozen, rawAvailable, rawFrozen, lotAvailable, lotReserved, walletAvailable, walletFrozen)
	}
}

func TestPostgresGenerationImmediateAndTerminalRollbackAreAtomic(t *testing.T) {
	db, pointStore, ctx := openPersonalPointFixRound1Postgres(t)
	ensurePersonalPointUserWalletTestSchema(t, db, ctx)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID, taskID := "generation-account-"+suffix, "generation-user-"+suffix, "generation-task-"+suffix
	if _, err := pointStore.grant(ctx, PersonalPointGrantCommand{AccountID: accountID, UserID: userID, Source: PointSourceRecharge, Points: 5, IdempotencyKey: "generation-seed"}); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := pointStore.reserveTx(ctx, tx, PersonalPointReserveCommand{AccountID: accountID, UserID: userID, BusinessType: "GENERATION_TASK", BusinessID: taskID, RequestedPoints: 2, IdempotencyKey: "generation:reserve:" + taskID})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := pointStore.captureTx(ctx, tx, PersonalPointCaptureCommand{AccountID: accountID, UserID: userID, ReservationID: reserved.Reservation.ID, Points: 2, IdempotencyKey: "generation:capture:" + taskID}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	markerRaw := fmt.Sprintf(`{"id":%q,"userId":%q,"billingEngine":%q,"personalPointAccountId":%q,"personalPointReservationId":%q}`, taskID, userID, personalLotBillingEngine, accountID, reserved.Reservation.ID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO xz_generation_tasks(id,user_id,type,model,status,progress,point_cost,prompt,params,result_ids,error,created_at,updated_at,worker_finished_at,raw) VALUES($1,$2,'TEXT_TO_IMAGE','test','SUCCEEDED',100,2,'','{}'::jsonb,'[]'::jsonb,'null'::jsonb,$3,$3,$3,$4::jsonb)`, taskID, userID, now, markerRaw); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var taskRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&taskRows); err != nil {
		t.Fatal(err)
	}
	balance, err := pointStore.getBalance(ctx, accountID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if taskRows != 0 || balance.Available != 5 || balance.Frozen != 0 {
		t.Fatalf("immediate rollback leaked task/balance: taskRows=%d balance=%+v", taskRows, balance)
	}

	committed, err := pointStore.reserve(ctx, PersonalPointReserveCommand{AccountID: accountID, UserID: userID, BusinessType: "GENERATION_TASK", BusinessID: taskID + "-terminal", RequestedPoints: 2, IdempotencyKey: "generation:reserve:" + taskID + "-terminal"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pointStore.captureTx(ctx, tx, PersonalPointCaptureCommand{AccountID: accountID, UserID: userID, ReservationID: committed.Reservation.ID, Points: 2, IdempotencyKey: "generation:capture:" + taskID + "-terminal"}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var reservedPoints, capturedPoints int64
	if err := db.QueryRowContext(ctx, `SELECT reserved_points,captured_points FROM xz_personal_point_reservations WHERE id=$1`, committed.Reservation.ID).Scan(&reservedPoints, &capturedPoints); err != nil {
		t.Fatal(err)
	}
	if reservedPoints != 2 || capturedPoints != 0 {
		t.Fatalf("terminal rollback reservation=(reserved:%d captured:%d), want 2/0", reservedPoints, capturedPoints)
	}
}

func ensurePersonalPointUserWalletTestSchema(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS xz_user_wallets (
		user_id text primary key,
		token_balance bigint not null default 0,
		cash_balance_cents bigint not null default 0,
		frozen_token bigint not null default 0,
		total_token_granted bigint not null default 0,
		total_token_used bigint not null default 0,
		updated_at timestamptz not null default now(),
		raw jsonb not null default '{}'::jsonb
	)`); err != nil {
		t.Fatal(err)
	}
}

func TestJSONPersonalPointsMainDocumentWriteFailureIsAtomic(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.save(platformData{Counters: map[string]int{}}); err != nil {
		t.Fatal(err)
	}
	store.backend = failWriteBackend{delegate: fileStateBackend{path: path}, err: errors.New("injected main document write failure")}
	err := store.updateWithPersonalPoints(ctx, func(data *platformData, points *JSONPersonalPointStore) error {
		data.GenerationTasks = append(data.GenerationTasks, generationTask{ID: "atomic-task", UserID: "atomic-user", Status: "pending"})
		_, err := points.grant(ctx, PersonalPointGrantCommand{
			AccountID: "atomic-account", UserID: "atomic-user", Source: PointSourceRecharge,
			Points: 17, IdempotencyKey: "atomic-grant", GrantedAt: time.Now().UTC(),
		})
		return err
	})
	if err == nil {
		t.Fatal("injected main document write failure was ignored")
	}

	reloaded := newJSONStore(path)
	data, err := reloaded.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.GenerationTasks) != 0 || len(data.PersonalPoints.Accounts) != 0 || len(data.PersonalPoints.Lots) != 0 || len(data.PersonalPoints.WalletLedger) != 0 || len(data.PointAccounts) != 0 || len(data.WalletLedger) != 0 {
		t.Fatalf("failed write leaked state: tasks=%d personal=(accounts:%d lots:%d ledger:%d) projections=(accounts:%d ledger:%d)", len(data.GenerationTasks), len(data.PersonalPoints.Accounts), len(data.PersonalPoints.Lots), len(data.PersonalPoints.WalletLedger), len(data.PointAccounts), len(data.WalletLedger))
	}
}

func TestJSONPersonalPointsIgnoreSidecarAfterSuccessfulImport(t *testing.T) {
	ctx := context.Background()
	for _, testCase := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "modified", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(`{"accounts":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "deleted", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "invalid json", mutate: func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(`{invalid`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "platform.json")
			if err := newJSONStore(path).save(platformData{Counters: map[string]int{}}); err != nil {
				t.Fatal(err)
			}
			sidecarPath := path + ".personal-points.json"
			sidecar := NewJSONPersonalPointStore(sidecarPath)
			if _, err := sidecar.grant(ctx, PersonalPointGrantCommand{AccountID: "import-account", UserID: "import-user", Source: PointSourceRecharge, Points: 19, IdempotencyKey: "sidecar-first"}); err != nil {
				t.Fatal(err)
			}
			store := newJSONStore(path)
			balance, err := store.PersonalPointService().GetBalance(ctx, "import-account", "import-user")
			if err != nil || balance.Available != 19 {
				t.Fatalf("initial import balance=%+v err=%v", balance, err)
			}
			data, err := store.load()
			if err != nil {
				t.Fatal(err)
			}
			if data.PersonalPointImport.Version != personalPointImportVersion || data.PersonalPointImport.SidecarChecksum == "" || len(data.PersonalPoints.Lots) != 1 {
				t.Fatalf("sidecar import metadata/state = %+v lots=%d", data.PersonalPointImport, len(data.PersonalPoints.Lots))
			}

			testCase.mutate(t, sidecarPath)
			service := newJSONStore(path).PersonalPointService()
			if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "import-account", UserID: "import-user", Source: PointSourceAdminGift, Points: 2, IdempotencyKey: "post-import-gift"}); err != nil {
				t.Fatalf("post-import gift: %v", err)
			}
			reserved, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "import-account", UserID: "import-user", BusinessType: "POST_IMPORT", BusinessID: "post-import", RequestedPoints: 1, IdempotencyKey: "post-import-reserve"})
			if err != nil {
				t.Fatalf("post-import reserve: %v", err)
			}
			if _, err := service.Capture(ctx, PersonalPointCaptureCommand{AccountID: "import-account", UserID: "import-user", ReservationID: reserved.Reservation.ID, Points: 1, IdempotencyKey: "post-import-capture"}); err != nil {
				t.Fatalf("post-import capture: %v", err)
			}
			if _, err := service.Correct(ctx, PersonalPointCorrectionCommand{AccountID: "import-account", UserID: "import-user", Points: 1, Reason: "post-import correction", IdempotencyKey: "post-import-correction"}); err != nil {
				t.Fatalf("post-import correction: %v", err)
			}
			balance, err = service.GetBalance(ctx, "import-account", "import-user")
			if err != nil || balance.Available != 21 {
				t.Fatalf("post-import balance=%+v err=%v, want 21", balance, err)
			}
		})
	}
}

func TestJSONPersonalPointsInvalidSidecarFailsClosedBeforeFirstImport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	if err := newJSONStore(path).save(platformData{Counters: map[string]int{}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".personal-points.json", []byte(`{invalid`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newJSONStore(path).PersonalPointService().GetBalance(context.Background(), "import-account", "import-user")
	if !errors.Is(err, ErrPersonalPointImportConflict) {
		t.Fatalf("initial invalid sidecar error=%v, want fail-closed import conflict", err)
	}
}

func TestJSONPersonalPointsSurviveAdminPlatformRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if _, err := store.PersonalPointService().Grant(ctx, PersonalPointGrantCommand{
		AccountID: "admin-roundtrip-account", UserID: "admin-roundtrip-user", Source: PointSourceRecharge,
		Points: 23, IdempotencyKey: "admin-roundtrip-grant",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		if data.Counters == nil {
			data.Counters = map[string]int{}
		}
		data.Counters["admin_roundtrip"]++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	balance, err := newJSONStore(path).PersonalPointService().GetBalance(ctx, "admin-roundtrip-account", "admin-roundtrip-user")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 23 {
		t.Fatalf("admin round-trip balance=%d, want 23", balance.Available)
	}
}

func TestJSONRegisteredCustomerAndGiftCommitTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	created, err := store.CreateRegisteredCustomer(adminCustomerMutation{Name: "Registration User", Email: "registration@example.test", PlanID: "plan_free"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Users) != 1 || data.Users[0].ID != created.ID || len(data.PersonalPoints.Lots) != 1 || data.PersonalPoints.Lots[0].SourceType != PointSourceRegistrationGift || data.PersonalPoints.Lots[0].OriginalPoints != 7 {
		t.Fatalf("atomic registration state users=%+v lots=%+v", data.Users, data.PersonalPoints.Lots)
	}
}
