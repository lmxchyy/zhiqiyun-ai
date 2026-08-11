package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestValidateV2MemberAgentSnapshotRejectsRightsProjectionDrift(t *testing.T) {
	memberSnapshot := func(rights map[string]any) virtualOrderSnapshot {
		return virtualOrderSnapshot{
			SnapshotVersion: 2, PlanID: "plan_member", PlanVersionID: "member-v1", PricePlanID: "member-normal",
			Currency: "CNY", ProductCode: "plan_member", ProductName: "member", ProductType: "MEMBERSHIP",
			PlanType: planTypeMemberPackage, AmountCents: 100, TransactionPriceCents: 100,
			WeChatGoodsPriceCents: 100, PaymentChannel: "WECHAT_VIRTUAL", PaymentEnvironment: "SANDBOX",
			MemberLevel: "PRO", MemberDays: 30, CreditUnits: 100, PointUnits: 13,
			BuyQuantity: 1, UnitPriceCents: 100, UnitCreditUnits: 100, WeChatProductID: "MEMBER_100",
			Rights: rights,
		}
	}
	validMember := memberSnapshot(map[string]any{
		"memberLevel": "PRO", "durationDays": json.Number("30"),
		"tokenAmount": float64(100), "pointsAmount": int64(13),
	})
	if err := validateV2MemberAgentSnapshot(validMember, 100); err != nil {
		t.Fatalf("equivalent JSON numeric representations were rejected: %v", err)
	}

	cases := []struct {
		name     string
		snapshot virtualOrderSnapshot
	}{
		{name: "member level", snapshot: memberSnapshot(map[string]any{"memberLevel": "BASIC", "durationDays": float64(30), "tokenAmount": float64(100), "pointsAmount": float64(13)})},
		{name: "duration", snapshot: memberSnapshot(map[string]any{"memberLevel": "PRO", "durationDays": float64(31), "tokenAmount": float64(100), "pointsAmount": float64(13)})},
		{name: "fractional duration", snapshot: memberSnapshot(map[string]any{"memberLevel": "PRO", "durationDays": json.Number("30.5"), "tokenAmount": float64(100), "pointsAmount": float64(13)})},
		{name: "token units", snapshot: memberSnapshot(map[string]any{"memberLevel": "PRO", "durationDays": float64(30), "tokenAmount": float64(101), "pointsAmount": float64(13)})},
		{name: "point units", snapshot: memberSnapshot(map[string]any{"memberLevel": "PRO", "durationDays": float64(30), "tokenAmount": float64(100), "pointsAmount": float64(14)})},
		{name: "agent level", snapshot: virtualOrderSnapshot{
			SnapshotVersion: 2, PlanID: "plan_agent", PlanVersionID: "agent-v1", PricePlanID: "agent-normal",
			Currency: "CNY", ProductCode: "plan_agent", ProductName: "agent", ProductType: "IDENTITY",
			PlanType: planTypeAgentJoinPackage, AmountCents: 100, TransactionPriceCents: 100,
			WeChatGoodsPriceCents: 100, PaymentChannel: "WECHAT_VIRTUAL", PaymentEnvironment: "SANDBOX",
			AgentLevel: "AGENT", MemberDays: 0, CreditUnits: 100, PointUnits: 0,
			BuyQuantity: 1, UnitPriceCents: 100, UnitCreditUnits: 100, WeChatProductID: "AGENT_100",
			Rights: map[string]any{"agentLevel": "PARTNER", "durationDays": float64(0), "tokenGrantAmount": json.Number("100"), "pointsAmount": float64(0)},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateV2MemberAgentSnapshot(tc.snapshot, 100); !errors.Is(err, errVirtualPaymentMismatch) {
				t.Fatalf("rights projection drift was accepted: %v", err)
			}
		})
	}
}

