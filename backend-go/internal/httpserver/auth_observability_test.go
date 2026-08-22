package httpserver

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthLoginFailureLogContainsStageAndCorrelationFields(t *testing.T) {
	previousWriter := log.Writer()
	defer log.SetOutput(previousWriter)
	var output bytes.Buffer
	log.SetOutput(&output)

	ctx := context.WithValue(context.Background(), requestIDContextKey, "req-observe-001")
	logAuthFlowFailure(ctx, authLoginStageUserLookup, errors.New("phone 15729336229 lookup failed"), time.Now())

	line := output.String()
	for _, field := range []string{
		"auth_flow=wechat_phone_login",
		"stage=user_lookup",
		"request_id=req-observe-001",
		"error_class=internal",
		"http_mapping=AUTH_INTERNAL_ERROR",
		"duration_ms=",
	} {
		if !strings.Contains(line, field) {
			t.Fatalf("diagnostic log missing %q: %s", field, line)
		}
	}
}

func TestAuthLoginFailureLogDoesNotExposeSensitiveValues(t *testing.T) {
	previousWriter := log.Writer()
	defer log.SetOutput(previousWriter)
	var output bytes.Buffer
	log.SetOutput(&output)

	ctx := context.WithValue(context.Background(), requestIDContextKey, "req-sensitive-001")
	rootCause := errors.New("phone=15729336229 openid=openid-secret unionid=unionid-secret code=wechat-code-secret token=access-token-secret secret=app-secret")
	logAuthFlowFailure(ctx, authLoginStageTokenSession, rootCause, time.Now())

	line := output.String()
	for _, secret := range []string{
		"15729336229",
		"openid-secret",
		"unionid-secret",
		"wechat-code-secret",
		"access-token-secret",
		"app-secret",
	} {
		if strings.Contains(line, secret) {
			t.Fatalf("diagnostic log exposed %q: %s", secret, line)
		}
	}
}

func TestAuthLoginFailureStagesRemainStable(t *testing.T) {
	stages := []authLoginStage{
		authLoginStageWechatCode2Session,
		authLoginStageWechatPhoneExchange,
		authLoginStagePhoneNormalize,
		authLoginStageUserLookup,
		authLoginStageUserCreate,
		authLoginStagePointAccountInit,
		authLoginStageRegistrationGrant,
		authLoginStageIdentityBind,
		authLoginStageTokenSession,
		authLoginStageResponseBuild,
	}
	want := []string{
		"wechat_code2session",
		"wechat_phone_exchange",
		"phone_normalize",
		"user_lookup",
		"user_create",
		"point_account_init",
		"registration_grant",
		"identity_bind",
		"token_session",
		"response_build",
	}
	for i, stage := range stages {
		if string(stage) != want[i] {
			t.Fatalf("stage %d = %q, want %q", i, stage, want[i])
		}
	}
}

func TestAuthLoginFailureLogUsesSpecificWrappedStage(t *testing.T) {
	previousWriter := log.Writer()
	defer log.SetOutput(previousWriter)

	for _, testCase := range []struct {
		name  string
		stage authLoginStage
	}{
		{name: "lookup", stage: authLoginStageUserLookup},
		{name: "create", stage: authLoginStageUserCreate},
		{name: "point account", stage: authLoginStagePointAccountInit},
		{name: "registration grant", stage: authLoginStageRegistrationGrant},
		{name: "token session", stage: authLoginStageTokenSession},
		{name: "provider", stage: authLoginStageWechatPhoneExchange},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			log.SetOutput(&output)
			ctx := context.WithValue(context.Background(), requestIDContextKey, "req-stage-001")
			wrapped := &authLoginStageError{stage: testCase.stage, err: errors.New("sanitized test failure")}
			logAuthFlowFailure(ctx, authLoginStageUserLookup, wrapped, time.Now())
			if !strings.Contains(output.String(), "stage="+string(testCase.stage)) {
				t.Fatalf("log = %s", output.String())
			}
		})
	}
}

func TestAuthProviderFailureLogIncludesProviderFields(t *testing.T) {
	previousWriter := log.Writer()
	defer log.SetOutput(previousWriter)
	var output bytes.Buffer
	log.SetOutput(&output)

	ctx := context.WithValue(context.Background(), requestIDContextKey, "req-provider-001")
	logAuthFlowFailureWithDetails(ctx, authLoginStageWechatPhoneExchange, errors.New("provider rejected request"), time.Now(), "WECHAT_PHONE_AUTH_FAILED", "wechat", http.StatusBadGateway)
	line := output.String()
	for _, field := range []string{
		"provider=wechat",
		"provider_error_class=wechat_provider",
		"provider_http_status=502",
		"http_mapping=WECHAT_PHONE_AUTH_FAILED",
	} {
		if !strings.Contains(line, field) {
			t.Fatalf("provider diagnostic log missing %q: %s", field, line)
		}
	}
}

func TestMappedAuthFlowErrorKeepsExternalContract(t *testing.T) {
	response := httptest.NewRecorder()
	writeMappedAuthFlowError(response, errors.New("sqlstate=23505 constraint=secret_internal_detail"))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"code":"AUTH_INTERNAL_ERROR"`) || !strings.Contains(body, "登录服务暂时不可用") {
		t.Fatalf("external error contract changed: %s", body)
	}
	if strings.Contains(body, "secret_internal_detail") {
		t.Fatalf("root cause leaked to client: %s", body)
	}
}
