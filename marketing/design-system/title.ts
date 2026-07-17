export const titleTreatment = {
  color: "#FFFFFF",
  maxWidth: 900,
  emphasisGradient: "linear-gradient(90deg, #FFFFFF 0%, #C9D4F2 48%, #45C8FF 100%)",
  shadow: "0 12px 48px rgba(0,0,0,.22)",
} as const;

export const titleRules = {
  useTaskTitleVerbatim: true,
  maxLines: 2,
  noExtraHeadline: true,
  noEnglishDecorationUnlessTaskRequiresIt: true,
} as const;
