# PPT 编辑与导出实施说明

## 已实现能力

- 根据主题生成 PPT 大纲和初始页面。
- 页面级标题、正文和演讲备注编辑。
- 页面上移、下移并自动重建页面序号。
- 一键重新生成大纲。
- 导出可继续编辑的 PPTX。
- 导出 PDF 预览交付文件。
- 编辑、排序和重新生成操作保留审计记录。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `PUT` | `/api/v1/presentations/{id}` | 更新主题、模板及页面内容和顺序 |
| `POST` | `/api/v1/presentations/{id}/regenerate-outline` | 重新生成大纲 |
| `GET` | `/api/v1/presentations/{id}/export-pptx` | 导出可编辑 PPTX |
| `GET` | `/api/v1/presentations/{id}/export-pdf` | 导出 PDF |
