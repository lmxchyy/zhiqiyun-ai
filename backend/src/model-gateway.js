import crypto from "node:crypto";

export class ProviderError extends Error {
  constructor(message, { retryable = false, status = null } = {}) {
    super(message);
    this.name = "ProviderError";
    this.retryable = retryable;
    this.status = status;
  }
}

export class ModelGateway {
  constructor({
    url = process.env.MODEL_PROVIDER_URL,
    apiKey = process.env.MODEL_PROVIDER_API_KEY,
    kind = process.env.MODEL_PROVIDER_KIND,
    openAiApiKey = process.env.OPENAI_API_KEY,
    openAiBaseUrl = process.env.OPENAI_BASE_URL || "https://api.openai.com",
    openAiImageModel = process.env.MODEL_PROVIDER_IMAGE_MODEL || "gpt-image-2",
    openAiVideoModel = process.env.MODEL_PROVIDER_VIDEO_MODEL || "sora-2",
    timeoutMs = Number(process.env.MODEL_PROVIDER_TIMEOUT_MS || 30000),
    providers = process.env.MODEL_PROVIDERS_JSON ? JSON.parse(process.env.MODEL_PROVIDERS_JSON) : null
  } = {}) {
    this.url = url;
    this.apiKey = apiKey;
    this.kind = kind;
    this.openAiApiKey = openAiApiKey;
    this.openAiBaseUrl = openAiBaseUrl.replace(/\/$/, "");
    this.openAiImageModel = openAiImageModel;
    this.openAiVideoModel = openAiVideoModel;
    this.timeoutMs = timeoutMs;
    this.providers = Array.isArray(providers) ? providers.filter((item) => item?.active !== false && (item?.url || item?.kind === "openai")) : [];
    this.providerCode = this.providers.length ? "provider-router" : (kind === "openai" ? "openai" : (url ? "http-provider" : "local-provider"));
  }

  async generate(task) {
    if (this.providers.length) return this.routeGenerate(task);
    if (this.kind === "openai") return this.openAiGenerate(task);
    if (!this.url) return this.localGenerate(task);
    return this.httpGenerate(task);
  }

  async routeGenerate(task) {
    const candidates = this.providers
      .filter((item) => !item.capabilities?.length || item.capabilities.includes(task.type))
      .filter((item) => !item.models?.length || item.models.includes(task.model))
      .sort((left, right) => Number(right.weight || 0) - Number(left.weight || 0));
    if (!candidates.length) throw new ProviderError(`No model provider supports ${task.type} with model ${task.model}`);
    let lastError = null;
    for (const provider of candidates) {
      try {
        if (provider.kind === "openai") return await this.openAiGenerate(task, provider);
        return await this.httpGenerate(task, provider);
      } catch (error) {
        error.providerCode = provider.code || "http-provider";
        lastError = error;
        if (!error.retryable) throw error;
      }
    }
    throw lastError || new ProviderError("All model providers failed", { retryable: true });
  }

