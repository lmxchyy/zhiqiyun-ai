const DB_NAME = "xianzhi-ai-image-workspace";
const DB_VERSION = 1;
const STORE_NAME = "kv";
const SNAPSHOT_KEY = "online-image-snapshot";
const DRAFT_KEY = "ai-image-draft";
const ORIGINAL_IMAGE_INDEX_KEY = "ai-original-image-index";
const ORIGINAL_IMAGE_KEY_PREFIX = "ai-original-image:";
const MAX_SNAPSHOT_TASKS = 30;
const MAX_SNAPSHOT_ASSETS = 30;
const MAX_REFERENCE_IMAGES = 10;
const MAX_REFERENCE_DATA_URL_LENGTH = 2_500_000;
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

export function normalizeAiImageSnapshotData(data: AiImageCacheRecord): AiImageCacheRecord {
  return {
    ...data,
    recentTasks: trimArray(data.recentTasks, MAX_SNAPSHOT_TASKS),
    assets: trimArray(data.assets, MAX_SNAPSHOT_ASSETS),
    recentAssets: trimArray(data.recentAssets, MAX_SNAPSHOT_ASSETS)
  };
}

export async function readAiImageSnapshot(): Promise<AiImageSnapshot | null> {
  return readValue<AiImageSnapshot>(SNAPSHOT_KEY);
}

export async function writeAiImageSnapshot(data: AiImageCacheRecord): Promise<void> {
  const plainData = JSON.parse(JSON.stringify(data)) as AiImageCacheRecord;
  await writeValue<AiImageSnapshot>(SNAPSHOT_KEY, {
    data: normalizeAiImageSnapshotData(plainData),
    savedAt: Date.now()
  });
}

export async function readAiImageDraft(): Promise<AiImageDraft | null> {
  return readValue<AiImageDraft>(DRAFT_KEY);
}

export async function writeAiImageDraft(draft: AiImageDraft): Promise<void> {
  await writeValue<AiImageDraft>(DRAFT_KEY, {
    ...draft,
    referenceImages: draft.referenceImages
      .filter((item) => item.url.startsWith("data:") && item.url.length <= MAX_REFERENCE_DATA_URL_LENGTH)
      .slice(0, MAX_REFERENCE_IMAGES),
    savedAt: Date.now()
  });
}

export async function readCachedOriginalImage(id: string): Promise<AiCachedOriginalImage | null> {
  if (!id) return null;
  return readValue<AiCachedOriginalImage>(`${ORIGINAL_IMAGE_KEY_PREFIX}${id}`);
}

export async function writeCachedOriginalImage(image: Omit<AiCachedOriginalImage, "savedAt">): Promise<void> {
  if (!image.id || !image.dataUrl.startsWith("data:image/")) return;
  const savedImage: AiCachedOriginalImage = { ...image, savedAt: Date.now() };
  await writeValue<AiCachedOriginalImage>(`${ORIGINAL_IMAGE_KEY_PREFIX}${image.id}`, savedImage);
  const currentIndex = (await readValue<string[]>(ORIGINAL_IMAGE_INDEX_KEY)) || [];
  const nextIndex = [image.id, ...currentIndex.filter((item) => item !== image.id)];
  await writeValue<string[]>(ORIGINAL_IMAGE_INDEX_KEY, nextIndex.slice(0, MAX_ORIGINAL_IMAGE_CACHE));
  for (const staleId of nextIndex.slice(MAX_ORIGINAL_IMAGE_CACHE)) {
    await deleteValue(`${ORIGINAL_IMAGE_KEY_PREFIX}${staleId}`);
  }
}
