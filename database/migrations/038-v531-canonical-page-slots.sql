-- Normalize already-seeded page-decoration data to the V5.3.1 canonical slot keys.
-- Safe to run repeatedly and intentionally preserves legacy auxiliary slots.

update xz_page_asset_slots set slot_key = 'home.employee.boss', updated_at = now()
where slot_key = 'home.employee.boss_assistant'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'home' and target.slot_key = 'home.employee.boss');

update xz_page_asset_slots set slot_key = 'studio.hero.illustration', slot_name = '创作页 Hero 插画', updated_at = now()
where slot_key = 'studio.banner'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'studio' and target.slot_key = 'studio.hero.illustration');

update xz_page_asset_slots set slot_key = 'studio.template.new_product', slot_name = '模板-新品发布', updated_at = now()
where slot_key = 'studio.template.poster'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'studio' and target.slot_key = 'studio.template.new_product');

update xz_page_asset_slots set slot_key = 'studio.template.investment', slot_name = '模板-招商方案', updated_at = now()
where slot_key = 'studio.template.video'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'studio' and target.slot_key = 'studio.template.investment');

update xz_page_asset_slots set slot_key = 'assets.cover.image', updated_at = now()
where slot_key = 'assets.default.image'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'assets' and target.slot_key = 'assets.cover.image');

update xz_page_asset_slots set slot_key = 'assets.cover.video', updated_at = now()
where slot_key = 'assets.default.video'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'assets' and target.slot_key = 'assets.cover.video');

update xz_page_asset_slots set slot_key = 'assets.cover.ppt', updated_at = now()
where slot_key = 'assets.default.ppt'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'assets' and target.slot_key = 'assets.cover.ppt');

update xz_page_asset_slots set slot_key = 'assets.cover.long_image', slot_name = '默认长图封面', updated_at = now()
where slot_key = 'assets.default.document'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'assets' and target.slot_key = 'assets.cover.long_image');

update xz_page_asset_slots set slot_key = 'profile.avatar', updated_at = now()
where slot_key = 'profile.default_avatar'
  and not exists (select 1 from xz_page_asset_slots target where target.tenant_id = xz_page_asset_slots.tenant_id and target.page_code = 'profile' and target.slot_key = 'profile.avatar');

insert into xz_page_asset_slots (id, tenant_id, page_code, module_code, slot_key, slot_name, sort_order, alt_text)
values
  ('slot_home_project_new_product','default','home','project','home.project.new_product','项目-新品上市',230,'新品上市'),
  ('slot_home_project_website','default','home','project','home.project.website','项目-企业官网',240,'企业官网')
on conflict (tenant_id, page_code, slot_key) do update set slot_name = excluded.slot_name, module_code = excluded.module_code, sort_order = excluded.sort_order, updated_at = now();
