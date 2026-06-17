package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type platformStore interface {
	ListGenerationTasks() ([]generationTask, error)
	CreateGenerationTask(createGenerationTaskRequest) (generationTask, error)
	ListAssets() ([]asset, error)
	DeleteAsset(id string) error
}

type api struct {
	store platformStore
}

func newAPI(store platformStore) api {
	return api{store: store}
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
		req.Model = "mock-standard"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
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
