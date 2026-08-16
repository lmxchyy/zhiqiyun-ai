# PPT V2 Phase 3 Slice D 实施报告

## 范围

本 Slice 在 Slice C Preview Workspace 上增加受控编辑命令、Deck Revision、局部 Deck JSON 更新、Preview 刷新和 Undo。自然语言入口通过现有 OpenAI-compatible Chat Provider 的 `EditPlanningPort` 转换为结构化 `EditCommand`；模型不能决定 tenant、owner、revision 或 artifact identity。

## 已实现

* `EditCommand` 包含 commandId、commandType、deckId、baseRevision、targetSlideId、targetElementId、structured payload 和 userIntentSummary。
* 支持 `UPDATE_TEXT`、`REGENERATE_SLIDE`、`CHANGE_LAYOUT`、`REPLACE_IMAGE`、`MOVE_SLIDE`、`ADD_SLIDE`、`DELETE_SLIDE` 的契约入口；当前生产安全执行路径覆盖文本、布局、已有私有资产替换、移动、增加和删除页面。
* stale base revision、未知 slide/element、非法 payload、页数边界和 tenant/owner scope 均 fail closed。
* 每次成功编辑创建 parent-linked immutable `DeckRevisionSnapshot`；旧 compilation 保留在现有 `agent_plans.deck_state` JSON 中，未新增 migration。
* 相同 commandId replay 不新增 revision、不再次渲染、不重复存储 artifact。
* Undo 将 current revision pointer 恢复到 parent revision，不删除历史 revision，也不反向调用模型。
* 编辑后的 compilation revision、render input、layout projection 和 PPTX bytes/file identity 一起更新；Preview 使用新的 revision，下载接口使用当前 artifact。
* 新增 `/api/v1/ppt/agent/jobs/:jobId/edit` 和 `/api/v1/ppt/agent/jobs/:jobId/undo`，仍属于既有 `/api/v1/ppt` boundary。
* Preview Workspace 提供修改输入框和 Undo 操作；用户不需要直接理解 EditCommand。

## Durable 与安全边界

Revision 快照写入既有 PostgreSQL `xz_ppt_v2_agent_plans.deck_state`，并同步 GenerationJob/DeckJob revision。PostgreSQL store 使用事务和现有 tenant/owner 查询边界。已有 lease/fencing 仍保护 GenerationJob worker；编辑 API 不接受客户端 tenant、owner、revision number 作为权威值，planner 输出的 deckId/baseRevision 会由应用层覆盖。

## 测试

已通过：

* EditCommand stale revision、unknown target、immutable parent snapshot；
* duplicate edit idempotent replay；
* move/delete page count 与 stable slide identity；
* Undo parent pointer 恢复；
* Slice C HTTP preview regression；
* Admin typecheck 与 Preview store/component tests（10 tests）；
* `git diff --check`。

Slice B/C 之前已通过的 Golden 1、Golden 2、OfficeCLI、Playwright Preview、Node regression 和 PostgreSQL CI 证据未被本轮代码修改；新的 Slice D 代码尚未完成独立 GitHub Actions run。

## 已知限制 / blocker

1. Edit planning 已接入 provider abstraction，但尚未建立独立的 Edit GenerationJob workflow stage（`EDIT_ACCEPTED` 等）和后台 worker；当前 edit API 在请求内完成 command validation、render 和 persistence。
2. `REGENERATE_SLIDE` 尚未接入完整 SlideContent provider 局部重生成；图片替换只接受已成功持久化的 asset identity，尚未接入新的图片 provider durable worker/计费 identity。
3. 新增页面使用受控结构化 payload 并复用现有布局投影，尚未建立完整 SlideObjective → Research evidence → SlideIR 的新增页规划链路。
4. 本轮没有新增 Golden 3、Slice D PostgreSQL integration gate、edit restart/fencing gate 或 artifact relation effectively-once gate。

## Exit Gate

由于上述 durable worker、局部内容/图片 provider、Golden 3 和 Slice D PostgreSQL gate 尚未完成，当前不能声明 Slice D READY。

`SLICE D STATUS: NOT READY`

本轮不进入 Phase 4。
