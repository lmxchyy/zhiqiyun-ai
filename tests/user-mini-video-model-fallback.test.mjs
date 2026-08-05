import test from "node:test";
import assert from "node:assert/strict";

let fallbackModule = null;
try {
  fallbackModule = await import("../packages/business-sdk/dist/mappers.js");
} catch {
  fallbackModule = null;
}

test("video model fallback requires an explicit user decision", async () => {
  assert.equal(typeof fallbackModule?.confirmResolvedVideoModel, "function");
  const prompts = [];
  const model = await fallbackModule.confirmResolvedVideoModel(
    "seedance-fast-2.0",
    "doubao-seedance-2.0",
    async message => {
      prompts.push(message);
      return true;
    },
  );

  assert.equal(model, "doubao-seedance-2.0");
  assert.deepEqual(prompts, ["当前模型不可用，是否切换为 doubao-seedance-2.0？"]);
});

test("rejecting fallback prevents the resolved model from being used", async () => {
  assert.equal(await fallbackModule.confirmResolvedVideoModel(
    "seedance-fast-2.0",
    "doubao-seedance-2.0",
    async () => false,
  ), null);
});

test("unchanged model does not open a confirmation dialog", async () => {
  assert.equal(typeof fallbackModule?.confirmResolvedVideoModel, "function");
  let prompts = 0;
  const model = await fallbackModule.confirmResolvedVideoModel(
    "seedance-fast-2.0",
    "seedance-fast-2.0",
    async () => {
      prompts += 1;
      return true;
    },
  );

  assert.equal(model, "seedance-fast-2.0");
  assert.equal(prompts, 0);
});
