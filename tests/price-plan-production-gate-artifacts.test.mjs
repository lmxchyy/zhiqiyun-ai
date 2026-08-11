import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const gateRoot = new URL("../docs/acceptance/price-plan-v2-production-gate/", import.meta.url);

function readGateFile(name) {
  return readFileSync(new URL(name, gateRoot), "utf8");
}

test("the production-gate handoff contains every approved artifact", () => {
  for (const name of [
    "README.md",
    "release-freeze-runbook.md",
    "dba-readonly-preflight.sql",
    "dba-preflight-decision-table.md",
    "isolated-migration-rehearsal.md",
    "wechat-goods-manual-checklist.md",
    "sandbox-v2-quote-real-device-acceptance.md",
    "go-no-go-gate.md"
  ]) {
    assert.ok(readGateFile(name).trim().length > 100, `${name} must be a substantive handoff artifact`);
  }
});

test("the DBA production preflight is mechanically read-only", () => {
  const source = readGateFile("dba-readonly-preflight.sql");
  assert.match(source, /BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;/);
  assert.match(source, /ROLLBACK;/);
  assert.match(source, /xz_price_plans/);
  assert.match(source, /xz_order_price_quotes/);
  assert.match(source, /xz_price_plan_user_whitelist/);
  assert.match(source, /xz_channel_rollout_configs/);
  assert.doesNotMatch(
    source,
    /^\s*(?:INSERT|UPDATE|DELETE|ALTER|CREATE|DROP|TRUNCATE|GRANT|REVOKE|COPY|VACUUM|ANALYZE)\b/gim,
    "production preflight must contain SELECT-only SQL"
  );
});

test("the sandbox acceptance uses the V2 quote chain instead of legacy productCode", () => {
  const source = readGateFile("sandbox-v2-quote-real-device-acceptance.md");
  assert.match(source, /POST \/api\/v1\/payment\/price-quotes/);
  assert.match(source, /POST \/api\/v1\/payment\/test-price-quotes/);
  assert.match(source, /POST \/api\/v1\/payment\/wechat-virtual\/orders/);
  assert.match(source, /quoteId/);
  assert.match(source, /productCode[^\n]*(?:不能|不得|不可).*V2/);
});

test("the gate explicitly keeps all V2 flags disabled during preparation", () => {
  const source = readGateFile("go-no-go-gate.md");
  for (const flag of [
    "PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=false",
    "PRICE_PLAN_TEST_ENTRY_ENABLED=false",
    "SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=false"
  ]) {
    assert.match(source, new RegExp(flag));
  }
  assert.match(source, /NO-GO/);
  assert.match(source, /GO/);
});
