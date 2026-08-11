package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	errMediaNotFound = errors.New("media resource not found")
	errMediaInUse    = errors.New("media asset is currently in use")
)

type mediaAsset struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenantId"`
	Name            string         `json:"name"`
	CategoryID      string         `json:"categoryId,omitempty"`
	CategoryName    string         `json:"categoryName,omitempty"`
	AssetType       string         `json:"assetType"`
	MimeType        string         `json:"mimeType"`
	FileExt         string         `json:"fileExt"`
	OriginalName    string         `json:"originalName"`
	StorageProvider string         `json:"storageProvider"`
	StorageBucket   string         `json:"storageBucket,omitempty"`
	StorageKey      string         `json:"-"`
	OriginalURL     string         `json:"originalUrl,omitempty"`
	CDNURL          string         `json:"cdnUrl,omitempty"`
	ThumbnailURL    string         `json:"thumbnailUrl,omitempty"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	AspectRatio     float64        `json:"aspectRatio"`
	FileSize        int64          `json:"fileSize"`
	FileHash        string         `json:"fileHash"`
	Status          string         `json:"status"`
	AuditStatus     string         `json:"auditStatus"`
	IsDefault       bool           `json:"isDefault"`
	UsageCount      int            `json:"usageCount"`
	SourceType      string         `json:"sourceType"`
	SourceName      string         `json:"sourceName,omitempty"`
	LicenseType     string         `json:"licenseType,omitempty"`
	LicenseNote     string         `json:"licenseNote,omitempty"`
	Prompt          string         `json:"prompt,omitempty"`
	ModelName       string         `json:"modelName,omitempty"`
	CopyrightOwner  string         `json:"copyrightOwner,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedBy       string         `json:"createdBy,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedBy       string         `json:"updatedBy,omitempty"`
	UpdatedAt       string         `json:"updatedAt"`
}

type mediaCategory struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	ParentID  string `json:"parentId,omitempty"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	SortOrder int    `json:"sortOrder"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type pageAssetSlot struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenantId"`
	PageCode           string         `json:"pageCode"`
	ModuleCode         string         `json:"moduleCode"`
	SlotKey            string         `json:"slotKey"`
	SlotName           string         `json:"slotName"`
	AssetID            string         `json:"assetId,omitempty"`
	FallbackAssetID    string         `json:"fallbackAssetId,omitempty"`
	MaterialURL        string         `json:"materialUrl,omitempty"`
	FallbackURL        string         `json:"fallbackUrl,omitempty"`
	AltText            string         `json:"altText,omitempty"`
	SortOrder          int            `json:"sortOrder"`
	IsEnabled          bool           `json:"isEnabled"`
	EffectiveStartTime string         `json:"effectiveStartTime,omitempty"`
	EffectiveEndTime   string         `json:"effectiveEndTime,omitempty"`
	ExtraConfig        map[string]any `json:"extraConfig,omitempty"`
	UpdatedAt          string         `json:"updatedAt,omitempty"`
}

type pageConfig struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenantId"`
	PageCode    string         `json:"pageCode"`
	Version     int            `json:"version"`
	ConfigJSON  map[string]any `json:"config"`
	Status      string         `json:"status"`
	PublishedAt string         `json:"publishedAt,omitempty"`
	CreatedBy   string         `json:"createdBy,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedBy   string         `json:"updatedBy,omitempty"`
	UpdatedAt   string         `json:"updatedAt"`
}

type pageConfigVersion struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenantId"`
	PageConfigID string         `json:"pageConfigId"`
	Version      int            `json:"version"`
	ConfigJSON   map[string]any `json:"config"`
	ChangeNote   string         `json:"changeNote,omitempty"`
	CreatedBy    string         `json:"createdBy,omitempty"`
	CreatedAt    string         `json:"createdAt"`
}

type mediaAssetUsage struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenantId"`
	AssetID      string `json:"assetId"`
	PageCode     string `json:"pageCode"`
	ModuleCode   string `json:"moduleCode"`
	SlotKey      string `json:"slotKey"`
	BusinessType string `json:"businessType"`
	BusinessID   string `json:"businessId,omitempty"`
	CreatedAt    string `json:"createdAt"`
}

type mediaAssetFilter struct {
	Query      string
	CategoryID string
	Status     string
	Limit      int
	Offset     int
}

type mediaRepository interface {
	ListAssets(context.Context, string, mediaAssetFilter) ([]mediaAsset, int, error)
	GetAsset(context.Context, string, string) (mediaAsset, error)
	FindAssetByHash(context.Context, string, string) (mediaAsset, bool, error)
	SaveAsset(context.Context, mediaAsset) (mediaAsset, error)
	UpdateAsset(context.Context, string, string, map[string]any) (mediaAsset, error)
	DeleteAsset(context.Context, string, string) error
	ListAssetUsages(context.Context, string, string) ([]mediaAssetUsage, error)
	ListCategories(context.Context, string, bool) ([]mediaCategory, error)
	SaveCategory(context.Context, mediaCategory) (mediaCategory, error)
	DeleteCategory(context.Context, string, string) error
	ListSlots(context.Context, string, string, bool) ([]pageAssetSlot, error)
	SaveSlot(context.Context, pageAssetSlot) (pageAssetSlot, error)
	GetPageConfig(context.Context, string, string) (pageConfig, error)
	SavePageDraft(context.Context, pageConfig) (pageConfig, error)
	PublishPage(context.Context, string, string, string, string) (pageConfig, error)
	ListPageVersions(context.Context, string, string) ([]pageConfigVersion, error)
	RollbackPage(context.Context, string, string, int, string) (pageConfig, error)
}

