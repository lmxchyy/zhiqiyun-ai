package operationcenter

import (
	"context"
	"time"
)

type RefundRequestCommand struct {
	ServiceOrderID, IdempotencyKey, Reason string
	ExpectedServiceStatus                  OperationCenterServiceStatus
	RequestedBy, RequestID                 string
}

type ManualRefundSubmitCommand struct {
	RefundTaskID, IdempotencyKey, ChannelRefundNo string
	VoucherReference, VoucherFileHash, Reason     string
	RefundAmountCents                             int64
	SubmittedBy, RequestID                        string
}

type ManualRefundReviewCommand struct {
	RefundTaskID, IdempotencyKey, Decision, Reason string
	ExpectedStatus                                 ManualRefundStatus
	ReviewedBy, RequestID                          string
}

type RefundManagementResult struct {
	RefundTask       *OperationCenterRefundTask   `json:"refundTask,omitempty"`
	ManualRefund     *OperationCenterManualRefund `json:"manualRefund,omitempty"`
	IdempotentReplay bool                         `json:"idempotentReplay"`
}

type RefundAuditSummary struct {
	FromStatus string    `json:"fromStatus,omitempty"`
	ToStatus   string    `json:"toStatus"`
	Reason     string    `json:"reason"`
	OperatorID *string   `json:"operatorId,omitempty"`
	RequestID  *string   `json:"requestId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type RefundManagementView struct {
	RefundTaskID                 string                       `json:"refundTaskId"`
	TenantID                     string                       `json:"tenantId"`
	ServiceOrderID               string                       `json:"serviceOrderId"`
	OrderID                      string                       `json:"orderId"`
	ServiceStatus                OperationCenterServiceStatus `json:"serviceStatus"`
	RefundStatus                 OperationCenterRefundStatus  `json:"refundStatus"`
	AmountCents                  int64                        `json:"amountCents"`
	Currency                     string                       `json:"currency"`
	PaymentChannel               string                       `json:"paymentChannel"`
	ProviderResult               *RefundProviderResult        `json:"providerResult,omitempty"`
	ProviderRefundNo             *string                      `json:"providerRefundNo,omitempty"`
	FailureClass                 *string                      `json:"failureClass,omitempty"`
	ProviderResponseSummary      JSONSnapshot                 `json:"providerResponseSummary"`
	ProviderQueryResponseSummary JSONSnapshot                 `json:"providerQueryResponseSummary"`
	AttemptCount                 int                          `json:"attemptCount"`
	VerificationAttemptCount     int                          `json:"verificationAttemptCount"`
	NextRetryAt                  *time.Time                   `json:"nextRetryAt,omitempty"`
	LastVerificationAt           *time.Time                   `json:"lastVerificationAt,omitempty"`
	CreatedAt                    *time.Time                   `json:"createdAt,omitempty"`
	ManualRefundStatus           *ManualRefundStatus          `json:"manualRefundStatus,omitempty"`
	ManualRefundID               *string                      `json:"manualRefundId,omitempty"`
	ManualVoucherReference       *string                      `json:"manualVoucherReference,omitempty"`
	RequiresManualAction         bool                         `json:"requiresManualAction"`
	Audit                        []RefundAuditSummary         `json:"audit"`
}

type RefundListFilter struct {
	TenantID, ServiceOrderID                string
	ServiceStatus                           OperationCenterServiceStatus
	RefundStatus                            OperationCenterRefundStatus
	ProviderResult                          RefundProviderResult
	FailureClass                            RefundFailureClass
	PaymentChannel                          string
	NextRetryBefore, CreatedFrom, CreatedTo *time.Time
	Limit, Offset                           int
}

type RefundRequestEvent struct {
	ID, TenantID, ServiceOrderID, RefundTaskID     string
	RequestedBy, RequestID, IdempotencyKey, Reason string
	ExpectedServiceStatus                          OperationCenterServiceStatus
	Snapshot                                       JSONSnapshot
	CreatedAt                                      time.Time
}

type ManualRefundEvent struct {
	ID, TenantID, RefundTaskID, ManualRefundID            string
	EventType, ActorID, RequestID, IdempotencyKey, Reason string
	BeforeStatus, AfterStatus                             OperationCenterRefundStatus
	Snapshot                                              JSONSnapshot
	CreatedAt                                             time.Time
}

type RefundManagement interface {
	RequestActiveRefund(context.Context, RefundRequestCommand) (RefundManagementResult, error)
	GetRefund(context.Context, string) (RefundManagementView, error)
	ListRefunds(context.Context, RefundListFilter) ([]RefundManagementView, error)
	SubmitManualRefund(context.Context, ManualRefundSubmitCommand) (RefundManagementResult, error)
	ReviewManualRefund(context.Context, ManualRefundReviewCommand) (RefundManagementResult, error)
}
