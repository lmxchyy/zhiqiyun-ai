package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	pointsdomain "xianzhi-ai/backend-go/internal/points"
)

// PointSource is a server-selected origin for a personal point lot.  The
// client never supplies this value directly; callers use one of the
// application-owned commands below.
type PointSource string

const (
	PointSourceRegistrationGift     PointSource = "REGISTRATION_GIFT"
	PointSourceActivityGift         PointSource = "ACTIVITY_GIFT"
	PointSourceAdminGift            PointSource = "ADMIN_GIFT"
	PointSourceRecharge             PointSource = "RECHARGE"
	PointSourceCorrection           PointSource = "CORRECTION"
	PointSourceAdminCorrection      PointSource = "ADMIN_CORRECTION"
	PointSourceMembershipGrant      PointSource = "MEMBERSHIP_GRANT"
	PointSourceMemberPackageGrant   PointSource = "MEMBER_PACKAGE_GRANT"
	PointSourceAgentGrant           PointSource = "AGENT_GRANT"
	PointSourceAgentJoinGrant       PointSource = "AGENT_JOIN_GRANT"
	PointSourceOperationCenterGrant PointSource = "OPERATION_CENTER_GRANT"
	PointSourceOrderGrant           PointSource = "ORDER_GRANT"
	PointSourceCommerceOrder        PointSource = "COMMERCE_ORDER"
	PointSourceUnifiedPaymentGrant  PointSource = "UNIFIED_PAYMENT_GRANT"
	PointSourceWechatVirtualOrder   PointSource = "WECHAT_VIRTUAL_ORDER"
	PointSourceWechatVirtualCoupon  PointSource = "WECHAT_VIRTUAL_COUPON"
	PointSourceCouponGrant          PointSource = "COUPON_GRANT"
	PointSourceRefund               PointSource = "REFUND"
	PointSourceRelease              PointSource = "RELEASE"
	PointSourceAdjustment           PointSource = "ADJUSTMENT"
	PointSourceLegacy               PointSource = "LEGACY"
	PointSourceReversal             PointSource = "REVERSAL"
	PointSourceManual               PointSource = "MANUAL"
)

var (
	ErrInsufficientPoints                    = errors.New("insufficient points")
	ErrUnknownPointSource                    = errors.New("unknown point source")
	ErrIdempotencyConflict                   = errors.New("point idempotency conflict")
	ErrPointPolicyRevisionConflict           = errors.New("point expiry policy revision conflict")
	ErrPointOwnership                        = errors.New("point account ownership mismatch")
	ErrInvalidPointCommand                   = errors.New("invalid point command")
	ErrPointNotFound                         = errors.New("point record not found")
	ErrPersonalPointImportConflict           = errors.New("personal point sidecar import conflict")
	ErrPersonalPointContextMismatch          = errors.New("personal point context mismatch")
	ErrPersonalPointReservationMarkerMissing = errors.New("personal point reservation marker missing")
	ErrPersonalPointMergeActiveReservation   = errors.New("personal point merge blocked by active reservation")
)

// InsufficientPointsError is the stable API error for a failed personal-point
// reservation. It keeps the legacy sentinel for callers that use errors.Is,
// while exposing enough data for clients to render an actionable message.
type InsufficientPointsError struct {
	CurrentPoints  int64
	RequiredPoints int64
}

func (e *InsufficientPointsError) Error() string { return "积分不足" }

func (e *InsufficientPointsError) Unwrap() error { return ErrInsufficientPoints }

func (e *InsufficientPointsError) BusinessCode() string { return "INSUFFICIENT_POINTS" }

func (e *InsufficientPointsError) ErrorDetails() map[string]any {
	if e == nil || e.CurrentPoints < 0 || e.RequiredPoints <= 0 {
		return nil
	}
	return map[string]any{
		"currentPoints":  e.CurrentPoints,
		"requiredPoints": e.RequiredPoints,
		"shortfall":      e.RequiredPoints - e.CurrentPoints,
	}
}

