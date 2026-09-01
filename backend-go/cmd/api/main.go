package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
	"xianzhi-ai/backend-go/internal/infra"
	"xianzhi-ai/backend-go/internal/messaging"
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
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	var db *sql.DB
	var redisClient *redis.Client
	var messagingManager *messaging.ConnectionManager
	if clients != nil {
		db, redisClient, messagingManager = clients.DB, clients.Redis, clients.Messaging
	}
	runtime := newAsyncMessagingRuntime(cfg, db, messagingManager)
	runtime.Start(workerCtx)
	var server = httpserver.NewWithInfrastructureAndReadyStatus(cfg, db, redisClient, runtime.Status)
	if clients != nil {
		httpserver.StartIdentityDowngradeWorker(workerCtx, clients.DB, time.Minute)
	}
	stopWorker := func() {
		cancelWorkers()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.APIShutdownTimeout())
		defer cancel()
		if err := runtime.Stop(shutdownCtx); err != nil {
			log.Printf("stop async messaging runtime: %v", err)
		}
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	log.Printf("xianzhi-ai go gin api listening on %s", cfg.Addr)
	return serveAPIUntilShutdown(server, signals, cfg.APIShutdownTimeout(), stopWorker, log.Default())
}
