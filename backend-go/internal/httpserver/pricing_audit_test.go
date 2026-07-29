package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSanitizePricingAuditValueRedactsNestedCredentialsWithoutDroppingPricingFields(t *testing.T) {
	input := map[string]any{
		"productId":         "wx-product-100",
		"offerId":           "offer-100",
		"amountCents":       float64(100),
		"giftTokens":        float64(20),
		"revision":          float64(7),
		"AppSecret":         "sentinel-app-secret",
		"appKey":            "sentinel-app-key",
		"clientSecret":      "sentinel-client-secret",
		"secret":            "sentinel-generic-secret",
		"privateKey":        "sentinel-private-key",
		"verificationToken": "sentinel-verification-token",
		"encryptKey":        "sentinel-encrypt-key",
		"database_url":      "sentinel-database-url",
		"evidenceUrl":       "https://evidence.example/confirmation.png?signature=sentinel-signature&expires=1#sentinel-fragment",
		"operatorNote":      "AppSecret=sentinel-free-text-secret",
		"connectionNote":    "postgres://audit_user:sentinel-dsn-password@db.example/pricing?sslmode=require",
		"ticketUrl":         "https://audit_user:sentinel-url-password@evidence.example/ticket?token=sentinel-url-token#sentinel-url-fragment",
		"nested": map[string]any{
			"sessionKey":    "sentinel-session-key",
			"access_token":  "sentinel-access-token",
			"authorization": "sentinel-authorization",
			"cookie":        "sentinel-cookie",
			"password":      "sentinel-password",
			"credential":    "sentinel-credential",
			"dsn":           "sentinel-dsn",
		},
		"items": []any{map[string]any{"connectionString": "sentinel-connection-string"}},
	}

	got := sanitizePricingAuditValue(input)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, secret := range []string{
		"sentinel-app-secret", "sentinel-app-key", "sentinel-client-secret", "sentinel-generic-secret",
		"sentinel-private-key", "sentinel-verification-token", "sentinel-encrypt-key",
		"sentinel-database-url", "sentinel-signature", "sentinel-fragment",
		"sentinel-free-text-secret", "sentinel-dsn-password", "sentinel-url-password", "sentinel-url-token", "sentinel-url-fragment",
		"sentinel-session-key", "sentinel-access-token", "sentinel-authorization", "sentinel-cookie",
		"sentinel-password", "sentinel-credential", "sentinel-dsn", "sentinel-connection-string",
	} {
		if strings.Contains(text, secret) {
			t.Fatalf("sanitized audit still contains %q: %s", secret, text)
		}
	}
	for _, retained := range []string{"wx-product-100", "offer-100", "amountCents", "giftTokens", "revision", "https://evidence.example/confirmation.png", "https://evidence.example/ticket"} {
		if !strings.Contains(text, retained) {
			t.Fatalf("sanitized audit dropped safe pricing field %q: %s", retained, text)
		}
	}
}

