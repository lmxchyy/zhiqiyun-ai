# Image and Video Pricing Optimization Audit

审计日期：2026-08-27
审计分支：`feat/image-video-price-optimization`
审计范围：AI 生图、AI 生视频；不涉及 PPT、RAG、Agent、订阅重构、企业定价或正式价格发布。

## Executive Summary

- **Pricing V2 的主链路已经具备实施新价格的架构条件。** 生成请求从 `xz_billing_rule_versions` 选择有效的 `PUBLISHED` 规则，报价和 submit 都重新取权威规则，任务保存规则版本和价格快照；不需要重建 `GenerationPricingEngine`。
- **当前价格不能直接作为商业毛利依据。** 代码中的 `pointUnitAmountCents=10` 把 1 point 按 ¥0.10 估算，而充值/套餐配置是 100 points/元，即 1 point 的名义价值应为 ¥0.01。该差异会使任务估算毛利约放大 10 倍，属于必须先修正的计量/毛利口径问题。
- **当前正式生图规则不完整。** GPT Image 2 已发布 `standard/high` 和 1K/部分尺寸；`low/medium/auto` 没有明确正式 tier，2K/4K 没有明确正式 tier，未命中尺寸会落到现有最高尺寸规则。这些是 `PRICING_POLICY_GAP`，不是 Pricing Engine 公式 bug。
- **当前正式视频规则明显分化。** Grok Imagine 1.5 Video 按秒 15 points，Seedance Fast/Doubao 按秒 80 points 再乘分辨率；当前 5s/720p Seedance 为 600 points。按套餐名义点值计算，Grok 6s/720p 已接近亏损，Seedance 5s/720p 毛利约 16.7%。
- **不能完成真实生产 7/30 天使用分布或可信收入影响模拟。** 可访问的本地 Postgres 最新任务为 2026-08-11，且未验证为生产库；因此本报告把本地数据作为结构/样本审计，不把它称为生产使用分布。结论状态：`PRICING_OPTIMIZATION_BLOCKED_BY_DATA`。

## Current Pricing Architecture

### 权威边界

| 领域 | 权威位置 | 结论 |
|---|---|---|
| 客户售价 | `xz_billing_rule_versions` 中有效的 `PUBLISHED` 版本 | 以规则版本为准；历史版本保留 |
| 规则计算 | `backend-go/internal/pricing/generation.go` 的 `Calculate` | 支持 `PER_REQUEST`、`PER_IMAGE`、`PER_SECOND` 等，最终统一 CEIL |
| Quote API | `POST /api/v1/generation-tasks/quote` | 只读报价，返回规则版本、计费单位、数量和 breakdown |
| submit 计价 | `generationQuoteForRequest` | submit 时重新读取发布规则，禁止信任前端价格 |
| 供应商成本 | `xz_provider_costs` | 与客户售价独立，按模型、通道、参数和有效期选择 |
| 任务快照 | `xz_generation_tasks` | 保存 `quoted_points`、`billing_rule_version_id`、`supplier_cost`、`estimated_margin` |
| 钱包/点数 | `xz_wallet_ledger`、个人点数 lot | reserve/capture/release 生命周期和幂等账本 |

### 当前价格规则的真实解析

Postgres 只读查询显示，当前生图/视频规则均为有效 `PUBLISHED` 版本。数据库中没有可作为正式售价依据的前端 fallback。代码默认规则只在 JSON/历史投影路径使用；生产 Postgres 路径存在版本数据时优先使用版本数据。

## Current Production Price Matrix

以下是权威规则恢复结果。`standard` 是当前已发布的质量名，不等于用户要求的 `medium`。

### IMAGE — GPT Image 2

计费单位：`PER_IMAGE`；基础价：10 points/image；minimum charge：1 point。

| quality | 1K / `1024x1024` | 其他已发布尺寸 `1024x1536` / `1536x1024` | 2K / 4K |
|---|---:|---:|---:|
| standard | 10 | 12 | 无显式 tier；可能落到最高已存在尺寸规则 |
| high | 15 | 18 | 无显式 tier；可能落到最高已存在尺寸规则 |
| low | 无显式 tier；当前计算会不乘 quality ratio | 无显式 tier | 无显式 tier |
| medium | 无显式 tier；当前计算会不乘 quality ratio | 无显式 tier | 无显式 tier |
| auto | 无显式 tier；当前计算会不乘 quality ratio | 无显式 tier | 无显式 tier |

实际正式规则 JSON：`quality={standard:1,high:1.5}`、`size={1024x1024:1,1024x1536:1.2,1536x1024:1.2}`。

### VIDEO

| model | billing unit | 当前公式 | 当前正式规则 |
|---|---|---|---|
| `grok-imagine-video-1.5-preview` | PER_REQUEST | 100 points/request | 10s、15s；480p/720p；分辨率不影响价格 |
| `grok-imagine-1.5-video` | PER_SECOND | 15 × duration | 6–30s；480p/720p；分辨率不影响价格 |
| `seedance-fast-2.0` | PER_SECOND | 80 × duration × resolution multiplier | 480p=1、720p=1.5、1080p=2；5s/720p=600 |
| `doubao-seedance-2.0` | PER_SECOND | 80 × duration × resolution multiplier | 480p=1、720p=1.5、1080p=2、4K=4 |

