package config

import (
	"testing"
	"time"
)

func TestImageTimeoutsDefaultToLongRunningGenerationBudget(t *testing.T) {
	cfg := Config{}
	if got := cfg.ImageProviderTimeout(); got != 10*time.Minute {
		t.Fatalf("ImageProviderTimeout() = %s, want 10m", got)
	}
	if got := cfg.ImageGenerationTimeout(); got != 12*time.Minute {
		t.Fatalf("ImageGenerationTimeout() = %s, want 12m", got)
	}
}

func TestImageGenerationTimeoutAlwaysExceedsProviderTimeout(t *testing.T) {
	cfg := Config{
		ImageProviderTimeoutMS:   "600000",
		ImageGenerationTimeoutMS: "300000",
	}
	if got := cfg.ImageGenerationTimeout(); got != 12*time.Minute {
		t.Fatalf("ImageGenerationTimeout() = %s, want provider timeout plus 2m", got)
	}
}

func TestImageProviderTimeoutFallsBackToLegacyModelTimeout(t *testing.T) {
	cfg := Config{ModelTimeoutMS: "180000"}
	if got := cfg.ImageProviderTimeout(); got != 3*time.Minute {
		t.Fatalf("ImageProviderTimeout() = %s, want 3m", got)
	}
}
