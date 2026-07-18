const ACCESS_TOKEN_KEY = "token";
const SESSION_MARKER_KEY = "zhiqiyun.web.has-session";
const AUTH_CHANGED_EVENT = "zhiqiyun:web-auth-changed";

export type WebAuthStatus = "guest" | "authenticated" | "expired";

export interface WebAuthPayload<TUser = Record<string, unknown>> {
  accessToken?: string;
  user: TUser;
  permissions?: string[];
  workspace?: string;
  defaultModule?: string;
  defaultRoute?: string;
}

function emitAuthChanged(status: WebAuthStatus) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent(AUTH_CHANGED_EVENT, { detail: { status } }));
}

export function getWebAccessToken() {
  if (typeof window === "undefined") return "";
  return window.localStorage.getItem(ACCESS_TOKEN_KEY) || window.sessionStorage.getItem(ACCESS_TOKEN_KEY) || "";
}

export function hasWebSessionMarker() {
  return typeof window !== "undefined" && (window.localStorage.getItem(SESSION_MARKER_KEY) === "1" || window.sessionStorage.getItem(SESSION_MARKER_KEY) === "1");
}

export function hasPersistentWebSessionMarker() {
  return typeof window !== "undefined" && window.localStorage.getItem(SESSION_MARKER_KEY) === "1";
}

export function isPersistentWebSession() {
  return typeof window !== "undefined" && Boolean(window.localStorage.getItem(ACCESS_TOKEN_KEY));
}

export function persistWebAccessToken(token: string, remember = true) {
  if (typeof window === "undefined") return;
  const normalized = token.trim();
  if (!normalized) {
    clearWebAuthSession("guest");
    return;
  }
  if (remember) {
    window.localStorage.setItem(ACCESS_TOKEN_KEY, normalized);
    window.localStorage.setItem(SESSION_MARKER_KEY, "1");
    window.sessionStorage.removeItem(ACCESS_TOKEN_KEY);
    window.sessionStorage.removeItem(SESSION_MARKER_KEY);
  } else {
    window.sessionStorage.setItem(ACCESS_TOKEN_KEY, normalized);
    window.sessionStorage.setItem(SESSION_MARKER_KEY, "1");
    window.localStorage.removeItem(ACCESS_TOKEN_KEY);
    window.localStorage.removeItem(SESSION_MARKER_KEY);
  }
  // This marker is intentionally non-sensitive. The long-lived refresh token
  // remains in the backend-issued HttpOnly cookie.
  emitAuthChanged("authenticated");
}

export function clearWebAuthSession(status: WebAuthStatus = "guest") {
  if (typeof window === "undefined") return;
  window.localStorage.removeItem(ACCESS_TOKEN_KEY);
  window.sessionStorage.removeItem(ACCESS_TOKEN_KEY);
  window.localStorage.removeItem(SESSION_MARKER_KEY);
  window.sessionStorage.removeItem(SESSION_MARKER_KEY);
  emitAuthChanged(status);
}

export function onWebAuthChanged(listener: (status: WebAuthStatus) => void) {
  if (typeof window === "undefined") return () => undefined;
  const handleCustom = (event: Event) => {
    const status = (event as CustomEvent<{ status?: WebAuthStatus }>).detail?.status;
    if (status) listener(status);
  };
  const handleStorage = (event: StorageEvent) => {
    if (event.key !== ACCESS_TOKEN_KEY && event.key !== SESSION_MARKER_KEY) return;
    listener(getWebAccessToken() ? "authenticated" : "guest");
  };
  window.addEventListener(AUTH_CHANGED_EVENT, handleCustom);
  window.addEventListener("storage", handleStorage);
  return () => {
    window.removeEventListener(AUTH_CHANGED_EVENT, handleCustom);
    window.removeEventListener("storage", handleStorage);
  };
}