func TestResolvePricePlanOrdinaryEntryNeverSelectsTestPlan(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	plans := []pricePlanV2{
		{ID: "normal", PriceType: "NORMAL", SalePriceCents: 99600, IsDefault: true, IsVisible: true, Enabled: true, Status: "ACTIVE"},
		{ID: "test", PriceType: "TEST", SalePriceCents: 100, IsDefault: false, IsVisible: false, Enabled: true, Status: "ACTIVE"},
	}

	got, err := resolvePricePlanForEntry(plans, pricePlanEntryPublic, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "normal" || got.SalePriceCents != 99600 {
		t.Fatalf("ordinary entry selected wrong plan: %+v", got)
	}
}

func TestResolvePricePlanTestEntryRequiresWhitelistAndDedicatedEntry(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	plans := []pricePlanV2{
		{ID: "normal", PriceType: "NORMAL", SalePriceCents: 99600, IsDefault: true, IsVisible: true, Enabled: true, Status: "ACTIVE"},
		{ID: "test", PriceType: "TEST", SalePriceCents: 100, IsVisible: false, Enabled: true, Status: "ACTIVE"},
	}

	if _, err := resolvePricePlanForEntry(plans, pricePlanEntryTest, false, now); !errors.Is(err, errPricePlanWhitelistRequired) {
		t.Fatalf("non-whitelisted user was not rejected: %v", err)
	}
	got, err := resolvePricePlanForEntry(plans, pricePlanEntryTest, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "test" || got.SalePriceCents != 100 {
		t.Fatalf("dedicated test entry did not select the one-yuan plan: %+v", got)
	}
}

func TestValidatePricePlanPaymentChainRejectsOneCentAndEnvironmentMismatch(t *testing.T) {
	base := pricePaymentChain{
		QuotePriceCents: 100, PlanPriceCents: 100, BindingPriceCents: 100, WeChatGoodsPriceCents: 100,
		PlanChannel: "WECHAT_VIRTUAL", BindingChannel: "WECHAT_VIRTUAL", GoodsChannel: "WECHAT_VIRTUAL",
		PlanEnvironment: "SANDBOX", BindingEnvironment: "SANDBOX", GoodsEnvironment: "SANDBOX",
	}
	if err := validatePricePlanPaymentChain(base); err != nil {
		t.Fatal(err)
	}
	oneCent := base
	oneCent.WeChatGoodsPriceCents = 101
	if !errors.Is(validatePricePlanPaymentChain(oneCent), errPricePlanPriceMismatch) {
		t.Fatal("one-cent mismatch was accepted")
	}
	crossEnvironment := base
	crossEnvironment.GoodsEnvironment = "PRODUCTION"
	if !errors.Is(validatePricePlanPaymentChain(crossEnvironment), errPricePlanEnvironmentMismatch) {
		t.Fatal("production and sandbox goods were cross-bound")
	}
}

func TestPriceQuoteConsumeRejectsExpiredCrossUserAndConcurrentReuse(t *testing.T) {
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	if _, err := consumePriceQuote(&priceQuoteV2{UserID: "u1", Status: quoteStatusAvailable, ExpiresAt: now.Add(-time.Second)}, "u1", now); !errors.Is(err, errPriceQuoteExpired) {
		t.Fatalf("expired quote was not rejected: %v", err)
	}
	if _, err := consumePriceQuote(&priceQuoteV2{UserID: "u1", Status: quoteStatusAvailable, ExpiresAt: now.Add(time.Minute)}, "u2", now); !errors.Is(err, errPriceQuoteForbidden) {
		t.Fatalf("cross-user quote was not rejected: %v", err)
	}

	quote := &priceQuoteV2{UserID: "u1", Status: quoteStatusAvailable, ExpiresAt: now.Add(time.Minute)}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := consumePriceQuote(quote, "u1", now)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	reused := 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, errPriceQuoteConsumed) {
			reused++
		}
	}
	if successes != 1 || reused != 1 {
		t.Fatalf("concurrent quote consume results: success=%d reused=%d", successes, reused)
	}
}

