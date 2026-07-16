package ppt

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusSuccess    = "success"
	StatusFailed     = "failed"
)

var (
	ErrInvalidPrompt  = errors.New("ppt prompt is required")
	ErrTaskNotFound   = errors.New("ppt task not found")
	ErrVisualNotFound = errors.New("ppt visual history item not found")
	ErrConcurrency    = errors.New("ppt generation concurrency limit reached")
)

type GenerateRequest struct {
	UserID                string   `json:"-"`
	Prompt                string   `json:"prompt"`
	SlideCount            int      `json:"slideCount"`
	Language              string   `json:"language"`
	Tone                  string   `json:"tone"`
	TextContent           string   `json:"textContent"`
	Audience              string   `json:"audience"`
	Scenario              string   `json:"scenario"`
	GenerationAspectRatio string   `json:"generationAspectRatio"`
	Theme                 string   `json:"theme"`
	AutoThemeEnabled      bool     `json:"autoThemeEnabled"`
	EnableWebSearch       bool     `json:"enableWebSearch"`
	ImageSource           string   `json:"imageSource"`
	TextModel             string   `json:"textModel"`
	ImageModel            string   `json:"imageModel"`
	ImageStyle            string   `json:"imageStyle,omitempty"`
	PeopleStyle           string   `json:"peopleStyle,omitempty"`
	ImageLighting         string   `json:"imageLighting,omitempty"`
	ImageComposition      string   `json:"imageComposition,omitempty"`
	TextInImage           bool     `json:"textInImage"`
	Outline               *Outline `json:"outline,omitempty"`
}

type GenerateResponse struct {
	TaskID string `json:"taskId"`
	Status string `json:"status"`
}

type Task struct {
	TaskID                string   `json:"taskId"`
	UserID                string   `json:"-"`
	Type                  string   `json:"type,omitempty"`
	MediaType             string   `json:"mediaType,omitempty"`
	Status                string   `json:"status"`
	Title                 string   `json:"title"`
	Prompt                string   `json:"prompt,omitempty"`
	SlideCount            int      `json:"slideCount,omitempty"`
	Language              string   `json:"language,omitempty"`
	Tone                  string   `json:"tone,omitempty"`
	TextContent           string   `json:"textContent,omitempty"`
	Audience              string   `json:"audience,omitempty"`
	Scenario              string   `json:"scenario,omitempty"`
	GenerationAspectRatio string   `json:"generationAspectRatio,omitempty"`
	Theme                 string   `json:"theme,omitempty"`
	AutoThemeEnabled      bool     `json:"autoThemeEnabled"`
	EnableWebSearch       bool     `json:"enableWebSearch,omitempty"`
	ImageSource           string   `json:"imageSource,omitempty"`
	TextModel             string   `json:"textModel,omitempty"`
	ImageModel            string   `json:"imageModel,omitempty"`
	ImageStyle            string   `json:"imageStyle,omitempty"`
	PeopleStyle           string   `json:"peopleStyle,omitempty"`
	ImageLighting         string   `json:"imageLighting,omitempty"`
	ImageComposition      string   `json:"imageComposition,omitempty"`
	TextInImage           bool     `json:"textInImage"`
	Progress              int      `json:"progress,omitempty"`
	CurrentPage           int      `json:"currentPage,omitempty"`
	Outline               *Outline `json:"outline,omitempty"`
	Slides                []Slide  `json:"slides,omitempty"`
	PPTURL                string   `json:"pptUrl"`
	PDFURL                string   `json:"pdfUrl"`
	ErrorMessage          string   `json:"errorMessage"`
	CreatedAt             string   `json:"createdAt,omitempty"`
	UpdatedAt             string   `json:"updatedAt,omitempty"`
}

type Outline struct {
	Title     string         `json:"title"`
	Slides    []OutlineSlide `json:"slides"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

type OutlineSlide struct {
	Page         int      `json:"page"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	BulletPoints []string `json:"bulletPoints"`
	Layout       string   `json:"layout,omitempty"`
	SlideType    string   `json:"slideType,omitempty"`
}

