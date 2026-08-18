# Git 分支治理 Phase 2A：本地清理受控重试与 Windows Worktree 故障定位

- 审计日期：2026-08-15
- 仓库：`E:/code/work/先知AI`
- 执行范围：仅 Phase 2 已确认安全但未完成的本地清理事项
- 执行结果：未删除任何 branch 或 worktree；未修改任何生产代码

## Executive Result

`GIT METADATA WRITE ACCESS: BLOCKED`

`SAFE AREA WIP PROTECTED: YES`

`WT14 ROOT CAUSE: PATH_PROBLEM`

`WT14 REMOVE: SKIPPED_METADATA_WRITE_BLOCKED`

`PHASE 2A: PARTIAL`

本阶段在 Gate 0 通过后，对 Git metadata 写权限进行了两种 Git 原生临时 ref 测试。最终的直接 `refs/heads` 测试明确返回 `.lock: Permission denied`，且测试 ref 在测试前后均不存在。依据 Task 1 的强制门禁，随后所有删除动作均停止。因此两个 ancestry 分支虽再次被证明已完整进入 `main`，仍未执行 `git branch -d`；WT14 也未再次执行 `git worktree remove`。

WT14 的只读检查未发现业务 WIP、非法路径、长路径、只读文件、根目录 reparse point 或 Git metadata 结构损坏。当前执行环境只允许写主工作区而不允许写主仓库 `.git`，WT14 又位于主工作区之外的同级目录。结合 ref lock 的明确权限错误，根因分类为 `PATH_PROBLEM`，具体是执行环境的路径/写权限边界，而不是已证实的 Windows Git 缺陷。

## Gate 0：仓库与 Safe Area 基线

### Main

- 当前路径：`E:/code/work/先知AI`
- 当前分支：`main`
- Phase 2A 开始时 HEAD：`6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- 预期 HEAD：`6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- `origin/main` cached ref：`6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- `gitee/main` cached ref：`6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- tracked diff：0
- staged diff：0
- 开始时仅有两个既有未跟踪治理报告：
  - `docs/git-branch-governance-audit-2026-08-14.md`
  - `docs/git-branch-governance-phase2-cleanup-2026-08-14.md`

Gate 0 还发现一个在 Phase 2 报告之后、Phase 2A 开始之前已经存在的新分支/worktree：

- branch：`codex/ppt-v2-phase0-contract`
- worktree：`E:/code/work/ppt-v2-phase0-contract`
- HEAD：`6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c`
- status：clean

该对象不在 Phase 2A 授权范围内，本阶段未触碰。它使 Phase 2A 的开始基线变为 29 个本地分支、22 个 worktree，而 Phase 2 报告记录的是 28 个本地分支、21 个 worktree。

### Safe Area

- worktree：`E:/code/work/先知AI-cross-platform-safe-area`
- branch：`feature/cross-platform-safe-area`
- HEAD：`81a66692fc8c8601007c3b055bfbf27b17d43f46`
- 普通 tracked/staged 修改：0
- 未跟踪文件仍严格为以下两个受保护 WIP：
  - `apps/user-uni/tests/harmony-output-path-contract.test.mjs`
  - `docs/verification/safe-area/harmony-baseline-81a66692fc.md`

内容哈希与 Phase 2 基线一致：

| 文件 | SHA-256 |
|---|---|
| `apps/user-uni/tests/harmony-output-path-contract.test.mjs` | `FEA4192766BABCA16B01766BBC88EBD9B9775786139B139DEA310BC7CC80E465` |
| `docs/verification/safe-area/harmony-baseline-81a66692fc.md` | `ED22DD5A34E51A7943C67F02D0CAA0318F0149BE70954B9C47F69438EF7B2DEE` |

结论：Safe Area 状态与 Phase 2 报告一致，Gate 0 通过；两个文件未修改、未提交、未删除。

## A. Environment

| 项目 | 结果 |
|---|---|
| Git | `git version 2.53.0.windows.3`，`x86_64` |
| PowerShell | `5.1.22621.6133` |
| Windows | `Microsoft Windows NT 10.0.22631.0` |
| 当前执行身份 | `DESKTOP-EGF6PT6\CodexSandboxOffline` |
| `core.longpaths` | 未设置 |
| 主仓库 `.git` | 当前执行环境只读；Git ref lock 创建被拒绝 |
| WT14 路径 | 位于主工作区之外的同级路径，当前执行环境没有该路径的写授权 |

