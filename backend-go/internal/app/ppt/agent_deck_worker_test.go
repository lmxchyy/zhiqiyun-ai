package ppt

import (
	"context"
	"errors"
	"testing"
	"time"
)

type deckContentFixture struct {
	calls         map[string]int
	failOnceSlide string
}

func (f *deckContentFixture) PlanSlideContent(_ context.Context, input SlideContentPlanningInput) (SlideContentPlanningOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[input.Objective.SlideID]++
	if input.Objective.SlideID == f.failOnceSlide && f.calls[input.Objective.SlideID] == 1 {
		return SlideContentPlanningOutput{}, errors.New("temporary content failure")
	}
	layout := "title-bullets"
	assets := []SlideAssetIntent{}
	if containsString(input.Objective.ExpectedElementTypes, "IMAGE") {
		layout = "text-image"
		assets = []SlideAssetIntent{{ID: "hero", Kind: "image", Prompt: "Professional market photograph", AltText: "Market evidence"}}
	}
	return SlideContentPlanningOutput{Draft: SlideContentDraft{
		Language: input.Intent.Language, Title: input.Objective.Title,
		BodyBlocks: []SlideBodyBlock{{Heading: "Finding", Text: input.Objective.KeyMessage}},
		Bullets:    []string{"Evidence", "Action"}, SupportingText: input.Objective.KeyMessage,
		SpeakerNotes: "Source context from the approved research pack.", AssetIntents: assets,
		CitationRefs: append([]string(nil), input.Objective.EvidenceRefs...), LayoutHint: layout,
	}, Provenance: PlanningProvenance{Mode: PlanningModeAI, Provider: "fixture", Model: "fixture"}}, nil
}

type deckAssetFixture struct {
	calls       map[string]int
	wrongTenant bool
}

