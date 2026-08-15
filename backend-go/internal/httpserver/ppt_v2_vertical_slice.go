package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

const pptxMIMEType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"

type pptV2LegacyTaskContext struct {
	TaskID             string         `json:"taskId"`
	UserID             string         `json:"userId"`
	ClientRequestID    string         `json:"clientRequestId"`
	Status             string         `json:"status"`
	Title              string         `json:"title"`
	SpeakerNotesByPage map[int]string `json:"speakerNotesByPage"`
}

type pptV2LegacyInput struct {
	GenerateRequest pptapp.GenerateRequest `json:"generateRequest"`
	Outline         *pptapp.Outline        `json:"outline"`
	TaskContext     pptV2LegacyTaskContext `json:"taskContext"`
}

type pptV2RenderOutput struct {
	DeckID     string `json:"deckId"`
	Revision   int    `json:"revision"`
	SlideCount int    `json:"slideCount"`
	Bytes      int    `json:"bytes"`
	PPTX       []byte `json:"-"`
}

type pptV2Renderer interface {
	Render(context.Context, pptV2LegacyInput) (pptV2RenderOutput, error)
}

type nodePPTV2Renderer struct {
	nodeExecutable string
	cliPath        string
}

func newNodePPTV2Renderer(nodeExecutable string, cliPath string) pptV2Renderer {
	return nodePPTV2Renderer{nodeExecutable: strings.TrimSpace(nodeExecutable), cliPath: strings.TrimSpace(cliPath)}
}

func (r nodePPTV2Renderer) Render(ctx context.Context, input pptV2LegacyInput) (pptV2RenderOutput, error) {
	if r.nodeExecutable == "" || r.cliPath == "" {
		return pptV2RenderOutput{}, errors.New("ppt v2 node renderer is not configured")
	}
	request, err := json.Marshal(input)
	if err != nil {
		return pptV2RenderOutput{}, err
	}
	output, err := os.CreateTemp("", "ppt-v2-phase1-*.pptx")
	if err != nil {
		return pptV2RenderOutput{}, err
	}
	outputPath := output.Name()
	if err := output.Close(); err != nil {
		_ = os.Remove(outputPath)
		return pptV2RenderOutput{}, err
	}
	defer func() { _ = os.Remove(outputPath) }()

	command := exec.CommandContext(ctx, r.nodeExecutable, r.cliPath, outputPath)
	command.Stdin = bytes.NewReader(request)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return pptV2RenderOutput{}, fmt.Errorf("ppt v2 renderer failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var result pptV2RenderOutput
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return pptV2RenderOutput{}, fmt.Errorf("decode ppt v2 renderer result: %w", err)
	}
	result.PPTX, err = os.ReadFile(outputPath)
	if err != nil {
		return pptV2RenderOutput{}, err
	}
	if result.DeckID == "" || result.Revision <= 0 || result.SlideCount != 2 || result.Bytes != len(result.PPTX) || len(result.PPTX) < 2 || string(result.PPTX[:2]) != "PK" {
		return pptV2RenderOutput{}, errors.New("ppt v2 renderer returned an invalid artifact")
	}
	return result, nil
}

type pptV2VerticalSliceResult struct {
	DeckID   string
	Revision int
	PPTX     []byte
	File     storagecenter.FileObject
	Asset    asset
	Task     pptapp.Task
}

func legacyPPTV2Input(task pptapp.Task) pptV2LegacyInput {
	request := pptapp.GenerateRequest{
		Prompt: task.Prompt, SlideCount: task.SlideCount, Language: task.Language, Tone: task.Tone,
		TextContent: task.TextContent, Audience: task.Audience, Scenario: task.Scenario,
		GenerationAspectRatio: task.GenerationAspectRatio, Theme: task.Theme, AutoThemeEnabled: task.AutoThemeEnabled,
		EnableWebSearch: task.EnableWebSearch, ImageSource: task.ImageSource, TextModel: task.TextModel,
		ImageModel: task.ImageModel, ImageStyle: task.ImageStyle, PeopleStyle: task.PeopleStyle,
		ImageLighting: task.ImageLighting, ImageComposition: task.ImageComposition, TextInImage: task.TextInImage,
	}
	outline := task.Outline
	if outline == nil {
		outline = &pptapp.Outline{Title: firstNonEmptyString(task.Title, task.Prompt)}
		for _, slide := range task.Slides {
			outline.Slides = append(outline.Slides, pptapp.OutlineSlide{
				Page: slide.Page, Title: slide.Title, Summary: slide.Content,
				BulletPoints: append([]string(nil), slide.BulletPoints...), Layout: slide.Layout, SlideType: slide.SlideType,
			})
		}
	}
	notes := map[int]string{}
	for _, slide := range task.Slides {
		if value := strings.TrimSpace(slide.SpeakerNotes); value != "" {
			notes[slide.Page] = value
		}
	}
	return pptV2LegacyInput{
		GenerateRequest: request,
		Outline:         outline,
		TaskContext: pptV2LegacyTaskContext{
			TaskID: task.TaskID, UserID: task.UserID, ClientRequestID: task.ClientRequestID,
			Status: task.Status, Title: task.Title, SpeakerNotesByPage: notes,
		},
	}
}

