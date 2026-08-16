package ppt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

func (s *AgentPlanningService) Wake() {
	if s == nil || s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *AgentPlanningService) Start(ctx context.Context) {
	if s == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(s.scanInterval)
		defer ticker.Stop()
		for {
			_ = s.ProcessReady(ctx, time.Now().UTC(), s.workerLimit*4)
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			case <-ticker.C:
			}
		}
	}()
}

func (s *AgentPlanningService) ProcessReady(ctx context.Context, now time.Time, limit int) error {
	if s == nil || s.store == nil {
		return ErrGenerationJobInvalid
	}
	if limit <= 0 {
		limit = s.workerLimit
	}
	jobs, err := s.store.ListReadyAgentPlanning(ctx, normalizedAgentTime(now), limit)
	if err != nil {
		return err
	}
	workerLimit := s.workerLimit
	if workerLimit <= 0 {
		workerLimit = 1
	}
	semaphore := make(chan struct{}, workerLimit)
	var wg sync.WaitGroup
	var firstErr error
	var errorMu sync.Mutex
	for index, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		semaphore <- struct{}{}
		wg.Add(1)
		go func(index int, job GenerationJob) {
			defer wg.Done()
			defer func() { <-semaphore }()
			workerID := fmt.Sprintf("%s:%d", s.workerID, index+1)
			lease, claimErr := s.store.Claim(ctx, GenerationJobScope{TenantID: job.TenantID, UserID: job.UserID}, job.ID, workerID, normalizedAgentTime(now), s.leaseDuration)
			if claimErr != nil {
				if benignPlanningClaimError(claimErr) {
					return
				}
				errorMu.Lock()
				if firstErr == nil {
					firstErr = claimErr
				}
				errorMu.Unlock()
				return
			}
			if processErr := s.processClaimedPlanningJob(ctx, lease, normalizedAgentTime(now)); processErr != nil && !benignPlanningClaimError(processErr) {
				errorMu.Lock()
				if firstErr == nil {
					firstErr = processErr
				}
				errorMu.Unlock()
			}
		}(index, job)
	}
	wg.Wait()
	return firstErr
}

