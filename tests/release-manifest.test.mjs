import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import test from "node:test";

const root = new URL("../", import.meta.url);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

test("release manifest validator accepts a matching immutable manifest", () => {
  const manifest = JSON.stringify({
    git_sha: "97361d7fe4cfcd32cce644b153532be480ad721a",
    image: "ghcr.io/lmxchyy/zhiqiyun-ai",
    digest: `sha256:${"a".repeat(64)}`,
    image_reference: `ghcr.io/lmxchyy/zhiqiyun-ai@sha256:${"a".repeat(64)}`,
    built_at: "2026-08-24T00:00:00Z",
    production_contract: "passed"
  });
  const output = execFileSync(bash, ["ops/verify-release-manifest.sh", "-", "97361d7fe4cfcd32cce644b153532be480ad721a"], {
    cwd: new URL("../", import.meta.url),
    input: manifest,
    encoding: "utf8"
  });
  assert.match(output, /ghcr\.io\/lmxchyy\/zhiqiyun-ai@sha256:/);
});

test("release manifest validator rejects mutable-only identity", () => {
  const manifest = JSON.stringify({
    git_sha: "97361d7fe4cfcd32cce644b153532be480ad721a",
    image: "ghcr.io/lmxchyy/zhiqiyun-ai",
    digest: "",
    image_reference: "ghcr.io/lmxchyy/zhiqiyun-ai:latest",
    built_at: "2026-08-24T00:00:00Z",
    production_contract: "passed"
  });
  assert.throws(() => execFileSync(bash, ["ops/verify-release-manifest.sh", "-", "97361d7fe4cfcd32cce644b153532be480ad721a"], {
    cwd: new URL("../", import.meta.url),
    input: manifest,
    encoding: "utf8",
    stdio: ["pipe", "pipe", "pipe"]
  }));
});
