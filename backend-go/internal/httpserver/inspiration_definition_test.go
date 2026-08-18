package httpserver

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func validImageTemplateDefinition() InternalTemplateDefinition {
	return InternalTemplateDefinition{
		SchemaVersion: 1,
		Inputs: []TemplateInputDefinition{
			{
				Key:      "subject",
				Type:     TemplateInputText,
				Label:    "Subject",
				Required: true,
				HelpText: "Describe the main subject",
				Validation: TemplateInputValidation{
					MinLength: intPointer(2),
					MaxLength: intPointer(120),
				},
			},
			{
				Key:     "style",
				Type:    TemplateInputSelect,
				Label:   "Style",
				Default: "minimal",
				Options: []TemplateInputOption{
					{Label: "Minimal", Value: "minimal"},
					{Label: "Editorial", Value: "editorial"},
				},
			},
		},
		Prompt: TemplatePromptDefinition{
			Template:         "Create {{subject}} in a {{style}} style",
			NegativeTemplate: "watermark, blurry",
			Composer: TemplateComposerDefinition{
				Key:     "deterministic-template",
				Version: 1,
			},
		},
		Bindings: []TemplateBindingDefinition{
			{Source: "inputs.subject", Target: "prompt.variables.subject", Transform: TemplateTransformTrim},
			{Source: "inputs.style", Target: "prompt.variables.style", Transform: TemplateTransformEnumValue},
			{Source: "inputs.style", Target: "parameters.style", Transform: TemplateTransformEnumValue},
		},
		Presets: TemplatePresetsDefinition{
			InputDefaults:      map[string]any{"style": "minimal"},
			GenerationDefaults: map[string]any{"ratio": "1:1"},
			Materials:          []TemplateMaterialPreset{{InputKey: "reference", AssetID: "asset_example"}},
		},
		Presentation: map[string]any{"layout": "single_column", "heroLabel": "Create yours"},
		Handoff: TemplateHandoffDefinition{
			TargetType: "IMAGE_CREATION",
			TargetKey:  "image.create",
			IntentKey:  "product_image",
		},
		Capability: TemplateCapabilityDefinition{
			CapabilityKey: "image_generation",
			ModelHint:     "gpt-image-2",
		},
	}
}

func intPointer(value int) *int { return &value }

func TestValidateTemplateDefinitionAcceptsDeclarativeImageSchema(t *testing.T) {
	issues := validateTemplateDefinition("IMAGE", validImageTemplateDefinition())
	if len(issues) != 0 {
		t.Fatalf("valid definition issues = %#v", issues)
	}
}