func newInsufficientPointsError(currentPoints, requiredPoints int64) error {
	return &InsufficientPointsError{CurrentPoints: currentPoints, RequiredPoints: requiredPoints}
}

const personalLotBillingEngine = "PERSONAL_LOT_V1"

type PointExpiryPolicy struct {
	ID            string    `json:"id"`
	Version       int64     `json:"version"`
	Revision      int64     `json:"revision"`
	Enabled       bool      `json:"enabled"`
	DurationValue int       `json:"duration_value"`
	DurationUnit  string    `json:"duration_unit"`
	TimeZone      string    `json:"time_zone"`
	SourceTypes   []string  `json:"source_types"`
	EffectiveFrom time.Time `json:"effective_from"`
	EffectiveTo   time.Time `json:"effective_to,omitempty"`
	Status        string    `json:"status"`
	CreatedBy     string    `json:"created_by,omitempty"`
	ChangeReason  string    `json:"change_reason,omitempty"`
}

type PersonalPointPolicyPublishCommand struct {
	ExpectedRevision int64
	Enabled          bool
	DurationValue    int
	ChangeReason     string
	ActorID          string
	PublishedAt      time.Time
}

type PersonalPointLotFilter struct {
	Source PointSource
	Status string
	Limit  int
	Offset int
}

type PersonalPointBalanceSummary struct {
	PersonalPointBalance
	PermanentAvailable int64     `json:"permanent_available"`
	ExpiringAvailable  int64     `json:"expiring_available"`
	NextExpiryAt       time.Time `json:"next_expiry_at,omitempty"`
	NextExpiryPoints   int64     `json:"next_expiry_points"`
}

type PersonalPointExpiryBatchResult struct {
	AccountsProcessed int   `json:"accounts_processed"`
	PointsExpired     int64 `json:"points_expired"`
}

type personalPointMergeResult struct {
	AccountsMoved int
	PointsMoved   int64
}

type PointPolicySnapshot struct {
	Version       int64  `json:"version"`
	Enabled       bool   `json:"enabled"`
	DurationValue int    `json:"duration_value"`
	DurationUnit  string `json:"duration_unit"`
	TimeZone      string `json:"time_zone"`
}

type PersonalPointLot struct {
	ID              string              `json:"id"`
	AccountID       string              `json:"account_id"`
	UserID          string              `json:"user_id"`
	SourceType      PointSource         `json:"source_type"`
	ReferenceType   string              `json:"reference_type"`
	ReferenceID     string              `json:"reference_id"`
	OriginalPoints  int64               `json:"original_points"`
	AvailablePoints int64               `json:"available_points"`
	ReservedPoints  int64               `json:"reserved_points"`
	ConsumedPoints  int64               `json:"consumed_points"`
	ExpiredPoints   int64               `json:"expired_points"`
	ReversedPoints  int64               `json:"reversed_points"`
	GrantedAt       time.Time           `json:"granted_at"`
	ExpiresAt       time.Time           `json:"expires_at"`
	PolicyVersionID string              `json:"policy_version_id,omitempty"`
	PolicySnapshot  PointPolicySnapshot `json:"policy_snapshot"`
	IdempotencyKey  string              `json:"idempotency_key"`
	Status          string              `json:"status"`
}

func (l PersonalPointLot) Permanent() bool { return l.ExpiresAt.IsZero() }

type PersonalPointAccount struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	AvailablePoints int64  `json:"available_points"`
	FrozenPoints    int64  `json:"frozen_points"`
	TotalGranted    int64  `json:"total_granted"`
	TotalConsumed   int64  `json:"total_consumed"`
	TotalExpired    int64  `json:"total_expired"`
	TotalReversed   int64  `json:"total_reversed"`
}

type PersonalPointBalance struct {
	AccountID string `json:"account_id"`
	UserID    string `json:"user_id"`
	Available int64  `json:"available"`
	Frozen    int64  `json:"frozen"`
	Total     int64  `json:"total"`
}

