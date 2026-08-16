package ppt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type agentPlanningQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *PostgresGenerationJobStore) CreateAgentPlanning(ctx context.Context, input CreateGenerationJobInput, intent IntentSpec) (GenerationJob, bool, error) {
	return s.createGenerationJob(ctx, input, &intent)
}

func (s *PostgresGenerationJobStore) ListReadyAgentPlanning(ctx context.Context, now time.Time, limit int) ([]GenerationJob, error) {
	if limit <= 0 {
		return nil, ErrGenerationJobInvalid
	}
	now = normalizedAgentTime(now)
	rows, err := s.db.QueryContext(ctx, `
select `+generationJobColumns+`
from xz_ppt_v2_generation_jobs
where workflow_type='AGENT_OUTLINE'
  and stage in ('CREATED','INTENT_RESOLVED','RESEARCHED','STORYLINE_PLANNED')
  and (
    (status in ('QUEUED','RETRY_WAIT') and run_after <= $1)
    or (status='RUNNING' and lease_expires_at <= $1)
  )
order by run_after,created_at,id
limit $2
`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]GenerationJob, 0, limit)
	for rows.Next() {
		job, err := scanPostgresGenerationJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *PostgresGenerationJobStore) RetryAgentPlanning(ctx context.Context, scope GenerationJobScope, jobID string, now time.Time) (GenerationJob, error) {
	now = normalizedAgentTime(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationJob{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForUpdate(ctx, tx, scope, jobID)
	if err != nil {
		return GenerationJob{}, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline {
		return GenerationJob{}, ErrGenerationJobNotFound
	}
	switch job.Status {
	case GenerationJobQueued, GenerationJobRunning:
		if err := tx.Commit(); err != nil {
			return GenerationJob{}, err
		}
		return job, nil
	case GenerationJobRetryWait:
		job.Status = GenerationJobQueued
		job.RunAfter = now
		job.LastError = nil
		job.UpdatedAt = now
	case GenerationJobFailed:
		if job.LastError == nil || !job.LastError.Retryable || job.AttemptCount >= 20 {
			return GenerationJob{}, ErrGenerationJobTerminal
		}
		job.Status = GenerationJobQueued
		job.MaxAttempts = job.AttemptCount + 1
		job.RunAfter = now
		job.LastError = nil
		job.FinishedAt = time.Time{}
		job.UpdatedAt = now
	default:
		return GenerationJob{}, ErrGenerationJobTransition
	}
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationJob{}, err
	}
	return job, nil
}

func (s *PostgresGenerationJobStore) SaveAgentIntent(ctx context.Context, lease GenerationLease, intent IntentSpec, now time.Time) (GenerationLease, error) {
	if strings.TrimSpace(intent.Topic) == "" {
		return GenerationLease{}, ErrGenerationJobInvalid
	}
	raw, err := json.Marshal(intent)
	if err != nil {
		return GenerationLease{}, err
	}
	return s.savePostgresAgentStage(ctx, lease, GenerationStageCreated, GenerationStageIntentResolved, 1, now, func(tx *sql.Tx, job GenerationJob) error {
		result, err := tx.ExecContext(ctx, `
update xz_ppt_v2_agent_plans set intent=$2::jsonb,updated_at=$3
where generation_job_id=$1 and intent=$2::jsonb
`, job.ID, string(raw), normalizedAgentTime(now))
		if err != nil {
			return err
		}
		return requireSingleAgentPlanRow(result)
	})
}

func (s *PostgresGenerationJobStore) SaveAgentResearch(ctx context.Context, lease GenerationLease, research ResearchPack, now time.Time) (GenerationLease, error) {
	if err := ValidateResearchPack(research); err != nil {
		return GenerationLease{}, err
	}
	raw, err := json.Marshal(research)
	if err != nil {
		return GenerationLease{}, err
	}
	return s.savePostgresAgentStage(ctx, lease, GenerationStageIntentResolved, GenerationStageResearched, 2, now, func(tx *sql.Tx, job GenerationJob) error {
		result, err := tx.ExecContext(ctx, `
update xz_ppt_v2_agent_plans
set research=$2::jsonb,research_execution_count=research_execution_count+1,updated_at=$3
where generation_job_id=$1 and research is null
`, job.ID, string(raw), normalizedAgentTime(now))
		if err != nil {
			return err
		}
		return requireSingleAgentPlanRow(result)
	})
}

func (s *PostgresGenerationJobStore) SaveAgentStoryline(ctx context.Context, lease GenerationLease, storyline Storyline, now time.Time) (GenerationLease, error) {
	raw, err := json.Marshal(storyline)
	if err != nil {
		return GenerationLease{}, err
	}
	return s.savePostgresAgentStage(ctx, lease, GenerationStageResearched, GenerationStageStorylinePlanned, 2, now, func(tx *sql.Tx, job GenerationJob) error {
		record, err := loadPostgresAgentRecord(ctx, tx, job.ID, true)
		if err != nil {
			return err
		}
		if err := ValidateStoryline(storyline, record.Intent, record.Research); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
update xz_ppt_v2_agent_plans set storyline=$2::jsonb,updated_at=$3
where generation_job_id=$1 and storyline is null
`, job.ID, string(raw), normalizedAgentTime(now))
		if err != nil {
			return err
		}
		return requireSingleAgentPlanRow(result)
	})
}

func (s *PostgresGenerationJobStore) SaveAgentOutline(ctx context.Context, lease GenerationLease, outline OutlinePlan, now time.Time) (AgentPlanningState, error) {
	now = normalizedAgentTime(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentPlanningState{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, now); err != nil {
		return AgentPlanningState{}, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline || job.Stage != GenerationStageStorylinePlanned {
		return AgentPlanningState{}, ErrGenerationJobTransition
	}
	record, err := loadPostgresAgentRecord(ctx, tx, job.ID, true)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := ValidateOutlinePlan(outline, record.Research); err != nil {
		return AgentPlanningState{}, err
	}
	raw, err := json.Marshal(outline)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if _, err := tx.ExecContext(ctx, `
insert into xz_ppt_v2_outline_revisions(generation_job_id,revision,parent_revision,outline,created_at)
values($1,$2,null,$3::jsonb,$4)
`, job.ID, outline.Revision, string(raw), now); err != nil {
		return AgentPlanningState{}, err
	}
	result, err := tx.ExecContext(ctx, `
update xz_ppt_v2_agent_plans set current_outline_revision=$2,updated_at=$3
where generation_job_id=$1 and current_outline_revision is null
`, job.ID, outline.Revision, now)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := requireSingleAgentPlanRow(result); err != nil {
		return AgentPlanningState{}, err
	}
	fromStage := job.Stage
	job.Stage = GenerationStageOutlinePlanned
	job.Status = GenerationJobWaitingForOutlineApproval
	job.CompletedWorkUnits = job.TotalWorkUnits
	job.SlideCount = outline.PageCount
	job.LeaseOwner = ""
	job.LeaseExpiresAt = time.Time{}
	job.UpdatedAt = now
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return AgentPlanningState{}, err
	}
	checkpoint, _ := json.Marshal(map[string]any{"completedWorkUnits": job.CompletedWorkUnits, "outlineRevision": outline.Revision})
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,attempt_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5,$6::jsonb,$7)`, job.ID, lease.AttemptID, fromStage, job.Stage, lease.FencingToken, string(checkpoint), now); err != nil {
		return AgentPlanningState{}, err
	}
	if err := finishPostgresGenerationAttempt(ctx, tx, lease.AttemptID, GenerationAttemptSucceeded, nil, now); err != nil {
		return AgentPlanningState{}, err
	}
	record.Outline = cloneOutlinePlan(outline)
	record.Revisions = map[int]OutlinePlan{outline.Revision: cloneOutlinePlan(outline)}
	if err := tx.Commit(); err != nil {
		return AgentPlanningState{}, err
	}
	return agentPlanningState(job, record), nil
}

func (s *PostgresGenerationJobStore) GetAgentPlanning(ctx context.Context, scope GenerationJobScope, jobID string) (AgentPlanningState, error) {
	job, err := scanPostgresGenerationJob(s.db.QueryRowContext(ctx, `select `+generationJobColumns+` from xz_ppt_v2_generation_jobs where id=$1 and tenant_id=$2 and user_id=$3`, strings.TrimSpace(jobID), strings.TrimSpace(scope.TenantID), strings.TrimSpace(scope.UserID)))
	if errors.Is(err, sql.ErrNoRows) {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	if err != nil {
		return AgentPlanningState{}, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	record, err := loadPostgresAgentRecord(ctx, s.db, job.ID, false)
	if err != nil {
		return AgentPlanningState{}, err
	}
	return agentPlanningState(job, record), nil
}

func (s *PostgresGenerationJobStore) UpdateAgentOutline(ctx context.Context, scope GenerationJobScope, jobID string, expectedRevision int, commands []OutlineEditCommand, now time.Time) (AgentPlanningState, error) {
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
	if job.WorkflowType != GenerationWorkflowAgentOutline {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	record, err := loadPostgresAgentRecord(ctx, tx, job.ID, true)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if record.ApprovedOutline != nil {
		return AgentPlanningState{}, ErrOutlinePlanApproved
	}
	if job.Status != GenerationJobWaitingForOutlineApproval || job.Stage != GenerationStageOutlinePlanned {
		return AgentPlanningState{}, ErrGenerationJobTransition
	}
	if record.Outline.Revision != expectedRevision {
		return AgentPlanningState{}, ErrStaleOutlineRevision
	}
	updated, err := ApplyOutlineCommands(record.Outline, commands, record.Research)
	if err != nil {
		return AgentPlanningState{}, err
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if _, err := tx.ExecContext(ctx, `
insert into xz_ppt_v2_outline_revisions(generation_job_id,revision,parent_revision,outline,created_at)
values($1,$2,$3,$4::jsonb,$5)
`, job.ID, updated.Revision, expectedRevision, string(raw), now); err != nil {
		return AgentPlanningState{}, err
	}
	result, err := tx.ExecContext(ctx, `
update xz_ppt_v2_agent_plans set current_outline_revision=$2,updated_at=$3
where generation_job_id=$1 and current_outline_revision=$4 and approved_outline_revision is null
`, job.ID, updated.Revision, now, expectedRevision)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := requireSingleAgentPlanRow(result); err != nil {
		return AgentPlanningState{}, err
	}
	job.SlideCount = updated.PageCount
	job.UpdatedAt = now
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return AgentPlanningState{}, err
	}
	record.Outline = cloneOutlinePlan(updated)
	if record.Revisions == nil {
		record.Revisions = map[int]OutlinePlan{}
	}
	record.Revisions[updated.Revision] = cloneOutlinePlan(updated)
	if err := tx.Commit(); err != nil {
		return AgentPlanningState{}, err
	}
	return agentPlanningState(job, record), nil
}

func (s *PostgresGenerationJobStore) ApproveAgentOutline(ctx context.Context, scope GenerationJobScope, jobID string, expectedRevision int, now time.Time) (AgentPlanningState, error) {
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
	if job.WorkflowType != GenerationWorkflowAgentOutline {
		return AgentPlanningState{}, ErrGenerationJobNotFound
	}
	record, err := loadPostgresAgentRecord(ctx, tx, job.ID, true)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if record.ApprovedOutline != nil {
		if record.ApprovedOutline.Revision != expectedRevision {
			return AgentPlanningState{}, ErrStaleOutlineRevision
		}
		if err := tx.Commit(); err != nil {
			return AgentPlanningState{}, err
		}
		return agentPlanningState(job, record), nil
	}
	if job.Status != GenerationJobWaitingForOutlineApproval || job.Stage != GenerationStageOutlinePlanned {
		return AgentPlanningState{}, ErrGenerationJobTransition
	}
	if record.Outline.Revision != expectedRevision {
		return AgentPlanningState{}, ErrStaleOutlineRevision
	}
	result, err := tx.ExecContext(ctx, `
update xz_ppt_v2_outline_revisions set approved_at=$3
where generation_job_id=$1 and revision=$2 and approved_at is null
`, job.ID, expectedRevision, now)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := requireSingleAgentPlanRow(result); err != nil {
		return AgentPlanningState{}, err
	}
	result, err = tx.ExecContext(ctx, `
update xz_ppt_v2_agent_plans set approved_outline_revision=$2,updated_at=$3
where generation_job_id=$1 and current_outline_revision=$2 and approved_outline_revision is null
`, job.ID, expectedRevision, now)
	if err != nil {
		return AgentPlanningState{}, err
	}
	if err := requireSingleAgentPlanRow(result); err != nil {
		return AgentPlanningState{}, err
	}
	approved := cloneOutlinePlan(record.Outline)
	approved.ApprovedAt = now
	record.Outline = cloneOutlinePlan(approved)
	record.ApprovedOutline = &approved
	job.Status = GenerationJobQueued
	job.Stage = GenerationStageOutlineApproved
	job.RunAfter = now
	job.UpdatedAt = now
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return AgentPlanningState{}, err
	}
	checkpoint, _ := json.Marshal(map[string]any{"completedWorkUnits": job.CompletedWorkUnits, "outlineRevision": expectedRevision})
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5::jsonb,$6)`, job.ID, GenerationStageOutlinePlanned, GenerationStageOutlineApproved, job.FencingToken, string(checkpoint), now); err != nil {
		return AgentPlanningState{}, err
	}
	if err := tx.Commit(); err != nil {
		return AgentPlanningState{}, err
	}
	return s.GetAgentPlanning(ctx, scope, job.ID)
}

func (s *PostgresGenerationJobStore) savePostgresAgentStage(ctx context.Context, lease GenerationLease, expectedStage, nextStage string, completedWorkUnits int, now time.Time, write func(*sql.Tx, GenerationJob) error) (GenerationLease, error) {
	now = normalizedAgentTime(now)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GenerationLease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	job, err := loadGenerationJobForLease(ctx, tx, lease)
	if err != nil {
		return GenerationLease{}, err
	}
	if err := validatePostgresGenerationLease(job, lease, now); err != nil {
		return GenerationLease{}, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline || job.Stage != expectedStage {
		return GenerationLease{}, ErrGenerationJobTransition
	}
	if err := write(tx, job); err != nil {
		return GenerationLease{}, err
	}
	fromStage := job.Stage
	job.Stage = nextStage
	job.CompletedWorkUnits = completedWorkUnits
	job.UpdatedAt = now
	if err := persistGenerationJob(ctx, tx, job); err != nil {
		return GenerationLease{}, err
	}
	checkpoint, _ := json.Marshal(map[string]any{"completedWorkUnits": completedWorkUnits})
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,attempt_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5,$6::jsonb,$7)`, job.ID, lease.AttemptID, fromStage, nextStage, lease.FencingToken, string(checkpoint), now); err != nil {
		return GenerationLease{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerationLease{}, err
	}
	lease.Job = job
	return lease, nil
}

func loadPostgresAgentRecord(ctx context.Context, query agentPlanningQuerier, jobID string, forUpdate bool) (AgentPlanningRecord, error) {
	suffix := ""
	if forUpdate {
		suffix = " for update"
	}
	var rawIntent, rawResearch, rawStoryline []byte
	var currentRevision, approvedRevision sql.NullInt64
	var record AgentPlanningRecord
	err := query.QueryRowContext(ctx, `
select intent,research,storyline,current_outline_revision,approved_outline_revision,research_execution_count
from xz_ppt_v2_agent_plans where generation_job_id=$1`+suffix, jobID).Scan(
		&rawIntent, &rawResearch, &rawStoryline, &currentRevision, &approvedRevision, &record.ResearchExecutionCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentPlanningRecord{}, ErrGenerationJobNotFound
	}
	if err != nil {
		return AgentPlanningRecord{}, err
	}
	if len(rawIntent) > 0 {
		if err := json.Unmarshal(rawIntent, &record.Intent); err != nil {
			return AgentPlanningRecord{}, err
		}
	}
	if len(rawResearch) > 0 {
		if err := json.Unmarshal(rawResearch, &record.Research); err != nil {
			return AgentPlanningRecord{}, err
		}
	}
	if len(rawStoryline) > 0 {
		if err := json.Unmarshal(rawStoryline, &record.Storyline); err != nil {
			return AgentPlanningRecord{}, err
		}
	}
	if currentRevision.Valid {
		var rawOutline []byte
		var approvedAt sql.NullTime
		err := query.QueryRowContext(ctx, `select outline,approved_at from xz_ppt_v2_outline_revisions where generation_job_id=$1 and revision=$2`, jobID, currentRevision.Int64).Scan(&rawOutline, &approvedAt)
		if err != nil {
			return AgentPlanningRecord{}, err
		}
		if err := json.Unmarshal(rawOutline, &record.Outline); err != nil {
			return AgentPlanningRecord{}, err
		}
		if approvedAt.Valid {
			record.Outline.ApprovedAt = approvedAt.Time
		}
		record.Revisions = map[int]OutlinePlan{record.Outline.Revision: cloneOutlinePlan(record.Outline)}
	}
	if approvedRevision.Valid {
		if !currentRevision.Valid || approvedRevision.Int64 != currentRevision.Int64 || record.Outline.ApprovedAt.IsZero() {
			return AgentPlanningRecord{}, ErrInvalidOutlinePlan
		}
		approved := cloneOutlinePlan(record.Outline)
		record.ApprovedOutline = &approved
	}
	return record, nil
}

func requireSingleAgentPlanRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrGenerationJobTransition
	}
	return nil
}

var _ AgentPlanningStore = (*PostgresGenerationJobStore)(nil)
