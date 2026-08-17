package ppt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPhase2GenerationMigrationContainsDurabilityConstraints(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "database", "migrations", "109-ppt-v2-durable-generation.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.ToLower(string(raw))
	required := []string{
		"xz_ppt_v2_generation_jobs", "xz_ppt_v2_deck_jobs", "xz_ppt_v2_slide_jobs",
		"xz_ppt_v2_generation_attempts", "xz_ppt_v2_generation_transitions",
		"uq_ppt_v2_generation_job_idempotency", "fencing_token", "lease_expires_at",
		"cancel_requested_at", "render_bytes", "completed_work_units", "total_work_units",
		"uq_ppt_v2_work_center_asset_per_job", "uq_ppt_v2_file_per_job",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("Phase 2 migration is missing %q", fragment)
		}
	}
}

func TestPhase3SliceAPlanningMigrationContainsDurableApprovalSchema(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "database", "migrations", "110-ppt-v2-agent-outline-approval.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.ToLower(string(raw))
	required := []string{
		"workflow_type", "agent_outline", "waiting_for_outline_approval",
		"intent_resolved", "researched", "storyline_planned", "outline_planned", "outline_approved",
		"xz_ppt_v2_agent_plans", "xz_ppt_v2_outline_revisions",
		"current_outline_revision", "approved_outline_revision", "research_execution_count",
		"unique(generation_job_id,revision)",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("Phase 3 Slice A migration is missing %q", fragment)
		}
	}
}

func TestPhase3SliceBMigrationContainsDurableDeckGenerationSchema(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "database", "migrations", "111-ppt-v2-agent-deck-generation.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sqlText := strings.ToLower(string(raw))
	required := []string{
		"deck_state", "content_ready", "assets_ready", "layout_compiled", "quality_checked",
		"rendered", "file_stored", "asset_created", "task_related", "completed",
		"uq_ppt_v2_image_asset_per_intent", "ppt_v2_image_asset",
	}
	for _, fragment := range required {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("Phase 3 Slice B migration is missing %q", fragment)
		}
	}
}
