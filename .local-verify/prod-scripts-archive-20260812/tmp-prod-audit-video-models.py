#!/usr/bin/env python3
# -*- coding: utf-8 -*-
from __future__ import print_function
import json, subprocess

def psql(sql):
    return subprocess.check_output([
        "docker", "exec", "-i", "zhiqiyun-ai-prod-postgres-1",
        "psql", "-U", "zhiqiyun_prod", "-d", "zhiqiyun", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
    ], universal_newlines=True).strip()

data = json.loads(psql("SELECT raw::text FROM xz_system_settings WHERE id='ai_capability_config'"))

# video models from aiModels
video_models = []
for m in data.get("aiModels") or []:
    module = (m.get("module_code") or m.get("moduleCode") or "").lower()
    mtype = (m.get("model_type") or m.get("modelType") or "").lower()
    name = m.get("model_name") or m.get("modelName") or ""
    if not name:
        continue
    if module == "video_generation" or mtype == "video" or "video" in name.lower() or "seedance" in name.lower() or "grok" in name.lower():
        video_models.append({
            "name": name,
            "status": m.get("status"),
            "provider": m.get("provider"),
            "channel_id": m.get("channel_id") or m.get("channelId") or "",
        })

# bound models
bound = []
for mod in data.get("aiModules") or []:
    if (mod.get("module_code") or mod.get("moduleCode")) == "video_generation":
        bound = list(mod.get("bound_models") or mod.get("boundModels") or [])

# limits
limits = []
for lim in data.get("tenantModuleLimits") or []:
    if (lim.get("module_code") or lim.get("moduleCode")) != "video_generation":
        continue
    lj = lim.get("limit_json") or lim.get("limitJson") or {}
    allowed = list(((lj.get("models") or {}).get("allowed")) or [])
    limits.append({
        "id": lim.get("id"),
        "package": lim.get("package_id") or lim.get("packageId") or "<default>",
        "allowed": allowed,
    })

# billing
billing = set()
for b in data.get("billingRules") or []:
    if (b.get("module_code") or b.get("moduleCode")) == "video_generation":
        billing.add((b.get("model_name") or b.get("modelName") or "").strip())

# channels
ch_rows = psql("SELECT id || E'\\t' || coalesce(raw->>'models','[]') FROM xz_api_channels ORDER BY id").splitlines()
channels = []
for row in ch_rows:
    if not row.strip():
        continue
    cid, models_json = row.split("\t", 1)
    try:
        models = json.loads(models_json)
    except Exception:
        models = []
    channels.append({"id": cid, "models": models})

names = sorted(set([m["name"] for m in video_models] + bound))
print("VIDEO_MODELS")
for m in sorted(video_models, key=lambda x: x["name"]):
    print("MODEL\t%s\tstatus=%s\tprovider=%s\tchannel_id=%s" % (m["name"], m["status"], m["provider"], m["channel_id"]))

print("\nBOUND\t%s" % ",".join(bound))
print("\nBILLING\t%s" % ",".join(sorted([x for x in billing if x])))

print("\nLIMITS")
for lim in limits:
    print("LIMIT\t%s\t%s" % (lim["package"], ",".join(lim["allowed"])))

print("\nCHANNELS")
for ch in channels:
    if any(("seedance" in x.lower()) or ("grok" in x.lower()) or ("video" in x.lower()) or ("veo" in x.lower()) or ("kling" in x.lower()) for x in ch["models"]) or "newapi" in ch["id"] or "seedance" in ch["id"] or "cme" in ch["id"] or "moxing" in ch["id"]:
        print("CHANNEL\t%s\t%s" % (ch["id"], ",".join(ch["models"])))

print("\nAUDIT")
for name in names:
    in_bound = name in bound
    in_billing = name in billing
    channel_hits = [c["id"] for c in channels if name in c["models"]]
    missing_limits = []
    for lim in limits:
        if lim["allowed"] and name not in lim["allowed"]:
            missing_limits.append(lim["package"])
    risk = []
    if not in_bound:
        risk.append("not_bound")
    if missing_limits:
        risk.append("blocked_by_limits:" + "|".join(missing_limits))
    if not channel_hits and name != "mock-video":
        risk.append("no_channel")
    if not in_billing and name != "mock-video":
        risk.append("no_billing_rule")
    status = "OK" if not risk else "RISK"
    print("%s\t%s\tbound=%s\tbilling=%s\tchannels=%s\t%s" % (
        status, name, in_bound, in_billing, ",".join(channel_hits) or "-", ";".join(risk) or "clean"
    ))
