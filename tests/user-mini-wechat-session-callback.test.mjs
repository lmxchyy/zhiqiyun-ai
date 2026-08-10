import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFile } from "node:fs/promises";
import ts from "typescript";

const sourceURL = new URL(
  "../apps/user-uni/src/features/auth/wechatSession.ts",
  import.meta.url,
);

function keepWechatMiniProgramBranch(source) {
  let keep = true;
  return source.split(/\r?\n/).filter(line => {
    if (line.includes("#ifdef MP-WEIXIN")) {
      keep = true;
      return false;
    }
    if (line.includes("#ifndef MP-WEIXIN")) {
      keep = false;
      return false;
    }
    if (line.includes("#endif")) {
      keep = true;
      return false;
    }
    return keep;
  }).join("\n");
}

async function loadWechatSessionModule({ uni, loginAPI }) {
  const source = keepWechatMiniProgramBranch(await readFile(sourceURL, "utf8"))
    .replace('import { loginAPI } from "./api";', "const loginAPI = globalThis.loginAPI;");
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
    loginAPI,
    uni,
  });
  context.globalThis = context;
  vm.runInContext(output, context, { filename: "wechatSession.js" });
  return module.exports;
}

test("WeChat session refresh invokes the generation continuation after the server session is ready", async () => {
  let refreshedCode = "";
  let continued = 0;
  let failed = 0;
  const module = await loadWechatSessionModule({
    uni: {
      login(options) {
        options.success({ code: "wx-login-code" });
      },
    },
    loginAPI: {
      async refreshWechatSession(code) {
        refreshedCode = code;
      },
    },
  });

  const result = module.ensureWechatMiniProgramSession({
    success() {
      continued += 1;
    },
    fail() {
      failed += 1;
    },
  });
  await new Promise(resolve => setImmediate(resolve));

  assert.equal(result, undefined);
  assert.equal(refreshedCode, "wx-login-code");
  assert.equal(continued, 1);
  assert.equal(failed, 0);
});

test("WeChat session refresh reports failure without running the generation continuation", async () => {
  const expected = new Error("session refresh failed");
  let continued = 0;
  let receivedError;
  const module = await loadWechatSessionModule({
    uni: {
      login(options) {
        options.success({ code: "wx-login-code" });
      },
    },
    loginAPI: {
      async refreshWechatSession() {
        throw expected;
      },
    },
  });

  const result = module.ensureWechatMiniProgramSession({
    success() {
      continued += 1;
    },
    fail(error) {
      receivedError = error;
    },
  });
  if (result && typeof result.catch === "function") await result.catch(() => undefined);
  await new Promise(resolve => setImmediate(resolve));

  assert.equal(continued, 0);
  assert.equal(receivedError, expected);
});

test("WeChat session bridge reports a synchronous uni.login failure", async () => {
  const expected = new Error("uni.login failed synchronously");
  let receivedError;
  const module = await loadWechatSessionModule({
    uni: {
      login() {
        throw expected;
      },
    },
    loginAPI: {
      refreshWechatSession() {
        return Promise.resolve();
      },
    },
  });

  assert.doesNotThrow(() => module.ensureWechatMiniProgramSession({
    success() {
      assert.fail("generation continuation must not run");
    },
    fail(error) {
      receivedError = error;
    },
  }));
  assert.equal(receivedError, expected);
});

test("WeChat session bridge reports a synchronous refresh failure", async () => {
  const expected = new Error("session refresh failed synchronously");
  let receivedError;
  const module = await loadWechatSessionModule({
    uni: {
      login(options) {
        options.success({ code: "wx-login-code" });
      },
    },
    loginAPI: {
      refreshWechatSession() {
        throw expected;
      },
    },
  });

  assert.doesNotThrow(() => module.ensureWechatMiniProgramSession({
    success() {
      assert.fail("generation continuation must not run");
    },
    fail(error) {
      receivedError = error;
    },
  }));
  assert.equal(receivedError, expected);
});
