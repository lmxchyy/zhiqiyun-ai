package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func TestPR4ARecoveryDiagnosisUnknownPostgresBoundaries(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	a := newPPTDBAPI(t, stage0PPTCanaryConfig(), nil)
	taskID := fmt.Sprintf("pr4a-task-%d", time.Now().UnixNano())
	userID := fmt.Sprintf("pr4a-user-%d", time.Now().UnixNano())
	ctx := context.Background()
	pptTask := pptapp.Task{TaskID: taskID, UserID: userID, Status: pptapp.StatusProcessing, SlideCount: 3, Title: "PR4A recovery diagnosis", Prompt: "recovery", PlannedPages: 3, Stage: pptapp.StageOutline}
	rawPPT, err := json.Marshal(pptTask)
	if err != nil {
		t.Fatal(err)
	}
	parent := generationTask{
		ID: taskID, UserID: userID, Type: "PPT_GENERATION", Model: "kimi-k2.6", Status: "PROCESSING", TaskStatus: "PROCESSING", BillingStatus: "RESERVED", Progress: 20,
		PointCost: 10, QuotedPoints: 10, ReservedPoints: 10, Params: map[string]any{"generation_async_canary": true, "generation_ppt_async_canary": true}, ResultIDs: []string{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), Prompt: "recovery",
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertGenerationTask(ctx, tx, parent); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	var hasTenant bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='xz_ppt_tasks' AND column_name='tenant_id')`).Scan(&hasTenant); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	var pptInsertErr error
	if hasTenant {
		_, pptInsertErr = tx.ExecContext(ctx, `INSERT INTO xz_ppt_tasks(task_id,user_id,client_request_id,status,created_at,updated_at,raw,tenant_id,stage,skill_code,source_file_ids,organization_id) VALUES($1,$2,'','processing',now(),now(),$3::jsonb,'tenant_default','GENERATING','', '[]'::jsonb,'')`, taskID, userID, string(rawPPT))
	} else {
		_, pptInsertErr = tx.ExecContext(ctx, `INSERT INTO xz_ppt_tasks(task_id,user_id,client_request_id,status,created_at,updated_at,raw) VALUES($1,$2,'','processing',now(),now(),$3::jsonb)`, taskID, userID, string(rawPPT))
	}
	if pptInsertErr != nil {
		_ = tx.Rollback()
		t.Fatal(pptInsertErr)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM provider_executions WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE aggregate_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_ppt_tasks WHERE task_id=$1`, taskID)
		_, _ = db.ExecContext(ctx, `DELETE FROM xz_generation_tasks WHERE id=$1`, taskID)
	}()

	if _, err := db.ExecContext(ctx, `INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,request_fingerprint,provider_request_id) VALUES($1,'configured','kimi-k2.6','ppt_outline',1,'unknown',repeat('a',64),'external-unknown')`, taskID); err != nil {
		t.Fatal(err)
	}
	diagnosis, _, err := a.buildGenerationRecoveryDiagnosis(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if diagnosis.Provider["status"] != string(pe.Unknown) {
		t.Fatalf("provider diagnosis status=%v", diagnosis.Provider["status"])
	}
	allowed, ok := diagnosis.Recovery["allowedActions"].([]string)
	if !ok || len(allowed) != 2 || !recoveryContainsString(allowed, recoveryActionManualReview) || recoveryContainsString(allowed, recoveryActionRedrive) {
		t.Fatalf("unknown allowed actions=%#v", diagnosis.Recovery["allowedActions"])
	}
	if _, err := a.updateGenerationRecoveryState(taskID, "MANUAL_REVIEW", recoveryActionRequest{Reason: "external provider ambiguity"}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM xz_generation_tasks WHERE id=$1`, taskID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "MANUAL_REVIEW" {
		t.Fatalf("manual review did not fence task, status=%s", status)
	}
	beforeCount := 0
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_audit_logs WHERE resource='generation_task' AND resource_id=$1`, taskID).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	if err := insertRecoveryAudit(ctx, db, "operator-pr4a", "SUPER_ADMIN", taskID, recoveryActionRequest{Action: recoveryActionManualReview, Reason: "external provider ambiguity"}, diagnosis, diagnosis); err != nil {
		t.Fatal(err)
	}
	var afterCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_audit_logs WHERE resource='generation_task' AND resource_id=$1`, taskID).Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if afterCount != beforeCount+1 {
		t.Fatalf("recovery audit count before=%d after=%d", beforeCount, afterCount)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM xz_audit_logs WHERE resource='generation_task' AND resource_id=$1`, taskID)
}
