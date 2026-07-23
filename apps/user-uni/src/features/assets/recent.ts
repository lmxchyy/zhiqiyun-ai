import type { AssetItem } from "./types";

export function stableAsset(previous: AssetItem | undefined, next: AssetItem): AssetItem {
  if (!previous || previous.updatedAt !== next.updatedAt || next.thumbnailUrl.startsWith("data:image/")) return next;
  return {
    ...next,
    remoteUrl: previous.remoteUrl || next.remoteUrl,
    thumbnailUrl: previous.thumbnailUrl || next.thumbnailUrl,
  };
}

export function mergeRecentWorks(previous: AssetItem[], incoming: AssetItem[], limit = 20): AssetItem[] {
  const previousByID = new Map(previous.map(item => [item.id, item]));
  const result: AssetItem[] = [];
  const seen = new Set<string>();
  for (const item of [...incoming, ...previous]) {
    if (!item.id || seen.has(item.id)) continue;
    seen.add(item.id);
    result.push(stableAsset(previousByID.get(item.id), item));
    if (result.length >= limit) break;
  }
  return result;
}

export function shouldDedupeRecentRequest(
  lastStartedAt: number,
  now: number,
  force = false,
  windowMs = 3000,
): boolean {
  return !force && lastStartedAt > 0 && now - lastStartedAt < windowMs;
}

export function isCurrentRecentRequest(
  requestSequence: number,
  currentSequence: number,
  requestScope: string,
  currentScope: string,
): boolean {
  return requestSequence === currentSequence && requestScope === currentScope;
}
