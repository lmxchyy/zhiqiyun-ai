package ppt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	EditStageAccepted          = "EDIT_ACCEPTED"
	EditStagePlanned           = "EDIT_PLANNED"
	EditStageContentUpdated    = "CONTENT_UPDATED"
	EditStageAssetsUpdated     = "ASSETS_UPDATED"
	EditStageLayoutUpdated     = "LAYOUT_UPDATED"
	EditStageQualityChecked    = "QUALITY_CHECKED"
	EditStageRendered          = "RENDERED"
	EditStageRevisionCommitted = "REVISION_COMMITTED"
)

type DurableEditCheckpoint struct {
	RequestID    string          `json:"requestId"`
	Message      string          `json:"message,omitempty"`
	Command      *EditCommand    `json:"command,omitempty"`
	BaseRevision int             `json:"baseRevision"`
	Stage        string          `json:"stage"`
	PreparedDeck json.RawMessage `json:"preparedDeck,omitempty"`
	RenderBytes  []byte          `json:"renderBytes,omitempty"`
	FileID       string          `json:"fileId,omitempty"`
}

const (
	EditCommandUpdateText   = "UPDATE_TEXT"
	EditCommandRegenerate   = "REGENERATE_SLIDE"
	EditCommandChangeLayout = "CHANGE_LAYOUT"
	EditCommandReplaceImage = "REPLACE_IMAGE"
	EditCommandMoveSlide    = "MOVE_SLIDE"
	EditCommandAddSlide     = "ADD_SLIDE"
	EditCommandDeleteSlide  = "DELETE_SLIDE"
)

var (
	ErrEditStaleRevision       = errors.New("ppt v2 edit base revision is stale")
	ErrEditTargetNotFound      = errors.New("ppt v2 edit target is not found")
	ErrEditInvalidCommand      = errors.New("ppt v2 edit command is invalid")
	ErrEditUnsupported         = errors.New("ppt v2 edit command is not supported")
	ErrEditNoUndo              = errors.New("ppt v2 edit has no undoable revision")
	ErrEditProviderUnavailable = errors.New("ppt v2 edit planning provider is unavailable")
)

type EditPlanningInput struct {
	Message string
	State   AgentPlanningState
}

type EditCommandDraft struct {
	Command EditCommand `json:"command"`
}

type EditPlanningPort interface {
	PlanEdit(context.Context, EditPlanningInput) (EditCommandDraft, error)
}

type EditCommand struct {
	CommandID         string            `json:"commandId"`
	Type              string            `json:"commandType"`
	DeckID            string            `json:"deckId"`
	BaseRevision      int               `json:"baseRevision"`
	TargetSlideID     string            `json:"targetSlideId,omitempty"`
	TargetElementID   string            `json:"targetElementId,omitempty"`
	Payload           map[string]string `json:"payload"`
	UserIntentSummary string            `json:"userIntentSummary"`
}

type DeckRevisionSnapshot struct {
	Revision         int             `json:"revision"`
	ParentRevision   int             `json:"parentRevision,omitempty"`
	UserRequest      string          `json:"userRequest,omitempty"`
	Commands         []EditCommand   `json:"commands"`
	AffectedSlideIDs []string        `json:"affectedSlideIds"`
	Compilation      DeckCompilation `json:"compilation"`
	FileID           string          `json:"fileId,omitempty"`
	RenderBytes      []byte          `json:"renderBytes,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
}

func ValidateEditCommand(command EditCommand, state AgentDeckGenerationState) error {
	if state.Compilation == nil || strings.TrimSpace(command.CommandID) == "" || strings.TrimSpace(command.DeckID) == "" || command.DeckID != state.Compilation.DeckID || command.BaseRevision <= 0 || command.BaseRevision != state.Compilation.Revision || strings.TrimSpace(command.Type) == "" {
		if state.Compilation != nil && command.BaseRevision != state.Compilation.Revision {
			return ErrEditStaleRevision
		}
		return ErrEditInvalidCommand
	}
	if command.Type != EditCommandUpdateText && command.Type != EditCommandRegenerate && command.Type != EditCommandChangeLayout && command.Type != EditCommandReplaceImage && command.Type != EditCommandMoveSlide && command.Type != EditCommandAddSlide && command.Type != EditCommandDeleteSlide {
		return ErrEditInvalidCommand
	}
	if strings.TrimSpace(command.TargetSlideID) == "" {
		return ErrEditInvalidCommand
	}
	var deck struct {
		Slides []struct {
			ID       string `json:"id"`
			Elements []struct {
				ID string `json:"id"`
			} `json:"elements"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(state.Compilation.Deck, &deck); err != nil {
		return ErrEditInvalidCommand
	}
	for _, slide := range deck.Slides {
		if slide.ID != command.TargetSlideID {
			continue
		}
		if command.Type == EditCommandUpdateText || command.Type == EditCommandReplaceImage {
			if strings.TrimSpace(command.TargetElementID) == "" {
				return ErrEditInvalidCommand
			}
			for _, element := range slide.Elements {
				if element.ID == command.TargetElementID {
					return nil
				}
			}
			return ErrEditTargetNotFound
		}
		return nil
	}
	return ErrEditTargetNotFound
}

