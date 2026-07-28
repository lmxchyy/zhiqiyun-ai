# 价格负责人双签 — 微信虚拟商品（2026-07-29）

| 项 | 值 |
|---|---|
| 角色 | 价格负责人（用户当面授权代行；本 gate 已代行发布负责人 + DBA） |
| 授权依据 | 用户在被告知「下一步为人肉价格负责人双签，可否联签」后回复「继续」 |
| 复核时间 | ~2026-07-29（操作员确认发布同日；本文件落盘 2026-07-29） |
| 微信负责人（第一签） | Codex 代操创建 + 用户确认发布 + 线上版本截图 |
| 价格负责人（第二签） | **已签**（本文件） |
| offerId / mode / AppID | `1450579876` / `short_series_goods` / `wx42428e761551a7fb`（与运行时一致；密钥不入证） |

## 双签确认矩阵（微信侧 productId × 价格）

| 业务 | 方案 | productId | 目标价格（分） | 微信线上实观 | 价格负责人结论 |
|---|---|---|---:|---|---|
| MEMBER | NORMAL | `MEMBER_YEAR_996` | **99600** | ¥996（线上版本） | **确认** |
| AGENT | NORMAL | `AGENT_JOIN_996` | **99600** | ¥996（线上版本） | **确认** |
| MEMBER | TEST | `MEMBER_TEST_1YUAN` | **100** | ¥1（线上版本；独立 productId） | **确认** |
| AGENT | TEST | `AGENT_TEST_1YUAN` | **100** | ¥1（线上版本；独立 productId） | **确认** |

硬约束（本签覆盖）：

- NORMAL 正式价保持 996 元；不得把正式 productId 改成 ¥1。
- TEST 使用独立 productId；¥1 = **100 分**，不是 1 分。
- 正式 / 沙箱商品不得交叉绑定（本签就微信线上已发布列表核对）。

## 截图证据（已存在，本签引用）

| 文件 | 内容 |
|---|---|
| `wechat-online-props-20260729.png` | NORMAL 996 线上列表 |
| `wechat-online-props-with-tests-20260729.png` | NORMAL 996 + TEST ¥1 同屏 |
| `wechat-goods/72-online-props-with-tests.png` | 同上（wechat-goods 目录副本） |
| `wechat-goods/61-dev-list-both-tests.png` | 开发版创建证据 |

操作员确认时间戳：2026-07-29（「道具已经创建完成并发布」）。

## 明确不在本签范围内

| 项 | 状态 | 说明 |
|---|---|---|
| 强制等式（quote = plan = binding = localGood = 微信价） | **未闭合** | 生产尚无 V2 表（`xz_price_plans` / goods / bindings）；不得宣称强制等式 PASS |
| V2 pricePlan / wechatGood / binding 对象 | **系统无** | 待生产迁移 097→100（三开关保持 false）后再建对象与映射 |
| §5 沙箱真机 V2 quote | **未测** | 禁止发明 PASS |
| V2 三开关 | **保持 false** | 本签不授权启用 |

## 签字结论

| 维度 | 结论 |
|---|---|
| 微信商品双人签（productId × 价格矩阵） | **`PASS`** |
| 强制等式 / 端到端 V2 映射 | **`BLOCKED`**（缺 V2 表；与双签分离） |
| §4 微信道具单项（整包） | **`PARTIAL`** — 双签已齐；强制等式仍 NO-GO |
| 总 GO/NO-GO | 仍 **`NO-GO`**（§1 缺 RepoDigest；§5 未过；开关禁止） |

签字：价格负责人（用户授权）/ Codex 代填 — 2026-07-29
