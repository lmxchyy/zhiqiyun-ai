package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/app/smartvideo"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestPersistConnectorVideoRejectsUndecodableOrStreamlessMP4(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "invalid bytes", raw: []byte("not-an-mp4")},
	}
	if audioOnly, ok := audioOnlyMP4Fixture(t); ok {
		tests = append(tests, struct {
			name string
			raw  []byte
		}{name: "no video stream", raw: audioOnly})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previousReader := generatedVideoArtifactReader
			generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
				return append([]byte(nil), test.raw...), "video/mp4", "mp4", nil
			}
			defer func() { generatedVideoArtifactReader = previousReader }()

			provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
			fileService := newVideoAdmissionStorageService(provider)
			_, stored, err := (api{fileService: fileService}).persistConnectorVideo(
				t.Context(),
				"task_invalid_video",
				videoAdmissionRequest(),
			)
			if err == nil {
				t.Fatalf("persistConnectorVideo accepted invalid media; stored file=%q", stored.FileID)
			}
			if stored.FileID != "" {
				t.Fatalf("invalid media reached private storage: file=%q", stored.FileID)
			}
			if len(provider.objects) != 0 {
				t.Fatalf("invalid media wrote %d private storage objects", len(provider.objects))
			}
		})
	}
}

func TestPersistConnectorVideoRejectsInvalidProbeMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata smartvideo.VideoMetadata
		probeErr error
	}{
		{
			name:     "probe error",
			probeErr: errors.New("probe failed"),
		},
		{
			name:     "zero duration",
			metadata: smartvideo.VideoMetadata{DurationMS: 0, Width: 640, Height: 360},
		},
		{
			name:     "zero width",
			metadata: smartvideo.VideoMetadata{DurationMS: 1000, Width: 0, Height: 360},
		},
		{
			name:     "zero height",
			metadata: smartvideo.VideoMetadata{DurationMS: 1000, Width: 640, Height: 0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previousReader := generatedVideoArtifactReader
			generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
				return []byte("probe-fixture"), "video/mp4", "mp4", nil
			}
			defer func() { generatedVideoArtifactReader = previousReader }()
			previousProbe := generatedVideoMetadataProbe
			generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
				return test.metadata, test.probeErr
			}
			defer func() { generatedVideoMetadataProbe = previousProbe }()
			previousThumbnailExtractor := generatedVideoThumbnailExtractor
			generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
				return jpegDataURL(t, 4, 3), nil
			}
			defer func() { generatedVideoThumbnailExtractor = previousThumbnailExtractor }()

			provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
			fileService := newVideoAdmissionStorageService(provider)
			_, stored, err := (api{fileService: fileService}).persistConnectorVideo(
				t.Context(),
				"task_invalid_probe",
				videoAdmissionRequest(),
			)
			if err == nil {
				t.Fatalf("persistConnectorVideo accepted invalid probe metadata %+v; stored file=%q", test.metadata, stored.FileID)
			}
			if stored.FileID != "" {
				t.Fatalf("invalid probe metadata reached private storage: file=%q", stored.FileID)
			}
			if len(provider.objects) != 0 {
				t.Fatalf("invalid probe metadata wrote %d private storage objects", len(provider.objects))
			}
		})
	}
}

func TestPersistConnectorVideoRejectsInvalidThumbnail(t *testing.T) {
	tests := []struct {
		name      string
		thumbnail string
		err       error
	}{
		{name: "extraction error", err: errors.New("thumbnail extraction failed")},
		{name: "empty thumbnail"},
		{name: "non JPEG data", thumbnail: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("not-a-jpeg"))},
		{name: "wrong MIME", thumbnail: "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not-a-jpeg"))},
		{name: "truncated JPEG", thumbnail: truncatedJPEGDataURL(t)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previousReader := generatedVideoArtifactReader
			generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
				return []byte("validated-video"), "video/mp4", "mp4", nil
			}
			defer func() { generatedVideoArtifactReader = previousReader }()
			previousProbe := generatedVideoMetadataProbe
			generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
				return smartvideo.VideoMetadata{DurationMS: 1000, Width: 640, Height: 360}, nil
			}
			defer func() { generatedVideoMetadataProbe = previousProbe }()
			previousThumbnailExtractor := generatedVideoThumbnailExtractor
			generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
				return test.thumbnail, test.err
			}
			defer func() { generatedVideoThumbnailExtractor = previousThumbnailExtractor }()

			provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
			fileService := newVideoAdmissionStorageService(provider)
			_, stored, err := (api{fileService: fileService}).persistConnectorVideo(
				t.Context(),
				"task_invalid_thumbnail",
				videoAdmissionRequest(),
			)
			if err == nil {
				t.Fatalf("persistConnectorVideo accepted invalid thumbnail %q; stored file=%q", test.thumbnail, stored.FileID)
			}
			if stored.FileID != "" {
				t.Fatalf("invalid thumbnail reached private storage: file=%q", stored.FileID)
			}
			if len(provider.objects) != 0 {
				t.Fatalf("invalid thumbnail wrote %d private storage objects", len(provider.objects))
			}
		})
	}
}

