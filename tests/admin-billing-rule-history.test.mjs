import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(
  new URL("../admin-vue/src/components/billing/BillingCenterV1.vue", import.meta.url),
  "utf8"
);

test("billing rules default to current versions and expose history explicitly", () => {
  assert.match(source, /showRuleHistory/);
  assert.match(source, /currentBillingRules/);
  assert.match(source, /显示历史版本/);
  assert.match(source, /当前生效/);
  assert.match(source, /历史归档/);
});
