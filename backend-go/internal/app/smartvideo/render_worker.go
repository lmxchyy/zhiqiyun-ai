package smartvideo

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
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
	speech     *SpeechPrepService
	settle     *SettleService
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

func (w *RenderWorker) SetSpeechPrep(speech *SpeechPrepService) *RenderWorker {
	w.speech = speech
	return w
}

func (w *RenderWorker) SetSettleService(settle *SettleService) *RenderWorker {
	w.settle = settle
	return w
}

func (w *RenderWorker) Run(ctx context.Context) error { return w.queue.Run(ctx, w.handle) }

func (w *RenderWorker) handle(parent context.Context, job RenderJob) QueueDecision {
	task, err := w.repository.AcquireRenderTask(parent, job.TaskID, w.options.WorkerID, w.options.LeaseDuration)
	if errors.Is(err, ErrNotFound) {
		// Another worker may still hold a fresh lease after a crash/restart. Retry
		// instead of silently ACKing the Redis job, otherwise the task becomes orphaned.
		log.Printf("smartvideo_render operation=acquire task_id=%s result=not_found retry=20s", job.TaskID)
		return QueueDecision{RetryAfter: 20 * time.Second}
	}
	if err != nil {
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	log.Printf("smartvideo_render operation=acquired task_id=%s worker=%s status=%s", task.ID, w.options.WorkerID, task.Status)
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

	if err = w.repository.AdvanceRenderTask(parent, task.ID, w.options.WorkerID, RenderStatusProcessing, RenderStatusSynthesizing, "synthesizing", 20); err != nil {
		log.Printf("smartvideo_render operation=advance task_id=%s from=%s to=%s err=%v", task.ID, RenderStatusProcessing, RenderStatusSynthesizing, err)
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	if w.speech != nil {
		access := Access{TenantID: task.TenantID, UserID: task.UserID}
		artifacts, prepErr := w.speech.Prepare(ctx, access, task)
		if prepErr != nil {
			// NewAPI may not expose TTS yet; skip narration instead of failing the whole export.
			if isSkippableSpeechError(prepErr) {
				log.Printf("smartvideo_render operation=speech task_id=%s result=skip err=%v", task.ID, prepErr)
			} else {
				return w.fail(parent, task, "SMARTVIDEO_SPEECH_FAILED", safeSpeechMessage(prepErr), true)
			}
		} else if !artifacts.Skipped {
			if err = w.repository.AttachVoiceCaptionArtifacts(parent, task.ID, w.options.WorkerID, artifacts.VoiceFileID, artifacts.CaptionFileID); err != nil {
				return QueueDecision{RetryAfter: 5 * time.Second}
			}
			task.VoiceFileID = artifacts.VoiceFileID
			task.CaptionFileID = artifacts.CaptionFileID
		}
	}

	if err = w.repository.AdvanceRenderTask(parent, task.ID, w.options.WorkerID, RenderStatusSynthesizing, RenderStatusRendering, "rendering", 40); err != nil {
		log.Printf("smartvideo_render operation=advance task_id=%s from=%s to=%s err=%v", task.ID, RenderStatusSynthesizing, RenderStatusRendering, err)
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	artifact, err := w.renderer.Render(ctx, task, workDir)
	if err != nil {
		return w.fail(parent, task, "SMARTVIDEO_RENDER_FFMPEG_FAILED", safeRenderMessage(err), true)
	}
	if err = w.repository.AdvanceRenderTask(parent, task.ID, w.options.WorkerID, RenderStatusRendering, RenderStatusUploading, "uploading", 80); err != nil {
		log.Printf("smartvideo_render operation=advance task_id=%s from=%s to=%s err=%v", task.ID, RenderStatusRendering, RenderStatusUploading, err)
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	var output RenderOutput
	if task.OutputFileID != "" {
		output = RenderOutput{
			VideoFileID: task.OutputFileID, CoverFileID: task.CoverFileID,
			DurationMS: task.Output.DurationMS, Width: task.Output.Width, Height: task.Output.Height,
			FrameRate: task.Output.FrameRate, FileSize: task.Output.FileSize,
			VideoCodec: task.Output.VideoCodec, AudioCodec: task.Output.AudioCodec, PixelFormat: task.Output.PixelFormat,
		}
		if output.DurationMS == 0 {
			output.DurationMS = artifact.DurationMS
			output.Width, output.Height, output.FrameRate = artifact.Width, artifact.Height, artifact.FrameRate
			output.FileSize, output.VideoCodec, output.AudioCodec, output.PixelFormat =
				artifact.FileSize, artifact.VideoCodec, artifact.AudioCodec, artifact.PixelFormat
		}
	} else {
		output, err = w.publisher.Publish(ctx, task, artifact)
		if err != nil {
			return w.fail(parent, task, "SMARTVIDEO_RENDER_UPLOAD_FAILED", "视频结果上传失败", true)
		}
	}
	if w.settle != nil {
		if _, err = w.settle.SettleSuccess(parent, task, output); err != nil {
			return w.fail(parent, task, "SMARTVIDEO_RENDER_PUBLISH_FAILED", "作品登记或积分结算失败", true)
		}
	} else if _, err = w.repository.CompleteRenderTask(parent, task.ID, w.options.WorkerID, output); err != nil {
		return w.fail(parent, task, "SMARTVIDEO_RENDER_PUBLISH_FAILED", "作品登记失败", true)
	}
	log.Printf("smartvideo_render operation=complete task_id=%s result=succeeded", task.ID)
	return QueueDecision{}
}

func (w *RenderWorker) fail(ctx context.Context, task RenderTask, code, message string, retryable bool) QueueDecision {
	delay := renderBackoff(task.AttemptCount)
	retry := retryable && task.AttemptCount < task.MaxAttempts
	_ = w.repository.FailRenderTask(ctx, task.ID, w.options.WorkerID, code, message, time.Now().Add(delay), retry)
	if !retry && w.settle != nil {
		_ = w.settle.SettleFinalFailure(ctx, task)
	}
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

func safeSpeechMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "配音生成超时"
	}
	if errors.Is(err, ErrSpeechNotReady) {
		return "配音服务未就绪"
	}
	return "配音或字幕生成失败"
}

func isSkippableSpeechError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrSpeechNotReady) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, token := range []string{
		"provider_unavailable",
		"invalid_model",
		"invalid_voice",
		"model_not_found",
		"no available channel",
		"smart_video_speech_not_ready",
	} {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}
