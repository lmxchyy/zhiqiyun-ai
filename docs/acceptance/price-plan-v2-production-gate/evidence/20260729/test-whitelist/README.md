# TEST 白名单写入（清 `TEST_WHITELIST_MISSING`）— 2026-07-29

| 项 | 值 |
|---|---|
| 授权/目标 | 清 pricing-health `TEST_WHITELIST_MISSING`；不发明用户；不切 sandbox |
| 主机 | `root@119.29.191.227` `/opt/zhiqiyun-ai` |
| API | `POST /api/v1/admin/price-plans/:id/whitelist`（短时 Redis admin session） |
| 支付环境 | **保持** `WECHAT_VIRTUAL_PAY_ENV=production`（未改） |
| V2 三开关 | **保持** `true/true/true`（未改） |

## 选用账号（真实 xz_users，未造号）

| userId | 邮箱 | 昵称 | 选用理由 |
|---|---|---|---|
| `user_000002` | **`demo@xianzhi.ai`** | 演示用户 | 邮件查找确认 `email=demo@xianzhi.ai` → 即本账号；系统既有演示/支付测试账号；`wechatOpenIds` 已绑定（2）；近期 MEMBER/AGENT 虚拟支付下单最多；artifacts 多处以此为小程序登录身份 |

**2026-07-29 复核：** 用户点名 `demo@xianzhi.ai` → 解析为 `user_000002`；MEMBER/AGENT TEST 白名单 **已 ACTIVE**（无需重复创建）；pricing-health 再查仍 **HEALTHY**。证据：`00-resolve-demo-email.txt`。

同一账号同时写入：

| 方案 | pricePlanId | product 语义 | whitelistEntryId |
|---|---|---|---|
| MEMBER_TEST | `price_plan_20260728212634000000000_049a91b1` | `MEMBER_TEST_1YUAN` @100 | `price_plan_whitelist_20260728214350000000000_3fb9334c` |
| AGENT_TEST | `price_plan_20260728212634000000000_2ec1c485` | `AGENT_TEST_1YUAN` @100 | `price_plan_whitelist_20260728214350000000000_557f3354` |

创建 HTTP：**201**；list：**ACTIVE**；DB `enabled=true`。

## pricing-health 复核

| 字段 | 结果 |
|---|---|
| HTTP | 200 |
| `status` | **HEALTHY** |
| `blockedIssueCount` | **0** |
| `TEST_WHITELIST_MISSING` | **0**（已清除） |

证据：`pricing-health.json`、`03-health.log`、`01-create.log`、`05-db.txt`。

## 未做 / 约束

- **未**翻转 `WECHAT_VIRTUAL_PAY_ENV`
- **未**发明沙箱真机 PASS / device PASS
- **未**改 V2 三开关
- 真机支付仍待另窗（需 sandbox 运行时或明确生产试付策略）

## 对测试同学下一步（小程序）

用已登录的 **「演示用户」** 账号即可（无需再填 openid/表单）。在体验版打开会员/代理购买页：若走 TEST 入口应能 quote 到 ¥1；正式入口仍应是 ¥996。非白名单账号仍看不到/买不到 TEST 价。
