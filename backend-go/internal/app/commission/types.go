package commission

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AmountCents is the only cash representation allowed in the commission domain.
// One unit is one cent; callers must not convert through float values.
type AmountCents int64

// PercentageBPS stores a percentage in basis points. 10_000 means 100%.
type PercentageBPS int64

type CalculationType string

const (
	CalculationFixedAmount          CalculationType = "FIXED_AMOUNT"
	CalculationOrderPercentage      CalculationType = "ORDER_PERCENTAGE"
	CalculationPaidAmountPercentage CalculationType = "PAID_AMOUNT_PERCENTAGE"
	CalculationQuantity             CalculationType = "QUANTITY"
	CalculationTiered               CalculationType = "TIERED"
	CalculationRemainderToPlatform  CalculationType = "REMAINDER_TO_PLATFORM"
)

type CommissionRecordType string

const (
	RecordEarning    CommissionRecordType = "EARNING"
	RecordReversal   CommissionRecordType = "REVERSAL"
	RecordAdjustment CommissionRecordType = "ADJUSTMENT"
)

type CommissionStatus string

const (
	CommissionExpected  CommissionStatus = "EXPECTED"
	CommissionFrozen    CommissionStatus = "FROZEN"
	CommissionAvailable CommissionStatus = "AVAILABLE"
	CommissionSettling  CommissionStatus = "SETTLING"
	CommissionSettled   CommissionStatus = "SETTLED"
	CommissionReversed  CommissionStatus = "REVERSED"
	CommissionCancelled CommissionStatus = "CANCELLED"
)

type BeneficiaryType string

const (
	BeneficiaryAgent           BeneficiaryType = "AGENT"
	BeneficiaryOperationCenter BeneficiaryType = "OPERATION_CENTER"
	BeneficiaryPlatform        BeneficiaryType = "PLATFORM"
)

