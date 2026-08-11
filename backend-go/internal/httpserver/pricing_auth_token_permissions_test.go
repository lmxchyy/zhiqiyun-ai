package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type pricingPermissionAuthTestStore struct {
	*jsonStore
	permissions map[string][]string
	err         error
}

func (s *pricingPermissionAuthTestStore) PricingPermissionsForRole(_ context.Context, role string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]string(nil), s.permissions[role]...), nil
}

type pricingAuthTokenResponse struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	Permissions  []string `json:"permissions"`
}

func TestPricingPermissionsAreConsistentAcrossLoginRefreshRegisterAndAuthMe(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := &pricingPermissionAuthTestStore{
		jsonStore: newJSONStore(dataPath),
		permissions: map[string][]string{
			"SUPER_ADMIN": {"pricing:plan:view", "pricing:price-plan:default"},
			"MEMBER":      {"pricing:plan:view"},
		},
	}
	sessions := newLocalAuthSessions()
	auth := newAuthAPI(store, sessions)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", auth.login)
	mux.HandleFunc("/api/v1/auth/register", auth.register)
	mux.HandleFunc("/api/v1/auth/refresh", auth.refresh)
	mux.HandleFunc("/api/v1/auth/me", auth.me)

	login := request(t, mux, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"admin@xianzhi.ai","password":"Admin123!"}`))
	loginPayload := decodePricingAuthTokenResponse(t, login)
	assertPricingAuthResponseMatchesMe(t, mux, "login", loginPayload)

	refresh := request(t, mux, http.MethodPost, "/api/v1/auth/refresh", bytes.NewBufferString(`{"refreshToken":"`+loginPayload.RefreshToken+`"}`))
	refreshPayload := decodePricingAuthTokenResponse(t, refresh)
	assertPricingAuthResponseMatchesMe(t, mux, "refresh", refreshPayload)

	register := request(t, mux, http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"Pricing User","email":"pricing-register@example.test","password":"Register123!","confirmPassword":"Register123!"}`))
	registerPayload := decodePricingAuthTokenResponse(t, register)
	assertPricingAuthResponseMatchesMe(t, mux, "register", registerPayload)
}

func TestPricingPermissionFailureDoesNotPersistAnyTokenSession(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	store := &pricingPermissionAuthTestStore{
		jsonStore: newJSONStore(dataPath),
		err:       errors.New("pricing permission query failed"),
	}
	sessions := newLocalAuthSessions().(*localAuthSessions)
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user, ok := findActiveUserByEmail(data.Users, "admin@xianzhi.ai")
	if !ok {
		t.Fatal("seed admin user missing")
	}

	if _, err := newAuthAPI(store, sessions).authResponseWithToken(context.Background(), data, user); err == nil {
		t.Fatal("expected pricing permission failure")
	}
	sessions.mu.Lock()
	defer sessions.mu.Unlock()
	if got := len(sessions.sessions); got != 0 {
		t.Fatalf("permission failure leaked %d token sessions", got)
	}
}

func decodePricingAuthTokenResponse(t *testing.T, response *httptest.ResponseRecorder) pricingAuthTokenResponse {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("auth response status=%d body=%s", result.StatusCode, response.Body.String())
	}
	var payload pricingAuthTokenResponse
	if err := json.NewDecoder(result.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccessToken == "" || payload.RefreshToken == "" {
		t.Fatalf("auth response missing token: %+v", payload)
	}
	return payload
}

func assertPricingAuthResponseMatchesMe(t *testing.T, handler http.Handler, source string, payload pricingAuthTokenResponse) {
	t.Helper()
	me := authedRequest(t, handler, http.MethodGet, "/api/v1/auth/me", nil, payload.AccessToken)
	mePayload := decodePricingAuthTokenResponseWithoutTokens(t, me)
	got := append([]string(nil), payload.Permissions...)
	want := append([]string(nil), mePayload.Permissions...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s permissions=%v auth/me permissions=%v", source, got, want)
	}
}

func decodePricingAuthTokenResponseWithoutTokens(t *testing.T, response *httptest.ResponseRecorder) pricingAuthTokenResponse {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("auth/me status=%d", result.StatusCode)
	}
	var payload pricingAuthTokenResponse
	if err := json.NewDecoder(result.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