func TestPersistConnectorVideoRejectsPrivateStorageAdmissionFailures(t *testing.T) {
	validProvider := func() storagecenter.Provider {
		return &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	}
	tests := []struct {
		name        string
		fileService func() *storagecenter.Service
	}{
		{name: "missing file service"},
		{
			name: "storage unavailable",
			fileService: func() *storagecenter.Service {
				return storagecenter.NewService(
					storagecenter.NewMemoryRepository(),
					generatedStorageTestFactory{provider: validProvider()},
					storagecenter.Options{MasterKey: "0123456789abcdef0123456789abcdef"},
				)
			},
		},
		{
			name: "upload failure",
			fileService: func() *storagecenter.Service {
				return newVideoAdmissionStorageService(failingVideoAdmissionProvider{
					Provider: validProvider(),
					err:      errors.New("upload failed"),
				})
			},
		},
		{
			name: "empty file ID",
			fileService: func() *storagecenter.Service {
				repo := contractViolatingEmptyFileIDRepository{Repository: storagecenter.NewMemoryRepository()}
				return newVideoAdmissionStorageServiceWithRepository(repo, validProvider())
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stubValidVideoAdmission(t)
			var fileService *storagecenter.Service
			if test.fileService != nil {
				fileService = test.fileService()
			}
			_, stored, err := (api{fileService: fileService}).persistConnectorVideo(
				t.Context(),
				"task_storage_failure",
				videoAdmissionRequest(),
			)
			if err == nil {
				t.Fatalf("persistConnectorVideo accepted storage admission failure; stored file=%q", stored.FileID)
			}
		})
	}
}

func TestRunVideoGenerationTaskReleasesBillingWhenStorageAdmissionFails(t *testing.T) {
	stubValidVideoAdmission(t)
	store := newBillingAcceptanceStore(t)
	const initialPoints = 1000
	if err := store.update(func(data *platformData) error {
		for index := range data.PointAccounts {
			if data.PointAccounts[index].UserID == billingAcceptanceUserID {
				data.PointAccounts[index].Available = initialPoints
				return nil
			}
		}
		return errors.New("billing acceptance point account not found")
	}); err != nil {
		t.Fatal(err)
	}
	req := videoAcceptanceRequest("artifact-storage-failure")
	pending, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatal(err)
	}
	if pending.ReservedPoints <= 0 {
		t.Fatalf("pending task did not reserve points: %+v", pending)
	}
	service := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider: generatedStorageVideoProvider{url: "https://video.example/result.mp4"},
	})

	(api{store: store}).runVideoGenerationTask(pending.ID, service, req)

	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	failed := generationBillingTaskByID(t, tasks, pending.ID)
	if failed.TaskStatus != taskStatusFailed || failed.BillingStatus != billingStatusReleased || failed.ReleasedPoints != pending.ReservedPoints {
		t.Fatalf("storage admission failure did not use task failure/release path: %+v", failed)
	}
	assets, err := store.ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 || len(failed.ResultIDs) != 0 {
		t.Fatalf("storage admission failure created a successful work: assets=%+v resultIDs=%+v", assets, failed.ResultIDs)
	}
	account, err := store.PointAccount(billingAcceptanceUserID)
	if err != nil {
		t.Fatal(err)
	}
	if account.Available != initialPoints || account.Frozen != 0 {
		t.Fatalf("storage admission failure did not restore balance: %+v", account)
	}
}

