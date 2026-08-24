# 最小监控接入（monitoring-minimal）

后端内置零依赖的 Prometheus 文本格式指标端点，配合任意 Prometheus 兼容抓取器即可建立第一道生产可观测防线。

## 暴露内容

`GET /metrics`（`XIANZHI_METRICS_ENABLED=0` 可关闭，默认开启）：

| 指标 | 类型 | 用途 |
| --- | --- | --- |
| `xianzhi_http_requests_total{method,path,status}` | counter | 流量、错误率（按路由模板聚合，无路径参数泄漏） |
| `xianzhi_http_request_duration_seconds_{sum,count}` | counter | 平均时延趋势 |
| `xianzhi_process_uptime_seconds` | gauge | 重启检测 |
| `xianzhi_process_goroutines` / `_heap_alloc_bytes` / `_sys_bytes` | gauge | 内存与协程健康 |
| `xianzhi_process_gc_cycles_total` | counter | GC 频率 |

健康探活沿用既有端点：`/healthz`（容器/LB 用）与 `/api/v1/health`。

## 接入步骤

1. **抓取**：把 `prometheus.yml` 中的 job 合入现有 Prometheus 配置；目标为 API 容器地址（compose 内网主机名 `api:3100` 或宿主 `127.0.0.1:3100`）。
2. **告警**：将 `alerts.yml` 的三条起步规则合入规则目录：实例失联、5xx 占比突增、平均时延翻倍。
3. **封锁公网**：`/metrics` 含内部运行信息，必须在反向代理/LB 层拒绝外部访问（nginx 示例见下）。仅内网或 VPN 网段放行。

```nginx
location = /metrics {
    allow 10.0.0.0/8;
    allow 172.16.0.0/12;
    deny all;
    proxy_pass http://127.0.0.1:3100;
}
```

## 设计约束

- 无新增 Go 依赖（未引入 prometheus/client_golang），序列化手写实现于 `backend-go/internal/httpserver/metrics.go`
- 标签维度固定为 method + 路由模板 + 状态码，序列数量以路由表规模为上界，不会随流量膨胀
- 未匹配路由计入 `path="unmatched"`，用于发现扫描流量与失效客户端
