function greatestCommonDivisor(left: number, right: number) {
  let a = left;
  let b = right;
  while (b !== 0) {
    const remainder = a % b;
    a = b;
    b = remainder;
  }
  return a;
}

function sizeAspectLabel(width: number, height: number) {
  const divisor = greatestCommonDivisor(width, height);
  return `${width / divisor}:${height / divisor}`;
}

function sizeTierLabel(width: number, height: number) {
  const pixels = width * height;
  const maxEdge = Math.max(width, height);
  if (pixels <= 1280 * 720 && maxEdge <= 1280) return "720p";
  if (pixels <= 1536 * 1024 && maxEdge <= 1536) return "1K";
  if (pixels <= 2048 * 2048 && maxEdge <= 2048) return "2K";
  return "4K";
}

const commonImageAspectLabels = new Set(["1:1", "16:9", "9:16", "3:2", "2:3"]);

export function displayGptImageSizeLabel(value: string) {
  const normalized = String(value || "").trim();
  if (!normalized || normalized.toLowerCase() === "auto") return "auto";
  const match = normalized.match(/^(\d+)x(\d+)$/i);
  if (!match) return normalized;
  const width = Number(match[1]);
  const height = Number(match[2]);
  const aspect = sizeAspectLabel(width, height);
  const tier = sizeTierLabel(width, height);
  if (commonImageAspectLabels.has(aspect)) return `${tier} · ${aspect}`;
  return `${tier} · 自定义 · ${width}×${height}`;
}