func TestExecuteConnectorVideoGenerationRejectsTerminalTaskReplay(t *testing.T) {
	for _, status := range []string{"FAILED", "CANCELLED"} {
		t.Run(status, func(t *testing.T) {
			stubValidVideoAdmission(t)
			const (
				userID   = "user_terminal_replay"
				tenantID = "tenant_terminal_replay"
			)
			store := &terminalVideoReplayStore{
				task: generationTask{
					ID:              "task_terminal_replay",
					ClientRequestID: "terminal-replay",
					UserID:          userID,
					Status:          status,
					TaskStatus:      status,
					BillingStatus:   billingStatusReleased,
				},
				user: adminUser{ID: userID, Role: roleUser, Status: "ACTIVE", PlanID: "plan_month"},
				data: seedAdminData(),
				authorization: modelCallAuthorization{
					ContextType: contextEnterprise, TenantID: tenantID, OrganizationID: "org_terminal_replay",
					UserID: userID, Role: roleUser, BillingScope: contextEnterprise, BillingAccountID: tenantID, ServiceState: "ACTIVE",
				},
			}
			provider := &countingTerminalReplayVideoProvider{}
			service := generation.NewServiceWithOptions(generation.ServiceOptions{VideoProvider: provider})
			storageProvider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
			req := generation.CreateRequest{
				ClientRequestID: "terminal-replay",
				Type:            "TEXT_TO_VIDEO",
				Prompt:          "terminal task replay must not regenerate",
				Model:           "doubao-seedance-2.0",
				Params: map[string]any{
					"duration": float64(5), "resolution": "720p", "aspect_ratio": "16:9",
				},
			}

			_, _, stored, err := (api{
				store: store, generationService: service, connectorGenerationService: &service,
				fileService: newVideoAdmissionStorageService(storageProvider),
			}).executeConnectorVideoGeneration(t.Context(), userID, tenantID, req)
			if err == nil || !strings.Contains(strings.ToUpper(err.Error()), status) {
				t.Fatalf("terminal replay error=%v, want explicit %s rejection", err, status)
			}
			if provider.calls != 0 {
				t.Fatalf("terminal replay invoked video provider %d times", provider.calls)
			}
			if len(storageProvider.objects) != 0 {
				t.Fatalf("terminal replay wrote %d private storage objects", len(storageProvider.objects))
			}
			if store.completeCalls != 0 || store.failCalls != 0 {
				t.Fatalf("terminal replay mutated terminal task: complete=%d fail=%d", store.completeCalls, store.failCalls)
			}
			if stored.FileID != "" {
				t.Fatalf("terminal replay returned new delivery artifact: stored=%+v", stored)
			}
		})
	}
}

