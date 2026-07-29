package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPricePlanAdminPhase2DCreateListAndDetailPostgresHTTP(t *testing.T) {
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

	var migrated bool
	if err := db.QueryRowContext(ctx, `
		select exists(
			select 1 from information_schema.columns
			where table_name='xz_price_plans' and column_name='audience_type'
		)
	`).Scan(&migrated); err != nil || !migrated {
		t.Skip("migration 099 is not applied to the test database")
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	adminID := "price_admin_" + suffix
	viewerID := "price_viewer_" + suffix
	planID := "plan_member_price_" + suffix
	versionID := "version_member_price_" + suffix
	priceCode := "member_launch_" + suffix
	now := time.Now().UTC()

	for _, user := range []struct{ id, role string }{{adminID, "SUPER_ADMIN"}, {viewerID, "ADMIN"}} {
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,created_at,updated_at,raw)
			values($1,$1||'@example.test',$1,$2,'ACTIVE',$3,$3,
			       jsonb_build_object('id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role',$2::text,'status','ACTIVE'))
		`, user.id, user.role, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active,raw)
		values($1,$1,'2D member plan','MEMBER_PACKAGE',true,
		       jsonb_build_object('id',$1::text,'code',$1::text,'name','2D member plan','planType','MEMBER_PACKAGE','active',true))
	`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,
			token_amount,points_amount,duration_days,commission_rule_version,commission_snapshot,
			status,effective_at,created_by,updated_by,change_reason
		) values($1,$2,1,'MEMBER',
			'{"memberLevel":"PRO","tokenAmount":100,"pointsAmount":10,"durationDays":30}'::jsonb,
			'PRO',100,10,30,'commission-v1','{"rules":[]}'::jsonb,
			'ACTIVE',$3,$4,$4,'activate fixture')
	`, versionID, planID, now.Add(-time.Hour), adminID); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	sessions := newMemoryAuthSessions()
	adminToken := "price-admin-token-" + suffix
	viewerToken := "price-viewer-token-" + suffix
	if err := sessions.Put(ctx, adminToken, adminID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Put(ctx, viewerToken, viewerID, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler
	jsonBody := func(value any) *bytes.Buffer {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.NewBuffer(raw)
	}
	decodeErrorCode := func(responseBody *bytes.Buffer) string {
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(responseBody).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload.Code
	}

	create := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/business-plans/"+planID+"/price-plans", jsonBody(map[string]any{
		"revision": 1, "planVersionId": versionID, "code": priceCode, "name": "Member launch",
		"kind": "NORMAL", "channel": "WECHAT_VIRTUAL", "environment": "PRODUCTION", "currency": "CNY",
		"salePriceCents": 99600, "listPriceCents": 109600, "giftPoints": 10, "giftTokens": 20,
		"validFrom": now.Add(-time.Minute), "validUntil": now.Add(24 * time.Hour),
		"audienceType": "PUBLIC", "audienceRule": map[string]any{}, "isVisible": true,
		"changeReason": "create public draft",
	}), adminToken)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Item struct {
			ID             string `json:"pricePlanId"`
			PlanID         string `json:"planId"`
			PlanVersionID  string `json:"planVersionId"`
			Code           string `json:"code"`
			Kind           string `json:"kind"`
			Currency       string `json:"currency"`
			AudienceType   string `json:"audienceType"`
			Status         string `json:"status"`
			IsDefault      bool   `json:"isDefault"`
			IsEnabled      bool   `json:"isEnabled"`
			Revision       int64  `json:"revision"`
			SalePriceCents int64  `json:"salePriceCents"`
			GiftPoints     int64  `json:"giftPoints"`
			GiftTokens     int64  `json:"giftTokens"`
		} `json:"item"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Item.ID == "" || created.Item.PlanID != planID || created.Item.PlanVersionID != versionID ||
		created.Item.Code != priceCode || created.Item.Kind != "NORMAL" || created.Item.Currency != "CNY" ||
		created.Item.AudienceType != "PUBLIC" || created.Item.Status != "DRAFT" || created.Item.IsDefault ||
		created.Item.IsEnabled || created.Item.Revision != 1 || created.Item.SalePriceCents != 99600 ||
		created.Item.GiftPoints != 10 || created.Item.GiftTokens != 20 {
		t.Fatalf("unexpected created price plan: %+v", created.Item)
	}

	list := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans/"+planID+"/price-plans", nil, adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	detail := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+created.Item.ID, nil, adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	forbidden := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans/"+planID+"/price-plans", nil, viewerToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	patch := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/price-plans/"+created.Item.ID, jsonBody(map[string]any{
		"revision": 1, "salePriceCents": 99500, "listPriceCents": 109500,
		"giftPoints": 0, "giftTokens": 40, "changeReason": "adjust draft economics",
	}), adminToken)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patch.Code, patch.Body.String())
	}
	var patched struct {
		Item struct {
			Revision       int64 `json:"revision"`
			SalePriceCents int64 `json:"salePriceCents"`
			ListPriceCents int64 `json:"listPriceCents"`
			GiftPoints     int64 `json:"giftPoints"`
			GiftTokens     int64 `json:"giftTokens"`
		} `json:"item"`
	}
	if err := json.NewDecoder(patch.Body).Decode(&patched); err != nil {
		t.Fatal(err)
	}
	if patched.Item.Revision != 2 || patched.Item.SalePriceCents != 99500 || patched.Item.ListPriceCents != 109500 ||
		patched.Item.GiftPoints != 0 || patched.Item.GiftTokens != 40 {
		t.Fatalf("unexpected patched price plan: %+v", patched.Item)
	}

	stale := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/price-plans/"+created.Item.ID, jsonBody(map[string]any{
		"revision": 1, "name": "stale name", "changeReason": "stale mutation",
	}), adminToken)
	if stale.Code != http.StatusConflict || decodeErrorCode(stale.Body) != "REVISION_CONFLICT" {
		t.Fatalf("stale patch status=%d body=%s", stale.Code, stale.Body.String())
	}

	cloneCode := "member_campaign_" + suffix
	clone := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+created.Item.ID+"/clone", jsonBody(map[string]any{
		"revision": 2, "code": cloneCode, "name": "Member campaign", "changeReason": "prepare replacement price",
	}), adminToken)
	if clone.Code != http.StatusCreated {
		t.Fatalf("clone status=%d body=%s", clone.Code, clone.Body.String())
	}
	var cloned struct {
		Item struct {
			ID             string `json:"pricePlanId"`
			Code           string `json:"code"`
			Status         string `json:"status"`
			IsDefault      bool   `json:"isDefault"`
			IsEnabled      bool   `json:"isEnabled"`
			Revision       int64  `json:"revision"`
			SalePriceCents int64  `json:"salePriceCents"`
		} `json:"item"`
	}
	if err := json.NewDecoder(clone.Body).Decode(&cloned); err != nil {
		t.Fatal(err)
	}
	if cloned.Item.ID == "" || cloned.Item.ID == created.Item.ID || cloned.Item.Code != cloneCode ||
		cloned.Item.Status != "DRAFT" || cloned.Item.IsDefault || cloned.Item.IsEnabled || cloned.Item.Revision != 1 ||
		cloned.Item.SalePriceCents != 99500 {
		t.Fatalf("unexpected cloned price plan: %+v", cloned.Item)
	}
	var clonedBindings int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_price_plan_payment_bindings where price_plan_id=$1`, cloned.Item.ID).Scan(&clonedBindings); err != nil {
		t.Fatal(err)
	}
	if clonedBindings != 0 {
		t.Fatalf("clone copied %d payment bindings", clonedBindings)
	}

	enableWithoutBinding := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+cloned.Item.ID+"/enable", jsonBody(map[string]any{
		"revision": 1, "changeReason": "must require active binding",
	}), adminToken)
	if enableWithoutBinding.Code != http.StatusUnprocessableEntity || decodeErrorCode(enableWithoutBinding.Body) != "PRICE_PLAN_BINDING_NOT_ACTIVE" {
		t.Fatalf("enable without binding status=%d body=%s", enableWithoutBinding.Code, enableWithoutBinding.Body.String())
	}

	goodResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods", jsonBody(map[string]any{
		"channel": "WECHAT_VIRTUAL", "environment": "PRODUCTION", "offerId": "offer-" + suffix,
		"productId": "PRODUCT_" + suffix, "goodsName": "2D replacement good", "platformPriceCents": 99500,
		"mode": "short_series_goods", "reason": "create local 2D fixture",
	}), adminToken)
	if goodResponse.Code != http.StatusCreated {
		t.Fatalf("create good status=%d body=%s", goodResponse.Code, goodResponse.Body.String())
	}
	var good struct {
		Item struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"item"`
	}
	if err := json.NewDecoder(goodResponse.Body).Decode(&good); err != nil {
		t.Fatal(err)
	}
	confirmedResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/wechat-virtual-goods/"+good.Item.ID+"/confirm-published", jsonBody(map[string]any{
		"revision": good.Item.Revision, "reason": "manual 2D fixture confirmation",
		"verificationReason": "operator checked the 2D fixture in WeChat console", "evidence": "ticket-" + suffix,
	}), adminToken)
	if confirmedResponse.Code != http.StatusOK {
		t.Fatalf("confirm good status=%d body=%s", confirmedResponse.Code, confirmedResponse.Body.String())
	}

	bindingResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+cloned.Item.ID+"/payment-bindings", jsonBody(map[string]any{
		"wechatGoodId": good.Item.ID, "reason": "bind replacement price",
	}), adminToken)
	if bindingResponse.Code != http.StatusCreated {
		t.Fatalf("create binding status=%d body=%s", bindingResponse.Code, bindingResponse.Body.String())
	}
	var binding struct {
		Item struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"item"`
	}
	if err := json.NewDecoder(bindingResponse.Body).Decode(&binding); err != nil {
		t.Fatal(err)
	}
	activateBinding := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/payment-bindings/"+binding.Item.ID, jsonBody(map[string]any{
		"revision": binding.Item.Revision, "enabled": true, "reason": "activate prepared binding",
	}), adminToken)
	if activateBinding.Code != http.StatusOK {
		t.Fatalf("activate draft price binding status=%d body=%s", activateBinding.Code, activateBinding.Body.String())
	}
	service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{Env: 0}}
	managedBeforeEnable, err := service.isManagedMemberAgentPlanRef(ctx, planID)
	if err != nil {
		t.Fatal(err)
	}
	if managedBeforeEnable {
		t.Fatal("ACTIVE binding on a DRAFT price plan prematurely cut legacy payment over to V2")
	}

	validation := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+cloned.Item.ID+"/validation", nil, adminToken)
	if validation.Code != http.StatusOK {
		t.Fatalf("validation status=%d body=%s", validation.Code, validation.Body.String())
	}
	var validationPayload struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(validation.Body).Decode(&validationPayload); err != nil {
		t.Fatal(err)
	}
	if !validationPayload.Valid {
		t.Fatalf("prepared price plan did not validate: %s", validation.Body.String())
	}

	enable := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+cloned.Item.ID+"/enable", jsonBody(map[string]any{
		"revision": 1, "changeReason": "activate replacement price",
	}), adminToken)
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%s", enable.Code, enable.Body.String())
	}
	var enabled struct {
		Item struct {
			Status    string `json:"status"`
			IsEnabled bool   `json:"isEnabled"`
			IsDefault bool   `json:"isDefault"`
			Revision  int64  `json:"revision"`
		} `json:"item"`
	}
	if err := json.NewDecoder(enable.Body).Decode(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled.Item.Status != "ACTIVE" || !enabled.Item.IsEnabled || enabled.Item.IsDefault || enabled.Item.Revision != 2 {
		t.Fatalf("unexpected enabled plan: %+v", enabled.Item)
	}

	activePricePatch := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/price-plans/"+cloned.Item.ID, jsonBody(map[string]any{
		"revision": 2, "salePriceCents": 99499, "changeReason": "must clone active price",
	}), adminToken)
	if activePricePatch.Code != http.StatusConflict || decodeErrorCode(activePricePatch.Body) != "PRICE_PLAN_CLONE_REQUIRED" {
		t.Fatalf("active economic patch status=%d body=%s", activePricePatch.Code, activePricePatch.Body.String())
	}

	disable := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+cloned.Item.ID+"/disable", jsonBody(map[string]any{
		"revision": 2, "changeReason": "temporarily disable replacement",
	}), adminToken)
	if disable.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disable.Code, disable.Body.String())
	}
	var disabled struct {
		Item struct {
			Status    string `json:"status"`
			IsEnabled bool   `json:"isEnabled"`
			Revision  int64  `json:"revision"`
		} `json:"item"`
	}
	if err := json.NewDecoder(disable.Body).Decode(&disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Item.Status != "INACTIVE" || disabled.Item.IsEnabled || disabled.Item.Revision != 3 {
		t.Fatalf("unexpected disabled plan: %+v", disabled.Item)
	}
	reenable := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+cloned.Item.ID+"/enable", jsonBody(map[string]any{
		"revision": 3, "changeReason": "reactivate replacement",
	}), adminToken)
	if reenable.Code != http.StatusOK {
		t.Fatalf("reenable status=%d body=%s", reenable.Code, reenable.Body.String())
	}

	badCode := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/business-plans/"+planID+"/price-plans", jsonBody(map[string]any{
		"revision": 1, "planVersionId": versionID, "code": "member_price_996", "name": "bad code",
		"kind": "NORMAL", "channel": "WECHAT_VIRTUAL", "environment": "PRODUCTION", "currency": "CNY",
		"salePriceCents": 100, "listPriceCents": 100, "audienceType": "PUBLIC", "audienceRule": map[string]any{},
		"isVisible": true, "changeReason": "must reject price semantics",
	}), adminToken)
	if badCode.Code != http.StatusBadRequest || decodeErrorCode(badCode.Body) != "PRICE_PLAN_CODE_HAS_PRICE" {
		t.Fatalf("bad code status=%d body=%s", badCode.Code, badCode.Body.String())
	}

	badTest := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/business-plans/"+planID+"/price-plans", jsonBody(map[string]any{
		"revision": 1, "planVersionId": versionID, "code": "member_test_visible_" + suffix, "name": "bad test",
		"kind": "TEST", "channel": "WECHAT_VIRTUAL", "environment": "PRODUCTION", "currency": "CNY",
		"salePriceCents": 100, "listPriceCents": 100, "audienceType": "PUBLIC", "audienceRule": map[string]any{},
		"isVisible": true, "changeReason": "must reject visible test",
	}), adminToken)
	if badTest.Code != http.StatusBadRequest || decodeErrorCode(badTest.Body) != "PRICE_PLAN_TEST_SCOPE_INVALID" {
		t.Fatalf("bad test status=%d body=%s", badTest.Code, badTest.Body.String())
	}

	legacyTestID := "price_legacy_test_" + suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_price_plans(
			id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
			sale_price_cents,original_price_cents,audience_type,audience_rule,is_visible,is_default,
			enabled,status,change_reason
		) values($1,$2,$3,$1,'legacy test','TEST','WECHAT_VIRTUAL','PRODUCTION','CNY',
			100,100,'PUBLIC','{}'::jsonb,false,false,false,'DRAFT','097 compatibility fixture')
	`, legacyTestID, planID, versionID); err != nil {
		t.Fatal(err)
	}
	legacyCloneCode := "member_legacy_test_copy_" + suffix
	legacyClone := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+legacyTestID+"/clone", jsonBody(map[string]any{
		"revision": 1, "code": legacyCloneCode, "name": "Normalized test copy", "changeReason": "normalize legacy test scope",
	}), adminToken)
	if legacyClone.Code != http.StatusCreated {
		t.Fatalf("legacy TEST clone status=%d body=%s", legacyClone.Code, legacyClone.Body.String())
	}
	var normalized struct {
		Item pricePlanAdminView `json:"item"`
	}
	if err := json.NewDecoder(legacyClone.Body).Decode(&normalized); err != nil {
		t.Fatal(err)
	}
	if normalized.Item.Kind != "TEST" || normalized.Item.AudienceType == "PUBLIC" || normalized.Item.IsVisible ||
		normalized.Item.IsDefault || normalized.Item.IsEnabled || normalized.Item.Status != "DRAFT" {
		t.Fatalf("legacy TEST clone was not normalized: %+v", normalized.Item)
	}
	var normalizedCreatedBy string
	if err := db.QueryRowContext(ctx, `select coalesce(created_by,'') from xz_price_plans where id=$1`, normalized.Item.ID).Scan(&normalizedCreatedBy); err != nil {
		t.Fatal(err)
	}
	if normalizedCreatedBy != adminID {
		t.Fatalf("legacy TEST clone actor=%q want=%q", normalizedCreatedBy, adminID)
	}
}

