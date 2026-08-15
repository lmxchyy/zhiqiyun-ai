# ADR-003：Phase 0 固定 16:9 画布并以 point 表示几何

- 状态：Accepted
- 日期：2026-08-15

## 背景

CSS pixel、inch、EMU 和不同预览缩放混用会造成 editor、renderer 与 PowerPoint 的位置偏差。

## 决策

V2 Phase 0 只接受 `960 × 540 pt` 的 16:9 画布。位置和尺寸使用 point；renderer 在边界处按 `72 pt = 1 inch` 转换为 PptxGenJS 坐标。

JSON Schema 固定 unit、width 和 height；语义验证额外检查 `x + width` 与 `y + height` 不越界。

## 后果

- 预览可以线性缩放，不需要猜测单位。
- renderer 不做自动缩放、自动重排或裁边。
- 新画幅需要新的契约决策，不能偷偷放宽当前 schema。
