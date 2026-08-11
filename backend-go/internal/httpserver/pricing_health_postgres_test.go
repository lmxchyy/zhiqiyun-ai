package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPricingHealthHTTPContractsAndInfrastructureErrors(t *testing.T) {
	cfg := config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}
	handler := newWithStoreAndSessions(cfg, newJSONStore(""), newLocalAuthSessions()).Handler

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-health", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("pricing health unauthenticated status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	requirePhase2ERBACError(t, unauthorized, "ADMIN_AUTHENTICATION_REQUIRED")

	public := httptest.NewRecorder()
	handler.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if public.Code != http.StatusOK || public.Body.String() != "{\"service\":\"xianzhi-ai-go-gin\",\"status\":\"ok\"}\n" {
		t.Fatalf("public health contract changed: status=%d body=%q", public.Code, public.Body.String())
	}
	healthz := httptest.NewRecorder()
	handler.ServeHTTP(healthz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthz.Code != http.StatusOK || healthz.Body.String() != public.Body.String() {
		t.Fatalf("healthz contract changed: status=%d body=%q", healthz.Code, healthz.Body.String())
	}

	unavailable := newPricingHealthAdminAPI(newJSONStore(""), cfg)
	response := httptest.NewRecorder()
	unavailable.get(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-health", nil))
	requirePricingHealthError(t, response, http.StatusServiceUnavailable, "PRICING_HEALTH_STORE_UNAVAILABLE")

	closedDB, err := sql.Open("pgx", phase2ETestDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := closedDB.Close(); err != nil {
		t.Fatal(err)
	}
	broken := newPricingHealthAdminAPI(&postgresStore{db: closedDB, ready: true}, cfg)
	internal := httptest.NewRecorder()
	broken.get(internal, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-health", nil))
	if internal.Code != http.StatusInternalServerError {
		t.Fatalf("infrastructure status=%d body=%s", internal.Code, internal.Body.String())
	}
}

func TestPricingHealthPostgresAggregatesServerOwnedSignals(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	t.Setenv("XIANZHI_APPLY_TEST_MIGRATION_100", "true")
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)

	priceFixture := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	testFixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	missingBindingFixture := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	environmentFixture := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	missingPlanID := "health_missing_price_plan_" + priceFixture.suffix
	missingVersionID := "health_missing_version_" + priceFixture.suffix
	if _, err := db.ExecContext(ctx, `insert into xz_plans(id,code,name,plan_type,active) values($1,$1,'missing health configuration','MEMBER_PACKAGE',true)`, missingPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status) values($1,$2,1,'MEMBER','{}'::jsonb,'PRO',0,30,'DRAFT')`, missingVersionID, missingPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `delete from xz_price_plan_payment_bindings where id=$1`, missingBindingFixture.bindingID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `delete from xz_price_plan_payment_bindings where id=$1`, environmentFixture.bindingID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set bonus_points=7,is_visible=true,audience_type='PUBLIC',currency='CNY' where id=$1`, priceFixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set enabled=true,status='ACTIVE' where id=$1`, priceFixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set is_visible=true,audience_type='PUBLIC',currency='CNY' where id=$1`, missingBindingFixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set enabled=true,status='ACTIVE' where id in($1,$2)`, testFixture.pricePlanID, missingBindingFixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set enabled=true,status='ACTIVE',is_default=true,is_visible=true,audience_type='PUBLIC' where id=$1`, environmentFixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plan_payment_bindings set enabled=true,status='ACTIVE',provider_price_snapshot_cents=101 where id=$1`, priceFixture.bindingID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_wechat_virtual_goods set enabled=true,published=true,status='PUBLISHED',
			verification_status='MANUALLY_CONFIRMED_PUBLISHED',verified_by=$1,verified_at=now()-interval '2 hours',
			verification_reason='isolated health fixture',verification_snapshot=jsonb_build_object(
				'productId',product_id,'offerId',offer_id,'environment',environment,'platformPriceCents',platform_price_cents),
			verification_expires_at=now()-interval '1 hour'
		where id=$2`, priceFixture.actorID, priceFixture.goodID); err != nil {
		t.Fatal(err)
	}
	historicalGoodID := "health_historical_good_" + priceFixture.suffix
	historicalBindingID := "health_historical_binding_" + priceFixture.suffix
	if _, err := db.ExecContext(ctx, `insert into xz_wechat_virtual_goods(id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode) values($1,'WECHAT_VIRTUAL','PRODUCTION','offer',$1,'historical',100,'short_series_goods')`, historicalGoodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_price_plan_payment_bindings(id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',100,false,'DRAFT')`, historicalBindingID, priceFixture.pricePlanID, historicalGoodID); err != nil {
		t.Fatal(err)
	}
	environmentGoodID := "health_environment_good_" + environmentFixture.suffix
	environmentBindingID := "health_environment_binding_" + environmentFixture.suffix
	if _, err := db.ExecContext(ctx, `insert into xz_wechat_virtual_goods(id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,published,enabled,status,verification_status,verified_by,verified_at,verification_reason,verification_snapshot) values($1,'WECHAT_VIRTUAL','PRODUCTION','offer',$1,'environment mismatch',100,'short_series_goods',true,true,'PUBLISHED','MANUALLY_CONFIRMED_PUBLISHED',$2,now(),'isolated health fixture',jsonb_build_object('productId',$1::text,'offerId','offer','environment','PRODUCTION','platformPriceCents',100))`, environmentGoodID, environmentFixture.actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_price_plan_payment_bindings(id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE')`, environmentBindingID, environmentFixture.pricePlanID, environmentGoodID); err != nil {
		t.Fatal(err)
	}
	quoteID := "health_quote_" + priceFixture.suffix
	orderID := "health_order_" + priceFixture.suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_order_price_quotes(
			id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
			entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
			offer_id,wechat_product_id,payment_mode,rights_snapshot,status,expires_at
		) select $1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'PUBLIC',100,101,100,'WECHAT_VIRTUAL','SANDBOX',
			offer_id,product_id,mode,'{}'::jsonb,'AVAILABLE',now()+interval '1 hour'
		from xz_wechat_virtual_goods where id=$7
	`, quoteID, priceFixture.userID, priceFixture.planID, priceFixture.versionID, priceFixture.pricePlanID,
		priceFixture.bindingID, priceFixture.goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_orders(id,user_id,plan_id,amount_cents,status,created_at,price_plan_id) values($1,$2,$3,100,'CREATED',now()::text,$4)`, orderID, priceFixture.userID, priceFixture.planID, priceFixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
	v132TenantID := insertPricingHealthV132Fixture(t, ctx, db, priceFixture.planID, priceFixture.suffix)

	cfg := config.Config{
		Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir(),
		PricePlanCreationEnabled: true, PricePlanTestEntryEnabled: true, SnapshotV2FulfillmentEnabled: true,
	}
	handler, token := pricingHealthHTTPHandler(t, ctx, db, priceFixture, cfg)
	response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/pricing-health", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload pricingHealthView
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode pricing health: %v body=%s", err, response.Body.String())
	}
	if payload.CheckedAt.IsZero() || payload.Status != pricingHealthStatusBlocked {
		t.Fatalf("status/checkedAt=%s/%s", payload.Status, payload.CheckedAt)
	}
	if payload.Runtime.PricePlanCreationEnabled != true || payload.Runtime.PricePlanTestEntryEnabled != true ||
		payload.Runtime.SnapshotV2FulfillmentEnabled != true || !payload.Runtime.V132Blocked || payload.Runtime.V132AffectedTenantCount < 1 {
		t.Fatalf("runtime=%+v", payload.Runtime)
	}
	if !pricingHealthContainsString(payload.Runtime.V132AffectedTenantIDs, v132TenantID) {
		t.Fatalf("runtime tenant scope=%+v want=%s", payload.Runtime.V132AffectedTenantIDs, v132TenantID)
	}
	for _, code := range []string{
		pricingHealthIssueEntitlementVersionMissing,
		pricingHealthIssuePricePlanMissing,
		pricingHealthIssueDefaultMissing,
		pricingHealthIssueGoodNotConfirmed,
		pricingHealthIssueGoodVerificationExpired,
		pricingHealthIssuePriceMismatch,
		pricingHealthIssuePaymentEnvironmentMismatch,
		pricingHealthIssueWhitelistMissing,
		pricingHealthIssueBindingMissing,
		pricingHealthIssueGiftPointsUnavailable,
		pricingHealthIssueV132Blocked,
		pricingHealthIssueDisabled,
	} {
		if !pricingHealthHasIssue(payload.Issues, code) {
			t.Fatalf("missing issue %s in %+v", code, payload.Issues)
		}
	}
	price := pricingHealthPricePlanByID(t, payload.PricePlans, priceFixture.pricePlanID)
	if price.QuoteCount != 1 || price.OrderCount != 1 || price.PaymentBindingID != priceFixture.bindingID || price.WeChatGoodID != priceFixture.goodID {
		t.Fatalf("price aggregate=%+v", price)
	}
	good := pricingHealthGoodByID(t, payload.WeChatGoods, priceFixture.goodID)
	if good.ReferenceCount != 1 {
		t.Fatalf("good aggregate=%+v", good)
	}
	unreferenced := pricingHealthGoodByID(t, payload.WeChatGoods, missingBindingFixture.goodID)
	if unreferenced.ReferenceCount != 0 {
		t.Fatalf("unreferenced good aggregate=%+v", unreferenced)
	}
	priceRows := 0
	for _, item := range payload.PricePlans {
		if item.PricePlanID == priceFixture.pricePlanID {
			priceRows++
		}
	}
	if priceRows != 1 {
		t.Fatalf("historical bindings duplicated price plan rows: %d", priceRows)
	}
	testPrice := pricingHealthPricePlanByID(t, payload.PricePlans, testFixture.pricePlanID)
	if !pricingHealthContainsString(testPrice.IssueCodes, pricingHealthIssueWhitelistMissing) {
		t.Fatalf("test price issues=%+v", testPrice.IssueCodes)
	}
	plan := pricingHealthBusinessPlanByID(t, payload.BusinessPlans, priceFixture.planID)
	if plan.Status != pricingHealthStatusBlocked || price.Status != pricingHealthStatusBlocked {
		t.Fatalf("blocking resource status plan=%s price=%s", plan.Status, price.Status)
	}
	if plan.Defaults.Production != nil || plan.Defaults.Sandbox != nil {
		t.Fatalf("unexpected default summary=%+v", plan.Defaults)
	}
	environmentPlan := pricingHealthBusinessPlanByID(t, payload.BusinessPlans, environmentFixture.planID)
	if environmentPlan.Defaults.Sandbox == nil || environmentPlan.Defaults.Sandbox.PricePlanID != environmentFixture.pricePlanID || environmentPlan.Defaults.Sandbox.SalePriceCents != 100 || environmentPlan.Defaults.Sandbox.Currency != "CNY" || environmentPlan.Defaults.Sandbox.WeChatGoodID != environmentGoodID {
		t.Fatalf("environment default summary=%+v", environmentPlan.Defaults.Sandbox)
	}
	if payload.Summary.BusinessPlanCount < 3 || payload.Summary.PricePlanCount < 3 || payload.Summary.IssueCount != len(payload.Issues) {
		t.Fatalf("summary=%+v issues=%d", payload.Summary, len(payload.Issues))
	}
	if _, err := db.ExecContext(ctx, `update xz_users set role='ADMIN',raw=jsonb_set(raw,'{role}',to_jsonb('ADMIN'::text)) where id=$1`, priceFixture.actorID); err != nil {
		t.Fatal(err)
	}
	forbidden := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/pricing-health", nil, token)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("ordinary admin status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	requirePhase2ERBACError(t, forbidden, "ADMIN_PERMISSION_DENIED")
}

func TestPricingHealthPostgresUsesReadOnlyRepeatableReadTransaction(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	db, ctx := openPhase2ETestPostgres(t)
	tx, err := beginPricingHealthTransaction(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var isolation, readOnly string
	if err := tx.QueryRowContext(ctx, `select current_setting('transaction_isolation'),current_setting('transaction_read_only')`).Scan(&isolation, &readOnly); err != nil {
		t.Fatal(err)
	}
	if isolation != "repeatable read" || readOnly != "on" {
		t.Fatalf("transaction settings=%q/%q", isolation, readOnly)
	}
	if _, err := tx.ExecContext(ctx, `create temporary table pricing_health_must_be_read_only(id int)`); err == nil {
		t.Fatal("pricing health transaction accepted a write")
	}
}

func TestPricingHealthPostgresDefaultProjectionMatchesPublicQuoteEligibility(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	t.Setenv("XIANZHI_APPLY_TEST_MIGRATION_100", "true")
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)

	hidden := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	makePricingHealthPublicCandidate(t, ctx, db, hidden, false, "PUBLIC", "CNY")

	rule := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	makePricingHealthPublicCandidate(t, ctx, db, rule, true, "RULE", "CNY")

	usd := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	makePricingHealthPublicCandidate(t, ctx, db, usd, true, "PUBLIC", "USD")

	nonCurrent := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	currentVersionID := "health_current_version_" + nonCurrent.suffix
	if _, err := db.ExecContext(ctx, `update xz_plan_versions set status='RETIRED' where id=$1`, nonCurrent.versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status,effective_at) values($1,$2,2,'MEMBER','{}'::jsonb,'PRO',0,30,'ACTIVE',now()-interval '1 hour')`, currentVersionID, nonCurrent.planID); err != nil {
		t.Fatal(err)
	}
	makePricingHealthPublicCandidate(t, ctx, db, nonCurrent, true, "PUBLIC", "CNY")

	expired := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	expiredVersionID := "health_expired_version_" + expired.suffix
	if _, err := db.ExecContext(ctx, `update xz_plan_versions set status='RETIRED' where id=$1`, expired.versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status,effective_at,expires_at) values($1,$2,2,'MEMBER','{}'::jsonb,'PRO',0,30,'ACTIVE',now()-interval '2 hours',now()-interval '1 hour')`, expiredVersionID, expired.planID); err != nil {
		t.Fatal(err)
	}
	makePricingHealthPublicCandidate(t, ctx, db, expired, true, "PUBLIC", "CNY")

	future := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	futureVersionID := "health_future_version_" + future.suffix
	if _, err := db.ExecContext(ctx, `update xz_plan_versions set status='RETIRED' where id=$1`, future.versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status,effective_at,expires_at) values($1,$2,2,'MEMBER','{}'::jsonb,'PRO',0,30,'ACTIVE',now()+interval '1 hour',now()+interval '2 hours')`, futureVersionID, future.planID); err != nil {
		t.Fatal(err)
	}
	makePricingHealthPublicCandidate(t, ctx, db, future, true, "PUBLIC", "CNY")

	inactiveParent := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	if _, err := db.ExecContext(ctx, `update xz_plans set active=false where id=$1`, inactiveParent.planID); err != nil {
		t.Fatal(err)
	}
	makePricingHealthPublicCandidate(t, ctx, db, inactiveParent, true, "PUBLIC", "CNY")

	missingDefault := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	makePricingHealthPublicCandidate(t, ctx, db, missingDefault, true, "PUBLIC", "CNY")
	usdDefaultID := "health_usd_default_" + missingDefault.suffix
	if _, err := db.ExecContext(ctx, `insert into xz_price_plans(
		id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
		sale_price_cents,original_price_cents,audience_type,audience_rule,is_visible,is_default,enabled,status
	) values($1,$2,$3,$1,'ineligible USD default','NORMAL','WECHAT_VIRTUAL','SANDBOX','USD',100,100,'PUBLIC','{}'::jsonb,true,true,true,'ACTIVE')`, usdDefaultID, missingDefault.planID, missingDefault.versionID); err != nil {
		t.Fatal(err)
	}

	view, err := (&postgresStore{db: db, ready: true}).pricingHealth(ctx, config.Config{
		PricePlanCreationEnabled: true, PricePlanTestEntryEnabled: true, SnapshotV2FulfillmentEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, fixture := range map[string]phase2EFixture{
		"hidden": hidden, "rule audience": rule, "USD": usd, "non-current version": nonCurrent,
		"expired version": expired, "future version": future, "inactive parent": inactiveParent,
	} {
		t.Run(name, func(t *testing.T) {
			plan := pricingHealthBusinessPlanByID(t, view.BusinessPlans, fixture.planID)
			if plan.Defaults.Production != nil || plan.Defaults.Sandbox != nil {
				t.Fatalf("ineligible price filled defaults: %+v", plan.Defaults)
			}
			if pricingHealthHasIssueForPlan(view.Issues, pricingHealthIssueDefaultMissing, fixture.planID) {
				t.Fatalf("ineligible price created DEFAULT_PRICE_PLAN_MISSING: %+v", plan.IssueCodes)
			}
		})
	}
	if got := pricingHealthBusinessPlanByID(t, view.BusinessPlans, nonCurrent.planID).ActiveVersionID; got != currentVersionID {
		t.Fatalf("non-current bound version activeVersionId=%q want=%q", got, currentVersionID)
	}
	for _, fixture := range []phase2EFixture{expired, future} {
		if got := pricingHealthBusinessPlanByID(t, view.BusinessPlans, fixture.planID).ActiveVersionID; got != "" {
			t.Fatalf("non-current activeVersionId=%q want empty", got)
		}
	}

	plan := pricingHealthBusinessPlanByID(t, view.BusinessPlans, missingDefault.planID)
	if !pricingHealthHasIssueForPlan(view.Issues, pricingHealthIssueDefaultMissing, missingDefault.planID) {
		t.Fatalf("public CNY non-default was masked by ineligible USD default: issues=%+v", plan.IssueCodes)
	}
	if plan.Defaults.Production != nil || plan.Defaults.Sandbox != nil {
		t.Fatalf("ineligible USD default filled summary: %+v", plan.Defaults)
	}
}

func makePricingHealthPublicCandidate(t *testing.T, ctx context.Context, db *sql.DB, fixture phase2EFixture, visible bool, audience, currency string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `update xz_price_plans set is_visible=$2,audience_type=$3,currency=$4,audience_rule=case when $3='RULE' then '{"cohort":"pilot"}'::jsonb else '{}'::jsonb end,is_default=false,effective_at=null,expires_at=null where id=$1`, fixture.pricePlanID, visible, audience, currency); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set enabled=true,status='ACTIVE' where id=$1`, fixture.pricePlanID); err != nil {
		t.Fatal(err)
	}
}

