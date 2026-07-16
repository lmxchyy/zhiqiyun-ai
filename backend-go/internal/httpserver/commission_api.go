package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type commissionRuleMutation struct {
	RuleName          string          `json:"ruleName"`
	RuleCode          string          `json:"ruleCode"`
	TenantID          string          `json:"tenantId"`
	ProductType       string          `json:"productType"`
	ProductID         string          `json:"productId"`
	BeneficiaryRole   string          `json:"beneficiaryRole"`
	RelationshipLevel int             `json:"relationshipLevel"`
	CalculationType   string          `json:"calculationType"`
	FixedAmountCents  int64           `json:"fixedAmountCents"`
	PercentageBPS     int64           `json:"percentageBps"`
	CalculationConfig json.RawMessage `json:"calculationConfig"`
	Priority          int             `json:"priority"`
	FreezeDays        int             `json:"freezeDays"`
	RefundPolicy      string          `json:"refundPolicy"`
	EffectiveStartAt  string          `json:"effectiveStartAt"`
	EffectiveEndAt    string          `json:"effectiveEndAt"`
	Status            string          `json:"status"`
}

func (a adminAPI) commissionRulesV2(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commissionRuleRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("commission rule storage requires PostgreSQL"))
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	page := positiveQueryInt(r, "page", 1)
	pageSize := positiveQueryInt(r, "pageSize", 50)
	if pageSize > 200 {
		pageSize = 200
	}
	items, total, err := repository.ListCommissionRules(r.Context(), commissionRuleQuery{
		TenantID: tenantID, ProductType: r.URL.Query().Get("productType"), ProductID: r.URL.Query().Get("productId"),
		Status: r.URL.Query().Get("status"), Limit: pageSize, Offset: (page - 1) * pageSize,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	response := make([]map[string]any, 0, len(items))
	for _, item := range items {
		response = append(response, commissionRuleView(item))
	}
	writeJSON(w, map[string]any{"items": response, "total": total, "page": page, "pageSize": pageSize})
}

func (a adminAPI) createCommissionRuleV2(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commissionRuleRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("commission rule storage requires PostgreSQL"))
		return
	}
	var payload commissionRuleMutation
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	_, actorRole := actorFromRequest(r)
	if actorRole == "SUPER_ADMIN" && strings.TrimSpace(payload.TenantID) != "" {
		tenantID = strings.TrimSpace(payload.TenantID)
	}
	if payload.TenantID != "" && payload.TenantID != tenantID {
		writeError(w, http.StatusForbidden, errForbidden)
		return
	}
	rule, err := commissionRuleFromMutation(payload, tenantID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	created, err := repository.CreateCommissionRule(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"item": commissionRuleView(created)})
}

func (a adminAPI) updateCommissionRuleV2(w http.ResponseWriter, r *http.Request) {
	repository, ok := a.store.(commissionRuleRepository)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, errors.New("commission rule storage requires PostgreSQL"))
		return
	}
	var payload commissionRuleMutation
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tenantID, err := a.commissionTenantForRequest(r)
	if err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}
	rule, err := commissionRuleFromMutation(payload, tenantID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	created, err := repository.VersionCommissionRule(r.Context(), tenantID, r.PathValue("id"), rule)
	if errors.Is(err, errCommissionRuleNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"item": commissionRuleView(created), "versioned": true})
}

func (a adminAPI) commissionTenantForRequest(r *http.Request) (string, error) {
	requested := firstNonEmptyString(strings.TrimSpace(r.URL.Query().Get("tenantId")), strings.TrimSpace(r.Header.Get("X-Tenant-Id")))
	actorID, actorRole := actorFromRequest(r)
	if actorRole == "SUPER_ADMIN" {
		return firstNonEmptyString(requested, "tenant_default"), nil
	}
	accessStore, ok := a.store.(userRoleAccessStore)
	if !ok || actorID == "" {
		return "", errForbidden
	}
	access, found, err := accessStore.GetUserRoleAccess(actorID)
	if err != nil {
		return "", err
	}
	if !found || access.TenantID == "" || (requested != "" && requested != access.TenantID) {
		return "", errForbidden
	}
	return access.TenantID, nil
}

func commissionRuleFromMutation(payload commissionRuleMutation, tenantID string) (commissionapp.CommissionRule, error) {
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(payload.EffectiveStartAt))
	if err != nil {
		return commissionapp.CommissionRule{}, errors.New("effectiveStartAt must use RFC3339")
	}
	var end *time.Time
	if value := strings.TrimSpace(payload.EffectiveEndAt); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return commissionapp.CommissionRule{}, errors.New("effectiveEndAt must use RFC3339")
		}
		end = &parsed
	}
	rule := commissionapp.CommissionRule{
		TenantID: tenantID, Code: payload.RuleCode, Name: payload.RuleName,
		ProductType: payload.ProductType, ProductID: strings.TrimSpace(payload.ProductID),
		BeneficiaryRole:   commissionapp.BeneficiaryType(strings.ToUpper(strings.TrimSpace(payload.BeneficiaryRole))),
		RelationshipLevel: payload.RelationshipLevel,
		CalculationType:   commissionapp.CalculationType(strings.ToUpper(strings.TrimSpace(payload.CalculationType))),
		FixedAmountCents:  commissionapp.AmountCents(payload.FixedAmountCents),
		PercentageBPS:     commissionapp.PercentageBPS(payload.PercentageBPS), CalculationConfig: payload.CalculationConfig,
		Priority: payload.Priority, FreezeDays: payload.FreezeDays,
		RefundPolicy:     firstNonEmptyString(strings.ToUpper(strings.TrimSpace(payload.RefundPolicy)), "REVERSE_OR_RECOVER"),
		EffectiveStartAt: start.UTC(), EffectiveEndAt: end, Status: payload.Status,
	}
	return rule, nil
}

func commissionRuleView(rule commissionapp.CommissionRule) map[string]any {
	config := map[string]any{}
	if len(rule.CalculationConfig) > 0 {
		_ = json.Unmarshal(rule.CalculationConfig, &config)
	}
	return map[string]any{
		"id": rule.ID, "tenantId": rule.TenantID, "ruleCode": rule.Code, "ruleName": rule.Name,
		"productType": rule.ProductType, "productId": rule.ProductID,
		"beneficiaryRole": rule.BeneficiaryRole, "relationshipLevel": rule.RelationshipLevel,
		"calculationType": rule.CalculationType, "fixedAmountCents": rule.FixedAmountCents,
		"percentageBps": rule.PercentageBPS, "calculationConfig": config,
		"priority": rule.Priority, "freezeDays": rule.FreezeDays, "refundPolicy": rule.RefundPolicy,
		"effectiveStartAt": rule.EffectiveStartAt, "effectiveEndAt": rule.EffectiveEndAt,
		"version": rule.Version, "status": rule.Status, "createdAt": rule.CreatedAt, "updatedAt": rule.UpdatedAt,
	}
}
