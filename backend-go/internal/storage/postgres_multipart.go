package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (r *PostgresRepository) CreateMultipartSession(ctx context.Context, session MultipartUploadRecord) error {
	_, err := r.db.ExecContext(ctx, `insert into xz_multipart_uploads
		(id,tenant_id,owner_user_id,file_id,provider_upload_id,object_key,file_name,content_type,
		 total_size,part_size,total_parts,state,idempotency_key,expires_at,created_at,completed_at)
		values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,nullif($13,''),$14,$15,$16)`,
		session.ID, session.TenantID, session.OwnerUserID, session.FileID, session.ProviderUploadID,
		session.ObjectKey, session.FileName, session.ContentType, session.TotalSize, session.PartSize,
		session.TotalParts, session.State, session.IdempotencyKey, session.ExpiresAt, session.CreatedAt, session.CompletedAt)
	return err
}

func (r *PostgresRepository) GetMultipartSession(ctx context.Context, tenantID, userID, uploadID string) (MultipartUploadRecord, error) {
	session, err := scanMultipartSession(r.db.QueryRowContext(ctx, `select id,tenant_id,owner_user_id,file_id,provider_upload_id,object_key,
		file_name,content_type,total_size,part_size,total_parts,state,coalesce(idempotency_key,''),expires_at,created_at,completed_at
		from xz_multipart_uploads where id=$1 and tenant_id=$2 and owner_user_id=$3`, uploadID, tenantID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return MultipartUploadRecord{}, ErrMultipartNotFound
	}
	if err != nil {
		return MultipartUploadRecord{}, err
	}
	parts, err := r.listMultipartParts(ctx, session.ID)
	if err != nil {
		return MultipartUploadRecord{}, err
	}
	session.Parts = parts
	return session, nil
}

func (r *PostgresRepository) GetMultipartSessionByIdempotency(ctx context.Context, tenantID, userID, key string) (MultipartUploadRecord, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return MultipartUploadRecord{}, ErrMultipartNotFound
	}
	session, err := scanMultipartSession(r.db.QueryRowContext(ctx, `select id,tenant_id,owner_user_id,file_id,provider_upload_id,object_key,
		file_name,content_type,total_size,part_size,total_parts,state,coalesce(idempotency_key,''),expires_at,created_at,completed_at
		from xz_multipart_uploads where tenant_id=$1 and owner_user_id=$2 and idempotency_key=$3
		order by created_at desc limit 1`, tenantID, userID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return MultipartUploadRecord{}, ErrMultipartNotFound
	}
	if err != nil {
		return MultipartUploadRecord{}, err
	}
	parts, err := r.listMultipartParts(ctx, session.ID)
	if err != nil {
		return MultipartUploadRecord{}, err
	}
	session.Parts = parts
	return session, nil
}

func (r *PostgresRepository) SaveMultipartPart(ctx context.Context, uploadID string, part CompletedPart) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `insert into xz_multipart_upload_parts(upload_id,part_number,etag,size_bytes,completed_at)
		values($1,$2,$3,$4,now())
		on conflict(upload_id,part_number) do update set etag=excluded.etag,size_bytes=excluded.size_bytes,completed_at=now()`,
		uploadID, part.PartNumber, part.ETag, part.SizeBytes)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `update xz_multipart_uploads set state=case when state='initialized' then 'uploading' else state end
		where id=$1`, uploadID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PostgresRepository) UpdateMultipartState(ctx context.Context, tenantID, uploadID, state string, completedAt *time.Time) error {
	result, err := r.db.ExecContext(ctx, `update xz_multipart_uploads set state=$3,completed_at=coalesce($4,completed_at)
		where id=$1 and ($2='' or tenant_id=$2)`, uploadID, tenantID, state, completedAt)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrMultipartNotFound
	}
	return nil
}

func (r *PostgresRepository) listMultipartParts(ctx context.Context, uploadID string) (map[int]CompletedPart, error) {
	rows, err := r.db.QueryContext(ctx, `select part_number,etag,size_bytes from xz_multipart_upload_parts where upload_id=$1`, uploadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	parts := map[int]CompletedPart{}
	for rows.Next() {
		var part CompletedPart
		if err := rows.Scan(&part.PartNumber, &part.ETag, &part.SizeBytes); err != nil {
			return nil, err
		}
		parts[part.PartNumber] = part
	}
	return parts, rows.Err()
}

func scanMultipartSession(row rowScanner) (MultipartUploadRecord, error) {
	var session MultipartUploadRecord
	var completedAt sql.NullTime
	err := row.Scan(
		&session.ID, &session.TenantID, &session.OwnerUserID, &session.FileID, &session.ProviderUploadID, &session.ObjectKey,
		&session.FileName, &session.ContentType, &session.TotalSize, &session.PartSize, &session.TotalParts, &session.State,
		&session.IdempotencyKey, &session.ExpiresAt, &session.CreatedAt, &completedAt,
	)
	if completedAt.Valid {
		session.CompletedAt = &completedAt.Time
	}
	return session, err
}
