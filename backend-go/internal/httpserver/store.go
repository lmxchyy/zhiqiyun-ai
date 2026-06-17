package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var errAssetNotFound = errors.New("asset not found")

type jsonStore struct {
	path string
	mu   sync.Mutex
}

func newJSONStore(path string) *jsonStore {
	return &jsonStore{path: path}
}

func (s *jsonStore) ListGenerationTasks() ([]generationTask, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	return data.GenerationTasks, nil
}

func (s *jsonStore) ListAssets() ([]asset, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	return data.Assets, nil
}

func (s *jsonStore) CreateGenerationTask(req createGenerationTaskRequest) (generationTask, error) {
	var task generationTask
	if err := s.update(func(data *platformData) error {
		taskID := nextID(data.Counters, "task")
		assetID := nextID(data.Counters, "asset")
		now := time.Now().UTC().Format(time.RFC3339Nano)
		task = generationTask{
			ID:               taskID,
			UserID:           "user_000002",
			Type:             req.Type,
			Prompt:           req.Prompt,
			Params:           req.Params,
			Model:            req.Model,
			Status:           "SUCCEEDED",
			Progress:         100,
			PointCost:        10,
			ResultIDs:        []string{assetID},
			CreatedAt:        now,
			UpdatedAt:        now,
			WorkerFinishedAt: now,
		}
		asset := asset{
			ID:        assetID,
			UserID:    "user_000002",
			TaskID:    taskID,
			Name:      "TEXT_TO_IMAGE-" + taskID,
			MediaType: "image",
			URL:       promptPreviewImage(req.Prompt),
			Favorite:  false,
			Metadata: map[string]any{
				"prompt":      req.Prompt,
				"model":       req.Model,
				"contentType": "image/svg+xml",
				"source":      "local-prompt-preview",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		data.GenerationTasks = append(data.GenerationTasks, task)
		data.Assets = append(data.Assets, asset)
		return nil
	}); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func (s *jsonStore) DeleteAsset(id string) error {
	return s.update(func(data *platformData) error {
		next := data.Assets[:0]
		deleted := false
		for _, item := range data.Assets {
			if item.ID == id {
				deleted = true
				continue
			}
			next = append(next, item)
		}
		if !deleted {
			return fmt.Errorf("%w: %s", errAssetNotFound, id)
		}
		data.Assets = next
		for i := range data.GenerationTasks {
			resultIDs := data.GenerationTasks[i].ResultIDs[:0]
			for _, resultID := range data.GenerationTasks[i].ResultIDs {
				if resultID != id {
					resultIDs = append(resultIDs, resultID)
				}
			}
			data.GenerationTasks[i].ResultIDs = resultIDs
		}
		return nil
	})
}

func (s *jsonStore) load() (platformData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *jsonStore) save(data platformData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(data)
}

func (s *jsonStore) update(mutator func(*platformData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return err
	}
	if err := mutator(&data); err != nil {
		return err
	}
	return s.saveLocked(data)
}

func (s *jsonStore) loadLocked() (platformData, error) {
	var data platformData
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			data.Counters = map[string]int{}
			return data, nil
		}
		return data, err
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, err
	}
	if data.Counters == nil {
		data.Counters = map[string]int{}
	}
	return data, nil
}

func (s *jsonStore) saveLocked(data platformData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return writeFileAtomically(s.path, append(raw, '\n'))
}

func writeFileAtomically(path string, content []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return errors.Join(err, removeErr)
		}
		if replaceErr := os.Rename(tmpPath, path); replaceErr != nil {
			return errors.Join(err, replaceErr)
		}
	}
	return nil
}

func nextID(counters map[string]int, name string) string {
	counters[name]++
	return fmt.Sprintf("%s_%06d", name, counters[name])
}
