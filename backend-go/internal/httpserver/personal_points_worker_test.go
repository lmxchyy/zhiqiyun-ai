package httpserver

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func TestPersonalPointExpiryWorkerRunOnceAndStopAreIdempotent(t *testing.T) {
	service := NewPersonalPointService(NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json")))
	if _, err := service.Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: "worker-account", UserID: "worker-user", Source: PointSourceAdminGift,
		Points: 9, IdempotencyKey: "worker-expiring", GrantedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	worker := newPersonalPointExpiryWorker(service, time.Hour, 10, slog.Default())
	result, err := worker.RunOnce(context.Background(), time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountsProcessed != 1 || result.PointsExpired != 9 {
		t.Fatalf("run result = %+v", result)
	}
	worker.Start()
	worker.Stop()
	worker.Stop()
	if result, err := worker.RunOnce(context.Background(), time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)); err != nil || result.AccountsProcessed != 0 || result.PointsExpired != 0 {
		t.Fatalf("idempotent run = %+v err=%v", result, err)
	}
}
