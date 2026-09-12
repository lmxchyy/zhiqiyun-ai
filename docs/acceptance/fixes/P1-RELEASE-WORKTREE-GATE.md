# P1 Release Worktree Gate

## 问题

`deploy.sh` 只检查 tracked diff，未跟踪文件可能进入部署上下文，导致部署内容与 Git commit 不一致。

## 修改

将部署前检查统一为：

```bash
git status --porcelain --untracked-files=all
```

任何 tracked、staged 或 untracked 变化都会 fail-closed。

## 测试

```text
git diff --check -- deploy.sh
PASS
```

当前工作区仍有未提交变更，故本次不能宣称 release gate 实际通过。

## 状态

`IMPLEMENTED_LOCALLY / WORKTREE_MUST_BE_COMMITTED_AND_PUSHED`
