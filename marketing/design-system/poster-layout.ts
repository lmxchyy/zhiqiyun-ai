export const posterCanvas = {
  width: 1080,
  height: 1920,
  format: "PNG",
  orientation: "portrait",
} as const;

export const posterLayout = {
  safeArea: { left: 72, top: 72, right: 72, bottom: 96 },
  logo: { x: 72, y: 72, maxWidth: 300, maxHeight: 86, align: "left" },
  content: { x: 72, y: 300, width: 936, align: "left" },
  productStage: { x: 72, y: 760, width: 936, height: 760 },
  cta: { x: 72, y: 1660, width: 300, height: 88 },
} as const;

export const layoutRules = {
  hierarchy: ["logo", "title", "subtitle", "single product visualization", "CTA"],
  maxPrimarySubjects: 1,
  preserveNegativeSpace: true,
  forbidden: ["collage", "grid", "multiple banners", "QR code", "contact details"],
} as const;

