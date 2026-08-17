package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptAgentPreviewAsset struct {
	AssetID   string `json:"assetId"`
	URL       string `json:"url"`
	ExpiresIn int64  `json:"expiresIn"`
	MIMEType  string `json:"mimeType"`
	AltText   string `json:"altText"`
}

type pptAgentPreviewProjection struct {
	DeckID       string                 `json:"deckId"`
	Revision     int                    `json:"revision"`
	Deck         json.RawMessage        `json:"deck"`
	LayoutResult json.RawMessage        `json:"layoutResult"`
	Assets       []pptAgentPreviewAsset `json:"assets"`
}

type previewDeckDocument struct {
	DeckID        string `json:"deckId"`
	Revision      int    `json:"revision"`
	AssetManifest []struct {
		ID       string `json:"id"`
		MIMEType string `json:"mimeType"`
		URI      string `json:"uri"`
		SHA256   string `json:"sha256"`
	} `json:"assetManifest"`
	Provenance struct {
		Sources []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Type    string `json:"type"`
			Locator string `json:"locator"`
		} `json:"sources"`
		Citations []struct {
			ID       string `json:"id"`
			SourceID string `json:"sourceId"`
			Locator  string `json:"locator"`
		} `json:"citations"`
		Claims []struct {
			ID                 string   `json:"id"`
			SourceID           string   `json:"sourceId"`
			CitationRefs       []string `json:"citationRefs"`
			Text               string   `json:"text"`
			VerificationStatus string   `json:"verificationStatus"`
		} `json:"claims"`
	} `json:"provenance"`
	Slides []struct {
		ID               string   `json:"id"`
		Sequence         int      `json:"sequence"`
		LayoutID         string   `json:"layoutId"`
		EvidenceRequired bool     `json:"evidenceRequired"`
		CitationRefs     []string `json:"citationRefs"`
		Elements         []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			AssetRef string `json:"assetRef"`
		} `json:"elements"`
	} `json:"slides"`
}

type previewLayoutDocument struct {
	DeckID   string `json:"deckId"`
	Revision int    `json:"revision"`
	Canvas   struct {
		Unit   string  `json:"unit"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"canvas"`
	Slides []struct {
		SlideID  string `json:"slideId"`
		LayoutID string `json:"layoutId"`
		Elements []struct {
			ElementID     string          `json:"elementId"`
			X             float64         `json:"x"`
			Y             float64         `json:"y"`
			Width         float64         `json:"width"`
			Height        float64         `json:"height"`
			ZIndex        int             `json:"zIndex"`
			ResolvedStyle json.RawMessage `json:"resolvedStyle"`
		} `json:"elements"`
	} `json:"slides"`
}

func (a api) previewPPTAgentDeck(w http.ResponseWriter, r *http.Request) {
	user, ok := a.pptAgentUser(w, r)
	if !ok {
		return
	}
	revision, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("revision")))
	if err != nil || revision <= 0 {
		writePPTAgentError(w, pptapp.ErrGenerationJobInvalid)
		return
	}
	scope := pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID}
	state, err := a.pptAgentService.Get(r.Context(), scope, r.PathValue("jobId"))
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	if state.Job.Status != pptapp.GenerationJobSucceeded || state.Job.Stage != pptapp.GenerationStageCompleted || state.DeckGeneration == nil || state.DeckGeneration.Compilation == nil {
		writePPTAgentError(w, pptapp.ErrGenerationJobNotReady)
		return
	}
	compilation := state.DeckGeneration.Compilation
	if revision != state.Job.Revision || revision != compilation.Revision {
		writePPTAgentError(w, pptapp.ErrStaleOutlineRevision)
		return
	}
	if a.fileService == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("private file storage is unavailable"))
		return
	}
	deck, _, referencedAssets, err := validatePPTAgentPreview(state)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	assetProjection := make([]pptAgentPreviewAsset, 0, len(referencedAssets))
	for _, asset := range referencedAssets {
		ticket, accessErr := a.fileService.AccessURL(r.Context(), storagecenter.AccessContext{TenantID: scope.TenantID, UserID: scope.UserID}, asset.FileID, false)
		if accessErr != nil {
			writeError(w, http.StatusBadGateway, accessErr)
			return
		}
		assetProjection = append(assetProjection, pptAgentPreviewAsset{
			AssetID: asset.ID, URL: ticket.URL, ExpiresIn: ticket.ExpiresIn, MIMEType: asset.MIMEType, AltText: asset.AltText,
		})
	}
	writeJSON(w, pptAgentPreviewProjection{
		DeckID: deck.DeckID, Revision: deck.Revision,
		Deck:         json.RawMessage(append([]byte(nil), compilation.Deck...)),
		LayoutResult: json.RawMessage(append([]byte(nil), compilation.LayoutResult...)),
		Assets:       assetProjection,
	})
}