type CommissionRule struct {
	ID                string
	TenantID          string
	Code              string
	Name              string
	ProductType       string
	ProductID         string
	BeneficiaryRole   BeneficiaryType
	RelationshipLevel int
	CalculationType   CalculationType
	FixedAmountCents  AmountCents
	PercentageBPS     PercentageBPS
	CalculationConfig json.RawMessage
	Priority          int
	FreezeDays        int
	RefundPolicy      string
	EffectiveStartAt  time.Time
	EffectiveEndAt    *time.Time
	Version           int
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (r CommissionRule) Validate() error {
	if blank(r.ID) || blank(r.TenantID) || blank(r.Code) || blank(r.Name) || blank(r.ProductType) {
		return errors.New("commission rule identity, tenant, code, name and product type are required")
	}
	if !r.BeneficiaryRole.Valid() {
		return fmt.Errorf("unsupported beneficiary role %q", r.BeneficiaryRole)
	}
	if r.RelationshipLevel < 0 || r.FreezeDays < 0 || r.Version <= 0 {
		return errors.New("relationship level and freeze days must be non-negative and version must be positive")
	}
	if r.EffectiveStartAt.IsZero() {
		return errors.New("effective start time is required")
	}
	if r.EffectiveEndAt != nil && !r.EffectiveEndAt.After(r.EffectiveStartAt) {
		return errors.New("effective end time must be after start time")
	}
	switch strings.ToUpper(strings.TrimSpace(r.Status)) {
	case "DRAFT", "ACTIVE", "INACTIVE", "ARCHIVED":
	default:
		return fmt.Errorf("unsupported commission rule status %q", r.Status)
	}
	switch r.CalculationType {
	case CalculationFixedAmount, CalculationQuantity:
		if r.FixedAmountCents <= 0 {
			return errors.New("fixed and quantity rules require a positive integer-cent amount")
		}
	case CalculationOrderPercentage, CalculationPaidAmountPercentage:
		if r.PercentageBPS <= 0 || r.PercentageBPS > 10_000 {
			return errors.New("percentage rules require basis points between 1 and 10000")
		}
	case CalculationTiered:
		if len(r.CalculationConfig) == 0 || !json.Valid(r.CalculationConfig) {
			return errors.New("tiered rules require valid calculation config")
		}
	case CalculationRemainderToPlatform:
		if r.BeneficiaryRole != BeneficiaryPlatform {
			return errors.New("remainder rule beneficiary must be platform")
		}
	default:
		return fmt.Errorf("unsupported calculation type %q", r.CalculationType)
	}
	return nil
}

type CommissionRecord struct {
	ID              string
	TenantID        string
	OrderID         string
	OrderNo         string
	BeneficiaryType BeneficiaryType
	BeneficiaryID   string
	SourceUserID    string
	RuleID          string
	RuleVersion     int
	AmountCents     AmountCents
	Currency        string
	RecordType      CommissionRecordType
	Status          CommissionStatus
	FreezeUntil     *time.Time
	AvailableAt     *time.Time
	ReversalOfID    string
	IdempotencyKey  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (r CommissionRecord) Validate() error {
	if blank(r.ID) || blank(r.TenantID) || blank(r.OrderID) || blank(r.OrderNo) ||
		blank(r.BeneficiaryID) || blank(r.SourceUserID) || blank(r.RuleID) || blank(r.IdempotencyKey) {
		return errors.New("commission record identifiers and idempotency key are required")
	}
	if !r.BeneficiaryType.Valid() {
		return fmt.Errorf("unsupported beneficiary type %q", r.BeneficiaryType)
	}
	if r.RuleVersion <= 0 || r.AmountCents == 0 {
		return errors.New("rule version must be positive and amount must be non-zero")
	}
	if currency := strings.ToUpper(strings.TrimSpace(r.Currency)); len(currency) != 3 {
		return errors.New("currency must be a three-letter code")
	}
	if !r.Status.Valid() {
		return fmt.Errorf("unsupported commission status %q", r.Status)
	}
	switch r.RecordType {
	case RecordEarning:
		if r.AmountCents <= 0 || !blank(r.ReversalOfID) {
			return errors.New("earning records must be positive and cannot reference a reversal source")
		}
	case RecordReversal:
		if r.AmountCents >= 0 || blank(r.ReversalOfID) {
			return errors.New("reversal records must be negative and reference the original record")
		}
	case RecordAdjustment:
	default:
		return fmt.Errorf("unsupported commission record type %q", r.RecordType)
	}
	return nil
}

type WalletBalances struct {
	ExpectedCents    AmountCents `json:"expectedCents"`
	FrozenCents      AmountCents `json:"frozenCents"`
	AvailableCents   AmountCents `json:"availableCents"`
	SettlingCents    AmountCents `json:"settlingCents"`
	SettledCents     AmountCents `json:"settledCents"`
	RecoverableCents AmountCents `json:"recoverableCents"`
}

func (b WalletBalances) Validate() error {
	if b.ExpectedCents < 0 || b.FrozenCents < 0 || b.AvailableCents < 0 ||
		b.SettlingCents < 0 || b.SettledCents < 0 || b.RecoverableCents < 0 {
		return errors.New("wallet balance buckets cannot be negative")
	}
	return nil
}

type SettlementApplicationStatus string

const (
	SettlementPendingReview      SettlementApplicationStatus = "PENDING_REVIEW"
	SettlementReviewing          SettlementApplicationStatus = "REVIEWING"
	SettlementApproved           SettlementApplicationStatus = "APPROVED"
	SettlementPartiallyApproved  SettlementApplicationStatus = "PARTIALLY_APPROVED"
	SettlementRejected           SettlementApplicationStatus = "REJECTED"
	SettlementBatched            SettlementApplicationStatus = "BATCHED"
	SettlementPayoutProcessing   SettlementApplicationStatus = "PAYOUT_PROCESSING"
	SettlementPartiallySucceeded SettlementApplicationStatus = "PARTIALLY_SUCCEEDED"
	SettlementCompleted          SettlementApplicationStatus = "COMPLETED"
)

type SettlementApplication struct {
	ID                    string
	ApplicationNo         string
	TenantID              string
	ApplicantType         BeneficiaryType
	ApplicantID           string
	SettlementPeriodStart time.Time
	SettlementPeriodEnd   time.Time
	AppliedAmountCents    AmountCents
	ApprovedAmountCents   AmountCents
	RejectedAmountCents   AmountCents
	Status                SettlementApplicationStatus
	RejectReason          string
	IdempotencyKey        string
}

func (a SettlementApplication) Validate() error {
	if blank(a.ID) || blank(a.ApplicationNo) || blank(a.TenantID) || blank(a.ApplicantID) || blank(a.IdempotencyKey) {
		return errors.New("settlement application identifiers and idempotency key are required")
	}
	if a.ApplicantType != BeneficiaryAgent && a.ApplicantType != BeneficiaryOperationCenter {
		return errors.New("settlement applicant must be an agent or operation center")
	}
	if a.SettlementPeriodStart.IsZero() || a.SettlementPeriodEnd.Before(a.SettlementPeriodStart) {
		return errors.New("invalid settlement period")
	}
	if a.AppliedAmountCents <= 0 || a.ApprovedAmountCents < 0 || a.RejectedAmountCents < 0 ||
		a.ApprovedAmountCents+a.RejectedAmountCents > a.AppliedAmountCents {
		return errors.New("invalid settlement amounts")
	}
	if !a.Status.Valid() {
		return fmt.Errorf("unsupported settlement status %q", a.Status)
	}
	return nil
}

type PayoutBatchStatus string

const (
	BatchDraft            PayoutBatchStatus = "DRAFT"
	BatchValidating       PayoutBatchStatus = "VALIDATING"
	BatchValidationFailed PayoutBatchStatus = "VALIDATION_FAILED"
	BatchPendingApproval  PayoutBatchStatus = "PENDING_APPROVAL"
	BatchApproved         PayoutBatchStatus = "APPROVED"
	BatchReadyToExport    PayoutBatchStatus = "READY_TO_EXPORT"
	BatchExported         PayoutBatchStatus = "EXPORTED"
	BatchSubmitted        PayoutBatchStatus = "SUBMITTED"
	BatchProcessing       PayoutBatchStatus = "PROCESSING"
	BatchPartialSuccess   PayoutBatchStatus = "PARTIAL_SUCCESS"
	BatchSuccess          PayoutBatchStatus = "SUCCESS"
	BatchClosed           PayoutBatchStatus = "CLOSED"
)

type PayoutDetail struct {
	ID                      string
	BatchID                 string
	DetailNo                string
	SettlementApplicationID string
	BeneficiaryType         BeneficiaryType
	BeneficiaryID           string
	WorkerProfileID         string
	AmountCents             AmountCents
	IdempotencyKey          string
}

type PayoutBatch struct {
	ID                 string
	BatchNo            string
	TenantID           string
	ProviderCode       string
	BusinessScene      string
	TotalCount         int
	TotalAmountCents   AmountCents
	SuccessCount       int
	SuccessAmountCents AmountCents
	FailedCount        int
	FailedAmountCents  AmountCents
	ProviderFeeCents   AmountCents
	Status             PayoutBatchStatus
	Details            []PayoutDetail
}

func (b PayoutBatch) ValidateTotals() error {
	if blank(b.ID) || blank(b.BatchNo) || blank(b.TenantID) || blank(b.ProviderCode) || blank(b.BusinessScene) {
		return errors.New("payout batch identifiers, provider and business scene are required")
	}
	if !b.Status.Valid() {
		return fmt.Errorf("unsupported payout batch status %q", b.Status)
	}
	if b.TotalCount < 0 || b.SuccessCount < 0 || b.FailedCount < 0 || b.TotalAmountCents < 0 ||
		b.SuccessAmountCents < 0 || b.FailedAmountCents < 0 || b.ProviderFeeCents < 0 {
		return errors.New("payout batch counts and amounts cannot be negative")
	}
	if b.SuccessCount+b.FailedCount > b.TotalCount || b.SuccessAmountCents+b.FailedAmountCents > b.TotalAmountCents {
		return errors.New("payout result totals exceed batch totals")
	}
	var detailTotal AmountCents
	seenApplications := make(map[string]struct{}, len(b.Details))
	seenDetails := make(map[string]struct{}, len(b.Details))
	for _, detail := range b.Details {
		if blank(detail.ID) || blank(detail.BatchID) || blank(detail.DetailNo) || blank(detail.SettlementApplicationID) ||
			blank(detail.BeneficiaryID) || blank(detail.WorkerProfileID) || blank(detail.IdempotencyKey) || detail.AmountCents <= 0 {
			return errors.New("invalid payout detail")
		}
		if detail.BatchID != b.ID {
			return errors.New("payout detail belongs to another batch")
		}
		if _, exists := seenApplications[detail.SettlementApplicationID]; exists {
			return errors.New("settlement application appears more than once in a payout batch")
		}
		if _, exists := seenDetails[detail.DetailNo]; exists {
			return errors.New("payout detail number is duplicated")
		}
		seenApplications[detail.SettlementApplicationID] = struct{}{}
		seenDetails[detail.DetailNo] = struct{}{}
		detailTotal += detail.AmountCents
	}
	if len(b.Details) != b.TotalCount || detailTotal != b.TotalAmountCents {
		return fmt.Errorf("payout batch summary mismatch: count=%d/%d amount=%d/%d", len(b.Details), b.TotalCount, detailTotal, b.TotalAmountCents)
	}
	return nil
}

func (t BeneficiaryType) Valid() bool {
	switch t {
	case BeneficiaryAgent, BeneficiaryOperationCenter, BeneficiaryPlatform:
		return true
	default:
		return false
	}
}

func (s CommissionStatus) Valid() bool {
	switch s {
	case CommissionExpected, CommissionFrozen, CommissionAvailable, CommissionSettling,
		CommissionSettled, CommissionReversed, CommissionCancelled:
		return true
	default:
		return false
	}
}

func (s SettlementApplicationStatus) Valid() bool {
	switch s {
	case SettlementPendingReview, SettlementReviewing, SettlementApproved, SettlementPartiallyApproved,
		SettlementRejected, SettlementBatched, SettlementPayoutProcessing, SettlementPartiallySucceeded,
		SettlementCompleted:
		return true
	default:
		return false
	}
}

func (s PayoutBatchStatus) Valid() bool {
	switch s {
	case BatchDraft, BatchValidating, BatchValidationFailed, BatchPendingApproval, BatchApproved,
		BatchReadyToExport, BatchExported, BatchSubmitted, BatchProcessing, BatchPartialSuccess,
		BatchSuccess, BatchClosed:
		return true
	default:
		return false
	}
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}
