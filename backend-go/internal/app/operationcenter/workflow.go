package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrPaymentNotSuccessful   = errors.New("operation center payment is not successful")
	ErrPaymentAmountMismatch  = errors.New("operation center frozen payment amount mismatch")
	ErrFrozenSnapshotMissing  = errors.New("operation center frozen commercial snapshot is missing")
	ErrExpectedServiceStatus  = errors.New("operation center service status does not match expected status")
	ErrReviewDecisionConflict = errors.New("operation center review idempotency key has a different decision")
	ErrWorkflowUnavailable    = errors.New("operation center workflow is unavailable")
)

type PaymentSucceededCommand struct {
	OrderID         string
	PaymentRecordID string
}

type ReviewCommand struct {
	ServiceOrderID string
	Decision       ReviewDecision
	ExpectedStatus OperationCenterServiceStatus
	IdempotencyKey string
	ReviewedBy     string
	RequestID      string
	Reason         string
}

type WorkflowResult struct {
	ServiceOrder     *OperationCenterServiceOrder
	ReviewEvent      *OperationCenterReviewEvent
	RefundTask       *OperationCenterRefundTask
	IdempotentReplay bool
}

type ReviewStatusView struct {
	ServiceOrderID       string                       `json:"serviceOrderId"`
	OrderID              string                       `json:"orderId"`
	OrderNo              string                       `json:"orderNo"`
	ApplicantUserID      string                       `json:"applicantUserId"`
	Status               OperationCenterServiceStatus `json:"status"`
	RefundStatus         OperationCenterRefundStatus  `json:"refundStatus"`
	ReviewDecision       *ReviewDecision              `json:"reviewDecision,omitempty"`
	ReviewEventStatus    *ReviewEventStatus           `json:"reviewEventStatus,omitempty"`
	ReviewIdempotencyKey *string                      `json:"reviewIdempotencyKey,omitempty"`
	StateVersion         int64                        `json:"stateVersion"`
}

type ReferralEligibilityTrigger interface {
	MarkEligible(context.Context, *sql.Tx, *OperationCenterServiceOrder) error
}

type ReferralRewardGrantHook interface {
	GrantForServiceOrder(context.Context, *sql.Tx, *OperationCenterServiceOrder) ([]ReferralReward, error)
}

type OperationCenterActivationHook interface {
	AfterActivated(context.Context, *sql.Tx, *OperationCenterServiceOrder) error
}

type NoopReferralEligibilityTrigger struct{}

type NoopReferralRewardGrantHook struct{}

func (NoopReferralEligibilityTrigger) MarkEligible(context.Context, *sql.Tx, *OperationCenterServiceOrder) error {
	return nil
}

func (NoopReferralRewardGrantHook) GrantForServiceOrder(context.Context, *sql.Tx, *OperationCenterServiceOrder) ([]ReferralReward, error) {
	return nil, nil
}

type NoopOperationCenterActivationHook struct{}

func (NoopOperationCenterActivationHook) AfterActivated(context.Context, *sql.Tx, *OperationCenterServiceOrder) error {
	return nil
}

type WorkflowOptions struct {
	ReferralEligibilityTrigger ReferralEligibilityTrigger
	ReferralRewardGrantHook    ReferralRewardGrantHook
	ActivationHook             OperationCenterActivationHook
}

type WorkflowService struct {
	db                         *sql.DB
	store                      *PostgresStore
	referralEligibilityTrigger ReferralEligibilityTrigger
	referralRewardGrantHook    ReferralRewardGrantHook
	activationHook             OperationCenterActivationHook
}

func NewWorkflowService(db *sql.DB, options WorkflowOptions) (*WorkflowService, error) {
	store, err := NewPostgresStore(db)
	if err != nil {
		return nil, err
	}
	trigger := options.ReferralEligibilityTrigger
	if trigger == nil {
		trigger = NewPostgresReferralEligibilityTrigger()
	}
	grantHook := options.ReferralRewardGrantHook
	if grantHook == nil {
		grantHook = NewPostgresReferralRewardGrantService(store)
	}
	hook := options.ActivationHook
	if hook == nil {
		hook = NoopOperationCenterActivationHook{}
	}
	return &WorkflowService{db: db, store: store, referralEligibilityTrigger: trigger, referralRewardGrantHook: grantHook, activationHook: hook}, nil
}

type paidOrderSnapshot struct {
	OrderID, TenantID, UserID, OrderNo, PlanID, Currency string
	OrderStatus, PaymentRecordID, PaymentStatus          string
	PaymentChannel, ProviderPaymentNo                    string
	OrderAmountCents, PayableAmountCents                 int64
	PaymentAmountCents, PlanPriceCents                   int64
	CommercialSnapshotPaidCents                          int64
	PriceSnapshot, RelationshipSnapshot                  JSONSnapshot
	RefundPolicySnapshot                                 JSONSnapshot
	CommercialSnapshotID, PlanVersionID, RuleSetID       string
	RuleSetVersion                                       int
	PaidAt                                               sql.NullTime
}

func validatePaymentAmounts(snapshot paidOrderSnapshot) error {
	price, ok := snapshotMoneyCents(snapshot.PriceSnapshot, "priceCents")
	if !ok {
		price, ok = snapshotMoneyCents(snapshot.PriceSnapshot, "payableAmountCents")
	}
	if !ok {
		return ErrFrozenSnapshotMissing
	}
	expected := snapshot.OrderAmountCents
	if snapshot.PayableAmountCents > 0 {
		expected = snapshot.PayableAmountCents
	}
	if expected <= 0 || snapshot.OrderAmountCents != expected || snapshot.PaymentAmountCents != expected ||
		snapshot.PlanPriceCents != expected || snapshot.CommercialSnapshotPaidCents != expected || price != expected {
		return fmt.Errorf("%w: order=%d payable=%d payment=%d plan=%d commercial_snapshot=%d price_snapshot=%d",
			ErrPaymentAmountMismatch, snapshot.OrderAmountCents, snapshot.PayableAmountCents,
			snapshot.PaymentAmountCents, snapshot.PlanPriceCents, snapshot.CommercialSnapshotPaidCents, price)
	}
	return nil
}

func snapshotMoneyCents(snapshot JSONSnapshot, key string) (int64, bool) {
	value, exists := snapshot[key]
	if !exists {
		return 0, false
	}
	switch number := value.(type) {
	case float64:
		converted := int64(number)
		return converted, float64(converted) == number
	case int64:
		return number, true
	case int:
		return int64(number), true
	default:
		return 0, false
	}
}

func snapshotObject(snapshot JSONSnapshot, key string) (JSONSnapshot, bool) {
	value, exists := snapshot[key]
	if !exists {
		return nil, false
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return JSONSnapshot(object), true
}
