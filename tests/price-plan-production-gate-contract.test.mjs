import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const repoRoot = new URL("../", import.meta.url);
const composeSource = readFileSync(new URL("compose.prod.yml", repoRoot), "utf8");
const productionEnvTemplate = readFileSync(new URL(".env.production.example", repoRoot), "utf8");

const v2Flags = [
  "PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED",
  "PRICE_PLAN_TEST_ENTRY_ENABLED",
  "SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED"
];

test("production compose explicitly injects every V2 flag with a fail-closed default", () => {
  for (const flag of v2Flags) {
    const mapping = new RegExp(`^\\s{6}${flag}: "\\$\\{${flag}:-false\\}"$`, "m");
    assert.match(composeSource, mapping, `${flag} must be injected into the application container`);
    assert.equal(
      [...composeSource.matchAll(new RegExp(`^\\s+${flag}:`, "gm"))].length,
      1,
      `${flag} must have exactly one compose mapping`
    );
  }
});

test("production environment template declares every V2 flag disabled", () => {
  for (const flag of v2Flags) {
    assert.match(
      productionEnvTemplate,
      new RegExp(`^${flag}=false$`, "m"),
      `${flag} must be documented as disabled by default`
    );
    assert.equal(
      [...productionEnvTemplate.matchAll(new RegExp(`^${flag}=`, "gm"))].length,
      1,
      `${flag} must have exactly one template definition`
    );
  }
});