func TestV2AgentSnapshotAtOneYuanDoesNotDependOnLegacy996Price(t *testing.T) {
	quote := resolvedPriceQuoteV2{
		PlanID: "plan_agent", PlanVersionID: "agent-v2", PricePlanID: "agent-test-1",
		PlanCode: "plan_agent", PlanName: "代理商", BusinessType: "AGENT", PriceType: "TEST",
		TransactionPriceCents: 100, WeChatGoodsPriceCents: 100, PlanChannel: "WECHAT_VIRTUAL",
		PlanEnvironment: "SANDBOX", OfferID: "offer", WeChatProductID: "AGENT_TEST_1", Mode: "short_series_goods",
		Rights:                map[string]any{"agentLevel": "AGENT", "tokenAmount": float64(20000), "pointsAmount": float64(0), "durationDays": float64(0)},
		CommissionRuleVersion: "commission-v2", CommissionSnapshot: map[string]any{"rules": []any{}},
	}
	snapshot, err := snapshotForResolvedPriceQuote(quote)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SnapshotVersion != 2 || snapshot.AmountCents != 100 || snapshot.UnitPriceCents != 100 || snapshot.AgentLevel != "AGENT" || snapshot.CreditUnits != 20000 {
		t.Fatalf("wrong V2 agent snapshot: %+v", snapshot)
	}
	if err := validateV2MemberAgentSnapshot(snapshot, 100); err != nil {
		t.Fatalf("valid one-yuan V2 agent snapshot was compared with legacy 996 price: %v", err)
	}
}

func TestPlanForV2AgentSnapshotUsesSnapshotTokenGrantInsteadOfLegacyDefault(t *testing.T) {
	snapshot := virtualOrderSnapshot{
		SnapshotVersion:       2,
		PlanID:                "plan_agent",
		PlanVersionID:         "agent-v2",
		PricePlanID:           "agent-promo",
		Currency:              "CNY",
		ProductCode:           "plan_agent",
		ProductName:           "agent",
		ProductType:           "IDENTITY",
		PlanType:              planTypeAgentJoinPackage,
		AmountCents:           100,
		TransactionPriceCents: 100,
		WeChatGoodsPriceCents: 100,
		PaymentChannel:        "WECHAT_VIRTUAL",
		PaymentEnvironment:    "SANDBOX",
		AgentLevel:            "AGENT",
		CreditUnits:           20004,
		PointUnits:            13,
		BuyQuantity:           1,
		UnitPriceCents:        100,
		UnitCreditUnits:       20004,
		WeChatProductID:       "AGENT_PROMO",
		Rights: map[string]any{
			"agentLevel": "AGENT", "tokenAmount": float64(20004),
			"pointsAmount": float64(13), "durationDays": float64(0),
		},
	}
	plan, err := planForV2Snapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := planTokenGrantAmount(plan); got != 20004 {
		t.Fatalf("V2 agent token grant=%d, want immutable snapshot value 20004", got)
	}
	if got := planPoints(plan); got != 13 {
		t.Fatalf("V2 agent points projection=%d, want immutable snapshot value 13", got)
	}
}

func TestV2SnapshotKeepsOriginalRightsAfterDefaultPriceChanges(t *testing.T) {
	quote := resolvedPriceQuoteV2{
		PlanID: "plan_member", PlanVersionID: "member-v1", PricePlanID: "member-normal-old",
		PlanCode: "plan_member", PlanName: "会员", BusinessType: "MEMBER", PriceType: "NORMAL",
		TransactionPriceCents: 99600, WeChatGoodsPriceCents: 99600, PlanChannel: "WECHAT_VIRTUAL",
		PlanEnvironment: "PRODUCTION", OfferID: "offer", WeChatProductID: "MEMBER_996", Mode: "short_series_goods",
		Rights: map[string]any{"memberLevel": "PRO", "tokenAmount": float64(40000), "pointsAmount": float64(0), "durationDays": float64(365)},
	}
	snapshot, err := snapshotForResolvedPriceQuote(quote)
	if err != nil {
		t.Fatal(err)
	}
	// A newly selected default may be 698 yuan, but fulfillment validates the immutable order amount.
	newDefaultPrice := int64(69800)
	if newDefaultPrice == snapshot.AmountCents {
		t.Fatal("test fixture does not model a default-price change")
	}
	if err := validateV2MemberAgentSnapshot(snapshot, 99600); err != nil {
		t.Fatalf("historical snapshot was affected by the new default: %v", err)
	}
}

