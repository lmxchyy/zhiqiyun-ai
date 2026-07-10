package knowledgerepo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) ResolveAccessContext(ctx context.Context, userID string, requestedTenantID string, requestedOrganizationID string) (knowledgeapp.AccessContext, error) {
	if p == nil || p.db == nil {
		return knowledgeapp.AccessContext{}, errors.New("knowledge postgres repository requires database")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return knowledgeapp.AccessContext{}, knowledgeapp.ErrValidation
	}
	var userRole, userName string
	if err := p.db.QueryRowContext(ctx, `select coalesce(role, ''), coalesce(name, '') from xz_users where id = $1 and status = 'ACTIVE'`, userID).Scan(&userRole, &userName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return knowledgeapp.AccessContext{}, knowledgeapp.ErrForbidden
		}
		return knowledgeapp.AccessContext{}, err
	}

	tenantID := strings.TrimSpace(requestedTenantID)
	role := ""
	if strings.EqualFold(userRole, "SUPER_ADMIN") {
		role = "PLATFORM_ADMIN"
		if tenantID != "" {
			var exists bool
			if err := p.db.QueryRowContext(ctx, `select exists(select 1 from xz_tenants where id = $1 and status = 'ACTIVE')`, tenantID).Scan(&exists); err != nil {
				return knowledgeapp.AccessContext{}, err
			}
			if !exists {
				return knowledgeapp.AccessContext{}, knowledgeapp.ErrNotFound
			}
		}
	}

	if tenantID != "" && role == "" {
		if err := p.db.QueryRowContext(ctx, `
			select role
			from xz_tenant_members
			where tenant_id = $1 and user_id = $2 and status = 'ACTIVE'
		`, tenantID, userID).Scan(&role); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return knowledgeapp.AccessContext{}, knowledgeapp.ErrForbidden
			}
			return knowledgeapp.AccessContext{}, err
		}
	}

	if tenantID == "" {
		err := p.db.QueryRowContext(ctx, `
			select m.tenant_id, m.role
			from xz_tenant_members m
			join xz_tenants t on t.id = m.tenant_id
			where m.user_id = $1 and m.status = 'ACTIVE' and t.status = 'ACTIVE'
			order by case t.tenant_type when 'ENTERPRISE' then 0 when 'PERSONAL' then 1 else 2 end, m.created_at
			limit 1
		`, userID).Scan(&tenantID, &role)
		if errors.Is(err, sql.ErrNoRows) {
			tenantID, role, err = p.ensurePersonalTenant(ctx, userID, userName)
		}
		if err != nil {
			return knowledgeapp.AccessContext{}, err
		}
	}
	if strings.EqualFold(userRole, "SUPER_ADMIN") {
		role = "PLATFORM_ADMIN"
	}
	if role == "" {
		role = "PLATFORM_ADMIN"
	}

	organizationID := strings.TrimSpace(requestedOrganizationID)
	if organizationID != "" {
		var allowed bool
		if role == "PLATFORM_ADMIN" || role == "ENTERPRISE_ADMIN" {
			err := p.db.QueryRowContext(ctx, `select exists(select 1 from xz_organizations where tenant_id = $1 and id = $2 and status = 'ACTIVE')`, tenantID, organizationID).Scan(&allowed)
			if err != nil {
				return knowledgeapp.AccessContext{}, err
			}
		} else {
			err := p.db.QueryRowContext(ctx, `
				select exists(
					select 1
					from xz_organization_members om
					join xz_tenant_members tm on tm.id = om.tenant_member_id and tm.tenant_id = om.tenant_id
					where om.tenant_id = $1 and om.organization_id = $2 and tm.user_id = $3
					  and om.status = 'ACTIVE' and tm.status = 'ACTIVE'
				)
			`, tenantID, organizationID, userID).Scan(&allowed)
			if err != nil {
				return knowledgeapp.AccessContext{}, err
			}
		}
		if !allowed {
			return knowledgeapp.AccessContext{}, knowledgeapp.ErrForbidden
		}
	}

	return knowledgeapp.AccessContext{
		TenantID:       tenantID,
		OrganizationID: organizationID,
		UserID:         userID,
		Roles:          []string{role},
		Permissions:    permissionsForTenantRole(role),
	}, nil
}

