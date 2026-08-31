package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

const providerExecutionTaskParam = "_provider_execution_task_id"

func providerExecutionHooks(store platformStore, enabled bool) generation.ExecutionHooks {
	if !enabled {
		return generation.ExecutionHooks{}
	}
	pg, ok := store.(*postgresStore)
	if !ok || pg == nil || pg.db == nil {
		return generation.ExecutionHooks{}
	}
	s := pe.NewStore(pg.db)
	return generation.ExecutionHooks{Image: func(ctx context.Context, req generation.CreateRequest, p generation.ImageProvider) ([]generation.GeneratedImage, error) {
		return guardedImage(ctx, req, p, s)
	}, Video: func(ctx context.Context, req generation.CreateRequest, p generation.VideoProvider) (any, error) {
		return guardedVideo(ctx, req, p, s)
	}}
}

func executionIdentity(req generation.CreateRequest, capability, provider string) (pe.Execution, string, error) {
	taskID, _ := req.Params[providerExecutionTaskParam].(string)
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return pe.Execution{}, "", nil
	}
	params := cloneAnyMap(req.Params)
	delete(params, providerExecutionTaskParam)
	delete(params, "terminal")
	fp, err := pe.Fingerprint(taskID, provider, req.Model, capability, params)
	if err != nil {
		return pe.Execution{}, taskID, err
	}
	return pe.Execution{TaskID: taskID, Provider: provider, ProviderModel: req.Model, Capability: capability, RequestFingerprint: fp, Attempt: 1}, taskID, nil
}
func providerName(req generation.CreateRequest) string {
	for _, k := range []string{"provider", "providerName", "channel"} {
		if v, ok := req.Params[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "configured"
}

func guardedImage(ctx context.Context, req generation.CreateRequest, p generation.ImageProvider, s *pe.Store) ([]generation.GeneratedImage, error) {
	e, taskID, err := executionIdentity(req, "image", providerName(req))
	if err != nil || taskID == "" {
		return p.Generate(ctx, req)
	}
	active, err := s.GetActiveByTask(ctx, taskID)
	if err == nil {
		if active.Status == pe.Unknown || active.Status == pe.Submitting {
			return nil, pe.ErrUnknownResubmitBlocked
		}
		if active.Status == pe.Succeeded {
			return nil, fmt.Errorf("local recovery required for succeeded provider execution")
		}
		return nil, fmt.Errorf("provider execution already active: %s", active.Status)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	e, err = s.CreatePrepared(ctx, e)
	if err != nil {
		return nil, err
	}
	e, err = s.ClaimPrepared(ctx, taskID)
	if err != nil {
		return nil, err
	}
	images, callErr := p.Generate(ctx, req)
	if callErr != nil {
		class := pe.Classify(callErr)
		if class == pe.DefinitiveNotSubmitted || class == pe.RetryableBeforeSubmit {
			_ = s.Transition(ctx, e.ID, pe.Failed, nil, ptrString(string(class)), ptrString(callErr.Error()))
		} else {
			_ = s.MarkUnknown(ctx, e.ID, class, callErr.Error())
		}
		return nil, callErr
	}
	if err := s.Transition(ctx, e.ID, pe.Succeeded, nil, ptrString(string(pe.ProviderSucceeded)), nil); err != nil {
		return nil, err
	}
	return images, nil
}

func guardedVideo(ctx context.Context, req generation.CreateRequest, p generation.VideoProvider, s *pe.Store) (any, error) {
	e, taskID, err := executionIdentity(req, "video", providerName(req))
	if err != nil || taskID == "" {
		return p.Create(ctx, req)
	}
	active, err := s.GetActiveByTask(ctx, taskID)
	if err == nil {
		if active.Status == pe.Unknown || active.Status == pe.Submitting {
			return nil, pe.ErrUnknownResubmitBlocked
		}
		if active.Status == pe.Succeeded {
			return nil, fmt.Errorf("local recovery required for succeeded provider execution")
		}
		if active.ProviderRequestID != nil {
			return nil, fmt.Errorf("provider execution requires query recovery before create")
		}
		return nil, fmt.Errorf("provider execution already active: %s", active.Status)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	e, err = s.CreatePrepared(ctx, e)
	if err != nil {
		return nil, err
	}
	e, err = s.ClaimPrepared(ctx, taskID)
	if err != nil {
		return nil, err
	}
	result, callErr := p.Create(ctx, req)
	if callErr != nil {
		_ = s.MarkUnknown(ctx, e.ID, pe.Classify(callErr), callErr.Error())
		return nil, callErr
	}
	requestID := providerTaskID(result)
	if requestID == "" {
		_ = s.Transition(ctx, e.ID, pe.Succeeded, nil, ptrString(string(pe.ProviderSucceeded)), nil)
	} else if err := s.Transition(ctx, e.ID, pe.Submitted, ptrString(requestID), nil, nil); err != nil {
		return nil, err
	}
	return result, nil
}
func providerTaskID(v any) string {
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"providerTaskId", "provider_request_id", "providerRequestID", "task_id"} {
			if x, ok := m[k].(string); ok {
				return strings.TrimSpace(x)
			}
		}
	}
	return ""
}
func ptrString(v string) *string { return &v }

func providerExecutionForRetry(store platformStore, cfg config.Config, taskID string) (pe.Execution, bool, error) {
	if !cfg.ProviderExecutionSafetyEnabled {
		return pe.Execution{}, false, nil
	}
	pg, ok := store.(*postgresStore)
	if !ok || pg == nil || pg.db == nil {
		return pe.Execution{}, false, nil
	}
	e, err := pe.NewStore(pg.db).GetLatestByTask(context.Background(), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return pe.Execution{}, false, nil
	}
	if err != nil {
		return pe.Execution{}, false, err
	}
	return e, true, nil
}
