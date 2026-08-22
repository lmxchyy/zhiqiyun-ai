package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

type rowScanner interface {
	Scan(...any) error
}

func scanConfig(row rowScanner) (Config, error) {
	var item Config
	var lastTestAt sql.NullTime
	err := row.Scan(
		&item.ID, &item.TenantID, &item.Name, &item.Purpose, &item.ObjectPrefix, &item.Provider, &item.Endpoint, &item.SigningEndpoint, &item.Region, &item.Bucket,
		&item.AccessKeyEncrypted, &item.SecretKeyEncrypted, &item.SessionTokenEncrypted,
		&item.PublicDomain, &item.CDNDomain, &item.UseSSL, &item.ForcePathStyle, &item.IsDefault, &item.IsSystem,
		&item.Status, &item.LastTestStatus, &item.LastTestMessage, &lastTestAt,
		&item.CreatedBy, &item.UpdatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, ErrConfigNotFound
	}
	if err != nil {
		return Config{}, err
	}
	if lastTestAt.Valid {
		item.LastTestAt = &lastTestAt.Time
	}
	item.HasAccessKey = item.AccessKeyEncrypted != ""
	item.HasSecretKey = item.SecretKeyEncrypted != ""
	return item, nil
}

const configColumns = `
  id, tenant_id, name, coalesce(purpose,''), coalesce(object_prefix,''), provider, endpoint, coalesce(signing_endpoint,''), coalesce(region,''), bucket,
  coalesce(access_key_encrypted,''), coalesce(secret_key_encrypted,''), coalesce(session_token_encrypted,''),
  coalesce(public_domain,''), coalesce(cdn_domain,''), use_ssl, force_path_style, is_default, is_system,
  status, coalesce(last_test_status,''), coalesce(last_test_message,''), last_test_at,
  coalesce(created_by,''), coalesce(updated_by,''), created_at, updated_at`

