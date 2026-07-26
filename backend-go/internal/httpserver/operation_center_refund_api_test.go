package httpserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"
)

func TestOperationCenterRefundPermissionMapping(t *testing.T) {
	tests := []struct{ method, path, want string }{
		{http.MethodPost, "/api/v1/admin/channel-ecosystem/operation-centers/service-1/refunds", "channel:operation-center:refund-request"},
		{http.MethodGet, "/api/v1/admin/channel-ecosystem/refunds/task-1", "channel:operation-center:refund-view"},
		{http.MethodGet, "/api/v1/admin/channel-ecosystem/refunds", "channel:operation-center:refund-view"},
		{http.MethodPost, "/api/v1/admin/channel-ecosystem/refunds/task-1/retry", "channel:operation-center:refund-retry"},
		{http.MethodPost, "/api/v1/admin/channel-ecosystem/refunds/task-1/manual-submit", "channel:operation-center:refund-manual-submit"},
		{http.MethodPost, "/api/v1/admin/channel-ecosystem/refunds/task-1/manual-approve", "channel:operation-center:refund-manual-approve"},
	}
	for _, item := range tests {
		request := httptest.NewRequest(item.method, item.path, nil)
		if got := adminPermissionForRequest(request); got != item.want {
			t.Fatalf("%s %s permission=%s want=%s", item.method, item.path, got, item.want)
		}
	}
	if containsString(rolePermissionMatrix[roleOperation], "channel:operation-center:refund-manual-approve") {
		t.Fatal("operation review role inherited finance approval")
	}
}

func TestOperationCenterRefundHTTPValidationReplayQueryAndSensitiveResponse(t *testing.T) {
	management := &fakeRefundManagement{}
	executor := &fakeRefundExecutor{}
	api := newOperationCenterRefundAPI(management, executor)
	unauthorized := httptest.NewRequest(http.MethodPost, "/refunds", bytes.NewBufferString(`{}`))
	unauthorized.SetPathValue("id", "service-1")
	unauthorizedResult := httptest.NewRecorder()
	api.requestActive(unauthorizedResult, unauthorized)
	if unauthorizedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized=%d", unauthorizedResult.Code)
	}
	bad := refundAPIRequest(http.MethodPost, `{"expectedServiceStatus":"ACTIVE"}`)
	bad.SetPathValue("id", "service-1")
	badResult := httptest.NewRecorder()
	api.requestActive(badResult, bad)
	if badResult.Code != http.StatusBadRequest {
		t.Fatalf("bad request=%d %s", badResult.Code, badResult.Body.String())
	}
	request := refundAPIRequest(http.MethodPost, `{"idempotencyKey":"request-key","expectedServiceStatus":"ACTIVE","reason":"full refund"}`)
	request.SetPathValue("id", "service-1")
	requestResult := httptest.NewRecorder()
	api.requestActive(requestResult, request)
	if requestResult.Code != http.StatusOK || management.requestCommand.ExpectedServiceStatus != operationcenter.OperationCenterServiceActive {
		t.Fatalf("request=%d %s command=%+v", requestResult.Code, requestResult.Body.String(), management.requestCommand)
	}
	query := refundAPIRequest(http.MethodGet, ``)
	query.SetPathValue("refundTaskId", "task-1")
	queryResult := httptest.NewRecorder()
	api.get(queryResult, query)
	if queryResult.Code != http.StatusOK || strings.Contains(strings.ToLower(queryResult.Body.String()), "accesstoken") || strings.Contains(queryResult.Body.String(), "openid-secret") {
		t.Fatalf("query response=%s", queryResult.Body.String())
	}
	submit := refundAPIRequest(http.MethodPost, `{"idempotencyKey":"submit-key","channelRefundNo":"channel-1","refundAmountCents":100,"voucherReference":"voucher","voucherFileHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reason":"manual"}`)
	submit.SetPathValue("refundTaskId", "task-1")
	submitResult := httptest.NewRecorder()
	api.manualSubmit(submitResult, submit)
	if submitResult.Code != http.StatusOK || management.submitCommand.RefundAmountCents != 100 {
		t.Fatalf("submit=%d %s", submitResult.Code, submitResult.Body.String())
	}
	approve := refundAPIRequest(http.MethodPost, `{"idempotencyKey":"approve-key","expectedManualStatus":"SUBMITTED","approvalDecision":"APPROVED","reason":"checked"}`)
	approve.SetPathValue("refundTaskId", "task-1")
	approveResult := httptest.NewRecorder()
	api.manualApprove(approveResult, approve)
	if approveResult.Code != http.StatusOK || management.reviewCommand.Decision != "APPROVED" {
		t.Fatalf("approve=%d %s", approveResult.Code, approveResult.Body.String())
	}
	retry := refundAPIRequest(http.MethodPost, `{"idempotencyKey":"retry-key","reason":"operator retry"}`)
	retry.SetPathValue("refundTaskId", "task-1")
	retryResult := httptest.NewRecorder()
	api.retry(retryResult, retry)
	if retryResult.Code != http.StatusOK || executor.command.RefundTaskID != "task-1" {
		t.Fatalf("retry=%d %s", retryResult.Code, retryResult.Body.String())
	}
}