func TestPersistConnectorVideoPopulatesValidatedAssetMetadata(t *testing.T) {
	stubValidVideoAdmission(t)
	const tenantID = "tenant_enterprise_video"
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	req := videoAdmissionRequest()
	req.Params["tenant_id"] = tenantID
	prepared, stored, err := (api{fileService: newVideoAdmissionStorageService(provider)}).persistConnectorVideo(
		t.Context(),
		"task_valid_video",
		req,
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stored.FileID) == "" || stored.Status != storagecenter.StatusActive || stored.Visibility != "PRIVATE" {
		t.Fatalf("valid video did not complete private persistence: %+v", stored)
	}
	if stored.TenantID != tenantID || !strings.HasPrefix(stored.ObjectKey, "tenants/"+tenantID+"/") {
		t.Fatalf("valid video storage route tenant=%q objectKey=%q, want non-default tenant %q", stored.TenantID, stored.ObjectKey, tenantID)
	}
	if len(provider.objects) != 1 {
		t.Fatalf("valid video wrote %d private storage objects, want 1", len(provider.objects))
	}
	providerTask := providerTaskPayload(prepared)
	if stringValue(providerTask["videoUrl"]) != "" || stringValue(providerTask["sourceUrl"]) != "" {
		t.Fatalf("prepared provider task retained upstream URL: %+v", providerTask)
	}
	if stringValue(providerTask["provider"]) != "test-video-provider" {
		t.Fatalf("prepared provider=%q, want test-video-provider", stringValue(providerTask["provider"]))
	}
	duration, durationOK := providerTask["duration"].(float64)
	if !durationOK || duration != 1.5 {
		t.Fatalf("provider task duration=%#v, want numeric 1.5 seconds", providerTask["duration"])
	}
	if intValue(providerTask["width"]) != 640 || intValue(providerTask["height"]) != 360 {
		t.Fatalf("provider task dimensions=%#vx%#v, want 640x360", providerTask["width"], providerTask["height"])
	}
	if !strings.HasPrefix(stringValue(providerTask["thumbnailUrl"]), "data:image/jpeg;base64,") {
		t.Fatalf("provider task thumbnail=%q, want JPEG data URL", stringValue(providerTask["thumbnailUrl"]))
	}
	if paramsDuration, ok := prepared.Params["duration"].(float64); !ok || paramsDuration != duration {
		t.Fatalf("request duration=%#v, want validated numeric duration %v", prepared.Params["duration"], duration)
	}
	record, ok := generatedStorageRecord(prepared.Params, 0)
	if !ok || stringValue(record["fileId"]) != stored.FileID {
		t.Fatalf("request storage record=%+v, want file ID %q", record, stored.FileID)
	}
	if stringValue(record["tenantId"]) != tenantID {
		t.Fatalf("request storage tenant=%q, want %q", stringValue(record["tenantId"]), tenantID)
	}
	if stringValue(record["sourceUrl"]) != "" {
		t.Fatalf("request storage record retained upstream URL: %+v", record)
	}

	item := generatedAssetForRequest(
		prepared,
		prepared.UserID,
		"task_valid_video",
		"asset_valid_video",
		0,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	assetDuration, assetDurationOK := item.Metadata["duration"].(float64)
	if !assetDurationOK || assetDuration != 1.5 {
		t.Fatalf("asset duration=%#v, want numeric 1.5 seconds", item.Metadata["duration"])
	}
	if intValue(item.Metadata["width"]) != 640 || intValue(item.Metadata["height"]) != 360 || stringValue(item.Metadata["resolution"]) != "640x360" {
		t.Fatalf("asset dimensions metadata=%+v, want 640x360", item.Metadata)
	}
	if item.ThumbnailURL != stringValue(providerTask["thumbnailUrl"]) || !strings.HasPrefix(item.ThumbnailURL, "data:image/jpeg;base64,") {
		t.Fatalf("asset thumbnail=%q, want validated JPEG first frame", item.ThumbnailURL)
	}
	if stringValue(item.Metadata["fileId"]) != stored.FileID || !boolValue(item.Metadata["storageManaged"]) {
		t.Fatalf("asset storage metadata=%+v, want managed file %q", item.Metadata, stored.FileID)
	}
	if item.MediaType != "video" || item.URL != "" {
		t.Fatalf("asset mediaType=%q url=%q, want managed video with no persisted upstream URL", item.MediaType, item.URL)
	}
	if stringValue(item.Metadata["sourceUrl"]) != "" || stringValue(item.Metadata["source"]) != "test-video-provider" {
		t.Fatalf("asset provider/privacy metadata=%+v", item.Metadata)
	}
}

func TestPersistConnectorVideoRejectsBusinessLimitViolationsBeforeThumbnailOrStorage(t *testing.T) {
	tests := []struct {
		name     string
		metadata smartvideo.VideoMetadata
	}{
		{name: "duration exceeds container tolerance", metadata: smartvideo.VideoMetadata{DurationMS: 16001, Width: 640, Height: 360}},
		{name: "width exceeds 4k side", metadata: smartvideo.VideoMetadata{DurationMS: 15000, Width: 4097, Height: 2160}},
		{name: "height exceeds 4k side", metadata: smartvideo.VideoMetadata{DurationMS: 15000, Width: 2160, Height: 4097}},
		{name: "pixel count exceeds 4k", metadata: smartvideo.VideoMetadata{DurationMS: 15000, Width: 4096, Height: 2161}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previousReader := generatedVideoArtifactReader
			generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
				return []byte("oversized-video"), "video/mp4", "mp4", nil
			}
			defer func() { generatedVideoArtifactReader = previousReader }()
			previousProbe := generatedVideoMetadataProbe
			generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
				return test.metadata, nil
			}
			defer func() { generatedVideoMetadataProbe = previousProbe }()
			thumbnailCalls := 0
			previousThumbnailExtractor := generatedVideoThumbnailExtractor
			generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
				thumbnailCalls++
				return jpegDataURL(t, 4, 3), nil
			}
			defer func() { generatedVideoThumbnailExtractor = previousThumbnailExtractor }()

			provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
			_, stored, err := (api{fileService: newVideoAdmissionStorageService(provider)}).persistConnectorVideo(
				t.Context(), "task_business_limit", videoAdmissionRequest(),
			)
			if err == nil {
				t.Fatalf("accepted out-of-policy metadata %+v", test.metadata)
			}
			if thumbnailCalls != 0 || stored.FileID != "" || len(provider.objects) != 0 {
				t.Fatalf("limit rejection occurred too late: thumbnailCalls=%d stored=%q objects=%d", thumbnailCalls, stored.FileID, len(provider.objects))
			}
		})
	}
}