func TestPricePlanBonusRightsAndCurrencyAreImmutableSnapshotInputs(t *testing.T) {
	rights := map[string]any{
		"memberLevel": "PRO", "durationDays": float64(30),
		"tokenAmount": float64(100), "pointsAmount": float64(10),
	}
	if err := mergePricePlanBonusRights(rights, 30, 40); err != nil {
		t.Fatal(err)
	}
	if int64Value(rights["baseTokenAmount"]) != 100 || int64Value(rights["basePointsAmount"]) != 10 ||
		int64Value(rights["pricePlanGiftTokens"]) != 40 || int64Value(rights["pricePlanGiftPoints"]) != 30 ||
		int64Value(rights["tokenAmount"]) != 140 || int64Value(rights["tokenGrantAmount"]) != 140 ||
		int64Value(rights["pointsAmount"]) != 40 {
		t.Fatalf("price plan gifts were not merged exactly once: %+v", rights)
	}
	quote := resolvedPriceQuoteV2{
		PlanID: "plan_member", PlanVersionID: "member-v1", PricePlanID: "member-promo",
		PlanCode: "plan_member", PlanName: "member", BusinessType: "MEMBER", PriceType: "ACTIVITY",
		Currency: "CNY", BonusPoints: 30, BonusTokens: 40,
		TransactionPriceCents: 69800, WeChatGoodsPriceCents: 69800, PlanChannel: "WECHAT_VIRTUAL",
		PlanEnvironment: "PRODUCTION", OfferID: "offer", WeChatProductID: "MEMBER_PROMO", Mode: "short_series_goods",
		Rights: rights,
	}
	snapshot, err := snapshotForResolvedPriceQuote(quote)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Currency != "CNY" || snapshot.PricePlanGiftPoints != 30 || snapshot.PricePlanGiftTokens != 40 ||
		snapshot.CreditUnits != 140 || snapshot.PointUnits != 40 {
		t.Fatalf("price plan gift snapshot is incomplete: %+v", snapshot)
	}

	configuration := resolvedPriceQuoteV2{
		DBID: "quote", PricePlanID: "price", PaymentBindingID: "binding", WeChatGoodID: "good",
		BoundPricePlanID: "price", BoundWeChatGoodID: "good",
		QuotedBindingPriceCents: 100, BindingPriceCents: 100, QuotedGoodsPriceCents: 100, WeChatGoodsPriceCents: 100,
		QuotedChannel: "WECHAT_VIRTUAL", PlanChannel: "WECHAT_VIRTUAL", BindingChannel: "WECHAT_VIRTUAL", GoodsChannel: "WECHAT_VIRTUAL",
		QuotedEnvironment: "PRODUCTION", PlanEnvironment: "PRODUCTION", BindingEnvironment: "PRODUCTION", GoodsEnvironment: "PRODUCTION",
		QuotedOfferID: "offer", OfferID: "offer", QuotedWeChatProductID: "product", WeChatProductID: "product", QuotedMode: "mode", Mode: "mode",
		QuotedCurrency: "CNY", Currency: "CNY", QuotedBonusPoints: 30, BonusPoints: 30, QuotedBonusTokens: 40, BonusTokens: 40,
		QuotedRights:                map[string]any{"tokenAmount": float64(140), "pointsAmount": float64(40)},
		Rights:                      map[string]any{"tokenAmount": float64(140), "pointsAmount": float64(40)},
		QuotedCommissionRuleVersion: "commission-v1", CommissionRuleVersion: "commission-v1",
		QuotedCommissionSnapshot: map[string]any{"rules": []any{}},
		CommissionSnapshot:       map[string]any{"rules": []any{}},
	}
	if err := validatePriceQuoteConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	configuration.BonusTokens++
	if !errors.Is(validatePriceQuoteConfiguration(configuration), errPriceQuoteConfigurationChanged) {
		t.Fatal("gift token drift did not invalidate the quote")
	}
	configuration.BonusTokens--
	configuration.Currency = "USD"
	if !errors.Is(validatePriceQuoteConfiguration(configuration), errPriceQuoteConfigurationChanged) {
		t.Fatal("currency drift did not invalidate the quote")
	}
	configuration.Currency = "CNY"
	configuration.Rights["tokenAmount"] = float64(141)
	if !errors.Is(validatePriceQuoteConfiguration(configuration), errPriceQuoteConfigurationChanged) {
		t.Fatal("entitlement drift did not invalidate the quote")
	}
	configuration.Rights["tokenAmount"] = float64(140)
	configuration.CommissionRuleVersion = "commission-v2"
	if !errors.Is(validatePriceQuoteConfiguration(configuration), errPriceQuoteConfigurationChanged) {
		t.Fatal("commission rule drift did not invalidate the quote")
	}

	if err := mergePricePlanBonusRights(map[string]any{"tokenAmount": int64(math.MaxInt64)}, 0, 1); err == nil {
		t.Fatal("gift token overflow was accepted")
	}
}

