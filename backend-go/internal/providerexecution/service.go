package providerexecution

import (
	"context"
	"fmt"
)

type Submission struct {
	ProviderRequestID string
	Succeeded         bool
	ResultMetadata    map[string]any
}
type QueryResult struct {
	Status            Status
	ProviderRequestID string
	ResultMetadata    map[string]any
}
type Adapter interface {
	Submit(context.Context) (Submission, error)
	Query(context.Context, string) (QueryResult, error)
}

// Service coordinates persistence around an adapter. Adapters remain unaware
// of database state and credentials are never accepted as execution data.
type Service struct {
	Store  *Store
	Policy ProviderPolicy
}

func (s *Service) Execute(ctx context.Context, e Execution, a Adapter) (Execution, error) {
	if s == nil || s.Store == nil || a == nil {
		return Execution{}, fmt.Errorf("provider execution dependencies are required")
	}
	created, err := s.Store.CreatePrepared(ctx, e)
	if err != nil {
		return Execution{}, err
	}
	current, err := s.Store.ClaimPrepared(ctx, created.TaskID)
	if err != nil {
		return created, err
	}
	sub, err := a.Submit(ctx)
	if err != nil {
		class := Classify(err)
		if class == DefinitiveNotSubmitted || class == RetryableBeforeSubmit {
			_ = s.Store.Transition(ctx, current.ID, Failed, nil, ptr(string(class)), ptr(err.Error()))
		} else {
			_ = s.Store.MarkUnknown(ctx, current.ID, class, err.Error())
		}
		return s.Store.GetByID(ctx, current.ID)
	}
	if sub.ProviderRequestID != "" {
		if err := s.Store.Transition(ctx, current.ID, Submitted, ptr(sub.ProviderRequestID), nil, nil); err != nil {
			return current, err
		}
	} else {
		if err := s.Store.Transition(ctx, current.ID, Succeeded, nil, nil, nil); err != nil {
			return current, err
		}
	}
	return s.Store.GetByID(ctx, current.ID)
}

func (s *Service) Recover(ctx context.Context, e Execution, a Adapter) (Execution, error) {
	if e.Status == Succeeded || e.Status == Failed {
		return e, nil
	}
	if e.ProviderRequestID == nil || *e.ProviderRequestID == "" {
		return e, ErrUnknownResubmitBlocked
	}
	q, err := a.Query(ctx, *e.ProviderRequestID)
	if err != nil {
		_ = s.Store.MarkUnknown(ctx, e.ID, ProviderUnknown, err.Error())
		return s.Store.GetByID(ctx, e.ID)
	}
	switch q.Status {
	case Submitted, Processing, Succeeded, Failed:
		err = s.Store.Transition(ctx, e.ID, q.Status, ptr(q.ProviderRequestID), nil, nil)
	default:
		err = s.Store.MarkUnknown(ctx, e.ID, ProviderUnknown, "provider returned unknown status")
	}
	if err != nil {
		return e, err
	}
	return s.Store.GetByID(ctx, e.ID)
}

func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ProviderSucceeded
	}
	if ctxErr, ok := err.(interface{ Timeout() bool }); ok && ctxErr.Timeout() {
		return PossiblySubmitted
	}
	return PossiblySubmitted
}
