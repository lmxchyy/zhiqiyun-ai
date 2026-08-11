package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/config"
)

type testWechatGoodEnvelope struct {
	Item struct {
		ID                       string         `json:"id"`
		Channel                  string         `json:"channel"`
		Environment              string         `json:"environment"`
		OfferID                  string         `json:"offerId"`
		ProductID                string         `json:"productId"`
		PlatformPriceCents       int64          `json:"platformPriceCents"`
		Published                bool           `json:"published"`
		Enabled                  bool           `json:"enabled"`
		Status                   string         `json:"status"`
		VerificationStatus       string         `json:"verificationStatus"`
		VerificationSource       string         `json:"verificationSource"`
		PlatformRealtimeVerified bool           `json:"platformRealtimeVerified"`
		Revision                 int64          `json:"revision"`
		VerifiedBy               string         `json:"verifiedBy"`
		VerificationReason       string         `json:"verificationReason"`
		VerificationEvidence     string         `json:"verificationEvidence"`
		VerificationSnapshot     map[string]any `json:"verificationSnapshot"`
	} `json:"item"`
}

type testPaymentBindingEnvelope struct {
	Item struct {
		ID                         string `json:"id"`
		PricePlanID                string `json:"pricePlanId"`
		WeChatGoodID               string `json:"wechatGoodId"`
		Channel                    string `json:"channel"`
		Environment                string `json:"environment"`
		ProviderPriceSnapshotCents int64  `json:"providerPriceSnapshotCents"`
		Enabled                    bool   `json:"enabled"`
		Status                     string `json:"status"`
		Revision                   int64  `json:"revision"`
	} `json:"item"`
}

