package operationcenter

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type OperationCenterRuntimeConfig struct {
	Environment                    string
	RefundRetrySchedulerEnabled    bool
	RefundVerificationEnabled      bool
	RewardReleaseSchedulerEnabled  bool
	DryRun                         bool
	ManualRefundAutoApproval       bool
	BatchLimit                     int
	MaxRetryAttempts               int
	MaxVerificationAttempts        int
	ProviderLeaseDuration          time.Duration
	TemporaryRetryDelay            time.Duration
	UnknownSafetyWait              time.Duration
	VerificationInterval           time.Duration
	RewardReleaseLeaseDuration     time.Duration
	RewardReleaseRetryDelay        time.Duration
	RefundRetrySchedulerInterval   time.Duration
	RefundVerificationInterval     time.Duration
	RewardReleaseSchedulerInterval time.Duration
	WorkerName                     string
}

type RuntimeEnvironmentLookup func(string) (string, bool)

func DefaultOperationCenterRuntimeConfig(environment string) OperationCenterRuntimeConfig {
	return OperationCenterRuntimeConfig{
		Environment: environment,
		BatchLimit:  20, MaxRetryAttempts: 5, MaxVerificationAttempts: 12,
		ProviderLeaseDuration: 2 * time.Minute, TemporaryRetryDelay: 10 * time.Minute,
		UnknownSafetyWait: 30 * time.Minute, VerificationInterval: 5 * time.Minute,
		RewardReleaseLeaseDuration: 2 * time.Minute, RewardReleaseRetryDelay: time.Minute,
		RefundRetrySchedulerInterval: time.Minute, RefundVerificationInterval: time.Minute,
		RewardReleaseSchedulerInterval: time.Minute,
		WorkerName:                     "operation-center-runtime",
	}
}

func LoadOperationCenterRuntimeConfig(environment string, lookup RuntimeEnvironmentLookup) (OperationCenterRuntimeConfig, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	result := DefaultOperationCenterRuntimeConfig(environment)
	prefix := operationCenterRuntimeEnvironmentPrefix(environment)
	var err error
	if result.RefundRetrySchedulerEnabled, err = runtimeBool(lookup, prefix+"REFUND_RETRY_SCHEDULER_ENABLED", result.RefundRetrySchedulerEnabled); err != nil {
		return result, err
	}
	if result.RefundVerificationEnabled, err = runtimeBool(lookup, prefix+"REFUND_VERIFICATION_SCHEDULER_ENABLED", result.RefundVerificationEnabled); err != nil {
		return result, err
	}
	if result.RewardReleaseSchedulerEnabled, err = runtimeBool(lookup, prefix+"REFERRAL_REWARD_RELEASE_SCHEDULER_ENABLED", result.RewardReleaseSchedulerEnabled); err != nil {
		return result, err
	}
	if result.DryRun, err = runtimeBool(lookup, prefix+"DRY_RUN", result.DryRun); err != nil {
		return result, err
	}
	if result.ManualRefundAutoApproval, err = runtimeBool(lookup, prefix+"MANUAL_REFUND_AUTO_APPROVAL", result.ManualRefundAutoApproval); err != nil {
		return result, err
	}
	if result.BatchLimit, err = runtimePositiveInt(lookup, prefix+"BATCH_LIMIT", result.BatchLimit); err != nil {
		return result, err
	}
	if result.MaxRetryAttempts, err = runtimePositiveInt(lookup, prefix+"MAX_RETRY_ATTEMPTS", result.MaxRetryAttempts); err != nil {
		return result, err
	}
	if result.MaxVerificationAttempts, err = runtimePositiveInt(lookup, prefix+"MAX_VERIFICATION_ATTEMPTS", result.MaxVerificationAttempts); err != nil {
		return result, err
	}
	durations := []struct {
		name   string
		target *time.Duration
	}{
		{"PROVIDER_LEASE_DURATION", &result.ProviderLeaseDuration},
		{"TEMPORARY_RETRY_DELAY", &result.TemporaryRetryDelay},
		{"UNKNOWN_SAFETY_WAIT", &result.UnknownSafetyWait},
		{"VERIFICATION_INTERVAL", &result.VerificationInterval},
		{"REWARD_RELEASE_LEASE_DURATION", &result.RewardReleaseLeaseDuration},
		{"REWARD_RELEASE_RETRY_DELAY", &result.RewardReleaseRetryDelay},
		{"REFUND_RETRY_SCHEDULER_INTERVAL", &result.RefundRetrySchedulerInterval},
		{"REFUND_VERIFICATION_SCHEDULER_INTERVAL", &result.RefundVerificationInterval},
		{"REFERRAL_REWARD_RELEASE_SCHEDULER_INTERVAL", &result.RewardReleaseSchedulerInterval},
	}
	for _, item := range durations {
		if *item.target, err = runtimePositiveDuration(lookup, prefix+item.name, *item.target); err != nil {
			return result, err
		}
	}
	if value, ok := lookup(prefix + "WORKER_NAME"); ok {
		result.WorkerName = strings.TrimSpace(value)
	}
	if err := result.Validate(); err != nil {
		return result, err
	}
	return result, nil
}

func (config OperationCenterRuntimeConfig) Validate() error {
	if config.BatchLimit <= 0 || config.MaxRetryAttempts <= 0 || config.MaxVerificationAttempts <= 0 ||
		config.ProviderLeaseDuration <= 0 || config.TemporaryRetryDelay <= 0 || config.UnknownSafetyWait <= 0 ||
		config.VerificationInterval <= 0 || config.RewardReleaseLeaseDuration <= 0 ||
		config.RewardReleaseRetryDelay <= 0 || config.RefundRetrySchedulerInterval <= 0 ||
		config.RefundVerificationInterval <= 0 || config.RewardReleaseSchedulerInterval <= 0 ||
		strings.TrimSpace(config.WorkerName) == "" {
		return ErrConstraintViolation
	}
	if config.ManualRefundAutoApproval {
		return fmt.Errorf("manual refund auto approval must remain disabled: %w", ErrConstraintViolation)
	}
	if config.UnknownSafetyWait < config.RefundVerificationInterval {
		return fmt.Errorf("unknown safety wait must cover at least one verification interval: %w", ErrConstraintViolation)
	}
	return nil
}

func operationCenterRuntimeEnvironmentPrefix(environment string) string {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "production", "prod":
		return "XIANZHI_PRODUCTION_OPERATION_CENTER_"
	case "test", "testing":
		return "XIANZHI_TEST_OPERATION_CENTER_"
	default:
		return "XIANZHI_DEVELOPMENT_OPERATION_CENTER_"
	}
}

func runtimeBool(lookup RuntimeEnvironmentLookup, name string, fallback bool) (bool, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("%s: %w", name, err)
	}
	return parsed, nil
}

func runtimePositiveInt(lookup RuntimeEnvironmentLookup, name string, fallback int) (int, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func runtimePositiveDuration(lookup RuntimeEnvironmentLookup, name string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return parsed, nil
}
