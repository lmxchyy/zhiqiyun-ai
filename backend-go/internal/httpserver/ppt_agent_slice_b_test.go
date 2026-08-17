package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptAgentHTTPContentPort struct{}

func (pptAgentHTTPContentPort) PlanSlideContent(_ context.Context, input pptapp.SlideContentPlanningInput) (pptapp.SlideContentPlanningOutput, error) {
	layout := "title-bullets"
	assetIntents := []pptapp.SlideAssetIntent(nil)
	for _, elementType := range input.Objective.ExpectedElementTypes {
		if strings.EqualFold(elementType, "IMAGE") {
			layout = "text-image"
			assetIntents = []pptapp.SlideAssetIntent{{ID: "market-image", Kind: "image", Prompt: "Professional electric vehicle market photograph", AltText: "Electric vehicle market"}}
			break
		}
	}
	return pptapp.SlideContentPlanningOutput{
		Draft: pptapp.SlideContentDraft{
			Language: input.Intent.Language, Title: input.Objective.Title,
			BodyBlocks:     []pptapp.SlideBodyBlock{{Heading: "Management finding", Text: input.Objective.KeyMessage}},
			Bullets:        []string{"Verified evidence", "Management implication"},
			SupportingText: input.Objective.KeyMessage,
			SpeakerNotes:   "Explain the approved evidence and its management implication.",
			AssetIntents:   assetIntents,
			CitationRefs:   append([]string(nil), input.Objective.EvidenceRefs...),
			LayoutHint:     layout,
		},
		Provenance: pptapp.PlanningProvenance{Mode: pptapp.PlanningModeDeterministicTest, Provider: "http-test", Model: "content-fixture"},
	}, nil
}

type pptAgentHTTPAssetPort struct{ calls int }

func (p *pptAgentHTTPAssetPort) ResolveImage(_ context.Context, scope pptapp.GenerationJobScope, _ string, slideID string, intent pptapp.SlideAssetIntent) (pptapp.ResolvedDeckAsset, error) {
	p.calls++
	return pptapp.ResolvedDeckAsset{
		ID: "image_asset_1", TenantID: scope.TenantID, UserID: scope.UserID,
		IntentID: intent.StableID, SlideID: slideID, MIMEType: "image/png",
		URI: "asset://ppt-v2/image_asset_1.png", SHA256: strings.Repeat("a", 64),
		FileID: "image_file_1", AltText: intent.AltText,
	}, nil
}

type pptAgentHTTPCompiler struct{}

func (pptAgentHTTPCompiler) Compile(_ context.Context, input pptapp.DeckBuildInput) (pptapp.DeckCompilation, error) {
	return pptapp.DeckCompilation{
		DeckID: "deck_" + input.GenerationJobID, Revision: input.Revision,
		SlideCount: len(input.ApprovedOutline.Slides), Deck: json.RawMessage(`{"valid":true}`),
		LayoutResult: json.RawMessage(`{"valid":true}`), RenderInput: json.RawMessage(`{"valid":true}`),
		QualityValid: true,
	}, nil
}

func (pptAgentHTTPCompiler) Render(_ context.Context, input pptapp.DeckCompilation, _ []pptapp.ResolvedDeckAsset) (pptapp.DeckRenderOutput, error) {
	return pptapp.DeckRenderOutput{DeckID: input.DeckID, Revision: input.Revision, SlideCount: input.SlideCount, PPTX: []byte("PK-slice-b-http-pptx")}, nil
}

type pptAgentHTTPArtifactPort struct {
	jobs  *pptapp.MemoryGenerationJobStore
	files *storagecenter.Service
}

func (p pptAgentHTTPArtifactPort) EnsureTask(_ context.Context, _ pptapp.GenerationJobScope, jobID string, _ pptapp.IntentSpec, _ pptapp.OutlinePlan, _ []pptapp.SlideContent) (string, error) {
	return "task_" + jobID, nil
}

func (p pptAgentHTTPArtifactPort) StorePPTX(ctx context.Context, scope pptapp.GenerationJobScope, jobID, title string, data []byte) (string, error) {
	stored, err := p.files.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: scope.TenantID, UserID: scope.UserID, FileName: title + ".pptx", FileSize: int64(len(data)),
		MIMEType: pptxMIMEType, BusinessType: "ppt_v2_generation", BusinessID: jobID, Visibility: "PRIVATE",
	}, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return stored.FileID, nil
}

