# NewAPI 接口整理

本文档根据当前 NewAPI 前端接口调用整理，主要用于先知 AI 项目对接、排查渠道和令牌问题。管理员接口需要登录后的管理员会话，并在请求头中携带 `New-Api-User: 1`。

> 注意：不要把管理员 session、模型调用 key、用户密码写入代码仓库或接口文档。

## 基础信息

- 服务地址：`https://code.lai1758.dpdns.org`
- 当前用于图片生成的模型调用 Key 属于 NewAPI 令牌，不是管理员 Token。
- 管理员接口鉴权通常依赖浏览器登录后的 `session` Cookie 和 `New-Api-User: 1` 请求头。
- 兼容 OpenAI 的调用接口使用 `/v1/...` 路径。
- 管理后台接口使用 `/api/...` 路径。

## 管理员接口鉴权示例

```bash
curl "https://code.lai1758.dpdns.org/api/user/self" \
  -H "accept: application/json, text/plain, */*" \
  -H "New-Api-User: 1" \
  -b "session=你的管理员会话; i18next=zh-CN"
```

## 系统与状态

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/status` | 查看系统状态 |
| GET | `/api/setup` | 查看初始化状态 |
| POST | `/api/setup` | 初始化系统 |
| GET | `/api/notice` | 查看公告 |
| GET | `/api/group` | 查看分组 |
| GET | `/api/group/` | 查看分组 |
| GET | `/api/pricing` | 查看价格信息 |

## 用户接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/user/self` | 查看当前登录用户 |
| PUT | `/api/user/self` | 修改当前用户信息 |
| DELETE | `/api/user/self` | 删除当前用户 |
| GET | `/api/user/?p=1&page_size=20` | 用户列表 |
| GET | `/api/user/:id` | 查看指定用户详情 |
| POST | `/api/user/` | 新增用户 |
| PUT | `/api/user/` | 修改用户，管理员可用于重置用户密码 |
| POST | `/api/user/manage` | 用户额度等管理操作 |
| GET | `/api/user/search?keyword=&group=` | 搜索用户 |
| GET | `/api/user/models` | 查看用户可用模型 |
| GET | `/api/user/token` | 查看用户 Token 信息 |
| GET | `/api/user/self/groups` | 查看当前用户分组 |
| GET | `/api/user/logout` | 退出登录 |

### 修改用户或重置密码

管理员修改用户通常先读取用户详情，再提交完整用户对象：

```http
GET /api/user/:id
PUT /api/user/
```

示例字段：

```json
{
  "id": 2,
  "username": "lmxchyy",
  "display_name": "lmxchyy",
  "password": "新密码",
  "group": "vip",
  "status": 1,
  "role": 1,
  "email": "",
  "remark": ""
}
```

当前用户修改自己的密码：

```http
PUT /api/user/self
```

```json
{
  "original_password": "旧密码",
  "password": "新密码"
}
```

## 用户安全接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/user/2fa/status` | 查看双因素认证状态 |
| POST | `/api/user/2fa/setup` | 初始化双因素认证 |
| POST | `/api/user/2fa/enable` | 启用双因素认证 |
| POST | `/api/user/2fa/disable` | 禁用双因素认证 |
| POST | `/api/user/2fa/backup_codes` | 生成备份码 |
| DELETE | `/api/user/:id/2fa` | 管理员重置指定用户双因素认证 |
| DELETE | `/api/user/:id/reset_passkey` | 管理员重置指定用户 Passkey |

## 登录、注册与找回密码

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/user/login?turnstile=` | 用户登录 |
| POST | `/api/user/login/2fa` | 双因素登录 |
| POST | `/api/user/register?turnstile=` | 用户注册 |
| GET | `/api/verification?email=` | 获取邮箱验证码 |
| GET | `/api/reset_password?email=` | 发送重置密码邮件 |
| POST | `/api/user/reset` | 重置密码 |
| POST | `/api/verify` | 校验验证码 |

## 令牌接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/token/?p=1&size=10` | 令牌列表 |
| GET | `/api/token/:id` | 查看令牌详情 |
| GET | `/api/token/search?keyword=` | 搜索令牌 |
| GET | `/api/token/?status_only=true` | 查看令牌状态 |
| POST | `/api/token/` | 新增令牌 |
| PUT | `/api/token/` | 更新令牌 |
| PUT | `/api/token/?status_only=true` | 更新令牌状态 |
| DELETE | `/api/token/:id/` | 删除令牌 |
| POST | `/api/token/:id/key` | 重置令牌 Key |
| POST | `/api/token/batch` | 批量操作令牌 |
| POST | `/api/token/batch/keys` | 批量重置令牌 Key |

## 渠道接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/channel/?p=1&page_size=20` | 渠道列表 |
| GET | `/api/channel/:id` | 查看渠道详情 |
| POST | `/api/channel/` | 新增渠道 |
| PUT | `/api/channel/` | 更新渠道 |
| DELETE | `/api/channel/:id/` | 删除渠道 |
| GET | `/api/channel/search?keyword=&group=` | 搜索渠道 |
| GET | `/api/channel/test` | 测试渠道 |
| POST | `/api/channel/test` | 测试渠道 |
| GET | `/api/channel/fetch_models/:id` | 拉取指定渠道模型 |
| POST | `/api/channel/fetch_models` | 拉取渠道模型 |
| GET | `/api/channel/update_balance` | 更新所有渠道余额 |
| GET | `/api/channel/update_balance/:id/` | 更新指定渠道余额 |
| POST | `/api/channel/batch` | 批量操作渠道 |
| POST | `/api/channel/batch/tag` | 批量设置渠道标签 |
| POST | `/api/channel/copy/:id` | 复制渠道 |
| DELETE | `/api/channel/disabled` | 删除禁用渠道 |
| POST | `/api/channel/fix` | 修复渠道配置 |

