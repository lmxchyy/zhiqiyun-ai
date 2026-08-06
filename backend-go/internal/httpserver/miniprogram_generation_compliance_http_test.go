package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type miniProgramComplianceVideoProvider struct {
	calls atomic.Int32
}

func (p *miniProgramComplianceVideoProvider) DefaultModel() string {
	return "doubao-seedance-2.0"
}

func (p *miniProgramComplianceVideoProvider) Create(context.Context, generation.CreateRequest) (any, error) {
	p.calls.Add(1)
	return map[string]any{"provider": "local-mock-video"}, nil
}

func TestMiniProgramGenerationRejectsActiveUnapprovedModelBeforeSideEffects(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	userID := "miniprogram-compliance-user-" + suffix
	token := "miniprogram-compliance-token-" + suffix

	if _, err := db.ExecContext(ctx, `
		insert into xz_users(id,email,name,role,member_level,status,plan_id,created_at,updated_at,raw)
		values($1,$1||'@example.test',$1,'MEMBER','PRO','ACTIVE','plan_month',now()::text,now()::text,
			jsonb_build_object('id',$1::text,'email',$1::text||'@example.test','name',$1::text,'role','MEMBER',
				'memberLevel','PRO','status','ACTIVE','planId','plan_month','tenantId','tenant_default'))
	`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_user_agreement_acceptances where user_id=$1`, userID)
		_, _ = db.Exec(`delete from xz_users where id=$1`, userID)
	})

	sessions := newLocalAuthSessions()
	if err := sessions.Put(ctx, token, userID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := sessions.(*localAuthSessions).PutWeChatSession(ctx, userID, wechatMiniProgramSession{OpenID: "openid-" + suffix, SessionKey: "session-key"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	baseAPI := api{store: store, sessions: sessions}
	acceptRequest := httptest.NewRequest(http.MethodPost, "/api/v1/legal/acceptances", nil)
	setCompleteMiniProgramHeaders(acceptRequest, token)
	acceptResponse := httptest.NewRecorder()
	baseAPI.acceptCurrentLegalDocuments(acceptResponse, acceptRequest)
	if acceptResponse.Code != http.StatusOK {
		t.Fatalf("accept current legal documents status=%d body=%s", acceptResponse.Code, acceptResponse.Body.String())
	}

	for _, testCase := range []struct {
		name        string
		legacyValue string
	}{
		{name: "default"},
		{name: "legacy bypass environment variable is ignored", legacyValue: "true"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("MINIPROGRAM_VIDEO_COMPLIANCE_BYPASS", testCase.legacyValue)
			provider := &miniProgramComplianceVideoProvider{}
			generationAPI := api{
				store: store,
				generationService: generation.NewServiceWithOptions(generation.ServiceOptions{
					VideoProvider: provider,
				}),
				sessions:        sessions,
				taskCancels:     &sync.Map{},
				pptVisualTasks:  &sync.Map{},
				contentSecurity: staticImageSecurityChecker{},
			}
			clientRequestID := "miniprogram-compliance-generation-" + suffix + "-" + strconv.Itoa(len(testCase.name))
			assertMiniProgramGenerationSideEffects(t, db, clientRequestID, 0, 0)

			body := fmt.Sprintf(`{
				"clientRequestId":%q,
				"module_code":"video_generation",
				"type":"TEXT_TO_VIDEO",
				"prompt":"unapproved Seedance model must be rejected before side effects",
				"model":"doubao-seedance-2.0",
				"params":{"duration":5,"resolution":"720p","aspect_ratio":"16:9"}
			}`, clientRequestID)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks", bytes.NewBufferString(body))
			setCompleteMiniProgramHeaders(request, token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", clientRequestID)
			response := httptest.NewRecorder()
			generationAPI.createGenerationTask(response, request)

			if response.Code != http.StatusForbidden {
				t.Errorf("generation status=%d want=%d body=%s", response.Code, http.StatusForbidden, response.Body.String())
			}
			assertMiniProgramGenerationSideEffects(t, db, clientRequestID, 0, 0)
			if calls := provider.calls.Load(); calls != 0 {
				t.Errorf("video provider calls=%d want=0", calls)
			}
		})
	}
}

func setCompleteMiniProgramHeaders(request *http.Request, token string) {
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Client-Platform", "mp-weixin")
	request.Header.Set("X-Client-Name", "xianzhi-user-mini-program")
	request.Header.Set("X-Client-Version", "2.0.40")
	request.Header.Set("X-Client-Language", "zh-CN")
}

func assertMiniProgramGenerationSideEffects(t *testing.T, db *sql.DB, clientRequestID string, wantTasks, wantBilling int) {
	t.Helper()
	var tasks, billing int
	if err := db.QueryRow(`select count(*) from xz_generation_tasks where client_request_id=$1`, clientRequestID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`
		select
			(select count(*) from xz_billing_events where task_id in (select id from xz_generation_tasks where client_request_id=$1)) +
			(select count(*) from xz_billing_lifecycle_events where task_id in (select id from xz_generation_tasks where client_request_id=$1))
	`, clientRequestID).Scan(&billing); err != nil {
		t.Fatal(err)
	}
	if tasks != wantTasks || billing != wantBilling {
		t.Errorf("side effects tasks=%d billing=%d want=%d/%d", tasks, billing, wantTasks, wantBilling)
	}
}
