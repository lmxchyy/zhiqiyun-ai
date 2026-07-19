package connector

import "context"

// AICommand is the platform-neutral command passed from a connector adapter to
// a capability handler. Tenant identity is resolved from ConnectorID and is
// never accepted from the external message payload.
type AICommand struct {
	EnterpriseID      string           `json:"enterprise_id"`
	InternalUserID    string           `json:"internal_user_id"`
	ExternalUserID    string           `json:"external_user_id"`
	ConnectorID       string           `json:"connector_id"`
	Source            string           `json:"source"`
	ChatID            string           `json:"chat_id"`
	ExternalMessageID string           `json:"external_message_id"`
	OriginalText      string           `json:"original_text"`
	Intent            string           `json:"intent"`
	Parameters        map[string]any   `json:"parameters"`
	ReferenceAssets   []ReferenceAsset `json:"reference_assets,omitempty"`
	Context           map[string]any   `json:"context,omitempty"`
}

type ReferenceAsset struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id,omitempty"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"media_type"`
	URL       string `json:"-"`
}

type CapabilityResult struct {
	InternalTaskID string         `json:"internal_task_id,omitempty"`
	Status         string         `json:"status"`
	Progress       int            `json:"progress"`
	EstimatedCost  int64          `json:"estimated_cost,omitempty"`
	ActualCost     int64          `json:"actual_cost,omitempty"`
	AssetIDs       []string       `json:"asset_ids,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
}

// CapabilityHandler is intentionally independent from Feishu SDK types so the
// same handlers can later be reused by DingTalk and WeCom adapters.
type CapabilityHandler interface {
	CanHandle(AICommand) bool
	Validate(context.Context, AICommand) error
	EstimateCost(context.Context, AICommand) (int64, error)
	Execute(context.Context, AICommand) (CapabilityResult, error)
	QueryStatus(context.Context, AICommand) (CapabilityResult, error)
	BuildResult(context.Context, AICommand, CapabilityResult) (OutgoingMessage, error)
}
