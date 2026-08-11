package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const currentTemplateSchemaVersion = 1

type TemplateInputType string

const (
	TemplateInputText        TemplateInputType = "TEXT"
	TemplateInputTextarea    TemplateInputType = "TEXTAREA"
	TemplateInputNumber      TemplateInputType = "NUMBER"
	TemplateInputSelect      TemplateInputType = "SELECT"
	TemplateInputMultiSelect TemplateInputType = "MULTI_SELECT"
	TemplateInputBoolean     TemplateInputType = "BOOLEAN"
	TemplateInputImage       TemplateInputType = "IMAGE"
	TemplateInputVideo       TemplateInputType = "VIDEO"
	TemplateInputFile        TemplateInputType = "FILE"
)

const (
	TemplateTransformTrim      = "trim"
	TemplateTransformEnumValue = "enumValue"
	TemplateTransformJoin      = "join"
	TemplateTransformToNumber  = "toNumber"
	TemplateTransformAssetIDs  = "assetIds"
)

type InternalTemplateDefinition struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Inputs        []TemplateInputDefinition    `json:"inputs"`
	Prompt        TemplatePromptDefinition     `json:"prompt"`
	Bindings      []TemplateBindingDefinition  `json:"bindings"`
	Presets       TemplatePresetsDefinition    `json:"presets"`
	Presentation  map[string]any               `json:"presentation"`
	Handoff       TemplateHandoffDefinition    `json:"handoff"`
	Capability    TemplateCapabilityDefinition `json:"capability"`
	Workflow      *TemplateWorkflowDefinition  `json:"workflow,omitempty"`
}

type TemplateInputDefinition struct {
	Key         string                       `json:"key"`
	Type        TemplateInputType            `json:"type"`
	Label       string                       `json:"label"`
	Required    bool                         `json:"required,omitempty"`
	HelpText    string                       `json:"helpText,omitempty"`
	Placeholder string                       `json:"placeholder,omitempty"`
	Default     any                          `json:"default,omitempty"`
	Options     []TemplateInputOption        `json:"options,omitempty"`
	Validation  TemplateInputValidation      `json:"validation,omitempty"`
	VisibleWhen *TemplateVisibilityCondition `json:"visibleWhen,omitempty"`
}

type TemplateInputOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

type TemplateInputValidation struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinItems  *int     `json:"minItems,omitempty"`
	MaxItems  *int     `json:"maxItems,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Accept    []string `json:"accept,omitempty"`
}

type TemplateVisibilityCondition struct {
	InputKey string `json:"inputKey"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

type TemplatePromptDefinition struct {
	Template         string                     `json:"template"`
	NegativeTemplate string                     `json:"negativeTemplate,omitempty"`
	Composer         TemplateComposerDefinition `json:"composer"`
}

type TemplateComposerDefinition struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
}

type TemplateBindingDefinition struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Transform string `json:"transform,omitempty"`
	Separator string `json:"separator,omitempty"`
}

type TemplatePresetsDefinition struct {
	InputDefaults      map[string]any           `json:"inputDefaults,omitempty"`
	GenerationDefaults map[string]any           `json:"generationDefaults,omitempty"`
	Materials          []TemplateMaterialPreset `json:"materials,omitempty"`
}

type TemplateMaterialPreset struct {
	InputKey string `json:"inputKey"`
	AssetID  string `json:"assetId"`
}

type TemplateHandoffDefinition struct {
	TargetType string `json:"targetType"`
	TargetKey  string `json:"targetKey"`
	IntentKey  string `json:"intentKey,omitempty"`
}

type TemplateCapabilityDefinition struct {
	CapabilityKey string `json:"capabilityKey"`
	ModelHint     string `json:"modelHint,omitempty"`
}

type TemplateWorkflowDefinition struct {
	WorkflowVersion int                           `json:"workflowVersion"`
	ExecutorKey     string                        `json:"executorKey"`
	Nodes           []TemplateWorkflowNode        `json:"nodes"`
	Edges           []TemplateWorkflowEdge        `json:"edges"`
	InputBindings   []TemplateWorkflowIOBinding   `json:"inputBindings,omitempty"`
	OutputBindings  []TemplateWorkflowIOBinding   `json:"outputBindings,omitempty"`
	FailurePolicy   TemplateWorkflowFailurePolicy `json:"failurePolicy"`
}

