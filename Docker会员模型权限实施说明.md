# Docker 会员模型权限实施说明

## 实施范围

- 模型目录统一管理模型编码、名称、支持能力和会员等级。
- 免费会员仅可使用免费模型，付费会员可使用高质量模型。
- `/api/v1/models` 根据当前登录用户返回可用模型，并支持通过 `capability` 参数筛选生成能力。
- 创建生成任务时，后端再次校验模型权限，避免绕过前端直接调用接口。
- 按会员套餐限制同时处于 `QUEUED`、`PROCESSING`、`RETRYING` 状态的任务数量。
- 生成工作台动态加载当前会员可用模型。

## 套餐并发规则

| 套餐 | 并发任务数 |
| --- | ---: |
| 免费会员 | 1 |
| 月度会员 | 3 |
| 年度会员 | 8 |

## Docker 部署

```powershell
docker compose -f compose.yml up -d --build
docker compose -f compose.yml ps
```

访问地址：<http://localhost:3100>

## 验证接口

登录后携带 Bearer Token 请求：

```http
GET /api/v1/models
GET /api/v1/models?capability=TEXT_TO_IMAGE
```

免费会员选择 `mock-quality` 时，接口拒绝创建任务；免费会员存在一个运行中任务时，再次创建任务会返回并发上限错误。

## 自动化验证

```powershell
npm.cmd test
node --check backend/src/platform.js
node --check backend/src/server.js
node --check frontend/app.js
```

当前自动化测试覆盖免费模型权限、付费模型权限、模型能力筛选和免费套餐并发限制。