func (p *Postgres) ensurePersonalTenant(ctx context.Context, userID string, userName string) (string, string, error) {
	digest := sha256.Sum256([]byte(userID))
	suffix := hex.EncodeToString(digest[:8])
	tenantID := "tenant_personal_" + suffix
	memberID := "tenant_member_" + suffix
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	if strings.TrimSpace(userName) == "" {
		userName = "个人空间"
	} else {
		userName += "的个人空间"
	}
	if _, err := tx.ExecContext(ctx, `
		insert into xz_tenants (id, tenant_type, owner_user_id, name, status)
		values ($1, 'PERSONAL', $2, $3, 'ACTIVE')
		on conflict (id) do update set updated_at = now()
	`, tenantID, userID, userName); err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into xz_tenant_members (id, tenant_id, user_id, role, status)
		values ($1, $2, $3, 'MEMBER', 'ACTIVE')
		on conflict (tenant_id, user_id) do update set status = 'ACTIVE', updated_at = now()
	`, memberID, tenantID, userID); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return tenantID, "MEMBER", nil
}

func permissionsForTenantRole(role string) []string {
	switch role {
	case "PLATFORM_ADMIN", "ENTERPRISE_ADMIN":
		return []string{"knowledge.view", "knowledge.upload", "knowledge.edit", "knowledge.delete", "knowledge.share", "knowledge.manage", "knowledge.agent.bind", "knowledge.config.manage", "knowledge.logs.read"}
	case "GUEST":
		return []string{"knowledge.view"}
	default:
		return []string{"knowledge.view"}
	}
}

func (p *Postgres) CreateKnowledgeBase(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.KnowledgeBase) (knowledgeapp.KnowledgeBase, error) {
	row := p.db.QueryRowContext(ctx, `
		insert into xz_knowledge_bases (
			id, tenant_id, organization_id, owner_user_id, category_id, knowledge_type, name, description,
			logo_object_key, visibility, status, ingestion_profile_id, retrieval_profile_id, metadata, version, created_at, updated_at
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16,$17)
		returning id, tenant_id, coalesce(organization_id, ''), owner_user_id, coalesce(category_id, ''), knowledge_type,
			name, description, coalesce(logo_object_key, ''), visibility, status, document_count, chunk_count,
			coalesce(ingestion_profile_id, ''), coalesce(retrieval_profile_id, ''), metadata, version, created_at, updated_at, deleted_at
	`, item.ID, access.TenantID, nullableText(item.OrganizationID), item.OwnerUserID, nullableText(item.CategoryID), item.KnowledgeType,
		item.Name, item.Description, nullableText(item.LogoObjectKey), item.Visibility, item.Status, nullableText(item.IngestionProfileID),
		nullableText(item.RetrievalProfileID), jsonText(item.Metadata), item.Version, item.CreatedAt, item.UpdatedAt)
	return scanKnowledgeBase(row)
}

func (p *Postgres) ListKnowledgeBases(ctx context.Context, access knowledgeapp.AccessContext, options knowledgeapp.ListOptions) ([]knowledgeapp.KnowledgeBase, string, error) {
	admin := access.HasRole("PLATFORM_ADMIN", "ENTERPRISE_ADMIN")
	rows, err := p.db.QueryContext(ctx, `
		select id, tenant_id, coalesce(organization_id, ''), owner_user_id, coalesce(category_id, ''), knowledge_type,
			name, description, coalesce(logo_object_key, ''), visibility, status, document_count, chunk_count,
			coalesce(ingestion_profile_id, ''), coalesce(retrieval_profile_id, ''), metadata, version, created_at, updated_at, deleted_at
		from xz_knowledge_bases
		where tenant_id = $1 and deleted_at is null
		  and ($2 = '' or status = $2)
		  and ($3 = '' or name ilike '%' || $3 || '%' or description ilike '%' || $3 || '%')
		  and (
			$4 = true or owner_user_id = $5 or (
			  not exists (
				select 1 from xz_knowledge_base_acl acl
				where acl.tenant_id=xz_knowledge_bases.tenant_id and acl.knowledge_base_id=xz_knowledge_bases.id
				  and acl.effect='DENY' and acl.permission in ('VIEW','READ','MANAGE')
				  and (acl.expires_at is null or acl.expires_at>now())
				  and (
					(acl.subject_type='USER' and acl.subject_id=$5)
					or (acl.subject_type in ('ORGANIZATION','DEPARTMENT') and acl.subject_id=nullif($6,''))
					or (acl.subject_type='TENANT' and (acl.subject_id is null or acl.subject_id=$1))
					or (acl.subject_type='ROLE' and acl.subject_id=any($7::text[]))
					or acl.subject_type in ('EVERYONE','GUEST')
				  )
			  )
			  and (visibility in ('TENANT', 'SHARED')
			  or (visibility = 'ORGANIZATION' and organization_id = nullif($6, ''))
			  or exists (
				select 1 from xz_knowledge_base_acl acl
				where acl.tenant_id=xz_knowledge_bases.tenant_id and acl.knowledge_base_id=xz_knowledge_bases.id
				  and acl.effect='ALLOW' and acl.permission in ('VIEW','READ','MANAGE')
				  and (acl.expires_at is null or acl.expires_at>now())
				  and (
					(acl.subject_type='USER' and acl.subject_id=$5)
					or (acl.subject_type in ('ORGANIZATION','DEPARTMENT') and acl.subject_id=nullif($6,''))
					or (acl.subject_type='TENANT' and (acl.subject_id is null or acl.subject_id=$1))
					or (acl.subject_type='ROLE' and acl.subject_id=any($7::text[]))
					or acl.subject_type in ('EVERYONE','GUEST')
				  )
			  )
			  )
			)
		  )
		order by updated_at desc, id desc
		limit $8
	`, access.TenantID, strings.ToUpper(strings.TrimSpace(options.Status)), strings.TrimSpace(options.Query), admin, access.UserID, access.OrganizationID, access.Roles, options.Limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]knowledgeapp.KnowledgeBase, 0)
	for rows.Next() {
		item, err := scanKnowledgeBase(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if err := p.attachKnowledgeBaseTags(ctx, access, items); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) == options.Limit && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

func (p *Postgres) GetKnowledgeBase(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.KnowledgeBase, error) {
	row := p.db.QueryRowContext(ctx, `
		select id, tenant_id, coalesce(organization_id, ''), owner_user_id, coalesce(category_id, ''), knowledge_type,
			name, description, coalesce(logo_object_key, ''), visibility, status, document_count, chunk_count,
			coalesce(ingestion_profile_id, ''), coalesce(retrieval_profile_id, ''), metadata, version, created_at, updated_at, deleted_at
		from xz_knowledge_bases
		where tenant_id = $1 and id = $2 and deleted_at is null
	`, access.TenantID, id)
	item, err := scanKnowledgeBase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrNotFound
	}
	if err == nil {
		item.Tags, err = p.listTagsForBase(ctx, access, item.ID)
	}
	return item, err
}

func (p *Postgres) UpdateKnowledgeBase(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.KnowledgeBase, expectedVersion int64) (knowledgeapp.KnowledgeBase, error) {
	row := p.db.QueryRowContext(ctx, `
		update xz_knowledge_bases set
			organization_id=$3, category_id=$4, name=$5, description=$6, logo_object_key=$7, visibility=$8, status=$9,
			ingestion_profile_id=$10, retrieval_profile_id=$11, metadata=$12::jsonb, version=version+1, updated_at=$13
		where tenant_id=$1 and id=$2 and deleted_at is null and ($14 = 0 or version = $14)
		returning id, tenant_id, coalesce(organization_id, ''), owner_user_id, coalesce(category_id, ''), knowledge_type,
			name, description, coalesce(logo_object_key, ''), visibility, status, document_count, chunk_count,
			coalesce(ingestion_profile_id, ''), coalesce(retrieval_profile_id, ''), metadata, version, created_at, updated_at, deleted_at
	`, access.TenantID, item.ID, nullableText(item.OrganizationID), nullableText(item.CategoryID), item.Name, item.Description,
		nullableText(item.LogoObjectKey), item.Visibility, item.Status, nullableText(item.IngestionProfileID), nullableText(item.RetrievalProfileID),
		jsonText(item.Metadata), item.UpdatedAt, expectedVersion)
	updated, err := scanKnowledgeBase(row)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedVersion > 0 {
			return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrConflict
		}
		return knowledgeapp.KnowledgeBase{}, knowledgeapp.ErrNotFound
	}
	return updated, err
}

func (p *Postgres) SoftDeleteKnowledgeBase(ctx context.Context, access knowledgeapp.AccessContext, id string) error {
	result, err := p.db.ExecContext(ctx, `
		update xz_knowledge_bases
		set status='DELETING', deleted_at=now(), updated_at=now(), version=version+1
		where tenant_id=$1 and id=$2 and deleted_at is null
	`, access.TenantID, id)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (p *Postgres) ReplaceKnowledgeBaseACL(ctx context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string, rules []knowledgeapp.ACLRule) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from xz_knowledge_base_acl where tenant_id=$1 and knowledge_base_id=$2`, access.TenantID, knowledgeBaseID); err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.TenantID != "" && rule.TenantID != access.TenantID {
			return knowledgeapp.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `
			insert into xz_knowledge_base_acl (id, tenant_id, knowledge_base_id, subject_type, subject_id, permission, effect, expires_at)
			values ($1,$2,$3,$4,$5,$6,$7,$8)
		`, rule.ID, access.TenantID, knowledgeBaseID, rule.SubjectType, nullableText(rule.SubjectID), rule.Permission, rule.Effect, rule.ExpiresAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) ListKnowledgeBaseACL(ctx context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string) ([]knowledgeapp.ACLRule, error) {
	rows, err := p.db.QueryContext(ctx, `
		select id, tenant_id, knowledge_base_id, subject_type, coalesce(subject_id, ''), permission, effect, expires_at
		from xz_knowledge_base_acl where tenant_id=$1 and knowledge_base_id=$2 order by created_at, id
	`, access.TenantID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []knowledgeapp.ACLRule{}
	for rows.Next() {
		var item knowledgeapp.ACLRule
		var expires sql.NullTime
		if err := rows.Scan(&item.ID, &item.TenantID, &item.KnowledgeBaseID, &item.SubjectType, &item.SubjectID, &item.Permission, &item.Effect, &expires); err != nil {
			return nil, err
		}
		if expires.Valid {
			item.ExpiresAt = &expires.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) CreateDocumentBundle(ctx context.Context, access knowledgeapp.AccessContext, document knowledgeapp.Document, version knowledgeapp.DocumentVersion, units []knowledgeapp.DocumentUnit, chunks []knowledgeapp.Chunk, job knowledgeapp.IngestionJob) (knowledgeapp.Document, knowledgeapp.IngestionJob, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		insert into xz_knowledge_documents (id, tenant_id, knowledge_base_id, source_id, owner_user_id, latest_version_id, name, document_type, mime_type, status, metadata, version, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14)
	`, document.ID, access.TenantID, document.KnowledgeBaseID, nullableText(document.SourceID), document.OwnerUserID, version.ID, document.Name,
		document.DocumentType, document.MIMEType, document.Status, jsonText(document.Metadata), document.Version, document.CreatedAt, document.UpdatedAt); err != nil {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into xz_knowledge_document_versions (id, tenant_id, document_id, version_no, original_object_key, preview_object_key, mime_type, file_size, content_hash, parse_status, parser_metadata, created_by, created_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13)
	`, version.ID, access.TenantID, document.ID, version.VersionNo, nullableText(version.OriginalObjectKey), nullableText(version.PreviewObjectKey),
		version.MIMEType, version.FileSize, version.ContentHash, version.ParseStatus, jsonText(version.ParserMetadata), version.CreatedBy, version.CreatedAt); err != nil {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
	}
	for _, unit := range units {
		if _, err := tx.ExecContext(ctx, `
			insert into xz_knowledge_document_units (id, tenant_id, document_version_id, unit_type, unit_no, title, content, locator, metadata)
			values ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb)
		`, unit.ID, access.TenantID, version.ID, unit.UnitType, unit.UnitNo, unit.Title, unit.Content, jsonText(unit.Locator), jsonText(unit.Metadata)); err != nil {
			return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
		}
	}
	for _, chunk := range chunks {
		if err := insertChunk(ctx, tx, access.TenantID, chunk); err != nil {
			return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		insert into xz_knowledge_ingestion_jobs (id, tenant_id, document_version_id, ingestion_profile_id, idempotency_key, stage, status, attempt, max_attempts, progress, config_snapshot, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13)
	`, job.ID, access.TenantID, version.ID, nullableText(job.IngestionProfileID), job.IdempotencyKey, job.Stage, job.Status, job.Attempt,
		job.MaxAttempts, job.Progress, jsonText(job.ConfigSnapshot), job.CreatedAt, job.UpdatedAt); err != nil {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		update xz_knowledge_bases set document_count=document_count+1, chunk_count=chunk_count+$3, updated_at=now()
		where tenant_id=$1 and id=$2 and deleted_at is null
	`, access.TenantID, document.KnowledgeBaseID, len(chunks)); err != nil {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return knowledgeapp.Document{}, knowledgeapp.IngestionJob{}, err
	}
	document.LatestVersionID = version.ID
	return document, job, nil
}

func (p *Postgres) ListDocuments(ctx context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string, options knowledgeapp.ListOptions) ([]knowledgeapp.Document, string, error) {
	rows, err := p.db.QueryContext(ctx, `
		select id, tenant_id, knowledge_base_id, coalesce(source_id, ''), owner_user_id, coalesce(latest_version_id, ''), name,
			document_type, mime_type, status, metadata, version, created_at, updated_at, deleted_at
		from xz_knowledge_documents
		where tenant_id=$1 and knowledge_base_id=$2 and deleted_at is null
		  and ($3='' or status=$3) and ($4='' or name ilike '%' || $4 || '%')
		order by updated_at desc, id desc limit $5
	`, access.TenantID, knowledgeBaseID, strings.ToUpper(strings.TrimSpace(options.Status)), strings.TrimSpace(options.Query), options.Limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []knowledgeapp.Document{}
	for rows.Next() {
		item, err := scanDocument(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) == options.Limit && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, nil
}

func (p *Postgres) GetDocument(ctx context.Context, access knowledgeapp.AccessContext, id string) (knowledgeapp.Document, error) {
	item, err := scanDocument(p.db.QueryRowContext(ctx, `
		select id, tenant_id, knowledge_base_id, coalesce(source_id, ''), owner_user_id, coalesce(latest_version_id, ''), name,
			document_type, mime_type, status, metadata, version, created_at, updated_at, deleted_at
		from xz_knowledge_documents where tenant_id=$1 and id=$2 and deleted_at is null
	`, access.TenantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.Document{}, knowledgeapp.ErrNotFound
	}
	return item, err
}

func (p *Postgres) SoftDeleteDocument(ctx context.Context, access knowledgeapp.AccessContext, id string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var knowledgeBaseID string
	if err := tx.QueryRowContext(ctx, `
		select knowledge_base_id from xz_knowledge_documents
		where tenant_id=$1 and id=$2 and deleted_at is null for update
	`, access.TenantID, id).Scan(&knowledgeBaseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return knowledgeapp.ErrNotFound
		}
		return err
	}
	var chunkCount int64
	if err := tx.QueryRowContext(ctx, `select count(*) from xz_knowledge_chunks where tenant_id=$1 and document_id=$2 and deleted_at is null`, access.TenantID, id).Scan(&chunkCount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update xz_knowledge_documents set status='DELETED', deleted_at=now(), updated_at=now(), version=version+1 where tenant_id=$1 and id=$2`, access.TenantID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update xz_knowledge_chunks set status='DELETED', deleted_at=now(), updated_at=now() where tenant_id=$1 and document_id=$2 and deleted_at is null`, access.TenantID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		update xz_knowledge_bases set document_count=greatest(document_count-1,0), chunk_count=greatest(chunk_count-$3,0), updated_at=now()
		where tenant_id=$1 and id=$2 and deleted_at is null
	`, access.TenantID, knowledgeBaseID, chunkCount); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) ListChunks(ctx context.Context, access knowledgeapp.AccessContext, knowledgeBaseIDs []string, options knowledgeapp.ListOptions) ([]knowledgeapp.Chunk, error) {
	args := []any{access.TenantID}
	where := "tenant_id=$1 and deleted_at is null"
	if len(knowledgeBaseIDs) > 0 {
		placeholders := make([]string, 0, len(knowledgeBaseIDs))
		for _, id := range knowledgeBaseIDs {
			args = append(args, id)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		where += " and knowledge_base_id in (" + strings.Join(placeholders, ",") + ")"
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 200
	}
	args = append(args, limit)
	query := `
		select id, tenant_id, knowledge_base_id, document_id, document_version_id, sequence_no, chunk_key, content,
			token_count, page_start, page_end, title, title_path, source_locator, content_hash, metadata, status, created_at, updated_at, deleted_at
		from xz_knowledge_chunks where ` + where + fmt.Sprintf(" order by document_id, sequence_no limit $%d", len(args))
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []knowledgeapp.Chunk{}
	for rows.Next() {
		item, err := scanChunk(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) ReplaceChunks(ctx context.Context, access knowledgeapp.AccessContext, documentVersionID string, chunks []knowledgeapp.Chunk) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from xz_knowledge_chunks where tenant_id=$1 and document_version_id=$2`, access.TenantID, documentVersionID); err != nil {
		return err
	}
	for _, chunk := range chunks {
		if chunk.TenantID != access.TenantID || chunk.DocumentVersionID != documentVersionID {
			return knowledgeapp.ErrForbidden
		}
		if err := insertChunk(ctx, tx, access.TenantID, chunk); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Postgres) UpdateIngestionJob(ctx context.Context, access knowledgeapp.AccessContext, job knowledgeapp.IngestionJob) error {
	result, err := p.db.ExecContext(ctx, `
		update xz_knowledge_ingestion_jobs set stage=$3, status=$4, attempt=$5, progress=$6, config_snapshot=$7::jsonb,
			error_code=$8, error_message=$9, updated_at=$10
		where tenant_id=$1 and id=$2
	`, access.TenantID, job.ID, job.Stage, job.Status, job.Attempt, job.Progress, jsonText(job.ConfigSnapshot), nullableText(job.ErrorCode), nullableText(job.ErrorMessage), job.UpdatedAt)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (p *Postgres) UpdateDocumentStatus(ctx context.Context, access knowledgeapp.AccessContext, documentID string, status string) error {
	result, err := p.db.ExecContext(ctx, `
		update xz_knowledge_documents set status=$3, updated_at=now(), version=version+1
		where tenant_id=$1 and id=$2 and deleted_at is null
	`, access.TenantID, documentID, status)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

type scanner interface {
	Scan(...any) error
}

func scanKnowledgeBase(row scanner) (knowledgeapp.KnowledgeBase, error) {
	var item knowledgeapp.KnowledgeBase
	var metadata []byte
	var deleted sql.NullTime
	err := row.Scan(&item.ID, &item.TenantID, &item.OrganizationID, &item.OwnerUserID, &item.CategoryID, &item.KnowledgeType,
		&item.Name, &item.Description, &item.LogoObjectKey, &item.Visibility, &item.Status, &item.DocumentCount, &item.ChunkCount,
		&item.IngestionProfileID, &item.RetrievalProfileID, &metadata, &item.Version, &item.CreatedAt, &item.UpdatedAt, &deleted)
	if err != nil {
		return knowledgeapp.KnowledgeBase{}, err
	}
	_ = json.Unmarshal(metadata, &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if deleted.Valid {
		item.DeletedAt = &deleted.Time
	}
	return item, nil
}

func scanDocument(row scanner) (knowledgeapp.Document, error) {
	var item knowledgeapp.Document
	var metadata []byte
	var deleted sql.NullTime
	err := row.Scan(&item.ID, &item.TenantID, &item.KnowledgeBaseID, &item.SourceID, &item.OwnerUserID, &item.LatestVersionID,
		&item.Name, &item.DocumentType, &item.MIMEType, &item.Status, &metadata, &item.Version, &item.CreatedAt, &item.UpdatedAt, &deleted)
	if err != nil {
		return knowledgeapp.Document{}, err
	}
	_ = json.Unmarshal(metadata, &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if deleted.Valid {
		item.DeletedAt = &deleted.Time
	}
	return item, nil
}

func scanChunk(row scanner) (knowledgeapp.Chunk, error) {
	var item knowledgeapp.Chunk
	var pageStart, pageEnd sql.NullInt64
	var titlePath, locator, metadata []byte
	var deleted sql.NullTime
	err := row.Scan(&item.ID, &item.TenantID, &item.KnowledgeBaseID, &item.DocumentID, &item.DocumentVersionID, &item.SequenceNo,
		&item.ChunkKey, &item.Content, &item.TokenCount, &pageStart, &pageEnd, &item.Title, &titlePath, &locator, &item.ContentHash,
		&metadata, &item.Status, &item.CreatedAt, &item.UpdatedAt, &deleted)
	if err != nil {
		return knowledgeapp.Chunk{}, err
	}
	if pageStart.Valid {
		value := int(pageStart.Int64)
		item.PageStart = &value
	}
	if pageEnd.Valid {
		value := int(pageEnd.Int64)
		item.PageEnd = &value
	}
	_ = json.Unmarshal(titlePath, &item.TitlePath)
	_ = json.Unmarshal(locator, &item.SourceLocator)
	_ = json.Unmarshal(metadata, &item.Metadata)
	if item.SourceLocator == nil {
		item.SourceLocator = map[string]any{}
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	if deleted.Valid {
		item.DeletedAt = &deleted.Time
	}
	return item, nil
}

func insertChunk(ctx context.Context, tx *sql.Tx, tenantID string, chunk knowledgeapp.Chunk) error {
	_, err := tx.ExecContext(ctx, `
		insert into xz_knowledge_chunks (id, tenant_id, knowledge_base_id, document_id, document_version_id, sequence_no, chunk_key,
			content, token_count, page_start, page_end, title, title_path, source_locator, content_hash, metadata, status, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15,$16::jsonb,$17,$18,$19)
	`, chunk.ID, tenantID, chunk.KnowledgeBaseID, chunk.DocumentID, chunk.DocumentVersionID, chunk.SequenceNo, chunk.ChunkKey,
		chunk.Content, chunk.TokenCount, chunk.PageStart, chunk.PageEnd, chunk.Title, jsonText(chunk.TitlePath), jsonText(chunk.SourceLocator),
		chunk.ContentHash, jsonText(chunk.Metadata), chunk.Status, chunk.CreatedAt, chunk.UpdatedAt)
	return err
}

func jsonText(value any) string {
	if value == nil {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func requireAffected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return knowledgeapp.ErrNotFound
	}
	return nil
}

var _ knowledgeapp.TenantRepository = (*Postgres)(nil)
var _ knowledgeapp.KnowledgeRepository = (*Postgres)(nil)