未发现独立正式 `seedance-2.0` 售价规则；生产模型清单中正式保留 `seedance-fast-2.0` 与 `doubao-seedance-2.0`。

## Provider Cost Matrix

成本查询时间：2026-08-27；金额统一换算为 CNY cents。数据库成本单位是 CNY 元，换算公式为 `unit_cost × quantity × 100`。

| provider | channel | model | billing unit | unit cost | effective from | effective to | priority/status | 可覆盖范围 |
|---|---|---|---|---:|---|---|---|---|
| OPENAI | `channel_openai` | `gpt-image-2` | PER_IMAGE | 60 cents | 2026-07-15 | open | ACTIVE | quality standard/high；未区分 size |
| NEWAPI | `channel_runtime_env` | `grok-imagine-1.5-video` | PER_SECOND | 13 cents | 2026-08-04 | open | ACTIVE | 6–30s、480p/720p |
| CME_CLOUD | `channel_cmecloud_seedance` | `seedance-fast-2.0` | PER_SECOND | 80 cents | 2026-07-15 | open | ACTIVE | 720p |
| CME_CLOUD | `channel_cmecloud_seedance` | `doubao-seedance-2.0` | PER_SECOND | 80 cents | 2026-07-15 | open | ACTIVE | 720p |
| — | — | `grok-imagine-video-1.5-preview` | — | COST_UNKNOWN | — | — | 没有 `xz_provider_costs` 记录 |

### 最低/典型/最高成本

在已有成本覆盖范围内，单位成本没有参数差异：

| model | minimum generation cost | typical generation cost | maximum generation cost | 说明 |
|---|---:|---:|---:|---|
| GPT Image 2 | ¥0.60/image | ¥0.60/image | ¥0.60/image | provider 未按 quality/size 区分 |
| Grok Imagine 1.5 Video | ¥0.78/6s | ¥1.56/12s | ¥3.90/30s | 480p/720p 相同成本记录 |
| Seedance Fast | COST_UNKNOWN（除 720p） | ¥4.00/5s/720p | COST_UNKNOWN | 仅 720p 有成本 |
| Doubao Seedance | COST_UNKNOWN（除 720p） | ¥4.00/5s/720p | COST_UNKNOWN | 4K 没有成本记录 |

成本治理建议继续沿用现有 fail-closed：未知成本不得被猜测为 0，不在本报告中补造供应商成本。

## Point Economics

### 发现的单位冲突

- `xz_billing_config.CREDITS_PER_CNY_YUAN = 100`。
- 充值包：100 元/10,000 points、400 元/40,000 points，均为 100 points/元。
- `recharge_small`：19.90 元/2,500 points = ¥0.00796/point。
- `recharge_standard`：99 元/15,000 points = ¥0.00660/point。
- `recharge_business`：299 元/50,000 points = ¥0.00598/point。
- `recharge_enterprise`：999 元/200,000 points = ¥0.004995/point。
- 但 `backend-go/internal/httpserver/store.go` 的 `pointUnitAmountCents = 10`，任务估算把 1 point 换算为 10 cents，即 ¥0.10/point。

因此必须拆开两个口径：

1. `NOMINAL_POINT_VALUE = ¥0.01/point`：由 100 points/元的商品配置决定。
2. `EFFECTIVE_REVENUE_PER_POINT`：按已售商品 mix 计算；由于当前可读库的订单状态/权益字段存在旧投影，且没有被验证为生产，先用情景值：

| scenario | effective revenue/point | 依据 |
|---|---:|---|
| LOW | ¥0.0050 | 企业包 999/200,000 的低价边界 |
| BASE | ¥0.0066 | 标准包 99/15,000，适合作为当前默认敏感性基准 |
| HIGH | ¥0.0100 | 100 元/10,000 的名义直充 |

正式实现前必须以生产支付成功订单的 `paid_amount / granted_points`，按自然月和产品类型重新计算 BASE；不能用会员展示价格或未支付订单代替现金收入。

### 公式

```text
nominal_revenue = user_price_points × 0.01 CNY
effective_revenue = user_price_points × effective_revenue_per_point
gross_profit = effective_revenue - provider_cost
gross_margin = gross_profit / effective_revenue, only when effective_revenue > 0
required_points(target_margin) = ceil(provider_cost / (effective_revenue_per_point × (1 - target_margin)))
```

`estimated_margin` 当前实现使用 10 cents/point，不能直接作为本报告的商业毛利结论，直到单位冲突被修复并回算。

## Usage Distribution

### 生产数据可用性

`PRODUCTION_DATA_NOT_AVAILABLE`。本次可访问的是本地 `ai-postgres-1` 数据库，数据库名为 `xianzhi`，任务总数 313，最新任务时间为 2026-08-11；审计日为 2026-08-27，未获得生产连接、环境证明或生产导出。因此：

- 最近 7 天生产 task/users/points/cost/margin：不可报告。
- 最近 30 天生产分布：不可报告。
- 以下数字仅为本地运行库样本，不得用于生产决策。

