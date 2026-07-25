package smartvideo

import (
	"context"
	"errors"
	"log"
	"time"
)

type AnalysisWorkerOptions struct {
	WorkerID       string
	LeaseDuration  time.Duration
	TaskTimeout    time.Duration
	HeartbeatEvery time.Duration
}

type AnalysisWorker struct {
	repository Repository
	queue      AnalysisWorkerQueue
	processor  AnalysisTaskProcessor
	options    AnalysisWorkerOptions
}

func NewAnalysisWorker(repository Repository, queue AnalysisWorkerQueue, processor AnalysisTaskProcessor, options AnalysisWorkerOptions) *AnalysisWorker {
	if options.WorkerID == "" {
		options.WorkerID = newID("smartvideo_worker")
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 2 * time.Minute
	}
	if options.TaskTimeout <= 0 {
		options.TaskTimeout = 10 * time.Minute
	}
	if options.HeartbeatEvery <= 0 || options.HeartbeatEvery >= options.LeaseDuration {
		options.HeartbeatEvery = options.LeaseDuration / 3
	}
	return &AnalysisWorker{repository: repository, queue: queue, processor: processor, options: options}
}

func (w *AnalysisWorker) Run(ctx context.Context) error {
	if w.queue == nil || w.processor == nil {
		return ErrAnalysisNotReady
	}
	return w.queue.Run(ctx, w.handle)
}

func (w *AnalysisWorker) handle(parent context.Context, job AnalysisJob) QueueDecision {
	task, asset, err := w.repository.AcquireAnalysisTask(parent, job.TaskID, w.options.WorkerID, w.options.LeaseDuration)
	if errors.Is(err, ErrAnalysisNotReady) {
		stored, getErr := w.repository.GetAnalysisTask(parent, job.TaskID)
		if getErr == nil {
			now := time.Now().UTC()
			if stored.Status == AnalysisStatusRunning && stored.LeaseExpiresAt != nil && stored.LeaseExpiresAt.After(now) {
				return QueueDecision{RetryAfter: stored.LeaseExpiresAt.Sub(now) + time.Second}
			}
			if (stored.Status == AnalysisStatusPending || stored.Status == AnalysisStatusQueued) && stored.RunAfter.After(now) {
				return QueueDecision{RetryAfter: stored.RunAfter.Sub(now) + time.Second}
			}
		}
		return QueueDecision{}
	}
	if errors.Is(err, ErrNotFound) {
		return QueueDecision{}
	}
	if err != nil {
		log.Printf("smartvideo_analysis operation=acquire task_id=%s result=failed error_code=SMARTVIDEO_ANALYSIS_ACQUIRE_FAILED", job.TaskID)
		return QueueDecision{RetryAfter: 5 * time.Second}
	}
	ctx, cancel := context.WithTimeout(parent, w.options.TaskTimeout)
	defer cancel()
	stopHeartbeat := make(chan struct{})
	go w.heartbeat(ctx, task.ID, stopHeartbeat)
	result, processErr := w.processor.Process(ctx, task, asset)
	close(stopHeartbeat)
	if processErr == nil {
		if err := w.repository.CompleteAnalysisTask(parent, task.ID, w.options.WorkerID, result); err != nil {
			log.Printf("smartvideo_analysis operation=complete task_id=%s result=failed error_code=SMARTVIDEO_ANALYSIS_COMPLETE_FAILED", task.ID)
			return QueueDecision{RetryAfter: w.options.LeaseDuration + time.Second}
		}
		return QueueDecision{}
	}
	code, message, retryable := classifyAnalysisError(processErr)
	final := !retryable || task.AttemptCount >= task.MaxAttempts
	delay := analysisBackoff(task.AttemptCount)
	if err := w.repository.FailAnalysisTask(parent, task.ID, w.options.WorkerID, code, message, time.Now().UTC().Add(delay), final); err != nil {
		log.Printf("smartvideo_analysis operation=fail task_id=%s result=failed error_code=SMARTVIDEO_ANALYSIS_FAIL_UPDATE_FAILED", task.ID)
		return QueueDecision{RetryAfter: w.options.LeaseDuration + time.Second}
	}
	log.Printf("smartvideo_analysis operation=process task_id=%s asset_id=%s result=failed error_code=%s attempt=%d", task.ID, task.AssetID, code, task.AttemptCount)
	if final {
		return QueueDecision{Dead: true}
	}
	return QueueDecision{RetryAfter: delay}
}

func (w *AnalysisWorker) heartbeat(ctx context.Context, taskID string, stop <-chan struct{}) {
	ticker := time.NewTicker(w.options.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			if err := w.repository.HeartbeatAnalysisTask(ctx, taskID, w.options.WorkerID, w.options.LeaseDuration); err != nil {
				return
			}
		}
	}
}

func classifyAnalysisError(err error) (string, string, bool) {
	var mediaErr *MediaError
	if errors.As(err, &mediaErr) {
		retryable := mediaErr.Code == MediaErrorTimeout ||
			mediaErr.Code == MediaErrorDownloadFailed ||
			mediaErr.Code == MediaErrorStorageFailed ||
			mediaErr.Code == MediaErrorProbeFailed ||
			mediaErr.Code == MediaErrorPreprocessFailed
		return mediaErr.Code, mediaErr.Message, retryable
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return MediaErrorTimeout, "媒体分析超时", true
	}
	return MediaErrorProbeFailed, "媒体分析失败", true
}

func analysisBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * 5 * time.Second
}
