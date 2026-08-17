package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptAgentPreviewAssetPort struct {
	files *storagecenter.Service
	calls int
}

func (p *pptAgentPreviewAssetPort) ResolveImage(ctx context.Context, scope pptapp.GenerationJobScope, jobID, slideID string, intent pptapp.SlideAssetIntent) (pptapp.ResolvedDeckAsset, error) {
	p.calls++
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		return pptapp.ResolvedDeckAsset{}, err
	}
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	stored, err := p.files.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: scope.TenantID, UserID: scope.UserID, FileName: intent.StableID + ".png", FileSize: int64(len(data)),
		MIMEType: "image/png", BusinessType: "ppt_v2_image_asset", BusinessID: jobID + ":" + intent.StableID, Visibility: "PRIVATE",
	}, bytes.NewReader(data))
	if err != nil {
		return pptapp.ResolvedDeckAsset{}, err
	}
	return pptapp.ResolvedDeckAsset{
		ID: "asset_" + sha[:16], IntentID: intent.StableID, SlideID: slideID, MIMEType: "image/png",
		URI: "asset://ppt-v2/" + sha[:24], SHA256: sha, FileID: stored.FileID, AltText: intent.AltText,
		TenantID: scope.TenantID, UserID: scope.UserID,
	}, nil
}

type pptAgentPreviewCompiler struct{}

func (pptAgentPreviewCompiler) Compile(_ context.Context, input pptapp.DeckBuildInput) (pptapp.DeckCompilation, error) {
	deckID := "deck_" + input.GenerationJobID
	assetsBySlide := make(map[string]pptapp.ResolvedDeckAsset, len(input.Assets))
	manifest := make([]map[string]any, 0, len(input.Assets))
	for _, asset := range input.Assets {
		assetsBySlide[asset.SlideID] = asset
		manifest = append(manifest, map[string]any{"id": asset.ID, "type": "image", "mimeType": asset.MIMEType, "uri": asset.URI, "sha256": asset.SHA256})
	}
	slides := make([]map[string]any, 0, len(input.ApprovedOutline.Slides))
	layoutSlides := make([]map[string]any, 0, len(input.ApprovedOutline.Slides))
	for index, objective := range input.ApprovedOutline.Slides {
		titleID := "element_" + objective.SlideID + "_title"
		bulletsID := "element_" + objective.SlideID + "_bullets"
		visualID := "element_" + objective.SlideID + "_visual"
		elements := []map[string]any{
			{"id": titleID, "type": "text", "slot": "title", "content": map[string]any{"kind": "plain", "text": objective.Title}, "styleRole": "professionalTitle"},
			{"id": bulletsID, "type": "text", "slot": "bullets", "content": map[string]any{"kind": "bullets", "items": []string{objective.Purpose, objective.KeyMessage}}, "styleRole": "professionalBody"},
		}
		layoutElements := []map[string]any{
			{"elementId": titleID, "x": 72, "y": 64, "width": 816, "height": 54, "zIndex": 3, "resolvedStyle": map[string]any{"kind": "text", "fontFace": "Aptos", "fontSizePt": 28, "color": "#172033", "bold": true, "italic": false, "align": "left", "verticalAlign": "middle", "marginPt": 0}},
			{"elementId": bulletsID, "x": 72, "y": 160, "width": 420, "height": 220, "zIndex": 2, "resolvedStyle": map[string]any{"kind": "text", "fontFace": "Aptos", "fontSizePt": 18, "color": "#344054", "bold": false, "italic": false, "align": "left", "verticalAlign": "top", "marginPt": 8}},
		}
		if asset, ok := assetsBySlide[objective.SlideID]; ok {
			elements = append(elements, map[string]any{"id": visualID, "type": "image", "slot": "image", "assetRef": asset.ID, "fit": "cover", "altText": asset.AltText, "citationRefs": objective.EvidenceRefs})
			layoutElements = append(layoutElements, map[string]any{"elementId": visualID, "x": 520, "y": 160, "width": 368, "height": 220, "zIndex": 1, "resolvedStyle": map[string]any{"kind": "image", "fit": "cover"}})
		} else {
			elements = append(elements, map[string]any{"id": visualID, "type": "shape", "slot": "visual", "shapeType": "roundRect", "styleRole": "professionalCard"})
			layoutElements = append(layoutElements, map[string]any{"elementId": visualID, "x": 520, "y": 160, "width": 368, "height": 220, "zIndex": 1, "resolvedStyle": map[string]any{"kind": "shape", "shapeType": "roundRect", "fillColor": "#E8EEF8", "lineColor": "#B5C4DD", "lineWidthPt": 1, "transparency": 0}})
		}
		slides = append(slides, map[string]any{
			"id": objective.SlideID, "sequence": index + 1, "role": "content", "layoutId": "layout_professional_title_bullets_v1",
			"backgroundToken": "background", "speakerNotes": "Source-backed management finding.", "objectiveId": objective.SlideID,
			"keyMessage": objective.KeyMessage, "evidenceRequired": objective.EvidenceRequired, "citationRefs": objective.EvidenceRefs, "elements": elements,
		})
		layoutSlides = append(layoutSlides, map[string]any{
			"slideId": objective.SlideID, "layoutId": "layout_professional_title_bullets_v1", "backgroundColor": "#FFFFFF", "elements": layoutElements,
		})
	}
	sources := make([]map[string]any, 0, len(input.Research.Sources))
	for _, source := range input.Research.Sources {
		sources = append(sources, map[string]any{"id": source.ID, "title": source.Title, "type": source.Type, "locator": source.Locator})
	}
	citations := make([]map[string]any, 0, len(input.Research.Citations))
	for _, citation := range input.Research.Citations {
		citations = append(citations, map[string]any{"id": citation.ID, "sourceId": citation.SourceID, "locator": citation.Locator})
	}
	claims := make([]map[string]any, 0, len(input.Research.Claims))
	for _, claim := range input.Research.Claims {
		claims = append(claims, map[string]any{"id": claim.ID, "sourceId": claim.SourceID, "citationRefs": claim.CitationRefs, "text": claim.Text, "verificationStatus": claim.VerificationStatus})
	}
	deck, err := json.Marshal(map[string]any{
		"contractVersion": "2.1", "deckId": deckID, "revision": input.Revision,
		"deckSpec":      map[string]any{"title": input.Intent.Topic, "language": input.Intent.Language},
		"assetManifest": manifest, "provenance": map[string]any{"sources": sources, "citations": citations, "claims": claims}, "slides": slides,
	})
	if err != nil {
		return pptapp.DeckCompilation{}, err
	}
	layout, err := json.Marshal(map[string]any{
		"contractVersion": "2.1", "deckId": deckID, "revision": input.Revision,
		"canvas": map[string]any{"unit": "pt", "width": 960, "height": 540}, "slides": layoutSlides,
	})
	if err != nil {
		return pptapp.DeckCompilation{}, err
	}
	return pptapp.DeckCompilation{DeckID: deckID, Revision: input.Revision, SlideCount: len(slides), Deck: deck, LayoutResult: layout, RenderInput: json.RawMessage(`{"valid":true}`), QualityValid: true}, nil
}