type PersonalPointReservation struct {
	ID              string    `json:"id"`
	AccountID       string    `json:"account_id"`
	UserID          string    `json:"user_id"`
	BusinessType    string    `json:"business_type"`
	BusinessID      string    `json:"business_id"`
	RequestedPoints int64     `json:"requested_points"`
	ReservedPoints  int64     `json:"reserved_points"`
	CapturedPoints  int64     `json:"captured_points"`
	ReleasedPoints  int64     `json:"released_points"`
	ExpiredPoints   int64     `json:"expired_points"`
	Status          string    `json:"status"`
	IdempotencyKey  string    `json:"idempotency_key"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type PersonalPointAllocation struct {
	ID              string      `json:"id"`
	ReservationID   string      `json:"reservation_id"`
	LotID           string      `json:"lot_id"`
	AccountID       string      `json:"account_id"`
	UserID          string      `json:"user_id"`
	SourceType      PointSource `json:"source_type"`
	AllocatedPoints int64       `json:"allocated_points"`
	ReservedPoints  int64       `json:"reserved_points"`
	CapturedPoints  int64       `json:"captured_points"`
	ReleasedPoints  int64       `json:"released_points"`
	ExpiredPoints   int64       `json:"expired_points"`
	Status          string      `json:"status"`
}

type PersonalPointLotMovement struct {
	ID              string    `json:"id"`
	LotID           string    `json:"lot_id"`
	AccountID       string    `json:"account_id"`
	UserID          string    `json:"user_id"`
	MovementType    string    `json:"movement_type"`
	Points          int64     `json:"points"`
	AvailableBefore int64     `json:"available_before"`
	AvailableAfter  int64     `json:"available_after"`
	ReservedBefore  int64     `json:"reserved_before"`
	ReservedAfter   int64     `json:"reserved_after"`
	ConsumedBefore  int64     `json:"consumed_before"`
	ConsumedAfter   int64     `json:"consumed_after"`
	ExpiredBefore   int64     `json:"expired_before"`
	ExpiredAfter    int64     `json:"expired_after"`
	ReversedBefore  int64     `json:"reversed_before"`
	ReversedAfter   int64     `json:"reversed_after"`
	ReferenceType   string    `json:"reference_type"`
	ReferenceID     string    `json:"reference_id"`
	ReservationID   string    `json:"reservation_id,omitempty"`
	IdempotencyKey  string    `json:"idempotency_key"`
	CreatedAt       time.Time `json:"created_at"`
}

type PersonalPointGrantCommand struct {
	AccountID      string
	UserID         string
	Source         PointSource
	Points         int64
	ReferenceType  string
	ReferenceID    string
	IdempotencyKey string
	Reason         string
	Audit          PersonalPointAudit
	GrantedAt      time.Time
}

type PersonalPointAudit struct {
	ActorID   string
	ActorRole string
	Action    string
	Method    string
	Path      string
	RequestID string
}

type PersonalPointCorrectionCommand struct {
	AccountID      string
	UserID         string
	Points         int64
	Reason         string
	IdempotencyKey string
	Audit          PersonalPointAudit
	CorrectedAt    time.Time
}

type PersonalPointCorrectionResult struct {
	Balance    PersonalPointBalance `json:"balance"`
	Lot        *PersonalPointLot    `json:"lot,omitempty"`
	Points     int64                `json:"points"`
	Idempotent bool                 `json:"idempotent"`
}

type PersonalPointRegistrationGrantCommand struct {
	AccountID       string
	UserID          string
	PlanID          string
	PlanGrantPoints int64
	IdempotencyKey  string
	GrantedAt       time.Time
}

type PersonalPointReserveCommand struct {
	AccountID       string
	UserID          string
	BusinessType    string
	BusinessID      string
	RequestedPoints int64
	IdempotencyKey  string
	ReservedAt      time.Time
}

type PersonalPointCaptureCommand struct {
	AccountID      string
	UserID         string
	ReservationID  string
	Points         int64
	IdempotencyKey string
	CapturedAt     time.Time
}

type PersonalPointReleaseCommand struct {
	AccountID      string
	UserID         string
	ReservationID  string
	Points         int64
	IdempotencyKey string
	ReleasedAt     time.Time
}

type PersonalPointExpiryCommand struct {
	AccountID string
	UserID    string
	Now       time.Time
}

type PersonalPointGrantResult struct {
	Lot        PersonalPointLot
	Idempotent bool
}

type PersonalPointRegistrationGrantResult = PersonalPointGrantResult

type PersonalPointReserveResult struct {
	Reservation PersonalPointReservation
	Allocations []PersonalPointAllocation
	Idempotent  bool
}

type PersonalPointMutationResult struct {
	Reservation PersonalPointReservation
	Allocations []PersonalPointAllocation
	Idempotent  bool
}

type personalPointOperation struct {
	Kind           string    `json:"kind"`
	AccountID      string    `json:"account_id"`
	UserID         string    `json:"user_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Fingerprint    string    `json:"fingerprint"`
	ReservationID  string    `json:"reservation_id,omitempty"`
	Points         int64     `json:"points,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// PersonalPointWalletLedgerEntry is the JSON adapter's account-level audit
// projection.  It mirrors the observable transition fields of xz_wallet_ledger
// while retaining source/business metadata for old file-backed deployments.
type PersonalPointWalletLedgerEntry struct {
	ID              string         `json:"id"`
	AccountID       string         `json:"account_id"`
	UserID          string         `json:"user_id"`
	TenantID        string         `json:"tenant_id,omitempty"`
	TaskID          string         `json:"task_id,omitempty"`
	BillingEventID  string         `json:"billing_event_id,omitempty"`
	EntryType       string         `json:"entry_type"`
	Points          int64          `json:"points"`
	AvailableBefore int64          `json:"available_before"`
	AvailableAfter  int64          `json:"available_after"`
	FrozenBefore    int64          `json:"frozen_before"`
	FrozenAfter     int64          `json:"frozen_after"`
	IdempotencyKey  string         `json:"idempotency_key"`
	ReferenceType   string         `json:"reference_type"`
	ReferenceID     string         `json:"reference_id"`
	Remark          string         `json:"remark,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	OccurredAt      time.Time      `json:"occurred_at"`
	CreatedAt       time.Time      `json:"created_at,omitempty"`
}

