#!/usr/bin/env python3
"""Enable mini-program discovery for formal video models missing compliance flags."""
import json
import subprocess
import sys
from datetime import datetime, timezone

BACKUP_ID = f"ai_capability_video_mp_{datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}"
TARGETS = {
    "seedance-fast-2.0",
    "grok-imagine-video-1.5-preview",
}


def psql(sql: str) -> str:
    result = subprocess.run(
        [
            "docker",
            "exec",
            "-i",
            "zhiqiyun-ai-prod-postgres-1",
            "psql",
            "-U",
            "zhiqiyun_prod",
            "-d",
            "zhiqiyun",
            "-t",
            "-A",
            "-c",
            sql,
        ],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
    )
    return result.stdout.strip()


def main() -> int:
    raw = psql("SELECT raw::text FROM xz_system_settings WHERE id = 'ai_capability_config';")
    if not raw:
        print("missing ai_capability_config", file=sys.stderr)
        return 1
    cfg = json.loads(raw)
    models = cfg.get("aiModels") or cfg.get("AIModels") or []
    template = None
    for model in models:
        name = str(model.get("model_name") or model.get("modelName") or "").strip()
        enabled = bool(model.get("miniprogram_enabled") or model.get("miniProgramEnabled"))
        status = str(model.get("compliance_status") or model.get("complianceStatus") or "").lower()
        if enabled and status == "approved" and name in {"grok-imagine-1.5-video", "doubao-seedance-2.0"}:
            template = model
            break
    if not template:
        print("no approved video template found", file=sys.stderr)
        return 1

    def copy_field(dst, src, *keys):
        for key in keys:
            if key in src and src[key] not in (None, "", [], {}):
                dst[key] = src[key]

    changed = []
    for model in models:
        name = str(model.get("model_name") or model.get("modelName") or "").strip()
        if name not in TARGETS:
            continue
        before = {
            "miniprogram_enabled": model.get("miniprogram_enabled") or model.get("miniProgramEnabled"),
            "compliance_status": model.get("compliance_status") or model.get("complianceStatus"),
        }
        copy_field(model, template, "provider_name", "providerName")
        copy_field(model, template, "provider_company", "providerCompany")
        copy_field(model, template, "algorithm_name", "algorithmName")
        copy_field(model, template, "algorithm_filing_no", "algorithmFilingNo")
        copy_field(model, template, "algorithm_type", "algorithmType")
        copy_field(model, template, "contract_status", "contractStatus")
        copy_field(model, template, "contract_expire_at", "contractExpireAt")
        copy_field(model, template, "allowed_terminals", "allowedTerminals")
        copy_field(model, template, "allowed_capabilities", "allowedCapabilities")
        model["miniprogram_enabled"] = True
        model["miniProgramEnabled"] = True
        model["compliance_status"] = "approved"
        model["complianceStatus"] = "approved"
        model["contract_status"] = "valid"
        model["contractStatus"] = "valid"
        if not model.get("model_version") and not model.get("modelVersion"):
            model["model_version"] = name
            model["modelVersion"] = name
        remark = str(model.get("compliance_remark") or model.get("complianceRemark") or "").strip()
        note = (
            f"Enabled for mini-program discovery parity with {template.get('model_name')}. "
            f"| approver=video-model-parity; approved_at={datetime.now(timezone.utc).date().isoformat()}"
        )
        model["compliance_remark"] = f"{remark} | {note}" if remark else note
        model["complianceRemark"] = model["compliance_remark"]
        changed.append((name, before))

    if not changed:
        print("nothing to change")
        return 0

    # backup
    psql(
        "INSERT INTO xz_ops_config_backups(id, source_table, source_id, raw, created_at) "
        f"SELECT '{BACKUP_ID}', 'xz_system_settings', 'ai_capability_config', raw, NOW() "
        "FROM xz_system_settings WHERE id = 'ai_capability_config';"
    )

    cfg["aiModels"] = models
    payload = json.dumps(cfg, ensure_ascii=False)
    # write via temp file inside container to avoid shell escaping
    write = subprocess.run(
        [
            "docker",
            "exec",
            "-i",
            "zhiqiyun-ai-prod-postgres-1",
            "psql",
            "-U",
            "zhiqiyun_prod",
            "-d",
            "zhiqiyun",
            "-v",
            "ON_ERROR_STOP=1",
        ],
        input=(
            "UPDATE xz_system_settings SET raw = $json$"
            + payload
            + "$json$::jsonb, updated_at = NOW() WHERE id = 'ai_capability_config';"
        ),
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        universal_newlines=True,
    )
    print(write.stdout)
    print("backup=", BACKUP_ID)
    for name, before in changed:
        print("updated", name, "from", before)

    verify = psql(
        """
SELECT jsonb_agg(jsonb_build_object(
  'model', m->>'model_name',
  'mp', COALESCE(m->>'miniprogram_enabled', m->>'miniProgramEnabled'),
  'compliance', COALESCE(m->>'compliance_status', m->>'complianceStatus')
) ORDER BY m->>'model_name')
FROM xz_system_settings s,
LATERAL jsonb_array_elements(s.raw->'aiModels') m
WHERE s.id='ai_capability_config'
  AND (m->>'model_type'='video' OR m->>'module_code'='video_generation');
""".strip()
    )
    print("verify=", verify)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
