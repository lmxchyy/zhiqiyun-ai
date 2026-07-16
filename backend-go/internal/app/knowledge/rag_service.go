package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type RAGService struct {
	agents    AgentRepository
	runs      RAGRepository
	retrieval *RetrievalService
	rewriter  QueryRewriter
	answers   AnswerGenerator
	billing   RAGBillingRecorder
	now       func() time.Time
	newID     func(string) string
	mu        sync.Mutex
	active    map[string]context.CancelFunc
}

func (s *RAGService) SetBillingRecorder(recorder RAGBillingRecorder) {
	if s != nil {
		s.billing = recorder
	}
}

func NewRAGService(agents AgentRepository, runs RAGRepository, retrieval *RetrievalService, rewriter QueryRewriter, answers AnswerGenerator) *RAGService {
	return &RAGService{
		agents: agents, runs: runs, retrieval: retrieval, rewriter: rewriter, answers: answers,
		now: func() time.Time { return time.Now().UTC() }, newID: newID, active: map[string]context.CancelFunc{},
	}
}

type RunInput struct {
	ConversationID string
	Question       string
	RetryOfRunID   string
	TopK           int
	Threshold      float64
	Mode           string
}

type RunResult struct {
	Run       RAGRun         `json:"run"`
	Message   Message        `json:"message"`
	Hits      []RetrievalHit `json:"hits"`
	Citations []Citation     `json:"citations"`
}

func (s *RAGService) Run(ctx context.Context, access AccessContext, input RunInput) (result RunResult, err error) {
	return s.run(ctx, access, input)
}

type RunEventSink func(RunEvent) error

type runEventSinkContextKey struct{}

func (s *RAGService) RunWithSink(ctx context.Context, access AccessContext, input RunInput, sink RunEventSink) (RunResult, error) {
	if sink != nil {
		ctx = context.WithValue(ctx, runEventSinkContextKey{}, sink)
	}
	return s.run(ctx, access, input)
}

