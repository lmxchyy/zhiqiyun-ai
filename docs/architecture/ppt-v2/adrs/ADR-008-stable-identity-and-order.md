# ADR-008：稳定 ID 与数组顺序定义可复现语义

- 状态：Amended by ADR-011 for LayoutResult `zIndex`
- 日期：2026-08-15

## 背景

编辑、差异比较和幂等重试需要定位同一 deck、slide 和 element。renderer 内生成随机 ID 会破坏这些能力。

## 决策

调用方必须提供稳定的 `deckId`、slide ID、element ID 和 asset ID。slide 数组定义页序；`2.1` 由 LayoutResult 的显式 `zIndex` 定义 z-order，同值时保留 LayoutResult 数组顺序。所有 element ID 在整个 deck 内唯一。

renderer 不生成业务 ID，不读取当前时间，也不按内容重排元素；只执行 `zIndex` 权威顺序。`2.1` adapter 规范化包级时间戳与 ZIP entry date，因此 Golden 1 同一 RenderInput 必须 byte-for-byte 可重复。

## 后果

- UI patch、审计和渲染问题可以引用稳定对象。
- 重排必须表现为数组变更，不能依赖隐藏 z-index。
- byte-for-byte equality 是 `2.1` 的额外回归门，但不能替代 schema、结构和视觉 gate。