func (p pptAgentHTTPArtifactPort) EnsureArtifact(ctx context.Context, lease pptapp.GenerationLease, _, _, deckID string, _ int) (string, pptapp.GenerationJob, error) {
	state, err := p.jobs.GetAgentPlanning(ctx, pptapp.GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID)
	if err != nil {
		return "", pptapp.GenerationJob{}, err
	}
	assetID := "artifact_" + deckID
	updated, err := p.jobs.SaveAgentDeckCheckpoint(ctx, lease, pptapp.AgentDeckCheckpoint{
		ExpectedStage: pptapp.GenerationStageFileStored, NextStage: pptapp.GenerationStageAssetCreated,
		State: *state.DeckGeneration, CompletedWorkUnits: lease.Job.TotalWorkUnits - 1, AssetID: assetID, Now: lease.Job.UpdatedAt,
	})
	return assetID, updated.Job, err
}

func (p pptAgentHTTPArtifactPort) RelateTask(ctx context.Context, lease pptapp.GenerationLease, _ string, _ pptapp.V2ArtifactRelation) (pptapp.GenerationJob, error) {
	state, err := p.jobs.GetAgentPlanning(ctx, pptapp.GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID)
	if err != nil {
		return pptapp.GenerationJob{}, err
	}
	updated, err := p.jobs.SaveAgentDeckCheckpoint(ctx, lease, pptapp.AgentDeckCheckpoint{
		ExpectedStage: pptapp.GenerationStageAssetCreated, NextStage: pptapp.GenerationStageTaskRelated,
		State: *state.DeckGeneration, CompletedWorkUnits: lease.Job.TotalWorkUnits, Now: lease.Job.UpdatedAt,
	})
	return updated.Job, err
}

