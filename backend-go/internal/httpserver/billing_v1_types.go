package httpserver

import "strings"

const (
	taskStatusCreated   = "CREATED"
	taskStatusQueued    = "QUEUED"
	taskStatusRunning   = "RUNNING"
	taskStatusSucceeded = "SUCCEEDED"
	taskStatusFailed    = "FAILED"
	taskStatusCancelled = "CANCELLED"

	billingStatusUnquoted      = "UNQUOTED"
	billingStatusQuoted        = "QUOTED"
	billingStatusReserved      = "RESERVED"
	billingStatusCaptured      = "CAPTURED"
	billingStatusReleased      = "RELEASED"
	billingStatusRefunded      = "REFUNDED"
	billingStatusBillingFailed = "BILLING_FAILED"
)

type billingRuleVersion struct {
	ID               string                      `json:"id"`
	RuleKey          string                      `json:"ruleKey"`
	LegacyRuleID     string                      `json:"legacyRuleId,omitempty"`
	ModelName        string                      `json:"modelName"`
	ModelCode        string                      `json:"modelCode"`
	ModuleCode       string                      `json:"moduleCode"`
	BillingUnit      string                      `json:"billingUnit"`
	BasePrice        float64                     `json:"basePrice"`
	MinimumCharge    float64                     `json:"minimumCharge"`
	ParameterRules   map[string]any              `json:"parameterRules"`
	RuleSource       string                      `json:"ruleSource"`
	TenantID         string                      `json:"tenantId,omitempty"`
	PlanID           string                      `json:"planId,omitempty"`
	Version          int                         `json:"version"`
	Status           string                      `json:"status"`
	EffectiveFrom    string                      `json:"effectiveFrom,omitempty"`
	EffectiveTo      string                      `json:"effectiveTo,omitempty"`
	ValidationResult billingRuleValidationResult `json:"validationResult"`
	CreatedBy        string                      `json:"createdBy,omitempty"`
	CreatedAt        string                      `json:"createdAt"`
	UpdatedAt        string                      `json:"updatedAt"`
	PublishedAt      string                      `json:"publishedAt,omitempty"`
}

type billingRuleValidationIssue struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type billingRuleValidationResult struct {
	Valid       bool                         `json:"valid"`
	ValidatedAt string                       `json:"validatedAt,omitempty"`
	Issues      []billingRuleValidationIssue `json:"issues"`
}

type providerCost struct {
	ID                string         `json:"id"`
	Provider          string         `json:"provider"`
	Channel           string         `json:"channel"`
	PlatformModelCode string         `json:"platformModelCode"`
	UpstreamModelName string         `json:"upstreamModelName"`
	BillingUnit       string         `json:"billingUnit"`
	ParameterRange    map[string]any `json:"parameterRange"`
	UnitCost          float64        `json:"unitCost"`
	Currency          string         `json:"currency"`
	EffectiveFrom     string         `json:"effectiveFrom"`
	EffectiveTo       string         `json:"effectiveTo,omitempty"`
	Status            string         `json:"status"`
	CreatedAt         string         `json:"createdAt,omitempty"`
	UpdatedAt         string         `json:"updatedAt"`
}

type providerCostMutation struct {
	Provider          string         `json:"provider"`
	Channel           string         `json:"channel"`
	PlatformModelCode string         `json:"platformModelCode"`
	UpstreamModelName string         `json:"upstreamModelName"`
	BillingUnit       string         `json:"billingUnit"`
	ParameterRange    map[string]any `json:"parameterRange"`
	UnitCost          *float64       `json:"unitCost"`
	Currency          string         `json:"currency"`
	EffectiveFrom     string         `json:"effectiveFrom"`
	EffectiveTo       string         `json:"effectiveTo"`
	Status            string         `json:"status"`
}

