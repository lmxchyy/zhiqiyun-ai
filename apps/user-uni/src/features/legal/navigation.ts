export type LegalDocumentCode =
  | "user-agreement"
  | "privacy-policy"
  | "ai-content-rules"
  | "member-service-agreement"
  | "agent-service-agreement"
  | "enterprise-space-service-agreement"
  | "recharge-service-agreement";

export function openLegalDocument(code: LegalDocumentCode) {
  uni.navigateTo({
    url: `/pages/user/ComplianceCenterPage?document=${encodeURIComponent(code)}&view=1`,
    fail: () => uni.showToast({ title: "协议页面打开失败，请稍后重试", icon: "none" }),
  });
}
