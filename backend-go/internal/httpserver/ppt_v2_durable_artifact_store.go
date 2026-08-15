package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptV2DurableArtifactInput struct {
	GenerationJobID string
	UserID          string
	TenantID        string
	OrganizationID  string
	TaskID          string
	Title           string
	DeckID          string
	Revision        int
	File            storagecenter.FileObject
}

type pptV2DurableArtifactStore interface {
	EnsurePPTV2DurableArtifact(pptV2DurableArtifactInput) (asset, bool, error)
	FindPPTV2DurableArtifact(context.Context, string, string, string) (asset, bool, error)
}

type pptV2FencedDurableArtifactStore interface {
	EnsurePPTV2DurableArtifactFenced(context.Context, pptV2DurableArtifactInput, pptapp.GenerationLease) (asset, bool, error)
}

func validatePPTV2DurableArtifactInput(input pptV2DurableArtifactInput) error {
	if strings.TrimSpace(input.GenerationJobID) == "" || strings.TrimSpace(input.UserID) == "" || strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.TaskID) == "" || strings.TrimSpace(input.DeckID) == "" || input.Revision <= 0 || strings.TrimSpace(input.File.FileID) == "" {
		return errors.New("ppt v2 durable artifact input is invalid")
	}
	return nil
}

func pptV2DurableWorkCenterAsset(input pptV2DurableArtifactInput, assetID, now string) asset {
	created := pptV2WorkCenterAsset(pptV2ArtifactAssetInput{
		UserID: input.UserID, TenantID: input.TenantID, OrganizationID: input.OrganizationID,
		TaskID: input.TaskID, Title: input.Title, DeckID: input.DeckID, Revision: input.Revision, File: input.File,
	}, assetID, now)
	created.Metadata["source"] = "ppt-v2"
	created.Metadata["pptV2GenerationJobId"] = input.GenerationJobID
	return created
}

func validateExistingPPTV2DurableArtifact(existing asset, input pptV2DurableArtifactInput) error {
	if existing.UserID != input.UserID || existing.TenantID != input.TenantID || existing.OrganizationID != input.OrganizationID || existing.TaskID != input.TaskID || stringValue(existing.Metadata["v2DeckId"]) != input.DeckID || intValue(existing.Metadata["v2Revision"]) != input.Revision || stringValue(existing.Metadata["fileId"]) != input.File.FileID {
		return errors.New("ppt v2 durable artifact idempotency conflict")
	}
	return nil
}

