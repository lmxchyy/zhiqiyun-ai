package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestManagedMemberAgentPlanCannotFallBackToLegacyVirtualPayment(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var governed bool
	if err := db.QueryRowContext(ctx, `
		select exists(
			select 1 from information_schema.columns
			where table_name='xz_wechat_virtual_goods' and column_name='verification_status'
		)
	`).Scan(&governed); err != nil || !governed {
		t.Skip("migrations 097 and 098 are not applied to the test database")
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "legacy_guard_user_" + suffix
	planID := "legacy_guard_plan_" + suffix
	legacyCode := "LEGACY_GUARD_PRODUCT_" + suffix
	legacyMappingID := "legacy_guard_mapping_" + suffix
	versionID := "legacy_guard_version_" + suffix
	pricePlanID := "legacy_guard_price_" + suffix
	goodID := "legacy_guard_good_" + suffix
	bindingID := "legacy_guard_binding_" + suffix
	var versionNo int
	if err := db.QueryRowContext(ctx, `select coalesce(max(version_no),0)+1 from xz_plan_versions where plan_id=$1`, planID).Scan(&versionNo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_users(id,email,name,role,status,created_at,updated_at)
		values($1,$1||'@example.test',$1,'MEMBER','ACTIVE',now(),now())
	`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,product_type,payment_product_code,price_cents,active)
		values($1,$1,'legacy guard member','MEMBER_PACKAGE','MEMBERSHIP',$2,99600,true)
	`, planID, legacyCode); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_product_mappings(
			id,plan_id,offer_id,wechat_product_id,mode,env,enabled
		) values($1,$2,'legacy-guard-offer',$3,'short_series_goods',0,true)
	`, legacyMappingID, planID, "LEGACY_GUARD_V1_GOOD_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,
			token_amount,duration_days,status
		) values($1,$2,$3,'MEMBER',
			'{"memberLevel":"PRO","tokenAmount":40000,"durationDays":365}'::jsonb,
			'PRO',40000,365,'DRAFT')
	`, versionID, planID, versionNo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_billing_payment_requests where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_billing_invoices where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_payment_records where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_orders where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_order_price_quotes where price_plan_id=$1`, pricePlanID)
		_, _ = db.Exec(`delete from xz_price_plan_payment_bindings where id=$1`, bindingID)
		_, _ = db.Exec(`delete from xz_wechat_virtual_goods where id=$1`, goodID)
		_, _ = db.Exec(`delete from xz_price_plans where id=$1`, pricePlanID)
		_, _ = db.Exec(`delete from xz_plan_versions where id=$1`, versionID)
		_, _ = db.Exec(`delete from xz_wechat_virtual_product_mappings where id=$1`, legacyMappingID)
		_, _ = db.Exec(`delete from xz_plans where id=$1`, planID)
		_, _ = db.Exec(`delete from xz_users where id=$1`, userID)
	})

	service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{
		Enabled: true, Env: 0, OfferID: "local-offer", AppKey: "local-key", NotifyToken: "local-token",
		Mode: "short_series_goods", AppID: "local-app", AppSecret: "local-secret",
		PricePlanCreationEnabled: false,
	}}
	managed, err := service.isManagedMemberAgentPlanRef(ctx, legacyCode)
	if err != nil {
		t.Fatal(err)
	}
	if managed {
		t.Fatal("a DRAFT entitlement version prematurely cut the historical plan over to V2")
	}
	if _, err := service.productByCode(ctx, legacyCode); err != nil {
		t.Fatalf("DRAFT V2 preparation broke the historical V1 catalog: %v", err)
	}
	if _, err := db.ExecContext(ctx, `update xz_plan_versions set status='ACTIVE' where id=$1`, versionID); err != nil {
		t.Fatal(err)
	}
	managed, err = service.isManagedMemberAgentPlanRef(ctx, legacyCode)
	if err != nil {
		t.Fatal(err)
	}
	if managed {
		t.Fatal("an ACTIVE entitlement version without a payment binding prematurely cut the historical plan over to V2")
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,
			sale_price_cents,original_price_cents,is_default,is_visible,enabled,status
		) values($1,$2,$3,$1,'legacy cutover','NORMAL','WECHAT_VIRTUAL','PRODUCTION',
			99600,99600,false,true,false,'INACTIVE')
	`, pricePlanID, planID, versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
			published,enabled,status,verification_status
		) values($1,'WECHAT_VIRTUAL','PRODUCTION','legacy-guard-offer',$2,'legacy guard',99600,
			'short_series_goods',false,false,'DISABLED','DISABLED')
	`, goodID, "LEGACY_GUARD_GOOD_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,
			enabled,status
		) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',99600,true,'ACTIVE')
	`, bindingID, pricePlanID, goodID); err != nil {
		t.Fatal(err)
	}
	managed, err = service.isManagedMemberAgentPlanRef(ctx, legacyCode)
	if err != nil {
		t.Fatal(err)
	}
	if managed {
		t.Fatal("a non-default V2 price plan prematurely cut the historical plan over to V2")
	}
	if _, err := service.productByCode(ctx, legacyCode); err != nil {
		t.Fatalf("a non-default V2 price plan broke the historical V1 catalog: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_wechat_virtual_goods set
			published=true,enabled=true,status='PUBLISHED',verification_status='MANUALLY_CONFIRMED_PUBLISHED',
			verified_by=$2,verified_at=now(),verification_reason='local cutover fixture',
			verification_snapshot=jsonb_build_object(
				'productId',product_id,'offerId',offer_id,'environment',environment,
				'platformPriceCents',platform_price_cents
			)
		where id=$1
	`, goodID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_price_plans set
			enabled=true,status='ACTIVE',is_default=true,enabled_by=$2,enabled_at=now(),
			change_reason='local cutover fixture'
		where id=$1
	`, pricePlanID, userID); err != nil {
		t.Fatal(err)
	}
	managed, err = service.isManagedMemberAgentPlanRef(ctx, legacyCode)
	if err != nil {
		t.Fatal(err)
	}
	if !managed {
		t.Fatal("a committed public V2 default did not cut the historical plan over to V2")
	}
	_, err = service.createOrderWithCouponAndSession(
		ctx,
		adminUser{ID: userID},
		"",
		legacyCode,
		1,
		"",
		&wechatMiniProgramSession{OpenID: "legacy-guard-openid", SessionKey: "legacy-guard-session"},
	)
	if !errors.Is(err, errPricePlanCreationDisabled) {
		t.Fatalf("managed member plan reached legacy virtual payment path: %v", err)
	}
	var orderCount int
	if countErr := db.QueryRowContext(ctx, `select count(*) from xz_orders where user_id=$1`, userID).Scan(&orderCount); countErr != nil {
		t.Fatal(countErr)
	}
	if orderCount != 0 {
		t.Fatalf("managed member plan created %d legacy orders", orderCount)
	}
}

