# Git 分支治理 Phase 2：本地安全清理报告

- 报告文件日期：2026-08-14
- 实际执行环境日期：2026-08-15（Asia/Shanghai）
- 仓库：E:/code/work/先知AI
- 依据：docs/git-branch-governance-audit-2026-08-14.md
- 允许范围：仅 7 个 SAFE_TO_CLEAN_CANDIDATE
- 实际结果：安全验证通过，但本地删除均未成功；没有使用任何强制或绕过手段

## 1. Gate 0：Safe Area WIP

Worktree：

- 路径：E:/code/work/先知AI-cross-platform-safe-area
- 分支：feature/cross-platform-safe-area
- HEAD：81a66692fc8c8601007c3b055bfbf27b17d43f46

git status --short 与 Phase 1 审计一致，仍然只有两个未跟踪文件：

1. apps/user-uni/tests/harmony-output-path-contract.test.mjs
2. docs/verification/safe-area/harmony-baseline-81a66692fc.md

内容判断：

- harmony-output-path-contract.test.mjs 是 Harmony 构建输出路径契约测试，确保 HBuilderX 只拥有一组 ASCII、绝对且互不相同的 dev/build 输出目录，并拒绝第二套输出 owner。
- harmony-baseline-81a66692fc.md 是 Safe Area / Harmony 环境基线记录，结论为 DevEco Studio 与 Harmony SDK/API 缺失导致 Gate 0 NO-GO，并明确 Safe Area 生产实现尚未开始。
- 两个文件均直接属于 feature/cross-platform-safe-area 当前工作，不属于本次历史分支清理范围。

执行前与执行后校验：

| 文件 | 存在 | SHA-256（前） | SHA-256（后） | 结果 |
|---|---|---|---|---|
| apps/user-uni/tests/harmony-output-path-contract.test.mjs | YES | FEA4192766BABCA16B01766BBC88EBD9B9775786139B139DEA310BC7CC80E465 | FEA4192766BABCA16B01766BBC88EBD9B9775786139B139DEA310BC7CC80E465 | UNCHANGED |
| docs/verification/safe-area/harmony-baseline-81a66692fc.md | YES | ED22DD5A34E51A7943C67F02D0CAA0318F0149BE70954B9C47F69438EF7B2DEE | ED22DD5A34E51A7943C67F02D0CAA0318F0149BE70954B9C47F69438EF7B2DEE | UNCHANGED |

**SAFE AREA WIP PROTECTED: YES**

## 2. Gate 1：main 重新验证

| 项目 | 清理前 | 清理操作后、写本报告前 |
|---|---|---|
| 当前分支 | main | main |
| main HEAD | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |
| refs/heads/main | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |
| cached origin/main | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |
| cached gitee/main | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c | 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |
| tracked diff | 0 | 0 |
| staged diff | 0 | 0 |
| status | 仅 Phase 1 审计报告为 untracked | 同前 |

说明：

- main 在 Phase 2 开始时不是字面意义上的 clean，唯一状态项是上一阶段按用户要求生成的未跟踪治理报告 docs/git-branch-governance-audit-2026-08-14.md。
- 该文件不是生产代码；本阶段没有修改它。写本报告前其长度仍为 39,383 bytes，SHA-256 为 FE98F95808226942EFF6F67EFAF7ACC4B3F49400DAB59721CE809A904A0EEC00。
- 没有任何 tracked、staged 或生产代码变更，因此没有触发“存在未提交代码”的停止条件。
- 未执行 fetch、pull、merge 或 rebase。

## 3. Before / After

| 指标 | Before | 删除尝试后、写报告前 | 变化 |
|---|---:|---:|---:|
| 本地分支数量 | 28 | 28 | 0 |
| worktree 数量 | 21 | 21 | 0 |
| main HEAD | 6ee5b36f... | 6ee5b36f... | unchanged |
| main tracked/staged 文件 | 0 / 0 | 0 / 0 | unchanged |
| Safe Area 普通 status 项 | 2 | 2 | unchanged |

本轮实际没有成功删除任何 worktree 或本地分支。

## 4. 七个候选的重新验证与执行结果

### 证据口径

