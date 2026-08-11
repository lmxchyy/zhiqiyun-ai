package channelrules

import (
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type ScenarioCode string

const (
	ScenarioMemberPurchase ScenarioCode = "MEMBER_PURCHASE"
	ScenarioAgentJoin      ScenarioCode = "AGENT_JOIN"
	ScenarioOCService      ScenarioCode = "OPERATION_CENTER_SERVICE"
)

type RuleSetStatus string

const (
	RuleSetDraft     RuleSetStatus = "DRAFT"
	RuleSetPublished RuleSetStatus = "PUBLISHED"
	RuleSetRetired   RuleSetStatus = "RETIRED"
	RuleSetArchived  RuleSetStatus = "ARCHIVED"
)

type CommercialRuleSet struct {
	ID               string
	TenantID         string
	Code             string
	Version          int
	Name             string
	Description      string
	Status           RuleSetStatus
	EffectiveStartAt time.Time
	EffectiveEndAt   *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PlanConfigVersion struct {
	ID                    string
	TenantID              string
	RuleSetID             string
	PlanID                string
	Version               int
	PriceCents            int64
	Currency              string
	TokenRightsValueCents int64
	TokenGrantAmount      int64
	DurationDays          int
	IdentityType          string
}

type RelationshipSnapshot struct {
	SourceUserID      string
	DirectAgentID     string
	OperationCenterID string
	EffectiveAt       time.Time
	SourceType        string
	SourceID          string
}

type ResolveOrderRequest struct {
	TenantID        string
	OrderID         string
	OrderNo         string
	PlanID          string
	SourceUserID    string
	PaidAmountCents int64
	BusinessTime    time.Time
}

type ResolvedOrderRules struct {
	RuleSet         CommercialRuleSet
	Plan            PlanConfigVersion
	Scenario        ScenarioCode
	Relationship    RelationshipSnapshot
	CommissionRules []commissionapp.CommissionRule
}

type Allocation struct {
	BusinessType    string
	BeneficiaryType string
	BeneficiaryID   string
	AmountCents     int64
	RuleID          string
	RuleVersion     int
}

type OrderCalculation struct {
	RuleSetID                  string
	RuleSetVersion             int
	Scenario                   ScenarioCode
	TokenRightsValueCents      int64
	TokenGrantAmount           int64
	Allocations                []Allocation
	DirectAgentAmountCents     int64
	OperationCenterAmountCents int64
	PlatformAmountCents        int64
}

type RuleSetValidationInput struct {
	RuleSet         CommercialRuleSet
	Plans           []PlanConfigVersion
	CommissionRules []commissionapp.CommissionRule
}

type ReferralRewardRule struct {
	ID                  string
	TenantID            string
	RuleSetID           string
	Code                string
	Version             int
	ReferrerType        string
	BeneficiaryType     string
	BeneficiaryRelation string
	AmountCents         int64
	FreezeDays          int
	Status              RuleSetStatus
}
