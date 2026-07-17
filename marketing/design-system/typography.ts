export const typography = {
  family: {
    chinese: '"HarmonyOS Sans", "PingFang SC", "Microsoft YaHei", sans-serif',
    numeric: '"DIN", "DIN Alternate", "HarmonyOS Sans", sans-serif',
  },
  heroTitle: { size: 92, lineHeight: 1.08, weight: 700, tracking: -2 },
  title: { size: 76, lineHeight: 1.12, weight: 700, tracking: -1 },
  subtitle: { size: 34, lineHeight: 1.45, weight: 400, tracking: 0 },
  eyebrow: { size: 24, lineHeight: 1.4, weight: 600, tracking: 3 },
  button: { size: 30, lineHeight: 1, weight: 600, tracking: 1 },
} as const;

export const typographyRules = {
  maxTitleLines: 2,
  maxSubtitleLines: 2,
  preferShortChineseCopy: true,
  forbidden: ["fake 3D bevel text", "outlined sale typography", "more than two font families"],
} as const;

