package smartvideoplan

import (
	"context"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

// DomainAdapter adapts the provider client to smartvideo.EditPlanner.
type DomainAdapter struct {
	Client *Client
}

func (a DomainAdapter) Plan(ctx context.Context, request smartvideo.PlanRequest) (smartvideo.EditPlanV1, smartvideo.PlanProviderUsage, error) {
	plan, usage, err := a.Client.Plan(ctx, request)
	return plan, smartvideo.PlanProviderUsage{
		ModelKey: usage.ModelKey, ProviderRequestID: usage.ProviderRequestID,
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		TotalTokens: usage.TotalTokens, LatencyMs: usage.LatencyMs,
	}, err
}
