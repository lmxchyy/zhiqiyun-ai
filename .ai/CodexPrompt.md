# Codex 主提示（防回归）

请严格按文档开发。默认同时遵守仓库根目录 `AGENTS.md`，以及：

- `docs/regression/protected-surfaces.md`（不能丢的功能清单）
- 本文件下列行为约束

## 行为约束

1. **只做当前事**：一次只实现用户指定的模块/缺陷；不要顺手重构、扩 scope、清债。
2. **别整页重写**：禁止为了小改动重写整页 / 整组件 / 整 store；优先最小 diff。
3. **先读保护面**：动手前打开 `docs/regression/protected-surfaces.md`，标出本次可能碰到的条目。
4. **做完要确认旧功能还在**：改完后对照 protected-surfaces 相关项逐条确认；能跑的回归测试必须跑绿。
5. **做完勾清单**：在回复里明确勾选本次已核对的 protected-surfaces 条目（用 `[x]`），未覆盖的写 `[ ]` 并说明原因。
6. **不改无关代码**：不要修改未点名的包、页面、配置、生产热修。
7. **发布不绕门禁**：生产 dirty tree 不得强行 deploy；只发 Git 已推送提交。

## 改页面时

优先引用：`.ai/前端工人CodexPrompt.md`

## 发版时

优先引用：`.ai/发版经理CodexPrompt.md`
