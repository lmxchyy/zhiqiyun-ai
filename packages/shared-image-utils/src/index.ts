export type ResolutionTier = "720p" | "1K" | "2K" | "4K" | "auto";

export interface SizeOption {
  value: string;
  label: string;
}

export interface RatioGroup {
  ratio: string;
  sizes: Array<{
    value: string;
    tier: ResolutionTier;
    width: number;
    height: number;
  }>;
}

const COMMON_IMAGE_ASPECTS: ReadonlyArray<{ label: string; width: number; height: number }> = [
  { label: "1:1", width: 1, height: 1 },
  { label: "16:9", width: 16, height: 9 },
  { label: "9:16", width: 9, height: 16 },
  { label: "4:3", width: 4, height: 3 },
  { label: "3:4", width: 3, height: 4 },
  { label: "3:2", width: 3, height: 2 },
  { label: "2:3", width: 2, height: 3 },
];

const COMMON_IMAGE_ASPECT_LABELS = new Set<string>(
  COMMON_IMAGE_ASPECTS.map(item => item.label),
);

export const VISIBLE_RESOLUTION_TIERS: ResolutionTier[] = ["1K", "2K", "4K"];

const TIER_ORDER: Record<ResolutionTier, number> = {
  "720p": 0,
  "1K": 1,
  "2K": 2,
  "4K": 3,
  auto: -1,
};

export function greatestCommonDivisor(left: number, right: number): number {
  let a = Math.abs(Math.round(left));
  let b = Math.abs(Math.round(right));
  while (b !== 0) {
    const remainder = a % b;
    a = b;
    b = remainder;
  }
  return a || 1;
}

export function canonicalSizeParts(value: unknown): [number, number] | undefined {
  if (typeof value !== "string") return undefined;
  const match = value.match(/^([1-9]\d*)x([1-9]\d*)$/i);
  if (!match) return undefined;
  const width = Number(match[1]);
  const height = Number(match[2]);
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width <= 0 || height <= 0) {
    return undefined;
  }
  return [width, height];
}

export function isCanonicalImageSize(value: unknown): value is string {
  return value === "auto" || Boolean(canonicalSizeParts(value));
}

export function deriveRatioFromSize(width: number, height: number): string {
  const divisor = greatestCommonDivisor(width, height);
  return `${width / divisor}:${height / divisor}`;
}

export function deriveRatioLabelFromSize(width: number, height: number): string {
  const ratio = deriveRatioFromSize(width, height);
  if (COMMON_IMAGE_ASPECT_LABELS.has(ratio)) return ratio;
  return `${width}x${height}`;
}

export function isCommonAspectRatio(ratio: string): boolean {
  return COMMON_IMAGE_ASPECT_LABELS.has(ratio);
}

function aspectLogDistance(width: number, height: number, ratioLabel: string): number {
  const target = COMMON_IMAGE_ASPECTS.find(item => item.label === ratioLabel);
  if (!target) return Number.POSITIVE_INFINITY;
  return Math.abs(Math.log(width / height) - Math.log(target.width / target.height));
}

export function classifyCommonAspectRatio(width: number, height: number): string {
  const exact = deriveRatioFromSize(width, height);
  if (COMMON_IMAGE_ASPECT_LABELS.has(exact)) return exact;

  let best = COMMON_IMAGE_ASPECTS[0];
  let bestDist = Number.POSITIVE_INFINITY;
  for (const candidate of COMMON_IMAGE_ASPECTS) {
    const dist = aspectLogDistance(width, height, candidate.label);
    if (dist < bestDist) {
      bestDist = dist;
      best = candidate;
    }
  }
  return best.label;
}

export function deriveTierFromSize(width: number, height: number): ResolutionTier {
  const pixels = width * height;
  const maxEdge = Math.max(width, height);
  if (pixels <= 1280 * 720 && maxEdge <= 1280) return "720p";
  if (pixels <= 1536 * 1024 && maxEdge <= 1536) return "1K";
  if (pixels <= 2048 * 2048 && maxEdge <= 2048) return "2K";
  return "4K";
}

export function deriveTierFromSizeValue(value: string): ResolutionTier {
  if (value === "auto") return "auto";
  const parts = canonicalSizeParts(value);
  if (!parts) return "auto";
  return deriveTierFromSize(parts[0], parts[1]);
}

