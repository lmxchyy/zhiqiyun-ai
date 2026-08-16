package ppt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	DeckGeneration         AgentDeckGenerationState
}

type AgentPlanningState struct {
	Job                    AgentPlanningJob          `json:"job"`
	Intent                 IntentSpec                `json:"intent"`
	Research               ResearchPack              `json:"research"`
	Storyline              Storyline                 `json:"storyline"`
	Outline                OutlinePlan               `json:"outline"`
	ApprovedOutline        *OutlinePlan              `json:"approvedOutline,omitempty"`
	ResearchExecutionCount int                       `json:"researchExecutionCount"`
	DeckGeneration         *AgentDeckGenerationState `json:"deckGeneration,omitempty"`
}

type AgentPlanningJob struct {
	ID                 string              `json:"id"`
	WorkflowType       string              `json:"workflowType"`
	TenantID           string              `json:"tenantId"`
	UserID             string              `json:"userId"`
	OrganizationID     string              `json:"organizationId"`
	Status             string              `json:"status"`
	Stage              string              `json:"stage"`
	CompletedWorkUnits int                 `json:"completedWorkUnits"`
	TotalWorkUnits     int                 `json:"totalWorkUnits"`
	SlideCount         int                 `json:"slideCount"`
	RunAfter           time.Time           `json:"runAfter"`
	Error              *GenerationJobError `json:"error,omitempty"`
	UpdatedAt          time.Time           `json:"updatedAt"`
	DeckID             string              `json:"deckId,omitempty"`
	Revision           int                 `json:"revision,omitempty"`
	FileID             string              `json:"fileId,omitempty"`
	AssetID            string              `json:"assetId,omitempty"`
	TaskID             string              `json:"taskId,omitempty"`
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
	CreateAgentPlanning(context.Context, CreateGenerationJobInput, IntentSpec) (GenerationJob, bool, error)
	ListReadyAgentPlanning(context.Context, time.Time, int) ([]GenerationJob, error)
	RetryAgentPlanning(context.Context, GenerationJobScope, string, time.Time) (GenerationJob, error)
	SaveAgentIntent(context.Context, GenerationLease, IntentSpec, time.Time) (GenerationLease, error)
	SaveAgentResearch(context.Context, GenerationLease, ResearchPack, time.Time) (GenerationLease, error)
	SaveAgentStoryline(context.Context, GenerationLease, Storyline, time.Time) (GenerationLease, error)
	SaveAgentOutline(context.Context, GenerationLease, OutlinePlan, time.Time) (AgentPlanningState, error)
	GetAgentPlanning(context.Context, GenerationJobScope, string) (AgentPlanningState, error)
	UpdateAgentOutline(context.Context, GenerationJobScope, string, int, []OutlineEditCommand, time.Time) (AgentPlanningState, error)
	ApproveAgentOutline(context.Context, GenerationJobScope, string, int, time.Time) (AgentPlanningState, error)
	SaveAgentEdit(context.Context, GenerationJobScope, string, AgentDeckGenerationState, []byte, string, time.Time) (AgentPlanningState, error)
	SaveAgentDeckCheckpoint(context.Context, GenerationLease, AgentDeckCheckpoint) (GenerationLease, error)
}

type AgentPlanningOptions struct {
	WorkerID      string
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	ScanInterval  time.Duration
	WorkerLimit   int
}

type AgentPlanningService struct {
	store         AgentPlanningStore
	research      ResearchProvider
	storyline     StorylinePlanningPort
	outline       OutlinePlanningPort
	content       SlideContentPlanningPort
	assets        AgentDeckAssetPort
	compiler      AgentDeckCompilerPort
	artifacts     AgentDeckArtifactPort
	editPlanner   EditPlanningPort
	workerID      string
	leaseDuration time.Duration
	retryDelay    time.Duration
	scanInterval  time.Duration
	workerLimit   int
	wake          chan struct{}
}

func (s *AgentPlanningService) ConfigureDeckGeneration(content SlideContentPlanningPort, assets AgentDeckAssetPort, compiler AgentDeckCompilerPort, artifacts AgentDeckArtifactPort) error {
	if s == nil || content == nil || assets == nil || compiler == nil || artifacts == nil {
		return ErrGenerationJobInvalid
	}
	s.content = content
	s.assets = assets
	s.compiler = compiler
	s.artifacts = artifacts
	return nil
}

