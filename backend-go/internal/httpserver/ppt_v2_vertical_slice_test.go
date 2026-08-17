package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func phase1PPTTask(t *testing.T, service *pptapp.Service, userID string) string {
	t.Helper()
	response, err := service.Generate(pptapp.GenerateRequest{
		UserID: userID, ClientRequestID: "request_phase1", Prompt: "2027 年企业增长计划", SlideCount: 12,
		Language: "zh-CN", Audience: "管理层", Scenario: "年度经营会", Theme: "technology",
		EnableWebSearch: true, ImageSource: "ai", TextModel: "legacy-text-model", ImageModel: "legacy-image-model",
		Outline: &pptapp.Outline{Title: "2027 年企业增长计划", Slides: []pptapp.OutlineSlide{
			{Page: 1, Title: "2027 年企业增长计划", Summary: "从共识到执行", Layout: "cover", SlideType: "cover"},
			{Page: 2, Title: "三个增长支柱形成闭环", Summary: "产品建立价值，渠道放大触达，客户成功驱动续费。", BulletPoints: []string{"产品：聚焦高价值场景", "渠道：建立可复制打法", "客户成功：提升续费与扩容"}, Layout: "content", SlideType: "text_image"},
			{Page: 3, Title: "Phase 2", Summary: "Phase 1 does not render this page", Layout: "content", SlideType: "text_image"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return response.TaskID
}

func phase1StorageService() (*storagecenter.Service, *generatedStorageTestProvider) {
	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "access", SecretKey: "secret",
		Bucket: "private-files", DefaultQuotaBytes: 16 << 20, MaxUploadBytes: 16 << 20,
		MasterKey: "0123456789abcdef0123456789abcdef",
	})
	return service, provider
}

func phase1RendererCLIPath(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "packages", "ppt-v2", "src", "cli.mjs"))
}

func TestPPTV2VerticalSliceStoresPrivateArtifactAndRelatesExistingTaskAndWorkCenter(t *testing.T) {
	pptService := pptapp.NewService()
	taskID := phase1PPTTask(t, pptService, "user_phase1")
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	fileService, provider := phase1StorageService()
	a := api{store: store, pptService: pptService, fileService: fileService}
	user := adminUser{ID: "user_phase1", TenantID: "tenant_phase1", OrganizationID: "org_phase1", Role: "USER"}
	beforeBilling, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.generatePPTV2VerticalSlice(t.Context(), user, taskID, newNodePPTV2Renderer("node", phase1RendererCLIPath(t)))
	if err != nil {
		t.Fatal(err)
	}
	if result.DeckID == "" || result.Revision != 1 || len(result.PPTX) < 15_000 {
		t.Fatalf("unexpected renderer result: deck=%q revision=%d bytes=%d", result.DeckID, result.Revision, len(result.PPTX))
	}
	if result.File.Visibility != "PRIVATE" || result.File.UserID != user.ID || result.File.TenantID != user.TenantID || result.File.BusinessID != taskID {
		t.Fatalf("private file scope mismatch: %+v", result.File)
	}
	if _, ok := provider.objects[result.File.ObjectKey]; !ok {
		t.Fatalf("PPTX was not written to the configured private provider: %+v", result.File)
	}
	if result.Asset.UserID != user.ID || result.Asset.TenantID != user.TenantID || result.Asset.OrganizationID != user.OrganizationID || result.Asset.TaskID != taskID || result.Asset.MediaType != "ppt" {
		t.Fatalf("work-center owner relation mismatch: %+v", result.Asset)
	}
	if stringValue(result.Asset.Metadata["v2DeckId"]) != result.DeckID || intValue(result.Asset.Metadata["v2Revision"]) != 1 || stringValue(result.Asset.Metadata["fileId"]) != result.File.FileID {
		t.Fatalf("work-center V2 metadata mismatch: %+v", result.Asset.Metadata)
	}
	if result.Task.V2DeckID != result.DeckID || result.Task.V2Revision != 1 || result.Task.PPTXAssetID != result.Asset.ID {
		t.Fatalf("legacy task relation mismatch: %+v", result.Task)
	}
	if len(result.Task.Slides) != 3 || result.Task.Slides[2].Title != "Phase 2" {
		t.Fatalf("legacy slide JSON was rewritten: %+v", result.Task.Slides)
	}

	archive, err := zip.NewReader(bytes.NewReader(result.PPTX), int64(len(result.PPTX)))
	if err != nil {
		t.Fatal(err)
	}
	slideCount, notesCount := 0, 0
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slideCount++
		}
		if strings.HasPrefix(file.Name, "ppt/notesSlides/notesSlide") && strings.HasSuffix(file.Name, ".xml") {
			notesCount++
		}
	}
	if slideCount != 2 || notesCount != 2 {
		t.Fatalf("PPTX package counts: slides=%d notes=%d", slideCount, notesCount)
	}

	assets, err := store.ListAssets()
	if err != nil || len(assets) != 1 || assets[0].ID != result.Asset.ID {
		t.Fatalf("work-center asset missing: assets=%+v err=%v", assets, err)
	}
	signed := a.signStoredAssetURLs(t.Context(), user.ID, []asset{result.Asset})
	if len(signed) != 1 || !strings.Contains(signed[0].URL, "/download/"+result.File.ObjectKey) {
		t.Fatalf("owner did not receive a signed private URL: %+v", signed)
	}
	wrongOwner := a.signStoredAssetURLs(t.Context(), "other_user", []asset{result.Asset})
	if len(wrongOwner) != 1 || wrongOwner[0].URL != result.Asset.URL {
		t.Fatalf("wrong owner received private file access: %+v", wrongOwner)
	}
	afterBilling, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterBilling) != len(beforeBilling) {
		t.Fatalf("Phase 1 changed billing/generation task state: before=%d after=%d", len(beforeBilling), len(afterBilling))
	}
}

type recordingPPTV2Renderer struct {
	called bool
}

func (r *recordingPPTV2Renderer) Render(context.Context, pptV2LegacyInput) (pptV2RenderOutput, error) {
	r.called = true
	return pptV2RenderOutput{}, errors.New("renderer must not be called")
}

func TestPPTV2VerticalSliceRejectsWrongOwnerBeforeRenderingOrStorage(t *testing.T) {
	pptService := pptapp.NewService()
	taskID := phase1PPTTask(t, pptService, "owner")
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	fileService, provider := phase1StorageService()
	a := api{store: store, pptService: pptService, fileService: fileService}
	renderer := &recordingPPTV2Renderer{}

	_, err := a.generatePPTV2VerticalSlice(t.Context(), adminUser{ID: "other", TenantID: "tenant_phase1"}, taskID, renderer)
	if !errors.Is(err, pptapp.ErrTaskNotFound) {
		t.Fatalf("wrong owner error = %v", err)
	}
	if renderer.called || len(provider.objects) != 0 {
		t.Fatalf("wrong-owner request crossed the authorization boundary: renderer=%v objects=%d", renderer.called, len(provider.objects))
	}
	assets, listErr := store.ListAssets()
	if listErr != nil || len(assets) != 0 {
		t.Fatalf("wrong-owner request created work-center state: assets=%+v err=%v", assets, listErr)
	}
}