### 本地样本结构审计（非生产）

最近 30 天本地样本有 22 个任务、至少 2 个用户：

| model/type | tasks | users | points | supplier cost | stored estimated margin | negative margin |
|---|---:|---:|---:|---:|---:|---:|
| GPT Image 2 / text-to-image | 8 | 2 | 88 | ¥4.80 | -¥0.32 | 4 |
| Doubao Seedance / image-to-video | 3 | 1 | 1,800 | ¥12.00 | ¥168.00 | 0 |
| Grok 1.5 / image-to-video | 2 | 1 | 180 | ¥1.56 | ¥16.44 | 0 |
| Grok 1.5 / text-to-video | 2 | 1 | 180 | ¥1.56 | ¥8.34 | 0 |
| Grok Preview / image-to-video | 2 | 1 | 200 | COST_UNKNOWN | ¥0 | 0 |

本地样本中图片为 1K 与 1536×1024，视频为 Grok 6s/480p、6s/720p、Doubao 5s/720p、Preview 4s/15s（其中 Preview 的参数与当前模型能力也存在旧数据混入）。这只能证明查询字段和快照字段可用，不能证明用户总体偏好。

## Current Margin Analysis

以 `BASE = ¥0.0066/point`、provider 成本可知范围计算：

| configuration | current points | effective revenue | provider cost | gross profit | gross margin |
|---|---:|---:|---:|---:|---:|
| GPT Image 2 1K standard | 10 | ¥0.066 | ¥0.60 | -¥0.534 | -809.1% |
| GPT Image 2 1536×1024 standard | 12 | ¥0.079 | ¥0.60 | -¥0.521 | -659.3% |
| GPT Image 2 1K high | 15 | ¥0.099 | ¥0.60 | -¥0.501 | -506.1% |
| Grok 6s | 90 | ¥0.594 | ¥0.78 | -¥0.186 | -31.3% |
| Grok 12s | 180 | ¥1.188 | ¥1.56 | -¥0.372 | -31.3% |
| Seedance 5s/720p | 600 | ¥3.960 | ¥4.00 | -¥0.040 | -1.0% |
| Doubao 5s/720p | 600 | ¥3.960 | ¥4.00 | -¥0.040 | -1.0% |

这些结果与当前任务的 `estimated_margin` 不能直接对比，因为任务估算使用了错误的 ¥0.10/point 换算。若按当前代码口径，Seedance 5s/720p 会显示约 93.3% 毛利；这正是单位冲突的证据。

## Pricing Policy Gaps

### PRICING_ENGINE_BUG

1. 生成 Pricing Engine 的计量和 CEIL 算法本身通过现有测试，未发现必须重写的公式 bug。
2. `pointUnitAmountCents=10` 与正式点数商品 100 points/元不一致，导致 `user_charge_amount`、`estimated_margin`、platform profit 口径错误。应先作为 billing/economics bug 修正，再用历史快照回算；不要通过抬高/压低用户售价掩盖它。

### PRICING_POLICY_GAP

1. GPT Image 2 质量正式 tier 只有 standard/high；low/medium/auto 没有显式倍率。
2. GPT Image 2 2K/4K 没有显式价格 tier；`gptImageBillingSizeLookupKey` 在未命中时会在现有 tier/最高规则中寻找，形成 higher-resolution fallback to highest existing multiplier。
3. Provider 成本未覆盖 Grok Preview、Seedance/Doubao 非 720p，无法可靠计算 margin。
4. Grok Preview 是按请求收费且当前成本未知，不能套用其他视频模型的 per-second 成本。
5. provider routing 改变可能改变成本，但用户售价按 public model 的规则计算；应继续使用合理成本上限或 weighted cost，不让路由实时改变用户价格。

### BUSINESS_PRICING_DECISION

1. quality/size 价格阶梯是否体现产品价值，即使供应商成本不随 quality 变化，仍是商业策略而非供应商事实；`QUALITY_MULTIPLIER_IS_PRODUCT_PRICING_POLICY`。
2. 视频是否按秒收费、是否设置最低消费、是否对 4K 设置高价，属于商业决策；rounding 必须继续遵循当前 Pricing V2 的最终 CEIL。
3. 是否用 trial credit 解决注册体验，不能用正式商业价格补贴视频成本。

## Image Pricing Analysis

建议将 `auto = medium` 作为可解释的产品映射，但只有在新版本规则显式写入后才成立；不能靠未命中参数的隐式行为实现。由于 provider 当前没有 quality/size 差异成本，质量和分辨率倍率应被记录为产品价值/体验分层。

### Target price points for known GPT Image 2 cost

以下按 provider cost ¥0.60/image、BASE ¥0.0066/point 计算最低点数，未做用户可读价格取整：

| target margin | required points |
|---:|---:|
| 20% | 114 |
| 30% | 130 |
| 40% | 152 |
| 50% | 182 |
| 60% | 228 |

因此 1K low/medium 不宜低于 120/150 的商业梯度；若要在 BASE 口径达到 50%，主流 1K 配置应接近 180–200 points，而不是当前 10–15 points。

