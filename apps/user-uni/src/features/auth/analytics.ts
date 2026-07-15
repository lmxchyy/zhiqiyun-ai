const allowedEvents = new Set([
  "login_page_view",
  "wechat_login_click",
  "phone_auth_success",
  "phone_auth_cancel",
  "sms_login_click",
  "sms_send_success",
  "sms_login_success",
  "password_login_click",
  "invite_entry_click",
  "invite_validate_success",
  "invite_validate_failed",
  "register_success",
  "login_success",
  "login_failed",
]);

export function trackLogin(event: string, properties: Record<string, string | number | boolean> = {}) {
  if (!allowedEvents.has(event)) return;
  // 当前项目暂无统一埋点 SDK，先保留无敏感信息的本地事件队列，接入 SDK 后可直接替换这里。
  const key = "zhiqiyun.auth.analytics-buffer.v1";
  const buffer = (uni.getStorageSync(key) as Array<Record<string, unknown>> | undefined) || [];
  buffer.push({ event, properties, at: Date.now() });
  uni.setStorageSync(key, buffer.slice(-30));
}
