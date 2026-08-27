package httpserver

import "testing"

func TestPointEconomicsUsesOneCnyCentPerPoint(t *testing.T) {
	for _, points := range []int{1, 10, 100} {
		t.Run(testPointCaseName(points), func(t *testing.T) {
			got := generationBillingEvent(
				generationTask{ID: "task-point-economics", UserID: "user-1", Type: "TEXT_TO_IMAGE", Model: "gpt-image-2", PointCost: points},
				0, 0, "2026-08-27T00:00:00Z", adminUser{ID: "user-1"}, adminChannelAgent{}, false,
			)
			if got.UnitAmountCents != 1 {
				t.Fatalf("unit amount cents = %d, want 1 for 1 point = 1 CNY cent", got.UnitAmountCents)
			}
			if got.AmountCents != points {
				t.Fatalf("amount cents = %d, want %d", got.AmountCents, points)
			}
		})
	}
}

func testPointCaseName(points int) string {
	return map[int]string{1: "one_point", 10: "ten_points", 100: "one_hundred_points"}[points]
}

func TestPointEconomicsSnapshotUsesCorrectRevenueForMargin(t *testing.T) {
	task := generationTask{ID: "task-margin", UserID: "user-1", Type: "TEXT_TO_IMAGE", Model: "gpt-image-2", PointCost: 100}
	applyGenerationTaskCapabilitySnapshot(&task, createGenerationTaskRequest{Type: task.Type, Model: task.Model, Params: map[string]any{}}, adminBillingRule{ID: "rule", ModuleCode: moduleImageGeneration, ModelName: task.Model, BillingType: "per_image", CostPrice: 60})
	if task.UserChargeAmount != 100 {
		t.Fatalf("user charge cents = %d, want 100", task.UserChargeAmount)
	}
	if task.UpstreamCost != 60 || task.PlatformProfit != 40 {
		t.Fatalf("cost/profit cents = %d/%d, want 60/40", task.UpstreamCost, task.PlatformProfit)
	}
}

func TestPointEconomicsHandlesFourYuanProviderCost(t *testing.T) {
	task := generationTask{
		ID:        "task-seedance-margin",
		UserID:    "user-1",
		Type:      "TEXT_TO_VIDEO",
		Model:     "seedance-fast-2.0",
		PointCost: 600,
		Params:    map[string]any{"duration": 5, "resolution": "720p"},
	}
	costs := []providerCost{{
		ID:                "seedance-cost",
		PlatformModelCode: "seedance-fast-2.0",
		BillingUnit:       "PER_SECOND",
		UnitCost:          0.80,
		Currency:          "CNY",
		Status:            "ACTIVE",
		EffectiveFrom:     "2026-01-01T00:00:00Z",
	}}
	applyTaskSupplierCost(&task, costs)
	if task.SupplierCost == nil || *task.SupplierCost != 4.00 {
		t.Fatalf("supplier cost = %v, want 4.00 CNY", task.SupplierCost)
	}
	if task.UpstreamCost != 400 || task.PlatformProfit != 200 {
		t.Fatalf("cost/profit cents = %d/%d, want 400/200", task.UpstreamCost, task.PlatformProfit)
	}
}
