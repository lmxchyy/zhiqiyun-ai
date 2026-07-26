package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"
)

type operationCenterReviewWorkflow interface {
	Review(context.Context, operationcenter.ReviewCommand) (operationcenter.WorkflowResult, error)
	GetReviewStatus(context.Context, string) (operationcenter.ReviewStatusView, error)
}

type operationCenterReviewAPI struct{ workflow operationCenterReviewWorkflow }

func newOperationCenterReviewAPI(workflow operationCenterReviewWorkflow) operationCenterReviewAPI {
	return operationCenterReviewAPI{workflow: workflow}
}

type operationCenterReviewRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
	ExpectedStatus string `json:"expectedStatus"`
	RequestID      string `json:"requestId"`
	Reason         string `json:"reason"`
}

func (api operationCenterReviewAPI) approve(w http.ResponseWriter, r *http.Request) {
	api.review(w, r, operationcenter.ReviewApproved)
}

func (api operationCenterReviewAPI) reject(w http.ResponseWriter, r *http.Request) {
	api.review(w, r, operationcenter.ReviewRejected)
}

func (api operationCenterReviewAPI) review(w http.ResponseWriter, r *http.Request, decision operationcenter.ReviewDecision) {
	actorID, _ := actorFromRequest(r)
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if api.workflow == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	var request operationCenterReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.ExpectedStatus = strings.ToUpper(strings.TrimSpace(request.ExpectedStatus))
	if request.IdempotencyKey == "" || request.ExpectedStatus == "" {
		writeError(w, http.StatusBadRequest, errors.New("idempotencyKey and expectedStatus are required"))
		return
	}
	result, err := api.workflow.Review(r.Context(), operationcenter.ReviewCommand{
		ServiceOrderID: r.PathValue("id"), Decision: decision,
		ExpectedStatus: operationcenter.OperationCenterServiceStatus(request.ExpectedStatus),
		IdempotencyKey: request.IdempotencyKey, ReviewedBy: actorID,
		RequestID: strings.TrimSpace(request.RequestID), Reason: strings.TrimSpace(request.Reason),
	})
	if err != nil {
		writeOperationCenterWorkflowError(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"item": result.ServiceOrder, "reviewEvent": result.ReviewEvent,
		"refundTask": result.RefundTask, "idempotentReplay": result.IdempotentReplay,
	})
}

func (api operationCenterReviewAPI) status(w http.ResponseWriter, r *http.Request) {
	actorID, _ := actorFromRequest(r)
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if api.workflow == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	item, err := api.workflow.GetReviewStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		writeOperationCenterWorkflowError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func writeOperationCenterWorkflowError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationcenter.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, operationcenter.ErrReviewDecisionConflict),
		errors.Is(err, operationcenter.ErrExpectedServiceStatus),
		errors.Is(err, operationcenter.ErrInvalidServiceTransition),
		errors.Is(err, operationcenter.ErrInvalidRefundTransition),
		errors.Is(err, operationcenter.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, operationcenter.ErrConstraintViolation),
		errors.Is(err, operationcenter.ErrFrozenSnapshotMissing),
		errors.Is(err, operationcenter.ErrPaymentAmountMismatch),
		errors.Is(err, operationcenter.ErrPaymentNotSuccessful):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, operationcenter.ErrWorkflowUnavailable):
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