func (s *AgentPlanningService) processClaimedPlanningJob(ctx context.Context, lease GenerationLease, now time.Time) error {
	providerCtx, cancelProviders := context.WithCancel(ctx)
	defer cancelProviders()
	stopHeartbeat := s.startPlanningLeaseHeartbeat(providerCtx, cancelProviders, lease)
	defer stopHeartbeat()

	scope := GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}
	for {
		state, err := s.store.GetAgentPlanning(ctx, scope, lease.JobID)
		if err != nil {
			return err
		}
		switch lease.Job.Stage {
		case GenerationStageCreated:
			lease, err = s.store.SaveAgentIntent(ctx, lease, state.Intent, now)
			if err != nil {
				return err
			}
		case GenerationStageIntentResolved:
			research, researchErr := s.executeResearch(providerCtx, state.Intent)
			if researchErr != nil {
				return s.failPlanningLease(ctx, lease, researchErr, now)
			}
			lease, err = s.store.SaveAgentResearch(ctx, lease, research, now)
			if err != nil {
				return err
			}
		case GenerationStageResearched:
			if s.storyline == nil {
				return s.failPlanningLease(ctx, lease, NewAgentWorkflowError(PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", true, nil), now)
			}
			output, planningErr := s.storyline.PlanStoryline(providerCtx, StorylinePlanningInput{Intent: state.Intent, Research: state.Research})
			if planningErr != nil {
				return s.failPlanningLease(ctx, lease, normalizePlanningError(planningErr), now)
			}
			storyline, planningErr := MaterializeStoryline(state.Intent, state.Research, output)
			if planningErr != nil {
				return s.failPlanningLease(ctx, lease, NewAgentWorkflowError(PlanningContractValidationFailed, "叙事规划结果未通过校验，请重试。", true, planningErr), now)
			}
			lease, err = s.store.SaveAgentStoryline(ctx, lease, storyline, now)
			if err != nil {
				return err
			}
		case GenerationStageStorylinePlanned:
			if s.outline == nil {
				return s.failPlanningLease(ctx, lease, NewAgentWorkflowError(PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", true, nil), now)
			}
			output, planningErr := s.outline.PlanOutline(providerCtx, OutlinePlanningInput{Intent: state.Intent, Research: state.Research, Storyline: state.Storyline})
			if planningErr != nil {
				return s.failPlanningLease(ctx, lease, normalizePlanningError(planningErr), now)
			}
			outline, planningErr := MaterializeOutlinePlan(lease.JobID, state.Intent, state.Research, state.Storyline, output, now)
			if planningErr != nil {
				code := PlanningContractValidationFailed
				message := "大纲规划结果未通过校验，请重试。"
				if errors.Is(planningErr, ErrInvalidEvidenceMapping) {
					code = PlanningEvidenceMappingInvalid
					message = "大纲证据关系不完整，请重试。"
				}
				return s.failPlanningLease(ctx, lease, NewAgentWorkflowError(code, message, true, planningErr), now)
			}
			_, err = s.store.SaveAgentOutline(ctx, lease, outline, now)
			return err
		case GenerationStageOutlineApproved, GenerationStageContentReady, GenerationStageAssetsReady,
			GenerationStageLayoutCompiled, GenerationStageQualityChecked, GenerationStageRendered,
			GenerationStageFileStored, GenerationStageAssetCreated, GenerationStageTaskRelated:
			lease, err = s.processClaimedDeckStage(providerCtx, lease, state, now)
			if err != nil {
				return err
			}
			if lease.JobID == "" {
				return nil
			}
			if lease.Job.Stage == GenerationStageCompleted {
				return nil
			}
		default:
			return nil
		}
	}
}

func (s *AgentPlanningService) processClaimedDeckStage(ctx context.Context, lease GenerationLease, planning AgentPlanningState, now time.Time) (GenerationLease, error) {
	if planning.ApprovedOutline == nil {
		return GenerationLease{}, ErrGenerationJobTransition
	}
	deck := AgentDeckGenerationState{}
	if planning.DeckGeneration != nil {
		deck = cloneAgentDeckGenerationState(*planning.DeckGeneration)
	}
	outline := *planning.ApprovedOutline
	contentBase := 3
	assetBase := contentBase + len(outline.Slides)
	layoutBase := assetBase + agentDeckImageCount(outline)
	qualityWork := layoutBase + len(outline.Slides) + 1
	renderWork := qualityWork + 1
	fileWork := renderWork + 1
	artifactWork := fileWork + 1
	totalWork := artifactWork + 1
	checkpoint := func(next string, completed int) (GenerationLease, error) {
		return s.store.SaveAgentDeckCheckpoint(ctx, lease, AgentDeckCheckpoint{ExpectedStage: lease.Job.Stage, NextStage: next, State: deck, CompletedWorkUnits: completed, Now: normalizedAgentTime(now)})
	}

	switch lease.Job.Stage {
	case GenerationStageOutlineApproved:
		if s.content == nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ContentProviderUnavailable, "内容生成服务暂时不可用，请稍后重试。", true, nil), now)
		}
		existing := map[string]struct{}{}
		for _, item := range deck.Contents {
			existing[item.SlideID] = struct{}{}
		}
		for _, objective := range outline.Slides {
			if _, ok := existing[objective.SlideID]; ok {
				continue
			}
			output, err := s.content.PlanSlideContent(ctx, SlideContentPlanningInput{Intent: planning.Intent, Research: planning.Research, Storyline: planning.Storyline, ApprovedOutline: outline, Objective: objective})
			if err != nil {
				return GenerationLease{}, s.failPlanningLease(ctx, lease, normalizeContentError(err), now)
			}
			content, err := MaterializeSlideContent(SlideContentPlanningInput{Intent: planning.Intent, Research: planning.Research, Storyline: planning.Storyline, ApprovedOutline: outline, Objective: objective}, output)
			if err != nil {
				code := ContentContractValidationFailed
				if errors.Is(err, ErrInvalidSlideContentEvidence) {
					code = ContentEvidenceMappingInvalid
				}
				return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(code, "页面内容未通过校验，请重试。", true, err), now)
			}
			deck.Contents = append(deck.Contents, content)
			deck.ContentExecutions++
			lease, err = s.store.SaveAgentDeckCheckpoint(ctx, lease, AgentDeckCheckpoint{ExpectedStage: GenerationStageOutlineApproved, NextStage: GenerationStageOutlineApproved, State: deck, CompletedWorkUnits: contentBase + len(deck.Contents), Now: normalizedAgentTime(now)})
			if err != nil {
				return GenerationLease{}, err
			}
		}
		return checkpoint(GenerationStageContentReady, assetBase)
	case GenerationStageContentReady:
		if s.assets == nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ImageProviderUnavailable, "图片服务暂时不可用，请稍后重试。", true, nil), now)
		}
		existing := map[string]struct{}{}
		for _, asset := range deck.Assets {
			existing[asset.IntentID] = struct{}{}
		}
		for _, content := range deck.Contents {
			for _, intent := range content.AssetIntents {
				if _, ok := existing[intent.StableID]; ok {
					continue
				}
				asset, err := s.assets.ResolveImage(ctx, GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, lease.JobID, content.SlideID, intent)
				if err != nil {
					return GenerationLease{}, s.failPlanningLease(ctx, lease, normalizeImageError(err), now)
				}
				if err := validateResolvedDeckAsset(asset, GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}, content.SlideID, intent); err != nil {
					return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ImageInvalidResult, "图片结果无效，请重试。", true, err), now)
				}
				deck.Assets = append(deck.Assets, asset)
				deck.AssetExecutions++
				lease, err = s.store.SaveAgentDeckCheckpoint(ctx, lease, AgentDeckCheckpoint{ExpectedStage: GenerationStageContentReady, NextStage: GenerationStageContentReady, State: deck, CompletedWorkUnits: assetBase + len(deck.Assets), Now: normalizedAgentTime(now)})
				if err != nil {
					return GenerationLease{}, err
				}
			}
		}
		return checkpoint(GenerationStageAssetsReady, layoutBase)
	case GenerationStageAssetsReady:
		if s.compiler == nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(LayoutCompilationFailed, "排版服务暂时不可用，请稍后重试。", true, nil), now)
		}
		compiled, err := s.compiler.Compile(ctx, DeckBuildInput{GenerationJobID: lease.JobID, Revision: outline.Revision, Intent: planning.Intent, Research: planning.Research, Storyline: planning.Storyline, ApprovedOutline: outline, SlideContents: deck.Contents, Assets: deck.Assets})
		if err != nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(LayoutCompilationFailed, "演示文稿排版失败，请重试。", true, err), now)
		}
		deck.Compilation = &compiled
		deck.LayoutExecutions++
		return checkpoint(GenerationStageLayoutCompiled, layoutBase+len(outline.Slides))
	case GenerationStageLayoutCompiled:
		if deck.Compilation == nil || !deck.Compilation.QualityValid || len(deck.Compilation.QualityIssues) > 0 {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(QualityGateFailed, "演示文稿存在阻断问题，暂时无法导出。", false, nil), now)
		}
		return checkpoint(GenerationStageQualityChecked, qualityWork)
	case GenerationStageQualityChecked:
		if s.compiler == nil || deck.Compilation == nil {
			return GenerationLease{}, ErrGenerationJobTransition
		}
		rendered, err := s.compiler.Render(ctx, *deck.Compilation, deck.Assets)
		if err != nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(PPTXRenderFailed, "PPTX 生成失败，请重试。", true, err), now)
		}
		if rendered.DeckID != deck.Compilation.DeckID || rendered.Revision != deck.Compilation.Revision || rendered.SlideCount != len(outline.Slides) || len(rendered.PPTX) < 2 || string(rendered.PPTX[:2]) != "PK" {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(PPTXRenderFailed, "PPTX 结果无效，请重试。", true, nil), now)
		}
		deck.RenderExecutions++
		digest := sha256.Sum256(rendered.PPTX)
		return s.store.SaveAgentDeckCheckpoint(ctx, lease, AgentDeckCheckpoint{ExpectedStage: GenerationStageQualityChecked, NextStage: GenerationStageRendered, State: deck, CompletedWorkUnits: renderWork, DeckID: rendered.DeckID, Revision: rendered.Revision, RenderSHA256: hex.EncodeToString(digest[:]), RenderBytes: rendered.PPTX, Now: normalizedAgentTime(now)})
	case GenerationStageRendered:
		if s.artifacts == nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ArtifactStorageFailed, "文件存储暂时不可用，请稍后重试。", true, nil), now)
		}
		scope := GenerationJobScope{TenantID: lease.TenantID, UserID: lease.UserID}
		taskID, err := s.artifacts.EnsureTask(ctx, scope, lease.JobID, planning.Intent, outline, deck.Contents)
		if err != nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ArtifactStorageFailed, "演示文稿项目创建失败，请重试。", true, err), now)
		}
		fileID, err := s.artifacts.StorePPTX(ctx, scope, lease.JobID, planning.Intent.Topic, lease.Job.RenderBytes)
		if err != nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ArtifactStorageFailed, "PPTX 保存失败，请重试。", true, err), now)
		}
		return s.store.SaveAgentDeckCheckpoint(ctx, lease, AgentDeckCheckpoint{ExpectedStage: GenerationStageRendered, NextStage: GenerationStageFileStored, State: deck, CompletedWorkUnits: fileWork, ExistingTaskID: taskID, FileID: fileID, Now: normalizedAgentTime(now)})
	case GenerationStageFileStored:
		assetID, assetJob, err := s.artifacts.EnsureArtifact(ctx, lease, lease.Job.ExistingTaskID, lease.Job.FileID, lease.Job.DeckID, lease.Job.Revision)
		if err != nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ArtifactStorageFailed, "作品保存失败，请重试。", true, err), now)
		}
		if assetID == "" || assetJob.Stage != GenerationStageAssetCreated || assetJob.AssetID != assetID || assetJob.CompletedWorkUnits != artifactWork {
			return GenerationLease{}, ErrGenerationJobTransition
		}
		lease.Job = assetJob
		return lease, nil
	case GenerationStageAssetCreated:
		related, err := s.artifacts.RelateTask(ctx, lease, lease.Job.ExistingTaskID, V2ArtifactRelation{DeckID: lease.Job.DeckID, Revision: lease.Job.Revision, PPTXAssetID: lease.Job.AssetID})
		if err != nil {
			return GenerationLease{}, s.failPlanningLease(ctx, lease, NewAgentWorkflowError(ArtifactRelationFailed, "作品关联失败，请重试。", true, err), now)
		}
		if related.Stage != GenerationStageTaskRelated || related.CompletedWorkUnits != totalWork {
			return GenerationLease{}, ErrGenerationJobTransition
		}
		lease.Job = related
		return lease, nil
	case GenerationStageTaskRelated:
		return checkpoint(GenerationStageCompleted, totalWork)
	default:
		return GenerationLease{}, ErrGenerationJobTransition
	}
}