type Slide struct {
	ID               string        `json:"id"`
	Page             int           `json:"page"`
	Title            string        `json:"title"`
	Content          string        `json:"content"`
	BulletPoints     []string      `json:"bulletPoints"`
	ImageURL         string        `json:"imageUrl,omitempty"`
	VisualStorageRef string        `json:"visualStorageRef,omitempty"`
	Layout           string        `json:"layout"`
	SpeakerNotes     string        `json:"speakerNotes,omitempty"`
	SlideType        string        `json:"slideType,omitempty"`
	VisualPlan       *VisualPlan   `json:"visualPlan,omitempty"`
	VisualHistory    []VisualAsset `json:"visualHistory,omitempty"`
	VisualTaskID     string        `json:"visualTaskId,omitempty"`
	VisualModelName  string        `json:"visualModelName,omitempty"`
	VisualCreatedAt  string        `json:"visualCreatedAt,omitempty"`
	VisualStatus     string        `json:"visualStatus,omitempty"`
	VisualError      string        `json:"visualError,omitempty"`
}

type Service struct {
	mu              sync.Mutex
	tasks           map[string]Task
	path            string
	db              *sql.DB
	postgresReadyMu sync.Mutex
	postgresReady   bool
}

type persistedState struct {
	Tasks []persistedTask `json:"tasks"`
}

type persistedTask struct {
	Task
	UserID string `json:"userId"`
}

func NewService() *Service {
	return &Service{tasks: map[string]Task{}}
}

func NewPersistentService(path string) *Service {
	s := &Service{tasks: map[string]Task{}, path: strings.TrimSpace(path)}
	s.load()
	return s
}

func NewPostgresService(db *sql.DB, legacyPath string) *Service {
	if db == nil {
		return NewPersistentService(legacyPath)
	}
	return &Service{tasks: map[string]Task{}, path: strings.TrimSpace(legacyPath), db: db}
}

func (s *Service) Generate(req GenerateRequest) (GenerateResponse, error) {
	return s.GenerateWithConcurrency(req, 0, 0)
}

