package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openPPTHTTPTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("PPT_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("PPT_TEST_DATABASE_URL is not configured")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, `select current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !isPPTAgentPhase1TestDatabaseName(databaseName) {
		t.Fatalf("PPT_TEST_DATABASE_URL must target a dedicated PPT Agent Phase 1 test database, got %q", databaseName)
	}
	return db
}

func isPPTAgentPhase1TestDatabaseName(name string) bool {
	const prefix = "ppt_agent_phase1_"
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, prefix) && strings.TrimSpace(strings.TrimPrefix(name, prefix)) != ""
}

type pptHTTPTestUser struct {
	ID       string
	Email    string
	Password string
}

type pptHTTPTestFixture struct {
	owner    pptHTTPTestUser
	other    pptHTTPTestUser
	admin    pptHTTPTestUser
	clientIP string
}

func newPPTHTTPTestFixture(t *testing.T) (*sql.DB, pptHTTPTestFixture) {
	t.Helper()
	db := openPPTHTTPTestDatabase(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	password := "PPTHTTPFixture123!"
	fixture := pptHTTPTestFixture{
		owner:    newPPTHTTPTestUser("owner", suffix, password),
		other:    newPPTHTTPTestUser("other", suffix, password),
		admin:    newPPTHTTPTestUser("admin", suffix, password),
		clientIP: pptHTTPTestClientIP(time.Now().UnixNano()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, user := range []struct {
		item pptHTTPTestUser
		role string
	}{
		{item: fixture.owner, role: "MEMBER"},
		{item: fixture.other, role: "MEMBER"},
		{item: fixture.admin, role: "SUPER_ADMIN"},
	} {
		passwordHash, err := hashPassword(user.item.Password)
		if err != nil {
			t.Fatal(err)
		}
		if err := insertUser(ctx, tx, adminUser{
			ID: user.item.ID, Email: user.item.Email, Name: user.item.ID, Role: user.role,
			MemberLevel: memberLevelBasic, AgentStatus: agentStatusNone, OperationCenterStatus: operationStatusNone,
			Status: "ACTIVE", PasswordHash: passwordHash, PlanID: "plan_month",
		}); err != nil {
			t.Fatal(err)
		}
		if err := insertPointAccount(ctx, tx, adminPointAccount{ID: "points_" + user.item.ID, UserID: user.item.ID, Available: 100}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupPPTHTTPTestFixture(t, db, fixture) })
	return db, fixture
}

func newPPTHTTPTestUser(kind string, suffix string, password string) pptHTTPTestUser {
	id := "ppt_http_" + kind + "_" + suffix
	return pptHTTPTestUser{ID: id, Email: id + "@example.test", Password: password}
}

func pptHTTPTestClientIP(seed int64) string {
	return fmt.Sprintf("198.18.%d.%d", 1+seed%250, 1+(seed/250)%250)
}

func (f pptHTTPTestFixture) loginToken(t *testing.T, handler http.Handler, user pptHTTPTestUser) string {
	t.Helper()
	response := f.request(t, handler, http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"`+user.Email+`","password":"`+user.Password+`"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, body = %s", user.Email, response.Code, response.Body.String())
	}
	var payload struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.AccessToken == "" {
		t.Fatalf("login %s returned empty token", user.Email)
	}
	return payload.AccessToken
}

func (f pptHTTPTestFixture) authedRequest(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, token string) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Authorization", "Bearer "+token)
	return f.serve(t, handler, request)
}

func (f pptHTTPTestFixture) request(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	return f.serve(t, handler, httptest.NewRequest(method, path, body))
}

func (f pptHTTPTestFixture) serve(t *testing.T, handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	request.Header.Set("X-Forwarded-For", f.clientIP)
	request.RemoteAddr = net.JoinHostPort(f.clientIP, "12345")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func (f pptHTTPTestFixture) waitForPPTUsageEvent(t *testing.T, handler http.Handler, token string, taskID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		response := f.authedRequest(t, handler, http.MethodGet, "/api/v1/user/usage", nil, token)
		if response.Code != http.StatusOK {
			t.Fatalf("usage status = %d, body = %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), taskID) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ppt usage event for %s did not appear before timeout: %s", taskID, response.Body.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (f pptHTTPTestFixture) waitForPPTBillingTaskID(t *testing.T, db *sql.DB, pptTaskID string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var billingTaskID string
		err := db.QueryRowContext(context.Background(), `
			select coalesce(raw->>'billingTaskId','')
			from xz_ppt_tasks
			where user_id = $1 and task_id = $2
		`, f.owner.ID, pptTaskID).Scan(&billingTaskID)
		if err != nil {
			t.Fatal(err)
		}
		if billingTaskID != "" {
			return billingTaskID
		}
		if time.Now().After(deadline) {
			t.Fatalf("ppt task %s did not bind a billing task before timeout", pptTaskID)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (f pptHTTPTestFixture) assertPPTBillingEventProjection(t *testing.T, db *sql.DB, userID string, billingTaskID string) {
	t.Helper()
	var count int
	var eventUserID, metricCode, rawUserID, rawMetricCode string
	var pointCost int
	var rawPointCost string
	err := db.QueryRowContext(context.Background(), `
		select count(*), coalesce(max(user_id), ''), coalesce(max(metric_code), ''), coalesce(max(point_cost), 0),
		       coalesce(max(raw->>'userId'), ''), coalesce(max(raw->>'metricCode'), ''), coalesce(max(raw->>'pointCost'), '')
		from xz_billing_events
		where user_id = $1 and task_id = $2
	`, userID, billingTaskID).Scan(&count, &eventUserID, &metricCode, &pointCost, &rawUserID, &rawMetricCode, &rawPointCost)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || eventUserID != userID || rawUserID != userID || metricCode != billingMetricPPTGenerate || rawMetricCode != billingMetricPPTGenerate || pointCost != 3 || rawPointCost != "3" {
		t.Fatalf("unexpected persisted PPT billing projection: count=%d user=%q rawUser=%q metric=%q rawMetric=%q pointCost=%d rawPointCost=%q", count, eventUserID, rawUserID, metricCode, rawMetricCode, pointCost, rawPointCost)
	}
}

func cleanupPPTHTTPTestFixture(t *testing.T, db *sql.DB, fixture pptHTTPTestFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, user := range []pptHTTPTestUser{fixture.owner, fixture.other, fixture.admin} {
		for _, statement := range []string{
			`delete from xz_assets where user_id = $1`,
			`delete from xz_generation_tasks where user_id = $1`,
			`delete from xz_ppt_tasks where user_id = $1`,
			`delete from xz_wallet_ledger where user_id = $1`,
			`delete from xz_billing_lifecycle_events where user_id = $1`,
			`delete from xz_billing_events where user_id = $1`,
			`delete from xz_audit_logs where actor_id = $1`,
			`delete from xz_user_wallets where user_id = $1`,
			`delete from xz_point_accounts where user_id = $1`,
			`delete from xz_users where id = $1`,
		} {
			if _, err := db.ExecContext(ctx, statement, user.ID); err != nil {
				t.Errorf("cleanup fixture user %s: %v", user.ID, err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `delete from xz_audit_logs where metadata->>'clientIP' = $1`, fixture.clientIP); err != nil {
		t.Errorf("cleanup fixture audit records: %v", err)
	}
}

func TestPPTHTTPTestDatabaseNameAllowed(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "ppt_agent_phase1_candidate_20260805", want: true},
		{name: "ppt_agent_phase1_isolated", want: true},
		{name: "xianzhi_test", want: false},
		{name: "production", want: false},
		{name: "ppt_agent_phase2_candidate", want: false},
	} {
		if got := isPPTAgentPhase1TestDatabaseName(test.name); got != test.want {
			t.Errorf("database name %q allowed=%v, want %v", test.name, got, test.want)
		}
	}
}