// PersonalPointRepository is the storage boundary for the service.  JSON and
// PostgreSQL repositories both implement the same domain behavior; keeping the
// interface here also gives future HTTP callers one audited entry point.
type PersonalPointRepository interface {
	grant(context.Context, PersonalPointGrantCommand) (PersonalPointGrantResult, error)
	correct(context.Context, PersonalPointCorrectionCommand) (PersonalPointCorrectionResult, error)
	grantRegistration(context.Context, PersonalPointRegistrationGrantCommand) (PersonalPointGrantResult, error)
	getBalance(context.Context, string, string) (PersonalPointBalance, error)
	reserve(context.Context, PersonalPointReserveCommand) (PersonalPointReserveResult, error)
	capture(context.Context, PersonalPointCaptureCommand) (PersonalPointMutationResult, error)
	release(context.Context, PersonalPointReleaseCommand) (PersonalPointMutationResult, error)
	expire(context.Context, PersonalPointExpiryCommand) error
	movementCount(context.Context, string, string, string) int
	currentPolicy(context.Context) (PointExpiryPolicy, error)
	publishPolicy(context.Context, PersonalPointPolicyPublishCommand) (PointExpiryPolicy, error)
	listLots(context.Context, string, string, PersonalPointLotFilter) ([]PersonalPointLot, error)
	summary(context.Context, string, string, time.Time) (PersonalPointBalanceSummary, error)
	expireDue(context.Context, time.Time, int) (PersonalPointExpiryBatchResult, error)
}

type PersonalPointService struct{ repo PersonalPointRepository }

func NewPersonalPointService(repo PersonalPointRepository) *PersonalPointService {
	return &PersonalPointService{repo: repo}
}