func (s *Service) GenerateWithConcurrency(req GenerateRequest, externalActive int, limit int) (GenerateResponse, error) {
	req = normalizeRequest(req)
	if req.Prompt == "" {
		return GenerateResponse{}, ErrInvalidPrompt
	}
	if s.db != nil {
		return s.generatePostgres(req, externalActive, limit)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit > 0 {
		active := externalActive
		for _, existing := range s.tasks {
			existing = s.materializeLocked(existing)
			if existing.UserID == req.UserID && (existing.Status == StatusPending || existing.Status == StatusProcessing) {
				active++
			}
		}
		if active >= limit {
			return GenerateResponse{}, fmt.Errorf("%w: active %d, limit %d", ErrConcurrency, active, limit)
		}
	}
	task := taskFromGenerateRequest(req)
	taskID := task.TaskID

	s.tasks[taskID] = task
	if err := s.saveLocked(); err != nil {
		delete(s.tasks, taskID)
		return GenerateResponse{}, err
	}
	return GenerateResponse{TaskID: taskID, Status: task.Status}, nil
}

func (s *Service) GetTask(userID string, taskID string) (Task, error) {
	if s.db != nil {
		return s.getTaskPostgres(userID, taskID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	task = s.materializeLocked(task)
	if err := s.saveLocked(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Service) History(userID string) []Task {
	items, _ := s.HistoryWithError(userID)
	return items
}

func (s *Service) HistoryWithError(userID string) ([]Task, error) {
	if s.db != nil {
		return s.historyPostgres(userID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.UserID != userID {
			continue
		}
		items = append(items, s.materializeLocked(task))
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items, nil
}

func (s *Service) Delete(userID string, taskID string) error {
	if s.db != nil {
		return s.deletePostgres(userID, taskID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return ErrTaskNotFound
	}
	delete(s.tasks, taskID)
	return s.saveLocked()
}

func (s *Service) UpdateSlideImage(userID string, taskID string, slideID string, imageURL string) (Task, error) {
	if s.db != nil {
		return s.updateSlideImagePostgres(userID, taskID, slideID, imageURL)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	previous := task
	task = cloneTask(task)
	slideID = strings.TrimSpace(slideID)
	imageURL = strings.TrimSpace(imageURL)
	updated := false
	for i := range task.Slides {
		if task.Slides[i].ID == slideID {
			if old := strings.TrimSpace(task.Slides[i].ImageURL); old != "" && old != imageURL {
				task.Slides[i].VisualHistory = append(task.Slides[i].VisualHistory, VisualAsset{URL: old, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
			}
			task.Slides[i].ImageURL = imageURL
			task.Slides[i].VisualStatus = "success"
			task.Slides[i].VisualError = ""
			updated = true
			break
		}
	}
	if !updated {
		return Task{}, ErrTaskNotFound
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.tasks[task.TaskID] = task
	if err := s.saveLocked(); err != nil {
		s.tasks[task.TaskID] = previous
		return Task{}, err
	}
	return task, nil
}

func (s *Service) UpdateSlideVisualPlan(userID, taskID, slideID string, plan VisualPlan, visualTaskID, status, errorMessage string) (Task, error) {
	if s.db != nil {
		return s.updateSlideVisualPlanPostgres(userID, taskID, slideID, plan, visualTaskID, status, errorMessage)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	previous := task
	task = cloneTask(task)
	found := false
	for i := range task.Slides {
		if task.Slides[i].ID != slideID {
			continue
		}
		task.Slides[i].VisualPlan = &plan
		if visualTaskID = strings.TrimSpace(visualTaskID); visualTaskID != "" {
			task.Slides[i].VisualTaskID = visualTaskID
		}
		task.Slides[i].VisualStatus = strings.TrimSpace(status)
		task.Slides[i].VisualError = strings.TrimSpace(errorMessage)
		found = true
		break
	}
	if !found {
		return Task{}, ErrTaskNotFound
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.tasks[task.TaskID] = task
	if err := s.saveLocked(); err != nil {
		s.tasks[task.TaskID] = previous
		return Task{}, err
	}
	return task, nil
}

func (s *Service) DisableSlideVisual(userID, taskID, slideID string, plan VisualPlan) (Task, error) {
	if s.db != nil {
		return s.disableSlideVisualPostgres(userID, taskID, slideID, plan)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	previous := task
	task = cloneTask(task)
	if err := disableSlideVisual(&task, slideID, plan); err != nil {
		return Task{}, err
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.tasks[task.TaskID] = task
	if err := s.saveLocked(); err != nil {
		s.tasks[task.TaskID] = previous
		return Task{}, err
	}
	return task, nil
}

func disableSlideVisual(task *Task, slideID string, plan VisualPlan) error {
	slideID = strings.TrimSpace(slideID)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range task.Slides {
		slide := &task.Slides[i]
		if slide.ID != slideID {
			continue
		}
		if currentURL := strings.TrimSpace(slide.ImageURL); currentURL != "" {
			slide.VisualHistory = append(slide.VisualHistory, VisualAsset{
				URL: currentURL, TaskID: strings.TrimSpace(slide.VisualTaskID),
				ModelName: firstNonEmptyVisual(slide.VisualModelName, task.ImageModel),
				CreatedAt: firstNonEmptyVisual(slide.VisualCreatedAt, now),
			})
		}
		plan.ImageRequired = false
		plan.TextInImage = false
		plan.Objects = append([]string(nil), plan.Objects...)
		slide.VisualPlan = &plan
		slide.ImageURL = ""
		slide.VisualTaskID = ""
		slide.VisualModelName = ""
		slide.VisualCreatedAt = ""
		slide.VisualStatus = "success"
		slide.VisualError = ""
		return nil
	}
	return ErrTaskNotFound
}

func (s *Service) CompleteSlideVisual(userID, taskID, slideID string, plan VisualPlan, asset VisualAsset) (Task, error) {
	if s.db != nil {
		return s.completeSlideVisualPostgres(userID, taskID, slideID, plan, asset)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	previous := task
	task = cloneTask(task)
	if err := completeSlideVisual(&task, slideID, plan, asset); err != nil {
		return Task{}, err
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.tasks[task.TaskID] = task
	if err := s.saveLocked(); err != nil {
		s.tasks[task.TaskID] = previous
		return Task{}, err
	}
	return task, nil
}

func completeSlideVisual(task *Task, slideID string, plan VisualPlan, asset VisualAsset) error {
	slideID, asset.URL = strings.TrimSpace(slideID), strings.TrimSpace(asset.URL)
	if asset.URL == "" {
		return ErrVisualNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(asset.CreatedAt) == "" {
		asset.CreatedAt = now
	}
	asset.TaskID = strings.TrimSpace(asset.TaskID)
	asset.ModelName = strings.TrimSpace(asset.ModelName)
	for i := range task.Slides {
		slide := &task.Slides[i]
		if slide.ID != slideID {
			continue
		}
		if oldURL := strings.TrimSpace(slide.ImageURL); oldURL != "" && oldURL != asset.URL {
			createdAt := strings.TrimSpace(slide.VisualCreatedAt)
			if createdAt == "" {
				createdAt = now
			}
			modelName := strings.TrimSpace(slide.VisualModelName)
			if modelName == "" {
				modelName = strings.TrimSpace(task.ImageModel)
			}
			slide.VisualHistory = append(slide.VisualHistory, VisualAsset{
				URL: oldURL, TaskID: strings.TrimSpace(slide.VisualTaskID), ModelName: modelName, CreatedAt: createdAt,
			})
		}
		plan.ImageRequired = true
		plan.TextInImage = false
		if strings.EqualFold(strings.TrimSpace(plan.VisualType), "none") {
			plan.VisualType = "illustration"
		}
		plan.Objects = append([]string(nil), plan.Objects...)
		slide.VisualPlan = &plan
		slide.ImageURL = asset.URL
		slide.VisualTaskID = asset.TaskID
		slide.VisualModelName = asset.ModelName
		slide.VisualCreatedAt = asset.CreatedAt
		slide.VisualStatus = "success"
		slide.VisualError = ""
		return nil
	}
	return ErrTaskNotFound
}

func (s *Service) RestoreSlideVisual(userID, taskID, slideID, createdAt, imageURL string) (Task, error) {
	if s.db != nil {
		return s.restoreSlideVisualPostgres(userID, taskID, slideID, createdAt, imageURL)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	previous := task
	task = cloneTask(task)
	if err := restoreSlideVisual(&task, slideID, createdAt, imageURL); err != nil {
		return Task{}, err
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.tasks[task.TaskID] = task
	if err := s.saveLocked(); err != nil {
		s.tasks[task.TaskID] = previous
		return Task{}, err
	}
	return task, nil
}

func restoreSlideVisual(task *Task, slideID, createdAt, imageURL string) error {
	slideID, createdAt, imageURL = strings.TrimSpace(slideID), strings.TrimSpace(createdAt), strings.TrimSpace(imageURL)
	for i := range task.Slides {
		slide := &task.Slides[i]
		if slide.ID != slideID {
			continue
		}
		for historyIndex, asset := range slide.VisualHistory {
			if strings.TrimSpace(asset.CreatedAt) != createdAt || strings.TrimSpace(asset.URL) != imageURL || imageURL == "" {
				continue
			}
			currentURL := strings.TrimSpace(slide.ImageURL)
			if currentURL != "" && currentURL != strings.TrimSpace(asset.URL) {
				modelName := firstNonEmptyVisual(slide.VisualModelName, task.ImageModel)
				createdAt := firstNonEmptyVisual(slide.VisualCreatedAt, time.Now().UTC().Format(time.RFC3339Nano))
				slide.VisualHistory = append(slide.VisualHistory, VisualAsset{
					URL: currentURL, TaskID: slide.VisualTaskID, ModelName: modelName, CreatedAt: createdAt,
				})
			}
			slide.VisualHistory = append(slide.VisualHistory[:historyIndex], slide.VisualHistory[historyIndex+1:]...)
			slide.ImageURL = strings.TrimSpace(asset.URL)
			slide.VisualTaskID = strings.TrimSpace(asset.TaskID)
			slide.VisualModelName = strings.TrimSpace(asset.ModelName)
			slide.VisualCreatedAt = strings.TrimSpace(asset.CreatedAt)
			slide.VisualStatus = "success"
			slide.VisualError = ""
			if slide.VisualPlan == nil {
				slide.VisualPlan = &VisualPlan{VisualType: "illustration", ImageRequired: true, TextInImage: false}
			} else {
				slide.VisualPlan.ImageRequired = true
				slide.VisualPlan.TextInImage = false
				if strings.EqualFold(strings.TrimSpace(slide.VisualPlan.VisualType), "none") {
					slide.VisualPlan.VisualType = "illustration"
				}
			}
			return nil
		}
		return ErrVisualNotFound
	}
	return ErrTaskNotFound
}

func (s *Service) GetSlide(userID, taskID, slideID string) (Task, Slide, error) {
	task, err := s.GetTask(userID, taskID)
	if err != nil {
		return Task{}, Slide{}, err
	}
	for _, slide := range task.Slides {
		if slide.ID == slideID {
			return task, slide, nil
		}
	}
	return Task{}, Slide{}, ErrTaskNotFound
}

func (s *Service) load() {
	if s.path == "" {
		return
	}
	raw, err := os.ReadFile(s.path)
	if err != nil || len(raw) == 0 {
		return
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		var legacy map[string]Task
		if legacyErr := json.Unmarshal(raw, &legacy); legacyErr != nil {
			return
		}
		for taskID, task := range legacy {
			if task.TaskID == "" {
				task.TaskID = taskID
			}
			if task.TaskID != "" {
				task = normalizeLegacyTask(task)
				s.tasks[task.TaskID] = task
			}
		}
		return
	}
	for _, item := range state.Tasks {
		task := item.Task
		task.UserID = item.UserID
		task = normalizeLegacyTask(task)
		if task.TaskID != "" {
			s.tasks[task.TaskID] = task
		}
	}
}

func (s *Service) saveLocked() error {
	if s.path == "" {
		return nil
	}
	tasks := make([]persistedTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, persistedTask{Task: task, UserID: task.UserID})
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Task.CreatedAt > tasks[j].Task.CreatedAt
	})
	raw, err := json.MarshalIndent(persistedState{Tasks: tasks}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o644)
}

func (s *Service) materializeLocked(task Task) Task {
	task = materializeTask(task)
	s.tasks[task.TaskID] = task
	return task
}

func materializeTask(task Task) Task {
	if task.Status == StatusSuccess || task.Status == StatusFailed {
		return task
	}
	createdAt, err := time.Parse(time.RFC3339Nano, task.CreatedAt)
	if err != nil {
		createdAt = time.Now().UTC()
	}
	elapsed := time.Since(createdAt)
	switch {
	case elapsed >= 2500*time.Millisecond:
		task.Status = StatusSuccess
		task.Progress = 100
		task.CurrentPage = task.SlideCount
	case elapsed >= 700*time.Millisecond:
		task.Status = StatusProcessing
		task.Progress = 65
		task.CurrentPage = maxInt(1, task.SlideCount/2)
	default:
		task.Status = StatusPending
		task.Progress = 20
		task.CurrentPage = 0
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return task
}

func normalizeRequest(req GenerateRequest) GenerateRequest {
	req.UserID = strings.TrimSpace(req.UserID)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.SlideCount <= 0 {
		req.SlideCount = 5
	}
	req.Language = strings.TrimSpace(req.Language)
	if req.Language != "en" {
		req.Language = "zh"
	}
	req.Tone = strings.TrimSpace(req.Tone)
	switch req.Tone {
	case "simple", "marketing", "education", "pitch":
	default:
		req.Tone = "professional"
	}
	req.TextContent = strings.TrimSpace(req.TextContent)
	switch req.TextContent {
	case "minimal", "detailed", "extensive":
	default:
		req.TextContent = "concise"
	}
	req.Audience = strings.TrimSpace(req.Audience)
	switch req.Audience {
	case "general", "business", "investor", "teacher", "student":
	default:
		req.Audience = "auto"
	}
	req.Scenario = strings.TrimSpace(req.Scenario)
	switch req.Scenario {
	case "general", "analysis-report", "teaching-training", "promotional-materials", "public-speeches":
	default:
		req.Scenario = "auto"
	}
	req.GenerationAspectRatio = strings.TrimSpace(req.GenerationAspectRatio)
	if req.GenerationAspectRatio != "16:9" {
		req.GenerationAspectRatio = "dynamic"
	}
	req.Theme = strings.TrimSpace(req.Theme)
	if req.Theme == "" {
		req.Theme = "business"
	}
	req.ImageSource = strings.TrimSpace(req.ImageSource)
	switch req.ImageSource {
	case "stock", "none":
	default:
		req.ImageSource = "ai"
	}
	req.TextModel = strings.TrimSpace(req.TextModel)
	if req.TextModel == "" {
		req.TextModel = "gpt-4o-mini"
	}
	req.ImageModel = strings.TrimSpace(req.ImageModel)
	req.ImageModel = normalizeImageModel(req.ImageModel)
	req.ImageStyle = strings.TrimSpace(req.ImageStyle)
	if req.ImageStyle == "" {
		req.ImageStyle = "modern enterprise illustration"
	}
	req.PeopleStyle = strings.TrimSpace(req.PeopleStyle)
	if req.PeopleStyle == "" {
		req.PeopleStyle = "professional natural people"
	}
	req.ImageLighting = strings.TrimSpace(req.ImageLighting)
	if req.ImageLighting == "" {
		req.ImageLighting = "soft cinematic corporate lighting"
	}
	req.ImageComposition = strings.TrimSpace(req.ImageComposition)
	if req.ImageComposition == "" {
		req.ImageComposition = "image_right"
	}
	req.TextInImage = false
	req.Outline = normalizeOutline(req.Outline, req)
	return req
}

func normalizeImageModel(value string) string {
	model := strings.TrimSpace(value)
	switch strings.ToLower(model) {
	case "":
		return "default-image"
	case "gpt-image":
		return "gpt-image-2"
	default:
		return model
	}
}

func titleFromPrompt(prompt string) string {
	title := strings.Join(strings.Fields(prompt), " ")
	if len([]rune(title)) <= 60 {
		return title
	}
	return string([]rune(title)[:60])
}

func normalizeOutline(outline *Outline, req GenerateRequest) *Outline {
	if outline == nil {
		return nil
	}
	normalized := *outline
	normalized.Title = strings.TrimSpace(normalized.Title)
	if normalized.Title == "" {
		normalized.Title = titleFromPrompt(req.Prompt)
	}
	if normalized.UpdatedAt == "" {
		normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	slides := make([]OutlineSlide, 0, len(normalized.Slides))
	for i, slide := range normalized.Slides {
		slide.Page = i + 1
		slide.Title = strings.TrimSpace(slide.Title)
		if slide.Title == "" {
			slide.Title = fmt.Sprintf("%s %d", normalized.Title, i+1)
		}
		slide.Summary = strings.TrimSpace(slide.Summary)
		points := make([]string, 0, len(slide.BulletPoints))
		for _, point := range slide.BulletPoints {
			point = strings.TrimSpace(point)
			if point != "" {
				points = append(points, point)
			}
		}
		slide.BulletPoints = points
		slide.Layout = normalizeSlideLayout(slide.Layout, i, len(normalized.Slides), req.ImageSource)
		if strings.TrimSpace(slide.SlideType) == "" {
			slide.SlideType = inferSlideType(slide.Layout, i)
		}
		slide.SlideType = NormalizeSlideType(slide.SlideType)
		slides = append(slides, slide)
	}
	normalized.Slides = slides
	return &normalized
}

func slidesFromOutline(outline *Outline, req GenerateRequest) []Slide {
	if outline == nil {
		return nil
	}
	slides := make([]Slide, 0, len(outline.Slides))
	for i, item := range outline.Slides {
		input := VisualPlannerInput{
			DeckTheme: req.Theme, SlideType: item.SlideType, SlideTitle: item.Title,
			CoreIdea: item.Summary, ContentSummary: item.Summary, Layout: item.Layout,
			ImagePosition: req.ImageComposition, ImageStyle: req.ImageStyle,
			PeopleStyle: req.PeopleStyle, ImageLighting: req.ImageLighting,
			ImageComposition: req.ImageComposition,
		}
		plan := NormalizeVisualPlan(VisualPlan{}, input)
		slides = append(slides, Slide{
			ID:           fmt.Sprintf("slide_%d", i+1),
			Page:         i + 1,
			Title:        item.Title,
			Content:      item.Summary,
			BulletPoints: append([]string{}, item.BulletPoints...),
			ImageURL:     slideImageURL(item, outline.Title, req),
			Layout:       normalizeSlideLayout(item.Layout, i, len(outline.Slides), req.ImageSource),
			SpeakerNotes: fmt.Sprintf("Page %d speaker notes can be refined after deck review.", i+1),
			SlideType:    NormalizeSlideType(item.SlideType),
			VisualPlan:   &plan,
		})
	}
	return slides
}

func inferSlideType(layout string, index int) string {
	if index == 0 || layout == "cover" {
		return "cover"
	}
	if layout == "section" {
		return "section"
	}
	if layout == "summary" {
		return "statement"
	}
	return "text_image"
}

func normalizeLegacyTask(task Task) Task {
	for i := range task.Slides {
		if strings.TrimSpace(task.Slides[i].SlideType) == "" {
			task.Slides[i].SlideType = "text_image"
		}
		task.Slides[i].SlideType = NormalizeSlideType(task.Slides[i].SlideType)
		if task.Slides[i].VisualPlan != nil {
			plan := NormalizeVisualPlan(*task.Slides[i].VisualPlan, VisualPlannerInput{
				SlideType: task.Slides[i].SlideType, SlideTitle: task.Slides[i].Title,
				CoreIdea: task.Slides[i].Content, Layout: task.Slides[i].Layout,
				ImagePosition: task.ImageComposition, ImageStyle: task.ImageStyle,
				PeopleStyle: task.PeopleStyle, ImageLighting: task.ImageLighting,
			})
			task.Slides[i].VisualPlan = &plan
		}
	}
	return task
}

func cloneTask(task Task) Task {
	cloned := task
	cloned.Slides = append([]Slide(nil), task.Slides...)
	for i := range cloned.Slides {
		cloned.Slides[i].BulletPoints = append([]string(nil), cloned.Slides[i].BulletPoints...)
		cloned.Slides[i].VisualHistory = append([]VisualAsset(nil), cloned.Slides[i].VisualHistory...)
		if cloned.Slides[i].VisualPlan != nil {
			plan := *cloned.Slides[i].VisualPlan
			plan.Objects = append([]string(nil), plan.Objects...)
			cloned.Slides[i].VisualPlan = &plan
		}
	}
	return cloned
}

func slideImageURL(slide OutlineSlide, deckTitle string, req GenerateRequest) string {
	if strings.TrimSpace(req.ImageSource) == "none" || !imageAllowedForSlideType(NormalizeSlideType(slide.SlideType)) {
		return ""
	}
	_ = deckTitle
	accent := []string{"#0ea5e9", "#22c55e", "#f59e0b", "#ef4444", "#8b5cf6"}[(slide.Page-1)%5]
	bg := []string{"#ecfeff", "#f0fdf4", "#fffbeb", "#fef2f2", "#f5f3ff"}[(slide.Page-1)%5]
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720">
<rect width="1280" height="720" rx="42" fill="%s"/>
<circle cx="1028" cy="164" r="118" fill="%s" opacity=".15"/>
<circle cx="178" cy="584" r="142" fill="%s" opacity=".12"/>
<rect x="96" y="102" width="1088" height="516" rx="34" fill="#ffffff" opacity=".86"/>
<path d="M268 406c42-84 82-126 120-126 56 0 72 88 126 88 42 0 68-48 102-122 22 60 44 120 66 180 34-84 66-126 96-126 44 0 64 54 104 54 36 0 64-42 94-126" fill="none" stroke="%s" stroke-width="22" stroke-linecap="round" stroke-linejoin="round"/>
<rect x="198" y="168" width="218" height="120" rx="28" fill="%s" opacity=".14"/>
<path d="M254 216h106M254 254h74" stroke="%s" stroke-width="18" stroke-linecap="round"/>
<circle cx="912" cy="214" r="54" fill="%s" opacity=".18"/>
<path d="M892 214l22 22 42-58" fill="none" stroke="%s" stroke-width="16" stroke-linecap="round" stroke-linejoin="round"/>
<circle cx="640" cy="540" r="18" fill="%s" opacity=".35"/>
</svg>`, bg, accent, accent, accent, accent, accent, accent, accent, accent)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func normalizeSlideLayout(layout string, index int, total int, imageSource string) string {
	switch strings.TrimSpace(layout) {
	case "cover", "section", "content", "imageText", "summary":
		return strings.TrimSpace(layout)
	}
	if index == 0 {
		return "cover"
	}
	if total > 1 && index == total-1 {
		return "summary"
	}
	if strings.TrimSpace(imageSource) == "none" {
		return "content"
	}
	return "imageText"
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
