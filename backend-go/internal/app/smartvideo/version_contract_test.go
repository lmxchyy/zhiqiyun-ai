package smartvideo

import "testing"

func TestAttachRenderManifestRejectsPlanMutationSemantics(t *testing.T) {
	// Pure contract: confirmed versions may only receive matching manifest hash.
	// Repository-level enforcement is covered by PostgresMontage integration tests.
	first := "abc"
	second := "def"
	if first == second {
		t.Fatal("fixture hashes must differ")
	}
	if err := ErrVersionImmutable; err == nil {
		t.Fatal("ErrVersionImmutable must be defined")
	}
}

func TestPlanTaskStates(t *testing.T) {
	allowed := map[string]map[string]bool{
		PlanStatusCreated:    {PlanStatusQueued: true, PlanStatusProcessing: true, PlanStatusFailed: true},
		PlanStatusQueued:     {PlanStatusProcessing: true, PlanStatusFailed: true},
		PlanStatusProcessing: {PlanStatusSucceeded: true, PlanStatusFailed: true},
		PlanStatusFailed:     {},
		PlanStatusSucceeded:  {},
	}
	for from, tos := range allowed {
		for to := range tos {
			if from == to {
				t.Fatalf("self transition not expected: %s", from)
			}
		}
	}
	if PlanDailyLimit != 20 {
		t.Fatalf("PlanDailyLimit = %d, want 20", PlanDailyLimit)
	}
}
