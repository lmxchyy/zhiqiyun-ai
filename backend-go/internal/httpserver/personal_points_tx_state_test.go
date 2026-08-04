package httpserver

import (
	"context"
	"database/sql"
	"errors"
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

func TestJSONPersonalPointsSidecarImportsOnceAndConflictsFailClosed(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "platform.json")
	if err := newJSONStore(path).save(platformData{Counters: map[string]int{}}); err != nil {
		t.Fatal(err)
	}
	sidecar := NewJSONPersonalPointStore(path + ".personal-points.json")
	if _, err := sidecar.grant(ctx, PersonalPointGrantCommand{AccountID: "import-account", UserID: "import-user", Source: PointSourceRecharge, Points: 19, IdempotencyKey: "sidecar-first"}); err != nil {
		t.Fatal(err)
	}
	store := newJSONStore(path)
	balance, err := store.PersonalPointService().GetBalance(ctx, "import-account", "import-user")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 19 {
		t.Fatalf("imported balance=%d, want 19", balance.Available)
	}
	data, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if data.PersonalPointImport.Version != 1 || data.PersonalPointImport.SidecarChecksum == "" || len(data.PersonalPoints.Lots) != 1 {
		t.Fatalf("sidecar import metadata/state = %+v lots=%d", data.PersonalPointImport, len(data.PersonalPoints.Lots))
	}

	if _, err := sidecar.grant(ctx, PersonalPointGrantCommand{AccountID: "import-account", UserID: "import-user", Source: PointSourceRecharge, Points: 1, IdempotencyKey: "sidecar-conflict"}); err != nil {
		t.Fatal(err)
	}
	_, err = newJSONStore(path).PersonalPointService().GetBalance(ctx, "import-account", "import-user")
	if !errors.Is(err, ErrPersonalPointImportConflict) {
		t.Fatalf("mutated sidecar error=%v, want fail-closed import conflict", err)
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
