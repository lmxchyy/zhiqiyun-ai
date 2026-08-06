package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"xianzhi-ai/backend-go/internal/app/generation"
	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/app/ppt/skills"
	chatprovider "xianzhi-ai/backend-go/internal/provider/chat"
	"xianzhi-ai/backend-go/internal/provider/parser"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

const (
	pptAgentMessageScope       = "message"
	pptAgentDefaultSlideCount  = 8
	pptAgentMaxMessageRunes    = 8000
	pptAgentRecentMessages     = 20
	pptAgentRecentMessageRunes = 2000
	pptAgentImportScope        = "import-outline"
	pptAgentRevisionScope      = "revise-slide"
	pptAgentMaxSourceFiles     = 3
	pptAgentMaxSourceFileBytes = int64(10 << 20)
	pptAgentMaxSourceTextRunes = 200000
)

var pptAgentFileIDPattern = regexp.MustCompile(`^file_[A-Za-z0-9_-]+$`)

type pptAgentStateStore interface {
	CreateSession(context.Context, pptapp.SessionRequest) (pptapp.Task, error)
	GetTask(context.Context, pptapp.OwnerScope, string) (pptapp.Task, error)
	History(context.Context, pptapp.OwnerScope) ([]pptapp.Task, error)
	BeginOperation(context.Context, pptapp.OwnerScope, string, string, string, string) (pptapp.OperationClaim, pptapp.Task, error)
	CompleteOutlineOperation(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, []pptapp.AgentMessage, pptapp.Outline) (pptapp.Task, error)
	CompleteImportOutlineOperation(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, []pptapp.AgentMessage, pptapp.Outline, []string) (pptapp.Task, error)
	CompleteSlideRevision(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, string, pptapp.Slide) (pptapp.Task, error)
	FailOperation(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, string) (pptapp.Task, error)
}

type pptAgentServiceAdapter struct {
	service *pptapp.Service
}

func (a pptAgentServiceAdapter) CreateSession(ctx context.Context, req pptapp.SessionRequest) (pptapp.Task, error) {
	return a.service.CreateSession(ctx, req)
}

func (a pptAgentServiceAdapter) GetTask(_ context.Context, owner pptapp.OwnerScope, taskID string) (pptapp.Task, error) {
	return a.service.GetTask(owner, taskID)
}

func (a pptAgentServiceAdapter) History(_ context.Context, owner pptapp.OwnerScope) ([]pptapp.Task, error) {
	return a.service.HistoryWithError(owner)
}

func (a pptAgentServiceAdapter) BeginOperation(ctx context.Context, owner pptapp.OwnerScope, taskID, scope, key, requestHash string) (pptapp.OperationClaim, pptapp.Task, error) {
	return a.service.BeginOperation(ctx, owner, taskID, scope, key, requestHash)
}

func (a pptAgentServiceAdapter) CompleteOutlineOperation(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, messages []pptapp.AgentMessage, outline pptapp.Outline) (pptapp.Task, error) {
	return a.service.CompleteOutlineOperation(ctx, owner, taskID, claim, messages, outline)
}

func (a pptAgentServiceAdapter) CompleteImportOutlineOperation(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, messages []pptapp.AgentMessage, outline pptapp.Outline, sourceFileIDs []string) (pptapp.Task, error) {
	return a.service.CompleteImportOutlineOperation(ctx, owner, taskID, claim, messages, outline, sourceFileIDs)
}

func (a pptAgentServiceAdapter) CompleteSlideRevision(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, slideID string, revision pptapp.Slide) (pptapp.Task, error) {
	return a.service.CompleteSlideRevision(ctx, owner, taskID, claim, slideID, revision)
}

func (a pptAgentServiceAdapter) FailOperation(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, errorCode string) (pptapp.Task, error) {
	return a.service.FailOperation(ctx, owner, taskID, claim, errorCode)
}

type pptAgentStateContextKey struct{}
type pptAgentChatContextKey struct{}
type pptAgentFileStoreContextKey struct{}
type pptAgentMarkdownParserContextKey struct{}

type pptAgentChatFunc func(context.Context, generation.CreateRequest) (chatprovider.Response, error)
type pptAgentFileStore interface {
	OpenObject(context.Context, storagecenter.AccessContext, string) (storagecenter.FileObject, io.ReadCloser, error)
}
type pptAgentMarkdownParseFunc func(context.Context, knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error)

type pptAgentError struct {
	status  int
	code    string
	message string
}

var errPPTAgentOperationCleanup = errors.New("PPT_OPERATION_CLEANUP_FAILED")

func (e pptAgentError) Error() string        { return e.message }
func (e pptAgentError) BusinessCode() string { return e.code }

func newPPTAgentError(status int, code, message string) error {
	return pptAgentError{status: status, code: code, message: message}
}

type pptCreateSessionRequest struct {
	Prompt          string   `json:"prompt"`
	SkillCode       string   `json:"skillCode"`
	SourceFileIDs   []string `json:"sourceFileIds"`
	SlideCount      int      `json:"slideCount"`
	Language        string   `json:"language"`
	Audience        string   `json:"audience"`
	ClientRequestID string   `json:"clientRequestId"`
}

type pptAgentMessageRequest struct {
	Message string `json:"message"`
}

type pptAgentImportOutlineRequest struct {
	SourceFileIDs []string `json:"sourceFileIds"`
}

type pptAgentReviseSlideRequest struct {
	SlideID     string `json:"slideId"`
	Instruction string `json:"instruction"`
}

type pptAgentSlideRevisionPayload struct {
	Blocks []pptapp.SlideBlock `json:"blocks"`
}

// pptTaskPublicResponse is an explicit allow-list shared by the Agent and
// legacy task readers. Persistence claims, billing bindings, run leases, and
// provider routing fields must never be added to this DTO.
type pptTaskPublicResponse struct {
	TaskID                string                  `json:"taskId"`
	SessionID             string                  `json:"sessionId,omitempty"`
	Type                  string                  `json:"type,omitempty"`
	MediaType             string                  `json:"mediaType,omitempty"`
	SkillCode             string                  `json:"skillCode,omitempty"`
	Stage                 pptapp.Stage            `json:"stage,omitempty"`
	Status                string                  `json:"status"`
	Title                 string                  `json:"title"`
	Prompt                string                  `json:"prompt,omitempty"`
	SlideCount            int                     `json:"slideCount,omitempty"`
	Language              string                  `json:"language,omitempty"`
	Tone                  string                  `json:"tone,omitempty"`
	TextContent           string                  `json:"textContent,omitempty"`
	Audience              string                  `json:"audience,omitempty"`
	Scenario              string                  `json:"scenario,omitempty"`
	GenerationAspectRatio string                  `json:"generationAspectRatio,omitempty"`
	Theme                 string                  `json:"theme,omitempty"`
	AutoThemeEnabled      bool                    `json:"autoThemeEnabled"`
	EnableWebSearch       bool                    `json:"enableWebSearch,omitempty"`
	ImageSource           string                  `json:"imageSource,omitempty"`
	ImageStyle            string                  `json:"imageStyle,omitempty"`
	PeopleStyle           string                  `json:"peopleStyle,omitempty"`
	ImageLighting         string                  `json:"imageLighting,omitempty"`
	ImageComposition      string                  `json:"imageComposition,omitempty"`
	TextInImage           bool                    `json:"textInImage"`
	Progress              int                     `json:"progress,omitempty"`
	CurrentPage           int                     `json:"currentPage,omitempty"`
	VisualProgress        int                     `json:"visualProgress,omitempty"`
	Outline               *pptapp.Outline         `json:"outline,omitempty"`
	Slides                []pptAgentSlideResponse `json:"slides,omitempty"`
	AgentMessages         []pptapp.AgentMessage   `json:"agentMessages,omitempty"`
	SourceFileIDs         []string                `json:"sourceFileIds,omitempty"`
	OutlineConfirmedAt    string                  `json:"outlineConfirmedAt,omitempty"`
	GenerationStartedAt   string                  `json:"generationStartedAt,omitempty"`
	CompletedAt           string                  `json:"completedAt,omitempty"`
	ErrorCode             string                  `json:"errorCode,omitempty"`
	PPTURL                string                  `json:"pptUrl"`
	PDFURL                string                  `json:"pdfUrl"`
	ErrorMessage          string                  `json:"errorMessage"`
	CreatedAt             string                  `json:"createdAt,omitempty"`
	UpdatedAt             string                  `json:"updatedAt,omitempty"`
}

