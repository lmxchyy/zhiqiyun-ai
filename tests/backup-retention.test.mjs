import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../", import.meta.url));
const harnessPath = path.join(repoRoot, "tests", "backup-retention.harness.sh");
const retentionScriptPath = path.join(repoRoot, "ops", "backup-retention.sh");
const retentionSource = readFileSync(retentionScriptPath, "utf8");

function findBash() {
  for (const candidate of [process.env.BASH_PATH, "C:\\Program Files\\Git\\bin\\bash.exe", "C:\\Program Files\\Git\\usr\\bin\\bash.exe", "bash"]) {
    if (!candidate) continue;
    const probe = spawnSync(candidate, ["-c", "echo ok"], { encoding: "utf8", timeout: 5000 });
    if (probe.status === 0 && probe.stdout.trim() === "ok") return candidate;
  }
  return null;
}

function runHarness(scenario = "full") {
  return JSON.parse(runHarnessRaw(scenario).stdout);
}

function runHarnessRaw(scenario = "full", extraEnv = {}) {
  const bash = findBash();
  assert.ok(bash, "bash is required");
  const gitUsrBin = "C:\\Program Files\\Git\\usr\\bin";
  const result = spawnSync(bash, [harnessPath, scenario], {
    cwd: repoRoot,
    encoding: "utf8",
    env: { ...process.env, BASH_PATH: bash, PATH: `${gitUsrBin};${process.env.PATH || ""}`, ...extraEnv }
  });
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
  return result;
}

function paths(items) { return new Set(items.map((item) => item.path)); }

test("retention engine stays within the Python 3.6 compatibility contract", () => {
  assert.doesNotMatch(retentionSource, /\.isocalendar\(\)\.(?:year|week|weekday)/);
  assert.doesNotMatch(retentionSource, /datetime\.fromisoformat/);
  assert.match(retentionSource, /def iso_parts\(value\):/);
});

test("weekly and monthly retention keep tuple-safe ISO calendar coverage", () => {
  const report = runHarness();
  const weeklyCoverage = new Set(report.keep.flatMap((item) => [...item.keep_reason.matchAll(/weekly coverage (\d{4}-W\d+)/g)].map((match) => match[1])));
  const monthlyCoverage = new Set(report.keep.flatMap((item) => [...item.keep_reason.matchAll(/monthly coverage (\d{4}-\d{2})/g)].map((match) => match[1])));
  assert.deepEqual([...weeklyCoverage].sort(), ["2026-W31", "2026-W32", "2026-W33", "2026-W34"]);
  assert.deepEqual([...monthlyCoverage].sort(), ["2026-06", "2026-07", "2026-08"]);
});