func pricingHealthHasIssueForPlan(items []pricingHealthIssue, code, planID string) bool {
	for _, item := range items {
		if item.Code == code && item.PlanID == planID {
			return true
		}
	}
	return false
}

func TestPricingHealthEvaluationCanBeDegradedWithoutInfrastructureFailure(t *testing.T) {
	view := pricingHealthView{Issues: []pricingHealthIssue{{Code: pricingHealthIssueDisabled, Severity: pricingHealthSeverityWarning}}}
	finalizePricingHealth(&view)
	if view.Status != pricingHealthStatusDegraded || view.Summary.DegradedIssueCount != 1 {
		t.Fatalf("view=%+v", view)
	}
}

func pricingHealthHTTPHandler(t *testing.T, ctx context.Context, db *sql.DB, fixture phase2EFixture, cfg config.Config) (http.Handler, string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `update xz_users set raw=jsonb_build_object('id',id,'email',email,'name',name,'role',role,'status',status) where id=$1`, fixture.actorID); err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	token := "pricing-health-token-" + fixture.suffix
	if err := sessions.Put(ctx, token, fixture.actorID, time.Hour); err != nil {
		t.Fatal(err)
	}
	return newWithStoreAndSessions(cfg, &postgresStore{db: db, ready: true}, sessions).Handler, token
}

