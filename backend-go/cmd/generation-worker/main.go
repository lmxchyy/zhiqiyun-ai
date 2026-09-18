package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
	"xianzhi-ai/backend-go/internal/infra"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := config.Load()
	if !cfg.AsyncMessagingEnabled {
		return fmt.Errorf("ASYNC_MESSAGING_ENABLED must be true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	clients, err := infra.Open(ctx, cfg)
	cancel()
	if err != nil {
		return err
	}
	if clients == nil || clients.DB == nil || clients.Messaging == nil {
		return fmt.Errorf("generation worker requires database and RabbitMQ")
	}
	defer clients.Close()
	clients.Messaging.Start()
	workerCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	errCh := make(chan error, 5)
	listener, err := net.Listen("tcp", cfg.GenerationWorkerMetricsAddr)
	if err != nil {
		return fmt.Errorf("worker metrics listen: %w", err)
	}
	metricsServer := &http.Server{Handler: httpserver.GenerationWorkerMetricsHandler(clients.DB), ReadHeaderTimeout: 5 * time.Second}
	defer metricsServer.Close()
	go func() {
		if err := metricsServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	go func() {
		errCh <- httpserver.RunConfiguredGenerationScheduler(workerCtx, clients.DB, cfg, httpserver.GenerationSchedulerOptions{Owner: "generation-worker-scheduler"})
	}()
	go func() {
		errCh <- httpserver.RunGenerationImageCanaryWorker(workerCtx, cfg, clients.DB, clients.Messaging)
	}()
	go func() {
		errCh <- httpserver.RunGenerationVideoCanaryWorker(workerCtx, cfg, clients.DB, clients.Messaging)
	}()
	go func() {
		errCh <- httpserver.RunGenerationPPTCanaryWorker(workerCtx, cfg, clients.DB, clients.Messaging)
	}()
	err = <-errCh
	if err == context.Canceled || err == context.DeadlineExceeded {
		return nil
	}
	return err
}