type pptAgentSlideResponse struct {
	ID               string                        `json:"id"`
	Page             int                           `json:"page"`
	Title            string                        `json:"title"`
	Content          string                        `json:"content"`
	BulletPoints     []string                      `json:"bulletPoints"`
	ImageURL         string                        `json:"imageUrl,omitempty"`
	VisualStorageRef string                        `json:"visualStorageRef,omitempty"`
	Layout           string                        `json:"layout"`
	SpeakerNotes     string                        `json:"speakerNotes,omitempty"`
	Blocks           []pptapp.SlideBlock           `json:"blocks,omitempty"`
	SlideType        string                        `json:"slideType,omitempty"`
	VisualPlan       *pptAgentVisualPlanResponse   `json:"visualPlan,omitempty"`
	VisualHistory    []pptAgentVisualAssetResponse `json:"visualHistory,omitempty"`
	VisualCreatedAt  string                        `json:"visualCreatedAt,omitempty"`
	VisualStatus     string                        `json:"visualStatus,omitempty"`
	VisualError      string                        `json:"visualError,omitempty"`
}

type pptAgentVisualPlanResponse struct {
	VisualType      string   `json:"visualType"`
	ImageRequired   bool     `json:"imageRequired"`
	ChartRequired   bool     `json:"chartRequired"`
	DiagramRequired bool     `json:"diagramRequired"`
	TextInImage     bool     `json:"textInImage"`
	Subject         string   `json:"subject"`
	Scene           string   `json:"scene"`
	Action          string   `json:"action"`
	Objects         []string `json:"objects"`
	Mood            string   `json:"mood"`
	Composition     string   `json:"composition"`
	Style           string   `json:"style"`
}