func TestParsePricingAuditQuerySupportsAllFiltersAndStablePagination(t *testing.T) {
	values := url.Values{
		"planId":           {"plan-1"},
		"planVersionId":    {"version-1"},
		"pricePlanId":      {"price-1"},
		"wechatGoodId":     {"good-1"},
		"bindingId":        {"binding-1"},
		"whitelistEntryId": {"whitelist-1"},
		"action":           {"price_plan.make_default"},
		"operatorId":       {"operator-1"},
		"operatorRole":     {"SUPER_ADMIN"},
		"startTime":        {"2026-07-01T00:00:00Z"},
		"endTime":          {"2026-07-31T23:59:59Z"},
		"result":           {"SUCCEEDED"},
		"page":             {"2"},
		"pageSize":         {"25"},
	}

	got, err := parsePricingAuditQuery(values)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlanID != "plan-1" || got.PlanVersionID != "version-1" || got.PricePlanID != "price-1" ||
		got.WeChatGoodID != "good-1" || got.PaymentBindingID != "binding-1" || got.WhitelistEntryID != "whitelist-1" ||
		got.Action != "price_plan.make_default" || got.OperatorID != "operator-1" || got.OperatorRole != "SUPER_ADMIN" ||
		got.Result != "SUCCEEDED" || got.Page != 2 || got.PageSize != 25 {
		t.Fatalf("unexpected parsed query: %#v", got)
	}
	if got.StartTime == nil || got.EndTime == nil || got.StartTime.Format(time.RFC3339) != "2026-07-01T00:00:00Z" || got.EndTime.Format(time.RFC3339) != "2026-07-31T23:59:59Z" {
		t.Fatalf("unexpected parsed times: start=%v end=%v", got.StartTime, got.EndTime)
	}

	defaults, err := parsePricingAuditQuery(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Page != 1 || defaults.PageSize != 50 {
		t.Fatalf("defaults page/pageSize=%d/%d want 1/50", defaults.Page, defaults.PageSize)
	}
}

func TestParsePricingAuditQueryRejectsInvalidBoundsWithStableCodes(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
		code   string
	}{
		{"page zero", url.Values{"page": {"0"}}, "PRICING_AUDIT_PAGE_INVALID"},
		{"page not integer", url.Values{"page": {"x"}}, "PRICING_AUDIT_PAGE_INVALID"},
		{"page exceeds bounded offset", url.Values{"page": {"9223372036854775807"}}, "PRICING_AUDIT_PAGE_INVALID"},
		{"page size zero", url.Values{"pageSize": {"0"}}, "PRICING_AUDIT_PAGE_SIZE_INVALID"},
		{"page size over max", url.Values{"pageSize": {"201"}}, "PRICING_AUDIT_PAGE_SIZE_INVALID"},
		{"bad result", url.Values{"result": {"UNKNOWN"}}, "PRICING_AUDIT_RESULT_INVALID"},
		{"bad start time", url.Values{"startTime": {"2026-07-01"}}, "PRICING_AUDIT_TIME_INVALID"},
		{"backwards range", url.Values{"startTime": {"2026-07-02T00:00:00Z"}, "endTime": {"2026-07-01T00:00:00Z"}}, "PRICING_AUDIT_TIME_RANGE_INVALID"},
		{"unknown filter", url.Values{"tenantId": {"tenant-1"}}, "PRICING_AUDIT_FILTER_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parsePricingAuditQuery(test.values)
			var coded interface{ BusinessCode() string }
			if err == nil || !strings.Contains(err.Error(), "pricing audit") || !errorAsBusinessCode(err, &coded) || coded.BusinessCode() != test.code {
				t.Fatalf("error=%v code=%v want=%s", err, func() string {
					if coded == nil {
						return ""
					}
					return coded.BusinessCode()
				}(), test.code)
			}
		})
	}
}

func errorAsBusinessCode(err error, target *interface{ BusinessCode() string }) bool {
	if err == nil {
		return false
	}
	coded, ok := err.(interface{ BusinessCode() string })
	if ok {
		*target = coded
	}
	return ok
}