func (r *PostgresRepository) ListConfigs(ctx context.Context, tenantID string, includePlatform bool) ([]Config, error) {
	rows, err := r.db.QueryContext(ctx, `select `+configColumns+` from xz_storage_configs
    where deleted_at is null and (tenant_id=$1 or ($2 and tenant_id='platform'))
    order by is_default desc, updated_at desc`, tenantID, includePlatform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Config{}
	for rows.Next() {
		item, scanErr := scanConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) GetConfig(ctx context.Context, id string) (Config, error) {
	return scanConfig(r.db.QueryRowContext(ctx, `select `+configColumns+` from xz_storage_configs where id=$1 and deleted_at is null`, id))
}

func (r *PostgresRepository) SaveConfig(ctx context.Context, item Config) (Config, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Config{}, err
	}
	defer tx.Rollback()
	if item.IsDefault {
		if _, err = tx.ExecContext(ctx, `update xz_storage_configs set is_default=false, updated_at=now() where tenant_id=$1 and id<>$2 and deleted_at is null`, item.TenantID, item.ID); err != nil {
			return Config{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `
    insert into xz_storage_configs (
      id,tenant_id,name,purpose,object_prefix,provider,endpoint,signing_endpoint,region,bucket,access_key_encrypted,secret_key_encrypted,session_token_encrypted,
      public_domain,cdn_domain,use_ssl,force_path_style,is_default,is_system,status,created_by,updated_by
    ) values ($1,$2,$3,nullif($4,''),nullif($5,''),$6,$7,nullif($8,''),nullif($9,''),$10,nullif($11,''),nullif($12,''),nullif($13,''),nullif($14,''),$15,$16,$17,$18,$19,nullif($20,''),nullif($21,''),nullif($22,''))
    on conflict (id) do update set
      name=excluded.name, purpose=excluded.purpose, object_prefix=excluded.object_prefix, provider=excluded.provider, endpoint=excluded.endpoint, signing_endpoint=excluded.signing_endpoint, region=excluded.region, bucket=excluded.bucket,
      access_key_encrypted=coalesce(excluded.access_key_encrypted,xz_storage_configs.access_key_encrypted),
      secret_key_encrypted=coalesce(excluded.secret_key_encrypted,xz_storage_configs.secret_key_encrypted),
      session_token_encrypted=coalesce(excluded.session_token_encrypted,xz_storage_configs.session_token_encrypted),
      public_domain=excluded.public_domain, cdn_domain=excluded.cdn_domain, use_ssl=excluded.use_ssl,
      force_path_style=excluded.force_path_style, is_default=excluded.is_default, status=excluded.status,
      updated_by=excluded.updated_by, updated_at=now(), deleted_at=null
	`, item.ID, item.TenantID, item.Name, item.Purpose, item.ObjectPrefix, item.Provider, item.Endpoint, item.SigningEndpoint, item.Region, item.Bucket,
		item.AccessKeyEncrypted, item.SecretKeyEncrypted, item.SessionTokenEncrypted, item.PublicDomain, item.CDNDomain,
		item.UseSSL, item.ForcePathStyle, item.IsDefault, item.IsSystem, item.Status, item.CreatedBy, item.UpdatedBy)
	if err != nil {
		return Config{}, err
	}
	if err = tx.Commit(); err != nil {
		return Config{}, err
	}
	return r.GetConfig(ctx, item.ID)
}

func (r *PostgresRepository) DeleteConfig(ctx context.Context, id string) error {
	var used bool
	if err := r.db.QueryRowContext(ctx, `select exists(select 1 from xz_file_objects where storage_config_id=$1 and status<>'DELETED')`, id).Scan(&used); err != nil {
		return err
	}
	if used {
		return fmt.Errorf("%w: storage config is still referenced", ErrDeleteFailed)
	}
	result, err := r.db.ExecContext(ctx, `update xz_storage_configs set deleted_at=now(), is_default=false, updated_at=now() where id=$1 and deleted_at is null`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrConfigNotFound
	}
	return nil
}

func (r *PostgresRepository) UpdateConfigTest(ctx context.Context, id string, status string, message string, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `update xz_storage_configs set last_test_status=$2,last_test_message=$3,last_test_at=$4,updated_at=now() where id=$1 and deleted_at is null`, id, status, message, at)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrConfigNotFound
	}
	return nil
}

func (r *PostgresRepository) CreatePending(ctx context.Context, file FileObject, defaultQuota int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `insert into xz_tenant_storage_quotas(tenant_id,quota_bytes) values($1,$2) on conflict(tenant_id) do nothing`, file.TenantID, defaultQuota); err != nil {
		return err
	}
	var quota, used, reserved int64
	if err = tx.QueryRowContext(ctx, `select quota_bytes,used_bytes,reserved_bytes from xz_tenant_storage_quotas where tenant_id=$1 for update`, file.TenantID).Scan(&quota, &used, &reserved); err != nil {
		return err
	}
	if quota > 0 && used+reserved+file.ReservedSize > quota {
		return ErrQuotaExceeded
	}
	metadata, _ := json.Marshal(file.Metadata)
	_, err = tx.ExecContext(ctx, `
    insert into xz_file_objects (
      file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name,
      extension,mime_type,file_size,reserved_size,business_type,business_id,visibility,status,is_temporary,expires_at,metadata
    ) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,nullif($10,''),nullif($11,''),$12,$13,$14,nullif($15,''),$16,$17,$18,$19,$20::jsonb)
  `, file.FileID, file.TenantID, file.UserID, file.StorageConfigID, file.Provider, file.Bucket, file.ObjectKey,
		file.OriginalName, file.StoredName, file.Extension, file.MIMEType, file.FileSize, file.ReservedSize,
		file.BusinessType, file.BusinessID, file.Visibility, file.Status, file.IsTemporary, file.ExpiresAt, string(metadata))
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `update xz_tenant_storage_quotas set reserved_bytes=reserved_bytes+$2,updated_at=now() where tenant_id=$1`, file.TenantID, file.ReservedSize); err != nil {
		return err
	}
	return tx.Commit()
}

const fileColumns = `
  file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name,
  coalesce(extension,''),coalesce(mime_type,''),file_size,reserved_size,coalesce(file_hash,''),coalesce(hash_algorithm,''),coalesce(etag,''),
  business_type,coalesce(business_id,''),visibility,status,is_temporary,expires_at,recycle_expires_at,reference_count,
  metadata,created_at,updated_at,deleted_at`

func scanFile(row rowScanner) (FileObject, error) {
	var file FileObject
	var expiresAt, recycleExpiresAt, deletedAt sql.NullTime
	var metadata []byte
	err := row.Scan(
		&file.FileID, &file.TenantID, &file.UserID, &file.StorageConfigID, &file.Provider, &file.Bucket, &file.ObjectKey,
		&file.OriginalName, &file.StoredName, &file.Extension, &file.MIMEType, &file.FileSize, &file.ReservedSize,
		&file.FileHash, &file.HashAlgorithm, &file.ETag, &file.BusinessType, &file.BusinessID, &file.Visibility, &file.Status,
		&file.IsTemporary, &expiresAt, &recycleExpiresAt, &file.ReferenceCount, &metadata, &file.CreatedAt, &file.UpdatedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return FileObject{}, ErrFileNotFound
	}
	if err != nil {
		return FileObject{}, err
	}
	if expiresAt.Valid {
		file.ExpiresAt = &expiresAt.Time
	}
	if recycleExpiresAt.Valid {
		file.RecycleExpiresAt = &recycleExpiresAt.Time
	}
	if deletedAt.Valid {
		file.DeletedAt = &deletedAt.Time
	}
	file.Metadata = map[string]any{}
	_ = json.Unmarshal(metadata, &file.Metadata)
	return file, nil
}

