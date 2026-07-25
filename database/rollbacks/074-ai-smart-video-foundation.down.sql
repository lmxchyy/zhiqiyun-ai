-- Rollback must be run only after confirming no smart-video data is needed.
begin;
drop table if exists video_render_tasks;
drop table if exists video_storyboard_scenes;
drop table if exists video_project_versions;
drop table if exists video_project_assets;
drop table if exists video_projects;
commit;
