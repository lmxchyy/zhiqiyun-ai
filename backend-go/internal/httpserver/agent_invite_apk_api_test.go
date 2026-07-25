package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

type fakeAgentInviteStore struct {
	mu            sync.Mutex
	invite        agentInviteInfo
	resolveErr    error
	registerErr   error
	registrations map[string]agentInviteRegistrationResult
	release       appRelease
	downloadCount int
	funnel        agentInviteFunnel
}

func TestAgentInviteShortLinkUsesDedicatedH5Prefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/d/ABCD1234", nil)
	req.SetPathValue("inviteCode", "abcd1234")
	response := httptest.NewRecorder()

	agentInviteH5Redirect(response, req)

	if response.Code != http.StatusFound {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusFound)
	}
	if got, want := response.Header().Get("Location"), "/h5/#/pages/invite/InviteRegisterPage?inviteCode=ABCD1234"; got != want {
		t.Fatalf("location=%q want=%q", got, want)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control=%q want=no-store", got)
	}
}

func TestUserH5StaticRouteDoesNotReplaceAdminRoot(t *testing.T) {
	adminRoot := t.TempDir()
	h5Root := t.TempDir()
	if err := os.WriteFile(filepath.Join(adminRoot, "index.html"), []byte("admin-root"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h5Root, "index.html"), []byte("invite-h5"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := New(config.Config{
		Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"),
		StaticDir: adminRoot, AdminStaticDir: adminRoot, UserH5StaticDir: h5Root,
	})

	root := request(t, server.Handler, http.MethodGet, "/", nil)
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), "admin-root") {
		t.Fatalf("root status=%d body=%q", root.Code, root.Body.String())
	}
	h5 := request(t, server.Handler, http.MethodGet, "/h5/", nil)
	if h5.Code != http.StatusOK || !strings.Contains(h5.Body.String(), "invite-h5") {
		t.Fatalf("h5 status=%d body=%q", h5.Code, h5.Body.String())
	}
}

func (s *fakeAgentInviteStore) ResolveAgentInvite(_ context.Context, _ string) (agentInviteInfo, error) {
	if s.resolveErr != nil {
		return agentInviteInfo{}, s.resolveErr
	}
	return s.invite, nil
}

func (s *fakeAgentInviteStore) FindAgentInviteRegistration(_ context.Context, key string) (agentInviteRegistrationResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.registrations[key]
	return item, ok, nil
}

func (s *fakeAgentInviteStore) RegisterAgentInvite(_ context.Context, invite agentInviteInfo, input agentInviteRegistrationInput) (agentInviteRegistrationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registerErr != nil {
		return agentInviteRegistrationResult{}, s.registerErr
	}
	result := agentInviteRegistrationResult{
		UserID: "user_new", Invite: invite, RegistrationEventID: input.RegistrationEvent,
		RegistrationStatus: "created", RelationshipStatus: "locked", Created: true,
	}
	s.registrations[input.IdempotencyKeyHash] = result
	return result, nil
}

func (s *fakeAgentInviteStore) RecordAgentInviteEvent(context.Context, agentInviteInfo, string, string, string) error {
	return nil
}

func (s *fakeAgentInviteStore) LatestAppRelease(context.Context, string, string) (appRelease, error) {
	if s.release.ID == "" {
		return appRelease{}, errAppReleaseUnavailable
	}
	return s.release, nil
}

func (s *fakeAgentInviteStore) RecordAPKDownload(context.Context, appRelease, string, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloadCount++
	return nil
}

func (s *fakeAgentInviteStore) AgentInviteProfile(context.Context, string) (agentInviteInfo, agentInviteFunnel, error) {
	if s.resolveErr != nil {
		return agentInviteInfo{}, agentInviteFunnel{}, s.resolveErr
	}
	return s.invite, s.funnel, nil
}

func (s *fakeAgentInviteStore) SaveAgentInviteLanding(context.Context, agentInviteInfo, string) error {
	return nil
}

func (s *fakeAgentInviteStore) RecordAppActivation(context.Context, string, string, string) error {
	return nil
}

func newAgentInviteTestAPI(store *fakeAgentInviteStore) (*agentInviteAPI, *localAuthSessions) {
	sessions := newLocalAuthSessions().(*localAuthSessions)
	return &agentInviteAPI{
		store: store,
		loadData: func() (adminPlatformData, error) {
			return adminPlatformData{}, nil
		},
		auth:                authAPI{sessions: sessions, flow: newAuthFlowCoordinator()},
		registrationEnabled: true,
		downloadEnabled:     true,
		activationEnabled:   true,
	}, sessions
}

