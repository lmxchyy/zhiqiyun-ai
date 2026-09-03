package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/canarypreflight"
	"xianzhi-ai/backend-go/internal/config"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var db *sql.DB
	if cfg.DatabaseURL != "" {
		var err error
		db, err = sql.Open("pgx", cfg.DatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid DATABASE_URL: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()
	}
	result := canarypreflight.Run(ctx, cfg, db)
	for _, line := range result.Lines() {
		fmt.Println(line)
	}
	if result.Ready != "PASS" {
		os.Exit(1)
	}
}
