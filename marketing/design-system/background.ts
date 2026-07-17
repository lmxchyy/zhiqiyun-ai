export const posterBackground = {
  base: "#07142E",
  atmosphere: ["soft blue-violet volumetric glow", "subtle depth haze", "restrained 3D light field"],
  surfaces: ["premium glass panels", "fine translucent layers", "soft specular edges"],
  texture: "very subtle grain; clean enterprise SaaS finish",
} as const;

export const backgroundRules = {
  keepTextReadable: true,
  keepLogoAreaQuiet: true,
  forbidden: ["busy circuit board", "stock-photo handshake", "e-commerce stage", "explosive particles", "dense icon wallpaper"],
} as const;
