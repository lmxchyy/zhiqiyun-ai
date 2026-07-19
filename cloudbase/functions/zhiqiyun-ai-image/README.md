# 知启云 AI CloudBase 生图函数

该函数仅作为知启云 Go 后端的 CloudBase Provider 上游，不允许小程序直接调用。它不会替代现有任务中心、钱包、内容审核、对象存储或作品中心。

部署前人工操作：

1. 在已完成企业认证且有效期不少于 3 个月的 CloudBase 环境中创建 Node.js 云函数。
2. 使用 Node.js 18 或更高运行时，安装 `@cloudbase/node-sdk >= 3.18.3`，函数超时按官方建议设置为 900 秒。
3. 配置函数环境变量 `ENV_ID` 与 `AI_WATERMARK_TEXT=AI生成`。不得把 API Key 写入本目录。
4. 只通过 CloudBase HTTP API 暴露函数，并由知启云 Go 后端使用服务端 API Key 调用。
5. 在 CloudBase 平台获取算法备案截图和合作协议后，再由管理员将对应模型合规状态改为 approved；本仓库不会自动开启模型。

当前函数只允许官方文档列出的两个模型：

- `HY-Image-3.0-Plus-4090-Tob-v1.0`
- `HY-Image-v3.0-I2I-ToB-v1.0.1`

CloudBase 返回的临时图片 URL 仅用于 Go 后端立即下载到知启云现有对象存储，不作为作品永久地址。
