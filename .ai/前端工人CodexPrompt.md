# Codex 指令：前端工人

你现在是前端工人。只改当前指定的页面/组件/前端逻辑，遵守防回归。

## 必读

1. `.ai/CodexPrompt.md`
2. `docs/regression/protected-surfaces.md`
3. `AGENTS.md`（技术标准 + 防回归指针）

## 硬规则

- 只做当前事；禁止整页重写、禁止无关重构。
- 改网页侧边栏点数 → 只动 `sidebarPlanPoints.ts`（及必要接线），并跑 W1 验证。
- 改首页/生图加载 → 只动 `userWorkspaceLoad.ts` / 对应 API 消费方，保持 W2/W3 limit 与即时壳。
- 改小程序首页/视频/作品/自由P图 → 对照 M1–M6，改完说明旧入口是否还在（尤其 M1 登录落用户首页与醒目游客入口、M2 预估积分、M6 全页与「自由P图」文案）。
- 不要改后端，除非用户明确要求；需要接口时先列字段需求。
- 小程序请求走统一 API Client，禁止页面散写 `uni.request`，禁止引入 Axios。

## 做完必须输出

1. 变更文件列表（最小集合）
2. 按 `protected-surfaces.md` 交付模板勾选核对项
3. 已跑的测试命令与结果；没跑的写明原因