func TestValidateTemplateDefinitionRejectsUnknownBindingAndDuplicateInput(t *testing.T) {
	definition := validImageTemplateDefinition()
	definition.Inputs = append(definition.Inputs, definition.Inputs[0])
	definition.Bindings = append(definition.Bindings, TemplateBindingDefinition{
		Source: "inputs.missing",
		Target: "prompt.variables.missing",
	})

	issues := validateTemplateDefinition("IMAGE", definition)
	wantCodes := map[string]bool{
		"DUPLICATE_INPUT_KEY":    false,
		"BINDING_SOURCE_UNKNOWN": false,
	}
	for _, issue := range issues {
		if _, expected := wantCodes[issue.Code]; expected {
			wantCodes[issue.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Fatalf("missing validation issue %s in %#v", code, issues)
		}
	}
}

func TestComposeTemplateDefinitionIsDeterministicAndAppliesBindings(t *testing.T) {
	definition := validImageTemplateDefinition()
	values := map[string]any{"subject": "  smart coffee machine  ", "style": "editorial"}

	first, err := composeTemplateDefinition(definition, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := composeTemplateDefinition(definition, values, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("composition is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.BasePrompt != "Create smart coffee machine in a editorial style" {
		t.Fatalf("base prompt = %q", first.BasePrompt)
	}
	if first.NegativePrompt != "watermark, blurry" {
		t.Fatalf("negative prompt = %q", first.NegativePrompt)
	}
	if first.Parameters["ratio"] != "1:1" || first.Parameters["style"] != "editorial" {
		t.Fatalf("parameters = %#v", first.Parameters)
	}
}

func TestComposeTemplateDefinitionRejectsMissingRequiredInput(t *testing.T) {
	_, err := composeTemplateDefinition(validImageTemplateDefinition(), map[string]any{"style": "minimal"}, nil)
	if err == nil || !strings.Contains(err.Error(), "subject") {
		t.Fatalf("missing required input error = %v", err)
	}
}

func TestProjectPublicTemplateDefinitionUsesStrictAllowlist(t *testing.T) {
	projection := projectPublicTemplateDefinition(validImageTemplateDefinition())
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(raw)
	for _, forbidden := range []string{
		"Create {{subject}}",
		"watermark, blurry",
		"composerKey",
		"bindings",
		"image_generation",
		"gpt-image-2",
		"image.create",
		"product_image",
		"asset_example",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, payload)
		}
	}
	if !strings.Contains(payload, `"inputs"`) || !strings.Contains(payload, `"presentation"`) ||
		!strings.Contains(payload, `"inputDefaults"`) || !strings.Contains(payload, `"targetType":"IMAGE_CREATION"`) {
		t.Fatalf("public projection omitted allowed fields: %s", payload)
	}
}

func TestValidateTemplateDefinitionRejectsWorkflowCycle(t *testing.T) {
	definition := InternalTemplateDefinition{
		SchemaVersion: 1,
		Inputs:        []TemplateInputDefinition{{Key: "topic", Type: TemplateInputText, Label: "Topic", Required: true}},
		Prompt: TemplatePromptDefinition{
			Template: "{{topic}}",
			Composer: TemplateComposerDefinition{Key: "deterministic-template", Version: 1},
		},
		Bindings:   []TemplateBindingDefinition{{Source: "inputs.topic", Target: "prompt.variables.topic", Transform: TemplateTransformTrim}},
		Handoff:    TemplateHandoffDefinition{TargetType: "WORKFLOW_CREATION", TargetKey: "workflow.create"},
		Capability: TemplateCapabilityDefinition{CapabilityKey: "workflow_execution"},
		Workflow: &TemplateWorkflowDefinition{
			WorkflowVersion: 1,
			ExecutorKey:     "workflow.default",
			Nodes: []TemplateWorkflowNode{
				{ID: "a", Type: "CAPABILITY", CapabilityKey: "image_generation"},
				{ID: "b", Type: "CAPABILITY", CapabilityKey: "video_generation"},
			},
			Edges:         []TemplateWorkflowEdge{{From: "a", To: "b"}, {From: "b", To: "a"}},
			FailurePolicy: TemplateWorkflowFailurePolicy{Strategy: "FAIL_FAST"},
		},
	}

	issues := validateTemplateDefinition("WORKFLOW", definition)
	for _, issue := range issues {
		if issue.Code == "WORKFLOW_CYCLE" {
			return
		}
	}
	t.Fatalf("workflow cycle was not rejected: %#v", issues)
}

func TestDecodeInternalTemplateDefinitionRejectsUnknownFields(t *testing.T) {
	raw, err := json.Marshal(validImageTemplateDefinition())
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":1,"providerApiKey":"secret"`), 1)

	_, err = decodeInternalTemplateDefinition(raw)
	if err == nil || !strings.Contains(err.Error(), "providerApiKey") {
		t.Fatalf("unknown definition field error = %v", err)
	}
}

func TestComposeTemplateDefinitionValidatesMaterialInputs(t *testing.T) {
	definition := validImageTemplateDefinition()
	definition.Inputs = append(definition.Inputs, TemplateInputDefinition{
		Key:      "referenceImages",
		Type:     TemplateInputImage,
		Label:    "Reference images",
		Required: true,
		Validation: TemplateInputValidation{
			MinItems: intPointer(1),
			MaxItems: intPointer(2),
		},
	})
	definition.Bindings = append(definition.Bindings, TemplateBindingDefinition{
		Source:    "materials.referenceImages",
		Target:    "parameters.referenceAssetIds",
		Transform: TemplateTransformAssetIDs,
	})
	materials := []TemplateComposeMaterial{{InputKey: "referenceImages", AssetID: "asset-1"}}

	composition, err := composeTemplateDefinition(definition, map[string]any{
		"subject": "coffee machine",
		"style":   "minimal",
	}, materials)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"asset-1"}
	got, ok := composition.Parameters["referenceAssetIds"].([]string)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("referenceAssetIds = %#v", composition.Parameters["referenceAssetIds"])
	}

	_, err = composeTemplateDefinition(definition, map[string]any{
		"subject": "coffee machine",
		"style":   "minimal",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "referenceImages") {
		t.Fatalf("missing material error = %v", err)
	}
}

func TestValidateTemplateDefinitionSupportsAllDeclaredContentTypes(t *testing.T) {
	testCases := []struct {
		contentType   string
		targetType    string
		targetKey     string
		capabilityKey string
	}{
		{"IMAGE", "IMAGE_CREATION", "image.create", "image_generation"},
		{"VIDEO", "VIDEO_CREATION", "video.create", "video_generation"},
		{"PPT", "PPT_CREATION", "ppt.create", "ppt_generation"},
		{"TEXT", "TEXT_CREATION", "text.create", "text_generation"},
		{"AGENT", "AGENT_CREATION", "agent.create", "agent_execution"},
		{"WORKFLOW", "WORKFLOW_CREATION", "workflow.create", "workflow_execution"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.contentType, func(t *testing.T) {
			definition := validImageTemplateDefinition()
			definition.Handoff.TargetType = testCase.targetType
			definition.Handoff.TargetKey = testCase.targetKey
			definition.Capability.CapabilityKey = testCase.capabilityKey
			definition.Capability.ModelHint = ""
			if testCase.contentType == "WORKFLOW" {
				definition.Workflow = &TemplateWorkflowDefinition{
					WorkflowVersion: 1,
					ExecutorKey:     "workflow.default",
					Nodes:           []TemplateWorkflowNode{{ID: "generate", Type: "CAPABILITY", CapabilityKey: "image_generation"}},
					FailurePolicy:   TemplateWorkflowFailurePolicy{Strategy: "FAIL_FAST"},
				}
			}
			if issues := validateTemplateDefinition(testCase.contentType, definition); len(issues) != 0 {
				t.Fatalf("%s definition issues = %#v", testCase.contentType, issues)
			}
		})
	}
}

func TestValidateTemplateDefinitionRejectsCapabilityContentTypeMismatch(t *testing.T) {
	definition := validImageTemplateDefinition()
	definition.Capability.CapabilityKey = "video_generation"

	issues := validateTemplateDefinition("IMAGE", definition)
	for _, issue := range issues {
		if issue.Code == "CAPABILITY_CONTENT_TYPE_MISMATCH" {
			return
		}
	}
	t.Fatalf("capability mismatch was not rejected: %#v", issues)
}
