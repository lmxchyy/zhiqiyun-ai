package ppt

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const postgresOperationTimeout = 5 * time.Second

func (s *Service) ensurePostgresReady(ctx context.Context) error {
	if s.db == nil {
		return errors.New("ppt postgres database is unavailable")
	}
	s.postgresReadyMu.Lock()
	defer s.postgresReadyMu.Unlock()
	if s.postgresReady {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
create table if not exists xz_ppt_tasks (
  task_id varchar(128) primary key,
  user_id varchar(128) not null,
	client_request_id varchar(256) not null default '',
  status varchar(32) not null,
  created_at timestamptz not null,
  updated_at timestamptz not null,
  raw jsonb not null
);
alter table xz_ppt_tasks add column if not exists client_request_id varchar(256) not null default '';
create index if not exists idx_xz_ppt_tasks_user_created on xz_ppt_tasks(user_id, created_at desc);
create index if not exists idx_xz_ppt_tasks_user_status on xz_ppt_tasks(user_id, status);
create unique index if not exists uk_xz_ppt_tasks_user_client_request on xz_ppt_tasks(user_id,client_request_id) where client_request_id<>'';
`); err != nil {
		return err
	}
	if err := s.importLegacyTasksPostgres(ctx); err != nil {
		return err
	}
	s.postgresReady = true
	return nil
}

func (s *Service) importLegacyTasksPostgres(ctx context.Context) error {
	if strings.TrimSpace(s.path) == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) || len(raw) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	tasks := decodePersistedTasks(raw)
	if len(tasks) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext('xz_ppt_tasks_legacy_import'))`); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from xz_ppt_tasks`).Scan(&count); err != nil || count > 0 {
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	for _, task := range tasks {
		if err := persistPostgresTask(ctx, tx, task); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func decodePersistedTasks(raw []byte) []Task {
	var state persistedState
	if err := json.Unmarshal(raw, &state); err == nil && len(state.Tasks) > 0 {
		result := make([]Task, 0, len(state.Tasks))
		for _, item := range state.Tasks {
			task := normalizeLegacyTask(item.Task)
			task.UserID = item.UserID
			if task.TaskID != "" {
				result = append(result, task)
			}
		}
		return result
	}
	var legacy map[string]Task
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil
	}
	result := make([]Task, 0, len(legacy))
	for id, task := range legacy {
		if task.TaskID == "" {
			task.TaskID = id
		}
		if task.TaskID != "" {
			result = append(result, normalizeLegacyTask(task))
		}
	}
	return result
}

func (s *Service) generatePostgres(req GenerateRequest, externalActive, limit int) (GenerateResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return GenerateResponse{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return GenerateResponse{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "ppt:user:"+req.UserID); err != nil {
		return GenerateResponse{}, err
	}
	if req.ClientRequestID != "" {
		var existingID, existingStatus string
		err := tx.QueryRowContext(ctx, `select task_id,status from xz_ppt_tasks where user_id=$1 and client_request_id=$2`, req.UserID, req.ClientRequestID).Scan(&existingID, &existingStatus)
		if err == nil {
			return GenerateResponse{TaskID: existingID, Status: existingStatus}, tx.Commit()
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return GenerateResponse{}, err
		}
	}
	if limit > 0 {
		var active int
		if err := tx.QueryRowContext(ctx, `select count(*) from xz_ppt_tasks where user_id=$1 and status in ('pending','processing') and created_at > now() - interval '3 seconds'`, req.UserID).Scan(&active); err != nil {
			return GenerateResponse{}, err
		}
		active += externalActive
		if active >= limit {
			return GenerateResponse{}, fmt.Errorf("%w: active %d, limit %d", ErrConcurrency, active, limit)
		}
	}
	task := taskFromGenerateRequest(req)
	if err := persistPostgresTask(ctx, tx, task); err != nil {
		return GenerateResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenerateResponse{}, err
	}
	s.mirrorPostgresTasksBestEffort()
	return GenerateResponse{TaskID: task.TaskID, Status: task.Status}, nil
}

func taskFromGenerateRequest(req GenerateRequest) Task {
	now := time.Now().UTC()
	return Task{
		TaskID: fmt.Sprintf("ppt_%d", now.UnixNano()), UserID: req.UserID, ClientRequestID: req.ClientRequestID, Type: "ppt", MediaType: "ppt",
		Status: StatusPending, Title: titleFromPrompt(req.Prompt), Prompt: req.Prompt, SlideCount: req.SlideCount,
		Language: req.Language, Tone: req.Tone, TextContent: req.TextContent, Audience: req.Audience, Scenario: req.Scenario,
		GenerationAspectRatio: req.GenerationAspectRatio, Theme: req.Theme, AutoThemeEnabled: req.AutoThemeEnabled,
		EnableWebSearch: req.EnableWebSearch, ImageSource: req.ImageSource, TextModel: req.TextModel, ImageModel: req.ImageModel,
		ImageStyle: req.ImageStyle, PeopleStyle: req.PeopleStyle, ImageLighting: req.ImageLighting,
		ImageComposition: req.ImageComposition, TextInImage: false, Outline: req.Outline, Slides: slidesFromOutline(req.Outline, req),
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
}

func (s *Service) getTaskPostgres(userID, taskID string) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		*task = materializeTask(*task)
		return nil
	})
}

func (s *Service) historyPostgres(userID string) ([]Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `select raw from xz_ppt_tasks where user_id=$1 order by created_at desc`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Task{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		task, err := taskFromPostgresRaw(raw, userID)
		if err != nil {
			return nil, err
		}
		items = append(items, materializeTask(task))
	}
	return items, rows.Err()
}

func (s *Service) deletePostgres(userID, taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `delete from xz_ppt_tasks where task_id=$1 and user_id=$2`, strings.TrimSpace(taskID), strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrTaskNotFound
	}
	s.mirrorPostgresTasksBestEffort()
	return nil
}

func (s *Service) updateSlideImagePostgres(userID, taskID, slideID, imageURL string) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		slideID, imageURL := strings.TrimSpace(slideID), strings.TrimSpace(imageURL)
		for i := range task.Slides {
			if task.Slides[i].ID != slideID {
				continue
			}
			if old := strings.TrimSpace(task.Slides[i].ImageURL); old != "" && old != imageURL {
				task.Slides[i].VisualHistory = append(task.Slides[i].VisualHistory, VisualAsset{URL: old, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
			}
			task.Slides[i].ImageURL = imageURL
			task.Slides[i].VisualStatus = "success"
			task.Slides[i].VisualError = ""
			return nil
		}
		return ErrTaskNotFound
	})
}

func (s *Service) attachV2ArtifactPostgres(userID, taskID string, relation V2ArtifactRelation) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		task.V2DeckID = relation.DeckID
		task.V2Revision = relation.Revision
		task.PPTXAssetID = relation.PPTXAssetID
		return nil
	})
}

func (s *Service) updateSlideContentPostgres(userID, taskID, slideID string, update Slide) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		return applySlideContentUpdate(task, slideID, update)
	})
}

func (s *Service) updateSlideVisualPlanPostgres(userID, taskID, slideID string, plan VisualPlan, visualTaskID, status, errorMessage string) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		for i := range task.Slides {
			if task.Slides[i].ID != strings.TrimSpace(slideID) {
				continue
			}
			task.Slides[i].VisualPlan = &plan
			if visualTaskID = strings.TrimSpace(visualTaskID); visualTaskID != "" {
				task.Slides[i].VisualTaskID = visualTaskID
			}
			task.Slides[i].VisualStatus = strings.TrimSpace(status)
			task.Slides[i].VisualError = strings.TrimSpace(errorMessage)
			return nil
		}
		return ErrTaskNotFound
	})
}

func (s *Service) disableSlideVisualPostgres(userID, taskID, slideID string, plan VisualPlan) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		return disableSlideVisual(task, slideID, plan)
	})
}

func (s *Service) completeSlideVisualPostgres(userID, taskID, slideID string, plan VisualPlan, asset VisualAsset) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		return completeSlideVisual(task, slideID, plan, asset)
	})
}

func (s *Service) restoreSlideVisualPostgres(userID, taskID, slideID, createdAt, imageURL string) (Task, error) {
	return s.updatePostgresTask(userID, taskID, func(task *Task) error {
		return restoreSlideVisual(task, slideID, createdAt, imageURL)
	})
}

func (s *Service) updatePostgresTask(userID, taskID string, update func(*Task) error) (Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	if err := s.ensurePostgresReady(ctx); err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var raw []byte
	if err := tx.QueryRowContext(ctx, `select raw from xz_ppt_tasks where task_id=$1 and user_id=$2 for update`, strings.TrimSpace(taskID), strings.TrimSpace(userID)).Scan(&raw); errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	} else if err != nil {
		return Task{}, err
	}
	task, err := taskFromPostgresRaw(raw, userID)
	if err != nil {
		return Task{}, err
	}
	task = cloneTask(task)
	if err := update(&task); err != nil {
		return Task{}, err
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := persistPostgresTask(ctx, tx, task); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	s.mirrorPostgresTasksBestEffort()
	return task, nil
}

func (s *Service) mirrorPostgresTasksBestEffort() {
	if strings.TrimSpace(s.path) == "" {
		return
	}
	if err := s.mirrorPostgresTasksToFile(); err != nil {
		log.Printf("ppt postgres legacy mirror failed: %v", err)
	}
}

func (s *Service) mirrorPostgresTasksToFile() error {
	ctx, cancel := context.WithTimeout(context.Background(), postgresOperationTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext('xz_ppt_tasks_legacy_mirror'))`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `select user_id,raw from xz_ppt_tasks order by created_at desc`)
	if err != nil {
		return err
	}
	items := []persistedTask{}
	for rows.Next() {
		var userID string
		var raw []byte
		if err := rows.Scan(&userID, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		task, err := taskFromPostgresRaw(raw, userID)
		if err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, persistedTask{Task: task, UserID: userID})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Task.CreatedAt > items[j].Task.CreatedAt })
	payload, err := json.MarshalIndent(persistedState{Tasks: items}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o644); err != nil {
		return err
	}
	return tx.Commit()
}

func taskFromPostgresRaw(raw []byte, userID string) (Task, error) {
	var task Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return Task{}, err
	}
	task.UserID = strings.TrimSpace(userID)
	return normalizeLegacyTask(task), nil
}

func persistPostgresTask(ctx context.Context, tx *sql.Tx, task Task) error {
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	createdAt := parseTaskTime(task.CreatedAt)
	updatedAt := parseTaskTime(task.UpdatedAt)
	_, err = tx.ExecContext(ctx, `
insert into xz_ppt_tasks(task_id,user_id,client_request_id,status,created_at,updated_at,raw)
values($1,$2,$3,$4,$5,$6,$7::jsonb)
on conflict(task_id) do update set user_id=excluded.user_id,client_request_id=excluded.client_request_id,status=excluded.status,updated_at=excluded.updated_at,raw=excluded.raw
`, task.TaskID, task.UserID, task.ClientRequestID, task.Status, createdAt, updatedAt, string(raw))
	return err
}

func parseTaskTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}
