package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

// These are genuine PostgreSQL integration tests. No sqlmock, SQLite or sleeps.
func issue190DB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("ISSUE190_REQUIRE_POSTGRES") == "1" {
			t.Fatal("required isolated PostgreSQL DSN missing")
		}
		t.Skip("isolated PostgreSQL unavailable; release BLOCKED")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost" && parsed.Hostname() != "::1") || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("Issue190 requires a loopback, explicitly named test database; refusing unknown/production DSN")
	}
	return openFencingTestDB(t)
}

type issue190Provider struct {
	calls   atomic.Int32
	gets    atomic.Int32
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *issue190Provider) DefaultModel() string { return "gpt-image-2" }
func issue190Images() []generation.GeneratedImage {
	return []generation.GeneratedImage{{URL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aB9sAAAAASUVORK5CYII=", ContentType: "image/png", Width: 1, Height: 1}}
}
func (p *issue190Provider) Generate(ctx context.Context, _ generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls.Add(1)
	if p.entered != nil {
		p.once.Do(func() { close(p.entered) })
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return issue190Images(), nil
}
func (p *issue190Provider) Get(context.Context, string) (any, error) {
	p.gets.Add(1)
	return map[string]any{"status": "processing"}, nil
}
func issue190Wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("barrier timed out")
	}
}
func issue190Result(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("worker timed out")
		return nil
	}
}

