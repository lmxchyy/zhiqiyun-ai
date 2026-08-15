# ADR-005：优先输出 PowerPoint 原生可编辑元素

- 状态：Accepted
- 日期：2026-08-15

## 背景

把整页或图表栅格化虽然容易保证截图一致，但会失去文本搜索、无障碍信息和用户编辑能力。

## 决策

V2 基础元素集合固定为 `text`、`shape`、`image`、`chart`：

- 文本保存为原生 text box。
- 矩形、圆角矩形和椭圆保存为原生 shape。
- 柱状、条形、折线和环形图保存为原生 chart 与 embedded workbook data。
- 图片作为嵌入媒体保存，并必须提供 alt text。

元素数组顺序就是 z-order。Phase 0 不支持任意 HTML、整页 SVG、视频、动画、group、table 或自由路径。

## 后果

- Golden Deck 可以检查 OOXML 中的 chart、notes 和 media 部件。
- 新元素必须通过契约、renderer 和 Golden Deck 三处同时增加。
- 复杂视觉不得以整页截图绕过契约。
