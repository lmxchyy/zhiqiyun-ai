package httpserver

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitableShutdownHookStopsExactlyOnce(t *testing.T) {
	server := &http.Server{}
	var calls atomic.Int32
	registerWaitableShutdownHook(server, func() { calls.Add(1) })
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitForShutdownHooks(ctx, server); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("shutdown hook calls=%d", calls.Load())
	}
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate shutdown reran hook calls=%d", calls.Load())
	}
}
