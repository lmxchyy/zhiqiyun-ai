# ADR-002：契约按 major 版本严格拒绝，不做向后兼容

- 状态：Amended by ADR-011 for contract `2.1`
- 日期：2026-08-15

## 背景

renderer 输入一旦容忍未知字段、旧字段或隐式默认，生产输出就会随调用方和部署版本漂移。

## 决策

Phase 0 契约唯一合法版本为 `contractVersion: "2.0"`；Phase 1 Contract Gap 由 ADR-011 显式升级为唯一合法的 `2.1`。所有 schema object 使用 `additionalProperties: false`。未知版本、未知字段、缺失必填字段立即失败。

破坏性变更必须发布新的 major contract 和独立 schema；不得在 V2 内增加 migration、alias、旧字段 fallback 或“尽力渲染”。

## 后果

- 部署错误在生成 artifact 前暴露。
- 调用方必须与契约版本一起发布。
- 未来 V3 可以并行存在，但 V2 renderer 不负责读取 V3 或 V1。
