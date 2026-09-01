package messaging

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestOutboxPostgresSchemaClaimAndAtomicTransitions(t *testing.T) {
	dsn := os.Getenv("XIANZHI_MESSAGING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_MESSAGING_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	envelope := MustEnvelope("x.ai.test", "test", "outbox-integration", nil)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := NewOutboxStore(db)
	if err := store.InsertTx(ctx, tx, envelope, "test", "outbox-integration", "trace"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), `DELETE FROM outbox_events WHERE event_id=$1`, envelope.EventID)
	rows, err := store.Claim(ctx, 1, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].EventID != envelope.EventID || rows[0].OccurredAt.IsZero() {
		t.Fatalf("claimed=%+v", rows)
	}
	if err := store.MarkFailure(ctx, rows[0].ID, context.DeadlineExceeded, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	var status string
	var attempts int
	if err := db.QueryRowContext(ctx, `SELECT status,attempt_count FROM outbox_events WHERE event_id=$1`, envelope.EventID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != OutboxFailed || attempts != 1 {
		t.Fatalf("status=%s attempts=%d", status, attempts)
	}
}
