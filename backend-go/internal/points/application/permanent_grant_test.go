package application

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestGrantPermanentTxKeepsCallerTransactionAndChecksProjection(t *testing.T) {
	called := 0
	got, err := GrantPermanentTx(context.Background(), &sql.Tx{}, PermanentGrantRequest{UserID: "user-1", Points: 1000, IdempotencyKey: "grant-1"},
		func(context.Context, *sql.Tx, string) (AccountSnapshot, error) {
			called++
			if called == 1 {
				return AccountSnapshot{ID: "account-1", UserID: "user-1", Available: 10}, nil
			}
			return AccountSnapshot{ID: "account-1", UserID: "user-1", Available: 1010}, nil
		},
		func(context.Context, *sql.Tx, PermanentGrantRequest, string) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available != 1010 || called != 2 {
		t.Fatalf("unexpected grant result: %+v calls=%d", got, called)
	}
}

func TestGrantPermanentTxRejectsProjectionMismatch(t *testing.T) {
	_, err := GrantPermanentTx(context.Background(), &sql.Tx{}, PermanentGrantRequest{UserID: "user-1", Points: 1000, IdempotencyKey: "grant-1"},
		func(context.Context, *sql.Tx, string) (AccountSnapshot, error) {
			return AccountSnapshot{ID: "account-1", Available: 10}, nil
		},
		func(context.Context, *sql.Tx, PermanentGrantRequest, string) error { return nil },
	)
	if !errors.Is(err, ErrProjectionMismatch) {
		t.Fatalf("err=%v, want projection mismatch", err)
	}
}