- MERGED_BY_ANCESTRY：git merge-base --is-ancestor candidate main 返回成功。
- PATCH_EQUIVALENT：候选不是 main 祖先，但 git cherry main candidate 的全部 branch-only commit 都为 -，且没有 +。
- 所有候选 HEAD 均与 Phase 1 报告完全一致。
- 远端分支一律不删除、不 push、不 prune，仅记录 REMOTE_CLEANUP_PENDING。

| Branch | Phase 1 HEAD | 当前 HEAD | 合并证据 | Worktree 状态 | 本地执行结果 | 远端状态 |
|---|---|---|---|---|---|---|
| codex/agent-invite-apk-production-readiness | 7695c8ae8376fb96599d94b9e643dd605bebd742 | 同左 | MERGED_BY_ANCESTRY | 无本地 worktree；无本地 branch | 无本地对象可删 | origin：REMOTE_CLEANUP_PENDING |
| codex/channel-ecosystem-v132-phase3 | e0b57e1efcd501854aaba2d6459e412ed679bad2 | 同左 | MERGED_BY_ANCESTRY | 无本地 worktree；无本地 branch | 无本地对象可删 | origin、gitee：REMOTE_CLEANUP_PENDING |
| codex/seedance-video-artifact-fix | dbe6e656107cf4ee855db37ac29a5a8c2016d0ec | 同左 | MERGED_BY_ANCESTRY | 无 worktree | git branch -d 被 .git ref lock 权限拒绝；LOCAL_DELETE_SKIPPED | 无 remote-tracking ref |
| codex/smartvideo-mini-2.0.42 | 38a8318b82cc55a7d5bd0f667d1c2230fd743c5a | 同左 | MERGED_BY_ANCESTRY | 无 worktree | git branch -d 被 .git ref lock 权限拒绝；LOCAL_DELETE_SKIPPED | 无 remote-tracking ref |
| codex/video-contract-2.0.36 | dfc70c84535c0d8b28c44c17248f08e0408fb37e | 同左 | MERGED_BY_ANCESTRY | 无本地 worktree；无本地 branch | 无本地对象可删 | origin、gitee：REMOTE_CLEANUP_PENDING |
| codex/agent-invite-apk-production-final | 27dde3b4e9130eff574b9d2aa48ebc0522b7d36b | 同左 | PATCH_EQUIVALENT；git cherry 仅 - 27dde3b4... | WT14 普通 status clean；无 tracked/staged/untracked；ignored 文件 23,822 个，全部归类为 dist/node_modules | 正常 git worktree remove 失败：Windows Invalid argument；复核后 worktree/branch/status/ignored 数量均未变；SKIPPED_FOR_SAFETY；未尝试删除 branch | gitee：REMOTE_CLEANUP_PENDING |
| codex/protect-main-production-ops-20260814 | fb4fc7cc9347883323142e3b1b5a5bf944e753ad | 同左 | PATCH_EQUIVALENT；git cherry 仅 - fb4fc7cc... | 无 worktree | git branch -d 因“not fully merged”被拒绝；未使用 -D；LOCAL_DELETE_SKIPPED | 无 remote-tracking ref |

## 5. Worktree 删除尝试详情

唯一关联 SAFE_TO_CLEAN_CANDIDATE worktree：

- 路径：E:/code/work/先知AI-phase2.1A
- 分支：codex/agent-invite-apk-production-final
- HEAD：27dde3b4e9130eff574b9d2aa48ebc0522b7d36b
- 普通 git status：clean
- tracked diff：0
- staged diff：0
- 普通 untracked：0
- ignored：23,822

ignored 文件只位于：

- apps/user-uni/dist
- apps/user-uni/node_modules
- repository-level node_modules

未发现 ignored 业务 WIP。随后执行了不带 --force 的正常 git worktree remove，Git 返回：

> failed to delete 'E:/code/work/先知AI-phase2.1A': Invalid argument

并同时报告无法删除对应 .git/worktrees 元数据。按规则立即停止处理该 worktree，没有手工删除、Remove-Item、rm、clean 或 force。

失败后重新验证：

- 路径仍存在。
- worktree 注册仍存在。
- 分支仍为 codex/agent-invite-apk-production-final。
- HEAD 未变。
- 普通 status 仍为 clean。
- ignored 文件仍为 23,822。