export function deriveRatioFromSizeValue(value: string): string | undefined {
  const parts = canonicalSizeParts(value);
  if (!parts) return undefined;
  return classifyCommonAspectRatio(parts[0], parts[1]);
}

export function displayImageSizeLabel(value: string): string {
  if (value === "auto") return "auto";
  const parts = canonicalSizeParts(value);
  if (!parts) throw new Error(`invalid canonical image size ${value}`);
  const [width, height] = parts;
  const ratio = deriveRatioFromSize(width, height);
  const tier = deriveTierFromSize(width, height);
  if (isCommonAspectRatio(ratio)) return `${tier} · ${ratio}`;
  return `${tier} · ${width}x${height}`;
}

export function groupSizesByRatio(
  sizeOptions: string[],
): RatioGroup[] {
  const groups = new Map<string, RatioGroup>();

  for (const value of sizeOptions) {
    if (value === "auto") continue;
    const parts = canonicalSizeParts(value);
    if (!parts) continue;
    const [width, height] = parts;
    const ratio = classifyCommonAspectRatio(width, height);
    const tier = deriveTierFromSize(width, height);

    if (!groups.has(ratio)) {
      groups.set(ratio, { ratio, sizes: [] });
    }
    groups.get(ratio)!.sizes.push({ value, tier, width, height });
  }

  const result = Array.from(groups.values());
  for (const group of result) {
    group.sizes.sort((a, b) => TIER_ORDER[a.tier] - TIER_ORDER[b.tier]);
  }

  const ratioOrder = [
    "1:1",
    "16:9",
    "9:16",
    "4:3",
    "3:4",
    "3:2",
    "2:3",
  ];
  result.sort((a, b) => {
    const ai = ratioOrder.indexOf(a.ratio);
    const bi = ratioOrder.indexOf(b.ratio);
    if (ai !== -1 && bi !== -1) return ai - bi;
    if (ai !== -1) return -1;
    if (bi !== -1) return 1;
    return a.ratio.localeCompare(b.ratio);
  });

  return result;
}

export function findSizeByRatioAndTier(
  sizeOptions: string[],
  ratio: string,
  tier: ResolutionTier,
): string | undefined {
  let best: string | undefined;
  let bestDist = Number.POSITIVE_INFINITY;
  for (const value of sizeOptions) {
    if (value === "auto") continue;
    const parts = canonicalSizeParts(value);
    if (!parts) continue;
    const [width, height] = parts;
    const derivedRatio = classifyCommonAspectRatio(width, height);
    const derivedTier = deriveTierFromSize(width, height);
    if (derivedRatio !== ratio || derivedTier !== tier) continue;
    const dist = aspectLogDistance(width, height, ratio);
    if (dist < bestDist) {
      bestDist = dist;
      best = value;
    }
  }
  return best;
}

export function getAvailableTiersForRatio(
  sizeOptions: string[],
  ratio: string,
): ResolutionTier[] {
  const tiers = new Set<ResolutionTier>();
  for (const value of sizeOptions) {
    if (value === "auto") continue;
    const parts = canonicalSizeParts(value);
    if (!parts) continue;
    const [width, height] = parts;
    const derivedRatio = classifyCommonAspectRatio(width, height);
    if (derivedRatio === ratio) {
      tiers.add(deriveTierFromSize(width, height));
    }
  }
  return Array.from(tiers).sort((a, b) => TIER_ORDER[a] - TIER_ORDER[b]);
}

export function getVisibleTiersForRatio(
  sizeOptions: string[],
  ratio: string,
): ResolutionTier[] {
  return getAvailableTiersForRatio(sizeOptions, ratio)
    .filter((tier): tier is "1K" | "2K" | "4K" => VISIBLE_RESOLUTION_TIERS.includes(tier));
}

export function getAvailableRatios(sizeOptions: string[]): string[] {
  return groupSizesByRatio(sizeOptions)
    .map(g => g.ratio)
    .filter(ratio => COMMON_IMAGE_ASPECT_LABELS.has(ratio));
}

export function hasAutoOption(sizeOptions: string[]): boolean {
  return sizeOptions.some(value => value === "auto");
}
