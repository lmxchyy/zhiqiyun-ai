import type { InviteSource, LoginRedirectInfo, LoginSourceParams } from "./types";

const sourceStorageKey = "zhiqiyun.auth.first-source.v1";
const inviteTokenPattern = /^inv_[aou][0-9a-f]{16}$/i;

function safeDecode(value: unknown): string {
  try {
    return decodeURIComponent(String(value || "").replace(/\+/g, "%20"));
  } catch {
    return String(value || "");
  }
}

function normalizeInviteToken(value: unknown): string {
  const token = String(value || "").trim().toLowerCase();
  return inviteTokenPattern.test(token) ? token : "";
}

function sceneParams(sceneValue: unknown): Record<string, string> {
  const scene = safeDecode(sceneValue);
  const result: Record<string, string> = {};
  const token = normalizeInviteToken(scene);
  if (token) return { inviteToken: token };
  const compact = /^C([A-Z0-9]+)T\d{2}(?:A([A-Z0-9]+))?$/i.exec(scene);
  if (compact) return { invite_code: compact[1], campaign_code: compact[2] || "" };
  scene.split("&").forEach(part => {
    const [key, ...rest] = part.split("=");
    if (key) result[key] = safeDecode(rest.join("="));
  });
  if (!result.inviteToken) {
    const nested = normalizeInviteToken(result.inviteToken || result.token || result.scene);
    if (nested) result.inviteToken = nested;
  }
  return result;
}

function first(query: Record<string, unknown>, scene: Record<string, string>, keys: string[]) {
  for (const key of keys) {
    const value = String(query[key] || scene[key] || "").trim();
    if (value) return value;
  }
  return "";
}

export function parseLoginSource(query: Record<string, unknown> = {}): LoginSourceParams {
  const scene = sceneParams(query.scene);
  const inviteToken = normalizeInviteToken(query.inviteToken || query.token || scene.inviteToken || query.scene);
  const inviteCode = first(query, scene, ["invite_code", "inviteCode", "invite", "agent_code", "agentCode", "c"]).toUpperCase();
  let inviteSource: InviteSource = "none";
  if (inviteToken || inviteCode) {
    if (query.scene || inviteToken) inviteSource = "scene";
    else if (query.agent_code || query.agentCode) inviteSource = "agent_code";
    else if (query.promoter_code || query.campaign_code) inviteSource = "promotion";
    else inviteSource = "query";
  }
  const value: LoginSourceParams = {
    inviteCode,
    inviteToken,
    inviteSource,
    sceneCode: String(query.scene || ""),
    promoterCode: first(query, scene, ["promoter_code", "promoterCode"]),
    campaignCode: first(query, scene, ["campaign_code", "campaignCode"]),
    channel: first(query, scene, ["channel", "source"]),
    sourcePage: first(query, scene, ["sourcePage", "source_page"]),
  };
  const cached = uni.getStorageSync(sourceStorageKey) as LoginSourceParams | undefined;
  if (!cached || ((!cached.inviteCode && value.inviteCode) || (!cached.inviteToken && value.inviteToken))) {
    uni.setStorageSync(sourceStorageKey, value);
  }
  return value;
}

export function parseRedirectInfo(query: Record<string, unknown> = {}): LoginRedirectInfo {
  const path = safeDecode(query.redirectPath || query.redirect || "");
  const rawQuery = safeDecode(query.redirectQuery || "");
  const values: Record<string, string> = {};
  rawQuery.split("&").forEach(part => {
    const [key, ...rest] = part.split("=");
    if (key) values[safeDecode(key)] = safeDecode(rest.join("="));
  });
  return {
    path,
    query: values,
    action: String(query.redirectAction || ""),
    sourcePage: String(query.sourcePage || ""),
  };
}
