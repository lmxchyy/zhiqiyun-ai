package ppt

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEditCommandValidationRejectsStaleRevisionAndUnknownTarget(t *testing.T) {
	state := AgentDeckGenerationState{Compilation: &DeckCompilation{
		DeckID: "deck-1", Revision: 4, SlideCount: 6,
		Deck: []byte(`{"slides":[{"id":"slide-1","elements":[{"id":"title-1","type":"text"}]}]}`),
	}}
	stale := EditCommand{CommandID: "cmd-1", Type: EditCommandUpdateText, DeckID: "deck-1", BaseRevision: 3, TargetSlideID: "slide-1", TargetElementID: "title-1", Payload: map[string]string{"text": "Updated"}}
	if !errors.Is(ValidateEditCommand(stale, state), ErrEditStaleRevision) {
		t.Fatalf("expected stale revision, got %v", ValidateEditCommand(stale, state))
	}
	unknown := stale
	unknown.BaseRevision = 4
	unknown.TargetElementID = "missing"
	if !errors.Is(ValidateEditCommand(unknown, state), ErrEditTargetNotFound) {
		t.Fatalf("expected missing target, got %v", ValidateEditCommand(unknown, state))
	}
}

func TestApplyEditCommandCreatesImmutableRevisionAndAffectedSlideSet(t *testing.T) {
	state := AgentDeckGenerationState{Compilation: &DeckCompilation{
		DeckID: "deck-1", Revision: 4, SlideCount: 6,
		Deck:         []byte(`{"slides":[{"id":"slide-1","elements":[{"id":"title-1","type":"text","content":{"kind":"plain","text":"Before"}}]}]}`),
		LayoutResult: []byte(`{"slides":[{"slideId":"slide-1","elements":[]}]}`),
	}}
	command := EditCommand{CommandID: "cmd-1", Type: EditCommandUpdateText, DeckID: "deck-1", BaseRevision: 4, TargetSlideID: "slide-1", TargetElementID: "title-1", Payload: map[string]string{"text": "After"}, UserIntentSummary: "精简第 1 页"}
	updated, err := ApplyEditCommand(state, command, time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentRevision != 5 || len(updated.Revisions) != 2 || updated.Revisions[0].Revision != 4 || updated.Revisions[1].ParentRevision != 4 || len(updated.Revisions[1].AffectedSlideIDs) != 1 || updated.Revisions[1].AffectedSlideIDs[0] != "slide-1" {
		t.Fatalf("unexpected revision state: %+v", updated)
	}
	if string(updated.Revisions[0].Compilation.Deck) != `{"slides":[{"id":"slide-1","elements":[{"id":"title-1","type":"text","content":{"kind":"plain","text":"Before"}}]}]}` {
		t.Fatalf("parent revision mutated: %s", updated.Revisions[0].Compilation.Deck)
	}
	var document map[string]any
	if err := json.Unmarshal(updated.Compilation.Deck, &document); err != nil {
		t.Fatal(err)
	}
	text := document["slides"].([]any)[0].(map[string]any)["elements"].([]any)[0].(map[string]any)["content"].(map[string]any)["text"]
	if text != "After" {
		t.Fatalf("updated element text=%v", text)
	}
}

func TestUndoEditRestoresParentRevisionWithoutNewModelCall(t *testing.T) {
	state := AgentDeckGenerationState{CurrentRevision: 5, Revisions: []DeckRevisionSnapshot{
		{Revision: 4, Compilation: DeckCompilation{DeckID: "deck-1", Revision: 4, Deck: []byte(`{"slides":[]`)}, CreatedAt: time.Now().UTC()},
		{Revision: 5, ParentRevision: 4, Compilation: DeckCompilation{DeckID: "deck-1", Revision: 5, Deck: []byte(`{"slides":[{"id":"added"}]}`)}, CreatedAt: time.Now().UTC()},
	}, Compilation: &DeckCompilation{DeckID: "deck-1", Revision: 5, Deck: []byte(`{"slides":[{"id":"added"}]}`)}}
	updated, err := UndoLastEdit(state)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentRevision != 4 || updated.Compilation == nil || updated.Compilation.Revision != 4 || len(updated.Revisions) != 2 {
		t.Fatalf("unexpected undo state: %+v", updated)
	}
}

func TestDuplicateEditCommandIsIdempotentReplay(t *testing.T) {
	state := AgentDeckGenerationState{Compilation: &DeckCompilation{DeckID: "deck-1", Revision: 4, SlideCount: 6, Deck: []byte(`{"slides":[{"id":"slide-1","elements":[{"id":"title-1","type":"text","content":{"kind":"plain","text":"Before"}}]}]}`)}, CurrentRevision: 4}
	command := EditCommand{CommandID: "cmd-replay", Type: EditCommandUpdateText, DeckID: "deck-1", BaseRevision: 4, TargetSlideID: "slide-1", TargetElementID: "title-1", Payload: map[string]string{"text": "After"}}
	first, err := ApplyEditCommand(state, command, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := ApplyEditCommand(first, command, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if replay.CurrentRevision != first.CurrentRevision || len(replay.Revisions) != len(first.Revisions) || string(replay.Compilation.Deck) != string(first.Compilation.Deck) {
		t.Fatalf("replay mutated edit state: first=%+v replay=%+v", first, replay)
	}
}

func TestMoveAndDeleteEditCommandsPreserveStableSlideIdentity(t *testing.T) {
	slides := []string{}
	for i := 1; i <= 7; i++ {
		slides = append(slides, `{"id":"slide-`+string(rune('0'+i))+`","sequence":`+string(rune('0'+i))+`,"elements":[]}`)
	}
	deck := `{"slides":[` + strings.Join(slides, ",") + `]}`
	state := AgentDeckGenerationState{CurrentRevision: 4, Compilation: &DeckCompilation{DeckID: "deck-1", Revision: 4, SlideCount: 7, Deck: []byte(deck), LayoutResult: []byte(`{"slides":[]}`)}}
	moved, err := ApplyEditCommand(state, EditCommand{CommandID: "move", Type: EditCommandMoveSlide, DeckID: "deck-1", BaseRevision: 4, TargetSlideID: "slide-7", Payload: map[string]string{"toIndex": "1"}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if moved.Compilation.SlideCount != 7 {
		t.Fatalf("move changed page count")
	}
	deleted, err := ApplyEditCommand(moved, EditCommand{CommandID: "delete", Type: EditCommandDeleteSlide, DeckID: "deck-1", BaseRevision: 5, TargetSlideID: "slide-2"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Compilation.SlideCount != 6 {
		t.Fatalf("delete page count=%d", deleted.Compilation.SlideCount)
	}
}
