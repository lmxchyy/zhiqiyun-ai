package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestPricePlanTestWhitelistAdminFiltersPaginationAndLegacyShape(t *testing.T) {
	t.Setenv("XIANZHI_TEST_DATABASE_URL", phase2ETestDSN)
	t.Setenv("XIANZHI_APPLY_TEST_MIGRATION_100", "true")
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	handler, token := phase2EWhitelistHTTPHandler(t, ctx, db, fixture)

	activeUser := fixture.userID
	pendingUser := "pending_whitelist_" + fixture.suffix
	if _, err := db.ExecContext(ctx, `insert into xz_users(id,email,name,role,status) values($1,$1||'@example.test',$1,'MEMBER','ACTIVE')`, pendingUser); err != nil {
		t.Fatal(err)
	}
	create := func(userID string, validFrom *time.Time) {
		body := map[string]any{"revision": 0, "userId": userID, "reason": "filter fixture", "changeReason": "exercise server filtering"}
		if validFrom != nil {
			body["validFrom"] = validFrom.Format(time.RFC3339Nano)
		}
		raw, _ := json.Marshal(body)
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", bytes.NewBuffer(raw), token)
		if response.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
		}
	}
	create(activeUser, nil)
	future := time.Now().UTC().Add(time.Hour)
	create(pendingUser, &future)

	filtered := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist?status=PENDING&page=1&pageSize=1", nil, token)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered status=%d body=%s", filtered.Code, filtered.Body.String())
	}
	var page struct {
		Items                 []pricePlanTestWhitelistView `json:"items"`
		Total, Page, PageSize int
	}
	decodeWhitelistHTTPResponse(t, filtered, &page)
	if page.Total != 1 || page.Page != 1 || page.PageSize != 1 || len(page.Items) != 1 || page.Items[0].UserID != pendingUser {
		t.Fatalf("page=%+v", page)
	}

	byUser := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist?userId="+activeUser+"&page=1&pageSize=20", nil, token)
	decodeWhitelistHTTPResponse(t, byUser, &page)
	if byUser.Code != http.StatusOK || page.Total != 1 || len(page.Items) != 1 || page.Items[0].UserID != activeUser {
		t.Fatalf("user filter status=%d page=%+v", byUser.Code, page)
	}

	legacy := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", nil, token)
	var legacyPayload map[string]json.RawMessage
	decodeWhitelistHTTPResponse(t, legacy, &legacyPayload)
	if legacy.Code != http.StatusOK || legacyPayload["items"] == nil || legacyPayload["total"] == nil || legacyPayload["page"] != nil || legacyPayload["pageSize"] != nil {
		t.Fatalf("legacy shape status=%d payload=%v", legacy.Code, legacyPayload)
	}

	for _, path := range []string{
		"/api/v1/admin/price-plans/" + fixture.pricePlanID + "/whitelist?page=0&pageSize=20",
		"/api/v1/admin/price-plans/" + fixture.pricePlanID + "/whitelist?page=1&pageSize=201",
		"/api/v1/admin/price-plans/" + fixture.pricePlanID + "/whitelist?status=UNKNOWN&page=1&pageSize=20",
		"/api/v1/admin/price-plans/" + fixture.pricePlanID + "/whitelist?tenantId=forged",
	} {
		invalid := authedRequest(t, handler, http.MethodGet, path, nil, token)
		requireWhitelistHTTPError(t, invalid, http.StatusBadRequest, "INVALID_WHITELIST_QUERY")
	}
}
