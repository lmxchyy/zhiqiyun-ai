package ppt

import (
	"context"
	"fmt"
	"strings"
)

const DefaultNegativePrompt = "text, letters, words, typography, numbers, logo, watermark, captions, subtitles, garbled text"

type VisualPlan struct {
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
	Prompt          string   `json:"prompt"`
	NegativePrompt  string   `json:"negativePrompt"`
}

type VisualAsset struct {
	URL        string `json:"url"`
	StorageRef string `json:"storageRef,omitempty"`
	TaskID     string `json:"taskId,omitempty"`
	ModelName  string `json:"modelName,omitempty"`
	CreatedAt  string `json:"createdAt"`
}

type VisualPlannerInput struct {
	DeckTheme        string
	SlideType        string
	SlideTitle       string
	CoreIdea         string
	ContentSummary   string
	Layout           string
	ImagePosition    string
	ImageStyle       string
	PeopleStyle      string
	ImageLighting    string
	ImageComposition string
}

type VisualPlanModelFunc func(context.Context, VisualPlannerInput) (VisualPlan, error)

type VisualPlannerService struct{ model VisualPlanModelFunc }

func NewVisualPlannerService(model VisualPlanModelFunc) *VisualPlannerService {
	return &VisualPlannerService{model: model}
}

func (s *VisualPlannerService) Plan(ctx context.Context, input VisualPlannerInput) (VisualPlan, error) {
	if s != nil && s.model != nil {
		if plan, err := s.model(ctx, input); err == nil {
			return NormalizeVisualPlan(plan, input), nil
		}
	}
	return NormalizeVisualPlan(VisualPlan{}, input), nil
}

func NormalizeVisualPlan(plan VisualPlan, input VisualPlannerInput) VisualPlan {
	slideType := NormalizeSlideType(input.SlideType)
	plan.VisualType = firstNonEmptyVisual(plan.VisualType, defaultVisualType(slideType))
	plan = normalizeVisualRequirements(plan, slideType)
	plan.TextInImage = false
	plan.Subject = firstNonEmptyVisual(plan.Subject, conciseVisualSubject(input.SlideTitle, input.CoreIdea))
	plan.Scene = firstNonEmptyVisual(plan.Scene, defaultVisualScene(slideType))
	plan.Action = firstNonEmptyVisual(plan.Action, defaultVisualAction(slideType))
	if len(plan.Objects) == 0 {
		plan.Objects = defaultVisualObjects(slideType)
	}
	plan.Mood = firstNonEmptyVisual(plan.Mood, "professional, efficient, trustworthy")
	plan.Composition = firstNonEmptyVisual(plan.Composition, CompositionInstruction(firstNonEmptyVisual(input.ImagePosition, input.Layout, input.ImageComposition)))
	plan.Style = firstNonEmptyVisual(plan.Style, input.ImageStyle, "modern enterprise illustration")
	plan = sanitizeVisualPlan(plan, input)
	plan.NegativePrompt = mergeNegativePrompt(plan.NegativePrompt)
	plan.Prompt = BuildVisualPrompt(plan, input)
	return plan
}

