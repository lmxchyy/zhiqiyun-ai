package ppt

import (
	"context"
	"strings"
	"time"
)

func validAgentDeckStageTransition(from, to string) bool {
	if from == to {
		return from == GenerationStageOutlineApproved || from == GenerationStageContentReady
	}
	return (from == GenerationStageOutlineApproved && to == GenerationStageContentReady) ||
		(from == GenerationStageContentReady && to == GenerationStageAssetsReady) ||
		(from == GenerationStageAssetsReady && to == GenerationStageLayoutCompiled) ||
		(from == GenerationStageLayoutCompiled && to == GenerationStageQualityChecked) ||
		(from == GenerationStageQualityChecked && to == GenerationStageRendered) ||
		(from == GenerationStageRendered && to == GenerationStageFileStored) ||
		(from == GenerationStageFileStored && to == GenerationStageAssetCreated) ||
		(from == GenerationStageAssetCreated && to == GenerationStageTaskRelated) ||
		(from == GenerationStageTaskRelated && to == GenerationStageCompleted)
}

func validateAgentDeckCheckpoint(job GenerationJob, checkpoint AgentDeckCheckpoint) error {
	if job.WorkflowType != GenerationWorkflowAgentOutline || checkpoint.ExpectedStage != job.Stage || !validAgentDeckStageTransition(job.Stage, checkpoint.NextStage) || checkpoint.CompletedWorkUnits < job.CompletedWorkUnits || checkpoint.CompletedWorkUnits > job.TotalWorkUnits {
		return ErrGenerationJobTransition
	}
	switch checkpoint.NextStage {
	case GenerationStageContentReady:
		if len(checkpoint.State.Contents) != job.SlideCount {
			return ErrGenerationJobInvalid
		}
	case GenerationStageAssetsReady:
		if len(checkpoint.State.Assets) != agentDeckAssetIntentCount(checkpoint.State.Contents) {
			return ErrGenerationJobInvalid
		}
	case GenerationStageLayoutCompiled, GenerationStageQualityChecked:
		if checkpoint.State.Compilation == nil || checkpoint.State.Compilation.SlideCount != job.SlideCount {
			return ErrGenerationJobInvalid
		}
	case GenerationStageRendered:
		if strings.TrimSpace(checkpoint.DeckID) == "" || checkpoint.Revision <= 0 || strings.TrimSpace(checkpoint.RenderSHA256) == "" || len(checkpoint.RenderBytes) == 0 {
			return ErrGenerationJobInvalid
		}
	case GenerationStageFileStored:
		if strings.TrimSpace(checkpoint.ExistingTaskID) == "" || strings.TrimSpace(checkpoint.FileID) == "" {
			return ErrGenerationJobInvalid
		}
	case GenerationStageAssetCreated:
		if strings.TrimSpace(checkpoint.AssetID) == "" {
			return ErrGenerationJobInvalid
		}
	case GenerationStageCompleted:
		if checkpoint.CompletedWorkUnits != job.TotalWorkUnits {
			return ErrGenerationJobInvalid
		}
	}
	return nil
}

func agentDeckAssetIntentCount(contents []SlideContent) int {
	count := 0
	for _, content := range contents {
		count += len(content.AssetIntents)
	}
	return count
}

