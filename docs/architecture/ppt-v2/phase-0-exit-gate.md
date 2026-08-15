# PPT Generation V2 Phase 0 Exit Gate

> Historical snapshot: this report records the Phase 0 `2.0` spike gate. Phase 1 contract `2.1`, ADR-011, the formal `packages/ppt-v2` renderer, and Golden 1 supersede the spike artifacts; this report is not a current build or compatibility specification.

- 日期：2026-08-15
- Worktree：`E:\code\work\ppt-v2-phase0-contract`
- 分支：`codex/ppt-v2-phase0-contract`
- 基线 HEAD：`6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- 状态：READY

## 范围

本阶段只交付 ADR、renderer-facing Deck IR、Golden Deck、renderer spike 和 Exit Gate。没有修改现有 PPT V1 API、Go 服务、数据库、任务状态、计费、作品中心、Connector、网页或小程序，也没有开始 Phase 1。

前两阶段审计未重新执行；现有 `docs/workflows/WORKFLOW-ppt-generation.md` 被当作 V1 现状输入使用。

## 交付物

| 交付物 | 结果 |
|---|---|
| ADR-001 ～ ADR-010 | 10/10 Accepted，位于 `docs/architecture/ppt-v2/adrs/` |
| JSON Schema | `contracts/ppt-v2/deck.schema.json`，Draft 2020-12，严格 `2.0`，拒绝未知字段 |
| Contract runtime | Ajv schema validation + ID/geometry/asset/chart semantic validation |
| Golden Deck | minimal、complete、CJK 三套 JSON fixture + SHA-256 SVG asset |
| Renderer spike | PptxGenJS 4.0.1 adapter，原生 text/shape/chart、嵌入 image、speaker notes、Node buffer |
| OOXML guard | 修正并回归保护 PptxGenJS 的 notes master 顺序与 orphan chart axis reference |
| CLI | JSON fixture -> `.pptx`，只允许目录型 `asset://golden/` resolver |

## Exit Gates

| Gate | 验证 | 结果 |
|---|---|---|
| Contract structure | JSON Schema `additionalProperties: false`、版本 const、枚举和范围 | PASS |
| Contract semantics | ID 唯一、960×540 pt 边界、资产引用、chart 数据维度 | PASS |
| Fail closed | 旧版本/未知字段/越界/缺资产/digest 不符均拒绝 | PASS |
| Golden fixtures | 3/3 schema + semantic validation | PASS |
| Renderer unit/integration | `npm.cmd run test:ppt-v2` | PASS：8 tests，0 failures |
| OOXML package | slide/notes/chart/media 部件、Unicode、无 external relationship | PASS |
| Dependency lock | `npm install --package-lock-only --offline --ignore-scripts` | PASS：70 packages，0 vulnerabilities |
| OfficeCLI schema | 3/3 deck，OfficeCLI 1.0.144 `validate` | PASS：0 errors |
| OfficeCLI issues | 3/3 deck，`view issues` | PASS：0 issues |
| Placeholder scan | `view text` + source placeholder scan | PASS：0 placeholders |
| Visual audit | 8/8 slides screenshot 检查 | PASS：无越界、裁切、重叠、低对比、拉伸或顺序错误 |

## Renderer Spike 结果

| Golden Deck | Slides | Artifact bytes | 关键覆盖 |
|---|---:|---:|---|
| minimal | 2 | 17,474 | 基础 text、shape、notes、显式 theme |
| complete | 4 | 30,275 | SVG asset、SHA-256、native chart、notes、closing |
| CJK | 2 | 17,836 | 中文、全角标点、Unicode 和换行 |

PptxGenJS 4.0.1 原始输出触发了两个 OfficeCLI schema 错误：

1. `notesMasterIdLst` 位于 `sldIdLst` 之后。
2. `barChart` 多出一个没有对应 axis definition 的 `axId`。

adapter 现在只在 OOXML 输出边界做确定性规范化：将 notes master 移到合法序列位置，并移除没有 axis definition 的 chart reference。测试直接断言这两个结构，OfficeCLI 复验为 0 errors。没有 fork 第三方库，也没有加入备用 renderer。

## 防回归与隔离

- 当前改动未触及 `docs/regression/protected-surfaces.md` 的 W1–W4、M1–M7 任何实现锚点。
- 未修改 `/points/account`、首页 limit、视频模型/计费/下载、登录、游客入口、自由 P 图或视频灵感下架逻辑。
- 未执行 merge、rebase、cherry-pick、fetch、push 或 main 写操作。
- 主工作区两个既有未跟踪治理报告继续作为授权基线保留，不属于本次变更。
- 最终复核时另观察到主工作区新增 `docs/git-branch-governance-phase2a-cleanup-2026-08-15.md`（SHA-256 `c6fa9582765a828c739710c0ba0313ac60025bdc7dad47f616111ec6c7b709b6`）。该文件不在 Phase 0 worktree，按用户约束只记录，未读取业务内容、未修改、未移动、未 add/commit。

## Exit Decision

**Phase 0 Status: READY**

Phase 1 最小 vertical slice 应只覆盖：固定的两页内容计划（cover + content）经最小布局编译器生成 V2 Deck IR，调用当前 renderer 生成一个私有 PPTX artifact，并把 artifact 关联到一个既有 PPT 任务。暂不加入图片生成、chart、编辑器、PDF、Connector 或新计费规则。
