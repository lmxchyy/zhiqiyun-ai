package httpserver

import "testing"

func TestMineRechargePackagesMatchProductContract(t *testing.T) {
	tests := []struct {
		id          string
		amountCents int
		points      int
	}{
		{id: "recharge_100", amountCents: 10000, points: 10000},
		{id: "recharge_400", amountCents: 40000, points: 40000},
	}

	for _, test := range tests {
		plan, ok := planCatalogByID(test.id)
		if !ok {
			t.Fatalf("plan %s not found", test.id)
		}
		if got := planPrice(plan); got != test.amountCents {
			t.Fatalf("plan %s price = %d, want %d", test.id, got, test.amountCents)
		}
		if got := planPoints(plan); got != test.points {
			t.Fatalf("plan %s points = %d, want %d", test.id, got, test.points)
		}
		if got := rechargePackageIDForAmount(test.amountCents); got != test.id {
			t.Fatalf("amount %d maps to %s, want %s", test.amountCents, got, test.id)
		}
	}
}
