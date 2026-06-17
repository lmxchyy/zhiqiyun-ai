import test from "node:test";
import assert from "node:assert/strict";
import { Observability, rateLimitPolicy } from "../src/observability.js";

test("接口限流在窗口内拒绝超额请求", () => {
  const metrics = new Observability();
  assert.equal(metrics.checkRateLimit("auth:ip", 2, 60000).allowed, true);
  assert.equal(metrics.checkRateLimit("auth:ip", 2, 60000).allowed, true);
  assert.equal(metrics.checkRateLimit("auth:ip", 2, 60000).allowed, false);
  assert.equal(rateLimitPolicy("/api/v1/auth/login").group, "auth");
  assert.equal(rateLimitPolicy("/api/v1/generations/images/text-to-image").group, "generation");
});

test("请求指标统计错误率、延迟和告警", () => {
  const metrics = new Observability();
  for (let index = 0; index < 10; index += 1) {
    metrics.record({ method: "POST", path: "/api/v1/test", status: index < 5 ? 500 : 200, latencyMs: index + 1 });
  }
  const snapshot = metrics.snapshot();
  assert.equal(snapshot.requests, 10);
  assert.equal(snapshot.errors, 5);
  assert.equal(snapshot.errorRate, 0.5);
  assert.equal(snapshot.routes[0].averageLatencyMs, 6);
  assert.ok(snapshot.alerts.some((item) => item.code === "HIGH_ERROR_RATE"));
});

test("Prometheus 指标导出包含全局和路由维度", () => {
  const metrics = new Observability();
  metrics.record({ method: "GET", path: "/api/v1/dashboard", status: 200, latencyMs: 12 });
  metrics.record({ method: "POST", path: "/api/v1/generations/images/text-to-image", status: 422, latencyMs: 40 });
  const output = metrics.prometheus();
  assert.match(output, /# TYPE xianzhi_http_requests_total counter/);
  assert.match(output, /xianzhi_http_requests_total 2/);
  assert.match(output, /xianzhi_http_errors_total 1/);
  assert.match(output, /xianzhi_route_requests_total\{route="POST \/api\/v1\/generations\/images\/text-to-image"\} 1/);
  assert.match(output, /xianzhi_route_latency_max_ms\{route="POST \/api\/v1\/generations\/images\/text-to-image"\} 40/);
});
