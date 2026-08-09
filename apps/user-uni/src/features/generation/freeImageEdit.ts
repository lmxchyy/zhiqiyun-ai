export interface FreeImageEditPreset {
  id: string;
  title: string;
  prompt: string;
}

export const freeImageEditPresets: readonly FreeImageEditPreset[] = [
  {
    id: "magazine-cover",
    title: "杂志封面",
    prompt: "将照片转换为时尚杂志封面，保留人物身份特征，使用高级摄影棚光线和简洁排版。",
  },
  {
    id: "remove-passersby",
    title: "去除路人",
    prompt: "去除主体人物之外的路人，保持主体人物、背景结构和画面光影自然。",
  },
  {
    id: "refine-makeup",
    title: "精致补妆",
    prompt: "为照片中的人物补充自然精致的妆容，保留原有五官与肤质，避免过度磨皮。",
  },
  {
    id: "restore-photo",
    title: "老照片修复",
    prompt: "修复照片的划痕、褪色和模糊区域，提升清晰度，同时保留原始人物特征和年代质感。",
  },
  {
    id: "product-retouch",
    title: "商品图精修",
    prompt: "清理商品周围杂物和瑕疵，优化光影与质感，保持商品结构、颜色和标识准确。",
  },
  {
    id: "change-hairstyle",
    title: "更换发型",
    prompt: "将人物发型改为自然的齐肩短发，保持脸型、五官、姿态和环境不变。",
  },
];

export function selectedFreeImageEditPresetId(prompt: string): string {
  const normalizedPrompt = prompt.trim();
  return freeImageEditPresets.find((preset) => preset.prompt === normalizedPrompt)?.id ?? "";
}

export function freeImageEditValidationMessage(imagePath: string, prompt: string): string {
  if (!imagePath.trim()) {
    return "请先添加需要编辑的图片";
  }
  if (!prompt.trim()) {
    return "请先选择图片效果或填写修改要求";
  }
  return "";
}
