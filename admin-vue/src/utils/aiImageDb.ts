const DB_NAME = "xianzhi-ai-image-workspace";
const DB_VERSION = 1;
const STORE_NAME = "kv";
const SNAPSHOT_KEY = "online-image-snapshot-v2";
const DRAFT_KEY = "ai-image-draft";
const ORIGINAL_IMAGE_INDEX_KEY = "ai-original-image-index";
const ORIGINAL_IMAGE_KEY_PREFIX = "ai-original-image:";
const MAX_SNAPSHOT_TASKS = 30;
const MAX_SNAPSHOT_ASSETS = 30;
const MAX_REFERENCE_IMAGES = 10;
const MAX_REFERENCE_DATA_URL_LENGTH = 2_500_000;
const MAX_SNAPSHOT_DATA_URL_LENGTH = 180_000;
const MAX_ORIGINAL_IMAGE_CACHE = 8;

export type AiImageCacheRecord = Record<string, unknown>;

export interface AiImageSnapshot {
  data: AiImageCacheRecord;
  savedAt: number;
}

export interface CachedReferenceImage {
  id: string;
  name: string;
  url: string;
}

export interface AiImageDraft {
  form: AiImageCacheRecord;
  ui: {
    playgroundMode?: string;
    galleryFilter?: string;
    favoriteOnly?: boolean;
    promptSearch?: string;
    activeCollectionId?: string;
  };
  referenceImages: CachedReferenceImage[];
  savedAt: number;
}

export interface AiCachedOriginalImage {
  id: string;
  dataUrl: string;
  sourceUrl?: string;
  savedAt: number;
}

function canUseIndexedDB() {
  return typeof window !== "undefined" && "indexedDB" in window;
}

function currentCacheScope() {
  if (typeof window === "undefined") return "anonymous";
  const token = window.localStorage.getItem("token") || window.sessionStorage.getItem("token") || "";
  if (!token) return "anonymous";
  let hash = 0;
  for (let index = 0; index < token.length; index += 1) {
    hash = (hash * 31 + token.charCodeAt(index)) >>> 0;
  }
  return hash.toString(36);
}

function scopedKey(key: string) {
  return `${key}:${currentCacheScope()}`;
}

function openDb(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (!canUseIndexedDB()) {
      reject(new Error("IndexedDB is not available"));
      return;
    }
    const request = window.indexedDB.open(DB_NAME, DB_VERSION);
    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(STORE_NAME)) db.createObjectStore(STORE_NAME);
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error || new Error("Open IndexedDB failed"));
  });
}

async function readValue<T>(key: string): Promise<T | null> {
  if (!canUseIndexedDB()) return null;
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, "readonly");
    const store = tx.objectStore(STORE_NAME);
    const request = store.get(key);
    request.onsuccess = () => resolve((request.result as T | undefined) || null);
    request.onerror = () => reject(request.error || new Error("Read IndexedDB failed"));
    tx.oncomplete = () => db.close();
    tx.onerror = () => {
      db.close();
      reject(tx.error || new Error("IndexedDB transaction failed"));
    };
  });
}

async function writeValue<T>(key: string, value: T): Promise<void> {
  if (!canUseIndexedDB()) return;
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, "readwrite");
    const store = tx.objectStore(STORE_NAME);
    const request = store.put(value, key);
    request.onerror = () => reject(request.error || new Error("Write IndexedDB failed"));
    tx.oncomplete = () => {
      db.close();
      resolve();
    };
    tx.onerror = () => {
      db.close();
      reject(tx.error || new Error("IndexedDB transaction failed"));
    };
  });
}

async function deleteValue(key: string): Promise<void> {
  if (!canUseIndexedDB()) return;
  const db = await openDb();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, "readwrite");
    const store = tx.objectStore(STORE_NAME);
    const request = store.delete(key);
    request.onerror = () => reject(request.error || new Error("Delete IndexedDB value failed"));
    tx.oncomplete = () => {
      db.close();
      resolve();
    };
    tx.onerror = () => {
      db.close();
      reject(tx.error || new Error("IndexedDB transaction failed"));
    };
  });
}

function trimArray(value: unknown, limit: number) {
  return Array.isArray(value) ? value.slice(0, limit) : value;
}

function isVideoLikeRecord(value: unknown) {
  if (!value || typeof value !== "object") return false;
  const record = value as AiImageCacheRecord;
  const type = String(record.type || record.sourceType || "").toUpperCase();
  const mediaType = String(record.mediaType || "").toLowerCase();
  const model = String(record.model || "").toLowerCase();
  const params = record.params && typeof record.params === "object" ? record.params as AiImageCacheRecord : {};
  const inputMode = String(params.inputMode || record.mode || "").toLowerCase();
  const url = String(record.outputUrl || record.resultUrl || record.imageUrl || record.url || "");
  return type.includes("VIDEO")
    || mediaType === "video"
    || inputMode.includes("video")
    || model.includes("video")
    || /\.mp4(\?|$)/i.test(url);
}

