#!/usr/bin/env python3
import subprocess, json

def at(sql):
    return subprocess.check_output([
        "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc", sql
    ], universal_newlines=True)

blob = json.loads(at("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config';"))
print("=== model doubao-seedance-2.0 ===")
for m in blob.get("aiModels") or []:
    name = m.get("modelName") or m.get("model_name") or ""
    if name != "doubao-seedance-2.0":
        continue
    print("channelId:", m.get("channelId") or m.get("channel_id"))
    print("provider:", m.get("provider"))
    print("status:", m.get("status"))

print("\n=== channels whose models include doubao-seedance-2.0 ===")
print(subprocess.check_output([
    "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-c",
    """
SELECT id,
       raw->>'name' AS name,
       raw->>'status' AS status,
       raw->>'baseUrl' AS base_url,
       raw->>'videoGenerationEndpoint' AS video_ep,
       raw->>'priority' AS priority,
       LEFT(COALESCE(raw->'models')::text, 260) AS models
FROM xz_api_channels
WHERE raw::text ILIKE '%doubao-seedance-2.0%'
ORDER BY COALESCE((raw->>'priority')::int, 999), id;
"""
], universal_newlines=True))
