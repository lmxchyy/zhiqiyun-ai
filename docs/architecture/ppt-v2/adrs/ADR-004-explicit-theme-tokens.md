# ADR-004：主题通过有限 token 显式解析

- 状态：Accepted
- 日期：2026-08-15

## 背景

依赖 Office master 默认字体、默认颜色或 renderer 内置主题会让跨平台结果不稳定，也让 Golden Deck 难以解释。

## 决策

每个 deck 必须声明两个字体角色 `heading`、`body`，以及八个颜色 token：`background`、`surface`、`primary`、`secondary`、`accent`、`text`、`muted`、`inverse`。

每个文本、形状、chart 必须显式引用 token 和必要样式。renderer 只解析 token，不自行挑选字体、颜色或字号。

## 后果

- 主题替换仍可集中完成，同时保持元素意图显式。
- 当前契约不支持任意散落的十六进制颜色。
- 需要更多语义颜色时必须先更新契约，而不是在 renderer 加隐藏默认。
