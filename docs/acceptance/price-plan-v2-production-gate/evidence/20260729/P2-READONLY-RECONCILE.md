# P2 — 生产只读对账（下一步立刻做）

**状态：** **PASS**（2026-07-29T10:52:20+08:00）  
**执行人：** 用户（运维+DBA）+ agent 代跑只读脚本  
**主机：** `root@119.29.191.227` `/opt/zhiqiyun-ai`  
**禁止项遵守：** 未部署、未迁移、未改开关、未操作微信、未落盘 Env 明文

## 目标（只确认三件事）— 结果

| # | 目标 | 结果 |
|---|---|---|
| 1 | 镜像 `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32`（`a39485ef…`） | **PASS** `A_IMAGE_MATCH=EXACT`；`local/xianzhi-ai-platform:a39485ef1` |
| 2 | 三开关 `true/true/true` + `WECHAT_VIRTUAL_PAY_ENV=production` | **PASS** |
| 3 | 097–100 `still_not_valid=0`；`AGENT_JOIN_996@99600`；TEST `@100` | **PASS** |

## 证据

- `p2-reconcile/p2-readonly.txt`
- `p2-reconcile/p2-db.out`
- 脚本：`p2-run-on-prod.sh`

## 现场摘要

```text
A_IMAGE_MATCH=EXACT
B_FLAGS_RESULT=PASS
C_STILL_NOT_VALID_097_100=0
C_AGENT_JOIN_996_CENTS=99600
C_AGENT_TEST_1YUAN_CENTS=100
P2_OVERALL=PASS
```

代理方案行：`pp_agent_normal_prod_996` ACTIVE default @99600；`pp_agent_test_prod_entry` ACTIVE non-default @100。  
微信商品 PRODUCTION+SANDBOX：`AGENT_JOIN_996`/`MEMBER_YEAR_996`=99600；`*_TEST_1YUAN`=100；均为 PUBLISHED。

说明：点查 `to_regclass('xz_price_plan_versions')` 为 false（表名可能不同或未用该名）；**不阻断** P2——097–100 命名约束 19 条均为 `convalidated=t`，核心业务表与价位齐全。

## 结论回填

| 字段 | 填写 |
|---|---|
| 执行时间 | 2026-07-29T10:52:20+08:00 |
| 执行人 | 运维+DBA（用户）/ agent 只读代跑 |
| A 镜像 | **PASS** |
| B 开关 | **PASS**（true/true/true + production） |
| C 约束 | **PASS**（still_not_valid=0；价位正确） |
| 总判 | **PASS** |
| 证据路径 | `evidence/20260729/p2-reconcile/` |

## 下一步

**P3 + P4：** 价格负责人 + 微信支付负责人真人确认（代理正式价 ¥996；微信后台 productId/价/发布状态）。  
然后 **P6** 最终 GO 签字。总门禁在 P3–P5/签字前仍为 **NO-GO**。
