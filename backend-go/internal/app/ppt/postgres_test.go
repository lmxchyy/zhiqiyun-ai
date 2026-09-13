package ppt

import (
	"encoding/json"
	"testing"
)

func TestDecodePersistedTasksImportsLegacySlidesWithoutVisualPlan(t *testing.T) {
	raw, err := json.Marshal(persistedState{Tasks: []persistedTask{{
		UserID: "user_legacy",
		Task:   Task{TaskID: "ppt_legacy", Status: StatusSuccess, Slides: []Slide{{ID: "slide_1", Title: "Legacy", Content: "Body"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	tasks := decodePersistedTasks(raw)
	if len(tasks) != 1 || tasks[0].UserID != "user_legacy" {
		t.Fatalf("unexpected imported tasks: %#v", tasks)
	}
	if tasks[0].Slides[0].SlideType != "text_image" {
		t.Fatalf("legacy slide type = %q", tasks[0].Slides[0].SlideType)
	}
	if tasks[0].Slides[0].VisualPlan != nil {
		t.Fatal("legacy task import must not require a visual plan")
	}
}

func TestTaskFromGenerateRequestKeepsDeckVisualStyleAndNoTextDefault(t *testing.T) {
	req := normalizeRequest(GenerateRequest{
		UserID: "user_a", Prompt: "Enterprise AI", SlideCount: 1, Theme: "techBlue",
		ImageStyle: "corporate 3D", PeopleStyle: "natural", ImageLighting: "soft",
		Outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", Summary: "AI assistant", SlideType: "cover"}}},
	})
	task := taskFromGenerateRequest(req)
	if task.UserID != req.UserID || task.ImageStyle != "corporate 3D" || task.TextInImage {
		t.Fatalf("unexpected task visual defaults: %#v", task)
	}
	if len(task.Slides) != 1 || task.Slides[0].VisualPlan == nil || task.Slides[0].VisualPlan.TextInImage {
		t.Fatalf("unexpected slide visual plan: %#v", task.Slides)
	}
}

func TestNewPostgresServiceWithoutDatabaseFallsBackToFileService(t *testing.T) {
	service := NewPostgresService(nil, "")
	if service == nil || service.db != nil {
		t.Fatalf("expected file service fallback: %#v", service)
	}
}

func TestTaskFromGenerateRequest_TenantID_ExplicitAndFallback(t *testing.T) {
	// Case 1: Explicit TenantID provided
	reqWithTenant := GenerateRequest{
		TenantID: "tenant_enterprise_99",
		UserID:   "user_100",
		Prompt:   "Q3 Review",
	}
	taskWithTenant := TaskFromGenerateRequest("task_001", reqWithTenant)
	if taskWithTenant.TenantID != "tenant_enterprise_99" {
		t.Fatalf("expected explicit tenant_id 'tenant_enterprise_99', got %q", taskWithTenant.TenantID)
	}

	// Case 2: Empty TenantID -> fallback to DefaultTenantID ("tenant_default")
	reqEmptyTenant := GenerateRequest{
		TenantID: "",
		UserID:   "user_101",
		Prompt:   "Annual Plan",
	}
	taskEmptyTenant := TaskFromGenerateRequest("task_002", reqEmptyTenant)
	if taskEmptyTenant.TenantID != DefaultTenantID {
		t.Fatalf("expected fallback tenant_id %q, got %q", DefaultTenantID, taskEmptyTenant.TenantID)
	}

	// Case 3: Whitespace TenantID -> trimmed, fallback to DefaultTenantID if blank after trim
	reqSpacesTenant := GenerateRequest{
		TenantID: "   ",
		UserID:   "user_102",
		Prompt:   "Budget Report",
	}
	taskSpacesTenant := TaskFromGenerateRequest("task_003", reqSpacesTenant)
	if taskSpacesTenant.TenantID != DefaultTenantID {
		t.Fatalf("expected whitespace tenant_id to fallback to %q, got %q", DefaultTenantID, taskSpacesTenant.TenantID)
	}

	// Case 4: Whitespace padded TenantID -> trimmed to valid tenant
	reqPaddedTenant := GenerateRequest{
		TenantID: "  tenant_trimmed_01  ",
		UserID:   "user_103",
		Prompt:   "Tech Spec",
	}
	taskPaddedTenant := TaskFromGenerateRequest("task_004", reqPaddedTenant)
	if taskPaddedTenant.TenantID != "tenant_trimmed_01" {
		t.Fatalf("expected trimmed tenant_id 'tenant_trimmed_01', got %q", taskPaddedTenant.TenantID)
	}
}

func TestNormalizeLegacyTask_TenantIDFallback(t *testing.T) {
	// Task with empty TenantID
	legacy := Task{
		TaskID: "legacy_001",
		Status: StatusPending,
	}
	normalized := normalizeLegacyTask(legacy)
	if normalized.TenantID != DefaultTenantID {
		t.Fatalf("expected normalized legacy task to have tenant_id %q, got %q", DefaultTenantID, normalized.TenantID)
	}

	// Task with existing TenantID preserved
	custom := Task{
		TaskID:   "custom_001",
		TenantID: "tenant_custom",
		Status:   StatusPending,
	}
	normalizedCustom := normalizeLegacyTask(custom)
	if normalizedCustom.TenantID != "tenant_custom" {
		t.Fatalf("expected existing tenant_id 'tenant_custom' to be preserved, got %q", normalizedCustom.TenantID)
	}
}

func TestTaskFromPostgresRaw_TenantID_ExplicitAndFallback(t *testing.T) {
	// Raw JSON with tenantId present
	rawExplicit := []byte(`{"taskId":"task_exp","tenantId":"tenant_finance_01","status":"pending","title":"Finance"}`)
	taskExp, err := taskFromPostgresRaw(rawExplicit, "user_f1")
	if err != nil {
		t.Fatalf("taskFromPostgresRaw error: %v", err)
	}
	if taskExp.TenantID != "tenant_finance_01" {
		t.Fatalf("expected tenant_id 'tenant_finance_01', got %q", taskExp.TenantID)
	}

	// Raw JSON without tenantId (legacy DB row)
	rawMissing := []byte(`{"taskId":"task_legacy","status":"pending","title":"Legacy Deck"}`)
	taskLegacy, err := taskFromPostgresRaw(rawMissing, "user_l1")
	if err != nil {
		t.Fatalf("taskFromPostgresRaw error: %v", err)
	}
	if taskLegacy.TenantID != DefaultTenantID {
		t.Fatalf("expected fallback tenant_id %q, got %q", DefaultTenantID, taskLegacy.TenantID)
	}

	// Raw JSON with blank tenantId
	rawBlank := []byte(`{"taskId":"task_blank","tenantId":"   ","status":"pending","title":"Blank Deck"}`)
	taskBlank, err := taskFromPostgresRaw(rawBlank, "user_b1")
	if err != nil {
		t.Fatalf("taskFromPostgresRaw error: %v", err)
	}
	if taskBlank.TenantID != DefaultTenantID {
		t.Fatalf("expected fallback tenant_id %q for blank string, got %q", DefaultTenantID, taskBlank.TenantID)
	}
}

func TestDecodePersistedTasks_TenantIDFallback(t *testing.T) {
	raw, err := json.Marshal(persistedState{Tasks: []persistedTask{{
		UserID: "user_legacy",
		Task:   Task{TaskID: "ppt_legacy_no_tenant", Status: StatusSuccess, Slides: []Slide{{ID: "s1", Title: "Title"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	tasks := decodePersistedTasks(raw)
	if len(tasks) != 1 {
		t.Fatalf("unexpected task count: %d", len(tasks))
	}
	if tasks[0].TenantID != DefaultTenantID {
		t.Fatalf("expected decoded legacy task to have tenant_id %q, got %q", DefaultTenantID, tasks[0].TenantID)
	}
}