### Git metadata 写权限验证

第一次使用 `git update-ref` 尝试创建 `refs/codex-governance/phase2a-write-test`，Git 无法在 `.git/refs` 下创建所需目录。为排除“仅嵌套目录创建失败”的歧义，又在已存在的 `refs/heads` 下进行更贴近 `git branch -d` 的测试：

```text
ref: refs/heads/codex-phase2a-metadata-write-test
BEFORE_EXISTS=0
CREATE_EXIT=128
fatal: update_ref failed for ref 'refs/heads/codex-phase2a-metadata-write-test':
cannot lock ref 'refs/heads/codex-phase2a-metadata-write-test':
Unable to create 'E:/code/work/先知AI/.git/refs/heads/codex-phase2a-metadata-write-test.lock': Permission denied
AFTER_EXISTS=0
```

测试没有创建 commit，也没有改变任何既有 ref；测试 ref 前后均不存在。由此确认：

`GIT METADATA WRITE ACCESS: BLOCKED`

未尝试 Windows ACL 修改、管理员接管、安全软件绕过或其他系统级处理。

## B. Ancestry Branch Cleanup

| Branch | Phase 2 HEAD | Phase 2A HEAD | Ancestry 证据 | Worktree | 动作 | 结果 |
|---|---|---|---|---|---|---|
| `codex/seedance-video-artifact-fix` | `dbe6e656107cf4ee855db37ac29a5a8c2016d0ec` | `dbe6e656107cf4ee855db37ac29a5a8c2016d0ec` | `git merge-base --is-ancestor ... main` exit 0；`main..branch` commit 数 0 | 无 | 未执行 `git branch -d` | `SKIPPED_METADATA_WRITE_BLOCKED` |
| `codex/smartvideo-mini-2.0.42` | `38a8318b82cc55a7d5bd0f667d1c2230fd743c5a` | `38a8318b82cc55a7d5bd0f667d1c2230fd743c5a` | `git merge-base --is-ancestor ... main` exit 0；`main..branch` commit 数 0 | 无 | 未执行 `git branch -d` | `SKIPPED_METADATA_WRITE_BLOCKED` |

两个分支的 HEAD 均未变化，均无 worktree 占用，也没有新的 branch-only commit。它们仍是高确定性的本地安全清理候选，但本轮没有获得执行删除所需的 Git metadata 写权限。

### 实际分支清理

- 删除的本地 branch：无
- 使用 `git branch -d`：无（写权限门禁之后未执行）
- 使用 `git branch -D`：无

## C. WT14 Root Cause Analysis

### 目标

- worktree：`E:/code/work/先知AI-phase2.1A`
- branch：`codex/agent-invite-apk-production-final`
- HEAD：`27dde3b4e9130eff574b9d2aa48ebc0522b7d36b`

### 数据安全检查

| 检查项 | 证据与结论 |
|---|---|
| 普通工作区状态 | `git status --short` 为空；tracked/staged/untracked 均为 0 |
| ignored 内容 | 23,822 个；仅位于 `apps/user-uni/dist`、`apps/user-uni/node_modules`、根 `node_modules`，属于可再生依赖/构建输出 |
| 业务 WIP | 未发现 |
| worktree `.git` 文件 | 内容为 `gitdir: E:/code/work/先知AI/.git/worktrees/先知AI-phase2.1A`，指向正确 |
| 主仓库 metadata | `HEAD`、`gitdir`、`commondir`、`index`、`ORIG_HEAD`、logs/refs 均存在；无 `locked` 标记 |
| metadata 回链 | `gitdir` 为 `E:/code/work/先知AI-phase2.1A/.git`；`commondir` 为 `../..`；结构一致 |
| 注册状态 | `git worktree list --porcelain` 正常列出 WT14；无 locked/prunable 标记 |
| 路径字符 | WT 路径 27 字符，metadata 路径 47 字符；未发现控制字符、尾随点/空格、保留 DOS 名称或异常冒号 |
| 仓库内最长路径 | 177 字符；大于等于 260 字符的路径为 0 |
| 文件属性 | worktree 和 metadata 根目录均为普通目录；只读文件 0、只读目录 0 |
| 根目录 reparse point | worktree 根、`.git` 文件和 metadata 根均不是 junction/symlink/reparse point |
| 内部 reparse point | 仅 6 个 `node_modules/@xianzhi/*` Junction，均指向 WT14 自身 `packages/*`；主 worktree 存在完全同类的 6 个 Junction，故不是 WT14 特有异常 |
| 嵌套 Git metadata | 除根 `.git` 文件外未发现其他 `.git` 条目 |
| metadata 文件占用 | WT `.git`、metadata `HEAD`、`index`、`gitdir` 均可用 `FileShare.None` 独占只读打开 |
| 进程证据 | 存在 VS Code/node 进程，但无证据证明其占用 WT14；进程命令行查询受系统权限限制，`handle.exe` 不可用 |
| 递归扫描 | 28,368 个文件系统对象，扫描错误 0 |

