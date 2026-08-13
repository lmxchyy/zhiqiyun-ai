import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import ts from "typescript";
import { createRequire } from "node:module";

const root = process.cwd();
const require = createRequire(import.meta.url);
const { parse } = require(join(root, "apps/user-uni/node_modules/@vue/compiler-sfc"));

function loadGuestBrowseModule(uni) {
  const source = readFileSync(join(root, "apps/user-uni/src/features/auth/guestBrowse.ts"), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const module = { exports: {} };
  new Function("exports", "module", "uni", compiled)(module.exports, module, uni);
  return module.exports;
}

test("guest browse persists consent, suppresses repeated prompts and opens home", () => {
  const storage = new Map();
  const navigation = [];
  const uni = {
    getStorageSync: key => storage.get(key),
    setStorageSync: (key, value) => storage.set(key, value),
    removeStorageSync: key => storage.delete(key),
    switchTab: options => navigation.push(["switchTab", options.url]),
    reLaunch: options => navigation.push(["reLaunch", options.url]),
  };
  const guest = loadGuestBrowseModule(uni);

  assert.equal(guest.hasAcceptedGuestBrowse(), false);
  guest.enterGuestBrowseHome();
  assert.equal(guest.hasAcceptedGuestBrowse(), true);
  assert.equal(guest.isLoginPromptSuppressed(), true);
  assert.deepEqual(navigation, [["switchTab", "/pages/user/UserHomePage"]]);

  guest.clearGuestBrowse();
  assert.equal(guest.hasAcceptedGuestBrowse(), false);
  assert.equal(guest.isLoginPromptSuppressed(), false);
});

test("every login mode and the phone authorization denial sheet expose guest browsing", () => {
  const page = readFileSync(join(root, "apps/user-uni/src/pages/WechatLoginPage.vue"), "utf8");
  const prominentGuestActions = page.match(/class="auth-guest-enter-button(?:\s|\")/g) || [];
  assert.equal(prominentGuestActions.length, 4);
  assert.doesNotMatch(page, /暂不登录，先浏览功能/);

  const deniedSheet = page.match(/<BottomSheet[^>]*authorizationSheetVisible[\s\S]*?<\/BottomSheet>/)?.[0] || "";
  assert.match(deniedSheet, /title="选择登录方式"/);
  assert.match(deniedSheet, /@close="closeAuthorizationSheet\(\)"/);
  assert.match(deniedSheet, /暂不登录，进入首页/);
  assert.doesNotMatch(deniedSheet, /:closable="false"|:close-on-overlay="false"/);
});

test("guest mode prevents background authorization failures from forcing login", () => {
  const gate = readFileSync(join(root, "apps/user-uni/src/features/auth/gate.ts"), "utf8");
  const client = readFileSync(join(root, "apps/user-uni/src/api/client.ts"), "utf8");
  const authStore = readFileSync(join(root, "apps/user-uni/src/stores/auth.ts"), "utf8");

  assert.match(gate, /isLoginPromptSuppressed/);
  assert.match(gate, /pendingActions\.clear\(\)/);
  assert.match(client, /hasAcceptedGuestBrowse/);
  assert.match(authStore, /handleTokenExpired\(\)[\s\S]*acceptGuestBrowse\(\)/);
  assert.match(authStore, /logout\(\)[\s\S]*acceptGuestBrowse\(\)/);
});

test("video creation detail uses the optimized card layout instead of the generic legacy panel", () => {
  const file = join(root, "apps/user-uni/src/components/MiniProgramRoleWorkbench.vue");
  const source = readFileSync(file, "utf8");
  const { descriptor, errors } = parse(source, { filename: file });
  assert.deepEqual(errors, []);
  const template = descriptor.template?.content || "";

  for (const className of [
    "video-safety-banner",
    "video-prompt-card",
    "video-reference-card",
    "video-model-row",
    "video-basic-card",
    "video-generation-summary",
    "video-primary-generate",
  ]) {
    assert.match(template, new RegExp(`(?:^|[\\s\"'])${className}(?:[\\s\"']|$)`), `missing optimized section ${className}`);
  }
  assert.doesNotMatch(template, /class="v31-video-parameter-panel"/);
  assert.doesNotMatch(template, /video-prompt-optimize/);
});

test("release metadata records a valid version advance", () => {
  const metadata = JSON.parse(readFileSync(join(root, "apps/user-uni/mp-weixin.release.json"), "utf8"));
  const semver = /^\d+\.\d+\.\d+$/;
  assert.match(metadata.version, semver);
  assert.match(metadata.previousVersion, semver);
  if (metadata.publishedBaseline !== undefined) {
    assert.match(metadata.publishedBaseline, semver);
  }
  assert.equal(typeof metadata.description, "string");
  assert.ok(metadata.description.trim().length > 0, "description must not be empty");

  const toParts = value => value.split(".").map(Number);
  const [major, minor, patch] = toParts(metadata.version);
  const [prevMajor, prevMinor, prevPatch] = toParts(metadata.previousVersion);
  const advances =
    major > prevMajor ||
    (major === prevMajor && (minor > prevMinor || (minor === prevMinor && patch > prevPatch)));
  assert.ok(advances, "version must advance past previousVersion");
});
