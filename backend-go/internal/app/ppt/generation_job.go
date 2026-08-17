package ppt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	GenerationJobQueued                    = "QUEUED"
	GenerationJobRunning                   = "RUNNING"
	GenerationJobRetryWait                 = "RETRY_WAIT"
	GenerationJobSucceeded                 = "SUCCEEDED"
	GenerationJobFailed                    = "FAILED"
	GenerationJobCancelled                 = "CANCELLED"
	GenerationJobWaitingForOutlineApproval = "WAITING_FOR_OUTLINE_APPROVAL"

	GenerationWorkflowRender       = "RENDER"
	GenerationWorkflowAgentOutline = "AGENT_OUTLINE"

	GenerationStageCreated          = "CREATED"
	GenerationStageTaskLoaded       = "TASK_LOADED"
	GenerationStageRendered         = "RENDERED"
	GenerationStageFileStored       = "FILE_STORED"
	GenerationStageAssetCreated     = "ASSET_CREATED"
	GenerationStageTaskRelated      = "TASK_RELATED"
	GenerationStageCompleted        = "COMPLETED"
	GenerationStageIntentResolved   = "INTENT_RESOLVED"
	GenerationStageResearched       = "RESEARCHED"
	GenerationStageStorylinePlanned = "STORYLINE_PLANNED"
	GenerationStageOutlinePlanned   = "OUTLINE_PLANNED"
	GenerationStageOutlineApproved  = "OUTLINE_APPROVED"
	GenerationStageContentReady     = "CONTENT_READY"
	GenerationStageAssetsReady      = "ASSETS_READY"
	GenerationStageLayoutCompiled   = "LAYOUT_COMPILED"
	GenerationStageQualityChecked   = "QUALITY_CHECKED"

	GenerationChildPending   = "PENDING"
	GenerationChildRunning   = "RUNNING"
	GenerationChildSucceeded = "SUCCEEDED"
	GenerationChildFailed    = "FAILED"
	GenerationChildCancelled = "CANCELLED"

	GenerationAttemptRunning   = "RUNNING"
	GenerationAttemptRetryWait = "RETRY_WAIT"
	GenerationAttemptSucceeded = "SUCCEEDED"
	GenerationAttemptFailed    = "FAILED"
	GenerationAttemptCancelled = "CANCELLED"

	GenerationTotalWorkUnits = 5
)

var (
	ErrGenerationJobNotFound                = errors.New("ppt v2 generation job not found")
	ErrGenerationJobIdempotencyConflict     = errors.New("ppt v2 generation job idempotency conflict")
	ErrGenerationJobTerminal                = errors.New("ppt v2 generation job is terminal")
	ErrGenerationJobTransition              = errors.New("ppt v2 generation job transition is invalid")
	ErrGenerationJobLeaseHeld               = errors.New("ppt v2 generation job lease is held")
	ErrGenerationJobLeaseLost               = errors.New("ppt v2 generation job lease is lost")
	ErrGenerationJobNotReady                = errors.New("ppt v2 generation job retry is not ready")
	ErrGenerationJobCancelled               = errors.New("ppt v2 generation job is cancelled")
	ErrGenerationJobInvalid                 = errors.New("ppt v2 generation job is invalid")
	ErrGenerationJobAwaitingOutlineApproval = errors.New("ppt v2 generation job is awaiting outline approval")
	ErrStaleOutlineRevision                 = errors.New("ppt v2 outline revision is stale")
)

type GenerationJobScope struct {
	TenantID string
	UserID   string
}

type CreateGenerationJobInput struct {
	JobID           string
	TenantID        string
	UserID          string
	OrganizationID  string
	ExistingTaskID  string
	ClientRequestID string
	IdempotencyKey  string
	MaxAttempts     int
	SlideCount      int
	WorkflowType    string
	InputSnapshot   []byte
	Now             time.Time
}

