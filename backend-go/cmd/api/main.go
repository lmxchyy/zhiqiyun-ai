package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
	"xianzhi-ai/backend-go/internal/infra"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.ValidateProduction(); err != nil {
		return fmt.Errorf("invalid production config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	clients, err := infra.Open(ctx, cfg)
	cancel()
	if err != nil {
		if cfg.IsProduction() {
			return fmt.Errorf("production infrastructure unavailable: %w", err)
		}
		log.Printf("infrastructure clients disabled: %v", err)
	} else {
		defer func() {
			if err := clients.Close(); err != nil {
				log.Printf("close infrastructure clients: %v", err)
			}
		}()
	}
	if cfg.IsProduction() && (clients == nil || clients.DB == nil || clients.Redis == nil) {
		return fmt.Errorf("production requires PostgreSQL and Redis infrastructure")
	}
	var server = httpserver.New(cfg)
	stopWorker := func() {}
	if clients != nil {
		server = httpserver.NewWithInfrastructure(cfg, clients.DB, clients.Redis)
		workerCtx, cancelWorker := context.WithCancel(context.Background())
		stopWorker = cancelWorker
		httpserver.StartIdentityDowngradeWorker(workerCtx, clients.DB, time.Minute)
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	log.Printf("xianzhi-ai go gin api listening on %s", cfg.Addr)
	return serveAPIUntilShutdown(server, signals, cfg.APIShutdownTimeout(), stopWorker, log.Default())
}
