# ADR-010：renderer fail closed，Phase 0 不进入业务接线

- 状态：Accepted
- 日期：2026-08-15

## 背景

部分成功的 PPTX、隐式降级和提前接入任务/计费会让架构 spike 变成不可控的生产路径。

## 决策

schema、语义、资产或 renderer 任一错误都终止并返回错误；不得输出部分 deck，不得改用占位图、旧字段或 V1 exporter。

Phase 0 只交付 ADR、Deck IR、Golden Deck、renderer spike 与 Exit Gate。不得接入 API、数据库、队列、计费、作品中心、Connector、网页或小程序，也不得宣称 PDF 已实现。

## 后果

- Phase 0 证据可以独立审查。
- Phase 1 只能在 READY 后实现一个最小 vertical slice。
- 生产状态机、幂等链、计费里程碑和交付失败语义仍需在进入对应阶段前单独落 ADR。