func (s *RAGService) run(ctx context.Context, access AccessContext, input RunInput) (result RunResult, err error) {
	if s == nil || s.agents == nil || s.runs == nil || s.retrieval == nil || s.answers == nil {
		return result, fmt.Errorf("RAG service is not configured: %w", ErrValidation)
	}
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.Question = strings.TrimSpace(input.Question)
	if input.ConversationID == "" || input.Question == "" {
		return result, fmt.Errorf("conversation id and question are required: %w", ErrValidation)
	}
	conversation, err := s.agents.GetConversation(ctx, access, input.ConversationID)
	if err != nil {
		return result, err
	}
	agent, err := s.agents.GetAgent(ctx, access, conversation.AgentID)
	if err != nil {
		return result, err
	}
	if agent.Status != "ACTIVE" {
		return result, fmt.Errorf("agent is not active: %w", ErrForbidden)
	}
	history, _, err := s.agents.ListMessages(ctx, access, conversation.ID, ListOptions{Limit: 100})
	if err != nil {
		return result, err
	}
	now := s.now()
	userMessage, err := s.agents.CreateMessage(ctx, access, Message{
		ID: s.newID("message"), TenantID: access.TenantID, ConversationID: conversation.ID, Role: "user",
		Content: input.Question, Status: "COMPLETED", Metadata: map[string]any{}, CreatedAt: now,
	})
	if err != nil {
		return result, err
	}
	bindings, err := s.agents.ListAgentKnowledgeBindings(ctx, access, agent.ID)
	if err != nil {
		return result, err
	}
	activeBindings := make([]AgentKnowledgeBinding, 0, len(bindings))
	knowledgeBaseIDs := make([]string, 0, len(bindings))
	for _, item := range bindings {
		if item.Enabled {
			activeBindings = append(activeBindings, item)
			knowledgeBaseIDs = append(knowledgeBaseIDs, item.KnowledgeBaseID)
		}
	}
	run := RAGRun{
		ID: s.newID("ragrun"), TenantID: access.TenantID, ConversationID: conversation.ID, UserMessageID: userMessage.ID,
		AgentID: agent.ID, RetryOfRunID: strings.TrimSpace(input.RetryOfRunID), OriginalQuery: input.Question, Status: "QUEUED",
		BindingSnapshot: activeBindings, RetrievalSnapshot: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	run, err = s.runs.CreateRun(ctx, access, run)
	if err != nil {
		return result, err
	}
	result.Run = run
	runCtx, cancel := context.WithCancel(ctx)
	s.register(run.ID, cancel)
	defer func() {
		cancel()
		s.unregister(run.ID)
		if err != nil && !errors.Is(err, context.Canceled) {
			run.Status = "FAILED"
			run.ErrorCode = "RAG_RUN_FAILED"
			run.ErrorMessage = err.Error()
			run.UpdatedAt = s.now()
			_ = s.runs.UpdateRun(context.WithoutCancel(ctx), access, run)
			_ = s.appendEvent(context.WithoutCancel(ctx), access, run.ID, 99, "error", map[string]any{"message": err.Error()})
			result.Run = run
		}
	}()
	if err = s.appendEvent(runCtx, access, run.ID, 1, "run.started", map[string]any{"question": input.Question}); err != nil {
		return result, err
	}
	rewritten := input.Question
	if s.rewriter != nil {
		if candidate, rewriteErr := s.rewriter.Rewrite(runCtx, history, input.Question); rewriteErr == nil && strings.TrimSpace(candidate) != "" {
			rewritten = strings.TrimSpace(candidate)
		}
	}
	run.RewrittenQuery = rewritten
	if len(knowledgeBaseIDs) == 0 {
		return result, fmt.Errorf("agent has no enabled knowledge base: %w", ErrValidation)
	}
	run.Status = "RETRIEVING"
	run.UpdatedAt = s.now()
	if err = s.runs.UpdateRun(runCtx, access, run); err != nil {
		return result, err
	}
	if err = s.appendEvent(runCtx, access, run.ID, 2, "retrieval.started", map[string]any{"query": rewritten, "knowledgeBaseIds": knowledgeBaseIDs}); err != nil {
		return result, err
	}
	retrievalStarted := s.now()
	hits := make([]RetrievalHit, 0)
	for _, binding := range activeBindings {
		request := SearchRequest{
			Access: access, KnowledgeBaseIDs: []string{binding.KnowledgeBaseID}, Query: rewritten, Mode: input.Mode, TopK: input.TopK,
			Threshold: input.Threshold, VectorWeight: 0.7, KeywordWeight: 0.3,
			RetrievalProfileIDs: map[string]string{binding.KnowledgeBaseID: binding.RetrievalProfileID},
		}
		applyRetrievalOverrides(&request, binding.RetrievalOverrides)
		items, searchErr := s.retrieval.Search(runCtx, request)
		if searchErr != nil {
			return result, searchErr
		}
		weight := binding.Weight
		if weight <= 0 {
			weight = 1
		}
		for index := range items {
			if items[index].Metadata == nil {
				items[index].Metadata = map[string]any{}
			}
			items[index].Metadata["bindingWeight"] = weight
			items[index].Metadata["bindingPriority"] = binding.Priority
			items[index].Metadata["weightedScore"] = items[index].FinalScore * weight
		}
		hits = append(hits, items...)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		leftPriority, rightPriority := metadataInt(hits[i].Metadata, "bindingPriority"), metadataInt(hits[j].Metadata, "bindingPriority")
		leftScore, rightScore := metadataFloat(hits[i].Metadata, "weightedScore", hits[i].FinalScore), metadataFloat(hits[j].Metadata, "weightedScore", hits[j].FinalScore)
		if leftScore == rightScore && leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		return leftScore > rightScore
	})
	limit := normalizedTopK(input.TopK)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	run.RetrievalLatencyMS = time.Since(retrievalStarted).Milliseconds()
	for index := range hits {
		hits[index].ID = s.newID("raghit")
		hits[index].TenantID = access.TenantID
		hits[index].RAGRunID = run.ID
		if hits[index].InitialRank == 0 {
			hits[index].InitialRank = index + 1
		}
		if hits[index].FinalRank == 0 {
			hits[index].FinalRank = index + 1
		}
	}
	if err = s.runs.SaveRetrievalHits(runCtx, access, run.ID, hits); err != nil {
		return result, err
	}
	run.RetrievalSnapshot = map[string]any{"mode": normalizedSearchMode(input.Mode), "topK": normalizedTopK(input.TopK), "threshold": normalizedThreshold(input.Threshold), "hitCount": len(hits)}
	if err = s.appendEvent(runCtx, access, run.ID, 3, "retrieval.completed", map[string]any{"hitCount": len(hits), "latencyMs": run.RetrievalLatencyMS}); err != nil {
		return result, err
	}
	run.Status = "GENERATING"
	run.UpdatedAt = s.now()
	if err = s.runs.UpdateRun(runCtx, access, run); err != nil {
		return result, err
	}
	if err = s.appendEvent(runCtx, access, run.ID, 4, "generation.started", map[string]any{"model": agent.ModelName}); err != nil {
		return result, err
	}
	generationStarted := s.now()
	stream, err := s.answers.Generate(runCtx, AnswerRequest{Agent: agent, Messages: history, Question: input.Question, Hits: hits})
	if err != nil {
		return result, err
	}
	var answer strings.Builder
	usage := map[string]any{}
	sequence := int64(5)
	for chunk := range stream {
		if chunk.Err != nil {
			return result, chunk.Err
		}
		if chunk.Delta != "" {
			answer.WriteString(chunk.Delta)
			if err = s.appendEvent(runCtx, access, run.ID, sequence, "answer.delta", map[string]any{"delta": chunk.Delta}); err != nil {
				return result, err
			}
			sequence++
		}
		for key, value := range chunk.Usage {
			usage[key] = value
		}
	}
	if strings.TrimSpace(answer.String()) == "" {
		return result, errors.New("answer generator returned empty content")
	}
	run.GenerationLatencyMS = time.Since(generationStarted).Milliseconds()
	inputTokens := usageInt(usage, "prompt_tokens", "input_tokens")
	if inputTokens <= 0 {
		inputTokens = estimateRAGTokens(input.Question, hits)
	}
	outputTokens := usageInt(usage, "completion_tokens", "output_tokens")
	if outputTokens <= 0 {
		outputTokens = estimateRAGTextTokens(answer.String())
	}
	assistantCreatedAt := s.now()
	if !assistantCreatedAt.After(userMessage.CreatedAt) {
		assistantCreatedAt = userMessage.CreatedAt.Add(time.Nanosecond)
	}
	assistantMessage, err := s.agents.CreateMessage(runCtx, access, Message{
		ID: s.newID("message"), TenantID: access.TenantID, ConversationID: conversation.ID, ParentMessageID: userMessage.ID,
		Role: "assistant", Content: answer.String(), Status: "COMPLETED", InputTokens: inputTokens,
		OutputTokens: outputTokens, Metadata: map[string]any{"ragRunId": run.ID}, CreatedAt: assistantCreatedAt,
	})
	if err != nil {
		return result, err
	}
	citations := buildCitations(s.newID, access.TenantID, run.ID, assistantMessage.ID, hits)
	if err = s.runs.SaveCitations(runCtx, access, run.ID, citations); err != nil {
		return result, err
	}
	run.AssistantMessageID = assistantMessage.ID
	run.InputTokens = assistantMessage.InputTokens
	run.OutputTokens = assistantMessage.OutputTokens
	run.PointCost = ragPointCost(run.InputTokens, run.OutputTokens)
	if s.billing != nil {
		if err = s.billing.RecordRAGUsage(runCtx, RAGBillingUsage{
			TenantID: access.TenantID, UserID: access.UserID, AgentID: agent.ID, RunID: run.ID, Model: agent.ModelName,
			InputTokens: run.InputTokens, OutputTokens: run.OutputTokens, PointCost: run.PointCost,
		}); err != nil {
			return result, err
		}
	}
	run.Status = "COMPLETED"
	run.UpdatedAt = s.now()
	if err = s.runs.UpdateRun(runCtx, access, run); err != nil {
		return result, err
	}
	if err = s.appendEvent(runCtx, access, run.ID, sequence, "run.completed", map[string]any{"messageId": assistantMessage.ID, "citationCount": len(citations)}); err != nil {
		return result, err
	}
	return RunResult{Run: run, Message: assistantMessage, Hits: hits, Citations: citations}, nil
}

