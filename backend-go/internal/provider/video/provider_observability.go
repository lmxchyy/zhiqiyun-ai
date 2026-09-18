package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	ProviderUnsupportedParameter = "PROVIDER_UNSUPPORTED_PARAMETER"
	ProviderInvalidModel         = "PROVIDER_INVALID_MODEL"
	ProviderAuthError            = "PROVIDER_AUTH_ERROR"
	ProviderRateLimit            = "PROVIDER_RATE_LIMIT"
	ProviderQuotaExceeded        = "PROVIDER_QUOTA_EXCEEDED"
	ProviderTimeout              = "PROVIDER_TIMEOUT"
	Provider4xx                  = "PROVIDER_4XX"
	Provider5xx                  = "PROVIDER_5XX"
	ProviderNetworkError         = "PROVIDER_NETWORK_ERROR"
	ProviderUnknownError         = "PROVIDER_UNKNOWN_ERROR"
)

const providerObservabilityMaxMessage = 512

// ProviderError carries the safe, structured facts needed to classify a
// provider failure without exposing the request body or authorization data.
type ProviderError struct {
	Provider      string
	HTTPStatus    int
	ProviderCode  string
	ProviderJobID string
	ProviderReqID string
	Message       string
	FailureClass  string
	Cause         error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "video provider error"
	}
	message := e.Message
	if message == "" && e.Cause != nil {
		message = sanitizeProviderMessage(e.Cause.Error())
	}
	if e.HTTPStatus > 0 {
		return fmt.Sprintf("video provider %s returned HTTP %d: %s", e.Provider, e.HTTPStatus, message)
	}
	if message != "" {
		return fmt.Sprintf("video provider %s: %s", e.Provider, message)
	}
	return fmt.Sprintf("video provider %s failed", e.Provider)
}

func (e *ProviderError) Unwrap() error { return e.Cause }

// FailureClassification is consumed by the generation execution layer to
// persist a stable provider-facing error code without coupling that package to
// this provider implementation.
func (e *ProviderError) FailureClassification() string {
	if e == nil || e.FailureClass == "" {
		return ProviderUnknownError
	}
	return e.FailureClass
}

func (e *ProviderError) ProviderErrorCode() string {
	if e == nil {
		return ""
	}
	return e.ProviderCode
}

func (e *ProviderError) ProviderRequestID() string {
	if e == nil {
		return ""
	}
	return e.ProviderReqID
}

func newProviderError(provider string, status int, providerCode, message, requestID, jobID string, cause error) *ProviderError {
	safeMessage := sanitizeProviderMessage(message)
	if safeMessage == "" && cause != nil {
		safeMessage = sanitizeProviderMessage(cause.Error())
	}
	return &ProviderError{
		Provider:      strings.TrimSpace(provider),
		HTTPStatus:    status,
		ProviderCode:  sanitizeProviderValue(providerCode, 96),
		ProviderJobID: sanitizeProviderValue(jobID, 128),
		ProviderReqID: sanitizeProviderValue(requestID, 128),
		Message:       safeMessage,
		FailureClass:  classifyProviderFailure(status, providerCode, safeMessage, cause),
		Cause:         cause,
	}
}

func classifyProviderFailure(status int, providerCode, message string, cause error) string {
	lower := strings.ToLower(strings.Join([]string{providerCode, message}, " "))
	switch {
	case strings.Contains(lower, "duration") && (strings.Contains(lower, "unsupported") || strings.Contains(lower, "invalid") || strings.Contains(lower, "must be") || strings.Contains(lower, "maximum") || strings.Contains(lower, "max")):
		return ProviderUnsupportedParameter
	case strings.Contains(lower, "unsupported parameter"), strings.Contains(lower, "parameter not supported"):
		return ProviderUnsupportedParameter
	case strings.Contains(lower, "model not found"), strings.Contains(lower, "invalid model"), strings.Contains(lower, "unknown model"), strings.Contains(lower, "model does not exist"), strings.Contains(lower, "unsupported model"):
		return ProviderInvalidModel
	case status == 401 || status == 403, strings.Contains(lower, "unauthorized"), strings.Contains(lower, "forbidden"), strings.Contains(lower, "invalid api key"), strings.Contains(lower, "invalid token"):
		return ProviderAuthError
	case status == 429, strings.Contains(lower, "rate limit"), strings.Contains(lower, "too many requests"):
		return ProviderRateLimit
	case status == 402, strings.Contains(lower, "quota"), strings.Contains(lower, "insufficient credit"), strings.Contains(lower, "insufficient balance"):
		return ProviderQuotaExceeded
	case status == 408 || status == 504, errors.Is(cause, context.DeadlineExceeded), isTimeoutError(cause):
		return ProviderTimeout
	case status >= 500 && status <= 599:
		return Provider5xx
	case status >= 400 && status <= 499:
		return Provider4xx
	case cause != nil:
		return ProviderNetworkError
	default:
		return ProviderUnknownError
	}
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func providerRequestIDFromHeaders(headers map[string][]string) string {
	for _, key := range []string{"X-Request-ID", "X-Request-Id", "Request-ID", "Request-Id"} {
		for _, value := range headers[key] {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return sanitizeProviderValue(trimmed, 128)
			}
		}
	}
	return ""
}

func providerResponseFields(raw []byte) (code, message, jobID string) {
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) == nil {
		code = firstNestedString(decoded, "code", "error_code", "errorCode", "type")
		message = firstNestedString(decoded, "message", "error", "detail", "reason", "fail_reason", "errorMessage")
		jobID = firstNestedString(decoded, "id", "taskId", "task_id", "providerTaskId", "provider_request_id")
	}
	if message == "" {
		message = strings.TrimSpace(string(raw))
	}
	return sanitizeProviderValue(code, 96), sanitizeProviderMessage(message), sanitizeProviderValue(jobID, 128)
}

