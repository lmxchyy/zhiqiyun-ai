# Grok Imagine 1.5 按秒视频模型设计

## 目标

在保留原按次计费的 `grok-imagine-video-1.5-preview` 模型的同时，接入按秒计费的 `grok-imagine-1.5-video`。新模型通过 NewAPI `https://newapi.zs-kjhn.cn` 调用，用户侧预估与正式扣费均为 15 积分/秒，并跑通文生视频、多参考图生视频、异步轮询、作品入库和账单链路。

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
| 参考图 | `image_urls` | 可选公网 HTTP(S) URL，最多 7 张 |

创建成功保存 `id`，每 5 秒查询：

```http
GET /v1/videos/{id}
```

- `queued`、`processing`：继续轮询。
- `completed`：读取 `data.result.videos[0].url`。
- `failed`：读取上游 `error` 并把任务标记失败，释放预留积分。
- 轮询遵守任务 Context 与现有超时，不新增第二套后台任务系统。

## 参数能力

两个模型必须使用各自独立的能力声明，前端切换模型时动态重建时长和参考图控件：

| 能力 | `grok-imagine-video-1.5-preview` | `grok-imagine-1.5-video` |
| --- | --- | --- |
| 生成模式 | 仅图生视频 | 文生视频、图生视频 |
| 参考图 | 必须且只能 1 张起始图 | 可选，0–7 张 |
| 视频时长 | 沿用现有 preview 能力，最大 15 秒 | 6–30 秒整数 |
| 分辨率 | 480p、720p | 480p、720p |
| 画幅 | 沿用 preview 现有能力 | 16:9、9:16、1:1、3:2、2:3 |
| 计费 | 按次 | 15 积分/秒 |

- 新模型的时长选择器必须覆盖 6 到 30 的每个整数，不能只提供少数离散值。
- 新模型无参考图时提交 `TEXT_TO_VIDEO`；有 1–7 张参考图时提交 `IMAGE_TO_VIDEO` 并完整传递 `image_urls`。
- preview 切换后必须要求恰好 1 张参考图；不能继承新模型的多图或文生视频能力。
- 当从新模型切回 preview 且已有多张参考图时，切换确认提示应说明只保留第一张；用户取消时不改变当前模型和图片。
- 后端的 `MaxReferenceImages` 必须独立于“是否支持尾帧”表达，不能因为模型不支持尾帧就把多参考图错误收窄为 1 张。

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
2. 新模型文生视频无参考图可以创建；1 张和 7 张参考图能完整发送 `image_urls`，第 8 张被前后端一致拒绝。
3. 创建返回 `queued` 后轮询 `/v1/videos/{id}`，能解析 `data.result.videos[0].url`。
4. `failed` 状态返回可见错误，不产生作品、不重复扣费。
5. preview 无图、两张图或大于 15 秒均被拒绝；恰好一张合法参考图仍按次计费。
6. 新模型 6 秒在 480p、720p 下均预估 90 积分；30 秒预估 450 积分。
7. 模型切换会同步切换时长和参考图数量限制，不丢失取消切换前的草稿。
8. 运行 M2 保护面规定的 Node 与 Go 回归测试，并构建相关前端包。
9. 用户明确授权凭据外发后，使用 `.env` 的 NewAPI Key 提交一次 6 秒真实任务，确认任务完成、作品入库和正式扣费 90 积分。

## 变更边界

只修改新模型所需的 Provider 协议、模型能力/展示、多参考图控件、NewAPI 路由声明、按秒计费与 Provider Cost 配置及对应测试。preview 只补齐独立能力约束，不改其协议和价格；不改图片、PPT、Seedance、登录、首页或其他保护面。

## 资料依据

- 新模型接口：`https://ivh2t5pupj.apifox.cn/9051016m0.md`
- preview 模型官方说明：`https://x.ai/news/grok-imagine-1-5`
