package operationcenter

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"xianzhi-ai/backend-go/internal/app/payment"
)

func TestRuntimeSchedulerDefaultsRemainClosed(t *testing.T) {
	config := DefaultOperationCenterRuntimeConfig("production")
	if config.RefundRetrySchedulerEnabled || config.RefundVerificationEnabled || config.RewardReleaseSchedulerEnabled {
		t.Fatal("all operation center schedulers must default to disabled")
	}
	if config.ManualRefundAutoApproval {
		t.Fatal("manual refund auto approval must default to disabled")
	}
	runtime := &OperationCenterRuntime{config: config}
	group, err := runtime.StartSchedulers(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("start disabled schedulers: %v", err)
	}
	group.Stop()
}

func TestRuntimeSchedulerRecoversRoundPanic(t *testing.T) {
	err := runRuntimeSchedulerRound(context.Background(), func(context.Context) error {
		panic("isolated scheduler failure")
	})
	if err == nil {
		t.Fatal("scheduler round panic must be surfaced without crashing the process")
	}
}

func TestRuntimeSchedulerRejectsMissingRefundProvider(t *testing.T) {
	config := DefaultOperationCenterRuntimeConfig("production")
	config.RefundRetrySchedulerEnabled = true
	runtime := &OperationCenterRuntime{config: config, providers: map[string]payment.RefundProvider{}}
	_, err := runtime.StartSchedulers(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !errors.Is(err, ErrRefundProviderUnavailable) {
		t.Fatalf("expected missing provider error, got %v", err)
	}
}