func TestInsertPricingAuditLogWritesStructuredSanitizedRecordInCallerTransaction(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	beforeRevision, afterRevision := int64(4), int64(5)
	requestID := "req-pricing-audit-structured"
	ctx = context.WithValue(ctx, requestIDContextKey, requestID)
	entityID := "price-audit-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	err = insertPricingAuditLog(ctx, tx, pricingAuditMutation{
		ActorID: "operator-audit", ActorRole: "PRICING_OWNER", Action: "price_plan.update",
		EntityType: "price_plan", EntityID: entityID, Method: "PATCH",
		Path: "/api/v1/admin/price-plans/" + entityID, Status: 200, Result: "SUCCEEDED",
		ChangeReason: "controlled price-plan metadata update",
		BeforeSnapshot: map[string]any{
			"pricePlanId": entityID, "salePriceCents": 100, "revision": 4,
			"credentials": map[string]any{"AppSecret": "sentinel-db-app-secret"},
		},
		AfterSnapshot: map[string]any{
			"pricePlanId": entityID, "salePriceCents": 100, "revision": 5,
			"evidenceUrl": "https://evidence.example/good.png?signature=sentinel-db-signature",
		},
		RevisionBefore: &beforeRevision, RevisionAfter: &afterRevision,
		PlanID: "plan-audit", PlanVersionID: "version-audit", PricePlanID: entityID,
		WeChatGoodID: "good-audit", PaymentBindingID: "binding-audit",
		WhitelistEntryID: "whitelist-audit", Environment: "SANDBOX",
		Metadata: map[string]any{"sessionKey": "sentinel-db-session-key", "productId": "wx-product-safe"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var actorID, actorRole, action, resource, resourceID, gotRequestID, domain, result, changeReason string
	var planID, versionID, pricePlanID, goodID, bindingID, whitelistID, environment string
	var revisionBefore, revisionAfter int64
	var metadataRaw, beforeRaw, afterRaw []byte
	if err := tx.QueryRowContext(ctx, `
		select actor_id,actor_role,action,resource,resource_id,request_id,domain,result,change_reason,
		       plan_id,plan_version_id,price_plan_id,wechat_good_id,payment_binding_id,whitelist_entry_id,environment,
		       revision_before,revision_after,metadata,before_snapshot,after_snapshot
		from xz_audit_logs where resource_id=$1 and action='price_plan.update'
	`, entityID).Scan(
		&actorID, &actorRole, &action, &resource, &resourceID, &gotRequestID, &domain, &result, &changeReason,
		&planID, &versionID, &pricePlanID, &goodID, &bindingID, &whitelistID, &environment,
		&revisionBefore, &revisionAfter, &metadataRaw, &beforeRaw, &afterRaw,
	); err != nil {
		t.Fatal(err)
	}
	if actorID != "operator-audit" || actorRole != "PRICING_OWNER" || action != "price_plan.update" ||
		resource != "price_plan" || resourceID != entityID || gotRequestID != requestID || domain != "PRICING_PRICE_PLAN" ||
		result != "SUCCEEDED" || changeReason != "controlled price-plan metadata update" ||
		planID != "plan-audit" || versionID != "version-audit" || pricePlanID != entityID || goodID != "good-audit" ||
		bindingID != "binding-audit" || whitelistID != "whitelist-audit" || environment != "SANDBOX" ||
		revisionBefore != 4 || revisionAfter != 5 {
		t.Fatalf("unexpected structured audit row: actor=%s/%s action=%s resource=%s/%s request=%s domain=%s result=%s reason=%s ids=%s/%s/%s/%s/%s/%s env=%s revisions=%d/%d",
			actorID, actorRole, action, resource, resourceID, gotRequestID, domain, result, changeReason,
			planID, versionID, pricePlanID, goodID, bindingID, whitelistID, environment, revisionBefore, revisionAfter)
	}
	allJSON := string(metadataRaw) + string(beforeRaw) + string(afterRaw)
	for _, secret := range []string{"sentinel-db-app-secret", "sentinel-db-signature", "sentinel-db-session-key"} {
		if strings.Contains(allJSON, secret) {
			t.Fatalf("database audit JSON contains %q: %s", secret, allJSON)
		}
	}
	for _, safe := range []string{"wx-product-safe", "salePriceCents", "evidence.example/good.png"} {
		if !strings.Contains(allJSON, safe) {
			t.Fatalf("database audit JSON lost %q: %s", safe, allJSON)
		}
	}

	var emptyRequestIDs int
	createRevisionBefore, createRevisionAfter := int64(0), int64(1)
	if err := insertPricingAuditLog(context.Background(), tx, pricingAuditMutation{
		ActorID: "operator-audit", ActorRole: "PRICING_OWNER", Action: "price_plan.create",
		EntityType: "price_plan", EntityID: entityID + "-fallback", Method: "POST", Status: 201,
		Result: "SUCCEEDED", ChangeReason: "fallback request ID test", AfterSnapshot: map[string]any{"revision": 1},
		RevisionBefore: &createRevisionBefore, RevisionAfter: &createRevisionAfter,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `select count(*) from xz_audit_logs where resource_id=$1 and nullif(request_id,'') is null`, entityID+"-fallback").Scan(&emptyRequestIDs); err != nil {
		t.Fatal(err)
	}
	if emptyRequestIDs != 0 {
		t.Fatal("non-HTTP pricing audit did not receive a server-generated requestId")
	}

	err = insertPricingAuditLog(ctx, tx, pricingAuditMutation{
		ActorID: "operator-audit", Action: "price_plan.create", EntityType: "price_plan",
		EntityID: entityID + "-missing-role", ChangeReason: "must reject missing role",
	})
	if err == nil || !strings.Contains(err.Error(), "actor role") {
		t.Fatalf("missing actor role error=%v", err)
	}

	sensitiveReasonID := entityID + "-sensitive-reason"
	if err := insertPricingAuditLog(ctx, tx, pricingAuditMutation{
		ActorID: "operator-audit", ActorRole: "PRICING_OWNER", Action: "price_plan.update",
		EntityType: "price_plan", EntityID: sensitiveReasonID,
		ChangeReason:   "incident AppSecret=sentinel-change-reason-secret",
		BeforeSnapshot: map[string]any{"revision": 0}, AfterSnapshot: map[string]any{"revision": 1},
		RevisionBefore: &createRevisionBefore, RevisionAfter: &createRevisionAfter,
	}); err != nil {
		t.Fatal(err)
	}
	var sensitiveReason string
	if err := tx.QueryRowContext(ctx, `select change_reason from xz_audit_logs where resource_id=$1`, sensitiveReasonID).Scan(&sensitiveReason); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sensitiveReason, "sentinel-change-reason-secret") || sensitiveReason != pricingAuditRedactedValue {
		t.Fatalf("sensitive change reason was not fully redacted: %q", sensitiveReason)
	}
}

func TestListPricingAuditLogsFiltersPaginatesAndSanitizesLegacyRows(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	planID := "plan-audit-query-" + suffix
	base := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	type row struct {
		id, action, operatorID, operatorRole, result           string
		versionID, pricePlanID, goodID, bindingID, whitelistID string
		createdAt                                              time.Time
		metadata                                               string
	}
	rows := []row{
		{"audit-query-1-" + suffix, "business_plan.version.activate", "operator-one", "ENTITLEMENT_OWNER", "SUCCEEDED", "version-one-" + suffix, "price-one-" + suffix, "good-one-" + suffix, "binding-one-" + suffix, "whitelist-one-" + suffix, base, `{"productId":"wx-product-one"}`},
		{"audit-query-2-" + suffix, "price_plan.enable", "operator-two", "PRICE_OPERATOR", "FAILED", "version-two-" + suffix, "price-two-" + suffix, "good-two-" + suffix, "binding-two-" + suffix, "whitelist-two-" + suffix, base.Add(time.Hour), `{"AppSecret":"sentinel-legacy-query-secret","offerId":"offer-two"}`},
		{"audit-query-3-" + suffix, "price_plan.make_default", "operator-one", "PRICE_OWNER", "SUCCEEDED", "version-three-" + suffix, "price-three-" + suffix, "good-three-" + suffix, "binding-three-" + suffix, "whitelist-three-" + suffix, base.Add(2 * time.Hour), `{"salePriceCents":100}`},
	}
	for _, item := range rows {
		changeReason := "query fixture"
		if item.id == rows[1].id {
			changeReason = "AppSecret=sentinel-legacy-query-reason"
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_audit_logs(
				id,actor_id,actor_role,action,resource,resource_id,method,path,status,metadata,created_at,
				request_id,domain,result,error_code,change_reason,before_snapshot,after_snapshot,
				revision_before,revision_after,plan_id,plan_version_id,price_plan_id,wechat_good_id,
				payment_binding_id,whitelist_entry_id,environment
			) values($1,$2,$3,$4,'pricing_fixture',$1,'POST','/test',200,$5::jsonb,$6,
				$7,'PRICING_TEST_QUERY',$8,null,$15,$5::jsonb,$5::jsonb,1,2,$9,$10,$11,$12,$13,$14,'SANDBOX')
		`, item.id, item.operatorID, item.operatorRole, item.action, item.metadata, item.createdAt,
			"req-"+item.id, item.result, planID, item.versionID, item.pricePlanID, item.goodID, item.bindingID, item.whitelistID, changeReason); err != nil {
			t.Fatal(err)
		}
	}

	store := &postgresStore{db: db}
	page, err := store.listPricingAuditLogs(ctx, pricingAuditQuery{PlanID: planID, Page: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 2 || page.Items[0].ID != rows[2].id || page.Items[1].ID != rows[1].id {
		t.Fatalf("page1 total/items/order=%d/%d/%v", page.Total, len(page.Items), []string{page.Items[0].ID, page.Items[1].ID})
	}
	page2, err := store.listPricingAuditLogs(ctx, pricingAuditQuery{PlanID: planID, Page: 2, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page2.Total != 3 || len(page2.Items) != 1 || page2.Items[0].ID != rows[0].id {
		t.Fatalf("page2 total/items=%d/%v", page2.Total, page2.Items)
	}

	filters := []struct {
		name   string
		query  pricingAuditQuery
		wantID string
	}{
		{"plan version", pricingAuditQuery{PlanVersionID: rows[0].versionID}, rows[0].id},
		{"price plan", pricingAuditQuery{PricePlanID: rows[1].pricePlanID}, rows[1].id},
		{"wechat good", pricingAuditQuery{WeChatGoodID: rows[2].goodID}, rows[2].id},
		{"binding", pricingAuditQuery{PaymentBindingID: rows[0].bindingID}, rows[0].id},
		{"whitelist", pricingAuditQuery{WhitelistEntryID: rows[1].whitelistID}, rows[1].id},
		{"action", pricingAuditQuery{Action: "price_plan.make_default", PlanID: planID}, rows[2].id},
		{"operator", pricingAuditQuery{OperatorID: "operator-two", PlanID: planID}, rows[1].id},
		{"role", pricingAuditQuery{OperatorRole: "ENTITLEMENT_OWNER", PlanID: planID}, rows[0].id},
		{"result", pricingAuditQuery{Result: "FAILED", PlanID: planID}, rows[1].id},
		{"time range", pricingAuditQuery{PlanID: planID, StartTime: timePointer(base.Add(30 * time.Minute)), EndTime: timePointer(base.Add(90 * time.Minute))}, rows[1].id},
	}
	for _, test := range filters {
		t.Run(test.name, func(t *testing.T) {
			test.query.Page, test.query.PageSize = 1, 50
			got, err := store.listPricingAuditLogs(ctx, test.query)
			if err != nil {
				t.Fatal(err)
			}
			if got.Total != 1 || len(got.Items) != 1 || got.Items[0].ID != test.wantID {
				t.Fatalf("filter result total/items=%d/%v want=%s", got.Total, got.Items, test.wantID)
			}
		})
	}

	unsafe, err := store.listPricingAuditLogs(ctx, pricingAuditQuery{PricePlanID: rows[1].pricePlanID, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(unsafe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sentinel-legacy-query-secret") || strings.Contains(string(raw), "sentinel-legacy-query-reason") || !strings.Contains(string(raw), "offer-two") {
		t.Fatalf("response sanitization failed: %s", raw)
	}

	api := pricingAuditAdminAPI{store: store}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-audit-logs?planId="+url.QueryEscape(planID)+"&page=1&pageSize=2", nil)
	api.list(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), fmt.Sprintf(`"total":%d`, 3)) {
		t.Fatalf("audit API status/body=%d/%s", recorder.Code, recorder.Body.String())
	}

	badRecorder := httptest.NewRecorder()
	badRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-audit-logs?page=0", nil)
	api.list(badRecorder, badRequest)
	if badRecorder.Code != http.StatusBadRequest || !strings.Contains(badRecorder.Body.String(), `"code":"PRICING_AUDIT_PAGE_INVALID"`) {
		t.Fatalf("invalid audit API status/body=%d/%s", badRecorder.Code, badRecorder.Body.String())
	}
}

func TestInsertAuditLogRoutesPricingActionsWithoutChangingLegacyAudits(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx = context.WithValue(ctx, requestIDContextKey, "req-pricing-dispatch")
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	tests := []struct {
		action, resource, domain string
	}{
		{"business_plan.version.update", "plan_version", "PRICING_ENTITLEMENT"},
		{"price_plan.update", "price_plan", "PRICING_PRICE_PLAN"},
		{"wechat_good.update", "wechat_virtual_good", "PRICING_WECHAT_GOOD"},
		{"price_plan.payment_binding.update", "price_plan_payment_binding", "PRICING_PAYMENT_BINDING"},
	}
	for index, test := range tests {
		entityID := fmt.Sprintf("pricing-dispatch-%d-%s", index, suffix)
		if err := insertAuditLog(ctx, tx, "operator-dispatch", "SUPER_ADMIN", test.action, test.resource, entityID, "PATCH", "/api/v1/admin/test", 200, map[string]any{
			"changeReason": "structured dispatch test", "planId": "plan-dispatch", "planVersionId": "version-dispatch",
			"pricePlanId": "price-dispatch", "wechatGoodId": "good-dispatch", "paymentBindingId": "binding-dispatch",
			"revisionBefore": int64(2), "revisionAfter": int64(3),
			"beforeSnapshot": map[string]any{"revision": 2}, "afterSnapshot": map[string]any{"revision": 3},
		}); err != nil {
			t.Fatal(err)
		}
		var domain, requestID, reason string
		if err := tx.QueryRowContext(ctx, `select domain,request_id,change_reason from xz_audit_logs where resource_id=$1`, entityID).Scan(&domain, &requestID, &reason); err != nil {
			t.Fatal(err)
		}
		if domain != test.domain || requestID != "req-pricing-dispatch" || reason != "structured dispatch test" {
			t.Fatalf("action=%s domain/request/reason=%s/%s/%s", test.action, domain, requestID, reason)
		}
	}

	legacyID := "legacy-dispatch-" + suffix
	if err := insertAuditLog(ctx, tx, "operator-dispatch", "SUPER_ADMIN", "admin.enterprise.update", "tenant", legacyID, "PATCH", "/api/v1/admin/enterprises/test", 200, map[string]any{"reason": "legacy remains generic"}); err != nil {
		t.Fatal(err)
	}
	var domain sql.NullString
	if err := tx.QueryRowContext(ctx, `select domain from xz_audit_logs where resource_id=$1`, legacyID).Scan(&domain); err != nil {
		t.Fatal(err)
	}
	if domain.Valid {
		t.Fatalf("legacy audit unexpectedly received pricing domain %q", domain.String)
	}
}
