package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptV2DurableGenerationOptions struct {
	IdempotencyKey  string
	ClientRequestID string
	WorkerID        string
	MaxAttempts     int
	LeaseDuration   time.Duration
	RetryDelay      time.Duration
}

// configuredPPTV2GenerationJobStore deliberately has no JSON/in-memory
// fallback: Phase 2 durable execution requires PostgreSQL.
func (a api) configuredPPTV2GenerationJobStore() (pptapp.GenerationJobStore, error) {
	postgres, ok := a.store.(*postgresStore)
	if !ok || postgres == nil || postgres.db == nil {
		return nil, errors.New("ppt v2 durable generation requires postgres")
	}
	return pptapp.NewPostgresGenerationJobStore(postgres.db)
}

func (a api) runConfiguredPPTV2DurableGeneration(ctx context.Context, user adminUser, taskID string, renderer pptV2Renderer, options pptV2DurableGenerationOptions) (pptV2VerticalSliceResult, pptapp.GenerationJob, error) {
	jobs, err := a.configuredPPTV2GenerationJobStore()
	if err != nil {
		return pptV2VerticalSliceResult{}, pptapp.GenerationJob{}, err
	}
	return a.runPPTV2DurableGeneration(ctx, user, taskID, jobs, renderer, options)
}

func normalizePPTV2DurableOptions(options pptV2DurableGenerationOptions) (pptV2DurableGenerationOptions, error) {
	options.IdempotencyKey = strings.TrimSpace(options.IdempotencyKey)
	options.ClientRequestID = strings.TrimSpace(options.ClientRequestID)
	options.WorkerID = strings.TrimSpace(options.WorkerID)
	if options.IdempotencyKey == "" || options.WorkerID == "" {
		return options, pptapp.ErrGenerationJobInvalid
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 3
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 30 * time.Second
	}
	if options.RetryDelay < 0 {
		return options, pptapp.ErrGenerationJobInvalid
	}
	return options, nil
}

