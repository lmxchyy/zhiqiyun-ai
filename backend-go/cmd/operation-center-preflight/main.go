package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"xianzhi-ai/backend-go/internal/app/operationcenter"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	config, err := operationcenter.LoadProductionReleaseGateConfig(os.LookupEnv, os.Environ())
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	report, gateErr := operationcenter.RunProductionReleaseGate(ctx, db, config)
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	return gateErr
}
