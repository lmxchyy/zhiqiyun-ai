package ppt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

type ResearchProvider interface {
	Research(context.Context, IntentSpec) (ResearchPack, error)
}

type AgentPlanningRecord struct {
	Intent                 IntentSpec
	Research               ResearchPack
	Storyline              Storyline
	Outline                OutlinePlan
	ApprovedOutline        *OutlinePlan
	ResearchExecutionCount int
	Revisions              map[int]OutlinePlan
}

type AgentPlanningState struct {
	Job                    AgentPlanningJob `json:"job"`
	Intent                 IntentSpec       `json:"intent"`
	Research               ResearchPack     `json:"research"`
	Storyline              Storyline        `json:"storyline"`
	Outline                OutlinePlan      `json:"outline"`
	ApprovedOutline        *OutlinePlan     `json:"approvedOutline,omitempty"`
	ResearchExecutionCount int              `json:"researchExecutionCount"`
}

type AgentPlanningJob struct {
	ID                 string    `json:"id"`
	WorkflowType       string    `json:"workflowType"`
	TenantID           string    `json:"tenantId"`
	UserID             string    `json:"userId"`
	OrganizationID     string    `json:"organizationId"`
	Status             string    `json:"status"`
	Stage              string    `json:"stage"`
	CompletedWorkUnits int       `json:"completedWorkUnits"`
	TotalWorkUnits     int       `json:"totalWorkUnits"`
	SlideCount         int       `json:"slideCount"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (j AgentPlanningJob) Progress() int {
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

type AgentPlanningStore interface {
	GenerationJobStore
	SaveAgentIntent(context.Context, GenerationLease, IntentSpec, time.Time) (GenerationLease, error)
	SaveAgentResearch(context.Context, GenerationLease, ResearchPack, time.Time) (GenerationLease, error)
	SaveAgentStoryline(context.Context, GenerationLease, Storyline, time.Time) (GenerationLease, error)
	SaveAgentOutline(context.Context, GenerationLease, OutlinePlan, time.Time) (AgentPlanningState, error)
	GetAgentPlanning(context.Context, GenerationJobScope, string) (AgentPlanningState, error)
	UpdateAgentOutline(context.Context, GenerationJobScope, string, int, []OutlineEditCommand, time.Time) (AgentPlanningState, error)
	ApproveAgentOutline(context.Context, GenerationJobScope, string, int, time.Time) (AgentPlanningState, error)
}

type AgentPlanningOptions struct {
	WorkerID      string
	LeaseDuration time.Duration
}

type AgentPlanningService struct {
	store         AgentPlanningStore
	research      ResearchProvider
	workerID      string
	leaseDuration time.Duration
}

type GuideAgentRequest struct {
	TenantID       string
	UserID         string
	OrganizationID string
	IdempotencyKey string
	Request        IntentRequest
	Now            time.Time
}

type AgentGuideResult struct {
	ClarificationQuestions []string            `json:"clarificationQuestions,omitempty"`
	State                  *AgentPlanningState `json:"state,omitempty"`
}

func NewAgentPlanningService(store AgentPlanningStore, research ResearchProvider, options AgentPlanningOptions) (*AgentPlanningService, error) {
	if store == nil || research == nil {
		return nil, ErrGenerationJobInvalid
	}
	options.WorkerID = strings.TrimSpace(options.WorkerID)
	if options.WorkerID == "" {
		options.WorkerID = "ppt_v2_agent_planner"
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = time.Minute
	}
	return &AgentPlanningService{store: store, research: research, workerID: options.WorkerID, leaseDuration: options.LeaseDuration}, nil
}

func (s *AgentPlanningService) Guide(ctx context.Context, request GuideAgentRequest) (AgentGuideResult, error) {
	resolution := InterpretAgentIntent(request.Request)
	if len(resolution.ClarificationQuestions) > 0 {
		return AgentGuideResult{ClarificationQuestions: append([]string(nil), resolution.ClarificationQuestions...)}, nil
	}
	if resolution.Intent == nil {
		return AgentGuideResult{}, ErrGenerationJobInvalid
	}
	now := normalizedAgentTime(request.Now)
	intent := *resolution.Intent
	pageCount := plannedAgentPageCount(intent)
	job, created, err := s.store.Create(ctx, CreateGenerationJobInput{
		TenantID: request.TenantID, UserID: request.UserID, OrganizationID: request.OrganizationID,
		ClientRequestID: agentGuideRequestIdentity(request.Request), IdempotencyKey: request.IdempotencyKey,
		SlideCount: pageCount, WorkflowType: GenerationWorkflowAgentOutline, Now: now,
	})
	if err != nil {
		return AgentGuideResult{}, err
	}
	scope := GenerationJobScope{TenantID: job.TenantID, UserID: job.UserID}
	if !created && (job.Status == GenerationJobWaitingForOutlineApproval || job.Stage == GenerationStageOutlineApproved) {
		state, err := s.store.GetAgentPlanning(ctx, scope, job.ID)
		if err != nil {
			return AgentGuideResult{}, err
		}
		return AgentGuideResult{State: &state}, nil
	}
	lease, err := s.store.Claim(ctx, scope, job.ID, s.workerID, now, s.leaseDuration)
	if err != nil {
		return AgentGuideResult{}, err
	}
	if lease.Job.Stage == GenerationStageCreated {
		lease, err = s.store.SaveAgentIntent(ctx, lease, intent, now)
		if err != nil {
			return AgentGuideResult{}, err
		}
	}
	if lease.Job.Stage == GenerationStageIntentResolved {
		var research ResearchPack
		if intent.ResearchRequired {
			lease, err = s.store.Renew(ctx, lease, now, s.leaseDuration)
			if err != nil {
				return AgentGuideResult{}, err
			}
			research, err = s.research.Research(ctx, intent)
			if err != nil {
				_, _ = s.store.Fail(ctx, lease, GenerationJobError{Code: "RESEARCH_FAILED", Message: err.Error(), Retryable: true}, now, time.Minute)
				return AgentGuideResult{}, err
			}
		}
		research, err = NormalizeResearchPack(research)
		if err != nil {
			return AgentGuideResult{}, err
		}
		lease, err = s.store.SaveAgentResearch(ctx, lease, research, now)
		if err != nil {
			return AgentGuideResult{}, err
		}
	}
	state, err := s.store.GetAgentPlanning(ctx, scope, job.ID)
	if err != nil {
		return AgentGuideResult{}, err
	}
	if lease.Job.Stage == GenerationStageResearched {
		storyline, buildErr := BuildProfessionalStoryline(state.Intent, state.Research)
		if buildErr != nil {
			return AgentGuideResult{}, buildErr
		}
		lease, err = s.store.SaveAgentStoryline(ctx, lease, storyline, now)
		if err != nil {
			return AgentGuideResult{}, err
		}
		state.Storyline = storyline
	}
	if lease.Job.Stage == GenerationStageStorylinePlanned {
		outline, buildErr := BuildDynamicOutlinePlan(job.ID, state.Intent, state.Research, state.Storyline, now)
		if buildErr != nil {
			return AgentGuideResult{}, buildErr
		}
		state, err = s.store.SaveAgentOutline(ctx, lease, outline, now)
		if err != nil {
			return AgentGuideResult{}, err
		}
	}
	return AgentGuideResult{State: &state}, nil
}

func (s *AgentPlanningService) Get(ctx context.Context, scope GenerationJobScope, jobID string) (AgentPlanningState, error) {
	return s.store.GetAgentPlanning(ctx, scope, jobID)
}

func (s *AgentPlanningService) UpdateOutline(ctx context.Context, scope GenerationJobScope, jobID string, expectedRevision int, commands []OutlineEditCommand, now time.Time) (AgentPlanningState, error) {
	if expectedRevision <= 0 || len(commands) == 0 {
		return AgentPlanningState{}, ErrInvalidOutlinePlan
	}
	return s.store.UpdateAgentOutline(ctx, scope, jobID, expectedRevision, commands, normalizedAgentTime(now))
}

func (s *AgentPlanningService) ApproveOutline(ctx context.Context, scope GenerationJobScope, jobID string, expectedRevision int, now time.Time) (AgentPlanningState, error) {
	if expectedRevision <= 0 {
		return AgentPlanningState{}, ErrInvalidOutlinePlan
	}
	return s.store.ApproveAgentOutline(ctx, scope, jobID, expectedRevision, normalizedAgentTime(now))
}

func plannedAgentPageCount(intent IntentSpec) int {
	pageCount := intent.PageCount.Preferred
	if !intent.PageCount.Explicit || pageCount == 0 {
		pageCount = len(professionalStorylineDefinitions(intent.Goal)) + 2
	}
	if pageCount < AgentMinimumPageCount {
		return AgentMinimumPageCount
	}
	if pageCount > AgentMaximumPageCount {
		return AgentMaximumPageCount
	}
	return pageCount
}

func agentGuideRequestIdentity(request IntentRequest) string {
	payload, _ := json.Marshal(request)
	digest := sha256.Sum256(payload)
	return "ppt_agent_guide_" + hex.EncodeToString(digest[:])
}

func normalizedAgentTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func cloneAgentPlanningRecord(input AgentPlanningRecord) AgentPlanningRecord {
	input.Research = cloneResearchPack(input.Research)
	input.Storyline = cloneStoryline(input.Storyline)
	input.Outline = cloneOutlinePlan(input.Outline)
	if input.ApprovedOutline != nil {
		approved := cloneOutlinePlan(*input.ApprovedOutline)
		input.ApprovedOutline = &approved
	}
	if input.Revisions != nil {
		revisions := make(map[int]OutlinePlan, len(input.Revisions))
		for revision, outline := range input.Revisions {
			revisions[revision] = cloneOutlinePlan(outline)
		}
		input.Revisions = revisions
	}
	return input
}

func cloneResearchPack(input ResearchPack) ResearchPack {
	input.Sources = append([]ResearchSource(nil), input.Sources...)
	input.Citations = append([]ResearchCitation(nil), input.Citations...)
	input.Claims = append([]ResearchClaim(nil), input.Claims...)
	for index := range input.Claims {
		input.Claims[index].CitationRefs = append([]string(nil), input.Claims[index].CitationRefs...)
	}
	input.Datasets = append([]ResearchDataset(nil), input.Datasets...)
	for index := range input.Datasets {
		input.Datasets[index].CitationRefs = append([]string(nil), input.Datasets[index].CitationRefs...)
	}
	return input
}

func cloneStoryline(input Storyline) Storyline {
	input.NarrativeArc = append([]string(nil), input.NarrativeArc...)
	input.Sections = append([]StorylineSection(nil), input.Sections...)
	for index := range input.Sections {
		input.Sections[index].EvidenceRefs = append([]string(nil), input.Sections[index].EvidenceRefs...)
	}
	return input
}

func agentPlanningState(job GenerationJob, record AgentPlanningRecord) AgentPlanningState {
	record = cloneAgentPlanningRecord(record)
	return AgentPlanningState{
		Job: AgentPlanningJob{
			ID: job.ID, WorkflowType: job.WorkflowType, TenantID: job.TenantID, UserID: job.UserID,
			OrganizationID: job.OrganizationID, Status: job.Status, Stage: job.Stage,
			CompletedWorkUnits: job.CompletedWorkUnits, TotalWorkUnits: job.TotalWorkUnits,
			SlideCount: job.SlideCount, UpdatedAt: job.UpdatedAt,
		}, Intent: record.Intent, Research: record.Research,
		Storyline: record.Storyline, Outline: record.Outline, ApprovedOutline: record.ApprovedOutline,
		ResearchExecutionCount: record.ResearchExecutionCount,
	}
}

func (s *MemoryGenerationJobStore) SaveAgentIntent(_ context.Context, lease GenerationLease, intent IntentSpec, now time.Time) (GenerationLease, error) {
	return s.saveAgentPlanningStage(lease, GenerationStageCreated, GenerationStageIntentResolved, 1, now, func(record *AgentPlanningRecord) error {
		if strings.TrimSpace(intent.Topic) == "" {
			return ErrGenerationJobInvalid
		}
		record.Intent = intent
		return nil
	})
}

func (s *MemoryGenerationJobStore) SaveAgentResearch(_ context.Context, lease GenerationLease, research ResearchPack, now time.Time) (GenerationLease, error) {
	if err := ValidateResearchPack(research); err != nil {
		return GenerationLease{}, err
	}
	return s.saveAgentPlanningStage(lease, GenerationStageIntentResolved, GenerationStageResearched, 2, now, func(record *AgentPlanningRecord) error {
		record.Research = cloneResearchPack(research)
		record.ResearchExecutionCount++
		return nil
	})
}

func (s *MemoryGenerationJobStore) SaveAgentStoryline(_ context.Context, lease GenerationLease, storyline Storyline, now time.Time) (GenerationLease, error) {
	if strings.TrimSpace(storyline.Thesis) == "" || len(storyline.Sections) == 0 || strings.TrimSpace(storyline.ClosingAction) == "" {
		return GenerationLease{}, ErrInvalidStoryline
	}
	return s.saveAgentPlanningStage(lease, GenerationStageResearched, GenerationStageStorylinePlanned, 2, now, func(record *AgentPlanningRecord) error {
		record.Storyline = cloneStoryline(storyline)
		return nil
	})
}

func (s *MemoryGenerationJobStore) SaveAgentOutline(_ context.Context, lease GenerationLease, outline OutlinePlan, now time.Time) (AgentPlanningState, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.validLeaseLocked(lease, now)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline || job.Stage != GenerationStageStorylinePlanned {
		return AgentPlanningState{}, ErrGenerationJobTransition
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	if err := ValidateOutlinePlan(outline, record.Research); err != nil {
		return AgentPlanningState{}, err
	}
	fromStage := job.Stage
	record.Outline = cloneOutlinePlan(outline)
	record.Revisions = map[int]OutlinePlan{outline.Revision: cloneOutlinePlan(outline)}
	job.Stage = GenerationStageOutlinePlanned
	job.Status = GenerationJobWaitingForOutlineApproval
	job.CompletedWorkUnits = job.TotalWorkUnits
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	s.agentPlans[job.ID] = record
	s.jobs[job.ID] = job
	s.transitions[job.ID] = append(s.transitions[job.ID], GenerationTransition{
		JobID: job.ID, AttemptID: lease.AttemptID, FromStage: fromStage, ToStage: job.Stage,
		FencingToken: lease.FencingToken, Checkpoint: map[string]any{"completedWorkUnits": job.CompletedWorkUnits, "outlineRevision": outline.Revision}, CreatedAt: now,
	})
	s.finishAttemptLocked(job.ID, lease.AttemptID, GenerationAttemptSucceeded, nil, now)
	return agentPlanningState(job, record), nil
}

func (s *MemoryGenerationJobStore) GetAgentPlanning(_ context.Context, scope GenerationJobScope, jobID string) (AgentPlanningState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.WorkflowType != GenerationWorkflowAgentOutline || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	return agentPlanningState(job, s.agentPlans[job.ID]), nil
}

func (s *MemoryGenerationJobStore) UpdateAgentOutline(_ context.Context, scope GenerationJobScope, jobID string, expectedRevision int, commands []OutlineEditCommand, now time.Time) (AgentPlanningState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, record, err := s.agentPlanningForUpdateLocked(scope, jobID)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if record.ApprovedOutline != nil {
		return AgentPlanningState{}, ErrOutlinePlanApproved
	}
	if record.Outline.Revision != expectedRevision {
		return AgentPlanningState{}, ErrStaleOutlineRevision
	}
	updated, err := ApplyOutlineCommands(record.Outline, commands, record.Research)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if record.Revisions == nil {
		record.Revisions = map[int]OutlinePlan{}
	}
	record.Outline = cloneOutlinePlan(updated)
	record.Revisions[updated.Revision] = cloneOutlinePlan(updated)
	job.SlideCount = updated.PageCount
	job.UpdatedAt = normalizedAgentTime(now)
	s.agentPlans[job.ID] = record
	s.jobs[job.ID] = job
	return agentPlanningState(job, record), nil
}

func (s *MemoryGenerationJobStore) ApproveAgentOutline(_ context.Context, scope GenerationJobScope, jobID string, expectedRevision int, now time.Time) (AgentPlanningState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, record, err := s.agentPlanningForUpdateLocked(scope, jobID)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if record.ApprovedOutline != nil {
		if record.ApprovedOutline.Revision == expectedRevision {
			return agentPlanningState(job, record), nil
		}
		return AgentPlanningState{}, ErrStaleOutlineRevision
	}
	if record.Outline.Revision != expectedRevision {
		return AgentPlanningState{}, ErrStaleOutlineRevision
	}
	approved := cloneOutlinePlan(record.Outline)
	approved.ApprovedAt = normalizedAgentTime(now)
	record.Outline = cloneOutlinePlan(approved)
	record.ApprovedOutline = &approved
	record.Revisions[approved.Revision] = cloneOutlinePlan(approved)
	job.Status = GenerationJobQueued
	job.Stage = GenerationStageOutlineApproved
	job.RunAfter = approved.ApprovedAt
	job.UpdatedAt = approved.ApprovedAt
	s.agentPlans[job.ID] = record
	s.jobs[job.ID] = job
	s.transitions[job.ID] = append(s.transitions[job.ID], GenerationTransition{
		JobID: job.ID, FromStage: GenerationStageOutlinePlanned, ToStage: GenerationStageOutlineApproved,
		Checkpoint: map[string]any{"completedWorkUnits": job.CompletedWorkUnits, "outlineRevision": approved.Revision}, CreatedAt: approved.ApprovedAt,
	})
	return agentPlanningState(job, record), nil
}

func (s *MemoryGenerationJobStore) saveAgentPlanningStage(lease GenerationLease, expectedStage, nextStage string, completedWorkUnits int, now time.Time, update func(*AgentPlanningRecord) error) (GenerationLease, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.validLeaseLocked(lease, now)
	if err != nil {
		return GenerationLease{}, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline || job.Stage != expectedStage {
		return GenerationLease{}, ErrGenerationJobTransition
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	if err := update(&record); err != nil {
		return GenerationLease{}, err
	}
	fromStage := job.Stage
	job.Stage = nextStage
	job.CompletedWorkUnits = completedWorkUnits
	job.UpdatedAt = now
	s.agentPlans[job.ID] = record
	s.jobs[job.ID] = job
	s.transitions[job.ID] = append(s.transitions[job.ID], GenerationTransition{
		JobID: job.ID, AttemptID: lease.AttemptID, FromStage: fromStage, ToStage: nextStage,
		FencingToken: lease.FencingToken, Checkpoint: map[string]any{"completedWorkUnits": completedWorkUnits}, CreatedAt: now,
	})
	lease.Job = cloneGenerationJob(job)
	return lease, nil
}

func (s *MemoryGenerationJobStore) agentPlanningForUpdateLocked(scope GenerationJobScope, jobID string) (GenerationJob, AgentPlanningRecord, error) {
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.WorkflowType != GenerationWorkflowAgentOutline || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return GenerationJob{}, AgentPlanningRecord{}, ErrGenerationJobNotFound
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	if job.Stage == GenerationStageOutlineApproved || record.ApprovedOutline != nil {
		return job, record, nil
	}
	if job.Status != GenerationJobWaitingForOutlineApproval || job.Stage != GenerationStageOutlinePlanned {
		return GenerationJob{}, AgentPlanningRecord{}, ErrGenerationJobTransition
	}
	return job, record, nil
}

var _ AgentPlanningStore = (*MemoryGenerationJobStore)(nil)
