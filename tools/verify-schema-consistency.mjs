#!/usr/bin/env node
// Schema consistency guard: database/schema.sql, database/migrations/, and
// runtime DDL embedded in backend-go sources are three independent sources of
// truth. CI replays schema.sql + migrations into a fresh Postgres (user-core.yml
// "Initialize test database"), so anything added to only one source ships to
// production while replayed environments silently diverge. This guard turns the
// drift classes below into hard failures:
//
//   1. migration filename convention (ops/run-migrations.sh validate_name)
//   2. migration number collisions (lexicographic apply order becomes ambiguous)
//   3. duplicate CREATE TABLE across different migrations (last writer wins)
//   4. unguarded CREATE TABLE in migrations (replay crash-loops after partial apply)
//   5. tables present in schema.sql but created by no migration
//   6. runtime CREATE TABLE statements in Go without a backing migration
//   7. orphan SQL outside the canonical directories
//
// Usage: node tools/verify-schema-consistency.mjs [--json]
// Exits non-zero when any error-level finding exists. Allowlists below carry
// comments explaining each historical exception; shrink them, never grow them.

import { readdirSync, readFileSync, existsSync, realpathSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// ---------------------------------------------------------------------------
// Allowlists — historical debt frozen at introduction of this guard.
// Entries may be removed once the underlying drift is fixed; new entries need
// a written rationale here.
// ---------------------------------------------------------------------------

// Parallel release work reused numbers; ledger tracks full filenames so nothing
// was lost, but apply order inside a number is lexicographic by suffix.
// Exact stem sets per number: adding another stem to these numbers fails here.
const KNOWN_NUMBER_COLLISIONS = {
  "047": ["storage-backup-purpose", "wechat-virtual-payment"],
  "050": ["commercial-billing-wechat", "wechat-virtual-custom-token-unit"],
  "056": ["connector-qr-authorization", "wechat-virtual-test-token-1fen"],
  "078": ["promotion-invite-token", "publish-full-legal-agreements"],
};

// Tables created by more than one migration. IF NOT EXISTS makes the later
// statement a no-op, so shape drift between definitions goes unnoticed.
const KNOWN_DUPLICATE_TABLES = {
  xz_payment_events: [
    "021-runtime-projections.sql",
    "032-payment-callback-events.sql",
  ],
  xz_tenants: [
    "033-knowledge-tenant-foundation.sql",
    "039-user-role-permission-v6.sql",
  ],
  xz_organizations: [
    "033-knowledge-tenant-foundation.sql",
    "039-user-role-permission-v6.sql",
  ],
};

// Unguarded CREATE TABLE in migrations. Migration 105 builds scratch tables;
// the points domain has an in-flight refactor branch, so this is allowlisted
// instead of patched until that lands.
const KNOWN_UNGUARDED_CREATES = {
  "105-personal-point-legacy-reservation-attribution.sql": [
    "migration_105_active_scope",
    "migration_105_personal_tasks",
    "migration_105_attribution",
  ],
};

// Epoch-zero tables living only in schema.sql: no migration ever creates them,
// fresh environments get them solely from the schema.sql baseline load.
const BASELINE_ONLY_TABLES = new Set([
  "agent_call_logs",
  "agents",
  "assets",
  "audit_logs",
  "channel_agents",
  "commissions",
  "generation_task_attempts",
  "generation_tasks",
  "geo_brands",
  "geo_monitor_tasks",
  "membership_plans",
  "model_definitions",
  "model_providers",
  "orders",
  "payment_events",
  "point_accounts",
  "point_transactions",
  "presentations",
  "users",
  "withdrawal_requests",
]);

// Runtime Go DDL creating tables no migration covers. Empty on purpose:
// today every runtime table has a migration. Add entries only with a comment
// naming the owning team/epic and the plan to migrate the DDL out of Go.
const KNOWN_RUNTIME_ONLY_TABLES = new Set([]);

// Directories that must not contain tracked SQL migration files. The ppt task
// DDL used to live duplicated here, in migration 0xx, and in runtime Go DDL.
const FORBIDDEN_MIGRATION_DIRS = ["backend-go/migrations"];

const MIGRATION_NAME_RE = /^\d{3}-[a-z0-9]+(?:-[a-z0-9]+)*(?:\.(?:up|down))?\.sql$/;
const CREATE_TABLE_RE =
  /\bcreate\s+(?:local\s+|global\s+)?(?:temp\s+|temporary\s+)?table\s+(if\s+not\s+exists\s+)?(?:public\.)?"?([a-zA-Z0-9_]+)"?/gi;

function extractCreateTables(sql) {
  const found = [];
  for (const match of sql.matchAll(CREATE_TABLE_RE)) {
    found.push({
      table: match[2],
      guarded: Boolean(match[1]),
    });
  }
  return found;
}

export function parseMigrationName(filename) {
  const numberMatch = filename.match(/^(\d{3})-/);
  const stemMatch = filename.match(/^\d{3}-([a-z0-9-]+?)(?:\.(?:up|down))?\.sql$/);
  return {
    number: numberMatch ? numberMatch[1] : null,
    stem: stemMatch ? stemMatch[1] : null,
    valid: MIGRATION_NAME_RE.test(filename),
    isDown: filename.endsWith(".down.sql"),
  };
}

export function checkFilenames(filenames) {
  return filenames
    .filter((name) => !parseMigrationName(name).valid)
    .map((name) => ({
      severity: "error",
      check: "filename-convention",
      message: `database/migrations/${name} does not match NNN-lowercase-slug[.up|.down].sql; ops/run-migrations.sh would reject or misorder it`,
    }));
}

export function checkNumberCollisions(entries) {
  const findings = [];
  const effective = entries.filter((entry) => !entry.isDown);
  const byNumber = new Map();
  for (const entry of effective) {
    if (!byNumber.has(entry.number)) byNumber.set(entry.number, []);
    byNumber.get(entry.number).push(entry.filename);
  }
  for (const [number, filenames] of [...byNumber.entries()].sort()) {
    if (filenames.length <= 1) continue;
    const known = KNOWN_NUMBER_COLLISIONS[number];
    if (!known) {
      findings.push({
        severity: "error",
        check: "number-collision",
        message: `migration number ${number} used by ${filenames.length} files (${filenames.join(", ")}); pick the next free number — apply order inside a collision is lexicographic, not intentional`,
      });
      continue;
    }
    const stems = filenames.map((name) => parseMigrationName(name).stem).sort();
    const expected = [...known].sort();
    if (stems.join("|") !== expected.join("|")) {
      findings.push({
        severity: "error",
        check: "number-collision",
        message: `collision group ${number} changed: now [${stems.join(", ")}], allowlisted [${expected.join(", ")}]; collisions are frozen debt — use a free number instead of extending these`,
      });
    }
  }
  return findings;
}

export function checkDuplicateCreates(createsByFile) {
  const findings = [];
  const ownersByTable = new Map();
  for (const [filename, creates] of createsByFile) {
    for (const { table } of creates) {
      if (!ownersByTable.has(table)) ownersByTable.set(table, new Set());
      ownersByTable.get(table).add(filename);
    }
  }
  for (const [table, owners] of [...ownersByTable.entries()].sort()) {
    if (owners.size <= 1) continue;
    const files = [...owners].sort();
    const known = KNOWN_DUPLICATE_TABLES[table];
    if (!known) {
      findings.push({
        severity: "error",
        check: "duplicate-create-table",
        message: `${table} created by ${files.length} migrations (${files.join(", ")}); with IF NOT EXISTS the later definition silently no-ops — keep exactly one owner`,
      });
      continue;
    }
    const expected = [...known].sort();
    if (files.join("|") !== expected.join("|")) {
      findings.push({
        severity: "error",
        check: "duplicate-create-table",
        message: `duplicate ownership of ${table} changed: now [${files.join(", ")}], allowlisted [${expected.join(", ")}]`,
      });
    }
  }
  return findings;
}

export function checkUnguardedCreates(createsByFile) {
  const findings = [];
  for (const [filename, creates] of createsByFile) {
    const allowlisted = new Set(KNOWN_UNGUARDED_CREATES[filename] ?? []);
    for (const { table, guarded } of creates) {
      if (!guarded && !allowlisted.has(table)) {
        findings.push({
          severity: "error",
          check: "unguarded-create-table",
          message: `${filename}: CREATE TABLE ${table} lacks IF NOT EXISTS — a retry after partial failure crashes the replay (ON_ERROR_STOP=1)`,
        });
      }
    }
  }
  return findings;
}

export function checkDownPairing(entries) {
  const findings = [];
  const stems = new Set(
    entries.filter((entry) => !entry.isDown).map((entry) => entry.filename),
  );
  for (const entry of entries.filter((entry) => entry.isDown)) {
    const upVariants = [
      entry.filename.replace(/\.down\.sql$/, ".sql"),
      entry.filename.replace(/\.down\.sql$/, ".up.sql"),
    ];
    if (!upVariants.some((variant) => stems.has(variant))) {
      findings.push({
        severity: "error",
        check: "down-pairing",
        message: `database/migrations/${entry.filename} has no matching up migration (${upVariants.join(" or ")})`,
      });
    }
  }
  return findings;
}

export function checkBaselineCoverage(schemaTables, migratedTables) {
  const findings = [];
  for (const table of [...schemaTables].sort()) {
    if (migratedTables.has(table) || BASELINE_ONLY_TABLES.has(table)) continue;
    findings.push({
      severity: "error",
      check: "baseline-coverage",
      message: `${table} exists in database/schema.sql but is created by no migration and not in BASELINE_ONLY_TABLES — production databases built by ops/run-migrations.sh will never receive it; add a migration, not just schema.sql`,
    });
  }
  return findings;
}

export function checkRuntimeDdlCoverage(runtimeTables, migratedTables, baselineTables) {
  const findings = [];
  const sorted = [...runtimeTables.values()].sort((a, b) =>
    a.table.localeCompare(b.table),
  );
  for (const { table, locations } of sorted) {
    if (
      migratedTables.has(table) ||
      baselineTables.has(table) ||
      KNOWN_RUNTIME_ONLY_TABLES.has(table)
    ) {
      continue;
    }
    findings.push({
      severity: "error",
      check: "runtime-ddl-coverage",
      message: `Go code creates ${table} at runtime (${locations}) but no migration defines it — schema managed outside database/migrations never reaches ops-run production databases`,
    });
  }
  return findings;
}

function listMigrations(migrationsDir) {
  if (!existsSync(migrationsDir)) return [];
  return readdirSync(migrationsDir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".sql"))
    .map((entry) => entry.name)
    .sort();
}

