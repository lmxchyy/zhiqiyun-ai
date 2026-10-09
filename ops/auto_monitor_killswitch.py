import urllib.request
import json
import os
import pathlib
import signal
import stat
import subprocess
import sys
import tempfile
import time
import uuid
from contextlib import contextmanager

ROOT = pathlib.Path(__file__).resolve().parents[1]
INTERRUPTED = 0


class ReleaseLocked(Exception):
    pass


def handle_signal(signum, _frame):
    # Do not unlock while a synchronous Compose operation is still running.
    global INTERRUPTED
    INTERRUPTED = signum


def check_interrupted():
    if INTERRUPTED:
        raise SystemExit(128 + INTERRUPTED)


@contextmanager
def release_lock(root):
    directory = pathlib.Path(os.environ.get("PRESTAGE_DIR", ".prestage"))
    if not directory.is_absolute():
        directory = root / directory
    lock = directory / "release.lock"
    recovery = directory / "release.lock.recovering"
    token = str(uuid.uuid4())
    # No monitor-side stale recovery: malformed, symlink and dead-owner locks
    # all defer to the release/operator protocol, without deleting anything.
    if release_lock_present(root):
        raise ReleaseLocked()
    directory.mkdir(parents=True, exist_ok=True)
    try:
        lock.mkdir(mode=0o700)  # Same atomic acquisition as all release scripts.
    except FileExistsError:
        raise ReleaseLocked()
    try:
        (lock / "owner_token").write_text(token + "\n")
        (lock / "owner_pid").write_text(str(os.getpid()) + "\n")
        (lock / "created_at").write_text(time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()) + "\n")
        if os.path.lexists(str(recovery)) or os.path.lexists(str(directory / "cold-recovery-required")):
            raise ReleaseLocked()
        check_interrupted()
        yield lock
    finally:
        # A failed contender or token mismatch must never remove another owner.
        try:
            if (lock / "owner_token").read_text().strip() == token:
                if (lock / "owner_pid").read_text().strip() == "indeterminate-compose":
                    log("Compose completion indeterminate; retain lock for operator review.")
                else:
                    for name in ("owner_pid", "created_at", "owner_token"):
                        (lock / name).unlink()
                    lock.rmdir()
        except OSError:
            log("Lock cleanup incomplete; left fail-closed for operator review.")


def write_env_atomically(path, content):
    mode = stat.S_IMODE(path.stat().st_mode)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=str(path.parent), delete=False) as f:
            temporary = f.name
            os.fchmod(f.fileno(), mode)
            f.write(content)
            f.flush()
            os.fsync(f.fileno())
        check_interrupted()
        os.replace(temporary, str(path))
    finally:
        if temporary and os.path.exists(temporary):
            os.unlink(temporary)

def log(msg):
    print(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] {msg}", flush=True)

def trigger_kill_switch(reason, image_dlq_only=False):
    with release_lock(ROOT) as lock:
        log(f"ALERT: KILL SWITCH TRIGGERED: {reason}")
        path = ROOT / ".env.production"
        original = path.read_text(encoding="utf-8")
        content = original
        flags = ("GENERATION_ASYNC_CANARY_ENABLED",)
        if not image_dlq_only:
            # Preserve the existing default for video/PPT and other alerts.
            flags += ("VIDEO_ASYNC_CANARY_ENABLED", "PPT_ASYNC_CANARY_ENABLED")
        for name in flags:
            content = content.replace(name + "=true", name + "=false")
        check_interrupted()
        if content != original:
            write_env_atomically(path, content)
        else:
            # Still reconcile API-only: a previous reload may have failed after
            # writing the disabled flags. Do not mistake env for runtime state.
            label = "Image canary" if image_dlq_only else "Canary switches"
            log(f"{label} already off; no env write; reconcile API-only.")
        check_interrupted()
        # If the monitor is SIGKILLed with a Compose child in flight, a dead PID
        # alone cannot prove that child/daemon actions stopped. Existing release
        # code treats this sentinel as indeterminate, so cannot reclaim the lock.
        (lock / "owner_pid").write_text("indeterminate-compose\n")
        subprocess.run(
            ["docker", "compose", "-f", str(ROOT / "compose.prod.yml"), "--env-file", str(path),
             "up", "-d", "--no-deps", "--no-build", "--pull", "never", "xianzhi-ai"],
            check=True,
        )
        # Only a successful synchronous Compose return permits normal unlock.
        (lock / "owner_pid").write_text(str(os.getpid()) + "\n")
        check_interrupted()
        label = "image canary" if image_dlq_only else "canary switches"
        log(f"KILL SWITCH COMPLETED: {label} off; API-only reload completed.")
        sys.exit(1)

