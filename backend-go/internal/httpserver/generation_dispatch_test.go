package httpserver

import (
	"errors"
	"testing"
	"xianzhi-ai/backend-go/internal/messaging"
)

func TestImageDispatchRoutingSeparatesNormalAndCanary(t *testing.T) {
	for _, tc := range []struct {
		name        string
		params      map[string]any
		route, mode string
		invalid     bool
	}{
		{"normal", map[string]any{imageDispatchModeParam: imageDispatchNormal}, messaging.GenerationImageNormalRoutingKey, imageDispatchNormal, false},
		{"canary", map[string]any{imageDispatchModeParam: imageDispatchCanary, "generation_async_canary": true}, messaging.GenerationCanaryRoutingKey, imageDispatchCanary, false},
		{"legacy normal", nil, messaging.GenerationImageNormalRoutingKey, imageDispatchNormal, false},
		{"legacy admitted canary", map[string]any{"generation_async_canary": true}, messaging.GenerationCanaryRoutingKey, imageDispatchCanary, false},
		{"forged canary", map[string]any{imageDispatchModeParam: imageDispatchCanary}, "", "", true},
		{"ambiguous normal", map[string]any{imageDispatchModeParam: imageDispatchNormal, "generation_async_canary": true}, "", "", true},
		{"unknown", map[string]any{imageDispatchModeParam: "other"}, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			route, mode, err := imageDispatchRouting(tc.params)
			if tc.invalid {
				if err == nil {
					t.Fatal("accepted invalid protocol")
				}
				return
			}
			if err != nil || route != tc.route || mode != tc.mode {
				t.Fatalf("route=%s mode=%s err=%v", route, mode, err)
			}
		})
	}
}
func TestImageDispatchConsumerProtocol(t *testing.T) {
	for _, mode := range []string{imageDispatchNormal, imageDispatchCanary} {
		route := messaging.GenerationImageNormalRoutingKey
		if mode == imageDispatchCanary {
			route = messaging.GenerationCanaryRoutingKey
		}
		e := &messaging.Envelope{EventType: route, AggregateType: "generation_task", AggregateID: "task", Data: map[string]any{"dispatch_mode": mode, "execution_generation": int64(2)}}
		if err := validateImageDispatchEnvelope(e, mode); err != nil {
			t.Fatal(err)
		}
		other := imageDispatchNormal
		if mode == other {
			other = imageDispatchCanary
		}
		if err := validateImageDispatchEnvelope(e, other); err == nil {
			t.Fatal("cross-consumed dispatch")
		}
	}
	e := &messaging.Envelope{EventType: messaging.GenerationImageNormalRoutingKey, AggregateType: "generation_task", AggregateID: "task", Data: map[string]any{}}
	if err := validateImageDispatchEnvelope(e, imageDispatchNormal); err == nil {
		t.Fatal("normal accepted missing protocol identity")
	}
	e.EventType = messaging.GenerationCanaryRoutingKey
	if err := validateImageDispatchEnvelope(e, imageDispatchCanary); err != nil {
		t.Fatalf("legacy canary requested no longer drains: %v", err)
	}
	e.Data["dispatch_mode"] = true
	if err := validateImageDispatchEnvelope(e, imageDispatchCanary); err == nil {
		t.Fatal("accepted non-string mode")
	}
	e.Data = map[string]any{"task_id": "other"}
	if err := validateImageDispatchEnvelope(e, imageDispatchCanary); err == nil {
		t.Fatal("accepted conflicting task identity")
	}
}
func TestImageDispatchClaimStateMachine(t *testing.T) {
	for _, tc := range []struct {
		name, state, worker, dispatchOwner string
		g, expected                        int64
		live                               bool
		want                               error
	}{
		{"fresh", "QUEUED", "", "", 1, 0, false, nil},
		{"live worker", "RUNNING", "worker-a", "", 2, 0, true, errGenerationOwnershipBusy},
		{"inconsistent queued lease", "QUEUED", "worker-a", "", 2, 0, true, errGenerationOwnershipBusy},
		{"handoff", "DISPATCHING", "scheduler", "scheduler", 2, 2, true, nil},
		{"wrong dispatch", "DISPATCHING", "scheduler", "scheduler", 3, 2, true, ErrFencedStaleExecution},
		{"missing dispatch", "DISPATCHING", "scheduler", "scheduler", 2, 0, true, ErrFencedStaleExecution},
		{"forged scheduler owner", "DISPATCHING", "worker", "scheduler", 2, 2, true, ErrFencedStaleExecution},
		{"expired retry", "RUNNING", "worker-a", "", 3, 2, false, nil},
		{"future event", "RUNNING", "", "", 2, 3, false, ErrFencedStaleExecution},
		{"terminal", "SUCCEEDED", "", "", 3, 0, false, ErrFencedStaleExecution},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGenerationClaim(tc.state, tc.worker, tc.dispatchOwner, tc.g, tc.expected, tc.live, true)
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}