## Video Pricing Analysis

### Per-request vs per-second

- Grok Preview 当前是 `PER_REQUEST`，保持按请求收费，直到获得正式成本。
- Grok 1.5、Seedance、Doubao 当前是 `PER_SECOND`；公式为 `base points/sec × duration × resolution multiplier`。
- 继续使用当前最终结果 CEIL，不新增“每秒分别取整”的语义，避免报价和 submit 不一致。
- 对同一 public model 的不同 provider，售价不应随路由实时变化；成本规则应覆盖实际通道并用上限/加权成本做商业定价。

### Target price points

以 BASE ¥0.0066/point、50% margin 计算：

| model/config | known cost | mathematical minimum | readable balanced price |
|---|---:|---:|---:|
| Grok 6s | ¥0.78 | 237 | 240 |
| Grok 12s | ¥1.56 | 455 | 480 |
| Seedance 5s/720p | ¥4.00 | 1,213 | 1,200–1,250 |
| Doubao 5s/720p | ¥4.00 | 1,213 | 1,200–1,250 |

由于结果会受 effective point value 影响，以下推荐矩阵采用三情景展示，不应在单位冲突修复前发布。

## Trial User Analysis

注册赠送 10 points：

- 按当前 published price，能体验 GPT Image 2 1K standard 一次；若未命中质量/尺寸，当前隐式行为仍可能收取 10–15 points。
- 不能体验 Grok 6s（90 points）、Seedance 5s/720p（600 points）或 Preview（100 points）。
- 按修正后的商业价格，10 points 不应被迫覆盖视频。

建议把 `Commercial Price` 和 `Trial/Acquisition Strategy` 分开：首图体验券、一次性视频体验额度或注册后发放专用试用权益均可作为后续实验；本阶段不改变注册赠送和正式价格。

## Pricing Scenario A

### OPTION A — Growth

目标：降低第一次使用门槛，接受较低但不能为负的目标毛利。建议画像/低清配置用较低价，视频仍按成本保护设置最低点数；Preview 在成本未知时不纳入可承诺毛利。

假设：effective point value ¥0.010，目标 margin 20%–30%。适用于短期拉新，但需要严格监控负毛利和试用消耗。

## Pricing Scenario B

### OPTION B — Balanced（推荐）

目标：以 BASE ¥0.0066/point 为基准，主流已知成本配置约 50% 毛利，保留简单、可解释的 120/150/200/300/600/1200 梯度。

这是当前推荐的评估基线，但必须在生产订单 mix、单位换算修正、成本覆盖补齐后再发布。

## Pricing Scenario C

### OPTION C — Profit Protection

目标：以 LOW ¥0.005/point 仍尽量保持 50%–60% 毛利，覆盖 provider 成本波动和渠道折价。视频 720p 及以上应使用更高最低价，4K/未知成本配置先关闭商业承诺或要求人工确认。

## Recommended Image Pricing

下表是 OPTION B 的**评估矩阵**，不是已发布价格。provider cost 目前没有 size/quality 差异，所以差异化价格是产品策略；`current` 仅对当前规则有显式 tier 的组合给出。

| model | configuration | current points | provider cost | effective revenue @BASE | current margin | recommended points | recommended margin @BASE | change | reason |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| GPT Image 2 | 1K auto/medium | 10* | ¥0.60 | ¥0.066 | -809% | 150 | 39.4% | +1400% | explicit default tier；auto=medium policy |
| GPT Image 2 | 1K low | 10* | ¥0.60 | ¥0.066 | -809% | 120 | 24.2% | +1100% | growth entry, still above cost floor |
| GPT Image 2 | 1K high | 15 | ¥0.60 | ¥0.099 | -506% | 200 | 54.5% | +1233% | quality value tier |
| GPT Image 2 | 2K auto/medium | fallback* | COST_UNKNOWN by size | — | — | 240 | COST_UNKNOWN | — | explicit 2K tier required |
| GPT Image 2 | 2K high | fallback* | COST_UNKNOWN by size | — | — | 300 | COST_UNKNOWN | — | no silent highest-tier fallback |
| GPT Image 2 | 4K auto/medium | fallback* | COST_UNKNOWN by size | — | — | 360 | COST_UNKNOWN | — | explicit 4K tier and cost required |
| GPT Image 2 | 4K high | fallback* | COST_UNKNOWN by size | — | — | 500 | COST_UNKNOWN | — | cost/quality review required |

`*` 当前规则没有该组合的显式 tier；表中的 current 仅表示现有未命中行为或 1K standard 基准，不能视为正式政策。

## Recommended Video Pricing

同样是 OPTION B 评估矩阵；未知成本项不输出伪造毛利。

