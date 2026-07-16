package httpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type connectorCapabilityStore interface {
	aiCapabilityAdminData(context.Context) (adminPlatformData, error)
}

// executeConnectorImageGeneration reuses the exact user-facing generation,
// storage, asset and billing pipeline, but waits inside the connector worker
// instead of inside the Feishu HTTP callback.
func (a api) executeConnectorImageGeneration(ctx context.Context, userID string, req generation.CreateRequest) (generationTask, generation.CreateRequest, error) {
	identityStore, ok := a.store.(activeIdentityStore)
	if !ok {
		return generationTask{}, req, errors.New("active identity store is unavailable")
	}
	user, found, err := identityStore.GetActiveUser(userID)
	if err != nil {
		return generationTask{}, req, fmt.Errorf("load connector user: %w", err)
	}
	if !found {
		return generationTask{}, req, errUnauthorized
	}
	// Shadow users created before package capability enforcement may not have a
	// personal plan snapshot. Enterprise authorization and tenant limits still
	// apply; plan_free supplies the platform's baseline module allowlist.
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
		return generationTask{}, req, fmt.Errorf("load AI capability data: %w", err)
	}
	req.UserID = user.ID
	if strings.TrimSpace(req.Type) == "" {
		req.Type = "TEXT_TO_IMAGE"
	}
	req.ModuleCode = moduleImageGeneration
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	connectorMetadata := map[string]any{}
	for _, key := range []string{"source_type", "source_id", "operator_external_id"} {
		if value, ok := req.Params[key]; ok {
			connectorMetadata[key] = value
			delete(req.Params, key)
		}
	}
	req, err = a.prepareGenerationRequest(data, user, req)
	if err != nil {
		return generationTask{}, req, fmt.Errorf("authorize connector generation: %w", err)
	}
	for key, value := range connectorMetadata {
		req.Params[key] = value
	}
	service := a.generationService
	if a.connectorGenerationService != nil {
		service = *a.connectorGenerationService
	} else if routeService, routeOK, routeErr := a.generationServiceForUserRoute(user, req.Model); routeErr != nil {
		return generationTask{}, req, routeErr
	} else if routeOK {
		service = routeService
	} else if providerID := selectedGenerationProvider(req.Params); providerID != "" {
		service, err = a.generationServiceForProvider(providerID, req)
		if err != nil {
			return generationTask{}, req, err
		}
	} else if configuredService, configured, configuredErr := a.generationServiceForConfiguredModel(req.Model); configuredErr != nil {
		return generationTask{}, req, configuredErr
	} else if configured {
		service = configuredService
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
	prepared, err := service.PrepareImageTask(ctx, cloneGenerationCreateRequest(req))
	if err != nil {
		prepared, err = a.prepareImageTaskWithFallback(ctx, req, err)
	}
	if err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return task, req, fmt.Errorf("generate connector image: %w", err)
	}
	prepared, storedFiles, err := a.persistGeneratedImages(ctx, task.ID, prepared)
	if err != nil {
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return task, prepared, fmt.Errorf("persist connector image: %w", err)
	}
	completed, err := a.store.CompleteGenerationTask(task.ID, prepared)
	if err != nil {
		a.cleanupGeneratedFiles(storedFiles)
		_, _ = a.store.FailGenerationTask(task.ID, generationErrorMessage(err))
		return task, prepared, fmt.Errorf("complete connector generation: %w", err)
	}
	return completed, prepared, nil
}
