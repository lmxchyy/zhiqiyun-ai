package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type connectorCapabilityStore interface {
	aiCapabilityAdminData(context.Context) (adminPlatformData, error)
}

// executeConnectorImageGeneration reuses the exact user-facing generation,
// storage, asset and billing pipeline, but waits inside the connector worker
// instead of inside the Feishu HTTP callback.
func (a api) executeConnectorImageGeneration(ctx context.Context, userID string, enterpriseID string, req generation.CreateRequest) (generationTask, generation.CreateRequest, error) {
	user, data, err := a.connectorUserAndCapabilityData(ctx, userID)
	if err != nil {
		return generationTask{}, req, err
	}
	req.UserID = user.ID
	if strings.TrimSpace(req.Type) == "" {
		req.Type = "TEXT_TO_IMAGE"
	}
	req.ModuleCode = moduleImageGeneration
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	connectorMetadata := takeConnectorMetadata(req.Params)
	req, err = a.prepareConnectorGenerationRequest(data, user, enterpriseID, req)
	if err != nil {
		return generationTask{}, req, fmt.Errorf("authorize connector generation: %w", err)
	}
	for key, value := range connectorMetadata {
		req.Params[key] = value
	}
	service, err := a.connectorGenerationServiceForRequest(user, req)
	if err != nil {
		return generationTask{}, req, err
	}
	task, err := a.store.CreatePendingGenerationTask(req)
	if err != nil {
		return generationTask{}, req, fmt.Errorf("reserve connector generation: %w", err)
	}
	if strings.EqualFold(task.Status, "SUCCEEDED") {
		return task, req, nil
	}
	if strings.EqualFold(task.Status, "FAILED") || strings.EqualFold(task.Status, "CANCELLED") {
		return task, req, fmt.Errorf("generation task is %s", strings.ToLower(task.Status))
	}
	// The task is created before the synchronous provider call. Bind the
	// durable execution identity now so a retry/restart cannot bypass the
	// provider-execution guard.
	req.Params[providerExecutionTaskParam] = task.ID
	prepared, err := service.PrepareImageTask(ctx, cloneGenerationCreateRequest(req))
	if err != nil {
		prepared, err = a.prepareImageTaskWithFallback(ctx, req, err)
	}
	if err != nil {
		if errors.Is(err, pe.ErrUnknownResubmitBlocked) || errors.Is(err, pe.ErrProviderStillProcessing) {
			return task, req, fmt.Errorf("connector image recovery deferred: %w", err)
		}
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return task, req, fmt.Errorf("generate connector image: %w", err)
	}
	prepared, _, err = a.persistGeneratedImages(ctx, task.ID, prepared)
	if err != nil {
		// Provider success is already durable. Keep the task recoverable instead
		// of releasing its reservation on a local storage failure.
		return task, prepared, fmt.Errorf("persist connector image: %w", err)
	}
	completed, err := a.store.CompleteGenerationTask(task.ID, prepared)
	if err != nil {
		// Completion is the local settlement boundary; do not fail/release a
		// task whose provider result can be replayed locally.
		return task, prepared, fmt.Errorf("complete connector generation: %w", err)
	}
	return completed, prepared, nil
}

func (a api) estimateConnectorGeneration(ctx context.Context, userID string, enterpriseID string, req generation.CreateRequest) (generation.CreateRequest, int64, error) {
	user, data, err := a.connectorUserAndCapabilityData(ctx, userID)
	if err != nil {
		return req, 0, err
	}
	req.UserID = user.ID
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	connectorMetadata := takeConnectorMetadata(req.Params)
	req, err = a.prepareConnectorGenerationRequest(data, user, enterpriseID, req)
	if err != nil {
		return req, 0, err
	}
	for key, value := range connectorMetadata {
		req.Params[key] = value
	}
	quote, err := generationQuoteForRequest(req, data)
	if err != nil {
		return req, 0, err
	}
	return req, int64(quote.RequiredPoints), nil
}

