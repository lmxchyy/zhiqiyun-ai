import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const clientSourceURL = new URL("../apps/user-uni/src/api/client.ts", import.meta.url);

async function loadApiFunction(request) {
  const source = await readFile(clientSourceURL, "utf8");
  const start = Math.max(
    source.lastIndexOf("export async function api"),
    source.lastIndexOf("export function api"),
  );
  assert.notEqual(start, -1, "api function not found");
  const output = ts.transpileModule(source.slice(start), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const module = { exports: {} };
  const context = vm.createContext({
    module,
    exports: module.exports,
    sharedApiClient: { request },
    normalizeHeaders: value => value || {},
    normalizeBody: value => value,
    requestTimeout: 600000,
  });
  vm.runInContext(output, context, { filename: "client-api.js" });
  return module.exports.api;
}

test("mini-program api returns the platform request promise without an async wrapper", async () => {
  let requestOptions;
  const platformPromise = Promise.resolve({ model_name: "grok-imagine-1.5-video" });
  const api = await loadApiFunction((_path, options) => {
    requestOptions = options;
    return platformPromise;
  });

  const result = api("/api/v1/module-schema", { method: "GET" });

  assert.equal(result, platformPromise);
  assert.equal(requestOptions.method, "GET");
  assert.deepEqual(requestOptions.headers, {});
  assert.equal(requestOptions.timeout, 600000);
});
