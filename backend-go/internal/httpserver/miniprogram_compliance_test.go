package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type staticImageSecurityChecker struct {
	imageErr error
}

func (s staticImageSecurityChecker) CheckImage(context.Context, []byte, string, string) error {
	return s.imageErr
}

func (s staticImageSecurityChecker) CheckText(context.Context, string, string) error {
	return nil
}

func compliantMiniProgramModel(expiry string) adminAIModel {
	return adminAIModel{
		ID: "model_approved", ModelName: "qualified-image", ModelType: "image",
		ChannelID:    "channel_qualified",
		Provider:     "CloudBase",
		ProviderName: "qualified-provider", ProviderCompany: "Qualified Technology Co., Ltd.",
		AlgorithmName: "Qualified Image Algorithm", AlgorithmFilingNo: "ALG-TEST-001",
		ContractStatus: "valid", ContractExpireAt: expiry, ComplianceStatus: "approved",
		AllowedTerminals: []string{"pc", "web", "miniprogram"}, AllowedCapabilities: []string{"image"},
		MiniProgramEnabled: true, ModelVersion: "v1",
	}
}

func TestMiniProgramRejectsGatewayAsFilingSubjectAndClosedCreationMode(t *testing.T) {
	now := time.Now().UTC()
	model := compliantMiniProgramModel(now.Add(24 * time.Hour).Format(time.RFC3339))
	model.ProviderCompany = "new-api"
	if ok, reason := modelAllowedForMiniProgram(model, now); ok || reason != "gateway_cannot_be_filing_subject" {
		t.Fatalf("gateway filing subject gate = %v %s", ok, reason)
	}

	request := generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Model: "qualified-image", Params: map[string]any{"terminal": terminalMiniProgram},
	}
	data := adminPlatformData{AIModels: []adminAIModel{compliantMiniProgramModel(now.Add(24 * time.Hour).Format(time.RFC3339))}}
	if err := enforceMiniProgramModelCompliance(data, &request); err == nil {
		t.Fatal("closed mini-program video mode was accepted")
	}
}

func TestMiniProgramVideoComplianceBypassAllowsConfiguredVideoTask(t *testing.T) {
	t.Setenv("MINIPROGRAM_VIDEO_COMPLIANCE_BYPASS", "true")
	t.Setenv("MINIPROGRAM_CREATION_MODES", "image,video")
	model := adminAIModel{
		ID:         "model_video_unreviewed",
		ModelName:  "video-unreviewed",
		ModelType:  "video",
		ModuleCode: moduleVideoGeneration,
		Status:     "ACTIVE",
		ChannelID:  "channel_video",
	}
	request := generation.CreateRequest{
		Type:   "TEXT_TO_VIDEO",
		Model:  model.ModelName,
		Params: map[string]any{"terminal": terminalMiniProgram},
	}

	if err := enforceMiniProgramModelCompliance(adminPlatformData{AIModels: []adminAIModel{model}}, &request); err != nil {
		t.Fatalf("configured video model was rejected while bypass was enabled: %v", err)
	}
	if request.Params["configured_channel_id"] != "channel_video" {
		t.Fatalf("configured channel snapshot missing: %#v", request.Params)
	}
}

func TestMiniProgramVideoComplianceBypassAllowsVideoSchema(t *testing.T) {
	t.Setenv("MINIPROGRAM_VIDEO_COMPLIANCE_BYPASS", "true")
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	user := adminUser{ID: "user_test", TenantID: "default", PlanID: "plan_pro", Role: "USER"}
	requested, err := resolveModuleSchema(data, user, moduleVideoGeneration, "mock-video")
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveMiniProgramCompliantModuleSchema(data, user, moduleVideoGeneration, requested)
	if err != nil {
		t.Fatalf("video schema was rejected while bypass was enabled: %v", err)
	}
	if resolved.Model.ModelName != "mock-video" {
		t.Fatalf("resolved video model = %q", resolved.Model.ModelName)
	}
}