func (r *PostgresRepository) CompleteUpload(ctx context.Context, tenantID string, fileID string, object ObjectMetadata) (FileObject, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return FileObject{}, err
	}
	defer tx.Rollback()
	file, err := scanFile(tx.QueryRowContext(ctx, `select `+fileColumns+` from xz_file_objects where tenant_id=$1 and file_id=$2 for update`, tenantID, fileID))
	if err != nil {
		return FileObject{}, err
	}
	if file.Status == StatusActive {
		return file, tx.Commit()
	}
	if file.Status != StatusPendingUpload {
		return FileObject{}, ErrUploadConfirmFailed
	}
	var quota, used, reserved int64
	if err = tx.QueryRowContext(ctx, `select quota_bytes,used_bytes,reserved_bytes from xz_tenant_storage_quotas where tenant_id=$1 for update`, tenantID).Scan(&quota, &used, &reserved); err != nil {
		return FileObject{}, err
	}
	projected := used + reserved - file.ReservedSize + object.Size
	if quota > 0 && projected > quota {
		return FileObject{}, ErrQuotaExceeded
	}
	metadata, _ := json.Marshal(stringMapToAny(object.Metadata))
	if _, err = tx.ExecContext(ctx, `update xz_file_objects set file_size=$3,reserved_size=0,etag=$4,mime_type=coalesce(nullif($5,''),mime_type),metadata=$6::jsonb,status='ACTIVE',updated_at=now() where tenant_id=$1 and file_id=$2`, tenantID, fileID, object.Size, object.ETag, object.ContentType, string(metadata)); err != nil {
		return FileObject{}, err
	}
	if _, err = tx.ExecContext(ctx, `update xz_tenant_storage_quotas set reserved_bytes=greatest(0,reserved_bytes-$2),used_bytes=used_bytes+$3,file_count=file_count+1,updated_at=now() where tenant_id=$1`, tenantID, file.ReservedSize, object.Size); err != nil {
		return FileObject{}, err
	}
	if err = tx.Commit(); err != nil {
		return FileObject{}, err
	}
	return r.GetFile(ctx, tenantID, fileID)
}

func (r *PostgresRepository) MarkUploadFailed(ctx context.Context, tenantID string, fileID string, reason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var reserved int64
	err = tx.QueryRowContext(ctx, `select reserved_size from xz_file_objects where tenant_id=$1 and file_id=$2 and status='PENDING_UPLOAD' for update`, tenantID, fileID).Scan(&reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `update xz_file_objects set reserved_size=0,status='UPLOAD_FAILED',metadata=jsonb_set(metadata,'{uploadError}',to_jsonb($3::text),true),updated_at=now() where tenant_id=$1 and file_id=$2`, tenantID, fileID, reason); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `update xz_tenant_storage_quotas set reserved_bytes=greatest(0,reserved_bytes-$2),updated_at=now() where tenant_id=$1`, tenantID, reserved); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) GetFile(ctx context.Context, tenantID string, fileID string) (FileObject, error) {
	return scanFile(r.db.QueryRowContext(ctx, `select `+fileColumns+` from xz_file_objects where file_id=$1 and ($2='' or tenant_id=$2)`, fileID, tenantID))
}

