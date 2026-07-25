import { apiRequestTask, getApiBaseURL } from "../../api/client";

export interface PublicInviteInfo {
  valid: boolean;
  inviteCode: string;
  agentDisplayName: string;
  agentStatus: string;
  activityIntro: string;
  registrationAllowed: boolean;
}

export interface InviteRegistrationResult {
  registered: boolean;
  idempotentReplay: boolean;
  registrationStatus: "created" | "existing";
  relationshipStatus: "locked";
  agentDisplayName: string;
  downloadPage: {
    platform: "android";
    channel: "official";
    downloadUrl: string;
    latestReleaseUrl: string;
  };
}

export interface AndroidRelease {
  id: string;
  platform: "android";
  channel: string;
  versionName: string;
  versionCode: number;
  fileSize: number;
  sha256: string;
  releaseNotes: string;
  minSupportedVersionCode: number;
  forceUpdate: boolean;
  status: "published";
  publishedAt: string;
}

export const inviteRegistrationAPI = {
  resolve(inviteCode: string) {
    return apiRequestTask<PublicInviteInfo>(
      `/api/v1/public/invites/${encodeURIComponent(inviteCode)}`,
      { auth: false },
    ).promise;
  },
  sendSMS(mobile: string) {
    return apiRequestTask<{ sent: boolean; retryAfterSeconds: number; expiresInSeconds: number }>(
      "/api/v1/auth/sms/send",
      { method: "POST", auth: false, data: { mobile, purpose: "agent_invite_register" } },
    ).promise;
  },
  register(input: {
    inviteCode: string;
    mobile: string;
    smsCode: string;
    agreementAccepted: boolean;
    privacyAccepted: boolean;
    idempotencyKey: string;
  }) {
    return apiRequestTask<InviteRegistrationResult>(
      `/api/v1/public/invites/${encodeURIComponent(input.inviteCode)}/register`,
      {
        method: "POST",
        auth: false,
        headers: { "Idempotency-Key": input.idempotencyKey },
        data: {
          mobile: input.mobile,
          sms_code: input.smsCode,
          agreement_accepted: input.agreementAccepted,
          privacy_accepted: input.privacyAccepted,
        },
      },
    ).promise;
  },
  latestAndroid() {
    return apiRequestTask<AndroidRelease>(
      "/api/v1/public/app/releases/latest?platform=android&channel=official",
      { auth: false },
    ).promise;
  },
  absoluteURL(path: string) {
    if (/^https?:\/\//i.test(path)) return path;
    return `${getApiBaseURL().replace(/\/+$/, "")}${path.startsWith("/") ? path : `/${path}`}`;
  },
};
