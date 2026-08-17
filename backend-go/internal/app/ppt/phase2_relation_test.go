package ppt

import (
	"errors"
	"testing"
)

func TestAttachV2ArtifactIsIdempotentAndRejectsConflictingOverwrite(t *testing.T) {
	service := NewService()
	response, err := service.Generate(GenerateRequest{UserID: "owner", Prompt: "Phase 2 relation"})
	if err != nil {
		t.Fatal(err)
	}
	relation := V2ArtifactRelation{DeckID: "deck_phase2", Revision: 1, PPTXAssetID: "asset_phase2"}
	if _, err := service.AttachV2Artifact("owner", response.TaskID, relation); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AttachV2Artifact("owner", response.TaskID, relation); err != nil {
		t.Fatalf("idempotent relation replay failed: %v", err)
	}
	if _, err := service.AttachV2Artifact("owner", response.TaskID, V2ArtifactRelation{DeckID: "other", Revision: 1, PPTXAssetID: "other"}); !errors.Is(err, ErrV2ArtifactRelationConflict) {
		t.Fatalf("conflicting relation error = %v", err)
	}
	persisted, err := service.GetTask("owner", response.TaskID)
	if err != nil || persisted.V2DeckID != relation.DeckID || persisted.PPTXAssetID != relation.PPTXAssetID {
		t.Fatalf("conflict overwrote relation: task=%+v err=%v", persisted, err)
	}
}
