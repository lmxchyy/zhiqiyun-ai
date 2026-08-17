# ADR-001：以 Deck IR 固定生成链路边界

- 状态：Superseded by ADR-011 for contract `2.1`
- 日期：2026-08-15

## 背景

现有 V1 把提示词、大纲、任务状态、逐页图片和导出字段混在一组业务 DTO 中。renderer 因而需要推断布局，业务层也无法稳定复现某次输出。

## 决策

V2 固定采用四段链路：`Content Plan -> Layout Compiler -> Deck IR -> Renderer Artifact`。

Deck IR 是唯一 renderer 输入；它包含最终画布、主题 token、资产清单、slide 顺序、元素几何、显式样式和 speaker notes。大模型不得直接调用 renderer，renderer 不读取任务、用户、计费、Connector 或作品中心类型。

## 后果

- 内容规划可以独立演进，不污染 renderer。
- 同一 Deck IR 可重复渲染和做 Golden Deck 回归。
- Phase 1 必须先实现最小布局编译器；不得把 V1 DTO 直接塞进 V2 renderer。
- 本 ADR 不要求替换 V1；V2 是新边界，不提供 V1 兼容适配器。