type TemplateWorkflowNode struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	CapabilityKey string         `json:"capabilityKey,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
}

type TemplateWorkflowEdge struct {
	From      string                     `json:"from"`
	To        string                     `json:"to"`
	Condition *TemplateWorkflowCondition `json:"condition,omitempty"`
}

type TemplateWorkflowCondition struct {
	Source   string `json:"source"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

type TemplateWorkflowIOBinding struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type TemplateWorkflowFailurePolicy struct {
	Strategy string `json:"strategy"`
	Retries  int    `json:"retries,omitempty"`
}

type TemplateValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type TemplateComposeMaterial struct {
	InputKey string `json:"inputKey"`
	AssetID  string `json:"assetId"`
}

type TemplateComposition struct {
	BasePrompt     string                    `json:"basePrompt"`
	NegativePrompt string                    `json:"negativePrompt,omitempty"`
	Values         map[string]any            `json:"values"`
	Parameters     map[string]any            `json:"parameters"`
	Materials      []TemplateComposeMaterial `json:"materials"`
}

type PublicTemplateDefinition struct {
	Inputs       []PublicTemplateInput `json:"inputs"`
	Presentation map[string]any        `json:"presentation"`
	Presets      PublicTemplatePresets `json:"presets"`
	Handoff      PublicTemplateHandoff `json:"handoff"`
}

type PublicTemplateInput struct {
	Key         string                             `json:"key"`
	Type        TemplateInputType                  `json:"type"`
	Label       string                             `json:"label"`
	Required    bool                               `json:"required,omitempty"`
	HelpText    string                             `json:"helpText,omitempty"`
	Placeholder string                             `json:"placeholder,omitempty"`
	Default     any                                `json:"default,omitempty"`
	Options     []PublicTemplateInputOption        `json:"options,omitempty"`
	Validation  PublicTemplateInputValidation      `json:"validation,omitempty"`
	VisibleWhen *PublicTemplateVisibilityCondition `json:"visibleWhen,omitempty"`
}

type PublicTemplateInputOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

type PublicTemplateInputValidation struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinItems  *int     `json:"minItems,omitempty"`
	MaxItems  *int     `json:"maxItems,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Accept    []string `json:"accept,omitempty"`
}

type PublicTemplateVisibilityCondition struct {
	InputKey string `json:"inputKey"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

type PublicTemplatePresets struct {
	InputDefaults map[string]any `json:"inputDefaults,omitempty"`
}

type PublicTemplateHandoff struct {
	TargetType string `json:"targetType"`
}

var templateKeyPattern = regexp.MustCompile(`^[a-z][a-zA-Z0-9_]{0,63}$`)
var templatePromptVariablePattern = regexp.MustCompile(`\{\{\s*([a-z][a-zA-Z0-9_]*)\s*\}\}`)

