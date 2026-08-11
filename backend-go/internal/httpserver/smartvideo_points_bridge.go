package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type personalPointLedgerAdapter struct {
	service *PersonalPointService
}

func (a personalPointLedgerAdapter) Reserve(ctx context.Context, accountID, userID, businessType, businessID string, points int64, idempotencyKey string) (string, error) {
	if a.service == nil {
		return "", fmt.Errorf("personal point service unavailable")
	}
	result, err := a.service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: accountID, UserID: userID, BusinessType: businessType, BusinessID: businessID,
		RequestedPoints: points, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return "", err
	}
	return result.Reservation.ID, nil
}

func (a personalPointLedgerAdapter) Capture(ctx context.Context, accountID, userID, reservationID string, points int64, idempotencyKey string) error {
	if a.service == nil {
		return fmt.Errorf("personal point service unavailable")
	}
	_, err := a.service.Capture(ctx, PersonalPointCaptureCommand{
		AccountID: accountID, UserID: userID, ReservationID: reservationID, Points: points, IdempotencyKey: idempotencyKey,
	})
	return err
}

func (a personalPointLedgerAdapter) Release(ctx context.Context, accountID, userID, reservationID string, points int64, idempotencyKey string) error {
	if a.service == nil {
		return fmt.Errorf("personal point service unavailable")
	}
	_, err := a.service.Release(ctx, PersonalPointReleaseCommand{
		AccountID: accountID, UserID: userID, ReservationID: reservationID, Points: points, IdempotencyKey: idempotencyKey,
	})
	return err
}

type personalPointAccountResolver struct {
	store platformStore
}

func (r personalPointAccountResolver) ResolvePersonalPointAccountID(_ context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id required")
	}
	account, err := r.store.PointAccount(userID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(account.ID) == "" {
		return "", fmt.Errorf("point account not found")
	}
	return account.ID, nil
}

func newSmartVideoPointsLifecycle(store platformStore) smartvideo.PointsLifecycle {
	provider, ok := store.(interface{ PersonalPointService() *PersonalPointService })
	if !ok || provider.PersonalPointService() == nil {
		return smartvideo.NewMemoryPointsLifecycle(1 << 62)
	}
	return smartvideo.NewPersonalPointsLifecycle(
		personalPointLedgerAdapter{service: provider.PersonalPointService()},
		personalPointAccountResolver{store: store},
	)
}

// NewSmartVideoPointsLifecycleFromDB wires worker settle/export to the personal point ledger.
func NewSmartVideoPointsLifecycleFromDB(db *sql.DB) smartvideo.PointsLifecycle {
	if db == nil {
		return smartvideo.NewMemoryPointsLifecycle(1 << 62)
	}
	return smartvideo.NewPersonalPointsLifecycle(
		personalPointLedgerAdapter{service: NewPersonalPointService(NewPostgresPersonalPointStore(db))},
		postgresPersonalPointAccountResolver{db: db},
	).SetReservationLoader(postgresPersonalReservationLoader{db: db})
}

type postgresPersonalPointAccountResolver struct {
	db *sql.DB
}

func (r postgresPersonalPointAccountResolver) ResolvePersonalPointAccountID(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || r.db == nil {
		return "", fmt.Errorf("user id required")
	}
	var accountID string
	err := r.db.QueryRowContext(ctx, `select id from xz_point_accounts where user_id=$1 order by id limit 1`, userID).Scan(&accountID)
	if err != nil {
		return "", err
	}
	return accountID, nil
}

type postgresPersonalReservationLoader struct {
	db *sql.DB
}

func (l postgresPersonalReservationLoader) LoadByBusinessID(ctx context.Context, businessType, businessID string) (accountID, userID, reservationID string, reservedPoints int64, status string, err error) {
	if l.db == nil {
		return "", "", "", 0, "", smartvideo.ErrNotFound
	}
	businessType = strings.TrimSpace(businessType)
	businessID = strings.TrimSpace(businessID)
	if businessType == "" || businessID == "" {
		return "", "", "", 0, "", smartvideo.ErrNotFound
	}
	err = l.db.QueryRowContext(ctx, `
		select id, account_id, user_id, reserved_points, status
		  from xz_personal_point_reservations
		 where business_type=$1 and business_id=$2
		 order by created_at desc
		 limit 1`, businessType, businessID).
		Scan(&reservationID, &accountID, &userID, &reservedPoints, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", 0, "", smartvideo.ErrNotFound
	}
	return accountID, userID, reservationID, reservedPoints, status, err
}
