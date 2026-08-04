package httpserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaidPersonalPointInflowsDoNotWriteLegacyBalances(t *testing.T) {
	t.Parallel()

	targets := map[string][]string{
		filepath.Join("..", "app", "payment", "service.go"): {"grantTokenTx"},
		"postgres_store.go": {
			"applyRechargeSettlementForTx",
			"grantTokensToUserTx",
		},
		"commercial_billing_wechat.go":   {"grantVirtualCouponCreditsTx"},
		"wechat_virtual_entitlements.go": {"grantCreationCreditsTx"},
	}
	for fileName, functions := range targets {
		fileName, functions := fileName, functions
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(".", fileName)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			set := token.NewFileSet()
			parsed, err := parser.ParseFile(set, path, source, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, functionName := range functions {
				body := functionSource(t, set, parsed, source, functionName)
				for _, forbidden := range []string{"account.Available +=", "setAdminPointAccountWithLedgerV1", "upsertPointAccountByUser", "UPDATE xz_point_accounts", "INSERT INTO xz_user_wallets"} {
					if strings.Contains(body, forbidden) {
						t.Errorf("%s still bypasses personal point lots with %q", functionName, forbidden)
					}
				}
				if !strings.Contains(body, "grantPermanentPersonalPointsTx") && !strings.Contains(body, "personalPointGrant(") {
					t.Errorf("%s does not grant through the caller-owned personal point transaction", functionName)
				}
			}
		})
	}
}

func TestPersonalPointIntegrationStaticBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("lot v1 generation never falls back to legacy balance mutation", func(t *testing.T) {
		t.Parallel()
		for _, target := range []struct {
			path      string
			functions []string
		}{
			{path: "store.go", functions: []string{"CreateGenerationTask", "CreatePendingGenerationTask"}},
			{path: "postgres_store.go", functions: []string{"CreateGenerationTask", "CreatePendingGenerationTask"}},
		} {
			for _, functionName := range target.functions {
				body := productionFunctionBody(t, target.path, functionName)
				for _, forbidden := range []string{"setAdminPointAccountWithLedgerV1", "upsertPointAccountByUser", "UPDATE xz_point_accounts"} {
					if strings.Contains(body, forbidden) {
						t.Errorf("%s.%s still has legacy personal balance fallback %q", target.path, functionName, forbidden)
					}
				}
				if !strings.Contains(body, "personalLotBillingEngine") || (!strings.Contains(body, "reserveTx(") && !strings.Contains(body, "points.reserve(")) {
					t.Errorf("%s.%s does not reserve through PERSONAL_LOT_V1", target.path, functionName)
				}
			}
		}
	})

	t.Run("json sidecar is read only after primary import", func(t *testing.T) {
		t.Parallel()
		body := productionFunctionBody(t, "personal_points_json.go", "preparePersonalPoints")
		if !strings.Contains(body, "os.ReadFile(sidecarPath)") {
			t.Fatal("preparePersonalPoints no longer reads the import sidecar")
		}
		for _, forbidden := range []string{"os.WriteFile(", "saveLocked(", "writeState("} {
			if strings.Contains(body, forbidden) {
				t.Errorf("preparePersonalPoints writes the read-only sidecar via %q", forbidden)
			}
		}
	})

	t.Run("payment grant hook is fail closed", func(t *testing.T) {
		t.Parallel()
		body := productionFunctionBody(t, filepath.Join("..", "app", "payment", "service.go"), "grantTokenTx")
		if !strings.Contains(body, "personalPointGrant == nil") || !strings.Contains(body, "ErrPersonalPointGrantHookUnavailable") {
			t.Fatal("grantTokenTx must reject a missing personal point grant hook")
		}
	})

	t.Run("enterprise credits never enter personal lots", func(t *testing.T) {
		t.Parallel()
		body := productionFunctionBody(t, "enterprise_runtime.go", "consumeEnterpriseCreditLotsTx")
		if !strings.Contains(body, "xz_compute_credit_lots") {
			t.Fatal("enterprise credit consumption no longer uses enterprise credit lots")
		}
		for _, forbidden := range []string{"xz_point_lots", "NewPostgresPersonalPointStore", "personalLotBillingEngine"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("enterprise credit consumption crossed into personal lots via %q", forbidden)
			}
		}
	})
}

func productionFunctionBody(t *testing.T, path, functionName string) string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	parsed, err := parser.ParseFile(set, path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	return functionSource(t, set, parsed, source, functionName)
}

func TestAdminDefaultsDoNotSeedDemoPersonalPoints(t *testing.T) {
	t.Parallel()
	data := withAdminDefaults(adminPlatformData{})
	for _, account := range data.PointAccounts {
		if account.UserID == "user_000002" {
			if account.Available != 0 {
				t.Fatalf("demo personal point balance = %d, want zero until an actual plan or grant applies", account.Available)
			}
			return
		}
	}
	t.Fatal("demo personal point account is missing")
}

func functionSource(t *testing.T, set *token.FileSet, parsed *ast.File, source []byte, name string) string {
	t.Helper()
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != name {
			continue
		}
		start := set.Position(function.Pos()).Offset
		end := set.Position(function.End()).Offset
		return string(source[start:end])
	}
	t.Fatalf("function %s was not found", name)
	return ""
}

func TestJSONPaidOrderGrantsCreatePermanentLots(t *testing.T) {
	t.Parallel()
	now := "2026-08-04T00:00:00Z"
	data := adminPlatformData{
		Users:         []adminUser{{ID: "paid-user"}},
		PointAccounts: []adminPointAccount{{ID: "paid-account", UserID: "paid-user"}},
		PersonalPoints: personalPointState{
			Accounts: []PersonalPointAccount{{ID: "paid-account", UserID: "paid-user"}},
		},
	}
	if err := grantTokensToUser(&data, "paid-user", "commerce-order", "MEMBER_PACKAGE_GRANT", 8, now); err != nil {
		t.Fatal(err)
	}
	assertSinglePermanentPaidLot(t, data.PersonalPoints, PointSourceMemberPackageGrant, 8)

	recharge := adminPlatformData{
		Users:         []adminUser{{ID: "recharge-user"}},
		PointAccounts: []adminPointAccount{{ID: "recharge-account", UserID: "recharge-user"}},
		PersonalPoints: personalPointState{
			Accounts: []PersonalPointAccount{{ID: "recharge-account", UserID: "recharge-user"}},
		},
	}
	order := adminOrder{
		ID: "recharge-order", UserID: "recharge-user", PlanID: "PLAN_RECHARGE", PaidAt: now,
		PriceSnapshot: map[string]any{"orderType": "COMPUTE_RECHARGE", "rechargePoints": 11},
	}
	applyRechargeSettlement(&recharge, &order, now)
	assertSinglePermanentPaidLot(t, recharge.PersonalPoints, PointSourceRecharge, 11)
}

func assertSinglePermanentPaidLot(t *testing.T, state personalPointState, source PointSource, points int64) {
	t.Helper()
	if len(state.Lots) != 1 {
		t.Fatalf("expected one lot, got %#v", state.Lots)
	}
	lot := state.Lots[0]
	if lot.SourceType != source || lot.OriginalPoints != points || !lot.Permanent() {
		t.Fatalf("unexpected paid lot: %#v", lot)
	}
	if err := validatePersonalPointState(&state); err != nil {
		t.Fatalf("paid lot state is invalid: %v", err)
	}
}
