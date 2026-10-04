#!/usr/bin/env python3
"""Issue203 Runtime Quarantine Zero Side-Effects & Race Safety Test Harness.

Spins up an owned disposable pgvector container, applies schema + 123 migrations,
tests read-only quarantine barrier attestation in ops/verify-release-runtime.py,
runs the Go quarantine suites across providerexecution and httpserver,
runs containerized -race checks with CGO to verify race safety,
asserts ZERO SKIPS and exit 0, and guarantees cleanup of the owned container.
"""

import importlib.util
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parent.parent


def get_free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


def assert_no_skips(output, package_name):
    for line in output.splitlines():
        if "--- SKIP:" in line or "=== SKIP:" in line:
            raise RuntimeError(f"{package_name} contained unexpected SKIP: {line}")
    if "PASS" not in output:
        raise RuntimeError(f"{package_name} did not contain PASS")


def test_fail_closed(container, runtime_gate):
    # Disable trigger temporarily to test fail closed
    subprocess.run([
        "docker", "exec", container, "psql", "-X", "-U", "postgres", "-d", "xianzhi_test",
        "-c", "ALTER TABLE provider_execution_quarantine DISABLE TRIGGER trg_provider_execution_quarantine_immutable;"
    ], check=True, stdout=subprocess.DEVNULL)
    try:
        try:
            runtime_gate.verify_quarantine_barrier_read_only("compose.yml", ".env")
            raise AssertionError("Disabled trigger must fail closed in attestation")
        except runtime_gate.GateError:
            pass  # Expected fail-closed
    finally:
        subprocess.run([
            "docker", "exec", container, "psql", "-X", "-U", "postgres", "-d", "xianzhi_test",
            "-c", "ALTER TABLE provider_execution_quarantine ENABLE TRIGGER trg_provider_execution_quarantine_immutable;"
        ], check=True, stdout=subprocess.DEVNULL)