结论：删除失败没有造成可观察的文件或 Git 状态变化；该 worktree 和分支保留。

## 6. 实际清理内容

### 删除的 worktree

无。

### 删除的本地 branch

无。

### 完全没有执行本地删除的候选

- codex/agent-invite-apk-production-readiness：仅远端跟踪 ref。
- codex/channel-ecosystem-v132-phase3：仅远端跟踪 refs。
- codex/video-contract-2.0.36：仅远端跟踪 refs。
- codex/agent-invite-apk-production-final：worktree 正常删除失败后按安全规则停止，没有尝试删除 branch。

### 尝试正常删除但保持原样

- codex/seedance-video-artifact-fix：branch -d 因无法创建 ref lock 被拒绝。
- codex/smartvideo-mini-2.0.42：branch -d 因无法创建 ref lock 被拒绝。
- codex/protect-main-production-ops-20260814：branch -d 因 Git ancestry 规则被拒绝。
- E:/code/work/先知AI-phase2.1A：正常 worktree remove 因 Windows Invalid argument 被拒绝。

## 7. 剩余远端清理候选

以下仅记录，不执行：

| Remote ref | 状态 |
|---|---|
| origin/codex/agent-invite-apk-production-readiness | REMOTE_CLEANUP_PENDING |
| origin/codex/channel-ecosystem-v132-phase3 | REMOTE_CLEANUP_PENDING |
| gitee/codex/channel-ecosystem-v132-phase3 | REMOTE_CLEANUP_PENDING |
| origin/codex/video-contract-2.0.36 | REMOTE_CLEANUP_PENDING |
| gitee/codex/video-contract-2.0.36 | REMOTE_CLEANUP_PENDING |
| gitee/codex/agent-invite-apk-production-final | REMOTE_CLEANUP_PENDING |

未执行 ls-remote、fetch、prune、push 或任何远端删除。

## 8. Safety Verification

| 安全项 | 结果 |
|---|---|
| main HEAD unchanged | YES — 6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c |
| main cached origin/gitee refs unchanged | YES |
| main tracked working tree unchanged | YES |
| main staged state unchanged | YES |
| Phase 1 审计报告 unchanged | YES |
| Safe Area WIP unchanged | YES — status 与两个 SHA-256 完全一致 |
| production code modified | NO |
| merge / rebase / cherry-pick | NO |
| reset / stash / clean | NO |
| force delete / branch -D / worktree --force | NO |
| fetch / pull / push / prune | NO |
| remote branch deleted | NO |
| tag modified or deleted | NO |
| 高风险或非候选分支被触碰 | NO |

写入本报告后，main worktree 会比执行前新增本报告这一项 untracked governance document；除此之外没有工作区变化。

## 9. 最终结论

**PHASE 2 LOCAL CLEANUP: PARTIAL**

原因：

1. Gate 0、Gate 1、Gate 2 的安全证据全部通过。
2. 7 个候选没有出现新 commit、HEAD 漂移、branch-only + commit 或业务 WIP。
3. 由于 Windows worktree 正常删除错误、当前环境对 .git ref lock 的写权限限制，以及 git branch -d 对 patch-equivalent 但非 ancestry 分支的保护，本轮实际清理为 0。
4. 严格遵守了不 force、不手工删除、不扩大范围的要求。

## 10. 下一阶段推荐

本轮不是 PASS，因此不直接进入 Phase 3。

建议先执行一个受控的 **Phase 2A：本地清理重试**：

1. 在明确具有 .git 写权限的人工终端中重新执行两个 ancestry 分支的 git branch -d。
2. 只读定位 E:/code/work/先知AI-phase2.1A 的 Windows Invalid argument 来源；不得先手工删除目录。
3. 对 protect-main-production-ops-20260814 和 production-final 继续遵守“不使用 -D”的限制；若要删除，需要新的人工批准和单独治理方案。
4. 远端 refs 仍等待实时 ls-remote 核验，不进入删除。

Phase 2A 完成或人工接受这些保留项后，再推荐进入 **Phase 3：Identity 安全与迁移只读审计**。

本阶段到此停止，等待人工批准。
