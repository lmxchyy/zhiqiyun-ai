package smartvideo

import (
	"context"
	"errors"
	"log"
	"os"
	"time"
)

type RenderWorkerOptions struct {
	WorkerID                                   string
	TempDir                                    string
	LeaseDuration, TaskTimeout, HeartbeatEvery time.Duration
}

type RenderWorker struct {
	repository RenderRepository
	queue      RenderWorkerQueue
	renderer   RenderProcessor
	publisher  RenderOutputPublisher
	options    RenderWorkerOptions
}

func NewRenderWorker(repository RenderRepository, queue RenderWorkerQueue, renderer RenderProcessor, publisher RenderOutputPublisher, options RenderWorkerOptions) *RenderWorker {
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 2 * time.Minute
	}
	if options.TaskTimeout <= 0 {
		options.TaskTimeout = 10 * time.Minute
	}
	if options.HeartbeatEvery <= 0 || options.HeartbeatEvery >= options.LeaseDuration {
		options.HeartbeatEvery = options.LeaseDuration / 3
	}
	return &RenderWorker{repository: repository, queue: queue, renderer: renderer, publisher: publisher, options: options}
}

func (w *RenderWorker) Run(ctx context.Context) error { return w.queue.Run(ctx, w.handle) }

func (w *RenderWorker) handle(parent context.Context, job RenderJob) QueueDecision {
	task, err := w.repository.AcquireRenderTask(parent, job.TaskID, w.options.WorkerID, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		return QueueDecision{}
	}
	if err != nil {
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	ctx, cancel := context.WithTimeout(parent, w.options.TaskTimeout)
	defer cancel()
	stop := make(chan struct{})
	go w.heartbeat(ctx, task.ID, stop)
	defer close(stop)
	workDir, err := os.MkdirTemp(w.options.TempDir, "render-"+task.ID+"-")
	if err != nil {
		return w.fail(parent, task, "SMARTVIDEO_RENDER_TEMP_FAILED", "无法创建隔离临时目录", true)
	}
	defer os.RemoveAll(workDir)
	if err = w.repository.AdvanceRenderTask(parent, task.ID, w.options.WorkerID, RenderStatusProcessing, RenderStatusRendering, "rendering", 30); err != nil {
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	artifact, err := w.renderer.Render(ctx, task, workDir)
	if err != nil {
		return w.fail(parent, task, "SMARTVIDEO_RENDER_FFMPEG_FAILED", safeRenderMessage(err), true)
	}
	if err = w.repository.AdvanceRenderTask(parent, task.ID, w.options.WorkerID, RenderStatusRendering, RenderStatusUploading, "uploading", 80); err != nil {
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	output, err := w.publisher.Publish(ctx, task, artifact)
	if err != nil {
		return w.fail(parent, task, "SMARTVIDEO_RENDER_UPLOAD_FAILED", "视频结果上传失败", true)
	}
	if _, err = w.repository.CompleteRenderTask(parent, task.ID, w.options.WorkerID, output); err != nil {
		return w.fail(parent, task, "SMARTVIDEO_RENDER_PUBLISH_FAILED", "作品登记失败", true)
	}
	log.Printf("smartvideo_render operation=complete task_id=%s result=succeeded", task.ID)
	return QueueDecision{}
}

func (w *RenderWorker) fail(ctx context.Context, task RenderTask, code, message string, retryable bool) QueueDecision {
	delay := renderBackoff(task.AttemptCount)
	retry := retryable && task.AttemptCount < task.MaxAttempts
	_ = w.repository.FailRenderTask(ctx, task.ID, w.options.WorkerID, code, message, time.Now().Add(delay), retry)
	if !retry {
		return QueueDecision{Dead: true}
	}
	return QueueDecision{RetryAfter: delay}
}

func (w *RenderWorker) heartbeat(ctx context.Context, taskID string, stop <-chan struct{}) {
	ticker := time.NewTicker(w.options.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.repository.HeartbeatRenderTask(ctx, taskID, w.options.WorkerID, w.options.LeaseDuration); err != nil {
				return
			}
		}
	}
}

func renderBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * 5 * time.Second
}

func safeRenderMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "视频渲染超时"
	}
	var mediaErr *MediaError
	if errors.As(err, &mediaErr) {
		return mediaErr.Message
	}
	return "FFmpeg 视频渲染失败"
}
