package httpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptV2AgentArtifacts struct {
	ppt       *pptapp.Service
	files     *storagecenter.Service
	assets    pptV2DurableArtifactStore
	jobs      pptapp.GenerationJobStore
	relations pptapp.GenerationTaskRelationStore
}

func (a pptV2AgentArtifacts) EnsureTask(ctx context.Context, scope pptapp.GenerationJobScope, jobID string, intent pptapp.IntentSpec, outline pptapp.OutlinePlan, contents []pptapp.SlideContent) (string, error) {
	if a.ppt == nil || a.jobs == nil {
		return "", errors.New("PPT task service is unavailable")
	}
	bundle, err := a.jobs.Get(ctx, scope, jobID)
	if err != nil {
		return "", err
	}
	contentBySlide := make(map[string]pptapp.SlideContent, len(contents))
	for _, content := range contents {
		contentBySlide[content.SlideID] = content
	}
	legacyOutline := &pptapp.Outline{Title: intent.Topic}
	for index, objective := range outline.Slides {
		content, ok := contentBySlide[objective.SlideID]
		if !ok {
			return "", pptapp.ErrInvalidSlideContent
		}
		legacyOutline.Slides = append(legacyOutline.Slides, pptapp.OutlineSlide{Page: index + 1, Title: content.Title, Summary: content.SupportingText, BulletPoints: append([]string(nil), content.Bullets...), Layout: content.LayoutHint, SlideType: "professional"})
	}
	result, err := a.ppt.Generate(pptapp.GenerateRequest{UserID: scope.UserID, TenantID: scope.TenantID, OrganizationID: bundle.Job.OrganizationID, ClientRequestID: "ppt-v2-agent:" + jobID, Prompt: intent.Topic, SlideCount: len(outline.Slides), Language: intent.Language, Tone: intent.ProfessionalStyle, Audience: intent.Audience, Scenario: intent.Scenario, GenerationAspectRatio: "16:9", Theme: "professional", ImageSource: "ai", Outline: legacyOutline})
	if err != nil {
		return "", err
	}
	return result.TaskID, nil
}

func (a pptV2AgentArtifacts) StorePPTX(ctx context.Context, scope pptapp.GenerationJobScope, jobID, title string, data []byte) (string, error) {
	if a.files == nil {
		return "", errors.New("private file storage is unavailable")
	}
	if existing, found, err := findPPTV2AgentFile(ctx, a.files, scope, jobID); err != nil {
		return "", err
	} else if found {
		return existing.FileID, nil
	}
	available, err := a.files.StorageAvailable(ctx, scope.TenantID)
	if err != nil {
		return "", err
	}
	if !available {
		return "", errors.New("private file storage is not configured")
	}
	fileName := strings.TrimSpace(title)
	if fileName == "" {
		fileName = "presentation"
	}
	fileName += ".pptx"
	stored, err := a.files.StoreObject(ctx, storagecenter.UploadInitInput{TenantID: scope.TenantID, UserID: scope.UserID, FileName: fileName, FileSize: int64(len(data)), MIMEType: pptxMIMEType, BusinessType: "ppt_v2_generation", BusinessID: jobID, Visibility: "PRIVATE"}, bytes.NewReader(data))
	if err != nil {
		if existing, found, findErr := findPPTV2AgentFile(ctx, a.files, scope, jobID); findErr == nil && found {
			return existing.FileID, nil
		}
		return "", err
	}
	return stored.FileID, nil
}

func findPPTV2AgentFile(ctx context.Context, files *storagecenter.Service, scope pptapp.GenerationJobScope, businessID string) (storagecenter.FileObject, bool, error) {
	for offset := 0; ; offset += 200 {
		items, total, err := files.ListFiles(ctx, storagecenter.FileFilter{TenantID: scope.TenantID, UserID: scope.UserID, BusinessType: "ppt_v2_generation", Status: storagecenter.StatusActive, Limit: 200, Offset: offset})
		if err != nil {
			return storagecenter.FileObject{}, false, err
		}
		for _, item := range items {
			if item.BusinessID == businessID {
				return item, true, nil
			}
		}
		if int64(offset+len(items)) >= total || len(items) == 0 {
			return storagecenter.FileObject{}, false, nil
		}
	}
}

func (a pptV2AgentArtifacts) EnsureArtifact(ctx context.Context, lease pptapp.GenerationLease, taskID, fileID, deckID string, revision int) (string, pptapp.GenerationJob, error) {
	if a.files == nil || a.assets == nil || a.jobs == nil {
		return "", pptapp.GenerationJob{}, errors.New("artifact services are unavailable")
	}
	file, err := a.files.GetFile(ctx, storagecenter.AccessContext{TenantID: lease.TenantID, UserID: lease.UserID}, fileID)
	if err != nil {
		return "", pptapp.GenerationJob{}, err
	}
	input := pptV2DurableArtifactInput{GenerationJobID: lease.JobID, UserID: lease.UserID, TenantID: lease.TenantID, OrganizationID: lease.Job.OrganizationID, TaskID: taskID, Title: lease.Job.DeckID, DeckID: deckID, Revision: revision, File: file}
	fenced, ok := a.assets.(pptV2FencedDurableArtifactStore)
	if !ok {
		return "", pptapp.GenerationJob{}, errors.New("fenced artifact storage is required")
	}
	created, _, err := fenced.EnsurePPTV2DurableArtifactFenced(ctx, input, lease)
	if err != nil {
		return "", pptapp.GenerationJob{}, err
	}
	bundle, err := a.jobs.Get(ctx, pptapp.GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID)
	if err != nil {
		return "", pptapp.GenerationJob{}, err
	}
	return created.ID, bundle.Job, nil
}

func (a pptV2AgentArtifacts) RelateTask(ctx context.Context, lease pptapp.GenerationLease, taskID string, relation pptapp.V2ArtifactRelation) (pptapp.GenerationJob, error) {
	if a.relations == nil {
		return pptapp.GenerationJob{}, errors.New("atomic task relation store is unavailable")
	}
	if taskID != lease.Job.ExistingTaskID {
		return pptapp.GenerationJob{}, fmt.Errorf("task relation scope mismatch")
	}
	return a.relations.RelateTaskArtifact(ctx, lease, relation, normalizedHTTPTime())
}

func normalizedHTTPTime() time.Time { return time.Now().UTC() }

var _ pptapp.AgentDeckArtifactPort = pptV2AgentArtifacts{}
