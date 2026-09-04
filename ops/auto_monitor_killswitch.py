import urllib.request
import json
import subprocess
import sys
import time

def log(msg):
    print(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] {msg}", flush=True)

def trigger_kill_switch(reason):
    log(f"ALERT: KILL SWITCH TRIGGERED: {reason}")
    path = "/opt/zhiqiyun-ai/.env.production"
    with open(path, "r", encoding="utf-8") as f:
        content = f.read()
    content = content.replace("GENERATION_ASYNC_CANARY_ENABLED=true", "GENERATION_ASYNC_CANARY_ENABLED=false")
    content = content.replace("VIDEO_ASYNC_CANARY_ENABLED=true", "VIDEO_ASYNC_CANARY_ENABLED=false")
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    subprocess.run(
        ["docker", "compose", "-f", "/opt/zhiqiyun-ai/compose.prod.yml", "--env-file", path, "up", "-d", "--no-build", "xianzhi-ai"]
    )
    log("KILL SWITCH COMPLETED: GENERATION_ASYNC_CANARY_ENABLED set to false and API reloaded.")
    sys.exit(1)

def check_metrics():
    try:
        req = urllib.request.Request("http://127.0.0.1:3100/metrics")
        with urllib.request.urlopen(req, timeout=5) as resp:
            text = resp.read().decode("utf-8")
    except Exception as e:
        log(f"Metrics scrape failed: {e}")
        return

    metrics = {}
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) == 2:
            try:
                metrics[parts[0]] = float(parts[1])
            except Exception:
                pass

    if metrics.get("xianzhi_async_canary_rabbitmq_dlq_depth", 0) > 0:
        trigger_kill_switch("RabbitMQ DLQ depth > 0")
    if metrics.get("xianzhi_async_canary_video_rabbitmq_dlq_depth", 0) > 0:
        trigger_kill_switch("Video RabbitMQ DLQ depth > 0")
    if metrics.get("xianzhi_async_canary_outbox_failed", 0) > 0:
        trigger_kill_switch("Outbox failed count > 0")
    if metrics.get("xianzhi_async_canary_points_settlement_conflicts_total", 0) > 0:
        trigger_kill_switch("Points settlement conflicts > 0")
    if metrics.get("xianzhi_async_canary_artifact_recovery_failures_total", 0) > 0:
        trigger_kill_switch("Artifact recovery failures > 0")
    if metrics.get("xianzhi_async_canary_generation_stuck", 0) > 0:
        trigger_kill_switch("Generation tasks stuck > 0")
    if metrics.get("xianzhi_async_canary_video_generation_stuck", 0) > 0:
        trigger_kill_switch("Video generation tasks stuck > 0")

    log("Metrics check PASS: all safety indicators normal.")

if __name__ == "__main__":
    check_metrics()
