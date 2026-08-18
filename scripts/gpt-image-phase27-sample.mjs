/**
 * Phase 2.7 live GPT Image / NewAPI sampler.
 * Does not publish billing rules or change production prices.
 */
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const envPath = resolve(root, ".env");
const env = Object.fromEntries(
  readFileSync(envPath, "utf8")
    .replace(/^\uFEFF/, "")
    .split(/\r?\n/)
    .filter((line) => line && !line.startsWith("#") && line.includes("="))
    .map((line) => {
      const idx = line.indexOf("=");
      return [line.slice(0, idx).trim().replace(/^\uFEFF/, ""), line.slice(idx + 1).trim()];
    }),
);

const BASE = String(env.MODEL_PROVIDER_URL || "").replace(/\/$/, "");
const KEY = env.MODEL_PROVIDER_API_KEY;
const MODEL = env.MODEL_PROVIDER_IMAGE_MODEL || "gpt-image-2";
const TIMEOUT_MS = Number(env.MODEL_PROVIDER_TIMEOUT_MS || 180000);
const TEXT_IN_USD = 5 / 1e6;
const IMAGE_IN_USD = 8 / 1e6;
const IMAGE_OUT_USD = 30 / 1e6;
const PROMPT = "Phase 2.7 cost sample: a simple red ceramic mug on a white table, studio lighting, no text.";
const OUTPUT_FORMAT = "jpeg";
const NEWAPI_CATALOG_USD_PER_REQUEST = 0.1;
const OFFICIAL_1K_OUTPUT_USD = {
  "1024x1024": { low: 0.006, medium: 0.053, high: 0.211 },
  "1024x1536": { low: 0.005, medium: 0.041, high: 0.165 },
  "1536x1024": { low: 0.005, medium: 0.041, high: 0.165 },
};

if (!BASE || !KEY) {
  throw new Error("MODEL_PROVIDER_URL / MODEL_PROVIDER_API_KEY missing");
}

function proposedPoints(quality, n) {
  const unit = quality === "low" ? 10 : quality === "high" ? 220 : 55;
  return unit * n;
}

function stripImages(value) {
  if (Array.isArray(value)) return value.map(stripImages);
  if (!value || typeof value !== "object") return value;
  const out = {};
  for (const [key, item] of Object.entries(value)) {
    if (["b64_json", "b64Json", "result"].includes(key) && typeof item === "string" && item.length > 80) {
      out[key] = `[omitted ${item.length} chars]`;
      continue;
    }
    if (key === "url" && typeof item === "string" && item.startsWith("data:")) {
      out[key] = `[data-url omitted ${item.length} chars]`;
      continue;
    }
    out[key] = stripImages(item);
  }
  return out;
}

function jpegSizeFromB64(b64) {
  try {
    const buf = Buffer.from(b64, "base64");
    if (buf.length < 4 || buf[0] !== 0xff || buf[1] !== 0xd8) return null;
    let offset = 2;
    while (offset + 8 < buf.length) {
      if (buf[offset] !== 0xff) {
        offset += 1;
        continue;
      }
      const marker = buf[offset + 1];
      if (marker === 0xd8 || marker === 0xd9) {
        offset += 2;
        continue;
      }
      if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc) {
        return `${buf.readUInt16BE(offset + 7)}x${buf.readUInt16BE(offset + 5)}`;
      }
      const length = buf.readUInt16BE(offset + 2);
      if (length < 2) break;
      offset += 2 + length;
    }
  } catch {}
  return null;
}

function imageMetaFromBody(body) {
  if (!Array.isArray(body?.data)) return [];
  return body.data.map((item) => ({
    b64Bytes: typeof item?.b64_json === "string" ? Buffer.from(item.b64_json, "base64").length : null,
    decodedSize: typeof item?.b64_json === "string" ? jpegSizeFromB64(item.b64_json) : null,
  }));
}

function officialEstimateUsd(size, quality, n) {
  const unit = OFFICIAL_1K_OUTPUT_USD[size]?.[quality];
  if (unit == null) return null;
  return unit * n;
}

function usageFromBody(body) {
  const usage = body?.usage || {};
  const details = usage.input_tokens_details || {};
  return {
    input_tokens: usage.input_tokens ?? usage.prompt_tokens ?? null,
    output_tokens: usage.output_tokens ?? usage.completion_tokens ?? null,
    total_tokens: usage.total_tokens ?? null,
    text_input_tokens: details.text_tokens ?? details.text ?? null,
    image_input_tokens: details.image_tokens ?? details.image ?? null,
    quota: body?.quota ?? usage.quota ?? null,
  };
}

