package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	errInspirationNotFound        = errors.New("inspiration template not found")
	errInspirationVersionConflict = errors.New("inspiration template version conflict")
)

type inspirationCategory struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort"`
	Status    string `json:"status"`
}

type inspirationTemplate struct {
	ID                  string                     `json:"id"`
	Slug                string                     `json:"slug"`
	TenantID            string                     `json:"tenantId"`
	Title               string                     `json:"title"`
	Description         string                     `json:"description"`
	ContentType         string                     `json:"contentType"`
	CategoryID          string                     `json:"categoryId"`
	CategoryCode        string                     `json:"categoryCode,omitempty"`
	CategoryName        string                     `json:"categoryName,omitempty"`
	CoverURL            string                     `json:"coverUrl"`
	ThumbnailURL        string                     `json:"thumbnailUrl,omitempty"`
	ResultURL           string                     `json:"resultUrl,omitempty"`
	Definition          InternalTemplateDefinition `json:"definition"`
	Platforms           []string                   `json:"platforms"`
	Tags                []string                   `json:"tags"`
	ApplicableTenantIDs []string                   `json:"applicableTenantIds"`
	Featured            bool                       `json:"featured"`
	Hot                 bool                       `json:"hot"`
	Pinned              bool                       `json:"pinned"`
	SortOrder           int                        `json:"sort"`
	Status              string                     `json:"status"`
	AuditStatus         string                     `json:"auditStatus"`
	AuditNote           string                     `json:"auditNote,omitempty"`
	StartTime           string                     `json:"startTime,omitempty"`
	EndTime             string                     `json:"endTime,omitempty"`
	Version             int                        `json:"version"`
	SourceAssetID       string                     `json:"sourceAssetId,omitempty"`
	SourceAuthorized    bool                       `json:"sourceAuthorized"`
	CreatedBy           string                     `json:"createdBy,omitempty"`
	UpdatedBy           string                     `json:"updatedBy,omitempty"`
	CreatedAt           string                     `json:"createdAt"`
	UpdatedAt           string                     `json:"updatedAt"`
	Favorite            bool                       `json:"favorite"`
	ViewCount           int64                      `json:"viewCount"`
	CopyCount           int64                      `json:"copyCount"`
	FavoriteCount       int64                      `json:"favoriteCount"`
	UseCount            int64                      `json:"useCount"`
	GenerateCount       int64                      `json:"generateCount"`
}

type inspirationListFilter struct {
	TenantID            string
	UserID              string
	Category            string
	ContentType         string
	ExcludeContentTypes []string
	Query               string
	Platform            string
	Featured            bool
	Hot                 bool
	Published           bool
	Status              string
	AuditStatus         string
	Limit               int
	Offset              int
	Seed                int
}

type inspirationVersion struct {
	ID         string              `json:"id"`
	TemplateID string              `json:"templateId"`
	TenantID   string              `json:"tenantId"`
	Version    int                 `json:"version"`
	Snapshot   inspirationTemplate `json:"snapshot"`
	ChangeNote string              `json:"changeNote"`
	CreatedBy  string              `json:"createdBy"`
	CreatedAt  string              `json:"createdAt"`
}

type inspirationRepository interface {
	ListCategories(context.Context, string, bool) ([]inspirationCategory, error)
	SaveCategory(context.Context, inspirationCategory) (inspirationCategory, error)
	ListTemplates(context.Context, inspirationListFilter) ([]inspirationTemplate, int, error)
	GetTemplate(context.Context, string, string, string, bool) (inspirationTemplate, error)
	GetTemplateBySlug(context.Context, string, string, string, bool) (inspirationTemplate, error)
	GetTemplateVersionBySlug(context.Context, string, string, string, int) (inspirationTemplate, error)
	SaveTemplate(context.Context, inspirationTemplate, string) (inspirationTemplate, error)
	DeleteTemplate(context.Context, string, string, string) error
	SetFavorite(context.Context, string, string, string, bool) error
	RecordEvent(context.Context, string, string, string, string, string, string, map[string]any) error
	ListVersions(context.Context, string, string) ([]inspirationVersion, error)
	Rollback(context.Context, string, string, int, string) (inspirationTemplate, error)
}

type memoryInspirationRepository struct {
	mu         sync.Mutex
	categories []inspirationCategory
	templates  []inspirationTemplate
	favorites  map[string]bool
	events     []map[string]any
	versions   []inspirationVersion
}

