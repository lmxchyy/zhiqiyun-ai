package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPersonalPointAdminPermissionsAreSeparated(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/v1/admin/points/expiry-policy", "points:gift-policy:view"},
		{http.MethodPut, "/api/v1/admin/points/expiry-policy", "points:gift-policy:manage"},
		{http.MethodPost, "/api/v1/admin/customers/user-1/point-gifts", "points:gift:grant"},
		{http.MethodPost, "/api/v1/admin/customers/user-1/point-corrections", "points:balance:correct"},
		{http.MethodGet, "/api/v1/admin/customers/user-1/point-lots", "points:lot:view"},
	}
	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, nil)
		if got := adminPermissionForRequest(req); got != test.want {
			t.Fatalf("%s %s permission = %q, want %q", test.method, test.path, got, test.want)
		}
	}
}

func TestPersonalPointPolicyAdminAPIRejectsUnknownEconomicFieldsAndStaleRevision(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	admin := newAdminAPI(store, newLocalAuthSessions())

	get := httptest.NewRecorder()
	admin.pointExpiryPolicy(get, httptest.NewRequest(http.MethodGet, "/api/v1/admin/points/expiry-policy", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	var body struct {
		Item PointExpiryPolicy `json:"item"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	unknown := httptest.NewRecorder()
	unknownReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewBufferString(`{"revision":1,"enabled":true,"durationValue":3,"changeReason":"no client source","sourceTypes":["RECHARGE"]}`))
	unknownReq = unknownReq.WithContext(context.WithValue(unknownReq.Context(), actorIDContextKey, "admin-policy"))
	admin.pointExpiryPolicy(unknown, unknownReq)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown economic field status=%d body=%s", unknown.Code, unknown.Body.String())
	}

	put := httptest.NewRecorder()
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewBufferString(`{"revision":1,"enabled":false,"durationValue":3,"changeReason":"pause campaign gifts"}`))
	putReq = putReq.WithContext(context.WithValue(putReq.Context(), actorIDContextKey, "admin-policy"))
	admin.pointExpiryPolicy(put, putReq)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", put.Code, put.Body.String())
	}

	stale := httptest.NewRecorder()
	staleReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewBufferString(`{"revision":1,"enabled":true,"durationValue":3,"changeReason":"stale update"}`))
	staleReq = staleReq.WithContext(context.WithValue(staleReq.Context(), actorIDContextKey, "admin-policy"))
	admin.pointExpiryPolicy(stale, staleReq)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestPersonalPointAdminLotQueryUsesServerResolvedAccount(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Lot User", Email: "lot@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: account.ID, UserID: created.ID, Source: PointSourceAdminGift, Points: 7, IdempotencyKey: "lot-query",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/customers/ignored/point-lots", nil)
	req.SetPathValue("id", created.ID)
	response := httptest.NewRecorder()
	newAdminAPI(store, newLocalAuthSessions()).customerPointLots(response, req)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"source_type":"ADMIN_GIFT"`)) {
		t.Fatalf("lot query status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPointAccountResponseAddsPersonalExpirySummaryWithoutChangingLegacyFields(t *testing.T) {
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Wallet User", Email: "wallet@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().Grant(ctx, PersonalPointGrantCommand{
		AccountID: account.ID, UserID: created.ID, Source: PointSourceAdminGift, Points: 7, IdempotencyKey: "wallet-summary",
	}); err != nil {
		t.Fatal(err)
	}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, "wallet-token", created.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/points/account", nil)
	req.Header.Set("Authorization", "Bearer wallet-token")
	req.Header.Set("X-Tenant-Id", "tenant-must-not-affect-personal-wallet")
	response := httptest.NewRecorder()
	api{store: store, sessions: sessions}.pointAccount(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Account struct {
			Available          int64  `json:"available"`
			Frozen             int64  `json:"frozen"`
			Total              int64  `json:"total"`
			PermanentAvailable int64  `json:"permanentAvailable"`
			ExpiringAvailable  int64  `json:"expiringAvailable"`
			NextExpiryAt       string `json:"nextExpiryAt"`
			NextExpiryPoints   int64  `json:"nextExpiryPoints"`
		} `json:"account"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Account.Available != 7 || payload.Account.Frozen != 0 || payload.Account.Total != 7 || payload.Account.PermanentAvailable != 0 || payload.Account.ExpiringAvailable != 7 || payload.Account.NextExpiryPoints != 7 || payload.Account.NextExpiryAt == "" {
		t.Fatalf("wallet summary = %+v; body=%s", payload.Account, response.Body.String())
	}
}