// generatePPTV2VerticalSlice is an internal Phase 1 application-service entry.
// It intentionally has no HTTP route and creates no billing or Connector work.
func (a api) generatePPTV2VerticalSlice(ctx context.Context, user adminUser, taskID string, renderer pptV2Renderer) (pptV2VerticalSliceResult, error) {
	if a.pptService == nil || renderer == nil {
		return pptV2VerticalSliceResult{}, errors.New("ppt v2 vertical slice is unavailable")
	}
	task, err := a.pptService.GetTask(strings.TrimSpace(user.ID), strings.TrimSpace(taskID))
	if err != nil {
		return pptV2VerticalSliceResult{}, err
	}
	rendered, err := renderer.Render(ctx, legacyPPTV2Input(task))
	if err != nil {
		return pptV2VerticalSliceResult{}, err
	}
	if a.fileService == nil {
		return pptV2VerticalSliceResult{}, errors.New("private file storage is unavailable")
	}
	tenantID := effectiveTenantID(user)
	available, err := a.fileService.StorageAvailable(ctx, tenantID)
	if err != nil {
		return pptV2VerticalSliceResult{}, err
	}
	if !available {
		return pptV2VerticalSliceResult{}, errors.New("private file storage is not configured")
	}
	file, err := a.fileService.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: tenantID, UserID: user.ID, FileName: pptxDownloadFileName(task), FileSize: int64(len(rendered.PPTX)),
		MIMEType: pptxMIMEType, BusinessType: "generation_result", BusinessID: task.TaskID, Visibility: "PRIVATE",
	}, bytes.NewReader(rendered.PPTX))
	if err != nil {
		return pptV2VerticalSliceResult{}, fmt.Errorf("store ppt v2 artifact: %w", err)
	}
	assetStore, ok := a.store.(pptV2ArtifactAssetStore)
	if !ok {
		a.cleanupGeneratedFiles([]storagecenter.FileObject{file})
		return pptV2VerticalSliceResult{}, errors.New("ppt v2 work-center asset store is unavailable")
	}
	createdAsset, err := assetStore.CreatePPTV2ArtifactAsset(pptV2ArtifactAssetInput{
		UserID: user.ID, TenantID: tenantID, OrganizationID: user.OrganizationID, TaskID: task.TaskID,
		Title: task.Title, DeckID: rendered.DeckID, Revision: rendered.Revision, File: file,
	})
	if err != nil {
		a.cleanupGeneratedFiles([]storagecenter.FileObject{file})
		return pptV2VerticalSliceResult{}, fmt.Errorf("create ppt v2 work-center asset: %w", err)
	}
	updated, err := a.pptService.AttachV2Artifact(user.ID, task.TaskID, pptapp.V2ArtifactRelation{
		DeckID: rendered.DeckID, Revision: rendered.Revision, PPTXAssetID: createdAsset.ID,
	})
	if err != nil {
		_ = a.store.DeleteAssetForUser(user.ID, createdAsset.ID)
		a.cleanupGeneratedFiles([]storagecenter.FileObject{file})
		return pptV2VerticalSliceResult{}, fmt.Errorf("relate ppt v2 artifact: %w", err)
	}
	return pptV2VerticalSliceResult{
		DeckID: rendered.DeckID, Revision: rendered.Revision, PPTX: rendered.PPTX,
		File: file, Asset: createdAsset, Task: updated,
	}, nil
}
