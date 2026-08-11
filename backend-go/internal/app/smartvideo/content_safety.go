package smartvideo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxPlanTitleRunes   = 80
	MaxPlanSummaryRunes = 500
	MaxSceneTitleRunes  = 80
	MaxNarrationRunes   = 500
	MaxChangeNoteRunes  = 200
)

var ErrContentSafetyRejected = errors.New("SMART_VIDEO_CONTENT_SAFETY_REJECTED")

// blockedContentMarkers are deterministic V1 safety markers used by tests and
// as a hard deny-list for clearly unsafe draft text. Production can wrap a
// stronger provider behind the same ValidateEditPlanContent entrypoint.
var blockedContentMarkers = []string{
	"blocked_content",
	"<script",
	"javascript:",
}

func ValidateEditPlanContent(plan EditPlanV1) error {
	if err := validateSafeText("title", plan.Title, MaxPlanTitleRunes, true); err != nil {
		return err
	}
	if err := validateSafeText("summary", plan.Summary, MaxPlanSummaryRunes, false); err != nil {
		return err
	}
	for i, scene := range plan.Scenes {
		prefix := "scene[" + strconv.Itoa(i) + "]"
		if err := validateSafeText(prefix+".title", scene.Title, MaxSceneTitleRunes, true); err != nil {
			return err
		}
		if err := validateSafeText(prefix+".narration", scene.Narration, MaxNarrationRunes, false); err != nil {
			return err
		}
	}
	return nil
}

func ValidateChangeNote(note string) error {
	return validateSafeText("changeNote", note, MaxChangeNoteRunes, false)
}

func validateSafeText(field, value string, maxRunes int, required bool) error {
	trimmed := strings.TrimSpace(value)
	if required && trimmed == "" {
		return &EditPlanValidationError{Code: "missing_" + field, Message: field + " is required"}
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		return &EditPlanValidationError{Code: "text_too_long", Message: field + " exceeds maximum length"}
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range blockedContentMarkers {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("%w: %s", ErrContentSafetyRejected, field)
		}
	}
	for _, r := range trimmed {
		if r == 0 || (r < 0x20 && r != '\n' && r != '\r' && r != '\t') {
			return fmt.Errorf("%w: %s", ErrContentSafetyRejected, field)
		}
	}
	return nil
}