func TestGeneratedVideoProcessGateCancellationDoesNotStartProcessing(t *testing.T) {
	for index := 0; index < cap(generatedVideoProcessGate); index++ {
		generatedVideoProcessGate <- struct{}{}
	}
	t.Cleanup(func() {
		for index := 0; index < cap(generatedVideoProcessGate); index++ {
			<-generatedVideoProcessGate
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := false
	err := runGeneratedVideoProcess(ctx, func() error {
		started = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("gate wait error=%v, want context.Canceled", err)
	}
	if started {
		t.Fatal("processing started while shared gate was saturated and context was canceled")
	}
}

func TestPersistConnectorVideoLimitsFullMediaAdmissionConcurrency(t *testing.T) {
	previousReader := generatedVideoArtifactReader
	previousProbe := generatedVideoMetadataProbe
	previousThumbnail := generatedVideoThumbnailExtractor
	t.Cleanup(func() {
		generatedVideoArtifactReader = previousReader
		generatedVideoMetadataProbe = previousProbe
		generatedVideoThumbnailExtractor = previousThumbnail
	})

	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	var active int32
	var maxActive int32
	generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}
		entered <- struct{}{}
		return []byte("validated-video"), "video/mp4", "mp4", nil
	}
	generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
		return smartvideo.VideoMetadata{DurationMS: 1500, Width: 640, Height: 360}, nil
	}
	thumbnail := jpegDataURL(t, 4, 3)
	generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
		<-release
		atomic.AddInt32(&active, -1)
		return thumbnail, nil
	}

	type admissionResult struct {
		id  int
		err error
	}
	storageProvider := &blockingGeneratedStorageProvider{
		generatedStorageTestProvider: &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}},
		entered:                      make(chan struct{}, 2),
		release:                      release,
	}
	admissionAPI := api{fileService: newVideoAdmissionStorageService(storageProvider)}
	results := make(chan admissionResult, 3)
	start := func(ctx context.Context, id int) {
		go func() {
			_, _, err := admissionAPI.persistConnectorVideo(ctx, "task_gate", videoAdmissionRequest())
			results <- admissionResult{id: id, err: err}
		}()
	}
	waitForEntry := func(label string) {
		t.Helper()
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s media admission", label)
		}
	}

	start(t.Context(), 1)
	start(t.Context(), 2)
	waitForEntry("first")
	waitForEntry("second")

	thirdCtx, cancelThird := context.WithCancel(t.Context())
	defer cancelThird()
	start(thirdCtx, 3)
	select {
	case <-entered:
		t.Fatal("third media admission entered while two complete chains were active")
	case <-time.After(200 * time.Millisecond):
	}
	cancelThird()
	select {
	case result := <-results:
		if result.id != 3 || !errors.Is(result.err, context.Canceled) {
			t.Fatalf("canceled waiter result=%+v, want id=3 context.Canceled", result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled media admission waiter did not exit")
	}

	close(release)
	for completed := 0; completed < 2; completed++ {
		select {
		case <-results:
		case <-time.After(2 * time.Second):
			t.Fatal("active media admission did not finish after release")
		}
	}
	if got := atomic.LoadInt32(&maxActive); got != 2 {
		t.Fatalf("max active media admissions=%d, want 2", got)
	}
	if got := atomic.LoadInt32(&active); got != 0 {
		t.Fatalf("active media admissions after completion=%d, want 0", got)
	}
}

type blockingGeneratedStorageProvider struct {
	*generatedStorageTestProvider
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
}

func (p *blockingGeneratedStorageProvider) PutObject(ctx context.Context, key string, source io.Reader, size int64, contentType string) (storagecenter.ObjectMetadata, error) {
	p.entered <- struct{}{}
	select {
	case <-ctx.Done():
		return storagecenter.ObjectMetadata{}, ctx.Err()
	case <-p.release:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generatedStorageTestProvider.PutObject(ctx, key, source, size, contentType)
}

func TestPersistConnectorVideoGateHoldsThroughPrivateStore(t *testing.T) {
	previousReader := generatedVideoArtifactReader
	previousProbe := generatedVideoMetadataProbe
	previousThumbnail := generatedVideoThumbnailExtractor
	t.Cleanup(func() {
		generatedVideoArtifactReader = previousReader
		generatedVideoMetadataProbe = previousProbe
		generatedVideoThumbnailExtractor = previousThumbnail
	})

	readerEntered := make(chan struct{}, 3)
	generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
		readerEntered <- struct{}{}
		return []byte("validated-video"), "video/mp4", "mp4", nil
	}
	generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
		return smartvideo.VideoMetadata{DurationMS: 1500, Width: 640, Height: 360}, nil
	}
	thumbnail := jpegDataURL(t, 4, 3)
	generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
		return thumbnail, nil
	}

	release := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	provider := &blockingGeneratedStorageProvider{
		generatedStorageTestProvider: &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}},
		entered:                      make(chan struct{}, 2),
		release:                      release,
	}
	a := api{fileService: newVideoAdmissionStorageService(provider)}
	results := make(chan error, 3)
	start := func(ctx context.Context, taskID string) {
		go func() {
			_, _, err := a.persistConnectorVideo(ctx, taskID, videoAdmissionRequest())
			results <- err
		}()
	}

	start(t.Context(), "task_store_gate_1")
	start(t.Context(), "task_store_gate_2")
	for index := 0; index < 2; index++ {
		select {
		case <-provider.entered:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for private storage admission")
		}
		select {
		case <-readerEntered:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for generated video download")
		}
	}

	thirdCtx, cancelThird := context.WithCancel(t.Context())
	start(thirdCtx, "task_store_gate_3")
	select {
	case <-readerEntered:
		t.Fatal("third video download entered while two private StoreObject calls held the shared gate")
	case <-time.After(200 * time.Millisecond):
	}
	cancelThird()
	select {
	case err := <-results:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled waiter error=%v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled StoreObject gate waiter did not exit")
	}

	close(release)
	for completed := 0; completed < 2; completed++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("active private storage admission failed: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("active private storage admission did not finish")
		}
	}
}

