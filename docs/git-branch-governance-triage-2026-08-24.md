# Git 分支活跃度甄别清单（2026-08-24）

- 性质：只读甄别 + 清理建议；本轮除本报告外无任何写入
- 方法：对全部 41 个本地分支执行 `rev-list --left-right --count`（落后/领先）、`git cherry main <branch>`（补丁等价性）、远端 ref 存在性核对、worktree 脏净检查
- 关键口径：**领先提交数不可信**（rebase/squash 合并会让已吸收分支仍显示 ahead）；以 `git cherry` 补丁等价为准——等价（`-`）= 内容已在 main，真实独特（`+`）= main 上没有的内容

## 结论速览

- 真正进行中的任务只有 **2 条**：`refactor/points-persistence-finalize`（今天，已推 origin）与 `codex/ppt-v2`（worktree 37 个未提交文件）
- **16 条分支内容已完全进入 main**，属可安全删除的僵尸分支
- 约 18 条分支含真实未合并内容，需逐条人工拍板
- 上一轮评估中担心的 migration-release-automation / phone-login / thumbnail 三个"冲突分支"均为僵尸，**不构成冲突**

## A 类 · 进行中（禁止清理）

| 分支 | 证据 | 影响 |
| --- | --- | --- |
| refactor/points-persistence-finalize | 落后0/领先1，今日提交，origin 存在 | points 域相关的拆包/重构冻结至其合并 |
| codex/ppt-v2 | 落后75/领先23；worktree E:/code/work/ppt-v2 有 37 个未提交文件 | 涉及 PPT 文件面的改动避开 |

## B 类 · 内容已进 main（可删候选）

### B1 补丁等价（ahead>0 但 cherry 全等价）

- chore/backup-offsite-obs
- chore/backup-offsite-upload
- chore/backup-retention
- fix/backup-retention-python36
- fix/migration-release-automation
- fix/phone-login-unavailable
- perf/online-image-thumbnail-externalization
- codex/protect-main-production-ops-20260814
- codex/agent-invite-apk-production-final

### B2 纯指针残留（ahead=0）

- codex/seedance-video-artifact-fix
- codex/smartvideo-mini-2.0.42
- fix/billing-point-cny-conversion
- fix/deploy-script-executable

### B2 附带 · 同名远端僵尸（origin，内容同已吸收）

origin/chore/backup-offsite-obs、origin/chore/backup-offsite-upload、origin/chore/backup-retention、origin/fix/backup-retention-python36、origin/fix/migration-release-automation、origin/fix/phone-login-unavailable、origin/perf/online-image-thumbnail-externalization

## C 类 · 含真实未合并内容（逐条拍板）

| 分支 | 落后/独特提交 | 建议 |
| --- | --- | --- |
| feature/identity-security-rebuild | 75 / 17（rollout readiness gate、运行时凭证切换；worktree 2 个脏文件） | 正经工程半途；先决策该线是否继续 |
| codex/identity-phase2-1-security | 290 / 1 | 高漂移，倾向作废归档 |
| codex/identity-phase2-2-deployment-gates | 290 / 2 | 同上 |
| codex/identity-phase2-2-release-readiness | 290 / 2 | 同上 |
| codex/protect-identity-phase2-2-deployment-gates-20260814 | 290 / 3 | 保护快照 |
| codex/protect-enterprise-v1-20260814 | 219 / 2 | 保护快照 |
| codex/enterprise-v1 | 219 / 1 | 同上 |
| codex/ppt-agent-phase1-regression-integration | 117 / 14（connector postgres lease fencing 等） | 与 ppt-v2 疑似重叠，向 ppt-v2 会话求证后处理 |
| feature/cross-platform-safe-area | 75 / 22（混入大量 codex supervisor loop 噪声提交） | 甄别有效提交后重做或放弃 |
| codex/video-compliance-release-tools-20260731 | 212 / 5（3 等价 + 2 真实） | 多半被 main 替代 |
| codex/video-model-params-2.0.36 | 212 / 4 | 同上 |
| codex/video-ui-login-2.0.39 | 212 / 7 | 同上 |
| codex/login-compliance-2.0.38 | 212 / 4（3 等价 + 1 真实） | 同上 |
| codex/grok-imagine-1.5-video | 258 / 2 | 归档 |
| codex/miniprogram-agent-invite-autobind-production | 220 / 3 | 归档 |
| codex/operation-center-invite-promotion | 258 / 3 | 归档 |
| codex/protect-seedance-prod-minimal-20260805 | 219 / 1 | 保护快照 |
| codex/protect-seedance-prod-release-20260805 | 219 / 1 | 保护快照 |
| codex/protect-seedance-r8-compliant-20260806 | 183 / 1 | 保护快照 |
| codex/protect-seedance-video-artifact-fix-20260814 | 219 / 1 | 保护快照 |
| codex/protect-smartvideo-grok-backend-20260814 | 258 / 3 | 保护快照 |
| codex/protect-smartvideo-mini-2.0.42-20260814 | 258 / 1 | 保护快照 |
| release/mini-program | 46 / 1（记录 2.0.68 版本号） | 近期发布线，确认发版完成后清理 |
| codex/agent-invite-apk-release-blockers | 277 / 2 | 归档 |

## 推荐处置流程（待批准）

1. B 类：`git branch -D <name>`；origin 同名僵尸一并删除；对应 worktree `git worktree remove`
2. C 类：先 `git tag archive/<name> <name>`（可选推送 tag 到远端作保险），再删本地分支与 worktree；gitee 镜像不动
3. A 类及 worktree 不动
4. 脏 worktree（ppt-v2 37 文件、identity-security-rebuild 2 文件）必须先由所属会话确认后再处理

## 对优化实施方案的影响修正

- httpserver 拆包（原方案暂缓项）**可以立即开工**：此前判定冲突的三个分支均已完成吸收
- 仍需避让：points 域（等 A 类第 1 条合并）、PPT 相关文件面（等 ppt-v2 会话确认范围）
- schema CI 校验（P0-3 第 3 步）不再有 workflow 冲突，也可提前
