package httpserver

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type issue190ConnectorProvider struct {
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (p *issue190ConnectorProvider) DefaultModel() string { return "gpt-image-2" }
func (p *issue190ConnectorProvider) Generate(ctx context.Context, _ generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls.Add(1)
	close(p.entered)
	select {
	case <-p.release:
		return issue190Images(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Real Connector authorization/reservation and provider-execution barriers,
// with an explicitly deposed same-generation caller. No production provider.
func TestIssue190PostgresConnector(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("ISSUE190_REQUIRE_POSTGRES") == "1" {
			t.Fatal("required isolated PostgreSQL DSN missing")
		}
		t.Skip("isolated PostgreSQL unavailable")
	}
	_ = issue190DB(t) // validate the explicitly isolated DSN before the enterprise fixture opens it
	t.Setenv("XIANZHI_P0_TEST_DATABASE_URL", dsn)
	p := &issue190ConnectorProvider{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	})
	generator, db, fixture, userID := connectorGenerationTestSubject(t, 100, p)
	service := generation.NewServiceWithOptions(generation.ServiceOptions{ImageProvider: p, ExecutionHooks: providerExecutionHooks(generator.store, true)})
	generator.connectorGenerationService = &service
	generator.cfg = config.Config{ProviderExecutionSafetyEnabled: true}
	storageProvider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	generator.fileService = storagecenter.NewService(storagecenter.NewMemoryRepository(), generatedStorageTestFactory{provider: storageProvider}, storagecenter.Options{DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "test", SecretKey: "test", Bucket: "test-private", DefaultQuotaBytes: 1 << 20, MaxUploadBytes: 1 << 20, MasterKey: "0123456789abcdef0123456789abcdef"})
	requestID := "feishu:" + fixture.prefix + "_issue190_owner"
	done := make(chan error, 1)
	go func() {
		_, _, err := generator.executeConnectorImageGeneration(context.Background(), userID, fixture.tenantIDs[0], connectorGenerationTestRequest(requestID))
		done <- err
	}()
	issue190Wait(t, p.entered)
	var taskID string
	if err := db.QueryRow(`SELECT id FROM xz_generation_tasks WHERE client_request_id=$1`, requestID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	// Other package tests reuse short task ids across separate go test processes.
	// Remove only this fixture's artifacts so a later isolated run cannot
	// misattribute an old artwork to this connector invocation.
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM xz_assets WHERE task_id=$1 AND tenant_id=$2`, taskID, fixture.tenantIDs[0])
		_, _ = db.Exec(`DELETE FROM provider_executions WHERE task_id=$1`, taskID)
		_, _ = db.Exec(`DELETE FROM xz_billing_lifecycle_events WHERE task_id=$1`, taskID)
		_, _ = db.Exec(`DELETE FROM xz_generation_tasks WHERE id=$1 AND client_request_id=$2`, taskID, requestID)
	})
	var g int64
	var oldOwner string
	if err := db.QueryRow(`SELECT execution_generation,worker_id FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&g, &oldOwner); err != nil {
		t.Fatal(err)
	}
	var bound int64
	if err := db.QueryRow(`SELECT task_execution_generation FROM provider_executions WHERE task_id=$1`, taskID).Scan(&bound); err != nil || bound != g {
		t.Fatalf("durable execution generation=%d claim=%d err=%v", bound, g, err)
	}
	expireFencingLease(t, db, taskID)
	if err := renewGenerationLease(generator.store, taskID, oldOwner, g); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("expired old owner renewed lease: %v", err)
	}
	next, newOwner, err := claimGenerationTaskOwnership(generator.store, taskID)
	if err != nil || next != g || newOwner == oldOwner {
		t.Fatalf("same-generation transfer gen=%d next=%d owner=%s err=%v", g, next, newOwner, err)
	}
	if err = renewGenerationLease(generator.store, taskID, oldOwner, g); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("deposed connector renewed: %v", err)
	}
	if err = renewGenerationLease(generator.store, taskID, newOwner, g); err != nil {
		t.Fatalf("current owner renewal failed: %v", err)
	}
	close(p.release)
	if err = issue190Result(t, done); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("deposed connector settled: %v", err)
	}
	var status, owner string
	var generationNow, reserved, captured, released int64
	var assets int
	if err = db.QueryRow(`SELECT status,coalesce(worker_id,''),execution_generation,reserved_points::bigint,captured_points::bigint,released_points::bigint,(SELECT count(*) FROM xz_assets WHERE task_id=$1) FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&status, &owner, &generationNow, &reserved, &captured, &released, &assets); err != nil {
		t.Fatal(err)
	}
	if status != "PROCESSING" || owner != newOwner || generationNow != g || reserved <= 0 || captured != 0 || released != 0 || assets != 0 || p.calls.Load() != 1 {
		t.Fatalf("owner=%s generation=%d status=%s reserve=%d capture=%d release=%d assets=%d generate=%d", owner, generationNow, status, reserved, captured, released, assets, p.calls.Load())
	}
	var executionStatus string
	if err = db.QueryRow(`SELECT status FROM provider_executions WHERE task_id=$1`, taskID).Scan(&executionStatus); err != nil || executionStatus != string(pe.Succeeded) {
		t.Fatalf("provider fact lost status=%s err=%v", executionStatus, err)
	}
	if err = renewGenerationLease(generator.store, taskID, newOwner, g); err != nil {
		t.Fatalf("new owner lease ended early: %v", err)
	}
	t.Logf("CONNECTOR owner=%s generation=%d lease=valid assets=%d reserved=%d captured=%d released=%d provider_calls=%d", owner, generationNow, assets, reserved, captured, released, p.calls.Load())
}