func validateTemplateDefinition(contentType string, definition InternalTemplateDefinition) []TemplateValidationIssue {
	issues := make([]TemplateValidationIssue, 0)
	add := func(code, path, message string) {
		issues = append(issues, TemplateValidationIssue{Code: code, Path: path, Message: message})
	}
	if definition.SchemaVersion != currentTemplateSchemaVersion {
		add("SCHEMA_VERSION_UNSUPPORTED", "schemaVersion", fmt.Sprintf("schemaVersion must be %d", currentTemplateSchemaVersion))
	}
	contentType = strings.ToUpper(strings.TrimSpace(contentType))
	validContentTypes := map[string]bool{"IMAGE": true, "VIDEO": true, "PPT": true, "TEXT": true, "AGENT": true, "WORKFLOW": true}
	if !validContentTypes[contentType] {
		add("CONTENT_TYPE_UNSUPPORTED", "contentType", "unsupported template content type")
	}
	inputKeys := make(map[string]TemplateInputDefinition, len(definition.Inputs))
	validInputTypes := map[TemplateInputType]bool{
		TemplateInputText: true, TemplateInputTextarea: true, TemplateInputNumber: true,
		TemplateInputSelect: true, TemplateInputMultiSelect: true, TemplateInputBoolean: true,
		TemplateInputImage: true, TemplateInputVideo: true, TemplateInputFile: true,
	}
	for index, input := range definition.Inputs {
		path := fmt.Sprintf("inputs[%d]", index)
		if !templateKeyPattern.MatchString(input.Key) {
			add("INPUT_KEY_INVALID", path+".key", "input key must be a lower camel-case identifier")
		}
		if _, exists := inputKeys[input.Key]; exists {
			add("DUPLICATE_INPUT_KEY", path+".key", "input key must be unique")
		}
		inputKeys[input.Key] = input
		if !validInputTypes[input.Type] {
			add("INPUT_TYPE_UNSUPPORTED", path+".type", "unsupported input type")
		}
		if strings.TrimSpace(input.Label) == "" {
			add("INPUT_LABEL_REQUIRED", path+".label", "input label is required")
		}
		if input.Validation.MinLength != nil && input.Validation.MaxLength != nil && *input.Validation.MinLength > *input.Validation.MaxLength {
			add("INPUT_VALIDATION_INVALID", path+".validation", "minLength cannot exceed maxLength")
		}
		if input.Validation.Min != nil && input.Validation.Max != nil && *input.Validation.Min > *input.Validation.Max {
			add("INPUT_VALIDATION_INVALID", path+".validation", "min cannot exceed max")
		}
		if input.Validation.MinItems != nil && input.Validation.MaxItems != nil && *input.Validation.MinItems > *input.Validation.MaxItems {
			add("INPUT_VALIDATION_INVALID", path+".validation", "minItems cannot exceed maxItems")
		}
		if input.Validation.Pattern != "" {
			if _, err := regexp.Compile(input.Validation.Pattern); err != nil {
				add("INPUT_PATTERN_INVALID", path+".validation.pattern", "pattern must be a valid regular expression")
			}
		}
		if input.VisibleWhen != nil && input.VisibleWhen.InputKey == input.Key {
			add("VISIBILITY_SELF_REFERENCE", path+".visibleWhen.inputKey", "input cannot control its own visibility")
		}
	}
	for index, input := range definition.Inputs {
		if input.VisibleWhen != nil {
			if _, exists := inputKeys[input.VisibleWhen.InputKey]; !exists {
				add("VISIBILITY_INPUT_UNKNOWN", fmt.Sprintf("inputs[%d].visibleWhen.inputKey", index), "visibility input does not exist")
			}
		}
	}
	if strings.TrimSpace(definition.Prompt.Template) == "" && contentType != "WORKFLOW" {
		add("PROMPT_TEMPLATE_REQUIRED", "prompt.template", "prompt template is required")
	}
	if definition.Prompt.Composer.Key != "deterministic-template" || definition.Prompt.Composer.Version != 1 {
		add("COMPOSER_UNSUPPORTED", "prompt.composer", "composer must be deterministic-template version 1")
	}
	validTransforms := map[string]bool{"": true, TemplateTransformTrim: true, TemplateTransformEnumValue: true, TemplateTransformJoin: true, TemplateTransformToNumber: true, TemplateTransformAssetIDs: true}
	promptVariables := map[string]bool{}
	for index, binding := range definition.Bindings {
		path := fmt.Sprintf("bindings[%d]", index)
		if !validTransforms[binding.Transform] {
			add("BINDING_TRANSFORM_UNSUPPORTED", path+".transform", "unsupported binding transform")
		}
		if sourceKey, ok := strings.CutPrefix(binding.Source, "inputs."); ok {
			if _, exists := inputKeys[sourceKey]; !exists {
				add("BINDING_SOURCE_UNKNOWN", path+".source", "binding input does not exist")
			}
		} else if sourceKey, ok := strings.CutPrefix(binding.Source, "materials."); ok {
			input, exists := inputKeys[sourceKey]
			if !exists || (input.Type != TemplateInputImage && input.Type != TemplateInputVideo && input.Type != TemplateInputFile) {
				add("BINDING_SOURCE_UNKNOWN", path+".source", "binding material input does not exist")
			}
		} else {
			add("BINDING_SOURCE_INVALID", path+".source", "binding source must reference inputs or materials")
		}
		if variable, ok := strings.CutPrefix(binding.Target, "prompt.variables."); ok && templateKeyPattern.MatchString(variable) {
			promptVariables[variable] = true
		} else if parameter, ok := strings.CutPrefix(binding.Target, "parameters."); !ok || !templateKeyPattern.MatchString(parameter) {
			add("BINDING_TARGET_INVALID", path+".target", "binding target must reference prompt.variables or parameters")
		}
	}
	for _, templateText := range []struct {
		path string
		text string
	}{{"prompt.template", definition.Prompt.Template}, {"prompt.negativeTemplate", definition.Prompt.NegativeTemplate}} {
		for _, match := range templatePromptVariablePattern.FindAllStringSubmatch(templateText.text, -1) {
			if !promptVariables[match[1]] {
				add("PROMPT_VARIABLE_UNBOUND", templateText.path, "prompt variable "+match[1]+" has no binding")
			}
		}
	}
	if strings.TrimSpace(definition.Handoff.TargetType) == "" {
		add("HANDOFF_TARGET_TYPE_REQUIRED", "handoff.targetType", "handoff target type is required")
	}
	if strings.TrimSpace(definition.Handoff.TargetKey) == "" {
		add("HANDOFF_TARGET_KEY_REQUIRED", "handoff.targetKey", "handoff target key is required")
	}
	if strings.TrimSpace(definition.Capability.CapabilityKey) == "" {
		add("CAPABILITY_KEY_REQUIRED", "capability.capabilityKey", "capability key is required")
	}
	contentContracts := map[string]struct {
		targetType    string
		targetKey     string
		capabilityKey string
	}{
		"IMAGE":    {"IMAGE_CREATION", "image.create", "image_generation"},
		"VIDEO":    {"VIDEO_CREATION", "video.create", "video_generation"},
		"PPT":      {"PPT_CREATION", "ppt.create", "ppt_generation"},
		"TEXT":     {"TEXT_CREATION", "text.create", "text_generation"},
		"AGENT":    {"AGENT_CREATION", "agent.create", "agent_execution"},
		"WORKFLOW": {"WORKFLOW_CREATION", "workflow.create", "workflow_execution"},
	}
	if contract, exists := contentContracts[contentType]; exists {
		if definition.Handoff.TargetType != contract.targetType || definition.Handoff.TargetKey != contract.targetKey {
			add("HANDOFF_CONTENT_TYPE_MISMATCH", "handoff", "handoff target does not match content type")
		}
		if definition.Capability.CapabilityKey != contract.capabilityKey {
			add("CAPABILITY_CONTENT_TYPE_MISMATCH", "capability.capabilityKey", "capability does not match content type")
		}
	}
	if contentType == "WORKFLOW" {
		issues = append(issues, validateTemplateWorkflow(definition.Workflow)...)
	} else if definition.Workflow != nil {
		add("WORKFLOW_NOT_ALLOWED", "workflow", "workflow definition is allowed only for WORKFLOW templates")
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Path < issues[j].Path
	})
	return issues
}

