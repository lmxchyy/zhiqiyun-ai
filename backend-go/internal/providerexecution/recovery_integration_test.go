package providerexecution

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

type integrationAdapter struct {
	creates atomic.Int32
	gets    atomic.Int32
	result  QueryResult
}

func (a *integrationAdapter) Submit(context.Context) (Submission, error) {
	a.creates.Add(1)
	return Submission{ProviderRequestID: "provider-1"}, nil
}
func (a *integrationAdapter) Query(context.Context, string) (QueryResult, error) {
	a.gets.Add(1)
	return a.result, nil
}

func TestPostgresRecoveryOrchestration(t *testing.T) {
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL is not configured")
	}
	db := openProviderExecutionTestDB(t, dsn)
	defer db.Close()
	s := NewStore(db)
	ctx := context.Background()
	prefix := "recovery-" + time.Now().UTC().Format("20060102150405.000000000")
	// Case A: prepared is claimed and submitted exactly once on the same attempt.
	a := &integrationAdapter{}
	e, err := (&Service{Store: s}).Execute(ctx, Execution{TaskID: prefix + "-a", Provider: "mock", Capability: "video", RequestFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, a)
	if err != nil || e.Attempt != 1 || a.creates.Load() != 1 {
		t.Fatalf("case A: %+v creates=%d err=%v", e, a.creates.Load(), err)
	}
	// Case B: submitting without a request id becomes unknown and never submits.
	b, err := s.CreatePrepared(ctx, Execution{TaskID: prefix + "-b", Provider: "mock", Capability: "image", RequestFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.ClaimPrepared(ctx, b.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	b, err = (&Service{Store: s}).Recover(ctx, b, &integrationAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Status != Unknown {
		t.Fatalf("case B status=%s", b.Status)
	}
	// Case C: persisted provider id recovers with Get only.
	c, err := s.CreatePrepared(ctx, Execution{TaskID: prefix + "-c", Provider: "mock", Capability: "video", RequestFingerprint: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.ClaimPrepared(ctx, c.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, c.ID, Submitted, ptr("provider-c"), nil, nil); err != nil {
		t.Fatal(err)
	}
	c, err = s.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	ca := &integrationAdapter{result: QueryResult{Status: Succeeded, ProviderRequestID: "provider-c"}}
	c, err = (&Service{Store: s}).Recover(ctx, c, ca)
	if err != nil || c.Status != Succeeded || ca.creates.Load() != 0 || ca.gets.Load() != 1 {
		t.Fatalf("case C: %+v create=%d get=%d err=%v", c, ca.creates.Load(), ca.gets.Load(), err)
	}
	// Case D: succeeded local recovery queries an async provider and never creates.
	d, err := s.CreatePrepared(ctx, Execution{TaskID: prefix + "-d", Provider: "mock", Capability: "video", RequestFingerprint: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"})
	if err != nil {
		t.Fatal(err)
	}
	d, err = s.ClaimPrepared(ctx, d.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, d.ID, Submitted, ptr("provider-d"), nil, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.Transition(ctx, d.ID, Succeeded, nil, ptr(string(ProviderSucceeded)), nil); err != nil {
		t.Fatal(err)
	}
	d, err = s.GetByID(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	da := &integrationAdapter{result: QueryResult{Status: Succeeded, ProviderRequestID: "provider-d"}}
	_, err = (&Service{Store: s}).Recover(ctx, d, da)
	if err != nil || da.creates.Load() != 0 || da.gets.Load() != 1 {
		t.Fatalf("case D create=%d get=%d err=%v", da.creates.Load(), da.gets.Load(), err)
	}
}
