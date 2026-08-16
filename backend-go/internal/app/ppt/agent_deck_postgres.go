package ppt

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

func (s *PostgresGenerationJobStore) SaveAgentDeckCheckpoint(ctx context.Context, lease GenerationLease, checkpoint AgentDeckCheckpoint) (GenerationLease, error) {
	checkpoint.Now = normalizedAgentTime(checkpoint.Now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationLease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return GenerationLease{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, checkpoint.Now); err != nil {
		return GenerationLease{}, err
	}
	if err := validateAgentDeckCheckpoint(job, checkpoint); err != nil {
		return GenerationLease{}, err
	}
	rawState, err := json.Marshal(checkpoint.State)
	if err != nil {
		return GenerationLease{}, err
	}
	result, err := tx.ExecContext(ctx, `update xz_ppt_v2_agent_plans set deck_state=$2::jsonb,updated_at=$3 where generation_job_id=$1`, job.ID, string(rawState), checkpoint.Now)
	if err != nil {
		return GenerationLease{}, err
	}
	if err := requireSingleAgentPlanRow(result); err != nil {
		return GenerationLease{}, err
	}

	fromStage := job.Stage
	job.Stage = checkpoint.NextStage
	job.CompletedWorkUnits = checkpoint.CompletedWorkUnits
	job.UpdatedAt = checkpoint.Now
	if checkpoint.DeckID != "" {
		job.DeckID = checkpoint.DeckID
	}
	if checkpoint.Revision > 0 {
		job.Revision = checkpoint.Revision
	}
	if checkpoint.RenderSHA256 != "" {
		job.RenderSHA256 = checkpoint.RenderSHA256
	}
	if len(checkpoint.RenderBytes) > 0 {
		job.RenderBytes = append([]byte(nil), checkpoint.RenderBytes...)
	}
	if checkpoint.ExistingTaskID != "" {
		job.ExistingTaskID = checkpoint.ExistingTaskID
	}
	if checkpoint.FileID != "" {
		job.FileID = checkpoint.FileID
	}
	if checkpoint.AssetID != "" {
		job.AssetID = checkpoint.AssetID
	}

	if checkpoint.NextStage == GenerationStageContentReady {
		if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_slide_jobs set status=$2,updated_at=$3 where generation_job_id=$1 and status=$4`, job.ID, GenerationChildRunning, checkpoint.Now, GenerationChildPending); err != nil {
			return GenerationLease{}, err
		}
	}
	if checkpoint.NextStage == GenerationStageRendered {
		if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set deck_id=$2,revision=$3,status=$4,updated_at=$5 where generation_job_id=$1`, job.ID, job.DeckID, job.Revision, GenerationChildRunning, checkpoint.Now); err != nil {
			return GenerationLease{}, err
		}
		if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_slide_jobs set status=$2,completed_work_units=1,updated_at=$3 where generation_job_id=$1`, job.ID, GenerationChildSucceeded, checkpoint.Now); err != nil {
			return GenerationLease{}, err
		}
	}
	if checkpoint.NextStage == GenerationStageCompleted {
		job.Status = GenerationJobSucceeded
		job.FinishedAt = checkpoint.Now
		job.LeaseOwner = ""
		job.LeaseExpiresAt = time.Time{}
		if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set status=$2,updated_at=$3 where generation_job_id=$1`, job.ID, GenerationChildSucceeded, checkpoint.Now); err != nil {
			return GenerationLease{}, err
		}
		if err := finishPostgresGenerationAttempt(ctx, tx, lease.AttemptID, GenerationAttemptSucceeded, nil, checkpoint.Now); err != nil {
			return GenerationLease{}, err
		}
	}
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationLease{}, err
	}
	if fromStage != checkpoint.NextStage {
		rawCheckpoint, _ := json.Marshal(map[string]any{"completedWorkUnits": checkpoint.CompletedWorkUnits})
		if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,attempt_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5,$6::jsonb,$7)`, job.ID, lease.AttemptID, fromStage, checkpoint.NextStage, lease.FencingToken, string(rawCheckpoint), checkpoint.Now); err != nil {
			return GenerationLease{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return GenerationLease{}, err
	}
	lease.Job = job
	return lease, nil
}

func (s *PostgresGenerationJobStore) SaveAgentEdit(ctx context.Context, scope GenerationJobScope, jobID string, deckState AgentDeckGenerationState, renderBytes []byte, fileID string, now time.Time) (AgentPlanningState, error) {
	now = normalizedAgentTime(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentPlanningState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForUpdate(ctx, tx, scope, jobID)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if job.Status != GenerationJobSucceeded || job.Stage != GenerationStageCompleted || deckState.Compilation == nil || deckState.CurrentRevision <= job.Revision || deckState.Compilation.Revision != deckState.CurrentRevision {
		return AgentPlanningState{}, ErrEditStaleRevision
	}
	rawState, err := json.Marshal(deckState)
	if err != nil {
		return AgentPlanningState{}, err
	}
	result, err := tx.ExecContext(ctx, `update xz_ppt_v2_agent_plans set deck_state=$2::jsonb,updated_at=$3 where generation_job_id=$1`, job.ID, string(rawState), now)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := requireSingleAgentPlanRow(result); err != nil {
		return AgentPlanningState{}, err
	}
	job.Revision = deckState.CurrentRevision
	job.UpdatedAt = now
	if len(renderBytes) > 0 {
		job.RenderBytes = append([]byte(nil), renderBytes...)
	}
	if strings.TrimSpace(fileID) != "" {
		job.FileID = strings.TrimSpace(fileID)
	}
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return AgentPlanningState{}, err
	}
	if _, err := tx.ExecContext(ctx, `update xz_ppt_v2_deck_jobs set revision=$2,updated_at=$3 where generation_job_id=$1`, job.ID, job.Revision, now); err != nil {
		return AgentPlanningState{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentPlanningState{}, err
	}
	return s.GetAgentPlanning(ctx, scope, job.ID)
}

var _ AgentPlanningStore = (*PostgresGenerationJobStore)(nil)
