# 知启云 AI 知识库智能体（RAG）开发计划

## 目标

在不破坏现有生图、视频、PPT、画布、计费和渠道业务的前提下，为 AI 智能体增加企业级知识库能力，形成“创建知识库 → 文档入库 → 解析切片 → 向量索引 → Agent 检索问答 → 引用原文 → 计费与观测”的完整闭环。

## 实施原则

- 继续使用 Go + Gin、PostgreSQL、Redis、RabbitMQ、MinIO、Vue 3、Element Plus、uni-app。
- 新知识库业务放入独立领域模块，不继续扩大现有 `store.go`、`postgres_store.go` 和 `App.vue`。
- 物理运行表使用当前主存储一致的 `xz_*` + text ID；旧 UUID `knowledge_bases`、`knowledge_documents` 只做兼容迁移来源。
- 所有领域数据必须携带 TenantID；TenantID 由服务端认证上下文产生，客户端不能越权指定。
- Parser、Chunker、Embedding、VectorStore、Rerank、DocumentSource 全部通过接口与注册表扩展。
- Embedding 或向量库切换必须重建候选索引并原子激活，不能直接复用旧向量。
- 每个阶段都以测试、构建或运行时证据验收，不以“代码已写”作为完成依据。

## 阶段与验收门槛

### 阶段 A：数据库与领域基础

交付：

- 租户、组织、知识库、ACL、文档版本、Chunk、索引、Agent 绑定、会话、RAG Run 与引用表。
- pgvector 默认基础设施与幂等迁移。
- Go 领域对象、Repository/Parser/Embedding/VectorStore 等端口。
- PostgreSQL Repository 与开发环境内存 Repository。

验收：迁移可重复执行；跨租户关联被数据库约束或 Repository 校验拒绝；领域单元测试通过。

### 阶段 B：AI Agent RAG 纵向闭环

交付：

- 知识库和文本/Markdown 文档 CRUD。
- 固定长度与 Markdown 切片。
- OpenAI 兼容 Embedding、开发环境确定性 Embedding、pgvector 检索。
- 向量、全文与 Hybrid Search。
- AI Agent 多知识库绑定、优先级和权重。
- 会话、Run、停止、重试、引用来源和流式事件。

验收：自动化测试证明一个用户可创建知识库、入库文档、绑定 Agent、提问并得到带文档和定位信息的引用；无权限租户无法检索该内容。

### 阶段 C：PC Agent Center

交付：将现有“企业知识库智能体”的模拟回复替换为真实会话、流式回答、历史、停止、重试、来源侧栏和原文预览。

验收：PC 构建通过；浏览器端完成真实问答；断流恢复和引用跳转可用；其他智能体入口不受影响。

### 阶段 D：主控后台

交付：知识库总览、知识库、文档与解析、Chunk、索引、Embedding、向量库、检索策略、日志、Token 和问答统计。

验收：管理员可完成配置、测试连接、重建并激活索引、查看失败步骤和检索命中；密钥不回显。

### 阶段 E：H5 / 小程序 / App

交付：知识助手首页、聊天、历史、引用原文、文件引用、流式回答、停止和重新回答，统一通过 `business-sdk` 与 `platform-adapter`。

验收：H5 与微信小程序构建通过；小程序分块响应或轮询降级可用；安全区和键盘交互正常。

### 阶段 F：企业级解析与插件扩展

交付：PDF、Word、Excel、PPT、TXT、Markdown、HTML、CSV；OCR、清洗、标题章节识别；OpenAI、Gemini、Qwen、BGE、BCE、Jina、SiliconFlow、OneAPI、NewAPI；pgvector、Milvus、Qdrant、Weaviate。

验收：每种插件拥有契约测试；Provider 健康检查与失败回退可观测；切换配置不会污染活动索引。

### 阶段 G：外部数据源、治理和生产化

交付：网页、Notion、飞书、钉钉、企业微信、OSS/S3 连接器骨架；权限/RLS、限流、审计、计费、热门问题、备份恢复和生产迁移。

验收：租户隔离测试、权限矩阵测试、计费幂等测试、生产 Compose 校验、迁移演练、备份恢复演练通过。

### 阶段 H：完成审计

逐项对照原始十五类需求，给每一项提供代码、迁移、API、界面或自动化测试证据。缺失、间接或未验证的项不标记完成。

## 变更边界

- 不重写现有生成任务、计费和认证模块，只通过稳定端口集成。
- 不删除旧知识库表或用户现有未提交文件。
- 不在已移除的旧用户端目录重复开发正式用户能力；正式多端入口为 `apps/user-uni`。
- 不把渠道代理商 `agent` 与 AI 智能体 `ai-agent` 命名混用。
