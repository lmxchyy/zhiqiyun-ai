import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const generationSourceURL = new URL("../packages/business-sdk/src/generation.ts", import.meta.url);

async function loadGenerationSdk(request) {
  const source = (await readFile(generationSourceURL, "utf8"))
    .replace(
      'import { taskRequestFromDraft } from "./mappers";',
      "const taskRequestFromDraft = globalThis.taskRequestFromDraft;",
    );
  const output = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
  }).outputText;
  const module = { exports: {} };
  const context = vm.createContext({
    module,
    exports: module.exports,
    globalThis: null,
    taskRequestFromDraft: draft => ({
      type: "TEXT_TO_IMAGE",
      moduleCode: "image_generation",
      prompt: draft.prompt,
      model: draft.model,
      params: {},
    }),
  });
  context.globalThis = context;
  vm.runInContext(output, context, { filename: "generation.js" });
  return module.exports.createGenerationSdk({ request });
}

test("generation createTask returns the API request promise without wrapping it", async () => {
  const apiPromise = Promise.resolve({ id: "task_test", status: "PENDING" });
  const sdk = await loadGenerationSdk(() => apiPromise);

  const result = sdk.createTask({
    mode: "image",
    prompt: "test",
    model: "gpt-image-2",
    style: "commercial",
    size: "1024x1024",
    quality: "auto",
    count: 1,
    referenceImages: [],
  });

  assert.equal(result, apiPromise);
});
