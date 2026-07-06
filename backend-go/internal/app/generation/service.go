package generation

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidPrompt = errors.New("prompt is required")
var ErrUnsupportedCapability = errors.New("unsupported generation capability")

type CreateRequest struct {
	Type            string           `json:"type"`
	ModuleCode      string           `json:"module_code,omitempty"`
	ModuleCodeCamel string           `json:"moduleCode,omitempty"`
	UserID          string           `json:"-"`
	Prompt          string           `json:"prompt"`
	Model           string           `json:"model"`
	Params          map[string]any   `json:"params"`
	GeneratedImages []GeneratedImage `json:"-"`
	VideoTask       any              `json:"-"`
	ChatResponse    any              `json:"-"`
}

type GeneratedImage struct {
	URL          string
	ThumbnailURL string
	ContentType  string
	Width        int
	Height       int
	Source       string
}

type ImageProvider interface {
	DefaultModel() string
	Generate(context.Context, CreateRequest) ([]GeneratedImage, error)
}

type VideoProvider interface {
	DefaultModel() string
	Create(context.Context, CreateRequest) (any, error)
}

type ChatProvider interface {
	DefaultModel() string
	Chat(context.Context, CreateRequest) (any, error)
}

type ImageDecorator interface {
	Decorate(context.Context, []GeneratedImage) []GeneratedImage
}

type TaskCreator func(CreateRequest) (any, error)

type Service struct {
	imageProvider  ImageProvider
	videoProvider  VideoProvider
	chatProvider   ChatProvider
	imageDecorator ImageDecorator
	createTask     TaskCreator
}

type ServiceOptions struct {
	ImageProvider  ImageProvider
	VideoProvider  VideoProvider
	ChatProvider   ChatProvider
	ImageDecorator ImageDecorator
	CreateTask     TaskCreator
}

func NewService(provider ImageProvider, decorator ImageDecorator, createTask TaskCreator) Service {
	return NewServiceWithOptions(ServiceOptions{ImageProvider: provider, ImageDecorator: decorator, CreateTask: createTask})
}

func NewServiceWithOptions(opts ServiceOptions) Service {
	return Service{
		imageProvider:  opts.ImageProvider,
		videoProvider:  opts.VideoProvider,
		chatProvider:   opts.ChatProvider,
		imageDecorator: opts.ImageDecorator,
		createTask:     opts.CreateTask,
	}
}

func (s Service) Create(ctx context.Context, req CreateRequest) (any, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return nil, ErrInvalidPrompt
	}
	if req.Type == "" {
		req.Type = "TEXT_TO_IMAGE"
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}

	switch capabilityForType(req.Type) {
	case "image":
		return s.createImageTask(ctx, req)
	case "video":
		return s.createVideoTask(ctx, req)
	case "chat":
		return s.createChatTask(ctx, req)
	default:
		return nil, ErrUnsupportedCapability
	}
}

func (s Service) createImageTask(ctx context.Context, req CreateRequest) (any, error) {
	prepared, err := s.PrepareImageTask(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.createTask(prepared)
}

func (s Service) PrepareImageTask(ctx context.Context, req CreateRequest) (CreateRequest, error) {
	if req.Model == "" && s.imageProvider != nil {
		req.Model = s.imageProvider.DefaultModel()
	}
	if req.Model == "" {
		req.Model = "mock-standard"
	}
	if req.Model != "mock-standard" && s.imageProvider != nil {
		images, err := s.imageProvider.Generate(ctx, req)
		if err != nil {
			return CreateRequest{}, err
		}
		if s.imageDecorator != nil {
			images = s.imageDecorator.Decorate(ctx, images)
		}
		req.GeneratedImages = images
	}
	return req, nil
}

func (s Service) createVideoTask(ctx context.Context, req CreateRequest) (any, error) {
	if s.videoProvider == nil {
		return nil, ErrUnsupportedCapability
	}
	prepared, err := s.PrepareVideoTask(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.createTask(prepared)
}

func (s Service) PrepareVideoTask(ctx context.Context, req CreateRequest) (CreateRequest, error) {
	if s.videoProvider == nil {
		return CreateRequest{}, ErrUnsupportedCapability
	}
	if req.Model == "" {
		req.Model = s.videoProvider.DefaultModel()
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	task, err := s.videoProvider.Create(ctx, req)
	if err != nil {
		return CreateRequest{}, err
	}
	req.VideoTask = task
	req.Params["providerTask"] = task
	return req, nil
}

func (s Service) createChatTask(ctx context.Context, req CreateRequest) (any, error) {
	if s.chatProvider == nil {
		return nil, ErrUnsupportedCapability
	}
	if req.Model == "" {
		req.Model = s.chatProvider.DefaultModel()
	}
	response, err := s.chatProvider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	req.ChatResponse = response
	req.Params["chatResponse"] = response
	return s.createTask(req)
}

func capabilityForType(taskType string) string {
	switch strings.ToUpper(strings.TrimSpace(taskType)) {
	case "TEXT_TO_IMAGE", "IMAGE_TO_IMAGE":
		return "image"
	case "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO":
		return "video"
	case "CHAT", "CHAT_COMPLETION", "AGENT_CHAT":
		return "chat"
	default:
		return ""
	}
}
