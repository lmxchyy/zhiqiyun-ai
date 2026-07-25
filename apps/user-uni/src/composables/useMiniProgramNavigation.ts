import { computed, readonly, reactive } from "vue";

interface SafeAreaInsets {
  top?: number;
  right?: number;
  bottom?: number;
  left?: number;
}

interface WindowInfo {
  statusBarHeight?: number;
  windowWidth?: number;
  windowHeight?: number;
  screenWidth?: number;
  screenHeight?: number;
  safeAreaInsets?: SafeAreaInsets;
  safeArea?: { top?: number; right?: number; bottom?: number; left?: number };
}

interface CapsuleRect {
  top: number;
  bottom: number;
  left: number;
  right: number;
  width: number;
  height: number;
}

interface NavigationRuntime {
  getWindowInfo?: () => WindowInfo;
  getSystemInfoSync?: () => WindowInfo;
  getMenuButtonBoundingClientRect?: () => CapsuleRect;
}

declare const wx: {
  getMenuButtonBoundingClientRect?: () => CapsuleRect;
};

const DEFAULT_STATUS_BAR_HEIGHT = 20;
const DEFAULT_NAVIGATION_BAR_HEIGHT = 44;
const DEFAULT_CAPSULE_HEIGHT = 32;
const DEFAULT_CAPSULE_GAP = 6;
const DEFAULT_CAPSULE_RIGHT_SPACE = 96;
const DEFAULT_WINDOW_WIDTH = 375;
const DEFAULT_WINDOW_HEIGHT = 667;

let isWeixinMiniProgram = false;
let isNativeApp = false;
// #ifdef MP-WEIXIN
isWeixinMiniProgram = true;
// #endif
// #ifdef APP-PLUS
isNativeApp = true;
// #endif

const state = reactive({
  statusBarHeight: isWeixinMiniProgram || isNativeApp ? DEFAULT_STATUS_BAR_HEIGHT : 0,
  navigationBarHeight: DEFAULT_NAVIGATION_BAR_HEIGHT,
  headerHeight: (isWeixinMiniProgram || isNativeApp ? DEFAULT_STATUS_BAR_HEIGHT : 0) + DEFAULT_NAVIGATION_BAR_HEIGHT,
  headerPaddingTop: isWeixinMiniProgram || isNativeApp ? DEFAULT_STATUS_BAR_HEIGHT : 0,
  capsuleRightSpace: isWeixinMiniProgram ? DEFAULT_CAPSULE_RIGHT_SPACE : 0,
  capsuleTop: (isWeixinMiniProgram || isNativeApp ? DEFAULT_STATUS_BAR_HEIGHT : 0) + DEFAULT_CAPSULE_GAP,
  capsuleBottom: (isWeixinMiniProgram || isNativeApp ? DEFAULT_STATUS_BAR_HEIGHT : 0) + DEFAULT_CAPSULE_GAP + DEFAULT_CAPSULE_HEIGHT,
  capsuleHeight: DEFAULT_CAPSULE_HEIGHT,
  capsuleRight: DEFAULT_WINDOW_WIDTH - 10,
  windowWidth: DEFAULT_WINDOW_WIDTH,
  windowHeight: DEFAULT_WINDOW_HEIGHT,
  safeAreaInsets: { top: 0, right: 0, bottom: 0, left: 0 } as Required<SafeAreaInsets>,
});

function positive(value: unknown, fallback = 0) {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : fallback;
}

function nonNegative(value: unknown, fallback = 0) {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : fallback;
}

function readWindowInfo(): WindowInfo {
  const runtime = uni as unknown as NavigationRuntime;
  try {
    const windowInfo = runtime.getWindowInfo?.();
    if (windowInfo) return windowInfo;
  } catch {
    // Older uni-app runtimes fall through to the compatibility API.
  }
  try {
    return runtime.getSystemInfoSync?.() || {};
  } catch {
    return {};
  }
}

function readCapsuleRect(): CapsuleRect | null {
  if (!isWeixinMiniProgram) return null;
  const valid = (rect: CapsuleRect | null | undefined): rect is CapsuleRect => Boolean(
    rect
    && positive(rect.height) > 0
    && positive(rect.top) > 0
    && positive(rect.right) > positive(rect.left),
  );
  try {
    // #ifdef MP-WEIXIN
    const rect = wx.getMenuButtonBoundingClientRect?.();
    if (valid(rect)) return rect;
    // #endif
  } catch {
    // Fall through to the uni compatibility bridge.
  }
  try {
    const rect = (uni as unknown as NavigationRuntime).getMenuButtonBoundingClientRect?.();
    return valid(rect) ? rect : null;
  } catch {
    return null;
  }
}

