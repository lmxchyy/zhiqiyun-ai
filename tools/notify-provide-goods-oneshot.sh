#!/usr/bin/env bash
# One-shot WeChat virtual-pay deliver confirmation for already-fulfilled orders.
# Runs on the prod host using AppKey/AppID/Secret from the running app container.
# Does NOT grant entitlements. Does NOT refund. Does NOT change prices/V2 flags.
#
# Usage (on root@119.29.191.227):
#   bash tools/notify-provide-goods-oneshot.sh ZQY... [ZQY...]
set -euo pipefail

APP_C="${APP_C:-zhiqiyun-ai-prod-xianzhi-ai-1}"
PG_C="${PG_C:-zhiqiyun-ai-prod-postgres-1}"
EVID_DIR="${EVID_DIR:-/tmp/deliver-notify-oneshot}"
mkdir -p "$EVID_DIR"

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <orderNo> [orderNo...]" >&2
  exit 2
fi

python3 - <<'PY' "$@"
import hashlib, hmac, json, os, ssl, sys, time, urllib.parse, urllib.request, subprocess

APP_C = os.environ.get("APP_C", "zhiqiyun-ai-prod-xianzhi-ai-1")
PG_C = os.environ.get("PG_C", "zhiqiyun-ai-prod-postgres-1")
EVID_DIR = os.environ.get("EVID_DIR", "/tmp/deliver-notify-oneshot")
URI = "/xpay/notify_provide_goods"
orders = [o.strip() for o in sys.argv[1:] if o.strip()]

def docker_env(name: str) -> str:
    out = subprocess.check_output(["docker", "exec", APP_C, "printenv", name], text=True).strip()
    if not out:
        raise SystemExit(f"missing env {name} in {APP_C}")
    return out

def pg_query(sql: str) -> str:
    db = subprocess.check_output(["docker", "exec", PG_C, "printenv", "POSTGRES_DB"], text=True).strip()
    user = subprocess.check_output(["docker", "exec", PG_C, "printenv", "POSTGRES_USER"], text=True).strip()
    return subprocess.check_output(
        ["docker", "exec", "-i", PG_C, "psql", "-U", user, "-d", db, "-At", "-F", "|", "-c", sql],
        text=True,
    ).strip()

appid = docker_env("WECHAT_MINI_PROGRAM_APPID")
secret = docker_env("WECHAT_MINI_PROGRAM_SECRET")
pay_env = docker_env("WECHAT_VIRTUAL_PAY_ENV").strip().lower()
env = 1 if pay_env in ("sandbox", "1") else 0
app_key = docker_env("WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY" if env == 1 else "WECHAT_VIRTUAL_PAY_APP_KEY")

token_body = json.dumps({
    "grant_type": "client_credential",
    "appid": appid,
    "secret": secret,
    "force_refresh": False,
}).encode()
token_req = urllib.request.Request(
    "https://api.weixin.qq.com/cgi-bin/stable_token",
    data=token_body,
    headers={"Content-Type": "application/json"},
    method="POST",
)
with urllib.request.urlopen(token_req, context=ssl.create_default_context(), timeout=20) as resp:
    token_payload = json.load(resp)
access_token = token_payload.get("access_token") or ""
if not access_token:
    raise SystemExit(f"access_token unavailable: {token_payload}")

