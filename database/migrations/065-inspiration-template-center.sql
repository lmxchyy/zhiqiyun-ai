CREATE TABLE IF NOT EXISTS inspiration_categories (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL DEFAULT 'default',
  code varchar(64) NOT NULL,
  name varchar(80) NOT NULL,
  sort_order integer NOT NULL DEFAULT 0,
  status varchar(20) NOT NULL DEFAULT 'ACTIVE',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  UNIQUE (tenant_id, code)
);

CREATE TABLE IF NOT EXISTS inspiration_templates (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL DEFAULT 'default',
  title varchar(160) NOT NULL,
  description text NOT NULL DEFAULT '',
  content_type varchar(20) NOT NULL,
  category_id varchar(64) NOT NULL REFERENCES inspiration_categories(id),
  cover_url text NOT NULL,
  thumbnail_url text NOT NULL DEFAULT '',
  result_url text NOT NULL DEFAULT '',
  prompt text NOT NULL,
  negative_prompt text NOT NULL DEFAULT '',
  model_id varchar(120) NOT NULL DEFAULT '',
  parameters_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  reference_assets_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  platforms_json jsonb NOT NULL DEFAULT '["miniprogram"]'::jsonb,
  tags_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  applicable_tenant_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb,
  featured boolean NOT NULL DEFAULT false,
  hot boolean NOT NULL DEFAULT false,
  pinned boolean NOT NULL DEFAULT false,
  sort_order integer NOT NULL DEFAULT 0,
  status varchar(20) NOT NULL DEFAULT 'DRAFT',
  audit_status varchar(20) NOT NULL DEFAULT 'PENDING',
  audit_note text NOT NULL DEFAULT '',
  start_time timestamptz,
  end_time timestamptz,
  version integer NOT NULL DEFAULT 1,
  source_asset_id varchar(64),
  source_authorized boolean NOT NULL DEFAULT false,
  created_by varchar(64) NOT NULL DEFAULT '',
  updated_by varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CHECK (content_type IN ('image', 'video', 'ppt')),
  CHECK (status IN ('DRAFT', 'PUBLISHED', 'WITHDRAWN', 'ARCHIVED')),
  CHECK (audit_status IN ('PENDING', 'APPROVED', 'REJECTED'))
);

CREATE TABLE IF NOT EXISTS inspiration_favorites (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  template_id varchar(64) NOT NULL REFERENCES inspiration_templates(id),
  user_id varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (template_id, user_id)
);

CREATE TABLE IF NOT EXISTS inspiration_events (
  id varchar(64) PRIMARY KEY,
  tenant_id varchar(64) NOT NULL,
  template_id varchar(64) NOT NULL REFERENCES inspiration_templates(id),
  user_id varchar(64),
  event_type varchar(32) NOT NULL,
  generation_task_id varchar(64),
  platform varchar(32) NOT NULL DEFAULT 'miniprogram',
  request_id varchar(120) NOT NULL DEFAULT '',
  metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (event_type IN ('view', 'copy_prompt', 'favorite', 'unfavorite', 'use_template', 'generate_success'))
);

CREATE TABLE IF NOT EXISTS inspiration_template_versions (
  id varchar(64) PRIMARY KEY,
  template_id varchar(64) NOT NULL REFERENCES inspiration_templates(id),
  tenant_id varchar(64) NOT NULL,
  version integer NOT NULL,
  snapshot_json jsonb NOT NULL,
  change_note text NOT NULL DEFAULT '',
  created_by varchar(64) NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (template_id, version)
);

