package main

import (
	"context"
	"log"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
	"xianzhi-ai/backend-go/internal/infra"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	clients, err := infra.Open(ctx, cfg)
	cancel()
	if err != nil {
		log.Printf("infrastructure clients disabled: %v", err)
	} else {
		defer func() {
			if err := clients.Close(); err != nil {
				log.Printf("close infrastructure clients: %v", err)
			}
		}()
	}
	var server = httpserver.New(cfg)
	if clients != nil {
		server = httpserver.NewWithInfrastructure(cfg, clients.DB, clients.Redis)
	}
	log.Printf("xianzhi-ai go gin api listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
