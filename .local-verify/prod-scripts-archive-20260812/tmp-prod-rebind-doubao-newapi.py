#!/usr/bin/env python3
import json, subprocess, datetime

def psql(sql):
    return subprocess.check_output([
        "docker","exec","-i","zhiqiyun-ai-prod-postgres-1",
        "psql","-U","zhiqiyun_prod","-d","zhiqiyun","-v","ON_ERROR_STOP=1","-At"
    ], input=sql, universal_newlines=True)

TS = datetime.datetime.utcnow().strftime("%Y%m%d_%H%M%S")
MODEL = "doubao-seedance-2.0"
NEW_CHANNEL = "channel_newapi_grok_imagine"
NEW_PROVIDER = "NewAPI"

raw = psql("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config';")
data = json.loads(raw)

backup_id = "ai_capability_doubao_newapi_%s" % TS
backup_payload = json.dumps(data, ensure_ascii=False)
psql("""
INSERT INTO xz_ops_config_backups(id, source_table, source_id, raw, created_at)
VALUES (
  '%s',
  'xz_system_settings',
  'ai_capability_config',
  '%s'::jsonb,
  NOW()
);
""" % (backup_id.replace("'","''"), backup_payload.replace("'","''")))
print("BACKUP_OK", backup_id)

found = False
before = None
after = None
for m in data.get("aiModels") or []:
    name = m.get("modelName") or m.get("model_name") or ""
    if name != MODEL:
        continue
    found = True
    before = {
        "channelId": m.get("channelId") or m.get("channel_id"),
        "provider": m.get("provider"),
        "status": m.get("status"),
    }
    m["channelId"] = NEW_CHANNEL
    m["channel_id"] = NEW_CHANNEL
    m["provider"] = NEW_PROVIDER
    after = {
        "channelId": m.get("channelId"),
        "provider": m.get("provider"),
        "status": m.get("status"),
    }
    break

if not found:
    raise SystemExit("model not found: " + MODEL)

channels_raw = psql("SELECT raw::text FROM xz_api_channels WHERE id='%s';" % NEW_CHANNEL)
ch = json.loads(channels_raw)
models = list(ch.get("models") or [])
if MODEL not in models:
    models.append(MODEL)
    ch["models"] = models
    psql("UPDATE xz_api_channels SET raw = '%s'::jsonb WHERE id='%s';" % (
        json.dumps(ch, ensure_ascii=False).replace("'","''"), NEW_CHANNEL))
    print("CHANNEL_MODELS_UPDATED", models)
else:
    print("CHANNEL_MODELS_OK", models)

new_raw = json.dumps(data, ensure_ascii=False)
psql("UPDATE xz_system_settings SET raw = '%s'::jsonb, updated_at = NOW() WHERE id='ai_capability_config';" % new_raw.replace("'","''"))
print("BEFORE", json.dumps(before, ensure_ascii=False))
print("AFTER", json.dumps(after, ensure_ascii=False))

verify = json.loads(psql("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config';"))
for m in verify.get("aiModels") or []:
    name = m.get("modelName") or m.get("model_name") or ""
    if name == MODEL:
        print("VERIFY", name, m.get("channelId") or m.get("channel_id"), m.get("provider"), m.get("status"))
        break

print("FOUR_MODELS:")
wanted = [
    "grok-imagine-video-1.5-preview",
    "grok-imagine-1.5-video",
    "seedance-fast-2.0",
    "doubao-seedance-2.0",
]
by_name = {(m.get("modelName") or m.get("model_name") or ""): m for m in (verify.get("aiModels") or [])}
for name in wanted:
    m = by_name.get(name) or {}
    print(name, "->", m.get("channelId") or m.get("channel_id"), m.get("provider"), m.get("status"))
