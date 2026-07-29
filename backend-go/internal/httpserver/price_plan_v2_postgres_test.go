package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPricePlanV2PostgresPublicTestAndPaymentIntegrity(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var migrated bool
	if err := db.QueryRowContext(ctx, `
		select to_regclass('xz_price_plans') is not null and exists(
			select 1 from information_schema.columns
			where table_name='xz_wechat_virtual_goods' and column_name='verification_status'
		)
	`).Scan(&migrated); err != nil || !migrated {
		t.Skip("migrations 097 and 098 are not applied to the test database")
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "price_user_" + suffix
	whiteUserID := "price_white_" + suffix
	planID := "plan_member_" + suffix
	versionID := "plan_version_" + suffix
	normalID := "price_normal_" + suffix
	testID := "price_test_" + suffix
	goodNormalID := "good_normal_" + suffix
	goodTestID := "good_test_" + suffix
	bindingNormalID := "binding_normal_" + suffix
	bindingTestID := "binding_test_" + suffix
	whitelistID := "white_" + suffix
	v132TenantID := "tenant_price_v132_" + suffix
	v132RuleSetID := "rules_price_v132_" + suffix
	v132PlanVersionID := "commercial_plan_v132_" + suffix
	v132RuleID := "commission_price_v132_" + suffix
	v132RolloutID := "rollout_price_v132_" + suffix
	defer func() {
		_, _ = db.Exec(`delete from xz_billing_payment_requests where user_id in($1,$2)`, userID, whiteUserID)
		_, _ = db.Exec(`delete from xz_billing_invoices where user_id in($1,$2)`, userID, whiteUserID)
		_, _ = db.Exec(`delete from xz_payment_records where user_id in($1,$2)`, userID, whiteUserID)
		_, _ = db.Exec(`delete from xz_orders where user_id in($1,$2)`, userID, whiteUserID)
		_, _ = db.Exec(`delete from xz_order_settlement_engine_decisions where tenant_id=$1`, v132TenantID)
		_, _ = db.Exec(`delete from xz_channel_rollout_configs where id=$1`, v132RolloutID)
		_, _ = db.Exec(`delete from xz_commission_rules where id=$1`, v132RuleID)
		_, _ = db.Exec(`delete from xz_commercial_plan_versions where id=$1`, v132PlanVersionID)
		_, _ = db.Exec(`delete from xz_commercial_rule_sets where id=$1`, v132RuleSetID)
		_, _ = db.Exec(`delete from xz_order_price_quotes where price_plan_id in($1,$2)`, normalID, testID)
		_, _ = db.Exec(`delete from xz_price_plan_user_whitelist where price_plan_id in($1,$2)`, normalID, testID)
		_, _ = db.Exec(`delete from xz_price_plan_payment_bindings where id in($1,$2)`, bindingNormalID, bindingTestID)
		_, _ = db.Exec(`delete from xz_wechat_virtual_goods where id in($1,$2)`, goodNormalID, goodTestID)
		_, _ = db.Exec(`delete from xz_price_plans where id in($1,$2)`, normalID, testID)
		_, _ = db.Exec(`delete from xz_plan_versions where id=$1`, versionID)
		_, _ = db.Exec(`delete from xz_plans where id=$1`, planID)
		_, _ = db.Exec(`delete from xz_tenants where id=$1`, v132TenantID)
		_, _ = db.Exec(`delete from xz_users where id in($1,$2)`, userID, whiteUserID)
	}()
	for _, id := range []string{userID, whiteUserID} {
		if _, err := db.ExecContext(ctx, `insert into xz_users(id,email,name,role,status,created_at,updated_at) values($1,$1||'@example.test',$1,'MEMBER','ACTIVE',$2,$2)`, id, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plans(id,code,name,plan_type,active) values($1,$1,'member','MEMBER_PACKAGE',true)`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,status)
		values($1,$2,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":40000,"durationDays":365}'::jsonb,'PRO',40000,365,'ACTIVE')
	`, versionID, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(id,plan_id,plan_version_id,code,name,price_type,channel,environment,sale_price_cents,original_price_cents,audience_type,is_default,is_visible,enabled,status)
		values($1,$3,$4,$1,'normal','NORMAL','WECHAT_VIRTUAL','PRODUCTION',99600,99600,'PUBLIC',true,true,true,'ACTIVE'),
		      ($2,$3,$4,$2,'test','TEST','WECHAT_VIRTUAL','PRODUCTION',100,99600,'PUBLIC',false,false,true,'ACTIVE')
	`, normalID, testID, planID, versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,published,enabled,status,
			verification_status,verified_by,verified_at,verification_reason,verification_snapshot
		)
		values($1,'WECHAT_VIRTUAL','PRODUCTION','offer','MEMBER_996','normal',99600,true,true,'PUBLISHED',
			'MANUALLY_CONFIRMED_PUBLISHED','phase1-test',now(),'test fixture',jsonb_build_object(
				'productId','MEMBER_996','offerId','offer','environment','PRODUCTION','platformPriceCents',99600)),
		      ($2,'WECHAT_VIRTUAL','PRODUCTION','offer','MEMBER_TEST_1','test',100,true,true,'PUBLISHED',
			'MANUALLY_CONFIRMED_PUBLISHED','phase1-test',now(),'test fixture',jsonb_build_object(
				'productId','MEMBER_TEST_1','offerId','offer','environment','PRODUCTION','platformPriceCents',100))
	`, goodNormalID, goodTestID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status)
		values($1,$3,$5,'WECHAT_VIRTUAL','PRODUCTION',99600,true,'ACTIVE'),
		      ($2,$4,$6,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE')
	`, bindingNormalID, bindingTestID, normalID, testID, goodNormalID, goodTestID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_price_plan_user_whitelist(id,price_plan_id,user_id) values($1,$2,$3)`, whitelistID, testID, whiteUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_tenants(id,tenant_type,name,status) values($1,'PERSONAL',$1,'ACTIVE')`, v132TenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_commercial_rule_sets(
			id,tenant_id,rule_code,version,name,status,effective_start_at,published_by,published_at
		) values($1,$2,$3,1,'V132 incompatible price fixture','PUBLISHED',now()-interval '1 day','test',now())
	`, v132RuleSetID, v132TenantID, "PRICE_V132_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_commercial_plan_versions(
			id,tenant_id,rule_set_id,plan_id,version,identity_type,price_cents,currency,
			token_grant_amount,token_rights_value_cents,duration_days
		) values($1,$2,$3,$4,1,'MEMBER',99600,'CNY',40000,25000,365)
	`, v132PlanVersionID, v132TenantID, v132RuleSetID, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_commission_rules(
			id,tenant_id,rule_code,rule_name,product_type,product_id,beneficiary_role,relationship_level,
			calculation_type,effective_start_at,version,status,commercial_rule_set_id,commercial_scenario_code
		) values($1,$2,$3,'platform remainder','MEMBER_PURCHASE',$4,'PLATFORM',0,
			'REMAINDER_TO_PLATFORM',now()-interval '1 day',1,'ACTIVE',$5,'MEMBER_PURCHASE')
	`, v132RuleID, v132TenantID, "PRICE_PLATFORM_"+suffix, planID, v132RuleSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_channel_rollout_configs(
			id,tenant_id,mode,enabled,pinned_rule_set_id,pinned_rule_set_version,
			canary_basis_points,real_switch_enabled,change_reason,updated_by
		) values($1,$2,'V132',true,$3,1,0,true,'isolated V132 price compatibility test','test')
	`, v132RolloutID, v132TenantID, v132RuleSetID); err != nil {
		t.Fatal(err)
	}

	service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{Env: 0}}
	now := time.Now().UTC()
	public, err := service.resolvePriceQuoteSource(ctx, "tenant", userID, planID, "", pricePlanEntryPublic, now)
	if err != nil || public.PricePlanID != normalID || public.TransactionPriceCents != 99600 {
		t.Fatalf("ordinary user did not resolve normal default: %+v %v", public, err)
	}
	whitePublic, err := service.resolvePriceQuoteSource(ctx, "tenant", whiteUserID, planID, "", pricePlanEntryPublic, now)
	if err != nil || whitePublic.PricePlanID != normalID {
		t.Fatalf("whitelisted user ordinary entry selected test plan: %+v %v", whitePublic, err)
	}
	if _, err := service.resolvePriceQuoteSource(ctx, "tenant", userID, planID, testID, pricePlanEntryPublic, now); !errors.Is(err, errPricePlanUnavailable) {
		t.Fatalf("ordinary entry accepted a forged test pricePlanId: %v", err)
	}
	if _, err := service.resolvePriceQuoteSource(ctx, "tenant", userID, planID, testID, pricePlanEntryTest, now); !errors.Is(err, errPricePlanNotEligible) {
		t.Fatalf("ordinary user accessed test plan: %v", err)
	}
	testQuote, err := service.resolvePriceQuoteSource(ctx, "tenant", whiteUserID, planID, testID, pricePlanEntryTest, now)
	if err != nil || testQuote.TransactionPriceCents != 100 || testQuote.PricePlanID != testID {
		t.Fatalf("whitelisted dedicated entry did not resolve one-yuan plan: %+v %v", testQuote, err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_wechat_virtual_goods set
			platform_price_cents=101,
			verification_snapshot=jsonb_set(verification_snapshot,'{platformPriceCents}','101'::jsonb)
		where id=$1
	`, goodTestID); err != nil {
		t.Fatal(err)
	}
	mismatch, err := service.resolvePriceQuoteSource(ctx, "tenant", whiteUserID, planID, testID, pricePlanEntryTest, now)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(validatePricePlanPaymentChain(mismatch.paymentChain()), errPricePlanPriceMismatch) {
		t.Fatal("one-cent WeChat price mismatch was accepted")
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents)
		values($1,$2,$3,'WECHAT_VIRTUAL','SANDBOX',99600)
	`, "cross_"+suffix, normalID, goodNormalID); err == nil {
		t.Fatal("database allowed a production WeChat good to be bound as sandbox")
	}
	if _, err := db.ExecContext(ctx, `
		update xz_wechat_virtual_goods set
			platform_price_cents=100,
			verification_snapshot=jsonb_set(verification_snapshot,'{platformPriceCents}','100'::jsonb)
		where id=$1
	`, goodTestID); err != nil {
		t.Fatal(err)
	}

	service.cfg = virtualPaymentConfig{
		Enabled: true, Env: 0, OfferID: "runtime-offer", AppKey: "test-app-key", NotifyToken: "test-token",
		Mode: "short_series_goods", AppID: "test-app", AppSecret: "test-secret",
		PricePlanCreationEnabled: true, PricePlanTestEntryEnabled: true,
		SnapshotV2FulfillmentEnabled: true,
	}
	buyer := adminUser{ID: userID}
	issued, err := service.issuePriceQuote(ctx, buyer, "", planID, "", pricePlanEntryPublic)
	if err != nil {
		t.Fatal(err)
	}
	var publicWhitelistID sql.NullString
	var publicWhitelistRevision sql.NullInt64
	var publicWhitelistCheckedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `
		select whitelist_entry_id,whitelist_revision,whitelist_checked_at
		from xz_order_price_quotes where quote_token_hash=$1
	`, hashSensitiveIdentifier(issued.QuoteID)).Scan(&publicWhitelistID, &publicWhitelistRevision, &publicWhitelistCheckedAt); err != nil {
		t.Fatal(err)
	}
	if publicWhitelistID.Valid || publicWhitelistRevision.Valid || publicWhitelistCheckedAt.Valid {
		t.Fatalf("PUBLIC quote persisted a TEST whitelist pin: id=%v revision=%v checkedAt=%v", publicWhitelistID, publicWhitelistRevision, publicWhitelistCheckedAt)
	}
	session := &wechatMiniProgramSession{OpenID: "openid_" + suffix, SessionKey: "session_" + suffix}
	results := make(chan error, 2)
	orders := make(chan createVirtualOrderResponse, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order, createErr := service.createOrderFromPriceQuote(context.Background(), buyer, "", issued.QuoteID, session)
			if createErr == nil {
				orders <- order
			}
			results <- createErr
		}()
	}
	wg.Wait()
	close(results)
	close(orders)
	succeeded, consumed := 0, 0
	var created createVirtualOrderResponse
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, errPriceQuoteConsumed) {
			consumed++
		}
	}
	for order := range orders {
		created = order
	}
	if succeeded != 1 || consumed != 1 || created.AmountCent != 99600 {
		t.Fatalf("concurrent quote consumption was not exactly-once: success=%d consumed=%d order=%+v", succeeded, consumed, created)
	}
	var snapshotVersion int
	var storedPlanVersion, storedPricePlan, storedProduct string
	var storedPrice, storedGoodsPrice int64
	if err := db.QueryRowContext(ctx, `
		select snapshot_version,plan_version_id,price_plan_id,transaction_price_cents,wechat_product_id_snapshot,wechat_goods_price_cents
		from xz_orders where order_no=$1
	`, created.OrderNo).Scan(&snapshotVersion, &storedPlanVersion, &storedPricePlan, &storedPrice, &storedProduct, &storedGoodsPrice); err != nil {
		t.Fatal(err)
	}
	if snapshotVersion != 2 || storedPlanVersion != versionID || storedPricePlan != normalID || storedPrice != 99600 || storedProduct != "MEMBER_996" || storedGoodsPrice != 99600 {
		t.Fatalf("order V2 columns are incomplete: version=%d planVersion=%s pricePlan=%s price=%d product=%s goods=%d", snapshotVersion, storedPlanVersion, storedPricePlan, storedPrice, storedProduct, storedGoodsPrice)
	}
	if _, err := service.createOrderFromPriceQuote(ctx, buyer, "", issued.QuoteID, session); !errors.Is(err, errPriceQuoteConsumed) {
		t.Fatalf("reused quote was not rejected: %v", err)
	}
	expired, err := service.issuePriceQuote(ctx, buyer, "", planID, "", pricePlanEntryPublic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_order_price_quotes set created_at=now()-interval '10 minutes',expires_at=now()-interval '1 second' where quote_token_hash=$1`, hashSensitiveIdentifier(expired.QuoteID)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.createOrderFromPriceQuote(ctx, buyer, "", expired.QuoteID, session); !errors.Is(err, errPriceQuoteExpired) {
		t.Fatalf("expired quote was not rejected: %v", err)
	}
	foreign, err := service.issuePriceQuote(ctx, adminUser{ID: whiteUserID}, "", planID, "", pricePlanEntryPublic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.createOrderFromPriceQuote(ctx, buyer, "", foreign.QuoteID, session); !errors.Is(err, errPriceQuoteForbidden) {
		t.Fatalf("cross-user quote was not rejected: %v", err)
	}

	v132Buyer := adminUser{ID: whiteUserID, TenantID: v132TenantID}
	v132Quote, err := service.issuePriceQuote(ctx, v132Buyer, "", planID, testID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	var pinnedWhitelistID string
	var pinnedWhitelistRevision int64
	var pinnedWhitelistCheckedAt time.Time
	if err := db.QueryRowContext(ctx, `
		select whitelist_entry_id,whitelist_revision,whitelist_checked_at
		from xz_order_price_quotes where quote_token_hash=$1
	`, hashSensitiveIdentifier(v132Quote.QuoteID)).Scan(&pinnedWhitelistID, &pinnedWhitelistRevision, &pinnedWhitelistCheckedAt); err != nil {
		t.Fatal(err)
	}
	if pinnedWhitelistID != whitelistID || pinnedWhitelistRevision < 1 || pinnedWhitelistCheckedAt.IsZero() {
		t.Fatalf("TEST quote whitelist pin=%s/%d/%s want=%s/positive/non-zero", pinnedWhitelistID, pinnedWhitelistRevision, pinnedWhitelistCheckedAt, whitelistID)
	}
	response, err := service.createOrderFromPriceQuote(ctx, v132Buyer, "", v132Quote.QuoteID, session)
	if !errors.Is(err, errPricePlanV132SnapshotIncompatible) {
		t.Fatalf("V132 pinned 996-yuan plan accepted a one-yuan order: response=%+v err=%v", response, err)
	}
	if response.OrderNo != "" || response.SignData != "" || response.PaySig != "" || response.Signature != "" {
		t.Fatalf("rejected V132 order returned payment signing material: %+v", response)
	}
	var quoteStatus string
	var consumedOrder sql.NullString
	if err := db.QueryRowContext(ctx, `
		select status,consumed_order_no from xz_order_price_quotes where quote_token_hash=$1
	`, hashSensitiveIdentifier(v132Quote.QuoteID)).Scan(&quoteStatus, &consumedOrder); err != nil {
		t.Fatal(err)
	}
	if quoteStatus != quoteStatusAvailable || consumedOrder.Valid {
		t.Fatalf("rejected V132 order consumed its quote: status=%s order=%v", quoteStatus, consumedOrder)
	}
	var orderCount, paymentCount, decisionCount int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_orders where user_id=$1 and price_plan_id=$2`, whiteUserID, testID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_payment_records where user_id=$1`, whiteUserID).Scan(&paymentCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_order_settlement_engine_decisions where tenant_id=$1`, v132TenantID).Scan(&decisionCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 || paymentCount != 0 || decisionCount != 0 {
		t.Fatalf("V132 compatibility rejection did not roll back: orders=%d payments=%d decisions=%d", orderCount, paymentCount, decisionCount)
	}
}

func TestPricePlanV2PostgresOneYuanAgentFulfillmentIsSnapshotOnlyAndIdempotent(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var migrated bool
	if err := db.QueryRowContext(ctx, `
		select to_regclass('xz_price_plans') is not null and exists(
			select 1 from information_schema.columns
			where table_name='xz_wechat_virtual_goods' and column_name='verification_status'
		)
	`).Scan(&migrated); err != nil || !migrated {
		t.Skip("migrations 097 and 098 are not applied to the test database")
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID, planID := "agent_user_"+suffix, "plan_agent_"+suffix
	versionID, priceID := "agent_version_"+suffix, "agent_price_"+suffix
	goodID, bindingID := "agent_good_"+suffix, "agent_binding_"+suffix
	ruleID, ruleCode := "agent_platform_rule_"+suffix, "AGENT_PLATFORM_"+suffix
	agentProductID := "AGENT_TEST_" + suffix
	defer func() {
		_, _ = db.Exec(`delete from xz_billing_payment_requests where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_billing_invoices where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_commission_records where source_user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_commissions where raw->>'sourceUserId'=$1`, userID)
		_, _ = db.Exec(`delete from xz_commission_rules where id=$1`, ruleID)
		_, _ = db.Exec(`delete from xz_token_records where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_point_accounts where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_channel_agents where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_user_business_identities where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_payment_records where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_orders where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_order_price_quotes where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_price_plan_user_whitelist where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_price_plan_payment_bindings where id=$1`, bindingID)
		_, _ = db.Exec(`delete from xz_wechat_virtual_goods where id=$1`, goodID)
		_, _ = db.Exec(`delete from xz_price_plans where id=$1`, priceID)
		_, _ = db.Exec(`delete from xz_plan_versions where id=$1`, versionID)
		_, _ = db.Exec(`delete from xz_plans where id=$1`, planID)
		_, _ = db.Exec(`delete from xz_users where id=$1`, userID)
	}()
	nowText := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `insert into xz_users(id,email,name,role,status,created_at,updated_at) values($1,$1||'@example.test',$1,'MEMBER','ACTIVE',$2,$2)`, userID, nowText); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_point_accounts(id,user_id,available,frozen,raw) values($1,$2,0,0,'{}')`, "points_"+suffix, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_plans(id,code,name,plan_type,active) values($1,$1,'agent','AGENT_JOIN_PACKAGE',true)`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_commission_rules(
			id,tenant_id,rule_code,rule_name,product_type,product_id,beneficiary_role,relationship_level,
			calculation_type,effective_start_at,version,status
		) values($1,'tenant_default',$2,'platform remainder','AGENT_JOIN_PACKAGE',$3,'PLATFORM',0,
		         'REMAINDER_TO_PLATFORM',now()-interval '1 day',1,'ACTIVE')
	`, ruleID, ruleCode, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(id,plan_id,version_no,business_type,rights_snapshot,agent_level,token_amount,duration_days,commission_rule_version,commission_snapshot,status)
		values($1,$2,1,'AGENT','{"agentLevel":"AGENT","tokenAmount":20000,"durationDays":0}'::jsonb,'AGENT',20000,0,
		       'agent-platform-test-v1',
		       jsonb_build_object('rules',jsonb_build_array(jsonb_build_object(
		         'id',$3::text,'code',$4::text,'name','platform remainder','version',1,'beneficiaryRole','PLATFORM',
		         'relationshipLevel',0,'calculationType','REMAINDER_TO_PLATFORM','fixedAmountCents',0,
		         'percentageBps',0,'freezeDays',0,'refundPolicy','REVERSE_OR_RECOVER'))),
		       'ACTIVE')
	`, versionID, planID, ruleID, ruleCode); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(id,plan_id,plan_version_id,code,name,price_type,channel,environment,sale_price_cents,original_price_cents,audience_type,is_default,is_visible,enabled,status)
		values($1,$2,$3,$1,'agent test','TEST','WECHAT_VIRTUAL','SANDBOX',100,99600,'WHITELIST',false,false,true,'ACTIVE')
	`, priceID, planID, versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,published,enabled,status,
			verification_status,verified_by,verified_at,verification_reason,verification_snapshot
		)
		values($1,'WECHAT_VIRTUAL','SANDBOX','offer',$2,'agent test',100,true,true,'PUBLISHED',
			'MANUALLY_CONFIRMED_PUBLISHED','phase1-test',now(),'test fixture',jsonb_build_object(
				'productId',$2::text,'offerId','offer','environment','SANDBOX','platformPriceCents',100))
	`, goodID, agentProductID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status)
		values($1,$2,$3,'WECHAT_VIRTUAL','SANDBOX',100,true,'ACTIVE')
	`, bindingID, priceID, goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into xz_price_plan_user_whitelist(id,price_plan_id,user_id) values($1,$2,$3)`, "white_"+suffix, priceID, userID); err != nil {
		t.Fatal(err)
	}
	service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{
		Enabled: true, Env: 1, OfferID: "runtime-offer", AppKey: "sandbox-key", NotifyToken: "token",
		Mode: "short_series_goods", AppID: "app", AppSecret: "secret",
		PricePlanCreationEnabled: true, PricePlanTestEntryEnabled: true,
		SnapshotV2FulfillmentEnabled: true,
	}}
	user := adminUser{ID: userID}
	quote, err := service.issuePriceQuote(ctx, user, "", planID, priceID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	order, err := service.createOrderFromPriceQuote(ctx, user, "", quote.QuoteID, &wechatMiniProgramSession{OpenID: "agent-openid", SessionKey: "agent-session"})
	if err != nil {
		t.Fatal(err)
	}
	var settlementDecisions int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_order_settlement_engine_decisions where order_id=$1`, order.OrderNo).Scan(&settlementDecisions); err != nil {
		t.Fatal(err)
	}
	if settlementDecisions != 1 {
		t.Fatalf("V2 order creation did not persist exactly one settlement decision: got %d", settlementDecisions)
	}
	var priceSnapshotBefore string
	if err := db.QueryRowContext(ctx, `select price_snapshot::text from xz_orders where order_no=$1`, order.OrderNo).Scan(&priceSnapshotBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_orders set status='PAID',paid_at=$2 where order_no=$1`, order.OrderNo, nowText); err != nil {
		t.Fatal(err)
	}
	if err := service.GrantOrderEntitlements(ctx, order.OrderNo); err != nil {
		t.Fatalf("one-yuan V2 agent fulfillment failed: %v", err)
	}
	if err := service.GrantOrderEntitlements(ctx, order.OrderNo); err != nil {
		t.Fatalf("duplicate V2 callback was not idempotent: %v", err)
	}
	var priceSnapshotAfter string
	if err := db.QueryRowContext(ctx, `select price_snapshot::text from xz_orders where order_no=$1`, order.OrderNo).Scan(&priceSnapshotAfter); err != nil {
		t.Fatal(err)
	}
	if priceSnapshotAfter != priceSnapshotBefore {
		t.Fatalf("V2 fulfillment mutated immutable price snapshot\nbefore: %s\nafter:  %s", priceSnapshotBefore, priceSnapshotAfter)
	}
	var entitlementStatus string
	var activeAgentIdentities, tokenRecords, commissionRecords int
	if err := db.QueryRowContext(ctx, `select entitlement_status from xz_orders where order_no=$1`, order.OrderNo).Scan(&entitlementStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_user_business_identities where user_id=$1 and identity_type='AGENT' and identity_status='ACTIVE' and ended_at is null`, userID).Scan(&activeAgentIdentities); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_token_records where user_id=$1 and order_id=$2`, userID, order.OrderNo).Scan(&tokenRecords); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_commission_records where source_user_id=$1 and order_id=$2`, userID, order.OrderNo).Scan(&commissionRecords); err != nil {
		t.Fatal(err)
	}
	if activeAgentIdentities != 1 || entitlementStatus != entitlementSuccess || tokenRecords != 1 || commissionRecords != 1 {
		t.Fatalf("V2 agent fulfillment/idempotency mismatch: activeAgentIdentities=%d entitlement=%s tokens=%d commissions=%d", activeAgentIdentities, entitlementStatus, tokenRecords, commissionRecords)
	}
}