### `git worktree repair` 评估

`git worktree repair -h` 显示该命令用于按路径修复 worktree administrative files。当前 metadata 指向、回链、注册状态和结构均完整，没有形成“metadata 已损坏”的证据；同时 `.git` 写权限门禁失败。因此没有执行 `git worktree repair`，也没有执行任何会改变 worktree 注册关系的命令。

### 根因分类

`PATH_PROBLEM`

这里的 `PATH_PROBLEM` 指执行环境的路径/权限边界，不是路径字符串非法：

1. 主仓库 `.git` 无法创建普通 `refs/heads/*.lock`，已得到明确 `Permission denied`。
2. 当前受控执行环境的可写根为 `E:/code/work/先知AI`；WT14 位于同级路径 `E:/code/work/先知AI-phase2.1A`，不在可写根内。
3. WT14 路径短且字符正常，根目录不是 reparse point。
4. Git metadata 完整，关键文件可独占读取。
5. `node_modules` Junction 与主 worktree 的标准工作区链接一致。

没有充分证据支持 `LOCKED_FILE`、`REPARSE_POINT`、`GIT_METADATA_PROBLEM` 或 `WINDOWS_GIT_BUG`。Phase 2 中的 `Invalid argument` 更可能是 Git for Windows 在受限执行边界下暴露出的表层错误；这是基于上述排除证据的判断，不等同于已经复现并证明 Git 本身存在缺陷。

### WT14 处理结果

Task 3 已证明普通业务工作区为空，但 Task 1 的上位门禁已经要求停止所有删除动作。因此本轮没有再次运行：

```text
git worktree remove E:/code/work/先知AI-phase2.1A
```

也没有运行 `--force`、手工目录删除、metadata 删除或 repair。

`WT14 REMOVE: SKIPPED_METADATA_WRITE_BLOCKED`

WT14 路径、注册关系、branch 和 HEAD 均保留。

## D. Patch-equivalent Branches

| Branch | HEAD | `git cherry main <branch>` | Worktree | 结论 |
|---|---|---|---|---|
| `codex/agent-invite-apk-production-final` | `27dde3b4e9130eff574b9d2aa48ebc0522b7d36b` | 仅 `- 27dde3b4e9130eff574b9d2aa48ebc0522b7d36b`，无 `+` | WT14 仍占用 | `PATCH_EQUIVALENT_DELETE_REQUIRES_EXPLICIT_FORCE_APPROVAL`；保留 |
| `codex/protect-main-production-ops-20260814` | `fb4fc7cc9347883323142e3b1b5a5bf944e753ad` | 仅 `- fb4fc7cc9347883323142e3b1b5a5bf944e753ad`，无 `+` | 无 | `PATCH_EQUIVALENT_DELETE_REQUIRES_EXPLICIT_FORCE_APPROVAL`；保留 |

两者均是 patch-equivalent 而非 main ancestry。Phase 2A 明确禁止 `git branch -D`，本轮没有尝试删除。

## E. Remote Verification

执行了只读命令：

```text
git ls-remote --heads origin
git ls-remote --heads gitee
```

两者均返回 exit 128，当前执行层未返回可用于判断服务器状态的错误文本或 refs。因此没有完成实时服务器核验，也没有将 cached remote-tracking ref 误当作远端现状。