func validateTemplateWorkflow(workflow *TemplateWorkflowDefinition) []TemplateValidationIssue {
	issues := make([]TemplateValidationIssue, 0)
	add := func(code, path, message string) {
		issues = append(issues, TemplateValidationIssue{Code: code, Path: path, Message: message})
	}
	if workflow == nil {
		add("WORKFLOW_REQUIRED", "workflow", "workflow definition is required")
		return issues
	}
	if workflow.WorkflowVersion < 1 {
		add("WORKFLOW_VERSION_INVALID", "workflow.workflowVersion", "workflowVersion must be positive")
	}
	if strings.TrimSpace(workflow.ExecutorKey) == "" {
		add("WORKFLOW_EXECUTOR_REQUIRED", "workflow.executorKey", "executorKey is required")
	}
	if len(workflow.Nodes) == 0 {
		add("WORKFLOW_NODES_REQUIRED", "workflow.nodes", "workflow must contain at least one node")
	}
	nodes := make(map[string]bool, len(workflow.Nodes))
	for index, node := range workflow.Nodes {
		path := fmt.Sprintf("workflow.nodes[%d]", index)
		if !templateKeyPattern.MatchString(node.ID) {
			add("WORKFLOW_NODE_ID_INVALID", path+".id", "workflow node id is invalid")
		}
		if nodes[node.ID] {
			add("WORKFLOW_NODE_DUPLICATE", path+".id", "workflow node id must be unique")
		}
		nodes[node.ID] = true
		if strings.TrimSpace(node.Type) == "" {
			add("WORKFLOW_NODE_TYPE_REQUIRED", path+".type", "workflow node type is required")
		}
	}
	adjacency := make(map[string][]string, len(nodes))
	for index, edge := range workflow.Edges {
		path := fmt.Sprintf("workflow.edges[%d]", index)
		if !nodes[edge.From] {
			add("WORKFLOW_EDGE_SOURCE_UNKNOWN", path+".from", "edge source node does not exist")
			continue
		}
		if !nodes[edge.To] {
			add("WORKFLOW_EDGE_TARGET_UNKNOWN", path+".to", "edge target node does not exist")
			continue
		}
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}
	state := map[string]int{}
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, next := range adjacency[node] {
			if visit(next) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range nodes {
		if visit(node) {
			add("WORKFLOW_CYCLE", "workflow.edges", "workflow graph must be acyclic")
			break
		}
	}
	validFailureStrategies := map[string]bool{"FAIL_FAST": true, "CONTINUE": true, "RETRY": true}
	if !validFailureStrategies[workflow.FailurePolicy.Strategy] {
		add("WORKFLOW_FAILURE_POLICY_INVALID", "workflow.failurePolicy.strategy", "unsupported workflow failure strategy")
	}
	if workflow.FailurePolicy.Strategy != "RETRY" && workflow.FailurePolicy.Retries != 0 {
		add("WORKFLOW_RETRIES_INVALID", "workflow.failurePolicy.retries", "retries are allowed only with RETRY strategy")
	}
	return issues
}

func composeTemplateDefinition(definition InternalTemplateDefinition, values map[string]any, materials []TemplateComposeMaterial) (TemplateComposition, error) {
	normalized := cloneTemplateMap(definition.Presets.InputDefaults)
	for _, input := range definition.Inputs {
		if _, exists := normalized[input.Key]; !exists && input.Default != nil {
			normalized[input.Key] = cloneJSONValue(input.Default)
		}
	}
	for key, value := range values {
		normalized[key] = cloneJSONValue(value)
	}
	inputs := make(map[string]TemplateInputDefinition, len(definition.Inputs))
	materialCounts := make(map[string]int)
	for _, material := range materials {
		if strings.TrimSpace(material.AssetID) == "" {
			return TemplateComposition{}, fmt.Errorf("material %s has an empty assetId", material.InputKey)
		}
		materialCounts[material.InputKey]++
	}
	for _, input := range definition.Inputs {
		inputs[input.Key] = input
		if templateMaterialInput(input.Type) {
			count := materialCounts[input.Key]
			minimum := 0
			if input.Required {
				minimum = 1
			}
			if input.Validation.MinItems != nil {
				minimum = *input.Validation.MinItems
			}
			if count < minimum {
				return TemplateComposition{}, fmt.Errorf("material input %s requires at least %d asset(s)", input.Key, minimum)
			}
			if input.Validation.MaxItems != nil && count > *input.Validation.MaxItems {
				return TemplateComposition{}, fmt.Errorf("material input %s accepts at most %d asset(s)", input.Key, *input.Validation.MaxItems)
			}
			continue
		}
		value, exists := normalized[input.Key]
		if input.Required && (!exists || emptyTemplateValue(value)) {
			return TemplateComposition{}, fmt.Errorf("required input %s is missing", input.Key)
		}
		if exists {
			if err := validateTemplateInputValue(input, value); err != nil {
				return TemplateComposition{}, fmt.Errorf("input %s: %w", input.Key, err)
			}
		}
	}
	for key := range normalized {
		if _, exists := inputs[key]; !exists {
			return TemplateComposition{}, fmt.Errorf("unknown input %s", key)
		}
	}
	for key := range materialCounts {
		input, exists := inputs[key]
		if !exists || !templateMaterialInput(input.Type) {
			return TemplateComposition{}, fmt.Errorf("unknown material input %s", key)
		}
	}
	promptVariables := map[string]string{}
	parameters := cloneTemplateMap(definition.Presets.GenerationDefaults)
	for _, binding := range definition.Bindings {
		value, err := templateBindingSource(binding.Source, normalized, materials)
		if err != nil {
			return TemplateComposition{}, err
		}
		value, err = applyTemplateBindingTransform(value, binding)
		if err != nil {
			return TemplateComposition{}, fmt.Errorf("binding %s: %w", binding.Source, err)
		}
		if variable, ok := strings.CutPrefix(binding.Target, "prompt.variables."); ok {
			promptVariables[variable] = templateString(value)
			continue
		}
		if parameter, ok := strings.CutPrefix(binding.Target, "parameters."); ok {
			parameters[parameter] = cloneTemplateBindingValue(value)
			continue
		}
		return TemplateComposition{}, fmt.Errorf("invalid binding target %s", binding.Target)
	}
	basePrompt, err := renderDeterministicPrompt(definition.Prompt.Template, promptVariables)
	if err != nil {
		return TemplateComposition{}, err
	}
	negativePrompt, err := renderDeterministicPrompt(definition.Prompt.NegativeTemplate, promptVariables)
	if err != nil {
		return TemplateComposition{}, err
	}
	return TemplateComposition{
		BasePrompt: strings.TrimSpace(basePrompt), NegativePrompt: strings.TrimSpace(negativePrompt),
		Values: normalized, Parameters: parameters, Materials: append([]TemplateComposeMaterial(nil), materials...),
	}, nil
}

func decodeInternalTemplateDefinition(raw []byte) (InternalTemplateDefinition, error) {
	var definition InternalTemplateDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return definition, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return definition, errors.New("template definition must contain exactly one JSON value")
		}
		return definition, err
	}
	return definition, nil
}