func videoAdmissionRequest() generation.CreateRequest {
	providerTask := map[string]any{
		"videoUrl": "https://video.example/result.mp4",
		"provider": "test-video-provider",
	}
	return generation.CreateRequest{
		UserID:    "user_000002",
		Type:      "TEXT_TO_VIDEO",
		Prompt:    "validate generated video",
		Model:     "mock-video",
		Params:    map[string]any{"tenant_id": "tenant_default", "providerTask": providerTask},
		VideoTask: providerTask,
	}
}

func newVideoAdmissionStorageService(provider storagecenter.Provider) *storagecenter.Service {
	return newVideoAdmissionStorageServiceWithRepository(storagecenter.NewMemoryRepository(), provider)
}

func newVideoAdmissionStorageServiceWithRepository(repo storagecenter.Repository, provider storagecenter.Provider) *storagecenter.Service {
	return storagecenter.NewService(
		repo,
		generatedStorageTestFactory{provider: provider},
		storagecenter.Options{
			DefaultProvider: "s3",
			Endpoint:        "https://storage.example",
			AccessKey:       "access",
			SecretKey:       "secret",
			Bucket:          "private-files",
			MaxUploadBytes:  2 << 20,
			MasterKey:       "0123456789abcdef0123456789abcdef",
		},
	)
}

type failingVideoAdmissionProvider struct {
	storagecenter.Provider
	err error
}