## 渠道标签、多 Key 与上游同步

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/channel/models` | 查看渠道模型 |
| GET | `/api/channel/models_enabled` | 查看启用模型 |
| GET | `/api/channel/tag/models?tag=` | 查看指定标签模型 |
| PUT | `/api/channel/tag` | 更新渠道标签 |
| POST | `/api/channel/tag/enabled` | 启用标签渠道 |
| POST | `/api/channel/tag/disabled` | 禁用标签渠道 |
| POST | `/api/channel/multi_key/manage` | 多 Key 管理 |
| POST | `/api/channel/:id/key` | 重置渠道 Key |

## 日志接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/log/?p=` | 管理员日志列表 |
| GET | `/api/log/self/?p=` | 当前用户日志列表 |
| GET | `/api/log/stat?type=` | 管理员日志统计 |
| GET | `/api/log/self/stat?type=` | 当前用户日志统计 |
| GET | `/api/log/channel_affinity_usage_cache` | 查看渠道亲和缓存 |
| DELETE | `/api/log/?target_timestamp=` | 删除指定时间之前的日志 |

## 模型接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/models/?p=` | 模型列表 |
| GET | `/api/models/:id` | 查看模型详情 |
| GET | `/api/models/search?keyword=` | 按关键词搜索模型 |
| GET | `/api/models/search?vendor=` | 按供应商搜索模型 |
| GET | `/api/models/missing` | 查看缺失模型 |
| GET | `/api/models/?status_only=true` | 查看模型状态 |
| POST | `/api/models/` | 新增模型 |
| PUT | `/api/models/` | 更新模型 |
| PUT | `/api/models/?status_only=true` | 更新模型状态 |
| DELETE | `/api/models/:id` | 删除模型 |
| GET | `/api/models/sync_upstream/preview` | 预览上游模型同步 |
| POST | `/api/models/sync_upstream` | 执行上游模型同步 |

## 兑换码接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/redemption/?p=&size=` | 兑换码列表 |
| GET | `/api/redemption/:id` | 查看兑换码详情 |
| POST | `/api/redemption/` | 新增兑换码 |
| PUT | `/api/redemption/` | 更新兑换码 |
| DELETE | `/api/redemption/:id/` | 删除兑换码 |
| GET | `/api/redemption/search?keyword=` | 搜索兑换码 |
| GET | `/api/redemption/invalid` | 查看无效兑换码 |

## 系统选项与配置

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/option/` | 查看系统选项 |
| PUT | `/api/option/` | 更新系统选项 |
| POST | `/api/option/migrate_console_setting` | 迁移控制台配置 |
| POST | `/api/option/payment_compliance` | 支付合规配置 |
| POST | `/api/option/rest_model_ratio` | 重置模型倍率 |
| GET | `/api/option/channel_affinity_cache` | 查看渠道亲和缓存 |
| DELETE | `/api/option/channel_affinity_cache` | 清理渠道亲和缓存 |

## OpenAI 兼容接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/v1/models` | 查看兼容模型列表 |
| POST | `/v1/chat/completions` | 聊天补全 |
| POST | `/v1/responses` | Responses 接口 |
| POST | `/v1/responses/compact` | Responses 压缩接口 |
| POST | `/v1/messages` | Messages 接口 |
| POST | `/v1/images/generations` | 图片生成 |
| POST | `/v1/images/edits` | 图片编辑 |
| POST | `/v1/images/variations` | 图片变体 |
| POST | `/v1/audio/speech` | 文本转语音 |
| POST | `/v1/audio/transcriptions` | 音频转文字 |
| POST | `/v1/audio/translations` | 音频翻译 |
| POST | `/v1/embeddings` | 向量接口 |
| POST | `/v1/rerank` | 重排接口 |
| GET | `/v1beta/models` | Gemini 兼容模型列表 |

## 已确认的关键数据关系

- 令牌 `gpt-image-2` 的令牌 ID 是 `20`，归属用户 ID 是 `1`，即 `root`。
- 用户 `lmxchyy` 存在，用户 ID 是 `2`，分组是 `vip`。
- 文生图报错 `INSUFFICIENT_BALANCE` 不是先知 AI 本地余额不足，而是 NewAPI 上游渠道返回的余额不足或渠道侧错误。
- 当前 NewAPI 后台近期已有 `gpt-image-2` 成功调用记录，后续如果仍失败，需要继续查先知 AI 后端请求内容、渠道路由或具体提示词。

## 后续建议

- 如果要正式作为开发文档使用，建议继续补充每个接口的请求体、响应体、权限要求和错误码。
- 对删除令牌、删除渠道、重置密码等高风险接口，建议在调用前先执行 `GET` 查询确认目标对象。
- 管理员 session 只适合临时排查，不建议写入配置文件；生产对接应使用稳定的管理凭证或服务端安全配置。
