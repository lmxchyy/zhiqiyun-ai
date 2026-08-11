#!/bin/bash
set -euo pipefail
cd /opt/zhiqiyun-ai
echo "=== env file V2 lines ==="
grep -nE 'SNAPSHOT_V2|PRICE_PLAN_MEMBER|PRICE_PLAN_TEST|WECHAT_VIRTUAL_PAY_ENV' .env.production || echo 'NONE_IN_FILE'
echo "=== compose mentions ==="
grep -nE 'SNAPSHOT_V2|PRICE_PLAN_MEMBER|PRICE_PLAN_TEST|WECHAT_VIRTUAL_PAY_ENV' compose.prod.yml | sed -n '1,40p'
echo "=== container ==="
docker exec zhiqiyun-ai-prod-xianzhi-ai-1 printenv \
  SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED \
  PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED \
  PRICE_PLAN_TEST_ENTRY_ENABLED \
  WECHAT_VIRTUAL_PAY_ENV
echo "=== backups recent ==="
ls -lt backups/compose/.env.production.* 2>/dev/null | sed -n '1,8p' || true
