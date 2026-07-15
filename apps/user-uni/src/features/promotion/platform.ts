const fsTimeout = 12_000;

function withTimeout<T>(promise: Promise<T>, message: string): Promise<T> {
  return Promise.race([promise, new Promise<T>((_, reject) => setTimeout(() => reject(new Error(message)), fsTimeout))]);
}

export async function imageDataUrlToLocalPath(dataUrl: string, cacheKey: string) {
  if (!dataUrl.startsWith("data:image/")) return dataUrl;
  // #ifdef MP-WEIXIN
  const wxApi = (globalThis as unknown as { wx: { env: { USER_DATA_PATH: string }; getFileSystemManager: () => { writeFile: (options: Record<string, unknown>) => void } } }).wx;
  const base64 = dataUrl.slice(dataUrl.indexOf(",") + 1);
  const filePath = `${wxApi.env.USER_DATA_PATH}/promotion-code-${cacheKey || Date.now()}.png`;
  await withTimeout(new Promise<void>((resolve, reject) => {
    wxApi.getFileSystemManager().writeFile({ filePath, data: base64, encoding: "base64", success: () => resolve(), fail: reject });
  }), "小程序码写入超时");
  return filePath;
  // #endif
  // #ifndef MP-WEIXIN
  return dataUrl;
  // #endif
}

export function getImageInfo(src: string): Promise<UniApp.GetImageInfoSuccessData> {
  return withTimeout(new Promise((resolve, reject) => {
    uni.getImageInfo({ src, success: resolve, fail: reject });
  }), "图片加载超时");
}

export async function ensureAlbumPermission() {
  // #ifdef MP-WEIXIN
  const setting = await new Promise<UniApp.GetSettingSuccessResult>((resolve, reject) => uni.getSetting({ success: resolve, fail: reject }));
  if (setting.authSetting["scope.writePhotosAlbum"] === false) {
    const opened = await new Promise<{ authSetting: Record<string, boolean> }>((resolve, reject) => uni.openSetting({ success: resolve, fail: reject }));
    if (!opened.authSetting["scope.writePhotosAlbum"]) throw new Error("请在设置中允许保存到相册");
  }
  // #endif
}

export async function savePosterToAlbum(filePath: string) {
  await ensureAlbumPermission();
  return new Promise<void>((resolve, reject) => {
    uni.saveImageToPhotosAlbum({ filePath, success: () => resolve(), fail: error => reject(new Error(error.errMsg || "保存失败")) });
  });
}
