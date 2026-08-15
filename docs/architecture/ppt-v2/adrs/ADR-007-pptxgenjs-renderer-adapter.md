# ADR-007：PPTX renderer 采用隔离的 PptxGenJS 4.0.1 adapter

- 状态：Accepted
- 日期：2026-08-15

## 背景

现有 Go exporter 直接拼接 OpenXML，覆盖范围和验证成本会随 chart、notes、媒体与可编辑性快速增长。Office COM 自动化不适合无桌面服务环境。

## 决策

V2 renderer adapter 使用 PptxGenJS 4.0.1，并固定在 Node.js 22+ 环境执行。契约验证使用 Ajv Draft 2020-12；OOXML spike 检查使用 JSZip。

PptxGenJS 只能出现在正式 `packages/ppt-v2` renderer adapter 内。业务服务依赖 renderer port 与 JSON contract，不依赖 PptxGenJS 类型。Phase 0 的 `tools/ppt-renderer-spike` 已由该正式 package 取代。

参考：

- <https://gitbrent.github.io/PptxGenJS/docs/introduction/>
- <https://gitbrent.github.io/PptxGenJS/docs/usage-saving/>
- <https://gitbrent.github.io/PptxGenJS/docs/speaker-notes/>

## 后果

- renderer 获得维护中的 text、shape、image、chart、notes 和 Node buffer 能力。
- 依赖升级必须重跑全部 Golden Deck 与 OfficeCLI gate。
- 不保留自定义 Go V2 renderer fallback；V1 exporter 也不会被 Phase 0 修改。