func insertPricingHealthV132Fixture(t *testing.T, ctx context.Context, db *sql.DB, planID, suffix string) string {
	t.Helper()
	tenantID := "tenant_health_v132_" + suffix
	ruleSetID := "rules_health_v132_" + suffix
	versionID := "commercial_health_v132_" + suffix
	ruleID := "commission_health_v132_" + suffix
	rolloutID := "rollout_health_v132_" + suffix
	if _, err := db.ExecContext(ctx, `insert into xz_tenants(id,tenant_type,name,status) values($1,'PERSONAL',$1,'ACTIVE')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_commercial_rule_sets(id,tenant_id,rule_code,version,name,status,effective_start_at,published_by,published_at) values($1,$2,$3,1,'health V132','PUBLISHED',now()-interval '1 day','test',now())`, ruleSetID, tenantID, "HEALTH_V132_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_commercial_plan_versions(id,tenant_id,rule_set_id,plan_id,version,identity_type,price_cents,currency,token_grant_amount,token_rights_value_cents,duration_days) values($1,$2,$3,$4,1,'MEMBER',100,'CNY',100,100,30)`, versionID, tenantID, ruleSetID, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_commission_rules(id,tenant_id,rule_code,rule_name,product_type,product_id,beneficiary_role,relationship_level,calculation_type,effective_start_at,version,status,commercial_rule_set_id,commercial_scenario_code) values($1,$2,$3,'remainder','MEMBER_PURCHASE',$4,'PLATFORM',0,'REMAINDER_TO_PLATFORM',now()-interval '1 day',1,'ACTIVE',$5,'MEMBER_PURCHASE')`, ruleID, tenantID, "HEALTH_PLATFORM_"+suffix, planID, ruleSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_channel_rollout_configs(id,tenant_id,mode,enabled,pinned_rule_set_id,pinned_rule_set_version,canary_basis_points,real_switch_enabled,change_reason,updated_by) values($1,$2,'V132',true,$3,1,0,true,'isolated health signal','test')`, rolloutID, tenantID, ruleSetID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_channel_rollout_configs where id=$1`, rolloutID)
		_, _ = db.Exec(`delete from xz_commission_rules where id=$1`, ruleID)
		_, _ = db.Exec(`delete from xz_commercial_plan_versions where id=$1`, versionID)
		_, _ = db.Exec(`delete from xz_commercial_rule_sets where id=$1`, ruleSetID)
		_, _ = db.Exec(`delete from xz_tenants where id=$1`, tenantID)
	})
	return tenantID
}