func newMemoryInspirationRepository() inspirationRepository {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	categories := []inspirationCategory{
		{ID: "inspiration-category-recommend", TenantID: "default", Code: "recommend", Name: "推荐", SortOrder: 100, Status: "ACTIVE"},
		{ID: "inspiration-category-product", TenantID: "default", Code: "product", Name: "商品图", SortOrder: 90, Status: "ACTIVE"},
		{ID: "inspiration-category-poster", TenantID: "default", Code: "poster", Name: "营销海报", SortOrder: 80, Status: "ACTIVE"},
		{ID: "inspiration-category-portrait", TenantID: "default", Code: "portrait", Name: "AI写真", SortOrder: 70, Status: "ACTIVE"},
		{ID: "inspiration-category-brand", TenantID: "default", Code: "brand", Name: "品牌设计", SortOrder: 60, Status: "ACTIVE"},
		{ID: "inspiration-category-video", TenantID: "default", Code: "video", Name: "AI视频", SortOrder: 50, Status: "ACTIVE"},
		{ID: "inspiration-category-ppt", TenantID: "default", Code: "ppt", Name: "PPT方案", SortOrder: 40, Status: "ACTIVE"},
	}
	makeTemplate := func(id, title, description, contentType, categoryID, cover, prompt, negative, model string, params map[string]any, sortOrder int) inspirationTemplate {
		return inspirationTemplate{ID: id, Slug: id, TenantID: "default", Title: title, Description: description, ContentType: contentType, CategoryID: categoryID, CoverURL: cover, ThumbnailURL: cover, ResultURL: cover, Definition: staticInspirationDefinition(contentType, prompt, negative, model, params), Platforms: []string{"miniprogram", "h5", "app", "pc"}, Tags: []string{"AI生成示例"}, Featured: true, Hot: sortOrder >= 60, SortOrder: sortOrder, Status: "PUBLISHED", AuditStatus: "APPROVED", SourceAuthorized: true, Version: 1, CreatedBy: "system", UpdatedBy: "system", CreatedAt: now, UpdatedAt: now}
	}
	templates := []inspirationTemplate{
		makeTemplate("inspiration-product-clean", "极简科技商品主图", "干净背景与商业级产品光影", "image", categories[1].ID, "/static/fallbacks/inspiration-ecommerce.jpg", "高端科技产品商业摄影，产品居中，柔和轮廓光，干净渐变背景，细节清晰，电商主图构图", "文字，水印，低清晰度，畸变，杂乱背景", "gpt-image-2", map[string]any{"ratio": "1:1", "quality": "high"}, 100),
		makeTemplate("inspiration-poster-brand", "品牌新品发布海报", "适合新品上市与社交媒体传播", "image", categories[2].ID, "/static/fallbacks/inspiration-poster.jpg", "品牌新品发布视觉海报，现代构图，强烈视觉焦点，留出中文标题排版空间，商业广告质感", "水印，错别字，模糊，廉价素材感", "gpt-image-2", map[string]any{"ratio": "3:4", "quality": "high"}, 90),
		makeTemplate("inspiration-portrait-office", "职场形象写真", "自然可信的专业商务形象", "image", categories[3].ID, "/static/fallbacks/default-inspiration.jpg", "专业职场人物肖像，真实自然肤质，柔和棚拍光线，简洁办公背景，可信亲和，高级商业摄影", "过度磨皮，畸形五官，多余手指，水印", "gpt-image-2", map[string]any{"ratio": "3:4", "quality": "high"}, 80),
		makeTemplate("inspiration-brand-identity", "未来感品牌视觉", "品牌主视觉与社交传播延展", "image", categories[4].ID, "/static/fallbacks/inspiration-store.jpg", "未来科技品牌主视觉，几何秩序，蓝紫与青色点缀，高级材质，适合企业品牌传播", "水印，拥挤排版，低对比度，模糊", "gpt-image-2", map[string]any{"ratio": "16:9", "quality": "high"}, 70),
		makeTemplate("inspiration-video-product", "产品电影感展示短片", "适合新品宣传的动态展示", "video", categories[5].ID, "/static/fallbacks/inspiration-video.jpg", "产品在深色摄影棚中缓慢旋转，镜头平滑推进，轮廓光扫过产品表面，电影感商业广告", "画面抖动，闪烁，产品变形，文字水印", "seedance-fast-2.0", map[string]any{"ratio": "16:9", "quality": "720p", "duration": 5}, 60),
		makeTemplate("inspiration-ppt-roadshow", "科技项目招商路演", "十页结构化招商与项目介绍方案", "ppt", categories[6].ID, "/static/fallbacks/inspiration-ppt.jpg", "为科技创新项目制作招商路演PPT，包含市场机会、产品方案、竞争优势、商业模式、落地计划和合作诉求，数据表达清晰", "", "kimi-k2.6", map[string]any{"pageCount": 10, "scenario": "roadshow", "style": "technology", "withImages": true, "language": "zh"}, 50),
	}
	return &memoryInspirationRepository{categories: categories, templates: templates, favorites: map[string]bool{}}
}

