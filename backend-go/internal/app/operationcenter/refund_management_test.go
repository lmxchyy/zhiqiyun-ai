package operationcenter

import (
	"strings"
	"testing"
	"time"
)

func TestRefundManagementValidationAndDefaultSchedulers(t *testing.T) {
	if !refundPolicyIsFullOnly(JSONSnapshot{"refundPolicy": map[string]any{"mode": "FULL_ONLY"}}) {
		t.Fatal("nested FULL_ONLY policy was not recognized")
	}
	if refundPolicyIsFullOnly(JSONSnapshot{"mode": "PARTIAL"}) {
		t.Fatal("partial refund policy was accepted")
	}
	valid := ManualRefundSubmitCommand{RefundTaskID: "task", IdempotencyKey: "key", ChannelRefundNo: "refund", VoucherReference: "voucher", VoucherFileHash: strings.Repeat("a", 64), Reason: "manual", RefundAmountCents: 100, SubmittedBy: "finance"}
	if !validManualSubmitCommand(valid) {
		t.Fatal("valid manual refund command was rejected")
	}
	valid.VoucherFileHash = "secret"
	if validManualSubmitCommand(valid) {
		t.Fatal("invalid voucher hash was accepted")
	}
	defaults := DefaultRefundSchedulerOptions()
	if defaults.RetryEnabled || defaults.VerificationEnabled || defaults.ManualAutoApproval {
		t.Fatalf("refund automation must default off: %+v", defaults)
	}
	if defaults.BatchLimit <= 0 || defaults.MaxRetryAttempts <= 0 || defaults.MaxVerificationAttempts <= 0 || defaults.LeaseDuration <= 0 {
		t.Fatalf("invalid default scheduler limits: %+v", defaults)
	}
}

func TestRefundSchedulerOptionValidation(t *testing.T) {
	valid := DefaultRefundSchedulerOptions()
	if err := validateRefundSchedulerOptions(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.MaxRetryAttempts = 0
	if err := validateRefundSchedulerOptions(invalid); err == nil {
		t.Fatal("zero retry limit was accepted")
	}
	invalid = valid
	invalid.LeaseDuration = 0 * time.Second
	if err := validateRefundSchedulerOptions(invalid); err == nil {
		t.Fatal("zero lease was accepted")
	}
}
