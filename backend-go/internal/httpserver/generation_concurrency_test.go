package httpserver

import (
	"errors"
	"testing"
	"time"
)

func TestQueueLimitErrorWrapping(t *testing.T) {
	if !errors.Is(errGenerationQueueLimitExceeded, errGenerationConcurrencyLimit) {
		t.Fatalf("errGenerationQueueLimitExceeded must wrap errGenerationConcurrencyLimit for backwards compatibility")
	}
	err := generationQueueLimitError("plan_basic", 30, 30)
	if !errors.Is(err, errGenerationConcurrencyLimit) {
		t.Fatalf("generationQueueLimitError must be recognizable as errGenerationConcurrencyLimit")
	}
	if !errors.Is(err, errGenerationQueueLimitExceeded) {
		t.Fatalf("generationQueueLimitError must be recognizable as errGenerationQueueLimitExceeded")
	}
}

func TestIsSubscriptionExpired(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiresAt string
		want      bool
	}{
		{name: "empty expiresAt", expiresAt: "", want: false},
		{name: "whitespace only", expiresAt: "   ", want: false},
		{name: "invalid date format", expiresAt: "not-a-date", want: false},
		{name: "expired past RFC3339", expiresAt: "2026-09-15T12:00:00Z", want: true},
		{name: "expired past RFC3339Nano", expiresAt: "2026-09-15T12:00:00.123456789Z", want: true},
		{name: "active future RFC3339", expiresAt: "2026-09-17T12:00:00Z", want: false},
		{name: "active future RFC3339Nano", expiresAt: "2026-09-17T12:00:00.123456789Z", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSubscriptionExpired(tt.expiresAt, now)
			if got != tt.want {
				t.Errorf("isSubscriptionExpired(%q) = %v, want %v", tt.expiresAt, got, tt.want)
			}
		})
	}
}

func TestResolveUserConcurrencyProfile_Tiers(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	future := "2026-10-16T12:00:00Z"

	tests := []struct {
		name                  string
		planID                string
		configuredConcurrency int
		expiresAt             string
		wantConcurrency       int
		wantMaxQueued         int
		wantExpired           bool
	}{
		{
			name:                  "Free trial",
			planID:                "plan_free",
			configuredConcurrency: 1,
			expiresAt:             future,
			wantConcurrency:       1,
			wantMaxQueued:         10,
			wantExpired:           false,
		},
		{
			name:                  "Basic month",
			planID:                "plan_month",
			configuredConcurrency: 3,
			expiresAt:             future,
			wantConcurrency:       3,
			wantMaxQueued:         30,
			wantExpired:           false,
		},
		{
			name:                  "Pro month",
			planID:                "plan_pro",
			configuredConcurrency: 8,
			expiresAt:             future,
			wantConcurrency:       8,
			wantMaxQueued:         50,
			wantExpired:           false,
		},
		{
			name:                  "Ultimate year",
			planID:                "plan_year",
			configuredConcurrency: 20,
			expiresAt:             future,
			wantConcurrency:       20,
			wantMaxQueued:         100,
			wantExpired:           false,
		},
		{
			name:                  "Enterprise plan",
			planID:                "plan_enterprise",
			configuredConcurrency: 0,
			expiresAt:             future,
			wantConcurrency:       0,
			wantMaxQueued:         200,
			wantExpired:           false,
		},
		{
			name:                  "Expired Pro falls back to Free",
			planID:                "plan_pro",
			configuredConcurrency: 8,
			expiresAt:             "2026-09-01T00:00:00Z",
			wantConcurrency:       1,
			wantMaxQueued:         10,
			wantExpired:           true,
		},
		{
			name:                  "Missing/Legacy negative concurrency falls back to Free",
			planID:                "plan_legacy",
			configuredConcurrency: -1,
			expiresAt:             "",
			wantConcurrency:       1,
			wantMaxQueued:         10,
			wantExpired:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := resolveUserConcurrencyProfile(tt.planID, tt.configuredConcurrency, tt.expiresAt, now)
			if profile.Concurrency != tt.wantConcurrency {
				t.Errorf("Concurrency = %d, want %d", profile.Concurrency, tt.wantConcurrency)
			}
			if profile.MaxQueued != tt.wantMaxQueued {
				t.Errorf("MaxQueued = %d, want %d", profile.MaxQueued, tt.wantMaxQueued)
			}
			if profile.Expired != tt.wantExpired {
				t.Errorf("Expired = %v, want %v", profile.Expired, tt.wantExpired)
			}
		})
	}
}

func TestMaxQueuedLimitForPlan(t *testing.T) {
	if got := maxQueuedLimitForPlan("plan_enterprise", 0); got != 200 {
		t.Errorf("enterprise backlog = %d, want 200", got)
	}
	if got := maxQueuedLimitForPlan("plan_ultimate_year", 20); got != 100 {
		t.Errorf("ultimate backlog = %d, want 100", got)
	}
	if got := maxQueuedLimitForPlan("plan_pro_single", 8); got != 50 {
		t.Errorf("pro backlog = %d, want 50", got)
	}
	if got := maxQueuedLimitForPlan("plan_basic_year", 3); got != 30 {
		t.Errorf("basic backlog = %d, want 30", got)
	}
	if got := maxQueuedLimitForPlan("plan_free", 1); got != 10 {
		t.Errorf("free backlog = %d, want 10", got)
	}
}

func TestJSONGenerationQueueBacklogLimitAndRunningSeparation(t *testing.T) {
	// User with Free plan (Concurrency: 1, MaxQueued: 10)
	tasks := []generationTask{
		{ID: "queued_1", UserID: "free_user", TaskStatus: taskStatusQueued, Status: "PROCESSING"},
		{ID: "queued_2", UserID: "free_user", TaskStatus: taskStatusQueued, Status: "PROCESSING"},
	}
	// 2 queued tasks should NOT count as actively running
	runningCount := activeRunningGenerationTaskCount(tasks, "free_user")
	if runningCount != 0 {
		t.Fatalf("queued tasks should not count as running, got %d", runningCount)
	}

	// 1 running task should count as running
	tasks = append(tasks, generationTask{ID: "running_1", UserID: "free_user", TaskStatus: taskStatusRunning, Status: "PROCESSING"})
	runningCount = activeRunningGenerationTaskCount(tasks, "free_user")
	if runningCount != 1 {
		t.Fatalf("running task should count as running, got %d", runningCount)
	}
}