// sanitizeVisualPlan treats model output as untrusted structured data. The
// image prompt is rebuilt from concise visual fields, but a model can still
// copy slide prose into those fields unless they are bounded here.
func sanitizeVisualPlan(plan VisualPlan, input VisualPlannerInput) VisualPlan {
	plan.Subject = sanitizeVisualField(plan.Subject, 48)
	plan.Scene = sanitizeVisualField(plan.Scene, 72)
	plan.Action = sanitizeVisualField(plan.Action, 72)
	plan.Mood = sanitizeVisualField(plan.Mood, 48)
	plan.Composition = sanitizeVisualField(plan.Composition, 140)
	plan.Style = sanitizeVisualField(plan.Style, 72)
	objects := make([]string, 0, min(len(plan.Objects), 6))
	for _, object := range plan.Objects {
		if object = sanitizeVisualField(object, 40); object != "" {
			objects = append(objects, object)
			if len(objects) == 6 {
				break
			}
		}
	}
	plan.Objects = objects

	content := normalizeVisualProse(firstNonEmptyVisual(input.ContentSummary, input.CoreIdea))
	if len([]rune(content)) >= 24 {
		if visualFieldCopiesProse(plan.Subject, content) {
			plan.Subject = conciseVisualSubject(input.SlideTitle, "")
		}
		if visualFieldCopiesProse(plan.Scene, content) {
			plan.Scene = defaultVisualScene(NormalizeSlideType(input.SlideType))
		}
		if visualFieldCopiesProse(plan.Action, content) {
			plan.Action = defaultVisualAction(NormalizeSlideType(input.SlideType))
		}
		filtered := plan.Objects[:0]
		for _, object := range plan.Objects {
			if !visualFieldCopiesProse(object, content) {
				filtered = append(filtered, object)
			}
		}
		plan.Objects = filtered
	}
	if plan.Subject == "" {
		plan.Subject = "enterprise AI assistant"
	}
	if len(plan.Objects) == 0 {
		plan.Objects = defaultVisualObjects(NormalizeSlideType(input.SlideType))
	}
	return plan
}

func sanitizeVisualField(value string, maxRunes int) string {
	value = normalizeVisualProse(value)
	runes := []rune(value)
	if maxRunes > 0 && len(runes) > maxRunes {
		value = strings.TrimSpace(string(runes[:maxRunes]))
	}
	return strings.Trim(value, " ,.;:，。；：、-—")
}

func normalizeVisualProse(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func visualFieldCopiesProse(field, prose string) bool {
	field, prose = normalizeVisualProse(field), normalizeVisualProse(prose)
	if field == "" || prose == "" {
		return false
	}
	return strings.Contains(field, prose) || strings.Contains(prose, field) && len([]rune(field)) >= 24
}

func normalizeVisualRequirements(plan VisualPlan, slideType string) VisualPlan {
	visualType := strings.ToLower(strings.TrimSpace(plan.VisualType))
	switch visualType {
	case "none":
		plan.ImageRequired = false
		plan.ChartRequired = false
		plan.DiagramRequired = false
	case "icon":
		plan.ImageRequired = false
		plan.ChartRequired = false
		plan.DiagramRequired = false
	case "chart":
		plan.ImageRequired = false
		plan.ChartRequired = true
		plan.DiagramRequired = false
	case "diagram", "flowchart":
		plan.VisualType = "diagram"
		plan.ImageRequired = false
		plan.ChartRequired = false
		plan.DiagramRequired = true
	default:
		plan.ImageRequired = imageAllowedForSlideType(slideType) && (plan.ImageRequired || visualType == "illustration" || visualType == "scene" || visualType == "product" || visualType == "office")
		plan.ChartRequired = plan.ChartRequired || slideType == "data_chart"
		plan.DiagramRequired = plan.DiagramRequired || slideType == "process" || slideType == "timeline" || slideType == "organization"
	}
	if !imageAllowedForSlideType(slideType) {
		plan.ImageRequired = false
	}
	return plan
}

func BuildVisualPrompt(plan VisualPlan, input VisualPlannerInput) string {
	objects := strings.Join(nonEmptyVisualStrings(plan.Objects), ", ")
	styleParts := nonEmptyVisualStrings([]string{plan.Style, input.PeopleStyle, input.ImageLighting})
	primary, secondary, accent := visualThemePalette(input.DeckTheme)
	return strings.Join(nonEmptyVisualStrings([]string{
		"Professional 16:9 presentation visual without text",
		fmt.Sprintf("subject: %s", plan.Subject),
		fmt.Sprintf("scene: %s", plan.Scene),
		fmt.Sprintf("action: %s", plan.Action),
		fmt.Sprintf("key objects: %s", objects),
		fmt.Sprintf("mood: %s", plan.Mood),
		fmt.Sprintf("composition: %s", plan.Composition),
		fmt.Sprintf("consistent deck style: %s", strings.Join(styleParts, ", ")),
		fmt.Sprintf("deck palette: primary %s, secondary %s, accent %s", primary, secondary, accent),
		"no text, no letters, no words, no numbers, no logo, no watermark, no captions, no interface text",
	}), ". ") + "."
}

func visualThemePalette(theme string) (string, string, string) {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "techblue", "indigo", "orbit", "cosmos":
		return "deep blue", "cyan", "electric violet"
	case "freshgreen", "canopy", "aurora", "mint", "jade":
		return "forest green", "soft mint", "warm gold"
	case "blackgold", "noir", "ebony", "phantom":
		return "charcoal black", "warm gray", "restrained gold"
	case "marketing", "ember", "sunset", "coral", "magma":
		return "warm red", "sunset orange", "bright coral"
	default:
		return "enterprise blue", "cool gray", "clear cyan"
	}
}

