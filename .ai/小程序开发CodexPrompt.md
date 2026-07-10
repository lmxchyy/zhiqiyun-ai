# Codex任务：完善小程序端功能

## 任务目标

在当前项目中新增或完善小程序端能力，定位为「移动轻量入口」。

## 请严格遵守

1. 不要改动 Web 后台无关代码。
2. 不要一次性实现全部功能。
3. 先完成一期 MVP。
4. 页面必须组件化。
5. 所有接口调用走统一 request 封装。
6. 登录态、Token、租户ID必须统一管理。

## 一期页面

请优先实现：

1. pages/index/index - 首页
2. pages/chat/index - AI 对话
3. pages/kb/index - 知识库列表
4. pages/kb/chat - 知识库问答
5. pages/works/index - 作品中心
6. pages/mine/index - 我的
7. pages/lead/form - 线索提交

## 一期组件

components/
- AppHeader
- FeatureGrid
- BalanceCard
- ChatInput
- ChatMessage
- ModelSelector
- KnowledgeSelector
- SourceReference
- WorkCard
- LeadForm
- EmptyState
- LoadingState

## 一期接口

api/
- auth.ts
- user.ts
- chat.ts
- knowledge.ts
- works.ts
- token.ts
- lead.ts

## 开发顺序

1. 创建小程序目录结构
2. 封装 request
3. 实现微信登录
4. 实现首页
5. 实现 AI 对话
6. 实现知识库问答
7. 实现我的页面
8. 实现作品中心
9. 实现线索表单
10. 自测并输出修改说明