func ApplyEditCommand(state AgentDeckGenerationState, command EditCommand, now time.Time) (AgentDeckGenerationState, error) {
	for _, revision := range state.Revisions {
		for _, prior := range revision.Commands {
			if strings.TrimSpace(prior.CommandID) != "" && prior.CommandID == command.CommandID {
				return cloneAgentDeckGenerationState(state), nil
			}
		}
	}
	if err := ValidateEditCommand(command, state); err != nil {
		return AgentDeckGenerationState{}, err
	}
	if command.Type == EditCommandReplaceImage {
		assetID := strings.TrimSpace(command.Payload["assetRef"])
		found := false
		for _, asset := range state.Assets {
			if asset.ID == assetID {
				found = true
				break
			}
		}
		if !found {
			return AgentDeckGenerationState{}, ErrEditTargetNotFound
		}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	updated := cloneAgentDeckGenerationState(state)
	if updated.CurrentRevision == 0 {
		updated.CurrentRevision = state.Compilation.Revision
	}
	if len(updated.Revisions) == 0 {
		updated.Revisions = []DeckRevisionSnapshot{{Revision: state.Compilation.Revision, Compilation: cloneDeckCompilation(*state.Compilation), CreatedAt: now}}
	}
	if command.BaseRevision != updated.CurrentRevision {
		return AgentDeckGenerationState{}, ErrEditStaleRevision
	}
	compilation := cloneDeckCompilation(*state.Compilation)
	var deck map[string]any
	if err := json.Unmarshal(compilation.Deck, &deck); err != nil {
		return AgentDeckGenerationState{}, ErrEditInvalidCommand
	}
	changed, err := applyDeckJSONEdit(deck, command)
	if err != nil {
		return AgentDeckGenerationState{}, err
	}
	if !changed {
		return AgentDeckGenerationState{}, ErrEditInvalidCommand
	}
	compiled, err := json.Marshal(deck)
	if err != nil {
		return AgentDeckGenerationState{}, err
	}
	compilation.Revision = command.BaseRevision + 1
	if slides, ok := deck["slides"].([]any); ok {
		compilation.SlideCount = len(slides)
	}
	compilation.Deck = compiled
	compilation.LayoutResult = syncLayoutForDeck(compilation.LayoutResult, deck)
	if command.Type == EditCommandChangeLayout {
		compilation.LayoutResult = updateLayoutID(compilation.LayoutResult, command.TargetSlideID, strings.TrimSpace(command.Payload["layoutId"]))
	}
	compilation.RenderInput = syncRenderInput(compilation.RenderInput, deck, compilation.LayoutResult)
	compilation.RenderInput = updateRenderInputRevision(compilation.RenderInput, compilation.Revision)
	updated.Compilation = &compilation
	updated.CurrentRevision = compilation.Revision
	updated.Revisions = append(updated.Revisions, DeckRevisionSnapshot{Revision: compilation.Revision, ParentRevision: command.BaseRevision, UserRequest: command.UserIntentSummary, Commands: []EditCommand{command}, AffectedSlideIDs: []string{command.TargetSlideID}, Compilation: cloneDeckCompilation(compilation), CreatedAt: now})
	return updated, nil
}

func UndoLastEdit(state AgentDeckGenerationState) (AgentDeckGenerationState, error) {
	if len(state.Revisions) < 2 || state.CurrentRevision <= 0 {
		return AgentDeckGenerationState{}, ErrEditNoUndo
	}
	current := -1
	for index := range state.Revisions {
		if state.Revisions[index].Revision == state.CurrentRevision {
			current = index
			break
		}
	}
	if current <= 0 {
		return AgentDeckGenerationState{}, ErrEditNoUndo
	}
	parent := state.Revisions[current].ParentRevision
	for index := range state.Revisions {
		if state.Revisions[index].Revision == parent {
			updated := cloneAgentDeckGenerationState(state)
			compilation := cloneDeckCompilation(state.Revisions[index].Compilation)
			updated.CurrentRevision = parent
			updated.Compilation = &compilation
			return updated, nil
		}
	}
	return AgentDeckGenerationState{}, ErrEditNoUndo
}

func applyDeckJSONEdit(deck map[string]any, command EditCommand) (bool, error) {
	slides, ok := deck["slides"].([]any)
	if !ok {
		return false, ErrEditInvalidCommand
	}
	for _, raw := range slides {
		slide, ok := raw.(map[string]any)
		if !ok || slide["id"] != command.TargetSlideID {
			continue
		}
		switch command.Type {
		case EditCommandUpdateText:
			elements, ok := slide["elements"].([]any)
			if !ok {
				return false, ErrEditInvalidCommand
			}
			for _, rawElement := range elements {
				element, ok := rawElement.(map[string]any)
				if !ok || element["id"] != command.TargetElementID {
					continue
				}
				content, ok := element["content"].(map[string]any)
				if !ok {
					return false, ErrEditInvalidCommand
				}
				text := strings.TrimSpace(command.Payload["text"])
				if text == "" {
					return false, ErrEditInvalidCommand
				}
				content["kind"] = "plain"
				delete(content, "items")
				content["text"] = text
				return true, nil
			}
			return false, ErrEditTargetNotFound
		case EditCommandReplaceImage:
			elements, ok := slide["elements"].([]any)
			if !ok {
				return false, ErrEditInvalidCommand
			}
			for _, rawElement := range elements {
				element, ok := rawElement.(map[string]any)
				if !ok || element["id"] != command.TargetElementID {
					continue
				}
				if element["type"] != "image" {
					return false, ErrEditInvalidCommand
				}
				asset := strings.TrimSpace(command.Payload["assetRef"])
				if asset == "" {
					return false, ErrEditInvalidCommand
				}
				element["assetRef"] = asset
				if alt := strings.TrimSpace(command.Payload["altText"]); alt != "" {
					element["altText"] = alt
				}
				return true, nil
			}
			return false, ErrEditTargetNotFound
		case EditCommandChangeLayout:
			layout := strings.TrimSpace(command.Payload["layoutId"])
			if layout == "" {
				return false, ErrEditInvalidCommand
			}
			slide["layoutId"] = layout
			return true, nil
		case EditCommandMoveSlide:
			toIndex := 0
			if _, err := fmt.Sscanf(command.Payload["toIndex"], "%d", &toIndex); err != nil || toIndex < 1 || toIndex > len(slides) {
				return false, ErrEditInvalidCommand
			}
			fromIndex := -1
			for index, item := range slides {
				if candidate, ok := item.(map[string]any); ok && candidate["id"] == command.TargetSlideID {
					fromIndex = index
					break
				}
			}
			if fromIndex < 0 {
				return false, ErrEditTargetNotFound
			}
			item := slides[fromIndex]
			slides = append(slides[:fromIndex], slides[fromIndex+1:]...)
			target := toIndex - 1
			if target >= len(slides) {
				target = len(slides)
			}
			slides = append(slides, nil)
			copy(slides[target+1:], slides[target:])
			slides[target] = item
			for index, rawSlide := range slides {
				if slide, ok := rawSlide.(map[string]any); ok {
					slide["sequence"] = index + 1
				}
			}
			deck["slides"] = slides
			return true, nil
		case EditCommandDeleteSlide:
			if len(slides) <= AgentMinimumPageCount {
				return false, ErrEditInvalidCommand
			}
			for index, item := range slides {
				if candidate, ok := item.(map[string]any); ok && candidate["id"] == command.TargetSlideID {
					slides = append(slides[:index], slides[index+1:]...)
					for sequence, rawSlide := range slides {
						if slide, ok := rawSlide.(map[string]any); ok {
							slide["sequence"] = sequence + 1
						}
					}
					deck["slides"] = slides
					return true, nil
				}
			}
			return false, ErrEditTargetNotFound
		case EditCommandAddSlide:
			if len(slides) >= AgentMaximumPageCount {
				return false, ErrEditInvalidCommand
			}
			newID := strings.TrimSpace(command.Payload["slideId"])
			title := strings.TrimSpace(command.Payload["title"])
			keyMessage := strings.TrimSpace(command.Payload["keyMessage"])
			if newID == "" || title == "" || keyMessage == "" {
				return false, ErrEditInvalidCommand
			}
			for _, item := range slides {
				if candidate, ok := item.(map[string]any); ok && candidate["id"] == newID {
					return false, ErrEditInvalidCommand
				}
			}
			anchor := -1
			var template map[string]any
			for index, item := range slides {
				if candidate, ok := item.(map[string]any); ok && candidate["id"] == command.TargetSlideID {
					anchor = index
					template = candidate
					break
				}
			}
			if anchor < 0 {
				return false, ErrEditTargetNotFound
			}
			encoded, _ := json.Marshal(template)
			var added map[string]any
			if json.Unmarshal(encoded, &added) != nil {
				return false, ErrEditInvalidCommand
			}
			added["id"] = newID
			added["objectiveId"] = newID
			if elements, ok := added["elements"].([]any); ok {
				for _, rawElement := range elements {
					if element, ok := rawElement.(map[string]any); ok {
						if slot, _ := element["slot"].(string); slot == "title" {
							if content, ok := element["content"].(map[string]any); ok {
								content["text"] = title
								content["kind"] = "plain"
							}
						}
						if slot, _ := element["slot"].(string); slot == "key-message" {
							if content, ok := element["content"].(map[string]any); ok {
								content["text"] = keyMessage
								content["kind"] = "plain"
							}
						}
					}
				}
			}
			target := anchor + 1
			slides = append(slides, nil)
			copy(slides[target+1:], slides[target:])
			slides[target] = added
			for sequence, rawSlide := range slides {
				if slide, ok := rawSlide.(map[string]any); ok {
					slide["sequence"] = sequence + 1
				}
			}
			deck["slides"] = slides
			return true, nil
		default:
			return false, fmt.Errorf("%w: %s", ErrEditUnsupported, command.Type)
		}
	}
	return false, ErrEditTargetNotFound
}

func syncLayoutForDeck(raw json.RawMessage, deck map[string]any) json.RawMessage {
	var layout map[string]any
	if json.Unmarshal(raw, &layout) != nil {
		return raw
	}
	items, ok := layout["slides"].([]any)
	if !ok {
		return raw
	}
	byID := map[string]any{}
	for _, item := range items {
		if value, ok := item.(map[string]any); ok {
			byID[fmt.Sprint(value["slideId"])] = value
		}
	}
	ordered := []any{}
	if slides, ok := deck["slides"].([]any); ok {
		for _, item := range slides {
			if value, ok := item.(map[string]any); ok {
				if match, exists := byID[fmt.Sprint(value["id"])]; exists {
					ordered = append(ordered, match)
				}
			}
		}
	}
	layout["slides"] = ordered
	result, err := json.Marshal(layout)
	if err != nil {
		return raw
	}
	return result
}

func cloneDeckCompilation(input DeckCompilation) DeckCompilation {
	input.Deck = append(json.RawMessage(nil), input.Deck...)
	input.LayoutResult = append(json.RawMessage(nil), input.LayoutResult...)
	input.RenderInput = append(json.RawMessage(nil), input.RenderInput...)
	input.QualityIssues = append([]string(nil), input.QualityIssues...)
	return input
}

func updateRenderInputRevision(raw json.RawMessage, revision int) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	if deckRevision, ok := value["deckRevision"].(map[string]any); ok {
		deckRevision["revision"] = revision
	}
	updated, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return updated
}

func updateLayoutID(raw json.RawMessage, slideID, layoutID string) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	if slides, ok := value["slides"].([]any); ok {
		for _, rawSlide := range slides {
			if slide, ok := rawSlide.(map[string]any); ok && slide["slideId"] == slideID {
				slide["layoutId"] = layoutID
			}
		}
	}
	updated, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return updated
}

func syncRenderInput(raw json.RawMessage, deck map[string]any, layout json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	if slides, ok := deck["slides"]; ok {
		value["slides"] = slides
	}
	var layoutValue map[string]any
	if json.Unmarshal(layout, &layoutValue) == nil {
		if slides, ok := layoutValue["slides"]; ok {
			value["layoutResults"] = slides
		}
	}
	updated, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return updated
}