func TestPriceQuoteRejectsBindingIdentityDrift(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "quote_drift_user_" + suffix
	planID := "plan_member_drift_" + suffix
	versionID := "version_member_drift_" + suffix
	pricePlanID := "price_member_drift_" + suffix
	goodAID := "good_member_drift_a_" + suffix
	goodBID := "good_member_drift_b_" + suffix
	bindingID := "binding_member_drift_" + suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_users(id,email,name,role,status,created_at,updated_at)
		values($1,$1||'@example.test',$1,'MEMBER','ACTIVE',now(),now());
	`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active)
		values($1,$1,'drift member','MEMBER_PACKAGE',true);
	`, planID); err != nil {
		t.Fatal(err)
	}
	legacyCode := "LEGACY_DRIFT_" + suffix
	legacyMappingID := "legacy_drift_mapping_" + suffix
	if _, err := db.ExecContext(ctx, `
		update xz_plans set payment_product_code=$2,price_cents=99600,product_type='MEMBERSHIP' where id=$1
	`, planID, legacyCode); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_product_mappings(
			id,plan_id,offer_id,wechat_product_id,mode,env,enabled
		) values($2,$1,'legacy-drift-offer','LEGACY_DRIFT_PRODUCT','short_series_goods',0,true)
	`, planID, legacyMappingID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,
			token_amount,duration_days,status
		) values($1,$2,1,'MEMBER',
			'{"memberLevel":"PRO","tokenAmount":100,"durationDays":30}'::jsonb,
			'PRO',100,30,'ACTIVE')
	`, versionID, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,
			sale_price_cents,original_price_cents,is_default,is_visible,enabled,status
		) values($1,$2,$3,$1,'drift price','NORMAL','WECHAT_VIRTUAL','PRODUCTION',
			100,100,true,true,true,'ACTIVE')
	`, pricePlanID, planID, versionID); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id        string
		productID string
	}{{goodAID, "DRIFT_PRODUCT_A_" + suffix}, {goodBID, "DRIFT_PRODUCT_B_" + suffix}} {
		if _, err := db.ExecContext(ctx, `
			insert into xz_wechat_virtual_goods(
				id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
				published,enabled,status,verification_status,verified_by,verified_at,
				verification_reason,verification_snapshot
			) values($1,'WECHAT_VIRTUAL','PRODUCTION','drift-offer',$2,'drift good',100,
				'short_series_goods',true,true,'PUBLISHED','MANUALLY_CONFIRMED_PUBLISHED',
				'test-operator',now(),'manual test confirmation',jsonb_build_object(
					'productId',$2::text,'offerId','drift-offer','environment','PRODUCTION',
					'platformPriceCents',100
				))
		`, item.id, item.productID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,
			provider_price_snapshot_cents,enabled,status
		) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE')
	`, bindingID, pricePlanID, goodAID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_payment_records where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_orders where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_order_price_quotes where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_price_plan_payment_bindings where id=$1`, bindingID)
		_, _ = db.Exec(`delete from xz_wechat_virtual_goods where id in($1,$2)`, goodAID, goodBID)
		_, _ = db.Exec(`delete from xz_price_plans where id=$1`, pricePlanID)
		_, _ = db.Exec(`delete from xz_plan_versions where id=$1`, versionID)
		_, _ = db.Exec(`delete from xz_wechat_virtual_product_mappings where id=$1`, legacyMappingID)
		_, _ = db.Exec(`delete from xz_plans where id=$1`, planID)
		_, _ = db.Exec(`delete from xz_users where id=$1`, userID)
	})

	service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{
		Enabled: true, Env: 0, OfferID: "drift-offer", AppKey: "local-key", NotifyToken: "local-token",
		Mode: "short_series_goods", AppID: "local-app", AppSecret: "local-secret",
		PricePlanCreationEnabled: true, SnapshotV2FulfillmentEnabled: true,
	}}
	quoteBlockingTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer quoteBlockingTx.Rollback()
	if _, err := quoteBlockingTx.ExecContext(ctx, `
		update xz_wechat_virtual_goods set enabled=false,published=false,status='DISABLED' where id=$1
	`, goodAID); err != nil {
		t.Fatal(err)
	}
	type quoteIssueResult struct{ err error }
	issueResult := make(chan quoteIssueResult, 1)
	go func() {
		_, issueErr := service.issuePriceQuote(context.Background(), adminUser{ID: userID}, "", planID, "", pricePlanEntryPublic)
		issueResult <- quoteIssueResult{err: issueErr}
	}()
	select {
	case result := <-issueResult:
		t.Fatalf("quote issuance did not wait for the concurrent goods update: %v", result.err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := quoteBlockingTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-issueResult:
		if !errors.Is(result.err, errPriceQuoteConfigurationChanged) {
			t.Fatalf("quote issuance survived a concurrent goods disable: %v", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quote issuance remained blocked after the goods update committed")
	}
	if _, err := db.ExecContext(ctx, `
		update xz_wechat_virtual_goods set enabled=true,published=true,status='PUBLISHED',
			verification_status='MANUALLY_CONFIRMED_PUBLISHED' where id=$1
	`, goodAID); err != nil {
		t.Fatal(err)
	}
	quote, err := service.issuePriceQuote(ctx, adminUser{ID: userID}, "", planID, "", pricePlanEntryPublic)
	if err != nil {
		t.Fatal(err)
	}
	products, err := service.listProducts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var projected *virtualPaymentProduct
	for index := range products {
		if products[index].PlanID == planID {
			projected = &products[index]
			break
		}
	}
	if projected == nil {
		t.Fatal("active V2 plan was absent from the payment catalog")
	}
	if projected.PriceCents != 100 || projected.WeChatProductID != "DRIFT_PRODUCT_A_"+suffix ||
		projected.OfferID != "drift-offer" {
		t.Fatalf("V2 catalog still projected legacy price or mapping: %+v", *projected)
	}

	blockingTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blockingTx.Rollback()
	if _, err := blockingTx.ExecContext(ctx, `
		update xz_wechat_virtual_goods set enabled=false,published=false,status='DISABLED' where id=$1
	`, goodAID); err != nil {
		t.Fatal(err)
	}
	type quoteLoadResult struct{ err error }
	loadResult := make(chan quoteLoadResult, 1)
	go func() {
		orderTx, beginErr := db.BeginTx(context.Background(), nil)
		if beginErr != nil {
			loadResult <- quoteLoadResult{err: beginErr}
			return
		}
		defer orderTx.Rollback()
		_, _, loadErr := loadPriceQuoteForUpdate(context.Background(), orderTx, quote.QuoteID)
		loadResult <- quoteLoadResult{err: loadErr}
	}()
	select {
	case result := <-loadResult:
		t.Fatalf("quote validation did not wait for the concurrent goods update: %v", result.err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := blockingTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-loadResult:
		if !errors.Is(result.err, errPriceQuoteConfigurationChanged) {
			t.Fatalf("concurrent goods disable was not rejected after lock acquisition: %v", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quote validation remained blocked after the goods update committed")
	}
	if _, err := db.ExecContext(ctx, `
		update xz_wechat_virtual_goods set enabled=true,published=true,status='PUBLISHED',
			verification_status='MANUALLY_CONFIRMED_PUBLISHED' where id=$1
	`, goodAID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plan_payment_bindings set wechat_good_id=$2 where id=$1`, bindingID, goodBID); err != nil {
		t.Fatal(err)
	}
	_, err = service.createOrderFromPriceQuote(
		ctx,
		adminUser{ID: userID},
		"",
		quote.QuoteID,
		&wechatMiniProgramSession{OpenID: "drift-openid", SessionKey: "drift-session"},
	)
	if !errors.Is(err, errPriceQuoteConfigurationChanged) {
		t.Fatalf("quote survived payment-binding identity drift: %v", err)
	}
	var orderCount int
	if countErr := db.QueryRowContext(ctx, `select count(*) from xz_orders where user_id=$1`, userID).Scan(&orderCount); countErr != nil {
		t.Fatal(countErr)
	}
	if orderCount != 0 {
		t.Fatalf("binding identity drift created %d orders", orderCount)
	}
}