function imageOnlyArray(value: unknown, limit: number) {
  return Array.isArray(value) ? value.filter((item) => !isVideoLikeRecord(item)).slice(0, limit) : value;
}

function compactSnapshotString(value: string) {
  if (/^data:(image|video)\//i.test(value) && value.length > MAX_SNAPSHOT_DATA_URL_LENGTH) return "";
  return value;
}

function compactSnapshotValue(value: unknown, depth = 0, arrayLimit = 0): unknown {
  if (typeof value === "string") return compactSnapshotString(value);
  if (Array.isArray(value)) {
    const items = arrayLimit > 0 ? value.slice(0, arrayLimit) : value;
    return items.map((item) => compactSnapshotValue(item, depth + 1));
  }
  if (!value || typeof value !== "object") return value;
  if (depth > 5) return undefined;
  const record = value as AiImageCacheRecord;
  return Object.entries(record).reduce<AiImageCacheRecord>((next, [key, item]) => {
    if (["referenceImages", "reference_images", "image_urls", "imageUrls", "inputImageUrls", "inputImagesSnapshot"].includes(key)) {
      const compacted = compactSnapshotValue(item, depth + 1, MAX_REFERENCE_IMAGES);
      if (Array.isArray(compacted) && compacted.length === 0) return next;
      next[key] = compacted;
      return next;
    }
    const compacted = compactSnapshotValue(item, depth + 1);
    if (compacted !== undefined) next[key] = compacted;
    return next;
  }, {});
}

export function normalizeAiImageSnapshotData(data: AiImageCacheRecord): AiImageCacheRecord {
  return compactSnapshotValue({
    ...data,
    recentTasks: imageOnlyArray(data.recentTasks, MAX_SNAPSHOT_TASKS),
    assets: imageOnlyArray(data.assets, MAX_SNAPSHOT_ASSETS),
    recentAssets: imageOnlyArray(data.recentAssets, MAX_SNAPSHOT_ASSETS)
  }) as AiImageCacheRecord;
}

export async function readAiImageSnapshot(): Promise<AiImageSnapshot | null> {
  const snapshot = await readValue<AiImageSnapshot>(scopedKey(SNAPSHOT_KEY));
  if (!snapshot?.data) return snapshot;
  return {
    ...snapshot,
    data: normalizeAiImageSnapshotData(snapshot.data)
  };
}

export async function writeAiImageSnapshot(data: AiImageCacheRecord): Promise<void> {
  const plainData = JSON.parse(JSON.stringify(normalizeAiImageSnapshotData(data))) as AiImageCacheRecord;
  await writeValue<AiImageSnapshot>(scopedKey(SNAPSHOT_KEY), {
    data: plainData,
    savedAt: Date.now()
  });
}

export async function readAiImageDraft(): Promise<AiImageDraft | null> {
  return readValue<AiImageDraft>(scopedKey(DRAFT_KEY));
}

export async function writeAiImageDraft(draft: AiImageDraft): Promise<void> {
  await writeValue<AiImageDraft>(scopedKey(DRAFT_KEY), {
    ...draft,
    referenceImages: draft.referenceImages
      .filter((item) => item.url.startsWith("data:") && item.url.length <= MAX_REFERENCE_DATA_URL_LENGTH)
      .slice(0, MAX_REFERENCE_IMAGES),
    savedAt: Date.now()
  });
}

export async function readCachedOriginalImage(id: string): Promise<AiCachedOriginalImage | null> {
  if (!id) return null;
  return readValue<AiCachedOriginalImage>(scopedKey(`${ORIGINAL_IMAGE_KEY_PREFIX}${id}`));
}

export async function writeCachedOriginalImage(image: Omit<AiCachedOriginalImage, "savedAt">): Promise<void> {
  if (!image.id || !image.dataUrl.startsWith("data:image/")) return;
  const savedImage: AiCachedOriginalImage = { ...image, savedAt: Date.now() };
  await writeValue<AiCachedOriginalImage>(scopedKey(`${ORIGINAL_IMAGE_KEY_PREFIX}${image.id}`), savedImage);
  const indexKey = scopedKey(ORIGINAL_IMAGE_INDEX_KEY);
  const currentIndex = (await readValue<string[]>(indexKey)) || [];
  const nextIndex = [image.id, ...currentIndex.filter((item) => item !== image.id)];
  await writeValue<string[]>(indexKey, nextIndex.slice(0, MAX_ORIGINAL_IMAGE_CACHE));
  for (const staleId of nextIndex.slice(MAX_ORIGINAL_IMAGE_CACHE)) {
    await deleteValue(scopedKey(`${ORIGINAL_IMAGE_KEY_PREFIX}${staleId}`));
  }
}

export async function clearCurrentAiImageCache(): Promise<void> {
  await Promise.all([
    deleteValue(scopedKey(SNAPSHOT_KEY)),
    deleteValue(scopedKey(DRAFT_KEY)),
    deleteValue(SNAPSHOT_KEY),
    deleteValue(DRAFT_KEY)
  ]);
}
