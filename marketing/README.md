# 知启云AI Marketing Poster System

本目录是知启云AI品牌宣传物料的唯一任务源。它管理任务、设计规范和发布记录，不把生成结果写死在业务代码中，也不把大体积 PNG/视频提交到 Git。

## 指令路由

当收到 `开始生成 Poster-001` 这类命令时：

1. 只定位并读取对应编号的 Markdown；编号与目录映射见下表。
2. 读取 `design-system/` 中全部共享规范和 `storage-policy.md`。
3. 加载 `marketing/assets/zhiqiyun-ai-logo-v2-transparent.png`，不得重绘 Logo。
4. 使用 `image2` 只生成一张 1080 × 1920 PNG，不得拼图、合集、网格或一次生成多个版本。
5. 校验成图后，以任务中的 `output.filename` 作为原始文件名，上传公共存储桶。
6. 把 `status` 改为 `completed`，并补充 `file_id`、`object_key`、`public_url`、`generated_at`；随后立即停止，等待下一张命令。

| 编号 | 任务目录 |
| --- | --- |
| 001–010 | `posters/01-brand/` |
| 011–030 | `posters/02-feature/` |
| 031–040 | `posters/03-platform/` |
| 041–060 | `posters/04-industry/` |
| 061–080 | `posters/05-business/` |

## 固定品牌信息

- 品牌：知启云AI
- 定位：企业AI生产力平台
- 副标题：企业级AI创作与智能体平台
- Slogan：让AI成为企业生产力
- CTA：立即体验
- 禁止出现：二维码、网址、邮箱、电话、电商促销风、廉价科技风
- 适用渠道：官网、公众号、朋友圈、视频号、小红书、抖音封面、展会背景和宣传物料

## 目录职责

- `posters/`：80 个独立内容任务；每个文件只定义主题、文案与专属画面方向。
- `design-system/`：所有海报共享的颜色、字体、间距、布局、按钮、标题、背景与 Logo 规则。
- `storage-policy.md`：海报和视频发布到公共存储桶时的唯一规则。
- `MASTER-TASK.md`：给 Codex 的总任务与不可违反的执行门槛。
