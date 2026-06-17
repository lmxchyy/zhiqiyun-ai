import crypto from "node:crypto";

const now = () => new Date().toISOString();
const clampRate = (value) => Math.max(0, Math.min(1, Number(value || 0)));

export class GeoSource {
  constructor({
    url = process.env.GEO_SOURCE_URL,
    apiKey = process.env.GEO_SOURCE_API_KEY,
    timeoutMs = Number(process.env.GEO_SOURCE_TIMEOUT_MS || 15000)
  } = {}) {
    this.url = url;
    this.apiKey = apiKey;
    this.timeoutMs = timeoutMs;
    this.sourceCode = url ? "http-geo-source" : "mock-ai-search";
  }

  async collect({ brand, question, platform = this.sourceCode }) {
    if (!this.url) return this.localResult(brand, question, platform);
    const response = await fetch(this.url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(this.apiKey ? { Authorization: `Bearer ${this.apiKey}` } : {})
      },
      body: JSON.stringify({
        brand: { id: brand.id, name: brand.name, keywords: brand.keywords || [], competitors: brand.competitors || [] },
        question,
        platform
      }),
      signal: AbortSignal.timeout(this.timeoutMs)
    });
    if (!response.ok) throw new Error(`GEO source returned HTTP ${response.status}`);
    const result = await response.json();
    return this.normalizeResult(result, platform);
  }

  localResult(brand, question, platform) {
    const day = new Date().toISOString().slice(0, 10);
    const digest = crypto.createHash("sha256").update(`${brand.id}:${question}:${platform}:${day}`).digest();
    const mentionRate = Number((0.35 + digest[0] / 510).toFixed(2));
    const citationRate = Number((0.2 + digest[1] / 638).toFixed(2));
    const competitorRates = Object.fromEntries((brand.competitors || []).map((name, index) => [
      name,
      Number((0.25 + digest[(index + 2) % digest.length] / 638).toFixed(2))
    ]));
    return {
      mentionRate, citationRate, sentiment: digest[10] % 4 ? "POSITIVE" : "NEUTRAL",
      rank: Math.max(1, Math.ceil(8 - mentionRate * 7)), competitorRates,
      confidence: 0.76, source: "mock-ai-search", collectedAt: now()
    };
  }

  normalizeResult(result, platform) {
    const mentionRate = clampRate(result.mentionRate);
    const citationRate = clampRate(result.citationRate);
    return {
      mentionRate,
      citationRate,
      sentiment: ["POSITIVE", "NEUTRAL", "NEGATIVE"].includes(result.sentiment) ? result.sentiment : "NEUTRAL",
      rank: Math.max(1, Math.round(Number(result.rank || 10))),
      competitorRates: Object.fromEntries(Object.entries(result.competitorRates || {}).map(([key, value]) => [key, clampRate(value)])),
      confidence: clampRate(result.confidence ?? 0.8),
      source: result.source || platform || this.sourceCode,
      collectedAt: result.collectedAt || now(),
      raw: result.raw || null
    };
  }
}
