package storage

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func (s *Service) InitMultipartUpload(ctx context.Context, input MultipartInitInput) (MultipartSession, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey != "" {
		if existing, err := s.repo.GetMultipartSessionByIdempotency(ctx, input.TenantID, input.UserID, input.IdempotencyKey); err == nil {
			return sessionTicket(existing, s.options.UploadURLTTL), nil
		} else if err != ErrMultipartNotFound {
			return MultipartSession{}, err
		}
	}
	partSize := normalizePartSize(input.PartSize)
	if input.FileSize <= 0 {
		return MultipartSession{}, ErrInvalidFileSize
	}
	totalParts := int(math.Ceil(float64(input.FileSize) / float64(partSize)))
	if totalParts < 1 {
		totalParts = 1
	}
	if totalParts > MaxMultipartParts {
		return MultipartSession{}, fmt.Errorf("%w: too many parts (%d)", ErrInvalidFileSize, totalParts)
	}

	file, provider, err := s.preparePendingUpload(ctx, input.UploadInitInput)
	if err != nil {
		return MultipartSession{}, err
	}
	providerUploadID, err := provider.CreateMultipartUpload(ctx, file.ObjectKey, file.MIMEType)
	if err != nil {
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		return MultipartSession{}, fmt.Errorf("STORAGE_MULTIPART_INIT_FAILED: %w", err)
	}
	now := time.Now().UTC()
	expiresAt := now.Add(MultipartSessionTTL)
	session := MultipartUploadRecord{
		ID: newID("mpu"), TenantID: file.TenantID, OwnerUserID: file.UserID, FileID: file.FileID,
		ProviderUploadID: providerUploadID, ObjectKey: file.ObjectKey, FileName: file.OriginalName,
		ContentType: file.MIMEType, TotalSize: input.FileSize, PartSize: partSize, TotalParts: totalParts,
		State: MultipartStateInitialized, IdempotencyKey: input.IdempotencyKey, ExpiresAt: expiresAt,
		CreatedAt: now, Parts: map[int]CompletedPart{},
	}
	if err := s.repo.CreateMultipartSession(ctx, session); err != nil {
		_ = provider.AbortMultipartUpload(ctx, file.ObjectKey, providerUploadID)
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, err.Error())
		return MultipartSession{}, err
	}
	return sessionTicket(session, s.options.UploadURLTTL), nil
}

func (s *Service) PresignMultipartPart(ctx context.Context, access AccessContext, uploadID string, partNumber int) (MultipartPartTicket, error) {
	session, err := s.getActiveMultipart(ctx, access, uploadID)
	if err != nil {
		return MultipartPartTicket{}, err
	}
	if partNumber < 1 || partNumber > session.TotalParts {
		return MultipartPartTicket{}, ErrInvalidMultipartPart
	}
	file, err := s.repo.GetFile(ctx, access.TenantID, session.FileID)
	if err != nil {
		return MultipartPartTicket{}, err
	}
	provider, err := s.providerForFile(ctx, file)
	if err != nil {
		return MultipartPartTicket{}, err
	}
	ttl := s.options.UploadURLTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	uploadURL, err := provider.PresignUploadPart(ctx, session.ObjectKey, session.ProviderUploadID, partNumber, ttl)
	if err != nil {
		return MultipartPartTicket{}, err
	}
	if session.State == MultipartStateInitialized {
		_ = s.repo.UpdateMultipartState(ctx, session.TenantID, session.ID, MultipartStateUploading, nil)
	}
	return MultipartPartTicket{
		PartNumber: partNumber, UploadURL: uploadURL, Headers: map[string]string{},
		ExpiresIn: int64(ttl.Seconds()),
	}, nil
}

func (s *Service) CompleteMultipartUpload(ctx context.Context, access AccessContext, uploadID string, parts []CompletedPart) (FileObject, error) {
	session, err := s.getActiveMultipart(ctx, access, uploadID)
	if err != nil {
		return FileObject{}, err
	}
	normalized, err := normalizeCompletedParts(parts, session.TotalParts)
	if err != nil {
		return FileObject{}, err
	}
	file, err := s.repo.GetFile(ctx, access.TenantID, session.FileID)
	if err != nil {
		return FileObject{}, err
	}
	provider, err := s.providerForFile(ctx, file)
	if err != nil {
		return FileObject{}, err
	}
	_ = s.repo.UpdateMultipartState(ctx, session.TenantID, session.ID, MultipartStateCompleting, nil)
	for _, part := range normalized {
		_ = s.repo.SaveMultipartPart(ctx, session.ID, part)
	}
	metadata, err := provider.CompleteMultipartUpload(ctx, session.ObjectKey, session.ProviderUploadID, normalized)
	if err != nil {
		return FileObject{}, fmt.Errorf("%w: %v", ErrUploadConfirmFailed, err)
	}
	if head, headErr := provider.HeadObject(ctx, session.ObjectKey); headErr == nil {
		if head.Size > 0 {
			metadata.Size = head.Size
		}
		if strings.TrimSpace(head.ContentType) != "" {
			metadata.ContentType = head.ContentType
		}
		if strings.TrimSpace(head.ETag) != "" {
			metadata.ETag = head.ETag
		}
	}
	if metadata.Size == 0 {
		metadata.Size = session.TotalSize
	}
	if metadata.Size != session.TotalSize {
		_ = provider.AbortMultipartUpload(ctx, session.ObjectKey, session.ProviderUploadID)
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, "multipart object size mismatch")
		_ = s.repo.UpdateMultipartState(ctx, session.TenantID, session.ID, MultipartStateAborted, ptrTime(time.Now().UTC()))
		return FileObject{}, fmt.Errorf("%w: stored object size does not match reservation", ErrUploadConfirmFailed)
	}
	if strings.TrimSpace(metadata.ContentType) == "" {
		metadata.ContentType = session.ContentType
	}
	completed, err := s.repo.CompleteUpload(ctx, file.TenantID, file.FileID, metadata)
	if err != nil {
		return FileObject{}, err
	}
	now := time.Now().UTC()
	_ = s.repo.UpdateMultipartState(ctx, session.TenantID, session.ID, MultipartStateCompleted, &now)
	return completed, nil
}

