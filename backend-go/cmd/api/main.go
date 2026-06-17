package main

import (
	"log"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/httpserver"
)

func main() {
	cfg := config.Load()
	server := httpserver.New(cfg)
	log.Printf("xianzhi-ai go api listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
