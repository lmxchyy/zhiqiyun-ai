export const primaryCTA = {
  label: "立即体验",
  width: 300,
  height: 88,
  radius: 44,
  background: "linear-gradient(135deg, #4F6BFF 0%, #725BFF 100%)",
  textColor: "#FFFFFF",
  border: "1px solid rgba(255,255,255,.28)",
  shadow: "0 22px 60px rgba(79,107,255,.34)",
} as const;

export const buttonRules = {
  exactLabelOnly: true,
  maxButtons: 1,
  forbidden: ["price badge", "countdown", "coupon", "QR code"],
} as const;
