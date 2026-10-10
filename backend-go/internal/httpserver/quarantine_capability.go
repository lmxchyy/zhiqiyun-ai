package httpserver

// This bounded command is part of the API and generation-worker executables.
// It never loads normal config or starts a server/consumer. Fixture provisioning
// and independent observations belong to the host verifier, not this command.
import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storage "xianzhi-ai/backend-go/internal/storage"
)

// CapabilityReleaseSHA is injected by the production Dockerfile, not env.
var CapabilityReleaseSHA string

var capabilityUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// DispatchQuarantineCapability rejects ALL nonempty arguments that are not the
// exact bounded protocol; malformed challenges can never fall into startup.
func DispatchQuarantineCapability(args []string, role string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) != 3 || args[0] != "--quarantine-capability-v1" || !capabilityUUID.MatchString(args[1]) {
		return true, errors.New("invalid quarantine capability command")
	}
	phase := args[2]
	if phase != "blocked" && phase != "allowed" && phase != "unavailable" && phase != "history" && phase != "history-control" {
		return true, errors.New("invalid quarantine capability phase")
	}
	if role != "api" && role != "generation-worker" {
		return true, errors.New("inapplicable generation capability role")
	}
	for _, key := range []string{"DATABASE_URL", "POSTGRES_HOST", "POSTGRES_PASSWORD", "PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "XIANZHI_ENV", "XIANZHI_TEST_CONTAINER", "XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL", "REDIS_URL", "RABBITMQ_URL", "APP_ENV", "ENVIRONMENT"} {
		if os.Getenv(key) != "" {
			return true, errors.New("normal runtime configuration forbidden in capability command")
		}
	}
	owner := args[1]
	password := os.Getenv("QUARANTINE_FIXTURE_PASSWORD")
	if len(password) != 64 || strings.Trim(password, "0123456789abcdef") != "" {
		return true, errors.New("invalid disposable fixture credentials")
	}
	name := "p4_" + strings.ReplaceAll(owner, "-", "")
	// No caller-supplied URL, host, database, provider or storage endpoint.
	db, err := sql.Open("pgx", fmt.Sprintf("postgres://%s:%s@fixture-db:5432/%s?sslmode=disable&connect_timeout=3", name, password, name))
	if err != nil {
		return true, errors.New("fixture database unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var database, user, marker string
	if err := db.QueryRowContext(ctx, `SELECT current_database(),current_user,owner FROM quarantine_capability_fixture`).Scan(&database, &user, &marker); err != nil || database != name || user != name || marker != owner {
		return true, errors.New("disposable database ownership mismatch")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(CapabilityReleaseSHA) {
		return true, errors.New("packaged release SHA missing")
	}
	// The host has already applied the production schema and migrations.
	// Do not run normal bootstrap/seeding against challenge fixtures.
	pg := &postgresStore{db: db, ready: true}
	a := api{store: pg}
	// Production Service/repository and persistence methods; only the remote
	// object provider is synthetic, with independently observed HTTP writes.
	a.fileService = storage.NewService(storage.NewPostgresRepository(db), capabilityStorageFactory{}, storage.Options{DefaultProvider: "s3", Endpoint: "http://fixture-sink:8080", AccessKey: "fixture", SecretKey: "fixture", Bucket: "fixture", DefaultQuotaBytes: 1 << 20, MaxUploadBytes: 1 << 20, MasterKey: password})
	s := pe.NewStore(db)
	if phase == "history" {
		return true, a.challengeHistoricalImages(ctx, db, owner, role)
	}
	if phase == "history-control" {
		// Only the remote is synthetic; the normal consumer, orchestration,
		// provider hook, persistence, billing and inbox completion are real.
		// The host seeds a real admitted OpenAI-compatible channel bound
		// only to fixture-sink. Sentinel/mock models skip provider hooks.
		a.cfg = config.Config{ProviderExecutionSafetyEnabled: true}
		taskID := owner + "-normal-control"
		envelope := &messaging.Envelope{EventID: taskID + "-event", AggregateType: "generation_task", AggregateID: taskID, EventType: messaging.GenerationImageNormalRoutingKey, Data: map[string]any{"task_id": taskID, "dispatch_mode": imageDispatchNormal, "execution_generation": int64(1)}}
		if err := a.processGenerationNormalMessage(ctx, messaging.NewInboxStore(db), envelope); err != nil {
			return true, fmt.Errorf("funded normal consumer control failed: %w", err)
		}
		return true, json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": 1, "history_protocol": 1, "owner": owner, "phase": phase, "role": role, "release_sha": CapabilityReleaseSHA, "operations": map[string]string{"normal-consumer": "completed"}})
	}
	points := NewPostgresPersonalPointStore(db)
	results := make(map[string]string)
	details := make(map[string]any)
	check := func(operation string, call func() error) error {
		err := call()
		wanted := pe.ErrQuarantined
		if phase == "unavailable" {
			wanted = pe.ErrQuarantineBarrierUnavailable
		}
		if phase != "allowed" {
			if !errors.Is(err, wanted) {
				return fmt.Errorf("%s: required barrier rejection absent", operation)
			}
			results[operation] = wanted.Error()
		} else {
			if err != nil {
				return fmt.Errorf("%s: normal control failed: %w", operation, err)
			}
			results[operation] = "allowed"
		}
		return nil
	}
	txCall := func(call func(*sql.Tx) error) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = call(tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	task := func(op string) string { return owner + "-" + op }
	executionID := func(op string) int64 {
		var id int64
		_ = db.QueryRowContext(ctx, `SELECT id FROM provider_executions WHERE task_id=$1`, task(op)).Scan(&id)
		return id
	}
	operations := []struct {
		name string
		call func() error
	}{
		{"provider", func() error {
			_, err := guardedImage(ctx, generation.CreateRequest{Model: "fixture", Params: map[string]any{providerExecutionTaskParam: task("provider")}}, capabilityImageProvider{}, s)
			return err
		}},
		{"video-provider", func() error {
			_, err := guardedVideo(ctx, generation.CreateRequest{Model: "fixture", Params: map[string]any{providerExecutionTaskParam: task("video-provider")}}, capabilityVideoProvider{}, s, pg)
			return err
		}},
		{"video-persistence", func() error {
			req := generation.CreateRequest{UserID: owner, VideoTask: map[string]any{"videoUrl": "http://fixture-sink:8080/notdownloaded"}, Params: map[string]any{}}
			result, stored, err := a.persistGeneratedVideos(ctx, task("video-persistence"), req)
			if err != nil {
				return err
			}
			record, found := generatedStorageRecord(result.Params, 0)
			if !found || record["fileId"] != owner+"-video-file" || len(stored) != 0 {
				return errors.New("owned video durable reuse control failed")
			}
			details["video-persistence"] = map[string]any{"positive_mode": "existing_durable_reuse", "file_id": record["fileId"], "task_id": task("video-persistence"), "new_files": len(stored), "record": record}
			return nil
		}},
		{"create", func() error {
			_, err := s.CreatePreparedForGenerationTask(ctx, pe.Execution{TaskID: task("create"), Provider: "fixture", ProviderModel: "fixture", Capability: "image", Attempt: 2, RequestFingerprint: strings.Repeat("1", 64)})
			return err
		}},
		{"execution-claim", func() error { _, err := s.ClaimPreparedForGenerationTask(ctx, task("execution-claim")); return err }},
		{"transition", func() error { return s.Transition(ctx, executionID("transition"), pe.Submitting, nil, nil, nil) }},
		{"result", func() error {
			return s.SaveSucceededResult(ctx, executionID("result"), nil, []byte(`{"fixture":true}`))
		}},
		{"correlation", func() error {
			_, err := s.RecordCorrelation(ctx, pe.CorrelationEvent{ExecutionID: executionID("correlation"), Kind: "submit", ProviderCode: "fixture", State: "success"})
			return err
		}},
		{"status", func() error {
			return txCall(func(tx *sql.Tx) error {
				_, err := claimGenerationOwnershipTx(ctx, tx, task("status"), "fixture-worker", time.Minute)
				return err
			})
		}},
		{"lease", func() error {
			return txCall(func(tx *sql.Tx) error {
				return renewGenerationLeaseTx(ctx, tx, task("lease"), "fixture-worker", 1, 2*time.Minute)
			})
		}},
		{"persistence", func() error {
			_, _, err := a.persistGeneratedImages(ctx, task("persistence"), generation.CreateRequest{UserID: owner, GeneratedImages: []generation.GeneratedImage{{URL: "data:image/png;base64,aGVsbG8=", ContentType: "image/png"}}})
			return err
		}},
		{"asset", func() error {
			_, err := a.ensureDurablePPTAsset(ctx, task("asset"), owner, "tenant_default", "", "fixture", storage.FileObject{FileID: owner}, "storage://"+owner)
			return err
		}},
		{"reserve", func() error {
			_, err := points.reserve(ctx, PersonalPointReserveCommand{AccountID: task("reserve"), UserID: owner, BusinessType: "GENERATION_TASK", BusinessID: task("reserve"), QuarantineTaskID: task("reserve"), RequestedPoints: 10, IdempotencyKey: "generation:reserve:" + task("reserve")})
			return err
		}},
		{"capture", func() error {
			_, err := points.capture(ctx, PersonalPointCaptureCommand{AccountID: task("capture"), UserID: owner, ReservationID: task("capture"), Points: 5, IdempotencyKey: "generation:capture:" + task("capture")})
			return err
		}},
		{"release", func() error {
			_, err := points.release(ctx, PersonalPointReleaseCommand{AccountID: task("release"), UserID: owner, ReservationID: task("release"), Points: 5, IdempotencyKey: "generation:release:" + task("release")})
			return err
		}},
	}
	for _, op := range operations {
		if err := check(op.name, op.call); err != nil {
			return true, err
		}
	}
	if phase == "allowed" {
		if _, err := (capabilityImageProvider{}).Get(ctx, "fixture-query-control"); err != nil {
			return true, err
		}
		details["provider-query"] = "observed-get-control"
	}
	// Typed nil interfaces must not panic or become a no-barrier store.
	var nilDB *sql.DB
	var nilTx *sql.Tx
	_, nilImageErr := guardedImage(ctx, generation.CreateRequest{Model: "fixture", Params: map[string]any{providerExecutionTaskParam: owner}}, capabilityImageProvider{}, nil)
	_, nilVideoErr := guardedVideo(ctx, generation.CreateRequest{Model: "fixture", Params: map[string]any{providerExecutionTaskParam: owner}}, capabilityVideoProvider{}, nil, pg)
	nilChecks := []error{nilImageErr, nilVideoErr, pe.RejectTask(ctx, nilDB, owner, "nil-db"), pe.RejectTask(ctx, nilTx, owner, "nil-tx"), rejectQuarantinedGeneration(ctx, nilDB, owner, "nil-db"), rejectQuarantinedTaskTx(ctx, nilTx, owner, "nil-tx")}
	for _, err := range nilChecks {
		if !errors.Is(err, pe.ErrQuarantineBarrierUnavailable) {
			return true, errors.New("nil required dependency failed open")
		}
	}
	return true, json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": 1, "owner": owner, "phase": phase, "role": role, "release_sha": CapabilityReleaseSHA, "operations": results, "details": details, "nil_dependency_checks": len(nilChecks)})
}

// Synthetic remotes contain no guard logic. They record effects outside the
// candidate; a successful stdout response is never sufficient evidence.
type capabilityImageProvider struct{}

func (capabilityImageProvider) DefaultModel() string { return "fixture" }

func (capabilityImageProvider) Generate(ctx context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	body := bytes.NewBufferString("provider")
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://fixture-sink:8080/provider", body)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("fixture provider failed")
	}
	return []generation.GeneratedImage{{URL: "data:image/png;base64,aGVsbG8=", ContentType: "image/png"}}, nil
}

type capabilityVideoProvider struct{}

func (capabilityVideoProvider) DefaultModel() string { return "fixture" }
func (capabilityVideoProvider) Create(ctx context.Context, req generation.CreateRequest) (any, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://fixture-sink:8080/provider-video", bytes.NewBufferString("video-provider"))
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("fixture video provider failed")
	}
	return map[string]any{"status": "succeeded", "providerTaskId": "fixture-video", "videoUrl": "http://fixture-sink:8080/notdownloaded"}, nil
}

type capabilityStorageFactory struct{}

func (capabilityStorageFactory) Build(storage.Config) (storage.Provider, error) {
	return capabilityStorageProvider{}, nil
}

type capabilityStorageProvider struct{ storage.Provider }

func (capabilityStorageProvider) PutObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) (storage.ObjectMetadata, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://fixture-sink:8080/object", body)
	if err != nil {
		return storage.ObjectMetadata{}, err
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		return storage.ObjectMetadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return storage.ObjectMetadata{}, errors.New("fixture object sink failed")
	}
	return storage.ObjectMetadata{Size: size, ContentType: contentType, ETag: "fixture"}, nil
}
func (capabilityStorageProvider) HeadObject(context.Context, string) (storage.ObjectMetadata, error) {
	return storage.ObjectMetadata{}, storage.ErrFileNotFound
}
func (capabilityStorageProvider) TestConnection(context.Context) error { return nil }

