# Sandbox runtime window — 2026-07-29

## Verdict

| Item | Status |
|---|---|
| SANDBOX V2 seed (4 plans + 4 goods + 4 bindings + TEST whitelist) | **DONE** |
| Temporary `WECHAT_VIRTUAL_PAY_ENV=sandbox` window | **DONE** (~07:34:32→07:34:55 +08) |
| Automatable sandbox quotes | **PASS** (NORMAL 99600 + TEST 100; U0 TEST 403) |
| Restore production pay env | **PASS** — container + `.env` = `production`; V2 flags remain **true** |
| Real-device sandbox pay | **NOT RUN** — payment pipe already ACCEPTED via PRODUCTION ¥1 TEST; no ¥996 charge |

## Window timeline

1. Precheck: flags true/true/true, pay=production  
2. Dry quotes PRODUCTION baseline  
3. Switch pay env → sandbox; recreate; healthy  
4. Quotes SANDBOX: MEMBER/AGENT NORMAL 201@99600; TEST 201@100; U0 TEST 403  
5. Restore pay env → production; recreate; healthy  
6. Quotes PRODUCTION after restore still 201@99600 / 201@100  

Evidence: `host-out/` (DONE.txt, 01-switch-sandbox.txt, 03-restore.txt, quotes-*.txt, quote-*-sandbox.json).  
Seed inventory: `created-inventory.json`.

## Note

`pricing-health-sandbox.json` returned `{"error":"not found"}` during the short window (admin health route miss or auth scope) — **not** used as PASS. Quote matrix is the acceptance evidence for this window.

## Residual vs full §5 matrix

§5 #5/#7/#8/#10 **CLOSED** 2026-07-29T09:57+08 — see `../section5-probes/`. Payment path + NORMAL config substituted per user policy (see `../normal-996/README.md`). Overall gate still **NO-GO** pending final signatures.