func applyRetrievalOverrides(request *SearchRequest, overrides map[string]any) {
	if request == nil || overrides == nil {
		return
	}
	if value := strings.TrimSpace(fmt.Sprint(overrides["mode"])); value != "" && value != "<nil>" {
		request.Mode = value
	}
	if value := anyInt(overrides["topK"]); value > 0 {
		request.TopK = value
	}
	if value := anyFloat(overrides["threshold"]); value > 0 {
		request.Threshold = value
	}
	if value := anyFloat(overrides["vectorWeight"]); value > 0 {
		request.VectorWeight = value
	}
	if value := anyFloat(overrides["keywordWeight"]); value > 0 {
		request.KeywordWeight = value
	}
	if filters, ok := overrides["filters"].(map[string]any); ok {
		request.Filters = cloneMap(filters)
	}
}

func anyInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func anyFloat(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

func metadataInt(metadata map[string]any, key string) int {
	return anyInt(metadata[key])
}

func metadataFloat(metadata map[string]any, key string, fallback float64) float64 {
	if value := anyFloat(metadata[key]); value != 0 {
		return value
	}
	return fallback
}

func ragPointCost(inputTokens int, outputTokens int) int64 {
	total := inputTokens + outputTokens
	if total <= 0 {
		return 0
	}
	return int64((total + 999) / 1000)
}

func estimateRAGTokens(question string, hits []RetrievalHit) int {
	total := len([]rune(question))
	for _, hit := range hits {
		total += len([]rune(hit.Content))
	}
	return (total + 2) / 3
}

func estimateRAGTextTokens(value string) int {
	return (len([]rune(strings.TrimSpace(value))) + 2) / 3
}

func (s *RAGService) Cancel(ctx context.Context, access AccessContext, runID string) (RAGRun, error) {
	run, err := s.runs.GetRun(ctx, access, strings.TrimSpace(runID))
	if err != nil {
		return RAGRun{}, err
	}
	if isTerminalRunStatus(run.Status) {
		return run, nil
	}
	s.mu.Lock()
	cancel := s.active[run.ID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	run.Status = "CANCELLED"
	run.UpdatedAt = s.now()
	if err := s.runs.UpdateRun(context.WithoutCancel(ctx), access, run); err != nil {
		return RAGRun{}, err
	}
	_ = s.appendEvent(context.WithoutCancel(ctx), access, run.ID, 98, "run.cancelled", map[string]any{})
	return run, nil
}

func (s *RAGService) GetRun(ctx context.Context, access AccessContext, runID string) (RAGRun, error) {
	return s.runs.GetRun(ctx, access, strings.TrimSpace(runID))
}

func (s *RAGService) ListEvents(ctx context.Context, access AccessContext, runID string, afterSequence int64) ([]RunEvent, error) {
	return s.runs.ListRunEvents(ctx, access, strings.TrimSpace(runID), afterSequence)
}

func (s *RAGService) ListCitations(ctx context.Context, access AccessContext, runID string) ([]Citation, error) {
	return s.runs.ListCitations(ctx, access, strings.TrimSpace(runID))
}

func (s *RAGService) appendEvent(ctx context.Context, access AccessContext, runID string, sequence int64, eventType string, payload map[string]any) error {
	event := RunEvent{
		ID: s.newID("ragevent"), TenantID: access.TenantID, RAGRunID: runID, SequenceNo: sequence,
		EventType: eventType, Payload: cloneMap(payload), CreatedAt: s.now(),
	}
	if err := s.runs.AppendRunEvent(ctx, access, event); err != nil {
		return err
	}
	if sink, ok := ctx.Value(runEventSinkContextKey{}).(RunEventSink); ok && sink != nil {
		return sink(event)
	}
	return nil
}

func (s *RAGService) register(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[runID] = cancel
}

func (s *RAGService) unregister(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, runID)
}

func buildCitations(id func(string) string, tenantID string, runID string, messageID string, hits []RetrievalHit) []Citation {
	limit := len(hits)
	if limit > 8 {
		limit = 8
	}
	items := make([]Citation, 0, limit)
	for index := 0; index < limit; index++ {
		hit := hits[index]
		items = append(items, Citation{
			ID: id("citation"), TenantID: tenantID, RAGRunID: runID, AssistantMessageID: messageID,
			DocumentID: hit.DocumentID, DocumentVersionID: hit.DocumentVersionID, ChunkID: hit.ChunkID, Order: index + 1,
			DocumentName: hit.DocumentName, Quote: truncateRunes(hit.Content, 320), Locator: cloneMap(hit.SourceLocator), SimilarityScore: hit.FinalScore,
		})
	}
	return items
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func usageInt(usage map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := usage[key].(type) {
		case int:
			return value
		case int64:
			return int(value)
		case float64:
			return int(value)
		}
	}
	return 0
}

func normalizedSearchMode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "HYBRID"
	}
	return value
}

func normalizedTopK(value int) int {
	if value <= 0 || value > 50 {
		return 8
	}
	return value
}

func normalizedThreshold(value float64) float64 {
	if value <= 0 {
		return 0.2
	}
	return value
}

func isTerminalRunStatus(value string) bool {
	return value == "COMPLETED" || value == "FAILED" || value == "CANCELLED"
}
