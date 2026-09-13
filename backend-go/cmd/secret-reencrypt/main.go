package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

func run(args []string) int {
	flags := flag.NewFlagSet("secret-reencrypt", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	dryRun := flags.Bool("dry-run", true, "Execute in dry-run mode without modifying the database (default: true)")
	batchSize := flags.Int("batch-size", 50, "Batch size for cursor pagination")
	table := flags.String("table", "all", "Target table to re-encrypt: 'all', 'storage', or 'connectors'")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "[reencrypt-error] DATABASE_URL environment variable is required")
		return 2
	}

	target := strings.ToLower(strings.TrimSpace(*table))
	if target != "all" && target != "storage" && target != "connectors" && target != "xz_storage_configs" && target != "enterprise_connectors" {
		fmt.Fprintf(os.Stderr, "[reencrypt-error] invalid target table %q; must be 'all', 'storage', or 'connectors'\n", *table)
		return 2
	}

	var storageCipher *storagecenter.SecretCipher
	if target == "all" || target == "storage" || target == "xz_storage_configs" {
		storageKeyConfig, err := storagecenter.ResolveKeyringConfig(
			os.Getenv("STORAGE_MASTER_KEY"),
			os.Getenv("STORAGE_MASTER_KEY_ACTIVE"),
			os.Getenv("STORAGE_MASTER_KEY_LEGACY"),
			os.Getenv("STORAGE_MASTER_KEY_RING"),
			nil,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[reencrypt-error] invalid storage keyring configuration: %v\n", err)
			return 2
		}
		storageCipher, err = storagecenter.NewKeyringCipher(storageKeyConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[reencrypt-error] failed to initialize storage cipher: %v\n", err)
			return 2
		}
	}

	var connectorCipher *storagecenter.SecretCipher
	if target == "all" || target == "connectors" || target == "enterprise_connectors" {
		connectorKeyConfig, err := storagecenter.ResolveKeyringConfig(
			os.Getenv("CONNECTOR_SECRET_ENCRYPTION_KEY"),
			os.Getenv("CONNECTOR_SECRET_ACTIVE_KEY_ID"),
			os.Getenv("CONNECTOR_SECRET_LEGACY_KEY"),
			os.Getenv("CONNECTOR_SECRET_KEYRING"),
			nil,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[reencrypt-error] invalid connector keyring configuration: %v\n", err)
			return 2
		}
		connectorCipher, err = storagecenter.NewKeyringCipher(connectorKeyConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[reencrypt-error] failed to initialize connector cipher: %v\n", err)
			return 2
		}
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[reencrypt-error] failed to connect to database: %v\n", err)
		return 2
	}
	defer db.Close()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		fmt.Fprintf(os.Stderr, "[reencrypt-error] database ping failed: %v\n", err)
		return 2
	}

	runner := storagecenter.ReencryptRunner{
		DB:              storagecenter.WrapDB(db),
		StorageCipher:   storageCipher,
		ConnectorCipher: connectorCipher,
		Options: storagecenter.ReencryptOptions{
			DryRun:    *dryRun,
			BatchSize: *batchSize,
			Logger:    os.Stdout,
		},
	}

	results, err := runner.Run(context.Background(), target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[reencrypt-error] execution failed: %v\n", err)
		return 1
	}

	total := storagecenter.ReencryptStats{}
	for _, st := range results {
		total.Add(st)
	}

	fmt.Printf("[reencrypt-summary] total: scanned=%d migrated=%d skipped=%d conflicted=%d failed=%d dry_run=%t\n",
		total.Scanned, total.Migrated, total.Skipped, total.Conflicted, total.Failed, *dryRun)

	if total.Failed > 0 {
		fmt.Fprintf(os.Stderr, "[reencrypt-result] migration completed with %d failure(s); see logs above\n", total.Failed)
		return 1
	}

	return 0
}