func (a api) runPPTV2DurableGeneration(ctx context.Context, user adminUser, taskID string, jobs pptapp.GenerationJobStore, renderer pptV2Renderer, options pptV2DurableGenerationOptions) (pptV2VerticalSliceResult, pptapp.GenerationJob, error) {
	if jobs == nil || renderer == nil || a.pptService == nil || a.fileService == nil {
		return pptV2VerticalSliceResult{}, pptapp.GenerationJob{}, errors.New("ppt v2 durable generation is unavailable")
	}
	artifactStore, ok := a.store.(pptV2DurableArtifactStore)
	if !ok {
		return pptV2VerticalSliceResult{}, pptapp.GenerationJob{}, errors.New("ppt v2 durable artifact store is unavailable")
	}
	options, err := normalizePPTV2DurableOptions(options)
	if err != nil {
		return pptV2VerticalSliceResult{}, pptapp.GenerationJob{}, err
	}
	scope := pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: strings.TrimSpace(user.ID)}
	now := time.Now().UTC()
	job, _, err := jobs.Create(ctx, pptapp.CreateGenerationJobInput{
		TenantID: scope.TenantID, UserID: scope.UserID, OrganizationID: strings.TrimSpace(user.OrganizationID),
		ExistingTaskID: strings.TrimSpace(taskID), ClientRequestID: options.ClientRequestID,
		IdempotencyKey: options.IdempotencyKey, MaxAttempts: options.MaxAttempts, SlideCount: 2, Now: now,
	})
	if err != nil {
		return pptV2VerticalSliceResult{}, pptapp.GenerationJob{}, err
	}
	if job.Status == pptapp.GenerationJobSucceeded {
		result, resultErr := a.pptV2DurableResult(ctx, user, jobs, artifactStore, job.ID)
		return result, job, resultErr
	}
	lease, err := jobs.Claim(ctx, scope, job.ID, options.WorkerID, now, options.LeaseDuration)
	if err != nil {
		return pptV2VerticalSliceResult{}, job, err
	}
	job = lease.Job
	fail := func(code string, retryable bool, cause error) (pptV2VerticalSliceResult, pptapp.GenerationJob, error) {
		failed, failErr := jobs.Fail(ctx, lease, pptapp.GenerationJobError{
			Code: code, Message: sanitizePPTV2GenerationError(cause), Retryable: retryable,
		}, time.Now().UTC(), options.RetryDelay)
		if failErr != nil {
			return pptV2VerticalSliceResult{}, job, fmt.Errorf("%w (persist failure: %v)", cause, failErr)
		}
		return pptV2VerticalSliceResult{}, failed, cause
	}

	for {
		switch job.Stage {
		case pptapp.GenerationStageCreated:
			task, loadErr := a.pptService.GetTask(scope.UserID, job.ExistingTaskID)
			if loadErr != nil {
				return fail("TASK_NOT_FOUND", false, loadErr)
			}
			if task.TenantID != scope.TenantID || (job.OrganizationID != "" && task.OrganizationID != job.OrganizationID) {
				return fail("TASK_SCOPE_MISMATCH", false, pptapp.ErrTaskNotFound)
			}
			inputSnapshot, marshalErr := json.Marshal(legacyPPTV2Input(task))
			if marshalErr != nil {
				return fail("TASK_SNAPSHOT_INVALID", false, marshalErr)
			}
			sourceSlideIDs := make([]string, 0, 2)
			for index := 0; index < len(task.Slides) && index < 2; index++ {
				sourceSlideIDs = append(sourceSlideIDs, task.Slides[index].ID)
			}
			job, err = jobs.Checkpoint(ctx, lease, pptapp.GenerationCheckpoint{
				NextStage: pptapp.GenerationStageTaskLoaded, InputSnapshot: inputSnapshot,
				SourceSlideIDs: sourceSlideIDs, Now: time.Now().UTC(),
			})
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
		case pptapp.GenerationStageTaskLoaded:
			lease, err = jobs.Renew(ctx, lease, time.Now().UTC(), options.LeaseDuration)
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
			var input pptV2LegacyInput
			if err := json.Unmarshal(job.InputSnapshot, &input); err != nil {
				return fail("TASK_SNAPSHOT_INVALID", false, err)
			}
			rendered, renderErr := renderer.Render(ctx, input)
			if renderErr != nil {
				return fail("RENDER_FAILED", true, renderErr)
			}
			digest := sha256.Sum256(rendered.PPTX)
			job, err = jobs.Checkpoint(ctx, lease, pptapp.GenerationCheckpoint{
				NextStage: pptapp.GenerationStageRendered, DeckID: rendered.DeckID, Revision: rendered.Revision,
				SlideCount: rendered.SlideCount, RenderSHA256: hex.EncodeToString(digest[:]), RenderBytes: rendered.PPTX,
				Now: time.Now().UTC(),
			})
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
		case pptapp.GenerationStageRendered:
			lease, err = jobs.Renew(ctx, lease, time.Now().UTC(), options.LeaseDuration)
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
			file, found, fileErr := a.findPPTV2DurableFile(ctx, user, job.ID)
			if fileErr != nil {
				return fail("FILE_LOOKUP_FAILED", true, fileErr)
			}
			if !found {
				available, availableErr := a.fileService.StorageAvailable(ctx, scope.TenantID)
				if availableErr != nil {
					return fail("STORAGE_CHECK_FAILED", true, availableErr)
				}
				if !available {
					return fail("STORAGE_UNAVAILABLE", false, errors.New("private file storage is not configured"))
				}
				task, taskErr := a.pptService.GetTask(scope.UserID, job.ExistingTaskID)
				if taskErr != nil || task.TenantID != scope.TenantID {
					if taskErr == nil {
						taskErr = pptapp.ErrTaskNotFound
					}
					return fail("TASK_SCOPE_MISMATCH", false, taskErr)
				}
				file, fileErr = a.fileService.StoreObject(ctx, storagecenter.UploadInitInput{
					TenantID: scope.TenantID, UserID: scope.UserID, FileName: pptxDownloadFileName(task), FileSize: int64(len(job.RenderBytes)),
					MIMEType: pptxMIMEType, BusinessType: "ppt_v2_generation", BusinessID: job.ID, Visibility: "PRIVATE",
				}, bytes.NewReader(job.RenderBytes))
				if fileErr != nil {
					file, found, _ = a.findPPTV2DurableFile(ctx, user, job.ID)
					if !found {
						return fail("FILE_STORE_FAILED", true, fileErr)
					}
				}
			}
			job, err = jobs.Checkpoint(ctx, lease, pptapp.GenerationCheckpoint{NextStage: pptapp.GenerationStageFileStored, FileID: file.FileID, Now: time.Now().UTC()})
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
		case pptapp.GenerationStageFileStored:
			lease, err = jobs.Renew(ctx, lease, time.Now().UTC(), options.LeaseDuration)
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
			file, fileErr := a.fileService.GetFile(ctx, storagecenter.AccessContext{TenantID: scope.TenantID, UserID: scope.UserID}, job.FileID)
			if fileErr != nil {
				return fail("FILE_CHECKPOINT_MISSING", true, fileErr)
			}
			var snapshot pptV2LegacyInput
			if err := json.Unmarshal(job.InputSnapshot, &snapshot); err != nil {
				return fail("TASK_SNAPSHOT_INVALID", false, err)
			}
			artifactInput := pptV2DurableArtifactInput{
				GenerationJobID: job.ID, UserID: scope.UserID, TenantID: scope.TenantID, OrganizationID: job.OrganizationID,
				TaskID: job.ExistingTaskID, Title: snapshot.TaskContext.Title, DeckID: job.DeckID, Revision: job.Revision, File: file,
			}
			var createdAsset asset
			var assetErr error
			if fencedStore, ok := artifactStore.(pptV2FencedDurableArtifactStore); ok {
				createdAsset, _, assetErr = fencedStore.EnsurePPTV2DurableArtifactFenced(ctx, artifactInput, lease)
				if assetErr == nil {
					var bundle pptapp.GenerationJobBundle
					bundle, assetErr = jobs.Get(ctx, scope, job.ID)
					job = bundle.Job
				}
			} else {
				createdAsset, _, assetErr = artifactStore.EnsurePPTV2DurableArtifact(artifactInput)
				if assetErr == nil {
					job, assetErr = jobs.Checkpoint(ctx, lease, pptapp.GenerationCheckpoint{NextStage: pptapp.GenerationStageAssetCreated, AssetID: createdAsset.ID, Now: time.Now().UTC()})
				}
			}
			if assetErr != nil {
				if errors.Is(assetErr, pptapp.ErrGenerationJobLeaseLost) || errors.Is(assetErr, pptapp.ErrGenerationJobCancelled) {
					return pptV2VerticalSliceResult{}, job, assetErr
				}
				return fail("ASSET_PERSIST_FAILED", true, assetErr)
			}
		case pptapp.GenerationStageAssetCreated:
			lease, err = jobs.Renew(ctx, lease, time.Now().UTC(), options.LeaseDuration)
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
			relation := pptapp.V2ArtifactRelation{DeckID: job.DeckID, Revision: job.Revision, PPTXAssetID: job.AssetID}
			if atomicStore, ok := jobs.(pptapp.GenerationTaskRelationStore); ok {
				job, err = atomicStore.RelateTaskArtifact(ctx, lease, relation, time.Now().UTC())
			} else {
				_, err = a.pptService.AttachV2Artifact(scope.UserID, job.ExistingTaskID, relation)
				if err == nil {
					job, err = jobs.Checkpoint(ctx, lease, pptapp.GenerationCheckpoint{NextStage: pptapp.GenerationStageTaskRelated, Now: time.Now().UTC()})
				}
			}
			if err != nil {
				if errors.Is(err, pptapp.ErrV2ArtifactRelationConflict) || errors.Is(err, pptapp.ErrTaskNotFound) {
					return fail("TASK_RELATION_CONFLICT", false, err)
				}
				return pptV2VerticalSliceResult{}, job, err
			}
		case pptapp.GenerationStageTaskRelated:
			job, err = jobs.Checkpoint(ctx, lease, pptapp.GenerationCheckpoint{NextStage: pptapp.GenerationStageCompleted, Now: time.Now().UTC()})
			if err != nil {
				return pptV2VerticalSliceResult{}, job, err
			}
		case pptapp.GenerationStageCompleted:
			result, resultErr := a.pptV2DurableResult(ctx, user, jobs, artifactStore, job.ID)
			return result, job, resultErr
		default:
			return fail("INVALID_STAGE", false, fmt.Errorf("unknown durable generation stage %q", job.Stage))
		}
	}
}

