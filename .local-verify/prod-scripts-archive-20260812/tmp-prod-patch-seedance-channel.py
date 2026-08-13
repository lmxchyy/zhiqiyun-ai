#!/usr/bin/env python3
# -*- coding: utf-8 -*-
from __future__ import print_function
import json, subprocess

def psql(sql):
    return subprocess.check_output([
        "docker","exec","-i","zhiqiyun-ai-prod-postgres-1",
        "psql","-U","zhiqiyun_prod","-d","zhiqiyun","-v","ON_ERROR_STOP=1","-At","-c",sql
    ], universal_newlines=True).strip()

def run_sql(sql):
    p = subprocess.Popen([
        "docker","exec","-i","zhiqiyun-ai-prod-postgres-1",
        "psql","-U","zhiqiyun_prod","-d","zhiqiyun","-v","ON_ERROR_STOP=1","-P","pager=off"
    ], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
    out, err = p.communicate(sql)
    if p.returncode != 0:
        raise RuntimeError(err or out)
    print(out)

# backup
run_sql("""
INSERT INTO xz_ops_config_backups(id, source_table, source_id, raw)
SELECT 'api_channel_newapi_grok_' || to_char(now(), 'YYYYMMDDHH24MISS'), 'xz_api_channels', id, raw
FROM xz_api_channels WHERE id='channel_newapi_grok_imagine';
INSERT INTO xz_ops_config_backups(id, source_table, source_id, raw)
SELECT 'ai_capability_seedance_' || to_char(now(), 'YYYYMMDDHH24MISS'), 'xz_system_settings', id, raw
FROM xz_system_settings WHERE id='ai_capability_config';
""")

# Add seedance-fast-2.0 (+ doubao) onto NewAPI grok channel so Seedance 2.0 UI model can route
ch = json.loads(psql("SELECT raw::text FROM xz_api_channels WHERE id='channel_newapi_grok_imagine'") or "{}")
models = list(ch.get("models") or [])
for m in ["grok-imagine-1.5-video", "grok-imagine-video-1.5-preview", "seedance-fast-2.0", "doubao-seedance-2.0"]:
    if m not in models:
        models.append(m)
ch["models"] = models
run_sql("UPDATE xz_api_channels SET raw = $json$%s$json$::jsonb WHERE id='channel_newapi_grok_imagine';" % json.dumps(ch, ensure_ascii=False))

# Ensure seedance-fast model has channel_id pointing to newapi grok channel when empty
data = json.loads(psql("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config'"))
changed = False
for m in data.get("aiModels") or []:
    name = m.get("model_name") or m.get("modelName") or ""
    if name == "seedance-fast-2.0":
        if not (m.get("channel_id") or m.get("channelId")):
            m["channel_id"] = "channel_newapi_grok_imagine"
            m["channelId"] = "channel_newapi_grok_imagine"
            changed = True
    if name == "grok-imagine-video-1.5-preview":
        if not (m.get("channel_id") or m.get("channelId")):
            m["channel_id"] = "channel_newapi_grok_imagine"
            m["channelId"] = "channel_newapi_grok_imagine"
            changed = True
if changed:
    run_sql("UPDATE xz_system_settings SET raw = $json$%s$json$::jsonb, updated_at=now() WHERE id='ai_capability_config';" % json.dumps(data, ensure_ascii=False))

print("channel_models", psql("SELECT raw->'models' FROM xz_api_channels WHERE id='channel_newapi_grok_imagine'"))
print("seedance_channel", psql("SELECT elem->>'model_name', coalesce(elem->>'channel_id','') FROM xz_system_settings, jsonb_array_elements(raw->'aiModels') elem WHERE id='ai_capability_config' AND elem->>'model_name' IN ('seedance-fast-2.0','grok-imagine-video-1.5-preview','doubao-seedance-2.0','grok-imagine-1.5-video')"))
print("OK")