| model | configuration | current points | provider cost | effective revenue @BASE | current margin | recommended points | recommended margin @BASE | change | reason |
|---|---|---:|---:|---:|---:|---:|---:|---:|---|
| Grok 1.5 Video | 6s/480p | 90 | ¥0.78 | ¥0.594 | -31.3% | 240 | 50.8% | +166.7% | per-second cost floor |
| Grok 1.5 Video | 6s/720p | 90 | ¥0.78 | ¥0.594 | -31.3% | 240 | 50.8% | +166.7% | same provider cost, same public price |
| Grok 1.5 Video | 12s/720p | 180 | ¥1.56 | ¥1.188 | -31.3% | 480 | 50.8% | +166.7% | duration-linear |
| Grok Preview | 10s/request | 100 | COST_UNKNOWN | — | — | 150 | COST_UNKNOWN | +50% | keep per-request; cost required |
| Grok Preview | 15s/request | 100 | COST_UNKNOWN | — | — | 200 | COST_UNKNOWN | +100% | avoid pretending duration is free |
| Seedance Fast | 5s/480p | 400 | COST_UNKNOWN | ¥2.640 | — | 800 | COST_UNKNOWN | +100% | provider cost coverage missing |
| Seedance Fast | 5s/720p | 600 | ¥4.00 | ¥3.960 | -1.0% | 1200 | 49.5% | +100% | protect current main configuration |
| Seedance Fast | 5s/1080p | 800 | COST_UNKNOWN | ¥5.280 | — | 1600 | COST_UNKNOWN | +100% | explicit policy, cost required |
| Doubao Seedance | 5s/720p | 600 | ¥4.00 | ¥3.960 | -1.0% | 1200 | 49.5% | +100% | same known cost |
| Doubao Seedance | 5s/4K | 1600 | COST_UNKNOWN | ¥10.560 | — | 2400 | COST_UNKNOWN | +50% | 4K cost record required |

## User Impact

真实 30 天任务明细不可用，无法计算任务级涨跌、平均涨幅、P50、P90 或最大涨幅。静态比较已经显示：

- Grok 6s 从 90 到 240：`HIGH_USER_IMPACT`（+166.7%）。
- Seedance 5s/720p 从 600 到 1200：`VERY_HIGH_USER_IMPACT`（+100%）。
- Preview 15s 从 100 到 200：`VERY_HIGH_USER_IMPACT`，且成本未知。
- GPT Image 1K high 从 15 到 200：`VERY_HIGH_USER_IMPACT`；不能直接上线。

后续若决定实施，应采用 grandfathering、旧用户/旧 quote 保持快照、分阶段调价和有期限 promotion；不要自动把以上评估价格发布。

## Revenue / Margin Simulation

### Known-cost configuration required points

| provider cost | target margin | LOW ¥0.005 | BASE ¥0.0066 | HIGH ¥0.010 |
|---:|---:|---:|---:|---:|
| ¥0.60/image | 20% | 150 | 114 | 75 |
| ¥0.60/image | 30% | 172 | 130 | 86 |
| ¥0.60/image | 40% | 200 | 152 | 100 |
| ¥0.60/image | 50% | 240 | 182 | 120 |
| ¥0.60/image | 60% | 300 | 228 | 150 |
| ¥0.78/6s Grok | 20% | 195 | 148 | 98 |
| ¥0.78/6s Grok | 30% | 223 | 169 | 112 |
| ¥0.78/6s Grok | 40% | 260 | 197 | 130 |
| ¥0.78/6s Grok | 50% | 312 | 237 | 156 |
| ¥0.78/6s Grok | 60% | 390 | 296 | 195 |
| ¥4.00/5s Seedance 720p | 20% | 1000 | 758 | 500 |
| ¥4.00/5s Seedance 720p | 30% | 1143 | 866 | 572 |
| ¥4.00/5s Seedance 720p | 40% | 1334 | 1011 | 667 |
| ¥4.00/5s Seedance 720p | 50% | 1600 | 1213 | 800 |
| ¥4.00/5s Seedance 720p | 60% | 2000 | 1516 | 1000 |

### Historical scenario simulation status

`CURRENT vs OPTION A/B/C` 的最近 30 天 revenue、cost、gross profit、margin、涨跌任务数和分位数均 **NOT COMPUTABLE**：没有经过生产认证的任务分布。将来应以任务快照逐行重放三套规则，按 task 的实际 model/configuration、成功状态和 provider cost snapshot 计算，不能用总平均数替代。

## Risks

1. **计量风险：** point value 冲突会使所有毛利结论失真；优先修复并验证账单/任务快照口径。
2. **成本未知风险：** Preview、Seedance/Doubao 非 720p 没有成本，不得输出 margin 或自动发布。
3. **用户冲击风险：** 主要配置多项超过 50% 调整，必须灰度、祖父价或促销缓冲。
4. **路由风险：** provider A/B 成本不同；售价按 public model 和合理成本上限稳定，不跟路由跳价。
5. **产品解释风险：** quality/size 的价格差异目前主要是产品政策，不应伪装成供应商成本差异。
6. **数据风险：** 本地库不是生产证明；没有真实 usage mix 就不能判断主流配置，也不能承诺收入提升。

建议设计 `MIN_MARGIN_PERCENT`、成本缺失、负毛利和成本上涨的治理告警；第一阶段不自动阻断现有请求，除非沿用现有机制，且不在本报告中改变运行行为。

## Implementation Requirements

### Required Code Changes