func (f *deckAssetFixture) ResolveImage(_ context.Context, scope GenerationJobScope, _ string, slideID string, intent SlideAssetIntent) (ResolvedDeckAsset, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[intent.StableID]++
	if f.wrongTenant {
		scope.TenantID = "other_tenant"
	}
	return ResolvedDeckAsset{ID: "asset_" + shortStableID(intent.StableID), TenantID: scope.TenantID, UserID: scope.UserID, IntentID: intent.StableID, SlideID: slideID, MIMEType: "image/png", URI: "asset://ppt-v2/" + shortStableID(intent.StableID) + ".png", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", FileID: "file_" + shortStableID(intent.StableID), AltText: intent.AltText}, nil
}

type deckCompilerFixture struct {
	compileCalls, renderCalls int
	failQuality               bool
}

func (f *deckCompilerFixture) Compile(_ context.Context, input DeckBuildInput) (DeckCompilation, error) {
	f.compileCalls++
	compiled := DeckCompilation{DeckID: "deck_" + shortStableID(input.GenerationJobID), Revision: input.Revision, SlideCount: len(input.ApprovedOutline.Slides), Deck: []byte(`{"valid":true}`), LayoutResult: []byte(`{"valid":true}`), RenderInput: []byte(`{"valid":true}`), QualityValid: !f.failQuality}
	if f.failQuality {
		compiled.QualityIssues = []string{"TEXT_OVERFLOW: fixture"}
	}
	return compiled, nil
}
func (f *deckCompilerFixture) Render(_ context.Context, input DeckCompilation, _ []ResolvedDeckAsset) (DeckRenderOutput, error) {
	f.renderCalls++
	return DeckRenderOutput{DeckID: input.DeckID, Revision: input.Revision, SlideCount: input.SlideCount, PPTX: []byte("PK-fixture-pptx")}, nil
}

type deckArtifactFixture struct {
	taskCalls, fileCalls, assetCalls, relationCalls int
	store                                           *MemoryGenerationJobStore
	failFileOnce                                    bool
}

func (f *deckArtifactFixture) EnsureTask(_ context.Context, _ GenerationJobScope, jobID string, _ IntentSpec, _ OutlinePlan, _ []SlideContent) (string, error) {
	f.taskCalls++
	return "ppt_task_" + shortStableID(jobID), nil
}
func (f *deckArtifactFixture) StorePPTX(_ context.Context, _ GenerationJobScope, jobID, _ string, _ []byte) (string, error) {
	f.fileCalls++
	if f.failFileOnce && f.fileCalls == 1 {
		return "", errors.New("temporary private storage failure")
	}
	return "file_" + shortStableID(jobID), nil
}
func (f *deckArtifactFixture) EnsureArtifact(_ context.Context, lease GenerationLease, _, _, deckID string, _ int) (string, GenerationJob, error) {
	f.assetCalls++
	assetID := "asset_" + shortStableID(deckID)
	state, err := f.store.GetAgentPlanning(context.Background(), GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID)
	if err != nil {
		return "", GenerationJob{}, err
	}
	updated, err := f.store.SaveAgentDeckCheckpoint(context.Background(), lease, AgentDeckCheckpoint{ExpectedStage: GenerationStageFileStored, NextStage: GenerationStageAssetCreated, State: *state.DeckGeneration, CompletedWorkUnits: lease.Job.TotalWorkUnits - 1, AssetID: assetID, Now: lease.Job.UpdatedAt})
	return assetID, updated.Job, err
}
func (f *deckArtifactFixture) RelateTask(_ context.Context, lease GenerationLease, _ string, _ V2ArtifactRelation) (GenerationJob, error) {
	f.relationCalls++
	state, err := f.store.GetAgentPlanning(context.Background(), GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID)
	if err != nil {
		return GenerationJob{}, err
	}
	updated, err := f.store.SaveAgentDeckCheckpoint(context.Background(), lease, AgentDeckCheckpoint{ExpectedStage: GenerationStageAssetCreated, NextStage: GenerationStageTaskRelated, State: *state.DeckGeneration, CompletedWorkUnits: lease.Job.TotalWorkUnits, Now: lease.Job.UpdatedAt})
	return updated.Job, err
}

func TestAgentDeckWorkerGeneratesApprovedMultiPageDeckDurably(t *testing.T) {
	store, service, _, now := newAgentPlanningServiceFixture(t)
	content := &deckContentFixture{}
	assets := &deckAssetFixture{}
	compiler := &deckCompilerFixture{}
	artifacts := &deckArtifactFixture{store: store}
	if err := service.ConfigureDeckGeneration(content, assets, compiler, artifacts); err != nil {
		t.Fatal(err)
	}
	scope, jobID := approvedDeckJobFixture(t, service, now)
	if err := service.ProcessReady(t.Context(), now.Add(3*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	state, err := service.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Job.Status != GenerationJobSucceeded || state.Job.Stage != GenerationStageCompleted || state.Job.Progress() != 100 || state.Job.DeckID == "" || state.Job.FileID == "" || state.Job.AssetID == "" || state.Job.TaskID == "" {
		t.Fatalf("deck did not complete: %+v", state.Job)
	}
	if state.DeckGeneration == nil || len(state.DeckGeneration.Contents) != 8 || len(state.DeckGeneration.Assets) != 1 || state.DeckGeneration.ContentExecutions != 8 || state.DeckGeneration.AssetExecutions != 1 || state.DeckGeneration.LayoutExecutions != 1 || state.DeckGeneration.RenderExecutions != 1 {
		t.Fatalf("durable deck state mismatch: %+v", state.DeckGeneration)
	}
	if compiler.compileCalls != 1 || compiler.renderCalls != 1 || artifacts.taskCalls != 1 || artifacts.fileCalls != 1 || artifacts.assetCalls != 1 || artifacts.relationCalls != 1 {
		t.Fatalf("side effects repeated: compiler=%+v artifacts=%+v", compiler, artifacts)
	}
	bundle, err := store.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Deck.Status != GenerationChildSucceeded || len(bundle.Slides) != 8 {
		t.Fatalf("deck/slide jobs missing: %+v", bundle)
	}
}

func TestAgentDeckRetryResumesContentCheckpointWithoutRepeatingCompletedSlides(t *testing.T) {
	store, service, _, now := newAgentPlanningServiceFixture(t)
	content := &deckContentFixture{}
	assets := &deckAssetFixture{}
	compiler := &deckCompilerFixture{}
	artifacts := &deckArtifactFixture{store: store}
	if err := service.ConfigureDeckGeneration(content, assets, compiler, artifacts); err != nil {
		t.Fatal(err)
	}
	scope, jobID := approvedDeckJobFixture(t, service, now)
	state, _ := service.Get(t.Context(), scope, jobID)
	content.failOnceSlide = state.ApprovedOutline.Slides[1].SlideID
	if err := service.ProcessReady(t.Context(), now.Add(3*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	failed, _ := service.Get(t.Context(), scope, jobID)
	if failed.Job.Status != GenerationJobRetryWait || len(failed.DeckGeneration.Contents) != 1 {
		t.Fatalf("first content checkpoint missing: %+v", failed)
	}
	firstSlide := failed.ApprovedOutline.Slides[0].SlideID
	if _, err := service.Retry(t.Context(), scope, jobID, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(4*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	completed, _ := service.Get(t.Context(), scope, jobID)
	if completed.Job.Status != GenerationJobSucceeded || content.calls[firstSlide] != 1 || content.calls[content.failOnceSlide] != 2 {
		t.Fatalf("retry repeated checkpointed content: calls=%v state=%+v", content.calls, completed.Job)
	}
}

func TestAgentDeckRetryAfterRenderCheckpointDoesNotRenderAgain(t *testing.T) {
	store, service, _, now := newAgentPlanningServiceFixture(t)
	compiler := &deckCompilerFixture{}
	artifacts := &deckArtifactFixture{store: store, failFileOnce: true}
	if err := service.ConfigureDeckGeneration(&deckContentFixture{}, &deckAssetFixture{}, compiler, artifacts); err != nil {
		t.Fatal(err)
	}
	scope, jobID := approvedDeckJobFixture(t, service, now)
	if err := service.ProcessReady(t.Context(), now.Add(3*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := store.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Job.Status != GenerationJobRetryWait || failed.Job.Stage != GenerationStageRendered || compiler.renderCalls != 1 || len(bundle.Job.RenderBytes) == 0 {
		t.Fatalf("render checkpoint was not durable: state=%+v renderCalls=%d", failed.Job, compiler.renderCalls)
	}
	if _, err := service.Retry(t.Context(), scope, jobID, now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(4*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Job.Status != GenerationJobSucceeded || compiler.renderCalls != 1 || artifacts.fileCalls != 2 || artifacts.assetCalls != 1 || artifacts.relationCalls != 1 {
		t.Fatalf("retry repeated render or final artifact side effects: state=%+v compiler=%+v artifacts=%+v", completed.Job, compiler, artifacts)
	}
}

func TestAgentDeckQualityGateFailsClosedBeforeRenderAndArtifact(t *testing.T) {
	store, service, _, now := newAgentPlanningServiceFixture(t)
	compiler := &deckCompilerFixture{failQuality: true}
	artifacts := &deckArtifactFixture{store: store}
	if err := service.ConfigureDeckGeneration(&deckContentFixture{}, &deckAssetFixture{}, compiler, artifacts); err != nil {
		t.Fatal(err)
	}
	scope, jobID := approvedDeckJobFixture(t, service, now)
	if err := service.ProcessReady(t.Context(), now.Add(3*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Job.Status != GenerationJobFailed || failed.Job.Stage != GenerationStageLayoutCompiled || failed.Job.Error == nil || failed.Job.Error.Code != QualityGateFailed || compiler.renderCalls != 0 || artifacts.taskCalls != 0 {
		t.Fatalf("quality gate did not fail closed: state=%+v compiler=%+v artifacts=%+v", failed.Job, compiler, artifacts)
	}
}

func TestAgentDeckRejectsCrossTenantResolvedImage(t *testing.T) {
	store, service, _, now := newAgentPlanningServiceFixture(t)
	compiler := &deckCompilerFixture{}
	artifacts := &deckArtifactFixture{store: store}
	if err := service.ConfigureDeckGeneration(&deckContentFixture{}, &deckAssetFixture{wrongTenant: true}, compiler, artifacts); err != nil {
		t.Fatal(err)
	}
	scope, jobID := approvedDeckJobFixture(t, service, now)
	if err := service.ProcessReady(t.Context(), now.Add(3*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	failed, err := service.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Job.Status != GenerationJobRetryWait || failed.Job.Stage != GenerationStageContentReady || failed.Job.Error == nil || failed.Job.Error.Code != ImageInvalidResult || compiler.compileCalls != 0 || artifacts.taskCalls != 0 {
		t.Fatalf("cross-tenant image result was accepted: state=%+v compiler=%+v artifacts=%+v", failed.Job, compiler, artifacts)
	}
}

func TestAgentDeckCheckpointRejectsStaleWorkerFencingToken(t *testing.T) {
	store, service, _, now := newAgentPlanningServiceFixture(t)
	if err := service.ConfigureDeckGeneration(&deckContentFixture{}, &deckAssetFixture{}, &deckCompilerFixture{}, &deckArtifactFixture{store: store}); err != nil {
		t.Fatal(err)
	}
	scope, jobID := approvedDeckJobFixture(t, service, now)
	first, err := store.Claim(t.Context(), scope, jobID, "worker_old", now.Add(3*time.Minute), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Claim(t.Context(), scope, jobID, "worker_new", now.Add(3*time.Minute+2*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveAgentDeckCheckpoint(t.Context(), first, AgentDeckCheckpoint{ExpectedStage: GenerationStageOutlineApproved, NextStage: GenerationStageOutlineApproved, State: *state.DeckGeneration, CompletedWorkUnits: 3, Now: now.Add(3*time.Minute + 3*time.Second)})
	if !errors.Is(err, ErrGenerationJobLeaseLost) {
		t.Fatalf("stale worker was accepted: %v", err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("fencing token did not advance: old=%d new=%d", first.FencingToken, second.FencingToken)
	}
}

func approvedDeckJobFixture(t *testing.T, service *AgentPlanningService, now time.Time) (GenerationJobScope, string) {
	t.Helper()
	scope := GenerationJobScope{TenantID: "tenant_deck", UserID: "user_deck"}
	guided, err := service.Guide(t.Context(), GuideAgentRequest{TenantID: scope.TenantID, UserID: scope.UserID, OrganizationID: "org_deck", IdempotencyKey: "deck_guide", Request: IntentRequest{Text: "做一份8页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	planned, err := service.Get(t.Context(), scope, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	imageObjective := cloneSlideObjective(planned.Outline.Slides[2])
	imageObjective.ExpectedElementTypes = append(imageObjective.ExpectedElementTypes, "IMAGE")
	updated, err := service.UpdateOutline(t.Context(), scope, guided.State.Job.ID, planned.Outline.Revision, []OutlineEditCommand{{Type: OutlineCommandUpdateSlideObjective, SlideID: imageObjective.SlideID, Objective: &imageObjective}}, now.Add(90*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApproveOutline(t.Context(), scope, guided.State.Job.ID, updated.Outline.Revision, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return scope, guided.State.Job.ID
}