func staticInspirationDefinition(contentType, prompt, negative, model string, parameters map[string]any) InternalTemplateDefinition {
	contracts := map[string]struct{ targetType, targetKey, capabilityKey string }{
		"image": {"IMAGE_CREATION", "image.create", "image_generation"},
		"video": {"VIDEO_CREATION", "video.create", "video_generation"},
		"ppt":   {"PPT_CREATION", "ppt.create", "ppt_generation"},
	}
	contract := contracts[contentType]
	return InternalTemplateDefinition{
		SchemaVersion: currentTemplateSchemaVersion,
		Inputs:        []TemplateInputDefinition{},
		Prompt: TemplatePromptDefinition{Template: prompt, NegativeTemplate: negative,
			Composer: TemplateComposerDefinition{Key: "deterministic-template", Version: 1}},
		Bindings:     []TemplateBindingDefinition{},
		Presets:      TemplatePresetsDefinition{InputDefaults: map[string]any{}, GenerationDefaults: cloneTemplateMap(parameters)},
		Presentation: map[string]any{},
		Handoff:      TemplateHandoffDefinition{TargetType: contract.targetType, TargetKey: contract.targetKey},
		Capability:   TemplateCapabilityDefinition{CapabilityKey: contract.capabilityKey, ModelHint: model},
	}
}

func (m *memoryInspirationRepository) ListCategories(_ context.Context, tenantID string, admin bool) ([]inspirationCategory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]inspirationCategory, 0)
	for _, item := range m.categories {
		if (item.TenantID == "default" || item.TenantID == tenantID) && (admin || item.Status == "ACTIVE") {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].SortOrder > items[j].SortOrder })
	return items, nil
}

func (m *memoryInspirationRepository) SaveCategory(_ context.Context, item inspirationCategory) (inspirationCategory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.categories {
		if m.categories[i].ID == item.ID {
			m.categories[i] = item
			return item, nil
		}
	}
	m.categories = append(m.categories, item)
	return item, nil
}

func inspirationVisible(item inspirationTemplate, filter inspirationListFilter) bool {
	if item.TenantID != "default" && item.TenantID != filter.TenantID {
		return false
	}
	if filter.Published && (item.Status != "PUBLISHED" || item.AuditStatus != "APPROVED") {
		return false
	}
	if filter.Published && len(item.ApplicableTenantIDs) > 0 && !stringListContains(item.ApplicableTenantIDs, filter.TenantID) {
		return false
	}
	if filter.Published {
		now := time.Now().UTC()
		if start := inspirationTime(item.StartTime); start != nil && start.After(now) {
			return false
		}
		if end := inspirationTime(item.EndTime); end != nil && !end.After(now) {
			return false
		}
	}
	if filter.Status != "" && item.Status != filter.Status {
		return false
	}
	if filter.AuditStatus != "" && item.AuditStatus != filter.AuditStatus {
		return false
	}
	if filter.Featured && !item.Featured {
		return false
	}
	if filter.Hot && !item.Hot {
		return false
	}
	if filter.ContentType != "" && item.ContentType != filter.ContentType {
		return false
	}
	if len(filter.ExcludeContentTypes) > 0 {
		for _, excluded := range filter.ExcludeContentTypes {
			if strings.EqualFold(strings.TrimSpace(item.ContentType), strings.TrimSpace(excluded)) {
				return false
			}
		}
	}
	if filter.Category != "" && filter.Category != "recommend" && item.CategoryID != filter.Category && item.CategoryCode != filter.Category {
		return false
	}
	if filter.Query != "" && !strings.Contains(strings.ToLower(item.Title+" "+item.Description+" "+strings.Join(item.Tags, " ")), strings.ToLower(filter.Query)) {
		return false
	}
	if filter.Platform != "" && !stringListContains(item.Platforms, filter.Platform) {
		return false
	}
	return true
}

func (m *memoryInspirationRepository) decorate(item inspirationTemplate, userID string) inspirationTemplate {
	item.Favorite = m.favorites[item.ID+"|"+userID]
	for _, event := range m.events {
		if event["templateId"] != item.ID {
			continue
		}
		switch event["eventType"] {
		case "view":
			item.ViewCount++
		case "copy_prompt":
			item.CopyCount++
		case "use_template":
			item.UseCount++
		case "generate_success":
			item.GenerateCount++
		}
	}
	for key, favorite := range m.favorites {
		if favorite && strings.HasPrefix(key, item.ID+"|") {
			item.FavoriteCount++
		}
	}
	return item
}

