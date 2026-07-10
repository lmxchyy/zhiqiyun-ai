package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type AgentService struct {
	core  *Service
	repo  AgentRepository
	now   func() time.Time
	newID func(string) string
}

func NewAgentService(core *Service, repo AgentRepository) *AgentService {
	return &AgentService{core: core, repo: repo, now: func() time.Time { return time.Now().UTC() }, newID: newID}
}

type CreateAgentInput struct {
	Name         string
	Description  string
	ModelName    string
	SystemPrompt string
	Status       string
	Config       map[string]any
}

func (s *AgentService) CreateAgent(ctx context.Context, access AccessContext, input CreateAgentInput) (Agent, error) {
	if s == nil || s.repo == nil || access.TenantID == "" || access.UserID == "" {
		return Agent{}, fmt.Errorf("agent service is not configured: %w", ErrValidation)
	}
	if access.HasRole("GUEST") {
		return Agent{}, ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 120 {
		return Agent{}, fmt.Errorf("agent name must be 1-120 characters: %w", ErrValidation)
	}
	status := strings.ToUpper(strings.TrimSpace(input.Status))
	if status == "" {
		status = "ACTIVE"
	}
	if status != "DRAFT" && status != "ACTIVE" && status != "DISABLED" {
		return Agent{}, fmt.Errorf("unsupported agent status: %w", ErrValidation)
	}
	now := s.now()
	return s.repo.CreateAgent(ctx, access, Agent{
		ID:           s.newID("agent"),
		TenantID:     access.TenantID,
		OwnerUserID:  access.UserID,
		Name:         input.Name,
		Description:  strings.TrimSpace(input.Description),
		ModelName:    strings.TrimSpace(input.ModelName),
		SystemPrompt: strings.TrimSpace(input.SystemPrompt),
		Status:       status,
		Version:      1,
		Config:       cloneMap(input.Config),
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func (s *AgentService) ListAgents(ctx context.Context, access AccessContext, options ListOptions) ([]Agent, string, error) {
	options.Limit = normalizedLimit(options.Limit, 20, 100)
	return s.repo.ListAgents(ctx, access, options)
}

func (s *AgentService) GetAgent(ctx context.Context, access AccessContext, id string) (Agent, error) {
	if strings.TrimSpace(id) == "" {
		return Agent{}, fmt.Errorf("agent id is required: %w", ErrValidation)
	}
	return s.repo.GetAgent(ctx, access, strings.TrimSpace(id))
}

type BindKnowledgeBaseInput struct {
	KnowledgeBaseID    string         `json:"knowledgeBaseId"`
	RetrievalProfileID string         `json:"retrievalProfileId"`
	Priority           int            `json:"priority"`
	Weight             float64        `json:"weight"`
	Enabled            *bool          `json:"enabled"`
	RetrievalOverrides map[string]any `json:"retrievalOverrides"`
}

func (s *AgentService) ReplaceBindings(ctx context.Context, access AccessContext, agentID string, inputs []BindKnowledgeBaseInput) ([]AgentKnowledgeBinding, error) {
	if _, err := s.GetAgent(ctx, access, agentID); err != nil {
		return nil, err
	}
	if len(inputs) > 50 {
		return nil, fmt.Errorf("an agent can bind at most 50 knowledge bases: %w", ErrValidation)
	}
	seen := map[string]bool{}
	bindings := make([]AgentKnowledgeBinding, 0, len(inputs))
	for _, input := range inputs {
		input.KnowledgeBaseID = strings.TrimSpace(input.KnowledgeBaseID)
		if input.KnowledgeBaseID == "" || seen[input.KnowledgeBaseID] {
			return nil, fmt.Errorf("knowledge base bindings must be unique: %w", ErrValidation)
		}
		seen[input.KnowledgeBaseID] = true
		if _, err := s.core.GetKnowledgeBase(ctx, access, input.KnowledgeBaseID); err != nil {
			return nil, err
		}
		weight := input.Weight
		if weight <= 0 {
			weight = 1
		}
		if weight > 10 {
			return nil, fmt.Errorf("knowledge base weight cannot exceed 10: %w", ErrValidation)
		}
		priority := input.Priority
		if priority == 0 {
			priority = 100
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		bindings = append(bindings, AgentKnowledgeBinding{
			ID:                 s.newID("agentkb"),
			TenantID:           access.TenantID,
			AgentID:            agentID,
			KnowledgeBaseID:    input.KnowledgeBaseID,
			RetrievalProfileID: strings.TrimSpace(input.RetrievalProfileID),
			Priority:           priority,
			Weight:             weight,
			Enabled:            enabled,
			RetrievalOverrides: cloneMap(input.RetrievalOverrides),
		})
	}
	if err := s.repo.ReplaceAgentKnowledgeBindings(ctx, access, agentID, bindings); err != nil {
		return nil, err
	}
	return bindings, nil
}

func (s *AgentService) ListBindings(ctx context.Context, access AccessContext, agentID string) ([]AgentKnowledgeBinding, error) {
	if _, err := s.GetAgent(ctx, access, agentID); err != nil {
		return nil, err
	}
	return s.repo.ListAgentKnowledgeBindings(ctx, access, agentID)
}

func (s *AgentService) CreateConversation(ctx context.Context, access AccessContext, agentID string, title string) (Conversation, error) {
	if _, err := s.GetAgent(ctx, access, agentID); err != nil {
		return Conversation{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新对话"
	}
	if len([]rune(title)) > 150 {
		return Conversation{}, fmt.Errorf("conversation title is too long: %w", ErrValidation)
	}
	now := s.now()
	return s.repo.CreateConversation(ctx, access, Conversation{
		ID:             s.newID("conversation"),
		TenantID:       access.TenantID,
		OrganizationID: access.OrganizationID,
		AgentID:        agentID,
		UserID:         access.UserID,
		Title:          title,
		Status:         "ACTIVE",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
}

func (s *AgentService) ListConversations(ctx context.Context, access AccessContext, agentID string, options ListOptions) ([]Conversation, string, error) {
	options.Limit = normalizedLimit(options.Limit, 20, 100)
	return s.repo.ListConversations(ctx, access, strings.TrimSpace(agentID), options)
}

func (s *AgentService) GetConversation(ctx context.Context, access AccessContext, id string) (Conversation, error) {
	return s.repo.GetConversation(ctx, access, strings.TrimSpace(id))
}

func (s *AgentService) ListMessages(ctx context.Context, access AccessContext, conversationID string, options ListOptions) ([]Message, string, error) {
	options.Limit = normalizedLimit(options.Limit, 100, 500)
	return s.repo.ListMessages(ctx, access, strings.TrimSpace(conversationID), options)
}
