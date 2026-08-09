package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type videoGenerationEstimate struct {
	Model           string  `json:"model"`
	EstimatedPoints int     `json:"estimatedPoints"`
	BillingType     string  `json:"billingType"`
	QuantityField   string  `json:"quantityField"`
	Quantity        float64 `json:"quantity"`
	Note            string  `json:"note"`
}

func (a api) prepareVideoGenerationEstimate(
	data adminPlatformData,
	user adminUser,
	req generation.CreateRequest,
) (generation.CreateRequest, videoGenerationEstimate, error) {
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	req.ModuleCode = moduleVideoGeneration
	req.ModuleCodeCamel = ""
	if strings.TrimSpace(req.Type) == "" {
		req.Type = videoModeText
	}
	if !isVideoGenerationRequest(req.Type) {
		return req, videoGenerationEstimate{}, errors.New("video estimate only accepts a video generation type")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		req.Prompt = "video generation estimate"
	}
	prepared, err := a.prepareGenerationRequest(data, user, req)
	if err != nil {
		return req, videoGenerationEstimate{}, err
	}
	rule := billingRuleForRequest(prepared, data)
	estimate := videoGenerationEstimate{
		Model:           prepared.Model,
		EstimatedPoints: generationPointCostForRequest(prepared, data),
		BillingType:     rule.BillingType,
		QuantityField:   billingQuantityField(rule.BillingType),
		Quantity:        billingQuantity(rule.BillingType, prepared),
		Note:            "试算不冻结或扣除积分，正式创建任务时以后端重新计算结果为准。",
	}
	return prepared, estimate, nil
}

func (a api) estimateVideoGenerationCost(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req generation.CreateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	_, estimate, err := a.prepareVideoGenerationEstimate(data, user, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, estimate)
}