func (s *PersonalPointService) ensure() error {
	if s == nil || s.repo == nil {
		return ErrInvalidPointCommand
	}
	return nil
}
func (s *PersonalPointService) Grant(ctx context.Context, cmd PersonalPointGrantCommand) (PersonalPointGrantResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointGrantResult{}, err
	}
	return s.repo.grant(ctx, cmd)
}
func (s *PersonalPointService) GrantRegistration(ctx context.Context, cmd PersonalPointRegistrationGrantCommand) (PersonalPointGrantResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointGrantResult{}, err
	}
	return s.repo.grantRegistration(ctx, cmd)
}
func (s *PersonalPointService) GetBalance(ctx context.Context, accountID, userID string) (PersonalPointBalance, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointBalance{}, err
	}
	return s.repo.getBalance(ctx, accountID, userID)
}
func (s *PersonalPointService) Reserve(ctx context.Context, cmd PersonalPointReserveCommand) (PersonalPointReserveResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointReserveResult{}, err
	}
	return s.repo.reserve(ctx, cmd)
}
func (s *PersonalPointService) Capture(ctx context.Context, cmd PersonalPointCaptureCommand) (PersonalPointMutationResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointMutationResult{}, err
	}
	return s.repo.capture(ctx, cmd)
}
func (s *PersonalPointService) Release(ctx context.Context, cmd PersonalPointReleaseCommand) (PersonalPointMutationResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointMutationResult{}, err
	}
	return s.repo.release(ctx, cmd)
}
func (s *PersonalPointService) Expire(ctx context.Context, cmd PersonalPointExpiryCommand) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return s.repo.expire(ctx, cmd)
}
func (s *PersonalPointService) MovementCount(ctx context.Context, accountID, movementType string) int {
	if s == nil || s.repo == nil {
		return 0
	}
	return s.repo.movementCount(ctx, accountID, "", movementType)
}

func (s *PersonalPointService) CurrentPolicy(ctx context.Context) (PointExpiryPolicy, error) {
	if err := s.ensure(); err != nil {
		return PointExpiryPolicy{}, err
	}
	return s.repo.currentPolicy(ctx)
}

func (s *PersonalPointService) PublishPolicy(ctx context.Context, cmd PersonalPointPolicyPublishCommand) (PointExpiryPolicy, error) {
	if err := s.ensure(); err != nil {
		return PointExpiryPolicy{}, err
	}
	if cmd.ExpectedRevision <= 0 || cmd.DurationValue <= 0 || strings.TrimSpace(cmd.ChangeReason) == "" || strings.TrimSpace(cmd.ActorID) == "" {
		return PointExpiryPolicy{}, ErrInvalidPointCommand
	}
	return s.repo.publishPolicy(ctx, cmd)
}

func (s *PersonalPointService) ListLots(ctx context.Context, accountID, userID string, filter PersonalPointLotFilter) ([]PersonalPointLot, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(userID) == "" || filter.Limit < 0 || filter.Offset < 0 {
		return nil, ErrInvalidPointCommand
	}
	return s.repo.listLots(ctx, accountID, userID, filter)
}

func (s *PersonalPointService) Summary(ctx context.Context, accountID, userID string, now time.Time) (PersonalPointBalanceSummary, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointBalanceSummary{}, err
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(userID) == "" {
		return PersonalPointBalanceSummary{}, ErrInvalidPointCommand
	}
	return s.repo.summary(ctx, accountID, userID, now)
}

func (s *PersonalPointService) ExpireDue(ctx context.Context, now time.Time, limit int) (PersonalPointExpiryBatchResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointExpiryBatchResult{}, err
	}
	if limit <= 0 {
		return PersonalPointExpiryBatchResult{}, ErrInvalidPointCommand
	}
	return s.repo.expireDue(ctx, now, limit)
}

func (s *PersonalPointService) Correct(ctx context.Context, cmd PersonalPointCorrectionCommand) (PersonalPointCorrectionResult, error) {
	if err := s.ensure(); err != nil {
		return PersonalPointCorrectionResult{}, err
	}
	if strings.TrimSpace(cmd.AccountID) == "" || strings.TrimSpace(cmd.UserID) == "" || cmd.Points == 0 || strings.TrimSpace(cmd.Reason) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return PersonalPointCorrectionResult{}, ErrInvalidPointCommand
	}
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	return s.repo.correct(ctx, cmd)
}

