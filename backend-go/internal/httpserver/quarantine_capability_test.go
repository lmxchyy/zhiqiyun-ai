package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func TestQuarantineRequiredDependenciesFailClosed(t *testing.T) {
	ctx := context.Background()
	var db *sql.DB
	var tx *sql.Tx
	for name, err := range map[string]error{
		"db":               rejectQuarantinedGeneration(ctx, db, "fixture", "nil"),
		"tx":               rejectQuarantinedTaskTx(ctx, tx, "fixture", "nil"),
		"point tx":         rejectQuarantinedTaskTx(ctx, tx, "fixture", "billing_reserve"),
		"postgres adapter": (api{store: &postgresStore{}}).rejectQuarantinedGeneration(ctx, "fixture", "nil"),
		"absent adapter":   (api{}).rejectQuarantinedGeneration(ctx, "fixture", "nil"),
	} {
		if !errors.Is(err, pe.ErrQuarantineBarrierUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	var pg *postgresStore
	hooks := providerExecutionHooks(pg, true)
	_, err := hooks.Image(ctx, generation.CreateRequest{}, nil)
	if !errors.Is(err, pe.ErrQuarantineBarrierUnavailable) {
		t.Errorf("nil provider DB: %v", err)
	}
	json := newJSONStore(t.TempDir() + "/store.json")
	if err := (api{store: json}).rejectQuarantinedGeneration(ctx, "fixture", "json"); err != nil {
		t.Fatal(err)
	}
	if hook := providerExecutionHooks(json, true); hook.Image != nil || hook.Video != nil {
		t.Fatal("explicit JSON-only behavior changed")
	}
}

func TestQuarantineCapabilityMalformedNeverStarts(t *testing.T) {
	for _, args := range [][]string{{"--quarantine-capability-v1"}, {"--quarantine-capability-v1", "production", "allowed"}, {"--unknown"}, {"--quarantine-capability-v1", "00000000-0000-4000-8000-000000000000", "serve"}} {
		handled, err := DispatchQuarantineCapability(args, "api")
		if !handled || err == nil {
			t.Fatalf("malformed command fell through: %v", args)
		}
	}
	if handled, err := DispatchQuarantineCapability(nil, "api"); handled || err != nil {
		t.Fatal("normal no-argument startup changed")
	}
}