func firstNestedString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			switch typed := raw.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			case map[string]any:
				if nested := firstNestedString(typed, keys...); nested != "" {
					return nested
				}
			default:
				if text := strings.TrimSpace(fmt.Sprint(typed)); text != "" && text != "<nil>" {
					return text
				}
			}
		}
	}
	for _, raw := range value {
		if nested, ok := raw.(map[string]any); ok {
			if result := firstNestedString(nested, keys...); result != "" {
				return result
			}
		}
	}
	return ""
}

var (
	providerURLPattern    = regexp.MustCompile(`https?://[^\s"']+`)
	providerBearerPattern = regexp.MustCompile(`(?i)\bbearer\s+[^,;\s]+`)
	providerSecretPattern = regexp.MustCompile(`(?i)(authorization|api[-_ ]?key|access[-_ ]?token|secret)[=: ]+[^,;\s]+`)
	providerPromptPattern = regexp.MustCompile(`(?i)(["']?prompt["']?\s*[:=]\s*)("[^"]*"|'[^']*'|[^,;}]+)`)
)

func sanitizeProviderMessage(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	text = providerURLPattern.ReplaceAllStringFunc(text, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "[url]"
		}
		return parsed.Scheme + "://" + parsed.Host + parsed.Path
	})
	text = providerBearerPattern.ReplaceAllString(text, "Bearer=[redacted]")
	text = providerSecretPattern.ReplaceAllString(text, "$1=[redacted]")
	text = providerPromptPattern.ReplaceAllString(text, "$1[redacted]")
	text = strings.Join(strings.Fields(text), " ")
	return sanitizeProviderValue(text, providerObservabilityMaxMessage)
}

func sanitizeProviderValue(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "..."
}

func logVideoProviderRequest(req *http.Request, provider, model, generationMode string, params map[string]any) {
	if req == nil || req.URL == nil {
		return
	}
	log.Printf("event=video_provider_request task_id=%s provider=%s provider_model=%s duration=%d resolution=%s aspect_ratio=%s generation_mode=%s request_method=%s request_host=%s request_path=%s attempt=%s",
		sanitizeProviderValue(stringParam(params, "_provider_execution_task_id"), 128),
		sanitizeProviderValue(provider, 96),
		sanitizeProviderValue(model, 128),
		videoSeconds(params),
		sanitizeProviderValue(videoResolution(params), 32),
		sanitizeProviderValue(videoAspectRatio(params), 32),
		sanitizeProviderValue(generationMode, 32),
		sanitizeProviderValue(req.Method, 16),
		sanitizeProviderValue(req.URL.Host, 160),
		sanitizeProviderValue(req.URL.Path, 240),
		sanitizeProviderValue(providerAttempt(params), 32),
	)
}

func logVideoProviderPollRequest(req *http.Request, provider, model, jobID string) {
	if req == nil || req.URL == nil {
		return
	}
	log.Printf("event=video_provider_request task_id= provider=%s provider_model=%s provider_job_id=%s request_method=%s request_host=%s request_path=%s attempt=poll",
		sanitizeProviderValue(provider, 96),
		sanitizeProviderValue(model, 128),
		sanitizeProviderValue(jobID, 128),
		sanitizeProviderValue(req.Method, 16),
		sanitizeProviderValue(req.URL.Host, 160),
		sanitizeProviderValue(req.URL.Path, 240),
	)
}

func logVideoProviderPollResponse(req *http.Request, provider, model, jobID string, status int, requestID, providerCode, message, result, failureClass string, elapsedMS int64) {
	log.Printf("event=video_provider_response task_id= provider=%s provider_model=%s provider_job_id=%s http_status=%d provider_request_id=%s duration_ms=%d result=%s provider_error_code=%s provider_error_message=%s failure_class=%s",
		sanitizeProviderValue(provider, 96),
		sanitizeProviderValue(model, 128),
		sanitizeProviderValue(jobID, 128),
		status,
		sanitizeProviderValue(requestID, 128),
		elapsedMS,
		sanitizeProviderValue(result, 24),
		sanitizeProviderValue(providerCode, 96),
		sanitizeProviderMessage(message),
		sanitizeProviderValue(failureClass, 64),
	)
	_ = req
}

func logVideoProviderResponse(req *http.Request, provider, model string, params map[string]any, status int, requestID, jobID, providerCode, message, result string, elapsedMS int64, failureClass string) {
	log.Printf("event=video_provider_response task_id=%s provider=%s provider_model=%s http_status=%d provider_request_id=%s provider_job_id=%s duration_ms=%d result=%s provider_error_code=%s provider_error_message=%s failure_class=%s",
		sanitizeProviderValue(stringParam(params, "_provider_execution_task_id"), 128),
		sanitizeProviderValue(provider, 96),
		sanitizeProviderValue(model, 128),
		status,
		sanitizeProviderValue(requestID, 128),
		sanitizeProviderValue(jobID, 128),
		elapsedMS,
		sanitizeProviderValue(result, 24),
		sanitizeProviderValue(providerCode, 96),
		sanitizeProviderMessage(message),
		sanitizeProviderValue(failureClass, 64),
	)
	_ = req
}

func providerAttempt(params map[string]any) string {
	for _, key := range []string{"attempt", "retryAttempt", "provider_attempt"} {
		if value := stringParam(params, key); value != "" {
			return value
		}
	}
	return "1"
}

func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
