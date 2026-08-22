package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type authLoginStage string

const (
	authLoginStageWechatCode2Session  authLoginStage = "wechat_code2session"
	authLoginStageWechatPhoneExchange authLoginStage = "wechat_phone_exchange"
	authLoginStagePhoneNormalize      authLoginStage = "phone_normalize"
	authLoginStageUserLookup          authLoginStage = "user_lookup"
	authLoginStageUserCreate          authLoginStage = "user_create"
	authLoginStagePointAccountInit    authLoginStage = "point_account_init"
	authLoginStageRegistrationGrant   authLoginStage = "registration_grant"
	authLoginStageIdentityBind        authLoginStage = "identity_bind"
	authLoginStageTokenSession        authLoginStage = "token_session"
	authLoginStageResponseBuild       authLoginStage = "response_build"
)

type authLoginStageError struct {
	stage authLoginStage
	err   error
}

func (e *authLoginStageError) Error() string { return e.err.Error() }
func (e *authLoginStageError) Unwrap() error { return e.err }

func authLoginStageForError(err error, fallback authLoginStage) authLoginStage {
	var stageErr *authLoginStageError
	if errors.As(err, &stageErr) && stageErr.stage != "" {
		return stageErr.stage
	}
	return fallback
}

func authErrorClass(stage authLoginStage, err error) string {
	if err == nil {
		return "none"
	}
	if stage == authLoginStageWechatCode2Session || stage == authLoginStageWechatPhoneExchange {
		return "wechat_provider"
	}
	if stage == authLoginStageTokenSession || errors.Is(err, errAuthSessionUnavailable) {
		return "session"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) || errors.Is(err, sql.ErrNoRows) || strings.Contains(strings.ToLower(err.Error()), "sqlstate") {
		return "database"
	}
	var flowErr *authFlowError
	if errors.As(err, &flowErr) {
		return "auth_flow"
	}
	return "internal"
}

func authHTTPMapping(err error) string {
	var flowErr *authFlowError
	if errors.As(err, &flowErr) && strings.TrimSpace(flowErr.code) != "" {
		return flowErr.code
	}
	if errors.Is(err, errAuthSessionUnavailable) {
		return "AUTH_SESSION_UNAVAILABLE"
	}
	return "AUTH_INTERNAL_ERROR"
}

func authDatabaseFields(err error) (string, string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code, pgErr.ConstraintName
	}
	return "", ""
}

func logAuthFlowFailure(ctx context.Context, stage authLoginStage, err error, started time.Time) {
	logAuthFlowFailureWithDetails(ctx, stage, err, started, "", "", 0)
}

func logAuthFlowFailureWithDetails(ctx context.Context, stage authLoginStage, err error, started time.Time, mapping, provider string, providerStatus int) {
	if err == nil {
		return
	}
	stage = authLoginStageForError(err, stage)
	requestID := requestIDFromContext(ctx)
	if requestID == "" {
		requestID = "unknown"
	}
	sqlState, constraint := authDatabaseFields(err)
	if strings.TrimSpace(mapping) == "" {
		mapping = authHTTPMapping(err)
	}
	fields := fmt.Sprintf(
		"auth_flow=wechat_phone_login request_id=%s stage=%s error_class=%s http_mapping=%s duration_ms=%d",
		requestID, stage, authErrorClass(stage, err), mapping, time.Since(started).Milliseconds(),
	)
	if sqlState != "" {
		fields += " sqlstate=" + sqlState
	}
	if constraint != "" {
		fields += " constraint_name=" + constraint
	}
	if provider != "" {
		fields += " provider=" + provider + " provider_error_class=" + authErrorClass(stage, err)
		if providerStatus > 0 {
			fields += fmt.Sprintf(" provider_http_status=%d", providerStatus)
		}
	}
	if stage == authLoginStageTokenSession {
		fields += " component=auth_session operation=put"
	}
	log.Printf("auth_flow_failure %s", fields)
}
