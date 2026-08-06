package ppt

import (
	"errors"
	"strings"
	"time"
)

const (
	maxAgentMessages              = 30
	maxIdempotencyRecords         = 64
	generationLeaseDuration       = 30 * time.Second
	operationProcessingStaleAfter = 5 * time.Minute
	idempotencyStateProcessing    = "processing"
	idempotencyStateCompleted     = "completed"
	idempotencyStateFailed        = "failed"
	idempotencyScopeConfirm       = "confirm-outline"
	idempotencyScopeCancel        = "cancel"
)

var (
	ErrPostgresUnavailable      = errors.New("PPT_POSTGRES_UNAVAILABLE")
	ErrIdempotencyKeyRequired   = errors.New("PPT_IDEMPOTENCY_KEY_REQUIRED")
	ErrIdempotencyConflict      = errors.New("PPT_IDEMPOTENCY_CONFLICT")
	ErrOperationInProgress      = errors.New("PPT_OPERATION_IN_PROGRESS")
	ErrInvalidStage             = errors.New("PPT_INVALID_STAGE")
	ErrOperationTokenMismatch   = errors.New("PPT_OPERATION_TOKEN_MISMATCH")
	ErrGenerationRunMismatch    = errors.New("PPT_GENERATION_RUN_MISMATCH")
	ErrGenerationAlreadyRunning = errors.New("PPT_GENERATION_ALREADY_RUNNING")
	ErrGenerationIncomplete     = errors.New("PPT_GENERATION_INCOMPLETE")
	ErrOutlineRequired          = errors.New("PPT_OUTLINE_REQUIRED")
	ErrSessionCancelled         = errors.New("PPT_SESSION_CANCELLED")
	ErrInvalidSlideCoordinate   = errors.New("PPT_SLIDE_COORDINATE_INVALID")
	ErrInvalidSlideIR           = errors.New("PPT_SLIDE_IR_INVALID")
	ErrSlideCoordinateConflict  = errors.New("PPT_SLIDE_COORDINATE_CONFLICT")
	ErrBillingTaskRequired      = errors.New("PPT_BILLING_TASK_REQUIRED")
	ErrBillingBindingMissing    = errors.New("PPT_BILLING_BINDING_MISSING")
	ErrBillingBindingMismatch   = errors.New("PPT_BILLING_BINDING_MISMATCH")
	ErrBillingAlreadyCaptured   = errors.New("PPT_BILLING_ALREADY_CAPTURED")
	ErrOwnerScopeRequired       = errors.New("PPT_OWNER_SCOPE_REQUIRED")
	ErrInvalidSkill             = errors.New("PPT_INVALID_SKILL")
)

type OwnerScope struct {
	TenantID string `json:"tenantId"`
	UserID   string `json:"userId"`
}

func (owner OwnerScope) Validated() (OwnerScope, error) {
	owner.TenantID = strings.TrimSpace(owner.TenantID)
	owner.UserID = strings.TrimSpace(owner.UserID)
	if owner.TenantID == "" || owner.UserID == "" {
		return OwnerScope{}, ErrOwnerScopeRequired
	}
	return owner, nil
}

type Stage string

const (
	StageDraft        Stage = "DRAFT"
	StageOutlineReady Stage = "OUTLINE_READY"
	StageGenerating   Stage = "GENERATING"
	StageReady        Stage = "READY"
	StageFailed       Stage = "FAILED"
	StageCancelled    Stage = "CANCELLED"
)

type AgentMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type GenerationLease struct {
	RunToken   string `json:"runToken"`
	LeaseUntil string `json:"leaseUntil"`
}