func (s *jsonStore) EnsurePPTV2DurableArtifact(input pptV2DurableArtifactInput) (asset, bool, error) {
	if err := validatePPTV2DurableArtifactInput(input); err != nil {
		return asset{}, false, err
	}
	var result asset
	createdNow := false
	err := s.update(func(data *platformData) error {
		for _, existing := range data.Assets {
			if stringValue(existing.Metadata["pptV2GenerationJobId"]) != input.GenerationJobID {
				continue
			}
			if err := validateExistingPPTV2DurableArtifact(existing, input); err != nil {
				return err
			}
			result = existing
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result = pptV2DurableWorkCenterAsset(input, nextID(data.Counters, "asset"), now)
		data.Assets = append(data.Assets, result)
		createdNow = true
		return nil
	})
	return result, createdNow, err
}

func (s *jsonStore) FindPPTV2DurableArtifact(_ context.Context, tenantID, userID, generationJobID string) (asset, bool, error) {
	assets, err := s.ListAssets()
	if err != nil {
		return asset{}, false, err
	}
	for _, existing := range assets {
		if existing.TenantID == strings.TrimSpace(tenantID) && existing.UserID == strings.TrimSpace(userID) && stringValue(existing.Metadata["pptV2GenerationJobId"]) == strings.TrimSpace(generationJobID) {
			return existing, true, nil
		}
	}
	return asset{}, false, nil
}

func (s *postgresStore) EnsurePPTV2DurableArtifact(input pptV2DurableArtifactInput) (asset, bool, error) {
	if err := validatePPTV2DurableArtifactInput(input); err != nil {
		return asset{}, false, err
	}
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return asset{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "ppt-v2-artifact:"+input.GenerationJobID); err != nil {
		return asset{}, false, err
	}
	existing, found, err := findPPTV2DurableArtifactRow(ctx, tx, input.TenantID, input.UserID, input.GenerationJobID, true)
	if err != nil {
		return asset{}, false, err
	}
	if found {
		if err := validateExistingPPTV2DurableArtifact(existing, input); err != nil {
			return asset{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return asset{}, false, err
		}
		return existing, false, nil
	}
	assetID, err := nextTableID(ctx, tx, "xz_assets", "asset")
	if err != nil {
		return asset{}, false, err
	}
	created := pptV2DurableWorkCenterAsset(input, assetID, time.Now().UTC().Format(time.RFC3339Nano))
	if err := insertAsset(ctx, tx, created); err != nil {
		return asset{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return asset{}, false, err
	}
	return created, true, nil
}

// EnsurePPTV2DurableArtifactFenced creates/reuses the Work Center asset and
// commits ASSET_CREATED in one transaction. A cancelled job, expired lease or
// stale fencing token therefore cannot make a visible artifact effective.
func (s *postgresStore) EnsurePPTV2DurableArtifactFenced(ctx context.Context, input pptV2DurableArtifactInput, lease pptapp.GenerationLease) (asset, bool, error) {
	if err := validatePPTV2DurableArtifactInput(input); err != nil {
		return asset{}, false, err
	}
	if input.GenerationJobID != lease.JobID || input.TenantID != lease.TenantID || input.UserID != lease.UserID {
		return asset{}, false, pptapp.ErrGenerationJobLeaseLost
	}
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return asset{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var status, stage, leaseOwner string
	var leaseExpiresAt time.Time
	var fencingToken int64
	err = tx.QueryRowContext(ctx, `select status,stage,coalesce(lease_owner,''),lease_expires_at,fencing_token from xz_ppt_v2_generation_jobs where id=$1 and tenant_id=$2 and user_id=$3 for update`, lease.JobID, lease.TenantID, lease.UserID).Scan(&status, &stage, &leaseOwner, &leaseExpiresAt, &fencingToken)
	if errors.Is(err, sql.ErrNoRows) {
		return asset{}, false, pptapp.ErrGenerationJobNotFound
	}
	if err != nil {
		return asset{}, false, err
	}
	if status == pptapp.GenerationJobCancelled {
		return asset{}, false, pptapp.ErrGenerationJobCancelled
	}
	if status != pptapp.GenerationJobRunning || stage != pptapp.GenerationStageFileStored || leaseOwner != lease.WorkerID || fencingToken != lease.FencingToken || !leaseExpiresAt.After(time.Now().UTC()) {
		return asset{}, false, pptapp.ErrGenerationJobLeaseLost
	}
	existing, found, err := findPPTV2DurableArtifactRow(ctx, tx, input.TenantID, input.UserID, input.GenerationJobID, true)
	if err != nil {
		return asset{}, false, err
	}
	createdNow := false
	if found {
		if err := validateExistingPPTV2DurableArtifact(existing, input); err != nil {
			return asset{}, false, err
		}
	} else {
		assetID, err := nextTableID(ctx, tx, "xz_assets", "asset")
		if err != nil {
			return asset{}, false, err
		}
		existing = pptV2DurableWorkCenterAsset(input, assetID, time.Now().UTC().Format(time.RFC3339Nano))
		if err := insertAsset(ctx, tx, existing); err != nil {
			return asset{}, false, err
		}
		createdNow = true
	}
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `update xz_ppt_v2_generation_jobs set stage=$2,completed_work_units=4,asset_id=$3,updated_at=$4 where id=$1 and status='RUNNING' and stage='FILE_STORED' and lease_owner=$5 and fencing_token=$6 and lease_expires_at>$4`, lease.JobID, pptapp.GenerationStageAssetCreated, existing.ID, now, lease.WorkerID, lease.FencingToken)
	if err != nil {
		return asset{}, false, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return asset{}, false, pptapp.ErrGenerationJobLeaseLost
	}
	checkpoint, _ := json.Marshal(map[string]any{"completedWorkUnits": 4, "assetId": existing.ID})
	if _, err := tx.ExecContext(ctx, `insert into xz_ppt_v2_generation_transitions(generation_job_id,attempt_id,from_stage,to_stage,fencing_token,checkpoint,created_at) values($1,$2,$3,$4,$5,$6::jsonb,$7)`, lease.JobID, lease.AttemptID, pptapp.GenerationStageFileStored, pptapp.GenerationStageAssetCreated, lease.FencingToken, string(checkpoint), now); err != nil {
		return asset{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return asset{}, false, err
	}
	return existing, createdNow, nil
}

type pptV2DurableArtifactRow interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func findPPTV2DurableArtifactRow(ctx context.Context, queryer pptV2DurableArtifactRow, tenantID, userID, generationJobID string, lock bool) (asset, bool, error) {
	query := `select raw from xz_assets where tenant_id=$1 and user_id=$2 and metadata->>'pptV2GenerationJobId'=$3 and deleted_at is null`
	if lock {
		query += ` for update`
	}
	var raw []byte
	err := queryer.QueryRowContext(ctx, query, strings.TrimSpace(tenantID), strings.TrimSpace(userID), strings.TrimSpace(generationJobID)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return asset{}, false, nil
	}
	if err != nil {
		return asset{}, false, err
	}
	var found asset
	if err := json.Unmarshal(raw, &found); err != nil {
		return asset{}, false, fmt.Errorf("decode ppt v2 durable artifact: %w", err)
	}
	return found, true, nil
}

func (s *postgresStore) FindPPTV2DurableArtifact(ctx context.Context, tenantID, userID, generationJobID string) (asset, bool, error) {
	if err := s.ensureReady(ctx); err != nil {
		return asset{}, false, err
	}
	return findPPTV2DurableArtifactRow(ctx, s.db, tenantID, userID, generationJobID, false)
}

var _ pptV2DurableArtifactStore = (*jsonStore)(nil)
var _ pptV2DurableArtifactStore = (*postgresStore)(nil)
var _ pptV2FencedDurableArtifactStore = (*postgresStore)(nil)
