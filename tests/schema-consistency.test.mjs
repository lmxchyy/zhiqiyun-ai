import assert from "node:assert/strict";
import test from "node:test";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  checkDuplicateCreates,
  checkFilenames,
  checkNumberCollisions,
  collectFindings,
} from "../tools/verify-schema-consistency.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

test("schema sources are consistent with the migration ledger contract", () => {
  const findings = collectFindings(root);
  assert.deepEqual(
    findings,
    [],
    `schema consistency drift detected:\n${findings.map((f) => `- [${f.check}] ${f.message}`).join("\n")}`,
  );
});

test("filename convention rejects timestamp-style and uppercase names", () => {
  const findings = checkFilenames([
    "20260716_01_create_ppt_tasks.up.sql",
    "047-Wechat-Virtual-Payment.sql",
    "999-lowercase-slug.sql",
  ]);
  assert.equal(findings.length, 2);
  assert.ok(findings.every((finding) => finding.check === "filename-convention"));
});

test("new number collisions fail while allowlisted groups pass", () => {
  const collisions = checkNumberCollisions([
    { number: "050", filename: "050-commercial-billing-wechat.sql", isDown: false },
    { number: "050", filename: "050-wechat-virtual-custom-token-unit.sql", isDown: false },
  ]);
  assert.deepEqual(collisions, []);

  const fresh = checkNumberCollisions([
    { number: "110", filename: "110-alpha.sql", isDown: false },
    { number: "110", filename: "110-beta.sql", isDown: false },
  ]);
  assert.equal(fresh.length, 1);
  assert.equal(fresh[0].check, "number-collision");
});

test("duplicate CREATE TABLE outside the allowlist fails", () => {
  const duplicates = checkDuplicateCreates(
    new Map([
      ["021-runtime-projections.sql", [{ table: "xz_payment_events", guarded: true }]],
      ["032-payment-callback-events.sql", [{ table: "xz_payment_events", guarded: true }]],
      ["040-something.sql", [{ table: "xz_new_thing", guarded: true }]],
      ["041-other.sql", [{ table: "xz_new_thing", guarded: true }]],
    ]),
  );
  assert.equal(duplicates.length, 1);
  assert.match(duplicates[0].message, /xz_new_thing/);
});
