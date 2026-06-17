# GEO 内容发布与效果跟踪实施说明

- GEO 优化文章从草稿进入已发布状态。
- 保存发布平台、内容 URL、发布时间和发布记录。
- 同一内容与 URL 重复提交保持幂等。
- 按时间记录曝光、引用、品牌提及和点击数据。
- 自动计算引用率、品牌提及率、点击率以及相对首次采集的增长。
- GEO 总览展示内容发布数量及最新效果。

API：

- `POST /api/v1/geo/contents/{id}/publish`
- `POST /api/v1/geo/publications/{id}/metrics`
- `GET /api/v1/geo/overview`

Docker Compose 启动时会自动执行 `018-geo-content-tracking.sql`。
