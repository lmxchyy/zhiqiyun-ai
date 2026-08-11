package httpserver

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestWechatVirtualGoodReferencesPhase2FPostgresHTTP(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	db, ctx := openPhase2ETestPostgres(t)

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	viewerRole := "PRICING_REFERENCE_VIEWER_" + suffix
	deniedRole := "PRICING_REFERENCE_DENIED_" + suffix
	viewerID := "pricing_reference_viewer_" + suffix
	deniedID := "pricing_reference_denied_" + suffix
	memberPlanID := "plan_reference_member_" + suffix
	agentPlanID := "plan_reference_agent_" + suffix
	operationPlanID := "plan_reference_operation_" + suffix
	mismatchedPlanID := "plan_reference_mismatch_" + suffix
	memberVersionID := "version_reference_member_" + suffix
	agentVersionID := "version_reference_agent_" + suffix
	operationVersionID := "version_reference_operation_" + suffix
	mismatchedVersionID := "version_reference_mismatch_" + suffix
	memberActivePriceID := "price_reference_member_active_" + suffix
	memberDraftPriceID := "price_reference_member_draft_" + suffix
	agentInactivePriceID := "price_reference_agent_inactive_" + suffix
	operationPriceID := "price_reference_operation_" + suffix
	mismatchedPriceID := "price_reference_mismatch_" + suffix
	goodID := "wechat_good_reference_" + suffix
	memberActiveBindingID := "binding_reference_member_active_" + suffix
	memberDraftBindingID := "binding_reference_member_draft_" + suffix
	agentDisabledBindingID := "binding_reference_agent_disabled_" + suffix
	operationBindingID := "binding_reference_operation_" + suffix
	mismatchedBindingID := "binding_reference_mismatch_" + suffix
	quoteIDs := []string{"quote_reference_1_" + suffix, "quote_reference_2_" + suffix}
	orderID := "order_reference_" + suffix

	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_orders where id=$1`, orderID)
		for _, id := range quoteIDs {
			_, _ = db.Exec(`delete from xz_order_price_quotes where id=$1`, id)
		}
		for _, id := range []string{memberActiveBindingID, memberDraftBindingID, agentDisabledBindingID, operationBindingID, mismatchedBindingID} {
			_, _ = db.Exec(`delete from xz_price_plan_payment_bindings where id=$1`, id)
		}
		_, _ = db.Exec(`delete from xz_wechat_virtual_goods where id=$1`, goodID)
		for _, id := range []string{memberActivePriceID, memberDraftPriceID, agentInactivePriceID, operationPriceID, mismatchedPriceID} {
			_, _ = db.Exec(`delete from xz_price_plans where id=$1`, id)
		}
		for _, id := range []string{memberVersionID, agentVersionID, operationVersionID, mismatchedVersionID} {
			_, _ = db.Exec(`delete from xz_plan_versions where id=$1`, id)
		}
		for _, id := range []string{memberPlanID, agentPlanID, operationPlanID, mismatchedPlanID} {
			_, _ = db.Exec(`delete from xz_plans where id=$1`, id)
		}
		_, _ = db.Exec(`delete from xz_role_permissions where role=$1`, viewerRole)
		_, _ = db.Exec(`delete from xz_users where id=$1`, viewerID)
		_, _ = db.Exec(`delete from xz_users where id=$1`, deniedID)
	})

	for _, identity := range []struct{ id, role string }{{viewerID, viewerRole}, {deniedID, deniedRole}} {
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,raw)
			values($1,$1||'@example.test',$1,$2,'ACTIVE',jsonb_build_object(
				'id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role',$2::text,'status','ACTIVE'
			))
		`, identity.id, identity.role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `insert into xz_role_permissions(role,permission) values($1,'pricing:plan:view')`, viewerRole); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active) values
			($1,$1,'Reference member plan','MEMBER_PACKAGE',true),
			($2,$2,'Reference agent plan','AGENT_JOIN_PACKAGE',true),
			($3,$3,'Reference operation plan','OPERATION_CENTER_PACKAGE',true),
			($4,$4,'Reference mismatched plan','MEMBER_PACKAGE',true)
	`, memberPlanID, agentPlanID, operationPlanID, mismatchedPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,agent_level,token_amount,duration_days,status
		) values
			($1,$2,1,'MEMBER','{"memberLevel":"PRO"}'::jsonb,'PRO',null,0,30,'ACTIVE'),
			($3,$4,1,'AGENT','{"agentLevel":"L1"}'::jsonb,null,'L1',0,365,'ACTIVE'),
			($5,$6,1,'MEMBER','{"memberLevel":"PRO"}'::jsonb,'PRO',null,0,30,'ACTIVE'),
			($7,$8,1,'AGENT','{"agentLevel":"L1"}'::jsonb,null,'L1',0,365,'ACTIVE')
	`, memberVersionID, memberPlanID, agentVersionID, agentPlanID,
		operationVersionID, operationPlanID, mismatchedVersionID, mismatchedPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,
			sale_price_cents,original_price_cents,is_default,is_visible,enabled,status
		) values
			($1,$6,$7,$1,'Member active','NORMAL','WECHAT_VIRTUAL','PRODUCTION',100,100,true,true,true,'ACTIVE'),
			($2,$6,$7,$2,'Member draft','ACTIVITY','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,false,'DRAFT'),
			($3,$8,$9,$3,'Agent inactive','ACTIVITY','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,false,'INACTIVE'),
			($4,$10,$11,$4,'Operation excluded','NORMAL','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,true,'ACTIVE'),
			($5,$12,$13,$5,'Mismatched excluded','NORMAL','WECHAT_VIRTUAL','PRODUCTION',100,100,false,true,true,'ACTIVE')
	`, memberActivePriceID, memberDraftPriceID, agentInactivePriceID, operationPriceID, mismatchedPriceID,
		memberPlanID, memberVersionID, agentPlanID, agentVersionID, operationPlanID, operationVersionID,
		mismatchedPlanID, mismatchedVersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_wechat_virtual_goods(
			id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
			published,enabled,status,verification_status
		) values($1,'WECHAT_VIRTUAL','PRODUCTION',$2,$3,'Reference good',100,'short_series_goods',false,false,'DRAFT','UNCONFIRMED')
	`, goodID, "offer-"+suffix, "product-"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plan_payment_bindings(
			id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,enabled,status
		) values
			($1,$6,$11,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE'),
			($2,$7,$11,'WECHAT_VIRTUAL','PRODUCTION',100,false,'DRAFT'),
			($3,$8,$11,'WECHAT_VIRTUAL','PRODUCTION',100,false,'DISABLED'),
			($4,$9,$11,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE'),
			($5,$10,$11,'WECHAT_VIRTUAL','PRODUCTION',100,true,'ACTIVE')
	`, memberActiveBindingID, memberDraftBindingID, agentDisabledBindingID, operationBindingID, mismatchedBindingID,
		memberActivePriceID, memberDraftPriceID, agentInactivePriceID, operationPriceID, mismatchedPriceID, goodID); err != nil {
		t.Fatal(err)
	}
	for _, quoteID := range quoteIDs {
		if _, err := db.ExecContext(ctx, `
			insert into xz_order_price_quotes(
				id,quote_token_hash,tenant_id,user_id,plan_id,plan_version_id,price_plan_id,payment_binding_id,wechat_good_id,
				entry_type,transaction_price_cents,provider_price_snapshot_cents,wechat_goods_price_cents,channel,environment,
				offer_id,wechat_product_id,payment_mode,rights_snapshot,status,expires_at
			) values($1,$1,'tenant_default',$2,$3,$4,$5,$6,$7,'PUBLIC',100,100,100,'WECHAT_VIRTUAL','PRODUCTION',
				$8,$9,'short_series_goods','{}'::jsonb,'AVAILABLE',now()+interval '10 minutes')
		`, quoteID, viewerID, memberPlanID, memberVersionID, memberActivePriceID, memberActiveBindingID, goodID,
			"offer-"+suffix, "product-"+suffix); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_orders(id,user_id,plan_id,amount_cents,status,created_at,price_plan_id)
		values($1,$2,$3,100,'CREATED',now()::text,$4)
	`, orderID, viewerID, memberPlanID, memberActivePriceID); err != nil {
		t.Fatal(err)
	}

	sessions := newLocalAuthSessions()
	viewerToken := "pricing-reference-viewer-token-" + suffix
	deniedToken := "pricing-reference-denied-token-" + suffix
	if err := sessions.Put(ctx, viewerToken, viewerID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Put(ctx, deniedToken, deniedID, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	// No WeChat credential is configured: references are local PostgreSQL reads only.
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler
	path := "/api/v1/admin/wechat-virtual-goods/" + goodID + "/references"

	denied := authedRequest(t, handler, http.MethodGet, path, nil, deniedToken)
	if denied.Code != http.StatusForbidden || decodeWechatReferenceErrorCode(t, denied) != "ADMIN_PERMISSION_DENIED" {
		t.Fatalf("references permission status=%d body=%s", denied.Code, denied.Body.String())
	}

	response := authedRequest(t, handler, http.MethodGet, path, nil, viewerToken)
	if response.Code != http.StatusOK {
		t.Fatalf("references status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 3 || len(payload.Items) != 3 {
		t.Fatalf("references total=%d items=%d want=3", payload.Total, len(payload.Items))
	}
	wantKeys := []string{
		"bindingEnabled", "bindingId", "bindingStatus", "channel", "environment", "isDefault", "orderCount", "planId",
		"planName", "pricePlanCode", "pricePlanId", "pricePlanName", "providerPriceSnapshotCents", "quoteCount", "salePriceCents", "wechatGoodId",
	}
	byBindingID := map[string]map[string]any{}
	for _, item := range payload.Items {
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(keys, wantKeys) {
			t.Fatalf("reference fields=%v want=%v", keys, wantKeys)
		}
		bindingID, _ := item["bindingId"].(string)
		byBindingID[bindingID] = item
	}
	memberActive := byBindingID[memberActiveBindingID]
	if memberActive == nil || memberActive["pricePlanId"] != memberActivePriceID || memberActive["pricePlanCode"] != memberActivePriceID ||
		memberActive["pricePlanName"] != "Member active" || memberActive["planId"] != memberPlanID || memberActive["planName"] != "Reference member plan" ||
		memberActive["isDefault"] != true || memberActive["bindingStatus"] != "ACTIVE" || memberActive["bindingEnabled"] != true ||
		memberActive["salePriceCents"] != float64(100) || memberActive["providerPriceSnapshotCents"] != float64(100) ||
		memberActive["channel"] != "WECHAT_VIRTUAL" || memberActive["environment"] != "PRODUCTION" || memberActive["wechatGoodId"] != goodID ||
		memberActive["quoteCount"] != float64(2) || memberActive["orderCount"] != float64(1) {
		t.Fatalf("wrong active member reference: %+v", memberActive)
	}
	if draft := byBindingID[memberDraftBindingID]; draft == nil || draft["bindingStatus"] != "DRAFT" || draft["bindingEnabled"] != false {
		t.Fatalf("draft binding reference missing: %+v", draft)
	}
	if disabled := byBindingID[agentDisabledBindingID]; disabled == nil || disabled["bindingStatus"] != "DISABLED" || disabled["bindingEnabled"] != false {
		t.Fatalf("disabled agent binding reference missing: %+v", disabled)
	}
	if byBindingID[operationBindingID] != nil || byBindingID[mismatchedBindingID] != nil {
		t.Fatalf("non-V2 or business-type-mismatched references leaked: %+v", byBindingID)
	}

	healthResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/pricing-health", nil, viewerToken)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("pricing health status=%d body=%s", healthResponse.Code, healthResponse.Body.String())
	}
	var health pricingHealthView
	if err := json.NewDecoder(healthResponse.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	healthReferenceCount := -1
	for _, item := range health.WeChatGoods {
		if item.WeChatGoodID == goodID {
			healthReferenceCount = item.ReferenceCount
			break
		}
	}
	if healthReferenceCount != payload.Total {
		t.Fatalf("references total=%d health referenceCount=%d", payload.Total, healthReferenceCount)
	}

	missing := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/wechat-virtual-goods/missing_"+suffix+"/references", nil, viewerToken)
	if missing.Code != http.StatusNotFound || decodeWechatReferenceErrorCode(t, missing) != "WECHAT_GOOD_NOT_FOUND" {
		t.Fatalf("missing good status=%d body=%s", missing.Code, missing.Body.String())
	}
}

func decodeWechatReferenceErrorCode(t *testing.T, response interface{ Result() *http.Response }) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(response.Result().Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.Code
}