type billingLifecycleEvent struct {
	ID              string         `json:"id"`
	TaskID          string         `json:"taskId"`
	UserID          string         `json:"userId,omitempty"`
	TenantID        string         `json:"tenantId,omitempty"`
	ModelCode       string         `json:"modelCode,omitempty"`
	EventType       string         `json:"eventType"`
	BillingStatus   string         `json:"billingStatus"`
	Points          float64        `json:"points"`
	RuleVersionID   string         `json:"ruleVersionId,omitempty"`
	ProviderChannel string         `json:"providerChannel,omitempty"`
	IdempotencyKey  string         `json:"idempotencyKey"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       string         `json:"createdAt"`
}

type walletLedgerEntry struct {
	ID              string         `json:"id"`
	AccountID       string         `json:"accountId"`
	UserID          string         `json:"userId,omitempty"`
	TenantID        string         `json:"tenantId,omitempty"`
	TaskID          string         `json:"taskId,omitempty"`
	BillingEventID  string         `json:"billingEventId,omitempty"`
	EntryType       string         `json:"entryType"`
	Points          float64        `json:"points"`
	AvailableBefore float64        `json:"availableBefore"`
	AvailableAfter  float64        `json:"availableAfter"`
	FrozenBefore    float64        `json:"frozenBefore"`
	FrozenAfter     float64        `json:"frozenAfter"`
	IdempotencyKey  string         `json:"idempotencyKey"`
	ReferenceType   string         `json:"referenceType,omitempty"`
	ReferenceID     string         `json:"referenceId,omitempty"`
	Remark          string         `json:"remark,omitempty"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       string         `json:"createdAt"`
}

type billingReconciliationItem struct {
	TaskID            string   `json:"taskId"`
	UserID            string   `json:"userId"`
	TenantID          string   `json:"tenantId,omitempty"`
	ModelCode         string   `json:"modelCode"`
	TaskStatus        string   `json:"taskStatus"`
	BillingStatus     string   `json:"billingStatus"`
	QuotedPoints      float64  `json:"quotedPoints"`
	ReservedPoints    float64  `json:"reservedPoints"`
	CapturedPoints    float64  `json:"capturedPoints"`
	ReleasedPoints    float64  `json:"releasedPoints"`
	RefundedPoints    float64  `json:"refundedPoints"`
	SupplierCost      *float64 `json:"supplierCost"`
	EstimatedMargin   *float64 `json:"estimatedMargin"`
	ProviderChannel   string   `json:"providerChannel,omitempty"`
	RuleVersionID     string   `json:"ruleVersionId,omitempty"`
	ClientRequestID   string   `json:"clientRequestId,omitempty"`
	BillingEventCount int      `json:"billingEventCount"`
	WalletLedgerCount int      `json:"walletLedgerCount"`
	Anomalies         []string `json:"anomalies"`
	CreatedAt         string   `json:"createdAt"`
}

type billingV1Store interface {
	ListBillingRuleVersions() ([]billingRuleVersion, error)
	GetBillingRuleVersion(string) (billingRuleVersion, error)
	ValidateBillingRuleVersion(string) (billingRuleValidationResult, error)
	PublishBillingRuleVersion(string) (billingRuleVersion, error)
	ListProviderCosts() ([]providerCost, error)
	UpdateProviderCost(string, providerCostMutation) (providerCost, error)
	ListBillingReconciliation() ([]billingReconciliationItem, error)
	ListWalletLedger() ([]walletLedgerEntry, error)
	ListBillingLifecycleEvents() ([]billingLifecycleEvent, error)
}

func canonicalTaskStatus(status string) string {
	switch upperTrim(status) {
	case "PENDING", "QUEUED":
		return taskStatusQueued
	case "PROCESSING", "RUNNING":
		return taskStatusRunning
	case "COMPLETED", "SUCCEEDED":
		return taskStatusSucceeded
	case "FAILED":
		return taskStatusFailed
	case "CANCELLED", "CANCELED":
		return taskStatusCancelled
	default:
		return taskStatusCreated
	}
}

func upperTrim(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