  localGenerate(task) {
    if (task.prompt.includes("[fail-retry]")) throw new ProviderError("Local provider temporary failure", { retryable: true, status: 503 });
    if (task.prompt.includes("[fail]")) throw new ProviderError("Local provider rejected generation", { retryable: false, status: 422 });
    const isVideo = task.type.includes("VIDEO");
    const safePrompt = task.prompt.replace(/[<>&]/g, "");
    const content = isVideo
      ? Buffer.from(`Xianzhi AI video provider output\nTask: ${task.id}\nPrompt: ${task.prompt}`)
      : Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="960" height="540"><defs><linearGradient id="g"><stop stop-color="#181b3a"/><stop offset="1" stop-color="#5b5cf0"/></linearGradient></defs><rect width="960" height="540" fill="url(#g)"/><text x="60" y="240" fill="white" font-size="42" font-family="Arial">Xianzhi AI provider output</text><text x="60" y="310" fill="#d8ddff" font-size="22" font-family="Arial">${safePrompt}</text></svg>`);
    return {
      providerCode: this.providerCode,
      providerRequestId: `local_${crypto.randomUUID()}`,
      content, contentType: isVideo ? "text/plain; charset=utf-8" : "image/svg+xml",
      extension: isVideo ? "txt" : "svg", costCents: isVideo ? 30 : 3,
      responseSnapshot: { mode: "local", bytes: content.length }
    };
  }

  async httpGenerate(task, provider = null) {
    const url = provider?.url || this.url;
    const apiKey = provider?.apiKey || this.apiKey;
    const timeoutMs = Number(provider?.timeoutMs || this.timeoutMs);
    let response;
    try {
      response = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {})
        },
        body: JSON.stringify({
          taskId: task.id, capability: task.type, model: task.model,
          prompt: task.prompt, params: task.params
        }),
        signal: AbortSignal.timeout(timeoutMs)
      });
    } catch (error) {
      throw new ProviderError(`Model provider request failed: ${error.message}`, { retryable: true });
    }
    if (!response.ok) {
      throw new ProviderError(`Model provider returned HTTP ${response.status}`, {
        retryable: response.status === 429 || response.status >= 500, status: response.status
      });
    }
    const result = await response.json();
    if (!result.dataBase64 || !result.contentType) throw new ProviderError("Model provider response is missing dataBase64 or contentType");
    return {
      providerCode: result.providerCode || provider?.code || this.providerCode,
      providerRequestId: result.providerRequestId || response.headers.get("x-request-id") || crypto.randomUUID(),
      content: Buffer.from(result.dataBase64, "base64"), contentType: result.contentType,
      extension: result.extension || (result.contentType.startsWith("image/") ? "png" : "mp4"),
      costCents: Number(result.costCents || 0),
      responseSnapshot: { contentType: result.contentType, bytes: Buffer.byteLength(result.dataBase64, "base64") }
    };
  }

  openAiConfig(provider = null) {
    return {
      apiKey: provider?.apiKey || this.openAiApiKey || this.apiKey,
      baseUrl: (provider?.baseUrl || provider?.url || this.openAiBaseUrl).replace(/\/$/, ""),
      providerCode: provider?.code || "openai",
      imageModel: provider?.imageModel || this.openAiImageModel,
      videoModel: provider?.videoModel || this.openAiVideoModel,
      timeoutMs: Number(provider?.timeoutMs || this.timeoutMs)
    };
  }

  async openAiGenerate(task, provider = null) {
    if (task.type.includes("IMAGE")) return this.openAiImageGenerate(task, provider);
    if (task.type.includes("VIDEO")) return this.openAiVideoGenerate(task, provider);
    throw new ProviderError(`OpenAI provider does not support ${task.type}`);
  }

  async openAiImageGenerate(task, provider = null) {
    if (task.type !== "TEXT_TO_IMAGE") {
      throw new ProviderError("OpenAI image adapter currently supports text-to-image tasks only");
    }
    const config = this.openAiConfig(provider);
    if (!config.apiKey) throw new ProviderError("OpenAI API key is not configured");
    const model = task.model?.startsWith("mock-") ? config.imageModel : (task.model || config.imageModel);
    let response;
    try {
      response = await fetch(`${config.baseUrl}/v1/images/generations`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${config.apiKey}`
        },
        body: JSON.stringify({
          model,
          prompt: task.prompt,
          size: task.params?.size || "1024x1024",
          n: 1,
          response_format: "b64_json"
        }),
        signal: AbortSignal.timeout(config.timeoutMs)
      });
    } catch (error) {
      throw new ProviderError(`OpenAI image request failed: ${error.message}`, { retryable: true });
    }
    if (!response.ok) {
      throw new ProviderError(`OpenAI image request returned HTTP ${response.status}`, {
        retryable: response.status === 429 || response.status >= 500, status: response.status
      });
    }
    const result = await response.json();
    const dataBase64 = result.data?.[0]?.b64_json;
    if (!dataBase64) throw new ProviderError("OpenAI image response is missing data[0].b64_json");
    return {
      providerCode: config.providerCode,
      providerRequestId: response.headers.get("x-request-id") || result.id || crypto.randomUUID(),
      content: Buffer.from(dataBase64, "base64"),
      contentType: "image/png",
      extension: "png",
      costCents: Number(provider?.costCents || 0),
      responseSnapshot: { contentType: "image/png", bytes: Buffer.byteLength(dataBase64, "base64"), model }
    };
  }

  async openAiVideoGenerate(task, provider = null) {
    if (task.type !== "TEXT_TO_VIDEO") {
      throw new ProviderError("OpenAI video adapter currently supports text-to-video tasks only");
    }
    const config = this.openAiConfig(provider);
    if (!config.apiKey) throw new ProviderError("OpenAI API key is not configured");
    const model = task.model?.startsWith("mock-") ? config.videoModel : (task.model || config.videoModel);
    const createPayload = {
      model,
      prompt: task.prompt,
      size: task.params?.size || "1280x720",
      seconds: Number(task.params?.seconds || task.params?.duration || 4)
    };
    let createResponse;
    try {
      createResponse = await fetch(`${config.baseUrl}/v1/videos`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${config.apiKey}`
        },
        body: JSON.stringify(createPayload),
        signal: AbortSignal.timeout(config.timeoutMs)
      });
    } catch (error) {
      throw new ProviderError(`OpenAI video request failed: ${error.message}`, { retryable: true });
    }
    if (!createResponse.ok) {
      throw new ProviderError(`OpenAI video request returned HTTP ${createResponse.status}`, {
        retryable: createResponse.status === 429 || createResponse.status >= 500, status: createResponse.status
      });
    }
    const created = await createResponse.json();
    const videoId = created.id;
    if (!videoId) throw new ProviderError("OpenAI video response is missing id");

    const maxPolls = Math.max(1, Number(provider?.maxPolls || task.params?.maxPolls || 30));
    const pollIntervalMs = Math.max(100, Number(provider?.pollIntervalMs || task.params?.pollIntervalMs || 2000));
    let status = created.status || "queued";
    for (let attempt = 0; attempt < maxPolls && !["completed", "failed", "cancelled"].includes(status); attempt += 1) {
      if (attempt > 0) await new Promise((resolve) => setTimeout(resolve, pollIntervalMs));
      const pollResponse = await fetch(`${config.baseUrl}/v1/videos/${videoId}`, {
        headers: { Authorization: `Bearer ${config.apiKey}` },
        signal: AbortSignal.timeout(config.timeoutMs)
      });
      if (!pollResponse.ok) {
        throw new ProviderError(`OpenAI video status returned HTTP ${pollResponse.status}`, {
          retryable: pollResponse.status === 429 || pollResponse.status >= 500, status: pollResponse.status
        });
      }
      const polled = await pollResponse.json();
      status = polled.status || status;
    }
    if (status !== "completed") throw new ProviderError(`OpenAI video generation did not complete: ${status}`, { retryable: status !== "failed" });

    const contentResponse = await fetch(`${config.baseUrl}/v1/videos/${videoId}/content`, {
      headers: { Authorization: `Bearer ${config.apiKey}` },
      signal: AbortSignal.timeout(config.timeoutMs)
    });
    if (!contentResponse.ok) {
      throw new ProviderError(`OpenAI video content returned HTTP ${contentResponse.status}`, {
        retryable: contentResponse.status === 429 || contentResponse.status >= 500, status: contentResponse.status
      });
    }
    const content = Buffer.from(await contentResponse.arrayBuffer());
    return {
      providerCode: config.providerCode,
      providerRequestId: createResponse.headers.get("x-request-id") || videoId,
      content,
      contentType: contentResponse.headers.get("content-type") || "video/mp4",
      extension: "mp4",
      costCents: Number(provider?.costCents || 0),
      responseSnapshot: { contentType: contentResponse.headers.get("content-type") || "video/mp4", bytes: content.length, model, videoId }
    };
  }
}
