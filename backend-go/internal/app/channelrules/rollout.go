package channelrules

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
)

type RolloutMode string

const (
	RolloutModeLegacy     RolloutMode = "LEGACY"
	RolloutModeShadow     RolloutMode = "SHADOW"
	RolloutModeV132Canary RolloutMode = "CANARY"
	RolloutModeV132Full   RolloutMode = "V132"
)

type RolloutConfig struct {
	TenantID                 string
	ConfigVersion            int
	Mode                     RolloutMode
	Enabled                  bool
	PinnedRuleSetID          string
	PinnedRuleSetVersion     int
	CanaryBasisPoints        int
	HashSalt                 string
	AllowOrderIDs            []string
	AllowUserIDs             []string
	AllowPlanIDs             []string
	AllowTenantIDs           []string
	DenyOrderIDs             []string
	DenyUserIDs              []string
	PercentageRolloutEnabled bool
	RealSwitchEnabled        bool
}

type RolloutSubject struct {
	TenantID               string
	OrderID                string
	UserID                 string
	PlanID                 string
	OperationCenterPackage bool
}

type RolloutDecision struct {
	CalculateShadow   bool
	UseV132Settlement bool
	Reason            string
	Bucket            int
}

func (c RolloutConfig) Validate() error {
	c.TenantID = strings.TrimSpace(c.TenantID)
	c.PinnedRuleSetID = strings.TrimSpace(c.PinnedRuleSetID)
	if c.TenantID == "" {
		return fmt.Errorf("%s: rollout tenant is required", ErrCodeRuleValidationFailed)
	}
	if !c.Enabled || c.Mode == RolloutModeLegacy {
		return nil
	}
	if c.PinnedRuleSetID == "" || c.PinnedRuleSetVersion <= 0 {
		return fmt.Errorf("%s: rollout requires an exact pinned rule set and version", ErrCodeRuleValidationFailed)
	}
	switch c.Mode {
	case RolloutModeShadow:
		if c.CanaryBasisPoints != 0 || c.PercentageRolloutEnabled {
			return fmt.Errorf("%s: shadow mode cannot have a real canary percentage", ErrCodeRuleValidationFailed)
		}
	case RolloutModeV132Canary:
		if !c.RealSwitchEnabled {
			return fmt.Errorf("%s: canary mode requires real-switch approval", ErrCodeRuleValidationFailed)
		}
		if c.PercentageRolloutEnabled && (c.CanaryBasisPoints <= 0 || c.CanaryBasisPoints > 10000) {
			return fmt.Errorf("%s: percentage rollout requires 1-10000 basis points", ErrCodeRuleValidationFailed)
		}
		if !c.PercentageRolloutEnabled && c.CanaryBasisPoints != 0 {
			return fmt.Errorf("%s: whitelist-only canary cannot configure a global percentage", ErrCodeRuleValidationFailed)
		}
	case RolloutModeV132Full:
		if !c.RealSwitchEnabled {
			return fmt.Errorf("%s: full rollout requires real-switch approval", ErrCodeRuleValidationFailed)
		}
	default:
		return fmt.Errorf("%s: unsupported rollout mode %s", ErrCodeRuleValidationFailed, c.Mode)
	}
	return nil
}

func EvaluateRollout(config RolloutConfig, subject RolloutSubject) (RolloutDecision, error) {
	if err := config.Validate(); err != nil {
		return RolloutDecision{}, err
	}
	if subject.OperationCenterPackage {
		return RolloutDecision{Reason: "OPERATION_CENTER_PACKAGE_EXCLUDED", Bucket: -1}, nil
	}
	if !config.Enabled || config.Mode == RolloutModeLegacy {
		return RolloutDecision{Reason: "LEGACY_MODE", Bucket: -1}, nil
	}
	decision := RolloutDecision{CalculateShadow: true, Reason: "SHADOW_ONLY", Bucket: -1}
	if config.Mode == RolloutModeShadow {
		return decision, nil
	}
	if containsRolloutID(config.DenyOrderIDs, subject.OrderID) || containsRolloutID(config.DenyUserIDs, subject.UserID) {
		decision.Reason = "DENY_LIST"
		return decision, nil
	}
	if containsRolloutID(config.AllowTenantIDs, subject.TenantID) || containsRolloutID(config.AllowOrderIDs, subject.OrderID) || containsRolloutID(config.AllowUserIDs, subject.UserID) || containsRolloutID(config.AllowPlanIDs, subject.PlanID) {
		decision.UseV132Settlement = true
		decision.Reason = "ALLOW_LIST"
		return decision, nil
	}
	if config.Mode == RolloutModeV132Full {
		decision.UseV132Settlement = true
		decision.Reason = "FULL_ROLLOUT"
		return decision, nil
	}
	stableID := strings.TrimSpace(subject.OrderID)
	if stableID == "" {
		stableID = strings.TrimSpace(subject.UserID)
	}
	if stableID == "" {
		return RolloutDecision{}, fmt.Errorf("%s: rollout subject requires an order or user id", ErrCodeRuleValidationFailed)
	}
	digest := sha256.Sum256([]byte(config.TenantID + "|" + stableID))
	decision.Bucket = int(binary.BigEndian.Uint64(digest[:8]) % 10000)
	decision.UseV132Settlement = config.PercentageRolloutEnabled && decision.Bucket < config.CanaryBasisPoints
	if decision.UseV132Settlement {
		decision.Reason = "CANARY_SELECTED"
	} else {
		decision.Reason = "CANARY_NOT_SELECTED"
	}
	return decision, nil
}

func containsRolloutID(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target && target != "" {
			return true
		}
	}
	return false
}