func sanitizePPTV2GenerationError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func (a api) findPPTV2DurableFile(ctx context.Context, user adminUser, jobID string) (storagecenter.FileObject, bool, error) {
	items, _, err := a.fileService.ListFiles(ctx, storagecenter.FileFilter{
		TenantID: effectiveTenantID(user), UserID: user.ID, Status: storagecenter.StatusActive,
		BusinessType: "ppt_v2_generation", Limit: 200,
	})
	if err != nil {
		return storagecenter.FileObject{}, false, err
	}
	for _, item := range items {
		if item.BusinessID == strings.TrimSpace(jobID) {
			return item, true, nil
		}
	}
	return storagecenter.FileObject{}, false, nil
}

func (a api) pptV2DurableResult(ctx context.Context, user adminUser, jobs pptapp.GenerationJobStore, artifactStore pptV2DurableArtifactStore, jobID string) (pptV2VerticalSliceResult, error) {
	scope := pptapp.GenerationJobScope{TenantID: effectiveTenantID(user), UserID: user.ID}
	bundle, err := jobs.Get(ctx, scope, jobID)
	if err != nil {
		return pptV2VerticalSliceResult{}, err
	}
	if bundle.Job.Status != pptapp.GenerationJobSucceeded {
		return pptV2VerticalSliceResult{}, pptapp.ErrGenerationJobNotReady
	}
	file, err := a.fileService.GetFile(ctx, storagecenter.AccessContext{TenantID: scope.TenantID, UserID: scope.UserID}, bundle.Job.FileID)
	if err != nil {
		return pptV2VerticalSliceResult{}, err
	}
	createdAsset, found, err := artifactStore.FindPPTV2DurableArtifact(ctx, scope.TenantID, scope.UserID, bundle.Job.ID)
	if err != nil {
		return pptV2VerticalSliceResult{}, err
	}
	if !found || createdAsset.ID != bundle.Job.AssetID {
		return pptV2VerticalSliceResult{}, errors.New("ppt v2 durable artifact checkpoint is missing")
	}
	task, err := a.pptService.GetTask(scope.UserID, bundle.Job.ExistingTaskID)
	if err != nil || task.TenantID != scope.TenantID {
		if err == nil {
			err = pptapp.ErrTaskNotFound
		}
		return pptV2VerticalSliceResult{}, err
	}
	return pptV2VerticalSliceResult{
		DeckID: bundle.Job.DeckID, Revision: bundle.Job.Revision, PPTX: append([]byte(nil), bundle.Job.RenderBytes...),
		File: file, Asset: createdAsset, Task: task,
	}, nil
}
