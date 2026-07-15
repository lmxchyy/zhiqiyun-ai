import type { PromotionProfile, PromotionTemplate } from "./types";

type CanvasContext = Record<string, (...args: any[]) => any>;

export interface PromotionPosterRenderInput {
  context: CanvasContext;
  width: number;
  height: number;
  template: PromotionTemplate;
  profile: PromotionProfile;
  qrPath: string;
  logoPath: string;
}

function setFill(ctx: CanvasContext, color: string) { ctx.setFillStyle(color); }
function setStroke(ctx: CanvasContext, color: string) { ctx.setStrokeStyle(color); }

function roundedRect(ctx: CanvasContext, x: number, y: number, width: number, height: number, radius: number, fill: string, stroke?: string) {
  const r = Math.min(radius, width / 2, height / 2);
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.lineTo(x + width - r, y);
  ctx.arcTo(x + width, y, x + width, y + r, r);
  ctx.lineTo(x + width, y + height - r);
  ctx.arcTo(x + width, y + height, x + width - r, y + height, r);
  ctx.lineTo(x + r, y + height);
  ctx.arcTo(x, y + height, x, y + height - r, r);
  ctx.lineTo(x, y + r);
  ctx.arcTo(x, y, x + r, y, r);
  ctx.closePath();
  setFill(ctx, fill);
  ctx.fill();
  if (stroke) { setStroke(ctx, stroke); ctx.setLineWidth(2); ctx.stroke(); }
}

function text(ctx: CanvasContext, value: string, x: number, y: number, size: number, color: string, weight = "normal", align: "left" | "center" | "right" = "left") {
  setFill(ctx, color);
  ctx.setFontSize(size);
  ctx.setTextAlign(align);
  ctx.setTextBaseline("top");
  (ctx as unknown as { font: string }).font = `${weight} ${size}px sans-serif`;
  ctx.fillText(value, x, y);
}

function wrapText(ctx: CanvasContext, value: string, x: number, y: number, maxWidth: number, lineHeight: number, size: number, color: string, maxLines = 3, weight = "normal") {
  ctx.setFontSize(size);
  const chars = [...value];
  const lines: string[] = [];
  let current = "";
  chars.forEach(char => {
    const next = current + char;
    if (ctx.measureText(next).width > maxWidth && current) { lines.push(current); current = char; }
    else current = next;
  });
  if (current) lines.push(current);
  lines.slice(0, maxLines).forEach((line, index) => {
    const finalLine = index === maxLines - 1 && lines.length > maxLines ? `${line.slice(0, -1)}…` : line;
    text(ctx, finalLine, x, y + index * lineHeight, size, color, weight);
  });
}

function drawDecor(ctx: CanvasContext, template: PromotionTemplate, width: number) {
  const primary = template.primaryColor;
  switch (template.layout) {
    case "feature-grid":
      [0, 1, 2].forEach(index => roundedRect(ctx, 110 + index * 288, 650, 244, 190, 28, "#FFFFFF", `${primary}33`));
      break;
    case "reward-card":
      roundedRect(ctx, 90, 540, width - 180, 320, 44, "#FFFFFF", `${primary}44`);
      text(ctx, "AI", width / 2, 610, 112, `${primary}22`, "bold", "center");
      break;
    case "enterprise-split":
      roundedRect(ctx, 70, 480, width - 140, 390, 36, template.secondaryColor);
      setFill(ctx, `${template.primaryColor}66`); ctx.fillRect(width * 0.62, 480, width * 0.25, 390);
      break;
    case "scene-stack":
      [0, 1, 2].forEach(index => roundedRect(ctx, 120 + index * 36, 590 + index * 72, width - 240 - index * 72, 126, 30, index === 2 ? "#FFFFFF" : `${primary}${index === 0 ? "24" : "36"}`));
      break;
    case "industry-panel":
      roundedRect(ctx, 80, 520, width - 160, 330, 34, "#FFFFFF");
      [0, 1, 2, 3].forEach(index => { setFill(ctx, `${primary}${30 + index * 12}`); ctx.fillRect(150 + index * 190, 735 - index * 42, 118, 70 + index * 42); });
      break;
    case "case-quote":
      text(ctx, "“", 100, 470, 210, `${primary}44`, "bold");
      roundedRect(ctx, 150, 610, width - 300, 220, 28, "#FFFFFF");
      break;
    case "campaign-countdown":
      ["03", "12", "48"].forEach((value, index) => { roundedRect(ctx, 170 + index * 250, 610, 190, 170, 32, "#FFFFFF"); text(ctx, value, 265 + index * 250, 650, 64, template.secondaryColor, "bold", "center"); });
      break;
    case "partner-steps":
      ["01", "02", "03"].forEach((value, index) => { roundedRect(ctx, 130, 570 + index * 92, 110, 66, 22, primary); text(ctx, value, 185, 586 + index * 92, 28, "#FFFFFF", "bold", "center"); setFill(ctx, `${primary}55`); ctx.fillRect(270, 599 + index * 92, width - 430, 4); });
      break;
    case "festival-frame":
      setStroke(ctx, `${primary}88`); ctx.setLineWidth(8); ctx.strokeRect(56, 56, width - 112, 910);
      [80, width - 80].forEach(x => { setFill(ctx, primary); ctx.beginPath(); ctx.arc(x, 100, 22, 0, Math.PI * 2); ctx.fill(); });
      break;
    default:
      setFill(ctx, `${primary}22`); ctx.beginPath(); ctx.arc(width - 130, 350, 270, 0, Math.PI * 2); ctx.fill();
      setFill(ctx, `${primary}18`); ctx.beginPath(); ctx.arc(160, 720, 210, 0, Math.PI * 2); ctx.fill();
  }
}

