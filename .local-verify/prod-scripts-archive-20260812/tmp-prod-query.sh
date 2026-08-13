#!/bin/bash
set -eu
# Check what estimate would compute if we can hit health/logs; also dump cfg billing from app if possible
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "SELECT count(*) AS versions FROM xz_billing_rule_versions; SELECT count(*) AS published FROM xz_billing_rule_versions WHERE status='PUBLISHED';"
# Look for platform config billing_rules in any table
docker exec zhiqiyun-ai-prod-postgres-1 psql -U zhiqiyun_prod -d zhiqiyun -P pager=off -c "SELECT tablename FROM pg_tables WHERE schemaname='public' AND (tablename ILIKE '%platform%' OR tablename ILIKE '%admin%' OR tablename ILIKE '%config%' OR tablename ILIKE '%store%') ORDER BY 1;"