func NormalizeSlideType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "cover", "section", "statement", "text_image", "case_study", "product_showcase", "industry_scene", "agenda", "feature_grid", "process", "timeline", "comparison", "data_chart", "swot", "matrix", "organization", "table":
		return value
	default:
		return "text_image"
	}
}

func ShouldGenerateImageForSlide(slide Slide) bool {
	if !imageAllowedForSlideType(NormalizeSlideType(slide.SlideType)) {
		return false
	}
	return slide.VisualPlan == nil || slide.VisualPlan.ImageRequired
}

func imageAllowedForSlideType(slideType string) bool {
	switch NormalizeSlideType(slideType) {
	case "cover", "section", "statement", "text_image", "case_study", "product_showcase", "industry_scene":
		return true
	default:
		return false
	}
}

func CompositionInstruction(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "image_left", "left", "imageleft":
		return "main subject on the left, keep about 40 percent clean negative space on the right for slide copy"
	case "background", "background_image", "cover":
		return "background visual with restrained complexity, keep the title area clear and avoid placing the subject behind headings"
	case "full_width", "wide", "banner":
		return "centered subject in a wide composition, reserve a clean title zone at the top or bottom"
	case "card", "card_image":
		return "centered subject, simple background, clean card-safe edges without distracting details"
	default:
		return "main subject on the right, keep about 40 percent clean negative space on the left for slide copy"
	}
}

func defaultVisualType(slideType string) string {
	switch slideType {
	case "product_showcase":
		return "product"
	case "industry_scene", "case_study":
		return "scene"
	case "data_chart":
		return "chart"
	case "process", "timeline", "organization":
		return "diagram"
	case "agenda", "feature_grid", "comparison", "swot", "matrix", "table":
		return "icon"
	default:
		return "illustration"
	}
}

func defaultVisualScene(slideType string) string {
	if slideType == "product_showcase" {
		return "clean enterprise product showcase environment"
	}
	if slideType == "industry_scene" || slideType == "case_study" {
		return "realistic modern enterprise working environment"
	}
	return "modern enterprise environment with abstract connected data elements"
}

func defaultVisualAction(slideType string) string {
	if slideType == "product_showcase" {
		return "the product is being used naturally in a business workflow"
	}
	return "people and intelligent systems collaborate to complete a clear business task"
}

func defaultVisualObjects(slideType string) []string {
	if slideType == "product_showcase" {
		return []string{"product device", "subtle data connections", "clean workspace"}
	}
	return []string{"computer workspace", "abstract communication bubbles", "enterprise data connection nodes"}
}

func conciseVisualSubject(title, coreIdea string) string {
	// Prefer the slide title over body-derived ideas. This fallback is used when
	// no planner model is configured, so copying prose here would bypass the
	// semantic extraction step.
	value := firstNonEmptyVisual(title, coreIdea, "enterprise AI assistant")
	value = strings.Join(strings.Fields(value), " ")
	if runes := []rune(value); len(runes) > 36 {
		value = string(runes[:36])
	}
	return value
}

func mergeNegativePrompt(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value + ", " + DefaultNegativePrompt
	}
	return DefaultNegativePrompt
}

func nonEmptyVisualStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstNonEmptyVisual(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
