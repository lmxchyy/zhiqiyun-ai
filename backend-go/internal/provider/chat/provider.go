package chat

import (
	"context"
	"errors"

	"xianzhi-ai/backend-go/internal/app/generation"
)

var ErrNoProvider = errors.New("no chat provider available")

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	ProviderCode string
	Model        string
	Message      Message
	Usage        map[string]any
	Metadata     map[string]any
}

type Chunk struct {
	Delta    string
	Done     bool
	Usage    map[string]any
	Metadata map[string]any
}

type Provider interface {
	Code() string
	DefaultModel() string
	Supports(generation.CreateRequest) bool
	Chat(context.Context, generation.CreateRequest) (Response, error)
	StreamChat(context.Context, generation.CreateRequest) (<-chan Chunk, error)
}