function imageCountFromBody(body, endpoint) {
  if (Array.isArray(body?.data)) return body.data.length;
  if (Array.isArray(body?.output)) {
    return body.output.filter((item) => item?.type === "image_generation_call" || item?.result).length;
  }
  if (endpoint.includes("responses") && Array.isArray(body?.output)) return body.output.length;
  return null;
}

function outputSizeHint(body) {
  const first = body?.data?.[0] || body?.output?.find((item) => item?.size || item?.result);
  return first?.size || first?.revised_prompt || body?.size || null;
}

function openaiUsd(usage) {
  const textIn = Number(usage.text_input_tokens ?? usage.input_tokens ?? 0);
  const imageIn = Number(usage.image_input_tokens ?? 0);
  const output = Number(usage.output_tokens ?? 0);
  if (!usage.output_tokens && !usage.input_tokens) return null;
  const textPart = usage.text_input_tokens != null ? textIn : Number(usage.input_tokens ?? 0);
  return textPart * TEXT_IN_USD + imageIn * IMAGE_IN_USD + output * IMAGE_OUT_USD;
}

async function fetchFx() {
  try {
    const res = await fetch("https://open.er-api.com/v6/latest/USD", { signal: AbortSignal.timeout(15000) });
    const json = await res.json();
    const rate = Number(json?.rates?.CNY);
    if (rate > 0) return { usdCny: rate, source: "open.er-api.com", asOf: json?.time_last_update_utc || null };
  } catch {}
  return { usdCny: 7.2, source: "fallback-constant", asOf: null };
}

function interestingHeaders(headers) {
  const out = {};
  for (const [key, value] of headers.entries()) {
    const lower = key.toLowerCase();
    if (
      lower.includes("quota") ||
      lower.includes("remain") ||
      lower.includes("request-id") ||
      lower.includes("new-api") ||
      lower.includes("x-oneapi") ||
      lower.includes("ratelimit")
    ) {
      out[key] = value;
    }
  }
  return out;
}

