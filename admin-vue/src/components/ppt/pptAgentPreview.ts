import type { PptPreviewLayoutElement } from "../../types/pptAgent";

export interface PreviewCanvasSize {
  width: number;
  height: number;
}

export function previewCanvasTransform(canvas: PreviewCanvasSize, viewportWidth: number) {
  const safeWidth = Number.isFinite(viewportWidth) && viewportWidth > 0 ? viewportWidth : canvas.width;
  const scale = Math.min(1, safeWidth / canvas.width);
  return {
    scale,
    width: canvas.width * scale,
    height: canvas.height * scale,
    transform: `scale(${scale})`
  };
}

export function authoritativeElementStyle(element: PptPreviewLayoutElement) {
  return {
    left: `${element.x}px`,
    top: `${element.y}px`,
    width: `${element.width}px`,
    height: `${element.height}px`,
    zIndex: String(element.zIndex)
  };
}
