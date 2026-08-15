import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import Ajv2020 from "ajv/dist/2020.js";

function readSchema(name) {
  const path = fileURLToPath(new URL(`../../../contracts/ppt-v2/${name}`, import.meta.url));
  return JSON.parse(readFileSync(path, "utf8"));
}

const schemas = {
  common: readSchema("common.schema.json"),
  slide: readSchema("slide-ir.schema.json"),
  deck: readSchema("deck.schema.json"),
  layout: readSchema("layout-result.schema.json"),
  renderInput: readSchema("render-input.schema.json"),
};
const ajv = new Ajv2020({ allErrors: true, strict: true });
for (const schema of Object.values(schemas)) {
  ajv.addSchema(schema);
}
const validators = {
  slide: ajv.getSchema(schemas.slide.$id),
  deck: ajv.getSchema(schemas.deck.$id),
  layout: ajv.getSchema(schemas.layout.$id),
  renderInput: ajv.getSchema(schemas.renderInput.$id),
};

class ContractError extends Error {
  constructor(kind, errors) {
    super(`PPT V2 ${kind} rejected:\n${errors.map((error) => `- ${error}`).join("\n")}`);
    this.name = "PPTV2ContractError";
    this.kind = kind;
    this.errors = errors;
  }
}

function schemaErrors(validator) {
  return (validator.errors ?? []).map((error) => {
    const path = error.instancePath || "/";
    const property = error.params?.additionalProperty ? ` (${error.params.additionalProperty})` : "";
    return `${path} ${error.message}${property}`;
  });
}

function duplicates(values, label) {
  const seen = new Set();
  const errors = [];
  for (const value of values) {
    if (seen.has(value)) {
      errors.push(`duplicate ${label}: ${value}`);
    }
    seen.add(value);
  }
  return errors;
}

function validate(validator, value, semantic = () => []) {
  if (!validator(value)) {
    return { valid: false, errors: schemaErrors(validator) };
  }
  const errors = semantic(value);
  return { valid: errors.length === 0, errors };
}

export function validateSlideIR(slide) {
  return validate(validators.slide, slide, (value) => duplicates(value.elements.map((item) => item.id), "element id"));
}

export function validateDeckRevision(deck) {
  return validate(validators.deck, deck, (value) => {
    const errors = [
      ...duplicates(value.slides.map((slide) => slide.id), "slide id"),
      ...duplicates(value.slides.flatMap((slide) => slide.elements.map((item) => item.id)), "element id"),
    ];
    value.slides.forEach((slide, index) => {
      if (slide.sequence !== index + 1) {
        errors.push(`slide ${slide.id} sequence ${slide.sequence} does not match position ${index + 1}`);
      }
    });
    return errors;
  });
}

export function validateLayoutResult(layoutResult) {
  return validate(validators.layout, layoutResult, (value) => {
    const errors = [
      ...duplicates(value.slides.map((slide) => slide.slideId), "layout slide id"),
      ...duplicates(value.slides.flatMap((slide) => slide.elements.map((item) => item.elementId)), "layout element id"),
    ];
    for (const item of value.diagnostics) {
      if (item.severity === "error") {
        errors.push(`${item.code}: ${item.message}`);
      }
    }
    return errors;
  });
}

export function validateRenderInput(renderInput) {
  return validate(validators.renderInput, renderInput, (value) => {
    const errors = [];
    const layoutBySlide = new Map(value.layoutResults.map((item) => [item.slideId, item]));
    for (const slide of value.slides) {
      const layout = layoutBySlide.get(slide.id);
      if (!layout) {
        errors.push(`missing layout result for slide ${slide.id}`);
        continue;
      }
      const layoutElements = new Set(layout.elements.map((item) => item.elementId));
      for (const element of slide.elements) {
        if (!layoutElements.has(element.id)) {
          errors.push(`missing layout element for ${element.id}`);
        }
      }
      const slideElements = new Set(slide.elements.map((item) => item.id));
      for (const element of layout.elements) {
        if (!slideElements.has(element.elementId)) {
          errors.push(`layout references unknown element ${element.elementId}`);
        }
      }
      for (const item of layout.diagnostics) {
        if (item.severity === "error") {
          errors.push(`${item.code}: ${item.message}`);
        }
      }
    }
    for (const layout of value.layoutResults) {
      if (!value.slides.some((slide) => slide.id === layout.slideId)) {
        errors.push(`layout references unknown slide ${layout.slideId}`);
      }
    }
    return errors;
  });
}

function assertResult(kind, value, result) {
  if (!result.valid) {
    throw new ContractError(kind, result.errors);
  }
  return value;
}

export function assertValidSlideIR(slide) {
  return assertResult("SlideIR", slide, validateSlideIR(slide));
}

export function assertValidDeckRevision(deck) {
  return assertResult("deck revision", deck, validateDeckRevision(deck));
}

export function assertValidLayoutResult(layoutResult) {
  return assertResult("layout result", layoutResult, validateLayoutResult(layoutResult));
}

export function assertValidRenderInput(renderInput) {
  return assertResult("render input", renderInput, validateRenderInput(renderInput));
}
