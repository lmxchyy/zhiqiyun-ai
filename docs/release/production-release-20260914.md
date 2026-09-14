# 先知 AI 生产发布完成与交付验收报告 (Production Release Note)

- **发布时间**：`2026-09-14 21:10:00 CST`
- **发布状态**：**`RELEASED / IN_PRODUCTION` (成功发布)**
- **发布 Commit SHA**：`9bb7de8a8d3ec1bc2dbe356ed5d7e186248dd39a`
- **不可变镜像 Digest**：  
  `sha256:aef4a9eb2172b074efae54ffc3095d1f6a1c78e265f3458f55ac4d3ee8572ddc`  
  - 镜像完整地址：`crpi-kvtl7houjvj5ftba.cn-hangzhou.personal.cr.aliyuncs.com/zhiqiyun-ai/zhiqiyun-ai@sha256:aef4a9eb2172b074efae54ffc3095d1f6a1c78e265f3458f55ac4d3ee8572ddc`
  - 镜像 Tag：`git-9bb7de8a8d3ec1bc2dbe356ed5d7e186248dd39a`
- **回滚镜像 (Rollback Baseline)**：  
  `sha256:2f330d87fba9b33f8d5be6f7dc71512bf8a231102cd5fc09ede95ca052ad87bb`  
  - 本地镜像 ID：`sha256:98cdea6742f1b5ef02367803f60c517cf9d36fa5a7ae45b66f0d17f49391939a`（保留在生产主机未清理）
- **回滚触发情况**：**`NOT USED` (零回滚，发布一次性通过)**

---

## 一、本次发布交付能力总览

本次生产发布合入了以下关键能力与稳定性保障：

1. **#124 GPT Image 2K/4K 分辨率阶梯计费对齐**：
   - 彻底修复 2K 与 4K 尺寸退化到相同 12 积分的封顶回退缺陷；
   - 统一使用 `tier_1k` / `tier_2k` / `tier_4k` 映射，实现 10 / 15 / 20 积分的权威阶梯计价；
   - 数据库迁移 `118-gpt-image-billing-tier-v2.sql` 已生效。
2. **#125 管理后台 GPT Image 计费规则结构化配置与安全发布**：
   - 管理端安全查看与发布新规则版本，阻止负毛利与跨 Tier 异常；
   - 历史已完成账单金额强隔离，不受新规则影响。
3. **#120 生图页面服务端 Quote 预估积分实时联动**：
   - Web 后台与微信小程序在生成前调用服务端 Quote 实时展示 `预计消耗：X 积分`；
   - 前端 100% 依赖服务端返回，杜绝本地自行计费与过期跳闪。
4. **#121 PPT 任务持久化 `tenant_id` 约束修复**：
   - 修复生产环境创建 PPT 时 `tenant_id violates not-null constraint` 问题；
   - 迁移 `117-ppt-tasks-tenant-id.sql` 已生效。
5. **#129 (SEC-02) 底层 Keyring 密钥管理与安全双读架构**：
   - 引入 `enc:v2:<key_id>:` 紧凑 Envelope 与 Keyring 双读架构，向后兼容 `enc:v1:`；
   - 交付 `cmd/secret-reencrypt` 批量安全重加密工具（带 `--dry-run` 与 CAS 乐观锁）；
   - 交付生产重加密运维 Runbook，为后续零停机密钥轮换铺平技术道路。
6. **#117 DR-01 华为云 OBS 异地备份与干净恢复演练闭环**：
   - 独立 PostgreSQL 16 干净测试库实测 21 秒完成 281MB 全量恢复，286 张表与钱包账本 100% 平账；
   - 生产本地与华为云 OBS 异地备份包 SHA256 逐字吻合，`OFFSITE_VERIFIED` 状态确认。
7. **#116 PAY-01 微信虚拟支付 2.0 生产环境就绪核验**：
   - 生产接口探活正常 (`paymentCapability: "available", status: "READY"`)；
   - 回调端点实测通过微信官方 SHA1 签名握手与 `echostr` 校验。

---

## 二、发布后健康检查 (Health Checks)

| 检查端点 | 请求方式 | 返回状态 | 响应结果 | 判定 |
|---|---|---|---|---|
| `/healthz` | GET | `HTTP 200` | `{"service":"xianzhi-ai-go-gin","status":"ok"}` | **PASS ✅** |
| `/api/v1/health` | GET | `HTTP 200` | `{"service":"xianzhi-ai-go-gin","status":"ok"}` | **PASS ✅** |
| `/api/v1/payment/capability?platform=mp-weixin` | GET | `HTTP 200` | `{"platform":"mp-weixin","paymentCapability":"available","paymentStatus":"READY","enabled":true}` | **PASS ✅** |
| `/api/v1/payment/wechat-virtual/notify` | GET (带签名) | `HTTP 200` | `hello_wechat` (原样回显，验签逻辑生效) | **PASS ✅** |
| 容器内部健康状态 | Docker Inspect | `healthy` | `xianzhi-ai` 与 `smartvideo-worker` 状态均为 `running healthy` | **PASS ✅** |
| 基础服务连接池 | 容器启动日志 | `connected` | PostgreSQL 零连接错误；RabbitMQ 握手成功；Redis 正常 | **PASS ✅** |

---

## 三、关键业务冒烟测试 (Smoke Tests)

1. **用户鉴权与 Token 签发**：
   - 接口：`POST /api/v1/auth/login`
   - 结果：`HTTP 200`，AccessToken 正常签发，用户身份及权限集解析正常。
2. **AI 生图阶梯报价实测**：
   - 接口：`POST /api/v1/generation-tasks/quote` (基于新部署镜像实测)
     - `1K (1024x1024), low, n=1` -> **10 积分** (Match: True, Sufficient: True)
     - `2K (2048x2048), low, n=1` -> **15 积分** (Match: True, Sufficient: True)
     - `4K (3840x2160), low, n=1` -> **20 积分** (Match: True, Sufficient: True)
   - 结果：报价契约 100% 符合 Issue #124 既定发布标准。
3. **用户控制台与资产数据加载**：
   - 接口：`GET /api/v1/user/dashboard`
   - 结果：`HTTP 200`，历史 30 条生图与生视频任务、积分账本正常呈现。
4. **异步任务工作节点 (smartvideo-worker)**：
   - 结果：正常常驻运行，各并发消费线程处于待命监听状态 (`analysis_concurrency=2, render_concurrency=1`)。

---

## 四、合规留痕与 Release Control 终态

- **安全凭据决策**：
  依据 `docs/acceptance/evidence/SEC-01-risk-acceptance-record.md`，安全负责人已签署正式 Risk Acceptance，声明本次发布不执行全量物理轮换，现有凭据无已知泄露，后续转入日常维护窗口排期。
- **全量门禁闭环**：
  Release Control 看板中的全部 11 项 Issue 已全部闭环为 `Done`。
- **发布状态决议**：
  **Release Control 正式流转至 `Released / Production`。**