export function renderPromotionPoster(input: PromotionPosterRenderInput) {
  const { context: ctx, width, height, template, profile, qrPath, logoPath } = input;
  const sx = width / 1080;
  const sy = height / 1440;
  ctx.save();
  ctx.scale(sx, sy);
  setFill(ctx, template.background); ctx.fillRect(0, 0, 1080, 1440);
  drawDecor(ctx, template, 1080);

  try { ctx.drawImage(logoPath, 84, 72, 96, 96); } catch { roundedRect(ctx, 84, 72, 96, 96, 26, template.primaryColor); text(ctx, "知", 132, 95, 42, "#FFFFFF", "bold", "center"); }
  text(ctx, profile.companyName || "知启云AI", 198, 82, 38, template.secondaryColor, "bold");
  text(ctx, "企业级 AI 创作平台", 198, 132, 24, `${template.secondaryColor}AA`);
  roundedRect(ctx, 82, 218, 214, 58, 29, template.primaryColor);
  text(ctx, template.badge, 189, 232, 26, "#FFFFFF", "bold", "center");
  wrapText(ctx, template.title, 82, 324, 850, 82, 66, template.secondaryColor, 3, "bold");
  wrapText(ctx, template.subtitle, 84, 520, 780, 48, 30, `${template.secondaryColor}CC`, 2);

  if (!["enterprise-split", "feature-grid"].includes(template.layout)) {
    template.featureItems.forEach((item, index) => {
      roundedRect(ctx, 84 + index * 292, 820, 256, 70, 24, "#FFFFFF", `${template.primaryColor}33`);
      setFill(ctx, template.primaryColor); ctx.beginPath(); ctx.arc(118 + index * 292, 855, 9, 0, Math.PI * 2); ctx.fill();
      text(ctx, item, 140 + index * 292, 838, 24, template.secondaryColor, "bold");
    });
  } else if (template.layout === "feature-grid") {
    template.featureItems.forEach((item, index) => { text(ctx, `0${index + 1}`, 232 + index * 288, 684, 44, template.primaryColor, "bold", "center"); text(ctx, item, 232 + index * 288, 760, 24, template.secondaryColor, "bold", "center"); });
  } else {
    template.featureItems.forEach((item, index) => text(ctx, `✓ ${item}`, 140, 570 + index * 78, 30, "#FFFFFF", "bold"));
  }

  roundedRect(ctx, 54, 1024, 972, 350, 42, "#FFFFFF", `${template.primaryColor}22`);
  text(ctx, "由", 96, 1110, 24, "#8A94A6");
  text(ctx, profile.name || "知启云用户", 96, 1150, 36, template.secondaryColor, "bold");
  text(ctx, profile.roleLabel || "普通用户", 96, 1202, 24, template.primaryColor, "bold");
  text(ctx, `邀请码  ${profile.inviteCode}`, 96, 1260, 27, template.secondaryColor, "bold");
  roundedRect(ctx, 684, 1088, 284, 248, 30, "#FFFFFF", `${template.primaryColor}44`);
  try { ctx.drawImage(qrPath, 708, 1112, 236, 196); } catch { text(ctx, "小程序码加载失败", 826, 1200, 22, "#A0A7B5", "normal", "center"); }
  text(ctx, "微信扫码 立即体验", 826, 1312, 20, template.secondaryColor, "bold", "center");
  ctx.restore();
}
