package httpserver

import (
	"strings"
	"time"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptV2ArtifactAssetInput struct {
	UserID         string
	TenantID       string
	OrganizationID string
	TaskID         string
	Title          string
	DeckID         string
	Revision       int
	File           storagecenter.FileObject
}

type pptV2ArtifactAssetStore interface {
	CreatePPTV2ArtifactAsset(pptV2ArtifactAssetInput) (asset, error)
}

func pptV2WorkCenterAsset(input pptV2ArtifactAssetInput, assetID string, now string) asset {
	return asset{
		ID: assetID, UserID: input.UserID, TenantID: input.TenantID, OrganizationID: input.OrganizationID,
		TaskID: input.TaskID, Name: firstNonEmptyString(strings.TrimSpace(input.Title), "AI演示文稿") + ".pptx",
		MediaType: "ppt", URL: pptStorageReference(input.File), Favorite: false,
		Metadata: map[string]any{
			"fileId": input.File.FileID, "storageFileId": input.File.FileID, "storageTenantId": input.File.TenantID,
			"storageProvider": input.File.Provider, "storageBucket": input.File.Bucket, "storageObjectKey": input.File.ObjectKey,
			"fileSize": input.File.FileSize, "fileSizeBytes": input.File.FileSize, "contentType": input.File.MIMEType,
			"storageManaged": true, "source": "ppt-v2-phase1", "sourceType": "PPT_GENERATION", "type": "PPT_GENERATION",
			"v2DeckId": input.DeckID, "v2Revision": input.Revision,
		},
		CreatedAt: now, UpdatedAt: now,
	}
}

func (s *jsonStore) CreatePPTV2ArtifactAsset(input pptV2ArtifactAssetInput) (asset, error) {
	var created asset
	err := s.update(func(data *platformData) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		created = pptV2WorkCenterAsset(input, nextID(data.Counters, "asset"), now)
		data.Assets = append(data.Assets, created)
		return nil
	})
	return created, err
}

func (s *postgresStore) CreatePPTV2ArtifactAsset(input pptV2ArtifactAssetInput) (asset, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return asset{}, err
	}
	defer func() { _ = tx.Rollback() }()
	assetID, err := nextTableID(ctx, tx, "xz_assets", "asset")
	if err != nil {
		return asset{}, err
	}
	created := pptV2WorkCenterAsset(input, assetID, time.Now().UTC().Format(time.RFC3339Nano))
	if err := insertAsset(ctx, tx, created); err != nil {
		return asset{}, err
	}
	if err := tx.Commit(); err != nil {
		return asset{}, err
	}
	return created, nil
}

var _ pptV2ArtifactAssetStore = (*jsonStore)(nil)
var _ pptV2ArtifactAssetStore = (*postgresStore)(nil)
