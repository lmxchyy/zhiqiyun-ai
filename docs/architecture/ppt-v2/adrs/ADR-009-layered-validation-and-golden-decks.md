# ADR-009：使用四层验证与 Golden Deck 作为发布门

- 状态：Amended by ADR-011 for Phase 1 Golden 1 scope
- 日期：2026-08-15

## 背景

JSON 合法不代表几何、资产引用、OOXML 或视觉结果正确；单一测试层无法覆盖 renderer 风险。

## 决策

发布门按顺序执行：

1. JSON Schema：版本、字段、类型、枚举和基础范围。
2. 语义验证：ID 唯一、画布边界、资产引用、chart 维度。
3. OOXML 结构：slide、notes、chart、media、无外部关系。
4. OfficeCLI：文件 schema、issues、文本占位和逐页视觉检查。

Phase 0 `2.0` 的发布门包含 minimal、complete、CJK 三组。Phase 1 `2.1` 按批准范围只冻结并执行两页 Professional Business Golden 1；Golden 2～5 不在本阶段实现。

## 后果

- “测试通过”必须指明通过了哪一层。
- 无法执行 OfficeCLI/真实 viewer 时不能把 Phase 0 标为 READY。
- Golden fixture 是版本化输入，不提交由测试产生的临时 screenshot。
