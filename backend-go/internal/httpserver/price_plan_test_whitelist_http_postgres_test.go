package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"xianzhi-ai/backend-go/internal/config"
)

func TestPricePlanTestWhitelistAdminHTTPExactRoutesAndLifecycle(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	handler, token := phase2EWhitelistHTTPHandler(t, ctx, db, fixture)

	validUntil := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	createBody := `{"revision":0,"userId":"` + fixture.userID + `","reason":"controlled pilot","validUntil":"` + validUntil + `","changeReason":"open controlled TEST access"}`

	nullBody := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", bytes.NewBufferString(`null`), token)
	requireWhitelistHTTPError(t, nullBody, http.StatusBadRequest, "INVALID_REQUEST")

	unknown := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", bytes.NewBufferString(createBody[:len(createBody)-1]+`,"actorId":"forged"}`), token)
	requireWhitelistHTTPError(t, unknown, http.StatusBadRequest, "INVALID_REQUEST")

	trailing := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", bytes.NewBufferString(createBody+` {}`), token)
	requireWhitelistHTTPError(t, trailing, http.StatusBadRequest, "INVALID_REQUEST")

	createRequestID := "phase2e-whitelist-create-" + fixture.suffix
	createdResponse := authedWhitelistRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", bytes.NewBufferString(createBody), token, createRequestID)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var created struct {
		Item pricePlanTestWhitelistView `json:"item"`
	}
	decodeWhitelistHTTPResponse(t, createdResponse, &created)
	if created.Item.ID == "" || created.Item.PricePlanID != fixture.pricePlanID || created.Item.UserID != fixture.userID || created.Item.Revision != 1 {
		t.Fatalf("created item=%+v", created.Item)
	}
	var storedRequestID string
	if err := db.QueryRowContext(ctx, `
		select coalesce(request_id,'') from xz_audit_logs
		where whitelist_entry_id=$1 and action='price_plan.test_whitelist.create'
	`, created.Item.ID).Scan(&storedRequestID); err != nil {
		t.Fatal(err)
	}
	if storedRequestID != createRequestID {
		t.Fatalf("create audit request_id=%q want=%q", storedRequestID, createRequestID)
	}

	listResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", nil, token)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var listed struct {
		Items []pricePlanTestWhitelistView `json:"items"`
		Total int                          `json:"total"`
	}
	decodeWhitelistHTTPResponse(t, listResponse, &listed)
	if listed.Total != 1 || len(listed.Items) != 1 || listed.Items[0].ID != created.Item.ID {
		t.Fatalf("listed=%+v", listed)
	}

	patchBody := `{"revision":1,"reason":"controlled pilot cohort 2","changeReason":"correct TEST cohort"}`
	updatedResponse := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist/"+created.Item.ID, bytes.NewBufferString(patchBody), token)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", updatedResponse.Code, updatedResponse.Body.String())
	}
	var updated struct {
		Item pricePlanTestWhitelistView `json:"item"`
	}
	decodeWhitelistHTTPResponse(t, updatedResponse, &updated)
	if updated.Item.Revision != 2 || updated.Item.Reason != "controlled pilot cohort 2" {
		t.Fatalf("updated item=%+v", updated.Item)
	}

	stale := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist/"+created.Item.ID, bytes.NewBufferString(patchBody), token)
	requireWhitelistHTTPError(t, stale, http.StatusConflict, "REVISION_CONFLICT")

	disableBody := `{"revision":2,"changeReason":"close TEST cohort"}`
	disabledResponse := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist/"+created.Item.ID+"/disable", bytes.NewBufferString(disableBody), token)
	if disabledResponse.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabledResponse.Code, disabledResponse.Body.String())
	}
	var disabled struct {
		Item            pricePlanTestWhitelistView `json:"item"`
		AlreadyDisabled bool                       `json:"alreadyDisabled"`
	}
	decodeWhitelistHTTPResponse(t, disabledResponse, &disabled)
	if disabled.AlreadyDisabled || disabled.Item.Status != pricePlanWhitelistStatusDisabled || disabled.Item.Revision != 3 {
		t.Fatalf("disabled=%+v", disabled)
	}

	var auditCountBefore int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where whitelist_entry_id=$1 and domain='PRICING_TEST_WHITELIST'`, created.Item.ID).Scan(&auditCountBefore); err != nil {
		t.Fatal(err)
	}
	repeat := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist/"+created.Item.ID+"/disable", bytes.NewBufferString(disableBody), token)
	if repeat.Code != http.StatusOK {
		t.Fatalf("repeat disable status=%d body=%s", repeat.Code, repeat.Body.String())
	}
	var repeated struct {
		Item            pricePlanTestWhitelistView `json:"item"`
		AlreadyDisabled bool                       `json:"alreadyDisabled"`
	}
	decodeWhitelistHTTPResponse(t, repeat, &repeated)
	var auditCountAfter int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where whitelist_entry_id=$1 and domain='PRICING_TEST_WHITELIST'`, created.Item.ID).Scan(&auditCountAfter); err != nil {
		t.Fatal(err)
	}
	if !repeated.AlreadyDisabled || repeated.Item.Revision != 3 || auditCountAfter != auditCountBefore {
		t.Fatalf("repeat disable dirtied state: response=%+v audits=%d->%d", repeated, auditCountBefore, auditCountAfter)
	}
}