// History attests reachable task-rooted entrypaths, NOT direct persist/transition
// primitives. Non-enrolled rows have no general-purpose immutability barrier.
func (a api) challengeHistoricalImages(ctx context.Context, db *sql.DB, owner, role string) error {
	s := pe.NewStore(db)
	results := map[string]string{}
	inbox := messaging.NewInboxStore(db)
	for _, name := range []string{"text-normal-legacy", "text-normal", "image-normal-legacy", "image-normal", "text-canary-legacy", "text-canary", "image-canary-legacy", "image-canary", "orphan"} {
		taskID := owner + "-" + name
		orphan := name == "orphan"
		if orphan {
			var count int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&count); err != nil || count != 0 {
				return errors.New("history orphan task association is not absent")
			}
		}
		mode := imageDispatchNormal
		if strings.Contains(name, "canary") {
			mode = imageDispatchCanary
		}
		route := messaging.GenerationImageNormalRoutingKey
		if mode == imageDispatchCanary {
			route = messaging.GenerationCanaryRoutingKey
		}
		for _, g := range []int64{2, 3, 4} {
			envelope := &messaging.Envelope{EventID: fmt.Sprintf("%s-history-%d", taskID, g), AggregateType: "generation_task", AggregateID: taskID, EventType: route, Data: map[string]any{"task_id": taskID, "dispatch_mode": mode, "execution_generation": g}}
			err := a.processGenerationImageMessage(ctx, inbox, envelope, mode)
			if orphan {
				if err == nil || !messaging.IsPermanent(err) || err.Error() != fmt.Sprintf("generation task %s not found", taskID) {
					return errors.New("history missing-task consumer rejection absent")
				}
			} else if !errors.Is(err, ErrFencedStaleExecution) {
				return errors.New("history consumer binding rejection absent")
			}
		}
		_, _, err := claimGenerationTaskOwnershipForDispatch(a.store, taskID, 3)
		if orphan {
			if !errors.Is(err, sql.ErrNoRows) {
				return errors.New("history missing-task claim rejection absent")
			}
		} else if !errors.Is(err, ErrFencedStaleExecution) {
			return errors.New("history claim fence absent")
		}
		params := map[string]any{"provider": "fixture", providerExecutionTaskParam: taskID}
		if mode == imageDispatchCanary {
			params["generation_async_canary"] = true
		}
		kind := "TEXT_TO_IMAGE"
		if strings.HasPrefix(name, "image") {
			kind = "IMAGE_TO_IMAGE"
		}
		req := generation.CreateRequest{UserID: owner, Type: kind, Model: "fixture", Prompt: "synthetic history", Params: params}
		_, err = guardedImage(ctx, req, capabilityImageProvider{}, s)
		if orphan {
			if !errors.Is(err, sql.ErrNoRows) {
				return errors.New("history missing-task provider rejection absent")
			}
			// generationTaskTerminal's PostgreSQL status SELECT returns this
			// exact error only for sql.ErrNoRows, before ownership/provider work.
			if err := a.runGenerationTaskForDispatch(taskID, a.generationService, req, 3); err == nil || err.Error() != fmt.Sprintf("generation task %s not found", taskID) {
				return fmt.Errorf("history missing-task orchestration rejection absent: %v", err)
			}
		} else {
			if !errors.Is(err, ErrFencedStaleExecution) {
				return fmt.Errorf("history provider binding fence absent: %w", err)
			}
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			task, err := generationTaskForUpdate(ctx, tx, taskID)
			_ = tx.Rollback()
			if err != nil {
				return err
			}
			execution, err := s.GetLatestByTask(ctx, taskID)
			if err != nil {
				return err
			}
			if err = a.recoverSucceededGenerationTask(task, execution); !errors.Is(err, ErrFencedStaleExecution) {
				return errors.New("history local recovery fence absent")
			}
			if err = a.runGenerationTaskForDispatch(taskID, a.generationService, req, 3); !errors.Is(err, ErrFencedStaleExecution) {
				return errors.New("history orchestration fence absent")
			}
		}
		results[name] = "read-only-rejected"
	}
	a.repairStaleGenerationTasksWithContext(ctx, time.Minute)
	scheduler := NewGenerationScheduler(db, GenerationSchedulerOptions{Owner: "fixture-history-scheduler", BatchTasksPerUser: 2})
	if n, err := scheduler.RecoverStaleDispatches(ctx); err != nil || n != 0 {
		return errors.New("history scheduler recovery changed history")
	}
	if n, err := scheduler.dispatchUserTx(ctx, owner, time.Now().UTC()); err != nil || n != 0 {
		return errors.New("history scheduler admission changed history")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"protocol": 1, "history_protocol": 1, "owner": owner, "phase": "history", "role": role, "release_sha": CapabilityReleaseSHA, "operations": results})
}

func (capabilityImageProvider) Get(ctx context.Context, id string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://fixture-sink:8080/provider-query", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return map[string]any{"status": "processing"}, nil
}