以下仅为本地 cached refs：

| Cached ref | Cached HEAD | 状态 |
|---|---|---|
| `origin/codex/agent-invite-apk-production-readiness` | `7695c8ae8376fb96599d94b9e643dd605bebd742` | `REMOTE_CLEANUP_PENDING` |
| `origin/codex/channel-ecosystem-v132-phase3` | `e0b57e1efcd501854aaba2d6459e412ed679bad2` | `REMOTE_CLEANUP_PENDING` |
| `gitee/codex/channel-ecosystem-v132-phase3` | `e0b57e1efcd501854aaba2d6459e412ed679bad2` | `REMOTE_CLEANUP_PENDING` |
| `origin/codex/video-contract-2.0.36` | `dfc70c84535c0d8b28c44c17248f08e0408fb37e` | `REMOTE_CLEANUP_PENDING` |
| `gitee/codex/video-contract-2.0.36` | `dfc70c84535c0d8b28c44c17248f08e0408fb37e` | `REMOTE_CLEANUP_PENDING` |
| `gitee/codex/agent-invite-apk-production-final` | `27dde3b4e9130eff574b9d2aa48ebc0522b7d36b` | `REMOTE_CLEANUP_PENDING` |

未执行 push、远端删除或 prune。

## F. Before / After

| 指标 | Phase 2A Before | Phase 2A After |
|---|---:|---:|
| 本地分支 | 29 | 29 |
| Worktree | 22 | 22 |
| Dirty worktree | 2 | 2 |
| Main HEAD | `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c` | `6ee5b36f12b73b6c64c3f9b93c5002570c52fa5c` |
| 删除的本地分支 | 0 | 0 |
| 删除的 worktree | 0 | 0 |

Dirty worktree 明细：

1. `main`：开始时仅有两份既有未跟踪治理报告；结束时另外新增本报告。tracked/staged 仍为 0。
2. `feature/cross-platform-safe-area`：仍仅有两个受保护的未跟踪 WIP，内容哈希不变。

其余 20 个 worktree 的 `git status --short` 均为空。WT14 仍 clean。

### 实际清理内容

- 删除的 worktree：无
- 删除的本地 branch：无
- 删除的远端 branch：无
- 完全没有操作的清理对象：两个 ancestry 分支、WT14、两个 patch-equivalent 分支及全部远端候选

## G. Safety Verification

- `main HEAD unchanged`: YES
- `main tracked working tree unchanged`: YES
- `main index unchanged`: YES
- `Safe Area WIP unchanged`: YES
- `SAFE AREA WIP PROTECTED`: YES
- `no production code modified`: YES
- `no high-risk branch changed`: YES
- `no merge/rebase/cherry-pick`: YES
- `no reset/stash/clean`: YES
- `no force delete`: YES
- `no worktree repair`: YES
- `no manual worktree directory deletion`: YES
- `no remote branch deleted`: YES
- `no fetch/pull/push/prune`: YES

本报告是本阶段唯一新增文件；它位于 `docs/`，未提交、未加入暂存区。

## Final Status And Recommendation

`PHASE 2A: PARTIAL`

未判定为 `PASS_WITH_DEFERRED_WT14`，因为两个 ancestry 本地分支也因 Git metadata 写权限门禁而未能完成清理。

建议下一步：

1. 在获得人工批准后，由具备主仓库 `.git` 写权限且对 WT14 同级路径有正常写权限的人工终端，重新执行一次最小化 Gate 0/Task 1 验证。
2. 只有 ref lock 验证通过后，才可对两个 ancestry 分支分别使用 `git branch -d`；仍禁止 `-D`。
3. WT14 只能先确认 clean，再尝试一次不带 `--force` 的 `git worktree remove`；若仍失败，应停止并收集原始 Git/Windows 诊断，不得手工删除目录或 metadata。
4. 两个 patch-equivalent 分支必须继续保留，直到收到明确的 force-delete 人工批准；远端 refs 也必须等实时 `ls-remote` 成功后再单独批准清理。
5. Phase 3（Identity 安全与迁移只读审计）可以作为独立只读工作安排，但本阶段不自动进入。

所有会删除 branch/worktree、使用 force、修改远端或改变 Git 历史的操作，均必须等待人工批准。
