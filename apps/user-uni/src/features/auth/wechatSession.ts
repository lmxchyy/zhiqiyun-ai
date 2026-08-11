import { loginAPI } from "./api";

export interface WechatMiniProgramSessionCallbacks {
  success: () => void;
  fail: (error: unknown) => void;
}

/** Refresh device WeChat openid onto the auth session before content-security-gated APIs. */
export function ensureWechatMiniProgramSession(callbacks: WechatMiniProgramSessionCallbacks): void {
  // #ifdef MP-WEIXIN
  try {
    uni.login({
      provider: "weixin",
      success: (result) => {
        const code = String(result.code || "").trim();
        if (!code) {
          callbacks.fail(new Error("微信授权未返回 code"));
          return;
        }
        try {
          void loginAPI.refreshWechatSession(code).then(
            callbacks.success,
            callbacks.fail,
          );
        } catch (error) {
          callbacks.fail(error);
        }
      },
      fail: (error) => {
        callbacks.fail(new Error(error?.errMsg || "微信授权失败"));
      },
    });
  } catch (error) {
    callbacks.fail(error);
  }
  return;
  // #endif
  // #ifndef MP-WEIXIN
  callbacks.success();
  // #endif
}