type memoryMediaRepository struct {
	mu         sync.RWMutex
	assets     map[string]mediaAsset
	categories map[string]mediaCategory
	slots      map[string]pageAssetSlot
	configs    map[string]pageConfig
	versions   map[string][]pageConfigVersion
}

func newMemoryMediaRepository() *memoryMediaRepository {
	now := time.Now().UTC().Format(time.RFC3339)
	r := &memoryMediaRepository{assets: map[string]mediaAsset{}, categories: map[string]mediaCategory{}, slots: map[string]pageAssetSlot{}, configs: map[string]pageConfig{}, versions: map[string][]pageConfigVersion{}}
	for index, item := range defaultMediaCategories() {
		item.CreatedAt, item.UpdatedAt, item.SortOrder = now, now, (index+1)*10
		r.categories[mediaScope(item.TenantID, item.ID)] = item
	}
	for _, item := range defaultPageAssetSlots() {
		item.UpdatedAt = now
		r.slots[mediaScope(item.TenantID, item.PageCode, item.SlotKey)] = item
	}
	for _, code := range []string{"home", "studio", "assets", "profile"} {
		r.configs[mediaScope("default", code)] = pageConfig{ID: "page_default_" + code, TenantID: "default", PageCode: code, ConfigJSON: map[string]any{"modules": []any{}}, Status: "DRAFT", CreatedAt: now, UpdatedAt: now}
	}
	return r
}

func mediaScope(parts ...string) string { return strings.Join(parts, "\x00") }

func defaultMediaCategories() []mediaCategory {
	names := []string{"首页 Hero", "首页快捷入口", "AI 创作能力", "AI 员工头像", "创作模板", "作品封面", "用户头像", "Banner", "Logo", "默认占位图", "系统图标", "节日活动", "宣传素材"}
	codes := []string{"home-hero", "home-quick", "ai-capability", "ai-employee", "creation-template", "work-cover", "user-avatar", "banner", "logo", "default-placeholder", "system-icon", "festival", "promotion"}
	items := make([]mediaCategory, 0, len(names))
	for i := range names {
		items = append(items, mediaCategory{ID: "media_cat_" + strings.ReplaceAll(codes[i], "-", "_"), TenantID: "default", Name: names[i], Code: codes[i], Status: "ACTIVE"})
	}
	return items
}

func defaultPageAssetSlots() []pageAssetSlot {
	specs := [][5]string{
		{"home", "hero", "home.hero.background", "首页主视觉背景", "知启云AI 首页主视觉"}, {"home", "hero", "home.hero.illustration", "首页主视觉插画", "AI 创作插画"},
		{"home", "quick", "home.quick.poster", "快捷入口-海报", "AI 海报"}, {"home", "quick", "home.quick.ppt", "快捷入口-PPT", "AI PPT"}, {"home", "quick", "home.quick.video", "快捷入口-视频", "AI 视频"}, {"home", "quick", "home.quick.knowledge", "快捷入口-知识库", "企业知识库"},
		{"home", "capability", "home.capability.ai_design", "能力-AI设计", "AI 设计能力"}, {"home", "capability", "home.capability.ai_video", "能力-AI视频", "AI 视频能力"}, {"home", "capability", "home.capability.ppt", "能力-PPT", "AI PPT 能力"}, {"home", "capability", "home.capability.office", "能力-自由P图", "自由P图能力"}, {"home", "capability", "home.capability.knowledge", "能力-知识库", "知识库能力"}, {"home", "capability", "home.capability.employee", "能力-AI员工", "AI 员工能力"},
		{"home", "employee", "home.employee.designer", "AI设计师头像", "AI 设计师"}, {"home", "employee", "home.employee.sales", "AI销售头像", "AI 销售"}, {"home", "employee", "home.employee.operation", "AI运营头像", "AI 运营"}, {"home", "employee", "home.employee.service", "AI客服头像", "AI 客服"}, {"home", "employee", "home.employee.boss_assistant", "老板助手头像", "老板助手"},
		{"home", "inspiration", "home.inspiration.poster", "灵感-海报", "企业宣传海报"}, {"home", "inspiration", "home.inspiration.video", "灵感-视频", "短视频模板"}, {"home", "inspiration", "home.inspiration.ppt", "灵感-PPT", "招商 PPT"}, {"home", "inspiration", "home.inspiration.store", "灵感-门店", "门店营销"}, {"home", "inspiration", "home.inspiration.ecommerce", "灵感-电商", "电商主图"},
		{"studio", "banner", "studio.banner", "创作页 Banner", "创作中心"}, {"studio", "template", "studio.template.poster", "模板-海报", "海报模板"}, {"studio", "template", "studio.template.video", "模板-视频", "视频模板"}, {"studio", "template", "studio.template.ppt", "模板-PPT", "PPT 模板"}, {"studio", "template", "studio.template.office", "模板-办公", "办公模板"}, {"studio", "template", "studio.template.knowledge", "模板-知识库", "知识库模板"}, {"studio", "template", "studio.template.employee", "模板-AI员工", "AI 员工模板"},
		{"assets", "default", "assets.default.image", "默认图片封面", "图片作品"}, {"assets", "default", "assets.default.video", "默认视频封面", "视频作品"}, {"assets", "default", "assets.default.ppt", "默认PPT封面", "PPT 作品"}, {"assets", "default", "assets.default.document", "默认文档封面", "文档作品"}, {"assets", "default", "assets.default.other", "默认其他封面", "其他作品"},
		{"profile", "header", "profile.default_avatar", "默认用户头像", "用户头像"}, {"profile", "member", "profile.member_background", "会员卡背景", "会员卡背景"}, {"profile", "header", "profile.header_background", "我的页头图", "个人中心背景"},
	}
	items := make([]pageAssetSlot, 0, len(specs))
	for index, spec := range specs {
		items = append(items, pageAssetSlot{ID: "slot_" + strings.NewReplacer(".", "_", "-", "_").Replace(spec[2]), TenantID: "default", PageCode: spec[0], ModuleCode: spec[1], SlotKey: spec[2], SlotName: spec[3], AltText: spec[4], SortOrder: (index + 1) * 10, IsEnabled: true, ExtraConfig: map[string]any{}})
	}
	return items
}