1. `WHY_CODE_CHANGE_REQUIRED`：修正 `pointUnitAmountCents` 与正式 100 points/元口径的冲突，或将其改为明确的服务端 authoritative economics constant，并补充回归测试。
2. 这是 billing/economics 计量修复，不是重新设计 `GenerationPricingEngine`；`Calculate` 的 quantity、multiplier、CEIL 语义保持不变。
3. 只有在产品需要正式支持 low/medium/auto、2K/4K 且现有 schema/quote 不足时，才增加显式参数校验/规则字段；先确认 API contract，不加 fallback。

### Required Billing Rule Changes（未来实施，不在本阶段执行）

- 新建 `DRAFT` Billing Rule Version，显式填入 quality/size/duration/resolution tiers。
- 补齐对应 provider cost，尤其是 Preview 与非 720p/4K；未知成本保持 fail closed。
- `auto` 如采用 medium 映射，必须在 parameter rules 中显式表达并在 quote breakdown 中可见。
- 不 update 已发布历史规则，不改任务历史 snapshot，不直接 SQL update production。

## Migration / Versioning Strategy

遵循现有版本机制：

```text
DRAFT → VALIDATE → PUBLISH → EFFECTIVE → MONITOR
```

`xz_billing_rule_versions` 新建版本，设置 `effective_from`；旧 PUBLISHED 版本转 ARCHIVED 并保留 `effective_to`。旧任务继续读取 `billing_rule_version_id` 和 pricing snapshot。所有变更须通过既有 Admin/API 治理，不直接 DB 写入。

## Rollback Strategy

回滚应重新发布上一个已验证的安全 pricing version，或按项目当前正式发布机制恢复前一个版本；不覆盖历史订单、quote、任务或账本记录。发布前应先做 shadow simulation 和 30 天历史重放。

## Monitoring Plan

调价后按 7 天、30 天观察：generation volume、unique generation users、quote→submit conversion、insufficient points rate、recharge conversion、points consumed、provider cost、gross margin、negative margin、task failure rate，并按 model/configuration/provider channel/pricing version 分层。

Admin Analytics Dashboard（`feature/admin-analytics-dashboard`）建议未来增加只读指标：daily generation revenue、provider cost、gross profit、gross margin、negative margin count、model margin ranking、pricing version。本次没有修改该 worktree。

## Final Recommendation

推荐把 OPTION B Balanced 作为下一阶段 shadow simulation 的基线，但当前不能发布价格。进入实施前的硬门槛：

1. 提供已认证的生产只读数据或导出，完成 7/30 天 usage distribution 与 CURRENT/A/B/C 任务级重放。
2. 修复并验证 point value / estimated margin 单位冲突。
3. 为 GPT Image 2 显式发布 low/medium/auto、2K/4K 政策（或明确不支持），消除最高 tier fallback。
4. 补齐视频未知成本，并审查 20%/50% 高冲击配置的 grandfathering 和分阶段方案。

本阶段结论：`PRICING_OPTIMIZATION_BLOCKED_BY_DATA`

## Files

- 新增本报告：`IMAGE_VIDEO_PRICING_OPTIMIZATION_AUDIT.md`
- 未新增 simulation tool；场景计算已在本报告中以确定性公式和固定输入表呈现。
- 未修改 `admin-vue`、`apps/user-uni`、`backend-go`、数据库迁移或其他 worktree。

## Tests

- 已执行只读 Postgres schema/rule/cost/task/order/point 查询；所有写入事务均显式 `BEGIN READ ONLY` 并 `ROLLBACK`。
- 未运行完整 Go 回归套件：本阶段只新增审计文档，没有运行时代码变更；后续若修正单位或发布规则，必须运行 billing/pricing/provider-cost/generation quote 相关测试。

## Git

- 分支：`feat/image-video-price-optimization`
- 起始 HEAD：`4ade71e26332201db81f80b3f54d2e79c83bbd00`
- `origin/main`：`4ade71e26332201db81f80b3f54d2e79c83bbd00`
- 本阶段未发布价格、未部署、未合并、未 push。

---

# Phase 2 — Point Economics Audit and Safe Remediation

## Executive Summary

第一阶段发现的 P0 已完成单位追踪和最小修复。商业口径由商品配置、充值 SKU 和钱包配置共同证明：`100 points = ¥1`，即 `1 point = ¥0.01 = 1 CNY cent`。旧实现使用 `pointUnitAmountCents = 10`，把积分转人民币的 margin/revenue 计算放大约 10 倍。

根因分类：`CONSTANT_BUG` + `MARGIN_ONLY_BUG`。没有证据表明历史上正式积分价值曾变更，因此不是 `HISTORICAL_SEMANTICS_DRIFT`；变量名本身表达的是 cents，故不是 `UNIT_NAMING_BUG`。用户实际扣积分不依赖该常量，结论为 `USER_CHARGE_UNAFFECTED`，不是 `BILLING_CRITICAL_BUG`。

本阶段只修正 CNY cents 侧的统一常量和相关 margin/reporting 计算，未修改任何正式售价、quote、reserve、capture、release、积分账本或已发布 Billing Rule。当前不发布新价格。

## UNIT TRACE

