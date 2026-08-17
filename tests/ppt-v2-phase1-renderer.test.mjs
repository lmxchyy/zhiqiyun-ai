import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmdirSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import JSZip from "jszip";

import { PptxGenJSRenderer } from "../packages/ppt-v2/src/pptxgenjs-renderer.mjs";
import { PptxRenderer } from "../packages/ppt-v2/src/renderer-port.mjs";
import { buildPhase1VerticalSlice } from "../packages/ppt-v2/src/vertical-slice.mjs";

function professionalBusinessInput() {
  return {
    generateRequest: {
      prompt: "2027 年企业增长计划",
      slideCount: 12,
      language: "zh-CN",
      audience: "管理层",
      scenario: "年度经营会",
      enableWebSearch: false,
      imageSource: "none",
    },
    outline: {
      title: "2027 年企业增长计划",
      slides: [
        { page: 1, title: "2027 年企业增长计划", summary: "从共识到执行", bulletPoints: [], layout: "cover", slideType: "cover" },
        {
          page: 2,
          title: "三个增长支柱形成闭环",
          summary: "产品建立价值，渠道放大触达，客户成功驱动续费。",
          bulletPoints: ["产品：聚焦高价值场景", "渠道：建立可复制打法", "客户成功：提升续费与扩容"],
          layout: "content",
          slideType: "text_image",
        },
      ],
    },
    taskContext: {
      taskId: "ppt_legacy_001",
      userId: "user_001",
      clientRequestId: "request_001",
      status: "success",
      title: "2027 年企业增长计划",
      speakerNotesByPage: { 2: "说明三个支柱如何在同一经营节奏中协同。" },
    },
  };
}

test("PptxGenJS adapter implements the PptxRenderer port without mutating RenderInput", async () => {
  const renderer = new PptxGenJSRenderer();
  const { renderInput } = buildPhase1VerticalSlice(professionalBusinessInput());
  const before = JSON.stringify(renderInput);

  assert.ok(renderer instanceof PptxRenderer);
  const buffer = await renderer.render(renderInput);

  assert.equal(JSON.stringify(renderInput), before);
  assert.ok(Buffer.isBuffer(buffer));
  assert.equal(buffer.subarray(0, 2).toString("ascii"), "PK");
  assert.ok(buffer.byteLength > 15_000);
});

test("renderer emits two editable slides, exact element IDs, bullets, notes, and z-order", async () => {
  const { renderInput } = buildPhase1VerticalSlice(professionalBusinessInput());
  const buffer = await new PptxGenJSRenderer().render(renderInput);
  const zip = await JSZip.loadAsync(buffer);
  const paths = Object.keys(zip.files);

  assert.equal(paths.filter((path) => /^ppt\/slides\/slide\d+\.xml$/.test(path)).length, 2);
  assert.equal(paths.filter((path) => /^ppt\/notesSlides\/notesSlide\d+\.xml$/.test(path)).length, 2);
  assert.equal(paths.filter((path) => /^ppt\/media\/[^/]+$/.test(path)).length, 0, "text and shapes must remain native editable objects");

  const slide1 = await zip.file("ppt/slides/slide1.xml").async("string");
  const slide2 = await zip.file("ppt/slides/slide2.xml").async("string");
  const notes2 = await zip.file("ppt/notesSlides/notesSlide2.xml").async("string");
  const slide1Names = [...slide1.matchAll(/<p:cNvPr[^>]+name="([^"]+)"/g)].map((match) => match[1]);
  const slide2Names = [...slide2.matchAll(/<p:cNvPr[^>]+name="([^"]+)"/g)].map((match) => match[1]);

  assert.deepEqual(slide1Names, [
    "element_ppt_legacy_001_cover_accent",
    "element_ppt_legacy_001_cover_title",
    "element_ppt_legacy_001_cover_subtitle",
    "element_ppt_legacy_001_cover_footer",
  ]);
  assert.deepEqual(slide2Names, [
    "element_ppt_legacy_001_content_panel",
    "element_ppt_legacy_001_content_title",
    "element_ppt_legacy_001_content_body",
  ]);
  assert.equal((slide1.match(/<p:sp>/g) ?? []).length, 4);
  assert.equal((slide2.match(/<p:sp>/g) ?? []).length, 3);
  assert.match(slide2, /<a:buChar/);
  assert.match(slide2, /产品：聚焦高价值场景/);
  assert.match(notes2, /说明三个支柱如何在同一经营节奏中协同。/);
});

test("renderer output is byte-for-byte deterministic", async () => {
  const renderer = new PptxGenJSRenderer();
  const { renderInput } = buildPhase1VerticalSlice(professionalBusinessInput());

  const first = await renderer.render(renderInput);
  const second = await renderer.render(structuredClone(renderInput));

  assert.deepEqual(second, first);
});

test("renderer fails closed before PptxGenJS when LayoutResult is incomplete", async () => {
  const { renderInput } = buildPhase1VerticalSlice(professionalBusinessInput());
  renderInput.layoutResults[1].elements.pop();

  await assert.rejects(
    () => new PptxGenJSRenderer().render(renderInput),
    /missing layout element/i,
  );
});

test("OfficeCLI accepts the generated package without a repair warning", async () => {
  const { renderInput } = buildPhase1VerticalSlice(professionalBusinessInput());
  const buffer = await new PptxGenJSRenderer().render(renderInput);
  const directory = mkdtempSync(join(tmpdir(), "ppt-v2-phase1-renderer-"));
  const output = join(directory, "golden-1.pptx");
  writeFileSync(output, buffer);
  try {
    const validation = spawnSync("officecli", ["validate", output], { encoding: "utf8" });
    const validationOutput = `${validation.stdout ?? ""}\n${validation.stderr ?? ""}`;
    assert.equal(validation.status, 0, validationOutput);
    assert.match(validationOutput, /Validation passed: no errors found/i);
    assert.doesNotMatch(validationOutput, /repair warning|repair required|validation error/i);
  } finally {
    execFileSync("officecli", ["close", output], { encoding: "utf8" });
    unlinkSync(output);
    rmdirSync(directory);
  }
});
