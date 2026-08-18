import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import test from "node:test";

const requireFromUserUni = createRequire(new URL("../apps/user-uni/package.json", import.meta.url));
const typescript = requireFromUserUni("typescript");

function loadModuleSchemaRouting() {
  const moduleURL = new URL("../apps/user-uni/src/features/generation/moduleSchema.ts", import.meta.url);
  if (!existsSync(moduleURL)) return {};
  const compiled = typescript.transpileModule(readFileSync(moduleURL, "utf8"), {
    compilerOptions: {
      module: typescript.ModuleKind.CommonJS,
      target: typescript.ScriptTarget.ES2022,
    },
  }).outputText;
  const module = { exports: {} };
  // eslint-disable-next-line no-new-func
  new Function("exports", "module", compiled)(module.exports, module);
  return module.exports;
}

const routing = loadModuleSchemaRouting();

test("guest generation reads the exact public module schema", () => {
  assert.equal(typeof routing.exactModuleSchemaPath, "function");
  assert.equal(
    routing.exactModuleSchemaPath("image_generation", "gpt-image-2", true),
    "/api/v1/public/module-schema?module_code=image_generation&model_name=gpt-image-2",
  );
  assert.equal(
    routing.exactModuleSchemaPath("video_generation", "grok-imagine-1.5-video", true),
    "/api/v1/public/module-schema?module_code=video_generation&model_name=grok-imagine-1.5-video",
  );
});

test("authenticated generation keeps the entitlement-aware exact schema route", () => {
  assert.equal(typeof routing.exactModuleSchemaPath, "function");
  assert.equal(
    routing.exactModuleSchemaPath("image_generation", "gpt-image-2", false),
    "/api/v1/module-schema?module_code=image_generation&model_name=gpt-image-2",
  );
});
