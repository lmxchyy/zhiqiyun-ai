package operationcenter

import (
	"errors"
	"testing"
)

func TestProductionReleaseGateConfigRejectsDuplicateProviderMappings(t *testing.T) {
	values := map[string]string{
		"XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_PROVIDER_MAPPINGS": "WECHAT_VIRTUAL=MANUAL,WECHAT_VIRTUAL=AUTO",
	}
	_, err := LoadProductionReleaseGateConfig(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}, nil)
	if err == nil {
		t.Fatal("duplicate provider mapping must fail")
	}
}

func TestProductionReleaseGateConfigUsesOnlyProductionNamespace(t *testing.T) {
	values := map[string]string{
		"XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_PROVIDER_MAPPINGS": "WECHAT_VIRTUAL=MANUAL",
		"XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_SUBMITTER_ID":   "finance_a",
		"XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_APPROVER_ID":    "finance_b",
		"XIANZHI_TEST_OPERATION_CENTER_REFUND_RETRY_SCHEDULER_ENABLED": "true",
	}
	config, err := LoadProductionReleaseGateConfig(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}, []string{"XIANZHI_TEST_OPERATION_CENTER_REFUND_RETRY_SCHEDULER_ENABLED=true"})
	if err != nil {
		t.Fatal(err)
	}
	if config.Runtime.RefundRetrySchedulerEnabled {
		t.Fatal("production runtime read the test scheduler setting")
	}
	if len(config.TestConfigurationKeys) != 1 {
		t.Fatalf("test namespace presence was not reported: %+v", config.TestConfigurationKeys)
	}
	if config.ProviderMappings["WECHAT_VIRTUAL"] != "MANUAL" {
		t.Fatalf("virtual payment mapping=%v", config.ProviderMappings)
	}
}

func TestProductionReleaseGateReturnsNonPassingErrorForNilDatabase(t *testing.T) {
	report, err := RunProductionReleaseGate(t.Context(), nil, ProductionReleaseGateConfig{})
	if !errors.Is(err, ErrProductionReleaseGateFailed) || report.Passed {
		t.Fatalf("report=%+v err=%v", report, err)
	}
}