func validateTemplateInputValue(input TemplateInputDefinition, value any) error {
	switch input.Type {
	case TemplateInputText, TemplateInputTextarea:
		text, ok := value.(string)
		if !ok {
			return errors.New("must be a string")
		}
		length := len([]rune(text))
		if input.Validation.MinLength != nil && length < *input.Validation.MinLength {
			return fmt.Errorf("must contain at least %d characters", *input.Validation.MinLength)
		}
		if input.Validation.MaxLength != nil && length > *input.Validation.MaxLength {
			return fmt.Errorf("must contain at most %d characters", *input.Validation.MaxLength)
		}
		if input.Validation.Pattern != "" && !regexp.MustCompile(input.Validation.Pattern).MatchString(text) {
			return errors.New("does not match the required pattern")
		}
	case TemplateInputNumber:
		number, ok := templateNumber(value)
		if !ok {
			return errors.New("must be a number")
		}
		if input.Validation.Min != nil && number < *input.Validation.Min {
			return fmt.Errorf("must be at least %v", *input.Validation.Min)
		}
		if input.Validation.Max != nil && number > *input.Validation.Max {
			return fmt.Errorf("must be at most %v", *input.Validation.Max)
		}
	case TemplateInputSelect:
		if !templateOptionContains(input.Options, value) {
			return errors.New("must be one of the configured options")
		}
	case TemplateInputMultiSelect:
		items, ok := templateSlice(value)
		if !ok {
			return errors.New("must be an array")
		}
		for _, item := range items {
			if !templateOptionContains(input.Options, item) {
				return errors.New("contains an unsupported option")
			}
		}
	case TemplateInputBoolean:
		if _, ok := value.(bool); !ok {
			return errors.New("must be a boolean")
		}
	}
	return nil
}

