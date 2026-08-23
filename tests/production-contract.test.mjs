import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = new URL("../", import.meta.url);
const harnessPath = new URL("production-contract.harness.sh", new URL("../tests/", import.meta.url));
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";

test("production contract harness exists and passes", async () => {
  await access(harnessPath);
  const output = execFileSync(bash, ["tests/production-contract.harness.sh"], {
    cwd: fileURLToPath(root),
    encoding: "utf8"
  });
  assert.match(output, /production contract harness PASS/);
});

test("production contract documents the runtime and migration gates", async () => {
  const workflow = await readFile(new URL(".github/workflows/user-core.yml", root), "utf8");
  const docs = await readFile(new URL("../docs/architecture/production-contract-ci.md", import.meta.url), "utf8");
  assert.match(workflow, /^  production-contract:\s*$/m);
  assert.match(docs, /PRODUCTION_CONTRACT_CI_READY/);
  assert.match(docs, /service_completed_successfully/);
});
