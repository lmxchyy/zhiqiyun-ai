import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const automator = require("miniprogram-automator");

const automationEndpoint = process.env.WECHAT_AUTOMATION_ENDPOINT || "ws://127.0.0.1:16167";
const apiBase = process.env.XIANZHI_API_BASE || "http://127.0.0.1:3100";
const productCode = process.env.WECHAT_VIRTUAL_PRODUCT_CODE || "TOKEN_CUSTOM_1YUAN";
const expectedEmail = process.env.XIANZHI_EXPECTED_EMAIL || "demo@xianzhi.ai";

function log(stage, fields = {}) {
  console.log(JSON.stringify({ at: new Date().toISOString(), stage, ...fields }));
}

async function request(path, options = {}) {
  const response = await fetch(`${apiBase}${path}`, options);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message = body?.message || body?.error || `HTTP ${response.status}`;
    throw new Error(`${path}: ${message}`);
  }
  return body;
}

async function main() {
  const miniProgram = await automator.connect({ wsEndpoint: automationEndpoint });
  try {
    const loginResult = await miniProgram.callWxMethod("login");
    if (!loginResult?.code) throw new Error("wx.login did not return a code");

    const auth = await request("/api/v1/auth/wechat-mini-program/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code: loginResult.code }),
    });
    if (auth?.user?.email !== expectedEmail) {
      throw new Error(`unexpected WeChat user: ${auth?.user?.email || "missing"}`);
    }
    if (!auth?.accessToken) throw new Error("WeChat login response did not include an access token");
    log("wechat_login_verified", { userId: auth.user.id, email: auth.user.email });

    const order = await request("/api/v1/payment/wechat-virtual/orders", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${auth.accessToken}`,
        "Content-Type": "application/json",
        "X-Tenant-Id": "",
      },
      body: JSON.stringify({ productCode, quantity: 1, couponCode: "" }),
    });
    log("order_created", {
      orderNo: order.orderNo,
      amountCent: order.amountCent,
      productCode,
      mode: order.mode,
    });

    log("payment_dialog_opening", { orderNo: order.orderNo, amountCent: order.amountCent });
    const paymentResult = await miniProgram.callWxMethod("requestVirtualPayment", {
      signData: order.signData,
      paySig: order.paySig,
      signature: order.signature,
      mode: order.mode,
    });
    log("payment_method_returned", {
      orderNo: order.orderNo,
      result: paymentResult?.errMsg || paymentResult?.message || "returned",
    });
  } finally {
    try {
      await miniProgram.disconnect();
    } catch {
      // The developer tools may close the automation socket while the native
      // payment sheet is taking over. That must not hide the payment result.
    }
  }
}

main().catch((error) => {
  log("failed", { error: error instanceof Error ? error.message : String(error) });
  process.exitCode = 1;
});