function normalizeSafeAreaInsets(info: WindowInfo): Required<SafeAreaInsets> {
  const safeAreaTop = nonNegative(info.safeAreaInsets?.top, nonNegative(info.safeArea?.top));
  const windowWidth = positive(info.windowWidth, DEFAULT_WINDOW_WIDTH);
  const windowHeight = positive(info.windowHeight, DEFAULT_WINDOW_HEIGHT);
  return {
    top: safeAreaTop,
    right: nonNegative(info.safeAreaInsets?.right, Math.max(0, windowWidth - nonNegative(info.safeArea?.right, windowWidth))),
    bottom: nonNegative(info.safeAreaInsets?.bottom, Math.max(0, windowHeight - nonNegative(info.safeArea?.bottom, windowHeight))),
    left: nonNegative(info.safeAreaInsets?.left, nonNegative(info.safeArea?.left)),
  };
}

export function syncMiniProgramNavigation() {
  const info = readWindowInfo();
  const safeAreaInsets = normalizeSafeAreaInsets(info);
  const windowWidth = positive(info.windowWidth, positive(info.screenWidth, DEFAULT_WINDOW_WIDTH));
  const windowHeight = positive(info.windowHeight, positive(info.screenHeight, DEFAULT_WINDOW_HEIGHT));
  const platformStatusFallback = isWeixinMiniProgram || isNativeApp ? DEFAULT_STATUS_BAR_HEIGHT : 0;
  const statusBarHeight = positive(info.statusBarHeight, positive(safeAreaInsets.top, platformStatusFallback));
  const capsule = readCapsuleRect();
  const capsuleHeight = positive(capsule?.height, DEFAULT_CAPSULE_HEIGHT);
  const capsuleTop = positive(capsule?.top, statusBarHeight + DEFAULT_CAPSULE_GAP);
  const capsuleBottom = positive(capsule?.bottom, capsuleTop + capsuleHeight);
  const capsuleGap = Math.max(0, capsuleTop - statusBarHeight);
  const navigationBarHeight = capsule
    ? Math.max(DEFAULT_NAVIGATION_BAR_HEIGHT, capsuleGap * 2 + capsuleHeight)
    : DEFAULT_NAVIGATION_BAR_HEIGHT;
  const capsuleRight = positive(capsule?.right, windowWidth - 10);
  const capsuleRightSpace = isWeixinMiniProgram
    ? Math.max(DEFAULT_CAPSULE_RIGHT_SPACE, capsule ? windowWidth - capsule.left + 8 : DEFAULT_CAPSULE_RIGHT_SPACE)
    : 0;

  state.statusBarHeight = statusBarHeight;
  state.navigationBarHeight = navigationBarHeight;
  state.headerHeight = statusBarHeight + navigationBarHeight;
  state.headerPaddingTop = statusBarHeight;
  state.capsuleRightSpace = capsuleRightSpace;
  state.capsuleTop = capsuleTop;
  state.capsuleBottom = capsuleBottom;
  state.capsuleHeight = capsuleHeight;
  state.capsuleRight = capsuleRight;
  state.windowWidth = windowWidth;
  state.windowHeight = windowHeight;
  state.safeAreaInsets = safeAreaInsets;
}

function cssSafeTop() {
  const pixels = `${state.headerPaddingTop}px`;
  return isWeixinMiniProgram ? pixels : `max(${pixels}, env(safe-area-inset-top, 0px))`;
}

export const miniProgramNavigationStyle = computed<Record<string, string>>(() => {
  const headerPaddingTop = cssSafeTop();
  return {
    "--status-bar-height": headerPaddingTop,
    "--navigation-bar-height": `${state.navigationBarHeight}px`,
    "--header-height": isWeixinMiniProgram
      ? `${state.headerHeight}px`
      : `calc(${state.navigationBarHeight}px + ${headerPaddingTop})`,
    "--header-padding-top": headerPaddingTop,
    "--capsule-right-space": `${state.capsuleRightSpace}px`,
    "--window-height": `${state.windowHeight}px`,
  };
});

export function useMiniProgramNavigation() {
  syncMiniProgramNavigation();
  return {
    metrics: readonly(state),
    statusBarHeight: computed(() => state.statusBarHeight),
    navigationBarHeight: computed(() => state.navigationBarHeight),
    headerHeight: computed(() => state.headerHeight),
    headerPaddingTop: computed(() => state.headerPaddingTop),
    capsuleRightSpace: computed(() => state.capsuleRightSpace),
    navigationStyle: miniProgramNavigationStyle,
    sync: syncMiniProgramNavigation,
  };
}