func requirePricingHealthError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body.String(), status)
	}
	var payload struct{ Code, Message, Error string }
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != code || payload.Message == "" || payload.Error == "" {
		t.Fatalf("payload=%+v", payload)
	}
}

func pricingHealthHasIssue(items []pricingHealthIssue, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func pricingHealthContainsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func pricingHealthPricePlanByID(t *testing.T, items []pricingHealthPricePlan, id string) pricingHealthPricePlan {
	t.Helper()
	for _, item := range items {
		if item.PricePlanID == id {
			return item
		}
	}
	t.Fatalf("price plan %s not found", id)
	return pricingHealthPricePlan{}
}
func pricingHealthGoodByID(t *testing.T, items []pricingHealthWeChatGood, id string) pricingHealthWeChatGood {
	t.Helper()
	for _, item := range items {
		if item.WeChatGoodID == id {
			return item
		}
	}
	t.Fatalf("good %s not found", id)
	return pricingHealthWeChatGood{}
}
func pricingHealthBusinessPlanByID(t *testing.T, items []pricingHealthBusinessPlan, id string) pricingHealthBusinessPlan {
	t.Helper()
	for _, item := range items {
		if item.PlanID == id {
			return item
		}
	}
	t.Fatalf("business plan %s not found", id)
	return pricingHealthBusinessPlan{}
}
