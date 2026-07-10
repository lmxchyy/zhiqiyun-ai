# 知启云 AI 知识库智能体（RAG）交付审计

审计日期：2026-07-10

## 交付结论

知识库智能体已在现有 AI Agent 中完成端到端落地，覆盖“知识库创建 → 文档解析清洗 → Chunk → Embedding → pgvector/全文混合检索 → Rerank → RAG 回答 → 引用原文 → Token 计费 → 后台观测”的完整闭环。模块通过独立领域层、Provider 注册表和 Repository 接口接入，没有重写原有生图、视频、PPT、画布、认证或计费主流程。

生产默认路径使用 pgvector。Milvus、Qdrant、Weaviate 通过统一 `Backend`/Registry 插件契约接入；OCR、外部向量库和远程 Rerank 的真实服务联调需要部署方提供相应端点与凭据。

## 十五项需求对照

| # | 需求 | 状态 | 主要证据 |
|---|---|---|---|
| 1 | 知识库管理 | 完成 | 多知识库、企业/部门/个人/智能体类型、Logo、分类、标签、权限、状态；`knowledge.Service` 与知识库 REST API |
| 2 | 文档管理 | 完成 | PDF、DOC/DOCX、XLS/XLSX、PPT/PPTX、TXT、Markdown、HTML、CSV 上传解析；外部数据源使用 `source.Registry` 预留 |
| 3 | 文档解析 | 完成 | OCR 端点适配、文本提取、页眉页脚/目录/重复/乱码清洗、标题章节识别与元数据生成 |
| 4 | 文本切片 | 完成 | fixed、semantic、markdown、heading；Chunk Size、Overlap、Min/Max Token 与来源定位字段 |
| 5 | Embedding | 完成 | OpenAI、Gemini、Qwen、BGE、BCE、Jina、SiliconFlow、OneAPI、NewAPI；运行时 Profile 解析与切换 |
| 6 | 向量数据库 | 完成 | pgvector 默认并完成真实集成测试；Milvus/Qdrant/Weaviate 插件 Backend 契约 |
| 7 | 检索 | 完成 | Vector、Fulltext、Hybrid、Metadata Filter、TopK、Threshold、权重与 Rerank |
| 8 | 知识库问答 | 完成 | Query Rewrite、检索、重排、上下文拼接、LLM、引用；SSE 流式、停止、重试、原文定位 |
| 9 | 多租户 | 完成 | TenantID、UserID、OrganizationID 上下文；Repository 查询、ACL 与数据库约束隔离 |
| 10 | 权限 | 完成 | 管理员/企业管理员/成员/游客上下文；查看、上传、编辑、删除、分享、管理；显式 DENY 优先 |
| 11 | 后台管理 | 完成 | 知识库、文档、Chunk、Embedding、向量库、解析日志、检索日志、Token、问答与热门问题共 12 个资源面板 |
| 12 | 智能体集成 | 完成 | 单/多知识库绑定、优先级、权重、启停、检索 Profile 覆盖 |
| 13 | REST API | 完成 | 知识库、文档、解析、删除、检索、问答、引用、历史、配置和后台治理接口 |
| 14 | 小程序 | 完成 | uni-app 知识库聊天、历史、文件引用、流式/轮询兼容、停止与重新回答 |
| 15 | 未来扩展 | 已预留 | DocumentSource 注册表、通用 `source_type`/配置/游标/同步运行表；可按相同契约增加 Web、协同文档、对象存储、数据库、API、Webhook、RSS 与 Git 连接器 |

## 核心目录

```text
backend-go/internal/app/knowledge/          # 领域模型、服务、ACL、入库、检索、RAG
backend-go/internal/provider/               # parser/cleaner/chunker/embedding/vectorstore/rerank/ocr/source
backend-go/internal/repository/knowledge/   # Memory 与 PostgreSQL Repository
backend-go/internal/httpserver/knowledge_*  # REST、SSE、计费接入
database/migrations/033-036                 # 租户、文档管线、Agent RAG、权限加固
admin-vue/src/components/knowledge/         # PC Agent 工作台与主控后台
apps/user-uni/src/components/               # H5/小程序知识库聊天
```

## API 摘要

- `/api/v1/knowledge-bases`：知识库 CRUD。
- `/api/v1/knowledge/tags`、`/api/v1/knowledge/categories`：标签和分类。
- `/api/v1/knowledge-bases/:id/acl`：权限矩阵。
- `/api/v1/knowledge-bases/:id/documents:ingest`：上传、解析、清洗、切片和索引。
- `/api/v1/knowledge-documents/:id`、`/api/v1/knowledge-chunks`：文档与 Chunk。
- `/api/v1/knowledge-search`：向量、全文和 Hybrid Search。
- `/api/v1/knowledge-agents`、`/:id/knowledge-bindings`：智能体和知识库绑定。
- `/api/v1/knowledge-conversations/:id/runs`、`runs:stream`：普通和流式 RAG。
- `/api/v1/knowledge-runs/:id/{cancel,retry,events,citations}`：运行控制与引用。
- `/api/v1/admin/knowledge/*`：总览、治理资源和运行 Profile 配置。

Provider 配置输出会统一移除 API Key、Token、Secret、Password 和 Authorization，仅返回“已配置”标记，防止后台接口回显密钥。

## 验收证据

- Go 全量测试：`go test ./...` 通过。
- PostgreSQL + pgvector 真实纵向测试：`TestPostgresKnowledgeAgentVerticalFlow` 通过。
- HTTP 纵向闭环、后台权限、租户 ACL、计费、解析、清洗、Chunk、Embedding 测试通过。
- Admin Vue 生产构建通过。
- uni-app TypeScript、H5 和微信小程序构建通过。
- Docker Compose 已完成重建；PostgreSQL healthy、migration 退出码 0、应用 health 为 `ok`。
- 实际运行环境完成创建知识库、Markdown 入库、Hybrid Search、文档删除和向量清理验证。
- 浏览器完成登录 → 智能体中心 → 企业知识库智能体工作台验收；浏览器控制台无错误。
- 变更前已生成 PostgreSQL 备份；迁移保留现有业务数据。

## 配置与扩展约束

- `KNOWLEDGE_OCR_ENDPOINT`、`KNOWLEDGE_OCR_API_KEY`、`KNOWLEDGE_OCR_PROVIDER` 用于远程 OCR。
- Embedding、VectorStore、Ingestion、Retrieval Profile 可在主控后台维护，并在新任务中动态解析。
- 已产生 Chunk 的知识库更换 Embedding 或向量维度时必须执行重建索引流程，不能复用旧向量；这是数据一致性约束，不做原地混用。
- 外部 Provider 的网络连通、证书、容量和效果验收属于部署环境联调；默认 pgvector 路径已完成本地真实数据库验证。
