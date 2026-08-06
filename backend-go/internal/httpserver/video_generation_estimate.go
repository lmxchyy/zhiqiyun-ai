package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
)

func (a api) estimateGenerationTaskCost(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	var req generation.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.UserID = user.ID
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, generation.ErrInvalidPrompt)
		return
	}
	if req.Type == "" {
		req.Type = "TEXT_TO_IMAGE"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	req.Params["terminal"] = requestTerminal(r)

	req, err = a.prepareGenerationRequest(data, user, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Params["terminal"] = requestTerminal(r)
	if err := enforceMiniProgramModelCompliance(data, &req); err != nil {
		writeError(w, http.StatusForbidden, err)
		return
	}

	rule := billingRuleForRequest(req, data)
	writeJSON(w, map[string]any{
		"pointCost":   generationPointCostForRequest(req, data),
		"moduleCode":  requestModuleCode(req),
		"type":        req.Type,
		"model":       req.Model,
		"billingType": rule.BillingType,
		"terminal":    stringValue(req.Params["terminal"]),
	})
}