// Use an admitted non-sentinel model. For routing consumers, the real channel
// factory targets this loopback fake, never a seeded external provider.
func issue190ProviderFixture(t *testing.T, db *sql.DB, userID string, blocking bool) (*issue190Provider, string) {
	t.Helper()
	p := &issue190Provider{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		images, err := p.Generate(r.Context(), generation.CreateRequest{})
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": strings.SplitN(images[0].URL, ",", 2)[1]}}})
	}))
	t.Cleanup(server.Close)
	if blocking {
		p.entered = make(chan struct{})
		p.release = make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-p.release:
			default:
				close(p.release)
			}
		})
	}
	id := "channel_" + userID
	keyID := "key_" + userID
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	channel := adminAPIChannel{ID: id, Name: id, BaseURL: server.URL, Protocol: "openai", Status: "ACTIVE", Models: []string{p.DefaultModel()}, ImageGenerationEndpoint: "/v1/images/generations"}
	if err = insertAPIChannel(context.Background(), tx, channel); err != nil {
		t.Fatal(err)
	}
	if err = insertAPIKey(context.Background(), tx, adminAPIKey{ID: keyID, Customer: id, Secret: "issue190-isolated-fake-key", Status: "ACTIVE"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM xz_api_keys WHERE id=$1`, keyID)
		_, _ = db.Exec(`DELETE FROM xz_api_channels WHERE id=$1`, id)
	})
	return p, id
}

// Exercise actual confirmed publication and manual-ack RabbitMQ delivery. A
// prior wake-up can be received before its worker crashes; DB publication facts
// must then survive scheduler recovery, unlike untouched pending replacements.
func issue190PublishOutbox(t *testing.T, f *issue190Fixture, e *messaging.Envelope) *messaging.Envelope {
	t.Helper()
	rawURL := os.Getenv("XIANZHI_MESSAGING_TEST_RABBITMQ_URL")
	if rawURL == "" {
		t.Fatal("real isolated RabbitMQ required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	manager, err := messaging.NewConnectionManager(messaging.RabbitMQConfig{URL: rawURL})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start()
	defer manager.Close()
	if _, err = manager.WaitForConnection(ctx); err != nil {
		t.Fatal(err)
	}
	publisher := messaging.NewPublisher(manager)
	if err = publisher.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	ch, err := manager.OpenChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	queue := messaging.GenerationImageNormalQueue
	if e.EventType == messaging.GenerationCanaryRoutingKey {
		queue = messaging.GenerationCanaryQueue
	}
	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	outbox := messaging.NewOutboxStore(f.db)
	var outboxID int64
	if err = f.db.QueryRowContext(ctx, `UPDATE outbox_events SET status='publishing',claim_owner='issue190-publisher',claimed_at=now(),updated_at=now() WHERE event_id=$1 AND aggregate_id=$2 AND status='pending' AND next_attempt_at<=now() AND claimed_at IS NULL RETURNING id`, e.EventID, f.task.ID).Scan(&outboxID); err != nil {
		t.Fatalf("targeted outbox CAS claim: %v", err)
	}
	if err = publisher.Publish(ctx, e, e.EventType); err != nil {
		t.Fatal(err)
	}
	if err = outbox.MarkPublished(ctx, outboxID); err != nil {
		t.Fatal(err)
	}
	select {
	case delivery := <-deliveries:
		actual, err := messaging.DecodeEnvelope(delivery.Body)
		if err != nil || actual.EventID != e.EventID || delivery.RoutingKey != e.EventType {
			t.Fatalf("broker route/event mismatch got=%v err=%v", actual, err)
		}
		if err = delivery.Ack(false); err != nil {
			t.Fatal(err)
		}
		t.Logf("RABBITMQ dispatch queue=%s event=%s confirmed=1 received=1 ack=1", queue, e.EventID)
		return actual
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return nil
	}
}

type issue190Fixture struct {
	db        *sql.DB
	store     *postgresStore
	a         *api
	task      generationTask
	provider  *issue190Provider
	scheduler *GenerationScheduler
	req       generation.CreateRequest
}

func issue190Setup(t *testing.T, db *sql.DB, mode string, blocking bool) *issue190Fixture {
	t.Helper()
	store := fencingStore(db)
	userID, _ := setupPaidTestUser(t, db, "issue190", 1000)
	p, channelID := issue190ProviderFixture(t, db, userID, blocking)
	planID := "plan_" + userID
	// Real isolated pricing, even when migrations publish only paid models.
	if _, err := db.Exec(`INSERT INTO xz_billing_rule_versions(id,rule_key,model_name,model_code,module_code,billing_unit,base_price_points,version,status) VALUES($1,$1,'gpt-image-2','gpt-image-2',$2,'PER_IMAGE',1,1,'PUBLISHED')`, planID, moduleImageGeneration); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM xz_billing_rule_versions WHERE id=$1`, planID) })
	if _, err := db.Exec(`INSERT INTO xz_plans(id,code,name,concurrency,active) VALUES($1,$1,'Issue190 isolated test',2,true)`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE xz_users SET plan_id=$2,status='ACTIVE',subscription_expires_at=$3 WHERE id=$1`, userID, planID, time.Now().Add(time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	req := generation.CreateRequest{ClientRequestID: "request_" + userID, UserID: userID, Type: "TEXT_TO_IMAGE", Model: p.DefaultModel(), Prompt: "Issue190 test image", FairScheduler: true, Params: map[string]any{imageDispatchModeParam: mode, "provider": channelID}}
	var task generationTask
	var err error
	if mode == imageDispatchCanary {
		task, err = store.CreatePendingGenerationTaskWithCanaryOutbox(req)
	} else {
		task, err = store.CreatePendingGenerationTask(req)
	}
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskStatus != taskStatusQueued || task.PersonalPointReservationID == "" || task.PointCost <= 0 {
		t.Fatalf("not a real funded queued image: %+v", task)
	}
	service := generation.NewServiceWithOptions(generation.ServiceOptions{ImageProvider: p, ExecutionHooks: providerExecutionHooks(store, true)})
	storageProvider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	fs := storagecenter.NewService(storagecenter.NewMemoryRepository(), generatedStorageTestFactory{provider: storageProvider}, storagecenter.Options{DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "test", SecretKey: "test", Bucket: "test-private", DefaultQuotaBytes: 1 << 20, MaxUploadBytes: 1 << 20, MasterKey: "0123456789abcdef0123456789abcdef"})
	a := &api{store: store, generationService: service, fileService: fs, cfg: config.Config{ProviderExecutionSafetyEnabled: true}}
	req.Params = cloneAnyMap(task.Params)
	f := &issue190Fixture{db: db, store: store, a: a, task: task, provider: p, req: req, scheduler: NewGenerationScheduler(db, GenerationSchedulerOptions{Owner: "issue190-scheduler", BatchTasksPerUser: 2})}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM consumer_inbox WHERE event_id IN (SELECT event_id FROM outbox_events WHERE aggregate_id=$1) OR event_id LIKE $2`, task.ID, "issue190:"+task.ID+":%")
		for _, table := range []string{"xz_assets", "xz_billing_lifecycle_events", "provider_executions"} {
			_, _ = db.Exec(`DELETE FROM `+table+` WHERE task_id=$1`, task.ID)
		}
		_, _ = db.Exec(`DELETE FROM outbox_events WHERE aggregate_id=$1`, task.ID)
		_, _ = db.Exec(`DELETE FROM xz_generation_tasks WHERE id=$1`, task.ID)
		_, _ = db.Exec(`UPDATE xz_users SET plan_id=NULL WHERE id=$1`, userID)
		_, _ = db.Exec(`DELETE FROM xz_plans WHERE id=$1`, planID)
	})
	return f
}
func (f *issue190Fixture) taskState(t *testing.T) (int64, string, string, bool) {
	t.Helper()
	var g int64
	var state, owner string
	var live bool
	if err := f.db.QueryRow(`SELECT execution_generation,task_status,coalesce(worker_id,''),coalesce(lease_until>now(),false) FROM xz_generation_tasks WHERE id=$1`, f.task.ID).Scan(&g, &state, &owner, &live); err != nil {
		t.Fatal(err)
	}
	return g, state, owner, live
}
func (f *issue190Fixture) assertEffects(t *testing.T, complete bool, outbox, inbox int) {
	t.Helper()
	var assets, results, events, processed int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM xz_assets WHERE task_id=$1),jsonb_array_length(result_ids),(SELECT count(*) FROM outbox_events WHERE aggregate_id=$1),(SELECT count(*) FROM consumer_inbox WHERE processed_at IS NOT NULL AND (event_id IN (SELECT event_id FROM outbox_events WHERE aggregate_id=$1) OR event_id LIKE $2)) FROM xz_generation_tasks WHERE id=$1`, f.task.ID, "issue190:"+f.task.ID+":%").Scan(&assets, &results, &events, &processed); err != nil {
		t.Fatal(err)
	}
	_, reserved, captured, released := getReservationState(t, f.db, f.task.PersonalPointReservationID)
	captures := countLedgerMovements(t, f.db, f.task.PersonalPointAccountID, f.task.PersonalPointReservationID, "CAPTURE")
	releases := countLedgerMovements(t, f.db, f.task.PersonalPointAccountID, f.task.PersonalPointReservationID, "RELEASE")
	_, frozen := getPointAccountBalances(t, f.db, f.task.PersonalPointAccountID, f.task.UserID)
	want := 0
	if complete {
		want = 1
	}
	if assets != want || results != want || captures != want || releases != 0 || events != outbox || processed != inbox || released != 0 || reserved+captured != int64(f.task.PointCost) {
		t.Fatalf("assets/artwork=%d results=%d capture=%d release=%d outbox=%d inbox=%d reserved=%d captured=%d released=%d", assets, results, captures, releases, events, processed, reserved, captured, released)
	}
	if complete {
		if captured != int64(f.task.PointCost) || reserved != 0 || frozen != 0 {
			t.Fatalf("completed billing captured=%d reserved=%d frozen=%d", captured, reserved, frozen)
		}
	} else if captured != 0 || frozen != reserved {
		t.Fatalf("reservation changed captured=%d frozen=%d reserved=%d", captured, frozen, reserved)
	}
	// xz_assets are the artwork records; result_ids must reference those same rows.
	if complete {
		var missing int
		if err := f.db.QueryRow(`SELECT count(*) FROM xz_generation_tasks t,jsonb_array_elements_text(t.result_ids) r(id) WHERE t.id=$1 AND NOT EXISTS(SELECT 1 FROM xz_assets a WHERE a.id=r.id AND a.task_id=t.id)`, f.task.ID).Scan(&missing); err != nil || missing != 0 {
			t.Fatalf("artwork association missing=%d err=%v", missing, err)
		}
	}
	generation, taskStatus, owner, leaseValid := f.taskState(t)
	reserves := countLedgerMovements(t, f.db, f.task.PersonalPointAccountID, f.task.PersonalPointReservationID, "RESERVE")
	t.Logf("ISSUE190_EFFECTS task=%s generation=%d task_status=%s owner=%s lease_valid=%t provider_generate=%d provider_get=%d assets=%d artworks=%d outbox=%d inbox=%d reserve_events=%d capture_events=%d release_events=%d reserved_remaining=%d captured=%d released=%d", f.task.ID, generation, taskStatus, owner, leaseValid, f.provider.calls.Load(), f.provider.gets.Load(), assets, results, events, processed, reserves, captures, releases, reserved, captured, released)
}
func (f *issue190Fixture) dispatch(t *testing.T) *messaging.Envelope {
	t.Helper()
	n, err := f.scheduler.dispatchUserTx(context.Background(), f.task.UserID, time.Now().UTC())
	if err != nil || n != 1 {
		t.Fatalf("dispatch n=%d err=%v", n, err)
	}
	var raw []byte
	if err := f.db.QueryRow(`SELECT payload FROM outbox_events WHERE aggregate_id=$1 ORDER BY created_at DESC LIMIT 1`, f.task.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	e, err := messaging.DecodeOutboxEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func (f *issue190Fixture) consume(ctx context.Context, e *messaging.Envelope) error {
	inbox := messaging.NewInboxStore(f.db)
	if canaryTaskMarker(f.task.Params) {
		return f.a.processGenerationCanaryMessage(ctx, inbox, e)
	}
	return f.a.processGenerationNormalMessage(ctx, inbox, e)
}

func TestIssue190PostgresA(t *testing.T) {
	db := issue190DB(t)
	f := issue190Setup(t, db, imageDispatchNormal, true)
	done := make(chan error, 1)
	go func() { done <- f.a.runGenerationTask(f.task.ID, f.a.generationService, f.req) }()
	issue190Wait(t, f.provider.entered)
	g, state, owner, live := f.taskState(t)
	if g != 2 || state != "RUNNING" || owner == "" || !live {
		t.Fatalf("non-atomic claim g=%d state=%s owner=%s live=%v", g, state, owner, live)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SELECT id FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, f.task.ID); err != nil {
		t.Fatal(err)
	}
	n, err := f.scheduler.dispatchUserTx(context.Background(), f.task.UserID, time.Now().UTC())
	_ = tx.Rollback()
	if err != nil || n != 0 {
		t.Fatalf("locked task stolen: n=%d err=%v", n, err)
	}
	// Legacy inconsistent QUEUED+live lease is also excluded by actual scheduler SQL.
	if _, err = db.Exec(`UPDATE xz_generation_tasks SET task_status='QUEUED' WHERE id=$1`, f.task.ID); err != nil {
		t.Fatal(err)
	}
	n, err = f.scheduler.dispatchUserTx(context.Background(), f.task.UserID, time.Now().UTC())
	if err != nil || n != 0 {
		t.Fatalf("live queued lease stolen: n=%d err=%v", n, err)
	}
	_, _ = db.Exec(`UPDATE xz_generation_tasks SET task_status='RUNNING' WHERE id=$1`, f.task.ID)
	close(f.provider.release)
	if err = issue190Result(t, done); err != nil {
		t.Fatal(err)
	}
	g, state, gotOwner, _ := f.taskState(t)
	if g != 2 || state != taskStatusSucceeded || gotOwner != owner || f.provider.calls.Load() != 1 {
		t.Fatalf("claim drift g=%d state=%s owner=%s calls=%d", g, state, gotOwner, f.provider.calls.Load())
	}
	f.assertEffects(t, true, 0, 0)
}
func TestIssue190PostgresB(t *testing.T) {
	db := issue190DB(t)
	f := issue190Setup(t, db, imageDispatchNormal, false)
	start := make(chan struct{})
	counts := make(chan int, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		s := NewGenerationScheduler(db, GenerationSchedulerOptions{Owner: fmt.Sprintf("issue190-scheduler-%d", i)})
		go func() {
			<-start
			n, err := s.dispatchUserTx(context.Background(), f.task.UserID, time.Now().UTC())
			counts <- n
			errs <- err
		}()
	}
	close(start)
	n := <-counts + <-counts
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("dispatch winners=%d", n)
	}
	g, state, owner, live := f.taskState(t)
	if g != 2 || state != "DISPATCHING" || owner == "" || !live {
		t.Fatalf("invalid dispatch g=%d state=%s owner=%s live=%t", g, state, owner, live)
	}
	f.assertEffects(t, false, 1, 0)
	var raw []byte
	if err := db.QueryRow(`SELECT payload FROM outbox_events WHERE aggregate_id=$1`, f.task.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	e, err := messaging.DecodeOutboxEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if e.EventType != messaging.GenerationImageNormalRoutingKey {
		t.Fatalf("ordinary image misrouted %s", e.EventType)
	}
	if err = f.consume(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	g, state, _, _ = f.taskState(t)
	if g != 3 || state != taskStatusSucceeded || f.provider.calls.Load() != 1 {
		t.Fatalf("handoff g=%d state=%s calls=%d", g, state, f.provider.calls.Load())
	}
	f.assertEffects(t, true, 1, 1)
}
func TestIssue190PostgresC(t *testing.T) {
	db := issue190DB(t)
	for _, mode := range []string{imageDispatchNormal, imageDispatchCanary} {
		t.Run(mode, func(t *testing.T) {
			f := issue190Setup(t, db, mode, true)
			e := f.dispatch(t)
			done := make(chan error, 1)
			go func() { done <- f.consume(context.Background(), e) }()
			issue190Wait(t, f.provider.entered)
			g, state, owner, live := f.taskState(t)
			if state != "RUNNING" || !live {
				t.Fatal("first consumer has no ownership")
			}
			if err := f.consume(context.Background(), e); !errors.Is(err, errGenerationOwnershipBusy) {
				t.Fatalf("same unfinished event err=%v", err)
			}
			other := *e
			other.EventID = "issue190:" + f.task.ID + ":duplicate"
			other.Data = map[string]any{"task_id": f.task.ID, "dispatch_mode": mode} // legacy no-generation retry (canary), valid gen for normal
			if mode == imageDispatchNormal {
				other.Data["execution_generation"] = g
			}
			if err := f.consume(context.Background(), &other); !errors.Is(err, errGenerationOwnershipBusy) {
				t.Fatalf("different unfinished event err=%v", err)
			}
			after, _, currentOwner, _ := f.taskState(t)
			if after != g || currentOwner != owner || f.provider.calls.Load() != 1 {
				t.Fatal("duplicate changed live ownership")
			}
			f.assertEffects(t, false, 1, 0)
			close(f.provider.release)
			if err := issue190Result(t, done); err != nil {
				t.Fatal(err)
			}
			if err := f.consume(context.Background(), e); err != nil {
				t.Fatal(err)
			}
			if err := f.consume(context.Background(), &other); err != nil {
				t.Fatal(err)
			}
			if f.provider.calls.Load() != 1 {
				t.Fatal("duplicate provider invocation")
			}
			f.assertEffects(t, true, 1, 2)
		})
	}
}
func TestIssue190PostgresD(t *testing.T) {
	db := issue190DB(t)
	for _, stage := range []string{"claim", "prepared", "submitting", "succeeded", "unknown_with_id", "submitted_with_id", "processing_with_id"} {
		t.Run(stage, func(t *testing.T) {
			f := issue190Setup(t, db, imageDispatchNormal, false)
			g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
			if err != nil {
				t.Fatal(err)
			}
			original := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
			pstore := pe.NewStore(db)
			req := f.req
			req.Params = cloneAnyMap(f.req.Params)
			req.Params[providerExecutionTaskParam] = f.task.ID
			if stage != "claim" {
				identity, _, err := executionIdentity(req, "image", providerName(req))
				if err != nil {
					t.Fatal(err)
				}
				e, err := pstore.CreatePreparedForGenerationTask(original, identity)
				if err != nil {
					t.Fatal(err)
				}
				if stage != "prepared" {
					e, err = pstore.ClaimPreparedForGenerationTask(original, f.task.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				if strings.HasSuffix(stage, "_with_id") {
					id := "known-provider-request"
					state := pe.Unknown
					if stage != "unknown_with_id" {
						state = pe.Submitted
					}
					if err = pstore.Transition(original, e.ID, state, &id, nil, nil); err != nil {
						t.Fatal(err)
					}
					if stage == "processing_with_id" {
						if err = pstore.Transition(original, e.ID, pe.Processing, &id, nil, nil); err != nil {
							t.Fatal(err)
						}
					}
				}
				if stage == "succeeded" {
					raw, _ := json.Marshal(issue190Images())
					if err = pstore.SaveSucceededResult(original, e.ID, nil, raw); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, _, err = claimGenerationTaskOwnership(f.store, f.task.ID); !errors.Is(err, errGenerationOwnershipBusy) {
				t.Fatalf("live owner stolen stage=%s err=%v", stage, err)
			}
			f.assertEffects(t, false, 0, 0)
			expireFencingLease(t, db, f.task.ID)
			if stage == "claim" {
				_, _ = db.Exec(`UPDATE xz_generation_tasks SET updated_at=$2 WHERE id=$1`, f.task.ID, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano))
				f.a.repairStaleGenerationTasksWithContext(context.Background(), time.Second)
				_, _, captured, released := getReservationState(t, db, f.task.PersonalPointReservationID)
				if captured != 0 || released != int64(f.task.PointCost) {
					t.Fatalf("pre-provider crash not safely released capture=%d release=%d", captured, released)
				}
				if countLedgerMovements(t, db, f.task.PersonalPointAccountID, f.task.PersonalPointReservationID, "RELEASE") != 1 {
					t.Fatal("release not exactly once")
				}
				f.a.repairStaleGenerationTasksWithContext(context.Background(), time.Second)
				if countLedgerMovements(t, db, f.task.PersonalPointAccountID, f.task.PersonalPointReservationID, "RELEASE") != 1 {
					t.Fatal("repeated crash release")
				}
				return
			}
			resumed, resumedOwner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if resumed != g || resumedOwner == owner {
				t.Fatal("recovery changed execution identity or reused owner")
			}
			if err = pstore.ValidateGenerationOwnership(original, f.task.ID); !errors.Is(err, ErrFencedStaleExecution) {
				t.Fatalf("old caller could submit: %v", err)
			}
			_, oldErr := guardedImage(original, req, f.provider, pstore)
			if stage != "submitting" && !errors.Is(oldErr, ErrFencedStaleExecution) {
				t.Fatalf("deposed provider caller not fenced stage=%s err=%v", stage, oldErr)
			}
			if f.provider.calls.Load() != 0 {
				t.Fatal("deposed owner invoked provider")
			}
			if _, err = f.store.completeGenerationTaskOwned(f.task.ID, req, g, owner); !errors.Is(err, ErrFencedStaleExecution) {
				t.Fatalf("old owner could settle same generation: %v", err)
			}
			if _, err = f.store.failGenerationTaskOwned(f.task.ID, "deposed failure", g, owner); !errors.Is(err, ErrFencedStaleExecution) {
				t.Fatalf("old owner could release: %v", err)
			}
			if _, err = f.store.failGenerationTaskDurable(f.task.ID, "deposed deadline", nil, g, owner); !errors.Is(err, ErrFencedStaleExecution) {
				t.Fatalf("old owner deadline could release: %v", err)
			}
			current := pe.WithGenerationOwnership(context.Background(), f.task.ID, resumedOwner, resumed)
			images, err := guardedImage(current, req, f.provider, pstore)
			if stage == "submitting" {
				if !errors.Is(err, pe.ErrUnknownResubmitBlocked) || f.provider.calls.Load() != 0 {
					t.Fatalf("ambiguous crash resubmitted err=%v calls=%d", err, f.provider.calls.Load())
				}
				f.assertEffects(t, false, 0, 0)
				return
			}
			if strings.HasSuffix(stage, "_with_id") {
				if !errors.Is(err, pe.ErrProviderStillProcessing) || f.provider.gets.Load() != 1 || f.provider.calls.Load() != 0 {
					t.Fatalf("GET must defer until durable success err=%v gets=%d generate=%d", err, f.provider.gets.Load(), f.provider.calls.Load())
				}
				f.assertEffects(t, false, 0, 0)
				var e pe.Execution
				e, err = pstore.GetLatestByTask(context.Background(), f.task.ID)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(issue190Images())
				if err = pstore.SaveSucceededResult(current, e.ID, e.ProviderRequestID, raw); err != nil {
					t.Fatal(err)
				}
				latest, readErr := pstore.GetLatestByTask(current, f.task.ID)
				if readErr != nil || latest.Status != pe.Succeeded {
					t.Fatalf("durable success not saved status=%s err=%v", latest.Status, readErr)
				}
				images, err = guardedImage(current, req, f.provider, pstore)
			}
			if err != nil || len(images) != 1 {
				t.Fatalf("same-generation recovery err=%v images=%d", err, len(images))
			}
			expectedCalls := int32(1)
			if stage == "succeeded" || strings.HasSuffix(stage, "_with_id") {
				expectedCalls = 0
			}
			if f.provider.calls.Load() != expectedCalls {
				t.Fatalf("calls=%d", f.provider.calls.Load())
			}
			if strings.HasSuffix(stage, "_with_id") && f.provider.gets.Load() != 1 {
				t.Fatalf("GET-only recovery gets=%d", f.provider.gets.Load())
			}
			prepared := req
			prepared.GeneratedImages = images
			prepared, _, err = f.a.persistGeneratedImages(current, f.task.ID, prepared)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.completeGenerationTaskOwned(f.task.ID, prepared, resumed, resumedOwner); err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.completeGenerationTaskOwned(f.task.ID, prepared, resumed, resumedOwner); err != nil {
				t.Fatal(err)
			}
			f.assertEffects(t, true, 0, 0)
		})
	}
	t.Run("watchdog_success_probe_owner_transfer", func(t *testing.T) {
		f := issue190Setup(t, db, imageDispatchNormal, false)
		g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		req := f.req
		req.Params = cloneAnyMap(req.Params)
		req.Params[providerExecutionTaskParam] = f.task.ID
		ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
		identity, _, err := executionIdentity(req, "image", providerName(req))
		if err != nil {
			t.Fatal(err)
		}
		pstore := pe.NewStore(db)
		e, err := pstore.CreatePreparedForGenerationTask(ctx, identity)
		if err != nil {
			t.Fatal(err)
		}
		e, err = pstore.ClaimPreparedForGenerationTask(ctx, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(issue190Images())
		if err = pstore.SaveSucceededResult(ctx, e.ID, nil, raw); err != nil {
			t.Fatal(err)
		}
		e, err = pstore.GetLatestByTask(ctx, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		expireFencingLease(t, db, f.task.ID)
		observed, err := getTaskFencing(context.Background(), db, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		next, newOwner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
		if err != nil || next != g || newOwner == owner {
			t.Fatalf("same-generation owner transfer g=%d next=%d err=%v", g, next, err)
		}
		if err = f.a.recoverSucceededGenerationTask(f.task, e, observed); !errors.Is(err, ErrFencedStaleExecution) {
			t.Fatalf("stale watchdog adopted new owner: %v", err)
		}
		if err = renewGenerationLease(f.store, f.task.ID, owner, g); !errors.Is(err, ErrFencedStaleExecution) {
			t.Fatalf("old heartbeat renewed: %v", err)
		}
		if err = renewGenerationLease(f.store, f.task.ID, newOwner, g); err != nil {
			t.Fatalf("new heartbeat rejected: %v", err)
		}
		current, _, actual, live := f.taskState(t)
		if current != g || actual != newOwner || !live {
			t.Fatal("watchdog disturbed current owner")
		}
		f.assertEffects(t, false, 0, 0)
	})
	t.Run("watchdog_unknown_grace_probe_owner_transfer", func(t *testing.T) {
		f := issue190Setup(t, db, imageDispatchNormal, false)
		g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		req := f.req
		req.Params = cloneAnyMap(req.Params)
		req.Params[providerExecutionTaskParam] = f.task.ID
		ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
		identity, _, err := executionIdentity(req, "image", providerName(req))
		if err != nil {
			t.Fatal(err)
		}
		pstore := pe.NewStore(db)
		e, err := pstore.CreatePreparedForGenerationTask(ctx, identity)
		if err != nil {
			t.Fatal(err)
		}
		e, err = pstore.ClaimPreparedForGenerationTask(ctx, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = pstore.MarkUnknown(ctx, e.ID, pe.ProviderUnknown, "ambiguous crash"); err != nil {
			t.Fatal(err)
		}
		expireFencingLease(t, db, f.task.ID)
		observed, err := getTaskFencing(context.Background(), db, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		next, newOwner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
		if err != nil || next != g || newOwner == owner {
			t.Fatalf("same-generation owner transfer g=%d next=%d err=%v", g, next, err)
		}
		grace := time.Nanosecond
		if _, err = failGenerationTaskForWatchdog(f.store, f.task, "old watchdog grace", &grace, observed); !errors.Is(err, ErrFencedStaleExecution) {
			t.Fatalf("old watchdog released new owner: %v", err)
		}
		current, _, actual, live := f.taskState(t)
		if current != g || actual != newOwner || !live {
			t.Fatal("watchdog disturbed current owner")
		}
		f.assertEffects(t, false, 0, 0)
	})
	t.Run("success_reconcile_distinct_wakeups", func(t *testing.T) {
		f := issue190Setup(t, db, imageDispatchNormal, false)
		g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		req := f.req
		req.Params = cloneAnyMap(req.Params)
		req.Params[providerExecutionTaskParam] = f.task.ID
		ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
		identity, _, err := executionIdentity(req, "image", providerName(req))
		if err != nil {
			t.Fatal(err)
		}
		pstore := pe.NewStore(db)
		e, err := pstore.CreatePreparedForGenerationTask(ctx, identity)
		if err != nil {
			t.Fatal(err)
		}
		e, err = pstore.ClaimPreparedForGenerationTask(ctx, f.task.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(issue190Images())
		if err = pstore.SaveSucceededResult(ctx, e.ID, nil, raw); err != nil {
			t.Fatal(err)
		}
		expireFencingLease(t, db, f.task.ID)
		if n, err := f.scheduler.RecoverStaleDispatches(context.Background()); err != nil || n != 1 {
			t.Fatalf("first recovery n=%d err=%v", n, err)
		}
		first := f.dispatch(t)
		issue190PublishOutbox(t, f, first)
		expireFencingLease(t, db, f.task.ID)
		if n, err := f.scheduler.RecoverStaleDispatches(context.Background()); err != nil || n != 1 {
			t.Fatalf("second recovery n=%d err=%v", n, err)
		}
		second := f.dispatch(t)
		issue190PublishOutbox(t, f, second)
		if first.EventID == second.EventID || envelopeExecutionGeneration(second.Data) != g {
			t.Fatal("GET recovery collapsed wakeup or changed binding")
		}
		expireFencingLease(t, db, f.task.ID)
		if n, err := f.scheduler.RecoverStaleDispatches(context.Background()); err != nil || n != 1 {
			t.Fatalf("third recovery duplicated joined row n=%d err=%v", n, err)
		}
		third := f.dispatch(t)
		if third.EventID == first.EventID || third.EventID == second.EventID || envelopeExecutionGeneration(third.Data) != g {
			t.Fatal("third recovery collapsed wakeup or changed binding")
		}
		third = issue190PublishOutbox(t, f, third)
		if err = f.consume(context.Background(), third); err != nil {
			t.Fatal(err)
		}
		if f.provider.calls.Load() != 0 || f.provider.gets.Load() != 0 {
			t.Fatalf("local success recovery invoked provider generate=%d get=%d", f.provider.calls.Load(), f.provider.gets.Load())
		}
		f.assertEffects(t, true, 3, 1)
	})
}
func TestIssue190PostgresE(t *testing.T) {
	db := issue190DB(t)
	f := issue190Setup(t, db, imageDispatchNormal, false)
	g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := f.req
	req.Params = cloneAnyMap(req.Params)
	req.Params[providerExecutionTaskParam] = f.task.ID
	ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
	identity, _, err := executionIdentity(req, "image", providerName(req))
	if err != nil {
		t.Fatal(err)
	}
	pstore := pe.NewStore(db)
	e, err := pstore.CreatePreparedForGenerationTask(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	e, err = pstore.ClaimPreparedForGenerationTask(ctx, f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(issue190Images())
	if err = pstore.SaveSucceededResult(ctx, e.ID, nil, raw); err != nil {
		t.Fatal(err)
	}
	expireFencingLease(t, db, f.task.ID)
	next := requeueAsReaper(t, db, f.task.ID, g)
	if next != g+1 {
		t.Fatal("replacement generation did not advance")
	}
	e, err = pstore.GetByID(context.Background(), e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.recoverSucceededGenerationTask(f.task, e); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("old local recovery accepted: %v", err)
	}
	if _, err = guardedImage(context.Background(), req, f.provider, pstore); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("old cache accepted: %v", err)
	}
	if err = f.a.runGenerationTask(f.task.ID, f.a.generationService, req); !errors.Is(err, ErrFencedStaleExecution) {
		t.Fatalf("old worker/cache accepted: %v", err)
	}
	if f.provider.calls.Load() != 0 {
		t.Fatal("regenerated old success")
	}
	got, state, _, live := f.taskState(t)
	if got != next || state != "RUNNING" || live {
		t.Fatalf("old result changed task g=%d state=%s live=%t", got, state, live)
	}
	f.assertEffects(t, false, 0, 0)
	for _, binding := range []string{"equal", "legacy_null"} {
		t.Run(binding, func(t *testing.T) {
			f := issue190Setup(t, db, imageDispatchNormal, false)
			g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
			if err != nil {
				t.Fatal(err)
			}
			req := f.req
			req.Params = cloneAnyMap(req.Params)
			req.Params[providerExecutionTaskParam] = f.task.ID
			ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
			identity, _, err := executionIdentity(req, "image", providerName(req))
			if err != nil {
				t.Fatal(err)
			}
			e, err := pstore.CreatePreparedForGenerationTask(ctx, identity)
			if err != nil {
				t.Fatal(err)
			}
			e, err = pstore.ClaimPreparedForGenerationTask(ctx, f.task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = pstore.SaveSucceededResult(ctx, e.ID, nil, raw); err != nil {
				t.Fatal(err)
			}
			if binding == "legacy_null" {
				if _, err = db.Exec(`UPDATE provider_executions SET task_execution_generation=NULL WHERE id=$1`, e.ID); err != nil {
					t.Fatal(err)
				}
			}
			images, err := guardedImage(ctx, req, f.provider, pstore)
			if err != nil || len(images) != 1 {
				t.Fatalf("valid cache rejected: %v", err)
			}
			req.GeneratedImages = images
			prepared, _, err := f.a.persistGeneratedImages(ctx, f.task.ID, req)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.completeGenerationTaskOwned(f.task.ID, prepared, g, owner); err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.completeGenerationTaskOwned(f.task.ID, prepared, g, owner); err != nil {
				t.Fatal(err)
			}
			if f.provider.calls.Load() != 0 {
				t.Fatal("valid cache regenerated")
			}
			f.assertEffects(t, true, 0, 0)
		})
	}
}
