-- PPT Agent Phase 1: migrate only the exact legacy table after tenant ownership is proven.
BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '45s';

SELECT pg_advisory_xact_lock(hashtextextended('migration:106:ppt-agent-phase1', 0));
LOCK TABLE public.xz_ppt_tasks IN ACCESS EXCLUSIVE MODE;

DO $migration$
DECLARE
  legacy_column_signature CONSTANT text :=
    'client_request_id|character varying(256)|1|'''';' ||
    'created_at|timestamp with time zone|1|;' ||
    'raw|jsonb|1|;' ||
    'status|character varying(32)|1|;' ||
    'task_id|character varying(128)|1|;' ||
    'updated_at|timestamp with time zone|1|;' ||
    'user_id|character varying(128)|1|';
  final_column_signature CONSTANT text :=
    'client_request_id|character varying(256)|1|'''';' ||
    'created_at|timestamp with time zone|1|;' ||
    'raw|jsonb|1|;' ||
    'session_id|character varying(128)|0|;' ||
    'skill_code|character varying(64)|1|'''';' ||
    'source_file_ids|jsonb|1|''[]'';' ||
    'stage|character varying(32)|1|''draft'';' ||
    'status|character varying(32)|1|;' ||
    'task_id|character varying(128)|1|;' ||
    'tenant_id|character varying(128)|1|;' ||
    'updated_at|timestamp with time zone|1|;' ||
    'user_id|character varying(128)|1|';
  legacy_index_signature CONSTANT text :=
    'idx_xz_ppt_tasks_user_created|0|0|1|1|btree|1|1|user_id,created_at|asc,desc|;' ||
    'idx_xz_ppt_tasks_user_status|0|0|1|1|btree|1|1|user_id,status|asc,asc|;' ||
    'uk_xz_ppt_tasks_user_client_request|1|0|1|1|btree|1|1|user_id,client_request_id|asc,asc|client_request_id<>'''';' ||
    'xz_ppt_tasks_pkey|1|1|1|1|btree|1|1|task_id|asc|';
  final_index_signature CONSTANT text :=
    'idx_xz_ppt_tasks_tenant_user_client_request|1|0|1|1|btree|1|1|tenant_id,user_id,client_request_id|asc,asc,asc|client_request_id<>'''';' ||
    'idx_xz_ppt_tasks_tenant_user_session|0|0|1|1|btree|1|1|tenant_id,user_id,session_id|asc,asc,asc|session_idisnotnull;' ||
    'idx_xz_ppt_tasks_tenant_user_stage_updated|0|0|1|1|btree|1|1|tenant_id,user_id,stage,updated_at|asc,asc,asc,desc|;' ||
    'xz_ppt_tasks_pkey|1|1|1|1|btree|1|1|task_id|asc|';
  legacy_constraint_signature CONSTANT text := 'xz_ppt_tasks_pkey|p|1|';
  final_constraint_signature CONSTANT text :=
    'ck_xz_ppt_tasks_session_nonblank|c|1|((session_idisnull)or(btrim((session_id))<>''''));' ||
    'ck_xz_ppt_tasks_source_file_ids_array|c|1|(jsonb_typeof(source_file_ids)=''array'');' ||
    'ck_xz_ppt_tasks_stage_status|c|1|((((stage)=''draft'')and((status)=''pending''))or(((stage)=''outline_ready'')and((status)=''pending''))or(((stage)=''generating'')and((status)=''processing''))or(((stage)=''ready'')and((status)=''success''))or(((stage)=''failed'')and((status)=''failed''))or(((stage)=''cancelled'')and((status)=''cancelled'')));' ||
    'ck_xz_ppt_tasks_tenant_nonblank|c|1|(btrim((tenant_id))<>'''');' ||
    'xz_ppt_tasks_pkey|p|1|';
  column_signature text;
  index_signature text;
  constraint_signature text;
  is_legacy boolean;
  is_final boolean;
  invalid_count bigint;
BEGIN
  SELECT coalesce(string_agg(
           a.attname || '|' || format_type(a.atttypid, a.atttypmod) || '|' ||
           CASE WHEN a.attnotnull THEN '1' ELSE '0' END || '|' ||
           regexp_replace(
             replace(replace(lower(coalesce(pg_get_expr(d.adbin, d.adrelid), '')), '::character varying', ''), '::jsonb', ''),
             '[[:space:]()]', '', 'g'
           ),
           ';' ORDER BY a.attname
         ), '')
  INTO column_signature
  FROM pg_catalog.pg_attribute a
  JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
  WHERE n.nspname = 'public'
    AND c.relname = 'xz_ppt_tasks'
    AND c.relkind IN ('r', 'p')
    AND a.attnum > 0
    AND NOT a.attisdropped;

  SELECT coalesce(string_agg(
           q.index_name || '|' || q.is_unique || '|' || q.is_primary || '|' ||
           q.is_valid || '|' || q.is_ready || '|' || q.access_method || '|' ||
           q.no_included_columns || '|' || q.no_expressions || '|' || q.key_columns || '|' ||
           q.key_orders || '|' || q.predicate,
           ';' ORDER BY q.index_name
         ), '')
  INTO index_signature
  FROM (
    SELECT index_rel.relname AS index_name,
           CASE WHEN i.indisunique THEN '1' ELSE '0' END AS is_unique,
           CASE WHEN i.indisprimary THEN '1' ELSE '0' END AS is_primary,
           CASE WHEN i.indisvalid THEN '1' ELSE '0' END AS is_valid,
           CASE WHEN i.indisready THEN '1' ELSE '0' END AS is_ready,
           am.amname AS access_method,
           CASE WHEN i.indnkeyatts = i.indnatts THEN '1' ELSE '0' END AS no_included_columns,
           CASE WHEN i.indexprs IS NULL THEN '1' ELSE '0' END AS no_expressions,
           array_to_string(ARRAY(
             SELECT a.attname
             FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
             JOIN pg_catalog.pg_attribute a
               ON a.attrelid = i.indrelid AND a.attnum = key.attnum
             WHERE key.position <= i.indnkeyatts
             ORDER BY key.position
           ), ',') AS key_columns,
           array_to_string(ARRAY(
             SELECT CASE WHEN (i.indoption[key.position - 1] & 1) <> 0 THEN 'desc' ELSE 'asc' END
             FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
             WHERE key.position <= i.indnkeyatts
             ORDER BY key.position
           ), ',') AS key_orders,
           regexp_replace(
             replace(replace(lower(coalesce(pg_get_expr(i.indpred, i.indrelid), '')), '::text', ''), '"', ''),
             '[[:space:]()]', '', 'g'
           ) AS predicate
    FROM pg_catalog.pg_index i
    JOIN pg_catalog.pg_class table_rel ON table_rel.oid = i.indrelid
    JOIN pg_catalog.pg_namespace n ON n.oid = table_rel.relnamespace
    JOIN pg_catalog.pg_class index_rel ON index_rel.oid = i.indexrelid
    JOIN pg_catalog.pg_am am ON am.oid = index_rel.relam
    WHERE n.nspname = 'public' AND table_rel.relname = 'xz_ppt_tasks'
  ) q;

  SELECT coalesce(string_agg(
           con.conname || '|' || con.contype::text || '|' ||
           CASE WHEN con.convalidated THEN '1' ELSE '0' END || '|' ||
           regexp_replace(
             replace(replace(replace(lower(coalesce(pg_get_expr(con.conbin, con.conrelid), '')), '::character varying', ''), '::text', ''), '"', ''),
             '[[:space:]]', '', 'g'
           ),
           ';' ORDER BY con.conname
         ), '')
  INTO constraint_signature
  FROM pg_catalog.pg_constraint con
  WHERE con.conrelid = 'public.xz_ppt_tasks'::regclass
    AND con.contype IN ('p', 'u', 'c', 'f', 'x');

  is_legacy := column_signature = legacy_column_signature
    AND index_signature = legacy_index_signature
    AND constraint_signature = legacy_constraint_signature;
  is_final := column_signature = final_column_signature
    AND index_signature = final_index_signature
    AND constraint_signature = final_constraint_signature;

  IF is_final THEN
    SELECT count(*) INTO invalid_count
    FROM public.xz_ppt_tasks task
    WHERE btrim(task.tenant_id) = ''
       OR jsonb_typeof(task.source_file_ids) <> 'array'
       OR (task.session_id IS NOT NULL AND btrim(task.session_id) = '')
       OR NOT (
         (task.stage = 'DRAFT' AND task.status = 'pending') OR
         (task.stage = 'OUTLINE_READY' AND task.status = 'pending') OR
         (task.stage = 'GENERATING' AND task.status = 'processing') OR
         (task.stage = 'READY' AND task.status = 'success') OR
         (task.stage = 'FAILED' AND task.status = 'failed') OR
         (task.stage = 'CANCELLED' AND task.status = 'cancelled')
       );
    IF invalid_count <> 0 THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'migration 106 exact-final table contains invalid PPT rows';
    END IF;
    RETURN;
  END IF;

  IF NOT is_legacy THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'migration 106 requires the exact legacy or exact final xz_ppt_tasks schema';
  END IF;

  SELECT count(*) INTO invalid_count
  FROM public.xz_ppt_tasks task
  WHERE task.status NOT IN ('success', 'failed', 'cancelled')
     OR jsonb_typeof(task.raw) <> 'object'
     OR task.raw->>'taskId' IS DISTINCT FROM task.task_id
     OR task.raw->>'status' IS DISTINCT FROM task.status;
  IF invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'migration 106 refuses active, unknown, or malformed legacy PPT tasks';
  END IF;

  CREATE TEMP TABLE migration_106_tenant_projection (
    task_id varchar(128) PRIMARY KEY,
    tenant_id text NOT NULL
  ) ON COMMIT DROP;

  INSERT INTO migration_106_tenant_projection(task_id, tenant_id)
  WITH tenant_evidence AS (
    SELECT task.task_id, btrim(event.tenant_id) AS tenant_id
    FROM public.xz_ppt_tasks task
    JOIN public.xz_billing_events event
      ON event.task_id = task.task_id
     AND event.user_id = task.user_id
    WHERE event.metric_code = 'ppt.generations'
      AND event.tenant_id IS NOT NULL
      AND btrim(event.tenant_id) <> ''

    UNION

    SELECT task.task_id, btrim(generation.tenant_id) AS tenant_id
    FROM public.xz_ppt_tasks task
    JOIN public.xz_generation_tasks generation
      ON generation.user_id = task.user_id
     AND generation.client_request_id = task.client_request_id
    WHERE generation.module_code = 'ppt_generation'
      AND generation.type = 'PPT_GENERATION'
      AND generation.params->>'source_type' = 'feishu'
      AND btrim(task.client_request_id) <> ''
      AND btrim(generation.client_request_id) <> ''
      AND btrim(generation.params->>'source_task_id') <> ''
      AND generation.params->>'source_task_id' = task.client_request_id
      AND generation.tenant_id IS NOT NULL
      AND btrim(generation.tenant_id) <> ''
  ), resolved AS (
    SELECT task.task_id,
           min(evidence.tenant_id) AS tenant_id,
           count(DISTINCT evidence.tenant_id) AS tenant_count
    FROM public.xz_ppt_tasks task
    LEFT JOIN tenant_evidence evidence ON evidence.task_id = task.task_id
    GROUP BY task.task_id
  )
  SELECT task_id, tenant_id
  FROM resolved
  WHERE tenant_count = 1;

  SELECT count(*) INTO invalid_count
  FROM public.xz_ppt_tasks task
  LEFT JOIN migration_106_tenant_projection projection ON projection.task_id = task.task_id
  LEFT JOIN public.xz_tenants tenant ON tenant.id = projection.tenant_id
  WHERE projection.task_id IS NULL
     OR length(projection.tenant_id) > 128
     OR tenant.id IS NULL;
  IF invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '23503', MESSAGE = 'migration 106 requires exactly one existing authoritative tenant for every PPT task';
  END IF;

  SELECT count(*) INTO invalid_count
  FROM (
    SELECT projection.tenant_id, task.user_id, task.client_request_id
    FROM public.xz_ppt_tasks task
    JOIN migration_106_tenant_projection projection ON projection.task_id = task.task_id
    WHERE task.client_request_id <> ''
    GROUP BY projection.tenant_id, task.user_id, task.client_request_id
    HAVING count(*) > 1
  ) duplicate_request;
  IF invalid_count <> 0 THEN
    RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'migration 106 projected tenant/user/client request is not unique';
  END IF;

  ALTER TABLE public.xz_ppt_tasks
    ADD COLUMN tenant_id varchar(128),
    ADD COLUMN session_id varchar(128),
    ADD COLUMN skill_code varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN stage varchar(32) NOT NULL DEFAULT 'DRAFT',
    ADD COLUMN source_file_ids jsonb NOT NULL DEFAULT '[]'::jsonb;

  UPDATE public.xz_ppt_tasks task
  SET tenant_id = projection.tenant_id,
      skill_code = 'general',
      stage = CASE task.status
        WHEN 'success' THEN 'READY'
        WHEN 'failed' THEN 'FAILED'
        WHEN 'cancelled' THEN 'CANCELLED'
      END
  FROM migration_106_tenant_projection projection
  WHERE projection.task_id = task.task_id;

  ALTER TABLE public.xz_ppt_tasks
    ALTER COLUMN tenant_id SET NOT NULL,
    ADD CONSTRAINT ck_xz_ppt_tasks_tenant_nonblank
      CHECK (btrim(tenant_id) <> ''),
    ADD CONSTRAINT ck_xz_ppt_tasks_session_nonblank
      CHECK (session_id IS NULL OR btrim(session_id) <> ''),
    ADD CONSTRAINT ck_xz_ppt_tasks_source_file_ids_array
      CHECK (jsonb_typeof(source_file_ids) = 'array'),
    ADD CONSTRAINT ck_xz_ppt_tasks_stage_status
      CHECK (
        (stage = 'DRAFT' AND status = 'pending') OR
        (stage = 'OUTLINE_READY' AND status = 'pending') OR
        (stage = 'GENERATING' AND status = 'processing') OR
        (stage = 'READY' AND status = 'success') OR
        (stage = 'FAILED' AND status = 'failed') OR
        (stage = 'CANCELLED' AND status = 'cancelled')
      );

  CREATE UNIQUE INDEX idx_xz_ppt_tasks_tenant_user_client_request
    ON public.xz_ppt_tasks(tenant_id, user_id, client_request_id)
    WHERE client_request_id <> '';
  CREATE INDEX idx_xz_ppt_tasks_tenant_user_session
    ON public.xz_ppt_tasks(tenant_id, user_id, session_id)
    WHERE session_id IS NOT NULL;
  CREATE INDEX idx_xz_ppt_tasks_tenant_user_stage_updated
    ON public.xz_ppt_tasks(tenant_id, user_id, stage, updated_at DESC);

  DROP INDEX public.uk_xz_ppt_tasks_user_client_request;
  DROP INDEX public.idx_xz_ppt_tasks_user_created;
  DROP INDEX public.idx_xz_ppt_tasks_user_status;

  SELECT coalesce(string_agg(
           a.attname || '|' || format_type(a.atttypid, a.atttypmod) || '|' ||
           CASE WHEN a.attnotnull THEN '1' ELSE '0' END || '|' ||
           regexp_replace(
             replace(replace(lower(coalesce(pg_get_expr(d.adbin, d.adrelid), '')), '::character varying', ''), '::jsonb', ''),
             '[[:space:]()]', '', 'g'
           ),
           ';' ORDER BY a.attname
         ), '')
  INTO column_signature
  FROM pg_catalog.pg_attribute a
  JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
  WHERE n.nspname = 'public' AND c.relname = 'xz_ppt_tasks'
    AND c.relkind IN ('r', 'p') AND a.attnum > 0 AND NOT a.attisdropped;

  SELECT coalesce(string_agg(
           q.index_name || '|' || q.is_unique || '|' || q.is_primary || '|' ||
           q.is_valid || '|' || q.is_ready || '|' || q.access_method || '|' ||
           q.no_included_columns || '|' || q.no_expressions || '|' || q.key_columns || '|' ||
           q.key_orders || '|' || q.predicate,
           ';' ORDER BY q.index_name
         ), '')
  INTO index_signature
  FROM (
    SELECT index_rel.relname AS index_name,
           CASE WHEN i.indisunique THEN '1' ELSE '0' END AS is_unique,
           CASE WHEN i.indisprimary THEN '1' ELSE '0' END AS is_primary,
           CASE WHEN i.indisvalid THEN '1' ELSE '0' END AS is_valid,
           CASE WHEN i.indisready THEN '1' ELSE '0' END AS is_ready,
           am.amname AS access_method,
           CASE WHEN i.indnkeyatts = i.indnatts THEN '1' ELSE '0' END AS no_included_columns,
           CASE WHEN i.indexprs IS NULL THEN '1' ELSE '0' END AS no_expressions,
           array_to_string(ARRAY(
             SELECT a.attname
             FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
             JOIN pg_catalog.pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = key.attnum
             WHERE key.position <= i.indnkeyatts ORDER BY key.position
           ), ',') AS key_columns,
           array_to_string(ARRAY(
             SELECT CASE WHEN (i.indoption[key.position - 1] & 1) <> 0 THEN 'desc' ELSE 'asc' END
             FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS key(attnum, position)
             WHERE key.position <= i.indnkeyatts ORDER BY key.position
           ), ',') AS key_orders,
           regexp_replace(
             replace(replace(lower(coalesce(pg_get_expr(i.indpred, i.indrelid), '')), '::text', ''), '"', ''),
             '[[:space:]()]', '', 'g'
           ) AS predicate
    FROM pg_catalog.pg_index i
    JOIN pg_catalog.pg_class table_rel ON table_rel.oid = i.indrelid
    JOIN pg_catalog.pg_namespace n ON n.oid = table_rel.relnamespace
    JOIN pg_catalog.pg_class index_rel ON index_rel.oid = i.indexrelid
    JOIN pg_catalog.pg_am am ON am.oid = index_rel.relam
    WHERE n.nspname = 'public' AND table_rel.relname = 'xz_ppt_tasks'
  ) q;

  SELECT coalesce(string_agg(
           con.conname || '|' || con.contype::text || '|' ||
           CASE WHEN con.convalidated THEN '1' ELSE '0' END || '|' ||
           regexp_replace(
             replace(replace(replace(lower(coalesce(pg_get_expr(con.conbin, con.conrelid), '')), '::character varying', ''), '::text', ''), '"', ''),
             '[[:space:]]', '', 'g'
           ),
           ';' ORDER BY con.conname
         ), '')
  INTO constraint_signature
  FROM pg_catalog.pg_constraint con
  WHERE con.conrelid = 'public.xz_ppt_tasks'::regclass
    AND con.contype IN ('p', 'u', 'c', 'f', 'x');

  IF column_signature <> final_column_signature
     OR index_signature <> final_index_signature
     OR constraint_signature <> final_constraint_signature THEN
    RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'migration 106 failed to produce the exact final xz_ppt_tasks schema';
  END IF;
END
$migration$;

COMMIT;
