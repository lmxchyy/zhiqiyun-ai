package knowledgerepo

import (
	"context"
	"database/sql"
	"errors"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func (p *Postgres) ListKnowledgeTags(ctx context.Context, access knowledgeapp.AccessContext) ([]knowledgeapp.Tag, error) {
	rows, err := p.db.QueryContext(ctx, `select id,tenant_id,name,color from xz_knowledge_tags where tenant_id=$1 order by name,id`, access.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []knowledgeapp.Tag{}
	for rows.Next() {
		var item knowledgeapp.Tag
		if err := rows.Scan(&item.ID, &item.TenantID, &item.Name, &item.Color); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) SaveKnowledgeTag(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Tag) (knowledgeapp.Tag, error) {
	err := p.db.QueryRowContext(ctx, `insert into xz_knowledge_tags(id,tenant_id,name,color) values($1,$2,$3,$4) on conflict(tenant_id,name) do update set color=excluded.color returning id,tenant_id,name,color`, item.ID, access.TenantID, item.Name, item.Color).Scan(&item.ID, &item.TenantID, &item.Name, &item.Color)
	return item, err
}

func (p *Postgres) ReplaceKnowledgeBaseTags(ctx context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string, tagIDs []string) ([]knowledgeapp.Tag, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from xz_knowledge_bases where tenant_id=$1 and id=$2 and deleted_at is null)`, access.TenantID, knowledgeBaseID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, knowledgeapp.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `delete from xz_knowledge_base_tags where tenant_id=$1 and knowledge_base_id=$2`, access.TenantID, knowledgeBaseID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, tagID := range tagIDs {
		if seen[tagID] {
			continue
		}
		seen[tagID] = true
		result, err := tx.ExecContext(ctx, `insert into xz_knowledge_base_tags(tenant_id,knowledge_base_id,tag_id) select $1,$2,id from xz_knowledge_tags where tenant_id=$1 and id=$3`, access.TenantID, knowledgeBaseID, tagID)
		if err != nil {
			return nil, err
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return nil, knowledgeapp.ErrNotFound
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p.listTagsForBase(ctx, access, knowledgeBaseID)
}

func (p *Postgres) listTagsForBase(ctx context.Context, access knowledgeapp.AccessContext, knowledgeBaseID string) ([]knowledgeapp.Tag, error) {
	rows, err := p.db.QueryContext(ctx, `select t.id,t.tenant_id,t.name,t.color from xz_knowledge_base_tags bt join xz_knowledge_tags t on t.tenant_id=bt.tenant_id and t.id=bt.tag_id where bt.tenant_id=$1 and bt.knowledge_base_id=$2 order by t.name,t.id`, access.TenantID, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []knowledgeapp.Tag{}
	for rows.Next() {
		var item knowledgeapp.Tag
		if err := rows.Scan(&item.ID, &item.TenantID, &item.Name, &item.Color); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) attachKnowledgeBaseTags(ctx context.Context, access knowledgeapp.AccessContext, items []knowledgeapp.KnowledgeBase) error {
	for index := range items {
		tags, err := p.listTagsForBase(ctx, access, items[index].ID)
		if err != nil {
			return err
		}
		items[index].Tags = tags
	}
	return nil
}

func (p *Postgres) ListKnowledgeCategories(ctx context.Context, access knowledgeapp.AccessContext) ([]knowledgeapp.Category, error) {
	rows, err := p.db.QueryContext(ctx, `select id,tenant_id,coalesce(parent_id,''),name,sort_order,created_at,updated_at from xz_knowledge_categories where tenant_id=$1 order by sort_order,name,id`, access.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []knowledgeapp.Category{}
	for rows.Next() {
		var item knowledgeapp.Category
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ParentID, &item.Name, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Postgres) SaveKnowledgeCategory(ctx context.Context, access knowledgeapp.AccessContext, item knowledgeapp.Category) (knowledgeapp.Category, error) {
	err := p.db.QueryRowContext(ctx, `insert into xz_knowledge_categories(id,tenant_id,parent_id,name,sort_order,created_at,updated_at) values($1,$2,$3,$4,$5,$6,$7) on conflict(id) do update set parent_id=excluded.parent_id,name=excluded.name,sort_order=excluded.sort_order,updated_at=excluded.updated_at returning id,tenant_id,coalesce(parent_id,''),name,sort_order,created_at,updated_at`, item.ID, access.TenantID, nullableText(item.ParentID), item.Name, item.SortOrder, item.CreatedAt, item.UpdatedAt).Scan(&item.ID, &item.TenantID, &item.ParentID, &item.Name, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return knowledgeapp.Category{}, knowledgeapp.ErrNotFound
	}
	return item, err
}