type GenerationJobError struct {
	Code              string    `json:"code"`
	Message           string    `json:"message"`
	Stage             string    `json:"stage"`
	Retryable         bool      `json:"retryable"`
	AttemptID         string    `json:"attemptId,omitempty"`
	UsageID           string    `json:"usageIdentity,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	ProviderRequestID string    `json:"providerRequestId,omitempty"`
	OccurredAt        time.Time `json:"occurredAt,omitempty"`
}

type GenerationJob struct {
	ID                 string
	WorkflowType       string
	TenantID           string
	UserID             string
	OrganizationID     string
	ExistingTaskID     string
	ClientRequestID    string
	IdempotencyKey     string
	Status             string
	Stage              string
	AttemptCount       int
	MaxAttempts        int
	RunAfter           time.Time
	LeaseOwner         string
	LeaseExpiresAt     time.Time
	FencingToken       int64
	CompletedWorkUnits int
	TotalWorkUnits     int
	DeckJobID          string
	InputSnapshot      []byte
	DeckID             string
	Revision           int
	SlideCount         int
	RenderSHA256       string
	RenderBytes        []byte
	FileID             string
	AssetID            string
	LastError          *GenerationJobError
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          time.Time
	FinishedAt         time.Time
	CancelRequestedAt  time.Time
}

func (j GenerationJob) Terminal() bool {
	return j.Status == GenerationJobSucceeded || j.Status == GenerationJobFailed || j.Status == GenerationJobCancelled
}

func (j GenerationJob) Progress() int {
	if j.TotalWorkUnits <= 0 {
		return 0
	}
	progress := j.CompletedWorkUnits * 100 / j.TotalWorkUnits
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

type DeckJob struct {
	ID              string
	GenerationJobID string
	DeckID          string
	Revision        int
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type SlideJob struct {
	ID                 string
	GenerationJobID    string
	DeckJobID          string
	SlideIndex         int
	SourceSlideID      string
	Status             string
	CompletedWorkUnits int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type GenerationAttempt struct {
	ID            string
	JobID         string
	AttemptNumber int
	WorkerID      string
	FencingToken  int64
	Status        string
	UsageIdentity string
	Error         *GenerationJobError
	StartedAt     time.Time
	FinishedAt    time.Time
}

type GenerationTransition struct {
	JobID        string
	AttemptID    string
	FromStage    string
	ToStage      string
	FencingToken int64
	Checkpoint   map[string]any
	CreatedAt    time.Time
}

type GenerationLease struct {
	JobID          string
	TenantID       string
	UserID         string
	WorkerID       string
	AttemptID      string
	FencingToken   int64
	LeaseExpiresAt time.Time
	Job            GenerationJob
}

type GenerationCheckpoint struct {
	NextStage      string
	InputSnapshot  []byte
	DeckID         string
	Revision       int
	SlideCount     int
	RenderSHA256   string
	RenderBytes    []byte
	FileID         string
	AssetID        string
	SourceSlideIDs []string
	Now            time.Time
}

type GenerationJobBundle struct {
	Job      GenerationJob
	Deck     DeckJob
	Slides   []SlideJob
	Attempts []GenerationAttempt
	History  []GenerationTransition
}

type GenerationJobStore interface {
	Create(context.Context, CreateGenerationJobInput) (GenerationJob, bool, error)
	Get(context.Context, GenerationJobScope, string) (GenerationJobBundle, error)
	Claim(context.Context, GenerationJobScope, string, string, time.Time, time.Duration) (GenerationLease, error)
	Renew(context.Context, GenerationLease, time.Time, time.Duration) (GenerationLease, error)
	Checkpoint(context.Context, GenerationLease, GenerationCheckpoint) (GenerationJob, error)
	Fail(context.Context, GenerationLease, GenerationJobError, time.Time, time.Duration) (GenerationJob, error)
	Cancel(context.Context, GenerationJobScope, string, time.Time) (GenerationJob, error)
}

type GenerationTaskRelationStore interface {
	RelateTaskArtifact(context.Context, GenerationLease, V2ArtifactRelation, time.Time) (GenerationJob, error)
}

func NormalizeCreateGenerationJob(input CreateGenerationJobInput) (GenerationJob, DeckJob, []SlideJob, error) {
	input.JobID = strings.TrimSpace(input.JobID)
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.UserID = strings.TrimSpace(input.UserID)
	input.OrganizationID = strings.TrimSpace(input.OrganizationID)
	input.ExistingTaskID = strings.TrimSpace(input.ExistingTaskID)
	input.ClientRequestID = strings.TrimSpace(input.ClientRequestID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.WorkflowType = strings.TrimSpace(input.WorkflowType)
	if input.WorkflowType == "" {
		input.WorkflowType = GenerationWorkflowRender
	}
	if input.TenantID == "" || input.UserID == "" || input.IdempotencyKey == "" {
		return GenerationJob{}, DeckJob{}, nil, ErrGenerationJobInvalid
	}
	if input.MaxAttempts <= 0 {
		input.MaxAttempts = 3
	}
	if input.MaxAttempts > 20 || input.SlideCount <= 0 {
		return GenerationJob{}, DeckJob{}, nil, ErrGenerationJobInvalid
	}
	if input.WorkflowType == GenerationWorkflowRender && input.ExistingTaskID == "" {
		return GenerationJob{}, DeckJob{}, nil, ErrGenerationJobInvalid
	}
	if input.WorkflowType == GenerationWorkflowAgentOutline {
		if input.ExistingTaskID != "" || input.SlideCount < AgentMinimumPageCount || input.SlideCount > AgentMaximumPageCount {
			return GenerationJob{}, DeckJob{}, nil, ErrGenerationJobInvalid
		}
	} else if input.WorkflowType != GenerationWorkflowRender {
		return GenerationJob{}, DeckJob{}, nil, ErrGenerationJobInvalid
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	} else {
		input.Now = input.Now.UTC()
	}
	if input.JobID == "" {
		input.JobID = newGenerationJobID()
	}
	deckJobID := ""
	totalWorkUnits := GenerationTotalWorkUnits
	if input.WorkflowType == GenerationWorkflowRender {
		deckJobID = input.JobID + ":deck"
	} else {
		totalWorkUnits = 3
	}
	job := GenerationJob{
		ID: input.JobID, WorkflowType: input.WorkflowType, TenantID: input.TenantID, UserID: input.UserID, OrganizationID: input.OrganizationID,
		ExistingTaskID: input.ExistingTaskID, ClientRequestID: input.ClientRequestID, IdempotencyKey: input.IdempotencyKey,
		Status: GenerationJobQueued, Stage: GenerationStageCreated, MaxAttempts: input.MaxAttempts,
		RunAfter: input.Now, TotalWorkUnits: totalWorkUnits, DeckJobID: deckJobID,
		InputSnapshot: append([]byte(nil), input.InputSnapshot...),
		SlideCount:    input.SlideCount, CreatedAt: input.Now, UpdatedAt: input.Now,
	}
	if input.WorkflowType == GenerationWorkflowAgentOutline {
		return job, DeckJob{}, nil, nil
	}
	deck := DeckJob{ID: deckJobID, GenerationJobID: input.JobID, Status: GenerationChildPending, CreatedAt: input.Now, UpdatedAt: input.Now}
	slides := make([]SlideJob, 0, input.SlideCount)
	for index := 1; index <= input.SlideCount; index++ {
		slides = append(slides, SlideJob{
			ID: fmt.Sprintf("%s:slide:%d", input.JobID, index), GenerationJobID: input.JobID,
			DeckJobID: deckJobID, SlideIndex: index, Status: GenerationChildPending,
			CreatedAt: input.Now, UpdatedAt: input.Now,
		})
	}
	return job, deck, slides, nil
}

func generationAttemptID(jobID string, attempt int) string {
	return fmt.Sprintf("%s:attempt:%d", jobID, attempt)
}

func generationStageWorkUnits(stage string) int {
	switch stage {
	case GenerationStageTaskLoaded:
		return 1
	case GenerationStageRendered:
		return 2
	case GenerationStageFileStored:
		return 3
	case GenerationStageAssetCreated:
		return 4
	case GenerationStageTaskRelated, GenerationStageCompleted:
		return 5
	default:
		return 0
	}
}

func validGenerationStageTransition(from, to string) bool {
	return (from == GenerationStageCreated && to == GenerationStageTaskLoaded) ||
		(from == GenerationStageTaskLoaded && to == GenerationStageRendered) ||
		(from == GenerationStageRendered && to == GenerationStageFileStored) ||
		(from == GenerationStageFileStored && to == GenerationStageAssetCreated) ||
		(from == GenerationStageAssetCreated && to == GenerationStageTaskRelated) ||
		(from == GenerationStageTaskRelated && to == GenerationStageCompleted)
}

func validateGenerationCheckpoint(job GenerationJob, checkpoint GenerationCheckpoint) error {
	if !validGenerationStageTransition(job.Stage, checkpoint.NextStage) {
		return ErrGenerationJobTransition
	}
	switch checkpoint.NextStage {
	case GenerationStageTaskLoaded:
		if len(checkpoint.InputSnapshot) == 0 {
			return ErrGenerationJobInvalid
		}
	case GenerationStageRendered:
		if strings.TrimSpace(checkpoint.DeckID) == "" || checkpoint.Revision <= 0 || checkpoint.SlideCount != job.SlideCount || strings.TrimSpace(checkpoint.RenderSHA256) == "" || len(checkpoint.RenderBytes) == 0 {
			return ErrGenerationJobInvalid
		}
	case GenerationStageFileStored:
		if strings.TrimSpace(checkpoint.FileID) == "" {
			return ErrGenerationJobInvalid
		}
	case GenerationStageAssetCreated:
		if strings.TrimSpace(checkpoint.AssetID) == "" {
			return ErrGenerationJobInvalid
		}
	}
	return nil
}

func applyGenerationCheckpoint(job *GenerationJob, deck *DeckJob, slides []SlideJob, checkpoint GenerationCheckpoint) {
	job.Stage = checkpoint.NextStage
	job.CompletedWorkUnits = generationStageWorkUnits(checkpoint.NextStage)
	job.UpdatedAt = checkpoint.Now
	switch checkpoint.NextStage {
	case GenerationStageTaskLoaded:
		job.InputSnapshot = append([]byte(nil), checkpoint.InputSnapshot...)
		for index := range slides {
			slides[index].Status = GenerationChildRunning
			slides[index].UpdatedAt = checkpoint.Now
			if index < len(checkpoint.SourceSlideIDs) {
				slides[index].SourceSlideID = strings.TrimSpace(checkpoint.SourceSlideIDs[index])
			}
		}
	case GenerationStageRendered:
		job.DeckID = strings.TrimSpace(checkpoint.DeckID)
		job.Revision = checkpoint.Revision
		job.RenderSHA256 = strings.TrimSpace(checkpoint.RenderSHA256)
		job.RenderBytes = append([]byte(nil), checkpoint.RenderBytes...)
		deck.DeckID = job.DeckID
		deck.Revision = job.Revision
		deck.Status = GenerationChildRunning
		deck.UpdatedAt = checkpoint.Now
		for index := range slides {
			slides[index].Status = GenerationChildSucceeded
			slides[index].CompletedWorkUnits = 1
			slides[index].UpdatedAt = checkpoint.Now
		}
	case GenerationStageFileStored:
		job.FileID = strings.TrimSpace(checkpoint.FileID)
	case GenerationStageAssetCreated:
		job.AssetID = strings.TrimSpace(checkpoint.AssetID)
	case GenerationStageCompleted:
		job.Status = GenerationJobSucceeded
		job.FinishedAt = checkpoint.Now
		job.LeaseOwner = ""
		job.LeaseExpiresAt = time.Time{}
		deck.Status = GenerationChildSucceeded
		deck.UpdatedAt = checkpoint.Now
	}
}

func cloneGenerationError(input *GenerationJobError) *GenerationJobError {
	if input == nil {
		return nil
	}
	copyValue := *input
	return &copyValue
}

func cloneGenerationJob(job GenerationJob) GenerationJob {
	job.InputSnapshot = append([]byte(nil), job.InputSnapshot...)
	job.RenderBytes = append([]byte(nil), job.RenderBytes...)
	job.LastError = cloneGenerationError(job.LastError)
	return job
}

func newGenerationJobID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err == nil {
		return "pptv2_job_" + hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("pptv2_job_%d", time.Now().UTC().UnixNano())
}

type MemoryGenerationJobStore struct {
	mu          sync.Mutex
	jobs        map[string]GenerationJob
	byKey       map[string]string
	byTask      map[string]string
	decks       map[string]DeckJob
	slides      map[string][]SlideJob
	attempts    map[string][]GenerationAttempt
	transitions map[string][]GenerationTransition
	agentPlans  map[string]AgentPlanningRecord
}

func NewMemoryGenerationJobStore() *MemoryGenerationJobStore {
	return &MemoryGenerationJobStore{
		jobs: map[string]GenerationJob{}, byKey: map[string]string{}, byTask: map[string]string{}, decks: map[string]DeckJob{},
		slides: map[string][]SlideJob{}, attempts: map[string][]GenerationAttempt{}, transitions: map[string][]GenerationTransition{}, agentPlans: map[string]AgentPlanningRecord{},
	}
}

func generationIdempotencyMapKey(tenantID, userID, key string) string {
	return strings.TrimSpace(tenantID) + "\x00" + strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(key)
}

func (s *MemoryGenerationJobStore) Create(_ context.Context, input CreateGenerationJobInput) (GenerationJob, bool, error) {
	job, deck, slides, err := NormalizeCreateGenerationJob(input)
	if err != nil {
		return GenerationJob{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createNormalizedLocked(job, deck, slides)
}

func (s *MemoryGenerationJobStore) createNormalizedLocked(job GenerationJob, deck DeckJob, slides []SlideJob) (GenerationJob, bool, error) {
	key := generationIdempotencyMapKey(job.TenantID, job.UserID, job.IdempotencyKey)
	if existingID := s.byKey[key]; existingID != "" {
		existing := s.jobs[existingID]
		if existing.WorkflowType != job.WorkflowType || existing.ExistingTaskID != job.ExistingTaskID || existing.OrganizationID != job.OrganizationID || existing.SlideCount != job.SlideCount ||
			(job.WorkflowType == GenerationWorkflowAgentOutline && existing.ClientRequestID != job.ClientRequestID) {
			return GenerationJob{}, false, ErrGenerationJobIdempotencyConflict
		}
		return cloneGenerationJob(existing), false, nil
	}
	if job.ExistingTaskID != "" && s.byTask[job.ExistingTaskID] != "" {
		return GenerationJob{}, false, ErrGenerationJobIdempotencyConflict
	}
	s.jobs[job.ID] = job
	s.byKey[key] = job.ID
	if job.ExistingTaskID != "" {
		s.byTask[job.ExistingTaskID] = job.ID
	}
	if deck.ID != "" {
		s.decks[job.ID] = deck
		s.slides[job.ID] = append([]SlideJob(nil), slides...)
	}
	s.transitions[job.ID] = []GenerationTransition{{JobID: job.ID, FromStage: "", ToStage: GenerationStageCreated, Checkpoint: map[string]any{"completedWorkUnits": 0}, CreatedAt: job.CreatedAt}}
	return cloneGenerationJob(job), true, nil
}

func (s *MemoryGenerationJobStore) Get(_ context.Context, scope GenerationJobScope, jobID string) (GenerationJobBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bundleLocked(scope, jobID)
}

func (s *MemoryGenerationJobStore) bundleLocked(scope GenerationJobScope, jobID string) (GenerationJobBundle, error) {
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return GenerationJobBundle{}, ErrGenerationJobNotFound
	}
	bundle := GenerationJobBundle{Job: cloneGenerationJob(job), Deck: s.decks[job.ID]}
	bundle.Slides = append([]SlideJob(nil), s.slides[job.ID]...)
	bundle.Attempts = append([]GenerationAttempt(nil), s.attempts[job.ID]...)
	for index := range bundle.Attempts {
		bundle.Attempts[index].Error = cloneGenerationError(bundle.Attempts[index].Error)
	}
	bundle.History = append([]GenerationTransition(nil), s.transitions[job.ID]...)
	return bundle, nil
}

func (s *MemoryGenerationJobStore) Claim(_ context.Context, scope GenerationJobScope, jobID, workerID string, now time.Time, duration time.Duration) (GenerationLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return GenerationLease{}, ErrGenerationJobNotFound
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || duration <= 0 {
		return GenerationLease{}, ErrGenerationJobInvalid
	}
	now = now.UTC()
	if job.Terminal() {
		if job.Status == GenerationJobCancelled {
			return GenerationLease{}, ErrGenerationJobCancelled
		}
		return GenerationLease{}, ErrGenerationJobTerminal
	}
	if job.Status == GenerationJobWaitingForOutlineApproval {
		return GenerationLease{}, ErrGenerationJobAwaitingOutlineApproval
	}
	if job.Status == GenerationJobRunning && job.LeaseExpiresAt.After(now) {
		return GenerationLease{}, ErrGenerationJobLeaseHeld
	}
	if job.Status == GenerationJobRetryWait && job.RunAfter.After(now) {
		return GenerationLease{}, ErrGenerationJobNotReady
	}
	if job.AttemptCount >= job.MaxAttempts {
		failure := GenerationJobError{Code: "LEASE_EXPIRED", Message: "generation worker lease expired", Stage: job.Stage, Retryable: false, AttemptID: generationAttemptID(job.ID, job.AttemptCount)}
		job.LastError = &failure
		job.Status = GenerationJobFailed
		job.FinishedAt = now
		job.UpdatedAt = now
		s.jobs[job.ID] = job
		s.finishAttemptLocked(job.ID, failure.AttemptID, GenerationAttemptFailed, &failure, now)
		return GenerationLease{}, ErrGenerationJobTerminal
	}
	if job.Status == GenerationJobRunning && !job.LeaseExpiresAt.After(now) && job.AttemptCount > 0 {
		failure := GenerationJobError{Code: "LEASE_EXPIRED", Message: "generation worker lease expired", Stage: job.Stage, Retryable: true, AttemptID: generationAttemptID(job.ID, job.AttemptCount)}
		s.finishAttemptLocked(job.ID, failure.AttemptID, GenerationAttemptRetryWait, &failure, now)
	}
	job.Status = GenerationJobRunning
	job.AttemptCount++
	job.FencingToken++
	job.LeaseOwner = workerID
	job.LeaseExpiresAt = now.Add(duration)
	job.UpdatedAt = now
	if job.StartedAt.IsZero() {
		job.StartedAt = now
	}
	attempt := GenerationAttempt{
		ID: generationAttemptID(job.ID, job.AttemptCount), JobID: job.ID, AttemptNumber: job.AttemptCount,
		WorkerID: workerID, FencingToken: job.FencingToken, Status: GenerationAttemptRunning, StartedAt: now,
	}
	s.attempts[job.ID] = append(s.attempts[job.ID], attempt)
	if job.DeckJobID != "" {
		deck := s.decks[job.ID]
		deck.Status = GenerationChildRunning
		deck.UpdatedAt = now
		s.decks[job.ID] = deck
	}
	s.jobs[job.ID] = job
	return GenerationLease{JobID: job.ID, TenantID: job.TenantID, UserID: job.UserID, WorkerID: workerID, AttemptID: attempt.ID, FencingToken: job.FencingToken, LeaseExpiresAt: job.LeaseExpiresAt, Job: cloneGenerationJob(job)}, nil
}

func (s *MemoryGenerationJobStore) Renew(_ context.Context, lease GenerationLease, now time.Time, duration time.Duration) (GenerationLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.validLeaseLocked(lease, now)
	if err != nil {
		return GenerationLease{}, err
	}
	if duration <= 0 {
		return GenerationLease{}, ErrGenerationJobInvalid
	}
	job.LeaseExpiresAt = now.UTC().Add(duration)
	job.UpdatedAt = now.UTC()
	s.jobs[job.ID] = job
	lease.LeaseExpiresAt = job.LeaseExpiresAt
	lease.Job = cloneGenerationJob(job)
	return lease, nil
}

func (s *MemoryGenerationJobStore) Checkpoint(_ context.Context, lease GenerationLease, checkpoint GenerationCheckpoint) (GenerationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if checkpoint.Now.IsZero() {
		checkpoint.Now = time.Now().UTC()
	} else {
		checkpoint.Now = checkpoint.Now.UTC()
	}
	job, err := s.validLeaseLocked(lease, checkpoint.Now)
	if err != nil {
		return GenerationJob{}, err
	}
	if err := validateGenerationCheckpoint(job, checkpoint); err != nil {
		return GenerationJob{}, err
	}
	fromStage := job.Stage
	deck := s.decks[job.ID]
	slides := append([]SlideJob(nil), s.slides[job.ID]...)
	applyGenerationCheckpoint(&job, &deck, slides, checkpoint)
	s.jobs[job.ID] = job
	s.decks[job.ID] = deck
	s.slides[job.ID] = slides
	s.transitions[job.ID] = append(s.transitions[job.ID], GenerationTransition{
		JobID: job.ID, AttemptID: lease.AttemptID, FromStage: fromStage, ToStage: checkpoint.NextStage,
		FencingToken: lease.FencingToken, Checkpoint: map[string]any{"completedWorkUnits": job.CompletedWorkUnits}, CreatedAt: checkpoint.Now,
	})
	if checkpoint.NextStage == GenerationStageCompleted {
		s.finishAttemptLocked(job.ID, lease.AttemptID, GenerationAttemptSucceeded, nil, checkpoint.Now)
	}
	return cloneGenerationJob(job), nil
}

func (s *MemoryGenerationJobStore) Fail(_ context.Context, lease GenerationLease, failure GenerationJobError, now time.Time, retryDelay time.Duration) (GenerationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	job, err := s.validLeaseLocked(lease, now)
	if err != nil {
		return GenerationJob{}, err
	}
	failure.Code = strings.TrimSpace(failure.Code)
	failure.Message = strings.TrimSpace(failure.Message)
	failure.Stage = job.Stage
	failure.AttemptID = lease.AttemptID
	if failure.Code == "" || failure.Message == "" {
		return GenerationJob{}, ErrGenerationJobInvalid
	}
	job.LastError = cloneGenerationError(&failure)
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	if failure.Retryable && job.AttemptCount < job.MaxAttempts {
		job.Status = GenerationJobRetryWait
		job.RunAfter = now.Add(retryDelay)
		s.finishAttemptLocked(job.ID, lease.AttemptID, GenerationAttemptRetryWait, &failure, now)
	} else {
		job.Status = GenerationJobFailed
		job.FinishedAt = now
		if job.DeckJobID != "" {
			deck := s.decks[job.ID]
			deck.Status = GenerationChildFailed
			deck.UpdatedAt = now
			s.decks[job.ID] = deck
		}
		s.finishAttemptLocked(job.ID, lease.AttemptID, GenerationAttemptFailed, &failure, now)
	}
	s.jobs[job.ID] = job
	return cloneGenerationJob(job), nil
}

func (s *MemoryGenerationJobStore) Cancel(_ context.Context, scope GenerationJobScope, jobID string, now time.Time) (GenerationJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return GenerationJob{}, ErrGenerationJobNotFound
	}
	if job.Terminal() {
		return cloneGenerationJob(job), nil
	}
	now = now.UTC()
	job.Status = GenerationJobCancelled
	job.CancelRequestedAt = now
	job.FinishedAt = now
	job.UpdatedAt = now
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	if job.DeckJobID != "" {
		deck := s.decks[job.ID]
		if deck.Status != GenerationChildSucceeded {
			deck.Status = GenerationChildCancelled
			deck.UpdatedAt = now
			s.decks[job.ID] = deck
		}
	}
	slides := s.slides[job.ID]
	for index := range slides {
		if slides[index].Status != GenerationChildSucceeded {
			slides[index].Status = GenerationChildCancelled
			slides[index].UpdatedAt = now
		}
	}
	s.slides[job.ID] = slides
	if job.AttemptCount > 0 {
		s.finishAttemptLocked(job.ID, generationAttemptID(job.ID, job.AttemptCount), GenerationAttemptCancelled, nil, now)
	}
	s.jobs[job.ID] = job
	return cloneGenerationJob(job), nil
}

func (s *MemoryGenerationJobStore) validLeaseLocked(lease GenerationLease, now time.Time) (GenerationJob, error) {
	job, ok := s.jobs[lease.JobID]
	if !ok || job.TenantID != lease.TenantID || job.UserID != lease.UserID {
		return GenerationJob{}, ErrGenerationJobNotFound
	}
	if job.Terminal() {
		if job.Status == GenerationJobCancelled {
			return GenerationJob{}, ErrGenerationJobCancelled
		}
		return GenerationJob{}, ErrGenerationJobTerminal
	}
	if job.Status != GenerationJobRunning || job.LeaseOwner != lease.WorkerID || job.FencingToken != lease.FencingToken || !job.LeaseExpiresAt.After(now.UTC()) {
		return GenerationJob{}, ErrGenerationJobLeaseLost
	}
	return cloneGenerationJob(job), nil
}

func (s *MemoryGenerationJobStore) finishAttemptLocked(jobID, attemptID, status string, failure *GenerationJobError, now time.Time) {
	attempts := s.attempts[jobID]
	for index := range attempts {
		if attempts[index].ID != attemptID {
			continue
		}
		attempts[index].Status = status
		attempts[index].Error = cloneGenerationError(failure)
		attempts[index].FinishedAt = now
		break
	}
	s.attempts[jobID] = attempts
}

var _ GenerationJobStore = (*MemoryGenerationJobStore)(nil)