async function postJson(path, payload) {
  const url = `${BASE}${path}`;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), TIMEOUT_MS);
  const started = Date.now();
  try {
    const res = await fetch(url, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${KEY}`,
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: JSON.stringify(payload),
      signal: controller.signal,
    });
    const text = await res.text();
    let body = null;
    try {
      body = JSON.parse(text);
    } catch {
      body = { raw: text.slice(0, 800) };
    }
    const imageMeta = imageMetaFromBody(body);
    return {
      url,
      status: res.status,
      ms: Date.now() - started,
      headers: interestingHeaders(res.headers),
      body: stripImages(body),
      imageMeta,
    };
  } finally {
    clearTimeout(timer);
  }
}

function responsesPayload(sample) {
  return {
    model: MODEL,
    input: [
      {
        role: "user",
        content: [
          {
            type: "input_text",
            text: `Use the following text as the complete prompt. Do not rewrite it:\n${PROMPT}`,
          },
        ],
      },
    ],
    tools: [
      {
        type: "image_generation",
        action: "generate",
        size: sample.size,
        quality: sample.quality,
      },
    ],
    tool_choice: "required",
  };
}

async function runSample(sample) {
  const generationsPayload = {
    model: MODEL,
    prompt: PROMPT,
    n: sample.n,
    size: sample.size,
    quality: sample.quality,
    output_format: OUTPUT_FORMAT,
  };
  let generations = await postJson("/v1/images/generations", generationsPayload);
  if (generations.status === 504 || generations.status === 502) {
    generations = await postJson("/v1/images/generations", generationsPayload);
  }
  const usage = usageFromBody(generations.body);
  return {
    label: sample.label,
    request: { ...sample, prompt: PROMPT, model: MODEL, output_format: OUTPUT_FORMAT },
    providerPayload: generationsPayload,
    endpoint: "images/generations",
    httpStatus: generations.status,
    latencyMs: generations.ms,
    headers: generations.headers,
    imageCount: imageCountFromBody(generations.body, "images/generations"),
    imageMeta: generations.imageMeta,
    outputSizeHint: generations.imageMeta?.[0]?.decodedSize || outputSizeHint(generations.body),
    usage,
    responseKeys: generations.body && typeof generations.body === "object" ? Object.keys(generations.body) : [],
    error: generations.status >= 300 ? generations.body : null,
    fallback: null,
  };
}

function enrich(row, fx, catalog) {
  const n = row.request.n;
  const quality = row.request.quality;
  const size = row.request.size;
  const sellPoints = proposedPoints(quality, n);
  const sellCny = sellPoints / 100;
  const tokenUsd = openaiUsd(row.usage);
  const tableUsd = officialEstimateUsd(size, quality, n);
  const officialUsd = tokenUsd ?? tableUsd;
  const officialSource = tokenUsd != null ? "response.usage" : tableUsd != null ? "openai-1k-calculator-estimate" : null;
  const officialCny = officialUsd == null ? null : officialUsd * fx.usdCny;
  const catalogUsd = catalog?.model_price != null ? Number(catalog.model_price) * n : NEWAPI_CATALOG_USD_PER_REQUEST * n;
  const newapiUsd = row.usage.quota != null && Number(row.usage.quota) > 0 ? Number(row.usage.quota) / 500000 : null;
  const newapiNote = newapiUsd != null
    ? "quota/500000 from response"
    : `Images API response has no usage/quota; NewAPI /api/pricing gpt-image-2 quota_type=1 model_price=${catalog?.model_price ?? 0.1} (treated as USD per image * n, unconfirmed debit)`;
  const costUsd = newapiUsd ?? officialUsd;
  const costCny = costUsd == null ? null : costUsd * fx.usdCny;
  const costPoints = costCny == null ? null : costCny * 100;
  const marginCny = costCny == null ? null : sellCny - costCny;
  const marginRate = costCny == null || sellCny === 0 ? null : (sellCny - costCny) / sellCny;
  const catalogCny = catalogUsd * fx.usdCny;
  const catalogMarginRate = (sellCny - catalogCny) / sellCny;
  return {
    ...row,
    fx,
    officialUsd,
    officialSource,
    officialCny,
    newapiUsd,
    newapiNote,
    newapiCatalogUsd: catalogUsd,
    newapiCatalogCny: catalogCny,
    catalogMarginRate,
    costUsd,
    costCny,
    costPoints,
    sellPoints,
    sellCny,
    marginCny,
    marginRate,
  };
}

const cases = [
  ...["1024x1024", "1024x1536", "1536x1024"].flatMap((size) =>
    ["low", "medium", "high"].map((quality) => ({
      label: `1k-${size}-${quality}-n1`,
      size,
      quality,
      n: 1,
    })),
  ),
  ...[2, 3, 4].map((n) => ({
    label: `n-1024x1024-medium-n${n}`,
    size: "1024x1024",
    quality: "medium",
    n,
  })),
  ...[1, 2, 3, 4, 5].map((i) => ({
    label: `auto-n1-sample-${i}`,
    size: "auto",
    quality: "auto",
    n: 1,
  })),
  ...["1280x720", "720x1280", "2048x1152", "2048x2048", "3840x2160", "2160x3840"].map((size) => ({
    label: `hidden-${size}-medium-n1`,
    size,
    quality: "medium",
    n: 1,
  })),
];

const only = process.argv.slice(2);
const selected = only.length ? cases.filter((item) => only.includes(item.label) || only.includes("smoke") && item.label === "1k-1024x1024-low-n1") : cases;

const outDir = resolve(root, "docs");
mkdirSync(outDir, { recursive: true });
const outFile = resolve(outDir, "gpt-image-phase-2.7-channel-sample.json");
const fx = await fetchFx();
const pricing = await fetch(`${BASE}/api/pricing`, { signal: AbortSignal.timeout(15000) }).then((res) => res.json()).catch(() => null);
const catalog = (pricing?.data || []).find((item) => item.model_name === MODEL) || null;
const rows = [];
for (const sample of selected) {
  process.stderr.write(`sampling ${sample.label}...\n`);
  try {
    const row = enrich(await runSample(sample), fx, catalog);
    rows.push(row);
    writeFileSync(outFile, JSON.stringify({
      generatedAt: new Date().toISOString(),
      base: BASE,
      model: MODEL,
      samplingNote: "output_format=jpeg to avoid OpenResty 504 on large PNG b64; usage.output_tokens absent from Images API response",
      fx,
      catalog,
      groupRatio: pricing?.group_ratio || null,
      rows,
    }, null, 2));
    process.stderr.write(
      `  ${row.httpStatus} ${row.endpoint} images=${row.imageCount} size=${row.outputSizeHint} out=${row.usage.output_tokens} officialUsd=${row.officialUsd}\n`,
    );
  } catch (error) {
    rows.push({ label: sample.label, request: sample, error: String(error) });
    process.stderr.write(`  ERROR ${error}\n`);
  }
}

process.stderr.write(`wrote ${outFile}\n`);
console.log(JSON.stringify({ fx, count: rows.length, labels: rows.map((row) => row.label) }));
