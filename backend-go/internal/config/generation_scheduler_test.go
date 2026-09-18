package config

import "testing"

func TestGenerationFairSchedulerStartupFlag(t *testing.T) {
	t.Setenv("GENERATION_FAIR_SCHEDULER_ENABLED", "")
	if Load().GenerationFairSchedulerEnabled {
		t.Fatal("scheduler must default off")
	}
	t.Setenv("GENERATION_FAIR_SCHEDULER_ENABLED", "true")
	enabled := Load()
	if !enabled.GenerationFairSchedulerEnabled {
		t.Fatal("explicit true not loaded")
	}
	t.Setenv("GENERATION_FAIR_SCHEDULER_ENABLED", "false")
	if Load().GenerationFairSchedulerEnabled {
		t.Fatal("explicit false ignored")
	}
	if !enabled.GenerationFairSchedulerEnabled {
		t.Fatal("startup config must not hot reload")
	}
	if Load().GenerationWorkerMetricsAddr != "127.0.0.1:9091" {
		t.Fatal("unsafe default worker bind")
	}
}
