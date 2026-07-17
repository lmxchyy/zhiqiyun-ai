export const brandLogo = {
  source: "../assets/zhiqiyun-ai-logo-v2-transparent.png",
  format: "PNG",
  background: "transparent",
  intrinsicWidth: 1328,
  intrinsicHeight: 360,
  placement: "top-left",
  maxDisplayWidth: 300,
} as const;

export const logoRules = {
  mustUseSourceAsset: true,
  preserveAspectRatio: true,
  preserveColorsAndLettering: true,
  supportSurface: "compact pale blue-violet translucent glass; never pure white",
  forbidden: ["redraw", "retype", "distort", "crop", "pure white plaque", "glow that reduces legibility"],
} as const;