func TestPPTAgentSliceBHTTPGeneratesAndDownloadsPrivateMultiPageDeck(t *testing.T) {
	a, token, jobs, _ := pptAgentHTTPFixture(t)
	files, _ := phase1StorageService()
	a.fileService = files
	imageAssets := &pptAgentHTTPAssetPort{}
	if err := a.pptAgentService.ConfigureDeckGeneration(pptAgentHTTPContentPort{}, imageAssets, pptAgentHTTPCompiler{}, pptAgentHTTPArtifactPort{jobs: jobs, files: files}); err != nil {
		t.Fatal(err)
	}

	guideResponse := httptest.NewRecorder()
	a.guidePPTAgent(guideResponse, pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/guide", []byte(`{"idempotencyKey":"slice-b-http","text":"Create an 8-page EV market analysis for management.","pageCount":8,"language":"en"}`)))
	if guideResponse.Code != http.StatusOK {
		t.Fatalf("guide status=%d body=%s", guideResponse.Code, guideResponse.Body.String())
	}
	var guided pptapp.AgentGuideResult
	if err := json.Unmarshal(guideResponse.Body.Bytes(), &guided); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := a.pptAgentService.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	state, err := a.pptAgentService.Get(t.Context(), pptapp.GenerationJobScope{TenantID: guided.State.Job.TenantID, UserID: guided.State.Job.UserID}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := a.pptAgentService.ApproveOutline(t.Context(), pptapp.GenerationJobScope{TenantID: state.Job.TenantID, UserID: state.Job.UserID}, state.Job.ID, state.Outline.Revision, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.pptAgentService.ProcessReady(t.Context(), now.Add(3*time.Second), 10); err != nil {
		t.Fatal(err)
	}
	completed, err := a.pptAgentService.Get(t.Context(), pptapp.GenerationJobScope{TenantID: approved.Job.TenantID, UserID: approved.Job.UserID}, approved.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Job.Status != pptapp.GenerationJobSucceeded || completed.Job.Stage != pptapp.GenerationStageCompleted || completed.Job.Progress() != 100 || len(completed.DeckGeneration.Contents) != 8 || imageAssets.calls != 1 {
		t.Fatalf("Slice B job did not complete: state=%+v imageCalls=%d", completed, imageAssets.calls)
	}

	downloadRequest := pptAgentHTTPRequest(t, token, http.MethodGet, "/api/v1/ppt/agent/jobs/"+completed.Job.ID+"/download", nil)
	downloadRequest.SetPathValue("jobId", completed.Job.ID)
	downloadResponse := httptest.NewRecorder()
	a.downloadPPTAgentDeck(downloadResponse, downloadRequest)
	if downloadResponse.Code != http.StatusOK || !strings.Contains(downloadResponse.Body.String(), "https://storage.example/download/") || !strings.Contains(downloadResponse.Body.String(), completed.Job.FileID) {
		t.Fatalf("download status=%d body=%s", downloadResponse.Code, downloadResponse.Body.String())
	}

	other, err := a.store.CreateAdminCustomer(adminCustomerMutation{Name: "Other tenant", Email: "other-slice-b@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.sessions.Put(t.Context(), "other-slice-b-token", other.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	wrongOwnerRequest := pptAgentHTTPRequest(t, "other-slice-b-token", http.MethodGet, "/api/v1/ppt/agent/jobs/"+completed.Job.ID+"/download", nil)
	wrongOwnerRequest.SetPathValue("jobId", completed.Job.ID)
	wrongOwnerResponse := httptest.NewRecorder()
	a.downloadPPTAgentDeck(wrongOwnerResponse, wrongOwnerRequest)
	if wrongOwnerResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-owner download status=%d body=%s", wrongOwnerResponse.Code, wrongOwnerResponse.Body.String())
	}
}

func TestPPTAgentSliceBGoToNodeCompilerRendersApprovedOutlineWithPrivateImage(t *testing.T) {
	a, _, _, _ := pptAgentHTTPFixture(t)
	files, _ := phase1StorageService()
	response, err := a.pptAgentService.Guide(t.Context(), pptapp.GuideAgentRequest{
		TenantID: "tenant_node_contract", UserID: "user_node_contract", OrganizationID: "org_node_contract",
		IdempotencyKey: "slice-b-node-contract", Request: pptapp.IntentRequest{Text: "Create an 8-page EV market analysis for management.", PageCount: 8, Language: "en"}, Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := a.pptAgentService.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	scope := pptapp.GenerationJobScope{TenantID: response.State.Job.TenantID, UserID: response.State.Job.UserID}
	planned, err := a.pptAgentService.Get(t.Context(), scope, response.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := a.pptAgentService.ApproveOutline(t.Context(), scope, planned.Job.ID, planned.Outline.Revision, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	contentPort := pptAgentHTTPContentPort{}
	contents := make([]pptapp.SlideContent, 0, len(approved.ApprovedOutline.Slides))
	assets := []pptapp.ResolvedDeckAsset{}
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(imageBytes)
	imageSHA := hex.EncodeToString(digest[:])
	for _, objective := range approved.ApprovedOutline.Slides {
		input := pptapp.SlideContentPlanningInput{Intent: approved.Intent, Research: approved.Research, Storyline: approved.Storyline, ApprovedOutline: *approved.ApprovedOutline, Objective: objective}
		output, err := contentPort.PlanSlideContent(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		content, err := pptapp.MaterializeSlideContent(input, output)
		if err != nil {
			t.Fatal(err)
		}
		contents = append(contents, content)
		for _, intent := range content.AssetIntents {
			stored, err := files.StoreObject(t.Context(), storagecenter.UploadInitInput{
				TenantID: scope.TenantID, UserID: scope.UserID, FileName: intent.StableID + ".png", FileSize: int64(len(imageBytes)),
				MIMEType: "image/png", BusinessType: "ppt_v2_image_asset", BusinessID: planned.Job.ID + ":" + intent.StableID, Visibility: "PRIVATE",
			}, bytes.NewReader(imageBytes))
			if err != nil {
				t.Fatal(err)
			}
			assets = append(assets, pptapp.ResolvedDeckAsset{
				ID: "asset_" + imageSHA[:16], TenantID: scope.TenantID, UserID: scope.UserID,
				IntentID: intent.StableID, SlideID: objective.SlideID, MIMEType: "image/png",
				URI: "asset://ppt-v2/" + imageSHA[:24], SHA256: imageSHA, FileID: stored.FileID, AltText: intent.AltText,
			})
		}
	}

	compiler := newConfiguredPPTV2AgentDeckCompiler(files)
	compiled, err := compiler.Compile(t.Context(), pptapp.DeckBuildInput{
		GenerationJobID: planned.Job.ID, Revision: approved.ApprovedOutline.Revision,
		Intent: approved.Intent, Research: approved.Research, Storyline: approved.Storyline,
		ApprovedOutline: *approved.ApprovedOutline, SlideContents: contents, Assets: assets,
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := compiler.Render(t.Context(), compiled, assets)
	if err != nil {
		t.Fatal(err)
	}
	if !compiled.QualityValid || compiled.SlideCount != 8 || rendered.SlideCount != 8 || len(rendered.PPTX) < 10_000 {
		t.Fatalf("Go to Node compilation mismatch: compiled=%+v renderedBytes=%d", compiled, len(rendered.PPTX))
	}
	archive, err := zip.NewReader(bytes.NewReader(rendered.PPTX), int64(len(rendered.PPTX)))
	if err != nil {
		t.Fatal(err)
	}
	slideCount := 0
	for _, file := range archive.File {
		if strings.HasPrefix(file.Name, "ppt/slides/slide") && strings.HasSuffix(file.Name, ".xml") {
			slideCount++
		}
	}
	if slideCount != 8 {
		t.Fatalf("rendered PPTX slide count=%d", slideCount)
	}
}
