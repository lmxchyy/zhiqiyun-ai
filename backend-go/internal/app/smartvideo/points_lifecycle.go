package smartvideo

import (
	"context"
	"sync"
	"time"
)

// MemoryPointsLifecycle is an in-memory ledger for unit tests.
// Production uses PersonalPointsLifecycle backed by PersonalPointService.
type MemoryPointsLifecycle struct {
	mu        sync.Mutex
	balance   int64
	reserved  map[string]int64
	captured  map[string]int64
	released  map[string]bool
	now       func() time.Time
}

func NewMemoryPointsLifecycle(balance int64) *MemoryPointsLifecycle {
	return &MemoryPointsLifecycle{
		balance:  balance,
		reserved: map[string]int64{},
		captured: map[string]int64{},
		released: map[string]bool{},
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (p *MemoryPointsLifecycle) Quote(_ context.Context, input RenderQuoteInput) (RenderQuote, error) {
	return EstimateRenderQuote(input, p.now()), nil
}

func (p *MemoryPointsLifecycle) Reserve(_ context.Context, _ Access, taskID string, quote RenderQuote) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if quote.ExpiresAt.Before(p.now()) {
		return "", ErrQuoteExpired
	}
	if _, ok := p.reserved[taskID]; ok {
		return "tx_" + taskID, nil
	}
	available := p.balance
	for id, pts := range p.reserved {
		if p.released[id] || p.captured[id] > 0 {
			continue
		}
		available -= pts
	}
	if quote.Points > available {
		return "", ErrInsufficientPoints
	}
	p.reserved[taskID] = quote.Points
	return "tx_" + taskID, nil
}

func (p *MemoryPointsLifecycle) Capture(_ context.Context, _ Access, taskID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	pts, ok := p.reserved[taskID]
	if !ok {
		return ErrNotFound
	}
	if p.released[taskID] {
		return ErrInvalidStateTransition
	}
	if p.captured[taskID] > 0 {
		return nil
	}
	p.captured[taskID] = pts
	p.balance -= pts
	return nil
}

func (p *MemoryPointsLifecycle) Release(_ context.Context, _ Access, taskID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.reserved[taskID]; !ok {
		return ErrNotFound
	}
	if p.captured[taskID] > 0 {
		return ErrInvalidStateTransition
	}
	p.released[taskID] = true
	return nil
}

func (p *MemoryPointsLifecycle) Reserved(taskID string) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reserved[taskID]
}

func (p *MemoryPointsLifecycle) IsReleased(taskID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.released[taskID]
}

func (p *MemoryPointsLifecycle) IsCaptured(taskID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.captured[taskID] > 0
}
