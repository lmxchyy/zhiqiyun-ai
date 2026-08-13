#!/usr/bin/env python3
import subprocess, json

def q(sql):
    return subprocess.check_output([
        "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-Atc", sql
    ], universal_newlines=True)

print("=== xz_api_channels for newapi/cme/seedance/grok ===")
print(subprocess.check_output([
    "docker","exec","zhiqiyun-ai-prod-postgres-1","psql","-U","zhiqiyun_prod","-d","zhiqiyun","-c",
    """
SELECT id,
       raw->>'name' AS name,
       raw->>'status' AS status,
       raw->>'baseUrl' AS base_url,
       raw->>'videoGenerationEndpoint' AS video_ep,
       raw->>'protocol' AS protocol,
       raw->>'priority' AS priority,
       LEFT(COALESCE(raw->'models', raw->'Models')::text, 300) AS models
FROM xz_api_channels
WHERE id ILIKE '%newapi%' OR id ILIKE '%cme%' OR id ILIKE '%seedance%' OR id ILIKE '%grok%'
   OR raw::text ILIKE '%seedance-fast%'
ORDER BY id;
"""
], universal_newlines=True))

print("=== channel_newapi_grok_imagine full (redact secrets) ===")
raw = q("SELECT raw::text FROM xz_api_channels WHERE id='channel_newapi_grok_imagine';")
if raw.strip():
    c = json.loads(raw)
    for k in ["id","name","status","protocol","baseUrl","base_url","videoGenerationEndpoint","video_generation_endpoint","priority","primary","models"]:
        if k in c:
            print(k, ":", c.get(k))
    # hide secrets
    for k,v in c.items():
        lk=k.lower()
        if any(x in lk for x in ["key","secret","token","password"]):
            print(k, ": <redacted>")

print("\n=== channel_newapi_gateway ===")
raw = q("SELECT raw::text FROM xz_api_channels WHERE id='channel_newapi_gateway';")
if raw.strip():
    c = json.loads(raw)
    for k in ["id","name","status","protocol","baseUrl","videoGenerationEndpoint","priority","models"]:
        print(k, ":", c.get(k))

print("\n=== channel_cmecloud_seedance ===")
raw = q("SELECT raw::text FROM xz_api_channels WHERE id='channel_cmecloud_seedance';")
if raw.strip():
    c = json.loads(raw)
    for k in ["id","name","status","protocol","baseUrl","videoGenerationEndpoint","priority","models"]:
        print(k, ":", c.get(k))