func (a api) executeConnectorVideoGeneration(ctx context.Context, userID string, enterpriseID string, req generation.CreateRequest) (generationTask, generation.CreateRequest, storagecenter.FileObject, []byte, string, error) {
	user, data, err := a.connectorUserAndCapabilityData(ctx, userID)
	if err != nil {
		return generationTask{}, req, storagecenter.FileObject{}, nil, "", err
	}
	req.UserID = user.ID
	req.ModuleCode = moduleVideoGeneration
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	connectorMetadata := takeConnectorMetadata(req.Params)
	req, err = a.prepareConnectorGenerationRequest(data, user, enterpriseID, req)
	if err != nil {
		return generationTask{}, req, storagecenter.FileObject{}, nil, "", fmt.Errorf("authorize connector video generation: %w", err)
	}
	for key, value := range connectorMetadata {
		req.Params[key] = value
	}
	service, err := a.connectorGenerationServiceForRequest(user, req)
	if err != nil {
		return generationTask{}, req, storagecenter.FileObject{}, nil, "", err
	}
	task, err := a.store.CreatePendingGenerationTask(req)
	if err != nil {
		return generationTask{}, req, storagecenter.FileObject{}, nil, "", fmt.Errorf("reserve connector video generation: %w", err)
	}
	if strings.EqualFold(task.Status, "SUCCEEDED") {
		return task, req, storagecenter.FileObject{}, nil, "", nil
	}
	if strings.EqualFold(task.Status, "FAILED") || strings.EqualFold(task.Status, "CANCELLED") {
		return task, req, storagecenter.FileObject{}, nil, "", fmt.Errorf("generation task is %s", strings.ToLower(task.Status))
	}
	// The task is created before the synchronous provider call. Bind the
	// durable execution identity now so a retry/restart cannot bypass the
	// provider-execution guard.
	req.Params[providerExecutionTaskParam] = task.ID
	prepared, err := service.PrepareVideoTask(ctx, cloneGenerationCreateRequest(req))
	if err != nil {
		if errors.Is(err, pe.ErrUnknownResubmitBlocked) || errors.Is(err, pe.ErrProviderStillProcessing) {
			return task, req, storagecenter.FileObject{}, nil, "", fmt.Errorf("connector video recovery deferred: %w", err)
		}
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return task, req, storagecenter.FileObject{}, nil, "", fmt.Errorf("generate connector video: %w", err)
	}
	prepared, stored, raw, contentType, err := a.persistConnectorVideo(ctx, task.ID, prepared)
	if err != nil {
		// Provider success is already durable. Keep the task recoverable instead
		// of releasing its reservation on a local storage failure.
		return task, prepared, storagecenter.FileObject{}, nil, "", fmt.Errorf("persist connector video: %w", err)
	}
	completed, err := a.store.CompleteGenerationTask(task.ID, prepared)
	if err != nil {
		// Do not delete a durable artifact or fail/release the task. A later local
		// recovery can reuse it and complete billing exactly once.
		return task, prepared, storagecenter.FileObject{}, nil, "", fmt.Errorf("complete connector video: %w", err)
	}
	return completed, prepared, stored, raw, contentType, nil
}

func takeConnectorMetadata(params map[string]any) map[string]any {
	metadata := map[string]any{}
	for _, key := range []string{"source_type", "source_id", "source_task_id", "operator_external_id", "connector_id", "connector_task_id", "external_user_id", "external_message_id", "capability"} {
		if value, ok := params[key]; ok {
			metadata[key] = value
			delete(params, key)
		}
	}
	return metadata
}

