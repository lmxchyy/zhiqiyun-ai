package smartvideoruntime

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
	"xianzhi-ai/backend-go/internal/infra"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
	mediaprovider "xianzhi-ai/backend-go/internal/provider/media"
	"xianzhi-ai/backend-go/internal/provider/smartvideoplan"
	"xianzhi-ai/backend-go/internal/provider/speech"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type Runtime struct {
	workers         []*smartvideo.AnalysisWorker
	renderWorkers   []*smartvideo.RenderWorker
	planWorkers     []*smartvideo.PlanWorker
	outboxPublisher *infra.OutboxPublisher
	probe           *mediaprovider.FFmpegAdapter
	tempDir         string
	db              *sql.DB
	redis           *redis.Client
	analysisQueue   *infra.SmartVideoAnalysisQueue
	planQueue       *infra.SmartVideoPlanQueue
	renderQueue     *infra.SmartVideoRenderQueue
	speechReady     bool
	planReady       bool
	metricsEvery    time.Duration
}

func New(cfg config.Config, db *sql.DB, redisClient *redis.Client) (*Runtime, error) {
	if !cfg.SmartVideoAnalysisEnabled {
		return nil, smartvideo.ErrAnalysisDisabled
	}
	if db == nil || redisClient == nil {
		return nil, fmt.Errorf("%w: PostgreSQL and Redis are required", smartvideo.ErrAnalysisNotReady)
	}
	repository := smartvideo.NewPostgresRepository(db)
	queue := infra.NewSmartVideoAnalysisQueue(redisClient, "")
	planQueue := infra.NewSmartVideoPlanQueue(redisClient)
	renderQueue := infra.NewSmartVideoRenderQueue(redisClient)
	fileRepository := storagecenter.NewPostgresRepository(db)
	fileService := storagecenter.NewService(
		fileRepository,
		storagecenter.S3ProviderFactory{AutoCreateBucket: cfg.StorageAutoCreateBucket},
		storagecenter.OptionsFromConfig(cfg),
	)
	probeTimeout := durationValue(cfg.SmartVideoProbeTimeout, 30*time.Second)
	processTimeout := durationValue(cfg.SmartVideoProcessTimeout, 10*time.Minute)
	probe := mediaprovider.NewFFmpegAdapter(
		mediaprovider.ExecCommandRunner{}, cfg.SmartVideoFFprobePath, cfg.SmartVideoFFmpegPath,
		probeTimeout, processTimeout,
	)
	tempDir := strings.TrimSpace(cfg.SmartVideoTempDir)
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return nil, fmt.Errorf("create smart-video temp directory: %w", err)
	}
	processor := mediaprovider.NewProcessor(fileService, probe, mediaprovider.ProcessorOptions{
		TempDir: tempDir, MaxFileBytes: int64Value(cfg.SmartVideoMaxFileBytes, 2<<30),
		MaxVideoDurationMS: int64(durationValue(cfg.SmartVideoMaxVideoDuration, 30*time.Minute) / time.Millisecond),
		MaxVideoPixels:     int64Value(cfg.SmartVideoMaxVideoPixels, 3840*2160),
		MaxImagePixels:     int64Value(cfg.SmartVideoMaxImagePixels, 80_000_000),
		ProxyMaxWidth:      intValue(cfg.SmartVideoProxyMaxWidth, 960, 64, 4096),
		ProxyVideoBitrate:  firstNonEmpty(cfg.SmartVideoProxyVideoBitrate, "1200k"),
		ProxyAudioBitrate:  firstNonEmpty(cfg.SmartVideoProxyAudioBitrate, "96k"),
	})
	concurrency := intValue(cfg.SmartVideoWorkerConcurrency, 2, 1, 32)
	workers := make([]*smartvideo.AnalysisWorker, 0, concurrency)
	for index := 0; index < concurrency; index++ {
		workers = append(workers, smartvideo.NewAnalysisWorker(repository, queue, processor, smartvideo.AnalysisWorkerOptions{
			WorkerID:      fmt.Sprintf("smartvideo_%d_%d", os.Getpid(), index+1),
			LeaseDuration: 2 * time.Minute, TaskTimeout: processTimeout, HeartbeatEvery: 30 * time.Second,
		}))
	}
	renderer := &mediaprovider.ManifestRenderer{
		Tools: probe,
		Manifests: mediaprovider.VersionManifestSource{Versions: repository},
		Store: mediaprovider.StorageRenderMediaStore{
			Open: func(ctx context.Context, access smartvideo.Access, fileID string) (io.ReadCloser, int64, string, error) {
				file, stream, err := fileService.OpenObject(ctx, storagecenter.AccessContext{
					TenantID: access.TenantID, UserID: access.UserID,
				}, fileID)
				if err != nil {
					return nil, 0, "", err
				}
				return stream, file.FileSize, file.FileHash, nil
			},
		},
		MaxOutputBytes: int64Value(os.Getenv("SMARTVIDEO_RENDER_MAX_OUTPUT_BYTES"), 512<<20),
	}
	publisher := &mediaprovider.RenderPublisher{Storage: fileService, MaxOutputBytes: renderer.MaxOutputBytes}
	points := newPersonalPointsLifecycle(db)
	works := &smartvideo.PostgresWorkPublisher{DB: db}
	settle := smartvideo.NewSettleService(repository, points, works)
	var speechPrep *smartvideo.SpeechPrepService
	speechReady := false
	if speechBase := strings.TrimSpace(firstNonEmpty(os.Getenv("SMARTVIDEO_SPEECH_BASE_URL"), cfg.ModelProviderURL, cfg.PPTProviderURL)); speechBase != "" {
		speechKey := strings.TrimSpace(firstNonEmpty(os.Getenv("SMARTVIDEO_SPEECH_API_KEY"), cfg.ModelProviderAPIKey, cfg.PPTProviderAPIKey))
		if speechKey != "" {
			speechClient := speech.NewClient(speech.Options{
				BaseURL: speechBase, APIKey: speechKey,
				DefaultModel: firstNonEmpty(os.Getenv("SMARTVIDEO_SPEECH_MODEL"), "smart-video-speech"),
				Timeout:      durationValue(os.Getenv("SMARTVIDEO_SPEECH_TIMEOUT"), 60*time.Second),
			})
			speechPrep = smartvideo.NewSpeechPrepService(&mediaprovider.VoiceCaptionBuilder{
				Synth: mediaprovider.SpeechClientAdapter{Client: speechClient},
				Store: mediaprovider.SpeechArtifactUploader{Storage: fileService},
			}, repository)
			speechReady = true
		}
	}
	renderConcurrency := intValue(cfg.SmartVideoRenderWorkerConcurrency, 1, 1, 4)
	renderWorkers := make([]*smartvideo.RenderWorker, 0, renderConcurrency)
	for index := 0; index < renderConcurrency; index++ {
		worker := smartvideo.NewRenderWorker(repository, renderQueue, renderer, publisher, smartvideo.RenderWorkerOptions{
			WorkerID: fmt.Sprintf("video_%d_%d", os.Getpid(), index+1), TempDir: tempDir,
			LeaseDuration: 2 * time.Minute, TaskTimeout: durationValue(os.Getenv("SMARTVIDEO_RENDER_TIMEOUT"), 10*time.Minute), HeartbeatEvery: 30 * time.Second,
		}).SetSettleService(settle)
		if speechPrep != nil {
			worker.SetSpeechPrep(speechPrep)
		}
		renderWorkers = append(renderWorkers, worker)
	}
	planConcurrency := intValue(cfg.SmartVideoPlanWorkerConcurrency, 1, 1, 8)
	planWorkers := make([]*smartvideo.PlanWorker, 0, planConcurrency)
	planModel := resolveSmartVideoPlanModel(cfg)
	planTimeout := durationValue(os.Getenv("SMARTVIDEO_PLAN_TIMEOUT"), 180*time.Second)
	planHTTPTimeoutMS := int(planTimeout / time.Millisecond)
	if parsed, err := strconv.Atoi(strings.TrimSpace(cfg.ModelTimeoutMS)); err == nil && parsed > planHTTPTimeoutMS {
		planHTTPTimeoutMS = parsed
	}
	if planHTTPTimeoutMS <= 0 {
		planHTTPTimeoutMS = 180000
	}
	plannerChat := chatprovider.NewOpenAICompatibleWithOptions(chatprovider.OpenAICompatibleOptions{
		Code:            "openai-compatible-chat",
		BaseURL:         firstNonEmpty(cfg.PPTProviderURL, cfg.ModelProviderURL),
		APIKey:          firstNonEmpty(cfg.PPTProviderAPIKey, cfg.ModelProviderAPIKey),
		Model:           planModel,
		Models:          []string{planModel},
		DisableThinking: cfg.PPTDisableThinking,
		TimeoutMS:       planHTTPTimeoutMS,
	})
	plannerClient := smartvideoplan.NewClient(plannerChat, smartvideoplan.Options{
		ModelKey: planModel,
		Timeout:  planTimeout,
	})
	planner := smartvideoplan.DomainAdapter{Client: plannerClient}
	planReady := strings.TrimSpace(firstNonEmpty(cfg.ModelProviderURL, cfg.PPTProviderURL, os.Getenv("SMARTVIDEO_PLAN_BASE_URL"))) != ""
	for index := 0; index < planConcurrency; index++ {
		planWorkers = append(planWorkers, smartvideo.NewPlanWorker(repository, repository, planQueue, planner, smartvideo.PlanWorkerOptions{
			WorkerID: fmt.Sprintf("plan_%d_%d", os.Getpid(), index+1),
			LeaseDuration: 2 * time.Minute, TaskTimeout: planTimeout, HeartbeatEvery: 20 * time.Second,
		}))
	}
	var outboxPublisher *infra.OutboxPublisher
	if cfg.SmartVideoOutboxEnabled {
		outboxPublisher = infra.NewOutboxPublisher(repository, infra.OutboxQueues{
			Analysis: queue, Plan: planQueue, Render: renderQueue,
		}, infra.OutboxPublisherOptions{})
	}
	return &Runtime{
		workers: workers, renderWorkers: renderWorkers, planWorkers: planWorkers,
		outboxPublisher: outboxPublisher, probe: probe, tempDir: tempDir,
		db: db, redis: redisClient, analysisQueue: queue, planQueue: planQueue, renderQueue: renderQueue,
		speechReady: speechReady, planReady: planReady,
		metricsEvery: durationValue(os.Getenv("SMARTVIDEO_METRICS_INTERVAL"), 30*time.Second),
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	ffprobeVersion, ffmpegVersion, err := r.probe.GetToolVersion(ctx)
	if err != nil {
		return fmt.Errorf("smart-video media tools unavailable: %w", err)
	}
	log.Printf("smartvideo_worker ffprobe=%q ffmpeg=%q analysis_concurrency=%d plan_concurrency=%d render_concurrency=%d outbox=%t plan_provider=%t speech_provider=%t",
		ffprobeVersion, ffmpegVersion, len(r.workers), len(r.planWorkers), len(r.renderWorkers), r.outboxPublisher != nil, r.planReady, r.speechReady)
	if err := r.refreshHealth(ctx); err != nil {
		return err
	}
	defer func() {
		healthPath := r.healthPath()
		_ = os.Remove(healthPath)
	}()

	var wg sync.WaitGroup
	errs := make(chan error, len(r.workers)+len(r.renderWorkers)+len(r.planWorkers)+1)
	for _, worker := range r.workers {
		wg.Add(1)
		go func(worker *smartvideo.AnalysisWorker) {
			defer wg.Done()
			if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
				errs <- err
			}
		}(worker)
	}
	for _, worker := range r.planWorkers {
		wg.Add(1)
		go func(worker *smartvideo.PlanWorker) {
			defer wg.Done()
			if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
				errs <- err
			}
		}(worker)
	}
	for _, worker := range r.renderWorkers {
		wg.Add(1)
		go func(worker *smartvideo.RenderWorker) {
			defer wg.Done()
			if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
				errs <- err
			}
		}(worker)
	}
	if r.outboxPublisher != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.outboxPublisher.Run(ctx); err != nil && ctx.Err() == nil {
				errs <- err
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.runHealthAndMetrics(ctx)
	}()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		<-done
		return nil
	case err := <-errs:
		return err
	case <-done:
		return nil
	}
}

