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
	latest, err := s.GetLatestByTask(ctx, taskID)
	if err == nil {
		switch latest.Status {
		case pe.Unknown, pe.Submitting, pe.Submitted, pe.Processing:
			return nil, pe.ErrUnknownResubmitBlocked
		case pe.Succeeded:
			return nil, fmt.Errorf("local recovery required for succeeded provider execution")
		case pe.Failed:
			if latest.ErrorClass == nil || (*latest.ErrorClass != string(pe.DefinitiveNotSubmitted) && *latest.ErrorClass != string(pe.RetryableBeforeSubmit)) {
				return nil, pe.ErrUnknownResubmitBlocked
			}
			e.Attempt = latest.Attempt + 1
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
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
	latest, err := s.GetLatestByTask(ctx, taskID)
	if err == nil {
		switch latest.Status {
		case pe.Unknown, pe.Submitting, pe.Submitted, pe.Processing:
			if latest.ProviderRequestID != nil {
				if getter, ok := p.(interface {
					Get(context.Context, string) (any, error)
				}); ok {
					result, queryErr := getter.Get(ctx, *latest.ProviderRequestID)
					if queryErr == nil {
						status := providerExecutionStatus(result)
						if status == pe.Succeeded || status == pe.Failed {
							_ = s.Transition(ctx, latest.ID, status, latest.ProviderRequestID, nil, nil)
							return result, nil
						}
						_ = s.Transition(ctx, latest.ID, pe.Processing, latest.ProviderRequestID, ptrString(string(pe.ProviderProcessing)), nil)
						return nil, pe.ErrProviderStillProcessing
					}
				}
			}
			return nil, pe.ErrUnknownResubmitBlocked
		case pe.Succeeded:
			if latest.ProviderRequestID == nil {
				return nil, fmt.Errorf("local recovery required for succeeded provider execution")
			}
			getter, ok := p.(interface {
				Get(context.Context, string) (any, error)
			})
			if !ok {
				return nil, fmt.Errorf("video provider does not support query recovery")
			}
			result, queryErr := getter.Get(ctx, *latest.ProviderRequestID)
			if queryErr != nil {
				return nil, queryErr
			}
			if providerExecutionStatus(result) != pe.Succeeded {
				return nil, pe.ErrProviderStillProcessing
			}
			return result, nil
		case pe.Failed:
			if latest.ErrorClass == nil || (*latest.ErrorClass != string(pe.DefinitiveNotSubmitted) && *latest.ErrorClass != string(pe.RetryableBeforeSubmit)) {
				return nil, pe.ErrUnknownResubmitBlocked
			}
			e.Attempt = latest.Attempt + 1
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
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
func providerExecutionStatus(v any) pe.Status {
	m, ok := v.(map[string]any)
	if !ok {
		return pe.Unknown
	}
	s, _ := m["status"].(string)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "succeeded", "completed":
		return pe.Succeeded
	case "failed", "error":
		return pe.Failed
	case "processing", "running":
		return pe.Processing
	case "queued", "pending", "submitted":
		return pe.Submitted
	default:
		return pe.Unknown
	}
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
