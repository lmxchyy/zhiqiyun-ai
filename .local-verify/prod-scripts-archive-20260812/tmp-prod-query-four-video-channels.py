#!/usr/bin/env python3
import subprocess, json

def at(sql):
    return subprocess.check_output([
        "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc", sql
    ], universal_newlines=True)

wanted = [
    "grok-imagine-video-1.5-preview",
    "grok-imagine-1.5-video",
    "seedance-fast-2.0",
    "doubao-seedance-2.0",
]

blob = json.loads(at("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config';"))
models = { (m.get("modelName") or m.get("model_name") or ""): m for m in (blob.get("aiModels") or []) }

channels = {}
raw_rows = at("SELECT id || '|||' || raw::text FROM xz_api_channels;")
for line in raw_rows.splitlines():
    if "|||" not in line:
        continue
    cid, raw = line.split("|||", 1)
    try:
        channels[cid] = json.loads(raw)
    except Exception:
        pass

print("model\tstatus\tchannelId\tchannelName\tchannelStatus\tbaseUrl\tvideoEndpoint")
for name in wanted:
    m = models.get(name) or {}
    cid = m.get("channelId") or m.get("channel_id") or ""
    c = channels.get(cid) or {}
    print("\t".join([
        name,
        str(m.get("status") or ""),
        cid or "(未绑定)",
        str(c.get("name") or ""),
        str(c.get("status") or ""),
        str(c.get("baseUrl") or c.get("base_url") or ""),
        str(c.get("videoGenerationEndpoint") or c.get("video_generation_endpoint") or ""),
    ]))