func TestRasterDownloadLabelIsRendered(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			source.Set(x, y, color.RGBA{R: 230, G: 240, B: 250, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, source); err != nil {
		t.Fatal(err)
	}
	marked, err := renderRasterAILabel(raw.Bytes(), "image/png", aiLabelSetting{Position: "bottom-right", Opacity: .65, Size: .035})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(raw.Bytes(), marked) {
		t.Fatal("marked PNG is identical to the original")
	}
	if _, _, err := image.Decode(bytes.NewReader(marked)); err != nil {
		t.Fatalf("marked PNG cannot be decoded: %v", err)
	}
}

func TestFormalOutputAuditFlagCannotPretendProviderIsConnected(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_OUTPUT_MODE", "formal")
	request := generation.CreateRequest{Prompt: "safe prompt", Params: map[string]any{"terminal": terminalMiniProgram}}
	if err := auditGeneratedOutput(&request); err == nil {
		t.Fatal("unconfigured formal output audit was allowed to approve content")
	}
	if request.Params["output_audit_status"] != auditManualReview || request.Params["output_audit_service"] != "formal-unconfigured" {
		t.Fatalf("unexpected fail-closed audit metadata: %#v", request.Params)
	}
}

func TestFormalGeneratedOutputUsesWeChatImageSecurity(t *testing.T) {
	t.Setenv("CONTENT_AUDIT_OUTPUT_MODE", "formal")
	raw := bytes.Buffer{}
	if err := png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	request := generation.CreateRequest{
		Params: map[string]any{"terminal": terminalMiniProgram},
		GeneratedImages: []generation.GeneratedImage{{
			URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw.Bytes()), ContentType: "image/png",
		}},
	}
	a := api{contentSecurity: staticImageSecurityChecker{}}
	if err := a.auditPreparedGeneratedOutput(context.Background(), &request); err != nil {
		t.Fatalf("formal output audit rejected safe image: %v", err)
	}
	if request.Params["output_audit_status"] != auditApproved || request.Params["output_audit_service"] != "wechat-content-security" {
		t.Fatalf("unexpected formal audit metadata: %#v", request.Params)
	}

	a.contentSecurity = staticImageSecurityChecker{imageErr: errContentSecurityRejected}
	if err := a.auditPreparedGeneratedOutput(context.Background(), &request); !errors.Is(err, errOutputAuditRejected) {
		t.Fatalf("explicit rejection returned %v", err)
	}
	if request.Params["output_audit_status"] != auditRejected {
		t.Fatalf("explicit rejection status: %#v", request.Params)
	}

	a.contentSecurity = staticImageSecurityChecker{imageErr: errContentSecurityUnavailable}
	if err := a.auditPreparedGeneratedOutput(context.Background(), &request); !errors.Is(err, errContentSecurityUnavailable) {
		t.Fatalf("unavailable audit returned %v", err)
	}
	if request.Params["output_audit_status"] != auditManualReview {
		t.Fatalf("unavailable audit status: %#v", request.Params)
	}
}

func TestMiniProgramGenerationRequiresPublishedAcceptedAgreements(t *testing.T) {
	if err := (api{}).enforceRequiredLegalAcceptances("user", terminalMiniProgram); err == nil {
		t.Fatal("mini-program generation bypassed unavailable agreement records")
	}
	if err := (api{}).enforceRequiredLegalAcceptances("user", "web"); err != nil {
		t.Fatalf("web terminal was incorrectly gated: %v", err)
	}
}

func TestLegalAcceptanceIDsAreStableAndDocumentSpecific(t *testing.T) {
	first := legalAcceptanceID("user_000002", terminalMiniProgram, "user-agreement", "2026-07-22")
	duplicate := legalAcceptanceID("user_000002", terminalMiniProgram, "user-agreement", "2026-07-22")
	second := legalAcceptanceID("user_000002", terminalMiniProgram, "privacy-policy", "2026-07-22")
	third := legalAcceptanceID("user_000002", terminalMiniProgram, "ai-content-rules", "2026-07-22")
	if first != duplicate {
		t.Fatalf("legal acceptance id is not stable: %q != %q", first, duplicate)
	}
	if first == second || first == third || second == third {
		t.Fatalf("different legal documents produced duplicate ids: %q %q %q", first, second, third)
	}
}

