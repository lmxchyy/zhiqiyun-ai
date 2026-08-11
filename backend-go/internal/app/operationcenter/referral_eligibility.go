package operationcenter

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoReferralRelationship              = errors.New("operation center order has no referral relationship")
	ErrReferralRelationshipSnapshotInvalid = errors.New("operation center referral relationship snapshot is invalid")
	ErrReferralRuleUnsupported             = errors.New("operation center referral rule is unsupported")
	ErrNoPublishedReferralRules            = errors.New("operation center has no published referral eligibility rules")
)

type ReferralReferrerType string

const (
	ReferralReferrerAgent           ReferralReferrerType = "AGENT"
	ReferralReferrerOperationCenter ReferralReferrerType = "OPERATION_CENTER"
)

type ReferralBeneficiaryType string

const (
	ReferralBeneficiaryAgent           ReferralBeneficiaryType = "AGENT"
	ReferralBeneficiaryOperationCenter ReferralBeneficiaryType = "OPERATION_CENTER"
)

type ReferralBeneficiaryRelation string

const (
	ReferralRelationReferrer                ReferralBeneficiaryRelation = "REFERRER"
	ReferralRelationReferrerOperationCenter ReferralBeneficiaryRelation = "REFERRER_OPERATION_CENTER"
)

type ReferralEligibilityStatus string

const (
	ReferralEligibilityEligible  ReferralEligibilityStatus = "ELIGIBLE"
	ReferralEligibilityConsumed  ReferralEligibilityStatus = "CONSUMED"
	ReferralEligibilityCancelled ReferralEligibilityStatus = "CANCELLED"
)

type ReferralEvent struct {
	ID, TenantID, ReferredOperationCenterUserID string
	ReferrerType                                ReferralReferrerType
	ReferrerUserID                              string
	ReferrerOperationCenterUserID               *string
	SourceOrderID, SourceOrderNo                string
	PaymentStatusSnapshot                       string
	ReviewStatusSnapshot                        string
	OperationCenterStatusSnapshot               string
	RelationshipSnapshot                        JSONSnapshot
	TriggeredAt                                 time.Time
	Status, IdempotencyKey                      string
	CreatedAt, UpdatedAt                        time.Time
}

type ReferralEligibility struct {
	ID, TenantID, ReferralEventID string
	CommercialRuleSetID           string
	CommercialRuleSetVersion      int
	ReferralRuleVersionID         string
	ReferralRuleVersion           int
	BeneficiaryType               ReferralBeneficiaryType
	BeneficiaryUserID             string
	BeneficiaryRelation           ReferralBeneficiaryRelation
	RelationshipSnapshot          JSONSnapshot
	Status                        ReferralEligibilityStatus
	IdempotencyKey                string
	RewardID                      *string
	ConsumedAt                    *time.Time
	CreatedAt, UpdatedAt          time.Time
}

type ReferralEligibilityRule struct {
	ID, RuleSetID       string
	RuleSetVersion      int
	Version             int
	ReferrerType        ReferralReferrerType
	BeneficiaryType     ReferralBeneficiaryType
	BeneficiaryRelation ReferralBeneficiaryRelation
}

type referralRelationshipSnapshot struct {
	ReferrerType                  ReferralReferrerType
	ReferrerUserID                string
	ReferrerOperationCenterUserID string
}

type resolvedReferralBeneficiary struct {
	Rule              ReferralEligibilityRule
	BeneficiaryUserID string
}

func parseReferralRelationshipSnapshot(snapshot JSONSnapshot) (referralRelationshipSnapshot, error) {
	referrerType := ReferralReferrerType(strings.ToUpper(snapshotStringValue(snapshot, "referrerType")))
	if referrerType == "" || referrerType == "NONE" {
		return referralRelationshipSnapshot{}, ErrNoReferralRelationship
	}
	result := referralRelationshipSnapshot{ReferrerType: referrerType}
	switch referrerType {
	case ReferralReferrerAgent:
		result.ReferrerUserID = firstSnapshotString(snapshot, "directAgentUserId", "referrerUserId")
		result.ReferrerOperationCenterUserID = firstSnapshotString(snapshot, "referrerOperationCenterUserId", "operationCenterUserId")
		if result.ReferrerUserID == "" {
			return result, ErrReferralRelationshipSnapshotInvalid
		}
	case ReferralReferrerOperationCenter:
		result.ReferrerUserID = firstSnapshotString(snapshot, "referrerUserId", "operationCenterUserId", "referrerOperationCenterUserId")
		result.ReferrerOperationCenterUserID = firstSnapshotString(snapshot, "referrerOperationCenterUserId", "operationCenterUserId", "referrerUserId")
		if result.ReferrerUserID == "" {
			return result, ErrReferralRelationshipSnapshotInvalid
		}
	default:
		return result, ErrReferralRelationshipSnapshotInvalid
	}
	return result, nil
}

func resolveReferralBeneficiaries(snapshot referralRelationshipSnapshot, rules []ReferralEligibilityRule) ([]resolvedReferralBeneficiary, error) {
	result := make([]resolvedReferralBeneficiary, 0, len(rules))
	for _, rule := range rules {
		if rule.ReferrerType != snapshot.ReferrerType {
			return nil, fmt.Errorf("%w: rule %s referrer type %s", ErrReferralRuleUnsupported, rule.ID, rule.ReferrerType)
		}
		beneficiaryID := ""
		switch {
		case rule.BeneficiaryRelation == ReferralRelationReferrer && rule.BeneficiaryType == ReferralBeneficiaryAgent && snapshot.ReferrerType == ReferralReferrerAgent:
			beneficiaryID = snapshot.ReferrerUserID
		case rule.BeneficiaryRelation == ReferralRelationReferrer && rule.BeneficiaryType == ReferralBeneficiaryOperationCenter && snapshot.ReferrerType == ReferralReferrerOperationCenter:
			beneficiaryID = snapshot.ReferrerUserID
		case rule.BeneficiaryRelation == ReferralRelationReferrerOperationCenter && rule.BeneficiaryType == ReferralBeneficiaryOperationCenter && snapshot.ReferrerType == ReferralReferrerAgent:
			beneficiaryID = snapshot.ReferrerOperationCenterUserID
		default:
			return nil, fmt.Errorf("%w: rule %s relation %s beneficiary %s", ErrReferralRuleUnsupported, rule.ID, rule.BeneficiaryRelation, rule.BeneficiaryType)
		}
		if beneficiaryID == "" {
			return nil, fmt.Errorf("%w: rule %s beneficiary snapshot is empty", ErrReferralRelationshipSnapshotInvalid, rule.ID)
		}
		result = append(result, resolvedReferralBeneficiary{Rule: rule, BeneficiaryUserID: beneficiaryID})
	}
	return result, nil
}

func snapshotStringValue(snapshot JSONSnapshot, key string) string {
	value, _ := snapshot[key].(string)
	return strings.TrimSpace(value)
}

func firstSnapshotString(snapshot JSONSnapshot, keys ...string) string {
	for _, key := range keys {
		if value := snapshotStringValue(snapshot, key); value != "" {
			return value
		}
	}
	return ""
}