func TestPricePlanTestWhitelistAdminHTTPRejectsNonTestAndCrossPlanMutation(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	testFixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	otherTestFixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	normalFixture := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	handler, token := phase2EWhitelistHTTPHandler(t, ctx, db, testFixture)

	revisionZero := int64(0)
	store := &postgresStore{db: db, ready: true}
	created, err := store.createPricePlanTestWhitelist(ctx, testFixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: testFixture.userID, Reason: "cross plan fixture", ChangeReason: "prepare ownership check",
	}, testFixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}

	nonTestBody := `{"revision":0,"userId":"` + normalFixture.userID + `","reason":"invalid normal access","changeReason":"must reject normal plan"}`
	nonTest := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+normalFixture.pricePlanID+"/whitelist", bytes.NewBufferString(nonTestBody), token)
	requireWhitelistHTTPError(t, nonTest, http.StatusUnprocessableEntity, "PRICE_PLAN_TEST_REQUIRED")

	crossPlanPatch := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/price-plans/"+otherTestFixture.pricePlanID+"/whitelist/"+created.ID, bytes.NewBufferString(`{"revision":1,"reason":"forged reassignment","changeReason":"cross plan mutation"}`), token)
	requireWhitelistHTTPError(t, crossPlanPatch, http.StatusUnprocessableEntity, "WHITELIST_ENTRY_PRICE_PLAN_MISMATCH")

	crossPlanDisable := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/price-plans/"+otherTestFixture.pricePlanID+"/whitelist/"+created.ID+"/disable", bytes.NewBufferString(`{"revision":1,"changeReason":"cross plan disable"}`), token)
	requireWhitelistHTTPError(t, crossPlanDisable, http.StatusUnprocessableEntity, "WHITELIST_ENTRY_PRICE_PLAN_MISMATCH")

	current, err := store.listPricePlanTestWhitelist(ctx, testFixture.pricePlanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].ID != created.ID || current[0].Revision != 1 || current[0].Status != pricePlanWhitelistStatusActive {
		t.Fatalf("cross-plan request changed entry: %+v", current)
	}
}

func TestPricePlanTestWhitelistAdminHTTPRequiresActorFromContext(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	api := newPricePlanTestWhitelistAdminAPI(&postgresStore{db: db, ready: true})
	revisionZero := int64(0)
	body, err := json.Marshal(pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "no actor", ChangeReason: "actor must come from middleware context",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/price-plans/"+fixture.pricePlanID+"/whitelist", bytes.NewReader(body))
	request.SetPathValue("pricePlanId", fixture.pricePlanID)
	response := httptest.NewRecorder()
	api.create(response, request)
	requireWhitelistHTTPError(t, response, http.StatusForbidden, "FORBIDDEN")
}

func TestRequestContextMiddlewarePropagatesRequestIDToRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestContextMiddleware())
	var observed string
	router.GET("/request-context", func(c *gin.Context) {
		observed = requestIDFromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/request-context", nil)
	request.Header.Set(requestIDHeader, "phase2e-request-123")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || observed != "phase2e-request-123" || response.Header().Get(requestIDHeader) != observed {
		t.Fatalf("status=%d observed=%q responseHeader=%q", response.Code, observed, response.Header().Get(requestIDHeader))
	}
}

func phase2EWhitelistHTTPHandler(t *testing.T, ctx context.Context, db *sql.DB, fixture phase2EFixture) (http.Handler, string) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		update xz_users
		set raw=jsonb_build_object('id',id,'email',email,'name',name,'role',role,'status',status)
		where id=$1
	`, fixture.actorID); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	sessions := newLocalAuthSessions()
	token := "phase2e-whitelist-token-" + fixture.suffix
	if err := sessions.Put(ctx, token, fixture.actorID, time.Hour); err != nil {
		t.Fatal(err)
	}
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler
	return handler, token
}

func decodeWhitelistHTTPResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
}

func authedWhitelistRequest(t *testing.T, handler http.Handler, method, path string, body *bytes.Buffer, token, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(requestIDHeader, requestID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func requireWhitelistHTTPError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d body=%s want=%d/%s", response.Code, response.Body.String(), status, code)
	}
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	decodeWhitelistHTTPResponse(t, response, &payload)
	if payload.Code != code || payload.Message == "" || payload.Error == "" {
		t.Fatalf("unstable error payload=%+v want code=%s", payload, code)
	}
}
