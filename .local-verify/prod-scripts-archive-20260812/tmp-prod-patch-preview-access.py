#!/usr/bin/env python3
# -*- coding: utf-8 -*-
from __future__ import print_function
import json, subprocess, datetime, os

def run(cmd, data=None):
    p = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, universal_newlines=True)
    out, err = p.communicate(data)
    if p.returncode != 0:
        raise RuntimeError("cmd failed: %s\n%s\n%s" % (cmd, out, err))
    return out

def psql_at(sql):
    return run([
        "docker", "exec", "-i", "zhiqiyun-ai-prod-postgres-1",
        "psql", "-U", "zhiqiyun_prod", "-d", "zhiqiyun", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
    ]).strip()

wanted = ["grok-imagine-video-1.5-preview", "grok-imagine-1.5-video"]
raw = psql_at("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config'")
data = json.loads(raw)

for mod in data.get("aiModules") or []:
    if (mod.get("module_code") or mod.get("moduleCode")) != "video_generation":
        continue
    key = "bound_models" if "bound_models" in mod else "boundModels"
    models = list(mod.get(key) or [])
    for m in wanted:
        if m not in models:
            models.append(m)
    mod[key] = models

for lim in data.get("tenantModuleLimits") or []:
    if (lim.get("module_code") or lim.get("moduleCode")) != "video_generation":
        continue
    lj_key = "limit_json" if "limit_json" in lim else "limitJson"
    lj = lim.get(lj_key) or {}
    models = lj.get("models") or {}
    allowed = list(models.get("allowed") or [])
    for m in wanted:
        if m not in allowed:
            allowed.append(m)
    models["allowed"] = allowed
    lj["models"] = models
    lim[lj_key] = lj

ai_models = data.get("aiModels") or []
names = set([(m.get("modelName") or m.get("model_name") or "").lower() for m in ai_models])
now = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%S.%fZ")
if "grok-imagine-video-1.5-preview" not in names:
    ai_models.append({
        "id": "ai_model_grok_imagine_video_15_preview",
        "model_name": "grok-imagine-video-1.5-preview",
        "modelName": "grok-imagine-video-1.5-preview",
        "model_type": "video",
        "modelType": "video",
        "provider": "NewAPI",
        "capability_code": ["image_to_video"],
        "capabilityCode": ["image_to_video"],
        "module_code": "video_generation",
        "moduleCode": "video_generation",
        "status": "ACTIVE",
        "sort_weight": 14,
        "sortWeight": 14,
        "video_capabilities": {
            "supports_text_to_video": False,
            "supports_image_to_video": True,
            "supports_first_frame": True,
            "supports_last_frame": False,
            "max_reference_images": 1,
            "supported_durations": [10, 15],
            "supported_resolutions": ["720p", "1080p"],
            "supported_aspect_ratios": ["16:9", "9:16", "1:1", "4:3", "3:4"],
            "supported_parameters": ["duration", "resolution", "aspect_ratio", "image_urls"],
        },
        "created_at": now,
        "updated_at": now,
    })
    data["aiModels"] = ai_models

billing = data.get("billingRules") or []
bill_names = set([(b.get("modelName") or b.get("model_name") or "").lower() for b in billing])
if "grok-imagine-video-1.5-preview" not in bill_names:
    billing.append({
        "id": "billing_rule_video_grok_imagine_15_preview",
        "module_code": "video_generation",
        "moduleCode": "video_generation",
        "model_name": "grok-imagine-video-1.5-preview",
        "modelName": "grok-imagine-video-1.5-preview",
        "billing_type": "per_request",
        "billingType": "per_request",
        "base_price": 100,
        "basePrice": 100,
        "minimum_charge": 100,
        "minimumCharge": 100,
        "cost_price": 80,
        "costPrice": 80,
        "currency_type": "credit",
        "currencyType": "credit",
        "parameter_multiplier": {},
        "parameterMultiplier": {},
        "status": "ACTIVE",
        "created_at": now,
        "updated_at": now,
    })
    data["billingRules"] = billing

payload = json.dumps(data, ensure_ascii=False)
sql = "UPDATE xz_system_settings SET raw = $json$%s$json$::jsonb, updated_at = now() WHERE id='ai_capability_config';" % payload
print(run([
    "docker", "exec", "-i", "zhiqiyun-ai-prod-postgres-1",
    "psql", "-U", "zhiqiyun_prod", "-d", "zhiqiyun", "-v", "ON_ERROR_STOP=1", "-P", "pager=off",
], sql))

ch_raw = psql_at("SELECT raw::text FROM xz_api_channels WHERE id='channel_newapi_grok_imagine'")
ch = json.loads(ch_raw or "{}")
models = list(ch.get("models") or [])
for m in wanted:
    if m not in models:
        models.append(m)
ch["models"] = models
ch_sql = "UPDATE xz_api_channels SET raw = $json$%s$json$::jsonb WHERE id='channel_newapi_grok_imagine';" % json.dumps(ch, ensure_ascii=False)
print(run([
    "docker", "exec", "-i", "zhiqiyun-ai-prod-postgres-1",
    "psql", "-U", "zhiqiyun_prod", "-d", "zhiqiyun", "-v", "ON_ERROR_STOP=1", "-P", "pager=off",
], ch_sql))

print("has_preview", "grok-imagine-video-1.5-preview" in psql_at("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config'"))
print("channel_models", psql_at("SELECT raw->'models' FROM xz_api_channels WHERE id='channel_newapi_grok_imagine'"))
print("OK")
