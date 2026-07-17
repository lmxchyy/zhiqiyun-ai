export const brandColors = {
  primary: "#4F6BFF",
  secondary: "#725BFF",
  accent: "#45C8FF",
  highlight: "#FF7A1A",
  background: "#07142E",
  textPrimary: "#FFFFFF",
  textSecondary: "#C9D4F2",
  glass: "rgba(255, 255, 255, 0.10)",
  glassBorder: "rgba(255, 255, 255, 0.22)",
} as const;

export const brandGradients = {
  primary: "linear-gradient(135deg, #4F6BFF 0%, #725BFF 52%, #45C8FF 100%)",
  atmospheric: "radial-gradient(circle at 68% 38%, rgba(69,200,255,.26), transparent 38%), radial-gradient(circle at 30% 66%, rgba(114,91,255,.30), transparent 44%), #07142E",
  highlight: "linear-gradient(135deg, #FF7A1A 0%, #FF9A4D 100%)",
} as const;

export const colorRules = {
  maxAccentRatio: 0.12,
  useOrangeFor: ["CTA", "small focal highlight"],
  forbidden: ["neon rainbow", "large pure-red sale blocks", "cheap cyan circuit-board texture"],
} as const;