func TestPricePlanAdminPhase2DMakeDefaultIsTransactionalConcurrentAndQuoteCompatible(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	adminID := "default_admin_" + suffix
	managerID := "default_manager_" + suffix
	managerRole := "PRICE_MANAGER_" + suffix
	planID := "plan_member_default_" + suffix
	versionID := "version_member_default_" + suffix
	oldID := "price_old_" + suffix
	firstID := "price_first_" + suffix
	secondID := "price_second_" + suffix
	thirdID := "price_third_" + suffix
	now := time.Now().UTC()

	for _, user := range []struct{ id, role string }{{adminID, "SUPER_ADMIN"}, {managerID, managerRole}} {
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,created_at,updated_at,raw)
			values($1,$1||'@example.test',$1,$2,'ACTIVE',$3,$3,
			       jsonb_build_object('id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role',$2::text,'status','ACTIVE'))
		`, user.id, user.role, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_role_permissions(role,permission) values
		($1,'pricing:plan:view'),
		($1,'pricing:price-plan:manage')
	`, managerRole); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_role_permissions where role=$1`, managerRole)
	})
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active,raw)
		values($1,$1,'2D default plan','MEMBER_PACKAGE',true,
		       jsonb_build_object('id',$1::text,'code',$1::text,'name','2D default plan','planType','MEMBER_PACKAGE','active',true))
	`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,points_amount,
			duration_days,commission_rule_version,commission_snapshot,status,effective_at,created_by,updated_by,change_reason
		) values($1,$2,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":200,"pointsAmount":20,"durationDays":30}'::jsonb,
			'PRO',200,20,30,'commission-default-v1','{"rules":[]}'::jsonb,'ACTIVE',$3,$4,$4,'activate default fixture')
	`, versionID, planID, now.Add(-time.Hour), adminID); err != nil {
		t.Fatal(err)
	}

	priceIDs := []string{oldID, firstID, secondID, thirdID}
	prices := []int64{99600, 69800, 59800, 49800}
	giftPoints := []int64{0, 0, 0, 0}
	giftTokens := []int64{4, 6, 0, 0}
	for index, priceID := range priceIDs {
		isDefault := index == 0
		if _, err := db.ExecContext(ctx, `
			insert into xz_price_plans(
				id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
				sale_price_cents,original_price_cents,bonus_points,bonus_tokens,effective_at,expires_at,
				audience_type,audience_rule,is_visible,is_default,enabled,status,created_by,updated_by,
				enabled_by,enabled_at,change_reason
			) values($1,$2,$3,$1,$1,'NORMAL','WECHAT_VIRTUAL','PRODUCTION','CNY',$4,$4,$5,$6,$7,$8,
				'PUBLIC','{}'::jsonb,true,$9,true,'ACTIVE',$10,$10,$10,$7,'seed active price plan')
		`, priceID, planID, versionID, prices[index], giftPoints[index], giftTokens[index],
			now.Add(-time.Hour), now.Add(24*time.Hour), isDefault, adminID); err != nil {
			t.Fatal(err)
		}
		goodID := "good_" + priceID
		bindingID := "binding_" + priceID
		if _, err := db.ExecContext(ctx, `
			insert into xz_wechat_virtual_goods(
				id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
				published,enabled,status,verification_status,verified_by,verified_at,verification_reason,
				verification_snapshot,verification_expires_at,created_by,updated_by
			) values($1,'WECHAT_VIRTUAL','PRODUCTION',$2,$3,$1,$4,'short_series_goods',true,true,'PUBLISHED',
				'MANUALLY_CONFIRMED_PUBLISHED',$5,$6,'manual isolated fixture',
				jsonb_build_object('productId',$3::text,'offerId',$2::text,'environment','PRODUCTION','platformPriceCents',$4::bigint),
				$7,$5,$5)
		`, goodID, "offer_"+priceID, "PRODUCT_"+priceID, prices[index], adminID, now, now.Add(12*time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_price_plan_payment_bindings(
				id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,
				enabled,status,created_by,updated_by,enabled_by,enabled_at
			) values($1,$2,$3,'WECHAT_VIRTUAL','PRODUCTION',$4,true,'ACTIVE',$5,$5,$5,$6)
		`, bindingID, priceID, goodID, prices[index], adminID, now); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	sessions := newMemoryAuthSessions()
	adminToken := "default-admin-token-" + suffix
	managerToken := "default-manager-token-" + suffix
	if err := sessions.Put(ctx, adminToken, adminID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Put(ctx, managerToken, managerID, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	t.Run("audit insert failure rolls back the whole default switch", func(t *testing.T) {
		var oldDefaultBefore, targetDefaultBefore bool
		var oldRevisionBefore, targetRevisionBefore int64
		if err := db.QueryRowContext(ctx, `select is_default,revision from xz_price_plans where id=$1`, oldID).Scan(&oldDefaultBefore, &oldRevisionBefore); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select is_default,revision from xz_price_plans where id=$1`, firstID).Scan(&targetDefaultBefore, &targetRevisionBefore); err != nil {
			t.Fatal(err)
		}
		if !oldDefaultBefore || targetDefaultBefore {
			t.Fatalf("unexpected precondition oldDefault=%v targetDefault=%v", oldDefaultBefore, targetDefaultBefore)
		}
		auditFailureFunction := "xz_test_fail_make_default_audit_" + suffix
		auditFailureTrigger := "trg_xz_test_fail_make_default_audit_" + suffix
		cleanupAuditFailure := func() {
			_, _ = db.Exec(fmt.Sprintf(`drop trigger if exists %s on xz_audit_logs; drop function if exists %s();`, auditFailureTrigger, auditFailureFunction))
		}
		cleanupAuditFailure()
		t.Cleanup(cleanupAuditFailure)
		if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		create function %s()
		returns trigger language plpgsql as $$
		begin
			raise exception using errcode='P0001',message='FORCED_MAKE_DEFAULT_AUDIT_FAILURE';
		end;
		$$;
		create trigger %s
		before insert on xz_audit_logs
		for each row when (new.action='price_plan.make_default' and new.resource_id='%s')
		execute function %s();
	`, auditFailureFunction, auditFailureTrigger, firstID, auditFailureFunction)); err != nil {
			t.Fatal(err)
		}
		forcedRevision := targetRevisionBefore
		if _, _, err := store.makeDefaultPricePlan(ctx, firstID, pricePlanTransitionMutation{
			Revision: &forcedRevision, ChangeReason: "force make-default audit rollback",
		}, adminID, "SUPER_ADMIN"); err == nil || !strings.Contains(err.Error(), "FORCED_MAKE_DEFAULT_AUDIT_FAILURE") {
			t.Fatalf("forced make-default audit failure err=%v", err)
		}
		var oldDefaultAfterFailure, targetDefaultAfterFailure bool
		var oldRevisionAfterFailure, targetRevisionAfterFailure int64
		if err := db.QueryRowContext(ctx, `select is_default,revision from xz_price_plans where id=$1`, oldID).Scan(&oldDefaultAfterFailure, &oldRevisionAfterFailure); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `select is_default,revision from xz_price_plans where id=$1`, firstID).Scan(&targetDefaultAfterFailure, &targetRevisionAfterFailure); err != nil {
			t.Fatal(err)
		}
		var failedAuditRows int
		if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where action='price_plan.make_default' and resource_id=$1`, firstID).Scan(&failedAuditRows); err != nil {
			t.Fatal(err)
		}
		if !oldDefaultAfterFailure || targetDefaultAfterFailure || oldRevisionAfterFailure != oldRevisionBefore ||
			targetRevisionAfterFailure != targetRevisionBefore || failedAuditRows != 0 {
			t.Fatalf("audit failure did not roll back default switch: old=%v/%d->%v/%d target=%v/%d->%v/%d audits=%d",
				oldDefaultBefore, oldRevisionBefore, oldDefaultAfterFailure, oldRevisionAfterFailure,
				targetDefaultBefore, targetRevisionBefore, targetDefaultAfterFailure, targetRevisionAfterFailure, failedAuditRows)
		}
		cleanupAuditFailure()
	})
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler
	jsonBody := func(value any) *bytes.Buffer {
		raw, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return bytes.NewBuffer(raw)
	}

	forbidden := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+firstID+"/make-default", jsonBody(map[string]any{
		"revision": 1, "changeReason": "manager lacks default permission",
	}), managerToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("manage-only default switch status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
	managedRead := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+firstID, nil, managerToken)
	if managedRead.Code != http.StatusOK {
		t.Fatalf("manage permission could not read price plan: status=%d body=%s", managedRead.Code, managedRead.Body.String())
	}

	service := &virtualPaymentService{db: db, cfg: virtualPaymentConfig{
		Enabled: true, Env: 0, OfferID: "runtime-offer", AppKey: "test-app-key", NotifyToken: "test-token",
		Mode: "short_series_goods", AppID: "test-app", AppSecret: "test-secret", PricePlanCreationEnabled: true,
		SnapshotV2FulfillmentEnabled: true,
	}}
	oldQuote, err := service.issuePriceQuote(ctx, adminUser{ID: adminID}, "", planID, "", pricePlanEntryPublic)
	if err != nil {
		t.Fatal(err)
	}
	if oldQuote.PricePlanID != oldID || oldQuote.AmountCent != prices[0] || oldQuote.Currency != "CNY" ||
		oldQuote.GiftPoints != giftPoints[0] || oldQuote.GiftTokens != giftTokens[0] ||
		int64Value(oldQuote.Entitlements["tokenAmount"]) != 204 || int64Value(oldQuote.Entitlements["pointsAmount"]) != 20 {
		t.Fatalf("old quote resolved wrong default: %+v", oldQuote)
	}
	var quoteCurrency string
	var quoteGiftPoints, quoteGiftTokens, quoteTokens int64
	if err := db.QueryRowContext(ctx, `
		select currency,bonus_points,bonus_tokens,(rights_snapshot->>'tokenAmount')::bigint
		from xz_order_price_quotes where quote_token_hash=$1
	`, hashSensitiveIdentifier(oldQuote.QuoteID)).Scan(&quoteCurrency, &quoteGiftPoints, &quoteGiftTokens, &quoteTokens); err != nil {
		t.Fatal(err)
	}
	if quoteCurrency != "CNY" || quoteGiftPoints != 0 || quoteGiftTokens != 4 || quoteTokens != 204 {
		t.Fatalf("quote gift snapshot mismatch: currency=%s points=%d tokens=%d total=%d", quoteCurrency, quoteGiftPoints, quoteGiftTokens, quoteTokens)
	}

	makeDefault := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+firstID+"/make-default", jsonBody(map[string]any{
		"revision": 1, "changeReason": "switch to first replacement",
	}), adminToken)
	if makeDefault.Code != http.StatusOK {
		t.Fatalf("make default status=%d body=%s", makeDefault.Code, makeDefault.Body.String())
	}
	var switched struct {
		Item           pricePlanAdminView `json:"item"`
		AlreadyDefault bool               `json:"alreadyDefault"`
	}
	if err := json.NewDecoder(makeDefault.Body).Decode(&switched); err != nil {
		t.Fatal(err)
	}
	if !switched.Item.IsDefault || switched.Item.Revision != 2 || switched.AlreadyDefault {
		t.Fatalf("unexpected default switch response: %+v", switched)
	}
	var auditActorID, auditRole, auditReason, auditPlanID, auditChannel, auditEnvironment, auditCurrency string
	var auditOldID, auditNewID, auditGoodID, auditProductID string
	var auditOldPrice, auditNewPrice, auditPlanPrice, auditBindingPrice, auditGoodPrice int64
	var auditValidationPassed bool
	if err := db.QueryRowContext(ctx, `
		select actor_id,actor_role,metadata->>'changeReason',metadata->>'planId',
		       metadata->>'channel',metadata->>'environment',metadata->>'currency',
		       metadata->>'oldDefaultPricePlanId',metadata->>'newDefaultPricePlanId',
		       metadata->>'wechatGoodId',metadata->>'wechatProductId',
		       (metadata->>'oldPriceCents')::bigint,(metadata->>'newPriceCents')::bigint,
		       (metadata->>'pricePlanPriceCents')::bigint,(metadata->>'bindingPriceCents')::bigint,
		       (metadata->>'wechatGoodPriceCents')::bigint,(metadata->'validation'->>'valid')::boolean
		from xz_audit_logs
		where action='price_plan.make_default' and resource_id=$1
		order by created_at desc limit 1
	`, firstID).Scan(&auditActorID, &auditRole, &auditReason, &auditPlanID, &auditChannel,
		&auditEnvironment, &auditCurrency, &auditOldID, &auditNewID, &auditGoodID, &auditProductID,
		&auditOldPrice, &auditNewPrice, &auditPlanPrice, &auditBindingPrice, &auditGoodPrice,
		&auditValidationPassed); err != nil {
		t.Fatal(err)
	}
	if auditActorID != adminID || auditRole != "SUPER_ADMIN" || auditReason != "switch to first replacement" ||
		auditPlanID != planID || auditChannel != "WECHAT_VIRTUAL" || auditEnvironment != "PRODUCTION" ||
		auditCurrency != "CNY" || auditOldID != oldID || auditNewID != firstID ||
		auditGoodID != "good_"+firstID || auditProductID != "PRODUCT_"+firstID ||
		auditOldPrice != prices[0] || auditNewPrice != prices[1] || auditPlanPrice != prices[1] ||
		auditBindingPrice != prices[1] || auditGoodPrice != prices[1] || !auditValidationPassed {
		t.Fatalf("default audit snapshot is incomplete: actor=%s/%s reason=%s plan=%s group=%s/%s/%s old=%s/%d new=%s/%d good=%s/%s prices=%d/%d/%d valid=%v",
			auditActorID, auditRole, auditReason, auditPlanID, auditChannel, auditEnvironment, auditCurrency,
			auditOldID, auditOldPrice, auditNewID, auditNewPrice, auditGoodID, auditProductID,
			auditPlanPrice, auditBindingPrice, auditGoodPrice, auditValidationPassed)
	}
	var oldDefault, oldEnabled bool
	var oldStatus string
	if err := db.QueryRowContext(ctx, `select is_default,enabled,status from xz_price_plans where id=$1`, oldID).Scan(&oldDefault, &oldEnabled, &oldStatus); err != nil {
		t.Fatal(err)
	}
	if oldDefault || !oldEnabled || oldStatus != "ACTIVE" {
		t.Fatalf("old default was disabled: default=%v enabled=%v status=%s", oldDefault, oldEnabled, oldStatus)
	}

	newQuote, err := service.issuePriceQuote(ctx, adminUser{ID: adminID}, "", planID, "", pricePlanEntryPublic)
	if err != nil {
		t.Fatal(err)
	}
	if newQuote.PricePlanID != firstID || newQuote.AmountCent != prices[1] || newQuote.GiftPoints != 0 ||
		newQuote.GiftTokens != 6 || int64Value(newQuote.Entitlements["tokenAmount"]) != 206 ||
		int64Value(newQuote.Entitlements["pointsAmount"]) != 20 {
		t.Fatalf("new quote did not use committed default: %+v", newQuote)
	}
	oldOrder, err := service.createOrderFromPriceQuote(ctx, adminUser{ID: adminID}, "", oldQuote.QuoteID, &wechatMiniProgramSession{
		OpenID: "default-openid-" + suffix, SessionKey: "default-session-" + suffix,
	})
	if err != nil {
		t.Fatalf("old quote was invalidated or repriced: %v", err)
	}
	if oldOrder.AmountCent != prices[0] {
		t.Fatalf("old quote was repriced: %+v", oldOrder)
	}
	var signData virtualPaySignData
	if err := json.Unmarshal([]byte(oldOrder.SignData), &signData); err != nil {
		t.Fatal(err)
	}
	var orderCurrency string
	var orderGiftPoints, orderGiftTokens, orderTokens int64
	if err := db.QueryRowContext(ctx, `
		select currency,coalesce((price_snapshot->>'pricePlanGiftPoints')::bigint,0),
		       (price_snapshot->>'pricePlanGiftTokens')::bigint,(rights_snapshot->>'tokenAmount')::bigint
		from xz_orders where order_no=$1
	`, oldOrder.OrderNo).Scan(&orderCurrency, &orderGiftPoints, &orderGiftTokens, &orderTokens); err != nil {
		t.Fatal(err)
	}
	if signData.CurrencyType != "CNY" || orderCurrency != "CNY" || orderGiftPoints != 0 || orderGiftTokens != 4 || orderTokens != 204 {
		t.Fatalf("order gift/currency snapshot mismatch: sign=%s currency=%s points=%d tokens=%d total=%d", signData.CurrencyType, orderCurrency, orderGiftPoints, orderGiftTokens, orderTokens)
	}

	var auditBefore int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where action='price_plan.make_default' and resource_id=$1`, firstID).Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}
	idempotent := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+firstID+"/make-default", jsonBody(map[string]any{
		"revision": 1, "changeReason": "retry same request",
	}), adminToken)
	if idempotent.Code != http.StatusOK {
		t.Fatalf("idempotent default retry status=%d body=%s", idempotent.Code, idempotent.Body.String())
	}
	var retried struct {
		Item           pricePlanAdminView `json:"item"`
		AlreadyDefault bool               `json:"alreadyDefault"`
	}
	if err := json.NewDecoder(idempotent.Body).Decode(&retried); err != nil {
		t.Fatal(err)
	}
	var auditAfter int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where action='price_plan.make_default' and resource_id=$1`, firstID).Scan(&auditAfter); err != nil {
		t.Fatal(err)
	}
	if !retried.AlreadyDefault || retried.Item.Revision != 2 || auditAfter != auditBefore {
		t.Fatalf("idempotent retry dirtied state: response=%+v audits=%d->%d", retried, auditBefore, auditAfter)
	}

	responses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, targetID := range []string{secondID, thirdID} {
		targetID := targetID
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+targetID+"/make-default", jsonBody(map[string]any{
				"revision": 1, "changeReason": "concurrent default switch",
			}), adminToken)
			responses <- response.Code
		}()
	}
	wg.Wait()
	close(responses)
	for status := range responses {
		if status != http.StatusOK {
			t.Fatalf("concurrent default switch status=%d", status)
		}
	}
	var defaults int
	if err := db.QueryRowContext(ctx, `
		select count(*) from xz_price_plans
		where plan_id=$1 and channel='WECHAT_VIRTUAL' and environment='PRODUCTION' and currency='CNY' and is_default=true
	`, planID).Scan(&defaults); err != nil {
		t.Fatal(err)
	}
	if defaults != 1 {
		t.Fatalf("concurrent switches left %d defaults", defaults)
	}
}

func TestPricePlanAdminPhase2DRejectsUnsafeEnableAndDefaultCandidates(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	adminID := "validation_admin_" + suffix
	planID := "plan_member_validation_" + suffix
	activeVersionID := "version_active_" + suffix
	draftVersionID := "version_draft_" + suffix
	currentDefaultID := "price_current_" + suffix
	now := time.Now().UTC()

	if _, err := db.ExecContext(ctx, `
		insert into xz_users(id,email,name,role,status,created_at,updated_at,raw)
		values($1,$1||'@example.test',$1,'SUPER_ADMIN','ACTIVE',$2,$2,
		       jsonb_build_object('id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role','SUPER_ADMIN','status','ACTIVE'))
	`, adminID, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active,raw)
		values($1,$1,'validation plan','MEMBER_PACKAGE',true,
		       jsonb_build_object('id',$1::text,'code',$1::text,'name','validation plan','planType','MEMBER_PACKAGE','active',true))
	`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,member_level,token_amount,points_amount,duration_days,
			commission_rule_version,commission_snapshot,status,effective_at,created_by,updated_by,change_reason
		) values
			($1,$3,1,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"pointsAmount":10,"durationDays":30}'::jsonb,
			 'PRO',100,10,30,'commission-v1','{"rules":[]}'::jsonb,'ACTIVE',$4,$5,$5,'active fixture'),
			($2,$3,2,'MEMBER','{"memberLevel":"PRO","tokenAmount":100,"pointsAmount":10,"durationDays":30}'::jsonb,
			 'PRO',100,10,30,'commission-v1','{"rules":[]}'::jsonb,'DRAFT',null,$5,$5,'draft fixture')
	`, activeVersionID, draftVersionID, planID, now.Add(-time.Hour), adminID); err != nil {
		t.Fatal(err)
	}

	type candidate struct {
		id                  string
		versionID           string
		kind                string
		audience            string
		visible             bool
		status              string
		enabled             bool
		priceEnvironment    string
		goodEnvironment     string
		verificationStatus  string
		verificationExpired bool
		goodAvailable       bool
		salePrice           int64
		bindingPrice        int64
		goodPrice           int64
		giftPoints          int64
	}
	seed := func(item candidate) {
		t.Helper()
		if item.versionID == "" {
			item.versionID = activeVersionID
		}
		if item.kind == "" {
			item.kind = "NORMAL"
		}
		if item.audience == "" {
			item.audience = "PUBLIC"
		}
		if item.priceEnvironment == "" {
			item.priceEnvironment = "PRODUCTION"
		}
		if item.goodEnvironment == "" {
			item.goodEnvironment = item.priceEnvironment
		}
		if item.verificationStatus == "" {
			item.verificationStatus = wechatGoodVerificationManual
		}
		if item.salePrice == 0 {
			item.salePrice = 10000
		}
		if item.bindingPrice == 0 {
			item.bindingPrice = item.salePrice
		}
		if item.goodPrice == 0 {
			item.goodPrice = item.salePrice
		}
		var enabledBy, enabledAt any
		if item.enabled {
			enabledBy, enabledAt = adminID, now
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_price_plans(
				id,plan_id,plan_version_id,code,name,price_type,channel,environment,currency,
				sale_price_cents,original_price_cents,bonus_points,audience_type,audience_rule,is_visible,is_default,
				enabled,status,created_by,updated_by,enabled_by,enabled_at,change_reason
			) values($1,$2,$3,$1,$1,$4,'WECHAT_VIRTUAL',$5,'CNY',$6,$6,$7,$8,'{}'::jsonb,$9,false,$10,$11,$12,$12,$13,$14,'fixture')
		`, item.id, planID, item.versionID, item.kind, item.priceEnvironment, item.salePrice, item.giftPoints,
			item.audience, item.visible, item.enabled, item.status, adminID, enabledBy, enabledAt); err != nil {
			t.Fatal(err)
		}
		goodID := "good_" + item.id
		bindingID := "binding_" + item.id
		verifiedAt := now.Add(-time.Hour)
		var verificationExpiresAt any = now.Add(12 * time.Hour)
		if item.verificationExpired {
			verifiedAt = now.Add(-2 * time.Hour)
			verificationExpiresAt = now.Add(-time.Hour)
		}
		if item.verificationStatus != wechatGoodVerificationManual {
			verificationExpiresAt = nil
		}
		goodStatus := "DRAFT"
		if item.goodAvailable {
			goodStatus = "PUBLISHED"
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_wechat_virtual_goods(
				id,channel,environment,offer_id,product_id,goods_name,platform_price_cents,mode,
				published,enabled,status,verification_status,verified_by,verified_at,verification_reason,
				verification_snapshot,verification_expires_at,created_by,updated_by
			) values($1,'WECHAT_VIRTUAL',$2,$3,$4,$1,$5,'short_series_goods',$6,$6,$7,$8,$9,$10,
				'isolated manual fixture',jsonb_build_object('productId',$4::text,'offerId',$3::text,'environment',$2::text,'platformPriceCents',$5::bigint),
				$11,$9,$9)
		`, goodID, item.goodEnvironment, "offer_"+item.id, "PRODUCT_"+item.id, item.goodPrice,
			item.goodAvailable, goodStatus, item.verificationStatus, adminID, verifiedAt, verificationExpiresAt); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_price_plan_payment_bindings(
				id,price_plan_id,wechat_good_id,channel,environment,provider_price_snapshot_cents,
				enabled,status,created_by,updated_by,enabled_by,enabled_at
			) values($1,$2,$3,'WECHAT_VIRTUAL',$4,$5,true,'ACTIVE',$6,$6,$6,$7)
		`, bindingID, item.id, goodID, item.goodEnvironment, item.bindingPrice, adminID, now); err != nil {
			t.Fatal(err)
		}
	}

	seed(candidate{id: currentDefaultID, visible: true, status: "ACTIVE", enabled: true, goodAvailable: true, salePrice: 99600})
	if _, err := db.ExecContext(ctx, `update xz_price_plans set is_default=true where id=$1`, currentDefaultID); err != nil {
		t.Fatal(err)
	}
	var defaultRevision int64
	if err := db.QueryRowContext(ctx, `select revision from xz_price_plans where id=$1`, currentDefaultID).Scan(&defaultRevision); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		item       candidate
		action     string
		wantStatus int
		wantCode   string
	}{
		{name: "TEST cannot default", item: candidate{id: "price_test_" + suffix, kind: "TEST", audience: "TEST", visible: false, status: "ACTIVE", enabled: true, goodAvailable: true}, action: "make-default", wantCode: "PRICE_PLAN_DEFAULT_TEST_FORBIDDEN"},
		{name: "hidden cannot default", item: candidate{id: "price_hidden_" + suffix, visible: false, status: "ACTIVE", enabled: true, goodAvailable: true}, action: "make-default", wantCode: "PRICE_PLAN_DEFAULT_HIDDEN"},
		{name: "non-public cannot default", item: candidate{id: "price_rule_" + suffix, audience: "RULE", visible: true, status: "ACTIVE", enabled: true, goodAvailable: true}, action: "make-default", wantCode: "PRICE_PLAN_DEFAULT_AUDIENCE_INVALID"},
		{name: "inactive version cannot enable", item: candidate{id: "price_version_" + suffix, versionID: draftVersionID, visible: true, status: "DRAFT", goodAvailable: true}, action: "enable", wantCode: "PRICE_PLAN_VERSION_NOT_ACTIVE"},
		{name: "terminal expired plan cannot enable", item: candidate{id: "price_terminal_expired_enable_" + suffix, visible: true, status: "EXPIRED", goodAvailable: true}, action: "enable", wantStatus: http.StatusConflict, wantCode: "PRICE_PLAN_STATE_INVALID"},
		{name: "terminal expired plan cannot disable", item: candidate{id: "price_terminal_expired_disable_" + suffix, visible: true, status: "EXPIRED", goodAvailable: true}, action: "disable", wantStatus: http.StatusConflict, wantCode: "PRICE_PLAN_STATE_INVALID"},
		{name: "disabled ACTIVE state cannot enable", item: candidate{id: "price_inconsistent_active_" + suffix, visible: true, status: "ACTIVE", goodAvailable: true}, action: "enable", wantStatus: http.StatusConflict, wantCode: "PRICE_PLAN_STATE_INVALID"},
		{name: "enabled INACTIVE state cannot disable", item: candidate{id: "price_inconsistent_inactive_" + suffix, visible: true, status: "INACTIVE", enabled: true, goodAvailable: true}, action: "disable", wantStatus: http.StatusConflict, wantCode: "PRICE_PLAN_STATE_INVALID"},
		{name: "gift points cannot enable without fulfillment", item: candidate{id: "price_gift_points_enable_" + suffix, visible: true, status: "DRAFT", goodAvailable: true, giftPoints: 1}, action: "enable", wantCode: "PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE"},
		{name: "gift points cannot default without fulfillment", item: candidate{id: "price_gift_points_default_" + suffix, visible: true, status: "ACTIVE", enabled: true, goodAvailable: true, giftPoints: 1}, action: "make-default", wantCode: "PRICE_PLAN_GIFT_POINTS_FULFILLMENT_UNAVAILABLE"},
		{name: "unconfirmed cannot enable", item: candidate{id: "price_unconfirmed_enable_" + suffix, visible: true, status: "DRAFT", verificationStatus: wechatGoodVerificationUnconfirmed}, action: "enable", wantCode: "WECHAT_GOOD_NOT_CONFIRMED"},
		{name: "unconfirmed cannot default", item: candidate{id: "price_unconfirmed_default_" + suffix, visible: true, status: "ACTIVE", enabled: true, verificationStatus: wechatGoodVerificationUnconfirmed}, action: "make-default", wantCode: "WECHAT_GOOD_NOT_CONFIRMED"},
		{name: "expired cannot enable", item: candidate{id: "price_expired_enable_" + suffix, visible: true, status: "DRAFT", goodAvailable: true, verificationExpired: true}, action: "enable", wantCode: "WECHAT_GOOD_VERIFICATION_EXPIRED"},
		{name: "expired cannot default", item: candidate{id: "price_expired_default_" + suffix, visible: true, status: "ACTIVE", enabled: true, goodAvailable: true, verificationExpired: true}, action: "make-default", wantCode: "WECHAT_GOOD_VERIFICATION_EXPIRED"},
		{name: "one-cent mismatch cannot enable", item: candidate{id: "price_mismatch_enable_" + suffix, visible: true, status: "DRAFT", goodAvailable: true, salePrice: 10000, bindingPrice: 10000, goodPrice: 10001}, action: "enable", wantCode: "PRICE_PLAN_WECHAT_PRICE_MISMATCH"},
		{name: "one-cent mismatch cannot default", item: candidate{id: "price_mismatch_default_" + suffix, visible: true, status: "ACTIVE", enabled: true, goodAvailable: true, salePrice: 10000, bindingPrice: 10000, goodPrice: 10001}, action: "make-default", wantCode: "PRICE_PLAN_WECHAT_PRICE_MISMATCH"},
		{name: "cross environment cannot enable", item: candidate{id: "price_cross_enable_" + suffix, visible: true, status: "DRAFT", goodEnvironment: "SANDBOX", goodAvailable: true}, action: "enable", wantCode: "PRICE_PLAN_PAYMENT_ENV_MISMATCH"},
		{name: "cross environment cannot default", item: candidate{id: "price_cross_default_" + suffix, visible: true, status: "ACTIVE", enabled: true, goodEnvironment: "SANDBOX", goodAvailable: true}, action: "make-default", wantCode: "PRICE_PLAN_PAYMENT_ENV_MISMATCH"},
	}

	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	sessions := newMemoryAuthSessions()
	adminToken := "validation-admin-token-" + suffix
	if err := sessions.Put(ctx, adminToken, adminID, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler
	decodeCode := func(body *bytes.Buffer) string {
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload.Code
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			seed(testCase.item)
			body, err := json.Marshal(map[string]any{"revision": 1, "changeReason": testCase.name})
			if err != nil {
				t.Fatal(err)
			}
			response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+testCase.item.id+"/"+testCase.action, bytes.NewBuffer(body), adminToken)
			gotCode := decodeCode(response.Body)
			wantStatus := testCase.wantStatus
			if wantStatus == 0 {
				wantStatus = http.StatusUnprocessableEntity
			}
			if response.Code != wantStatus || gotCode != testCase.wantCode {
				t.Fatalf("status=%d code=%s want=%s", response.Code, gotCode, testCase.wantCode)
			}
		})
	}

	validStaleID := "price_stale_" + suffix
	seed(candidate{id: validStaleID, visible: true, status: "ACTIVE", enabled: true, goodAvailable: true})
	staleBody, _ := json.Marshal(map[string]any{"revision": 99, "changeReason": "stale revision"})
	stale := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+validStaleID+"/make-default", bytes.NewBuffer(staleBody), adminToken)
	if stale.Code != http.StatusConflict || decodeCode(stale.Body) != "REVISION_CONFLICT" {
		t.Fatalf("stale default status=%d body=%s", stale.Code, stale.Body.String())
	}
	defaultDisableBody, _ := json.Marshal(map[string]any{"revision": defaultRevision, "changeReason": "must switch first"})
	defaultDisable := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+currentDefaultID+"/disable", bytes.NewBuffer(defaultDisableBody), adminToken)
	if defaultDisable.Code != http.StatusConflict || decodeCode(defaultDisable.Body) != "PRICE_PLAN_DEFAULT_DISABLE_FORBIDDEN" {
		t.Fatalf("default disable status=%d body=%s", defaultDisable.Code, defaultDisable.Body.String())
	}
	missingReasonBody, _ := json.Marshal(map[string]any{"revision": 1})
	missingReason := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+validStaleID+"/make-default", bytes.NewBuffer(missingReasonBody), adminToken)
	if missingReason.Code != http.StatusBadRequest || decodeCode(missingReason.Body) != "REASON_REQUIRED" {
		t.Fatalf("missing reason status=%d body=%s", missingReason.Code, missingReason.Body.String())
	}

	var finalDefaultID string
	var finalDefaultRevision int64
	var defaultCount int
	if err := db.QueryRowContext(ctx, `
		select id,revision from xz_price_plans
		where plan_id=$1 and channel='WECHAT_VIRTUAL' and environment='PRODUCTION' and currency='CNY' and is_default=true
	`, planID).Scan(&finalDefaultID, &finalDefaultRevision); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `select count(*) from xz_price_plans where plan_id=$1 and is_default=true`, planID).Scan(&defaultCount); err != nil {
		t.Fatal(err)
	}
	if finalDefaultID != currentDefaultID || finalDefaultRevision != defaultRevision || defaultCount != 1 {
		t.Fatalf("failed switch changed old default: id=%s revision=%d count=%d want=%s/%d/1", finalDefaultID, finalDefaultRevision, defaultCount, currentDefaultID, defaultRevision)
	}
}