func templateBindingSource(source string, values map[string]any, materials []TemplateComposeMaterial) (any, error) {
	if key, ok := strings.CutPrefix(source, "inputs."); ok {
		value, exists := values[key]
		if !exists {
			return nil, fmt.Errorf("binding source %s is missing", source)
		}
		return value, nil
	}
	if key, ok := strings.CutPrefix(source, "materials."); ok {
		assetIDs := make([]string, 0)
		for _, material := range materials {
			if material.InputKey == key {
				assetIDs = append(assetIDs, material.AssetID)
			}
		}
		return assetIDs, nil
	}
	return nil, fmt.Errorf("invalid binding source %s", source)
}

func applyTemplateBindingTransform(value any, binding TemplateBindingDefinition) (any, error) {
	switch binding.Transform {
	case "", TemplateTransformEnumValue, TemplateTransformAssetIDs:
		return value, nil
	case TemplateTransformTrim:
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("trim requires a string")
		}
		return strings.TrimSpace(text), nil
	case TemplateTransformJoin:
		items, ok := templateSlice(value)
		if !ok {
			return nil, errors.New("join requires an array")
		}
		values := make([]string, len(items))
		for index := range items {
			values[index] = templateString(items[index])
		}
		return strings.Join(values, firstNonEmptyString(binding.Separator, ", ")), nil
	case TemplateTransformToNumber:
		number, ok := templateNumber(value)
		if !ok {
			return nil, errors.New("toNumber requires a numeric value")
		}
		return number, nil
	default:
		return nil, fmt.Errorf("unsupported transform %s", binding.Transform)
	}
}

