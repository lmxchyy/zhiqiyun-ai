package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresPersonalPointPolicyPUTArchivesAndPublishesAtomically(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PERSONAL_POINTS_POLICY_POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("PERSONAL_POINTS_POLICY_POSTGRES_TEST_DSN is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var triggerDefinition string
	if err := db.QueryRowContext(ctx, `SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid='xz_point_expiry_policy_versions'::regclass AND tgname='trg_xz_point_expiry_policy_versions_immutable' AND NOT tgisinternal`).Scan(&triggerDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(triggerDefinition, "INSERT") {
		t.Fatalf("policy trigger is not migration 104: %s", triggerDefinition)
	}

	admin := newAdminAPI(&postgresStore{db: db, ready: true}, newLocalAuthSessions())
	get := httptest.NewRecorder()
	admin.pointExpiryPolicy(get, httptest.NewRequest(http.MethodGet, "/api/v1/admin/points/expiry-policy", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}
	var currentBody struct {
		Item PointExpiryPolicy `json:"item"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &currentBody); err != nil {
		t.Fatal(err)
	}
	current := currentBody.Item

	type policyBusinessFields struct {
		enabled, durationValue                                 any
		durationUnit, timeZone, sourceTypes, createdBy, reason any
		metadata, createdAt                                    any
	}
	var oldBefore policyBusinessFields
	if err := db.QueryRowContext(ctx, `SELECT enabled,duration_value,duration_unit,time_zone,source_types::text,created_by,change_reason,metadata::text,created_at::text FROM xz_point_expiry_policy_versions WHERE id=$1`, current.ID).Scan(
		&oldBefore.enabled, &oldBefore.durationValue, &oldBefore.durationUnit, &oldBefore.timeZone, &oldBefore.sourceTypes, &oldBefore.createdBy, &oldBefore.reason, &oldBefore.metadata, &oldBefore.createdAt,
	); err != nil {
		t.Fatal(err)
	}

	invalid := httptest.NewRecorder()
	invalidReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewBufferString(`{"revision":1,"enabled":true,"durationValue":0,"changeReason":"invalid duration"}`))
	invalidReq = invalidReq.WithContext(context.WithValue(invalidReq.Context(), actorIDContextKey, "policy-api-test"))
	admin.pointExpiryPolicy(invalid, invalidReq)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	payload, err := json.Marshal(map[string]any{
		"revision": current.Revision, "enabled": !current.Enabled, "durationValue": 4, "changeReason": "migration 104 API verification",
	})
	if err != nil {
		t.Fatal(err)
	}
	put := httptest.NewRecorder()
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewReader(payload))
	putReq = putReq.WithContext(context.WithValue(putReq.Context(), actorIDContextKey, "policy-api-test"))
	admin.pointExpiryPolicy(put, putReq)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", put.Code, put.Body.String())
	}
	var publishedBody struct {
		Item PointExpiryPolicy `json:"item"`
	}
	if err := json.Unmarshal(put.Body.Bytes(), &publishedBody); err != nil {
		t.Fatal(err)
	}
	published := publishedBody.Item
	if published.Revision != current.Revision+1 || published.Status != "PUBLISHED" || published.DurationValue != 4 || published.Enabled == current.Enabled {
		t.Fatalf("published policy = %+v, current = %+v", published, current)
	}

	var oldStatus string
	var oldAfter policyBusinessFields
	if err := db.QueryRowContext(ctx, `SELECT status,enabled,duration_value,duration_unit,time_zone,source_types::text,created_by,change_reason,metadata::text,created_at::text FROM xz_point_expiry_policy_versions WHERE id=$1`, current.ID).Scan(
		&oldStatus, &oldAfter.enabled, &oldAfter.durationValue, &oldAfter.durationUnit, &oldAfter.timeZone, &oldAfter.sourceTypes, &oldAfter.createdBy, &oldAfter.reason, &oldAfter.metadata, &oldAfter.createdAt,
	); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "ARCHIVED" || oldAfter != oldBefore {
		t.Fatalf("old policy status/business fields changed unexpectedly: status=%s before=%+v after=%+v", oldStatus, oldBefore, oldAfter)
	}
	var publishedCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' AND id=$1`, published.ID).Scan(&publishedCount); err != nil {
		t.Fatal(err)
	}
	if publishedCount != 1 {
		t.Fatalf("new PUBLISHED policy count=%d, want 1", publishedCount)
	}

	stale := httptest.NewRecorder()
	staleReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewReader(payload))
	staleReq = staleReq.WithContext(context.WithValue(staleReq.Context(), actorIDContextKey, "policy-api-test"))
	admin.pointExpiryPolicy(stale, staleReq)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale PUT status=%d body=%s", stale.Code, stale.Body.String())
	}
	var concurrentOldBefore policyBusinessFields
	if err := db.QueryRowContext(ctx, `SELECT enabled,duration_value,duration_unit,time_zone,source_types::text,created_by,change_reason,metadata::text,created_at::text FROM xz_point_expiry_policy_versions WHERE id=$1`, published.ID).Scan(
		&concurrentOldBefore.enabled, &concurrentOldBefore.durationValue, &concurrentOldBefore.durationUnit, &concurrentOldBefore.timeZone, &concurrentOldBefore.sourceTypes, &concurrentOldBefore.createdBy, &concurrentOldBefore.reason, &concurrentOldBefore.metadata, &concurrentOldBefore.createdAt,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `CREATE FUNCTION aa_test_policy_publish_delay() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.status='PUBLISHED' AND NEW.status='ARCHIVED' THEN PERFORM pg_sleep(1); END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER aa_test_policy_publish_delay BEFORE UPDATE ON xz_point_expiry_policy_versions FOR EACH ROW EXECUTE FUNCTION aa_test_policy_publish_delay()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS aa_test_policy_publish_delay ON xz_point_expiry_policy_versions`)
		_, _ = db.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS aa_test_policy_publish_delay()`)
	})
	concurrentPayload, err := json.Marshal(map[string]any{
		"revision": published.Revision, "enabled": published.Enabled, "durationValue": 5, "changeReason": "concurrent migration 104 API verification",
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var requests sync.WaitGroup
	for range 2 {
		requests.Add(1)
		go func() {
			defer requests.Done()
			<-start
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/points/expiry-policy", bytes.NewReader(concurrentPayload))
			request = request.WithContext(context.WithValue(request.Context(), actorIDContextKey, "policy-api-concurrent-test"))
			admin.pointExpiryPolicy(recorder, request)
			statuses <- recorder.Code
		}()
	}
	close(start)
	requests.Wait()
	close(statuses)
	statusCounts := map[int]int{}
	for status := range statuses {
		statusCounts[status]++
	}
	if statusCounts[http.StatusOK] != 1 || statusCounts[http.StatusConflict] != 1 || len(statusCounts) != 2 {
		t.Fatalf("concurrent PUT statuses=%v, want one 200 and one 409", statusCounts)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED'`).Scan(&publishedCount); err != nil {
		t.Fatal(err)
	}
	if publishedCount != 1 {
		t.Fatalf("concurrent PUT left PUBLISHED count=%d, want 1", publishedCount)
	}
	var concurrentOldStatus string
	var concurrentOldAfter policyBusinessFields
	if err := db.QueryRowContext(ctx, `SELECT status,enabled,duration_value,duration_unit,time_zone,source_types::text,created_by,change_reason,metadata::text,created_at::text FROM xz_point_expiry_policy_versions WHERE id=$1`, published.ID).Scan(
		&concurrentOldStatus, &concurrentOldAfter.enabled, &concurrentOldAfter.durationValue, &concurrentOldAfter.durationUnit, &concurrentOldAfter.timeZone, &concurrentOldAfter.sourceTypes, &concurrentOldAfter.createdBy, &concurrentOldAfter.reason, &concurrentOldAfter.metadata, &concurrentOldAfter.createdAt,
	); err != nil {
		t.Fatal(err)
	}
	if concurrentOldStatus != "ARCHIVED" || concurrentOldAfter != concurrentOldBefore {
		t.Fatalf("concurrent old policy status/business fields changed unexpectedly: status=%s before=%+v after=%+v", concurrentOldStatus, concurrentOldBefore, concurrentOldAfter)
	}
}