func TestEvaluatePricePlanActivationRejectsEntitlementVersionOutsideValidity(t *testing.T) {
	now := time.Now().UTC()
	price := pricePlanAdminView{
		ID: "price", PlanID: "plan", PlanVersionID: "version", Currency: "CNY", SalePriceCents: 100,
		Channel: "WECHAT_VIRTUAL", Environment: "PRODUCTION", Status: "DRAFT", AudienceRule: map[string]any{},
	}
	version := businessPlanVersionView{
		ID: "version", PlanID: "plan", Status: "ACTIVE", EffectiveAt: timePointer(now.Add(time.Hour)),
		RightsSnapshot: map[string]any{"tokenAmount": float64(100), "pointsAmount": float64(0)},
	}
	binding := paymentBindingRow{
		ID: "binding", PricePlanID: "price", WeChatGoodID: "good", Channel: "WECHAT_VIRTUAL",
		Environment: "PRODUCTION", ProviderPriceSnapshotCents: 100, Enabled: true, Status: "ACTIVE",
	}
	good := wechatVirtualGoodAdminView{
		ID: "good", Channel: "WECHAT_VIRTUAL", Environment: "PRODUCTION", ProductID: "PRODUCT",
		PlatformPriceCents: 100, Published: true, Enabled: true, Status: "PUBLISHED",
		recordedVerificationStatus: wechatGoodVerificationManual,
	}
	result, err := evaluatePricePlanActivation(price, true, version, &binding, &good, now)
	var businessErr *businessPlanAdminError
	if result.Valid || !errors.As(err, &businessErr) || businessErr.BusinessCode() != "PRICE_PLAN_VERSION_OUTSIDE_VALIDITY" {
		t.Fatalf("future entitlement version was accepted: result=%+v err=%v", result, err)
	}
}

