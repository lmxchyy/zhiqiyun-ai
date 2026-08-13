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

function assertComponentReferenceResolves(fromRelativePath, componentReference) {
  const basePath = path.resolve(path.dirname(outputFile(fromRelativePath)), componentReference);
  for (const extension of [".js", ".json", ".wxml", ".wxss"]) {
    assert.equal(
      fs.existsSync(`${basePath}${extension}`),
      true,
      `${fromRelativePath} has unresolved component ${componentReference}${extension}`,
    );
  }
}

function listTextOutputFiles(directory = outputRoot) {
  const files = [];
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const entryPath = path.resolve(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...listTextOutputFiles(entryPath));
      continue;
    }
    if (entry.isFile() && [".js", ".json", ".wxml", ".wxss"].includes(path.extname(entry.name))) {
      files.push(path.relative(outputRoot, entryPath).split(path.sep).join("/"));
    }
  }
  return files;
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

test("PPT editor component stays inside user-creation subpackage", () => {
  const mainComponent = "components/PptDocumentGeneration";
  const subpackageComponent = "pages/user-creation/components/PptDocumentGeneration";
  const pageJs = "pages/user-creation/UserPptEditorPage.js";
  const pageJson = "pages/user-creation/UserPptEditorPage.json";

  for (const extension of ["js", "json", "wxml", "wxss"]) {
    assert.equal(fs.existsSync(outputFile(`${mainComponent}.${extension}`)), false);
    assert.equal(fs.existsSync(outputFile(`${subpackageComponent}.${extension}`)), true);
  }

  const appJson = JSON.parse(readOutput("app.json"));
  const userCreationPackage = appJson.subPackages.find(item => item.root === "pages/user-creation");
  assert.ok(userCreationPackage);
  assert.ok(userCreationPackage.pages.includes("UserPptEditorPage"));

  assert.match(readOutput(pageJs), /"\.\/components\/PptDocumentGeneration\.js"/);
  const pageConfig = JSON.parse(readOutput(pageJson));
  assert.equal(
    pageConfig.usingComponents["ppt-document-generation"],
    "./components/PptDocumentGeneration",
  );
  assertComponentReferenceResolves(pageJson, pageConfig.usingComponents["ppt-document-generation"]);

  const componentJs = readOutput(`${subpackageComponent}.js`);
  for (const requiredModule of [
    "../../../common/vendor.js",
    "../../../api/files.js",
    "../../../api/client.js",
    "../../../features/auth/gate.js",
    "../../../api/ppt.js",
  ]) {
    assert.match(componentJs, new RegExp(`require\\(["']${requiredModule.replaceAll(".", "\\.")}["']\\)`));
  }
  assert.match(
    componentJs,
    /["']\.\.\/\.\.\/\.\.\/components\/compliance\/AiGeneratedContentNotice\.js["']/,
  );
  assertRelativeRequiresResolve(`${subpackageComponent}.js`);

  const componentConfig = JSON.parse(readOutput(`${subpackageComponent}.json`));
  assert.equal(
    componentConfig.usingComponents["ai-generated-content-notice"],
    "../../../components/compliance/AiGeneratedContentNotice",
  );
  assertComponentReferenceResolves(
    `${subpackageComponent}.json`,
    componentConfig.usingComponents["ai-generated-content-notice"],
  );

  const componentOutputs = new Set(
    ["js", "json", "wxml", "wxss"].map(extension => `${subpackageComponent}.${extension}`),
  );
  const references = listTextOutputFiles()
    .filter(relativePath => !componentOutputs.has(relativePath))
    .filter(relativePath => readOutput(relativePath).includes("PptDocumentGeneration"))
    .sort();
  assert.deepEqual(references, [pageJs, pageJson].sort());
});

test("generated login form remains preserved behind the lightweight login gate", () => {
  const entry = readOutput("pages/WechatLoginPage.js");
  const form = readOutput("pages/WechatLoginFormPage.js");

  assert.match(entry, /wx\.redirectTo/);
  assert.match(entry, /\/pages\/WechatLoginFormPage/);
  assert.match(form, /require\("\.\.\/common\/vendor\.js"\)/);
  assert.match(form, /__name:"WechatLoginPage"/);
  assert.match(form, /\.\.\/components\/auth\/LoginCard\.js/);
});

test("existing native generation controls remain patched in generated output", () => {
  const workbenchJs = readOutput("components/MiniProgramRoleWorkbench.js");
  const workbenchWxml = readOutput("components/MiniProgramRoleWorkbench.wxml");
  const homeJs = readOutput("components/v531/V531HomePage.js");
  const homeWxml = readOutput("components/v531/V531HomePage.wxml");
  const studioJs = readOutput("components/v531/V531StudioPage.js");
  const studioWxml = readOutput("components/v531/V531StudioPage.wxml");

  for (const handler of ["nativeBackToCreation", "nativeGenerate", "nativeChooseReferenceImages"]) {
    assert.match(workbenchJs, new RegExp(handler));
    assert.match(workbenchWxml, new RegExp(handler));
  }
  for (const handler of ["nativeHomePromptInput", "nativeHomePromptSubmit"]) {
    assert.match(homeJs, new RegExp(handler));
    assert.match(homeWxml, new RegExp(handler));
  }
  for (const handler of ["nativeStudioChooseReference", "nativeStudioChooseFile", "nativeStudioGenerate"]) {
    assert.match(studioJs, new RegExp(handler));
    assert.match(studioWxml, new RegExp(handler));
  }
});
