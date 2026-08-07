package httpserver

import (
	"math"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestGenerationBillingEventPersistsOneCentPerPoint(t *testing.T) {
	store := newBillingAcceptanceStore(t)
	task, err := store.CreatePendingGenerationTask(generationBillingTestRequest(billingAcceptanceUserID, 3))
	if err != nil {
		t.Fatalf("create pending generation task: %v", err)
	}
	if _, err := store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{}); err != nil {
		t.Fatalf("complete generation task: %v", err)
	}
	data, err := store.AdminData()
	if err != nil {
		t.Fatalf("read persisted billing events: %v", err)
	}
	for _, event := range data.BillingEvents {
		if event.TaskID != task.ID {
			continue
		}
		if event.UnitAmountCents != 1 || event.AmountCents != 3 || event.PointCost != 3 {
			t.Fatalf("persisted usage billing event = unit:%d amount:%d points:%d, want 1/3/3", event.UnitAmountCents, event.AmountCents, event.PointCost)
		}
		return
	}
	t.Fatalf("persisted billing event not found for task %s", task.ID)
}

func TestPPTBillingEventUsesOneCentPerPoint(t *testing.T) {
	event := pptBillingEvent(pptapp.Task{TaskID: "ppt-point-unit", UserID: "ppt-user", SlideCount: 3}, 3, 8, 5, "2026-08-07T00:00:00Z", adminUser{}, adminChannelAgent{}, false)
	if event.UnitAmountCents != 1 || event.AmountCents != 3 || event.PointCost != 3 {
		t.Fatalf("ppt usage billing event = unit:%d amount:%d points:%d, want 1/3/3", event.UnitAmountCents, event.AmountCents, event.PointCost)
	}
}

func TestBillingV1SupplierMarginUsesOneCentPerPoint(t *testing.T) {
	task := generationTask{
		PointCost: 3,
		Model:     "ppt-model",
		Params:    map[string]any{"page_count": 1},
	}
	applyTaskSupplierCost(&task, []providerCost{{
		PlatformModelCode: "ppt-model",
		BillingUnit:       "PER_PAGE",
		UnitCost:          0.02,
		Status:            "ACTIVE",
	}})
	if task.SupplierCost == nil || task.EstimatedMargin == nil {
		t.Fatalf("supplier cost or margin was not calculated: %+v", task)
	}
	if math.Abs(*task.SupplierCost-0.02) > 0.000001 || math.Abs(*task.EstimatedMargin-0.01) > 0.000001 {
		t.Fatalf("supplier cost/margin = %.2f/%.2f, want 0.02/0.01", *task.SupplierCost, *task.EstimatedMargin)
	}
}

func TestBillingViewsValueEachPointAtOneCent(t *testing.T) {
	data := adminPlatformData{
		Users: []adminUser{{
			ID:     "point-unit-user",
			Name:   "Point Unit User",
			Email:  "point-unit@example.test",
			Role:   "MEMBER",
			Status: "ACTIVE",
		}},
		PointAccounts: []adminPointAccount{{
			ID:        "points-point-unit-user",
			UserID:    "point-unit-user",
			Available: 1,
			Frozen:    1,
		}},
	}

	customer := billingCustomerRows(data)[0]
	if got := intValue(customer["prepaidBalanceCents"]); got != 1 {
		t.Fatalf("customer prepaid balance cents = %d, want 1 for one point", got)
	}

	subscription := billingSubscriptionRows(data)[0]
	if got := intValue(subscription["prepaidBalanceCents"]); got != 1 {
		t.Fatalf("subscription prepaid balance cents = %d, want 1 for one point", got)
	}
	if got := intValue(subscription["lifetimeUsageCents"]); got != 1 {
		t.Fatalf("subscription lifetime usage cents = %d, want 1 for one point", got)
	}

	wallet := billingWalletRows(data)[0]
	if got := wallet["rateAmount"]; got != 0.01 {
		t.Fatalf("wallet point rate = %#v, want 0.01 CNY", got)
	}
	if got := intValue(wallet["balanceCents"]); got != 1 {
		t.Fatalf("wallet balance cents = %d, want 1 for one point", got)
	}
	if got := intValue(wallet["consumedAmountCents"]); got != 1 {
		t.Fatalf("wallet consumed cents = %d, want 1 for one point", got)
	}
}

func TestJSONRechargeBillingEventUsesPointUnitButPreservesCashOrderAmount(t *testing.T) {
	data := adminPlatformData{
		Users:         []adminUser{{ID: "point-unit-user", Role: "MEMBER", Status: "ACTIVE"}},
		PointAccounts: []adminPointAccount{{ID: "points-point-unit-user", UserID: "point-unit-user"}},
	}
	order := adminOrder{
		ID:          "order-point-unit-recharge",
		UserID:      "point-unit-user",
		PlanID:      "recharge_100",
		AmountCents: 10000,
		PriceSnapshot: map[string]any{
			"rechargePoints": 10000,
		},
	}

	applyRechargeSettlement(&data, &order, "2026-08-07T00:00:00Z")
	if len(data.BillingEvents) != 1 {
		t.Fatalf("recharge billing event count = %d, want 1", len(data.BillingEvents))
	}
	event := data.BillingEvents[0]
	if event.UnitAmountCents != 1 {
		t.Fatalf("recharge unit amount cents = %d, want 1", event.UnitAmountCents)
	}
	if event.AmountCents != 10000 {
		t.Fatalf("recharge amount cents = %d, want original cash order amount 10000", event.AmountCents)
	}
}

func TestJSONNonCatalogRechargeSettlementDerivesOnePointPerCent(t *testing.T) {
	data := adminPlatformData{
		Users:         []adminUser{{ID: "point-unit-user", Role: "MEMBER", Status: "ACTIVE"}},
		PointAccounts: []adminPointAccount{{ID: "points-point-unit-user", UserID: "point-unit-user"}},
	}
	order := adminOrder{
		ID:          "order-point-unit-derived-recharge",
		UserID:      "point-unit-user",
		PlanID:      "manual_recharge_10000",
		AmountCents: 10000,
		PriceSnapshot: map[string]any{
			"orderType": "COMPUTE_RECHARGE",
		},
	}

	applyRechargeSettlement(&data, &order, "2026-08-07T00:00:00Z")
	if len(data.BillingEvents) != 1 {
		t.Fatalf("derived recharge billing event count = %d, want 1", len(data.BillingEvents))
	}
	event := data.BillingEvents[0]
	if event.Quantity != 10000 || event.PointCost != -10000 {
		t.Fatalf("derived recharge points = quantity:%d pointCost:%d, want 10000/-10000", event.Quantity, event.PointCost)
	}
	if got := intValue(order.PriceSnapshot["rechargePoints"]); got != 10000 {
		t.Fatalf("derived recharge snapshot points = %d, want 10000", got)
	}
}
