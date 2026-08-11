package smartvideo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrPlanDailyLimitExceeded = errors.New("SMART_VIDEO_PLAN_DAILY_LIMIT")
	ErrPlanNotReady           = errors.New("SMART_VIDEO_PLAN_NOT_READY")
)

type PlanService struct {
	repository PlanRepository
	projects   Repository
	versions   VersionRepository
	planner    EditPlanner
	now        func() time.Time
}

func NewPlanService(projects Repository, plans PlanRepository, versions VersionRepository, planner EditPlanner) *PlanService {
	return &PlanService{
		repository: plans,
		projects:   projects,
		versions:   versions,
		planner:    planner,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

type CreatePlanTaskInput struct {
	Instruction             string `json:"instruction"`
	RegenerateFromVersionID string `json:"regenerateFromVersionId"`
	IdempotencyKey          string `json:"idempotencyKey"`
	ModelKey                string `json:"modelKey"`
}

func (s *PlanService) CreatePlanTask(ctx context.Context, access Access, projectID string, input CreatePlanTaskInput) (PlanTask, error) {
	if s == nil || s.repository == nil || s.projects == nil {
		return PlanTask{}, ErrPlanNotReady
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" {
		return PlanTask{}, ErrIdempotencyKeyRequired
	}
	project, err := s.projects.GetProject(ctx, access, strings.TrimSpace(projectID))
	if err != nil {
		return PlanTask{}, err
	}
	switch project.Status {
	case ProjectStatusMaterialReady, ProjectStatusStoryboardReady:
	default:
		return PlanTask{}, fmt.Errorf("%w: project status %s cannot plan", ErrInvalidStateTransition, project.Status)
	}
	if err := s.enforceDailyLimit(ctx, access); err != nil {
		return PlanTask{}, err
	}
	modelKey := strings.TrimSpace(input.ModelKey)
	if modelKey == "" {
		modelKey = "smart-video-standard"
	}
	now := s.now()
	task := PlanTask{
		ID: newID("svplan"), TenantID: access.TenantID, ProjectID: project.ID, UserID: access.UserID,
		State: PlanStatusCreated, Instruction: strings.TrimSpace(input.Instruction),
		SourceVersionID: strings.TrimSpace(input.RegenerateFromVersionID),
		ModelKey: modelKey, Attempt: 1, IdempotencyKey: input.IdempotencyKey, CreatedAt: now,
	}
	payload, _ := json.Marshal(map[string]string{"taskId": task.ID})
	err = s.repository.CreatePlanTaskWithOutbox(ctx, task, OutboxEvent{
		TenantID: access.TenantID, AggregateType: "plan", AggregateID: task.ID,
		EventType: "enqueue_requested", Payload: payload,
	})
	if errors.Is(err, ErrIdempotencyConflict) {
		return s.repository.GetPlanTaskByIdempotencyKey(ctx, access, input.IdempotencyKey)
	}
	return task, err
}

func (s *PlanService) GetPlanTask(ctx context.Context, access Access, taskID string) (PlanTask, error) {
	return s.repository.GetPlanTask(ctx, access, strings.TrimSpace(taskID))
}

func (s *PlanService) enforceDailyLimit(ctx context.Context, access Access) error {
	counter, ok := s.projects.(interface {
		CountSuccessfulPlansToday(context.Context, Access) (int, error)
	})
	if !ok {
		return nil
	}
	count, err := counter.CountSuccessfulPlansToday(ctx, access)
	if err != nil {
		return err
	}
	if count >= PlanDailyLimit {
		return ErrPlanDailyLimitExceeded
	}
	return nil
}

type PlanWorkerOptions struct {
	WorkerID       string
	LeaseDuration  time.Duration
	TaskTimeout    time.Duration
	HeartbeatEvery time.Duration
}

type PlanWorker struct {
	plans    PlanRepository
	projects Repository
	queue    PlanWorkerQueue
	planner  EditPlanner
	options  PlanWorkerOptions
}

func NewPlanWorker(projects Repository, plans PlanRepository, queue PlanWorkerQueue, planner EditPlanner, options PlanWorkerOptions) *PlanWorker {
	if options.WorkerID == "" {
		options.WorkerID = newID("plan_worker")
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 2 * time.Minute
	}
	if options.TaskTimeout <= 0 {
		options.TaskTimeout = 3 * time.Minute
	}
	if options.HeartbeatEvery <= 0 || options.HeartbeatEvery >= options.LeaseDuration {
		options.HeartbeatEvery = options.LeaseDuration / 3
	}
	return &PlanWorker{plans: plans, projects: projects, queue: queue, planner: planner, options: options}
}

func (w *PlanWorker) Run(ctx context.Context) error {
	if w.queue == nil || w.planner == nil {
		return ErrPlanNotReady
	}
	return w.queue.Run(ctx, w.handle)
}

func (w *PlanWorker) handle(parent context.Context, job PlanJob) QueueDecision {
	task, err := w.plans.ClaimPlanTask(parent, job.TaskID, w.options.WorkerID, w.options.LeaseDuration)
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

	access := Access{TenantID: task.TenantID, UserID: task.UserID}
	project, err := w.projects.GetProject(ctx, access, task.ProjectID)
	if err != nil {
		_ = w.plans.FailPlanTask(parent, task.ID, w.options.WorkerID, "invalid_state", "project missing")
		return QueueDecision{Dead: true}
	}
	assets, err := w.projects.ListAssets(ctx, access, task.ProjectID)
	if err != nil {
		return w.fail(parent, task, "asset_not_owned", err.Error(), true)
	}
	plan, usage, err := w.planner.Plan(ctx, PlanRequest{
		ProjectID: project.ID, Requirement: project.Requirement, TargetSpec: project.TargetSpec,
		Assets: assets, Instruction: task.Instruction, SourceVersionID: task.SourceVersionID,
	})
	if err != nil {
		code, retryable := classifyPlanError(err)
		return w.fail(parent, task, code, safePlanMessage(err), retryable)
	}
	version := ProjectVersion{
		ID: newID("svv"), ProjectID: project.ID, TenantID: project.TenantID,
		VersionNumber: project.CurrentVersion + 1, Source: VersionSourceAI,
		ParentVersionID: task.SourceVersionID, PlanSchemaVersion: EditPlanSchemaVersion,
		PlanSnapshot: plan, PlannerModelKey: firstNonEmpty(usage.ModelKey, task.ModelKey),
		PlannerRequestID: usage.ProviderRequestID, Requirement: project.Requirement,
		CreatedBy: task.UserID, CreatedAt: time.Now().UTC(),
	}
	if err := w.plans.CompletePlanTask(parent, task.ID, w.options.WorkerID, version); err != nil {
		return QueueDecision{RetryAfter: w.options.LeaseDuration + time.Second}
	}
	return QueueDecision{}
}

func (w *PlanWorker) fail(ctx context.Context, task PlanTask, code, message string, retryable bool) QueueDecision {
	_ = w.plans.FailPlanTask(ctx, task.ID, w.options.WorkerID, code, message)
	if !retryable {
		return QueueDecision{Dead: true}
	}
	return QueueDecision{RetryAfter: 10 * time.Second}
}

func (w *PlanWorker) heartbeat(ctx context.Context, taskID string, stop <-chan struct{}) {
	ticker := time.NewTicker(w.options.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.plans.HeartbeatPlanTask(ctx, taskID, w.options.WorkerID, w.options.LeaseDuration); err != nil {
				return
			}
		}
	}
}

func classifyPlanError(err error) (string, bool) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "rate_limited"), strings.Contains(msg, "429"):
		return "provider_rate_limited", true
	case strings.Contains(msg, "unavailable"), strings.Contains(msg, "timeout"), strings.Contains(msg, "5"):
		return "provider_unavailable", true
	case strings.Contains(msg, "invalid_json"), strings.Contains(msg, "invalid_plan"), strings.Contains(msg, "invalid_schema"):
		return "invalid_plan", false
	default:
		return "provider_unavailable", true
	}
}

func safePlanMessage(err error) string {
	if err == nil {
		return "规划失败"
	}
	msg := err.Error()
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
