package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var schedulerMetrics struct {
	dispatched atomic.Uint64
	recovered  atomic.Uint64
	errors     atomic.Uint64
}

// Counters belong to this process. Scrape every API and independent worker;
// database gauges are shared snapshots and must NOT be summed across replicas.
func renderSchedulerMetrics(b *strings.Builder, db *sql.DB) {
	for _, m := range []struct {
		name  string
		value uint64
	}{
		{"dispatched", schedulerMetrics.dispatched.Load()},
		{"recovered", schedulerMetrics.recovered.Load()},
		{"errors", schedulerMetrics.errors.Load()},
	} {
		name := "generation_scheduler_" + m.name + "_total"
		fmt.Fprintf(b, "# TYPE %s counter\n%s %d\n", name, name, m.value)
	}
	if db == nil {
		fmt.Fprintln(b, "generation_scheduler_db_scrape_success 0")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `
		SELECT upper(coalesce(nullif(task_status,''), status)), count(*)
		FROM xz_generation_tasks
		WHERE upper(coalesce(type, '')) NOT IN ('PPT_GENERATION', 'PPT')
		GROUP BY 1
	`)
	if err != nil {
		fmt.Fprintln(b, "generation_scheduler_db_scrape_success 0")
		return
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var state string
		var n int64
		if err := rows.Scan(&state, &n); err != nil {
			fmt.Fprintln(b, "generation_scheduler_db_scrape_success 0")
			return
		}
		counts[state] = n
	}
	if rows.Err() != nil {
		fmt.Fprintln(b, "generation_scheduler_db_scrape_success 0")
		return
	}
	fmt.Fprintln(b, "generation_scheduler_db_scrape_success 1")
	for _, state := range []string{"QUEUED", "DISPATCHING", "RUNNING", "PROCESSING", "FAILED"} {
		fmt.Fprintf(b, "generation_scheduler_tasks{state=%q} %d\n", state, counts[state])
	}
}

// GenerationWorkerMetricsHandler exposes independent-worker process counters
// as well as shared DB diagnostics, without HTTP API initialization.
func GenerationWorkerMetricsHandler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		b := &strings.Builder{}
		renderSchedulerMetrics(b, db)
		fmt.Fprintf(b, "# TYPE generation_worker_failed_total counter\ngeneration_worker_failed_total %d\n", generationCanaryMetrics.failed.Load())
		fmt.Fprintf(b, "# TYPE generation_worker_recovered_total counter\ngeneration_worker_recovered_total %d\n", generationCanaryMetrics.recovered.Load())
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
	})
}
