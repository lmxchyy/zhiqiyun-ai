# P1 Image Storage Fail-Closed

## 问题

图片生成完成后，如果私有对象存储不可用，`persistGeneratedImages` 原先静默返回，资产可能继续保存 Provider URL。

## 修改

- 有生成图片但没有 `fileService` 时直接失败。
- `StorageAvailable=false` 时直接失败。
- 只有完成 File Object 持久化后才继续生成任务/资产写入。
- 保留已有幂等复用、签名 URL 和缩略图逻辑。

## 测试

```text
cd backend-go && go test -count=1 ./internal/httpserver -run 'TestPersistGeneratedImagesFailsClosedWithoutPrivateStorage|TestPersistGeneratedImages|TestWriteAssetDownloadStreamsPrivateObjectStorage|TestPPTStorageReferenceMaterializesFreshSignedURLs'
PASS
```

真实 Provider + MinIO/R2 图片 Canary 尚未执行。

## 状态

`IMPLEMENTED_LOCALLY / REAL_IMAGE_STORAGE_E2E_NOT_VERIFIED`
