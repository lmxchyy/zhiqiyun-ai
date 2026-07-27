import { loginAPI } from "./api";

function requestWechatMiniProgramCode() {
  return new Promise<string>((resolve, reject) => {
    // #ifdef MP-WEIXIN
    uni.login({
      provider: "weixin",
      success: (result) => {
        const code = String(result.code || "").trim();
        if (!code) {
          reject(new Error("微信授权未返回 code"));
          return;
        }
        resolve(code);
      },
      fail: (error) => {
        reject(new Error(error?.errMsg || "微信授权失败"));
      },
    });
    // #endif
    // #ifndef MP-WEIXIN
    reject(new Error("当前环境不支持微信授权"));
    // #endif
  });
}

/** Refresh device WeChat openid onto the auth session before content-security-gated APIs. */
export async function ensureWechatMiniProgramSession() {
  // #ifdef MP-WEIXIN
  const wxLoginCode = await requestWechatMiniProgramCode();
  return loginAPI.refreshWechatSession(wxLoginCode);
  // #endif
  // #ifndef MP-WEIXIN
  return { sessionReady: true, linked: false, userId: "", boundToOther: false };
  // #endif
}
