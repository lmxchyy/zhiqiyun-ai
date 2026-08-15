export function professionalBusinessDesignSystem() {
  return {
    fonts: {
      heading: "Microsoft YaHei",
      body: "Microsoft YaHei",
    },
    colors: {
      background: "FFFFFF",
      surface: "F3F6FA",
      primary: "1E2761",
      secondary: "CADCFC",
      accent: "F96167",
      text: "333333",
      muted: "6B7B8D",
      inverse: "FFFFFF",
    },
    textStyles: {
      coverTitle: {
        fontRole: "heading", fontSizePt: 44, colorToken: "inverse", bold: true,
        italic: false, align: "left", verticalAlign: "middle", marginPt: 0,
      },
      coverSubtitle: {
        fontRole: "body", fontSizePt: 22, colorToken: "secondary", bold: false,
        italic: false, align: "left", verticalAlign: "top", marginPt: 0,
      },
      coverFooter: {
        fontRole: "body", fontSizePt: 12, colorToken: "secondary", bold: false,
        italic: false, align: "left", verticalAlign: "middle", marginPt: 0,
      },
      contentTitle: {
        fontRole: "heading", fontSizePt: 36, colorToken: "primary", bold: true,
        italic: false, align: "left", verticalAlign: "middle", marginPt: 0,
      },
      contentBody: {
        fontRole: "body", fontSizePt: 22, colorToken: "text", bold: false,
        italic: false, align: "left", verticalAlign: "middle", marginPt: 8,
      },
    },
    shapeStyles: {
      coverAccent: {
        fillColorToken: "accent", lineColorToken: "none", lineWidthPt: 0, transparency: 0,
      },
      contentPanel: {
        fillColorToken: "surface", lineColorToken: "secondary", lineWidthPt: 1, transparency: 0,
      },
    },
  };
}
