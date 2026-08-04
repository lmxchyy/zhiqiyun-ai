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

func TestAdminPointGiftComputesSourceAndExpiryServerSideAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Gift User", Email: "gift@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	admin := newAdminAPI(store, newLocalAuthSessions())

	request := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/customers/ignored/point-gifts", bytes.NewBufferString(body))
		req.SetPathValue("id", created.ID)
		req = req.WithContext(context.WithValue(context.WithValue(req.Context(), actorIDContextKey, "admin-gift"), actorRoleContextKey, "SUPER_ADMIN"))
		response := httptest.NewRecorder()
		admin.customerPointGift(response, req)
		return response
	}

	first := request(`{"points":11,"reason":"summer campaign","idempotencyKey":"gift-1"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	if !bytes.Contains(first.Body.Bytes(), []byte(`"idempotent":false`)) {
		t.Fatalf("first body=%s", first.Body.String())
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	lots, err := store.PersonalPointService().ListLots(ctx, account.ID, created.ID, PersonalPointLotFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lots) != 1 || lots[0].SourceType != PointSourceAdminGift || lots[0].ExpiresAt.IsZero() || lots[0].ReferenceType != "ADMIN_GIFT" || lots[0].ReferenceID != "gift-1" {
		t.Fatalf("gift lots = %+v", lots)
	}

	replay := request(`{"points":11,"reason":"summer campaign","idempotencyKey":"gift-1"}`)
	if replay.Code != http.StatusOK || !bytes.Contains(replay.Body.Bytes(), []byte(`"idempotent":true`)) {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	lots, err = store.PersonalPointService().ListLots(ctx, account.ID, created.ID, PersonalPointLotFilter{})
	if err != nil || len(lots) != 1 {
		t.Fatalf("replay lots=%+v err=%v", lots, err)
	}

	conflict := request(`{"points":12,"reason":"summer campaign","idempotencyKey":"gift-1"}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestAdminPointGiftRejectsClientEconomicFieldsAndRequiresReasonAndIdempotency(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Gift Validation", Email: "gift-validation@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	admin := newAdminAPI(store, newLocalAuthSessions())

	for name, body := range map[string]string{
		"source":      `{"points":1,"reason":"x","idempotencyKey":"gift-source","source":"RECHARGE"}`,
		"expires_at":  `{"points":1,"reason":"x","idempotencyKey":"gift-expiry","expiresAt":"2099-01-01T00:00:00Z"}`,
		"tenant":      `{"points":1,"reason":"x","idempotencyKey":"gift-tenant","tenantId":"tenant-1"}`,
		"amount":      `{"points":1,"reason":"x","idempotencyKey":"gift-amount","amountCents":1}`,
		"reason":      `{"points":1,"idempotencyKey":"gift-reason"}`,
		"idempotency": `{"points":1,"reason":"missing idempotency"}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/customers/ignored/point-gifts", bytes.NewBufferString(body))
			req.SetPathValue("id", created.ID)
			req = req.WithContext(context.WithValue(req.Context(), actorIDContextKey, "admin-gift"))
			response := httptest.NewRecorder()
			admin.customerPointGift(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAdminPointCorrectionIsPermanentAndDistinctFromGift(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Correction User", Email: "correction@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/customers/ignored/point-corrections", bytes.NewBufferString(`{"points":9,"reason":"ledger repair","idempotencyKey":"correction-1"}`))
	req.SetPathValue("id", created.ID)
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), actorIDContextKey, "admin-correction"), actorRoleContextKey, "SUPER_ADMIN"))
	response := httptest.NewRecorder()
	newAdminAPI(store, newLocalAuthSessions()).customerPointCorrection(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	lots, err := store.PersonalPointService().ListLots(context.Background(), account.ID, created.ID, PersonalPointLotFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lots) != 1 || lots[0].SourceType != PointSourceAdminCorrection || !lots[0].ExpiresAt.IsZero() || lots[0].ReferenceType != "ADMIN_CORRECTION" {
		t.Fatalf("correction lots = %+v", lots)
	}
}

func TestAdminPointCorrectionSupportsNegativeAdjustmentWithConservationAndIdempotency(t *testing.T) {
	ctx := context.Background()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Correction Debit", Email: "correction-debit@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.PointAccount(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().Grant(ctx, PersonalPointGrantCommand{
		AccountID: account.ID, UserID: created.ID, Source: PointSourceRecharge, Points: 10,
		ReferenceType: "ORDER", ReferenceID: "seed-order", IdempotencyKey: "seed-correction-debit",
	}); err != nil {
		t.Fatal(err)
	}
	admin := newAdminAPI(store, newLocalAuthSessions())
	request := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/customers/ignored/point-corrections", bytes.NewBufferString(body))
		req.SetPathValue("id", created.ID)
		req = req.WithContext(context.WithValue(context.WithValue(req.Context(), actorIDContextKey, "admin-correction"), actorRoleContextKey, "SUPER_ADMIN"))
		response := httptest.NewRecorder()
		admin.customerPointCorrection(response, req)
		return response
	}
	first := request(`{"points":-4,"reason":"remove duplicate grant","idempotencyKey":"correction-debit-1"}`)
	if first.Code != http.StatusOK || !bytes.Contains(first.Body.Bytes(), []byte(`"idempotent":false`)) {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	replay := request(`{"points":-4,"reason":"remove duplicate grant","idempotencyKey":"correction-debit-1"}`)
	if replay.Code != http.StatusOK || !bytes.Contains(replay.Body.Bytes(), []byte(`"idempotent":true`)) {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	conflict := request(`{"points":-4,"reason":"different reason","idempotencyKey":"correction-debit-1"}`)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	balance, err := store.PersonalPointService().GetBalance(ctx, account.ID, created.ID)
	if err != nil || balance.Available != 6 {
		t.Fatalf("balance=%+v err=%v", balance, err)
	}
	lots, err := store.PersonalPointService().ListLots(ctx, account.ID, created.ID, PersonalPointLotFilter{})
	if err != nil || len(lots) != 1 || lots[0].OriginalPoints != lots[0].AvailablePoints+lots[0].ReservedPoints+lots[0].ConsumedPoints+lots[0].ExpiredPoints+lots[0].ReversedPoints || lots[0].ReversedPoints != 4 {
		t.Fatalf("lots=%+v err=%v", lots, err)
	}
}

func TestLegacyCustomerProfilePatchWithoutAvailableIsNotDeprecated(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Profile User", Email: "profile@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/customers/ignored", bytes.NewBufferString(`{"name":"Updated Profile"}`))
	req.SetPathValue("id", created.ID)
	response := httptest.NewRecorder()
	newAdminAPI(store, newLocalAuthSessions()).updateCustomer(response, req)
	if response.Code != http.StatusOK || response.Header().Get("Deprecation") != "" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

func TestLegacyAvailableRequiresSecondaryCorrectionPermissionWithoutBlockingProfilePatch(t *testing.T) {
	profile := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/customers/user-1", bytes.NewBufferString(`{"name":"Profile"}`))
	if got := adminPermissionsForRequest(profile, "admin.write"); len(got) != 1 || got[0] != "admin.write" {
		t.Fatalf("profile permissions=%v", got)
	}
	available := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/customers/user-1", bytes.NewBufferString(`{"name":"Profile","available":17}`))
	got := adminPermissionsForRequest(available, "admin.write")
	if len(got) != 2 || got[0] != "admin.write" || got[1] != "points:balance:correct" {
		t.Fatalf("available permissions=%v", got)
	}
	var body map[string]any
	if err := json.NewDecoder(available.Body).Decode(&body); err != nil || body["available"] != float64(17) {
		t.Fatalf("permission inspection consumed body=%v err=%v", body, err)
	}
}

func TestLegacyCustomerCreateAndDecreaseUsePermanentCorrectionLots(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	admin := newAdminAPI(store, newLocalAuthSessions())
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/customers", bytes.NewBufferString(`{"name":"Legacy Create","email":"legacy-create@example.test","available":23}`))
	createRequest = createRequest.WithContext(context.WithValue(context.WithValue(createRequest.Context(), actorIDContextKey, "admin-correction"), actorRoleContextKey, "SUPER_ADMIN"))
	createdResponse := httptest.NewRecorder()
	admin.createCustomer(createdResponse, createRequest)
	if createdResponse.Code != http.StatusOK || createdResponse.Header().Get("Deprecation") != "true" {
		t.Fatalf("create status=%d headers=%v body=%s", createdResponse.Code, createdResponse.Header(), createdResponse.Body.String())
	}
	var createdPayload struct {
		Item adminUser `json:"item"`
	}
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &createdPayload); err != nil || createdPayload.Item.ID == "" {
		t.Fatalf("create payload=%+v err=%v", createdPayload, err)
	}
	account, err := store.PointAccount(createdPayload.Item.ID)
	if err != nil || account.Available != 23 {
		t.Fatalf("created account=%+v err=%v", account, err)
	}
	lots, err := store.PersonalPointService().ListLots(context.Background(), account.ID, createdPayload.Item.ID, PersonalPointLotFilter{})
	if err != nil || len(lots) != 1 || lots[0].SourceType != PointSourceAdminCorrection || !lots[0].ExpiresAt.IsZero() {
		t.Fatalf("create lots=%+v err=%v", lots, err)
	}

	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/customers/ignored", bytes.NewBufferString(`{"available":9}`))
	patchRequest.SetPathValue("id", createdPayload.Item.ID)
	patchRequest = patchRequest.WithContext(context.WithValue(context.WithValue(patchRequest.Context(), actorIDContextKey, "admin-correction"), actorRoleContextKey, "SUPER_ADMIN"))
	patchResponse := httptest.NewRecorder()
	admin.updateCustomer(patchResponse, patchRequest)
	if patchResponse.Code != http.StatusOK || patchResponse.Header().Get("Deprecation") != "true" {
		t.Fatalf("patch status=%d headers=%v body=%s", patchResponse.Code, patchResponse.Header(), patchResponse.Body.String())
	}
	balance, err := store.PersonalPointService().GetBalance(context.Background(), account.ID, createdPayload.Item.ID)
	if err != nil || balance.Available != 9 {
		t.Fatalf("decreased balance=%+v err=%v", balance, err)
	}
	lots, err = store.PersonalPointService().ListLots(context.Background(), account.ID, createdPayload.Item.ID, PersonalPointLotFilter{})
	if err != nil || len(lots) != 1 || lots[0].AvailablePoints != 9 || lots[0].ReversedPoints != 14 || lots[0].OriginalPoints != 23 {
		t.Fatalf("decreased lots=%+v err=%v", lots, err)
	}
}

func TestLegacyAbsoluteBalanceIsDeprecatedAndNeverCreatesAdminGift(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	created, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Legacy User", Email: "legacy@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/customers/ignored", bytes.NewBufferString(`{"available":959}`))
	req.SetPathValue("id", created.ID)
	req = req.WithContext(context.WithValue(context.WithValue(req.Context(), actorIDContextKey, "admin-correction"), actorRoleContextKey, "SUPER_ADMIN"))
	response := httptest.NewRecorder()
	newAdminAPI(store, newLocalAuthSessions()).updateCustomer(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Deprecation") != "true" || response.Header().Get("Sunset") == "" || response.Header().Get("Link") == "" {
		t.Fatalf("legacy headers = %#v", response.Header())
	}
	account, err := store.PointAccount(created.ID)
	if err != nil || account.Available != 959 {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	lots, err := store.PersonalPointService().ListLots(context.Background(), account.ID, created.ID, PersonalPointLotFilter{Source: PointSourceAdminGift})
	if err != nil || len(lots) != 0 {
		t.Fatalf("admin gift lots=%+v err=%v", lots, err)
	}
}