| 层 | 位置/字段 | 实际单位 | 是否依赖 point value |
|---|---|---|---|
| 商品口径 | `xz_billing_config.CREDITS_PER_CNY_YUAN=100`；充值 SKU 100/10000、400/40000 | points / CNY | 证明 1 point=1 cent |
| 售价计算 | `GenerationPricingEngine` / `PointCost` | points | 否；只产生应扣积分 |
| Quote | quote response `pointCost` | points | 否 |
| Reserve/Capture/Release | `RequestedPoints`、`Points`、wallet ledger | points | 否；不做人民币换算 |
| 统一转换 | `backend-go/internal/httpserver/store.go` 的 `pointNominalValueCents` | CNY cents / point | 是；当前值 1 |
| Generation/PPT/Recharge 事件 | `UnitAmountCents`、`AmountCents` | CNY cents | 是 |
| Task snapshot | `UserChargeAmount`、`UpstreamCost`、`PlatformProfit` | CNY cents | 是（仅报告/快照侧） |
| Task snapshot | `SupplierCost`、`EstimatedMargin` | decimal CNY | 通过 cents 计算后除以 100 |
| Provider cost | `xz_provider_costs.unit_cost` | decimal CNY / billing unit | 不使用 point value；由 `providerCostCents` ×100 转 cents |
| Admin/Reporting | `amountCents`、`minimumRevenue`、knowledge billing | CNY cents | 是 |
| 数据库 | `supplier_cost`、`estimated_margin` numeric；`upstream_cost`/`platform_profit` cents | 如左 | snapshot 写入后不可变 |

调用链核验：`generationQuoteForRequest → pricingdomain.Calculate → PointCost → reserve/capture/release` 全程使用 points；`pointNominalValueCents` 不在这条链上。旧的 `pointUnitAmountCents` 引用已统一替换，未建立第二套 Pricing Engine。

## Point Value

名义积分价值：`NOMINAL_POINT_VALUE = 1 CNY cent / point`。有效收入仍受赠送积分、套餐和折扣影响，第一阶段沿用已审计情景：`LOW ¥0.005`、`BASE ¥0.0066`、`HIGH ¥0.010` / point；生产购买、赠送、活动和企业额度的完整归因仍需只读生产数据验证。

## Current Wrong Formula

修复前实际公式为：

```text
revenue_cents = captured_points × 10
gross_profit_cents = revenue_cents - supplier_cost_cents
margin = gross_profit_cents / revenue_cents
```

同一错误常量还影响 `UserChargeAmount` 的 CNY 报告字段、Billing Center `minimumRevenue`、billing event、Admin amount 以及 knowledge billing。它没有影响 points 字段本身。

## Correct Formula

当前统一定义为：

```text
pointNominalValueCents = 1
revenue_cents = captured_points × pointNominalValueCents
gross_profit_cents = revenue_cents - supplier_cost_cents
margin = gross_profit_cents / revenue_cents, when revenue_cents > 0
```

Provider cost 仍是 `xz_provider_costs.unit_cost` 的 decimal CNY；例如 5 秒 × ¥0.80/秒 = ¥4.00 = 400 cents。不存在把供应商成本当 points 的转换。

## User Charge Impact

`USER_PRICING: UNAFFECTED`、`QUOTE: UNAFFECTED`、`RESERVE: UNAFFECTED`、`CAPTURE: UNAFFECTED`、`RELEASE: UNAFFECTED`。证据是上述链路只传递 `PointCost`/`RequestedPoints`/`Points`，不会读取 `pointNominalValueCents`。因此 600 points 仍 reserve/capture/release 600 points；没有改变用户实际扣积分。

## Ledger Impact

`POINTS_LEDGER: UNAFFECTED`。账户余额、冻结余额、ledger entry 的 points 整数算法未修改；没有 migration、批量改账或生产数据库写入。

## Margin Impact

`MARGIN: AFFECTED`，`PROVIDER_COST: UNAFFECTED`（仅其进入 cents 计算的消费方受益）。修复后 1、10、100 points 分别对应 1、10、100 revenue cents；100 points + 60 cents 成本得到 40 cents gross profit、40% nominal margin。100 points + 400 cents 成本得到 -300 cents gross profit，margin 为 -300%。

## Reconciliation Impact

`RECONCILIATION: AFFECTED`，仅影响异常解释和新计算，不回写历史异常。`NEGATIVE_MARGIN` 的规则校验和任务 margin 读取过去可能因旧常量产生 false negative；例如 GPT Image 2 的 10-point base 在旧口径看似 100 cents、成本 60 cents，在正确口径是 10 cents、确实亏损。此前记录的 5 条 `NEGATIVE_MARGIN` 不自动更改，需标记为 legacy-unit interpretation，后续只读重放后再判断。

## Historical Snapshot Impact

`supplier_cost`、`estimated_margin`、pricing snapshot 和 provider cost snapshot 已按任务保存；Postgres upsert 对已有 supplier cost 使用保留旧值的逻辑，`applyTaskSupplierCost` 也不会覆盖非空快照。本修复不 UPDATE historical tasks、不批量重算历史任务。新口径从本修复 commit 生效；历史快照仍是历史事实，不能用新公式覆盖。

