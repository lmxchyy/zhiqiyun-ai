package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/config"
)

func TestBusinessPlanAdminPhase2BPostgresHTTP(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var migrated bool
	if err := db.QueryRowContext(ctx, `select to_regclass('xz_plan_versions') is not null`).Scan(&migrated); err != nil || !migrated {
		t.Skip("migration 097 is not applied to the test database")
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	adminID := "bp_admin_" + suffix
	viewerID := "bp_viewer_" + suffix
	memberPlanID := "plan_member_" + suffix
	agentPlanID := "plan_agent_" + suffix
	otherPlanID := "plan_operation_" + suffix
	retirePlanID := "plan_retire_" + suffix
	concurrentPlanID := "plan_concurrent_" + suffix
	legacyRacePlanID := "plan_legacy_race_" + suffix
	legacyRaceVersionID := "version_legacy_race_" + suffix
	memberActiveID := "version_member_active_" + suffix
	memberDraftID := "version_member_draft_" + suffix
	retireActiveID := "version_retire_active_" + suffix
	concurrentActiveID := "version_concurrent_active_" + suffix
	concurrentDraftAID := "version_concurrent_a_" + suffix
	concurrentDraftBID := "version_concurrent_b_" + suffix

	seedUser := func(id, role string) {
		t.Helper()
		email := id + "@example.test"
		if _, err := db.ExecContext(ctx, `
			insert into xz_users(id,email,name,role,status,created_at,updated_at,raw)
			values($1,$2,$1,$3,'ACTIVE',$4,$4,jsonb_build_object(
				'id',$1::text,'email',$2::text,'name',$1::text,'role',$3::text,'status','ACTIVE'
			))
		`, id, email, role, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	seedPlan := func(id, planType string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `
			insert into xz_plans(id,code,name,plan_type,active,raw)
			values($1,$1,$1,$2,true,jsonb_build_object(
				'id',$1::text,'code',$1::text,'name',$1::text,'planType',$2::text,'active',true,
				'priceCents',0,'grantPoints',0,'durationDays',0,'concurrency',0,'entitlements','{}'::jsonb
			))
		`, id, planType); err != nil {
			t.Fatal(err)
		}
	}
	seedVersion := func(id, planID, businessType, status string, versionNo int) {
		t.Helper()
		memberLevel, agentLevel := any(nil), any(nil)
		if businessType == "MEMBER" {
			memberLevel = "PRO"
		} else {
			agentLevel = "AGENT"
		}
		if _, err := db.ExecContext(ctx, `
			insert into xz_plan_versions(
				id,plan_id,version_no,business_type,rights_snapshot,member_level,agent_level,
				token_amount,points_amount,duration_days,commission_rule_version,commission_snapshot,status
			) values($1,$2,$3,$4,
				jsonb_build_object('memberLevel',$5::text,'agentLevel',$6::text,'tokenAmount',100,'pointsAmount',10,'durationDays',30),
				$5,$6,100,10,30,'commission-v1','{"rules":[]}'::jsonb,$7)
		`, id, planID, versionNo, businessType, memberLevel, agentLevel, status); err != nil {
			t.Fatal(err)
		}
	}

	seedUser(adminID, "SUPER_ADMIN")
	seedUser(viewerID, "ADMIN")
	seedPlan(memberPlanID, "MEMBER_PACKAGE")
	seedPlan(agentPlanID, "AGENT_JOIN_PACKAGE")
	seedPlan(otherPlanID, "OPERATION_CENTER_PACKAGE")
	seedPlan(retirePlanID, "MEMBER_PACKAGE")
	seedPlan(concurrentPlanID, "MEMBER_PACKAGE")
	seedPlan(legacyRacePlanID, "MEMBER_PACKAGE")
	seedVersion(memberActiveID, memberPlanID, "MEMBER", "ACTIVE", 1)
	seedVersion(memberDraftID, memberPlanID, "MEMBER", "DRAFT", 2)
	seedVersion("version_agent_active_"+suffix, agentPlanID, "AGENT", "ACTIVE", 1)
	seedVersion("version_operation_misbound_"+suffix, otherPlanID, "MEMBER", "DRAFT", 1)
	seedVersion(retireActiveID, retirePlanID, "MEMBER", "ACTIVE", 1)
	seedVersion(concurrentActiveID, concurrentPlanID, "MEMBER", "ACTIVE", 1)
	seedVersion(concurrentDraftAID, concurrentPlanID, "MEMBER", "DRAFT", 2)
	seedVersion(concurrentDraftBID, concurrentPlanID, "MEMBER", "DRAFT", 3)
	var legacyVersionNo int
	if err := db.QueryRowContext(ctx, `select coalesce(max(version_no),0)+1 from xz_plan_versions where plan_id='plan_ai_creator_996'`).Scan(&legacyVersionNo); err != nil {
		t.Fatal(err)
	}
	seedVersion("version_legacy_"+suffix, "plan_ai_creator_996", "MEMBER", "DRAFT", legacyVersionNo)

	t.Setenv("XIANZHI_ENFORCE_RBAC", "true")
	sessions := newMemoryAuthSessions()
	adminToken := "bp-admin-token-" + suffix
	viewerToken := "bp-viewer-token-" + suffix
	if err := sessions.Put(ctx, adminToken, adminID, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Put(ctx, viewerToken, viewerID, time.Hour); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	handler := newWithStoreAndSessions(config.Config{Addr: ":0", StaticDir: t.TempDir(), AdminStaticDir: t.TempDir()}, store, sessions).Handler

	decodeErrorCode := func(t *testing.T, response *httptest.ResponseRecorder) string {
		t.Helper()
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return payload.Code
	}

	t.Run("business plan endpoints are read-only and V2 scoped", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans", nil, adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
		}
		var payload struct {
			Items []struct {
				ID           string `json:"id"`
				Code         string `json:"code"`
				BusinessType string `json:"businessType"`
				LegacyCode   bool   `json:"legacyCode"`
				CodeReadOnly bool   `json:"codeReadOnly"`
			} `json:"items"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		found := map[string]struct {
			BusinessType string
			Legacy       bool
			ReadOnly     bool
		}{}
		for _, item := range payload.Items {
			found[item.ID] = struct {
				BusinessType string
				Legacy       bool
				ReadOnly     bool
			}{item.BusinessType, item.LegacyCode, item.CodeReadOnly}
		}
		if found[memberPlanID].BusinessType != "MEMBER" || !found[memberPlanID].ReadOnly {
			t.Fatalf("member plan missing or mutable: %+v", found[memberPlanID])
		}
		if found[agentPlanID].BusinessType != "AGENT" || !found[agentPlanID].ReadOnly {
			t.Fatalf("agent plan missing or mutable: %+v", found[agentPlanID])
		}
		if _, ok := found[otherPlanID]; ok {
			t.Fatal("non-member/agent plan leaked into V2 business plan list")
		}
		if !found["plan_ai_creator_996"].Legacy {
			t.Fatal("historical member code is missing the legacy compatibility marker")
		}

		detail := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans/"+memberPlanID, nil, adminToken)
		if detail.Code != http.StatusOK {
			t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
		}
		patch := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/business-plans/"+memberPlanID, bytes.NewBufferString(`{"code":"changed"}`), adminToken)
		if patch.Code != http.StatusNotFound {
			t.Fatalf("business plan code update route exists: status=%d body=%s", patch.Code, patch.Body.String())
		}
	})

	t.Run("V2 managed plan rejects legacy PATCH", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/plans/"+memberPlanID, bytes.NewBufferString(`{"name":"must not mutate"}`), adminToken)
		if response.Code != http.StatusConflict || decodeErrorCode(t, response) != "MANAGED_PLAN_REQUIRES_VERSION" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("V2 managed plan rejects legacy capability writes before mutation", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodPut, "/api/v1/admin/plans/"+memberPlanID+"/capabilities", bytes.NewBufferString(`{"modules":[{"moduleCode":"image_generation","enabled":false,"allowedModels":[],"limits":{}}]}`), adminToken)
		if response.Code != http.StatusConflict || decodeErrorCode(t, response) != "MANAGED_PLAN_REQUIRES_VERSION" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("legacy PATCH cannot race a new V2 version after the ownership check", func(t *testing.T) {
		lockTx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lockTx.Rollback() }()
		var lockedID string
		if err := lockTx.QueryRowContext(ctx, `select id from xz_plans where id=$1 for update`, legacyRacePlanID).Scan(&lockedID); err != nil {
			t.Fatal(err)
		}

		applicationName := "phase2f_legacy_patch_" + suffix
		raceDB, err := sql.Open("pgx", dsn+"&application_name="+applicationName)
		if err != nil {
			t.Fatal(err)
		}
		defer raceDB.Close()
		raceDB.SetMaxOpenConns(1)
		raceStore := &postgresStore{db: raceDB, ready: true}
		result := make(chan error, 1)
		go func() {
			_, updateErr := raceStore.UpdateAdminPlan(legacyRacePlanID, adminPlanMutation{Name: "must stay legacy-only"})
			result <- updateErr
		}()

		deadline := time.Now().Add(5 * time.Second)
		for {
			var waiting bool
			if err := db.QueryRowContext(ctx, `
				select exists(
					select 1 from pg_stat_activity
					where datname=current_database() and application_name=$1
					  and state='active' and wait_event_type='Lock'
				)
			`, applicationName).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("legacy PATCH did not reach the parent-plan lock")
			}
			time.Sleep(10 * time.Millisecond)
		}

		if _, err := lockTx.ExecContext(ctx, `
			insert into xz_plan_versions(
				id,plan_id,version_no,business_type,rights_snapshot,member_level,
				token_amount,points_amount,duration_days,commission_rule_version,commission_snapshot,status
			) values($1,$2,1,'MEMBER','{}'::jsonb,'PRO',0,0,30,'commission-v1','{}'::jsonb,'DRAFT')
		`, legacyRaceVersionID, legacyRacePlanID); err != nil {
			t.Fatal(err)
		}
		if err := lockTx.Commit(); err != nil {
			t.Fatal(err)
		}

		select {
		case updateErr := <-result:
			var businessErr *businessPlanAdminError
			if !errors.As(updateErr, &businessErr) || businessErr.code != "MANAGED_PLAN_REQUIRES_VERSION" {
				t.Fatalf("legacy PATCH escaped V2 ownership gate: %v", updateErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("legacy PATCH did not finish after the V2 version committed")
		}
	})

	t.Run("user without pricing permission receives forbidden", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans", nil, viewerToken)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("non-member-agent plan cannot be managed", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans/"+otherPlanID, nil, adminToken)
		if response.Code != http.StatusNotFound {
			t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
		}
		response = authedRequest(t, handler, http.MethodPost, "/api/v1/admin/business-plans/"+otherPlanID+"/versions", bytes.NewBufferString(`{"reason":"not allowed"}`), adminToken)
		if response.Code != http.StatusNotFound {
			t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("create and list entitlement versions", func(t *testing.T) {
		body := `{"memberLevel":"PRO","durationDays":365,"tokenAmount":40000,"pointsAmount":100,"rightsSnapshot":{"feature":"member"},"commissionRuleVersion":"commission-v2","commissionSnapshot":{"rules":[]},"reason":"annual rights draft"}`
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/business-plans/"+memberPlanID+"/versions", bytes.NewBufferString(body), adminToken)
		if response.Code != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
		}
		var created struct {
			Item struct {
				Status    string         `json:"status"`
				Revision  int64          `json:"revision"`
				VersionNo int            `json:"versionNo"`
				CreatedBy string         `json:"createdBy"`
				Rights    map[string]any `json:"rightsSnapshot"`
			} `json:"item"`
		}
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		if created.Item.Status != "DRAFT" || created.Item.Revision != 1 || created.Item.VersionNo != 3 || created.Item.CreatedBy != adminID {
			t.Fatalf("unexpected created version: %+v", created.Item)
		}
		if created.Item.Rights["tokenAmount"] != float64(40000) || created.Item.Rights["memberLevel"] != "PRO" {
			t.Fatalf("canonical rights snapshot missing: %+v", created.Item.Rights)
		}

		list := authedRequest(t, handler, http.MethodGet, "/api/v1/admin/business-plans/"+memberPlanID+"/versions", nil, adminToken)
		if list.Code != http.StatusOK {
			t.Fatalf("list versions status=%d body=%s", list.Code, list.Body.String())
		}
	})

	t.Run("draft update uses revision and records actor reason", func(t *testing.T) {
		body := `{"revision":1,"memberLevel":"PRO","durationDays":60,"tokenAmount":500,"pointsAmount":20,"rightsSnapshot":{"feature":"updated"},"commissionRuleVersion":"commission-v2","commissionSnapshot":{"rules":[]},"reason":"increase member rights"}`
		response := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/plan-versions/"+memberDraftID, bytes.NewBufferString(body), adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("update status=%d body=%s", response.Code, response.Body.String())
		}
		var revision, tokenAmount int64
		var updatedBy, reason, status string
		if err := db.QueryRowContext(ctx, `select revision,token_amount,updated_by,change_reason,status from xz_plan_versions where id=$1`, memberDraftID).Scan(&revision, &tokenAmount, &updatedBy, &reason, &status); err != nil {
			t.Fatal(err)
		}
		if revision != 2 || tokenAmount != 500 || updatedBy != adminID || reason != "increase member rights" || status != "DRAFT" {
			t.Fatalf("revision=%d token=%d actor=%q reason=%q status=%q", revision, tokenAmount, updatedBy, reason, status)
		}

		conflict := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/plan-versions/"+memberDraftID, bytes.NewBufferString(body), adminToken)
		if conflict.Code != http.StatusConflict || decodeErrorCode(t, conflict) != "REVISION_CONFLICT" {
			t.Fatalf("status=%d body=%s", conflict.Code, conflict.Body.String())
		}
	})

	t.Run("active and retired versions cannot be edited or restored", func(t *testing.T) {
		updateActive := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/plan-versions/"+retireActiveID, bytes.NewBufferString(`{"revision":1,"tokenAmount":999,"reason":"invalid active edit"}`), adminToken)
		if updateActive.Code != http.StatusConflict {
			t.Fatalf("active update status=%d body=%s", updateActive.Code, updateActive.Body.String())
		}
		retire := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/plan-versions/"+retireActiveID+"/retire", bytes.NewBufferString(`{"revision":1,"reason":"replace active rights"}`), adminToken)
		if retire.Code != http.StatusOK {
			t.Fatalf("retire status=%d body=%s", retire.Code, retire.Body.String())
		}
		updateRetired := authedRequest(t, handler, http.MethodPatch, "/api/v1/admin/plan-versions/"+retireActiveID, bytes.NewBufferString(`{"revision":2,"tokenAmount":999,"reason":"invalid retired edit"}`), adminToken)
		if updateRetired.Code != http.StatusConflict {
			t.Fatalf("retired update status=%d body=%s", updateRetired.Code, updateRetired.Body.String())
		}
		restore := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/plan-versions/"+retireActiveID+"/activate", bytes.NewBufferString(`{"revision":2,"reason":"invalid restore"}`), adminToken)
		if restore.Code != http.StatusConflict {
			t.Fatalf("retired activate status=%d body=%s", restore.Code, restore.Body.String())
		}
	})

	t.Run("activating draft retires old active in one transaction", func(t *testing.T) {
		response := authedRequest(t, handler, http.MethodPost, "/api/v1/admin/plan-versions/"+memberDraftID+"/activate", bytes.NewBufferString(`{"revision":2,"reason":"publish revised member rights"}`), adminToken)
		if response.Code != http.StatusOK {
			t.Fatalf("activate status=%d body=%s", response.Code, response.Body.String())
		}
		rows, err := db.QueryContext(ctx, `select id,status,revision from xz_plan_versions where id in($1,$2) order by id`, memberActiveID, memberDraftID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		statuses := map[string]string{}
		for rows.Next() {
			var id, status string
			var revision int64
			if err := rows.Scan(&id, &status, &revision); err != nil {
				t.Fatal(err)
			}
			statuses[id] = status
		}
		if statuses[memberActiveID] != "RETIRED" || statuses[memberDraftID] != "ACTIVE" {
			t.Fatalf("unexpected statuses: %+v", statuses)
		}
	})

	t.Run("concurrent activation leaves exactly one active version", func(t *testing.T) {
		activate := func(versionID string) int {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plan-versions/"+versionID+"/activate", bytes.NewBufferString(`{"revision":1,"reason":"concurrent activation"}`))
			req.Header.Set("Authorization", "Bearer "+adminToken)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			return response.Code
		}
		var wg sync.WaitGroup
		statuses := make(chan int, 2)
		for _, versionID := range []string{concurrentDraftAID, concurrentDraftBID} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				statuses <- activate(id)
			}(versionID)
		}
		wg.Wait()
		close(statuses)
		for status := range statuses {
			if status != http.StatusOK && status != http.StatusConflict {
				t.Fatalf("unexpected activation status %d", status)
			}
		}
		var activeCount int
		if err := db.QueryRowContext(ctx, `select count(*) from xz_plan_versions where plan_id=$1 and status='ACTIVE'`, concurrentPlanID).Scan(&activeCount); err != nil {
			t.Fatal(err)
		}
		if activeCount != 1 {
			t.Fatalf("active version count=%d", activeCount)
		}
	})
}
