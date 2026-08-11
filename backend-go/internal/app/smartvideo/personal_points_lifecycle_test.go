package smartvideo

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakePersonalLedger struct {
	mu            sync.Mutex
	balance       int64
	reservations  map[string]int64
	captured      map[string]bool
	released      map[string]bool
	failInsufficient bool
}

func newFakePersonalLedger(balance int64) *fakePersonalLedger {
	return &fakePersonalLedger{
		balance: balance, reservations: map[string]int64{}, captured: map[string]bool{}, released: map[string]bool{},
	}
}

func (f *fakePersonalLedger) Reserve(_ context.Context, _, _, _, businessID string, points int64, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.reservations[businessID]; ok {
		return "res_" + businessID, nil
		_ = existing
	}
	if f.failInsufficient || points > f.balance {
		return "", errors.New("insufficient points")
	}
	f.balance -= 0 // freeze only tracked in reservations map for simplicity
	f.reservations[businessID] = points
	return "res_" + businessID, nil
}

func (f *fakePersonalLedger) Capture(_ context.Context, _, _, reservationID string, points int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	taskID := strings.TrimPrefix(reservationID, "res_")
	if f.released[taskID] {
		return ErrInvalidStateTransition
	}
	if f.captured[taskID] {
		return nil
	}
	if _, ok := f.reservations[taskID]; !ok {
		return ErrNotFound
	}
	f.captured[taskID] = true
	f.balance -= points
	return nil
}

func (f *fakePersonalLedger) Release(_ context.Context, _, _, reservationID string, _ int64, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	taskID := strings.TrimPrefix(reservationID, "res_")
	if f.captured[taskID] {
		return ErrInvalidStateTransition
	}
	if _, ok := f.reservations[taskID]; !ok {
		return ErrNotFound
	}
	f.released[taskID] = true
	return nil
}

type fakeAccountResolver struct{}

func (fakeAccountResolver) ResolvePersonalPointAccountID(_ context.Context, userID string) (string, error) {
	return "acct_" + userID, nil
}

func TestPersonalPointsLifecycleReserveCaptureRelease(t *testing.T) {
	ledger := newFakePersonalLedger(1000)
	points := NewPersonalPointsLifecycle(ledger, fakeAccountResolver{})
	access := Access{TenantID: "t1", UserID: "u1"}
	quote := EstimateRenderQuote(RenderQuoteInput{AspectRatio: "9:16", Resolution: "1080p", DurationMs: 15000, Voice: true}, points.now())

	txID, err := points.Reserve(context.Background(), access, "task_1", quote)
	if err != nil || txID == "" {
		t.Fatalf("reserve: %v tx=%s", err, txID)
	}
	again, err := points.Reserve(context.Background(), access, "task_1", quote)
	if err != nil || again != txID {
		t.Fatalf("idempotent reserve: %v %s", err, again)
	}
	if err := points.Capture(context.Background(), access, "task_1"); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if err := points.Capture(context.Background(), access, "task_1"); err != nil {
		t.Fatalf("idempotent capture: %v", err)
	}
	if err := points.Release(context.Background(), access, "task_1", "late"); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("release after capture = %v", err)
	}

	tx2, err := points.Reserve(context.Background(), access, "task_2", quote)
	if err != nil || tx2 == "" {
		t.Fatalf("reserve2: %v", err)
	}
	if err := points.Release(context.Background(), access, "task_2", "cancel"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := points.Release(context.Background(), access, "task_2", "cancel"); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
}

func TestPersonalPointsLifecycleInsufficient(t *testing.T) {
	ledger := newFakePersonalLedger(0)
	ledger.failInsufficient = true
	points := NewPersonalPointsLifecycle(ledger, fakeAccountResolver{})
	quote := EstimateRenderQuote(RenderQuoteInput{AspectRatio: "9:16", Resolution: "720p", DurationMs: 5000}, points.now())
	_, err := points.Reserve(context.Background(), Access{UserID: "u1"}, "task_x", quote)
	if !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("want insufficient, got %v", err)
	}
}
