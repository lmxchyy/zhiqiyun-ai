package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"
)

type operationCenterRefundManagement interface {
	RequestActiveRefund(context.Context, operationcenter.RefundRequestCommand) (operationcenter.RefundManagementResult, error)
	GetRefund(context.Context, string) (operationcenter.RefundManagementView, error)
	ListRefunds(context.Context, operationcenter.RefundListFilter) ([]operationcenter.RefundManagementView, error)
	SubmitManualRefund(context.Context, operationcenter.ManualRefundSubmitCommand) (operationcenter.RefundManagementResult, error)
	ReviewManualRefund(context.Context, operationcenter.ManualRefundReviewCommand) (operationcenter.RefundManagementResult, error)
}

type operationCenterRefundExecutor interface {
	Execute(context.Context, operationcenter.RefundSagaCommand) (operationcenter.RefundSagaResult, error)
}

type operationCenterRefundAPI struct {
	management operationCenterRefundManagement
	executor   operationCenterRefundExecutor
}

func newOperationCenterRefundAPI(management operationCenterRefundManagement, executor operationCenterRefundExecutor) operationCenterRefundAPI {
	return operationCenterRefundAPI{management: management, executor: executor}
}

type activeRefundRequest struct {
	IdempotencyKey        string `json:"idempotencyKey"`
	ExpectedServiceStatus string `json:"expectedServiceStatus"`
	Reason                string `json:"reason"`
	RequestID             string `json:"requestId"`
}

func (api operationCenterRefundAPI) requestActive(w http.ResponseWriter, r *http.Request) {
	actorID, _ := actorFromRequest(r)
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if api.management == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	var payload activeRefundRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload.ExpectedServiceStatus = strings.ToUpper(strings.TrimSpace(payload.ExpectedServiceStatus))
	if strings.TrimSpace(payload.IdempotencyKey) == "" || payload.ExpectedServiceStatus != string(operationcenter.OperationCenterServiceActive) || strings.TrimSpace(payload.Reason) == "" {
		writeError(w, http.StatusBadRequest, errors.New("idempotencyKey, expectedServiceStatus=ACTIVE and reason are required"))
		return
	}
	result, err := api.management.RequestActiveRefund(r.Context(), operationcenter.RefundRequestCommand{
		ServiceOrderID: r.PathValue("id"), IdempotencyKey: strings.TrimSpace(payload.IdempotencyKey),
		ExpectedServiceStatus: operationcenter.OperationCenterServiceActive, Reason: strings.TrimSpace(payload.Reason),
		RequestedBy: actorID, RequestID: strings.TrimSpace(payload.RequestID),
	})
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	writeJSON(w, result)
}

func (api operationCenterRefundAPI) get(w http.ResponseWriter, r *http.Request) {
	if api.management == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	item, err := api.management.GetRefund(r.Context(), r.PathValue("refundTaskId"))
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (api operationCenterRefundAPI) list(w http.ResponseWriter, r *http.Request) {
	if api.management == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	filter, err := parseRefundListFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	items, err := api.management.ListRefunds(r.Context(), filter)
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "count": len(items)})
}

type retryRefundRequest struct{ IdempotencyKey, Reason, RequestID string }

func (api operationCenterRefundAPI) retry(w http.ResponseWriter, r *http.Request) {
	actorID, _ := actorFromRequest(r)
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if api.management == nil || api.executor == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrRefundSchedulerDisabled)
		return
	}
	var payload retryRefundRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(payload.IdempotencyKey) == "" || strings.TrimSpace(payload.Reason) == "" {
		writeError(w, http.StatusBadRequest, errors.New("idempotencyKey and reason are required"))
		return
	}
	view, err := api.management.GetRefund(r.Context(), r.PathValue("refundTaskId"))
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	result, err := api.executor.Execute(r.Context(), operationcenter.RefundSagaCommand{
		ServiceOrderID: view.ServiceOrderID, RefundTaskID: view.RefundTaskID, OperatorID: actorID,
		RequestID: strings.TrimSpace(payload.RequestID), TransactionGroupID: strings.TrimSpace(payload.IdempotencyKey), Reason: strings.TrimSpace(payload.Reason),
	})
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	writeJSON(w, result)
}

type manualSubmitRequest struct {
	IdempotencyKey, ChannelRefundNo, VoucherReference, VoucherFileHash, Reason, RequestID string
	RefundAmountCents                                                                     int64 `json:"refundAmountCents"`
}

