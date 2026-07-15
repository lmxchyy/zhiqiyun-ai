import { getApiBaseURL, getAuthToken } from "../../api/client";
import type { AssetItem } from "./types";

function downloadUrl(asset: AssetItem) {
  return `${getApiBaseURL()}/api/v1/assets/${encodeURIComponent(asset.id)}/download`;
}

function openAlbumPermissionGuide() {
  uni.showModal({
    title: "需要相册权限",
    content: "请在小程序设置中允许保存图片和视频到相册，然后重新保存。",
    confirmText: "去设置",
    success: result => {
      if (result.confirm) uni.openSetting({});
    },
  });
}

function ensureAlbumPermission(): Promise<void> {
  return new Promise((resolve, reject) => {
    if (typeof uni.getSetting !== "function") {
      resolve();
      return;
    }
    uni.getSetting({
      success: result => {
        if (result.authSetting?.["scope.writePhotosAlbum"] === false) {
          openAlbumPermissionGuide();
          reject(new Error("未获得相册权限"));
          return;
        }
        resolve();
      },
      fail: () => resolve(),
    });
  });
}

function handleAlbumSaveFailure(message: string, previewPath = "") {
  if (/auth|authorize|permission|deny/i.test(message)) {
    openAlbumPermissionGuide();
  } else if (previewPath) {
    uni.previewImage({ urls: [previewPath], current: previewPath });
  }
  return new Error(message.includes("cancel") ? "已取消保存" : "未保存到相册，请检查相册权限");
}

export function previewAsset(asset: AssetItem) {
  const url = asset.remoteUrl || asset.thumbnailUrl || asset.fallbackUrl;
  if (!url) {
    uni.showToast({ title: "当前资产暂无可预览内容", icon: "none" });
    return;
  }
  if (asset.type === "image" || asset.type === "infographic") {
    uni.previewImage({ urls: [url], current: url });
    return;
  }
  if (asset.type === "video") {
    uni.navigateTo({ url: `/pages/user/UserAssetDetailPage?id=${encodeURIComponent(asset.id)}&autoplay=1` });
    return;
  }
  uni.navigateTo({ url: `/pages/user/UserAssetDetailPage?id=${encodeURIComponent(asset.id)}` });
}

export function downloadAssetFile(asset: AssetItem): Promise<string> {
  return new Promise((resolve, reject) => {
    uni.showLoading({ title: "正在下载" });
    uni.downloadFile({
      url: downloadUrl(asset),
      header: { Authorization: `Bearer ${getAuthToken()}` },
      success: async result => {
        if (result.statusCode < 200 || result.statusCode >= 300) {
          reject(new Error("下载失败，请稍后重试"));
          return;
        }
        const filePath = result.tempFilePath;
        if (asset.type === "image" || asset.type === "infographic") {
          try {
            await ensureAlbumPermission();
          } catch (error) {
            reject(error instanceof Error ? error : new Error("未获得相册权限"));
            return;
          }
          uni.saveImageToPhotosAlbum({
            filePath,
            success: () => {
              uni.showToast({ title: "已保存到相册", icon: "success" });
              resolve(filePath);
            },
            fail: reason => {
              const message = reason.errMsg || "保存到相册失败";
              reject(handleAlbumSaveFailure(message, filePath));
            },
          });
          return;
        } else if (asset.type === "video") {
          try {
            await ensureAlbumPermission();
          } catch (error) {
            reject(error instanceof Error ? error : new Error("未获得相册权限"));
            return;
          }
          uni.saveVideoToPhotosAlbum({
            filePath,
            success: () => {
              uni.showToast({ title: "已保存到相册", icon: "success" });
              resolve(filePath);
            },
            fail: reason => reject(handleAlbumSaveFailure(reason.errMsg || "视频保存失败")),
          });
          return;
        } else {
          uni.openDocument({
            filePath,
            showMenu: true,
            success: () => resolve(filePath),
            fail: reason => reject(new Error(reason.errMsg || "文件打开失败")),
          });
          return;
        }
      },
      fail: reason => reject(new Error(reason.errMsg || "下载失败，请检查网络")),
      complete: () => uni.hideLoading(),
    });
  });
}

export function shareAsset(asset: AssetItem) {
  const url = asset.remoteUrl || downloadUrl(asset);
  uni.setClipboardData({
    data: url,
    success: () => uni.showToast({ title: "分享链接已复制", icon: "success" }),
  });
}

export function copyText(value: string, successTitle = "已复制") {
  if (!value) return;
  uni.setClipboardData({ data: value, success: () => uni.showToast({ title: successTitle, icon: "success" }) });
}