func (s *AgentPlanningService) ConfigureEditPlanning(planner EditPlanningPort) error {
	if s == nil || planner == nil {
		return ErrEditProviderUnavailable
	}
	s.editPlanner = planner
	return nil
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

func NewAgentPlanningService(store AgentPlanningStore, research ResearchProvider, storyline StorylinePlanningPort, outline OutlinePlanningPort, options AgentPlanningOptions) (*AgentPlanningService, error) {
	if store == nil {
		return nil, ErrGenerationJobInvalid
	}
	options.WorkerID = strings.TrimSpace(options.WorkerID)
	if options.WorkerID == "" {
		options.WorkerID = "ppt_v2_agent_planner:" + newGenerationJobID()
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = time.Minute
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = 5 * time.Second
	}
	if options.ScanInterval <= 0 {
		options.ScanInterval = 2 * time.Second
	}
	if options.WorkerLimit <= 0 {
		options.WorkerLimit = 2
	}
	return &AgentPlanningService{
		store: store, research: research, storyline: storyline, outline: outline,
		workerID: options.WorkerID, leaseDuration: options.LeaseDuration, retryDelay: options.RetryDelay,
		scanInterval: options.ScanInterval, workerLimit: options.WorkerLimit, wake: make(chan struct{}, 1),
	}, nil
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
	inputSnapshot, err := json.Marshal(struct {
		Version int           `json:"version"`
		Request IntentRequest `json:"request"`
		Intent  IntentSpec    `json:"intent"`
	}{Version: 1, Request: request.Request, Intent: intent})
	if err != nil {
		return AgentGuideResult{}, err
	}
	job, _, err := s.store.CreateAgentPlanning(ctx, CreateGenerationJobInput{
		TenantID: request.TenantID, UserID: request.UserID, OrganizationID: request.OrganizationID,
		ClientRequestID: agentGuideRequestIdentity(request.Request), IdempotencyKey: request.IdempotencyKey,
		SlideCount: pageCount, WorkflowType: GenerationWorkflowAgentOutline, InputSnapshot: inputSnapshot, Now: now,
	}, intent)
	if err != nil {
		return AgentGuideResult{}, err
	}
	scope := GenerationJobScope{TenantID: job.TenantID, UserID: job.UserID}
	state, err := s.store.GetAgentPlanning(ctx, scope, job.ID)
	if err != nil {
		return AgentGuideResult{}, err
	}
	s.Wake()
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
	state, err := s.store.ApproveAgentOutline(ctx, scope, jobID, expectedRevision, normalizedAgentTime(now))
	if err == nil {
		s.Wake()
	}
	return state, err
}

func (s *AgentPlanningService) ApplyEdit(ctx context.Context, scope GenerationJobScope, jobID string, command EditCommand, message string, now time.Time) (AgentPlanningState, error) {
	state, err := s.store.GetAgentPlanning(ctx, scope, jobID)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if state.Job.Status != GenerationJobSucceeded || state.Job.Stage != GenerationStageCompleted || state.DeckGeneration == nil || state.DeckGeneration.Compilation == nil {
		return AgentPlanningState{}, ErrGenerationJobNotReady
	}
	if strings.TrimSpace(message) != "" {
		if s.editPlanner == nil {
			return AgentPlanningState{}, ErrEditProviderUnavailable
		}
		draft, planErr := s.editPlanner.PlanEdit(ctx, EditPlanningInput{Message: strings.TrimSpace(message), State: state})
		if planErr != nil {
			return AgentPlanningState{}, NewAgentWorkflowError("edit_provider_unavailable", "修改规划服务暂时不可用，请重试。", true, planErr)
		}
		command = draft.Command
		command.CommandID = "edit_" + newGenerationJobID()
		command.DeckID = state.Job.DeckID
		command.BaseRevision = state.Job.Revision
		command.UserIntentSummary = strings.TrimSpace(message)
	}
	updated, err := ApplyEditCommand(*state.DeckGeneration, command, normalizedAgentTime(now))
	if err != nil {
		return AgentPlanningState{}, err
	}
	if updated.CurrentRevision == state.DeckGeneration.CurrentRevision {
		return state, nil
	}
	var renderBytes []byte
	var fileID string
	if len(updated.Revisions) > 0 {
		updated.Revisions[0].FileID = state.Job.FileID
	}
	if s.compiler == nil || s.artifacts == nil {
		return AgentPlanningState{}, ErrEditUnsupported
	}
	rendered, err := s.compiler.Render(ctx, *updated.Compilation, updated.Assets)
	if err != nil || len(rendered.PPTX) == 0 {
		return AgentPlanningState{}, NewAgentWorkflowError("edit_render_failed", "新版 PPT 生成失败，请重试。", true, err)
	}
	renderBytes = rendered.PPTX
	fileID, err = s.artifacts.StorePPTX(ctx, scope, jobID, state.Job.TaskID, renderBytes)
	if err != nil {
		return AgentPlanningState{}, NewAgentWorkflowError("edit_asset_failed", "新版 PPT 保存失败，请重试。", true, err)
	}
	if len(updated.Revisions) > 0 {
		last := len(updated.Revisions) - 1
		updated.Revisions[last].FileID = fileID
		updated.Revisions[last].RenderBytes = append([]byte(nil), renderBytes...)
	}
	return s.store.SaveAgentEdit(ctx, scope, jobID, updated, renderBytes, fileID, normalizedAgentTime(now))
}

func (s *AgentPlanningService) UndoEdit(ctx context.Context, scope GenerationJobScope, jobID string, now time.Time) (AgentPlanningState, error) {
	state, err := s.store.GetAgentPlanning(ctx, scope, jobID)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if state.DeckGeneration == nil {
		return AgentPlanningState{}, ErrGenerationJobNotReady
	}
	updated, err := UndoLastEdit(*state.DeckGeneration)
	if err != nil {
		return AgentPlanningState{}, err
	}
	var fileID string
	var parentCompilation *DeckCompilation
	for _, revision := range updated.Revisions {
		if revision.Revision == updated.CurrentRevision {
			fileID = revision.FileID
			compilation := cloneDeckCompilation(revision.Compilation)
			parentCompilation = &compilation
			break
		}
	}
	if parentCompilation == nil || s.compiler == nil || s.artifacts == nil {
		return AgentPlanningState{}, ErrEditNoUndo
	}
	rendered, renderErr := s.compiler.Render(ctx, *parentCompilation, updated.Assets)
	if renderErr != nil || len(rendered.PPTX) == 0 {
		return AgentPlanningState{}, NewAgentWorkflowError("edit_render_failed", "撤销后的 PPT 生成失败，请重试。", true, renderErr)
	}
	renderBytes := rendered.PPTX
	if fileID == "" {
		fileID = state.Job.FileID
	}
	fileID, err = s.artifacts.StorePPTX(ctx, scope, jobID, state.Job.TaskID, renderBytes)
	if err != nil {
		return AgentPlanningState{}, NewAgentWorkflowError("edit_asset_failed", "撤销后的 PPT 保存失败，请重试。", true, err)
	}
	return s.store.SaveAgentEdit(ctx, scope, jobID, updated, renderBytes, fileID, normalizedAgentTime(now))
}

func (s *AgentPlanningService) Retry(ctx context.Context, scope GenerationJobScope, jobID string, now time.Time) (AgentPlanningState, error) {
	job, err := s.store.RetryAgentPlanning(ctx, scope, jobID, normalizedAgentTime(now))
	if err != nil {
		return AgentPlanningState{}, err
	}
	state, err := s.store.GetAgentPlanning(ctx, scope, job.ID)
	if err != nil {
		return AgentPlanningState{}, err
	}
	s.Wake()
	return state, nil
}

func plannedAgentPageCount(intent IntentSpec) int {
	pageCount := intent.PageCount.Preferred
	if !intent.PageCount.Explicit || pageCount == 0 {
		return AgentMinimumPageCount
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
	input.DeckGeneration = cloneAgentDeckGenerationState(input.DeckGeneration)
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
			SlideCount: job.SlideCount, RunAfter: job.RunAfter, Error: cloneGenerationError(job.LastError), UpdatedAt: job.UpdatedAt,
			DeckID: job.DeckID, Revision: job.Revision, FileID: job.FileID, AssetID: job.AssetID, TaskID: job.ExistingTaskID,
		}, Intent: record.Intent, Research: record.Research,
		Storyline: record.Storyline, Outline: record.Outline, ApprovedOutline: record.ApprovedOutline,
		ResearchExecutionCount: record.ResearchExecutionCount,
		DeckGeneration:         &record.DeckGeneration,
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
	return s.saveAgentPlanningStage(lease, GenerationStageResearched, GenerationStageStorylinePlanned, 2, now, func(record *AgentPlanningRecord) error {
		if err := ValidateStoryline(storyline, record.Intent, record.Research); err != nil {
			return err
		}
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
	job.SlideCount = outline.PageCount
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
	job.TotalWorkUnits = agentDeckTotalWorkUnits(approved)
	job.CompletedWorkUnits = 3
	job.DeckJobID = job.ID + ":deck"
	job.RunAfter = approved.ApprovedAt
	job.UpdatedAt = approved.ApprovedAt
	s.agentPlans[job.ID] = record
	s.decks[job.ID] = DeckJob{ID: job.DeckJobID, GenerationJobID: job.ID, Status: GenerationChildPending, CreatedAt: approved.ApprovedAt, UpdatedAt: approved.ApprovedAt}
	s.slides[job.ID] = make([]SlideJob, 0, len(approved.Slides))
	for index, objective := range approved.Slides {
		s.slides[job.ID] = append(s.slides[job.ID], SlideJob{ID: fmt.Sprintf("%s:slide:%d", job.ID, index+1), GenerationJobID: job.ID, DeckJobID: job.DeckJobID, SlideIndex: index + 1, SourceSlideID: objective.SlideID, Status: GenerationChildPending, CreatedAt: approved.ApprovedAt, UpdatedAt: approved.ApprovedAt})
	}
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
