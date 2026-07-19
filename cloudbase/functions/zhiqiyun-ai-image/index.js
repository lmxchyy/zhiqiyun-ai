"use strict";

const cloudbase = require("@cloudbase/node-sdk");

const TEXT_TO_IMAGE_MODEL = "HY-Image-3.0-Plus-4090-Tob-v1.0";
const IMAGE_TO_IMAGE_MODEL = "HY-Image-v3.0-I2I-ToB-v1.0.1";
const ALLOWED_MODELS = new Set([TEXT_TO_IMAGE_MODEL, IMAGE_TO_IMAGE_MODEL]);
const ALLOWED_SIZES = new Set(["1024x1024", "1280x720", "720x1280", "1280x1280"]);

function parseEvent(event) {
  if (!event || typeof event !== "object") return {};
  if (typeof event.body !== "string") return event;
  try {
    return { ...event, ...JSON.parse(event.body) };
  } catch {
    return event;
  }
}

function normalizedReferenceImages(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map(item => String(item || "").trim())
    .filter(item => {
      try {
        return new URL(item).protocol === "https:";
      } catch {
        return false;
      }
    })
    .slice(0, 1);
}

exports.main = async event => {
  const input = parseEvent(event);
  const prompt = String(input.prompt || "").trim();
  const model = String(input.model || "").trim();
  const requestId = String(input.requestId || "").trim();
  const size = ALLOWED_SIZES.has(input.size) ? input.size : "1024x1024";
  const referenceImages = normalizedReferenceImages(input.referenceImages);

  if (!prompt || [...prompt].length > 500) {
    return { error: "invalid_prompt", requestId };
  }
  if (!ALLOWED_MODELS.has(model)) {
    return { error: "model_not_allowed", requestId };
  }
  if (model === IMAGE_TO_IMAGE_MODEL && referenceImages.length !== 1) {
    return { error: "reference_image_required", requestId };
  }
  if (model === TEXT_TO_IMAGE_MODEL && referenceImages.length) {
    return { error: "reference_image_not_supported", requestId };
  }

  const app = cloudbase.init({ env: process.env.ENV_ID });
  const imageModel = app.ai().createImageModel("hunyuan-image");
  const watermark = String(process.env.AI_WATERMARK_TEXT || "AI生成").trim().slice(0, 16) || "AI生成";
  const generationInput = {
    model,
    prompt,
    size,
    revise: { value: true },
    footnote: watermark,
  };
  if (model === IMAGE_TO_IMAGE_MODEL) generationInput.image_urls = referenceImages;

  const response = await imageModel.generateImage(generationInput);
  const first = Array.isArray(response.data) ? response.data[0] : null;
  if (!first || typeof first.url !== "string" || !first.url.startsWith("https://")) {
    return { error: "empty_generation_result", requestId };
  }
  return {
    requestId,
    providerTaskId: String(response.id || ""),
    model,
    url: first.url,
    revisedPrompt: String(first.revised_prompt || ""),
    contentType: "image/jpeg",
    aiGenerated: true,
    watermarkApplied: true,
  };
};