func normalizeContentError(err error) *AgentWorkflowError {
	var workflowErr *AgentWorkflowError
	if errors.As(err, &workflowErr) {
		return workflowErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewAgentWorkflowError(ContentTimeout, "页面内容生成超时，请重试。", true, err)
	}
	return NewAgentWorkflowError(ContentProviderUnavailable, "内容生成服务暂时不可用，请稍后重试。", true, err)
}

func normalizeImageError(err error) *AgentWorkflowError {
	var workflowErr *AgentWorkflowError
	if errors.As(err, &workflowErr) {
		return workflowErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewAgentWorkflowError(ImageTimeout, "图片生成超时，请重试。", true, err)
	}
	return NewAgentWorkflowError(ImageProviderUnavailable, "图片服务暂时不可用，请稍后重试。", true, err)
}

func validateResolvedDeckAsset(asset ResolvedDeckAsset, scope GenerationJobScope, slideID string, intent SlideAssetIntent) error {
	if asset.ID == "" || asset.TenantID != scope.TenantID || asset.UserID != scope.UserID || asset.IntentID != intent.StableID || asset.SlideID != slideID || asset.MIMEType == "" || !strings.HasPrefix(asset.URI, "asset://") || len(asset.SHA256) != 64 || asset.FileID == "" || asset.AltText == "" {
		return ErrGenerationJobInvalid
	}
	return nil
}