func validatePPTAgentPreview(state pptapp.AgentPlanningState) (previewDeckDocument, previewLayoutDocument, []pptapp.ResolvedDeckAsset, error) {
	if state.DeckGeneration == nil || state.DeckGeneration.Compilation == nil {
		return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview compilation is unavailable")
	}
	compilation := state.DeckGeneration.Compilation
	if !compilation.QualityValid || strings.TrimSpace(compilation.DeckID) == "" || compilation.Revision <= 0 || compilation.SlideCount < 6 || compilation.SlideCount > 12 || state.Job.DeckID != compilation.DeckID || state.Job.Revision != compilation.Revision || state.ApprovedOutline == nil || state.ApprovedOutline.Revision != compilation.Revision || len(state.ApprovedOutline.Slides) != compilation.SlideCount {
		return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview compilation identity is invalid")
	}
	if err := rejectSensitivePreviewFields(compilation.Deck); err != nil {
		return previewDeckDocument{}, previewLayoutDocument{}, nil, err
	}
	var deck previewDeckDocument
	if err := json.Unmarshal(compilation.Deck, &deck); err != nil {
		return previewDeckDocument{}, previewLayoutDocument{}, nil, fmt.Errorf("decode PPT V2 preview deck: %w", err)
	}
	var layout previewLayoutDocument
	if err := json.Unmarshal(compilation.LayoutResult, &layout); err != nil {
		return previewDeckDocument{}, previewLayoutDocument{}, nil, fmt.Errorf("decode PPT V2 preview layout: %w", err)
	}
	if deck.DeckID != compilation.DeckID || deck.Revision != compilation.Revision || layout.DeckID != deck.DeckID || layout.Revision != deck.Revision || layout.Canvas.Unit != "pt" || layout.Canvas.Width <= 0 || layout.Canvas.Height <= 0 || len(deck.Slides) != compilation.SlideCount || len(layout.Slides) != compilation.SlideCount {
		return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview deck and layout identity do not match")
	}

	sourceIDs := make(map[string]struct{}, len(deck.Provenance.Sources))
	for _, source := range deck.Provenance.Sources {
		if source.ID == "" || source.Title == "" || source.Type == "" || source.Locator == "" {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview source provenance is invalid")
		}
		if _, exists := sourceIDs[source.ID]; exists {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview source identity is duplicated")
		}
		sourceIDs[source.ID] = struct{}{}
	}
	citations := make(map[string]string, len(deck.Provenance.Citations))
	for _, citation := range deck.Provenance.Citations {
		if citation.ID == "" || citation.Locator == "" {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview citation provenance is invalid")
		}
		if _, ok := sourceIDs[citation.SourceID]; !ok {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview citation source is invalid")
		}
		if _, exists := citations[citation.ID]; exists {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview citation identity is duplicated")
		}
		citations[citation.ID] = citation.SourceID
	}
	claimIDs := make(map[string]struct{}, len(deck.Provenance.Claims))
	for _, claim := range deck.Provenance.Claims {
		if claim.ID == "" || claim.Text == "" || claim.VerificationStatus == "" || len(claim.CitationRefs) == 0 {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview claim provenance is invalid")
		}
		if _, ok := sourceIDs[claim.SourceID]; !ok {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview claim source is invalid")
		}
		for _, citationID := range claim.CitationRefs {
			if citations[citationID] != claim.SourceID {
				return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview claim citation chain is invalid")
			}
		}
		if _, exists := claimIDs[claim.ID]; exists {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview claim identity is duplicated")
		}
		claimIDs[claim.ID] = struct{}{}
	}

	manifest := make(map[string]struct {
		MIMEType, URI, SHA256 string
	}, len(deck.AssetManifest))
	manifestOrder := make([]string, 0, len(deck.AssetManifest))
	for _, asset := range deck.AssetManifest {
		if asset.ID == "" || !strings.HasPrefix(asset.MIMEType, "image/") || !strings.HasPrefix(asset.URI, "asset://ppt-v2/") || asset.SHA256 == "" {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview asset manifest is invalid")
		}
		if _, exists := manifest[asset.ID]; exists {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview asset identity is duplicated")
		}
		manifest[asset.ID] = struct{ MIMEType, URI, SHA256 string }{asset.MIMEType, asset.URI, asset.SHA256}
		manifestOrder = append(manifestOrder, asset.ID)
	}
	referencedAssetIDs := make(map[string]struct{})
	for index, slide := range deck.Slides {
		if slide.ID == "" || slide.ID != state.ApprovedOutline.Slides[index].SlideID || slide.Sequence != index+1 || slide.LayoutID == "" || layout.Slides[index].SlideID != slide.ID || layout.Slides[index].LayoutID != slide.LayoutID {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview slide order or layout identity is invalid")
		}
		if slide.EvidenceRequired && len(slide.CitationRefs) == 0 {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview factual slide has no evidence")
		}
		for _, claimID := range slide.CitationRefs {
			if _, ok := claimIDs[claimID]; !ok {
				return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview slide references an unknown claim")
			}
		}
		deckElements := make(map[string]string, len(slide.Elements))
		for _, element := range slide.Elements {
			if element.ID == "" || (element.Type != "text" && element.Type != "shape" && element.Type != "image") {
				return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview element is invalid")
			}
			if _, exists := deckElements[element.ID]; exists {
				return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview element identity is duplicated")
			}
			deckElements[element.ID] = element.Type
			if element.Type == "image" {
				if _, ok := manifest[element.AssetRef]; !ok {
					return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview image asset reference is invalid")
				}
				referencedAssetIDs[element.AssetRef] = struct{}{}
			}
		}
		if len(layout.Slides[index].Elements) != len(deckElements) {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview layout element count does not match SlideIR")
		}
		layoutElements := make(map[string]struct{}, len(layout.Slides[index].Elements))
		for _, element := range layout.Slides[index].Elements {
			if _, ok := deckElements[element.ElementID]; !ok || element.Width <= 0 || element.Height <= 0 || element.X < 0 || element.Y < 0 || element.X+element.Width > layout.Canvas.Width || element.Y+element.Height > layout.Canvas.Height || element.ZIndex < 0 || len(element.ResolvedStyle) == 0 {
				return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview layout geometry is invalid")
			}
			if _, exists := layoutElements[element.ElementID]; exists {
				return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview layout element identity is duplicated")
			}
			layoutElements[element.ElementID] = struct{}{}
		}
	}

	resolvedByID := make(map[string]pptapp.ResolvedDeckAsset, len(state.DeckGeneration.Assets))
	for _, asset := range state.DeckGeneration.Assets {
		resolvedByID[asset.ID] = asset
	}
	resolved := make([]pptapp.ResolvedDeckAsset, 0, len(referencedAssetIDs))
	for _, assetID := range manifestOrder {
		if _, used := referencedAssetIDs[assetID]; !used {
			continue
		}
		asset, ok := resolvedByID[assetID]
		metadata := manifest[assetID]
		if !ok || asset.FileID == "" || asset.TenantID != state.Job.TenantID || asset.UserID != state.Job.UserID || asset.MIMEType != metadata.MIMEType || asset.URI != metadata.URI || asset.SHA256 != metadata.SHA256 {
			return previewDeckDocument{}, previewLayoutDocument{}, nil, errors.New("PPT V2 preview private asset checkpoint is invalid")
		}
		resolved = append(resolved, asset)
	}
	return deck, layout, resolved, nil
}

func rejectSensitivePreviewFields(raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode PPT V2 preview safety projection: %w", err)
	}
	forbidden := map[string]struct{}{
		"apikey": {}, "secret": {}, "accesstoken": {}, "refreshtoken": {}, "credentials": {},
		"provideridentity": {}, "providerrequestid": {}, "providerraw": {}, "providerrawresponse": {}, "rawresponse": {},
	}
	var inspect func(any) bool
	inspect = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
				if _, blocked := forbidden[normalized]; blocked || inspect(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if inspect(child) {
					return true
				}
			}
		}
		return false
	}
	if inspect(value) {
		return errors.New("PPT V2 preview deck contains provider-private fields")
	}
	return nil
}
