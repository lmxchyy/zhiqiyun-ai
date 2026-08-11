-- Rollback 107: storage multipart upload.

begin;

drop table if exists xz_multipart_upload_parts;
drop table if exists xz_multipart_uploads;

commit;