func refundAPIRequest(method, body string) *http.Request {
	request := httptest.NewRequest(method, "/refund", bytes.NewBufferString(body))
	return request.WithContext(context.WithValue(context.WithValue(request.Context(), actorIDContextKey, "finance-1"), actorRoleContextKey, roleFinance))
}

type fakeRefundManagement struct {
	requestCommand operationcenter.RefundRequestCommand
	submitCommand  operationcenter.ManualRefundSubmitCommand
	reviewCommand  operationcenter.ManualRefundReviewCommand
}

func (fake *fakeRefundManagement) RequestActiveRefund(_ context.Context, command operationcenter.RefundRequestCommand) (operationcenter.RefundManagementResult, error) {
	fake.requestCommand = command
	return operationcenter.RefundManagementResult{RefundTask: &operationcenter.OperationCenterRefundTask{ID: "task-1"}}, nil
}
func (fake *fakeRefundManagement) GetRefund(context.Context, string) (operationcenter.RefundManagementView, error) {
	now := operationcenter.RefundManagementView{RefundTaskID: "task-1", ServiceOrderID: "service-1", ProviderResponseSummary: operationcenter.JSONSnapshot{"provider": "mock"}}
	return now, nil
}
func (fake *fakeRefundManagement) ListRefunds(context.Context, operationcenter.RefundListFilter) ([]operationcenter.RefundManagementView, error) {
	item, _ := fake.GetRefund(context.Background(), "task-1")
	return []operationcenter.RefundManagementView{item}, nil
}
func (fake *fakeRefundManagement) SubmitManualRefund(_ context.Context, command operationcenter.ManualRefundSubmitCommand) (operationcenter.RefundManagementResult, error) {
	fake.submitCommand = command
	return operationcenter.RefundManagementResult{RefundTask: &operationcenter.OperationCenterRefundTask{ID: command.RefundTaskID}}, nil
}
func (fake *fakeRefundManagement) ReviewManualRefund(_ context.Context, command operationcenter.ManualRefundReviewCommand) (operationcenter.RefundManagementResult, error) {
	fake.reviewCommand = command
	return operationcenter.RefundManagementResult{RefundTask: &operationcenter.OperationCenterRefundTask{ID: command.RefundTaskID}}, nil
}

type fakeRefundExecutor struct {
	command operationcenter.RefundSagaCommand
}

func (fake *fakeRefundExecutor) Execute(_ context.Context, command operationcenter.RefundSagaCommand) (operationcenter.RefundSagaResult, error) {
	fake.command = command
	return operationcenter.RefundSagaResult{RefundTaskID: command.RefundTaskID}, nil
}