test("retention inventory classifies and calculates the full policy without deleting", () => {
  const raw = runHarnessRaw();
  const report = JSON.parse(raw.stdout);
  assert.ok(report.summary.total_files > 0);
  assert.ok(report.summary.total_bytes > 0);
  assert.equal(report.summary.delete_candidates_count, report.delete_candidates.length);
  assert.equal(report.summary.delete_candidates_bytes, report.delete_candidates.reduce((n, item) => n + item.size, 0));

  const keep = paths(report.keep);
  const deletes = paths(report.delete_candidates);
  for (const item of report.keep) assert.equal(item.category !== "", true);
  assert.equal([...keep].some((p) => p.includes("db_20260818")), true);
  assert.equal([...deletes].some((p) => p.includes("db_20260816")), true);
  assert.equal([...deletes].some((p) => p.endsWith("xianzhi-legacy.sql")), true);
  assert.equal(report.keep.filter((item) => item.category === "deploy").length, 5);
  assert.equal(report.delete_candidates.filter((item) => item.category === "deploy").length, 2);
  assert.equal([...keep].filter((p) => p.includes("compose.prod.yml")).length, 20);
  assert.equal([...deletes].filter((p) => p.includes("compose.prod.yml")).length, 5);
  assert.equal([...keep].some((p) => p.includes("deploy-20260801")), true);
  assert.equal([...deletes].some((p) => p.includes("deploy-20260701")), true);
  for (const day of ["09", "10", "11", "12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22"]) {
    assert.equal([...keep].some((p) => p.includes(`xianzhi-202608${day}`)), true);
  }
  assert.equal([...report.manual_review].every((item) => /pre-|before-|manifest|\.dump|\.csv/.test(item.path)), true);
  assert.equal(report.analyze_only.length, 2);
  assert.equal(report.out_of_scope.length >= 5, true);
  assert.equal(report.expected_reclaimed_bytes, report.delete_candidates.reduce((n, item) => n + item.size, 0));
  if (raw.stderr.includes("SYMLINK_FIXTURE_UNAVAILABLE")) {
    assert.equal([...keep, ...deletes].some((p) => p.includes("symlink-outside")), false);
  } else {
    assert.equal(report.warnings.some((warning) => warning.includes("SKIP_SYMLINK")), true);
  }
  assert.equal(report.out_of_scope.some((item) => item.path.includes("odd name [old] --backup.sql")), true);
  assert.equal(report.out_of_scope.some((item) => item.path.includes(".env.production.old bak")), true);
  assert.equal([...keep].filter((p) => p.includes("db_")).length >= 5, true);
  assert.equal([...deletes].every((p) => !p.includes("events") && !p.includes("releases") && !p.includes("release/")), true);
  assert.equal([...keep].some((p) => p.includes("symlink-outside")), false);

  const overlap = report.keep.find((item) => item.path.includes("xianzhi-20260822"));
  assert.ok(overlap);
  assert.match(overlap.keep_reason, /daily/);
  assert.match(overlap.keep_reason, /weekly/);
  assert.match(overlap.keep_reason, /monthly/);
  const weeklyCoverage = new Set(report.keep.flatMap((item) => [...item.keep_reason.matchAll(/weekly coverage (\d{4}-W\d+)/g)].map((match) => match[1])));
  const monthlyCoverage = new Set(report.keep.flatMap((item) => [...item.keep_reason.matchAll(/monthly coverage (\d{4}-\d{2})/g)].map((match) => match[1])));
  assert.deepEqual([...weeklyCoverage].sort(), ["2026-W31", "2026-W32", "2026-W33", "2026-W34"]);
  assert.deepEqual([...monthlyCoverage].sort(), ["2026-06", "2026-07", "2026-08"]);
  assert.equal(report.keep.some((item) => item.path === "releases/release-2020.dump"), false);
  assert.equal(report.analyze_only.some((item) => item.path === "releases/release-2020.dump"), true);
  assert.equal(report.analyze_only.some((item) => item.path === "release/release-2020.dump"), true);
  assert.equal(new Set([...keep, ...deletes]).size, keep.size + deletes.size);
  assert.equal(report.summary.keep_count, report.keep.length);
  assert.equal(report.summary.manual_review_count, report.manual_review.length);
  assert.equal(report.summary.analyze_only_count, report.analyze_only.length);
  assert.equal(report.summary.out_of_scope_count, report.out_of_scope.length);
});

test("retention preserves every deploy backup when fewer than five exist", () => {
  const report = runHarness("insufficient");
  const deploy = [...report.keep, ...report.delete_candidates].filter((item) => item.path.startsWith("postgres/"));
  assert.equal(deploy.length, 3);
  assert.equal(report.delete_candidates.filter((item) => item.path.startsWith("postgres/")).length, 0);
});

test("JSON output is valid and deterministic", () => {
  const first = runHarnessRaw();
  const second = runHarnessRaw();
  assert.equal(first.stdout, second.stdout);
  const report = JSON.parse(first.stdout);
  assert.deepEqual(Object.keys(report).sort(), [
    "analyze_only",
    "delete_candidates",
    "expected_reclaimed_bytes",
    "keep",
    "manual_review",
    "out_of_scope",
    "summary",
    "warnings"
  ]);
});

test("human output reports expected reclaim in bytes and readable form", () => {
  const result = runHarnessRaw("full", { BACKUP_RETENTION_JSON: "0" });
  assert.match(result.stdout, /# Backup Retention Dry Run/);
  assert.match(result.stdout, /## Expected Reclaimed Space/);
  assert.match(result.stdout, /## OUT OF SCOPE/);
  assert.match(result.stdout, /bytes:/);
  assert.match(result.stdout, /human:/);
});

test("broad roots are rejected", () => {
  const bash = findBash();
  assert.ok(bash);
  const result = spawnSync(bash, [path.join(repoRoot, "ops", "backup-retention.sh"), "--json", "--root", repoRoot], {
    cwd: repoRoot,
    encoding: "utf8",
    env: { ...process.env, PATH: `C:\\Program Files\\Git\\usr\\bin;${process.env.PATH || ""}` }
  });
  assert.notEqual(result.status, 0);
});

test("apply is explicitly unavailable and never deletes", () => {
  const bash = findBash();
  assert.ok(bash);
  const result = spawnSync(bash, [path.join(repoRoot, "ops", "backup-retention.sh"), "--apply", "--root", repoRoot], {
    encoding: "utf8",
    env: { ...process.env, PATH: `C:\\Program Files\\Git\\usr\\bin;${process.env.PATH || ""}` }
  });
  assert.notEqual(result.status, 0);
  assert.match(`${result.stdout}\n${result.stderr}`, /APPLY_NOT_IMPLEMENTED/);
});