func TestEvaluatePricePlanActivationRejectsIncompleteCommissionSnapshot(t *testing.T) {
	now := time.Now().UTC()
	price := pricePlanAdminView{
		ID: "price", PlanID: "plan", PlanVersionID: "version", Currency: "CNY", SalePriceCents: 100,
		Channel: "WECHAT_VIRTUAL", Environment: "PRODUCTION", Status: "DRAFT", AudienceRule: map[string]any{},
	}
	version := businessPlanVersionView{
		ID: "version", PlanID: "plan", BusinessType: "MEMBER", Status: "ACTIVE",
		RightsSnapshot: map[string]any{"tokenAmount": float64(100), "pointsAmount": float64(0)},
		CommissionSnapshot: map[string]any{"rules": []any{map[string]any{
			"id": "tiered-v1", "code": "TIERED_V1", "name": "tiered", "version": 1,
			"beneficiaryRole": "AGENT", "relationshipLevel": 1, "calculationType": "TIERED",
			"refundPolicy": "REVERSE_OR_RECOVER",
		}}},
	}
	result, err := evaluatePricePlanActivation(price, true, version, nil, nil, now)
	var businessErr *businessPlanAdminError
	if result.Valid || !errors.As(err, &businessErr) || businessErr.BusinessCode() != "PRICE_PLAN_COMMISSION_SNAPSHOT_INVALID" {
		t.Fatalf("incomplete commission snapshot was accepted: result=%+v err=%v", result, err)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
