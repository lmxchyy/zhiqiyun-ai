import { writeFile } from "node:fs/promises";

import { buildProfessionalDeck } from "./professional-deck.mjs";
import { PptxGenJSRenderer } from "./pptxgenjs-renderer.mjs";

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

async function main() {
  const mode = process.argv[2];
  const request = await readStdin();
  if (mode === "compile") {
    const result = buildProfessionalDeck(request);
    process.stdout.write(JSON.stringify({
      deckId: result.deck.deckId,
      revision: result.deck.revision,
      slideCount: result.deck.slides.length,
      deck: result.deck,
      layoutResult: result.layoutResult,
      renderInput: result.renderInput,
      qualityValid: result.quality.valid,
      qualityIssues: result.quality.diagnostics.map((item) => `${item.code}: ${item.message}`),
    }));
    return;
  }
  if (mode === "render") {
    const outputPath = process.argv[3];
    if (!outputPath) throw new Error("PPT V2 Slice B render output path is required");
    const assetData = request.assetData ?? {};
    const renderer = new PptxGenJSRenderer({
      resolveAsset(asset) {
        const data = assetData[asset.id];
        if (typeof data !== "string" || !data.startsWith("data:")) {
          throw new Error(`PPT V2 asset ${asset.id} was not resolved from private storage`);
        }
        return { data };
      },
    });
    const pptx = await renderer.render(request.compilation.renderInput);
    await writeFile(outputPath, pptx);
    process.stdout.write(JSON.stringify({
      deckId: request.compilation.deckId,
      revision: request.compilation.revision,
      slideCount: request.compilation.slideCount,
      bytes: pptx.length,
    }));
    return;
  }
  throw new Error(`unsupported PPT V2 Slice B CLI mode ${mode}`);
}

await main();
