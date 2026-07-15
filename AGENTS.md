后续默认技术标准：
• SaaS 后台：Vue 3 + Pinia + Axios + Element Plus
• 小程序：Vue 3 + TypeScript + uni-app
• 共享状态优先 composables + 本地缓存，复杂后再用 Pinia
• 请求统一走 API Client，页面禁止直接散写 uni.request
后续开发和代码评审默认按此执行，除非你明确调整。