## Affected Surface

| Surface | Status | 说明 |
|---|---|---|
| USER_PRICING | UNAFFECTED | 正式 points 售价未改 |
| POINTS_LEDGER | UNAFFECTED | 余额和账本未改 |
| QUOTE | UNAFFECTED | PointCost 未改 |
| RESERVE / CAPTURE | UNAFFECTED | 请求积分未改 |
| PROVIDER_COST | UNAFFECTED | 成本来源和单位未改 |
| MARGIN | AFFECTED | CNY cents 换算修正 |
| RECONCILIATION | AFFECTED | 新异常解释使用正确口径 |
| ADMIN_ANALYTICS | AFFECTED | amount/margin 展示计算修正 |
| REPORTING | AFFECTED | billing event / knowledge billing 修正 |

## Safe Remediation

修改集中在既有 HTTP server billing/reporting consumers：`store.go`、`ai_capability.go`、`billing_v1_store_json.go`、`admin_api.go`、`knowledge_billing.go`、`postgres_store.go`。只将散落的 10 改为唯一明确语义的 `pointNominalValueCents = 1`，未重构 billing 模块，未修改 `GenerationPricingEngine`，因此 `NO_ENGINE_CHANGE_REQUIRED`。

## Updated Pricing Simulation

这是 shadow re-simulation，不是生产价格建议发布。成本输入为 GPT Image 2 ¥0.60、Grok 1.5 6 秒 × ¥0.13 = ¥0.78、Seedance/Doubao 5 秒 × ¥0.80 = ¥4.00。修复后的 nominal revenue 与目标 margin 所需 points 如下：

| Cost | Effective point value | 20% | 30% | 40% | 50% | 60% |
|---|---:|---:|---:|---:|---:|---:|
| GPT ¥0.60 | LOW .005 | 150 | 172 | 200 | 240 | 300 |
| GPT ¥0.60 | BASE .0066 | 114 | 130 | 152 | 182 | 228 |
| GPT ¥0.60 | HIGH .010 | 75 | 86 | 100 | 120 | 150 |
| Grok ¥0.78 | LOW .005 | 195 | 223 | 260 | 312 | 390 |
| Grok ¥0.78 | BASE .0066 | 148 | 169 | 197 | 237 | 296 |
| Grok ¥0.78 | HIGH .010 | 98 | 112 | 130 | 156 | 195 |
| Seedance ¥4.00 | LOW .005 | 1000 | 1143 | 1334 | 1600 | 2000 |
| Seedance ¥4.00 | BASE .0066 | 758 | 866 | 1011 | 1213 | 1516 |
| Seedance ¥4.00 | HIGH .010 | 500 | 572 | 667 | 800 | 1000 |

公式：`required_points = ceil(cost_cny / (effective_revenue_per_point × (1-target_margin)))`。视频成本先按现有 engine 的 PER_SECOND rounding 语义计算；本阶段不改变 rounding algorithm。旧报告中的 GPT `120–500`、Grok `240`、Seedance `1200` 只能作为简化 ladder 参考，不能直接沿用为新生产价格。

## Tests

新增 deterministic regression 覆盖 1、10、100 points，以及 provider cost ¥0.60 和 ¥4.00；验证 revenue cents、supplier cost cents、gross profit cents 和 margin 输入。既有 provider-cost 测试同步改为正确 1-cent 口径。目标测试通过：

```text
go test ./internal/httpserver -run 'Test(PointEconomics|BillingCenterV1Acceptance|GPTImageBillingRulePhase26Draft|ApplyTaskSupplierCostUsesCents|AssessMarginHealth)' -count=1
ok   xianzhi-ai/backend-go/internal/httpserver  2.874s
```

修正后的 GPT draft 测试现在预期 `NEGATIVE_MARGIN`，因为正确口径揭示该 draft 的成本问题；没有发布该 draft。全包测试中仅剩需要本地 Postgres `127.0.0.1:55441` 的集成测试无法运行，属于环境缺失，不是本修复失败。

## Remaining Data Gap

保持 `PRODUCTION_USAGE_DATA_UNVERIFIED`。本地数据库最新任务为 2026-08-11，不能作为当前生产 7/30 天 usage。没有伪造 MOST_USED_CONFIGURATION、涨跌价任务比例或真实收入分布；后续应以只读生产 extraction 做 shadow replay。

## Implementation / Versioning / Rollout

未来若批准调价，只能新增 versioned Billing Rule：`DRAFT → VALIDATE → PUBLISH → EFFECTIVE → MONITOR`，保留旧版本和历史 task pricing snapshot；回滚通过重新发布前一个安全版本，不 UPDATE 历史规则或任务。本阶段 `NO_BILLING_RULE_PUBLISH`、`NO_PRODUCTION_MUTATION`、`NO_DEPLOY`。

## Final Phase 2 Decision

`POINT_ECONOMICS_REMEDIATION_READY`

Point economics 已完成安全修复和回归；正式用户价格仍未改变，下一步仅能在生产只读 usage 数据和业务批准后进入独立的价格方案评审/发布流程。
