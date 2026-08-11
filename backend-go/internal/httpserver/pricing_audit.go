package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const pricingAuditRedactedValue = "[REDACTED]"
const pricingAuditMaxPage = 1000000

type pricingAuditMutation struct {
	ActorID          string
	ActorRole        string
	Action           string
	EntityType       string
	EntityID         string
	Method           string
	Path             string
	Status           int
	Result           string
	ErrorCode        string
	ChangeReason     string
	BeforeSnapshot   any
	AfterSnapshot    any
	RevisionBefore   *int64
	RevisionAfter    *int64
	PlanID           string
	PlanVersionID    string
	PricePlanID      string
	WeChatGoodID     string
	PaymentBindingID string
	WhitelistEntryID string
	Environment      string
	Metadata         map[string]any
}

func isPricingAuditAction(action string) bool {
	action = strings.TrimSpace(action)
	return strings.HasPrefix(action, "business_plan.version.") ||
		strings.HasPrefix(action, "price_plan.") ||
		strings.HasPrefix(action, "wechat_good.")
}

func pricingAuditMutationFromLegacy(actorID, actorRole, action, resource, resourceID, method, path string, status int, metadata map[string]any) pricingAuditMutation {
	mutation := pricingAuditMutation{
		ActorID: actorID, ActorRole: actorRole, Action: action, EntityType: resource, EntityID: resourceID,
		Method: method, Path: path, Status: status, Result: "SUCCEEDED", Metadata: metadata,
		ChangeReason:   pricingAuditMetadataString(metadata, "changeReason", "reason"),
		BeforeSnapshot: pricingAuditMetadataValue(metadata, "beforeSnapshot", "before"),
		AfterSnapshot:  pricingAuditMetadataValue(metadata, "afterSnapshot", "after"),
		RevisionBefore: pricingAuditMetadataRevision(metadata, "revisionBefore", "targetRevisionBefore", "oldDefaultRevisionBefore"),
		RevisionAfter:  pricingAuditMetadataRevision(metadata, "revisionAfter", "targetRevisionAfter", "revision"),
		PlanID:         pricingAuditMetadataString(metadata, "planId"), PlanVersionID: pricingAuditMetadataString(metadata, "planVersionId"),
		PricePlanID:      pricingAuditMetadataString(metadata, "pricePlanId", "newDefaultPricePlanId"),
		WeChatGoodID:     pricingAuditMetadataString(metadata, "wechatGoodId"),
		PaymentBindingID: pricingAuditMetadataString(metadata, "paymentBindingId", "bindingId"),
		WhitelistEntryID: pricingAuditMetadataString(metadata, "whitelistEntryId"),
		Environment:      pricingAuditMetadataString(metadata, "environment"),
	}
	if status >= http.StatusBadRequest {
		mutation.Result = "FAILED"
	}
	switch resource {
	case "plan_version":
		if mutation.PlanVersionID == "" {
			mutation.PlanVersionID = resourceID
		}
	case "price_plan":
		if mutation.PricePlanID == "" {
			mutation.PricePlanID = resourceID
		}
	case "wechat_virtual_good":
		if mutation.WeChatGoodID == "" {
			mutation.WeChatGoodID = resourceID
		}
	case "price_plan_payment_binding":
		if mutation.PaymentBindingID == "" {
			mutation.PaymentBindingID = resourceID
		}
	case "price_plan_test_whitelist":
		if mutation.WhitelistEntryID == "" {
			mutation.WhitelistEntryID = resourceID
		}
	}
	return mutation
}

func pricingAuditMetadataValue(metadata map[string]any, keys ...string) any {
	for _, key := range keys {
		if metadata != nil {
			if value, ok := metadata[key]; ok {
				return value
			}
		}
	}
	return nil
}

func pricingAuditMetadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if metadata == nil {
			continue
		}
		if value, ok := metadata[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func pricingAuditMetadataRevision(metadata map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		if metadata == nil {
			continue
		}
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		var revision int64
		switch typed := value.(type) {
		case int:
			revision = int64(typed)
		case int32:
			revision = int64(typed)
		case int64:
			revision = typed
		case float64:
			revision = int64(typed)
		case json.Number:
			parsed, err := typed.Int64()
			if err != nil {
				continue
			}
			revision = parsed
		default:
			continue
		}
		return &revision
	}
	return nil
}

