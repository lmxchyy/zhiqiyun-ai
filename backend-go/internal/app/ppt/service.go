package ppt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
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
	ErrInvalidPrompt = errors.New("ppt prompt is required")
	ErrTaskNotFound  = errors.New("ppt task not found")
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
}

type Slide struct {
	ID           string   `json:"id"`
	Page         int      `json:"page"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	BulletPoints []string `json:"bulletPoints"`
	ImageURL     string   `json:"imageUrl,omitempty"`
	Layout       string   `json:"layout"`
	SpeakerNotes string   `json:"speakerNotes,omitempty"`
}

type Service struct {
	mu    sync.Mutex
	tasks map[string]Task
	path  string
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

func (s *Service) Generate(req GenerateRequest) (GenerateResponse, error) {
	req = normalizeRequest(req)
	if req.Prompt == "" {
		return GenerateResponse{}, ErrInvalidPrompt
	}
	now := time.Now().UTC()
	taskID := fmt.Sprintf("ppt_%d", now.UnixNano())
	task := Task{
		TaskID:                taskID,
		UserID:                req.UserID,
		Type:                  "ppt",
		MediaType:             "ppt",
		Status:                StatusPending,
		Title:                 titleFromPrompt(req.Prompt),
		Prompt:                req.Prompt,
		SlideCount:            req.SlideCount,
		Language:              req.Language,
		Tone:                  req.Tone,
		TextContent:           req.TextContent,
		Audience:              req.Audience,
		Scenario:              req.Scenario,
		GenerationAspectRatio: req.GenerationAspectRatio,
		Theme:                 req.Theme,
		AutoThemeEnabled:      req.AutoThemeEnabled,
		EnableWebSearch:       req.EnableWebSearch,
		ImageSource:           req.ImageSource,
		TextModel:             req.TextModel,
		ImageModel:            req.ImageModel,
		Progress:              0,
		CurrentPage:           0,
		Outline:               req.Outline,
		Slides:                slidesFromOutline(req.Outline, req),
		PPTURL:                "",
		PDFURL:                "",
		ErrorMessage:          "",
		CreatedAt:             now.Format(time.RFC3339Nano),
		UpdatedAt:             now.Format(time.RFC3339Nano),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = task
	if err := s.saveLocked(); err != nil {
		delete(s.tasks, taskID)
		return GenerateResponse{}, err
	}
	return GenerateResponse{TaskID: taskID, Status: task.Status}, nil
}

func (s *Service) GetTask(userID string, taskID string) (Task, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		if task.UserID != userID {
			continue
		}
		items = append(items, s.materializeLocked(task))
	}
	_ = s.saveLocked()
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items
}

func (s *Service) Delete(userID string, taskID string) error {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok || task.UserID != userID {
		return Task{}, ErrTaskNotFound
	}
	slideID = strings.TrimSpace(slideID)
	imageURL = strings.TrimSpace(imageURL)
	updated := false
	for i := range task.Slides {
		if task.Slides[i].ID == slideID {
			task.Slides[i].ImageURL = imageURL
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
		return Task{}, err
	}
	return task, nil
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
				s.tasks[task.TaskID] = task
			}
		}
		return
	}
	for _, item := range state.Tasks {
		task := item.Task
		task.UserID = item.UserID
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
	s.tasks[task.TaskID] = task
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
		slides = append(slides, Slide{
			ID:           fmt.Sprintf("slide_%d", i+1),
			Page:         i + 1,
			Title:        item.Title,
			Content:      item.Summary,
			BulletPoints: append([]string{}, item.BulletPoints...),
			ImageURL:     slideImageURL(item, outline.Title, req),
			Layout:       normalizeSlideLayout(item.Layout, i, len(outline.Slides), req.ImageSource),
			SpeakerNotes: fmt.Sprintf("Page %d speaker notes can be refined after deck review.", i+1),
		})
	}
	return slides
}

func slideImageURL(slide OutlineSlide, deckTitle string, req GenerateRequest) string {
	if strings.TrimSpace(req.ImageSource) == "none" {
		return ""
	}
	title := strings.TrimSpace(slide.Title)
	if title == "" {
		title = deckTitle
	}
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
<text x="640" y="540" text-anchor="middle" font-family="Arial, Microsoft YaHei, sans-serif" font-size="52" font-weight="800" fill="#0f172a">%s</text>
</svg>`, bg, accent, accent, accent, accent, accent, accent, accent, html.EscapeString(title))
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
