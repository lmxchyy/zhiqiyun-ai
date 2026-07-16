# 先知AI 企业级 AI SaaS 开发手册 v1.1 - 小程序增强版

本版本重点补充「小程序端功能规划」。

## 小程序定位

Web 端作为完整工作台，小程序端作为移动轻量入口。

小程序重点承载：

- 移动端 AI 对话
- 企业知识库问答
- Agent 助手
- 作品查看
- 消息通知
- 客户线索收集
- 会员与 Token 查看
- 轻量移动审批
- 分享裂变
- 企业展示与获客

## 标准开发流程

需求 → PRD → 架构 → 数据库 → API → UI → 小程序页面 → 组件清单 → Codex开发 → Review → 测试 → 上线

## 飞书企业机器人

企业管理员可在「我的 → 企业中心 → 企业设置 → 企业连接 · 飞书」配置企业自建应用机器人。第一阶段支持飞书文本指令触发单轮 AI 生图、结果图片回传、作品中心保存、企业算力结算、消息/任务日志和重复消息幂等。配置、API、迁移、测试与生产部署见 [飞书企业 Connector 指南](docs/feishu-enterprise-connector.md)。

## 素材中心与页面装修

主控后台「运营中心」已提供素材中心、素材分类、页面装修及首页/创作/作品/我的四页配置。小程序只保留基础错误图，运营图片通过 `Asset Slot` 和 `/api/v1/app/pages/:pageCode` 获取，更换并发布后不需要重新发布小程序。

- 数据迁移：`database/migrations/037-media-page-decoration.sql`
- 演示素材初始化：在 `backend-go` 目录执行 `go run ./cmd/seed-media`，可重复运行并按 SHA-256 去重。
- 默认本地存储：`MEDIA_STORAGE_PROVIDER=local`，文件落在 `MEDIA_STORAGE_ROOT`，Docker 中持久化到 `app-data`。
- 完整运维、API、版权和联调说明见 [docs/media-center-page-decoration.md](docs/media-center-page-decoration.md)。
