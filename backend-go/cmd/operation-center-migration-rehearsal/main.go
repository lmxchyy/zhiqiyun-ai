package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	var (
		databaseURL       string
		migrationDir      string
		environment       string
		acknowledgement   string
		backupReference   string
		outputPath        string
		availableDiskByte int64
		apply             bool
	)

	flag.StringVar(&databaseURL, "database-url", strings.TrimSpace(os.Getenv("XIANZHI_REHEARSAL_DATABASE_URL")), "non-production rehearsal database URL")
	flag.StringVar(&migrationDir, "migration-dir", envOrDefault("XIANZHI_REHEARSAL_MIGRATION_DIR", filepath.Clean(filepath.Join("..", "database", "migrations"))), "migration directory")
	flag.StringVar(&environment, "environment", strings.TrimSpace(os.Getenv("XIANZHI_REHEARSAL_ENVIRONMENT")), "rehearsal environment label")
	flag.StringVar(&acknowledgement, "ack", strings.TrimSpace(os.Getenv("XIANZHI_REHEARSAL_ACK")), "must equal NON_PRODUCTION_COPY")
	flag.StringVar(&backupReference, "backup-reference", strings.TrimSpace(os.Getenv("XIANZHI_REHEARSAL_BACKUP_REFERENCE")), "verified backup or sanitized-copy reference")
	flag.StringVar(&outputPath, "output", strings.TrimSpace(os.Getenv("XIANZHI_REHEARSAL_REPORT_PATH")), "optional JSON report path")
	flag.Int64Var(&availableDiskByte, "available-disk-bytes", envInt64("XIANZHI_REHEARSAL_AVAILABLE_DISK_BYTES"), "available disk bytes for the rehearsal database")
	flag.BoolVar(&apply, "apply", false, "apply migrations 089 through 096 to the non-production copy")
	flag.Parse()

	if databaseURL == "" {
		return errors.New("XIANZHI_REHEARSAL_DATABASE_URL or --database-url is required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open rehearsal database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	report, rehearsalErr := operationcenter.RunOperationCenterMigrationRehearsal(ctx, db, operationcenter.MigrationRehearsalOptions{
		Environment:        environment,
		Acknowledgement:    acknowledgement,
		BackupReference:    backupReference,
		MigrationDirectory: migrationDir,
		AvailableDiskBytes: availableDiskByte,
		Apply:              apply,
	})

	payload, marshalErr := json.MarshalIndent(report, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("encode rehearsal report: %w", marshalErr)
	}
	if outputPath != "" {
		if err := os.WriteFile(outputPath, append(payload, '\n'), 0o600); err != nil {
			return fmt.Errorf("write rehearsal report: %w", err)
		}
	} else {
		fmt.Println(string(payload))
	}
	return rehearsalErr
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
