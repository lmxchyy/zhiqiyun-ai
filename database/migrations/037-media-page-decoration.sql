-- Media center and tenant-aware page decoration.
-- PostgreSQL is the current primary store; all statements are safe to re-run.

create table if not exists xz_media_categories (
  id text primary key,
  tenant_id text not null default 'default',
  parent_id text,
  name text not null,
  code text not null,
  sort_order integer not null default 0,
  status text not null default 'ACTIVE',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, code),
  unique (tenant_id, id),
  foreign key (tenant_id, parent_id) references xz_media_categories(tenant_id, id)
);

create table if not exists xz_media_assets (
  id text primary key,
  tenant_id text not null default 'default',
  name text not null,
  category_id text,
  asset_type text not null default 'IMAGE',
  mime_type text not null,
  file_ext text not null,
  original_name text not null,
  storage_provider text not null default 'local',
  storage_bucket text,
  storage_key text not null,
  original_url text,
  cdn_url text,
  thumbnail_url text,
  width integer not null default 0,
  height integer not null default 0,
  aspect_ratio numeric(12,6) not null default 0,
  file_size bigint not null default 0,
  file_hash text not null,
  status text not null default 'ACTIVE',
  audit_status text not null default 'APPROVED',
  is_default boolean not null default false,
  usage_count integer not null default 0,
  source_type text not null default 'OPERATION_UPLOAD',
  source_name text,
  license_type text,
  license_note text,
  prompt text,
  model_name text,
  copyright_owner text,
  metadata jsonb not null default '{}'::jsonb,
  created_by text,
  created_at timestamptz not null default now(),
  updated_by text,
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id),
  unique (tenant_id, file_hash)
);

create table if not exists xz_page_asset_slots (
  id text primary key,
  tenant_id text not null default 'default',
  page_code text not null,
  module_code text not null,
  slot_key text not null,
  slot_name text not null,
  asset_id text,
  fallback_asset_id text,
  material_url text,
  fallback_url text,
  alt_text text,
  sort_order integer not null default 0,
  is_enabled boolean not null default true,
  effective_start_time timestamptz,
  effective_end_time timestamptz,
  extra_config jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, id),
  unique (tenant_id, page_code, slot_key)
);

create table if not exists xz_page_configs (
  id text primary key,
  tenant_id text not null default 'default',
  page_code text not null,
  version integer not null default 0,
  config_json jsonb not null default '{}'::jsonb,
  status text not null default 'DRAFT',
  published_at timestamptz,
  created_by text,
  created_at timestamptz not null default now(),
  updated_by text,
  updated_at timestamptz not null default now(),
  deleted_at timestamptz,
  unique (tenant_id, page_code)
);

create table if not exists xz_page_config_versions (
  id text primary key,
  tenant_id text not null default 'default',
  page_config_id text not null references xz_page_configs(id),
  version integer not null,
  config_json jsonb not null default '{}'::jsonb,
  change_note text,
  created_by text,
  created_at timestamptz not null default now(),
  unique (tenant_id, page_config_id, version)
);

create table if not exists xz_media_asset_usage (
  id text primary key,
  tenant_id text not null default 'default',
  asset_id text not null,
  page_code text not null,
  module_code text not null,
  slot_key text not null,
  business_type text not null default 'PAGE_SLOT',
  business_id text,
  created_at timestamptz not null default now(),
  unique (tenant_id, asset_id, page_code, slot_key, business_type)
);

create index if not exists idx_xz_media_assets_list on xz_media_assets(tenant_id, status, category_id, created_at desc) where deleted_at is null;
create index if not exists idx_xz_media_assets_name on xz_media_assets(tenant_id, lower(name)) where deleted_at is null;
create index if not exists idx_xz_page_slots_page on xz_page_asset_slots(tenant_id, page_code, sort_order) where deleted_at is null;
create index if not exists idx_xz_page_versions_page on xz_page_config_versions(tenant_id, page_config_id, version desc);
create index if not exists idx_xz_media_usage_asset on xz_media_asset_usage(tenant_id, asset_id, created_at desc);

insert into xz_media_categories (id, tenant_id, name, code, sort_order)
values
  ('media_cat_home_hero', 'default', '首页 Hero', 'home-hero', 10),
  ('media_cat_home_quick', 'default', '首页快捷入口', 'home-quick', 20),
  ('media_cat_ai_capability', 'default', 'AI 创作能力', 'ai-capability', 30),
  ('media_cat_ai_employee', 'default', 'AI 员工头像', 'ai-employee', 40),
  ('media_cat_template', 'default', '创作模板', 'creation-template', 50),
  ('media_cat_work_cover', 'default', '作品封面', 'work-cover', 60),
  ('media_cat_avatar', 'default', '用户头像', 'user-avatar', 70),
  ('media_cat_banner', 'default', 'Banner', 'banner', 80),
  ('media_cat_logo', 'default', 'Logo', 'logo', 90),
  ('media_cat_placeholder', 'default', '默认占位图', 'default-placeholder', 100),
  ('media_cat_icon', 'default', '系统图标', 'system-icon', 110),
  ('media_cat_festival', 'default', '节日活动', 'festival', 120),
  ('media_cat_promotion', 'default', '宣传素材', 'promotion', 130)
on conflict (tenant_id, code) do update set name=excluded.name, sort_order=excluded.sort_order, updated_at=now();

