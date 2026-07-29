# Product Owner — production gate GO signature

**PO_GO_AT:** 2026-07-29T10:03:20+08:00  
**Decision:** **`GO`** (with residual conditions below)  
**Verbatim intent:** 「接受 §5 替代并签字 GO」  
**Signer:** Product Owner（用户会话正式决策）  
**ExecutedBy:** Codex agent（docs-only；未改生产开关 / 未部署 / 未改价）

## What PO accepted

| Item | Status | Notes |
|---|---|---|
| §5 overall | **`PASS-WITH-SUBSTITUTIONS` ACCEPTED** | 清单 1–10 已关；见 `section5-probes/`、`HANDOFF-ROLE-EXECUTION-PACK.md` §5 |
| §5 #5 substitution | **ACCEPTED** | 幂等证明 = 已履约两单服务端 `POST .../sync`×2；**不是**微信 push 风暴并发压测 |
| §5 sandbox 真机付缺失 | **ACCEPTED** | 由生产 ¥1 TEST MEMBER+AGENT 支付链路 ACCEPTED + `POLICY-NO-REAL-996.md` 覆盖；sandbox dry quote 窗已 PASS 并恢复 production |
| Overall gate | **`GO`** | 最终签字栏已填；条件/残差见下 |

## Evidence basis (FULL SHAs — do not truncate)

```text
docsGoCommit (FULL)          = 719f898c5ca160348ca5d597f9644901d0a60242
  subject = docs(pricing): PO accept §5 substitutions; declare production gate GO
basisDocsParentCommit (FULL) = 4fc14fffe1acc534b573c2baf4e60bf78caf85e1
  subject = docs(pricing): close §5 #5/#7/#8/#10 probes; keep overall NO-GO

releaseGitSha (FULL)         = a39485ef159dabf348a71059a0e922af4894ab5a
imageId       (FULL)         = sha256:1bd6777d671bddbe0bab226bd2f508be3e1179e0a99f53076a408dd3c4bd7a32
imageRef                     = local/xianzhi-ai-platform:a39485ef1
currentTarSha256 (FULL)      = 4341d6b1cdac84d83fb2962729ac654684d2ec0ff90660f527da992016378d09
previousImageId (FULL)       = sha256:ead3963844183429a30fc20f6a69eefaf264df882afa425c8e406502b242a331
previousTarSha256 (FULL)     = 4c08c51a41d6fc527e854c4e4c64988a35e9773622b5506433eb4d5a76f9a9ee
repoDigest                   = null (CONFIRMED ABSENT; never invent)
```

§1 amendment: `repo-digest/AMENDMENT-LOCAL-IMMUTABLE.md`（PO ACCEPTED 2026-07-29T09:44:00+08:00）。  
§5 probes closed: `section5-probes/`（2026-07-29T09:57+08）。  
支付链路: `member-test-pay/` + `agent-test-pay/` + `deliver-notify/`。  
NORMAL ¥996: `POLICY-NO-REAL-996.md` + `normal-996/`（WAIVED）。

## GO conditions / residual operational notes

1. **Registry 仍为长期标准** — §1 = `PASS-WITH-LOCAL-IMMUTABLE`；本轮允许无 `repository@sha256:...`；引入 registry 后 Gate A 镜像行必须改回真实 RepoDigest。
2. **本地不可变约束继续生效** — 禁止 `docker compose up -d --build`；禁止同 tag 覆盖重建；部署须核验 FULL IMAGE_ID。
3. **NORMAL ¥996 真机付 = WAIVED** — dry quote 201@99600 + bindings；不以真实收费为门禁。
4. **Sandbox 真机付未跑** — 已由生产 ¥1 TEST 两单 + 政策接受；本 GO **不**授权发明 sandbox device PASS。
5. **#5 幂等替代诚实声明** — re-sync 幂等 ≠ push 风暴；后续若需 push 并发专项，另开变更单。
6. **第三阶段（退款/补偿/人工补发 V2）= OUT OF SCOPE** — 本 GO 不含 phase3。
7. **本签字动作本身** — **未**改 `SNAPSHOT_V2_*` / creation / TEST 开关；**未**改 `WECHAT_VIRTUAL_PAY_ENV`；**未**改价；**未**部署。现网保持 true/true/true + production（以运维实读为准）。

## Role signatures (honest)

| Role | Signature |
|---|---|
| 应用负责人 | 用户授权代行（会话；§1–§6 技术证据已回填） |
| DBA | 用户授权代行（§2/§3 PASS + 生产 097–100 APPLIED） |
| 微信支付负责人 | 用户授权代行 + 价格负责人双签（§4 PASS；¥1 ACCEPTED） |
| 安全负责人 | 用户授权代行（Gate A RBAC/secrets 证据；无新密钥入仓） |
| 业务负责人 / Product Owner | **SIGNED GO** — 「接受 §5 替代并签字 GO」@ 2026-07-29T10:03:20+08:00 |
| 发布负责人 | 用户授权代行（§1 local-immutable + §6 开关窗 DONE） |

**最终决定：** `GO`  
**决定时间：** 2026-07-29T10:03:20+08:00  
**变更单号：** PO-GO-20260729-§5-SUBSTITUTIONS（用户 verbatim「接受 §5 替代并签字 GO」）

## Cross-links

- `../../go-no-go-gate.md` — 总状态翻为 `GO`
- `../../HANDOFF-ROLE-EXECUTION-PACK.md` — §5 签字 + §7 汇总翻为 `GO`
- `section5-probes/README.md` — #5/#7/#8/#10 CLOSED
- `repo-digest/AMENDMENT-LOCAL-IMMUTABLE.md` — §1 PO ACCEPTED
- `POLICY-NO-REAL-996.md` — NORMAL ¥996 WAIVE
