#!/usr/bin/env python3
import subprocess, json

raw = subprocess.check_output([
    "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc",
    "SELECT raw::text FROM xz_system_settings WHERE id LIKE '%ai%' OR id LIKE '%capab%' OR id='default' ORDER BY id;"
], universal_newlines=True)

# find the capability blob
rows = subprocess.check_output([
    "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc",
    "SELECT id, length(raw::text) FROM xz_system_settings ORDER BY length(raw::text) DESC LIMIT 20;"
], universal_newlines=True)
print("settings sizes:\n", rows)

cap_id = subprocess.check_output([
    "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc",
    "SELECT id FROM xz_system_settings WHERE raw::text ILIKE '%grok-imagine-video-1.5-preview%' LIMIT 5;"
], universal_newlines=True).strip()
print("ids with preview:\n", cap_id)

if cap_id:
    first = cap_id.splitlines()[0]
    blob = subprocess.check_output([
        "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc",
        "SELECT raw::text FROM xz_system_settings WHERE id='%s';" % first.replace("'","''")
    ], universal_newlines=True)
    data = json.loads(blob)
    models = data.get("aiModels") or data.get("AIModels") or []
    print("=== ACTIVE video models ===")
    for m in models:
        name = m.get("modelName") or m.get("model_name") or ""
        module = m.get("moduleCode") or m.get("module_code") or ""
        status = m.get("status") or ""
        mtype = m.get("modelType") or m.get("model_type") or ""
        if "video" in module.lower() or mtype.lower()=="video" or "video" in name.lower() or "seedance" in name.lower() or "grok" in name.lower():
            caps = m.get("capabilityCode") or m.get("capability_code") or []
            print(name, "|", status, "|", module, "|", caps)

    print("=== billingRules video ===")
    for r in data.get("billingRules") or []:
        name = r.get("modelName") or r.get("model_name") or ""
        module = r.get("moduleCode") or r.get("module_code") or ""
        if "video" in module.lower() or "seedance" in name.lower() or "grok" in name.lower() or "video" in name.lower():
            print(name, "|", r.get("status"), "|", r.get("billingType") or r.get("billing_type"), "| base=", r.get("basePrice") or r.get("base_price"), "| min=", r.get("minimumCharge") or r.get("minimum_charge"), "| mult=", r.get("parameterMultiplier") or r.get("parameter_multiplier"))

    print("=== tenantModuleLimits video allowed ===")
    for lim in data.get("tenantModuleLimits") or []:
        module = lim.get("moduleCode") or lim.get("module_code") or ""
        if "video" not in module.lower():
            continue
        lj = lim.get("limitJSON") or lim.get("limit_json") or {}
        print(lim.get("id"), lim.get("status"), lj)
