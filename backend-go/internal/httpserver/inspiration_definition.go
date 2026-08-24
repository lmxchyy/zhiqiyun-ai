package httpserver

import inspiration "xianzhi-ai/backend-go/internal/inspiration"

const currentTemplateSchemaVersion = inspiration.CurrentTemplateSchemaVersion

const (
	TemplateInputText        = inspiration.TemplateInputText
	TemplateInputTextarea    = inspiration.TemplateInputTextarea
	TemplateInputNumber      = inspiration.TemplateInputNumber
	TemplateInputSelect      = inspiration.TemplateInputSelect
	TemplateInputMultiSelect = inspiration.TemplateInputMultiSelect
	TemplateInputBoolean     = inspiration.TemplateInputBoolean
	TemplateInputImage       = inspiration.TemplateInputImage
	TemplateInputVideo       = inspiration.TemplateInputVideo
	TemplateInputFile        = inspiration.TemplateInputFile
)

const (
	TemplateTransformTrim      = inspiration.TemplateTransformTrim
	TemplateTransformEnumValue = inspiration.TemplateTransformEnumValue
	TemplateTransformJoin      = inspiration.TemplateTransformJoin
	TemplateTransformToNumber  = inspiration.TemplateTransformToNumber
	TemplateTransformAssetIDs  = inspiration.TemplateTransformAssetIDs
)

type TemplateInputType = inspiration.TemplateInputType
type InternalTemplateDefinition = inspiration.InternalTemplateDefinition
type TemplateInputDefinition = inspiration.TemplateInputDefinition
type TemplateInputOption = inspiration.TemplateInputOption
type TemplateInputValidation = inspiration.TemplateInputValidation
type TemplateVisibilityCondition = inspiration.TemplateVisibilityCondition
type TemplatePromptDefinition = inspiration.TemplatePromptDefinition
type TemplateComposerDefinition = inspiration.TemplateComposerDefinition
type TemplateBindingDefinition = inspiration.TemplateBindingDefinition
type TemplatePresetsDefinition = inspiration.TemplatePresetsDefinition
type TemplateMaterialPreset = inspiration.TemplateMaterialPreset
type TemplateHandoffDefinition = inspiration.TemplateHandoffDefinition
type TemplateCapabilityDefinition = inspiration.TemplateCapabilityDefinition
type TemplateWorkflowDefinition = inspiration.TemplateWorkflowDefinition
type TemplateWorkflowNode = inspiration.TemplateWorkflowNode
type TemplateWorkflowEdge = inspiration.TemplateWorkflowEdge
type TemplateWorkflowCondition = inspiration.TemplateWorkflowCondition
type TemplateWorkflowIOBinding = inspiration.TemplateWorkflowIOBinding
type TemplateWorkflowFailurePolicy = inspiration.TemplateWorkflowFailurePolicy
type TemplateValidationIssue = inspiration.TemplateValidationIssue
type TemplateComposeMaterial = inspiration.TemplateComposeMaterial
type TemplateComposition = inspiration.TemplateComposition
type PublicTemplateDefinition = inspiration.PublicTemplateDefinition
type PublicTemplateInput = inspiration.PublicTemplateInput
type PublicTemplateInputOption = inspiration.PublicTemplateInputOption
type PublicTemplateInputValidation = inspiration.PublicTemplateInputValidation
type PublicTemplateVisibilityCondition = inspiration.PublicTemplateVisibilityCondition
type PublicTemplatePresets = inspiration.PublicTemplatePresets
type PublicTemplateHandoff = inspiration.PublicTemplateHandoff

func validateTemplateDefinition(contentType string, definition InternalTemplateDefinition) []TemplateValidationIssue {
	return inspiration.ValidateTemplateDefinition(contentType, definition)
}

func composeTemplateDefinition(definition InternalTemplateDefinition, values map[string]any, materials []TemplateComposeMaterial) (TemplateComposition, error) {
	return inspiration.ComposeTemplateDefinition(definition, values, materials)
}

func decodeInternalTemplateDefinition(raw []byte) (InternalTemplateDefinition, error) {
	return inspiration.DecodeInternalTemplateDefinition(raw)
}

func projectPublicTemplateDefinition(definition InternalTemplateDefinition) PublicTemplateDefinition {
	return inspiration.ProjectPublicTemplateDefinition(definition)
}

func templateMaterialInput(inputType TemplateInputType) bool {
	return inspiration.TemplateMaterialInput(inputType)
}

func cloneTemplateMap(source map[string]any) map[string]any {
	return inspiration.CloneTemplateMap(source)
}