func (r *PostgresRepository) ListFiles(ctx context.Context, filter FileFilter) ([]FileObject, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := "%" + strings.ToLower(strings.TrimSpace(filter.Query)) + "%"
	args := []any{filter.TenantID, filter.UserID, strings.ToUpper(filter.Status), strings.ToLower(filter.BusinessType), strings.ToLower(filter.Provider), query}
	where := `($1='' or tenant_id=$1) and ($2='' or user_id=$2) and ($3='' or status=$3) and ($4='' or lower(business_type)=$4) and ($5='' or lower(provider)=$5) and ($6='%%' or lower(file_id||' '||original_name) like $6)`
	var total int64
	if err := r.db.QueryRowContext(ctx, `select count(*) from xz_file_objects where `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `select `+fileColumns+` from xz_file_objects where `+where+` order by created_at desc limit $7 offset $8`, append(args, limit, maxInt(filter.Offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []FileObject{}
	for rows.Next() {
		file, scanErr := scanFile(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, file)
	}
	return items, total, rows.Err()
}

func (r *PostgresRepository) MarkDeletePending(ctx context.Context, tenantID string, fileID string, recycleExpiresAt time.Time) (FileObject, error) {
	result, err := r.db.ExecContext(ctx, `update xz_file_objects set status='DELETE_PENDING',deleted_at=now(),recycle_expires_at=$3,updated_at=now() where tenant_id=$1 and file_id=$2 and status='ACTIVE' and reference_count=0`, tenantID, fileID, recycleExpiresAt)
	if err != nil {
		return FileObject{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		file, getErr := r.GetFile(ctx, tenantID, fileID)
		if getErr != nil {
			return FileObject{}, getErr
		}
		if file.Status != StatusDeletePending {
			return FileObject{}, ErrDeleteFailed
		}
	}
	return r.GetFile(ctx, tenantID, fileID)
}

func (r *PostgresRepository) RestoreFile(ctx context.Context, tenantID string, fileID string) (FileObject, error) {
	result, err := r.db.ExecContext(ctx, `update xz_file_objects set status='ACTIVE',deleted_at=null,recycle_expires_at=null,updated_at=now() where tenant_id=$1 and file_id=$2 and status='DELETE_PENDING'`, tenantID, fileID)
	if err != nil {
		return FileObject{}, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return FileObject{}, ErrDeleteFailed
	}
	return r.GetFile(ctx, tenantID, fileID)
}

func (r *PostgresRepository) MarkDeleted(ctx context.Context, tenantID string, fileID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var size int64
	var status string
	if err = tx.QueryRowContext(ctx, `select file_size,status from xz_file_objects where tenant_id=$1 and file_id=$2 for update`, tenantID, fileID).Scan(&size, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrFileNotFound
	} else if err != nil {
		return err
	}
	if status == StatusDeleted {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `update xz_file_objects set status='DELETED',updated_at=now() where tenant_id=$1 and file_id=$2`, tenantID, fileID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `update xz_tenant_storage_quotas set used_bytes=greatest(0,used_bytes-$2),file_count=greatest(0,file_count-1),updated_at=now() where tenant_id=$1`, tenantID, size); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) GetQuota(ctx context.Context, tenantID string, defaultQuota int64) (Quota, error) {
	if _, err := r.db.ExecContext(ctx, `insert into xz_tenant_storage_quotas(tenant_id,quota_bytes) values($1,$2) on conflict(tenant_id) do nothing`, tenantID, defaultQuota); err != nil {
		return Quota{}, err
	}
	var quota Quota
	err := r.db.QueryRowContext(ctx, `select tenant_id,quota_bytes,used_bytes,reserved_bytes,file_count,warning_percent,critical_percent,updated_at from xz_tenant_storage_quotas where tenant_id=$1`, tenantID).Scan(&quota.TenantID, &quota.QuotaBytes, &quota.UsedBytes, &quota.ReservedBytes, &quota.FileCount, &quota.WarningPercent, &quota.CriticalPercent, &quota.UpdatedAt)
	return quota, err
}

func (r *PostgresRepository) UpdateQuota(ctx context.Context, quota Quota) (Quota, error) {
	_, err := r.db.ExecContext(ctx, `insert into xz_tenant_storage_quotas(tenant_id,quota_bytes,warning_percent,critical_percent) values($1,$2,$3,$4) on conflict(tenant_id) do update set quota_bytes=excluded.quota_bytes,warning_percent=excluded.warning_percent,critical_percent=excluded.critical_percent,updated_at=now()`, quota.TenantID, quota.QuotaBytes, quota.WarningPercent, quota.CriticalPercent)
	if err != nil {
		return Quota{}, err
	}
	return r.GetQuota(ctx, quota.TenantID, quota.QuotaBytes)
}

func (r *PostgresRepository) Overview(ctx context.Context, tenantID string) (Overview, error) {
	overview := Overview{ProviderBytes: map[string]int64{}}
	err := r.db.QueryRowContext(ctx, `select
    count(*) filter(where status='ACTIVE'),coalesce(sum(file_size) filter(where status='ACTIVE'),0),
    count(*) filter(where status='PENDING_UPLOAD'),count(*) filter(where status='DELETE_PENDING'),
    count(*) filter(where status in ('UPLOAD_FAILED','PROCESSING_FAILED','QUARANTINED','MIGRATION_FAILED')),
    coalesce(sum(file_size) filter(where status='ACTIVE' and is_temporary),0)
    from xz_file_objects where ($1='' or tenant_id=$1)`, tenantID).Scan(&overview.TotalFiles, &overview.TotalBytes, &overview.PendingFiles, &overview.RecycleFiles, &overview.AbnormalFiles, &overview.TemporaryBytes)
	if err != nil {
		return Overview{}, err
	}
	rows, err := r.db.QueryContext(ctx, `select provider,coalesce(sum(file_size),0) from xz_file_objects where status='ACTIVE' and ($1='' or tenant_id=$1) group by provider`, tenantID)
	if err != nil {
		return Overview{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		var bytes int64
		if err = rows.Scan(&provider, &bytes); err != nil {
			return Overview{}, err
		}
		overview.ProviderBytes[provider] = bytes
	}
	if tenantID != "" {
		overview.Quota, err = r.GetQuota(ctx, tenantID, 0)
	}
	return overview, err
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