summary = {"env": env, "orders": [], "checkedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}

for order_no in orders:
    row = pg_query(
        "select status, entitlement_status, coalesce(wechat_order_id,''), coalesce(wechat_transaction_id,'') "
        f"from xz_orders where order_no='{order_no.replace(chr(39), '')}' and payment_channel='WECHAT_VIRTUAL'"
    )
    if not row:
        entry = {"orderNo": order_no, "ok": False, "error": "order not found"}
        summary["orders"].append(entry)
        print(json.dumps(entry, ensure_ascii=False))
        continue
    status, entitlement, wx_order_id, wx_tx = row.split("|")
    if status != "PAID" or entitlement != "SUCCESS":
        entry = {
            "orderNo": order_no,
            "ok": False,
            "error": f"not PAID/SUCCESS (status={status} entitlement={entitlement})",
            "wechatOrderId": wx_order_id,
            "wechatTransactionId": wx_tx,
        }
        summary["orders"].append(entry)
        print(json.dumps(entry, ensure_ascii=False))
        continue

    body_obj = {"order_id": order_no, "env": env}
    body = json.dumps(body_obj, separators=(",", ":")).encode()
    pay_sig = hmac.new(app_key.encode(), (URI + "&" + body.decode()).encode(), hashlib.sha256).hexdigest()
    url = (
        "https://api.weixin.qq.com/xpay/notify_provide_goods"
        f"?access_token={urllib.parse.quote(access_token)}"
        f"&pay_sig={urllib.parse.quote(pay_sig)}"
    )
    req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, context=ssl.create_default_context(), timeout=20) as resp:
            raw = resp.read().decode()
            code = resp.status
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()
        code = exc.code
    try:
        payload = json.loads(raw) if raw else {}
    except json.JSONDecodeError:
        payload = {"raw": raw}
    errcode = int(payload.get("errcode", -1)) if isinstance(payload, dict) else -1
    errmsg = str(payload.get("errmsg", "")) if isinstance(payload, dict) else ""
    ok = errcode == 0 or errcode == 268490004 or ("已发货" in errmsg) or ("already" in errmsg.lower()) or ("重复" in errmsg)
    entry = {
        "orderNo": order_no,
        "ok": ok,
        "httpStatus": code,
        "errcode": errcode,
        "errmsg": errmsg,
        "wechatOrderId": wx_order_id,
        "wechatTransactionId": wx_tx,
        "status": status,
        "entitlementStatus": entitlement,
    }
    # Persist a local payment-event marker (no secrets).
    safe_raw = json.dumps({"event": "notify_provide_goods", "orderNo": order_no, "env": env, "result": {"errcode": errcode, "errmsg": errmsg}}, ensure_ascii=False).replace("'", "''")
    event_id = f"notify_provide_goods:{order_no}"
    idem = f"WECHAT_VIRTUAL:{event_id}"
    resource_id = "payment_event_" + hashlib.sha256(f"payment_event:{idem}".encode()).hexdigest()[:24]
    proc = "SUCCESS" if ok else "FAILED"
    pg_query(
        "insert into xz_payment_events("
        "id, provider, event_id, event_type, order_id, transaction_id, amount_cents, "
        "raw, raw_body, verified, idempotency_key, status, processing_status, process_attempts, error_message, processed_at"
        ") values ("
        f"'{resource_id}','WECHAT_VIRTUAL','{event_id}','notify_provide_goods','{order_no}',null,0,"
        f"'{safe_raw}'::jsonb,'{safe_raw}',true,'{idem}','{proc}','{proc}',1,"
        f"'{('' if ok else errmsg).replace(chr(39), '')}',"
        f"{'now()' if ok else 'null'}"
        ") on conflict (idempotency_key) do update set "
        "process_attempts = xz_payment_events.process_attempts + 1, "
        f"processing_status = case when '{proc}' = 'SUCCESS' then 'SUCCESS' else excluded.processing_status end, "
        f"status = case when '{proc}' = 'SUCCESS' then 'SUCCESS' else excluded.status end, "
        f"error_message = case when '{proc}' = 'SUCCESS' then '' else excluded.error_message end, "
        f"processed_at = case when '{proc}' = 'SUCCESS' then now() else xz_payment_events.processed_at end, "
        "raw = excluded.raw, raw_body = excluded.raw_body"
    )
    summary["orders"].append(entry)
    print(json.dumps(entry, ensure_ascii=False))

out_path = os.path.join(EVID_DIR, f"oneshot-{int(time.time())}.json")
with open(out_path, "w", encoding="utf-8") as fh:
    json.dump(summary, fh, ensure_ascii=False, indent=2)
print(f"WROTE {out_path}", file=sys.stderr)
if not all(item.get("ok") for item in summary["orders"]):
    sys.exit(1)
PY