func (api operationCenterRefundAPI) manualSubmit(w http.ResponseWriter, r *http.Request) {
	actorID, _ := actorFromRequest(r)
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if api.management == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	var payload manualSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := api.management.SubmitManualRefund(r.Context(), operationcenter.ManualRefundSubmitCommand{
		RefundTaskID: r.PathValue("refundTaskId"), IdempotencyKey: strings.TrimSpace(payload.IdempotencyKey),
		ChannelRefundNo: strings.TrimSpace(payload.ChannelRefundNo), RefundAmountCents: payload.RefundAmountCents,
		VoucherReference: strings.TrimSpace(payload.VoucherReference), VoucherFileHash: strings.ToLower(strings.TrimSpace(payload.VoucherFileHash)),
		Reason: strings.TrimSpace(payload.Reason), SubmittedBy: actorID, RequestID: strings.TrimSpace(payload.RequestID),
	})
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	writeJSON(w, result)
}

type manualReviewRequest struct {
	IdempotencyKey, ExpectedManualStatus, ApprovalDecision, Reason, RequestID string
}

func (api operationCenterRefundAPI) manualApprove(w http.ResponseWriter, r *http.Request) {
	actorID, _ := actorFromRequest(r)
	if actorID == "" {
		writeError(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	if api.management == nil {
		writeError(w, http.StatusServiceUnavailable, operationcenter.ErrWorkflowUnavailable)
		return
	}
	var payload manualReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := api.management.ReviewManualRefund(r.Context(), operationcenter.ManualRefundReviewCommand{
		RefundTaskID: r.PathValue("refundTaskId"), IdempotencyKey: strings.TrimSpace(payload.IdempotencyKey),
		ExpectedStatus: operationcenter.ManualRefundStatus(strings.ToUpper(strings.TrimSpace(payload.ExpectedManualStatus))),
		Decision:       strings.ToUpper(strings.TrimSpace(payload.ApprovalDecision)), Reason: strings.TrimSpace(payload.Reason),
		ReviewedBy: actorID, RequestID: strings.TrimSpace(payload.RequestID),
	})
	if err != nil {
		writeOperationCenterRefundError(w, err)
		return
	}
	writeJSON(w, result)
}

func parseRefundListFilter(r *http.Request) (operationcenter.RefundListFilter, error) {
	query := r.URL.Query()
	filter := operationcenter.RefundListFilter{
		TenantID: strings.TrimSpace(query.Get("tenant_id")), ServiceOrderID: strings.TrimSpace(query.Get("service_order_id")),
		ServiceStatus:  operationcenter.OperationCenterServiceStatus(strings.ToUpper(strings.TrimSpace(query.Get("service_status")))),
		RefundStatus:   operationcenter.OperationCenterRefundStatus(strings.ToUpper(strings.TrimSpace(query.Get("refund_status")))),
		ProviderResult: operationcenter.RefundProviderResult(strings.ToUpper(strings.TrimSpace(query.Get("provider_result")))),
		FailureClass:   operationcenter.RefundFailureClass(strings.ToUpper(strings.TrimSpace(query.Get("failure_class")))),
		PaymentChannel: strings.TrimSpace(query.Get("payment_channel")),
	}
	if value := strings.TrimSpace(query.Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return filter, err
		}
		filter.Limit = parsed
	}
	if value := strings.TrimSpace(query.Get("offset")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return filter, err
		}
		filter.Offset = parsed
	}
	for key, target := range map[string]**time.Time{"next_retry_at": &filter.NextRetryBefore, "created_from": &filter.CreatedFrom, "created_to": &filter.CreatedTo} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return filter, err
			}
			*target = &parsed
		}
	}
	return filter, nil
}

func writeOperationCenterRefundError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationcenter.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, operationcenter.ErrIdempotencyConflict), errors.Is(err, operationcenter.ErrUniqueConflict),
		errors.Is(err, operationcenter.ErrRefundAlreadyRequested), errors.Is(err, operationcenter.ErrExpectedServiceStatus),
		errors.Is(err, operationcenter.ErrRefundSagaNotReady), errors.Is(err, operationcenter.ErrManualRefundConflict),
		errors.Is(err, operationcenter.ErrManualRefundNotSubmitted), errors.Is(err, operationcenter.ErrManualRefundSelfApproval):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, operationcenter.ErrConstraintViolation), errors.Is(err, operationcenter.ErrRefundPolicyNotFullOnly),
		errors.Is(err, operationcenter.ErrPaymentAmountMismatch), errors.Is(err, operationcenter.ErrPaymentNotSuccessful):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, operationcenter.ErrRefundSchedulerDisabled), errors.Is(err, operationcenter.ErrRewardReleaseSchedulerDisabled),
		errors.Is(err, operationcenter.ErrRefundProviderUnavailable), errors.Is(err, operationcenter.ErrWorkflowUnavailable):
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
