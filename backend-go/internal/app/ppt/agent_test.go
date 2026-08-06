package ppt

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStageStatusMappingIncludesCancelled(t *testing.T) {
	cases := map[Stage]string{
		StageDraft:        StatusPending,
		StageOutlineReady: StatusPending,
		StageGenerating:   StatusProcessing,
		StageReady:        StatusSuccess,
		StageFailed:       StatusFailed,
		StageCancelled:    StatusCancelled,
	}
	for stage, want := range cases {
		if got := StageStatus(stage); got != want {
			t.Fatalf("%s: got %s want %s", stage, got, want)
		}
	}
}
func TestValidateTaskStageFailsClosed(t *testing.T) {
	for _, task := range []Task{
		{Status: StatusSuccess},
		{Stage: Stage("UNKNOWN"), Status: StatusPending},
		{Stage: StageReady, Status: StatusPending},
	} {
		if err := ValidateTaskStage(task); !errors.Is(err, ErrInvalidStage) {
			t.Fatalf("ValidateTaskStage(%#v) error = %v, want ErrInvalidStage", task, err)
		}
	}
}

func TestOwnerScopeRejectsBlankTenantOrUser(t *testing.T) {
	for _, owner := range []OwnerScope{{}, {TenantID: "tenant_a"}, {UserID: "user_a"}} {
		if _, err := owner.Validated(); !errors.Is(err, ErrOwnerScopeRequired) {
			t.Fatalf("OwnerScope.Validated(%#v) error = %v, want ErrOwnerScopeRequired", owner, err)
		}
	}
	got, err := (OwnerScope{TenantID: " tenant_a ", UserID: " user_a "}).Validated()
	if err != nil || got.TenantID != "tenant_a" || got.UserID != "user_a" {
		t.Fatalf("OwnerScope.Validated() = %#v, %v", got, err)
	}
}

func TestDeckSpecWithImagesUsesCanonicalNormalizedStrategy(t *testing.T) {
	tests := []struct {
		imageSource string
		want        bool
	}{
		{imageSource: "none", want: false},
		{imageSource: " NONE ", want: false},
		{imageSource: "ai", want: true},
		{imageSource: "stock", want: true},
		{imageSource: "", want: true},
	}
	for _, test := range tests {
		t.Run(test.imageSource, func(t *testing.T) {
			if got := (DeckSpec{ImageSource: test.imageSource}).WithImages(); got != test.want {
				t.Fatalf("DeckSpec.WithImages() = %v, want %v for %q", got, test.want, test.imageSource)
			}
			if got := (Task{ImageSource: test.imageSource}).WithImages(); got != test.want {
				t.Fatalf("Task.WithImages() = %v, want %v for %q", got, test.want, test.imageSource)
			}
		})
	}
}

func TestNormalizeTaskDoesNotRepairConflictingTerminalStatus(t *testing.T) {
	for _, stage := range []Stage{StageFailed, StageCancelled} {
		t.Run(string(stage), func(t *testing.T) {
			got := NormalizeTask(Task{Stage: stage, Status: StatusSuccess, Progress: 37, CurrentPage: 3})
			if got.Status != StatusSuccess || got.Progress != 37 || got.CurrentPage != 3 || !errors.Is(ValidateTaskStage(got), ErrInvalidStage) {
				t.Fatalf("NormalizeTask() terminal task = %#v", got)
			}
		})
	}
}

func TestNormalizeTaskDoesNotRegressGeneratingProgress(t *testing.T) {
	got := NormalizeTask(Task{
		Stage: StageGenerating, Status: StatusProcessing,
		SlideCount: 4, CurrentPage: 3, Progress: 75,
		Slides: []Slide{{ID: "slide_1", Page: 1}},
	})
	if got.CurrentPage != 3 || got.Progress != 75 {
		t.Fatalf("NormalizeTask() progress = %d/%d, want 3/75", got.CurrentPage, got.Progress)
	}
}

func TestNormalizePPTSourceFileIDsPreservesOrderAndRemovesDuplicates(t *testing.T) {
	got := normalizePPTSourceFileIDs([]string{" file_b ", "file_a", "file_b", "", " file_c"})
	want := []string{"file_b", "file_a", "file_c"}
	if len(got) != len(want) {
		t.Fatalf("normalizePPTSourceFileIDs() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("normalizePPTSourceFileIDs()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestAgentDomainContractsUseApprovedJSONTags(t *testing.T) {
	task := Task{
		SessionID: "session_1", SkillCode: "general", Stage: StageDraft,
		AgentMessages: []AgentMessage{{Role: "assistant", Content: "hello", CreatedAt: "2026-08-03T00:00:00Z"}},
		SourceFileIDs: []string{"file_1"}, OutlineConfirmedAt: "outline-time", GenerationStartedAt: "generation-time",
		CompletedAt: "completed-time", ErrorCode: "PPT_FAILED", BillingTaskID: "billing_1", VisualProgress: 25,
		GenerationLease: &GenerationLease{RunToken: "run_1", LeaseUntil: "lease-time"},
		IdempotencyRecords: []IdempotencyRecord{{
			Scope: "message", Key: "key_1", RequestHash: "hash_1", State: "completed", ResponseJSON: `{"ok":true}`,
			ErrorCode: "PPT_FAILED", OperationToken: "operation_1", CreatedAt: "created-time", UpdatedAt: "updated-time",
		}},
	}
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{
		"sessionId", "skillCode", "stage", "agentMessages", "sourceFileIds", "outlineConfirmedAt",
		"generationStartedAt", "completedAt", "errorCode", "billingTaskId", "visualProgress", "generationLease", "idempotencyRecords",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("Task JSON missing approved key %q: %s", key, raw)
		}
	}
	message := payload["agentMessages"].([]any)[0].(map[string]any)
	for _, key := range []string{"role", "content", "createdAt"} {
		if _, ok := message[key]; !ok {
			t.Fatalf("AgentMessage JSON missing approved key %q: %s", key, raw)
		}
	}
	lease := payload["generationLease"].(map[string]any)
	for _, key := range []string{"runToken", "leaseUntil"} {
		if _, ok := lease[key]; !ok {
			t.Fatalf("GenerationLease JSON missing approved key %q: %s", key, raw)
		}
	}
	record := payload["idempotencyRecords"].([]any)[0].(map[string]any)
	for _, key := range []string{"scope", "key", "requestHash", "state", "responseJson", "errorCode", "operationToken", "createdAt", "updatedAt"} {
		if _, ok := record[key]; !ok {
			t.Fatalf("IdempotencyRecord JSON missing approved key %q: %s", key, raw)
		}
	}
}
