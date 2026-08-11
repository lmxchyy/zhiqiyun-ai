package smartvideo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryPlanRepo struct {
	mu    sync.Mutex
	tasks map[string]PlanTask
	byKey map[string]string
}

func newMemoryPlanRepo() *memoryPlanRepo {
	return &memoryPlanRepo{tasks: map[string]PlanTask{}, byKey: map[string]string{}}
}

func (r *memoryPlanRepo) CreatePlanTaskWithOutbox(_ context.Context, task PlanTask, _ OutboxEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := task.TenantID + "\x00" + task.UserID + "\x00" + task.IdempotencyKey
	if _, ok := r.byKey[key]; ok {
		return ErrIdempotencyConflict
	}
	r.tasks[task.ID] = task
	r.byKey[key] = task.ID
	return nil
}

func (r *memoryPlanRepo) GetPlanTask(_ context.Context, access Access, taskID string) (PlanTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.TenantID != access.TenantID || task.UserID != access.UserID {
		return PlanTask{}, ErrNotFound
	}
	return task, nil
}

func (r *memoryPlanRepo) GetPlanTaskByIdempotencyKey(_ context.Context, access Access, key string) (PlanTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byKey[access.TenantID+"\x00"+access.UserID+"\x00"+key]
	if !ok {
		return PlanTask{}, ErrNotFound
	}
	return r.tasks[id], nil
}

func (r *memoryPlanRepo) ClaimPlanTask(_ context.Context, taskID, workerID string, _ time.Duration) (PlanTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return PlanTask{}, ErrNotFound
	}
	if task.State != PlanStatusCreated && task.State != PlanStatusQueued {
		return PlanTask{}, ErrNotFound
	}
	task.State = PlanStatusProcessing
	task.LeaseOwner = workerID
	task.Attempt++
	r.tasks[taskID] = task
	return task, nil
}

func (r *memoryPlanRepo) HeartbeatPlanTask(_ context.Context, taskID, workerID string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.LeaseOwner != workerID || task.State != PlanStatusProcessing {
		return ErrAnalysisLeaseLost
	}
	return nil
}

func (r *memoryPlanRepo) CompletePlanTask(_ context.Context, taskID, workerID string, version ProjectVersion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.LeaseOwner != workerID || task.State != PlanStatusProcessing {
		return ErrInvalidStateTransition
	}
	task.State = PlanStatusSucceeded
	task.OutputVersionID = version.ID
	task.PlanSnapshot = version.PlanSnapshot
	task.Progress = 100
	r.tasks[taskID] = task
	return nil
}

func (r *memoryPlanRepo) FailPlanTask(_ context.Context, taskID, workerID, code, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrNotFound
	}
	if workerID != "" && task.LeaseOwner != workerID {
		return ErrAnalysisLeaseLost
	}
	task.State = PlanStatusFailed
	task.ErrorCode, task.ErrorMessage = code, message
	r.tasks[taskID] = task
	return nil
}

type stubPlanner struct {
	plan EditPlanV1
	err  error
}

func (s stubPlanner) Plan(context.Context, PlanRequest) (EditPlanV1, PlanProviderUsage, error) {
	return s.plan, PlanProviderUsage{ModelKey: "smart-video-standard", ProviderRequestID: "req_1"}, s.err
}

type memoryPlanQueue struct {
	jobs []PlanJob
}

func (q *memoryPlanQueue) Enqueue(_ context.Context, job PlanJob, _ time.Duration) error {
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *memoryPlanQueue) Run(ctx context.Context, handler func(context.Context, PlanJob) QueueDecision) error {
	for _, job := range q.jobs {
		handler(ctx, job)
	}
	return nil
}

func TestPlanServiceOnlyAllowsMaterialOrStoryboardReady(t *testing.T) {
	service, repository, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	plans := newMemoryPlanRepo()
	planService := NewPlanService(repository, plans, nil, stubPlanner{})
	_, err = planService.CreatePlanTask(context.Background(), access, project.ID, CreatePlanTaskInput{IdempotencyKey: "k1"})
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("draft plan error = %v", err)
	}
	project.Status = ProjectStatusMaterialReady
	project.TargetSpec = TargetSpec{AspectRatio: TargetAspectRatio9x16, Resolution: TargetResolution720p, DurationMs: 15000}
	if _, err := repository.UpdateProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	task, err := planService.CreatePlanTask(context.Background(), access, project.ID, CreatePlanTaskInput{
		IdempotencyKey: "k1", Instruction: "更紧凑",
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := planService.CreatePlanTask(context.Background(), access, project.ID, CreatePlanTaskInput{IdempotencyKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != task.ID {
		t.Fatalf("idempotent ids differ: %s vs %s", again.ID, task.ID)
	}
}

func TestPlanWorkerCompletesValidatedVersion(t *testing.T) {
	service, repository, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	project.Status = ProjectStatusMaterialReady
	project.Requirement = "开业促销"
	project.TargetSpec = TargetSpec{AspectRatio: TargetAspectRatio9x16, Resolution: TargetResolution720p, DurationMs: 15000}
	if _, err := repository.UpdateProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	asset, err := service.CreateAsset(context.Background(), access, project.ID, CreateAssetInput{FileID: "file_1", AssetType: AssetTypeImage})
	if err != nil {
		t.Fatal(err)
	}
	plan := makeValidEditPlanV1()
	plan.Scenes[0].Clips[0].AssetID = asset.ID
	plans := newMemoryPlanRepo()
	queue := &memoryPlanQueue{}
	worker := NewPlanWorker(repository, plans, queue, stubPlanner{plan: plan}, PlanWorkerOptions{WorkerID: "w1"})
	planService := NewPlanService(repository, plans, nil, stubPlanner{plan: plan})
	task, err := planService.CreatePlanTask(context.Background(), access, project.ID, CreatePlanTaskInput{IdempotencyKey: "plan-1"})
	if err != nil {
		t.Fatal(err)
	}
	queue.jobs = append(queue.jobs, PlanJob{TaskID: task.ID})
	if err := worker.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	done, err := plans.GetPlanTask(context.Background(), access, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != PlanStatusSucceeded || done.OutputVersionID == "" {
		t.Fatalf("unexpected task: %+v", done)
	}
}

func TestPlanWorkerFailsInvalidPlanWithoutRetryLoop(t *testing.T) {
	service, repository, access := newTestService()
	project, err := service.CreateProject(context.Background(), access, CreateProjectInput{Title: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	project.Status = ProjectStatusMaterialReady
	if _, err := repository.UpdateProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	plans := newMemoryPlanRepo()
	queue := &memoryPlanQueue{}
	worker := NewPlanWorker(repository, plans, queue, stubPlanner{err: errors.New("invalid_json")}, PlanWorkerOptions{WorkerID: "w1"})
	planService := NewPlanService(repository, plans, nil, stubPlanner{})
	task, err := planService.CreatePlanTask(context.Background(), access, project.ID, CreatePlanTaskInput{IdempotencyKey: "plan-fail"})
	if err != nil {
		t.Fatal(err)
	}
	queue.jobs = append(queue.jobs, PlanJob{TaskID: task.ID})
	_ = worker.Run(context.Background())
	done, err := plans.GetPlanTask(context.Background(), access, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != PlanStatusFailed || done.ErrorCode != "invalid_plan" {
		t.Fatalf("unexpected failed task: %+v", done)
	}
}