func (r *Runtime) healthPath() string {
	return r.tempDir + string(os.PathSeparator) + "worker.healthy"
}

func (r *Runtime) refreshHealth(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return fmt.Errorf("smart-video worker db unhealthy: %w", err)
	}
	if err := r.redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("smart-video worker redis unhealthy: %w", err)
	}
	if _, _, err := r.probe.GetToolVersion(ctx); err != nil {
		return fmt.Errorf("smart-video worker media tools unhealthy: %w", err)
	}
	if err := os.WriteFile(r.healthPath(), []byte("ok\n"), 0o600); err != nil {
		return fmt.Errorf("write worker health marker: %w", err)
	}
	return nil
}

func (r *Runtime) runHealthAndMetrics(ctx context.Context) {
	interval := r.metricsEvery
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	r.emitMetrics(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.refreshHealth(ctx); err != nil && ctx.Err() == nil {
				log.Printf("smartvideo_worker operation=health result=failed err=%v", err)
				_ = os.Remove(r.healthPath())
				continue
			}
			r.emitMetrics(ctx)
		}
	}
}

func (r *Runtime) emitMetrics(ctx context.Context) {
	r.recoverStuckRenders(ctx)
	analysisDepth, analysisErr := r.analysisQueue.Depth(ctx)
	planDepth, planErr := r.planQueue.Depth(ctx)
	renderDepth, renderErr := r.renderQueue.Depth(ctx)
	outboxPending, outboxErr := countPendingOutbox(ctx, r.db)
	oldestAgeMS, oldestErr := oldestPendingOutboxAgeMS(ctx, r.db)
	if analysisErr != nil || planErr != nil || renderErr != nil || outboxErr != nil || oldestErr != nil {
		log.Printf("smartvideo_metrics operation=collect result=partial analysis_err=%v plan_err=%v render_err=%v outbox_err=%v oldest_err=%v",
			analysisErr, planErr, renderErr, outboxErr, oldestErr)
	}
	log.Printf("smartvideo_metrics analysis_pending=%d analysis_working=%d analysis_delayed=%d analysis_dead=%d plan_pending=%d plan_working=%d plan_delayed=%d plan_dead=%d render_pending=%d render_working=%d render_delayed=%d render_dead=%d outbox_pending=%d outbox_oldest_age_ms=%d plan_provider=%t speech_provider=%t",
		analysisDepth.Pending, analysisDepth.Working, analysisDepth.Delayed, analysisDepth.Dead,
		planDepth.Pending, planDepth.Working, planDepth.Delayed, planDepth.Dead,
		renderDepth.Pending, renderDepth.Working, renderDepth.Delayed, renderDepth.Dead,
		outboxPending, oldestAgeMS, r.planReady, r.speechReady,
	)
}