func normalizePointCommand(cmd PersonalPointGrantCommand) error {
	err := pointsdomain.ValidateGrantCommand(pointsdomain.GrantCommand{
		AccountID: cmd.AccountID, UserID: cmd.UserID, Source: pointsdomain.Source(cmd.Source),
		Points: cmd.Points, ReferenceType: cmd.ReferenceType, ReferenceID: cmd.ReferenceID,
		IdempotencyKey: cmd.IdempotencyKey, Reason: cmd.Reason,
	})
	switch {
	case errors.Is(err, pointsdomain.ErrUnknownSource):
		return ErrUnknownPointSource
	case errors.Is(err, pointsdomain.ErrInvalidGrant):
		return ErrInvalidPointCommand
	default:
		return err
	}
}

func isKnownPointSource(source PointSource) bool {
	return pointsdomain.IsKnownSource(pointsdomain.Source(source))
}

func validatePointExpiryPolicy(policy PointExpiryPolicy) error {
	err := pointsdomain.ValidateExpiryPolicy(pointsdomain.ExpiryPolicy{
		ID: policy.ID, Version: policy.Version, Enabled: policy.Enabled,
		DurationValue: policy.DurationValue, DurationUnit: policy.DurationUnit,
		TimeZone: policy.TimeZone,
	})
	if errors.Is(err, pointsdomain.ErrInvalidPolicy) {
		return ErrInvalidPointCommand
	}
	return err
}

func isGiftPointSource(source PointSource) bool {
	return source == PointSourceRegistrationGift || source == PointSourceActivityGift || source == PointSourceAdminGift
}

func pointCommandFingerprint(v any) string {
	raw, _ := json.Marshal(v)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func personalPointGrantFingerprint(cmd PersonalPointGrantCommand) string {
	return pointCommandFingerprint(struct {
		AccountID, UserID, Source, ReferenceType, ReferenceID, IdempotencyKey, Reason string
		Points                                                                        int64
		GrantedAt                                                                     time.Time
	}{cmd.AccountID, cmd.UserID, string(cmd.Source), cmd.ReferenceType, cmd.ReferenceID, cmd.IdempotencyKey, strings.TrimSpace(cmd.Reason), cmd.Points, cmd.GrantedAt})
}

func personalPointCorrectionFingerprint(cmd PersonalPointCorrectionCommand) string {
	return pointCommandFingerprint(struct {
		AccountID, UserID, Reason, IdempotencyKey string
		Points                                    int64
	}{cmd.AccountID, cmd.UserID, strings.TrimSpace(cmd.Reason), cmd.IdempotencyKey, cmd.Points})
}

func stablePointID(prefix, accountID, key string) string {
	hash := sha256.Sum256([]byte(prefix + "\x00" + accountID + "\x00" + key))
	return prefix + "_" + hex.EncodeToString(hash[:16])
}

func pointNow(v time.Time) time.Time {
	if v.IsZero() {
		return time.Now().UTC()
	}
	return v.UTC()
}

func addCalendarMonthsClamp(granted time.Time, months int, zone string) (time.Time, error) {
	if months <= 0 {
		return time.Time{}, ErrInvalidPointCommand
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, err
	}
	local := granted.In(loc)
	y, m := local.Year(), local.Month()
	targetMonth := int(m) - 1 + months
	y += targetMonth / 12
	targetMonth %= 12
	if targetMonth < 0 {
		targetMonth += 12
		y--
	}
	tm := time.Month(targetMonth + 1)
	day := local.Day()
	last := time.Date(y, tm+1, 0, 0, 0, 0, 0, loc).Day()
	if day > last {
		day = last
	}
	return time.Date(y, tm, day, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), loc).UTC(), nil
}

