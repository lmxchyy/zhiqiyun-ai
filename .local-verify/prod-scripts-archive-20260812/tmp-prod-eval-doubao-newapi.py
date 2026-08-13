#!/usr/bin/env python3
import subprocess, json

def at(sql):
    return subprocess.check_output([
        "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc", sql
    ], universal_newlines=True)

blob = json.loads(at("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config';"))
for m in blob.get("aiModels") or []:
    name = m.get("modelName") or m.get("model_name") or ""
    if name not in ("doubao-seedance-2.0", "seedance-fast-2.0"):
        continue
    print("MODEL", name)
    print("  channelId:", m.get("channelId") or m.get("channel_id"))
    print("  provider:", m.get("provider"))
    caps = m.get("videoCapabilities") or m.get("video_capabilities") or {}
    print("  video_capabilities:", json.dumps(caps, ensure_ascii=False)[:500])
    print("  capabilityCode:", m.get("capabilityCode") or m.get("capability_code"))

print("\n=== channels ===")
for cid in ("channel_newapi_grok_imagine", "channel_moxing_seedance", "channel_cmecloud_seedance"):
    raw = at("SELECT raw::text FROM xz_api_channels WHERE id='%s';" % cid)
    if not raw.strip():
        print(cid, "MISSING")
        continue
    c = json.loads(raw)
    print(cid)
    print("  status:", c.get("status"))
    print("  baseUrl:", c.get("baseUrl"))
    print("  videoEp:", c.get("videoGenerationEndpoint"))
    print("  models:", c.get("models"))
    print("  protocol:", c.get("protocol"))
    print("  hasApiKeyEnv:", bool(c.get("apiKeyEnv")))
