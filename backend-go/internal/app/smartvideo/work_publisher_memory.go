package smartvideo

import (
	"context"
	"sync"
)

// MemoryWorkPublisher records private works for settle/billing tests.
type MemoryWorkPublisher struct {
	mu      sync.Mutex
	works   map[string]WorkPublishInput
	fail    error
	counter int
}

func NewMemoryWorkPublisher() *MemoryWorkPublisher {
	return &MemoryWorkPublisher{works: map[string]WorkPublishInput{}}
}

func (p *MemoryWorkPublisher) SetFail(err error) { p.mu.Lock(); defer p.mu.Unlock(); p.fail = err }

func (p *MemoryWorkPublisher) PublishVideo(context.Context, Access, string, string, string) (string, error) {
	return "", ErrSettleNotReady
}

func (p *MemoryWorkPublisher) PublishPrivateWork(_ context.Context, input WorkPublishInput) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail != nil {
		return "", p.fail
	}
	if input.RenderTaskID != "" {
		for id, existing := range p.works {
			if existing.RenderTaskID == input.RenderTaskID {
				return id, nil
			}
		}
	}
	p.counter++
	id := newID("work")
	p.works[id] = input
	return id, nil
}

func (p *MemoryWorkPublisher) Get(workID string) (WorkPublishInput, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.works[workID]
	return item, ok
}

func (p *MemoryWorkPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.works)
}
