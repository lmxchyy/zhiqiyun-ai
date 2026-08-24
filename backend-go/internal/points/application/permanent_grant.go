package application

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidPermanentGrant = errors.New("invalid permanent point grant")
	ErrProjectionMismatch    = errors.New("personal point inflow projection mismatch")
)

type PermanentGrantRequest struct {
	UserID, Source, ReferenceType, ReferenceID, IdempotencyKey string
	Points                                                     int64
	GrantedAt                                                  time.Time
}

type AccountSnapshot struct {
	ID, UserID                                 string
	Available, Frozen, TotalGranted, TotalUsed int64
}

type LoadAccountTx func(context.Context, *sql.Tx, string) (AccountSnapshot, error)
type GrantTx func(context.Context, *sql.Tx, PermanentGrantRequest, string) error

// GrantPermanentTx owns the cross-store application sequence for a permanent
// point inflow. Persistence remains in the injected Points repository hooks,
// while the caller owns the surrounding transaction and commit.
func GrantPermanentTx(ctx context.Context, tx *sql.Tx, request PermanentGrantRequest, load LoadAccountTx, grant GrantTx) (AccountSnapshot, error) {
	if tx == nil || strings.TrimSpace(request.UserID) == "" || request.Points <= 0 || strings.TrimSpace(request.IdempotencyKey) == "" || load == nil || grant == nil {
		return AccountSnapshot{}, ErrInvalidPermanentGrant
	}
	before, err := load(ctx, tx, request.UserID)
	if err != nil {
		return AccountSnapshot{}, err
	}
	if err := grant(ctx, tx, request, before.ID); err != nil {
		return AccountSnapshot{}, err
	}
	after, err := load(ctx, tx, request.UserID)
	if err != nil {
		return AccountSnapshot{}, err
	}
	if after.Available-before.Available != request.Points {
		return AccountSnapshot{}, ErrProjectionMismatch
	}
	return after, nil
}
