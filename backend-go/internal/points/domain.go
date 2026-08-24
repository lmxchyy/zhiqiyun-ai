// Package points owns the transport-independent invariants for personal
// points commands. Persistence and HTTP adapters remain in their existing
// packages while extraction proceeds in small, behavior-preserving slices.
package points

import (
	"errors"
	"strings"
)

type Source string

const (
	SourceRegistrationGift     Source = "REGISTRATION_GIFT"
	SourceActivityGift         Source = "ACTIVITY_GIFT"
	SourceAdminGift            Source = "ADMIN_GIFT"
	SourceRecharge             Source = "RECHARGE"
	SourceCorrection           Source = "CORRECTION"
	SourceAdminCorrection      Source = "ADMIN_CORRECTION"
	SourceMembershipGrant      Source = "MEMBERSHIP_GRANT"
	SourceMemberPackageGrant   Source = "MEMBER_PACKAGE_GRANT"
	SourceAgentGrant           Source = "AGENT_GRANT"
	SourceAgentJoinGrant       Source = "AGENT_JOIN_GRANT"
	SourceOperationCenterGrant Source = "OPERATION_CENTER_GRANT"
	SourceOrderGrant           Source = "ORDER_GRANT"
	SourceCommerceOrder        Source = "COMMERCE_ORDER"
	SourceUnifiedPaymentGrant  Source = "UNIFIED_PAYMENT_GRANT"
	SourceWechatVirtualOrder   Source = "WECHAT_VIRTUAL_ORDER"
	SourceWechatVirtualCoupon  Source = "WECHAT_VIRTUAL_COUPON"
	SourceCouponGrant          Source = "COUPON_GRANT"
	SourceRefund               Source = "REFUND"
	SourceRelease              Source = "RELEASE"
	SourceAdjustment           Source = "ADJUSTMENT"
	SourceLegacy               Source = "LEGACY"
	SourceReversal             Source = "REVERSAL"
	SourceManual               Source = "MANUAL"
)

var (
	ErrInvalidGrant  = errors.New("invalid point grant")
	ErrUnknownSource = errors.New("unknown point source")
	ErrInvalidPolicy = errors.New("invalid point expiry policy")
)

type GrantCommand struct {
	AccountID      string
	UserID         string
	Source         Source
	Points         int64
	ReferenceType  string
	ReferenceID    string
	IdempotencyKey string
	Reason         string
}

func ValidateGrantCommand(cmd GrantCommand) error {
	if strings.TrimSpace(cmd.AccountID) == "" || strings.TrimSpace(cmd.UserID) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" || cmd.Points <= 0 {
		return ErrInvalidGrant
	}
	if !knownSource(cmd.Source) {
		return ErrUnknownSource
	}
	return nil
}

func knownSource(source Source) bool {
	switch source {
	case SourceRegistrationGift, SourceActivityGift, SourceAdminGift,
		SourceRecharge, SourceCorrection, SourceAdminCorrection,
		SourceMembershipGrant, SourceMemberPackageGrant, SourceAgentGrant,
		SourceAgentJoinGrant, SourceOperationCenterGrant, SourceOrderGrant,
		SourceCommerceOrder, SourceUnifiedPaymentGrant, SourceWechatVirtualOrder,
		SourceWechatVirtualCoupon, SourceCouponGrant, SourceRefund, SourceRelease,
		SourceAdjustment, SourceReversal, SourceManual:
		return true
	default:
		return false
	}
}

func IsKnownSource(source Source) bool { return knownSource(source) }

type ExpiryPolicy struct {
	ID            string
	Version       int64
	Enabled       bool
	DurationValue int
	DurationUnit  string
	TimeZone      string
}

func ValidateExpiryPolicy(policy ExpiryPolicy) error {
	if strings.TrimSpace(policy.ID) == "" || policy.Version <= 0 || strings.TrimSpace(policy.TimeZone) == "" {
		return ErrInvalidPolicy
	}
	if policy.DurationUnit != "CALENDAR_MONTH" || policy.DurationValue < 0 {
		return ErrInvalidPolicy
	}
	if policy.Enabled && policy.DurationValue <= 0 {
		return ErrInvalidPolicy
	}
	return nil
}