func (pptAgentPreviewCompiler) Render(_ context.Context, input pptapp.DeckCompilation, _ []pptapp.ResolvedDeckAsset) (pptapp.DeckRenderOutput, error) {
	return pptapp.DeckRenderOutput{DeckID: input.DeckID, Revision: input.Revision, SlideCount: input.SlideCount, PPTX: []byte("PK-slice-c-preview-pptx")}, nil
}

func TestPPTAgentSliceCPreviewReturnsAuthoritativeGeometryAndTenantSafeAssets(t *testing.T) {
	a, token, jobs, _ := pptAgentHTTPFixture(t)
	files, _ := phase1StorageService()
	a.fileService = files
	assets := &pptAgentPreviewAssetPort{files: files}
	if err := a.pptAgentService.ConfigureDeckGeneration(pptAgentHTTPContentPort{}, assets, pptAgentPreviewCompiler{}, pptAgentHTTPArtifactPort{jobs: jobs, files: files}); err != nil {
		t.Fatal(err)
	}

	guideResponse := httptest.NewRecorder()
	a.guidePPTAgent(guideResponse, pptAgentHTTPRequest(t, token, http.MethodPost, "/api/v1/ppt/agent/guide", []byte(`{"idempotencyKey":"slice-c-preview","text":"Create an 8-page EV market analysis for management.","pageCount":8,"language":"en"}`)))
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
	scope := pptapp.GenerationJobScope{TenantID: guided.State.Job.TenantID, UserID: guided.State.Job.UserID}
	planned, err := a.pptAgentService.Get(t.Context(), scope, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.pptAgentService.ApproveOutline(t.Context(), scope, planned.Job.ID, planned.Outline.Revision, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := a.pptAgentService.ProcessReady(t.Context(), now.Add(3*time.Second), 10); err != nil {
		t.Fatal(err)
	}
	completed, err := a.pptAgentService.Get(t.Context(), scope, planned.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Job.Status != pptapp.GenerationJobSucceeded || assets.calls != 1 {
		t.Fatalf("preview fixture did not complete: status=%s assets=%d", completed.Job.Status, assets.calls)
	}

	request := pptAgentHTTPRequest(t, token, http.MethodGet, "/api/v1/ppt/agent/jobs/"+completed.Job.ID+"/preview?revision="+strconv.Itoa(completed.Job.Revision)+"&assetId=arbitrary_unowned_asset", nil)
	request.SetPathValue("jobId", completed.Job.ID)
	response := httptest.NewRecorder()
	a.previewPPTAgentDeck(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", response.Code, response.Body.String())
	}
	var projection struct {
		DeckID       string          `json:"deckId"`
		Revision     int             `json:"revision"`
		Deck         json.RawMessage `json:"deck"`
		LayoutResult json.RawMessage `json:"layoutResult"`
		Assets       []struct {
			AssetID   string `json:"assetId"`
			URL       string `json:"url"`
			MIMEType  string `json:"mimeType"`
			AltText   string `json:"altText"`
			ExpiresIn int64  `json:"expiresIn"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	var projectedLayout struct {
		Canvas struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"canvas"`
		Slides []struct {
			SlideID  string `json:"slideId"`
			Elements []struct {
				ElementID string  `json:"elementId"`
				X         float64 `json:"x"`
				Y         float64 `json:"y"`
				Width     float64 `json:"width"`
				Height    float64 `json:"height"`
				ZIndex    int     `json:"zIndex"`
			} `json:"elements"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(projection.LayoutResult, &projectedLayout); err != nil {
		t.Fatal(err)
	}
	if !jsonEquivalent(projection.Deck, completed.DeckGeneration.Compilation.Deck) || !jsonEquivalent(projection.LayoutResult, completed.DeckGeneration.Compilation.LayoutResult) {
		t.Fatal("preview projection changed persisted DeckRevision or LayoutResult")
	}
	if projection.DeckID != completed.Job.DeckID || projection.Revision != completed.Job.Revision || projectedLayout.Canvas.Width != 960 || projectedLayout.Canvas.Height != 540 || len(projectedLayout.Slides) != 8 {
		t.Fatalf("preview identity mismatch: %+v", projection)
	}
	first := projectedLayout.Slides[0].Elements[0]
	if first.X != 72 || first.Y != 64 || first.Width != 816 || first.Height != 54 || first.ZIndex != 3 {
		t.Fatalf("preview changed authoritative geometry: %+v", first)
	}
	if len(projection.Assets) != 1 || !strings.HasPrefix(projection.Assets[0].URL, "https://storage.example/download/") || projection.Assets[0].ExpiresIn <= 0 {
		t.Fatalf("preview asset projection mismatch: %+v", projection.Assets)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"fileId", "tenantId", "userId", "providerIdentity"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("preview leaked %s: %s", forbidden, body)
		}
	}

	replay := httptest.NewRecorder()
	a.previewPPTAgentDeck(replay, request.Clone(t.Context()))
	if replay.Code != http.StatusOK || assets.calls != 1 {
		t.Fatalf("asset URL refresh regenerated the deck: status=%d assetCalls=%d body=%s", replay.Code, assets.calls, replay.Body.String())
	}

	staleRequest := pptAgentHTTPRequest(t, token, http.MethodGet, "/api/v1/ppt/agent/jobs/"+completed.Job.ID+"/preview?revision=999", nil)
	staleRequest.SetPathValue("jobId", completed.Job.ID)
	staleResponse := httptest.NewRecorder()
	a.previewPPTAgentDeck(staleResponse, staleRequest)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale revision status=%d body=%s", staleResponse.Code, staleResponse.Body.String())
	}

	other, err := a.store.CreateAdminCustomer(adminCustomerMutation{Name: "Other preview owner", Email: "other-slice-c@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.sessions.Put(t.Context(), "other-slice-c-token", other.ID, authSessionTTL); err != nil {
		t.Fatal(err)
	}
	wrongOwnerRequest := pptAgentHTTPRequest(t, "other-slice-c-token", http.MethodGet, "/api/v1/ppt/agent/jobs/"+completed.Job.ID+"/preview?revision="+strconv.Itoa(completed.Job.Revision), nil)
	wrongOwnerRequest.SetPathValue("jobId", completed.Job.ID)
	wrongOwnerResponse := httptest.NewRecorder()
	a.previewPPTAgentDeck(wrongOwnerResponse, wrongOwnerRequest)
	if wrongOwnerResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-owner preview status=%d body=%s", wrongOwnerResponse.Code, wrongOwnerResponse.Body.String())
	}
	jsonStore, ok := a.store.(*jsonStore)
	if !ok {
		t.Fatal("preview fixture store is not JSON-backed")
	}
	if err := jsonStore.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.Users {
			if data.Users[index].ID == other.ID {
				data.Users[index].TenantID = "tenant_other_preview"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wrongTenantResponse := httptest.NewRecorder()
	a.previewPPTAgentDeck(wrongTenantResponse, wrongOwnerRequest.Clone(t.Context()))
	if wrongTenantResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant preview status=%d body=%s", wrongTenantResponse.Code, wrongTenantResponse.Body.String())
	}
}

func jsonEquivalent(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func TestPPTAgentSliceCPreviewRejectsProviderPrivatePayloads(t *testing.T) {
	for _, raw := range []string{
		`{"deckId":"deck_1","providerRawResponse":{"results":[]}}`,
		`{"deckId":"deck_1","provenance":{"sources":[{"providerIdentity":"private-provider-key"}]}}`,
		`{"deckId":"deck_1","nested":{"api_key":"secret"}}`,
	} {
		if err := rejectSensitivePreviewFields([]byte(raw)); err == nil {
			t.Fatalf("provider-private preview field was accepted: %s", raw)
		}
	}
	if err := rejectSensitivePreviewFields([]byte(`{"deckId":"deck_1","backgroundToken":"primary","assetManifest":[]}`)); err != nil {
		t.Fatalf("safe render tokens were rejected: %v", err)
	}
}