type pptAgentVisualAssetResponse struct {
	URL        string `json:"url"`
	StorageRef string `json:"storageRef,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type pptAgentOutlinePayload struct {
	Title string                       `json:"title"`
	Pages []pptAgentOutlinePagePayload `json:"pages"`
}

type pptAgentOutlinePagePayload struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Bullets []string `json:"bullets"`
}

func (a api) listPPTSkills(w http.ResponseWriter, r *http.Request) {
	if _, _, err := a.authenticatedUser(r); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, skills.List())
}

func (a api) createPPTSession(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	var req pptCreateSessionRequest
	if err := decodePPTAgentJSON(r, &req); err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	req.Prompt = normalizePPTAgentText(req.Prompt, 0)
	req.SkillCode = strings.TrimSpace(req.SkillCode)
	if req.SkillCode == "" {
		req.SkillCode = "general"
	}
	if req.Prompt == "" {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_PROMPT_REQUIRED", "请输入演示主题"))
		return
	}
	if len([]rune(req.Prompt)) > pptAgentMaxMessageRunes {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_PROMPT_TOO_LONG", "演示主题过长"))
		return
	}
	skill, ok := skills.Resolve(req.SkillCode)
	if !ok {
		writePPTAgentError(w, newPPTAgentError(http.StatusNotFound, "PPT_SKILL_NOT_FOUND", "未找到指定的 PPT Skill"))
		return
	}
	var sourceFileIDs []string
	if len(req.SourceFileIDs) > 0 {
		sourceFileIDs, err = validatePPTAgentSourceFileIDs(req.SourceFileIDs)
		if err != nil {
			writePPTAgentError(w, err)
			return
		}
	}
	if err := a.checkMiniProgramText(r.Context(), r, user, req.Prompt); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	slideCount := boundedPPTAgentSlideCount(req.SlideCount, skill.MaxSlides)
	capability, err := a.preparePPTCapabilityRequest(data, user, req.Prompt, "", slideCount, false, len(req.SourceFileIDs) > 0)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "当前账户无法使用该 PPT 能力"))
		return
	}
	tenantContext, err := pptAgentTenantContextFromCapability(capability)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	slideCount = boundedPPTAgentSlideCount(int(anyFloatOrDefault(capability.Params["page_count"], float64(slideCount))), skill.MaxSlides)
	language := strings.ToLower(strings.TrimSpace(req.Language))
	if language != "en" {
		language = "zh"
	}
	audience := strings.TrimSpace(req.Audience)
	if audience == "" {
		audience = "auto"
	}
	task, err := pptAgentStateForRequest(a, r).CreateSession(r.Context(), pptapp.SessionRequest{
		Owner:           pptapp.OwnerScope{TenantID: tenantContext.TenantID, UserID: user.ID},
		ClientRequestID: strings.TrimSpace(req.ClientRequestID), Prompt: req.Prompt,
		OrganizationID: tenantContext.OrganizationID, ContextType: tenantContext.ContextType,
		BillingScope: tenantContext.BillingScope, BillingAccountID: tenantContext.BillingAccountID,
		SkillCode: skill.Code, SourceFileIDs: sourceFileIDs,
		SlideCount: slideCount, Language: language, Audience: audience,
	})
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	writeJSON(w, pptTaskResponse(task))
}

func (a api) postPPTSessionMessage(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED", "缺少 Idempotency-Key"))
		return
	}
	if len(idempotencyKey) > 256 || strings.ContainsAny(idempotencyKey, "\r\n") {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_INVALID", "Idempotency-Key 无效"))
		return
	}
	var req pptAgentMessageRequest
	if err := decodePPTAgentJSON(r, &req); err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	message := normalizePPTAgentText(req.Message, 0)
	if message == "" {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_MESSAGE_REQUIRED", "请输入对话内容"))
		return
	}
	if len([]rune(message)) > pptAgentMaxMessageRunes {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_MESSAGE_TOO_LONG", "对话内容过长"))
		return
	}
	if err := a.checkMiniProgramText(r.Context(), r, user, message); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	state := pptAgentStateForRequest(a, r)
	owner, err := pptOwnerForCapability(a.store, user)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	task, err := state.GetTask(r.Context(), owner, taskID)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	skill, ok := skills.Resolve(strings.TrimSpace(task.SkillCode))
	if !ok {
		writePPTAgentError(w, newPPTAgentError(http.StatusNotFound, "PPT_SKILL_NOT_FOUND", "未找到指定的 PPT Skill"))
		return
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, message, "", boundedPPTAgentSlideCount(task.SlideCount, skill.MaxSlides), false, len(task.SourceFileIDs) > 0)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "当前账户无法使用该 PPT 能力"))
		return
	}
	tenantContext, err := pptAgentTenantContextFromCapability(capability)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	if !pptAgentTaskMatchesTenantContext(task, tenantContext) {
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_TENANT_CONTEXT_MISMATCH", "当前租户上下文与 PPT 会话不匹配"))
		return
	}
	messageScope := pptAgentMessageScopeForTask(task)
	requestHash := pptAgentMessageHash(messageScope, message)
	claim, claimedTask, err := state.BeginOperation(r.Context(), owner, taskID, messageScope, idempotencyKey, requestHash)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	if claim.CompletedReplay {
		writeJSON(w, pptTaskResponse(claimedTask))
		return
	}
	request := buildPPTAgentChatRequest(skill, claimedTask, message, capability.Model)
	response, err := pptAgentChatForRequest(a, r)(r.Context(), request)
	if err != nil {
		if failErr := failPPTAgentOperation(state, owner, taskID, claim, "PPT_AGENT_PROVIDER_UNAVAILABLE"); failErr != nil {
			writePPTAgentOperationCleanupError(w)
			return
		}
		writePPTAgentError(w, newPPTAgentError(http.StatusBadGateway, "PPT_AGENT_PROVIDER_UNAVAILABLE", "PPT Agent 服务暂时不可用，请稍后重试"))
		return
	}
	outline, err := parsePPTAgentOutline(response.Message.Content, claimedTask.SlideCount, skill)
	if err != nil {
		if failErr := failPPTAgentOperation(state, owner, taskID, claim, "PPT_AGENT_RESPONSE_INVALID"); failErr != nil {
			writePPTAgentOperationCleanupError(w)
			return
		}
		writePPTAgentError(w, newPPTAgentError(http.StatusBadGateway, "PPT_AGENT_RESPONSE_INVALID", "PPT Agent 返回了无效的大纲"))
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updated, err := state.CompleteOutlineOperation(r.Context(), owner, taskID, claim, []pptapp.AgentMessage{
		{Role: "user", Content: message, CreatedAt: now},
		{Role: "assistant", Content: pptAgentOutlineAssistantMessage(outline), CreatedAt: now},
	}, outline)
	if err != nil {
		recovered, ok, recoveryErr := recoverPPTAgentCompletion(state, owner, taskID, claim)
		if ok {
			writeJSON(w, pptTaskResponse(recovered))
			return
		}
		if errors.Is(recoveryErr, errPPTAgentOperationCleanup) {
			writePPTAgentOperationCleanupError(w)
			return
		}
		if recoveryErr != nil {
			writePPTAgentStateError(w, recoveryErr)
			return
		}
		writePPTAgentStateError(w, err)
		return
	}
	writeJSON(w, pptTaskResponse(updated))
}

func (a api) importPPTSessionOutline(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED", "缺少 Idempotency-Key"))
		return
	}
	if len(idempotencyKey) > 256 || strings.ContainsAny(idempotencyKey, "\r\n") {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_INVALID", "Idempotency-Key 无效"))
		return
	}
	var req pptAgentImportOutlineRequest
	if err := decodePPTAgentJSON(r, &req); err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	sourceFileIDs, err := validatePPTAgentSourceFileIDs(req.SourceFileIDs)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	state := pptAgentStateForRequest(a, r)
	owner, err := pptOwnerForCapability(a.store, user)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	task, err := state.GetTask(r.Context(), owner, taskID)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	skill, ok := skills.Resolve(strings.TrimSpace(task.SkillCode))
	if !ok {
		writePPTAgentError(w, newPPTAgentError(http.StatusNotFound, "PPT_SKILL_NOT_FOUND", "未找到指定的 PPT Skill"))
		return
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, task.Prompt, "", boundedPPTAgentSlideCount(task.SlideCount, skill.MaxSlides), false, true)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "当前账户无法使用该 PPT 能力"))
		return
	}
	tenantContext, err := pptAgentTenantContextFromCapability(capability)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	if !pptAgentTaskMatchesTenantContext(task, tenantContext) {
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_TENANT_CONTEXT_MISMATCH", "当前租户上下文与 PPT 会话不匹配"))
		return
	}
	requestHash := pptAgentImportHash(task, sourceFileIDs)
	claim, claimedTask, err := state.BeginOperation(r.Context(), owner, taskID, pptAgentImportScope, idempotencyKey, requestHash)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	if claim.CompletedReplay {
		writeJSON(w, pptTaskResponse(claimedTask))
		return
	}
	fileStore := pptAgentFileStoreForRequest(a, r)
	if fileStore == nil {
		writePPTImportFailure(w, state, owner, taskID, claim, newPPTAgentError(http.StatusServiceUnavailable, "PPT_SESSION_STORAGE_UNAVAILABLE", "PPT 文件存储暂时不可用"))
		return
	}
	importedText, importErr := readPPTAgentMarkdownFiles(r.Context(), fileStore, pptAgentMarkdownParserForRequest(r), storagecenter.AccessContext{
		TenantID: tenantContext.TenantID,
		UserID:   user.ID,
	}, sourceFileIDs)
	if importErr != nil {
		writePPTImportFailure(w, state, owner, taskID, claim, importErr)
		return
	}
	request := buildPPTAgentChatRequestWithLimit(skill, claimedTask, importedText, capability.Model, pptAgentMaxSourceTextRunes)
	response, err := pptAgentChatForRequest(a, r)(r.Context(), request)
	if err != nil {
		writePPTImportFailure(w, state, owner, taskID, claim, newPPTAgentError(http.StatusBadGateway, "PPT_AGENT_PROVIDER_UNAVAILABLE", "PPT Agent 服务暂时不可用，请稍后重试"))
		return
	}
	outline, err := parsePPTAgentOutline(response.Message.Content, claimedTask.SlideCount, skill)
	if err != nil {
		writePPTImportFailure(w, state, owner, taskID, claim, newPPTAgentError(http.StatusBadGateway, "PPT_AGENT_RESPONSE_INVALID", "PPT Agent 返回了无效的大纲"))
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updated, err := state.CompleteImportOutlineOperation(r.Context(), owner, taskID, claim, []pptapp.AgentMessage{
		{Role: "user", Content: "已导入 Markdown 文件", CreatedAt: now},
		{Role: "assistant", Content: pptAgentOutlineAssistantMessage(outline), CreatedAt: now},
	}, outline, sourceFileIDs)
	if err != nil {
		recovered, ok, recoveryErr := recoverPPTAgentCompletion(state, owner, taskID, claim)
		if ok {
			writeJSON(w, pptTaskResponse(recovered))
			return
		}
		if errors.Is(recoveryErr, errPPTAgentOperationCleanup) {
			writePPTAgentOperationCleanupError(w)
			return
		}
		if recoveryErr != nil {
			writePPTAgentStateError(w, recoveryErr)
			return
		}
		writePPTAgentStateError(w, err)
		return
	}
	writeJSON(w, pptTaskResponse(updated))
}

func (a api) revisePPTSessionSlide(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED", "缺少 Idempotency-Key"))
		return
	}
	if len(idempotencyKey) > 256 || strings.ContainsAny(idempotencyKey, "\r\n") {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_INVALID", "Idempotency-Key 无效"))
		return
	}
	var req pptAgentReviseSlideRequest
	if err := decodePPTAgentJSON(r, &req); err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	req.SlideID = strings.TrimSpace(req.SlideID)
	req.Instruction = normalizePPTAgentText(req.Instruction, 0)
	if req.SlideID == "" || len(req.SlideID) > 256 || req.Instruction == "" || len([]rune(req.Instruction)) > pptAgentMaxMessageRunes {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "单页修订请求无效"))
		return
	}
	if err := a.checkMiniProgramText(r.Context(), r, user, req.Instruction); err != nil {
		writeContentSecurityError(w, err)
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	state := pptAgentStateForRequest(a, r)
	owner, err := pptOwnerForCapability(a.store, user)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	task, err := state.GetTask(r.Context(), owner, taskID)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	skill, ok := skills.Resolve(strings.TrimSpace(task.SkillCode))
	if !ok {
		writePPTAgentError(w, newPPTAgentError(http.StatusNotFound, "PPT_SKILL_NOT_FOUND", "未找到指定的 PPT Skill"))
		return
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, req.Instruction, "", boundedPPTAgentSlideCount(task.SlideCount, skill.MaxSlides), false, len(task.SourceFileIDs) > 0)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "当前账户无法使用该 PPT 能力"))
		return
	}
	tenantContext, err := pptAgentTenantContextFromCapability(capability)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	if !pptAgentTaskMatchesTenantContext(task, tenantContext) {
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_TENANT_CONTEXT_MISMATCH", "当前租户上下文与 PPT 会话不匹配"))
		return
	}
	if task.Stage != pptapp.StageReady {
		writePPTAgentStateError(w, pptapp.ErrInvalidStage)
		return
	}
	if _, found := pptAgentSlideByID(task, req.SlideID); !found {
		writePPTAgentStateError(w, pptapp.ErrTaskNotFound)
		return
	}
	scope := pptAgentRevisionScopeForTask(task)
	requestHash := pptAgentRevisionHash(scope, taskID, req.SlideID, req.Instruction)
	claim, claimedTask, err := state.BeginOperation(r.Context(), owner, taskID, scope, idempotencyKey, requestHash)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	if claim.CompletedReplay {
		writeJSON(w, pptTaskResponse(claimedTask))
		return
	}
	target, found := pptAgentSlideByID(claimedTask, req.SlideID)
	if !found {
		writePPTRevisionFailure(w, state, owner, taskID, claim, newPPTAgentError(http.StatusNotFound, "PPT_TASK_NOT_FOUND", "PPT 页面不存在"))
		return
	}
	response, err := pptAgentChatForRequest(a, r)(r.Context(), buildPPTAgentSlideRevisionRequest(target, req.Instruction, capability.Model))
	if err != nil {
		writePPTRevisionFailure(w, state, owner, taskID, claim, newPPTAgentError(http.StatusBadGateway, "PPT_AGENT_PROVIDER_UNAVAILABLE", "PPT Agent 服务暂时不可用，请稍后重试"))
		return
	}
	revision, err := parsePPTAgentSlideRevision(response.Message.Content)
	if err != nil {
		writePPTRevisionFailure(w, state, owner, taskID, claim, newPPTAgentError(http.StatusBadGateway, "PPT_AGENT_RESPONSE_INVALID", "PPT Agent 返回了无效的单页内容"))
		return
	}
	updated, err := state.CompleteSlideRevision(r.Context(), owner, taskID, claim, req.SlideID, revision)
	if err != nil {
		recovered, ok, recoveryErr := recoverPPTAgentCompletion(state, owner, taskID, claim)
		if ok {
			writeJSON(w, pptTaskResponse(recovered))
			return
		}
		if errors.Is(recoveryErr, errPPTAgentOperationCleanup) {
			writePPTAgentOperationCleanupError(w)
			return
		}
		if recoveryErr != nil {
			writePPTAgentStateError(w, recoveryErr)
			return
		}
		writePPTAgentStateError(w, err)
		return
	}
	writeJSON(w, pptTaskResponse(updated))
}

func pptAgentStateForRequest(a api, r *http.Request) pptAgentStateStore {
	if state, ok := r.Context().Value(pptAgentStateContextKey{}).(pptAgentStateStore); ok && state != nil {
		return state
	}
	return pptAgentServiceAdapter{service: a.pptService}
}

func pptAgentChatForRequest(a api, r *http.Request) pptAgentChatFunc {
	if chat, ok := r.Context().Value(pptAgentChatContextKey{}).(pptAgentChatFunc); ok && chat != nil {
		return chat
	}
	return func(ctx context.Context, req generation.CreateRequest) (chatprovider.Response, error) {
		provider := chatprovider.NewOpenAICompatibleForModel(a.cfg, req.Model)
		return provider.Chat(ctx, req)
	}
}

func pptAgentFileStoreForRequest(a api, r *http.Request) pptAgentFileStore {
	if store, ok := r.Context().Value(pptAgentFileStoreContextKey{}).(pptAgentFileStore); ok && store != nil {
		return store
	}
	if a.fileService == nil {
		return nil
	}
	return a.fileService
}

func pptAgentMarkdownParserForRequest(r *http.Request) pptAgentMarkdownParseFunc {
	if parse, ok := r.Context().Value(pptAgentMarkdownParserContextKey{}).(pptAgentMarkdownParseFunc); ok && parse != nil {
		return parse
	}
	return func(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
		return (parser.Markdown{}).Parse(ctx, source)
	}
}

func buildPPTAgentChatRequest(skill skills.Skill, task pptapp.Task, currentMessage, model string) generation.CreateRequest {
	return buildPPTAgentChatRequestWithLimit(skill, task, currentMessage, model, pptAgentMaxMessageRunes)
}

func buildPPTAgentChatRequestWithLimit(skill skills.Skill, task pptapp.Task, currentMessage, model string, currentMessageMaxRunes int) generation.CreateRequest {
	messages := make([]chatprovider.Message, 0, pptAgentRecentMessages+3)
	systemPrompt := strings.Join([]string{
		strings.TrimSpace(skill.SystemPrompt),
		"Return one JSON object only. Do not use Markdown or commentary.",
		"The response must conform exactly to this server schema: " + strings.TrimSpace(skill.OutlineSchema),
	}, "\n")
	messages = append(messages, chatprovider.Message{Role: "system", Content: systemPrompt})
	recent := task.AgentMessages
	if len(recent) > pptAgentRecentMessages {
		recent = recent[len(recent)-pptAgentRecentMessages:]
	}
	for _, message := range recent {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := normalizePPTAgentText(message.Content, pptAgentRecentMessageRunes)
		if content != "" {
			messages = append(messages, chatprovider.Message{Role: role, Content: content})
		}
	}
	if task.Outline != nil {
		if raw, err := json.Marshal(task.Outline); err == nil {
			messages = append(messages, chatprovider.Message{Role: "assistant", Content: "Current outline JSON for reference: " + string(raw)})
		}
	}
	messages = append(messages, chatprovider.Message{Role: "user", Content: normalizePPTAgentText(currentMessage, currentMessageMaxRunes)})
	return generation.CreateRequest{
		Type: "CHAT_COMPLETION", Model: strings.TrimSpace(model),
		Params: map[string]any{
			"messages": messages, "temperature": 0.2,
			"max_tokens":      maxPPTOutlineTokens(task.SlideCount),
			"response_format": map[string]string{"type": "json_object"},
		},
	}
}

func buildPPTAgentSlideRevisionRequest(slide pptapp.Slide, instruction, model string) generation.CreateRequest {
	slide = pptapp.NormalizeSlideIR(slide)
	contentBlocks := make([]pptapp.SlideBlock, 0, len(slide.Blocks))
	for _, block := range slide.Blocks {
		if block.Type != "image" && block.Type != "note" {
			contentBlocks = append(contentBlocks, block)
		}
	}
	payload, _ := json.Marshal(struct {
		SlideID     string              `json:"slideId"`
		Page        int                 `json:"page"`
		Blocks      []pptapp.SlideBlock `json:"blocks"`
		Instruction string              `json:"instruction"`
	}{
		SlideID: slide.ID, Page: slide.Page, Blocks: contentBlocks,
		Instruction: normalizePPTAgentText(instruction, pptAgentMaxMessageRunes),
	})
	return generation.CreateRequest{
		Type: "CHAT_COMPLETION", Model: strings.TrimSpace(model),
		Params: map[string]any{
			"messages": []chatprovider.Message{
				{Role: "system", Content: "Revise exactly one PPT slide. Return one JSON object only with schema {\"blocks\":[...]}. Allowed block types are title, subtitle, paragraph, and bullets. Do not return image or note blocks, slide coordinates, URLs, Markdown, or commentary."},
				{Role: "user", Content: string(payload)},
			},
			"temperature":     0.2,
			"max_tokens":      1800,
			"response_format": map[string]string{"type": "json_object"},
		},
	}
}

func parsePPTAgentSlideRevision(content string) (pptapp.Slide, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > 1<<20 {
		return pptapp.Slide{}, errors.New("invalid slide revision payload")
	}
	var payload pptAgentSlideRevisionPayload
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return pptapp.Slide{}, err
	}
	if err := requirePPTAgentJSONEOF(decoder); err != nil {
		return pptapp.Slide{}, err
	}
	if len(payload.Blocks) == 0 || len(payload.Blocks) > 12 {
		return pptapp.Slide{}, errors.New("invalid slide revision block count")
	}
	blocks := make([]pptapp.SlideBlock, 0, len(payload.Blocks))
	for _, block := range payload.Blocks {
		block.Type = strings.ToLower(strings.TrimSpace(block.Type))
		switch block.Type {
		case "title":
			block.Text = normalizePPTAgentText(block.Text, 200)
			if block.Text == "" || len(block.Items) != 0 || strings.TrimSpace(block.ImageRef) != "" {
				return pptapp.Slide{}, errors.New("invalid title block")
			}
		case "subtitle", "paragraph":
			block.Text = normalizePPTAgentText(block.Text, 2000)
			if block.Text == "" || len(block.Items) != 0 || strings.TrimSpace(block.ImageRef) != "" {
				return pptapp.Slide{}, errors.New("invalid text block")
			}
		case "bullets":
			if strings.TrimSpace(block.Text) != "" || strings.TrimSpace(block.ImageRef) != "" || len(block.Items) == 0 || len(block.Items) > 12 {
				return pptapp.Slide{}, errors.New("invalid bullets block")
			}
			block.Items = normalizePPTAgentStringList(block.Items, 12, 500)
			if len(block.Items) == 0 {
				return pptapp.Slide{}, errors.New("empty bullets block")
			}
		default:
			return pptapp.Slide{}, errors.New("unsupported slide revision block")
		}
		block.ImageRef = ""
		blocks = append(blocks, block)
	}
	return pptapp.NormalizeSlideIR(pptapp.Slide{Blocks: blocks}), nil
}

func pptAgentSlideByID(task pptapp.Task, slideID string) (pptapp.Slide, bool) {
	slideID = strings.TrimSpace(slideID)
	for _, slide := range task.Slides {
		if strings.TrimSpace(slide.ID) == slideID {
			return slide, true
		}
	}
	return pptapp.Slide{}, false
}

func validatePPTAgentSourceFileIDs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > pptAgentMaxSourceFiles {
		return nil, newPPTAgentError(http.StatusBadRequest, "PPT_SOURCE_FILE_NOT_FOUND", "请选择 1 至 3 个 Markdown 文件")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if len(value) > 256 || !pptAgentFileIDPattern.MatchString(value) {
			return nil, newPPTAgentError(http.StatusBadRequest, "PPT_SOURCE_FILE_NOT_FOUND", "文件标识无效")
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, newPPTAgentError(http.StatusBadRequest, "PPT_SOURCE_FILE_NOT_FOUND", "请选择 Markdown 文件")
	}
	return result, nil
}

type pptAgentBufferedSource struct {
	file    storagecenter.FileObject
	content []byte
}

type pptAgentSourceReadBudget struct {
	rawRunes           int
	normalizedRunes    int
	normalizedHasText  bool
	normalizedSawSpace bool
}

func readPPTAgentMarkdownFiles(ctx context.Context, store pptAgentFileStore, parse pptAgentMarkdownParseFunc, access storagecenter.AccessContext, sourceFileIDs []string) (string, error) {
	buffered := make([]pptAgentBufferedSource, 0, len(sourceFileIDs))
	budget := pptAgentSourceReadBudget{}
	for _, fileID := range sourceFileIDs {
		file, stream, err := store.OpenObject(ctx, access, fileID)
		if err != nil {
			if stream != nil {
				if closeErr := stream.Close(); closeErr != nil {
					return "", pptAgentSourceStorageError()
				}
			}
			return "", pptAgentSourceOpenError(err)
		}
		if stream == nil {
			return "", pptAgentSourceStorageError()
		}
		if metadataErr := validatePPTAgentMarkdownFile(file, access, fileID, time.Now().UTC()); metadataErr != nil {
			if closeErr := stream.Close(); closeErr != nil {
				return "", pptAgentSourceStorageError()
			}
			return "", metadataErr
		}
		raw, readErr := readPPTAgentSourceStream(stream, &budget)
		closeErr := stream.Close()
		if closeErr != nil {
			return "", pptAgentSourceStorageError()
		}
		if readErr != nil {
			return "", readErr
		}
		if !containsPPTAgentNonSpaceRune(raw) {
			return "", newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "Markdown 文件内容无效")
		}
		buffered = append(buffered, pptAgentBufferedSource{file: file, content: raw})
	}

	parts := make([]string, 0, len(sourceFileIDs))
	totalRunes := 0
	for _, source := range buffered {
		file := source.file
		raw := source.content
		units, err := parse(ctx, knowledgeapp.SourceDocument{Name: file.OriginalName, MIMEType: file.MIMEType, Content: raw})
		if err != nil || len(units) == 0 {
			return "", newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "Markdown 文件无法解析")
		}
		for _, unit := range units {
			part := normalizePPTAgentText(strings.TrimSpace(unit.Title)+"\n"+strings.TrimSpace(unit.Content), 0)
			if part == "" {
				continue
			}
			separatorRunes := 0
			if len(parts) > 0 {
				separatorRunes = 2
			}
			if totalRunes+separatorRunes+len([]rune(part)) > pptAgentMaxSourceTextRunes {
				return "", newPPTAgentError(http.StatusRequestEntityTooLarge, "PPT_SOURCE_TEXT_TOO_LARGE", "合并后的 Markdown 文本不能超过 200,000 个字符")
			}
			parts = append(parts, part)
			totalRunes += separatorRunes + len([]rune(part))
		}
	}
	if len(parts) == 0 {
		return "", newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "Markdown 文件内容为空")
	}
	return strings.Join(parts, "\n\n"), nil
}

func readPPTAgentSourceStream(stream io.Reader, budget *pptAgentSourceReadBudget) ([]byte, error) {
	reader := bufio.NewReader(io.LimitReader(stream, pptAgentMaxSourceFileBytes+1))
	var content bytes.Buffer
	fileBytes := int64(0)
	firstRune := true
	if budget.normalizedHasText {
		budget.normalizedSawSpace = true
	}
	for {
		value, size, err := reader.ReadRune()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, pptAgentSourceStorageError()
		}
		fileBytes += int64(size)
		if fileBytes > pptAgentMaxSourceFileBytes {
			return nil, newPPTAgentError(http.StatusRequestEntityTooLarge, "PPT_SOURCE_FILE_TOO_LARGE", "单个来源文件不能超过 10 MiB")
		}
		if value == utf8.RuneError && size == 1 {
			return nil, newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "Markdown 文件编码无效")
		}
		if _, writeErr := content.WriteRune(value); writeErr != nil {
			return nil, pptAgentSourceStorageError()
		}
		if firstRune && value == '\ufeff' {
			firstRune = false
			continue
		}
		firstRune = false
		budget.rawRunes++
		if budget.rawRunes > pptAgentMaxSourceTextRunes {
			return nil, newPPTAgentError(http.StatusRequestEntityTooLarge, "PPT_SOURCE_TEXT_TOO_LARGE", "合并后的 Markdown 文本不能超过 200,000 个字符")
		}
		if unicode.IsSpace(value) {
			if budget.normalizedHasText {
				budget.normalizedSawSpace = true
			}
			continue
		}
		if budget.normalizedHasText && budget.normalizedSawSpace {
			budget.normalizedRunes++
		}
		budget.normalizedRunes++
		budget.normalizedHasText = true
		budget.normalizedSawSpace = false
		if budget.normalizedRunes > pptAgentMaxSourceTextRunes {
			return nil, newPPTAgentError(http.StatusRequestEntityTooLarge, "PPT_SOURCE_TEXT_TOO_LARGE", "合并后的 Markdown 文本不能超过 200,000 个字符")
		}
	}
	return append([]byte(nil), content.Bytes()...), nil
}

func containsPPTAgentNonSpaceRune(raw []byte) bool {
	for len(raw) > 0 {
		value, size := utf8.DecodeRune(raw)
		if value == utf8.RuneError && size == 1 {
			return false
		}
		raw = raw[size:]
		if value != '\ufeff' && !unicode.IsSpace(value) {
			return true
		}
	}
	return false
}

func validatePPTAgentMarkdownFile(file storagecenter.FileObject, access storagecenter.AccessContext, requestedFileID string, now time.Time) error {
	if strings.TrimSpace(file.FileID) != strings.TrimSpace(requestedFileID) {
		return newPPTAgentError(http.StatusForbidden, "PPT_SOURCE_FILE_FORBIDDEN", "无权读取来源文件")
	}
	if !strings.EqualFold(strings.TrimSpace(file.Status), storagecenter.StatusActive) {
		return newPPTAgentError(http.StatusNotFound, "PPT_SOURCE_FILE_NOT_FOUND", "来源文件不存在")
	}
	if file.ExpiresAt != nil && !file.ExpiresAt.After(now) {
		return newPPTAgentError(http.StatusNotFound, "PPT_SOURCE_FILE_NOT_FOUND", "来源文件不存在")
	}
	if strings.TrimSpace(file.TenantID) != strings.TrimSpace(access.TenantID) {
		return newPPTAgentError(http.StatusForbidden, "PPT_SOURCE_FILE_FORBIDDEN", "无权读取来源文件")
	}
	visibility := strings.ToUpper(strings.TrimSpace(file.Visibility))
	switch visibility {
	case "PRIVATE":
		if strings.TrimSpace(file.UserID) != strings.TrimSpace(access.UserID) {
			return newPPTAgentError(http.StatusForbidden, "PPT_SOURCE_FILE_FORBIDDEN", "无权读取来源文件")
		}
	case "TENANT", "SHARED", "PUBLIC", "SYSTEM":
	default:
		return newPPTAgentError(http.StatusForbidden, "PPT_SOURCE_FILE_FORBIDDEN", "无权读取来源文件")
	}
	if file.FileSize > pptAgentMaxSourceFileBytes {
		return newPPTAgentError(http.StatusRequestEntityTooLarge, "PPT_SOURCE_FILE_TOO_LARGE", "单个来源文件不能超过 10 MiB")
	}
	name := strings.TrimSpace(file.OriginalName)
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "仅支持 Markdown 文件")
	}
	lowerName := strings.ToLower(name)
	extension := ""
	switch {
	case strings.HasSuffix(lowerName, ".markdown"):
		extension = "markdown"
	case strings.HasSuffix(lowerName, ".md"):
		extension = "md"
	default:
		return newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "仅支持 Markdown 文件")
	}
	stem := strings.TrimSuffix(lowerName, "."+extension)
	if strings.TrimSpace(stem) == "" || strings.Contains(stem, ".") || strings.ToLower(strings.TrimPrefix(strings.TrimSpace(file.Extension), ".")) != extension {
		return newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "Markdown 文件扩展名不一致")
	}
	mediaType, _, err := mime.ParseMediaType(strings.ToLower(strings.TrimSpace(file.MIMEType)))
	if err != nil || (mediaType != "text/markdown" && mediaType != "text/x-markdown") {
		return newPPTAgentError(http.StatusUnsupportedMediaType, "PPT_SOURCE_FILE_TYPE_UNSUPPORTED", "Markdown 文件 MIME 类型不受支持")
	}
	return nil
}

func pptAgentSourceStorageError() error {
	return newPPTAgentError(http.StatusServiceUnavailable, "PPT_SESSION_STORAGE_UNAVAILABLE", "PPT 文件存储暂时不可用")
}

func pptAgentSourceOpenError(err error) error {
	switch {
	case errors.Is(err, storagecenter.ErrFileForbidden):
		return newPPTAgentError(http.StatusForbidden, "PPT_SOURCE_FILE_FORBIDDEN", "无权读取来源文件")
	case errors.Is(err, storagecenter.ErrFileNotFound), errors.Is(err, storagecenter.ErrFileExpired), errors.Is(err, storagecenter.ErrFileQuarantined):
		return newPPTAgentError(http.StatusNotFound, "PPT_SOURCE_FILE_NOT_FOUND", "来源文件不存在")
	default:
		return newPPTAgentError(http.StatusServiceUnavailable, "PPT_SESSION_STORAGE_UNAVAILABLE", "PPT 文件存储暂时不可用")
	}
}

func pptAgentImportHash(task pptapp.Task, sourceFileIDs []string) string {
	contextValue := strings.Join([]string{
		strings.TrimSpace(task.TenantID), strings.TrimSpace(task.OrganizationID), strings.ToUpper(strings.TrimSpace(task.ContextType)),
		strings.ToUpper(strings.TrimSpace(task.BillingScope)), strings.TrimSpace(task.BillingAccountID),
	}, "\x00")
	sum := sha256.Sum256([]byte(pptAgentImportScope + "\x00" + contextValue + "\x00" + strings.Join(sourceFileIDs, "\x00")))
	return hex.EncodeToString(sum[:])
}

func pptAgentRevisionScopeForTask(task pptapp.Task) string {
	contextValue := strings.Join([]string{
		strings.TrimSpace(task.UserID), strings.TrimSpace(task.TenantID), strings.TrimSpace(task.OrganizationID),
		strings.ToUpper(strings.TrimSpace(task.ContextType)), strings.ToUpper(strings.TrimSpace(task.BillingScope)),
		strings.TrimSpace(task.BillingAccountID),
	}, "\x00")
	sum := sha256.Sum256([]byte(contextValue))
	return pptAgentRevisionScope + ":" + hex.EncodeToString(sum[:])
}

func pptAgentRevisionHash(scope, taskID, slideID, instruction string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(scope), strings.TrimSpace(taskID), strings.TrimSpace(slideID), normalizePPTAgentText(instruction, 0),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func writePPTImportFailure(w http.ResponseWriter, state pptAgentStateStore, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, publicErr error) {
	var agentErr pptAgentError
	errorCode := "PPT_SESSION_STORAGE_UNAVAILABLE"
	if errors.As(publicErr, &agentErr) {
		errorCode = agentErr.code
	}
	if failErr := failPPTAgentOperation(state, owner, taskID, claim, errorCode); failErr != nil {
		writePPTAgentOperationCleanupError(w)
		return
	}
	writePPTAgentError(w, publicErr)
}

func writePPTRevisionFailure(w http.ResponseWriter, state pptAgentStateStore, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, publicErr error) {
	var agentErr pptAgentError
	errorCode := "PPT_AGENT_RESPONSE_INVALID"
	if errors.As(publicErr, &agentErr) {
		errorCode = agentErr.code
	}
	if failErr := failPPTAgentOperation(state, owner, taskID, claim, errorCode); failErr != nil {
		writePPTAgentOperationCleanupError(w)
		return
	}
	writePPTAgentError(w, publicErr)
}

func parsePPTAgentOutline(content string, taskSlideCount int, skill skills.Skill) (pptapp.Outline, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return pptapp.Outline{}, errors.New("empty agent response")
	}
	if len(content) > 1<<20 {
		return pptapp.Outline{}, errors.New("agent response is too large")
	}
	var payload pptAgentOutlinePayload
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return pptapp.Outline{}, err
	}
	if err := requirePPTAgentJSONEOF(decoder); err != nil {
		return pptapp.Outline{}, err
	}
	payload.Title = normalizePPTAgentText(payload.Title, 200)
	maxSlides := skill.MaxSlides
	if taskSlideCount > 0 && (maxSlides <= 0 || taskSlideCount < maxSlides) {
		maxSlides = taskSlideCount
	}
	if maxSlides <= 0 {
		maxSlides = pptAgentDefaultSlideCount
	}
	if payload.Title == "" || len(payload.Pages) == 0 || len(payload.Pages) > maxSlides {
		return pptapp.Outline{}, errors.New("agent outline exceeds the approved shape")
	}
	slides := make([]pptapp.OutlineSlide, 0, len(payload.Pages))
	for index, page := range payload.Pages {
		page.Title = normalizePPTAgentText(page.Title, 200)
		page.Summary = normalizePPTAgentText(page.Summary, 2000)
		page.Bullets = normalizePPTAgentStringList(page.Bullets, 12, 500)
		if page.Title == "" || page.Summary == "" || len(page.Bullets) == 0 {
			return pptapp.Outline{}, errors.New("agent outline page is incomplete")
		}
		layout := "content"
		if len(skill.PreferredLayouts) > 0 {
			layout = skill.PreferredLayouts[index%len(skill.PreferredLayouts)]
		}
		slides = append(slides, pptapp.OutlineSlide{
			Page: index + 1, Title: page.Title, Summary: page.Summary,
			BulletPoints: page.Bullets, Layout: layout, SlideType: defaultPPTSlideType(index+1, len(payload.Pages)),
		})
	}
	return pptapp.Outline{Title: payload.Title, Slides: slides, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func pptTaskResponse(task pptapp.Task) pptTaskPublicResponse {
	task = pptapp.NormalizeTask(task)
	response := pptTaskPublicResponse{
		TaskID: task.TaskID, SessionID: task.SessionID, Type: task.Type, MediaType: task.MediaType,
		SkillCode: task.SkillCode, Stage: task.Stage, Status: task.Status, Title: task.Title, Prompt: task.Prompt,
		SlideCount: task.SlideCount, Language: task.Language, Tone: task.Tone, TextContent: task.TextContent,
		Audience: task.Audience, Scenario: task.Scenario, GenerationAspectRatio: task.GenerationAspectRatio,
		Theme: task.Theme, AutoThemeEnabled: task.AutoThemeEnabled, EnableWebSearch: task.EnableWebSearch,
		ImageSource: task.ImageSource, ImageStyle: task.ImageStyle, PeopleStyle: task.PeopleStyle,
		ImageLighting: task.ImageLighting, ImageComposition: task.ImageComposition, TextInImage: task.TextInImage,
		Progress: task.Progress, CurrentPage: task.CurrentPage, VisualProgress: task.VisualProgress,
		Outline: task.Outline, SourceFileIDs: append([]string(nil), task.SourceFileIDs...),
		OutlineConfirmedAt: task.OutlineConfirmedAt, GenerationStartedAt: task.GenerationStartedAt,
		CompletedAt: task.CompletedAt, ErrorCode: safePPTAgentTaskErrorCode(task.ErrorCode), PPTURL: task.PPTURL, PDFURL: task.PDFURL,
		ErrorMessage: safePPTAgentTaskError(task), CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
	}
	for _, message := range task.AgentMessages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		switch role {
		case "user":
			message.Role = role
			message.Content = normalizePPTAgentText(message.Content, pptAgentMaxMessageRunes)
			if message.Content == "" {
				continue
			}
			response.AgentMessages = append(response.AgentMessages, message)
		case "assistant":
			response.AgentMessages = append(response.AgentMessages, pptapp.AgentMessage{
				Role: "assistant", Content: pptAgentOutlineAssistantMessageValue(task.Outline), CreatedAt: message.CreatedAt,
			})
		}
	}
	for _, slide := range task.Slides {
		response.Slides = append(response.Slides, pptAgentSlideResponseFromTask(slide))
	}
	return response
}

func pptAgentOutlineAssistantMessage(outline pptapp.Outline) string {
	return pptAgentOutlineAssistantMessageValue(&outline)
}

func pptAgentOutlineAssistantMessageValue(outline *pptapp.Outline) string {
	if outline != nil {
		if title := normalizePPTAgentText(outline.Title, 200); title != "" {
			return "已生成大纲：" + title
		}
	}
	return "PPT 内容已更新"
}

func pptAgentSlideResponseFromTask(slide pptapp.Slide) pptAgentSlideResponse {
	slide = projectPPTSlideForHTTP(slide)
	response := pptAgentSlideResponse{
		ID: slide.ID, Page: slide.Page, Title: slide.Title, Content: slide.Content,
		BulletPoints: append([]string(nil), slide.BulletPoints...), ImageURL: slide.ImageURL,
		VisualStorageRef: slide.VisualStorageRef, Layout: slide.Layout, SpeakerNotes: slide.SpeakerNotes,
		Blocks: append([]pptapp.SlideBlock(nil), slide.Blocks...), SlideType: slide.SlideType,
		VisualCreatedAt: slide.VisualCreatedAt, VisualStatus: slide.VisualStatus,
	}
	if strings.TrimSpace(slide.VisualError) != "" {
		response.VisualError = "配图生成失败，请重试"
	}
	if slide.VisualPlan != nil {
		response.VisualPlan = &pptAgentVisualPlanResponse{
			VisualType: slide.VisualPlan.VisualType, ImageRequired: slide.VisualPlan.ImageRequired,
			ChartRequired: slide.VisualPlan.ChartRequired, DiagramRequired: slide.VisualPlan.DiagramRequired,
			TextInImage: slide.VisualPlan.TextInImage, Subject: slide.VisualPlan.Subject, Scene: slide.VisualPlan.Scene,
			Action: slide.VisualPlan.Action, Objects: append([]string(nil), slide.VisualPlan.Objects...),
			Mood: slide.VisualPlan.Mood, Composition: slide.VisualPlan.Composition, Style: slide.VisualPlan.Style,
		}
	}
	for _, asset := range slide.VisualHistory {
		response.VisualHistory = append(response.VisualHistory, pptAgentVisualAssetResponse{
			URL: asset.URL, StorageRef: asset.StorageRef, CreatedAt: asset.CreatedAt,
		})
	}
	return response
}

func safePPTAgentTaskError(task pptapp.Task) string {
	if strings.TrimSpace(task.ErrorMessage) == "" {
		return ""
	}
	switch strings.TrimSpace(task.ErrorCode) {
	case "PPT_AGENT_PROVIDER_UNAVAILABLE":
		return "PPT Agent 服务暂时不可用，请稍后重试"
	case "PPT_AGENT_RESPONSE_INVALID":
		return "PPT Agent 返回了无效的大纲"
	default:
		return "PPT 生成失败，请稍后重试"
	}
}

func safePPTAgentTaskErrorCode(value string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	if _, approved := pptAgentPublicErrorCodes[code]; approved {
		return code
	}
	return ""
}

var pptAgentPublicErrorCodes = map[string]struct{}{
	"PPT_SKILL_NOT_FOUND":              {},
	"PPT_INVALID_STAGE":                {},
	"PPT_IDEMPOTENCY_KEY_REQUIRED":     {},
	"PPT_IDEMPOTENCY_KEY_INVALID":      {},
	"PPT_IDEMPOTENCY_CONFLICT":         {},
	"PPT_OPERATION_IN_PROGRESS":        {},
	"PPT_OPERATION_CLEANUP_FAILED":     {},
	"PPT_OPERATION_TOKEN_MISMATCH":     {},
	"PPT_OUTLINE_REQUIRED":             {},
	"PPT_SESSION_CANCELLED":            {},
	"PPT_GENERATION_RUN_MISMATCH":      {},
	"PPT_GENERATION_ALREADY_RUNNING":   {},
	"PPT_GENERATION_INCOMPLETE":        {},
	"PPT_SLIDE_COORDINATE_INVALID":     {},
	"PPT_SLIDE_COORDINATE_CONFLICT":    {},
	"PPT_BILLING_TASK_REQUIRED":        {},
	"PPT_BILLING_BINDING_MISSING":      {},
	"PPT_BILLING_BINDING_MISMATCH":     {},
	"PPT_BILLING_ALREADY_CAPTURED":     {},
	"PPT_AGENT_PROVIDER_UNAVAILABLE":   {},
	"PPT_AGENT_RESPONSE_INVALID":       {},
	"PPT_SOURCE_FILE_NOT_FOUND":        {},
	"PPT_SOURCE_FILE_FORBIDDEN":        {},
	"PPT_SOURCE_FILE_TYPE_UNSUPPORTED": {},
	"PPT_SOURCE_FILE_TOO_LARGE":        {},
	"PPT_SOURCE_TEXT_TOO_LARGE":        {},
	"PPT_BILLING_RESERVATION_FAILED":   {},
	"PPT_BILLING_FINALIZE_FAILED":      {},
	"PPT_TENANT_CONTEXT_UNAVAILABLE":   {},
	"PPT_TENANT_CONTEXT_MISMATCH":      {},
	"PPT_TASK_NOT_FOUND":               {},
	"PPT_POSTGRES_UNAVAILABLE":         {},
	"PPT_SESSION_STORAGE_UNAVAILABLE":  {},
}

func boundedPPTAgentSlideCount(value, skillMax int) int {
	if value <= 0 {
		value = pptAgentDefaultSlideCount
	}
	if skillMax > 0 && value > skillMax {
		value = skillMax
	}
	if value < 1 {
		return 1
	}
	return value
}

func normalizePPTAgentText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		return strings.TrimSpace(string(runes[:maxRunes]))
	}
	return value
}

func normalizePPTAgentStringList(values []string, maxItems, maxRunes int) []string {
	if maxItems > 0 && len(values) > maxItems {
		values = values[:maxItems]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = normalizePPTAgentText(value, maxRunes); value != "" {
			result = append(result, value)
		}
	}
	return result
}

type pptAgentTenantContext struct {
	TenantID         string
	OrganizationID   string
	ContextType      string
	BillingScope     string
	BillingAccountID string
}

func pptAgentTenantContextFromCapability(capability generation.CreateRequest) (pptAgentTenantContext, error) {
	context := pptAgentTenantContext{
		TenantID:         strings.TrimSpace(stringValue(capability.Params["tenant_id"])),
		OrganizationID:   strings.TrimSpace(stringValue(capability.Params["organization_id"])),
		ContextType:      strings.ToUpper(strings.TrimSpace(stringValue(capability.Params["context_type"]))),
		BillingScope:     strings.ToUpper(strings.TrimSpace(stringValue(capability.Params["billing_scope"]))),
		BillingAccountID: strings.TrimSpace(stringValue(capability.Params["billing_account_id"])),
	}
	if context.TenantID == "" || context.OrganizationID == "" || context.ContextType == "" || context.BillingScope == "" || context.BillingAccountID == "" {
		return pptAgentTenantContext{}, errors.New("incomplete server-resolved tenant context")
	}
	return context, nil
}

func pptAgentTaskMatchesTenantContext(task pptapp.Task, current pptAgentTenantContext) bool {
	return strings.TrimSpace(task.TenantID) == current.TenantID &&
		strings.TrimSpace(task.OrganizationID) == current.OrganizationID &&
		strings.EqualFold(strings.TrimSpace(task.ContextType), current.ContextType) &&
		strings.EqualFold(strings.TrimSpace(task.BillingScope), current.BillingScope) &&
		strings.TrimSpace(task.BillingAccountID) == current.BillingAccountID
}

func pptAgentMessageScopeForTask(task pptapp.Task) string {
	contextValue := strings.Join([]string{
		strings.TrimSpace(task.TenantID), strings.TrimSpace(task.OrganizationID),
		strings.ToUpper(strings.TrimSpace(task.ContextType)), strings.ToUpper(strings.TrimSpace(task.BillingScope)),
		strings.TrimSpace(task.BillingAccountID),
	}, "\x00")
	sum := sha256.Sum256([]byte(contextValue))
	return pptAgentMessageScope + ":" + hex.EncodeToString(sum[:])
}

func pptAgentMessageHash(scope, message string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(scope) + "\x00" + message))
	return hex.EncodeToString(sum[:])
}

func failPPTAgentOperation(state pptAgentStateStore, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, errorCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := state.FailOperation(ctx, owner, taskID, claim, errorCode)
	return err
}

func recoverPPTAgentCompletion(state pptAgentStateStore, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim) (pptapp.Task, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	task, err := state.GetTask(ctx, owner, taskID)
	if err != nil {
		return pptapp.Task{}, false, err
	}
	for index := len(task.IdempotencyRecords) - 1; index >= 0; index-- {
		record := task.IdempotencyRecords[index]
		if !strings.EqualFold(strings.TrimSpace(record.Scope), strings.TrimSpace(claim.Scope)) || strings.TrimSpace(record.Key) != strings.TrimSpace(claim.Key) {
			continue
		}
		if strings.TrimSpace(record.RequestHash) != strings.TrimSpace(claim.RequestHash) {
			return pptapp.Task{}, false, pptapp.ErrIdempotencyConflict
		}
		switch strings.ToLower(strings.TrimSpace(record.State)) {
		case "completed":
			if strings.TrimSpace(record.ResponseJSON) == "" {
				return task, true, nil
			}
			var snapshot pptapp.Task
			if json.Unmarshal([]byte(record.ResponseJSON), &snapshot) == nil && strings.TrimSpace(snapshot.TaskID) != "" {
				snapshot.UserID = task.UserID
				return pptapp.NormalizeTask(snapshot), true, nil
			}
			return task, true, nil
		case "processing":
			if strings.TrimSpace(record.OperationToken) != strings.TrimSpace(claim.OperationToken) {
				return pptapp.Task{}, false, pptapp.ErrOperationInProgress
			}
			if _, failErr := state.FailOperation(ctx, owner, taskID, claim, "PPT_SESSION_STORAGE_UNAVAILABLE"); failErr != nil {
				return pptapp.Task{}, false, errPPTAgentOperationCleanup
			}
			return pptapp.Task{}, false, nil
		default:
			return pptapp.Task{}, false, nil
		}
	}
	return pptapp.Task{}, false, nil

}

func writePPTAgentOperationCleanupError(w http.ResponseWriter) {
	writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_OPERATION_CLEANUP_FAILED", "PPT 操作状态清理失败，请稍后重试"))
}

func decodePPTAgentJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requirePPTAgentJSONEOF(decoder)
}

func requirePPTAgentJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func writePPTAgentStateError(w http.ResponseWriter, err error) {
	var agentErr pptAgentError
	if errors.As(err, &agentErr) {
		writePPTAgentError(w, agentErr)
		return
	}
	switch {
	case errors.Is(err, pptapp.ErrTaskNotFound):
		writePPTAgentError(w, newPPTAgentError(http.StatusNotFound, "PPT_TASK_NOT_FOUND", "PPT 会话不存在"))
	case errors.Is(err, pptapp.ErrPostgresUnavailable):
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_POSTGRES_UNAVAILABLE", "PPT 会话存储暂时不可用"))
	case errors.Is(err, pptapp.ErrInvalidPrompt):
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_PROMPT_REQUIRED", "请输入演示主题"))
	case errors.Is(err, pptapp.ErrIdempotencyKeyRequired):
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED", "缺少 Idempotency-Key"))
	case errors.Is(err, pptapp.ErrIdempotencyConflict):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_IDEMPOTENCY_CONFLICT", "同一幂等键对应了不同请求"))
	case errors.Is(err, pptapp.ErrOperationInProgress):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_OPERATION_IN_PROGRESS", "PPT 操作正在处理中，请稍后重试"))
	case errors.Is(err, pptapp.ErrInvalidStage):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_INVALID_STAGE", "当前 PPT 阶段不允许此操作"))
	case errors.Is(err, pptapp.ErrSessionCancelled):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_SESSION_CANCELLED", "PPT 会话已取消"))
	case errors.Is(err, pptapp.ErrOperationTokenMismatch):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_OPERATION_TOKEN_MISMATCH", "PPT 操作状态已变化，请重试"))
	case errors.Is(err, pptapp.ErrOutlineRequired):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_OUTLINE_REQUIRED", "请先生成并确认 PPT 大纲"))
	case errors.Is(err, pptapp.ErrGenerationAlreadyRunning):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_GENERATION_ALREADY_RUNNING", "PPT 正在生成中"))
	case errors.Is(err, pptapp.ErrGenerationRunMismatch):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_GENERATION_RUN_MISMATCH", "PPT 生成运行权已变化，请重试"))
	case errors.Is(err, pptapp.ErrGenerationIncomplete):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_GENERATION_INCOMPLETE", "PPT 正文页面尚未完成"))
	case errors.Is(err, pptapp.ErrBillingTaskRequired):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_BILLING_TASK_REQUIRED", "PPT 费用绑定状态异常，请重试"))
	case errors.Is(err, pptapp.ErrBillingBindingMissing):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_BILLING_BINDING_MISSING", "PPT 费用绑定状态异常，请重试"))
	case errors.Is(err, pptapp.ErrBillingBindingMismatch):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_BILLING_BINDING_MISMATCH", "PPT 费用绑定状态异常，请重试"))
	case errors.Is(err, pptapp.ErrBillingAlreadyCaptured):
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_BILLING_ALREADY_CAPTURED", "PPT 费用已确认，当前生成结果不能取消"))
	default:
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_SESSION_STORAGE_UNAVAILABLE", "PPT 会话存储暂时不可用"))
	}
}

func writePPTAgentError(w http.ResponseWriter, err error) {
	var agentErr pptAgentError
	if errors.As(err, &agentErr) {
		writeError(w, agentErr.status, agentErr)
		return
	}
	writeError(w, http.StatusInternalServerError, errors.New("PPT Agent request failed"))
}