func (r *memoryMediaRepository) ListAssets(_ context.Context, tenant string, filter mediaAssetFilter) ([]mediaAsset, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []mediaAsset{}
	for _, item := range r.assets {
		if item.TenantID != tenant || item.Status == "DELETED" {
			continue
		}
		if filter.Query != "" && !strings.Contains(strings.ToLower(item.Name), strings.ToLower(filter.Query)) {
			continue
		}
		if filter.CategoryID != "" && item.CategoryID != filter.CategoryID {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(item.Status, filter.Status) {
			continue
		}
		items = append(items, item)
	}
	total := len(items)
	start := filter.Offset
	if start > total {
		start = total
	}
	end := total
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return items[start:end], total, nil
}
func (r *memoryMediaRepository) GetAsset(_ context.Context, tenant, id string) (mediaAsset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.assets[mediaScope(tenant, id)]
	if !ok || item.Status == "DELETED" {
		return mediaAsset{}, errMediaNotFound
	}
	return item, nil
}
func (r *memoryMediaRepository) FindAssetByHash(_ context.Context, tenant, hash string) (mediaAsset, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.assets {
		if item.TenantID == tenant && item.FileHash == hash && item.Status != "DELETED" {
			return item, true, nil
		}
	}
	return mediaAsset{}, false, nil
}
func (r *memoryMediaRepository) SaveAsset(_ context.Context, item mediaAsset) (mediaAsset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	r.assets[mediaScope(item.TenantID, item.ID)] = item
	return item, nil
}
func (r *memoryMediaRepository) UpdateAsset(ctx context.Context, tenant, id string, patch map[string]any) (mediaAsset, error) {
	item, err := r.GetAsset(ctx, tenant, id)
	if err != nil {
		return item, err
	}
	if v, ok := patch["name"].(string); ok && strings.TrimSpace(v) != "" {
		item.Name = strings.TrimSpace(v)
	}
	if v, ok := patch["categoryId"].(string); ok {
		item.CategoryID = v
	}
	if v, ok := patch["status"].(string); ok {
		item.Status = strings.ToUpper(v)
	}
	if v, ok := patch["auditStatus"].(string); ok {
		item.AuditStatus = strings.ToUpper(v)
	}
	if v, ok := patch["isDefault"].(bool); ok {
		item.IsDefault = v
	}
	return r.SaveAsset(ctx, item)
}
func (r *memoryMediaRepository) DeleteAsset(ctx context.Context, tenant, id string) error {
	usages, _ := r.ListAssetUsages(ctx, tenant, id)
	if len(usages) > 0 {
		return errMediaInUse
	}
	item, err := r.GetAsset(ctx, tenant, id)
	if err != nil {
		return err
	}
	item.Status = "DELETED"
	_, err = r.SaveAsset(ctx, item)
	return err
}
func (r *memoryMediaRepository) ListAssetUsages(_ context.Context, tenant, id string) ([]mediaAssetUsage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []mediaAssetUsage{}
	for _, slot := range r.slots {
		if slot.TenantID == tenant && (slot.AssetID == id || slot.FallbackAssetID == id) {
			items = append(items, mediaAssetUsage{ID: "usage_" + slot.ID, TenantID: tenant, AssetID: id, PageCode: slot.PageCode, ModuleCode: slot.ModuleCode, SlotKey: slot.SlotKey, BusinessType: "PAGE_SLOT", BusinessID: slot.ID, CreatedAt: slot.UpdatedAt})
		}
	}
	return items, nil
}
func (r *memoryMediaRepository) ListCategories(_ context.Context, tenant string, inherit bool) ([]mediaCategory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := []mediaCategory{}
	for _, item := range r.categories {
		if item.TenantID == tenant || (inherit && item.TenantID == "default") {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *memoryMediaRepository) SaveCategory(_ context.Context, item mediaCategory) (mediaCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if item.ID == "" {
		item.ID = "media_cat_" + newRequestID()
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.Status == "" {
		item.Status = "ACTIVE"
	}
	r.categories[mediaScope(item.TenantID, item.ID)] = item
	return item, nil
}
func (r *memoryMediaRepository) DeleteCategory(_ context.Context, tenant, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaScope(tenant, id)
	if _, ok := r.categories[key]; !ok {
		return errMediaNotFound
	}
	for _, asset := range r.assets {
		if asset.TenantID == tenant && asset.CategoryID == id && asset.Status != "DELETED" {
			return errors.New("media category contains assets")
		}
	}
	delete(r.categories, key)
	return nil
}
func (r *memoryMediaRepository) ListSlots(_ context.Context, tenant, page string, inherit bool) ([]pageAssetSlot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	merged := map[string]pageAssetSlot{}
	if inherit && tenant != "default" {
		for _, item := range r.slots {
			if item.TenantID == "default" && item.PageCode == page {
				merged[item.SlotKey] = item
			}
		}
	}
	for _, item := range r.slots {
		if item.TenantID == tenant && item.PageCode == page {
			merged[item.SlotKey] = item
		}
	}
	items := make([]pageAssetSlot, 0, len(merged))
	for _, item := range merged {
		items = append(items, item)
	}
	sortPageSlots(items)
	return items, nil
}
func (r *memoryMediaRepository) SaveSlot(_ context.Context, item pageAssetSlot) (pageAssetSlot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == "" {
		item.ID = "slot_" + newRequestID()
	}
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if item.ExtraConfig == nil {
		item.ExtraConfig = map[string]any{}
	}
	r.slots[mediaScope(item.TenantID, item.PageCode, item.SlotKey)] = item
	return item, nil
}
func (r *memoryMediaRepository) GetPageConfig(_ context.Context, tenant, page string) (pageConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.configs[mediaScope(tenant, page)]
	if !ok {
		return pageConfig{}, errMediaNotFound
	}
	return item, nil
}
func (r *memoryMediaRepository) SavePageDraft(_ context.Context, item pageConfig) (pageConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaScope(item.TenantID, item.PageCode)
	current, ok := r.configs[key]
	now := time.Now().UTC().Format(time.RFC3339)
	if ok {
		item.ID = current.ID
		item.Version = current.Version
		item.CreatedAt = current.CreatedAt
	} else {
		item.ID = "page_" + newRequestID()
		item.CreatedAt = now
	}
	item.Status = "DRAFT"
	item.UpdatedAt = now
	r.configs[key] = item
	return item, nil
}
func (r *memoryMediaRepository) PublishPage(_ context.Context, tenant, page, note, user string) (pageConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaScope(tenant, page)
	item, ok := r.configs[key]
	if !ok {
		return pageConfig{}, errMediaNotFound
	}
	item.Version++
	item.Status = "PUBLISHED"
	item.PublishedAt = time.Now().UTC().Format(time.RFC3339)
	item.UpdatedAt = item.PublishedAt
	item.UpdatedBy = user
	r.configs[key] = item
	r.versions[key] = append(r.versions[key], pageConfigVersion{ID: "page_ver_" + newRequestID(), TenantID: tenant, PageConfigID: item.ID, Version: item.Version, ConfigJSON: item.ConfigJSON, ChangeNote: note, CreatedBy: user, CreatedAt: item.PublishedAt})
	return item, nil
}
func (r *memoryMediaRepository) ListPageVersions(_ context.Context, tenant, page string) ([]pageConfigVersion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]pageConfigVersion(nil), r.versions[mediaScope(tenant, page)]...), nil
}
func (r *memoryMediaRepository) RollbackPage(ctx context.Context, tenant, page string, version int, user string) (pageConfig, error) {
	r.mu.Lock()
	key := mediaScope(tenant, page)
	var source *pageConfigVersion
	for i := range r.versions[key] {
		if r.versions[key][i].Version == version {
			copy := r.versions[key][i]
			source = &copy
			break
		}
	}
	if source == nil {
		r.mu.Unlock()
		return pageConfig{}, errMediaNotFound
	}
	item := r.configs[key]
	item.ConfigJSON = source.ConfigJSON
	item.Status = "DRAFT"
	item.UpdatedBy = user
	item.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	r.configs[key] = item
	r.mu.Unlock()
	return r.PublishPage(ctx, tenant, page, fmt.Sprintf("rollback to version %d", version), user)
}

type postgresMediaRepository struct{ db *sql.DB }

func newPostgresMediaRepository(db *sql.DB) *postgresMediaRepository {
	return &postgresMediaRepository{db: db}
}

func (r *postgresMediaRepository) ListAssets(ctx context.Context, tenant string, filter mediaAssetFilter) ([]mediaAsset, int, error) {
	args := []any{tenant}
	where := "a.tenant_id=$1 and a.deleted_at is null"
	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		where += fmt.Sprintf(" and a.name ilike $%d", len(args))
	}
	if filter.CategoryID != "" {
		args = append(args, filter.CategoryID)
		where += fmt.Sprintf(" and a.category_id=$%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, strings.ToUpper(filter.Status))
		where += fmt.Sprintf(" and a.status=$%d", len(args))
	}
	var total int
	if err := r.db.QueryRowContext(ctx, "select count(*) from xz_media_assets a where "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, filter.Limit, filter.Offset)
	query := fmt.Sprintf(`select a.id,a.tenant_id,a.name,coalesce(a.category_id,''),coalesce(c.name,''),a.asset_type,a.mime_type,a.file_ext,a.original_name,a.storage_provider,coalesce(a.storage_bucket,''),a.storage_key,coalesce(a.original_url,''),coalesce(a.cdn_url,''),coalesce(a.thumbnail_url,''),a.width,a.height,a.aspect_ratio,a.file_size,a.file_hash,a.status,a.audit_status,a.is_default,a.usage_count,a.source_type,coalesce(a.source_name,''),coalesce(a.license_type,''),coalesce(a.license_note,''),coalesce(a.prompt,''),coalesce(a.model_name,''),coalesce(a.copyright_owner,''),a.metadata,coalesce(a.created_by,''),a.created_at,coalesce(a.updated_by,''),a.updated_at from xz_media_assets a left join xz_media_categories c on c.tenant_id=a.tenant_id and c.id=a.category_id where %s order by a.created_at desc limit $%d offset $%d`, where, len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []mediaAsset{}
	for rows.Next() {
		item, err := scanMediaAsset(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
func (r *postgresMediaRepository) GetAsset(ctx context.Context, tenant, id string) (mediaAsset, error) {
	row := r.db.QueryRowContext(ctx, `select a.id,a.tenant_id,a.name,coalesce(a.category_id,''),coalesce(c.name,''),a.asset_type,a.mime_type,a.file_ext,a.original_name,a.storage_provider,coalesce(a.storage_bucket,''),a.storage_key,coalesce(a.original_url,''),coalesce(a.cdn_url,''),coalesce(a.thumbnail_url,''),a.width,a.height,a.aspect_ratio,a.file_size,a.file_hash,a.status,a.audit_status,a.is_default,a.usage_count,a.source_type,coalesce(a.source_name,''),coalesce(a.license_type,''),coalesce(a.license_note,''),coalesce(a.prompt,''),coalesce(a.model_name,''),coalesce(a.copyright_owner,''),a.metadata,coalesce(a.created_by,''),a.created_at,coalesce(a.updated_by,''),a.updated_at from xz_media_assets a left join xz_media_categories c on c.tenant_id=a.tenant_id and c.id=a.category_id where a.tenant_id=$1 and a.id=$2 and a.deleted_at is null`, tenant, id)
	item, err := scanMediaAsset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errMediaNotFound
	}
	return item, err
}
func (r *postgresMediaRepository) FindAssetByHash(ctx context.Context, tenant, hash string) (mediaAsset, bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `select id from xz_media_assets where tenant_id=$1 and file_hash=$2 and deleted_at is null limit 1`, tenant, hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return mediaAsset{}, false, nil
	}
	if err != nil {
		return mediaAsset{}, false, err
	}
	item, err := r.GetAsset(ctx, tenant, id)
	return item, err == nil, err
}
func (r *postgresMediaRepository) SaveAsset(ctx context.Context, item mediaAsset) (mediaAsset, error) {
	_, err := r.db.ExecContext(ctx, `insert into xz_media_assets(id,tenant_id,name,category_id,asset_type,mime_type,file_ext,original_name,storage_provider,storage_bucket,storage_key,original_url,cdn_url,thumbnail_url,width,height,aspect_ratio,file_size,file_hash,status,audit_status,is_default,source_type,source_name,license_type,license_note,prompt,model_name,copyright_owner,metadata,created_by,updated_by) values($1,$2,$3,nullif($4,''),$5,$6,$7,$8,$9,nullif($10,''),$11,nullif($12,''),nullif($13,''),nullif($14,''),$15,$16,$17,$18,$19,$20,$21,$22,$23,nullif($24,''),nullif($25,''),nullif($26,''),nullif($27,''),nullif($28,''),nullif($29,''),$30::jsonb,nullif($31,''),nullif($32,'')) on conflict(tenant_id,file_hash) do update set name=excluded.name,category_id=coalesce(excluded.category_id,xz_media_assets.category_id),updated_by=excluded.updated_by,updated_at=now(),deleted_at=null`, item.ID, item.TenantID, item.Name, item.CategoryID, item.AssetType, item.MimeType, item.FileExt, item.OriginalName, item.StorageProvider, item.StorageBucket, item.StorageKey, item.OriginalURL, item.CDNURL, item.ThumbnailURL, item.Width, item.Height, item.AspectRatio, item.FileSize, item.FileHash, item.Status, item.AuditStatus, item.IsDefault, item.SourceType, item.SourceName, item.LicenseType, item.LicenseNote, item.Prompt, item.ModelName, item.CopyrightOwner, jsonBytes(item.Metadata), item.CreatedBy, item.UpdatedBy)
	if err != nil {
		return mediaAsset{}, err
	}
	found, _, err := r.FindAssetByHash(ctx, item.TenantID, item.FileHash)
	return found, err
}
func (r *postgresMediaRepository) UpdateAsset(ctx context.Context, tenant, id string, patch map[string]any) (mediaAsset, error) {
	item, err := r.GetAsset(ctx, tenant, id)
	if err != nil {
		return item, err
	}
	if v, ok := patch["name"].(string); ok && strings.TrimSpace(v) != "" {
		item.Name = strings.TrimSpace(v)
	}
	if v, ok := patch["categoryId"].(string); ok {
		item.CategoryID = v
	}
	if v, ok := patch["status"].(string); ok {
		item.Status = strings.ToUpper(v)
	}
	if v, ok := patch["auditStatus"].(string); ok {
		item.AuditStatus = strings.ToUpper(v)
	}
	if v, ok := patch["isDefault"].(bool); ok {
		item.IsDefault = v
	}
	_, err = r.db.ExecContext(ctx, `update xz_media_assets set name=$3,category_id=nullif($4,''),status=$5,audit_status=$6,is_default=$7,updated_at=now() where tenant_id=$1 and id=$2 and deleted_at is null`, tenant, id, item.Name, item.CategoryID, item.Status, item.AuditStatus, item.IsDefault)
	if err != nil {
		return item, err
	}
	return r.GetAsset(ctx, tenant, id)
}
func (r *postgresMediaRepository) DeleteAsset(ctx context.Context, tenant, id string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `select count(*) from xz_page_asset_slots where tenant_id=$1 and deleted_at is null and (asset_id=$2 or fallback_asset_id=$2)`, tenant, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errMediaInUse
	}
	res, err := r.db.ExecContext(ctx, `update xz_media_assets set status='DELETED',deleted_at=now(),updated_at=now() where tenant_id=$1 and id=$2 and deleted_at is null`, tenant, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errMediaNotFound
	}
	return nil
}
func (r *postgresMediaRepository) ListAssetUsages(ctx context.Context, tenant, id string) ([]mediaAssetUsage, error) {
	rows, err := r.db.QueryContext(ctx, `select id,tenant_id,asset_id,page_code,module_code,slot_key,business_type,coalesce(business_id,''),created_at from xz_media_asset_usage where tenant_id=$1 and asset_id=$2 order by created_at desc`, tenant, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []mediaAssetUsage{}
	for rows.Next() {
		var item mediaAssetUsage
		var created time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.AssetID, &item.PageCode, &item.ModuleCode, &item.SlotKey, &item.BusinessType, &item.BusinessID, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = created.UTC().Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *postgresMediaRepository) ListCategories(ctx context.Context, tenant string, inherit bool) ([]mediaCategory, error) {
	query := `select id,tenant_id,coalesce(parent_id,''),name,code,sort_order,status,created_at,updated_at from xz_media_categories where deleted_at is null and tenant_id=$1`
	args := []any{tenant}
	if inherit && tenant != "default" {
		query += ` or (deleted_at is null and tenant_id='default')`
	}
	query += ` order by tenant_id desc,sort_order,name`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []mediaCategory{}
	for rows.Next() {
		var item mediaCategory
		var created, updated time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ParentID, &item.Name, &item.Code, &item.SortOrder, &item.Status, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt = created.UTC().Format(time.RFC3339)
		item.UpdatedAt = updated.UTC().Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *postgresMediaRepository) SaveCategory(ctx context.Context, item mediaCategory) (mediaCategory, error) {
	if item.ID == "" {
		item.ID = "media_cat_" + newRequestID()
	}
	if item.Status == "" {
		item.Status = "ACTIVE"
	}
	_, err := r.db.ExecContext(ctx, `insert into xz_media_categories(id,tenant_id,parent_id,name,code,sort_order,status) values($1,$2,nullif($3,''),$4,$5,$6,$7) on conflict(tenant_id,id) do update set parent_id=excluded.parent_id,name=excluded.name,code=excluded.code,sort_order=excluded.sort_order,status=excluded.status,updated_at=now()`, item.ID, item.TenantID, item.ParentID, item.Name, item.Code, item.SortOrder, item.Status)
	if err != nil {
		return item, err
	}
	items, err := r.ListCategories(ctx, item.TenantID, false)
	if err == nil {
		for _, current := range items {
			if current.ID == item.ID {
				return current, nil
			}
		}
	}
	return item, err
}
func (r *postgresMediaRepository) DeleteCategory(ctx context.Context, tenant, id string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, `select count(*) from xz_media_assets where tenant_id=$1 and category_id=$2 and deleted_at is null`, tenant, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("media category contains assets")
	}
	res, err := r.db.ExecContext(ctx, `update xz_media_categories set deleted_at=now(),status='DELETED',updated_at=now() where tenant_id=$1 and id=$2 and deleted_at is null`, tenant, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errMediaNotFound
	}
	return nil
}
func pageSlotListQuery(tenant string, inherit bool) string {
	query := `select s.id,s.tenant_id,s.page_code,s.module_code,s.slot_key,s.slot_name,coalesce(s.asset_id,'') as asset_id,coalesce(s.fallback_asset_id,'') as fallback_asset_id,coalesce(nullif(a.cdn_url,''),nullif(a.original_url,''),s.material_url,'') as material_url,coalesce(nullif(f.cdn_url,''),nullif(f.original_url,''),s.fallback_url,'') as fallback_url,coalesce(s.alt_text,'') as alt_text,s.sort_order,s.is_enabled,s.effective_start_time,s.effective_end_time,s.extra_config,s.updated_at from xz_page_asset_slots s left join lateral (select aa.cdn_url,aa.original_url from xz_media_assets aa where aa.id=s.asset_id and aa.tenant_id in (s.tenant_id,'default') and aa.deleted_at is null and aa.status='ACTIVE' order by (aa.tenant_id=s.tenant_id) desc limit 1) a on true left join lateral (select ff.cdn_url,ff.original_url from xz_media_assets ff where ff.id=s.fallback_asset_id and ff.tenant_id in (s.tenant_id,'default') and ff.deleted_at is null and ff.status='ACTIVE' order by (ff.tenant_id=s.tenant_id) desc limit 1) f on true where s.deleted_at is null and s.page_code=$2 and s.tenant_id=$1`
	if inherit && tenant != "default" {
		query = `select distinct on (slot_key) id,tenant_id,page_code,module_code,slot_key,slot_name,asset_id,fallback_asset_id,material_url,fallback_url,alt_text,sort_order,is_enabled,effective_start_time,effective_end_time,extra_config,updated_at from (` + query + ` union all ` + strings.ReplaceAll(query, "s.tenant_id=$1", "s.tenant_id='default'") + `) inherited order by slot_key,(tenant_id=$1) desc`
	}
	return query
}

func (r *postgresMediaRepository) ListSlots(ctx context.Context, tenant, page string, inherit bool) ([]pageAssetSlot, error) {
	query := pageSlotListQuery(tenant, inherit)
	rows, err := r.db.QueryContext(ctx, query, tenant, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pageAssetSlot{}
	for rows.Next() {
		item, err := scanPageSlot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sortPageSlots(items)
	return items, rows.Err()
}
func (r *postgresMediaRepository) SaveSlot(ctx context.Context, item pageAssetSlot) (pageAssetSlot, error) {
	if item.ID == "" {
		item.ID = "slot_" + newRequestID()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return item, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `insert into xz_page_asset_slots(id,tenant_id,page_code,module_code,slot_key,slot_name,asset_id,fallback_asset_id,material_url,fallback_url,alt_text,sort_order,is_enabled,effective_start_time,effective_end_time,extra_config) values($1,$2,$3,$4,$5,$6,nullif($7,''),nullif($8,''),nullif($9,''),nullif($10,''),nullif($11,''),$12,$13,nullif($14,'')::timestamptz,nullif($15,'')::timestamptz,$16::jsonb) on conflict(tenant_id,page_code,slot_key) do update set module_code=excluded.module_code,slot_name=excluded.slot_name,asset_id=excluded.asset_id,fallback_asset_id=excluded.fallback_asset_id,material_url=excluded.material_url,fallback_url=excluded.fallback_url,alt_text=excluded.alt_text,sort_order=excluded.sort_order,is_enabled=excluded.is_enabled,effective_start_time=excluded.effective_start_time,effective_end_time=excluded.effective_end_time,extra_config=excluded.extra_config,updated_at=now()`, item.ID, item.TenantID, item.PageCode, item.ModuleCode, item.SlotKey, item.SlotName, item.AssetID, item.FallbackAssetID, item.MaterialURL, item.FallbackURL, item.AltText, item.SortOrder, item.IsEnabled, item.EffectiveStartTime, item.EffectiveEndTime, jsonBytes(item.ExtraConfig))
	if err != nil {
		return item, err
	}
	_, err = tx.ExecContext(ctx, `delete from xz_media_asset_usage where tenant_id=$1 and page_code=$2 and slot_key=$3 and business_type='PAGE_SLOT'`, item.TenantID, item.PageCode, item.SlotKey)
	if err != nil {
		return item, err
	}
	for _, assetID := range []string{item.AssetID, item.FallbackAssetID} {
		if assetID == "" {
			continue
		}
		_, err = tx.ExecContext(ctx, `insert into xz_media_asset_usage(id,tenant_id,asset_id,page_code,module_code,slot_key,business_type,business_id) values($1,$2,$3,$4,$5,$6,'PAGE_SLOT',$7) on conflict do nothing`, `usage_`+newRequestID(), item.TenantID, assetID, item.PageCode, item.ModuleCode, item.SlotKey, item.ID)
		if err != nil {
			return item, err
		}
	}
	_, err = tx.ExecContext(ctx, `update xz_media_assets a set usage_count=(select count(*) from xz_media_asset_usage u where u.tenant_id=a.tenant_id and u.asset_id=a.id) where a.tenant_id=$1`, item.TenantID)
	if err != nil {
		return item, err
	}
	if err = tx.Commit(); err != nil {
		return item, err
	}
	items, err := r.ListSlots(ctx, item.TenantID, item.PageCode, false)
	if err == nil {
		for _, current := range items {
			if current.SlotKey == item.SlotKey {
				return current, nil
			}
		}
	}
	return item, err
}
func (r *postgresMediaRepository) GetPageConfig(ctx context.Context, tenant, page string) (pageConfig, error) {
	var item pageConfig
	var raw []byte
	var created, updated time.Time
	var published sql.NullTime
	err := r.db.QueryRowContext(ctx, `select id,tenant_id,page_code,version,config_json,status,published_at,coalesce(created_by,''),created_at,coalesce(updated_by,''),updated_at from xz_page_configs where tenant_id=$1 and page_code=$2 and deleted_at is null`, tenant, page).Scan(&item.ID, &item.TenantID, &item.PageCode, &item.Version, &raw, &item.Status, &published, &item.CreatedBy, &created, &item.UpdatedBy, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errMediaNotFound
	}
	if err != nil {
		return item, err
	}
	json.Unmarshal(raw, &item.ConfigJSON)
	item.CreatedAt = created.UTC().Format(time.RFC3339)
	item.UpdatedAt = updated.UTC().Format(time.RFC3339)
	if published.Valid {
		item.PublishedAt = published.Time.UTC().Format(time.RFC3339)
	}
	return item, nil
}
func (r *postgresMediaRepository) SavePageDraft(ctx context.Context, item pageConfig) (pageConfig, error) {
	if item.ID == "" {
		item.ID = "page_" + newRequestID()
	}
	_, err := r.db.ExecContext(ctx, `insert into xz_page_configs(id,tenant_id,page_code,config_json,status,created_by,updated_by) values($1,$2,$3,$4::jsonb,'DRAFT',nullif($5,''),nullif($5,'')) on conflict(tenant_id,page_code) do update set config_json=excluded.config_json,status='DRAFT',updated_by=excluded.updated_by,updated_at=now()`, item.ID, item.TenantID, item.PageCode, jsonBytes(item.ConfigJSON), item.UpdatedBy)
	if err != nil {
		return item, err
	}
	return r.GetPageConfig(ctx, item.TenantID, item.PageCode)
}
func (r *postgresMediaRepository) PublishPage(ctx context.Context, tenant, page, note, user string) (pageConfig, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return pageConfig{}, err
	}
	defer tx.Rollback()
	var item pageConfig
	var raw []byte
	err = tx.QueryRowContext(ctx, `update xz_page_configs set version=version+1,status='PUBLISHED',published_at=now(),updated_by=nullif($3,''),updated_at=now() where tenant_id=$1 and page_code=$2 and deleted_at is null returning id,tenant_id,page_code,version,config_json,status,published_at`, tenant, page, user).Scan(&item.ID, &item.TenantID, &item.PageCode, &item.Version, &raw, &item.Status, &item.PublishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return item, errMediaNotFound
	}
	if err != nil {
		return item, err
	}
	json.Unmarshal(raw, &item.ConfigJSON)
	_, err = tx.ExecContext(ctx, `insert into xz_page_config_versions(id,tenant_id,page_config_id,version,config_json,change_note,created_by) values($1,$2,$3,$4,$5::jsonb,nullif($6,''),nullif($7,''))`, `page_ver_`+newRequestID(), tenant, item.ID, item.Version, raw, note, user)
	if err != nil {
		return item, err
	}
	if err = tx.Commit(); err != nil {
		return item, err
	}
	return r.GetPageConfig(ctx, tenant, page)
}
func (r *postgresMediaRepository) ListPageVersions(ctx context.Context, tenant, page string) ([]pageConfigVersion, error) {
	rows, err := r.db.QueryContext(ctx, `select v.id,v.tenant_id,v.page_config_id,v.version,v.config_json,coalesce(v.change_note,''),coalesce(v.created_by,''),v.created_at from xz_page_config_versions v join xz_page_configs p on p.id=v.page_config_id and p.tenant_id=v.tenant_id where v.tenant_id=$1 and p.page_code=$2 order by v.version desc`, tenant, page)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []pageConfigVersion{}
	for rows.Next() {
		var item pageConfigVersion
		var raw []byte
		var created time.Time
		if err := rows.Scan(&item.ID, &item.TenantID, &item.PageConfigID, &item.Version, &raw, &item.ChangeNote, &item.CreatedBy, &created); err != nil {
			return nil, err
		}
		json.Unmarshal(raw, &item.ConfigJSON)
		item.CreatedAt = created.UTC().Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *postgresMediaRepository) RollbackPage(ctx context.Context, tenant, page string, version int, user string) (pageConfig, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `select v.config_json from xz_page_config_versions v join xz_page_configs p on p.id=v.page_config_id and p.tenant_id=v.tenant_id where v.tenant_id=$1 and p.page_code=$2 and v.version=$3`, tenant, page, version).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return pageConfig{}, errMediaNotFound
	}
	if err != nil {
		return pageConfig{}, err
	}
	_, err = r.db.ExecContext(ctx, `update xz_page_configs set config_json=$3::jsonb,status='DRAFT',updated_by=nullif($4,''),updated_at=now() where tenant_id=$1 and page_code=$2`, tenant, page, raw, user)
	if err != nil {
		return pageConfig{}, err
	}
	return r.PublishPage(ctx, tenant, page, fmt.Sprintf("rollback to version %d", version), user)
}

type mediaAssetScanner interface{ Scan(...any) error }

func scanMediaAsset(row mediaAssetScanner) (mediaAsset, error) {
	var item mediaAsset
	var metadata []byte
	var created, updated time.Time
	err := row.Scan(&item.ID, &item.TenantID, &item.Name, &item.CategoryID, &item.CategoryName, &item.AssetType, &item.MimeType, &item.FileExt, &item.OriginalName, &item.StorageProvider, &item.StorageBucket, &item.StorageKey, &item.OriginalURL, &item.CDNURL, &item.ThumbnailURL, &item.Width, &item.Height, &item.AspectRatio, &item.FileSize, &item.FileHash, &item.Status, &item.AuditStatus, &item.IsDefault, &item.UsageCount, &item.SourceType, &item.SourceName, &item.LicenseType, &item.LicenseNote, &item.Prompt, &item.ModelName, &item.CopyrightOwner, &metadata, &item.CreatedBy, &created, &item.UpdatedBy, &updated)
	if err != nil {
		return item, err
	}
	json.Unmarshal(metadata, &item.Metadata)
	item.CreatedAt = created.UTC().Format(time.RFC3339)
	item.UpdatedAt = updated.UTC().Format(time.RFC3339)
	return item, nil
}
func scanPageSlot(row mediaAssetScanner) (pageAssetSlot, error) {
	var item pageAssetSlot
	var start, end sql.NullTime
	var extra []byte
	var updated time.Time
	err := row.Scan(&item.ID, &item.TenantID, &item.PageCode, &item.ModuleCode, &item.SlotKey, &item.SlotName, &item.AssetID, &item.FallbackAssetID, &item.MaterialURL, &item.FallbackURL, &item.AltText, &item.SortOrder, &item.IsEnabled, &start, &end, &extra, &updated)
	if err != nil {
		return item, err
	}
	json.Unmarshal(extra, &item.ExtraConfig)
	if start.Valid {
		item.EffectiveStartTime = start.Time.UTC().Format(time.RFC3339)
	}
	if end.Valid {
		item.EffectiveEndTime = end.Time.UTC().Format(time.RFC3339)
	}
	item.UpdatedAt = updated.UTC().Format(time.RFC3339)
	return item, nil
}
func jsonBytes(value any) []byte {
	raw, _ := json.Marshal(value)
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}
func sortPageSlots(items []pageAssetSlot) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && (items[j].SortOrder < items[j-1].SortOrder || (items[j].SortOrder == items[j-1].SortOrder && items[j].SlotKey < items[j-1].SlotKey)); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