func (a api) persistConnectorVideo(ctx context.Context, taskID string, req generation.CreateRequest) (generation.CreateRequest, storagecenter.FileObject, []byte, string, error) {
	videoURL := providerTaskString(req, "videoUrl")
	if videoURL == "" {
		return req, storagecenter.FileObject{}, nil, "", errors.New("video provider returned no video")
	}
	raw, contentType, extension, err := readGeneratedVideoArtifact(ctx, videoURL)
	if err != nil {
		return req, storagecenter.FileObject{}, nil, "", err
	}
	if contentType == "" {
		contentType = "video/mp4"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if a.fileService == nil {
		return req, storagecenter.FileObject{}, raw, contentType, nil
	}
	tenantID := firstNonEmptyString(stringValue(req.Params["tenant_id"]), "tenant_default")
	available, err := a.fileService.StorageAvailable(ctx, tenantID)
	if err != nil {
		return req, storagecenter.FileObject{}, nil, "", err
	}
	if !available {
		return req, storagecenter.FileObject{}, raw, contentType, nil
	}
	file, err := a.fileService.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID: tenantID, UserID: req.UserID, FileName: fmt.Sprintf("%s.%s", taskID, extension),
		FileSize: int64(len(raw)), MIMEType: contentType, BusinessType: "generation_result", BusinessID: taskID, Visibility: "PRIVATE",
	}, bytes.NewReader(raw))
	if err != nil {
		return req, storagecenter.FileObject{}, nil, "", err
	}
	req.Params[generatedStorageFilesParam] = []map[string]any{{
		"fileId": file.FileID, "tenantId": file.TenantID, "provider": file.Provider, "bucket": file.Bucket,
		"objectKey": file.ObjectKey, "fileSize": file.FileSize, "contentType": file.MIMEType, "sourceUrl": videoURL,
	}}
	return req, file, raw, contentType, nil
}

func (a api) connectorUserAndCapabilityData(ctx context.Context, userID string) (adminUser, adminPlatformData, error) {
	identityStore, ok := a.store.(activeIdentityStore)
	if !ok {
		return adminUser{}, adminPlatformData{}, errors.New("active identity store is unavailable")
	}
	user, found, err := identityStore.GetActiveUser(userID)
	if err != nil {
		return adminUser{}, adminPlatformData{}, fmt.Errorf("load connector user: %w", err)
	}
	if !found {
		return adminUser{}, adminPlatformData{}, errUnauthorized
	}
	if strings.TrimSpace(user.PlanID) == "" {
		user.PlanID = "plan_free"
	}
	var data adminPlatformData
	if capabilityStore, ok := a.store.(connectorCapabilityStore); ok {
		data, err = capabilityStore.aiCapabilityAdminData(ctx)
	} else {
		data, err = a.store.AdminData()
	}
	if err != nil {
		return adminUser{}, adminPlatformData{}, fmt.Errorf("load AI capability data: %w", err)
	}
	return user, data, nil
}

func (a api) prepareConnectorGenerationRequest(data adminPlatformData, user adminUser, enterpriseID string, req generation.CreateRequest) (generation.CreateRequest, error) {
	moduleCode := canonicalModuleCode(requestModuleCode(req))
	if moduleCode == "" {
		moduleCode = canonicalModuleCode(moduleCodeForType(req.Type))
	}
	authorization, err := a.authorizeConnectorCapability(user.ID, enterpriseID, moduleCode)
	if err != nil {
		return req, err
	}
	return a.prepareGenerationRequestWithAuthorization(data, user, req, &authorization)
}

func (a api) authorizeConnectorCapability(userID string, enterpriseID string, moduleCode string) (modelCallAuthorization, error) {
	authorizer, ok := a.store.(connectorModelCallAuthorizer)
	if !ok {
		return modelCallAuthorization{}, errEnterpriseServiceUnavailable
	}
	return authorizer.AuthorizeConnectorModelCall(userID, enterpriseID, moduleCode)
}

func (a api) connectorGenerationServiceForRequest(user adminUser, req generation.CreateRequest) (generation.Service, error) {
	service := a.generationService
	if a.connectorGenerationService != nil {
		return *a.connectorGenerationService, nil
	}
	if routeService, ok, err := a.generationServiceForUserRoute(user, req.Model); err != nil {
		return service, err
	} else if ok {
		return routeService, nil
	}
	if providerID := selectedGenerationProvider(req.Params); providerID != "" {
		return a.generationServiceForProvider(providerID, req)
	}
	if configured, ok, err := a.generationServiceForConfiguredModel(req.Model); err != nil {
		return service, err
	} else if ok {
		return configured, nil
	}
	return service, nil
}
