#!/usr/bin/env python3
# Verify WeChat virtual-pay order ship status after notify_provide_goods.
# status 4 = 已发货. Does not grant entitlements / refund / change flags.
import hashlib, hmac, json, ssl, subprocess, sys, urllib.parse, urllib.request

APP_C = "zhiqiyun-ai-prod-xianzhi-ai-1"
PG_C = "zhiqiyun-ai-prod-postgres-1"
ORDERS = ["ZQY202607282159389857812495", "ZQY20260728221656E339AB7A54"]
# Known payer openid from MEMBER TEST console evidence for demo@xianzhi.ai
OPENID = "oCTyT7ZlaHEPWG2bPDm0d0Ok6Q5Y"
STATUS_MAP = {2: "PAID_PENDING_SHIP", 3: "SHIPPING", 4: "SHIPPED", 5: "REFUNDED", 6: "CLOSED"}


def env(name):
    return subprocess.check_output(["docker", "exec", APP_C, "printenv", name], universal_newlines=True).strip()


def pg(sql):
    db = subprocess.check_output(["docker", "exec", PG_C, "printenv", "POSTGRES_DB"], universal_newlines=True).strip()
    user = subprocess.check_output(["docker", "exec", PG_C, "printenv", "POSTGRES_USER"], universal_newlines=True).strip()
    return subprocess.check_output(
        ["docker", "exec", "-i", PG_C, "psql", "-U", user, "-d", db, "-At", "-F", "|", "-c", sql],
        universal_newlines=True,
    ).strip()


def main():
    appid, secret = env("WECHAT_MINI_PROGRAM_APPID"), env("WECHAT_MINI_PROGRAM_SECRET")
    pay_env = env("WECHAT_VIRTUAL_PAY_ENV").strip().lower()
    env_n = 1 if pay_env in ("sandbox", "1") else 0
    app_key = env("WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY" if env_n == 1 else "WECHAT_VIRTUAL_PAY_APP_KEY")
    token_body = json.dumps({
        "grant_type": "client_credential",
        "appid": appid,
        "secret": secret,
        "force_refresh": False,
    }).encode()
    req = urllib.request.Request(
        "https://api.weixin.qq.com/cgi-bin/stable_token",
        data=token_body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, context=ssl.create_default_context(), timeout=20) as resp:
        access_token = json.load(resp)["access_token"]

    out = {"env": env_n, "orders": [], "events": []}
    for order_no in ORDERS:
        body_obj = {"openid": OPENID, "env": env_n, "order_id": order_no}
        body = json.dumps(body_obj, separators=(",", ":")).encode()
        pay_sig = hmac.new(app_key.encode(), ("/xpay/query_order&" + body.decode()).encode(), hashlib.sha256).hexdigest()
        url = "https://api.weixin.qq.com/xpay/query_order?access_token=%s&pay_sig=%s" % (
            urllib.parse.quote(access_token),
            urllib.parse.quote(pay_sig),
        )
        q = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(q, context=ssl.create_default_context(), timeout=20) as resp:
            payload = json.load(resp)
        order = payload.get("order") or {}
        status = order.get("status")
        entry = {
            "orderNo": order_no,
            "errcode": payload.get("errcode"),
            "errmsg": payload.get("errmsg"),
            "status": status,
            "statusLabel": STATUS_MAP.get(status, str(status)),
            "provide_time": order.get("provide_time"),
            "wx_order_id": order.get("wx_order_id"),
            "wxpay_order_id": order.get("wxpay_order_id"),
            "shipped": status == 4,
        }
        out["orders"].append(entry)
        print(json.dumps(entry, ensure_ascii=False))

    events = pg(
        "select order_id||'|'||coalesce(event_type,'')||'|'||processing_status||'|'||"
        "coalesce(error_message,'')||'|'||coalesce(processed_at::text,'') "
        "from xz_payment_events where order_id in "
        "('ZQY202607282159389857812495','ZQY20260728221656E339AB7A54') order by created_at"
    )
    for line in events.splitlines():
        if not line.strip():
            continue
        parts = line.split("|")
        out["events"].append({
            "order_id": parts[0],
            "event_type": parts[1] if len(parts) > 1 else "",
            "processing_status": parts[2] if len(parts) > 2 else "",
            "error_message": parts[3] if len(parts) > 3 else "",
            "processed_at": parts[4] if len(parts) > 4 else "",
        })
        print("EVENT", line)

    path = "/tmp/deliver-notify-oneshot/query-order-after.json"
    with open(path, "w") as fh:
        json.dump(out, fh, ensure_ascii=False, indent=2)
    print("WROTE", path)
    if not all(item.get("shipped") for item in out["orders"]):
        sys.exit(2)


if __name__ == "__main__":
    main()