func (p failingVideoAdmissionProvider) PutObject(context.Context, string, io.Reader, int64, string) (storagecenter.ObjectMetadata, error) {
	return storagecenter.ObjectMetadata{}, p.err
}

type contractViolatingEmptyFileIDRepository struct {
	storagecenter.Repository
}

func (r contractViolatingEmptyFileIDRepository) CompleteUpload(ctx context.Context, tenantID string, fileID string, metadata storagecenter.ObjectMetadata) (storagecenter.FileObject, error) {
	file, err := r.Repository.CompleteUpload(ctx, tenantID, fileID, metadata)
	file.FileID = ""
	return file, err
}

type terminalVideoReplayStore struct {
	platformStore
	task          generationTask
	user          adminUser
	data          adminPlatformData
	authorization modelCallAuthorization
	completeCalls int
	failCalls     int
}

func (s *terminalVideoReplayStore) AdminData() (adminPlatformData, error) {
	return s.data, nil
}

func (s *terminalVideoReplayStore) GetActiveUser(userID string) (adminUser, bool, error) {
	return s.user, userID == s.user.ID, nil
}

func (s *terminalVideoReplayStore) GetChannelAgentForUser(string) (adminChannelAgent, bool, error) {
	return adminChannelAgent{}, false, nil
}

func (s *terminalVideoReplayStore) AuthorizeConnectorModelCall(string, string, string) (modelCallAuthorization, error) {
	return s.authorization, nil
}

func (s *terminalVideoReplayStore) CreatePendingGenerationTask(createGenerationTaskRequest) (generationTask, error) {
	return s.task, nil
}

func (s *terminalVideoReplayStore) CompleteGenerationTask(string, createGenerationTaskRequest) (generationTask, error) {
	s.completeCalls++
	return s.task, nil
}

func (s *terminalVideoReplayStore) FailGenerationTask(string, string) (generationTask, error) {
	s.failCalls++
	return s.task, nil
}

type countingTerminalReplayVideoProvider struct {
	calls int
}

func (p *countingTerminalReplayVideoProvider) DefaultModel() string {
	return "doubao-seedance-2.0"
}

func (p *countingTerminalReplayVideoProvider) Create(context.Context, generation.CreateRequest) (any, error) {
	p.calls++
	return map[string]any{"videoUrl": "https://video.example/replayed.mp4", "provider": "terminal-replay-provider"}, nil
}

func stubValidVideoAdmission(t *testing.T) {
	t.Helper()
	previousReader := generatedVideoArtifactReader
	generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
		return []byte("validated-video"), "video/mp4", "mp4", nil
	}
	t.Cleanup(func() { generatedVideoArtifactReader = previousReader })
	previousProbe := generatedVideoMetadataProbe
	generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
		return smartvideo.VideoMetadata{DurationMS: 1500, Width: 640, Height: 360}, nil
	}
	t.Cleanup(func() { generatedVideoMetadataProbe = previousProbe })
	previousThumbnailExtractor := generatedVideoThumbnailExtractor
	generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
		return jpegDataURL(t, 4, 3), nil
	}
	t.Cleanup(func() { generatedVideoThumbnailExtractor = previousThumbnailExtractor })
}

func audioOnlyMP4Fixture(t *testing.T) ([]byte, bool) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Log("ffmpeg is unavailable; skipping no-video-stream fixture")
		return nil, false
	}
	path := filepath.Join(t.TempDir(), "audio-only.mp4")
	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.2",
		"-vn", "-c:a", "aac", path,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create audio-only MP4 fixture: %v: %s", err, output)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw, true
}

func jpegDataURL(t *testing.T, width int, height int) string {
	t.Helper()
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, frame, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func truncatedJPEGDataURL(t *testing.T) string {
	t.Helper()
	encodedURL := jpegDataURL(t, 32, 24)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encodedURL, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	for size := len(raw) - 1; size > 0; size-- {
		candidate := raw[:size]
		config, configErr := jpeg.DecodeConfig(bytes.NewReader(candidate))
		_, decodeErr := jpeg.Decode(bytes.NewReader(candidate))
		if configErr == nil && config.Width == 32 && config.Height == 24 && decodeErr != nil {
			return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(candidate)
		}
	}
	t.Fatal("could not construct a JPEG that passes DecodeConfig but fails Decode")
	return ""
}
