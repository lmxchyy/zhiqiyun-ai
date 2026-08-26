package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
	pricingdomain "xianzhi-ai/backend-go/internal/pricing"
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
	// Cost quotes should not require uploaded images. Inject a placeholder so
	// IMAGE_TO_VIDEO models (e.g. Preview) can still return list/price estimates
	// before the user picks a reference frame.
	if strings.EqualFold(strings.TrimSpace(req.Type), videoModeImage) {
		firstFrame := strings.TrimSpace(fmt.Sprint(req.Params["first_frame"]))
		if firstFrame == "" || firstFrame == "<nil>" {
			req.Params["first_frame"] = "estimate://placeholder"
		}
	}
	prepared, err := a.prepareGenerationRequest(data, user, req)
	if err != nil {
		return req, videoGenerationEstimate{}, err
	}
	quote, err := generationQuoteForRequest(prepared, data)
	if err != nil {
		return prepared, videoGenerationEstimate{}, err
	}
	rule := billingRuleForRequest(prepared, data)
	estimate := videoGenerationEstimate{
		Model:           prepared.Model,
		EstimatedPoints: quote.RequiredPoints,
		BillingType:     rule.BillingType,
		QuantityField:   billingQuantityField(rule.BillingType),
		Quantity:        billingQuantity(rule.BillingType, prepared),
		Note:            "试算不冻结或扣除积分，正式创建任务时以后端重新计算结果为准。",
	}
	return prepared, estimate, nil
}

type generationQuoteResponse struct {
	Model                string         `json:"model"`
	BusinessType         string         `json:"businessType"`
	RequiredPoints       int            `json:"requiredPoints"`
	PricingRuleID        string         `json:"pricingRuleId"`
	PricingRuleVersion   int            `json:"pricingRuleVersion"`
	BillingUnit          string         `json:"billingUnit"`
	Quantity             float64        `json:"quantity"`
	Breakdown            map[string]any `json:"breakdown"`
	NormalizedParameters map[string]any `json:"normalizedParameters"`
}

func (a api) prepareGenerationQuote(data adminPlatformData, user adminUser, req generation.CreateRequest) (generation.CreateRequest, pricingdomain.Quote, error) {
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	moduleCode := canonicalModuleCode(requestModuleCode(req))
	if moduleCode == "" {
		moduleCode = moduleCodeForType(req.Type)
	}
	if moduleCode == moduleVideoGeneration {
		if strings.TrimSpace(req.Type) == "" {
			req.Type = videoModeText
		}
		if strings.EqualFold(strings.TrimSpace(req.Type), videoModeImage) {
			firstFrame := strings.TrimSpace(fmt.Sprint(req.Params["first_frame"]))
			if firstFrame == "" || firstFrame == "<nil>" {
				req.Params["first_frame"] = "estimate://placeholder"
			}
		}
	} else {
		moduleCode = moduleImageGeneration
		if strings.TrimSpace(req.Type) == "" {
			req.Type = "TEXT_TO_IMAGE"
		}
	}
	req.ModuleCode = moduleCode
	req.ModuleCodeCamel = ""
	if strings.TrimSpace(req.Prompt) == "" {
		req.Prompt = "generation pricing quote"
	}
	prepared, err := a.prepareGenerationRequest(data, user, req)
	if err != nil {
		return req, pricingdomain.Quote{}, err
	}
	quote, err := generationQuoteForRequest(prepared, data)
	if err != nil {
		return prepared, pricingdomain.Quote{}, err
	}
	return prepared, quote, nil
}

func (a api) quoteGenerationCost(w http.ResponseWriter, r *http.Request) {
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
	prepared, quote, err := a.prepareGenerationQuote(data, user, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, generationQuoteResponse{
		Model: prepared.Model, BusinessType: canonicalModuleCode(requestModuleCode(prepared)),
		RequiredPoints: quote.RequiredPoints, PricingRuleID: quote.PricingRuleID,
		PricingRuleVersion: quote.PricingRuleVersion, BillingUnit: quote.BillingUnit,
		Quantity: quote.Quantity, Breakdown: quote.Breakdown,
		NormalizedParameters: quote.NormalizedParameters,
	})
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