func TestWechatVirtualGoodsAdminPhase2CPostgresHTTP(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	adminID := "good_admin_" + suffix
	viewerID := "good_viewer_" + suffix
	planID := "plan_member_good_" + suffix
	versionID := "version_member_good_" + suffix
	priceID := "price_member_good_" + suffix
	mismatchPriceID := "price_member_mismatch_" + suffix
	sandboxPriceID := "price_member_sandbox_" + suffix
	unconfirmedPriceID := "price_member_unconfirmed_" + suffix
	expiredPriceID := "price_member_expired_" + suffix
	otherPlanID := "plan_operation_good_" + suffix
	otherVersionID := "version_operation_good_" + suffix
	otherPriceID := "price_operation_good_" + suffix
	legacyMappingID := "legacy_mapping_good_" + suffix

	seedUser := func(id, role string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,created_at,updated_at,raw)
			values($1,$2,$1,$3,'ACTIVE',now(),now(),jsonb_build_object(
				'id',$1::text,'email',$2::text,'name',$1::text,'role',$3::text,'status','ACTIVE'
			))
		`, id, id+"@example.test", role); err != nil {
			t.Fatal(err)
		}
	}
	seedUser(adminID, "SUPER_ADMIN")
	seedUser(viewerID, "ADMIN")
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active)
		values($1,$1,'member good plan','MEMBER_PACKAGE',true),
		      ($2,$2,'operation good plan','OPERATION_CENTER_PACKAGE',true)
	`, planID, otherPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,
			token_amount,duration_days,status
		) values($1,$2,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"durationDays":30}'::jsonb,'PRO',100,30,'ACTIVE'),
		        ($3,$4,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"durationDays":30}'::jsonb,'PRO',100,30,'ACTIVE')
	`, versionID, planID, otherVersionID, otherPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,
			sale_price_cents,original_price_cents,is_default,is_visible,enabled,status
		) values
			($1,$6,$7,$1,'default production','NORMAL','WECHAT_VIRTUAL','PRODUCTION',100,100,true,true,true,'ACTIVE'),
			($2,$6,$7,$2,'mismatch production','ACTIVITY','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,true,'ACTIVE'),
			($3,$6,$7,$3,'sandbox','ACTIVITY','WECHAT_VIRTUAL','SANDBOX',100,100,false,true,true,'ACTIVE'),
			($4,$6,$7,$4,'unconfirmed','ACTIVITY','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,true,'ACTIVE'),
			($5,$6,$7,$5,'expired','ACTIVITY','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,true,'ACTIVE'),
			($8,$9,$10,$8,'unsupported operation','NORMAL','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,true,'ACTIVE')
	`, priceID, mismatchPriceID, sandboxPriceID, unconfirmedPriceID, expiredPriceID,
		planID, versionID, otherPriceID, otherPlanID, otherVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_product_mappings(id,plan_id,wechat_product_id,offer_id,mode,env,enabled)
		values($1,$2,'LEGACY_V2_PRODUCT','legacy-offer','short_series_goods',0,true)
	`, legacyMappingID, planID); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	sessions := newLocalAuthSessions()
	adminToken := "good-admin-token-" + suffix
	viewerToken := "good-viewer-token-" + suffix
	if err := sessions.Put(ctx, adminToken, adminID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Put(ctx, viewerToken, viewerID, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	// Deliberately omit WeChat credentials: 2C is local configuration only.
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler

	jsonBody := func(value any) *bytes.Buffer {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewBuffer(raw)
	}
	decodeCode := func(responseBody *bytes.Buffer) string {
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(responseBody).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload.Code
	}
	createGood := func(environment, productID string, price int64) testWechatGoodEnvelope {
		t.Helper()
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods", jsonBody(map[string]any{
			"channel": "WECHAT_VIRTUAL", "environment": environment, "offerId": "offer-" + suffix,
			"productId": productID, "goodsName": "local good " + productID,
			"platformPriceCents": price, "mode": "short_series_goods", "reason": "create local record",
		}), adminToken)
		if response.Code != http.StatusCreated {
			t.Fatalf("create good status=%d body=%s", response.Code, response.Body.String())
		}
		var payload testWechatGoodEnvelope
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	confirmGood := func(goodID string, revision int64, evidence string, expiresAt *time.Time) testWechatGoodEnvelope {
		t.Helper()
		body := map[string]any{
			"revision": revision, "reason": "confirm local publication record",
			"verificationReason": "operator checked WeChat console", "evidence": evidence,
		}
		if expiresAt != nil {
			body["verificationExpiresAt"] = expiresAt.UTC().Format(time.RFC3339Nano)
		}
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+goodID+"/confirm-published", jsonBody(body), adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("confirm good status=%d body=%s", response.Code, response.Body.String())
		}
		var payload testWechatGoodEnvelope
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	createBinding := func(pricePlanID, goodID string, wantStatus int) (testPaymentBindingEnvelope, string) {
		t.Helper()
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+pricePlanID+"/payment-bindings", jsonBody(map[string]any{
			"wechatGoodId": goodID, "reason": "bind local WeChat good",
		}), adminToken)
		if response.Code != wantStatus {
			t.Fatalf("create binding status=%d want=%d body=%s", response.Code, wantStatus, response.Body.String())
		}
		if response.Code != http.StatusCreated {
			return testPaymentBindingEnvelope{}, decodeCode(response.Body)
		}
		var payload testPaymentBindingEnvelope
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload, ""
	}
	mutateBinding := func(bindingID string, revision int64, enabled bool, wantStatus int) testPaymentBindingEnvelope {
		t.Helper()
		response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+bindingID, jsonBody(map[string]any{
			"revision": revision, "enabled": enabled, "reason": "change binding state",
		}), adminToken)
		if response.Code != wantStatus {
			t.Fatalf("mutate binding status=%d want=%d body=%s", response.Code, wantStatus, response.Body.String())
		}
		var payload testPaymentBindingEnvelope
		if response.Code == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
		}
		return payload
	}

	t.Run("pricing permission protects local goods", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/wechat-virtual-goods", nil, viewerToken)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	exact := createGood("PRODUCTION", "MEMBER_GOOD_"+suffix, 100)
	if exact.Item.Channel != "WECHAT_VIRTUAL" || exact.Item.Environment != "PRODUCTION" ||
		exact.Item.PlatformPriceCents != 100 || exact.Item.Published || exact.Item.Enabled ||
		exact.Item.Status != "DRAFT" || exact.Item.VerificationStatus != "UNCONFIRMED" || exact.Item.Revision != 1 {
		t.Fatalf("unsafe default good: %+v", exact.Item)
	}
	if exact.Item.VerificationSource != "LOCAL_MANUAL_OPERATOR" || exact.Item.PlatformRealtimeVerified {
		t.Fatalf("manual-only verification was presented as realtime: %+v", exact.Item)
	}

	t.Run("invalid and duplicate goods are rejected", func(t *testing.T) {
		invalid := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods", jsonBody(map[string]any{
			"channel": "WECHAT_VIRTUAL", "environment": "STAGING", "offerId": "offer", "productId": "bad",
			"goodsName": "bad", "platformPriceCents": 100, "reason": "invalid environment",
		}), adminToken)
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", invalid.Code, invalid.Body.String())
		}
		duplicate := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods", jsonBody(map[string]any{
			"channel": exact.Item.Channel, "environment": exact.Item.Environment, "offerId": exact.Item.OfferID,
			"productId": exact.Item.ProductID, "goodsName": "duplicate", "platformPriceCents": 100,
			"mode": "short_series_goods", "reason": "duplicate record",
		}), adminToken)
		if duplicate.Code != http.StatusConflict || decodeCode(duplicate.Body) != "WECHAT_GOOD_ALREADY_EXISTS" {
			t.Fatalf("status=%d body=%s", duplicate.Code, duplicate.Body.String())
		}
	})

	t.Run("manual confirmation records actor reason evidence and exact snapshot", func(t *testing.T) {
		missingReason := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID+"/confirm-published", jsonBody(map[string]any{
			"revision": 1, "verificationReason": "operator checked WeChat console",
		}), adminToken)
		if missingReason.Code != http.StatusBadRequest || decodeCode(missingReason.Body) != "REASON_REQUIRED" {
			t.Fatalf("status=%d body=%s", missingReason.Code, missingReason.Body.String())
		}
		missingVerificationReason := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID+"/confirm-published", jsonBody(map[string]any{
			"revision": 1, "reason": "confirm local publication record",
		}), adminToken)
		if missingVerificationReason.Code != http.StatusBadRequest || decodeCode(missingVerificationReason.Body) != "WECHAT_GOOD_VERIFICATION_REASON_REQUIRED" {
			t.Fatalf("status=%d body=%s", missingVerificationReason.Code, missingVerificationReason.Body.String())
		}
		blankVerificationReason := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID+"/confirm-published", jsonBody(map[string]any{
			"revision": 1, "reason": "confirm local publication record", "verificationReason": "  \t ",
		}), adminToken)
		if blankVerificationReason.Code != http.StatusBadRequest || decodeCode(blankVerificationReason.Body) != "WECHAT_GOOD_VERIFICATION_REASON_REQUIRED" {
			t.Fatalf("status=%d body=%s", blankVerificationReason.Code, blankVerificationReason.Body.String())
		}
		unknownConfirmationField := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID+"/confirm-published", jsonBody(map[string]any{
			"revision": 1, "reason": "confirm local publication record", "verificationReason": "operator checked WeChat console",
			"changeReason": "unsupported alias",
		}), adminToken)
		if unknownConfirmationField.Code != http.StatusBadRequest {
			t.Fatalf("strict JSON accepted unknown confirmation field: status=%d body=%s", unknownConfirmationField.Code, unknownConfirmationField.Body.String())
		}
		exact = confirmGood(exact.Item.ID, 1, "ticket-"+suffix, nil)
		if !exact.Item.Published || !exact.Item.Enabled || exact.Item.Status != "PUBLISHED" ||
			exact.Item.VerificationStatus != "MANUALLY_CONFIRMED_PUBLISHED" || exact.Item.Revision != 2 ||
			exact.Item.VerifiedBy != adminID || exact.Item.VerificationReason != "operator checked WeChat console" ||
			exact.Item.VerificationEvidence != "ticket-"+suffix {
			t.Fatalf("incomplete manual confirmation: %+v", exact.Item)
		}
		if exact.Item.VerificationSnapshot["productId"] != exact.Item.ProductID ||
			exact.Item.VerificationSnapshot["offerId"] != exact.Item.OfferID ||
			exact.Item.VerificationSnapshot["environment"] != "PRODUCTION" ||
			exact.Item.VerificationSnapshot["platformPriceCents"] != float64(100) {
			t.Fatalf("wrong confirmation snapshot: %+v", exact.Item.VerificationSnapshot)
		}
		var auditChangeReason, auditVerificationReason, auditEvidence string
		if err := db.QueryRowContext(ctx, `
			select change_reason,coalesce(metadata->>'verificationReason',''),coalesce(metadata->>'evidence','')
			from xz_audit_logs
			where action='wechat_good.confirm_published' and resource_id=$1
			order by created_at desc,id desc limit 1
		`, exact.Item.ID).Scan(&auditChangeReason, &auditVerificationReason, &auditEvidence); err != nil {
			t.Fatal(err)
		}
		if auditChangeReason != "confirm local publication record" || auditVerificationReason != "operator checked WeChat console" || auditEvidence != "ticket-"+suffix {
			t.Fatalf("confirmation audit fields were conflated: changeReason=%q verificationReason=%q evidence=%q", auditChangeReason, auditVerificationReason, auditEvidence)
		}
		stale := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID+"/confirm-published", jsonBody(map[string]any{
			"revision": 1, "reason": "stale confirmation", "verificationReason": "stale verification evidence",
		}), adminToken)
		if stale.Code != http.StatusConflict || decodeCode(stale.Body) != "REVISION_CONFLICT" {
			t.Fatalf("status=%d body=%s", stale.Code, stale.Body.String())
		}
	})

	t.Run("binding derives provider price and controls new quotes", func(t *testing.T) {
		unknownPrice := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+priceID+"/payment-bindings", jsonBody(map[string]any{
			"wechatGoodId": exact.Item.ID, "providerPriceSnapshotCents": 1, "reason": "client must not set price",
		}), adminToken)
		if unknownPrice.Code != http.StatusBadRequest {
			t.Fatalf("client provider price accepted: status=%d body=%s", unknownPrice.Code, unknownPrice.Body.String())
		}
		binding, _ := createBinding(priceID, exact.Item.ID, http.StatusCreated)
		if binding.Item.ProviderPriceSnapshotCents != 100 || binding.Item.Enabled || binding.Item.Status != "DRAFT" || binding.Item.Revision != 1 {
			t.Fatalf("wrong draft binding: %+v", binding.Item)
		}
		legacySource, err := store.CreateAdminOrder(adminOrderMutation{
			UserID: adminID, PlanID: planID, AmountCents: 100, Status: "PENDING", PaymentMethod: "WECHAT",
		})
		if err != nil {
			t.Fatalf("prepare pre-cutover legacy order: %v", err)
		}
		legacyTx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		managed, err := legacyOrderManagedV2Tx(ctx, legacyTx, planID, "PRODUCTION")
		if err != nil || managed {
			legacyTx.Rollback()
			t.Fatalf("draft binding was treated as cut over: managed=%t err=%v", managed, err)
		}
		type activationResult struct {
			item pricePlanPaymentBindingAdminView
			err  error
		}
		activated := make(chan activationResult, 1)
		enableBinding := true
		go func() {
			item, activateErr := store.updatePricePlanPaymentBinding(context.Background(), binding.Item.ID,
				pricePlanPaymentBindingMutation{Revision: 1, Enabled: &enableBinding, Reason: "activate after legacy transaction"},
				adminID, "SUPER_ADMIN")
			activated <- activationResult{item: item, err: activateErr}
		}()
		select {
		case result := <-activated:
			legacyTx.Rollback()
			t.Fatalf("binding activation bypassed the legacy-order plan lock: %v", result.err)
		case <-time.After(250 * time.Millisecond):
		}
		if err := legacyTx.Commit(); err != nil {
			t.Fatal(err)
		}
		var activation activationResult
		select {
		case activation = <-activated:
			if activation.err != nil {
				t.Fatal(activation.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("binding activation remained blocked after the legacy transaction committed")
		}
		binding.Item.ID = activation.item.ID
		binding.Item.WeChatGoodID = activation.item.WeChatGoodID
		binding.Item.ProviderPriceSnapshotCents = activation.item.ProviderPriceSnapshotCents
		binding.Item.Enabled = activation.item.Enabled
		binding.Item.Status = activation.item.Status
		binding.Item.Revision = activation.item.Revision
		if !binding.Item.Enabled || binding.Item.Status != "ACTIVE" || binding.Item.Revision != 2 {
			t.Fatalf("binding was not activated: %+v", binding.Item)
		}
		legacyOrder := authedRequest(t, handler, http.MethodPost, "/api/v1/orders/create", jsonBody(map[string]any{
			"planId": planID, "amountCents": 100, "paymentMethod": "WECHAT",
			"idempotencyKey": "legacy-v2-order-" + suffix,
		}), adminToken)
		if legacyOrder.Code != http.StatusConflict || decodeCode(legacyOrder.Body) != "MANAGED_PLAN_REQUIRES_PRICE_QUOTE" {
			t.Fatalf("V2 managed plan reached legacy commerce order creation: status=%d body=%s", legacyOrder.Code, legacyOrder.Body.String())
		}
		renew := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/orders/"+legacySource.ID+"/renew", nil, adminToken)
		if renew.Code != http.StatusConflict || decodeCode(renew.Body) != "MANAGED_PLAN_REQUIRES_PRICE_QUOTE" {
			t.Fatalf("V2 managed plan renewed a legacy order: status=%d body=%s", renew.Code, renew.Body.String())
		}
		staleBinding := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
			"revision": 1, "enabled": false, "reason": "stale binding mutation",
		}), adminToken)
		if staleBinding.Code != http.StatusConflict || decodeCode(staleBinding.Body) != "REVISION_CONFLICT" {
			t.Fatalf("status=%d body=%s", staleBinding.Code, staleBinding.Body.String())
		}

		service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{
			Enabled: true, Env: 0, OfferID: exact.Item.OfferID, AppKey: "local-key", NotifyToken: "local-token",
			Mode: "short_series_goods", AppID: "local-app", AppSecret: "local-secret",
			PricePlanCreationEnabled: true, SnapshotV2FulfillmentEnabled: true,
		}}
		quote, err := service.issuePriceQuote(ctx, adminUser{ID: adminID}, "", planID, "", pricePlanEntryPublic)
		if err != nil || quote.AmountCent != 100 {
			t.Fatalf("admin binding did not drive quote: quote=%+v err=%v", quote, err)
		}

		criticalPatch := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID, jsonBody(map[string]any{
			"revision": exact.Item.Revision, "platformPriceCents": 101, "reason": "unsafe active mutation",
		}), adminToken)
		if criticalPatch.Code != http.StatusConflict || decodeCode(criticalPatch.Body) != "WECHAT_GOOD_HAS_PAYMENT_BINDING" {
			t.Fatalf("status=%d body=%s", criticalPatch.Code, criticalPatch.Body.String())
		}

		t.Run("default active binding cannot be disabled", func(t *testing.T) {
			response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
				"revision": binding.Item.Revision, "enabled": false, "reason": "default must be switched first",
			}), adminToken)
			if response.Code != http.StatusConflict || decodeCode(response.Body) != "PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})

		t.Run("default active binding cannot be rebound", func(t *testing.T) {
			replacement := confirmGood(createGood("PRODUCTION", "DEFAULT_REBIND_GOOD_"+suffix, 100).Item.ID, 1, "default-rebind-ticket-"+suffix, nil)
			response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
				"revision": binding.Item.Revision, "wechatGoodId": replacement.Item.ID, "reason": "default must be switched first",
			}), adminToken)
			if response.Code != http.StatusConflict || decodeCode(response.Body) != "PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})

		t.Run("good serving default price plan cannot be disabled", func(t *testing.T) {
			response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+exact.Item.ID+"/disable", jsonBody(map[string]any{
				"revision": exact.Item.Revision, "reason": "default must be switched first",
			}), adminToken)
			if response.Code != http.StatusConflict || decodeCode(response.Body) != "PRICE_PLAN_DEFAULT_DEPENDENCY_DISABLE_FORBIDDEN" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var bindingEnabled, goodEnabled, goodPublished bool
			if err := db.QueryRowContext(ctx, `select enabled from xz_price_plan_payment_bindings where id=$1`, binding.Item.ID).Scan(&bindingEnabled); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRowContext(ctx, `select enabled,published from xz_wechat_virtual_goods where id=$1`, exact.Item.ID).Scan(&goodEnabled, &goodPublished); err != nil {
				t.Fatal(err)
			}
			if !bindingEnabled || !goodEnabled || !goodPublished {
				t.Fatalf("rejected disable changed state: binding=%t goodEnabled=%t goodPublished=%t", bindingEnabled, goodEnabled, goodPublished)
			}
		})
	})

	t.Run("local disable changes status without contacting WeChat", func(t *testing.T) {
		disabled := confirmGood(createGood("PRODUCTION", "DISABLED_GOOD_"+suffix, 100).Item.ID, 1, "", nil)
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+disabled.Item.ID+"/disable", jsonBody(map[string]any{
			"revision": 2, "reason": "retire local good",
		}), adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var payload testWechatGoodEnvelope
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Item.Status != "DISABLED" || payload.Item.VerificationStatus != "DISABLED" || payload.Item.Enabled || payload.Item.Published || payload.Item.Revision != 3 {
			t.Fatalf("wrong disabled good: %+v", payload.Item)
		}
	})

	t.Run("price environment confirmation and managed scope are enforced", func(t *testing.T) {
		mismatch := confirmGood(createGood("PRODUCTION", "MISMATCH_GOOD_"+suffix, 101).Item.ID, 1, "", nil)
		if _, code := createBinding(mismatchPriceID, mismatch.Item.ID, http.StatusConflict); code != "PRICE_PLAN_WECHAT_PRICE_MISMATCH" {
			t.Fatalf("wrong price mismatch code %q", code)
		}
		sandbox := confirmGood(createGood("SANDBOX", "SANDBOX_GOOD_"+suffix, 100).Item.ID, 1, "", nil)
		if _, code := createBinding(mismatchPriceID, sandbox.Item.ID, http.StatusConflict); code != "PRICE_PLAN_PAYMENT_ENV_MISMATCH" {
			t.Fatalf("wrong environment mismatch code %q", code)
		}
		sandboxBinding, _ := createBinding(sandboxPriceID, sandbox.Item.ID, http.StatusCreated)
		sandboxBinding = mutateBinding(sandboxBinding.Item.ID, sandboxBinding.Item.Revision, true, http.StatusOK)
		if _, err := db.ExecContext(ctx, `
			update xz_price_plan_payment_bindings set provider_price_snapshot_cents=101 where id=$1
		`, sandboxBinding.Item.ID); err != nil {
			t.Fatal(err)
		}
		mismatchStatus := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/wechat-virtual-goods/"+sandbox.Item.ID, nil, adminToken)
		if mismatchStatus.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", mismatchStatus.Code, mismatchStatus.Body.String())
		}
		var mismatchPayload testWechatGoodEnvelope
		if err := json.NewDecoder(mismatchStatus.Body).Decode(&mismatchPayload); err != nil {
			t.Fatal(err)
		}
		if mismatchPayload.Item.VerificationStatus != "PRICE_MISMATCH" {
			t.Fatalf("active binding price drift was not surfaced on the good: %+v", mismatchPayload.Item)
		}
		if _, err := db.ExecContext(ctx, `
			update xz_price_plan_payment_bindings set provider_price_snapshot_cents=100 where id=$1
		`, sandboxBinding.Item.ID); err != nil {
			t.Fatal(err)
		}
		var sandboxBindingRevision int64
		if err := db.QueryRowContext(ctx, `select revision from xz_price_plan_payment_bindings where id=$1`, sandboxBinding.Item.ID).Scan(&sandboxBindingRevision); err != nil {
			t.Fatal(err)
		}
		sandboxBinding = mutateBinding(sandboxBinding.Item.ID, sandboxBindingRevision, false, http.StatusOK)
		if sandboxBinding.Item.Enabled || sandboxBinding.Item.Status != "DISABLED" {
			t.Fatalf("non-default binding was not disabled: %+v", sandboxBinding.Item)
		}
		unconfirmed := createGood("PRODUCTION", "UNCONFIRMED_GOOD_"+suffix, 100)
		binding, _ := createBinding(unconfirmedPriceID, unconfirmed.Item.ID, http.StatusCreated)
		criticalPatch := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/wechat-virtual-goods/"+unconfirmed.Item.ID, jsonBody(map[string]any{
			"revision": unconfirmed.Item.Revision, "platformPriceCents": 101, "reason": "binding identity must remain stable",
		}), adminToken)
		if criticalPatch.Code != http.StatusConflict || decodeCode(criticalPatch.Body) != "WECHAT_GOOD_HAS_PAYMENT_BINDING" {
			t.Fatalf("draft binding allowed a critical goods mutation: status=%d body=%s", criticalPatch.Code, criticalPatch.Body.String())
		}
		replacement := confirmGood(createGood("PRODUCTION", "REBOUND_GOOD_"+suffix, 100).Item.ID, 1, "replacement-ticket-"+suffix, nil)
		rebind := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
			"revision": binding.Item.Revision, "wechatGoodId": replacement.Item.ID, "reason": "correct draft binding",
		}), adminToken)
		if rebind.Code != http.StatusOK {
			t.Fatalf("safe draft binding rebind failed: status=%d body=%s", rebind.Code, rebind.Body.String())
		}
		if err := json.NewDecoder(rebind.Body).Decode(&binding); err != nil {
			t.Fatal(err)
		}
		if binding.Item.WeChatGoodID != replacement.Item.ID || binding.Item.Enabled || binding.Item.Status != "DRAFT" || binding.Item.Revision != 2 {
			t.Fatalf("unsafe rebound binding: %+v", binding.Item)
		}
		activation := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
			"revision": binding.Item.Revision, "enabled": true, "reason": "activate corrected binding",
		}), adminToken)
		if activation.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", activation.Code, activation.Body.String())
		}
		stillUnconfirmed, _ := createBinding(mismatchPriceID, unconfirmed.Item.ID, http.StatusCreated)
		activation = authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+stillUnconfirmed.Item.ID, jsonBody(map[string]any{
			"revision": 1, "enabled": true, "reason": "must reject unconfirmed good",
		}), adminToken)
		if activation.Code != http.StatusConflict || decodeCode(activation.Body) != "WECHAT_GOOD_NOT_CONFIRMED" {
			t.Fatalf("status=%d body=%s", activation.Code, activation.Body.String())
		}
		if _, code := createBinding(otherPriceID, exact.Item.ID, http.StatusNotFound); code != "PRICE_PLAN_NOT_MANAGED" {
			t.Fatalf("wrong unmanaged price plan code %q", code)
		}
	})

	t.Run("expired manual confirmation blocks new and already issued quotes", func(t *testing.T) {
		service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{
			Enabled: true, Env: 0, OfferID: exact.Item.OfferID, AppKey: "local-key", NotifyToken: "local-token",
			Mode: "short_series_goods", AppID: "local-app", AppSecret: "local-secret",
			PricePlanCreationEnabled: true, SnapshotV2FulfillmentEnabled: true,
		}}
		quote, err := service.issuePriceQuote(ctx, adminUser{ID: adminID}, "", planID, "", pricePlanEntryPublic)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			update xz_wechat_virtual_goods
			set verified_at=now()-interval '2 hours',verification_expires_at=now()-interval '1 hour'
			where id=$1
		`, exact.Item.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.issuePriceQuote(ctx, adminUser{ID: adminID}, "", planID, "", pricePlanEntryPublic); !errors.Is(err, errPricePlanUnavailable) {
			t.Fatalf("expired confirmation still produced a quote: %v", err)
		}
		if _, err := service.createOrderFromPriceQuote(ctx, adminUser{ID: adminID}, "", quote.QuoteID, &wechatMiniProgramSession{
			OpenID: "expired-quote-openid", SessionKey: "expired-quote-session",
		}); !errors.Is(err, errWechatGoodVerificationExpired) {
			t.Fatalf("already issued quote survived confirmation expiry: %v", err)
		}
	})

	t.Run("expired confirmation blocks activation and payment resolution", func(t *testing.T) {
		expiresAt := time.Now().UTC().Add(time.Hour)
		expired := confirmGood(createGood("PRODUCTION", "EXPIRED_GOOD_"+suffix, 100).Item.ID, 1, "", &expiresAt)
		binding, _ := createBinding(expiredPriceID, expired.Item.ID, http.StatusCreated)
		if _, err := db.ExecContext(ctx, `
			update xz_wechat_virtual_goods
			set verified_at=now()-interval '2 hours', verification_expires_at=now()-interval '1 hour'
			where id=$1
		`, expired.Item.ID); err != nil {
			t.Fatal(err)
		}
		activation := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
			"revision": 1, "enabled": true, "reason": "expired confirmation",
		}), adminToken)
		if activation.Code != http.StatusConflict || decodeCode(activation.Body) != "WECHAT_GOOD_VERIFICATION_EXPIRED" {
			t.Fatalf("status=%d body=%s", activation.Code, activation.Body.String())
		}
	})

	t.Run("legacy mapping cannot mutate a V2 managed plan", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment/virtual/mappings/"+legacyMappingID, jsonBody(map[string]any{
			"wechatProductId": "BYPASS_PRODUCT",
		}), adminToken)
		if response.Code != http.StatusConflict || decodeCode(response.Body) != "MANAGED_PLAN_REQUIRES_PAYMENT_BINDING" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("business audit and default price remain intact", func(t *testing.T) {
		var auditCount int
		if err := db.QueryRowContext(ctx, `
			select count(*) from xz_audit_logs
			where actor_id=$1 and action in(
				'wechat_good.create','wechat_good.confirm_published',
				'price_plan.payment_binding.create','price_plan.payment_binding.activate',
				'price_plan.payment_binding.disable'
			)
		`, adminID).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount < 5 {
			t.Fatalf("business audit count=%d", auditCount)
		}
		var isDefault bool
		if err := db.QueryRowContext(ctx, `select is_default from xz_price_plans where id=$1`, priceID).Scan(&isDefault); err != nil {
			t.Fatal(err)
		}
		if !isDefault {
			t.Fatal("2C changed the default price plan")
		}
	})
}
