# §5 remaining probes — #5 / #7 / #8 / #10 — 2026-07-29

Host: `root@119.29.191.227` · Image `sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32` · pay env **production** · V2 flags **true/true/true**

## Verdict

| # | Case | Result | Evidence |
|---|---|---|---|
| 5 | 重复履约幂等（re-sync×2） | **PASS** | `host-out/05-idempotency.txt` — MEMBER memb=1/tok=1；AGENT memb=0/tok=1；token bal 60793 不变 |
| 7 | quote 后白名单失效 | **PASS** | soft-expire → checkout **403 `PRICE_PLAN_NOT_ELIGIBLE`**；orders 27→27；quote 仍 AVAILABLE；恢复：admin 新建 ACTIVE whitelist（旧 entry EXPIRED immutable） |
| 8 | 价格差 1 分 | **PASS** | binding `provider_price_snapshot_cents` 100→101 → quote **409 `PRICE_PLAN_WECHAT_PRICE_MISMATCH`**；立即恢复 100；quote 201。注：直接改 good.platform_price_cents 被 `ck_xz_wechat_goods_manual_confirmation_098` 拒绝（快照锁价） |
| 10 | V1 历史订单回归 | **PASS** | V1 CLOSED status **200**；legacy `TOKEN_CUSTOM_1YUAN` productCode create **201**（未支付，随即 CLOSED）；V2 两单仍 SUCCESS。CLOSED sync 返回 500=`openid错误`（微信查单，terminal CLOSED 非阻断） |

`DONE.txt`: `IDEM_PASS=1 WL7_PASS=1 MM8_PASS=1 V10_PASS=1 PAY_ENV=production`

## Substitutions / honesty

- **#5** 不是微信 push 风暴并发压测；用已履约 ¥1 两单 **官方 sync×2** 证明 Grant 幂等（无双发 Token/会员）。
- **#7** soft-expire 使原 whitelist 进入 **EXPIRED immutable**；不得回写 expires_at；已用 admin `POST .../whitelist` 重建 ACTIVE（`price_plan_whitelist_20260729015641000000000_7e4a20c8`）。AGENT whitelist 未动。
- **#8** 用 binding +1 分触发链校验（与 good/plan 三角强制等式同一错误码）；未永久改价。
- **#10** 无现网 V1 `PAID` 样本；用 status + legacy create 证明 V1 分支仍活；未发明 device PASS。
- **未**收 ¥996；**未**永久切 sandbox；**未** `--build`/retag。

## Scripts

- `probe-all.sh` — #5+#7 start（#7 restore 曾撞 immutable）
- `probe-recover-and-finish.sh` — whitelist 恢复 + #8/#10 首轮
- `probe-finish-8-10.sh` — #8 binding bump 成功 + #10 legacy create
- `finalize-health-redact.sh` — 脱敏 quoteId/signData

## Health note

本窗 `GET /api/v1/admin/pricing/health` 对短 Redis admin session 返回 `{"error":"not found"}`（与 sandbox 窗相同现象）。**不以本窗 health 作为 PASS**；白名单 ACTIVE + TEST quote 201 + NORMAL quote 201@99600 作为运行时健康替代证据。历史 `test-whitelist/pricing-health.json` 仍为 HEALTHY / blocked=0。
