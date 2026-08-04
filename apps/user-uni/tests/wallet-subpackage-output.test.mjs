import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const appRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const outputRoot = path.resolve(appRoot, "dist", "build", "mp-weixin");

function outputFile(relativePath) {
  return path.resolve(outputRoot, relativePath);
}

function readOutput(relativePath) {
  return fs.readFileSync(outputFile(relativePath), "utf8");
}

function assertRelativeRequiresResolve(relativePath) {
  const filePath = outputFile(relativePath);
  const source = fs.readFileSync(filePath, "utf8");
  for (const match of source.matchAll(/require\(["'](\.[^"']+)["']\)/g)) {
    const resolved = path.resolve(path.dirname(filePath), match[1]);
    assert.ok(
      [resolved, `${resolved}.js`, path.resolve(resolved, "index.js")].some(candidate => fs.existsSync(candidate)),
      `${relativePath} has unresolved require ${match[1]}`,
    );
  }
}

test("personal wallet modules and rewritten imports stay inside user-account subpackage", () => {
  const page = "pages/user-account/UserWalletPage.js";
  const composable = "pages/user-account/composables/usePersonalPointsWallet.js";
  const domain = "pages/user-account/features/wallet/personalPointsWallet.js";

  assert.equal(fs.existsSync(outputFile("composables/usePersonalPointsWallet.js")), false);
  assert.equal(fs.existsSync(outputFile("features/wallet/personalPointsWallet.js")), false);
  assert.equal(fs.existsSync(outputFile(page)), true);
  assert.equal(fs.existsSync(outputFile(composable)), true);
  assert.equal(fs.existsSync(outputFile(domain)), true);

  assert.match(readOutput(page), /require\("\.\/composables\/usePersonalPointsWallet\.js"\)/);
  assert.match(readOutput(page), /require\("\.\/features\/wallet\/personalPointsWallet\.js"\)/);
  assert.match(readOutput(composable), /require\("\.\.\/\.\.\/\.\.\/api\/client\.js"\)/);
  assert.match(readOutput(composable), /require\("\.\.\/features\/wallet\/personalPointsWallet\.js"\)/);

  assertRelativeRequiresResolve(page);
  assertRelativeRequiresResolve(composable);
  assertRelativeRequiresResolve(domain);
});