def main():
    net = "issue203-race-net-" + uuid.uuid4().hex[:8]
    container = "issue203-runtime-race-" + uuid.uuid4().hex[:8]
    port = get_free_port()
    dsn_host = f"postgres://postgres:test_secret_pass@127.0.0.1:{port}/xianzhi_test?sslmode=disable"
    dsn_net = f"postgres://postgres:test_secret_pass@{container}:5432/xianzhi_test?sslmode=disable"

    subprocess.run(["docker", "network", "create", net], check=True, stdout=subprocess.DEVNULL)
    print(f"[runtime-race-harness] Starting owned container {container} on network {net} and port {port}...")

    # Start disposable container attached to owned network and published to host port
    subprocess.run([
        "docker", "run", "-d", "--rm",
        "--name", container,
        "--network", net,
        "-p", f"127.0.0.1:{port}:5432",
        "-e", "POSTGRES_PASSWORD=test_secret_pass",
        "-e", "POSTGRES_USER=postgres",
        "-e", "POSTGRES_DB=xianzhi_test",
        "pgvector/pgvector:pg16"
    ], check=True, stdout=subprocess.DEVNULL)

    try:
        # Wait for ready
        ready = False
        for _ in range(40):
            res = subprocess.run(
                ["docker", "exec", container, "psql", "-X", "-U", "postgres", "-d", "xianzhi_test", "-c", "SELECT 1;"],
                capture_output=True
            )
            if res.returncode == 0:
                ready = True
                break
            time.sleep(0.5)

        if not ready:
            raise RuntimeError(f"PostgreSQL container {container} failed to become ready")

        print("[runtime-race-harness] PostgreSQL is ready. Replaying schema and 123 migrations...")
        migration_paths = sorted(
            p for p in (ROOT / "database/migrations").glob("[0-9][0-9][0-9]-*.sql")
            if not p.name.endswith(".down.sql")
        )
        if len(migration_paths) != 123:
            raise RuntimeError(f"Expected 123 migrations, found {len(migration_paths)}")

        replay_sql = (ROOT / "database/schema.sql").read_text(encoding="utf-8") + "\n"
        replay_sql += "\n".join(p.read_text(encoding="utf-8") for p in migration_paths)

        res = subprocess.run(
            ["docker", "exec", "-i", container, "psql", "-X", "-U", "postgres", "-d", "xianzhi_test", "-v", "ON_ERROR_STOP=1"],
            input=replay_sql.encode("utf-8"),
            capture_output=True
        )
        if res.returncode != 0:
            err = res.stderr.decode("utf-8", "replace")
            raise RuntimeError(f"Migration replay failed: {err}")

        print(f"[runtime-race-harness] Successfully replayed schema + {len(migration_paths)} migrations.")

        # Set environment for attestation and Go tests
        for k, v in [
            ("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL", dsn_host),
            ("XIANZHI_TEST_DATABASE_URL", dsn_host),
            ("PERSONAL_POINTS_POSTGRES_TEST_DSN", dsn_host),
            ("XIANZHI_PERSONAL_POINT_TEST_DATABASE_URL", dsn_host),
            ("XIANZHI_TEST_CONTAINER", container),
            ("POSTGRES_USER", "postgres"),
            ("POSTGRES_PASSWORD", "test_secret_pass"),
            ("POSTGRES_DB", "xianzhi_test"),
        ]:
            os.environ[k] = v
        env = dict(os.environ)

        # 1. Test read-only quarantine barrier health attestation in ops/verify-release-runtime.py
        print("[runtime-race-harness] Testing read-only quarantine barrier attestation...")
        spec = importlib.util.spec_from_file_location("runtime_gate", str(ROOT / "ops/verify-release-runtime.py"))
        runtime_gate = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runtime_gate)

        # Attestation against healthy migrated database: must succeed
        runtime_gate.verify_quarantine_barrier_read_only("compose.yml", ".env")
        print("[runtime-race-harness] Barrier attestation: PASS (relation and immutable trigger active)")

        # Verify fail-closed behavior on unhealthy/missing barrier
        test_fail_closed(container, runtime_gate)
        print("[runtime-race-harness] Barrier fail-closed attestation: PASS")

        # 2. Run Go tests on host
        print("[runtime-race-harness] Running Go providerexecution quarantine suite on host...")
        pe_cmd = ["go", "test", "-v", "-count=1", "./internal/providerexecution", "-run", "Quarantine"]
        pe_proc = subprocess.run(pe_cmd, cwd=str(ROOT / "backend-go"), env=env, capture_output=True, text=True)
        print(pe_proc.stdout)
        if pe_proc.returncode != 0:
            print(pe_proc.stderr, file=sys.stderr)
            raise RuntimeError(f"providerexecution quarantine tests failed with code {pe_proc.returncode}")
        assert_no_skips(pe_proc.stdout, "providerexecution")

        print("[runtime-race-harness] Running Go httpserver quarantine suite on host...")
        hs_cmd = ["go", "test", "-v", "-count=1", "./internal/httpserver", "-run", "Quarantine"]
        hs_proc = subprocess.run(hs_cmd, cwd=str(ROOT / "backend-go"), env=env, capture_output=True, text=True)
        print(hs_proc.stdout)
        if hs_proc.returncode != 0:
            print(hs_proc.stderr, file=sys.stderr)
            raise RuntimeError(f"httpserver quarantine tests failed with code {hs_proc.returncode}")
        assert_no_skips(hs_proc.stdout, "httpserver")

        # 3. Run containerized Go tests with -race and CGO enabled
        print("[runtime-race-harness] Running containerized Go tests with -race and CGO enabled...")
        race_cmd = [
            "docker", "run", "--rm",
            "--network", net,
            "-v", f"{ROOT}:/workspace",
            "-w", "/workspace/backend-go",
            "-e", f"XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL={dsn_net}",
            "-e", f"XIANZHI_TEST_DATABASE_URL={dsn_net}",
            "-e", f"PERSONAL_POINTS_POSTGRES_TEST_DSN={dsn_net}",
            "-e", f"XIANZHI_PERSONAL_POINT_TEST_DATABASE_URL={dsn_net}",
            "-e", "CGO_ENABLED=1",
            "golang:1.25-bookworm",
            "go", "test", "-v", "-race", "-count=1",
            "./internal/providerexecution", "./internal/httpserver",
            "-run", "Quarantine"
        ]
        race_proc = subprocess.run(race_cmd, capture_output=True, text=True)
        print(race_proc.stdout)
        if race_proc.returncode != 0:
            print(race_proc.stderr, file=sys.stderr)
            raise RuntimeError(f"Containerized -race tests failed with code {race_proc.returncode}")
        assert_no_skips(race_proc.stdout, "containerized-race-suite")

        print("[runtime-race-harness] ALL TESTS PASSED: Zero side-effects, zero mutations, zero skips, and race detector clean!")
        return 0

    finally:
        print(f"[runtime-race-harness] Cleaning up owned container {container} and network {net}...")
        subprocess.run(["docker", "rm", "-f", container], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(["docker", "network", "rm", net], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    sys.exit(main())
