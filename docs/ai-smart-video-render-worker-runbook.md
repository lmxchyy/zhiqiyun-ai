# AI 智能成片渲染 Worker（已退役）

> **已退役。** Smoke 直入队与零积分路径不再使用。  
> 请改用：[AI 自动混剪运维手册](./runbooks/ai-auto-montage.md)（相对路径：`docs/runbooks/ai-auto-montage.md`）。

创建/导出必须经 `ExportService`（事务 Outbox + 个人积分）。禁止对 Redis render 队列手工 `LPUSH` 绕过计费。