func seedInviteSMS(t *testing.T, sessions *localAuthSessions, mobile, code string) {
	t.Helper()
	err := sessions.PutSMSChallenge(context.Background(), mobile, smsChallenge{
		codeHash: authCodeHash(mobile, code), expiresAt: time.Now().Add(time.Minute),
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
}

func inviteRegisterRequest(code, mobile, sms, key string) *http.Request {
	body := `{"mobile":"` + mobile + `","sms_code":"` + sms + `","agreement_accepted":true,"privacy_accepted":true}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/public/invites/"+code+"/register", strings.NewReader(body))
	request.SetPathValue("inviteCode", code)
	request.Header.Set("Idempotency-Key", key)
	return request
}

func TestAgentInviteRegistrationIsBoundAndIdempotent(t *testing.T) {
	store := &fakeAgentInviteStore{
		invite: agentInviteInfo{
			AgentID: "agent_1", InviterUserID: "user_agent", TenantID: "tenant_default",
			InviteCode: "A7K9M2QX", DisplayName: "华东代理商", AgentStatus: "ACTIVE", RegistrationOK: true,
		},
		registrations: map[string]agentInviteRegistrationResult{},
	}
	api, sessions := newAgentInviteTestAPI(store)
	seedInviteSMS(t, sessions, "13800138000", "246810")

	first := httptest.NewRecorder()
	api.register(first, inviteRegisterRequest("A7K9M2QX", "13800138000", "246810", "registration-key-00000001"))
	if first.Code != http.StatusOK {
		t.Fatalf("first registration status=%d body=%s", first.Code, first.Body.String())
	}
	var firstPayload map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
		t.Fatal(err)
	}
	if firstPayload["relationshipStatus"] != "locked" || firstPayload["registered"] != true {
		t.Fatalf("unexpected registration payload: %#v", firstPayload)
	}

	replay := httptest.NewRecorder()
	api.register(replay, inviteRegisterRequest("A7K9M2QX", "13800138000", "expired-on-replay", "registration-key-00000001"))
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), `"idempotentReplay":true`) {
		t.Fatalf("idempotent replay status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestAgentInviteIdempotencyKeyIsScopedToInviteAndMobile(t *testing.T) {
	store := &fakeAgentInviteStore{
		invite: agentInviteInfo{
			AgentID: "agent_1", InviterUserID: "user_agent", TenantID: "tenant_default",
			InviteCode: "A7K9M2QX", DisplayName: "华东合作伙伴", AgentStatus: "ACTIVE", RegistrationOK: true,
		},
		registrations: map[string]agentInviteRegistrationResult{},
	}
	api, sessions := newAgentInviteTestAPI(store)
	seedInviteSMS(t, sessions, "13800138000", "246810")
	seedInviteSMS(t, sessions, "13900139000", "135790")

	first := httptest.NewRecorder()
	api.register(first, inviteRegisterRequest("A7K9M2QX", "13800138000", "246810", "shared-registration-key"))
	second := httptest.NewRecorder()
	api.register(second, inviteRegisterRequest("A7K9M2QX", "13900139000", "135790", "shared-registration-key"))

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("first=%d second=%d", first.Code, second.Code)
	}
	if strings.Contains(second.Body.String(), `"idempotentReplay":true`) {
		t.Fatalf("second mobile incorrectly replayed first registration: %s", second.Body.String())
	}
	if len(store.registrations) != 2 {
		t.Fatalf("registration keys=%d want=2", len(store.registrations))
	}
}

func TestAgentInviteRegistrationRejectsExistingRelationshipCases(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		storeError error
		wantCode   string
	}{
		{name: "existing user without relation", storeError: errInviteExistingUnbound, wantCode: "EXISTING_USER_BINDING_REQUIRED"},
		{name: "bound to another agent", storeError: errInviteAlreadyBoundOther, wantCode: "AGENT_RELATION_LOCKED"},
		{name: "agent disabled", storeError: errInviteUnavailable, wantCode: "INVITE_CODE_UNAVAILABLE"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &fakeAgentInviteStore{
				invite:      agentInviteInfo{AgentID: "agent_1", InviteCode: "A7K9M2QX", AgentStatus: "ACTIVE", RegistrationOK: true},
				registerErr: testCase.storeError, registrations: map[string]agentInviteRegistrationResult{},
			}
			api, sessions := newAgentInviteTestAPI(store)
			seedInviteSMS(t, sessions, "13800138000", "246810")
			response := httptest.NewRecorder()
			api.register(response, inviteRegisterRequest("A7K9M2QX", "13800138000", "246810", "registration-key-"+testCase.name))
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), testCase.wantCode) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAgentInviteInvalidCodeCannotRegister(t *testing.T) {
	store := &fakeAgentInviteStore{resolveErr: errInviteUnavailable, registrations: map[string]agentInviteRegistrationResult{}}
	api, sessions := newAgentInviteTestAPI(store)
	seedInviteSMS(t, sessions, "13800138000", "246810")
	response := httptest.NewRecorder()
	api.register(response, inviteRegisterRequest("INVALID", "13800138000", "246810", "registration-key-invalid"))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "INVITE_CODE_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDisabledAgentInviteCannotRegister(t *testing.T) {
	store := &fakeAgentInviteStore{
		invite:        agentInviteInfo{AgentID: "agent_disabled", InviteCode: "D7S9B2LX", AgentStatus: "DISABLED", RegistrationOK: false},
		registrations: map[string]agentInviteRegistrationResult{},
	}
	api, sessions := newAgentInviteTestAPI(store)
	seedInviteSMS(t, sessions, "13800138000", "246810")
	response := httptest.NewRecorder()
	api.register(response, inviteRegisterRequest("D7S9B2LX", "13800138000", "246810", "registration-key-disabled"))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "INVITE_CODE_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAndroidLatestDownloadFollowsPublishedReleaseAndRollback(t *testing.T) {
	store := &fakeAgentInviteStore{
		registrations: map[string]agentInviteRegistrationResult{},
		release:       appRelease{ID: "release_100", Platform: "android", Channel: "official", VersionName: "1.0.0", VersionCode: 100, APKURL: "https://cdn.example.com/android/releases/1.0.0/zhiqiyun-ai-1.0.0.apk", SHA256: strings.Repeat("a", 64), Status: "published"},
	}
	api, _ := newAgentInviteTestAPI(store)
	assertDownload := func(want string) {
		t.Helper()
		response := httptest.NewRecorder()
		api.download(response, httptest.NewRequest(http.MethodGet, "/android/latest", nil))
		if response.Code != http.StatusFound || response.Header().Get("Location") != want || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d location=%s cache=%s", response.Code, response.Header().Get("Location"), response.Header().Get("Cache-Control"))
		}
	}
	assertDownload(store.release.APKURL)

	old := store.release
	store.release = appRelease{ID: "release_101", Platform: "android", Channel: "official", VersionName: "1.0.1", VersionCode: 101, APKURL: "https://cdn.example.com/android/releases/1.0.1/zhiqiyun-ai-1.0.1.apk", SHA256: strings.Repeat("b", 64), Status: "published"}
	assertDownload(store.release.APKURL)

	store.release = old
	assertDownload(old.APKURL)
	if store.downloadCount != 3 {
		t.Fatalf("download events=%d want=3", store.downloadCount)
	}
}

func TestAgentInviteEmergencySwitches(t *testing.T) {
	store := &fakeAgentInviteStore{
		invite: agentInviteInfo{
			AgentID: "agent_1", InviteCode: "A7K9M2QX", AgentStatus: "ACTIVE", RegistrationOK: true,
		},
		registrations: map[string]agentInviteRegistrationResult{},
		release: appRelease{
			ID: "release_024", Platform: "android", Channel: "official", VersionName: "0.2.4",
			VersionCode: 24, APKURL: "https://cdn.example.com/android/releases/0.2.4/zhiqiyun-ai-0.2.4.apk",
			SHA256: strings.Repeat("a", 64), Status: "published",
		},
	}
	api, _ := newAgentInviteTestAPI(store)
	api.registrationEnabled = false
	api.downloadEnabled = false
	api.activationEnabled = false

	inviteResponse := httptest.NewRecorder()
	inviteRequest := httptest.NewRequest(http.MethodGet, "/api/v1/public/invites/A7K9M2QX", nil)
	inviteRequest.SetPathValue("inviteCode", "A7K9M2QX")
	api.invite(inviteResponse, inviteRequest)
	if inviteResponse.Code != http.StatusOK ||
		!strings.Contains(inviteResponse.Body.String(), `"valid":true`) ||
		!strings.Contains(inviteResponse.Body.String(), `"registrationAllowed":false`) {
		t.Fatalf("invite status=%d body=%s", inviteResponse.Code, inviteResponse.Body.String())
	}

	registerResponse := httptest.NewRecorder()
	api.register(registerResponse, inviteRegisterRequest("A7K9M2QX", "13800138000", "246810", "registration-key-disabled-globally"))
	if registerResponse.Code != http.StatusServiceUnavailable ||
		!strings.Contains(registerResponse.Body.String(), "AGENT_INVITE_REGISTRATION_DISABLED") {
		t.Fatalf("register status=%d body=%s", registerResponse.Code, registerResponse.Body.String())
	}

	downloadResponse := httptest.NewRecorder()
	api.download(downloadResponse, httptest.NewRequest(http.MethodGet, "/android/latest", nil))
	if downloadResponse.Code != http.StatusServiceUnavailable ||
		!strings.Contains(downloadResponse.Body.String(), "APK_DOWNLOAD_DISABLED") ||
		store.downloadCount != 0 {
		t.Fatalf("download status=%d body=%s events=%d", downloadResponse.Code, downloadResponse.Body.String(), store.downloadCount)
	}

	activationResponse := httptest.NewRecorder()
	api.activation(activationResponse, httptest.NewRequest(http.MethodPost, "/api/v1/app/activation", strings.NewReader(`{}`)))
	if activationResponse.Code != http.StatusOK ||
		!strings.Contains(activationResponse.Body.String(), `"reportingEnabled":false`) {
		t.Fatalf("activation status=%d body=%s", activationResponse.Code, activationResponse.Body.String())
	}
}

func TestAgentInvitePosterRequiresAgentAuthentication(t *testing.T) {
	store := &fakeAgentInviteStore{
		invite:        agentInviteInfo{AgentID: "agent_1", InviterUserID: "user_agent", TenantID: "tenant_default", InviteCode: "A7K9M2QX", DisplayName: "华东代理商", AgentStatus: "ACTIVE"},
		registrations: map[string]agentInviteRegistrationResult{},
	}
	api, sessions := newAgentInviteTestAPI(store)
	response := httptest.NewRecorder()
	api.poster(response, httptest.NewRequest(http.MethodPost, "/api/v1/agent/invite/poster", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated poster status=%d", response.Code)
	}
	if err := sessions.Put(context.Background(), "agent-token", "user_agent", time.Minute); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/invite/poster", nil)
	request.Header.Set("Authorization", "Bearer agent-token")
	response = httptest.NewRecorder()
	api.poster(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"qrCodeDataUrl":"data:image/png;base64,`) {
		t.Fatalf("authenticated poster status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateAppRelease(t *testing.T) {
	valid := appRelease{Platform: "android", Channel: "official", VersionName: "1.0.0", VersionCode: 100, APKURL: "https://cdn.example.com/android/releases/1.0.0/app.apk", SHA256: strings.Repeat("a", 64)}
	if err := validateAppRelease(valid); err != nil {
		t.Fatal(err)
	}
	valid.APKURL = "https://cdn.example.com/latest"
	if validateAppRelease(valid) == nil {
		t.Fatal("expected non-versioned non-apk URL to be rejected")
	}
}

func TestAgentInviteDownloadRejectsUnversionedAPKURL(t *testing.T) {
	store := &fakeAgentInviteStore{
		registrations: map[string]agentInviteRegistrationResult{},
		release: appRelease{
			ID: "release_invalid", Platform: "android", Channel: "official",
			VersionName: "1.2.3", VersionCode: 123,
			APKURL: "https://cdn.example.com/android/latest.apk",
			SHA256: strings.Repeat("a", 64), Status: "published",
		},
	}
	api, _ := newAgentInviteTestAPI(store)
	response := httptest.NewRecorder()

	api.download(response, httptest.NewRequest(http.MethodGet, "/android/latest", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusServiceUnavailable)
	}
	if store.downloadCount != 0 {
		t.Fatalf("download events=%d want=0", store.downloadCount)
	}
}

func TestSecureAgentInviteCodeFormat(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		code := secureAgentInviteCode()
		if len(code) != 12 || strings.ToUpper(code) != code {
			t.Fatalf("invalid secure invite code %q", code)
		}
		if seen[code] {
			t.Fatalf("duplicate secure invite code %q", code)
		}
		seen[code] = true
	}
}

func TestAgentInvitePublicDisplayNameIsSanitized(t *testing.T) {
	if got := sanitizeAgentInviteDisplayName(" \n华东\u0000合作伙伴\t "); got != "华东合作伙伴" {
		t.Fatalf("sanitized display name=%q", got)
	}
	if got := sanitizeAgentInviteDisplayName(" \r\n "); got != "知启云AI合作代理商" {
		t.Fatalf("fallback display name=%q", got)
	}
	if got := []rune(sanitizeAgentInviteDisplayName(strings.Repeat("代", 50))); len(got) != 40 {
		t.Fatalf("display name length=%d want=40", len(got))
	}
}
