# Release commit、镜像与迁移文件冻结手册

本手册描述未来经批准的冻结操作。本轮不得执行 commit、tag、镜像构建、registry push 或部署。

## 1. 冻结前提

- 2F 范围文件已完成评审，第三阶段文件不在 release 范围内。
- 所有已知测试失败已有修复或书面豁免。
- 使用新的干净工作树或经过人工逐文件确认的暂存区。
- 禁止使用 `git add .`、`git add -A` 或把当前脏工作区整体提交。
- release commit、迁移清单和镜像必须来自同一 Git tree。

## 2. 冻结 release commit

在干净工作树中执行：

```powershell
git status --short
git diff --check

# 只暂存审批文件清单中的路径；不要使用 git add .
git add -- <approved-file-1> <approved-file-2> <approved-file-N>
git diff --cached --name-status
git diff --cached --check

# 人工核对：无第三阶段文件、无本地凭证、无临时探测脚本。
git commit -m "chore(pricing): freeze member-agent V2 release candidate"

$releaseCommit = (git rev-parse HEAD).Trim()
$releaseTree = (git rev-parse HEAD^{tree}).Trim()
git show --stat --oneline --decorate $releaseCommit
git status --short
```

冻结记录必须保存：

- `releaseCommit`
- `releaseTree`
- 分支名
- 审核人和审核时间
- `git status --short` 空工作树证据
- `git diff <approved-base>...$releaseCommit --name-status`

如需 tag，必须在独立发布审批后创建；本准备任务不创建 tag。

## 3. 冻结 097–100 SHA256

仓库当前没有 `.gitattributes`，且工作站可能启用 `core.autocrlf`。最终 hash 必须从 release commit 的 `git archive` 解包结果计算，不能直接使用任意工作树中的换行版本。

```powershell
$releaseCommit = (git rev-parse HEAD).Trim()
$bundleRoot = '<approved-release-bundle-directory>'
$archivePath = Join-Path $bundleRoot "source-$releaseCommit.tar"
$sourceRoot = Join-Path $bundleRoot "source-$releaseCommit"

git archive --format=tar --output=$archivePath $releaseCommit
New-Item -ItemType Directory -Path $sourceRoot | Out-Null
tar -xf $archivePath -C $sourceRoot

$migrationFiles = @(
  'database/migrations/097-member-agent-price-plan-v2.sql',
  'database/migrations/098-price-plan-admin-governance.sql',
  'database/migrations/099-price-plan-default-switch.sql',
  'database/migrations/100-price-plan-test-whitelist-audit.sql'
)

$migrationFiles | ForEach-Object {
  $path = Join-Path $sourceRoot $_
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    throw "Missing migration: $_"
  }
}

$numberConflicts = Get-ChildItem -LiteralPath (Join-Path $sourceRoot 'database/migrations') -File |
  Where-Object { $_.Name -match '^(097|098|099|100)-.*\.sql$' } |
  Group-Object { $_.Name.Substring(0, 3) } |
  Where-Object { $_.Count -ne 1 }
if ($numberConflicts) {
  $numberConflicts | Format-Table Name, Count
  throw 'Migration number conflict'
}

$frozenMigrationPaths = $migrationFiles | ForEach-Object { Join-Path $sourceRoot $_ }
$migrationHashes = Get-FileHash -Algorithm SHA256 -LiteralPath $frozenMigrationPaths |
  Select-Object @{Name='file';Expression={ Split-Path -Leaf $_.Path }}, Hash
$migrationHashes | Format-Table -AutoSize
```

将结果写入 release manifest，并在以下三个位置交叉验证：

1. release commit 中的文件。
2. 镜像构建上下文或发布制品中的文件。
3. DBA 实际执行前挂载到 `/migrations` 的只读文件。

任一 SHA256 不一致立即 `NO-GO`，不得现场覆盖文件。

## 4. 冻结本地镜像内容摘要

镜像必须从该 `git archive` 或同 commit 的全新 clean checkout 构建。不得从本轮脏工作区构建：

```powershell
$releaseCommit = (git rev-parse HEAD).Trim()
$imageRepository = '<approved-registry>/xianzhi-ai-platform'
$imageRef = "${imageRepository}:git-${releaseCommit}"

if (git status --porcelain=v1) {
  throw 'Release checkout is dirty'
}
if ((git rev-parse HEAD).Trim() -ne $releaseCommit) {
  throw 'Release checkout commit mismatch'
}

docker build --pull `
  --platform linux/amd64 `
  --label "org.opencontainers.image.revision=$releaseCommit" `
  --label "org.opencontainers.image.source=<approved-repository-url>" `
  --tag $imageRef `
  --file Dockerfile .

$imageId = (docker image inspect --format '{{.Id}}' $imageRef).Trim()
$embeddedRevision = (docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' $imageRef).Trim()
if ($embeddedRevision -ne $releaseCommit) {
  throw 'Image revision label does not match release commit'
}

docker image save --output "release-${releaseCommit}.tar" $imageRef
$archiveHash = (Get-FileHash -Algorithm SHA256 -LiteralPath "release-${releaseCommit}.tar").Hash
```

`imageId` 是本地 OCI 配置摘要，`archiveHash` 是导出 tar 的文件摘要；两者都要进入 release manifest。

## 5. 冻结 registry RepoDigest

只有 registry 发布获得单独批准后才执行，push 不代表部署：

```powershell
docker push $imageRef
$repoDigest = docker image inspect --format '{{index .RepoDigests 0}}' $imageRef
if (-not $repoDigest -or $repoDigest -notmatch '@sha256:[0-9a-f]{64}$') {
  throw 'Registry RepoDigest was not resolved'
}
docker buildx imagetools inspect $repoDigest
```

生产部署单只能引用不可变的 `$repoDigest`，不得引用可移动的 `prod`、`latest` 或仅有版本 tag 的镜像。

## 6. Release manifest 必填字段

```json
{
  "releaseCommit": "<40-hex-commit>",
  "releaseTree": "<40-hex-tree>",
  "imageRef": "<registry/repository:tag>",
  "imageId": "sha256:<local-config-digest>",
  "repoDigest": "<registry/repository@sha256:...>",
  "imageArchiveSha256": "<sha256>",
  "migrations": [
    { "file": "097-member-agent-price-plan-v2.sql", "sha256": "<sha256>" },
    { "file": "098-price-plan-admin-governance.sql", "sha256": "<sha256>" },
    { "file": "099-price-plan-default-switch.sql", "sha256": "<sha256>" },
    { "file": "100-price-plan-test-whitelist-audit.sql", "sha256": "<sha256>" }
  ],
  "builtAt": "<RFC3339>",
  "reviewedBy": ["<reviewer-1>", "<reviewer-2>"]
}
```

## 7. 冻结失败判定

以下任一项为 `NO-GO`：

- 工作树或暂存区混入未审批文件。
- release commit、镜像 revision label、迁移 SHA256 不属于同一 Git tree。
- 迁移编号冲突或文件缺失。
- 镜像只记录 tag，没有 RepoDigest。
- 构建日志、镜像层或 manifest 出现 Secret、AppKey、sessionKey、数据库密码。
- 无法重现同一 commit 的镜像或迁移 hash。