func insertPricingAuditLog(ctx context.Context, tx *sql.Tx, mutation pricingAuditMutation) error {
	if tx == nil {
		return errors.New("pricing audit transaction is required")
	}
	mutation.ActorID = strings.TrimSpace(mutation.ActorID)
	mutation.ActorRole = strings.TrimSpace(mutation.ActorRole)
	mutation.Action = strings.TrimSpace(mutation.Action)
	mutation.EntityType = strings.TrimSpace(mutation.EntityType)
	mutation.EntityID = strings.TrimSpace(mutation.EntityID)
	mutation.Method = strings.ToUpper(strings.TrimSpace(mutation.Method))
	mutation.Path = stripPricingAuditURLSecrets(strings.TrimSpace(mutation.Path))
	mutation.Result = strings.ToUpper(strings.TrimSpace(mutation.Result))
	mutation.ErrorCode = strings.TrimSpace(mutation.ErrorCode)
	mutation.ChangeReason = sanitizePricingAuditText(strings.TrimSpace(mutation.ChangeReason))
	mutation.Environment = strings.ToUpper(strings.TrimSpace(mutation.Environment))
	if mutation.ActorID == "" || mutation.ActorRole == "" || mutation.Action == "" || mutation.EntityType == "" || mutation.EntityID == "" || mutation.ChangeReason == "" {
		return errors.New("pricing audit actor ID, actor role, action, entity, and change reason are required")
	}
	if mutation.Result == "" {
		mutation.Result = "SUCCEEDED"
	}
	if mutation.Result != "SUCCEEDED" && mutation.Result != "FAILED" {
		return errors.New("pricing audit result must be SUCCEEDED or FAILED")
	}
	if mutation.Status == 0 {
		mutation.Status = http.StatusOK
	}
	requestID := sanitizeRequestID(requestIDFromContext(ctx))
	if requestID == "" {
		requestID = newRequestID()
	}
	metadataRaw, err := marshalSanitizedPricingAuditValue(mutation.Metadata, true)
	if err != nil {
		return err
	}
	beforeRaw, err := marshalSanitizedPricingAuditValue(mutation.BeforeSnapshot, false)
	if err != nil {
		return err
	}
	afterRaw, err := marshalSanitizedPricingAuditValue(mutation.AfterSnapshot, false)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		insert into xz_audit_logs(
			id,actor_id,actor_role,action,resource,resource_id,method,path,status,metadata,
			request_id,domain,result,error_code,change_reason,before_snapshot,after_snapshot,
			revision_before,revision_after,plan_id,plan_version_id,price_plan_id,wechat_good_id,
			payment_binding_id,whitelist_entry_id,environment
		) values(
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,
			$11,$12,$13,nullif($14,''),$15,$16::jsonb,$17::jsonb,
			$18,$19,nullif($20,''),nullif($21,''),nullif($22,''),nullif($23,''),
			nullif($24,''),nullif($25,''),nullif($26,'')
		)
	`, newAuditID(), mutation.ActorID, mutation.ActorRole, mutation.Action, mutation.EntityType, mutation.EntityID,
		mutation.Method, mutation.Path, mutation.Status, metadataRaw, requestID, pricingAuditDomain(mutation.Action),
		mutation.Result, mutation.ErrorCode, mutation.ChangeReason, beforeRaw, afterRaw,
		pricingAuditRevisionValue(mutation.RevisionBefore), pricingAuditRevisionValue(mutation.RevisionAfter),
		strings.TrimSpace(mutation.PlanID), strings.TrimSpace(mutation.PlanVersionID), strings.TrimSpace(mutation.PricePlanID),
		strings.TrimSpace(mutation.WeChatGoodID), strings.TrimSpace(mutation.PaymentBindingID), strings.TrimSpace(mutation.WhitelistEntryID),
		mutation.Environment)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New("pricing audit was not written")
	}
	return nil
}

func pricingAuditDomain(action string) string {
	switch {
	case strings.HasPrefix(action, "business_plan.version."):
		return "PRICING_ENTITLEMENT"
	case strings.HasPrefix(action, "price_plan.test_whitelist."):
		return "PRICING_TEST_WHITELIST"
	case strings.HasPrefix(action, "price_plan.payment_binding."):
		return "PRICING_PAYMENT_BINDING"
	case strings.HasPrefix(action, "wechat_good."):
		return "PRICING_WECHAT_GOOD"
	case strings.HasPrefix(action, "price_plan."):
		return "PRICING_PRICE_PLAN"
	default:
		return "PRICING"
	}
}

func pricingAuditRevisionValue(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func marshalSanitizedPricingAuditValue(value any, emptyObject bool) (any, error) {
	if value == nil {
		if emptyObject {
			return []byte("{}"), nil
		}
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return json.Marshal(sanitizePricingAuditValue(normalized))
}

type pricingAuditQuery struct {
	PlanID           string
	PlanVersionID    string
	PricePlanID      string
	WeChatGoodID     string
	PaymentBindingID string
	WhitelistEntryID string
	Action           string
	OperatorID       string
	OperatorRole     string
	StartTime        *time.Time
	EndTime          *time.Time
	Result           string
	Page             int
	PageSize         int
}

func parsePricingAuditQuery(values url.Values) (pricingAuditQuery, error) {
	allowed := map[string]bool{
		"planId": true, "planVersionId": true, "pricePlanId": true, "wechatGoodId": true,
		"bindingId": true, "whitelistEntryId": true, "action": true, "operatorId": true,
		"operatorRole": true, "startTime": true, "endTime": true, "result": true,
		"page": true, "pageSize": true,
	}
	for key, items := range values {
		if !allowed[key] || len(items) != 1 {
			return pricingAuditQuery{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_FILTER_INVALID", "pricing audit query contains an unknown or repeated filter")
		}
	}
	query := pricingAuditQuery{
		PlanID: strings.TrimSpace(values.Get("planId")), PlanVersionID: strings.TrimSpace(values.Get("planVersionId")),
		PricePlanID: strings.TrimSpace(values.Get("pricePlanId")), WeChatGoodID: strings.TrimSpace(values.Get("wechatGoodId")),
		PaymentBindingID: strings.TrimSpace(values.Get("bindingId")), WhitelistEntryID: strings.TrimSpace(values.Get("whitelistEntryId")),
		Action: strings.TrimSpace(values.Get("action")), OperatorID: strings.TrimSpace(values.Get("operatorId")),
		OperatorRole: strings.TrimSpace(values.Get("operatorRole")), Result: strings.ToUpper(strings.TrimSpace(values.Get("result"))),
		Page: 1, PageSize: 50,
	}
	for _, value := range []string{
		query.PlanID, query.PlanVersionID, query.PricePlanID, query.WeChatGoodID, query.PaymentBindingID,
		query.WhitelistEntryID, query.Action, query.OperatorID, query.OperatorRole,
	} {
		if len(value) > 256 {
			return pricingAuditQuery{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_FILTER_INVALID", "pricing audit filter exceeds the maximum length")
		}
	}
	if raw := strings.TrimSpace(values.Get("page")); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 || page > pricingAuditMaxPage {
			return pricingAuditQuery{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_PAGE_INVALID", "pricing audit page must be a positive integer")
		}
		query.Page = page
	}
	if raw := strings.TrimSpace(values.Get("pageSize")); raw != "" {
		pageSize, err := strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 200 {
			return pricingAuditQuery{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_PAGE_SIZE_INVALID", "pricing audit pageSize must be between 1 and 200")
		}
		query.PageSize = pageSize
	}
	if query.Result != "" && query.Result != "SUCCEEDED" && query.Result != "FAILED" {
		return pricingAuditQuery{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_RESULT_INVALID", "pricing audit result must be SUCCEEDED or FAILED")
	}
	parseTime := func(name string) (*time.Time, error) {
		raw := strings.TrimSpace(values.Get(name))
		if raw == "" {
			return nil, nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_TIME_INVALID", "pricing audit startTime and endTime must use RFC3339")
		}
		parsed = parsed.UTC()
		return &parsed, nil
	}
	var err error
	if query.StartTime, err = parseTime("startTime"); err != nil {
		return pricingAuditQuery{}, err
	}
	if query.EndTime, err = parseTime("endTime"); err != nil {
		return pricingAuditQuery{}, err
	}
	if query.StartTime != nil && query.EndTime != nil && query.StartTime.After(*query.EndTime) {
		return pricingAuditQuery{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_TIME_RANGE_INVALID", "pricing audit endTime must not be before startTime")
	}
	return query, nil
}

func sanitizePricingAuditValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitivePricingAuditKey(key) {
				result[key] = pricingAuditRedactedValue
				continue
			}
			result[key] = sanitizePricingAuditValue(item)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = sanitizePricingAuditValue(item)
		}
		return result
	case string:
		return sanitizePricingAuditText(typed)
	default:
		return value
	}
}

func isSensitivePricingAuditKey(key string) bool {
	var normalized strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(key)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	value := normalized.String()
	if value == "token" || value == "authtoken" || value == "bearertoken" || value == "refreshtoken" || value == "dsn" ||
		value == "appkey" || value == "secret" || value == "secretkey" || value == "privatekey" ||
		value == "verificationtoken" || value == "encryptkey" {
		return true
	}
	for _, marker := range []string{
		"appsecret", "clientsecret", "sessionkey", "accesstoken", "authorization", "cookie", "password",
		"credential", "databaseurl", "connectionstring",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func stripPricingAuditURLSecrets(value string) string {
	return sanitizePricingAuditText(value)
}

func sanitizePricingAuditText(value string) string {
	trimmed := strings.TrimSpace(value)
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Host != "" {
		switch strings.ToLower(parsed.Scheme) {
		case "postgres", "postgresql", "mysql", "mariadb", "mongodb", "mongodb+srv", "redis", "rediss", "sqlserver":
			return pricingAuditRedactedValue
		case "http", "https":
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.ForceQuery = false
			parsed.Fragment = ""
			return parsed.String()
		}
	}
	lower := strings.ToLower(trimmed)
	for _, scheme := range []string{"postgres://", "postgresql://", "mysql://", "mariadb://", "mongodb://", "mongodb+srv://", "redis://", "rediss://", "sqlserver://"} {
		if strings.Contains(lower, scheme) {
			return pricingAuditRedactedValue
		}
	}
	var normalized strings.Builder
	for _, character := range lower {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	compact := normalized.String()
	for _, marker := range []string{
		"appsecret", "clientsecret", "sessionkey", "accesstoken", "authorization", "password",
		"privatekey", "verificationtoken", "encryptkey", "databaseurl", "connectionstring",
	} {
		if strings.Contains(compact, marker) {
			return pricingAuditRedactedValue
		}
	}
	return value
}
