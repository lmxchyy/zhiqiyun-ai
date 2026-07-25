import { apiRequestTask, getAuthToken } from "../../api/client";

function installationID() {
  const key = "zhiqiyunAppInstallationId";
  const existing = String(uni.getStorageSync(key) || "");
  if (existing) return existing;
  const cryptoRuntime = globalThis.crypto;
  const created = cryptoRuntime?.randomUUID
    ? cryptoRuntime.randomUUID()
    : `install_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 14)}`;
  uni.setStorageSync(key, created);
  return created;
}

export async function recordAgentInviteAppActivation() {
  if (!getAuthToken()) return false;
  try {
    await apiRequestTask<{ activated: boolean }>(
      "/api/v1/app/activation",
      { method: "POST", data: { installationId: installationID() }, auth: true },
    ).promise;
    return true;
  } catch {
    return false;
  }
}
