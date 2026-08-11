package httpserver

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"
)

func TestOperationCenterReviewAdminPermissionMapping(t *testing.T) {
	paths := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/admin/channel-ecosystem/operation-centers/service-1/approve"},
		{http.MethodPost, "/api/v1/admin/channel-ecosystem/operation-centers/service-1/reject"},
		{http.MethodGet, "/api/v1/admin/channel-ecosystem/operation-centers/service-1"},
	}
	for _, item := range paths {
		request := httptest.NewRequest(item.method, item.path, nil)
		if got := adminPermissionForRequest(request); got != "channel:operation-center:review" {
			t.Fatalf("%s %s permission=%s", item.method, item.path, got)
		}
	}
}

func TestOperationCenterReviewHTTPValidationReplayConflictAndQuery(t *testing.T) {
	service := &fakeOperationCenterReviewWorkflow{}
	api := newOperationCenterReviewAPI(service)

	unauthorized := httptest.NewRequest(http.MethodPost, "/approve", bytes.NewBufferString(`{"idempotencyKey":"key","expectedStatus":"REVIEW_REQUIRED"}`))
	unauthorized.SetPathValue("id", "service-1")
	unauthorizedResult := httptest.NewRecorder()
	api.approve(unauthorizedResult, unauthorized)
	if unauthorizedResult.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%s", unauthorizedResult.Code, unauthorizedResult.Body.String())
	}

	missingKey := reviewAPIRequest(http.MethodPost, "service-1", `{"expectedStatus":"REVIEW_REQUIRED"}`)
	missingResult := httptest.NewRecorder()
	api.approve(missingResult, missingKey)
	if missingResult.Code != http.StatusBadRequest {
		t.Fatalf("missing key status=%d body=%s", missingResult.Code, missingResult.Body.String())
	}

	approved := reviewAPIRequest(http.MethodPost, "service-1", `{"idempotencyKey":"approve-key","expectedStatus":"REVIEW_REQUIRED","reason":"ok"}`)
	approvedResult := httptest.NewRecorder()
	api.approve(approvedResult, approved)
	if approvedResult.Code != http.StatusOK || service.lastReview.Decision != operationcenter.ReviewApproved {
		t.Fatalf("approve status=%d body=%s command=%+v", approvedResult.Code, approvedResult.Body.String(), service.lastReview)
	}

	service.reviewResult.IdempotentReplay = true
	replay := reviewAPIRequest(http.MethodPost, "service-1", `{"idempotencyKey":"approve-key","expectedStatus":"REVIEW_REQUIRED"}`)
	replayResult := httptest.NewRecorder()
	api.approve(replayResult, replay)
	if replayResult.Code != http.StatusOK || !bytes.Contains(replayResult.Body.Bytes(), []byte(`"idempotentReplay":true`)) {
		t.Fatalf("replay status=%d body=%s", replayResult.Code, replayResult.Body.String())
	}

	service.reviewErr = operationcenter.ErrReviewDecisionConflict
	conflict := reviewAPIRequest(http.MethodPost, "service-1", `{"idempotencyKey":"same-key","expectedStatus":"REVIEW_REQUIRED"}`)
	conflictResult := httptest.NewRecorder()
	api.reject(conflictResult, conflict)
	if conflictResult.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflictResult.Code, conflictResult.Body.String())
	}

	service.reviewErr = operationcenter.ErrExpectedServiceStatus
	illegal := reviewAPIRequest(http.MethodPost, "service-1", `{"idempotencyKey":"new-key","expectedStatus":"REVIEW_REQUIRED"}`)
	illegalResult := httptest.NewRecorder()
	api.approve(illegalResult, illegal)
	if illegalResult.Code != http.StatusConflict {
		t.Fatalf("illegal status=%d body=%s", illegalResult.Code, illegalResult.Body.String())
	}

	service.reviewErr = nil
	query := reviewAPIRequest(http.MethodGet, "service-1", ``)
	queryResult := httptest.NewRecorder()
	api.status(queryResult, query)
	if queryResult.Code != http.StatusOK || !bytes.Contains(queryResult.Body.Bytes(), []byte(`"status":"REVIEW_REQUIRED"`)) {
		t.Fatalf("query status=%d body=%s", queryResult.Code, queryResult.Body.String())
	}
}

func reviewAPIRequest(method, id, body string) *http.Request {
	request := httptest.NewRequest(method, "/operation-centers/"+id, bytes.NewBufferString(body))
	request.SetPathValue("id", id)
	request = request.WithContext(context.WithValue(context.WithValue(request.Context(), actorIDContextKey, "reviewer-1"), actorRoleContextKey, "SUPER_ADMIN"))
	return request
}

type fakeOperationCenterReviewWorkflow struct {
	lastReview   operationcenter.ReviewCommand
	reviewResult operationcenter.WorkflowResult
	reviewErr    error
}

func (workflow *fakeOperationCenterReviewWorkflow) Review(_ context.Context, command operationcenter.ReviewCommand) (operationcenter.WorkflowResult, error) {
	workflow.lastReview = command
	if workflow.reviewResult.ServiceOrder == nil {
		workflow.reviewResult.ServiceOrder = &operationcenter.OperationCenterServiceOrder{ID: command.ServiceOrderID, Status: operationcenter.OperationCenterServiceActive}
	}
	return workflow.reviewResult, workflow.reviewErr
}

func (workflow *fakeOperationCenterReviewWorkflow) GetReviewStatus(context.Context, string) (operationcenter.ReviewStatusView, error) {
	if errors.Is(workflow.reviewErr, operationcenter.ErrNotFound) {
		return operationcenter.ReviewStatusView{}, workflow.reviewErr
	}
	return operationcenter.ReviewStatusView{ServiceOrderID: "service-1", Status: operationcenter.OperationCenterServiceReviewRequired}, nil
}
