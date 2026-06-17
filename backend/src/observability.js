const now = () => Date.now();
const escapeLabel = (value) => String(value).replace(/\\/g, "\\\\").replace(/"/g, '\\"').replace(/\n/g, "\\n");

export class Observability {
  constructor() {
    this.startedAt = new Date().toISOString();
    this.startedAtMs = now();
    this.requests = 0;
    this.errors = 0;
    this.routes = new Map();
    this.buckets = new Map();
  }

  checkRateLimit(key, limit, windowMs) {
    const timestamp = now();
    const bucket = this.buckets.get(key);
    if (!bucket || bucket.resetAt <= timestamp) {
      this.buckets.set(key, { count: 1, resetAt: timestamp + windowMs });
      return { allowed: true, remaining: limit - 1, resetAt: timestamp + windowMs };
    }
    bucket.count += 1;
    return { allowed: bucket.count <= limit, remaining: Math.max(0, limit - bucket.count), resetAt: bucket.resetAt };
  }

  record({ method, path, status, latencyMs }) {
    this.requests += 1;
    if (status >= 400) this.errors += 1;
    const key = `${method} ${path}`;
    const route = this.routes.get(key) || { route: key, requests: 0, errors: 0, totalLatencyMs: 0, maxLatencyMs: 0 };
    route.requests += 1;
    route.errors += status >= 400 ? 1 : 0;
    route.totalLatencyMs += latencyMs;
    route.maxLatencyMs = Math.max(route.maxLatencyMs, latencyMs);
    this.routes.set(key, route);
  }

  snapshot() {
    const errorRate = this.requests ? Number((this.errors / this.requests).toFixed(4)) : 0;
    const routes = [...this.routes.values()].map((item) => ({
      ...item, averageLatencyMs: item.requests ? Math.round(item.totalLatencyMs / item.requests) : 0
    })).sort((a, b) => b.requests - a.requests);
    const alerts = [];
    if (this.requests >= 10 && errorRate >= 0.2) alerts.push({ level: "WARNING", code: "HIGH_ERROR_RATE", message: `HTTP error rate is ${Math.round(errorRate * 100)}%` });
    for (const route of routes.filter((item) => item.requests >= 5 && item.errors / item.requests >= 0.5)) {
      alerts.push({ level: "WARNING", code: "ROUTE_ERROR_RATE", message: `${route.route} error rate is high` });
    }
    return {
      startedAt: this.startedAt,
      uptimeSeconds: Math.floor((now() - this.startedAtMs) / 1000),
      requests: this.requests,
      errors: this.errors,
      errorRate,
      routes: routes.slice(0, 30),
      alerts
    };
  }

  prometheus() {
    const snapshot = this.snapshot();
    const lines = [
      "# HELP xianzhi_http_requests_total Total HTTP requests.",
      "# TYPE xianzhi_http_requests_total counter",
      `xianzhi_http_requests_total ${snapshot.requests}`,
      "# HELP xianzhi_http_errors_total Total HTTP error responses.",
      "# TYPE xianzhi_http_errors_total counter",
      `xianzhi_http_errors_total ${snapshot.errors}`,
      "# HELP xianzhi_http_error_rate Current HTTP error rate.",
      "# TYPE xianzhi_http_error_rate gauge",
      `xianzhi_http_error_rate ${snapshot.errorRate}`,
      "# HELP xianzhi_uptime_seconds Application uptime in seconds.",
      "# TYPE xianzhi_uptime_seconds gauge",
      `xianzhi_uptime_seconds ${snapshot.uptimeSeconds}`,
      "# HELP xianzhi_alerts Active application alerts.",
      "# TYPE xianzhi_alerts gauge",
      `xianzhi_alerts ${snapshot.alerts.length}`,
      "# HELP xianzhi_route_requests_total HTTP requests by route.",
      "# TYPE xianzhi_route_requests_total counter"
    ];
    for (const route of snapshot.routes) {
      const label = escapeLabel(route.route);
      lines.push(`xianzhi_route_requests_total{route="${label}"} ${route.requests}`);
    }
    lines.push("# HELP xianzhi_route_errors_total HTTP errors by route.");
    lines.push("# TYPE xianzhi_route_errors_total counter");
    for (const route of snapshot.routes) {
      const label = escapeLabel(route.route);
      lines.push(`xianzhi_route_errors_total{route="${label}"} ${route.errors}`);
    }
    lines.push("# HELP xianzhi_route_latency_average_ms Average HTTP latency by route.");
    lines.push("# TYPE xianzhi_route_latency_average_ms gauge");
    for (const route of snapshot.routes) {
      const label = escapeLabel(route.route);
      lines.push(`xianzhi_route_latency_average_ms{route="${label}"} ${route.averageLatencyMs}`);
    }
    lines.push("# HELP xianzhi_route_latency_max_ms Max HTTP latency by route.");
    lines.push("# TYPE xianzhi_route_latency_max_ms gauge");
    for (const route of snapshot.routes) {
      const label = escapeLabel(route.route);
      lines.push(`xianzhi_route_latency_max_ms{route="${label}"} ${route.maxLatencyMs}`);
    }
    return `${lines.join("\n")}\n`;
  }
}

export function rateLimitPolicy(path) {
  if (path === "/api/v1/auth/login" || path === "/api/v1/auth/register") return { group: "auth", limit: 20, windowMs: 60000 };
  if (path.startsWith("/api/v1/generations/")) return { group: "generation", limit: 60, windowMs: 60000 };
  if (path.startsWith("/api/v1/payments/callback/")) return { group: "payment-callback", limit: 120, windowMs: 60000 };
  return { group: "default", limit: 600, windowMs: 60000 };
}
