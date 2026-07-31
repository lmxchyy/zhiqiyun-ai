import test from "node:test";
import assert from "node:assert/strict";
import {
  VIDEO_PARAMETER_KEYS,
  buildVideoSubmissionParameters,
  deriveEditableVideoFields,
  transitionVideoParameterValues,
} from "../packages/business-sdk/dist/videoParameters.js";

const coreCapabilities = {
  supportsTextToVideo: true,
  supportsImageToVideo: false,
  supportsFirstFrame: false,
  supportsLastFrame: false,
  maxReferenceImages: 0,
  supportedDurations: [5, 10],
  supportedResolutions: ["720p", "1080p"],
  supportedAspectRatios: ["16:9", "9:16"],
  supportedParameters: ["duration", "resolution", "aspect_ratio"],
};

function schema(fields) {
  return { fields };
}

function field(key, overrides = {}) {
  return {
    key,
    label: key,
    type: "select",
    visible: true,
    userEditable: true,
    ...overrides,
  };
}

test("video parameter whitelist is exact and canonical", () => {
  assert.deepEqual(VIDEO_PARAMETER_KEYS, [
    "duration",
    "resolution",
    "aspect_ratio",
    "fps",
    "generate_audio",
    "motion_strength",
    "camera_movement",
  ]);
});

test("editable fields require visible editable schema fields and provider support", () => {
  const fields = deriveEditableVideoFields(schema([
    field("duration", { options: [5, 10], default: 5 }),
    field("resolution", { options: ["720p", "1080p"], default: "720p" }),
    field("ratio", { options: ["16:9", "9:16"], default: "16:9" }),
    field("fps", { options: [24, 30], default: 24 }),
    field("generate_audio", { type: "boolean", default: true }),
    field("motion_strength", { visible: false }),
    field("camera_movement", { userEditable: false }),
    field("prompt", { type: "textarea" }),
  ]), coreCapabilities);

  assert.deepEqual(fields.map(item => item.key), [
    "duration",
    "resolution",
    "aspect_ratio",
  ]);
  assert.equal(fields[2].defaultValue, "16:9");
});

test("seedance audio is exposed only when both schema and provider support it", () => {
  const audioSchema = schema([
    field("generate_audio", { type: "boolean", default: true }),
  ]);
  assert.deepEqual(deriveEditableVideoFields(audioSchema, coreCapabilities), []);
  assert.deepEqual(
    deriveEditableVideoFields(audioSchema, {
      ...coreCapabilities,
      supportedParameters: [...coreCapabilities.supportedParameters, "generate_audio"],
    }).map(item => item.key),
    ["generate_audio"],
  );
});

test("model transition keeps common legal values and clears unsupported values", () => {
  const modelBFields = deriveEditableVideoFields(schema([
    field("duration", { options: [5, 8], default: 8 }),
    field("resolution", { options: ["480p", "720p"], default: "720p" }),
    field("aspect_ratio", { options: ["16:9", "9:16"], default: "16:9" }),
  ]), {
    ...coreCapabilities,
    supportedDurations: [5, 8],
    supportedResolutions: ["480p", "720p"],
  });

  assert.deepEqual(
    transitionVideoParameterValues({
      duration: 5,
      resolution: "1080p",
      ratio: "9:16",
      generate_audio: true,
      fps: 30,
    }, modelBFields),
    {
      duration: 5,
      resolution: "720p",
      aspect_ratio: "9:16",
    },
  );
});

test("invalid value falls back to schema default then first legal option", () => {
  const fields = deriveEditableVideoFields(schema([
    field("duration", { options: [5, 8], default: 8 }),
    field("resolution", { options: ["480p", "720p"], default: "4k" }),
    field("aspect_ratio", { options: ["16:9", "9:16"] }),
  ]), {
    ...coreCapabilities,
    supportedDurations: [5, 8],
    supportedResolutions: ["480p", "720p"],
  });

  assert.deepEqual(
    transitionVideoParameterValues({
      duration: 99,
      resolution: "1080p",
      aspect_ratio: "1:1",
    }, fields),
    {
      duration: 8,
      resolution: "480p",
      aspect_ratio: "16:9",
    },
  );
});

test("submission contains only fields in the final editable contract", () => {
  const fields = deriveEditableVideoFields(schema([
    field("duration", { options: [5, 10], default: 5 }),
    field("aspect_ratio", { options: ["16:9", "9:16"], default: "16:9" }),
  ]), coreCapabilities);

  assert.deepEqual(buildVideoSubmissionParameters({
    duration: 10,
    aspect_ratio: "9:16",
    ratio: "1:1",
    fps: 30,
    generate_audio: true,
  }, fields), {
    duration: 10,
    aspect_ratio: "9:16",
  });
});
