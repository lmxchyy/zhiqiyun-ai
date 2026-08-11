import type { AssetStatus, AssetType } from "./types";

export interface AssetNativeBridgeHandlers {
  setType?: (value: AssetType) => void;
  setStatus?: (value: AssetStatus) => void;
  emptyAction?: () => void;
  toggleSearch?: () => void;
  updateSearch?: (value: string) => void;
  submitSearch?: () => void;
  clearSearch?: () => void;
  openFilter?: () => void;
  openSort?: () => void;
  openManage?: () => void;
  openAllAssets?: () => void;
  openAllTasks?: () => void;
  openTask?: (id: string) => void;
  cancelTask?: (id: string) => void;
  retryTask?: (id: string) => void;
  deleteTask?: (id: string) => void;
  openTaskResult?: (id: string) => void;
  openAsset?: (id: string) => void;
  favoriteAsset?: (id: string) => void;
  openAssetActions?: (id: string) => void;
  previewCurrentAsset?: () => void;
  downloadCurrentAsset?: () => void;
  editCurrentAsset?: () => void;
  regenerateCurrentAsset?: () => void;
  copyCurrentPrompt?: () => void;
}

interface AssetNativeBridgeGlobal {
  __xianzhiAssetNativeBridge?: AssetNativeBridgeHandlers;
}

export function registerAssetNativeBridge(handlers: AssetNativeBridgeHandlers) {
  const runtime = globalThis as unknown as AssetNativeBridgeGlobal;
  const previous = runtime.__xianzhiAssetNativeBridge;
  runtime.__xianzhiAssetNativeBridge = handlers;

  return () => {
    if (runtime.__xianzhiAssetNativeBridge !== handlers) return;
    runtime.__xianzhiAssetNativeBridge = previous;
  };
}
