package inspiration

import (
	"encoding/json"
	"strings"
	"testing"
)

func testImageDefinition() InternalTemplateDefinition {
	return InternalTemplateDefinition{
		SchemaVersion: 1,
		Inputs: []TemplateInputDefinition{
			{Key: "subject", Type: TemplateInputText, Label: "Subject", Required: true, Validation: TemplateInputValidation{MinLength: intPointer(2)}},
			{Key: "style", Type: TemplateInputSelect, Label: "Style", Default: "minimal", Options: []TemplateInputOption{{Label: "Minimal", Value: "minimal"}, {Label: "Editorial", Value: "editorial"}}},
		},
		Prompt: TemplatePromptDefinition{
			Template:         "Create {{subject}} in a {{style}} style",
			NegativeTemplate: "watermark",
			Composer:         TemplateComposerDefinition{Key: "deterministic-template", Version: 1},
		},
		Bindings: []TemplateBindingDefinition{
			{Source: "inputs.subject", Target: "prompt.variables.subject", Transform: TemplateTransformTrim},
			{Source: "inputs.style", Target: "prompt.variables.style"},
			{Source: "inputs.style", Target: "parameters.style"},
		},
		Presets:      TemplatePresetsDefinition{InputDefaults: map[string]any{"style": "minimal"}, GenerationDefaults: map[string]any{"ratio": "1:1"}},
		Presentation: map[string]any{"layout": "single_column", "provider": "internal-only"},
		Handoff:      TemplateHandoffDefinition{TargetType: "IMAGE_CREATION", TargetKey: "image.create"},
		Capability:   TemplateCapabilityDefinition{CapabilityKey: "image_generation", ModelHint: "gpt-image-2"},
	}
}

func intPointer(value int) *int { return &value }

func TestComposeTemplateDefinitionUsesDefaultSeparatorWhenConfiguredSeparatorIsBlank(t *testing.T) {
	definition := InternalTemplateDefinition{
		SchemaVersion: 1,
		Inputs: []TemplateInputDefinition{
			{Key: "tags", Type: TemplateInputMultiSelect, Label: "Tags", Required: true, Options: []TemplateInputOption{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}},
		},
		Prompt: TemplatePromptDefinition{
			Template: "{{tags}}",
			Composer: TemplateComposerDefinition{Key: "deterministic-template", Version: 1},
		},
		Bindings:   []TemplateBindingDefinition{{Source: "inputs.tags", Target: "prompt.variables.tags", Transform: TemplateTransformJoin, Separator: "  "}},
		Handoff:    TemplateHandoffDefinition{TargetType: "IMAGE_CREATION", TargetKey: "image.create"},
		Capability: TemplateCapabilityDefinition{CapabilityKey: "image_generation"},
	}

	composition, err := ComposeTemplateDefinition(definition, map[string]any{"tags": []any{"a", "b"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if composition.BasePrompt != "a, b" {
		t.Fatalf("base prompt = %q, want %q", composition.BasePrompt, "a, b")
	}
}

func TestValidateTemplateDefinitionRejectsMissingRequiredInputAndUnknownBinding(t *testing.T) {
	definition := testImageDefinition()
	definition.Bindings = append(definition.Bindings, TemplateBindingDefinition{Source: "inputs.missing", Target: "prompt.variables.missing"})
	issues := ValidateTemplateDefinition("IMAGE", definition)
	if len(issues) == 0 {
		t.Fatal("expected validation issues")
	}
	seen := map[string]bool{}
	for _, issue := range issues {
		seen[issue.Code] = true
	}
	if !seen["BINDING_SOURCE_UNKNOWN"] {
		t.Fatalf("missing unknown binding issue: %#v", issues)
	}

	_, err := ComposeTemplateDefinition(definition, map[string]any{"style": "minimal"}, nil)
	if err == nil || !strings.Contains(err.Error(), "subject") {
		t.Fatalf("missing required input error = %v", err)
	}
}

func TestComposeTemplateDefinitionRendersPromptAndDefaults(t *testing.T) {
	composition, err := ComposeTemplateDefinition(testImageDefinition(), map[string]any{"subject": "  coffee machine  ", "style": "editorial"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if composition.BasePrompt != "Create coffee machine in a editorial style" {
		t.Fatalf("base prompt = %q", composition.BasePrompt)
	}
	if composition.NegativePrompt != "watermark" || composition.Parameters["ratio"] != "1:1" || composition.Parameters["style"] != "editorial" {
		t.Fatalf("composition = %#v", composition)
	}
}

func TestProjectPublicTemplateDefinitionOmitsInternalFields(t *testing.T) {
	payload, err := json.Marshal(ProjectPublicTemplateDefinition(testImageDefinition()))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"gpt-image-2", "image_generation", "image.create", "watermark", "provider"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, `"inputs"`) || !strings.Contains(encoded, `"presentation"`) || !strings.Contains(encoded, `"targetType":"IMAGE_CREATION"`) {
		t.Fatalf("public projection omitted allowed fields: %s", encoded)
	}
}

func TestDecodeInternalTemplateDefinitionRejectsUnknownFields(t *testing.T) {
	raw, err := json.Marshal(testImageDefinition())
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw[:len(raw)-1], []byte(`,"providerApiKey":"secret"}`)...)
	if _, err := DecodeInternalTemplateDefinition(raw); err == nil || !strings.Contains(err.Error(), "providerApiKey") {
		t.Fatalf("unknown field error = %v", err)
	}
}