type IdempotencyRecord struct {
	Scope          string `json:"scope"`
	Key            string `json:"key"`
	RequestHash    string `json:"requestHash"`
	State          string `json:"state"`
	ResponseJSON   string `json:"responseJson"`
	ErrorCode      string `json:"errorCode,omitempty"`
	OperationToken string `json:"operationToken"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type SessionRequest struct {
	Owner            OwnerScope `json:"-"`
	OrganizationID   string     `json:"organizationId,omitempty"`
	ContextType      string     `json:"contextType,omitempty"`
	BillingScope     string     `json:"billingScope,omitempty"`
	BillingAccountID string     `json:"billingAccountId,omitempty"`
	ClientRequestID  string     `json:"clientRequestId,omitempty"`
	Prompt           string     `json:"prompt"`
	SkillCode        string     `json:"skillCode"`
	SourceFileIDs    []string   `json:"sourceFileIds"`
	SlideCount       int        `json:"slideCount"`
	Language         string     `json:"language"`
	Audience         string     `json:"audience"`
}

type OperationClaim struct {
	Scope           string `json:"scope"`
	Key             string `json:"key"`
	RequestHash     string `json:"requestHash"`
	OperationToken  string `json:"operationToken"`
	Replay          bool   `json:"replay"`
	CompletedReplay bool   `json:"completedReplay"`
	InFlight        bool   `json:"inFlight"`
}

type GenerationClaim struct {
	RunToken   string `json:"runToken"`
	LeaseUntil string `json:"leaseUntil"`
}

type CancelClaim struct {
	Key            string `json:"key"`
	RequestHash    string `json:"requestHash"`
	OperationToken string `json:"operationToken"`
	Replay         bool   `json:"replay"`
}

func StageStatus(stage Stage) string {
	switch stage {
	case StageGenerating:
		return StatusProcessing
	case StageReady:
		return StatusSuccess
	case StageFailed:
		return StatusFailed
	case StageCancelled:
		return StatusCancelled
	case StageDraft, StageOutlineReady:
		return StatusPending
	default:
		return ""
	}
}

func ValidateTaskStage(task Task) error {
	expected := StageStatus(task.Stage)
	if expected == "" || strings.TrimSpace(task.Status) != expected {
		return ErrInvalidStage
	}
	return nil
}

func normalizePPTSourceFileIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func NormalizeTask(task Task) Task {
	task = cloneTask(task)
	if strings.TrimSpace(task.SessionID) == "" {
		task.SessionID = strings.TrimSpace(task.TaskID)
	}
	for i := range task.Slides {
		slide := NormalizeSlideIR(task.Slides[i])
		if strings.TrimSpace(slide.SlideType) == "" {
			slide.SlideType = "text_image"
		}
		slide.SlideType = NormalizeSlideType(slide.SlideType)
		if slide.VisualPlan != nil {
			plan := NormalizeVisualPlan(*slide.VisualPlan, VisualPlannerInput{
				SlideType: slide.SlideType, SlideTitle: slideTitle(slide),
				CoreIdea: slideContent(slide), Layout: slide.Layout,
				ImagePosition: task.ImageComposition, ImageStyle: task.ImageStyle,
				PeopleStyle: task.PeopleStyle, ImageLighting: task.ImageLighting,
			})
			slide.VisualPlan = &plan
		}
		task.Slides[i] = slide
	}
	if task.SlideCount <= 0 {
		if task.Outline != nil && len(task.Outline.Slides) > 0 {
			task.SlideCount = len(task.Outline.Slides)
		} else if len(task.Slides) > 0 {
			task.SlideCount = len(task.Slides)
		}
	}
	switch task.Stage {
	case StageDraft, StageOutlineReady:
		task.Progress = 0
		task.CurrentPage = 0
	case StageGenerating:
		completed, _ := textSlidePageCoverage(task.Slides, task.SlideCount)
		if task.CurrentPage > completed {
			completed = task.CurrentPage
		}
		if task.SlideCount > 0 && completed > task.SlideCount {
			completed = task.SlideCount
		}
		task.CurrentPage = completed
		if task.SlideCount > 0 {
			computedProgress := completed * 100 / task.SlideCount
			if computedProgress > task.Progress {
				task.Progress = computedProgress
			}
		}
		if task.Progress < 0 {
			task.Progress = 0
		}
		if task.Progress > 100 {
			task.Progress = 100
		}
	case StageReady:
		task.Progress = 100
		task.CurrentPage = task.SlideCount
	}
	return task
}

func textSlidePageCoverage(slides []Slide, total int) (int, bool) {
	if total <= 0 {
		return 0, false
	}
	pages := make(map[int]struct{}, total)
	ids := make(map[string]struct{}, total)
	exact := len(slides) == total
	for _, slide := range slides {
		id := strings.TrimSpace(slide.ID)
		if id == "" || slide.Page < 1 || slide.Page > total {
			exact = false
			continue
		}
		if _, exists := pages[slide.Page]; exists {
			exact = false
			continue
		}
		if _, exists := ids[id]; exists {
			exact = false
			continue
		}
		pages[slide.Page] = struct{}{}
		ids[id] = struct{}{}
	}
	return len(pages), exact && len(pages) == total
}