func (s *MemoryGenerationJobStore) SaveAgentDeckCheckpoint(_ context.Context, lease GenerationLease, checkpoint AgentDeckCheckpoint) (GenerationLease, error) {
	checkpoint.Now = normalizedAgentTime(checkpoint.Now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.validLeaseLocked(lease, checkpoint.Now)
	if err != nil {
		return GenerationLease{}, err
	}
	if err := validateAgentDeckCheckpoint(job, checkpoint); err != nil {
		return GenerationLease{}, err
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	record.DeckGeneration = cloneAgentDeckGenerationState(checkpoint.State)
	fromStage := job.Stage
	job.Stage = checkpoint.NextStage
	job.CompletedWorkUnits = checkpoint.CompletedWorkUnits
	job.UpdatedAt = checkpoint.Now
	if checkpoint.DeckID != "" {
		job.DeckID = strings.TrimSpace(checkpoint.DeckID)
	}
	if checkpoint.Revision > 0 {
		job.Revision = checkpoint.Revision
	}
	if checkpoint.RenderSHA256 != "" {
		job.RenderSHA256 = strings.TrimSpace(checkpoint.RenderSHA256)
	}
	if len(checkpoint.RenderBytes) > 0 {
		job.RenderBytes = append([]byte(nil), checkpoint.RenderBytes...)
	}
	if checkpoint.ExistingTaskID != "" {
		job.ExistingTaskID = strings.TrimSpace(checkpoint.ExistingTaskID)
	}
	if checkpoint.FileID != "" {
		job.FileID = strings.TrimSpace(checkpoint.FileID)
	}
	if checkpoint.AssetID != "" {
		job.AssetID = strings.TrimSpace(checkpoint.AssetID)
	}
	if checkpoint.NextStage == GenerationStageContentReady {
		for index := range s.slides[job.ID] {
			s.slides[job.ID][index].Status = GenerationChildRunning
			s.slides[job.ID][index].UpdatedAt = checkpoint.Now
		}
	}
	if checkpoint.NextStage == GenerationStageRendered {
		deck := s.decks[job.ID]
		deck.DeckID = job.DeckID
		deck.Revision = job.Revision
		deck.Status = GenerationChildRunning
		deck.UpdatedAt = checkpoint.Now
		s.decks[job.ID] = deck
		for index := range s.slides[job.ID] {
			s.slides[job.ID][index].Status = GenerationChildSucceeded
			s.slides[job.ID][index].CompletedWorkUnits = 1
			s.slides[job.ID][index].UpdatedAt = checkpoint.Now
		}
	}
	if checkpoint.NextStage == GenerationStageCompleted {
		job.Status = GenerationJobSucceeded
		job.FinishedAt = checkpoint.Now
		job.LeaseOwner = ""
		job.LeaseExpiresAt = time.Time{}
		deck := s.decks[job.ID]
		deck.Status = GenerationChildSucceeded
		deck.UpdatedAt = checkpoint.Now
		s.decks[job.ID] = deck
		s.finishAttemptLocked(job.ID, lease.AttemptID, GenerationAttemptSucceeded, nil, checkpoint.Now)
	}
	s.agentPlans[job.ID] = record
	s.jobs[job.ID] = job
	if fromStage != checkpoint.NextStage {
		s.transitions[job.ID] = append(s.transitions[job.ID], GenerationTransition{JobID: job.ID, AttemptID: lease.AttemptID, FromStage: fromStage, ToStage: checkpoint.NextStage, FencingToken: lease.FencingToken, Checkpoint: map[string]any{"completedWorkUnits": checkpoint.CompletedWorkUnits}, CreatedAt: checkpoint.Now})
	}
	lease.Job = cloneGenerationJob(job)
	return lease, nil
}

func (s *MemoryGenerationJobStore) SaveAgentEdit(_ context.Context, scope GenerationJobScope, jobID string, deckState AgentDeckGenerationState, renderBytes []byte, fileID string, now time.Time) (AgentPlanningState, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.WorkflowType != GenerationWorkflowAgentOutline || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	if job.Status != GenerationJobSucceeded || job.Stage != GenerationStageCompleted || deckState.Compilation == nil || deckState.CurrentRevision <= 0 {
		return AgentPlanningState{}, ErrGenerationJobNotReady
	}
	if deckState.Compilation.Revision != deckState.CurrentRevision || deckState.CurrentRevision <= job.Revision {
		return AgentPlanningState{}, ErrEditStaleRevision
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	record.DeckGeneration = cloneAgentDeckGenerationState(deckState)
	job.Revision = deckState.CurrentRevision
	if len(renderBytes) > 0 {
		job.RenderBytes = append([]byte(nil), renderBytes...)
	}
	if strings.TrimSpace(fileID) != "" {
		job.FileID = strings.TrimSpace(fileID)
	}
	job.UpdatedAt = now
	deck := s.decks[job.ID]
	deck.Revision = job.Revision
	deck.UpdatedAt = now
	s.decks[job.ID] = deck
	s.agentPlans[job.ID] = record
	s.jobs[job.ID] = job
	return agentPlanningState(job, record), nil
}

func (s *MemoryGenerationJobStore) EnqueueAgentEdit(_ context.Context, scope GenerationJobScope, jobID string, request DurableEditCheckpoint, now time.Time) (AgentPlanningState, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.WorkflowType != GenerationWorkflowAgentOutline || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	for _, revision := range record.DeckGeneration.Revisions {
		for _, command := range revision.Commands {
			if command.CommandID == request.RequestID {
				return agentPlanningState(job, record), nil
			}
		}
	}
	if record.DeckGeneration.PendingEdit != nil {
		if record.DeckGeneration.PendingEdit.RequestID == request.RequestID {
			return agentPlanningState(job, record), nil
		}
		return AgentPlanningState{}, ErrEditStaleRevision
	}
	if job.Status != GenerationJobSucceeded || job.Stage != GenerationStageCompleted || request.BaseRevision != job.Revision {
		return AgentPlanningState{}, ErrEditStaleRevision
	}
	if request.Command != nil {
		if err := ValidateEditCommand(*request.Command, record.DeckGeneration); err != nil {
			return AgentPlanningState{}, err
		}
	}
	record.DeckGeneration.PendingEdit = &request
	job.Status, job.Stage, job.RunAfter, job.LastError = GenerationJobQueued, GenerationStageOutlineApproved, now, nil
	job.LeaseOwner, job.LeaseExpiresAt = "", time.Time{}
	job.UpdatedAt = now
	job.FinishedAt = time.Time{}
	s.agentPlans[job.ID], s.jobs[job.ID] = record, job
	return agentPlanningState(job, record), nil
}

func (s *MemoryGenerationJobStore) SaveAgentEditCheckpoint(_ context.Context, lease GenerationLease, state AgentDeckGenerationState, now time.Time) (GenerationLease, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.validLeaseLocked(lease, now)
	if err != nil {
		return GenerationLease{}, err
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	record.DeckGeneration = cloneAgentDeckGenerationState(state)
	s.agentPlans[job.ID], s.jobs[job.ID] = record, job
	lease.Job = cloneGenerationJob(job)
	return lease, nil
}

func (s *MemoryGenerationJobStore) CompleteAgentEdit(_ context.Context, lease GenerationLease, state AgentDeckGenerationState, renderBytes []byte, fileID string, now time.Time) (AgentPlanningState, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.validLeaseLocked(lease, now)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if state.Compilation == nil || state.CurrentRevision <= job.Revision {
		return AgentPlanningState{}, ErrEditStaleRevision
	}
	record := cloneAgentPlanningRecord(s.agentPlans[job.ID])
	state.PendingEdit = nil
	record.DeckGeneration = cloneAgentDeckGenerationState(state)
	job.Status, job.Stage, job.FinishedAt = GenerationJobSucceeded, GenerationStageCompleted, now
	job.Revision = state.CurrentRevision
	job.FileID = strings.TrimSpace(fileID)
	job.RenderBytes = append([]byte(nil), renderBytes...)
	job.LeaseOwner, job.LeaseExpiresAt = "", time.Time{}
	job.UpdatedAt = now
	s.agentPlans[job.ID], s.jobs[job.ID] = record, job
	return agentPlanningState(job, record), nil
}

var _ AgentPlanningStore = (*MemoryGenerationJobStore)(nil)