func (r *Runtime) recoverStuckRenders(ctx context.Context) {
	if r == nil || r.db == nil || r.renderQueue == nil {
		return
	}
	repo := smartvideo.NewPostgresRepository(r.db)
	ids, err := repo.RecoverExpiredRenderTasks(ctx, 20)
	if err != nil {
		log.Printf("smartvideo_render operation=recover result=failed err=%v", err)
		return
	}
	for _, id := range ids {
		if err := r.renderQueue.Enqueue(ctx, smartvideo.RenderJob{TaskID: id}, 0); err != nil {
			log.Printf("smartvideo_render operation=requeue task_id=%s result=failed err=%v", id, err)
			continue
		}
		log.Printf("smartvideo_render operation=requeue task_id=%s result=ok", id)
	}
}

func countPendingOutbox(ctx context.Context, db *sql.DB) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var count int64
	err := db.QueryRowContext(ctx, `select count(*) from video_task_outbox where state='pending'`).Scan(&count)
	return count, err
}

func oldestPendingOutboxAgeMS(ctx context.Context, db *sql.DB) (int64, error) {
	if db == nil {
		return 0, nil
	}
	var ageMS sql.NullInt64
	err := db.QueryRowContext(ctx, `
		select coalesce(extract(epoch from (now() - min(created_at))) * 1000, 0)::bigint
		from video_task_outbox
		where state='pending'`).Scan(&ageMS)
	if err != nil {
		return 0, err
	}
	if !ageMS.Valid {
		return 0, nil
	}
	return ageMS.Int64, nil
}

func durationValue(raw string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func int64Value(raw string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func intValue(raw string, fallback, minimum, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < minimum || value > maximum {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// resolveSmartVideoPlanModel maps the logical montage planner alias onto a real
// upstream chat model that the configured OpenAI-compatible provider accepts.
func resolveSmartVideoPlanModel(cfg config.Config) string {
	requested := firstNonEmpty(os.Getenv("SMARTVIDEO_PLAN_MODEL"), "smart-video-standard")
	real := firstNonEmpty(cfg.PPTTextModel, os.Getenv("MODEL_PROVIDER_TEXT_MODEL"), os.Getenv("PPT_TEXT_MODEL"), "gpt-4o-mini")
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "", "smart-video-standard", "smart_video_standard", "smart-video-plan":
		return real
	default:
		return requested
	}
}

func newPersonalPointsLifecycle(db *sql.DB) smartvideo.PointsLifecycle {
	return httpserver.NewSmartVideoPointsLifecycleFromDB(db)
}