func (s *AgentPlanningService) executeResearch(ctx context.Context, intent IntentSpec) (ResearchPack, *AgentWorkflowError) {
	if !intent.ResearchRequired {
		return ResearchPack{}, nil
	}
	if s.research == nil {
		return ResearchPack{}, NewAgentWorkflowError(ResearchProviderUnavailable, "研究服务暂时不可用，请稍后重试。", true, nil)
	}
	pack, err := s.research.Research(ctx, intent)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) && ctx.Err() == context.DeadlineExceeded {
			return ResearchPack{}, NewAgentWorkflowError(ResearchTimeout, "研究资料请求超时，请重试。", true, err)
		}
		return ResearchPack{}, NewAgentWorkflowError(ResearchProviderUnavailable, "研究服务暂时不可用，请稍后重试。", true, err)
	}
	pack, err = NormalizeResearchPack(pack)
	if err != nil {
		return ResearchPack{}, NewAgentWorkflowError(ResearchContractValidationFailed, "研究资料未通过结构校验，请重试。", true, err)
	}
	if len(pack.Claims) == 0 {
		return ResearchPack{}, NewAgentWorkflowError(ResearchInvalidResult, "没有获得可验证的研究结论，请重试。", true, nil)
	}
	return pack, nil
}

func (s *AgentPlanningService) failPlanningLease(ctx context.Context, lease GenerationLease, workflowErr *AgentWorkflowError, now time.Time) error {
	if workflowErr == nil {
		workflowErr = NewAgentWorkflowError(PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", true, nil)
	}
	_, err := s.store.Fail(ctx, lease, GenerationJobError{
		Code: workflowErr.Code, Message: workflowErr.SafeMessage, Retryable: workflowErr.Retryable,
		Provider: workflowErr.Provider, ProviderRequestID: workflowErr.ProviderRequestID, OccurredAt: now,
	}, now, s.retryDelay)
	return err
}

func (s *AgentPlanningService) startPlanningLeaseHeartbeat(ctx context.Context, cancel context.CancelFunc, lease GenerationLease) func() {
	interval := s.leaseDuration / 3
	if interval <= 0 {
		interval = time.Second
	}
	heartbeatCtx, stop := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case tick := <-ticker.C:
				if _, err := s.store.Renew(heartbeatCtx, lease, tick.UTC(), s.leaseDuration); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return stop
}

func normalizePlanningError(err error) *AgentWorkflowError {
	var workflowErr *AgentWorkflowError
	if errors.As(err, &workflowErr) {
		return workflowErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewAgentWorkflowError(PlanningTimeout, "规划服务响应超时，请重试。", true, err)
	}
	return NewAgentWorkflowError(PlanningProviderUnavailable, "规划服务暂时不可用，请稍后重试。", true, err)
}

func benignPlanningClaimError(err error) bool {
	return errors.Is(err, ErrGenerationJobLeaseHeld) || errors.Is(err, ErrGenerationJobNotReady) ||
		errors.Is(err, ErrGenerationJobAwaitingOutlineApproval) || errors.Is(err, ErrGenerationJobTerminal) ||
		errors.Is(err, ErrGenerationJobCancelled) || errors.Is(err, ErrGenerationJobLeaseLost)
}

func (s *MemoryGenerationJobStore) CreateAgentPlanning(_ context.Context, input CreateGenerationJobInput, intent IntentSpec) (GenerationJob, bool, error) {
	job, deck, slides, err := NormalizeCreateGenerationJob(input)
	if err != nil {
		return GenerationJob{}, false, err
	}
	if job.WorkflowType != GenerationWorkflowAgentOutline || len(job.InputSnapshot) == 0 || strings.TrimSpace(intent.Topic) == "" {
		return GenerationJob{}, false, ErrGenerationJobInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, created, err := s.createNormalizedLocked(job, deck, slides)
	if err != nil {
		return GenerationJob{}, false, err
	}
	if created {
		s.agentPlans[job.ID] = AgentPlanningRecord{Intent: intent}
	}
	return job, created, nil
}

func (s *MemoryGenerationJobStore) ListReadyAgentPlanning(_ context.Context, now time.Time, limit int) ([]GenerationJob, error) {
	if limit <= 0 {
		return nil, ErrGenerationJobInvalid
	}
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]GenerationJob, 0, limit)
	for _, job := range s.jobs {
		if job.WorkflowType != GenerationWorkflowAgentOutline || !readyAgentPlanningJob(job, now) {
			continue
		}
		jobs = append(jobs, cloneGenerationJob(job))
	}
	sort.Slice(jobs, func(i, j int) bool {
		if !jobs[i].RunAfter.Equal(jobs[j].RunAfter) {
			return jobs[i].RunAfter.Before(jobs[j].RunAfter)
		}
		if !jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
		}
		return jobs[i].ID < jobs[j].ID
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs, nil
}

func (s *MemoryGenerationJobStore) RetryAgentPlanning(_ context.Context, scope GenerationJobScope, jobID string, now time.Time) (GenerationJob, error) {
	now = normalizedAgentTime(now)
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.jobs[strings.TrimSpace(jobID)]
	if !exists || job.WorkflowType != GenerationWorkflowAgentOutline || job.TenantID != strings.TrimSpace(scope.TenantID) || job.UserID != strings.TrimSpace(scope.UserID) {
		return GenerationJob{}, ErrGenerationJobNotFound
	}
	switch job.Status {
	case GenerationJobQueued, GenerationJobRunning:
		return cloneGenerationJob(job), nil
	case GenerationJobRetryWait:
		job.Status = GenerationJobQueued
		job.RunAfter = now
		job.LastError = nil
		job.UpdatedAt = now
	case GenerationJobFailed:
		if job.LastError == nil || !job.LastError.Retryable || job.AttemptCount >= 20 {
			return GenerationJob{}, ErrGenerationJobTerminal
		}
		job.Status = GenerationJobQueued
		job.MaxAttempts = job.AttemptCount + 1
		job.RunAfter = now
		job.LastError = nil
		job.FinishedAt = time.Time{}
		job.UpdatedAt = now
	default:
		return GenerationJob{}, ErrGenerationJobTransition
	}
	s.jobs[job.ID] = job
	return cloneGenerationJob(job), nil
}

func readyAgentPlanningJob(job GenerationJob, now time.Time) bool {
	switch job.Stage {
	case GenerationStageCreated, GenerationStageIntentResolved, GenerationStageResearched, GenerationStageStorylinePlanned,
		GenerationStageOutlineApproved, GenerationStageContentReady, GenerationStageAssetsReady, GenerationStageLayoutCompiled,
		GenerationStageQualityChecked, GenerationStageRendered, GenerationStageFileStored, GenerationStageAssetCreated, GenerationStageTaskRelated:
	default:
		return false
	}
	switch job.Status {
	case GenerationJobQueued, GenerationJobRetryWait:
		return !job.RunAfter.After(now)
	case GenerationJobRunning:
		return !job.LeaseExpiresAt.After(now)
	default:
		return false
	}
}
