package ppt

import (
	"strings"
	"testing"
)

func TestVisualPromptDoesNotCopyFullSlideBody(t *testing.T) {
	body := "This complete page body must remain in the slide layout and must never be copied into the image generation prompt."
	plan := NormalizeVisualPlan(VisualPlan{}, VisualPlannerInput{SlideType: "text_image", SlideTitle: "Enterprise AI", CoreIdea: "AI assistant collaboration", ContentSummary: body, ImagePosition: "image_right"})
	if strings.Contains(plan.Prompt, body) {
		t.Fatalf("visual prompt copied the full slide body: %q", plan.Prompt)
	}
}

func TestVisualPlanSanitizesModelFieldsThatCopySlideProse(t *testing.T) {
	body := "This complete page body describes revenue growth, customer retention, operating efficiency, and every supporting point from the slide."
	plan := NormalizeVisualPlan(VisualPlan{
		Subject: body, Scene: body, Action: body, Objects: []string{body}, Prompt: body,
	}, VisualPlannerInput{
		SlideType: "text_image", SlideTitle: "Business momentum", CoreIdea: body, ContentSummary: body,
	})
	if strings.Contains(plan.Prompt, body) {
		t.Fatalf("sanitized visual prompt still contains full slide prose: %q", plan.Prompt)
	}
	if plan.Subject == body || plan.Scene == body || plan.Action == body {
		t.Fatalf("model prose was not removed from visual fields: %#v", plan)
	}
	for _, object := range plan.Objects {
		if strings.Contains(body, object) && len([]rune(object)) >= 24 {
			t.Fatalf("model prose remained in visual objects: %q", object)
		}
	}
}

func TestVisualPromptContainsNoTextRules(t *testing.T) {
	plan := NormalizeVisualPlan(VisualPlan{}, VisualPlannerInput{SlideType: "cover", SlideTitle: "Enterprise AI"})
	for _, required := range []string{"presentation visual without text", "no text", "no letters", "no words", "no numbers", "no logo", "no watermark", "no captions", "no interface text"} {
		if !strings.Contains(strings.ToLower(plan.Prompt), required) {
			t.Fatalf("prompt missing %q: %s", required, plan.Prompt)
		}
	}
	for _, required := range []string{"typography", "subtitles", "garbled text"} {
		if !strings.Contains(strings.ToLower(plan.NegativePrompt), required) {
			t.Fatalf("negative prompt missing %q: %s", required, plan.NegativePrompt)
		}
	}
}

func TestNonImageSlideTypesSkipImageGeneration(t *testing.T) {
	for _, slideType := range []string{"feature_grid", "process"} {
		plan := NormalizeVisualPlan(VisualPlan{}, VisualPlannerInput{SlideType: slideType})
		if ShouldGenerateImageForSlide(Slide{SlideType: slideType, VisualPlan: &plan}) {
			t.Fatalf("%s slide should not generate an image", slideType)
		}
	}
}

func TestCoverSlideAllowsImageGeneration(t *testing.T) {
	plan := NormalizeVisualPlan(VisualPlan{}, VisualPlannerInput{SlideType: "cover"})
	if !ShouldGenerateImageForSlide(Slide{SlideType: "cover", VisualPlan: &plan}) {
		t.Fatal("cover slide should allow image generation")
	}
}

func TestNonImageVisualTypesExplicitlyDisableImageGeneration(t *testing.T) {
	for _, visualType := range []string{"icon", "chart", "diagram", "none"} {
		t.Run(visualType, func(t *testing.T) {
			plan := NormalizeVisualPlan(VisualPlan{VisualType: visualType, ImageRequired: true}, VisualPlannerInput{SlideType: "text_image", SlideTitle: "AI service"})
			if plan.ImageRequired {
				t.Fatalf("visual type %s must disable image generation", visualType)
			}
		})
	}
}

func TestCompositionInstructionsReserveOppositeWhitespace(t *testing.T) {
	left, right := CompositionInstruction("image_left"), CompositionInstruction("image_right")
	if left == right || !strings.Contains(left, "right") || !strings.Contains(right, "left") {
		t.Fatalf("unexpected composition instructions: left=%q right=%q", left, right)
	}
}
