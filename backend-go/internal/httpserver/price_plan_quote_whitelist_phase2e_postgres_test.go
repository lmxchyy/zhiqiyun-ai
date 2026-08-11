package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"
)

type phase2EQuoteFixture struct {
	db                       *sql.DB
	ctx                      context.Context
	service                  *virtualPaymentService
	session                  *wechatMiniProgramSession
	suffix                   string
	userID                   string
	otherUserID              string
	planID                   string
	versionID                string
	pricePlanID              string
	goodID                   string
	bindingID                string
	whitelistID              string
	whitelistInitialRevision int64
}

func TestPhase2ETestQuoteWhitelistPinAndCheckoutRevalidation(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := newPhase2EQuoteFixture(t, ctx, db)
	buyer := adminUser{ID: fixture.userID}

	validQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	var pinnedID string
	var pinnedRevision int64
	var pinnedCheckedAt time.Time
	if err := db.QueryRowContext(ctx, `
		select whitelist_entry_id,whitelist_revision,whitelist_checked_at
		from xz_order_price_quotes where quote_token_hash=$1
	`, hashSensitiveIdentifier(validQuote.QuoteID)).Scan(&pinnedID, &pinnedRevision, &pinnedCheckedAt); err != nil {
		t.Fatal(err)
	}
	if pinnedID != fixture.whitelistID || pinnedRevision != fixture.whitelistInitialRevision || pinnedCheckedAt.IsZero() {
		t.Fatalf("persisted whitelist pin=%s/%d/%s want=%s/%d/non-zero", pinnedID, pinnedRevision, pinnedCheckedAt, fixture.whitelistID, fixture.whitelistInitialRevision)
	}

	if _, err := db.ExecContext(ctx, `update xz_price_plan_user_whitelist set reason='reason-only revision increase' where id=$1`, fixture.whitelistID); err != nil {
		t.Fatal(err)
	}
	var currentRevision int64
	if err := db.QueryRowContext(ctx, `select revision from xz_price_plan_user_whitelist where id=$1`, fixture.whitelistID).Scan(&currentRevision); err != nil {
		t.Fatal(err)
	}
	if currentRevision <= pinnedRevision {
		t.Fatalf("whitelist reason update did not increase revision: before=%d after=%d", pinnedRevision, currentRevision)
	}
	if _, err := fixture.service.createOrderFromPriceQuote(ctx, buyer, "", validQuote.QuoteID, fixture.session); err != nil {
		t.Fatalf("same pinned whitelist remained eligible after reason-only revision increase: %v", err)
	}

	disabledQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_price_plan_user_whitelist
		set lifecycle_status='DISABLED',enabled=false,disabled_by='phase2e-test',disabled_at=now()
		where id=$1
	`, fixture.whitelistID); err != nil {
		t.Fatal(err)
	}
	assertPhase2EQuoteRejectedWithoutSideEffects(t, fixture, disabledQuote.QuoteID, errPricePlanNotEligible)
	if _, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest); !errors.Is(err, errPricePlanNotEligible) {
		t.Fatalf("disabled whitelist issued a TEST quote: %v", err)
	}

	replacementID := "whitelist_replacement_" + fixture.suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_user_whitelist(
			id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
		) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '2 hours','replacement','phase2e-test','phase2e-test')
	`, replacementID, fixture.pricePlanID, fixture.userID); err != nil {
		t.Fatal(err)
	}
	assertPhase2EQuoteRejectedWithoutSideEffects(t, fixture, disabledQuote.QuoteID, errPricePlanNotEligible)

	legacyUnpinnedQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `alter table xz_order_price_quotes disable trigger trg_xz_order_price_quotes_whitelist_pin_100`); err != nil {
		t.Fatal(err)
	}
	triggerEnabled := false
	t.Cleanup(func() {
		if !triggerEnabled {
			_, _ = db.Exec(`alter table xz_order_price_quotes enable trigger trg_xz_order_price_quotes_whitelist_pin_100`)
		}
	})
	if _, err := db.ExecContext(ctx, `
		update xz_order_price_quotes
		set whitelist_entry_id=null,whitelist_revision=null,whitelist_checked_at=null
		where quote_token_hash=$1
	`, hashSensitiveIdentifier(legacyUnpinnedQuote.QuoteID)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `alter table xz_order_price_quotes enable trigger trg_xz_order_price_quotes_whitelist_pin_100`); err != nil {
		t.Fatal(err)
	}
	triggerEnabled = true
	assertPhase2EQuoteRejectedWithoutSideEffects(t, fixture, legacyUnpinnedQuote.QuoteID, errPricePlanNotEligible)

	revisionAheadQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `alter table xz_order_price_quotes disable trigger trg_xz_order_price_quotes_whitelist_pin_100`); err != nil {
		t.Fatal(err)
	}
	triggerEnabled = false
	if _, err := db.ExecContext(ctx, `
		update xz_order_price_quotes set whitelist_revision=whitelist_revision+100
		where quote_token_hash=$1
	`, hashSensitiveIdentifier(revisionAheadQuote.QuoteID)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `alter table xz_order_price_quotes enable trigger trg_xz_order_price_quotes_whitelist_pin_100`); err != nil {
		t.Fatal(err)
	}
	triggerEnabled = true
	assertPhase2EQuoteRejectedWithoutSideEffects(t, fixture, revisionAheadQuote.QuoteID, errPricePlanNotEligible)

	futureQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_price_plan_user_whitelist
		set effective_at=now()+interval '1 hour',expires_at=now()+interval '2 hours'
		where id=$1
	`, replacementID); err != nil {
		t.Fatal(err)
	}
	assertPhase2EQuoteRejectedWithoutSideEffects(t, fixture, futureQuote.QuoteID, errPricePlanNotEligible)
	if _, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest); !errors.Is(err, errPricePlanNotEligible) {
		t.Fatalf("not-yet-effective whitelist issued a TEST quote: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		update xz_price_plan_user_whitelist
		set effective_at=now()-interval '1 hour',expires_at=now()+interval '1 hour'
		where id=$1
	`, replacementID); err != nil {
		t.Fatal(err)
	}
	expiringQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_price_plan_user_whitelist set expires_at=now()-interval '1 second' where id=$1
	`, replacementID); err != nil {
		t.Fatal(err)
	}
	assertPhase2EQuoteRejectedWithoutSideEffects(t, fixture, expiringQuote.QuoteID, errPricePlanNotEligible)
	if _, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest); !errors.Is(err, errPricePlanNotEligible) {
		t.Fatalf("expired whitelist issued a TEST quote: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		update xz_price_plan_user_whitelist set lifecycle_status='EXPIRED',enabled=false where id=$1
	`, replacementID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest); !errors.Is(err, errPricePlanNotEligible) {
		t.Fatalf("terminal whitelist records issued a TEST quote: %v", err)
	}

	concurrentWhitelistID := "whitelist_concurrent_" + fixture.suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_user_whitelist(
			id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
		) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '2 hours','concurrent','phase2e-test','phase2e-test')
	`, concurrentWhitelistID, fixture.pricePlanID, fixture.userID); err != nil {
		t.Fatal(err)
	}
	concurrentQuote, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, fixture.pricePlanID, pricePlanEntryTest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.createOrderFromPriceQuote(ctx, adminUser{ID: fixture.otherUserID}, "", concurrentQuote.QuoteID, fixture.session); !errors.Is(err, errPriceQuoteForbidden) {
		t.Fatalf("cross-user quote did not return PRICE_QUOTE_FORBIDDEN first: %v", err)
	}

	blockingTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blockingTx.Rollback()
	if err := blockingTx.QueryRowContext(ctx, `select id from xz_plans where id=$1 for update`, fixture.planID).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
	if _, err := blockingTx.ExecContext(ctx, `
		update xz_price_plan_user_whitelist
		set lifecycle_status='DISABLED',enabled=false,disabled_by='phase2e-test',disabled_at=now()
		where id=$1
	`, concurrentWhitelistID); err != nil {
		t.Fatal(err)
	}
	orderResult := make(chan error, 1)
	go func() {
		_, createErr := fixture.service.createOrderFromPriceQuote(context.Background(), buyer, "", concurrentQuote.QuoteID, fixture.session)
		orderResult <- createErr
	}()
	select {
	case createErr := <-orderResult:
		t.Fatalf("checkout did not serialize behind whitelist disable: %v", createErr)
	case <-time.After(250 * time.Millisecond):
	}
	if err := blockingTx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case createErr := <-orderResult:
		if !errors.Is(createErr, errPricePlanNotEligible) {
			t.Fatalf("serialized checkout error=%v want PRICE_PLAN_NOT_ELIGIBLE", createErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("checkout remained blocked after whitelist disable committed")
	}
	assertPhase2EQuoteStateHasNoOrder(t, fixture, concurrentQuote.QuoteID)
}

func TestPhase2EGiftPointsRuntimeFailClosedAtQuoteAndCheckout(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := newPhase2EQuoteFixture(t, ctx, db)
	pricePlanID := "price_gift_points_" + fixture.suffix
	goodID := "good_gift_points_" + fixture.suffix
	bindingID := "binding_gift_points_" + fixture.suffix
	whitelistID := "whitelist_gift_points_" + fixture.suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
			sale_price_cents,original_price_cents,bonus_points,audience_type,audience_rule,
			is_default,is_visible,enabled,status
		) values($1,$2,$3,$1,'gift points anomaly','TEST','WECHAT_VIRTUAL','PRODUCTION','CNY',
			100,100,1,'TEST','{}'::jsonb,false,false,true,'ACTIVE')
	`, pricePlanID, fixture.planID, fixture.versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
			published,enabled,status,verification_status,verified_by,verified_at,verification_reason,verification_snapshot
		) values($1,'WECHAT_VIRTUAL','PRODUCTION','offer',$1,'gift points good',100,'short_series_goods',
			true,true,'PUBLISHED','MANUALLY_CONFIRMED_PUBLISHED','phase2e-test',now(),'fixture',
			jsonb_build_object('productId',$1::text,'offerId','offer','environment','PRODUCTION','platformPriceCents',100))
	`, goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status
		) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE')
	`, bindingID, pricePlanID, goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_user_whitelist(
			id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
		) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '1 hour','gift points fixture','phase2e-test','phase2e-test')
	`, whitelistID, pricePlanID, fixture.userID); err != nil {
		t.Fatal(err)
	}

	buyer := adminUser{ID: fixture.userID}
	if _, err := fixture.service.issuePriceQuote(ctx, buyer, "", fixture.planID, pricePlanID, pricePlanEntryTest); !errors.Is(err, errPricePlanGiftPointsFulfillmentUnavailable) {
		t.Fatalf("giftPoints anomaly issued a quote: %v", err)
	}
	var quoteCount int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_order_price_quotes where price_plan_id=$1`, pricePlanID).Scan(&quoteCount); err != nil {
		t.Fatal(err)
	}
	if quoteCount != 0 {
		t.Fatalf("giftPoints quote rejection persisted %d quotes", quoteCount)
	}

	quoteToken := "QUOTE_GIFT_" + fixture.suffix
	quoteID := "quote_gift_points_" + fixture.suffix
	var whitelistRevision int64
	if err := db.QueryRowContext(ctx, `select revision from xz_price_plan_user_whitelist where id=$1`, whitelistID).Scan(&whitelistRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_order_price_quotes(
			id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
			entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
			offer_id,wechat_product_id,payment_mode,currency,bonus_points,bonus_tokens,rights_snapshot,
			commission_rule_version,commission_snapshot,snapshot_version,status,expires_at,
			whitelist_entry_id,whitelist_revision,whitelist_checked_at
		) values($1,$2,$3,$4,$5,$6,$7,$8,$9,
			'TEST',100,100,100,'WECHAT_VIRTUAL','PRODUCTION','offer',$9,'short_series_goods','CNY',1,0,
			'{"memberLevel":"PRO","tokenAmount":100,"durationDays":30,"pointsAmount":1}'::jsonb,
			'','{}'::jsonb,2,'AVAILABLE',now()+interval '5 minutes',$10,$11,now())
	`, quoteID, hashSensitiveIdentifier(quoteToken), "personal:"+fixture.userID, fixture.userID, fixture.planID, fixture.versionID,
		pricePlanID, bindingID, goodID, whitelistID, whitelistRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.createOrderFromPriceQuote(ctx, buyer, "", quoteToken, fixture.session); !errors.Is(err, errPricePlanGiftPointsFulfillmentUnavailable) {
		t.Fatalf("giftPoints anomaly reached payment signing: %v", err)
	}
	assertPhase2EQuoteStateHasNoOrder(t, fixture, quoteToken)
}

func newPhase2EQuoteFixture(t *testing.T, ctx context.Context, db *sql.DB) phase2EQuoteFixture {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	fixture := phase2EQuoteFixture{
		db: db, ctx: ctx, suffix: suffix,
		userID: "quote_user_" + suffix, otherUserID: "quote_other_" + suffix,
		planID: "plan_member_quote_" + suffix, versionID: "version_member_quote_" + suffix,
		pricePlanID: "price_plan_test_quote_" + suffix, goodID: "good_test_quote_" + suffix,
		bindingID: "binding_test_quote_" + suffix, whitelistID: "whitelist_test_quote_" + suffix,
	}
	for _, userID := range []string{fixture.userID, fixture.otherUserID} {
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,created_at,updated_at)
			values($1,$1||'@example.test',$1,'MEMBER','ACTIVE',now(),now())
		`, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active)
		values($1,$1,'Phase 2E TEST quote plan','MEMBER_PACKAGE',true)
	`, fixture.planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,duration_days,
			commission_rule_version,commission_snapshot,status
		) values($1,$2,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"durationDays":30}'::jsonb,
			'PRO',100,30,'','{}'::jsonb,'ACTIVE')
	`, fixture.versionID, fixture.planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
			sale_price_cents,original_price_cents,bonus_points,bonus_tokens,audience_type,audience_rule,
			is_default,is_visible,enabled,status
		) values($1,$2,$3,$1,'Phase 2E TEST quote','TEST','WECHAT_VIRTUAL','PRODUCTION','CNY',
			100,100,0,0,'TEST','{}'::jsonb,false,false,true,'ACTIVE')
	`, fixture.pricePlanID, fixture.planID, fixture.versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
			published,enabled,status,verification_status,verified_by,verified_at,verification_reason,verification_snapshot
		) values($1,'WECHAT_VIRTUAL','PRODUCTION','offer',$1,'Phase 2E TEST good',100,'short_series_goods',
			true,true,'PUBLISHED','MANUALLY_CONFIRMED_PUBLISHED','phase2e-test',now(),'fixture',
			jsonb_build_object('productId',$1::text,'offerId','offer','environment','PRODUCTION','platformPriceCents',100))
	`, fixture.goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status
		) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE')
	`, fixture.bindingID, fixture.pricePlanID, fixture.goodID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_user_whitelist(
			id,price_plan_id,user_id,enabled,lifecycle_status,effective_at,expires_at,reason,created_by,updated_by
		) values($1,$2,$3,true,'ACTIVE',now()-interval '1 minute',now()+interval '2 hours','initial','phase2e-test','phase2e-test')
	`, fixture.whitelistID, fixture.pricePlanID, fixture.userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select revision from xz_price_plan_user_whitelist where id=$1`, fixture.whitelistID).Scan(&fixture.whitelistInitialRevision); err != nil {
		t.Fatal(err)
	}
	fixture.service = &virtualPaymentService{
		db: db,
		cfg: virtualPaymentConfig{
			Enabled: true, Env: 0, OfferID: "runtime-offer", AppKey: "test-app-key", NotifyToken: "test-token",
			Mode: "short_series_goods", AppID: "test-app", AppSecret: "test-secret",
			PricePlanCreationEnabled: true, PricePlanTestEntryEnabled: true, SnapshotV2FulfillmentEnabled: true,
		},
	}
	fixture.session = &wechatMiniProgramSession{OpenID: "openid_" + suffix, SessionKey: "session_" + suffix}
	return fixture
}

func assertPhase2EQuoteRejectedWithoutSideEffects(t *testing.T, fixture phase2EQuoteFixture, quoteToken string, want error) {
	t.Helper()
	_, err := fixture.service.createOrderFromPriceQuote(fixture.ctx, adminUser{ID: fixture.userID}, "", quoteToken, fixture.session)
	if !errors.Is(err, want) {
		t.Fatalf("checkout error=%v want=%v", err, want)
	}
	assertPhase2EQuoteStateHasNoOrder(t, fixture, quoteToken)
}

func assertPhase2EQuoteStateHasNoOrder(t *testing.T, fixture phase2EQuoteFixture, quoteToken string) {
	t.Helper()
	var quoteID, status string
	var consumedOrder sql.NullString
	if err := fixture.db.QueryRowContext(fixture.ctx, `
		select id,status,consumed_order_no from xz_order_price_quotes where quote_token_hash=$1
	`, hashSensitiveIdentifier(quoteToken)).Scan(&quoteID, &status, &consumedOrder); err != nil {
		t.Fatal(err)
	}
	if status != quoteStatusAvailable || consumedOrder.Valid {
		t.Fatalf("rejected quote changed lifecycle: status=%s consumedOrder=%v", status, consumedOrder)
	}
	var orderCount, paymentCount int
	if err := fixture.db.QueryRowContext(fixture.ctx, `select count(*) from xz_orders where price_quote_id=$1`, quoteID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.QueryRowContext(fixture.ctx, `
		select count(*) from xz_payment_records payment
		join xz_orders orders on orders.id=payment.order_id
		where orders.price_quote_id=$1
	`, quoteID).Scan(&paymentCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 || paymentCount != 0 {
		t.Fatalf("rejected quote created partial side effects: orders=%d payments=%d", orderCount, paymentCount)
	}
}
