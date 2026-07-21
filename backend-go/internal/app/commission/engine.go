package commission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const basisPointsDenominator AmountCents = 10_000

type RelationshipSnapshot struct {
	AgentIDsByLevel   map[int]string
	OperationCenterID string
	PlatformID        string
}

type CalculationInput struct {
	TenantID         string
	OrderID          string
	OrderNo          string
	ProductType      string
	ProductID        string
	SourceUserID     string
	OrderAmountCents AmountCents
	PaidAmountCents  AmountCents
	Quantity         int64
	PaidAt           time.Time
	Relationships    RelationshipSnapshot
	Rules            []CommissionRule
}

type CalculationResult struct {
	Records             []CommissionRecord
	CashCommissionCents AmountCents
	PlatformIncomeCents AmountCents
	SkippedRuleIDs      []string
}

type Engine struct{}

func NewEngine() Engine { return Engine{} }

func (Engine) Calculate(input CalculationInput) (CalculationResult, error) {
	if err := validateCalculationInput(input); err != nil {
		return CalculationResult{}, err
	}
	rules := append([]CommissionRule(nil), input.Rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		leftRemainder := rules[i].CalculationType == CalculationRemainderToPlatform
		rightRemainder := rules[j].CalculationType == CalculationRemainderToPlatform
		if leftRemainder != rightRemainder {
			return !leftRemainder
		}
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		if rules[i].Version != rules[j].Version {
			return rules[i].Version > rules[j].Version
		}
		return rules[i].ID < rules[j].ID
	})

	result := CalculationResult{Records: make([]CommissionRecord, 0, len(rules))}
	seen := make(map[string]struct{}, len(rules))
	remainderRules := 0
	allocated := AmountCents(0)
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			return CalculationResult{}, fmt.Errorf("invalid rule %s: %w", rule.ID, err)
		}
		if !ruleApplies(rule, input) {
			continue
		}
		beneficiaryID := resolveBeneficiary(rule, input.Relationships)
		if beneficiaryID == "" {
			result.SkippedRuleIDs = append(result.SkippedRuleIDs, rule.ID)
			continue
		}
		if rule.CalculationType == CalculationRemainderToPlatform {
			remainderRules++
			if remainderRules > 1 {
				return CalculationResult{}, errors.New("only one remainder-to-platform rule may apply")
			}
		}
		amount, err := calculateRuleAmount(rule, input, allocated)
		if err != nil {
			return CalculationResult{}, fmt.Errorf("calculate rule %s: %w", rule.ID, err)
		}
		if amount == 0 {
			continue
		}
		if amount < 0 || amount > input.PaidAmountCents-allocated {
			return CalculationResult{}, fmt.Errorf("cash commission exceeds paid amount: rule=%s allocated=%d amount=%d paid=%d", rule.ID, allocated, amount, input.PaidAmountCents)
		}
		dedupKey := fmt.Sprintf("%s|%d|%s|%s", rule.ID, rule.Version, rule.BeneficiaryRole, beneficiaryID)
		if _, exists := seen[dedupKey]; exists {
			return CalculationResult{}, fmt.Errorf("duplicate rule beneficiary result: %s", dedupKey)
		}
		seen[dedupKey] = struct{}{}
		allocated += amount
		freezeUntil := input.PaidAt.AddDate(0, 0, rule.FreezeDays)
		idempotencyKey := deterministicKey(input.OrderID, rule.ID, fmt.Sprint(rule.Version), string(rule.BeneficiaryRole), beneficiaryID, string(RecordEarning))
		result.Records = append(result.Records, CommissionRecord{
			ID:              "commission_record_" + idempotencyKey[:24],
			TenantID:        input.TenantID,
			OrderID:         input.OrderID,
			OrderNo:         input.OrderNo,
			BeneficiaryType: rule.BeneficiaryRole,
			BeneficiaryID:   beneficiaryID,
			SourceUserID:    input.SourceUserID,
			RuleID:          rule.ID,
			RuleVersion:     rule.Version,
			AmountCents:     amount,
			Currency:        "CNY",
			RecordType:      RecordEarning,
			Status:          CommissionExpected,
			FreezeUntil:     &freezeUntil,
			AvailableAt:     &freezeUntil,
			IdempotencyKey:  idempotencyKey,
			CreatedAt:       input.PaidAt,
			UpdatedAt:       input.PaidAt,
		})
	}
	if remainderRules != 1 {
		return CalculationResult{}, errors.New("one remainder-to-platform rule is required")
	}
	if allocated != input.PaidAmountCents {
		return CalculationResult{}, fmt.Errorf("cash settlement mismatch: allocated=%d paid=%d", allocated, input.PaidAmountCents)
	}
	result.CashCommissionCents = allocated
	for _, record := range result.Records {
		if record.BeneficiaryType == BeneficiaryPlatform {
			result.PlatformIncomeCents += record.AmountCents
		}
	}
	return result, nil
}

func validateCalculationInput(input CalculationInput) error {
	if blank(input.TenantID) || blank(input.OrderID) || blank(input.OrderNo) || blank(input.ProductType) || blank(input.ProductID) || blank(input.SourceUserID) {
		return errors.New("tenant, order, product and source user identifiers are required")
	}
	if input.OrderAmountCents <= 0 || input.PaidAmountCents <= 0 || input.PaidAmountCents > input.OrderAmountCents {
		return errors.New("order and paid amounts must be positive and paid amount cannot exceed order amount")
	}
	if input.Quantity <= 0 {
		return errors.New("quantity must be positive")
	}
	if input.PaidAt.IsZero() {
		return errors.New("paid time is required")
	}
	if len(input.Rules) == 0 {
		return errors.New("no commission rules are configured")
	}
	return nil
}

