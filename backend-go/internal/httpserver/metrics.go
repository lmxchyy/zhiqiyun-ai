package httpserver

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// httpMetricsCollector accumulates per-route request counters and latencies in
// process and renders them as Prometheus text format 0.0.4. It is deliberately
// dependency-free and bounded: label dimensions are HTTP method, gin route
// template, and status code, so series cardinality tracks the route table,
// not request traffic. Enable with XIANZHI_METRICS_ENABLED (default on) and
// block external access at the reverse proxy; see ops/monitoring-minimal/.
type httpMetricsCollector struct {
	mu       sync.Mutex
	started  time.Time
	requests map[httpMetricsKey]*httpMetricsSeries
}

type httpMetricsKey struct {
	method string
	path   string
	status string
}

type httpMetricsSeries struct {
	count           float64
	durationSeconds float64
}

func newHTTPMetricsCollector() *httpMetricsCollector {
	return &httpMetricsCollector{
		started:  time.Now(),
		requests: make(map[httpMetricsKey]*httpMetricsSeries),
	}
}

func (m *httpMetricsCollector) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		key := httpMetricsKey{
			method: c.Request.Method,
			path:   path,
			status: strconv.Itoa(c.Writer.Status()),
		}
		elapsed := time.Since(start).Seconds()
		m.mu.Lock()
		series, ok := m.requests[key]
		if !ok {
			series = &httpMetricsSeries{}
			m.requests[key] = series
		}
		series.count++
		series.durationSeconds += elapsed
		m.mu.Unlock()
	}
}

func (m *httpMetricsCollector) handler(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	snapshot := make(map[httpMetricsKey]httpMetricsSeries, len(m.requests))
	for key, series := range m.requests {
		snapshot[key] = *series
	}
	uptimeSeconds := time.Since(m.started).Seconds()
	m.mu.Unlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	rendered := &strings.Builder{}
	writeMetricFamily(rendered, "xianzhi_http_requests_total", "Total HTTP requests handled.", "counter",
		func() {
			keys := make([]httpMetricsKey, 0, len(snapshot))
			for key := range snapshot {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool {
				if keys[i].path != keys[j].path {
					return keys[i].path < keys[j].path
				}
				if keys[i].method != keys[j].method {
					return keys[i].method < keys[j].method
				}
				return keys[i].status < keys[j].status
			})
			for _, key := range keys {
				labels := fmt.Sprintf("method=%q,path=%q,status=%q",
					escapeLabelValue(key.method), escapeLabelValue(key.path), escapeLabelValue(key.status))
				fmt.Fprintf(rendered, "xianzhi_http_requests_total{%s} %s\n", labels, metricsFormatFloat(snapshot[key].count))
			}
		})
	writeMetricFamily(rendered, "xianzhi_http_request_duration_seconds_sum", "Cumulative request duration in seconds.", "counter",
		func() {
			renderDurationLabels(rendered, snapshot, "_sum", func(s httpMetricsSeries) float64 { return s.durationSeconds })
		})
	writeMetricFamily(rendered, "xianzhi_http_request_duration_seconds_count", "Number of requests observed for duration accounting.", "counter",
		func() {
			renderDurationLabels(rendered, snapshot, "_count", func(s httpMetricsSeries) float64 { return s.count })
		})

	gauges := []struct {
		name  string
		help  string
		value float64
	}{
		{"xianzhi_process_uptime_seconds", "Seconds since process start.", uptimeSeconds},
		{"xianzhi_process_goroutines", "Current goroutine count.", float64(runtime.NumGoroutine())},
		{"xianzhi_process_heap_alloc_bytes", "Heap bytes allocated and still in use.", float64(mem.HeapAlloc)},
		{"xianzhi_process_sys_bytes", "Total bytes of memory obtained from the OS.", float64(mem.Sys)},
		{"xianzhi_process_gc_cycles_total", "Completed GC cycles.", float64(mem.NumGC)},
	}
	for _, gauge := range gauges {
		writeMetricFamily(rendered, gauge.name, gauge.help, "gauge", func() {
			fmt.Fprintf(rendered, "%s %s\n", gauge.name, metricsFormatFloat(gauge.value))
		})
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(rendered.String()))
}

func writeMetricFamily(b *strings.Builder, name, help, kind string, samples func()) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, kind)
	samples()
}

func renderDurationLabels(b *strings.Builder, snapshot map[httpMetricsKey]httpMetricsSeries, suffix string, value func(httpMetricsSeries) float64) {
	type durationKey struct{ method, path string }
	aggregates := make(map[durationKey]float64)
	order := make([]durationKey, 0, len(snapshot))
	for key := range snapshot {
		aggregate := durationKey{method: key.method, path: key.path}
		if _, ok := aggregates[aggregate]; !ok {
			order = append(order, aggregate)
		}
		aggregates[aggregate] += value(snapshot[key])
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].path != order[j].path {
			return order[i].path < order[j].path
		}
		return order[i].method < order[j].method
	})
	for _, aggregate := range order {
		labels := fmt.Sprintf("method=%q,path=%q", escapeLabelValue(aggregate.method), escapeLabelValue(aggregate.path))
		fmt.Fprintf(b, "xianzhi_http_request_duration_seconds%s{%s} %s\n", suffix, labels, metricsFormatFloat(aggregates[aggregate]))
	}
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return replacer.Replace(value)
}

func metricsFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
