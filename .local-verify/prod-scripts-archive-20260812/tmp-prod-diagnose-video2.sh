#!/usr/bin/env bash
set -uo pipefail
PSQL='docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off'
echo "=== columns ==="
$PSQL -c "SELECT column_name, data_type FROM information_schema.columns WHERE table_name='xz_generation_tasks' ORDER BY ordinal_position;"
echo "=== system_settings columns ==="
$PSQL -c "SELECT column_name FROM information_schema.columns WHERE table_name='xz_system_settings' ORDER BY ordinal_position;"
echo "=== sample task row keys via select * limit 0 ==="
$PSQL -c "SELECT * FROM xz_generation_tasks LIMIT 0;"
