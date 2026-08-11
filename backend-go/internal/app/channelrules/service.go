package channelrules

import (
	"context"
	"fmt"
	"strings"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type RuleBundleQuery struct {
	TenantID     string
	PlanID       string
	BusinessTime time.Time
}

type RelationshipQuery struct {
	TenantID     string
	SourceUserID string
	BusinessTime time.Time
}

type RuleBundle struct {
	RuleSet       CommercialRuleSet
	Plan          PlanConfigVersion
	Plans         []PlanConfigVersion
	Rules         []commissionapp.CommissionRule
	ReferralRules []ReferralRewardRule
}

type PublishRuleSetRequest struct {
	TenantID    string
	RuleSetID   string
	OperatorID  string
	PublishedAt time.Time
}

type RuleSetPublisher interface {
	LoadRuleBundleByID(context.Context, string, string) (RuleBundle, error)
	PublishRuleBundle(context.Context, PublishRuleSetRequest) error
}

type ShadowRuleStore interface {
	LoadShadowRuleBundle(context.Context, RuleBundleQuery) (RuleBundle, error)
}

type OrderRuleSnapshot struct {
	TenantID              string
	OrderID               string
	OrderNo               string
	SourceUserID          string
	PlanID                string
	PlanVersionID         string
	RuleSetID             string
	RuleSetVersion        int
	Scenario              ScenarioCode
	PaidAmountCents       int64
	TokenRightsValueCents int64
	TokenGrantAmount      int64
	DirectAgentID         string
	OperationCenterID     string
	BusinessTime          time.Time
	Relationship          RelationshipSnapshot
	CommissionRules       []commissionapp.CommissionRule
}

type Store interface {
	LoadEffectiveRuleBundle(context.Context, RuleBundleQuery) (RuleBundle, error)
	ResolveRelationshipSnapshot(context.Context, RelationshipQuery) (RelationshipSnapshot, error)
	SaveOrderRuleSnapshot(context.Context, OrderRuleSnapshot) error
}

type ChannelRuleService struct {
	store Store
}

func NewChannelRuleService(store Store) ChannelRuleService {
	return ChannelRuleService{store: store}
}

func (s ChannelRuleService) ResolveOrder(ctx context.Context, request ResolveOrderRequest) (ResolvedOrderRules, error) {
	if s.store == nil {
		return ResolvedOrderRules{}, fmt.Errorf("%s: rule store is required", ErrCodeRuleValidationFailed)
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.OrderID = strings.TrimSpace(request.OrderID)
	request.OrderNo = strings.TrimSpace(request.OrderNo)
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.SourceUserID = strings.TrimSpace(request.SourceUserID)
	if request.TenantID == "" || request.OrderID == "" || request.OrderNo == "" || request.PlanID == "" || request.SourceUserID == "" || request.PaidAmountCents <= 0 || request.BusinessTime.IsZero() {
		return ResolvedOrderRules{}, fmt.Errorf("%s: complete order context is required", ErrCodeRuleValidationFailed)
	}

	bundle, err := s.store.LoadEffectiveRuleBundle(ctx, RuleBundleQuery{
		TenantID: request.TenantID, PlanID: request.PlanID, BusinessTime: request.BusinessTime,
	})
	if err != nil {
		return ResolvedOrderRules{}, err
	}
	if bundle.RuleSet.Status != RuleSetPublished || bundle.RuleSet.TenantID != request.TenantID || bundle.Plan.TenantID != request.TenantID || bundle.Plan.PlanID != request.PlanID {
		return ResolvedOrderRules{}, fmt.Errorf("%s: effective rule bundle does not match order", ErrCodeRuleValidationFailed)
	}
	if bundle.RuleSet.EffectiveStartAt.After(request.BusinessTime) || (bundle.RuleSet.EffectiveEndAt != nil && !bundle.RuleSet.EffectiveEndAt.After(request.BusinessTime)) {
		return ResolvedOrderRules{}, fmt.Errorf("%s: rule set is outside its effective window", ErrCodeRuleValidationFailed)
	}
	if request.PaidAmountCents != bundle.Plan.PriceCents {
		return ResolvedOrderRules{}, fmt.Errorf("%s: paid amount does not match the snapshotted plan price", ErrCodeRuleValidationFailed)
	}
	if err := ValidateRuleSet(RuleSetValidationInput{
		RuleSet: bundle.RuleSet, Plans: []PlanConfigVersion{bundle.Plan}, CommissionRules: bundle.Rules,
	}); err != nil {
		return ResolvedOrderRules{}, err
	}
	scenario, err := scenarioForIdentity(bundle.Plan.IdentityType)
	if err != nil {
		return ResolvedOrderRules{}, err
	}
	relationship, err := s.store.ResolveRelationshipSnapshot(ctx, RelationshipQuery{
		TenantID: request.TenantID, SourceUserID: request.SourceUserID, BusinessTime: request.BusinessTime,
	})
	if err != nil {
		return ResolvedOrderRules{}, err
	}
	relationship.SourceUserID = request.SourceUserID
	if relationship.EffectiveAt.IsZero() {
		relationship.EffectiveAt = request.BusinessTime
	}

	snapshot := OrderRuleSnapshot{
		TenantID: request.TenantID, OrderID: request.OrderID, OrderNo: request.OrderNo,
		SourceUserID: request.SourceUserID, PlanID: request.PlanID, PlanVersionID: bundle.Plan.ID,
		RuleSetID: bundle.RuleSet.ID, RuleSetVersion: bundle.RuleSet.Version, Scenario: scenario,
		PaidAmountCents: request.PaidAmountCents, TokenRightsValueCents: bundle.Plan.TokenRightsValueCents,
		TokenGrantAmount: bundle.Plan.TokenGrantAmount, DirectAgentID: relationship.DirectAgentID,
		OperationCenterID: relationship.OperationCenterID, BusinessTime: request.BusinessTime,
		Relationship: relationship, CommissionRules: append([]commissionapp.CommissionRule(nil), bundle.Rules...),
	}
	if err := s.store.SaveOrderRuleSnapshot(ctx, snapshot); err != nil {
		return ResolvedOrderRules{}, err
	}
	return ResolvedOrderRules{
		RuleSet: bundle.RuleSet, Plan: bundle.Plan, Scenario: scenario, Relationship: relationship,
		CommissionRules: append([]commissionapp.CommissionRule(nil), bundle.Rules...),
	}, nil
}

func (s ChannelRuleService) PublishRuleSet(ctx context.Context, request PublishRuleSetRequest) error {
	publisher, ok := s.store.(RuleSetPublisher)
	if !ok {
		return fmt.Errorf("%s: rule store does not support publishing", ErrCodeRuleValidationFailed)
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.RuleSetID = strings.TrimSpace(request.RuleSetID)
	request.OperatorID = strings.TrimSpace(request.OperatorID)
	if request.TenantID == "" || request.RuleSetID == "" || request.OperatorID == "" || request.PublishedAt.IsZero() {
		return fmt.Errorf("%s: complete publish context is required", ErrCodeRuleValidationFailed)
	}
	bundle, err := publisher.LoadRuleBundleByID(ctx, request.TenantID, request.RuleSetID)
	if err != nil {
		return err
	}
	if bundle.RuleSet.Status != RuleSetDraft {
		return fmt.Errorf("%s: only draft rule sets can be published", ErrCodeRuleValidationFailed)
	}
	plans := bundle.Plans
	if len(plans) == 0 && bundle.Plan.ID != "" {
		plans = []PlanConfigVersion{bundle.Plan}
	}
	if err := ValidateRuleSet(RuleSetValidationInput{RuleSet: bundle.RuleSet, Plans: plans, CommissionRules: bundle.Rules}); err != nil {
		return err
	}
	if err := ValidateReferralRewardRules(bundle.RuleSet, bundle.ReferralRules); err != nil {
		return err
	}
	return publisher.PublishRuleBundle(ctx, request)
}

func (s ChannelRuleService) ResolveShadowOrder(ctx context.Context, request ResolveOrderRequest, relationship RelationshipSnapshot) (ResolvedOrderRules, error) {
	shadowStore, ok := s.store.(ShadowRuleStore)
	if !ok {
		return ResolvedOrderRules{}, fmt.Errorf("%s: rule store does not support shadow resolution", ErrCodeRuleValidationFailed)
	}
	request.TenantID = strings.TrimSpace(request.TenantID)
	request.OrderID = strings.TrimSpace(request.OrderID)
	request.OrderNo = strings.TrimSpace(request.OrderNo)
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.SourceUserID = strings.TrimSpace(request.SourceUserID)
	if request.TenantID == "" || request.OrderID == "" || request.OrderNo == "" || request.PlanID == "" || request.SourceUserID == "" || request.PaidAmountCents <= 0 || request.BusinessTime.IsZero() {
		return ResolvedOrderRules{}, fmt.Errorf("%s: complete shadow order context is required", ErrCodeRuleValidationFailed)
	}
	bundle, err := shadowStore.LoadShadowRuleBundle(ctx, RuleBundleQuery{
		TenantID: request.TenantID, PlanID: request.PlanID, BusinessTime: request.BusinessTime,
	})
	if err != nil {
		return ResolvedOrderRules{}, err
	}
	if bundle.RuleSet.Status != RuleSetDraft && bundle.RuleSet.Status != RuleSetPublished {
		return ResolvedOrderRules{}, fmt.Errorf("%s: shadow rule set must be draft or published", ErrCodeRuleValidationFailed)
	}
	if bundle.RuleSet.TenantID != request.TenantID || bundle.Plan.TenantID != request.TenantID || bundle.Plan.PlanID != request.PlanID || request.PaidAmountCents != bundle.Plan.PriceCents {
		return ResolvedOrderRules{}, fmt.Errorf("%s: shadow rule bundle does not match order", ErrCodeRuleValidationFailed)
	}
	if err := ValidateRuleSet(RuleSetValidationInput{
		RuleSet: bundle.RuleSet, Plans: []PlanConfigVersion{bundle.Plan}, CommissionRules: bundle.Rules,
	}); err != nil {
		return ResolvedOrderRules{}, err
	}
	scenario, err := scenarioForIdentity(bundle.Plan.IdentityType)
	if err != nil {
		return ResolvedOrderRules{}, err
	}
	relationship.SourceUserID = request.SourceUserID
	if relationship.EffectiveAt.IsZero() {
		relationship.EffectiveAt = request.BusinessTime
	}
	return ResolvedOrderRules{
		RuleSet: bundle.RuleSet, Plan: bundle.Plan, Scenario: scenario, Relationship: relationship,
		CommissionRules: append([]commissionapp.CommissionRule(nil), bundle.Rules...),
	}, nil
}
