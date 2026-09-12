# P1 PPTX File Object Persistence

## 问题

普通 PPTX 导出接口此前只在请求内调用 `buildPPTX` 并写 HTTP response，任务完成后没有最终 PPTX File Object。

## 修改

- 增加 `ppt.Service.SetPPTURL`，把最终产物 reference 持久化到 PostgreSQL/raw task state。
- `exportPPT` 与 `downloadPPTExport` 在返回文件前，使用 `StoreObjectIdempotent` 写入 `pptx_export` File Object。
- 使用内容 SHA-256 进入原始文件名，保证同一任务内容幂等，内容变化生成新 artifact。
- 任务读取时将 `storage://tenant/file` 动态解析为短期 signed URL。
- 对象存储不可用时导出 fail-closed，不继续把仅存在于请求内的 PPTX 当作长期资产。

## 测试

```text
cd backend-go && go test -count=1 ./internal/app/ppt ./internal/httpserver -run 'TestSetPPTURLPersistsArtifactReference|TestPPTExport|TestPPTStorage|^$'
PASS
```

真实 PostgreSQL + MinIO 产物写入和下载验证仍需在带对应运行时的环境执行。

## 状态

`IMPLEMENTED_LOCALLY / REAL_STORAGE_E2E_NOT_VERIFIED`
