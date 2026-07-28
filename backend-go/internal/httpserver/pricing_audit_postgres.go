package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type pricingAuditLogView struct {
	ID               string         `json:"auditLogId"`
	OperatorID       string         `json:"operatorId"`
	OperatorRole     string         `json:"operatorRole"`
	OperationTime    time.Time      `json:"operationTime"`
	Action           string         `json:"action"`
	EntityType       string         `json:"entityType"`
	EntityID         string         `json:"entityId"`
	ChangeReason     string         `json:"changeReason"`
	BeforeSnapshot   any            `json:"beforeSnapshot"`
	AfterSnapshot    any            `json:"afterSnapshot"`
	RevisionBefore   *int64         `json:"revisionBefore"`
	RevisionAfter    *int64         `json:"revisionAfter"`
	RequestID        string         `json:"requestId"`
	Result           string         `json:"result"`
	ErrorCode        string         `json:"errorCode,omitempty"`
	PlanID           string         `json:"planId,omitempty"`
	PlanVersionID    string         `json:"planVersionId,omitempty"`
	PricePlanID      string         `json:"pricePlanId,omitempty"`
	WeChatGoodID     string         `json:"wechatGoodId,omitempty"`
	PaymentBindingID string         `json:"bindingId,omitempty"`
	WhitelistEntryID string         `json:"whitelistEntryId,omitempty"`
	Environment      string         `json:"environment,omitempty"`
	Metadata         map[string]any `json:"metadata"`
}

type pricingAuditPage struct {
	Items    []pricingAuditLogView `json:"items"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"pageSize"`
}

func (s *postgresStore) listPricingAuditLogs(ctx context.Context, query pricingAuditQuery) (pricingAuditPage, error) {
	if s == nil || s.db == nil {
		return pricingAuditPage{}, newBusinessPlanAdminError(http.StatusServiceUnavailable, "PRICING_AUDIT_STORE_UNAVAILABLE", "pricing audit PostgreSQL store is unavailable")
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.Page < 1 || query.Page > pricingAuditMaxPage {
		return pricingAuditPage{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_PAGE_INVALID", "pricing audit page must be within the supported range")
	}
	if query.PageSize < 1 || query.PageSize > 200 {
		return pricingAuditPage{}, newBusinessPlanAdminError(http.StatusBadRequest, "PRICING_AUDIT_PAGE_SIZE_INVALID", "pricing audit pageSize must be between 1 and 200")
	}
	conditions := []string{"domain like 'PRICING%'"}
	args := []any{}
	add := func(column string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s=$%d", column, len(args)))
	}
	for _, filter := range []struct {
		column string
		value  string
	}{
		{"plan_id", query.PlanID}, {"plan_version_id", query.PlanVersionID}, {"price_plan_id", query.PricePlanID},
		{"wechat_good_id", query.WeChatGoodID}, {"payment_binding_id", query.PaymentBindingID},
		{"whitelist_entry_id", query.WhitelistEntryID}, {"action", query.Action}, {"actor_id", query.OperatorID},
		{"actor_role", query.OperatorRole}, {"result", query.Result},
	} {
		if value := strings.TrimSpace(filter.value); value != "" {
			add(filter.column, value)
		}
	}
	if query.StartTime != nil {
		args = append(args, query.StartTime.UTC())
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if query.EndTime != nil {
		args = append(args, query.EndTime.UTC())
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	where := strings.Join(conditions, " and ")
	var total int64
	if err := s.db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where `+where, args...).Scan(&total); err != nil {
		return pricingAuditPage{}, err
	}
	offset := (int64(query.Page) - 1) * int64(query.PageSize)
	queryArgs := append(append([]any{}, args...), query.PageSize, offset)
	rows, err := s.db.QueryContext(ctx, `
		select id,coalesce(actor_id,''),coalesce(actor_role,''),created_at,action,resource,coalesce(resource_id,''),
		       coalesce(change_reason,''),before_snapshot::text,after_snapshot::text,revision_before,revision_after,
		       coalesce(request_id,''),coalesce(result,''),coalesce(error_code,''),coalesce(plan_id,''),
		       coalesce(plan_version_id,''),coalesce(price_plan_id,''),coalesce(wechat_good_id,''),
		       coalesce(payment_binding_id,''),coalesce(whitelist_entry_id,''),coalesce(environment,''),metadata::text
		from xz_audit_logs
		where `+where+`
		order by created_at desc,id desc
		limit $`+fmt.Sprint(len(args)+1)+` offset $`+fmt.Sprint(len(args)+2), queryArgs...)
	if err != nil {
		return pricingAuditPage{}, err
	}
	defer rows.Close()
	items := make([]pricingAuditLogView, 0, query.PageSize)
	for rows.Next() {
		item, err := scanPricingAuditLog(rows)
		if err != nil {
			return pricingAuditPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return pricingAuditPage{}, err
	}
	return pricingAuditPage{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

type pricingAuditScanner interface {
	Scan(...any) error
}

func scanPricingAuditLog(scanner pricingAuditScanner) (pricingAuditLogView, error) {
	var item pricingAuditLogView
	var beforeRaw, afterRaw sql.NullString
	var beforeRevision, afterRevision sql.NullInt64
	var metadataRaw string
	err := scanner.Scan(
		&item.ID, &item.OperatorID, &item.OperatorRole, &item.OperationTime, &item.Action, &item.EntityType, &item.EntityID,
		&item.ChangeReason, &beforeRaw, &afterRaw, &beforeRevision, &afterRevision, &item.RequestID, &item.Result,
		&item.ErrorCode, &item.PlanID, &item.PlanVersionID, &item.PricePlanID, &item.WeChatGoodID, &item.PaymentBindingID,
		&item.WhitelistEntryID, &item.Environment, &metadataRaw,
	)
	if err != nil {
		return pricingAuditLogView{}, err
	}
	item.ChangeReason = sanitizePricingAuditText(item.ChangeReason)
	if beforeRevision.Valid {
		value := beforeRevision.Int64
		item.RevisionBefore = &value
	}
	if afterRevision.Valid {
		value := afterRevision.Int64
		item.RevisionAfter = &value
	}
	if beforeRaw.Valid {
		item.BeforeSnapshot, err = decodeSanitizedPricingAuditJSON(beforeRaw.String)
		if err != nil {
			return pricingAuditLogView{}, err
		}
	}
	if afterRaw.Valid {
		item.AfterSnapshot, err = decodeSanitizedPricingAuditJSON(afterRaw.String)
		if err != nil {
			return pricingAuditLogView{}, err
		}
	}
	metadata, err := decodeSanitizedPricingAuditJSON(metadataRaw)
	if err != nil {
		return pricingAuditLogView{}, err
	}
	if metadataMap, ok := metadata.(map[string]any); ok {
		item.Metadata = metadataMap
	} else {
		item.Metadata = map[string]any{}
	}
	return item, nil
}

func decodeSanitizedPricingAuditJSON(raw string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return sanitizePricingAuditValue(value), nil
}
