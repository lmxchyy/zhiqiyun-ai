package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"xianzhi-ai/backend-go/internal/config"
)

type platformStore interface {
	ListGenerationTasks() ([]generationTask, error)
	CreateGenerationTask(createGenerationTaskRequest) (generationTask, error)
	ListAssets() ([]asset, error)
	DeleteAsset(id string) error
	PointAccount() (pointAccount, error)
	AdminData() (adminPlatformData, error)
	CreateAdminCustomer(adminCustomerMutation) (adminUser, error)
	UpdateAdminCustomer(string, adminCustomerMutation) (adminUser, error)
	UpdateAdminChannelAgent(string, adminChannelMutation) (adminChannelAgent, error)
	UpdateAdminProduct(string, adminProductMutation) (adminProduct, error)
	UpdateAdminPlan(string, adminPlanMutation) (adminPlan, error)
	CreateAdminOrder(adminOrderMutation) (adminOrder, error)
	MarkAdminOrderPaid(string) (adminOrder, error)
	RenewAdminOrder(string) (adminOrder, error)
	UpdateAdminDeliveryProject(string, adminDeliveryMutation) (map[string]any, error)
	UpdateAdminSystemSettings(adminSystemMutation) (adminSystemSettings, error)
	CreateAdminAPIChannel(adminAPIChannelMutation) (adminAPIChannel, error)
	UpdateAdminAPIChannel(string, adminAPIChannelMutation) (adminAPIChannel, error)
	TestAdminAPIChannel(string) (map[string]any, error)
	UpdateAdminAPIModel(string, adminAPIModelMutation) (adminAPIModel, error)
	CreateAdminAPIKey(adminAPIKeyMutation) (adminAPIKey, error)
	UpdateAdminAPIKey(string, adminAPIKeyMutation) (adminAPIKey, error)
	UpdateAdminCustomerGroup(string, adminCustomerGroupMutation) (adminCustomerGroup, error)
	CreateAdminCommission(adminCommissionMutation) (adminCommission, error)
	CreateAdminWithdrawal(adminWithdrawalMutation) (adminWithdrawal, error)
	ReviewAdminWithdrawal(string, string) (adminWithdrawal, error)
}

type api struct {
	store         platformStore
	imageProvider imageProvider
}

func newAPI(store platformStore, cfg config.Config) api {
	return api{store: store, imageProvider: newImageProvider(cfg)}
}

func (a api) listGenerationTasks(w http.ResponseWriter, _ *http.Request) {
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, tasks)
}

func (a api) createGenerationTask(w http.ResponseWriter, r *http.Request) {
	var req createGenerationTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, errors.New("prompt is required"))
		return
	}
	if req.Type == "" {
		req.Type = "TEXT_TO_IMAGE"
	}
	if req.Model == "" {
		req.Model = a.imageProvider.model
	}
	if req.Model == "" {
		req.Model = "mock-standard"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if req.Model != "mock-standard" {
		images, err := a.imageProvider.generate(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		req.GeneratedImages = images
	}

	task, err := a.store.CreateGenerationTask(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, task)
}

func (a api) listAssets(w http.ResponseWriter, _ *http.Request) {
	assets, err := a.store.ListAssets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, assets)
}

func (a api) pointAccount(w http.ResponseWriter, _ *http.Request) {
	account, err := a.store.PointAccount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{
		"account":      account,
		"transactions": []any{},
	})
}

func (a api) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.store.DeleteAsset(id); err != nil {
		if errors.Is(err, errAssetNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}
