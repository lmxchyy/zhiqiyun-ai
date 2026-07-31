import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const generationSource = await readFile(
  new URL("../packages/business-sdk/src/generation.ts", import.meta.url),
  "utf8",
);
const typesSource = await readFile(
  new URL("../packages/business-sdk/src/types.ts", import.meta.url),
  "utf8",
);

test("video estimate SDK uses the read-only generation estimate endpoint", () => {
  assert.match(generationSource, /estimateVideo\(request:\s*CreateGenerationTaskRequest\)/);
  assert.match(generationSource, /"\/api\/v1\/generation-tasks\/estimate"/);
  assert.match(generationSource, /body:\s*request/);
  assert.match(generationSource, /auth:\s*"required"/);
  assert.match(generationSource, /retryOnUnauthorized:\s*false/);
});

test("video estimate response exposes resolved billing evidence", () => {
  for (const field of [
    "model: string",
    "estimatedPoints: number",
    "billingType: string",
    "quantityField: string",
    "quantity: number",
    "note: string",
  ]) {
    assert.ok(typesSource.includes(field), `missing estimate response field: ${field}`);
  }
  assert.match(typesSource, /estimateVideo\(request:\s*CreateGenerationTaskRequest\):\s*Promise<VideoGenerationEstimate>/);
});
