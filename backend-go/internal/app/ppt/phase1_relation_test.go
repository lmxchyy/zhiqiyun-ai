package ppt

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAttachV2ArtifactPersistsOnlyCanonicalRelationFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppt-tasks.json")
	service := NewPersistentService(path)
	response, err := service.Generate(GenerateRequest{
		UserID: "user_phase1", Prompt: "Phase 1", SlideCount: 2,
		Outline: &Outline{Title: "Phase 1", Slides: []OutlineSlide{
			{Page: 1, Title: "Phase 1", Summary: "Cover", Layout: "cover", SlideType: "cover"},
			{Page: 2, Title: "Contract", Summary: "Vertical slice", BulletPoints: []string{"SlideIR", "LayoutResult"}, Layout: "content"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := service.AttachV2Artifact("user_phase1", response.TaskID, V2ArtifactRelation{
		DeckID: "deck_phase1", Revision: 1, PPTXAssetID: "asset_phase1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.V2DeckID != "deck_phase1" || updated.V2Revision != 1 || updated.PPTXAssetID != "asset_phase1" {
		t.Fatalf("unexpected V2 relation: %+v", updated)
	}
	if len(updated.Slides) != 2 || updated.Slides[0].Title != "Phase 1" {
		t.Fatalf("legacy slides were rewritten: %+v", updated.Slides)
	}

	reopened := NewPersistentService(path)
	persisted, err := reopened.GetTask("user_phase1", response.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.V2DeckID != "deck_phase1" || persisted.V2Revision != 1 || persisted.PPTXAssetID != "asset_phase1" {
		t.Fatalf("V2 relation did not persist: %+v", persisted)
	}
}

func TestAttachV2ArtifactRejectsWrongOwnerAndIncompleteRelation(t *testing.T) {
	service := NewService()
	response, err := service.Generate(GenerateRequest{UserID: "owner", Prompt: "Phase 1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AttachV2Artifact("other", response.TaskID, V2ArtifactRelation{
		DeckID: "deck_phase1", Revision: 1, PPTXAssetID: "asset_phase1",
	}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("wrong owner error = %v", err)
	}
	if _, err := service.AttachV2Artifact("owner", response.TaskID, V2ArtifactRelation{}); !errors.Is(err, ErrInvalidV2ArtifactRelation) {
		t.Fatalf("incomplete relation error = %v", err)
	}
}
