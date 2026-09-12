package httpserver

import (
	"testing"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func TestPR4AUnknownAllowsOnlyDiagnosisAndManualReview(t *testing.T) {
	execution := pe.Execution{Status: pe.Unknown}
	actions, blocked, evidence := allowedRecoveryActions(generationTask{Status: "PROCESSING", BillingStatus: "RESERVED"}, execution, true, map[string]any{"dlq": true})
	if len(actions) != 2 || !recoveryContainsString(actions, recoveryActionDiagnose) || !recoveryContainsString(actions, recoveryActionManualReview) {
		t.Fatalf("unknown allowed actions = %#v", actions)
	}
	if blocked == "" || !evidence {
		t.Fatalf("unknown recovery boundary lost: blocked=%q evidence=%v", blocked, evidence)
	}
	response := recoveryDiagnosisResponse{Provider: map[string]any{"status": string(pe.Unknown)}, Recovery: map[string]any{"allowedActions": actions}}
	if err := (api{}).validateRecoveryAction(recoveryActionRequest{Action: recoveryActionRedrive, Reason: "test"}, response); err == nil {
		t.Fatal("unknown redrive must be blocked")
	}
	if err := (api{}).validateRecoveryAction(recoveryActionRequest{Action: recoveryActionResolveRelease, Reason: "external confirmation", Evidence: map[string]any{"providerOutcome": "not_submitted"}}, response); err != nil {
		t.Fatalf("external evidence should permit release resolution: %v", err)
	}
}

func TestPR4AVideoUnknownCannotCreateChild(t *testing.T) {
	requestID := "provider-request-1"
	if videoRetryChildAllowed(pe.Execution{Status: pe.Unknown, ProviderRequestID: &requestID}) {
		t.Fatal("UNKNOWN execution must not enter the video child retry path")
	}
	if !videoRetryChildAllowed(pe.Execution{Status: pe.Processing, ProviderRequestID: &requestID}) {
		t.Fatal("queryable processing execution should retain the existing get-first child path")
	}
}