func (m *memoryInspirationRepository) ListTemplates(_ context.Context, filter inspirationListFilter) ([]inspirationTemplate, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]inspirationTemplate, 0)
	for _, raw := range m.templates {
		item := m.decorate(raw, filter.UserID)
		if inspirationVisible(item, filter) {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder > items[j].SortOrder
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	if len(items) > 0 && filter.Seed > 0 {
		offset := filter.Seed % len(items)
		items = append(items[offset:], items[:offset]...)
	}
	total := len(items)
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}
	return append([]inspirationTemplate(nil), items[start:end]...), total, nil
}

func (m *memoryInspirationRepository) GetTemplate(_ context.Context, tenantID, userID, id string, admin bool) (inspirationTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.templates {
		publicVisible := inspirationVisible(item, inspirationListFilter{TenantID: tenantID, Published: true})
		if item.ID == id && (item.TenantID == "default" || item.TenantID == tenantID) && (admin || publicVisible) {
			return m.decorate(item, userID), nil
		}
	}
	return inspirationTemplate{}, errInspirationNotFound
}

func (m *memoryInspirationRepository) GetTemplateBySlug(_ context.Context, tenantID, userID, slug string, admin bool) (inspirationTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := len(m.templates) - 1; index >= 0; index-- {
		item := m.templates[index]
		publicVisible := inspirationVisible(item, inspirationListFilter{TenantID: tenantID, Published: true})
		if item.Slug == slug && (item.TenantID == "default" || item.TenantID == tenantID) && (admin || publicVisible) {
			return m.decorate(item, userID), nil
		}
	}
	return inspirationTemplate{}, errInspirationNotFound
}

func (m *memoryInspirationRepository) GetTemplateVersionBySlug(_ context.Context, tenantID, userID, slug string, version int) (inspirationTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var current *inspirationTemplate
	for index := range m.templates {
		item := m.templates[index]
		if item.Slug == slug && (item.TenantID == "default" || item.TenantID == tenantID) {
			candidate := item
			current = &candidate
			if item.TenantID == tenantID {
				break
			}
		}
	}
	if current == nil {
		return inspirationTemplate{}, errInspirationNotFound
	}
	if current.Version == version {
		if !inspirationVisible(*current, inspirationListFilter{TenantID: tenantID, Published: true}) {
			return inspirationTemplate{}, errInspirationVersionConflict
		}
		return m.decorate(*current, userID), nil
	}
	for _, historical := range m.versions {
		if historical.TemplateID == current.ID && historical.Version == version && inspirationVisible(historical.Snapshot, inspirationListFilter{TenantID: tenantID, Published: true}) {
			return m.decorate(historical.Snapshot, userID), nil
		}
	}
	return inspirationTemplate{}, errInspirationVersionConflict
}

