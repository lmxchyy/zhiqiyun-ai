package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type connectorPPTExecution struct {
	Task           pptapp.Task
	Payload        []byte
	File           storagecenter.FileObject
	PointCost      int64
	TenantID       string
	OrganizationID string
	BillingTask    generationTask
	BillingRequest generation.CreateRequest
}

func (a api) estimateConnectorPPT(ctx context.Context, userID string, enterpriseID string, req pptapp.GenerateRequest) (pptapp.GenerateRequest, int64, error) {
	user, data, err := a.connectorUserAndCapabilityData(ctx, userID)
	if err != nil {
		return req, 0, err
	}
	req.UserID = user.ID
	authorization, err := a.authorizeConnectorCapability(user.ID, enterpriseID, modulePPTGeneration)
	if err != nil {
		return req, 0, err
	}
	capability, err := a.preparePPTCapabilityRequestWithAuthorization(data, user, req.Prompt, req.TextModel, req.SlideCount, pptImagesEnabled(req.ImageSource), false, &authorization)
	if err != nil {
		return req, 0, err
	}
	req.TextModel = capability.Model
	req.SlideCount = int(anyFloatOrDefault(capability.Params["page_count"], float64(req.SlideCount)))
	task := pptapp.Task{UserID: user.ID, Prompt: req.Prompt, SlideCount: req.SlideCount, TextModel: req.TextModel, ImageSource: normalizedPPTImageSource(req.ImageSource)}
	return req, int64(pptPointCostWithRules(task, data)), nil
}

func (a api) executeConnectorPPT(ctx context.Context, userID, enterpriseID, clientRequestID string, req pptapp.GenerateRequest, billingMetadata map[string]any) (connectorPPTExecution, error) {
	user, data, err := a.connectorUserAndCapabilityData(ctx, userID)
	if err != nil {
		return connectorPPTExecution{}, err
	}
	req.UserID, req.ClientRequestID = user.ID, clientRequestID
	authorization, err := a.authorizeConnectorCapability(user.ID, enterpriseID, modulePPTGeneration)
	if err != nil {
		return connectorPPTExecution{}, err
	}
	capability, err := a.preparePPTCapabilityRequestWithAuthorization(data, user, req.Prompt, req.TextModel, req.SlideCount, pptImagesEnabled(req.ImageSource), false, &authorization)
	if err != nil {
		return connectorPPTExecution{}, err
	}
	req.TextModel = capability.Model
	req.SlideCount = int(anyFloatOrDefault(capability.Params["page_count"], float64(req.SlideCount)))
	billingParams := map[string]any{
		"page_count":     req.SlideCount,
		"with_images":    pptImagesEnabled(req.ImageSource),
		"source_type":    "feishu",
		"source_task_id": clientRequestID,
	}
	for key, value := range billingMetadata {
		billingParams[key] = value
	}
	billingReq := generation.CreateRequest{
		UserID: user.ID, ClientRequestID: clientRequestID, Type: "PPT_GENERATION", ModuleCode: modulePPTGeneration,
		Prompt: req.Prompt, Model: req.TextModel, Params: billingParams,
	}
	connectorMetadata := takeConnectorMetadata(billingReq.Params)
	billingReq, err = a.prepareGenerationRequestWithAuthorization(data, user, billingReq, &authorization)
	if err != nil {
		return connectorPPTExecution{}, fmt.Errorf("authorize connector ppt billing: %w", err)
	}
	for key, value := range connectorMetadata {
		billingReq.Params[key] = value
	}
	billingTask, err := a.store.CreatePendingGenerationTask(billingReq)
	if err != nil {
		return connectorPPTExecution{}, fmt.Errorf("reserve connector ppt generation: %w", err)
	}
	failBilling := func(cause error) (connectorPPTExecution, error) {
		_, _ = a.store.FailGenerationTask(billingTask.ID, generationErrorMessage(cause))
		return connectorPPTExecution{}, cause
	}
	if req.Outline == nil {
		outline, outlineErr := a.generatePPTOutlineWithModel(ctx, outlineRequestFromGenerate(req))
		if outlineErr != nil {
			return failBilling(fmt.Errorf("generate ppt outline: %w", outlineErr))
		}
		req.Outline, req.SlideCount = &outline, len(outline.Slides)
	}
	response, err := a.pptService.GenerateWithConcurrency(req, 0, 0)
	if err != nil {
		return failBilling(err)
	}
	task, err := a.pptService.GetTask(user.ID, response.TaskID)
	if err != nil {
		return failBilling(err)
	}
	if shouldAutoGeneratePPTImages(req, a.cfg) && pptAutoImageEnabled(a.cfg) {
		a.runPPTTaskImageGeneration(user, task)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		task, err = a.pptService.GetTask(user.ID, response.TaskID)
		if err != nil {
			return failBilling(err)
		}
		if task.Status == pptapp.StatusSuccess {
			break
		}
		if task.Status == pptapp.StatusFailed {
			return failBilling(errors.New(firstNonEmptyString(task.ErrorMessage, "ppt generation failed")))
		}
		if time.Now().After(deadline) {
			return failBilling(errors.New("ppt generation timed out"))
		}
		select {
		case <-ctx.Done():
			return failBilling(ctx.Err())
		case <-time.After(350 * time.Millisecond):
		}
	}
	task = a.materializePPTTaskVisualURLs(ctx, user, task)
	payload, err := buildPPTX(task)
	if err != nil {
		return failBilling(fmt.Errorf("export pptx: %w", err))
	}
	if a.fileService == nil {
		return failBilling(errors.New("private file storage is unavailable"))
	}
	tenantID := firstNonEmptyString(stringValue(capability.Params["tenant_id"]), "tenant_default")
	available, err := a.fileService.StorageAvailable(ctx, tenantID)
	if err != nil {
		return failBilling(err)
	}
	if !available {
		return failBilling(errors.New("private file storage is not configured"))
	}
	fileName := pptxDownloadFileName(task)
	file, err := a.fileService.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: tenantID, UserID: user.ID, FileName: fileName, FileSize: int64(len(payload)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "generation_result", BusinessID: task.TaskID, Visibility: "PRIVATE",
	}, bytes.NewReader(payload))
	if err != nil {
		return failBilling(fmt.Errorf("store pptx: %w", err))
	}
	return connectorPPTExecution{Task: task, Payload: payload, File: file, PointCost: int64(billingTask.PointCost),
		TenantID: tenantID, OrganizationID: stringValue(capability.Params["organization_id"]), BillingTask: billingTask, BillingRequest: billingReq}, nil
}

func (a api) connectorStoredFileURL(ctx context.Context, userID string, file storagecenter.FileObject) (string, int64, error) {
	if a.fileService == nil || strings.TrimSpace(file.FileID) == "" {
		return "", 0, errors.New("stored file is unavailable")
	}
	ticket, err := a.fileService.AccessURL(ctx, storagecenter.AccessContext{TenantID: file.TenantID, UserID: userID}, file.FileID, true)
	if err != nil {
		return "", 0, err
	}
	return ticket.URL, ticket.ExpiresIn, nil
}
