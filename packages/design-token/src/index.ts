export const designTokens = {
  color: {
    grayBlue: "#D2D4D6",
    primary: "#7D8DF6",
    primaryDark: "#5A4DB2",
    accent: "#FF771B",
    bgPage: "#F7F8FC",
    bgCard: "#FFFFFF",
    textPrimary: "#111827",
    textSecondary: "#6B7280",
    textMuted: "#9CA3AF",
    border: "#E5E7EB"
  },
  radius: {
    sm: "8px",
    md: "12px",
    lg: "16px",
    xl: "20px"
  }
} as const;

export const cssVars = {
  "--color-gray-blue": designTokens.color.grayBlue,
  "--color-primary": designTokens.color.primary,
  "--color-primary-dark": designTokens.color.primaryDark,
  "--color-accent": designTokens.color.accent,
  "--color-bg-page": designTokens.color.bgPage,
  "--color-bg-card": designTokens.color.bgCard,
  "--color-text-primary": designTokens.color.textPrimary,
  "--color-text-secondary": designTokens.color.textSecondary,
  "--color-text-muted": designTokens.color.textMuted,
  "--color-border": designTokens.color.border,
  "--radius-sm": designTokens.radius.sm,
  "--radius-md": designTokens.radius.md,
  "--radius-lg": designTokens.radius.lg,
  "--radius-xl": designTokens.radius.xl,
  "--wot-color-theme": designTokens.color.primary,
  "--wot-button-primary-bg-color": designTokens.color.primary,
  "--wot-button-primary-border-color": designTokens.color.primary,
  "--wot-button-primary-color": "#FFFFFF",
  "--wot-tabs-nav-color": designTokens.color.primary
} as const;

export const wotThemeVars = {
  colorTheme: designTokens.color.primary,
  buttonPrimaryBgColor: designTokens.color.primary,
  buttonPrimaryBorderColor: designTokens.color.primary,
  tabsNavColor: designTokens.color.primary,
  inputBorderColor: designTokens.color.border,
  textareaBorderColor: designTokens.color.border,
  searchBg: designTokens.color.bgCard
} as const;