func (m *memoryInspirationRepository) SaveTemplate(_ context.Context, item inspirationTemplate, note string) (inspirationTemplate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.templates {
		if existing.ID != item.ID && existing.TenantID == item.TenantID && existing.Slug == item.Slug {
			return inspirationTemplate{}, errors.New("inspiration template slug already exists in tenant")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = now
	for i := range m.templates {
		if m.templates[i].ID == item.ID {
			item.Version = m.templates[i].Version + 1
			item.CreatedAt = m.templates[i].CreatedAt
			m.versions = append(m.versions, inspirationVersion{ID: newInspirationID("version"), TemplateID: item.ID, TenantID: item.TenantID, Version: m.templates[i].Version, Snapshot: m.templates[i], ChangeNote: note, CreatedBy: item.UpdatedBy, CreatedAt: now})
			m.templates[i] = item
			return item, nil
		}
	}
	item.Version = 1
	item.CreatedAt = now
	m.templates = append(m.templates, item)
	return item, nil
}

func (m *memoryInspirationRepository) DeleteTemplate(_ context.Context, tenantID, id, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.templates {
		if m.templates[i].ID == id && (m.templates[i].TenantID == tenantID || tenantID == "default") {
			m.templates[i].Status = "ARCHIVED"
			return nil
		}
	}
	return errInspirationNotFound
}
func (m *memoryInspirationRepository) SetFavorite(_ context.Context, tenantID, userID, id string, favorite bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.favorites[id+"|"+userID] = favorite
	m.events = append(m.events, map[string]any{"templateId": id, "tenantId": tenantID, "userId": userID, "eventType": map[bool]string{true: "favorite", false: "unfavorite"}[favorite]})
	return nil
}
func (m *memoryInspirationRepository) RecordEvent(_ context.Context, tenantID, userID, id, eventType, taskID, platform string, metadata map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, map[string]any{"templateId": id, "tenantId": tenantID, "userId": userID, "eventType": eventType, "taskId": taskID, "platform": platform, "metadata": metadata})
	return nil
}
func (m *memoryInspirationRepository) ListVersions(_ context.Context, tenantID, id string) ([]inspirationVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := []inspirationVersion{}
	for _, item := range m.versions {
		if item.TemplateID == id && (item.TenantID == tenantID || tenantID == "default") {
			items = append(items, item)
		}
	}
	return items, nil
}
func (m *memoryInspirationRepository) Rollback(ctx context.Context, tenantID, id string, version int, actor string) (inspirationTemplate, error) {
	versions, _ := m.ListVersions(ctx, tenantID, id)
	for _, v := range versions {
		if v.Version == version {
			item := v.Snapshot
			item.Status = "DRAFT"
			item.AuditStatus = "PENDING"
			item.AuditNote = ""
			item.UpdatedBy = actor
			return m.SaveTemplate(ctx, item, fmt.Sprintf("rollback to version %d", version))
		}
	}
	return inspirationTemplate{}, errInspirationNotFound
}

type postgresInspirationRepository struct{ db *sql.DB }

const inspirationTemplateSelect = `SELECT t.id,t.slug,t.tenant_id,t.title,t.description,t.content_type,t.category_id,coalesce(c.code,''),coalesce(c.name,''),t.cover_url,t.thumbnail_url,t.result_url,t.definition_json,t.platforms_json,t.tags_json,t.applicable_tenant_ids_json,t.featured,t.hot,t.pinned,t.sort_order,t.status,t.audit_status,t.audit_note,t.start_time,t.end_time,t.version,coalesce(t.source_asset_id,''),t.source_authorized,t.created_by,t.updated_by,t.created_at,t.updated_at,
EXISTS(SELECT 1 FROM inspiration_favorites f WHERE f.template_id=t.id AND f.user_id=$2),
(SELECT count(*) FROM inspiration_events e WHERE e.template_id=t.id AND e.event_type='view'),
(SELECT count(*) FROM inspiration_events e WHERE e.template_id=t.id AND e.event_type='copy_prompt'),
(SELECT count(*) FROM inspiration_favorites f WHERE f.template_id=t.id),
(SELECT count(*) FROM inspiration_events e WHERE e.template_id=t.id AND e.event_type='use_template'),
(SELECT count(*) FROM inspiration_events e WHERE e.template_id=t.id AND e.event_type='generate_success')
FROM inspiration_templates t LEFT JOIN inspiration_categories c ON c.id=t.category_id`

func scanInspirationTemplate(scanner interface{ Scan(...any) error }) (inspirationTemplate, error) {
	var item inspirationTemplate
	var definition, platforms, tags, tenants []byte
	var start, end sql.NullTime
	var created, updated time.Time
	err := scanner.Scan(&item.ID, &item.Slug, &item.TenantID, &item.Title, &item.Description, &item.ContentType, &item.CategoryID, &item.CategoryCode, &item.CategoryName, &item.CoverURL, &item.ThumbnailURL, &item.ResultURL, &definition, &platforms, &tags, &tenants, &item.Featured, &item.Hot, &item.Pinned, &item.SortOrder, &item.Status, &item.AuditStatus, &item.AuditNote, &start, &end, &item.Version, &item.SourceAssetID, &item.SourceAuthorized, &item.CreatedBy, &item.UpdatedBy, &created, &updated, &item.Favorite, &item.ViewCount, &item.CopyCount, &item.FavoriteCount, &item.UseCount, &item.GenerateCount)
	if err != nil {
		return item, err
	}
	item.Definition, err = decodeInternalTemplateDefinition(definition)
	if err != nil {
		return item, fmt.Errorf("decode inspiration template %s definition: %w", item.ID, err)
	}
	_ = json.Unmarshal(platforms, &item.Platforms)
	_ = json.Unmarshal(tags, &item.Tags)
	_ = json.Unmarshal(tenants, &item.ApplicableTenantIDs)
	item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
	item.UpdatedAt = updated.UTC().Format(time.RFC3339Nano)
	if start.Valid {
		item.StartTime = start.Time.UTC().Format(time.RFC3339Nano)
	}
	if end.Valid {
		item.EndTime = end.Time.UTC().Format(time.RFC3339Nano)
	}
	return item, nil
}

func (p postgresInspirationRepository) ListCategories(ctx context.Context, tenantID string, admin bool) ([]inspirationCategory, error) {
	query := `SELECT id,tenant_id,code,name,sort_order,status FROM inspiration_categories WHERE deleted_at IS NULL AND (tenant_id='default' OR tenant_id=$1)`
	if !admin {
		query += ` AND status='ACTIVE'`
	}
	query += ` ORDER BY sort_order DESC,name`
	rows, err := p.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []inspirationCategory{}
	for rows.Next() {
		var item inspirationCategory
		if err = rows.Scan(&item.ID, &item.TenantID, &item.Code, &item.Name, &item.SortOrder, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (p postgresInspirationRepository) SaveCategory(ctx context.Context, item inspirationCategory) (inspirationCategory, error) {
	_, err := p.db.ExecContext(ctx, `INSERT INTO inspiration_categories(id,tenant_id,code,name,sort_order,status) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,sort_order=excluded.sort_order,status=excluded.status,updated_at=now()`, item.ID, item.TenantID, item.Code, item.Name, item.SortOrder, item.Status)
	return item, err
}

func (p postgresInspirationRepository) ListTemplates(ctx context.Context, f inspirationListFilter) ([]inspirationTemplate, int, error) {
	where := []string{"t.deleted_at IS NULL", "(t.tenant_id='default' OR t.tenant_id=$1)", "$2=$2"}
	args := []any{f.TenantID, f.UserID}
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if f.Published {
		where = append(where, "t.status='PUBLISHED'", "t.audit_status='APPROVED'", "(t.start_time IS NULL OR t.start_time<=now())", "(t.end_time IS NULL OR t.end_time>now())", "(jsonb_array_length(t.applicable_tenant_ids_json)=0 OR t.applicable_tenant_ids_json ? $1)")
	}
	if f.Featured {
		where = append(where, "t.featured=true")
	}
	if f.Hot {
		where = append(where, "t.hot=true")
	}
	if f.Status != "" {
		add("t.status=$%d", f.Status)
	}
	if f.AuditStatus != "" {
		add("t.audit_status=$%d", f.AuditStatus)
	}
	if f.ContentType != "" {
		add("t.content_type=$%d", f.ContentType)
	}
	if len(f.ExcludeContentTypes) > 0 {
		excluded := make([]string, 0, len(f.ExcludeContentTypes))
		for _, item := range f.ExcludeContentTypes {
			value := strings.ToLower(strings.TrimSpace(item))
			if value != "" {
				excluded = append(excluded, value)
			}
		}
		if len(excluded) == 1 {
			add("lower(t.content_type)<>$%d", excluded[0])
		} else if len(excluded) > 1 {
			args = append(args, excluded)
			where = append(where, fmt.Sprintf("NOT (lower(t.content_type) = ANY($%d))", len(args)))
		}
	}
	if f.Category != "" && f.Category != "recommend" {
		args = append(args, f.Category)
		n := len(args)
		where = append(where, fmt.Sprintf("(t.category_id=$%d OR c.code=$%d)", n, n))
	}
	if f.Query != "" {
		add("(t.title ILIKE '%%'||$%d||'%%' OR t.description ILIKE '%%'||$%d||'%%')", f.Query)
	}
	if f.Platform != "" {
		add("t.platforms_json ? $%d", f.Platform)
	}
	baseWhere := " WHERE " + strings.Join(where, " AND ")
	var total int
	countQuery := "SELECT count(*) FROM inspiration_templates t LEFT JOIN inspiration_categories c ON c.id=t.category_id" + baseWhere
	if err := p.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := " ORDER BY t.pinned DESC,t.sort_order DESC,t.updated_at DESC"
	if f.Seed > 0 {
		order = fmt.Sprintf(" ORDER BY md5(t.id || '%d'),t.pinned DESC,t.sort_order DESC", f.Seed)
	}
	args = append(args, f.Limit, f.Offset)
	query := inspirationTemplateSelect + baseWhere + order + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []inspirationTemplate{}
	for rows.Next() {
		item, scanErr := scanInspirationTemplate(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (p postgresInspirationRepository) GetTemplate(ctx context.Context, tenantID, userID, id string, admin bool) (inspirationTemplate, error) {
	where := " WHERE t.id=$3 AND t.deleted_at IS NULL AND (t.tenant_id='default' OR t.tenant_id=$1)"
	if !admin {
		where += " AND t.status='PUBLISHED' AND t.audit_status='APPROVED' AND (t.start_time IS NULL OR t.start_time<=now()) AND (t.end_time IS NULL OR t.end_time>now()) AND (jsonb_array_length(t.applicable_tenant_ids_json)=0 OR t.applicable_tenant_ids_json ? $1)"
	}
	item, err := scanInspirationTemplate(p.db.QueryRowContext(ctx, inspirationTemplateSelect+where, tenantID, userID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return item, errInspirationNotFound
	}
	return item, err
}

func (p postgresInspirationRepository) GetTemplateBySlug(ctx context.Context, tenantID, userID, slug string, admin bool) (inspirationTemplate, error) {
	where := " WHERE t.slug=$3 AND t.deleted_at IS NULL AND (t.tenant_id='default' OR t.tenant_id=$1)"
	if !admin {
		where += " AND t.status='PUBLISHED' AND t.audit_status='APPROVED' AND (t.start_time IS NULL OR t.start_time<=now()) AND (t.end_time IS NULL OR t.end_time>now()) AND (jsonb_array_length(t.applicable_tenant_ids_json)=0 OR t.applicable_tenant_ids_json ? $1)"
	}
	where += " ORDER BY (t.tenant_id=$1) DESC LIMIT 1"
	item, err := scanInspirationTemplate(p.db.QueryRowContext(ctx, inspirationTemplateSelect+where, tenantID, userID, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return item, errInspirationNotFound
	}
	return item, err
}

func (p postgresInspirationRepository) GetTemplateVersionBySlug(ctx context.Context, tenantID, userID, slug string, version int) (inspirationTemplate, error) {
	current, err := p.GetTemplateBySlug(ctx, tenantID, userID, slug, true)
	if err != nil {
		return inspirationTemplate{}, err
	}
	if current.Version == version {
		if !inspirationVisible(current, inspirationListFilter{TenantID: tenantID, Published: true}) {
			return inspirationTemplate{}, errInspirationVersionConflict
		}
		return current, nil
	}
	versions, err := p.ListVersions(ctx, tenantID, current.ID)
	if err != nil {
		return inspirationTemplate{}, err
	}
	for _, historical := range versions {
		if historical.Version == version && inspirationVisible(historical.Snapshot, inspirationListFilter{TenantID: tenantID, Published: true}) {
			return historical.Snapshot, nil
		}
	}
	return inspirationTemplate{}, errInspirationVersionConflict
}

func inspirationJSON(value any) []byte { raw, _ := json.Marshal(value); return raw }
func inspirationTime(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func (p postgresInspirationRepository) SaveTemplate(ctx context.Context, item inspirationTemplate, note string) (inspirationTemplate, error) {
	oldItem, oldErr := p.GetTemplate(ctx, item.TenantID, "", item.ID, true)
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return item, err
	}
	defer func() { _ = tx.Rollback() }()
	var oldVersion int
	_ = tx.QueryRowContext(ctx, `SELECT version FROM inspiration_templates WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, item.ID).Scan(&oldVersion)
	if oldVersion > 0 {
		snapshot := inspirationJSON(oldItem)
		if oldErr != nil {
			return item, oldErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO inspiration_template_versions(id,template_id,tenant_id,version,snapshot_json,change_note,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(template_id,version) DO NOTHING`, newInspirationID("version"), item.ID, item.TenantID, oldVersion, snapshot, note, item.UpdatedBy)
		if err != nil {
			return item, err
		}
		item.Version = oldVersion + 1
	} else {
		item.Version = 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO inspiration_templates(id,slug,tenant_id,title,description,content_type,category_id,cover_url,thumbnail_url,result_url,prompt,definition_json,platforms_json,tags_json,applicable_tenant_ids_json,featured,hot,pinned,sort_order,status,audit_status,audit_note,start_time,end_time,version,source_asset_id,source_authorized,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'',$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,NULLIF($25,''),$26,$27,$28) ON CONFLICT(id) DO UPDATE SET slug=excluded.slug,title=excluded.title,description=excluded.description,content_type=excluded.content_type,category_id=excluded.category_id,cover_url=excluded.cover_url,thumbnail_url=excluded.thumbnail_url,result_url=excluded.result_url,definition_json=excluded.definition_json,platforms_json=excluded.platforms_json,tags_json=excluded.tags_json,applicable_tenant_ids_json=excluded.applicable_tenant_ids_json,featured=excluded.featured,hot=excluded.hot,pinned=excluded.pinned,sort_order=excluded.sort_order,status=excluded.status,audit_status=excluded.audit_status,audit_note=excluded.audit_note,start_time=excluded.start_time,end_time=excluded.end_time,version=excluded.version,source_asset_id=excluded.source_asset_id,source_authorized=excluded.source_authorized,updated_by=excluded.updated_by,updated_at=now()`, item.ID, item.Slug, item.TenantID, item.Title, item.Description, item.ContentType, item.CategoryID, item.CoverURL, item.ThumbnailURL, item.ResultURL, inspirationJSON(item.Definition), inspirationJSON(item.Platforms), inspirationJSON(item.Tags), inspirationJSON(item.ApplicableTenantIDs), item.Featured, item.Hot, item.Pinned, item.SortOrder, item.Status, item.AuditStatus, item.AuditNote, inspirationTime(item.StartTime), inspirationTime(item.EndTime), item.Version, item.SourceAssetID, item.SourceAuthorized, item.CreatedBy, item.UpdatedBy)
	if err != nil {
		return item, err
	}
	if err = tx.Commit(); err != nil {
		return item, err
	}
	return p.GetTemplate(ctx, item.TenantID, "", item.ID, true)
}

func (p postgresInspirationRepository) DeleteTemplate(ctx context.Context, tenantID, id, actor string) error {
	result, err := p.db.ExecContext(ctx, `UPDATE inspiration_templates SET deleted_at=now(),status='ARCHIVED',updated_by=$3,updated_at=now() WHERE id=$1 AND (tenant_id=$2 OR $2='default') AND deleted_at IS NULL`, id, tenantID, actor)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return errInspirationNotFound
	}
	return nil
}
func (p postgresInspirationRepository) SetFavorite(ctx context.Context, tenantID, userID, id string, favorite bool) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if favorite {
		_, err = tx.ExecContext(ctx, `INSERT INTO inspiration_favorites(id,tenant_id,template_id,user_id) VALUES($1,$2,$3,$4) ON CONFLICT(template_id,user_id) DO NOTHING`, newInspirationID("favorite"), tenantID, id, userID)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM inspiration_favorites WHERE template_id=$1 AND user_id=$2`, id, userID)
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO inspiration_events(id,tenant_id,template_id,user_id,event_type) VALUES($1,$2,$3,$4,$5)`, newInspirationID("event"), tenantID, id, userID, map[bool]string{true: "favorite", false: "unfavorite"}[favorite])
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (p postgresInspirationRepository) RecordEvent(ctx context.Context, tenantID, userID, id, eventType, taskID, platform string, metadata map[string]any) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO inspiration_events(id,tenant_id,template_id,user_id,event_type,generation_task_id,platform,request_id,metadata_json) VALUES($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),$7,$8,$9)`, newInspirationID("event"), tenantID, id, userID, eventType, taskID, firstNonEmptyString(platform, "miniprogram"), stringValue(metadata["requestId"]), inspirationJSON(metadata))
	return err
}
func (p postgresInspirationRepository) ListVersions(ctx context.Context, tenantID, id string) ([]inspirationVersion, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id,template_id,tenant_id,version,snapshot_json,change_note,created_by,created_at FROM inspiration_template_versions WHERE template_id=$1 AND (tenant_id=$2 OR $2='default') ORDER BY version DESC`, id, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []inspirationVersion{}
	for rows.Next() {
		var item inspirationVersion
		var raw []byte
		var created time.Time
		if err = rows.Scan(&item.ID, &item.TemplateID, &item.TenantID, &item.Version, &raw, &item.ChangeNote, &item.CreatedBy, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &item.Snapshot)
		item.CreatedAt = created.UTC().Format(time.RFC3339Nano)
		items = append(items, item)
	}
	return items, rows.Err()
}
func (p postgresInspirationRepository) Rollback(ctx context.Context, tenantID, id string, version int, actor string) (inspirationTemplate, error) {
	var raw []byte
	err := p.db.QueryRowContext(ctx, `SELECT snapshot_json FROM inspiration_template_versions WHERE template_id=$1 AND version=$2 AND (tenant_id=$3 OR $3='default')`, id, version, tenantID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return inspirationTemplate{}, errInspirationNotFound
	}
	if err != nil {
		return inspirationTemplate{}, err
	}
	var item inspirationTemplate
	if err = json.Unmarshal(raw, &item); err != nil {
		return item, err
	}
	item.ID = id
	item.Status = "DRAFT"
	item.AuditStatus = "PENDING"
	item.AuditNote = ""
	item.UpdatedBy = actor
	return p.SaveTemplate(ctx, item, fmt.Sprintf("rollback to version %d", version))
}

func newInspirationID(prefix string) string {
	return fmt.Sprintf("inspiration_%s_%d", prefix, time.Now().UTC().UnixNano())
}
