package smartvideoruntime

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/infra"
	mediaprovider "xianzhi-ai/backend-go/internal/provider/media"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type Runtime struct {
	workers       []*smartvideo.AnalysisWorker
	renderWorkers []*smartvideo.RenderWorker
	probe         *mediaprovider.FFmpegAdapter
	tempDir       string
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
	renderer := &mediaprovider.SmokeRenderer{
		Tools: probe, FontPath: firstNonEmpty(os.Getenv("SMARTVIDEO_RENDER_FONT_PATH"), "/usr/share/fonts/noto/NotoSansCJK-Regular.ttc"),
		MaxOutputBytes: int64Value(os.Getenv("SMARTVIDEO_RENDER_MAX_OUTPUT_BYTES"), 512<<20),
	}
	publisher := &mediaprovider.RenderPublisher{Storage: fileService, MaxOutputBytes: renderer.MaxOutputBytes}
	renderConcurrency := intValue(os.Getenv("SMARTVIDEO_RENDER_CONCURRENCY"), 1, 1, 4)
	renderWorkers := make([]*smartvideo.RenderWorker, 0, renderConcurrency)
	for index := 0; index < renderConcurrency; index++ {
		renderWorkers = append(renderWorkers, smartvideo.NewRenderWorker(repository, renderQueue, renderer, publisher, smartvideo.RenderWorkerOptions{
			WorkerID: fmt.Sprintf("video_%d_%d", os.Getpid(), index+1), TempDir: tempDir,
			LeaseDuration: 2 * time.Minute, TaskTimeout: durationValue(os.Getenv("SMARTVIDEO_RENDER_TIMEOUT"), 10*time.Minute), HeartbeatEvery: 30 * time.Second,
		}))
	}
	return &Runtime{workers: workers, renderWorkers: renderWorkers, probe: probe, tempDir: tempDir}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	ffprobeVersion, ffmpegVersion, err := r.probe.GetToolVersion(ctx)
	if err != nil {
		return fmt.Errorf("smart-video media tools unavailable: %w", err)
	}
	log.Printf("smartvideo_worker ffprobe=%q ffmpeg=%q analysis_concurrency=%d render_concurrency=%d", ffprobeVersion, ffmpegVersion, len(r.workers), len(r.renderWorkers))
	healthPath := r.tempDir + string(os.PathSeparator) + "worker.healthy"
	if err := os.WriteFile(healthPath, []byte("ok\n"), 0o600); err != nil {
		return fmt.Errorf("write worker health marker: %w", err)
	}
	defer func() { _ = os.Remove(healthPath) }()
	var wg sync.WaitGroup
	errs := make(chan error, len(r.workers)+len(r.renderWorkers))
	for _, worker := range r.workers {
		wg.Add(1)
		go func(worker *smartvideo.AnalysisWorker) {
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
