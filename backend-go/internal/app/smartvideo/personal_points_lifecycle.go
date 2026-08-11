package smartvideo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const PersonalPointBusinessType = "SMART_VIDEO_RENDER"

// PersonalPointLedger is the subset of personal-point operations SmartVideo needs.
// Implemented by httpserver adapters around PersonalPointService.
type PersonalPointLedger interface {
	Reserve(ctx context.Context, accountID, userID, businessType, businessID string, points int64, idempotencyKey string) (reservationID string, err error)
	Capture(ctx context.Context, accountID, userID, reservationID string, points int64, idempotencyKey string) error
	Release(ctx context.Context, accountID, userID, reservationID string, points int64, idempotencyKey string) error
}

// AccountIDResolver maps a user to their personal point account.
type AccountIDResolver interface {
	ResolvePersonalPointAccountID(ctx context.Context, userID string) (string, error)
}

type personalPointTaskState struct {
	AccountID     string
	UserID        string
	ReservationID string
	Points        int64
	Captured      bool
	Released      bool
}

// PersonalPointsLifecycle freezes/captures/releases personal points for montage exports.
type PersonalPointsLifecycle struct {
	ledger  PersonalPointLedger
	accounts AccountIDResolver
	now     func() time.Time

	mu    sync.Mutex
	byTask map[string]personalPointTaskState
}

func NewPersonalPointsLifecycle(ledger PersonalPointLedger, accounts AccountIDResolver) *PersonalPointsLifecycle {
	return &PersonalPointsLifecycle{
		ledger: ledger, accounts: accounts,
		now:    func() time.Time { return time.Now().UTC() },
		byTask: map[string]personalPointTaskState{},
	}
}

func (p *PersonalPointsLifecycle) Quote(_ context.Context, input RenderQuoteInput) (RenderQuote, error) {
	return EstimateRenderQuote(input, p.now()), nil
}

func (p *PersonalPointsLifecycle) Reserve(ctx context.Context, access Access, taskID string, quote RenderQuote) (string, error) {
	if p == nil || p.ledger == nil || p.accounts == nil {
		return "", ErrExportNotReady
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || strings.TrimSpace(access.UserID) == "" {
		return "", ErrInvalidInput
	}
	if quote.ExpiresAt.Before(p.now()) {
		return "", ErrQuoteExpired
	}
	p.mu.Lock()
	if existing, ok := p.byTask[taskID]; ok && existing.ReservationID != "" && !existing.Released {
		p.mu.Unlock()
		return existing.ReservationID, nil
	}
	p.mu.Unlock()

	accountID, err := p.accounts.ResolvePersonalPointAccountID(ctx, access.UserID)
	if err != nil {
		return "", err
	}
	reservationID, err := p.ledger.Reserve(
		ctx, accountID, access.UserID, PersonalPointBusinessType, taskID, quote.Points,
		fmt.Sprintf("sv_render_%s_reserve", taskID),
	)
	if err != nil {
		if isInsufficientPointsError(err) {
			return "", ErrInsufficientPoints
		}
		return "", err
	}
	p.mu.Lock()
	p.byTask[taskID] = personalPointTaskState{
		AccountID: accountID, UserID: access.UserID, ReservationID: reservationID, Points: quote.Points,
	}
	p.mu.Unlock()
	return reservationID, nil
}

func (p *PersonalPointsLifecycle) Capture(ctx context.Context, access Access, taskID string) error {
	state, err := p.lookupTask(taskID)
	if err != nil {
		return err
	}
	if state.Released {
		return ErrInvalidStateTransition
	}
	if state.Captured {
		return nil
	}
	if err := p.ledger.Capture(
		ctx, state.AccountID, firstNonEmpty(access.UserID, state.UserID), state.ReservationID, state.Points,
		fmt.Sprintf("sv_render_%s_capture", taskID),
	); err != nil {
		return err
	}
	p.mu.Lock()
	state.Captured = true
	p.byTask[taskID] = state
	p.mu.Unlock()
	return nil
}

func (p *PersonalPointsLifecycle) Release(ctx context.Context, access Access, taskID, _ string) error {
	state, err := p.lookupTask(taskID)
	if err != nil {
		return err
	}
	if state.Captured {
		return ErrInvalidStateTransition
	}
	if state.Released {
		return nil
	}
	if err := p.ledger.Release(
		ctx, state.AccountID, firstNonEmpty(access.UserID, state.UserID), state.ReservationID, state.Points,
		fmt.Sprintf("sv_render_%s_release", taskID),
	); err != nil {
		return err
	}
	p.mu.Lock()
	state.Released = true
	p.byTask[taskID] = state
	p.mu.Unlock()
	return nil
}

func (p *PersonalPointsLifecycle) lookupTask(taskID string) (personalPointTaskState, error) {
	if p == nil {
		return personalPointTaskState{}, ErrExportNotReady
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state, ok := p.byTask[strings.TrimSpace(taskID)]
	if !ok || state.ReservationID == "" {
		return personalPointTaskState{}, ErrNotFound
	}
	return state, nil
}

func isInsufficientPointsError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrInsufficientPoints) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "insufficient points") || strings.Contains(msg, "insufficient_points")
}
