import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");

const wsEndpoint = process.env.WX_AUTOMATOR_WS || "ws://127.0.0.1:9441";

async function main() {
  const miniProgram = await automator.connect({ wsEndpoint });
  try {
    const stack = await miniProgram.pageStack();
    console.log("stack", stack.map(page => page.path));
    const storage = await miniProgram.evaluate(() => {
      const legacy = wx.getStorageSync("xianzhiMiniProgramAuth");
      const auth = wx.getStorageSync("auth");
      return {
        token: wx.getStorageSync("token") || "",
        hasAuth: Boolean(auth),
        authKeys: auth && typeof auth === "object" ? Object.keys(auth) : [],
        hasLegacyAuth: Boolean(legacy),
        legacyKeys: legacy && typeof legacy === "object" ? Object.keys(legacy) : [],
        legacyAccessToken: legacy && legacy.accessToken || "",
        legacyRefreshToken: legacy && legacy.refreshToken || "",
        legacyUser: legacy && legacy.user || null,
      };
    });
    console.log("storage", JSON.stringify(storage, null, 2));
    const page = await miniProgram.currentPage();
    for (const selector of [
      "mini-program-role-workbench",
      "v531-home-page",
      ".state-card",
      ".hero-text-input",
      ".capability-feature-card",
    ]) {
      const element = await page.$(selector);
      console.log(selector, Boolean(element));
      if (element) {
        const wxml = await element.outerWxml().catch(error => error.message);
        console.log(String(wxml).slice(0, 1200));
      }
    }
  } finally {
    miniProgram.disconnect();
  }
}

main().catch(error => {
  console.error(error);
  process.exit(1);
});