func TestMiniProgramModelComplianceGates(t *testing.T) {
	now := time.Now().UTC()
	model := compliantMiniProgramModel(now.Add(24 * time.Hour).Format(time.RFC3339))
	if ok, reason := modelAllowedForMiniProgram(model, now); !ok {
		t.Fatalf("approved model rejected: %s", reason)
	}
	model.ContractExpireAt = now.Add(-time.Minute).Format(time.RFC3339)
	if ok, reason := modelAllowedForMiniProgram(model, now); ok || reason != "contract_expired" {
		t.Fatalf("expired model gate = %v %s", ok, reason)
	}
	model = compliantMiniProgramModel(now.Add(24 * time.Hour).Format(time.RFC3339))
	model.AlgorithmFilingNo = ""
	if err := validateAIModelMiniProgramEnable(model); err == nil {
		t.Fatal("model with missing filing number was enabled")
	}
}

func TestForgedMiniProgramModelIDCannotBypassCompliance(t *testing.T) {
	data := adminPlatformData{AIModels: []adminAIModel{
		compliantMiniProgramModel(time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)),
		{ID: "model_unqualified", ModelName: "unqualified-image", ModelType: "image", Status: "ACTIVE"},
	}}
	request := generation.CreateRequest{Model: "unqualified-image", Params: map[string]any{"terminal": terminalMiniProgram}}
	if err := enforceMiniProgramModelCompliance(data, &request); err == nil {
		t.Fatal("forged unqualified model was accepted")
	}
	request.Model = "qualified-image"
	if err := enforceMiniProgramModelCompliance(data, &request); err != nil {
		t.Fatalf("qualified model rejected: %v", err)
	}
	if request.Params["algorithm_filing_no"] != "ALG-TEST-001" || request.Params["provider_company"] == "" || request.Params["configured_channel_id"] != "channel_qualified" {
		t.Fatalf("compliance snapshot missing: %#v", request.Params)
	}
}

func TestMiniProgramModuleSchemaSwitchesToCompliantModel(t *testing.T) {
	data := normalizeAICapabilityDefaults(adminPlatformData{})
	expiry := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	for index := range data.AIModels {
		switch data.AIModels[index].ModelName {
		case "gpt-image-2":
			data.AIModels[index].ComplianceStatus = "draft"
		case "mock-standard":
			qualified := compliantMiniProgramModel(expiry)
			qualified.ID = data.AIModels[index].ID
			qualified.ModelName = "mock-standard"
			qualified.ModuleCode = moduleImageGeneration
			qualified.Status = "ACTIVE"
			data.AIModels[index] = qualified
		}
	}
	user := adminUser{ID: "user_test", TenantID: "default", PlanID: "plan_pro", Role: "USER"}
	requested, err := resolveModuleSchema(data, user, moduleImageGeneration, "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveMiniProgramCompliantModuleSchema(data, user, moduleImageGeneration, requested)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.ModelName != "mock-standard" {
		t.Fatalf("mini-program model fallback = %q", resolved.Model.ModelName)
	}
}

func TestOutputAuditAndMarkedDownload(t *testing.T) {
	request := generation.CreateRequest{Prompt: "safe prompt", Params: map[string]any{"terminal": terminalMiniProgram}}
	if err := auditGeneratedOutput(&request); err != nil {
		t.Fatal(err)
	}
	if request.Params["output_audit_status"] != auditApproved || !boolValue(request.Params["ai_generated"]) {
		t.Fatalf("unexpected output audit metadata: %#v", request.Params)
	}
	asset := asset{ID: "asset_1", Name: "result", URL: promptPreviewImage("safe"), Metadata: map[string]any{"ai_generated": true, "output_audit_status": auditApproved, "contentType": "image/svg+xml"}}
	response := httptest.NewRecorder()
	if handled := (api{}).writeCompliantAssetDownload(response, httptest.NewRequest("GET", "/download", nil), asset); !handled {
		t.Fatal("AI-generated download bypassed compliance writer")
	}
	if response.Code != 200 || !containsAll(response.Body.String(), "AI生成", "ai-generated-label") {
		t.Fatalf("marked download status=%d body=%s", response.Code, response.Body.String())
	}
}

func containsAll(value string, items ...string) bool {
	for _, item := range items {
		if !stringContains(value, item) {
			return false
		}
	}
	return true
}

func stringContains(value string, item string) bool {
	for index := 0; index+len(item) <= len(value); index++ {
		if value[index:index+len(item)] == item {
			return true
		}
	}
	return false
}
