package operationcenter

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type JSONSnapshot map[string]any

func (snapshot JSONSnapshot) Value() (driver.Value, error) {
	if snapshot == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(snapshot)
}

func (snapshot *JSONSnapshot) Scan(source any) error {
	if source == nil {
		*snapshot = JSONSnapshot{}
		return nil
	}
	var raw []byte
	switch value := source.(type) {
	case []byte:
		raw = value
	case string:
		raw = []byte(value)
	default:
		return fmt.Errorf("%w: unsupported source %T", ErrInvalidJSONSnapshot, source)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidJSONSnapshot, err)
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: snapshot must be an object", ErrInvalidJSONSnapshot)
	}
	*snapshot = JSONSnapshot(object)
	return nil
}

type OperationCenterServiceOrder struct {
	ID, TenantID, OrderID, OrderNo, ApplicantUserID string
	TechnicalServiceFeeCents                        int64
	Currency                                        string
	Status                                          OperationCenterServiceStatus
	RefundStatus                                    OperationCenterRefundStatus
	PaidAt, ReviewedAt, ActivatedAt, RevokedAt      *time.Time
	ReviewedBy, RefundOrderID                       *string
	StateVersion                                    int64
	Metadata                                        JSONSnapshot
	CreatedAt, UpdatedAt                            time.Time
	CommercialRuleSetID                             *string
	CommercialRuleSetVersion                        *int
	PlanVersionID, CommercialOrderSnapshotID        *string
	RelationshipSnapshot, RefundPolicySnapshot      JSONSnapshot
	ReviewIdempotencyKey, RefundIdempotencyKey      *string
	PaymentChannel, ProviderRefundNo                *string
	RefundFailureClass                              *RefundFailureClass
	RefundFailureDetail                             JSONSnapshot
	RefundAttemptCount                              int
	NextRefundRetryAt                               *time.Time
	ManualRefundVoucherReference                    *string
	ManualRefundVoucherFileHash                     *string
	ManualRefundSubmittedBy                         *string
	ManualRefundApprovedBy                          *string
	CurrentRefundTaskID                             *string
}

type OperationCenterRefundTask struct {
	ID, TenantID, ServiceOrderID, OrderID string
	PaymentRecordID                       *string
	CommercialRuleSetID                   string
	Origin                                RefundOrigin
	Scope                                 RefundScope
	AmountCents                           int64
	Currency, PaymentChannel              string
	ProviderPaymentNo, ProviderRefundNo   *string
	ProviderOutcome                       *RefundProviderResult
	Status                                RefundTaskStatus
	FailureClass                          *RefundFailureClass
	FailureDetail                         JSONSnapshot
	IdempotencyKey                        string
	AttemptCount                          int
	NextRetryAt                           *time.Time
	LeaseOwner                            *string
	LeaseExpiresAt, UnknownSince          *time.Time
	PreparedAt, CompletedAt               *time.Time
	ProviderRefundedAt                    *time.Time
	ProviderResponseSummary               JSONSnapshot
	ProviderQueryOutcome                  *payment.QueryRefundOutcome
	ProviderQueryResponseSummary          JSONSnapshot
	VerificationAttemptCount              int
	LastVerificationAt                    *time.Time
	ManualProviderTransactionID           *string
	ManualVoucherReference                *string
	ManualVoucherFileHash                 *string
	ManualSubmittedBy, ManualApprovedBy   *string
	StateVersion                          int64
	CreatedAt, UpdatedAt                  time.Time
}

type OperationCenterManualRefund struct {
	ID, TenantID, RefundTaskID, PaymentChannel string
	AmountCents                                int64
	Currency, ProviderTransactionID            string
	ProviderRefundNo                           *string
	VoucherReference, VoucherFileHash          string
	Status                                     ManualRefundStatus
	SubmittedBy                                string
	SubmittedAt                                time.Time
	ApprovedBy                                 *string
	ApprovedAt                                 *time.Time
	RejectionReason, Remark                    *string
	CreatedAt, UpdatedAt                       time.Time
}

type OperationCenterReviewEvent struct {
	ID, TenantID, ServiceOrderID string
	Decision                     ReviewDecision
	Status                       ReviewEventStatus
	ReviewedBy                   string
	RequestID                    *string
	IdempotencyKey               string
	FailureClass                 *RefundFailureClass
	FailureDetail, EventSnapshot JSONSnapshot
	AppliedAt                    *time.Time
	CreatedAt, UpdatedAt         time.Time
}

type ReferralRewardReleaseTask struct {
	ID, TenantID, ReferralRewardID, IdempotencyKey string
	Status                                         ReferralRewardReleaseTaskStatus
	ExecuteAt                                      time.Time
	AttemptCount                                   int
	NextRetryAt                                    *time.Time
	LeaseOwner                                     *string
	LeaseExpiresAt                                 *time.Time
	FailureClass                                   *RefundFailureClass
	FailureDetail                                  JSONSnapshot
	StartedAt, CompletedAt                         *time.Time
	CancellationReason                             *string
	CancelledAt                                    *time.Time
	CreatedAt, UpdatedAt                           time.Time
}

type OperationCenterStateTransition struct {
	ID, TenantID               string
	EntityType                 StateTransitionEntityType
	EntityID                   string
	FromStatus                 *string
	ToStatus, TransitionReason string
	TransactionGroupID         string
	OperatorID, RequestID      *string
	IdempotencyKey             string
	Metadata                   JSONSnapshot
	CreatedAt                  time.Time
}

func nullableStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullableTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullableIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}