func (s *Service) AbortMultipartUpload(ctx context.Context, access AccessContext, uploadID string) error {
	session, err := s.repo.GetMultipartSession(ctx, access.TenantID, access.UserID, strings.TrimSpace(uploadID))
	if err != nil {
		return err
	}
	switch session.State {
	case MultipartStateCompleted, MultipartStateAborted:
		return nil
	}
	file, err := s.repo.GetFile(ctx, access.TenantID, session.FileID)
	if err != nil && err != ErrFileNotFound {
		return err
	}
	if err == nil {
		if provider, providerErr := s.providerForFile(ctx, file); providerErr == nil {
			_ = provider.AbortMultipartUpload(ctx, session.ObjectKey, session.ProviderUploadID)
		}
		_ = s.repo.MarkUploadFailed(ctx, file.TenantID, file.FileID, "multipart aborted")
	}
	now := time.Now().UTC()
	return s.repo.UpdateMultipartState(ctx, session.TenantID, session.ID, MultipartStateAborted, &now)
}

func (s *Service) getActiveMultipart(ctx context.Context, access AccessContext, uploadID string) (MultipartUploadRecord, error) {
	session, err := s.repo.GetMultipartSession(ctx, access.TenantID, access.UserID, strings.TrimSpace(uploadID))
	if err != nil {
		return MultipartUploadRecord{}, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.AbortMultipartUpload(ctx, access, session.ID)
		return MultipartUploadRecord{}, ErrMultipartExpired
	}
	switch session.State {
	case MultipartStateInitialized, MultipartStateUploading, MultipartStateCompleting:
		return session, nil
	case MultipartStateCompleted:
		return MultipartUploadRecord{}, fmt.Errorf("%w: already completed", ErrMultipartState)
	case MultipartStateAborted, MultipartStateExpired:
		return MultipartUploadRecord{}, fmt.Errorf("%w: %s", ErrMultipartState, session.State)
	default:
		return MultipartUploadRecord{}, ErrMultipartState
	}
}

func normalizePartSize(partSize int64) int64 {
	if partSize <= 0 {
		return DefaultMultipartPartSize
	}
	if partSize < MinMultipartPartSize {
		return MinMultipartPartSize
	}
	if partSize > MaxMultipartPartSize {
		return MaxMultipartPartSize
	}
	return partSize
}

func normalizeCompletedParts(parts []CompletedPart, totalParts int) ([]CompletedPart, error) {
	if len(parts) == 0 {
		return nil, ErrInvalidMultipartPart
	}
	seen := map[int]CompletedPart{}
	for _, part := range parts {
		if part.PartNumber < 1 || part.PartNumber > totalParts {
			return nil, ErrInvalidMultipartPart
		}
		etag := strings.TrimSpace(strings.Trim(part.ETag, `"`))
		if etag == "" {
			return nil, ErrInvalidMultipartPart
		}
		part.ETag = etag
		seen[part.PartNumber] = part
	}
	if len(seen) != totalParts {
		return nil, fmt.Errorf("%w: expected %d parts got %d", ErrInvalidMultipartPart, totalParts, len(seen))
	}
	normalized := make([]CompletedPart, 0, totalParts)
	for i := 1; i <= totalParts; i++ {
		normalized = append(normalized, seen[i])
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].PartNumber < normalized[j].PartNumber })
	return normalized, nil
}

func sessionTicket(session MultipartUploadRecord, uploadTTL time.Duration) MultipartSession {
	expiresIn := int64(time.Until(session.ExpiresAt).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}
	if uploadTTL > 0 && int64(uploadTTL.Seconds()) < expiresIn {
		// expose remaining session TTL; upload URL TTL is per-part.
	}
	return MultipartSession{
		UploadID: session.ID, FileID: session.FileID, ObjectKey: session.ObjectKey,
		PartSize: session.PartSize, TotalParts: session.TotalParts,
		ExpiresIn: expiresIn, ExpiresAt: session.ExpiresAt,
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
