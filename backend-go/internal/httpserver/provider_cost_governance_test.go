package httpserver

import (
	"testing"
	"time"
)

func TestSelectProviderCostHonorsEffectiveWindowAndSpecificity(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	task := generationTask{
		Model:  "seedance-fast-2.0",
		Params: map[string]any{"resolution": "720p", "duration": float64(5)},
	}
	costs := []providerCost{
		{ID: "expired", PlatformModelCode: task.Model, BillingUnit: "PER_SECOND", UnitCost: 0.20, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-01-01T00:00:00Z", EffectiveTo: "2026-08-01T00:00:00Z"},
		{ID: "future", PlatformModelCode: task.Model, BillingUnit: "PER_SECOND", UnitCost: 0.90, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-09-01T00:00:00Z"},
		{ID: "broad", PlatformModelCode: task.Model, BillingUnit: "PER_SECOND", UnitCost: 0.80, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-01-01T00:00:00Z"},
		{ID: "specific", PlatformModelCode: task.Model, BillingUnit: "PER_SECOND", UnitCost: 0.85, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-02-01T00:00:00Z", ParameterRange: map[string]any{"resolution": map[string]any{"value": "720p"}}},
	}
	got := selectProviderCost(costs, task, now)
	if !got.Found || got.Issue != "" || got.Cost.ID != "specific" {
		t.Fatalf("selection = %#v, want specific active cost", got)
	}
}

func TestSelectProviderCostRejectsAmbiguousTopCandidates(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	task := generationTask{Model: "gpt-image-2", Params: map[string]any{"quality": "high"}}
	costs := []providerCost{
		{ID: "a", PlatformModelCode: task.Model, BillingUnit: "PER_IMAGE", UnitCost: 0.60, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-01-01T00:00:00Z", CreatedAt: "2026-01-02T00:00:00Z", ParameterRange: map[string]any{"quality": map[string]any{"value": "high"}}},
		{ID: "b", PlatformModelCode: task.Model, BillingUnit: "PER_IMAGE", UnitCost: 0.70, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-01-01T00:00:00Z", CreatedAt: "2026-01-02T00:00:00Z", ParameterRange: map[string]any{"quality": map[string]any{"value": "high"}}},
	}
	got := selectProviderCost(costs, task, now)
	if got.Issue != providerCostIssueAmbiguous || !got.Found {
		t.Fatalf("selection = %#v, want ambiguous selection", got)
	}
}

func TestApplyTaskSupplierCostUsesCentsAndFreezesSnapshot(t *testing.T) {
	task := generationTask{ID: "task-1", Model: "gpt-image-2", Type: "IMAGE", PointCost: 10, Params: map[string]any{"quality": "standard"}}
	costs := []providerCost{{ID: "cost-v1", PlatformModelCode: task.Model, BillingUnit: "PER_IMAGE", UnitCost: 0.60, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-01-01T00:00:00Z"}}
	applyTaskSupplierCost(&task, costs)
	if task.SupplierCost == nil || *task.SupplierCost != 0.60 {
		t.Fatalf("supplier cost = %v, want 0.60 CNY", task.SupplierCost)
	}
	if task.UpstreamCost != 60 || task.PlatformProfit != -50 {
		t.Fatalf("cost/profit cents = %d/%d, want 60/-50", task.UpstreamCost, task.PlatformProfit)
	}

	updated := []providerCost{{ID: "cost-v2", PlatformModelCode: task.Model, BillingUnit: "PER_IMAGE", UnitCost: 0.90, Currency: "CNY", Status: "ACTIVE", EffectiveFrom: "2026-08-02T00:00:00Z"}}
	applyTaskSupplierCost(&task, updated)
	if *task.SupplierCost != 0.60 || task.UpstreamCost != 60 || task.PlatformProfit != -50 {
		t.Fatalf("snapshot changed after cost update: supplier=%v upstream=%d profit=%d", *task.SupplierCost, task.UpstreamCost, task.PlatformProfit)
	}
}

func TestReconciliationDetectsGenerationBillingFindings(t *testing.T) {
	task := generationTask{ID: "task-1", Model: "gpt-image-2", TaskStatus: taskStatusSucceeded, BillingStatus: billingStatusBillingFailed, QuotedPoints: 10, ReservedPoints: 10, CapturedPoints: 0, SupplierCost: nil}
	item := reconciliationItemForTask(task, nil, nil, []providerCost{})
	want := map[string]bool{"SUCCESS_WITHOUT_CAPTURE": true, "RESERVE_LEDGER_MISSING": true, "PROVIDER_SUCCESS_BILLING_FAILED": true, providerCostIssueMissing: true}
	for _, anomaly := range item.Anomalies {
		delete(want, anomaly)
	}
	for missing := range want {
		t.Errorf("missing anomaly %q in %#v", missing, item.Anomalies)
	}
}

func TestAssessMarginHealthUsesCentsAndNoImplicitTarget(t *testing.T) {
	status, issue, margin, rate, ok := assessMarginHealth(100, 60, nil)
	if !ok || status != marginHealthHealthy || issue != "" || margin != 40 || rate != 0.4 {
		t.Fatalf("positive margin = %v/%q/%d/%v/%v", status, issue, margin, rate, ok)
	}
	status, issue, margin, _, ok = assessMarginHealth(100, 120, nil)
	if !ok || status != marginHealthBlocked || issue != marginIssueNegative || margin != -20 {
		t.Fatalf("negative margin = %v/%q/%d/%v", status, issue, margin, ok)
	}
	status, issue, _, _, ok = assessMarginHealth(0, 1, nil)
	if ok || status != marginHealthBlocked || issue != marginIssueInvalid {
		t.Fatalf("invalid margin = %v/%q/%v", status, issue, ok)
	}
}
