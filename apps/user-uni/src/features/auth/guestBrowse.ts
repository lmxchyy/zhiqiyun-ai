const GUEST_BROWSE_KEY = "xianzhi.guestBrowseAccepted.v1";
const LOGIN_PROMPT_COOLDOWN_MS = 2500;
const HOME_PAGE = "/pages/user/UserHomePage";

let loginPromptSuppressedUntil = 0;

function storageGet(key: string): string {
  try {
    return String(uni.getStorageSync(key) || "");
  } catch {
    return "";
  }
}

function storageSet(key: string, value: string) {
  try {
    uni.setStorageSync(key, value);
  } catch {
    // Guest browsing must remain available even when local storage is unavailable.
  }
}

function storageRemove(key: string) {
  try {
    uni.removeStorageSync(key);
  } catch {
    // Ignore unavailable storage; the in-memory prompt state is still reset below.
  }
}

export function hasAcceptedGuestBrowse(): boolean {
  return storageGet(GUEST_BROWSE_KEY) === "1";
}

export function acceptGuestBrowse(): void {
  storageSet(GUEST_BROWSE_KEY, "1");
}

export function clearGuestBrowse(): void {
  storageRemove(GUEST_BROWSE_KEY);
  loginPromptSuppressedUntil = 0;
}

export function suppressLoginPrompt(ms = LOGIN_PROMPT_COOLDOWN_MS): void {
  loginPromptSuppressedUntil = Date.now() + Math.max(0, ms);
}

export function isLoginPromptSuppressed(): boolean {
  return Date.now() < loginPromptSuppressedUntil;
}

export function enterGuestBrowseHome(): void {
  acceptGuestBrowse();
  suppressLoginPrompt();
  uni.switchTab({
    url: HOME_PAGE,
    fail: () => uni.reLaunch({ url: HOME_PAGE }),
  });
}
