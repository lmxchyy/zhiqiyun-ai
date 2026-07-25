package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/infra"
	"xianzhi-ai/backend-go/internal/smartvideoruntime"
)

func main() {
	cfg := config.Load()
	if !cfg.SmartVideoAnalysisEnabled {
		log.Fatal("smart-video analysis worker is disabled; set SMARTVIDEO_ANALYSIS_ENABLED=true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	clients, err := infra.Open(ctx, cfg)
	cancel()
	if err != nil {
		log.Fatalf("open worker infrastructure: %v", err)
	}
	defer func() {
		if err := clients.Close(); err != nil {
			log.Printf("close worker infrastructure: %v", err)
		}
	}()
	runtime, err := smartvideoruntime.New(cfg, clients.DB, clients.Redis)
	if err != nil {
		if errors.Is(err, smartvideo.ErrAnalysisDisabled) {
			log.Fatal("smart-video analysis worker is disabled")
		}
		log.Fatalf("initialize smart-video worker: %v", err)
	}
	workerCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runtime.Run(workerCtx); err != nil {
		log.Fatalf("smart-video worker stopped: %v", err)
	}
}