insert into xz_page_asset_slots (id, tenant_id, page_code, module_code, slot_key, slot_name, sort_order, alt_text)
values
  ('slot_home_hero_background','default','home','hero','home.hero.background','首页主视觉背景',10,'知启云AI 首页主视觉'),
  ('slot_home_hero_illustration','default','home','hero','home.hero.illustration','首页主视觉插画',20,'AI 创作插画'),
  ('slot_home_quick_poster','default','home','quick','home.quick.poster','快捷入口-海报',30,'AI 海报'),
  ('slot_home_quick_ppt','default','home','quick','home.quick.ppt','快捷入口-PPT',40,'AI PPT'),
  ('slot_home_quick_video','default','home','quick','home.quick.video','快捷入口-视频',50,'AI 视频'),
  ('slot_home_quick_knowledge','default','home','quick','home.quick.knowledge','快捷入口-知识库',60,'企业知识库'),
  ('slot_home_cap_design','default','home','capability','home.capability.ai_design','能力-AI设计',70,'AI 设计能力'),
  ('slot_home_cap_video','default','home','capability','home.capability.ai_video','能力-AI视频',80,'AI 视频能力'),
  ('slot_home_cap_ppt','default','home','capability','home.capability.ppt','能力-PPT',90,'AI PPT 能力'),
  ('slot_home_cap_office','default','home','capability','home.capability.office','能力-AI办公',100,'AI 办公能力'),
  ('slot_home_cap_knowledge','default','home','capability','home.capability.knowledge','能力-知识库',110,'知识库能力'),
  ('slot_home_cap_employee','default','home','capability','home.capability.employee','能力-AI员工',120,'AI 员工能力'),
  ('slot_home_emp_designer','default','home','employee','home.employee.designer','AI设计师头像',130,'AI 设计师'),
  ('slot_home_emp_sales','default','home','employee','home.employee.sales','AI销售头像',140,'AI 销售'),
  ('slot_home_emp_operation','default','home','employee','home.employee.operation','AI运营头像',150,'AI 运营'),
  ('slot_home_emp_service','default','home','employee','home.employee.service','AI客服头像',160,'AI 客服'),
  ('slot_home_emp_boss','default','home','employee','home.employee.boss','老板助手头像',170,'老板助手'),
  ('slot_home_ins_poster','default','home','inspiration','home.inspiration.poster','灵感-海报',180,'企业宣传海报'),
  ('slot_home_ins_video','default','home','inspiration','home.inspiration.video','灵感-视频',190,'短视频模板'),
  ('slot_home_ins_ppt','default','home','inspiration','home.inspiration.ppt','灵感-PPT',200,'招商 PPT'),
  ('slot_home_ins_store','default','home','inspiration','home.inspiration.store','灵感-门店',210,'门店营销'),
  ('slot_home_ins_ecommerce','default','home','inspiration','home.inspiration.ecommerce','灵感-电商',220,'电商主图'),
  ('slot_home_project_new_product','default','home','project','home.project.new_product','项目-新品上市',230,'新品上市'),
  ('slot_home_project_website','default','home','project','home.project.website','项目-企业官网',240,'企业官网'),
  ('slot_studio_banner','default','studio','banner','studio.hero.illustration','创作页 Hero 插画',10,'创作中心'),
  ('slot_studio_poster','default','studio','template','studio.template.new_product','模板-新品发布',20,'新品发布模板'),
  ('slot_studio_video','default','studio','template','studio.template.investment','模板-招商方案',30,'招商方案模板'),
  ('slot_studio_ppt','default','studio','template','studio.template.ppt','模板-PPT',40,'PPT 模板'),
  ('slot_studio_office','default','studio','template','studio.template.office','模板-办公',50,'办公模板'),
  ('slot_studio_knowledge','default','studio','template','studio.template.knowledge','模板-知识库',60,'知识库模板'),
  ('slot_studio_employee','default','studio','template','studio.template.employee','模板-AI员工',70,'AI 员工模板'),
  ('slot_assets_image','default','assets','default','assets.cover.image','默认图片封面',10,'图片作品'),
  ('slot_assets_video','default','assets','default','assets.cover.video','默认视频封面',20,'视频作品'),
  ('slot_assets_ppt','default','assets','default','assets.cover.ppt','默认PPT封面',30,'PPT 作品'),
  ('slot_assets_document','default','assets','default','assets.cover.long_image','默认长图封面',40,'长图作品'),
  ('slot_assets_other','default','assets','default','assets.default.other','默认其他封面',50,'其他作品'),
  ('slot_profile_avatar','default','profile','header','profile.avatar','默认用户头像',10,'用户头像'),
  ('slot_profile_member_bg','default','profile','member','profile.member_background','会员卡背景',20,'会员卡背景'),
  ('slot_profile_header_bg','default','profile','header','profile.header_background','我的页头图',30,'个人中心背景')
on conflict (tenant_id, page_code, slot_key) do update set slot_name=excluded.slot_name, module_code=excluded.module_code, sort_order=excluded.sort_order, updated_at=now();

insert into xz_page_configs (id, tenant_id, page_code, version, config_json, status)
values
  ('page_default_home','default','home',0,'{"modules":[]}'::jsonb,'DRAFT'),
  ('page_default_studio','default','studio',0,'{"modules":[]}'::jsonb,'DRAFT'),
  ('page_default_assets','default','assets',0,'{"modules":[]}'::jsonb,'DRAFT'),
  ('page_default_profile','default','profile',0,'{"modules":[]}'::jsonb,'DRAFT')
on conflict (tenant_id, page_code) do nothing;