ALERT_RULES = (
    ("xianzhi_async_canary_rabbitmq_dlq_depth", "RabbitMQ DLQ depth > 0", True),
    ("xianzhi_async_canary_video_rabbitmq_dlq_depth", "Video RabbitMQ DLQ depth > 0", False),
    ("xianzhi_async_canary_outbox_failed", "Outbox failed count > 0", False),
    ("xianzhi_async_canary_points_settlement_conflicts_total", "Points settlement conflicts > 0", False),
    ("xianzhi_async_canary_artifact_recovery_failures_total", "Artifact recovery failures > 0", False),
    ("xianzhi_async_canary_generation_stuck", "Generation tasks stuck > 0", False),
    ("xianzhi_async_canary_video_generation_stuck", "Video generation tasks stuck > 0", False),
    ("xianzhi_async_canary_ppt_rabbitmq_dlq_depth", "PPT RabbitMQ DLQ depth > 0", False),
    ("xianzhi_async_canary_ppt_generation_stuck", "PPT generation tasks stuck > 0", False),
)
CANARY_FLAGS = (
    "GENERATION_ASYNC_CANARY_ENABLED",
    "VIDEO_ASYNC_CANARY_ENABLED",
    "PPT_ASYNC_CANARY_ENABLED",
)


def release_lock_path(root=None):
    root = pathlib.Path(root if root is not None else ROOT)
    directory = pathlib.Path(os.environ.get("PRESTAGE_DIR", ".prestage"))
    if not directory.is_absolute():
        directory = root / directory
    return directory


def release_lock_present(root=None):
    directory = release_lock_path(root)
    return any(os.path.lexists(str(directory / name))
               for name in ("release.lock", "release.lock.recovering", "cold-recovery-required"))


def scrape_metrics():
    req = urllib.request.Request("http://127.0.0.1:3100/metrics")
    with urllib.request.urlopen(req, timeout=5) as resp:
        text = resp.read().decode("utf-8")
    metrics = {}
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) == 2:
            try:
                metrics[parts[0]] = float(parts[1])
            except (TypeError, ValueError):
                continue
    return metrics


def triggered_rules(metrics):
    return [(name, reason, image_only) for name, reason, image_only in ALERT_RULES
            if metrics.get(name, 0) > 0]


def read_canary_flags():
    """Read only the three kill-switch keys; never echo other env values."""
    path = ROOT / ".env.production"
    values = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        for name in CANARY_FLAGS:
            prefix = name + "="
            if stripped.startswith(prefix):
                if name in values:
                    return {key: "UNKNOWN" for key in CANARY_FLAGS}
                value = stripped[len(prefix):].strip().lower()
                values[name] = value if value in ("true", "false") else "UNKNOWN"
    return {name: values.get(name, "UNKNOWN") for name in CANARY_FLAGS}


def observe_only(metrics):
    """Report the normal-mode decision without touching files or containers."""
    if release_lock_present():
        log("SUPPRESSED_BY_RELEASE_LOCK: observe-only; no business action evaluated.")
        return 0
    rules = triggered_rules(metrics)
    if not rules:
        log("OBSERVE_ONLY_NO_ACTION: all safety indicators normal.")
        return 0
    flags = read_canary_flags()
    if release_lock_present():
        log("SUPPRESSED_BY_RELEASE_LOCK: lock appeared during observe-only decision.")
        return 0
    first = rules[0]
    planned_flags = [CANARY_FLAGS[0]] if first[2] else list(CANARY_FLAGS)
    would_change = [name for name in planned_flags if flags[name] == "true"]
    log("OBSERVE_ONLY_WOULD_TRIGGER: " + json.dumps({
        "next_normal_action": first[1],
        "metric": first[0],
        "remaining_alerts": [item[1] for item in rules[1:]],
        "canary_state": flags,
        "canaries_that_would_change": would_change,
        "api_reconcile": True,
        "migration": False,
        "executed": False,
    }, sort_keys=True, separators=(",", ":")))
    return 0


def check_metrics():
    try:
        metrics = scrape_metrics()
    except Exception as e:
        log(f"Metrics scrape failed: {type(e).__name__}")
        return
    for _metric, reason, image_only in triggered_rules(metrics):
        trigger_kill_switch(reason, image_dlq_only=image_only)
    log("Metrics check PASS: all safety indicators normal.")


def parse_args(argv):
    if not argv:
        return False
    if list(argv) == ["--observe-only"]:
        return True
    sys.stderr.write("Usage: auto_monitor_killswitch.py [--observe-only]\n")
    raise SystemExit(2)


def main(argv=None):
    signal.signal(signal.SIGINT, handle_signal)
    signal.signal(signal.SIGTERM, handle_signal)
    observe = parse_args(sys.argv[1:] if argv is None else argv)
    # Immediate no-op while release/recovery owns a lock; no metrics request.
    if release_lock_present():
        log("SUPPRESSED_BY_RELEASE_LOCK: deferred; no env or container changes.")
        return 0
    try:
        if observe:
            metrics = scrape_metrics()
            check_interrupted()
            # Close the scrape-to-decision race. A newly acquired release lock
            # suppresses even the observe-only would-trigger report.
            if release_lock_present():
                log("SUPPRESSED_BY_RELEASE_LOCK: lock appeared during metrics scrape.")
                return 0
            return observe_only(metrics)
        check_metrics()
        check_interrupted()
    except ReleaseLocked:
        log("SUPPRESSED_BY_RELEASE_LOCK: deferred; no env or container changes.")
    except (OSError, subprocess.CalledProcessError) as exc:
        log("KILL SWITCH FAILED: " + type(exc).__name__)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