func TestSnapshotForResolvedPriceQuoteUsesPersistedQuotedEntitlements(t *testing.T) {
	quote := resolvedPriceQuoteV2{
		PlanID: "plan_member", PlanVersionID: "member-v1", PricePlanID: "member-normal",
		PlanCode: "plan_member", PlanName: "member", BusinessType: "MEMBER", PriceType: "NORMAL",
		Currency: "CNY", TransactionPriceCents: 100, WeChatGoodsPriceCents: 100,
		PlanChannel: "WECHAT_VIRTUAL", PlanEnvironment: "PRODUCTION", OfferID: "offer",
		WeChatProductID: "MEMBER_100", Mode: "short_series_goods",
		QuotedRights: map[string]any{
			"memberLevel": "PRO", "tokenAmount": float64(110), "pointsAmount": float64(0), "durationDays": float64(30),
		},
		Rights: map[string]any{
			"memberLevel": "PRO", "tokenAmount": float64(999), "pointsAmount": float64(0), "durationDays": float64(30),
		},
		QuotedCommissionRuleVersion: "commission-quoted",
		CommissionRuleVersion:       "commission-current",
		QuotedCommissionSnapshot:    map[string]any{"rules": []any{}},
		CommissionSnapshot:          map[string]any{"rules": []any{"current"}},
	}
	snapshot, err := snapshotForResolvedPriceQuote(quote)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CreditUnits != 110 || snapshot.CommissionRuleVersion != "commission-quoted" ||
		len(snapshot.CommissionRules) != 0 {
		t.Fatalf("order snapshot did not use persisted quote entitlements: %+v", snapshot)
	}
}