function walkGoFiles(rootDir, rootPackageDir) {
  const files = [];
  const stack = [path.join(rootDir, rootPackageDir)];
  while (stack.length > 0) {
    const current = stack.pop();
    let dirents;
    try {
      dirents = readdirSync(current, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const dirent of dirents) {
      const full = path.join(current, dirent.name);
      if (dirent.isDirectory()) {
        stack.push(full);
      } else if (dirent.isFile() && dirent.name.endsWith(".go") && !dirent.name.endsWith("_test.go")) {
        files.push(full);
      }
    }
  }
  return files;
}

export function collectFindings(root = process.cwd()) {
  const migrationsDir = path.join(root, "database", "migrations");
  const schemaPath = path.join(root, "database", "schema.sql");

  const filenames = listMigrations(migrationsDir);
  const entries = filenames.map((filename) => ({
    filename,
    ...parseMigrationName(filename),
  }));

  const createsByFile = new Map();
  const migratedTables = new Set();
  for (const filename of filenames) {
    const sql = readFileSync(path.join(migrationsDir, filename), "utf8");
    const creates = extractCreateTables(sql);
    createsByFile.set(filename, creates);
    for (const { table } of creates) migratedTables.add(table);
  }

  const schemaSql = readFileSync(schemaPath, "utf8");
  const baselineTables = new Set(
    extractCreateTables(schemaSql).map(({ table }) => table),
  );

  const runtimeTables = new Map();
  for (const goFile of walkGoFiles(root, "backend-go")) {
    const sql = readFileSync(goFile, "utf8");
    for (const { table } of extractCreateTables(sql)) {
      if (!runtimeTables.has(table)) runtimeTables.set(table, new Set());
      runtimeTables.get(table).add(path.relative(root, goFile).replaceAll("\\", "/"));
    }
  }
  const runtimeEntries = new Map(
    [...runtimeTables.entries()].map(([table, locations]) => [
      table,
      { table, locations: [...locations].join(", ") },
    ]),
  );

  const forbiddenDirFindings = [];
  for (const relDir of FORBIDDEN_MIGRATION_DIRS) {
    if (existsSync(path.join(root, relDir))) {
      forbiddenDirFindings.push({
        severity: "error",
        check: "orphan-migration-dir",
        message: `${relDir}/ exists — migration SQL must live only in database/migrations (canonical) or database/drafts (non-executed drafts)`,
      });
    }
  }

  return [
    ...checkFilenames(filenames),
    ...checkNumberCollisions(entries),
    ...checkDuplicateCreates(createsByFile),
    ...checkUnguardedCreates(createsByFile),
    ...checkDownPairing(entries),
    ...checkBaselineCoverage(baselineTables, migratedTables),
    ...checkRuntimeDdlCoverage(runtimeEntries, migratedTables, baselineTables),
    ...forbiddenDirFindings,
  ];
}

function main() {
  const root = process.cwd();
  const findings = collectFindings(root);
  const json = process.argv.includes("--json");
  if (json) {
    console.log(JSON.stringify(findings, null, 2));
  } else if (findings.length === 0) {
    console.log("schema consistency: OK");
  } else {
    for (const finding of findings) {
      console.error(`[${finding.severity}] ${finding.check}: ${finding.message}`);
    }
    console.error(`schema consistency: ${findings.length} finding(s)`);
  }
  process.exitCode = findings.some((finding) => finding.severity === "error") ? 1 : 0;
}

function isEntrypoint() {
  if (!process.argv[1]) return false;
  try {
    return (
      realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))
    );
  } catch {
    return false;
  }
}

if (isEntrypoint()) {
  main();
}
