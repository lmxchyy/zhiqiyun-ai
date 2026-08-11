package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestPersonalPointExpiryWorkerRuntimeConfigIsDefaultOffAndInvalidValuesFailClosed(t *testing.T) {
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED", "")
	if enabled, err := personalPointExpiryWorkerEnabled(); err != nil || enabled {
		t.Fatalf("default config enabled=%v err=%v", enabled, err)
	}
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED", "true")
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL", "not-a-duration")
	if _, _, err := personalPointExpiryWorkerOptions(); err == nil {
		t.Fatal("invalid interval was accepted")
	}
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL", "1m")
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH", "1001")
	if _, _, err := personalPointExpiryWorkerOptions(); err == nil {
		t.Fatal("out-of-range batch was accepted")
	}
}

func TestConfigurePersonalPointExpiryWorkerUsesActualServerLifecycle(t *testing.T) {
	server := &http.Server{}
	store := &postgresStore{}
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED", "")
	worker, err := configurePersonalPointExpiryWorker(server, store, slog.Default())
	if err != nil || worker != nil {
		t.Fatalf("default worker=%v err=%v", worker, err)
	}
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_ENABLED", "true")
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_INTERVAL", "1h")
	t.Setenv("XIANZHI_PERSONAL_POINT_EXPIRY_WORKER_BATCH", "25")
	worker, err = configurePersonalPointExpiryWorker(server, store, slog.Default())
	if err != nil || worker == nil || worker.interval != time.Hour || worker.batch != 25 {
		t.Fatalf("enabled worker=%+v err=%v", worker, err)
	}
	worker.Stop()
}

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
