import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";
import ts from "typescript";

const root = process.cwd();

function loadGuestBrowseModule(uni) {
  const source = readFileSync(join(root, "apps/user-uni/src/features/auth/guestBrowse.ts"), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const module = { exports: {} };
  // eslint-disable-next-line no-new-func
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

test("every login mode and the phone authorization denial sheet expose prominent guest browsing", () => {
  const page = readFileSync(join(root, "apps/user-uni/src/pages/WechatLoginPage.vue"), "utf8");
  const prominentGuestActions = page.match(/class="auth-guest-enter-button(?:\s|")/g) || [];
  assert.equal(prominentGuestActions.length, 4);
  assert.match(page, /暂不登录，进入首页/);
  assert.doesNotMatch(page, /暂不登录，先浏览功能/);

  const deniedSheet = page.match(/<BottomSheet[^>]*authorizationSheetVisible[\s\S]*?<\/BottomSheet>/)?.[0] || "";
  assert.match(deniedSheet, /title="选择登录方式"/);
  assert.match(deniedSheet, /@close="closeAuthorizationSheet\(\)"/);
  assert.match(deniedSheet, /暂不登录，进入首页/);
  assert.doesNotMatch(deniedSheet, /:closable="false"|:close-on-overlay="false"/);
});

test("login defaults to USER home instead of agent or operation overview", () => {
  const page = readFileSync(join(root, "apps/user-uni/src/pages/WechatLoginPage.vue"), "utf8");
  assert.match(page, /if \(roles\?\.includes\("USER"\)\) return "USER"/);
  assert.doesNotMatch(
    page,
    /function defaultRole[\s\S]*if \(roles\?\.includes\("OPERATION"\)\) return "OPERATION";\s*if \(roles\?\.includes\("AGENT"\)\) return "AGENT"/,
  );
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
