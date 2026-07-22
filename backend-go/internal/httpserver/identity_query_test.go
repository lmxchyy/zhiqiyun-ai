package httpserver

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestAdminIdentityReadEndpoints(t *testing.T) {
	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	handler := server.Handler
	adminToken := loginToken(t, handler, "admin@xianzhi.ai", "Admin123!")

	profileResponse := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers/user_000003/identity-profile", nil, adminToken)
	if profileResponse.Code != http.StatusOK {
		t.Fatalf("identity profile status = %d, body = %s", profileResponse.Code, profileResponse.Body.String())
	}
	var profilePayload struct {
		Item adminIdentityProfile `json:"item"`
	}
	if err := json.NewDecoder(profileResponse.Body).Decode(&profilePayload); err != nil {
		t.Fatal(err)
	}
	if profilePayload.Item.UserID != "user_000003" || profilePayload.Item.PrimaryIdentity != "AGENT" {
		t.Fatalf("unexpected identity profile: %+v", profilePayload.Item)
	}
	if !containsString(profilePayload.Item.AccountRoles, roleAgent) {
		t.Fatalf("agent account role missing: %+v", profilePayload.Item.AccountRoles)
	}

	paths := []string{
		"/api/v1/admin/customers/user_000003/identity-history",
		"/api/v1/admin/customers/user_000003/relationship",
		"/api/v1/admin/customers/user_000003/relationship-history",
		"/api/v1/admin/customers/user_000003/identity-financial-overview",
	}
	for _, path := range paths {
		response := authedRequest(t, handler, http.MethodGet, path, nil, adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}

	missing := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers/missing/identity-profile", nil, adminToken)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing identity profile status = %d, body = %s", missing.Code, missing.Body.String())
	}

	memberToken := loginToken(t, handler, "demo@xianzhi.ai", "Demo123!")
	forbidden := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/customers/user_000003/identity-profile", nil, memberToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member identity admin query status = %d, body = %s", forbidden.Code, forbidden.Body.String())
	}
}
