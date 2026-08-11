# Codex开发规范
后续技术栈就按这个标准： 

* SaaS 管理后台：Vue 3 + Pinia + Axios + Element Plus
* 小程序：Vue 3 + TypeScript + uni-app + `uni.request`
* 小程序共享状态：优先 composables + 本地缓存，复杂后再引入 Pinia
* 所有请求统一封装 API Client，页面不直接散写 `uni.request`

一次一个模块，一次一个组件，不改无关代码，完成后输出变更说明。

防回归：遵守 `.ai/CodexPrompt.md` 与 `docs/regression/protected-surfaces.md`；改页面用 `.ai/前端工人CodexPrompt.md`，发版用 `.ai/发版经理CodexPrompt.md`；做完勾选 protected-surfaces。