func TestSnapshotForResolvedPriceQuoteRejectsIncompleteCommissionSnapshotBeforeSigning(t *testing.T) {
	baseQuote := resolvedPriceQuoteV2{
		PlanID: "plan_member", PlanVersionID: "member-v1", PricePlanID: "member-normal",
		PlanCode: "plan_member", PlanName: "member", BusinessType: "MEMBER", PriceType: "NORMAL",
		Currency: "CNY", TransactionPriceCents: 100, WeChatGoodsPriceCents: 100,
		PlanChannel: "WECHAT_VIRTUAL", PlanEnvironment: "SANDBOX", OfferID: "offer",
		WeChatProductID: "MEMBER_100", Mode: "short_series_goods",
		QuotedRights: map[string]any{
			"memberLevel": "PRO", "tokenAmount": float64(100), "pointsAmount": float64(0), "durationDays": float64(30),
		},
		QuotedCommissionRuleVersion: "commission-v1",
	}

	t.Run("malformed rules payload", func(t *testing.T) {
		quote := baseQuote
		quote.QuotedCommissionSnapshot = map[string]any{"rules": "not-an-array"}
		if _, err := snapshotForResolvedPriceQuote(quote); !errors.Is(err, errCommissionRuleSnapshotIncomplete) {
			t.Fatalf("malformed commission snapshot reached signing path: %v", err)
		}
	})

	t.Run("tiered rule missing calculation config", func(t *testing.T) {
		quote := baseQuote
		quote.QuotedCommissionSnapshot = map[string]any{"rules": []any{map[string]any{
			"id": "tiered-v1", "code": "TIERED_V1", "name": "tiered", "version": 1,
			"beneficiaryRole": "AGENT", "relationshipLevel": 1, "calculationType": "TIERED",
			"fixedAmountCents": 0, "percentageBps": 0, "freezeDays": 0,
			"refundPolicy": "REVERSE_OR_RECOVER",
		}}}
		if _, err := snapshotForResolvedPriceQuote(quote); !errors.Is(err, errCommissionRuleSnapshotIncomplete) {
			t.Fatalf("incomplete tiered commission snapshot reached signing path: %v", err)
		}
	})

	t.Run("top-level rules must match normalized commission snapshot", func(t *testing.T) {
		quote := baseQuote
		quote.QuotedCommissionSnapshot = map[string]any{"rules": []any{}}
		snapshot, err := snapshotForResolvedPriceQuote(quote)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.CommissionRules = []commissionRuleSnapshot{{
			ID: "injected", Code: "INJECTED", Name: "injected", Version: 1,
			BeneficiaryRole: "PLATFORM", CalculationType: "REMAINDER_TO_PLATFORM",
			RefundPolicy: "REVERSE_OR_RECOVER",
		}}
		if err := validateV2MemberAgentSnapshot(snapshot, 100); !errors.Is(err, errCommissionRuleSnapshotIncomplete) {
			t.Fatalf("divergent top-level commission rules were accepted: %v", err)
		}
	})
}

func TestLockedVirtualOrderUsesDatabaseSnapshotVersionAndCrossChecksV2Columns(t *testing.T) {
	rights := map[string]any{
		"memberLevel": "PRO", "tokenAmount": float64(100), "pointsAmount": float64(0), "durationDays": float64(30),
	}
	commission := map[string]any{"rules": []any{}}
	order := lockedVirtualOrder{
		PlanID: "plan_member", ProductCode: "plan_member", AmountCents: 100,
		StoredSnapshotVersion:       2,
		StoredPlanVersionID:         "member-v1",
		StoredPricePlanID:           "member-normal",
		StoredTransactionPriceCents: 100,
		StoredWeChatProductID:       "MEMBER_100",
		StoredWeChatGoodsPriceCents: 100,
		StoredCurrency:              "CNY",
		StoredPaymentChannel:        "WECHAT_VIRTUAL",
		StoredPaymentEnvironment:    "PRODUCTION",
		StoredRights:                rights,
		StoredCommissionRuleVersion: "commission-v1",
		StoredCommissionSnapshot:    commission,
		Snapshot: virtualOrderSnapshot{
			SnapshotVersion: 2, PlanID: "plan_member", PlanVersionID: "member-v1", PricePlanID: "member-normal",
			Currency: "CNY", ProductCode: "plan_member", ProductName: "member", ProductType: "MEMBERSHIP",
			PlanType: planTypeMemberPackage, AmountCents: 100, TransactionPriceCents: 100,
			WeChatGoodsPriceCents: 100, PaymentChannel: "WECHAT_VIRTUAL", PaymentEnvironment: "PRODUCTION",
			MemberLevel: "PRO", MemberDays: 30, CreditUnits: 100, BuyQuantity: 1, UnitPriceCents: 100,
			UnitCreditUnits: 100, WeChatProductID: "MEMBER_100", Rights: rights,
			CommissionRuleVersion: "commission-v1", CommissionSnapshotV2: commission,
		},
	}
	if !order.isV2() {
		t.Fatal("database snapshot_version=2 was not treated as V2")
	}
	if err := validateLockedVirtualOrderSnapshot(order); err != nil {
		t.Fatalf("matching V2 normalized columns were rejected: %v", err)
	}

	order.Snapshot.SnapshotVersion = 0
	if err := validateLockedVirtualOrderSnapshot(order); !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("database/JSON snapshot version mismatch was accepted: %v", err)
	}
	order.StoredSnapshotVersion = 0
	order.Snapshot.SnapshotVersion = 2
	if order.isV2() {
		t.Fatal("JSON snapshotVersion overrode the database V1/V2 discriminator")
	}
	if err := validateLockedVirtualOrderSnapshot(order); !errors.Is(err, errVirtualPaymentMismatch) {
		t.Fatalf("V1 database row with V2 JSON was accepted: %v", err)
	}
}