func renderDeterministicPrompt(templateText string, variables map[string]string) (string, error) {
	if templateText == "" {
		return "", nil
	}
	missing := ""
	rendered := templatePromptVariablePattern.ReplaceAllStringFunc(templateText, func(match string) string {
		parts := templatePromptVariablePattern.FindStringSubmatch(match)
		value, exists := variables[parts[1]]
		if !exists {
			missing = parts[1]
			return match
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("prompt variable %s is missing", missing)
	}
	return rendered, nil
}

func projectPublicTemplateDefinition(definition InternalTemplateDefinition) PublicTemplateDefinition {
	inputs := make([]PublicTemplateInput, len(definition.Inputs))
	for index, input := range definition.Inputs {
		options := make([]PublicTemplateInputOption, len(input.Options))
		for optionIndex, option := range input.Options {
			options[optionIndex] = PublicTemplateInputOption{Label: option.Label, Value: sanitizePublicTemplateValue(option.Value)}
		}
		validation := PublicTemplateInputValidation{
			MinLength: input.Validation.MinLength, MaxLength: input.Validation.MaxLength,
			Min: input.Validation.Min, Max: input.Validation.Max,
			MinItems: input.Validation.MinItems, MaxItems: input.Validation.MaxItems,
			Pattern: input.Validation.Pattern, Accept: append([]string(nil), input.Validation.Accept...),
		}
		var visibleWhen *PublicTemplateVisibilityCondition
		if input.VisibleWhen != nil {
			visibleWhen = &PublicTemplateVisibilityCondition{
				InputKey: input.VisibleWhen.InputKey, Operator: input.VisibleWhen.Operator,
				Value: sanitizePublicTemplateValue(input.VisibleWhen.Value),
			}
		}
		inputs[index] = PublicTemplateInput{
			Key: input.Key, Type: input.Type, Label: input.Label, Required: input.Required,
			HelpText: input.HelpText, Placeholder: input.Placeholder, Default: sanitizePublicTemplateValue(input.Default),
			Options: options, Validation: validation, VisibleWhen: visibleWhen,
		}
	}
	return PublicTemplateDefinition{
		Inputs: inputs, Presentation: sanitizePublicTemplateMap(definition.Presentation),
		Presets: PublicTemplatePresets{InputDefaults: sanitizePublicTemplateInputDefaults(definition.Presets.InputDefaults)},
		Handoff: PublicTemplateHandoff{TargetType: definition.Handoff.TargetType},
	}
}

func sanitizePublicTemplateInputDefaults(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = sanitizePublicTemplateValue(value)
	}
	return result
}

func sanitizePublicTemplateMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		if publicTemplateInternalKey(key) {
			continue
		}
		result[key] = sanitizePublicTemplateValue(value)
	}
	return result
}

func sanitizePublicTemplateValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizePublicTemplateMap(typed)
	case []any:
		items := make([]any, len(typed))
		for index := range typed {
			items[index] = sanitizePublicTemplateValue(typed[index])
		}
		return items
	default:
		return cloneJSONValue(value)
	}
}

func publicTemplateInternalKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
	for _, forbidden := range []string{"prompt", "composer", "binding", "model", "provider", "executor", "workflow", "failurepolicy", "apikey", "secret", "token", "capability"} {
		if strings.Contains(normalized, forbidden) {
			return true
		}
	}
	return false
}

func emptyTemplateValue(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	}
	return false
}

func templateOptionContains(options []TemplateInputOption, value any) bool {
	want, err := json.Marshal(value)
	if err != nil {
		return false
	}
	for _, option := range options {
		candidate, marshalErr := json.Marshal(option.Value)
		if marshalErr == nil && string(candidate) == string(want) {
			return true
		}
	}
	return false
}

func templateMaterialInput(inputType TemplateInputType) bool {
	return inputType == TemplateInputImage || inputType == TemplateInputVideo || inputType == TemplateInputFile
}

func cloneTemplateBindingValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	default:
		return cloneJSONValue(value)
	}
}

func templateSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		items := make([]any, len(typed))
		for index := range typed {
			items[index] = typed[index]
		}
		return items, true
	default:
		return nil, false
	}
}

func templateNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func templateString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		raw, _ := json.Marshal(typed)
		return string(raw)
	}
}

func cloneTemplateMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneJSONValue(value)
	}
	return cloned
}

func cloneJSONValue(value any) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned any
	if err = json.Unmarshal(raw, &cloned); err != nil {
		return value
	}
	return cloned
}
