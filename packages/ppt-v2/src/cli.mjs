#!/usr/bin/env node

import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";

import { PptxGenJSRenderer } from "./pptxgenjs-renderer.mjs";
import { buildPhase1VerticalSlice } from "./vertical-slice.mjs";

async function readStandardInput() {
  const chunks = [];
  for await (const chunk of process.stdin) {
    chunks.push(chunk);
  }
  return Buffer.concat(chunks).toString("utf8");
}

const [outputArgument, inputArgument] = process.argv.slice(2);
if (!outputArgument) {
  console.error("Usage: node packages/ppt-v2/src/cli.mjs <output.pptx> [legacy-input.json]");
  process.exitCode = 2;
} else {
  const outputPath = resolve(outputArgument);
  if (!outputPath.toLowerCase().endsWith(".pptx")) {
    throw new Error("Output path must end with .pptx");
  }
  const inputText = inputArgument
    ? await readFile(resolve(inputArgument), "utf8")
    : await readStandardInput();
  const input = JSON.parse(inputText);
  const slice = buildPhase1VerticalSlice(input);
  const buffer = await new PptxGenJSRenderer().render(slice.renderInput);
  await mkdir(dirname(outputPath), { recursive: true });
  await writeFile(outputPath, buffer);
  const persisted = await readFile(outputPath);
  if (!persisted.equals(buffer)) {
    throw new Error("persisted PPTX bytes do not match renderer output");
  }
  process.stdout.write(JSON.stringify({
    deckId: slice.deck.deckId,
    revision: slice.deck.revision,
    slideCount: slice.deck.slides.length,
    bytes: buffer.byteLength,
  }));
}