func TestV132PricePlanSettlementDecisionFailsClosedBeforeSigning(t *testing.T) {
	if err := validateV132PricePlanSettlementDecision(orderSettlementDecision{SettlementEngine: settlementEngineLegacy}); err != nil {
		t.Fatalf("LEGACY V2 settlement was rejected: %v", err)
	}
	err := validateV132PricePlanSettlementDecision(orderSettlementDecision{
		TenantID: "tenant-v132", PlanID: "plan_member", SettlementEngine: settlementEngineV132,
		RuleSetID: "rules-v132", RuleSetVersion: 1,
	})
	if !errors.Is(err, errPricePlanV132SnapshotIncompatible) {
		t.Fatalf("V132 V2 settlement was not rejected before signing: %v", err)
	}
}

func TestWritePricePlanErrorMapsV132SnapshotMismatchToUnprocessableEntity(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePricePlanError(recorder, fmt.Errorf("%w: pinned price differs", errPricePlanV132SnapshotIncompatible))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "PRICE_PLAN_SETTLEMENT_CONFIGURATION_MISMATCH" {
		t.Fatalf("code=%v body=%s", payload["code"], recorder.Body.String())
	}
}

func TestWritePricePlanErrorMapsIncompleteCommissionSnapshotToUnprocessableEntity(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePricePlanError(recorder, fmt.Errorf("%w: tiered rule is incomplete", errCommissionRuleSnapshotIncomplete))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["code"] != "PRICE_PLAN_COMMISSION_SNAPSHOT_INVALID" {
		t.Fatalf("code=%v body=%s", payload["code"], recorder.Body.String())
	}
}

func TestWritePricePlanErrorMapsTestEligibilityAndGiftPointsFailures(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "test whitelist is not eligible",
			err:        fmt.Errorf("%w: pinned whitelist is disabled", errPricePlanNotEligible),
			wantStatus: http.StatusForbidden,
			wantCode:   "PRICE_PLAN_NOT_ELIGIBLE",
		},
		{
			name:       "gift points fulfillment is unavailable",
			err:        fmt.Errorf("%w: giftPoints=1", errPricePlanGiftPointsFulfillmentUnavailable),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writePricePlanError(recorder, test.err)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s want=%d", recorder.Code, recorder.Body.String(), test.wantStatus)
			}
			var payload map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload["code"] != test.wantCode {
				t.Fatalf("code=%v body=%s want=%s", payload["code"], recorder.Body.String(), test.wantCode)
			}
		})
	}
}

func TestValidatePricePlanRuntimeFulfillmentFailsClosedForGiftPoints(t *testing.T) {
	if err := validatePricePlanRuntimeFulfillment(resolvedPriceQuoteV2{BonusPoints: 0}); err != nil {
		t.Fatalf("zero gift points were rejected: %v", err)
	}
	if err := validatePricePlanRuntimeFulfillment(resolvedPriceQuoteV2{BonusPoints: 1}); !errors.Is(err, errPricePlanGiftPointsFulfillmentUnavailable) {
		t.Fatalf("positive gift points were not rejected: %v", err)
	}
}
