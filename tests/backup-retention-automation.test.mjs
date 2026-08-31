import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const harnessPath = path.join(repoRoot, "tests", "backup-retention-automation.harness.sh");

function findBash() {
  for (const candidate of [process.env.BASH_PATH, "C:\\Program Files\\Git\\bin\\bash.exe", "C:\\Program Files\\Git\\usr\\bin\\bash.exe", "bash"]) {
    if (!candidate) continue;
    const probe = spawnSync(candidate, ["-c", "echo ok"], { encoding: "utf8", timeout: 5000 });
    if (probe.status === 0 && probe.stdout.trim() === "ok") return candidate;
  }
  return null;
}

function run(scenario, env = {}) {
  const bash = findBash();
  assert.ok(bash, "bash is required");
  const result = spawnSync(bash, [harnessPath, scenario], {
    cwd: repoRoot,
    encoding: "utf8",
    env: { ...process.env, BASH_PATH: bash, ...env }
  });
  return JSON.parse(result.stdout);
}

test("auto apply is disabled by default and never invokes apply", () => {
  const result = run("disabled", { RETENTION_AUTO_APPLY_ENABLED: "false" });
  assert.match(result.stdout, /RETENTION_APPLY=NOT_RUN/);
  assert.equal(result.events.some((event) => event.startsWith("apply ")), false);
});

test("upstream offsite failure prevents retention apply", () => {
  const result = run("upstream-failure", { RETENTION_AUTO_APPLY_ENABLED: "true" });
  assert.notEqual(result.status, 0);
  assert.match(result.stdout + result.stderr, /RETENTION_APPLY=NOT_RUN/);
  assert.equal(result.events.some((event) => event.startsWith("apply ")), false);
});

test("enabled automation runs offsite before bounded apply", () => {
  const result = run("enabled", { RETENTION_AUTO_APPLY_ENABLED: "true" });
  assert.match(result.stdout, /RETENTION_APPLY=PASS/);
  const executionEvents = result.events.filter((event) => !event.startsWith("flock "));
  assert.deepEqual(executionEvents.slice(0, 2), [
    "offsite ",
    "apply --apply --max-count 4 --max-bytes 1073741824 --json"
  ]);
});

test("invalid or excessive bounds fail closed before upstream", () => {
  for (const env of [
    { RETENTION_AUTO_MAX_COUNT: "0" },
    { RETENTION_AUTO_MAX_COUNT: "6" },
    { RETENTION_AUTO_MAX_BYTES: "0" },
    { RETENTION_AUTO_MAX_BYTES: "1073741825" },
    { RETENTION_AUTO_MAX_BYTES: "not-a-number" }
  ]) {
    const result = run("invalid", { RETENTION_AUTO_APPLY_ENABLED: "true", ...env });
    assert.notEqual(result.status, 0);
    assert.equal(result.events.length, 0);
  }
});

test("apply failure is returned without a replacement invocation", () => {
  const result = run("apply-failure", { RETENTION_AUTO_APPLY_ENABLED: "true" });
  assert.notEqual(result.status, 0);
  assert.equal(result.events.filter((event) => event.startsWith("apply ")).length, 1);
  assert.match(result.stdout + result.stderr, /RETENTION_APPLY=FAILED/);
});
