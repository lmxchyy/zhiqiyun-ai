# Grok Imagine 1.5 按秒视频模型设计

## 目标

在保留原按次计费的 `grok-imagine-video-1.5-preview` 模型的同时，接入按秒计费的 `grok-imagine-1.5-video`。新模型通过 NewAPI `https://newapi.zs-kjhn.cn` 调用，用户侧预估与正式扣费均为 15 积分/秒，并跑通文生视频、单图生视频、异步轮询、作品入库和账单链路。

## 已确认的根因

当前模型、租户白名单和 15 积分/秒的发布计费规则已经存在，但调用链仍走通用视频协议：

- 默认创建地址是 `/v1/video/generations`，文档要求 `/v1/videos`。
- 当前请求字段是 `seconds`、`aspect_ratio`、`resolution`，文档要求 `duration`、`size`、`quality`。
- 本地旧 Grok 通道仍指向 `https://sub.xlcsh.top`，而本模型应通过 `.env` 中的 NewAPI 地址与 Key 调用。
- 新模型的供应商成本记录当前为 0，未反映 0.13 元/秒的真实成本。

因此“无反应”不是单纯缺少模型选项，而是模型已经可见、后台任务启动后使用了错误的上游地址和协议。

## 方案选择

### 采用：在现有 OpenAI-compatible 视频 Provider 中增加模型专用协议分支

`grok-imagine-1.5-video` 仍复用现有 Provider 的鉴权、HTTP 客户端、异步任务轮询、失败处理和任务结果封装，只在模型协议存在差异的位置分支：创建地址、请求字段和结果判定。

优点：改动小，继续走既有生成任务、作品和账单链路；不会复制一套 Provider；旧 Grok 模型与 Seedance 行为不变。

### 不采用：新建独立 Grok Provider

会重复鉴权、轮询、错误处理和结果封装，当前只有一个协议差异模型，不值得增加新的 Provider 层。

### 不采用：只修改通道 endpoint

即使把 endpoint 改为 `/v1/videos`，通用请求仍会发送错误字段，无法解决问题。

## 模型与路由

- 原模型：`grok-imagine-video-1.5-preview`，保留现有按次计费和现有协议，不改名、不改价。
- 新模型：`grok-imagine-1.5-video`，按秒计费。
- 新模型上游：`https://newapi.zs-kjhn.cn`。
- 运行时使用现有 `MODEL_PROVIDER_URL` 与 `MODEL_PROVIDER_API_KEY`，并让 NewAPI 运行通道声明支持新模型；不得复用旧 `sub.xlcsh.top` 通道保存的 Key。

## 请求与轮询

创建请求：

```http
POST /v1/videos
Authorization: Bearer <MODEL_PROVIDER_API_KEY>
Content-Type: application/json
```

字段映射：

| 平台字段 | NewAPI 字段 | 规则 |
| --- | --- | --- |
| `model` | `model` | 固定为 `grok-imagine-1.5-video` |
| `prompt` | `prompt` | 原样传递 |
| `duration` | `duration` | JSON 数字，6–30 秒 |
| `aspect_ratio` | `size` | `16:9`、`9:16`、`1:1`、`3:2`、`2:3` |
| `resolution` | `quality` | `480p` 或 `720p` |
| 参考图 | `image_urls` | 可选公网 HTTP(S) URL；当前产品 UI 继续使用单参考图 |

创建成功保存 `id`，每 5 秒查询：

```http
GET /v1/videos/{id}
```

- `queued`、`processing`：继续轮询。
- `completed`：读取 `data.result.videos[0].url`。
- `failed`：读取上游 `error` 并把任务标记失败，释放预留积分。
- 轮询遵守任务 Context 与现有超时，不新增第二套后台任务系统。

## 参数能力

- 文生视频和图生视频均可用，不能再要求新模型必须上传参考图。
- UI 使用离散时长：6、8、10、12、15、20、25、30 秒。
- 分辨率：480p、720p。
- 画幅：16:9、9:16、1:1、3:2、2:3。
- 上游最多支持 7 张参考图；本次不扩展现有单参考图 UI，避免扩大 M2 页面改动。Provider 不套用旧 `grok-video-1.5` 的“恰好一张图”限制。

## 计费与成本

两个金额口径必须分开：

- 用户预估与正式扣费：15 积分/秒，即按 0.15 元/秒预估。
- 上游供应商真实成本：0.13 元/秒，写入 Provider Cost，用于毛利核算。
- 新模型不应用分辨率倍率，480p 与 720p 均按 15 积分/秒。
- 最低扣费为 15 积分。
- 例：6 秒视频预估和正式扣费均为 90 积分；供应商预计成本为 0.78 元；预计毛利为 0.12 元。
- 原 preview 模型继续按次计费，规则互不覆盖。

## 测试

先写失败测试，再实现：

1. Provider 请求必须命中 `/v1/videos`，并发送数字 `duration`、`size`、`quality`，不泄漏 `seconds`、`aspect_ratio`、`resolution`。
2. 文生视频无参考图可以创建；图生视频正确发送 `image_urls`。
3. 创建返回 `queued` 后轮询 `/v1/videos/{id}`，能解析 `data.result.videos[0].url`。
4. `failed` 状态返回可见错误，不产生作品、不重复扣费。
5. 新模型 6 秒在 480p、720p 下均预估 90 积分；旧 preview 模型仍按次计费。
6. 运行 M2 保护面规定的 Node 与 Go 回归测试，并构建相关前端包。
7. 用户明确授权凭据外发后，使用 `.env` 的 NewAPI Key 提交一次 6 秒真实任务，确认任务完成、作品入库和正式扣费 90 积分。

## 变更边界

只修改新模型所需的 Provider 协议、模型能力/展示、NewAPI 路由声明、按秒计费与 Provider Cost 配置及对应测试。不改图片、PPT、Seedance、旧 Grok preview、登录、首页或其他保护面。