CREATE INDEX IF NOT EXISTS idx_inspiration_templates_public
  ON inspiration_templates (status, audit_status, featured, pinned, sort_order DESC, updated_at DESC)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_inspiration_templates_tenant_category
  ON inspiration_templates (tenant_id, category_id, content_type, updated_at DESC)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_inspiration_events_template_type
  ON inspiration_events (template_id, event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inspiration_favorites_user
  ON inspiration_favorites (user_id, created_at DESC);

INSERT INTO inspiration_categories (id, tenant_id, code, name, sort_order)
VALUES
  ('inspiration-category-recommend', 'default', 'recommend', '推荐', 100),
  ('inspiration-category-product', 'default', 'product', '商品图', 90),
  ('inspiration-category-poster', 'default', 'poster', '营销海报', 80),
  ('inspiration-category-portrait', 'default', 'portrait', 'AI写真', 70),
  ('inspiration-category-brand', 'default', 'brand', '品牌设计', 60),
  ('inspiration-category-video', 'default', 'video', 'AI视频', 50),
  ('inspiration-category-ppt', 'default', 'ppt', 'PPT方案', 40)
ON CONFLICT (tenant_id, code) DO UPDATE SET name=excluded.name, sort_order=excluded.sort_order, updated_at=now();

INSERT INTO inspiration_templates (
  id, tenant_id, title, description, content_type, category_id, cover_url, thumbnail_url,
  result_url, prompt, negative_prompt, model_id, parameters_json, platforms_json, tags_json,
  featured, hot, pinned, sort_order, status, audit_status, source_authorized, created_by, updated_by
)
VALUES
  ('inspiration-product-clean', 'default', '极简科技商品主图', '干净背景与商业级产品光影', 'image', 'inspiration-category-product', '/static/fallbacks/inspiration-ecommerce.jpg', '/static/fallbacks/inspiration-ecommerce.jpg', '/static/fallbacks/inspiration-ecommerce.jpg', '高端科技产品商业摄影，产品居中，柔和轮廓光，干净渐变背景，细节清晰，电商主图构图', '文字，水印，低清晰度，畸变，杂乱背景', 'gpt-image-2', '{"ratio":"1:1","quality":"high"}', '["miniprogram","h5","app","pc"]', '["商品图","电商"]', true, true, true, 100, 'PUBLISHED', 'APPROVED', true, 'system', 'system'),
  ('inspiration-poster-brand', 'default', '品牌新品发布海报', '适合新品上市与社交媒体传播', 'image', 'inspiration-category-poster', '/static/fallbacks/inspiration-poster.jpg', '/static/fallbacks/inspiration-poster.jpg', '/static/fallbacks/inspiration-poster.jpg', '品牌新品发布视觉海报，现代构图，强烈视觉焦点，留出中文标题排版空间，商业广告质感', '水印，错别字，模糊，廉价素材感', 'gpt-image-2', '{"ratio":"3:4","quality":"high"}', '["miniprogram","h5","app","pc"]', '["海报","新品发布"]', true, true, false, 90, 'PUBLISHED', 'APPROVED', true, 'system', 'system'),
  ('inspiration-portrait-office', 'default', '职场形象写真', '自然可信的专业商务形象', 'image', 'inspiration-category-portrait', '/static/fallbacks/default-inspiration.jpg', '/static/fallbacks/default-inspiration.jpg', '/static/fallbacks/default-inspiration.jpg', '专业职场人物肖像，真实自然肤质，柔和棚拍光线，简洁办公背景，可信亲和，高级商业摄影', '过度磨皮，畸形五官，多余手指，水印', 'gpt-image-2', '{"ratio":"3:4","quality":"high"}', '["miniprogram","h5","app","pc"]', '["AI写真","职场"]', true, false, false, 80, 'PUBLISHED', 'APPROVED', true, 'system', 'system'),
  ('inspiration-brand-identity', 'default', '未来感品牌视觉', '品牌主视觉与社交传播延展', 'image', 'inspiration-category-brand', '/static/fallbacks/inspiration-store.jpg', '/static/fallbacks/inspiration-store.jpg', '/static/fallbacks/inspiration-store.jpg', '未来科技品牌主视觉，几何秩序，蓝紫与青色点缀，高级材质，适合企业品牌传播', '水印，拥挤排版，低对比度，模糊', 'gpt-image-2', '{"ratio":"16:9","quality":"high"}', '["miniprogram","h5","app","pc"]', '["品牌设计","主视觉"]', true, false, false, 70, 'PUBLISHED', 'APPROVED', true, 'system', 'system'),
  ('inspiration-video-product', 'default', '产品电影感展示短片', '适合新品宣传的动态展示', 'video', 'inspiration-category-video', '/static/fallbacks/inspiration-video.jpg', '/static/fallbacks/inspiration-video.jpg', '/static/fallbacks/inspiration-video.jpg', '产品在深色摄影棚中缓慢旋转，镜头平滑推进，轮廓光扫过产品表面，电影感商业广告', '画面抖动，闪烁，产品变形，文字水印', 'seedance-fast-2.0', '{"ratio":"16:9","quality":"720p","duration":5}', '["miniprogram","h5","app","pc"]', '["AI视频","产品宣传"]', true, true, false, 60, 'PUBLISHED', 'APPROVED', true, 'system', 'system'),
  ('inspiration-ppt-roadshow', 'default', '科技项目招商路演', '十页结构化招商与项目介绍方案', 'ppt', 'inspiration-category-ppt', '/static/fallbacks/inspiration-ppt.jpg', '/static/fallbacks/inspiration-ppt.jpg', '/static/fallbacks/inspiration-ppt.jpg', '为科技创新项目制作招商路演PPT，包含市场机会、产品方案、竞争优势、商业模式、落地计划和合作诉求，数据表达清晰', '', 'kimi-k2.6', '{"pageCount":10,"scenario":"roadshow","style":"technology","withImages":true,"language":"zh"}', '["miniprogram","h5","app","pc"]', '["PPT方案","招商路演"]', true, true, false, 50, 'PUBLISHED', 'APPROVED', true, 'system', 'system')
ON CONFLICT (id) DO NOTHING;

INSERT INTO xz_role_permissions (role, permission)
SELECT role, permission
FROM (VALUES ('SUPER_ADMIN'), ('PLATFORM_ADMIN'), ('ADMIN'), ('CONTENT_OPERATOR')) AS roles(role)
CROSS JOIN (VALUES
  ('content:inspiration:view'),
  ('content:inspiration:manage'),
  ('content:inspiration:audit'),
  ('content:inspiration:publish')
) AS permissions(permission)
ON CONFLICT DO NOTHING;