func setPointLotStatus(lot *PersonalPointLot) {
	if lot.SourceType == PointSourceLegacy {
		lot.Status = "LEGACY"
		return
	}
	if lot.AvailablePoints+lot.ReservedPoints > 0 {
		lot.Status = "ACTIVE"
		return
	}
	if lot.ExpiredPoints > 0 && lot.ConsumedPoints == 0 && lot.ReversedPoints == 0 {
		lot.Status = "EXPIRED"
		return
	}
	if lot.ReversedPoints > 0 && lot.AvailablePoints == 0 && lot.ReservedPoints == 0 && lot.ConsumedPoints == 0 {
		lot.Status = "REVERSED"
		return
	}
	lot.Status = "EXHAUSTED"
}

func setReservationStatus(r *PersonalPointReservation) {
	if r.ReservedPoints > 0 {
		if r.CapturedPoints > 0 || r.ReleasedPoints > 0 || r.ExpiredPoints > 0 {
			r.Status = "PARTIAL"
		} else {
			r.Status = "RESERVED"
		}
		return
	}
	if r.CapturedPoints > 0 && r.CapturedPoints == r.RequestedPoints {
		r.Status = "CAPTURED"
		return
	}
	if r.ReleasedPoints+r.ExpiredPoints == r.RequestedPoints {
		r.Status = "RELEASED"
		return
	}
	if r.CapturedPoints > 0 {
		r.Status = "PARTIAL"
		return
	}
	r.Status = "RELEASED"
}

func setAllocationStatus(a *PersonalPointAllocation) {
	if a.ReservedPoints > 0 {
		if a.CapturedPoints > 0 || a.ReleasedPoints > 0 || a.ExpiredPoints > 0 {
			a.Status = "PARTIAL"
		} else {
			a.Status = "RESERVED"
		}
		return
	}
	if a.CapturedPoints == a.AllocatedPoints {
		a.Status = "CAPTURED"
		return
	}
	if a.ReleasedPoints+a.ExpiredPoints == a.AllocatedPoints {
		a.Status = "RELEASED"
		return
	}
	if a.CapturedPoints > 0 {
		a.Status = "PARTIAL"
		return
	}
	a.Status = "RELEASED"
}

func sortLotsFEFO(lots []PersonalPointLot) {
	sort.SliceStable(lots, func(i, j int) bool {
		a, b := lots[i], lots[j]
		if a.ExpiresAt.IsZero() != b.ExpiresAt.IsZero() {
			return !a.ExpiresAt.IsZero()
		}
		if !a.ExpiresAt.Equal(b.ExpiresAt) {
			if a.ExpiresAt.IsZero() {
				return false
			}
			return a.ExpiresAt.Before(b.ExpiresAt)
		}
		if !a.GrantedAt.Equal(b.GrantedAt) {
			return a.GrantedAt.Before(b.GrantedAt)
		}
		return a.ID < b.ID
	})
}

func validateOwned(accountID, userID, ownerAccount, ownerUser string) error {
	if accountID == "" || userID == "" || accountID != ownerAccount || userID != ownerUser {
		return ErrPointOwnership
	}
	return nil
}

// A small mutex keyed by account is used by the JSON adapter in addition to
// its file mutex.  It also documents the lock order used by both adapters:
// account, then lots, then reservation allocations.
type personalPointLockSet struct{ mu sync.Mutex }

// PersonalPointService exposes the same lot-aware core to existing stores
// without changing their legacy wallet methods.  Callers still choose the
// concrete command on the server side, so HTTP payloads cannot set a source or
// expiry policy.
func (s *jsonStore) PersonalPointService() *PersonalPointService {
	if s == nil {
		return NewPersonalPointService(nil)
	}
	s.personalPointMu.Lock()
	defer s.personalPointMu.Unlock()
	if s.personalPointStore == nil {
		store := &JSONPersonalPointStore{owner: s}
		s.personalPointStore = store
	}
	return NewPersonalPointService(s.personalPointStore)
}

func (s *postgresStore) PersonalPointService() *PersonalPointService {
	if s == nil {
		return NewPersonalPointService(nil)
	}
	return NewPersonalPointService(NewPostgresPersonalPointStore(s.db))
}
