package operationcenter

import (
	"testing"
	"time"
)

func TestOperationCenterRuntimeConfigDefaultsAndEnvironmentIsolation(t *testing.T) {
	defaults, err := LoadOperationCenterRuntimeConfig("production", func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if defaults.RefundRetrySchedulerEnabled || defaults.RefundVerificationEnabled || defaults.RewardReleaseSchedulerEnabled || defaults.DryRun {
		t.Fatalf("runtime switches must default off: %+v", defaults)
	}
	values := map[string]string{
		"XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_RETRY_SCHEDULER_ENABLED":  "true",
		"XIANZHI_PRODUCTION_OPERATION_CENTER_BATCH_LIMIT":                     "7",
		"XIANZHI_PRODUCTION_OPERATION_CENTER_UNKNOWN_SAFETY_WAIT":             "45m",
		"XIANZHI_TEST_OPERATION_CENTER_REFUND_VERIFICATION_SCHEDULER_ENABLED": "true",
		"XIANZHI_TEST_OPERATION_CENTER_BATCH_LIMIT":                           "3",
	}
	lookup := func(name string) (string, bool) { value, ok := values[name]; return value, ok }
	production, err := LoadOperationCenterRuntimeConfig("prod", lookup)
	if err != nil {
		t.Fatal(err)
	}
	testConfig, err := LoadOperationCenterRuntimeConfig("test", lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !production.RefundRetrySchedulerEnabled || production.RefundVerificationEnabled || production.BatchLimit != 7 || production.UnknownSafetyWait != 45*time.Minute {
		t.Fatalf("production config isolation failed: %+v", production)
	}
	if testConfig.RefundRetrySchedulerEnabled || !testConfig.RefundVerificationEnabled || testConfig.BatchLimit != 3 || testConfig.UnknownSafetyWait != 30*time.Minute {
		t.Fatalf("test config isolation failed: %+v", testConfig)
	}
}

func TestOperationCenterRuntimeConfigRejectsUnsafeValues(t *testing.T) {
	_, err := LoadOperationCenterRuntimeConfig("test", func(name string) (string, bool) {
		if name == "XIANZHI_TEST_OPERATION_CENTER_BATCH_LIMIT" {
			return "0", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("zero batch limit must be rejected")
	}
}