func ruleApplies(rule CommissionRule, input CalculationInput) bool {
	isTemplateRule := strings.EqualFold(strings.TrimSpace(rule.ProductType), "COMMISSION_TEMPLATE")
	if !strings.EqualFold(strings.TrimSpace(rule.Status), "ACTIVE") ||
		(rule.TenantID != input.TenantID && rule.TenantID != "tenant_default") ||
		(!isTemplateRule && !strings.EqualFold(rule.ProductType, input.ProductType)) {
		return false
	}
	if !isTemplateRule && rule.ProductID != "" && rule.ProductID != input.ProductID {
		return false
	}
	if input.PaidAt.Before(rule.EffectiveStartAt) || (rule.EffectiveEndAt != nil && !input.PaidAt.Before(*rule.EffectiveEndAt)) {
		return false
	}
	return true
}

func resolveBeneficiary(rule CommissionRule, relationships RelationshipSnapshot) string {
	switch rule.BeneficiaryRole {
	case BeneficiaryAgent:
		return strings.TrimSpace(relationships.AgentIDsByLevel[rule.RelationshipLevel])
	case BeneficiaryOperationCenter:
		return strings.TrimSpace(relationships.OperationCenterID)
	case BeneficiaryPlatform:
		if value := strings.TrimSpace(relationships.PlatformID); value != "" {
			return value
		}
		return "platform"
	default:
		return ""
	}
}

func calculateRuleAmount(rule CommissionRule, input CalculationInput, allocated AmountCents) (AmountCents, error) {
	switch rule.CalculationType {
	case CalculationFixedAmount:
		return rule.FixedAmountCents, nil
	case CalculationOrderPercentage:
		return percentageOf(input.OrderAmountCents, rule.PercentageBPS)
	case CalculationPaidAmountPercentage:
		return percentageOf(input.PaidAmountCents, rule.PercentageBPS)
	case CalculationQuantity:
		return multiplyAmount(rule.FixedAmountCents, input.Quantity)
	case CalculationTiered:
		return calculateTieredAmount(rule.CalculationConfig, input)
	case CalculationRemainderToPlatform:
		return input.PaidAmountCents - allocated, nil
	default:
		return 0, fmt.Errorf("unsupported calculation type %q", rule.CalculationType)
	}
}

func percentageOf(amount AmountCents, bps PercentageBPS) (AmountCents, error) {
	if amount < 0 || bps < 0 || bps > 10_000 {
		return 0, errors.New("invalid percentage operands")
	}
	whole := (amount / basisPointsDenominator) * AmountCents(bps)
	fraction := (amount % basisPointsDenominator) * AmountCents(bps) / basisPointsDenominator
	if whole > AmountCents(math.MaxInt64)-fraction {
		return 0, errors.New("percentage result overflows int64 cents")
	}
	return whole + fraction, nil
}

func multiplyAmount(amount AmountCents, quantity int64) (AmountCents, error) {
	if amount < 0 || quantity < 0 {
		return 0, errors.New("amount and quantity cannot be negative")
	}
	if quantity != 0 && int64(amount) > math.MaxInt64/quantity {
		return 0, errors.New("quantity result overflows int64 cents")
	}
	return amount * AmountCents(quantity), nil
}

type tieredConfig struct {
	Basis string       `json:"basis"`
	Tiers []tierConfig `json:"tiers"`
}

type tierConfig struct {
	MinAmountCents   AmountCents     `json:"minAmountCents"`
	MaxAmountCents   *AmountCents    `json:"maxAmountCents,omitempty"`
	CalculationType  CalculationType `json:"calculationType"`
	FixedAmountCents AmountCents     `json:"fixedAmountCents,omitempty"`
	PercentageBPS    PercentageBPS   `json:"percentageBps,omitempty"`
}

func calculateTieredAmount(raw json.RawMessage, input CalculationInput) (AmountCents, error) {
	var config tieredConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return 0, fmt.Errorf("decode tiered config: %w", err)
	}
	basis := input.PaidAmountCents
	if strings.EqualFold(strings.TrimSpace(config.Basis), "ORDER_AMOUNT") {
		basis = input.OrderAmountCents
	}
	for _, tier := range config.Tiers {
		if basis < tier.MinAmountCents || (tier.MaxAmountCents != nil && basis > *tier.MaxAmountCents) {
			continue
		}
		switch tier.CalculationType {
		case CalculationFixedAmount:
			if tier.FixedAmountCents <= 0 {
				return 0, errors.New("tier fixed amount must be positive")
			}
			return tier.FixedAmountCents, nil
		case CalculationOrderPercentage:
			return percentageOf(input.OrderAmountCents, tier.PercentageBPS)
		case CalculationPaidAmountPercentage:
			return percentageOf(input.PaidAmountCents, tier.PercentageBPS)
		default:
			return 0, fmt.Errorf("unsupported tier calculation type %q", tier.CalculationType)
		}
	}
	return 0, errors.New("no tier matches calculation basis")
}

func deterministicKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}
