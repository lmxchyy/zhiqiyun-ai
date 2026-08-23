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
	code := run()
	os.Exit(code)
}

func run() int {
	flags := flag.NewFlagSet("backup-config-clone", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	sourceID := flags.String("source-id", "", "explicit enabled business Huawei OBS config ID")
	targetID := flags.String("target-id", "", "new backup config ID")
	name := flags.String("name", "Temporary PostgreSQL Backup", "new backup config name")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if strings.TrimSpace(*sourceID) == "" || strings.TrimSpace(*targetID) == "" {
		fmt.Fprintln(os.Stderr, "BACKUP_STORAGE_CONFIG_NOT_FOUND")
		return 1
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	masterKey := strings.TrimSpace(os.Getenv("STORAGE_MASTER_KEY"))
	if databaseURL == "" || masterKey == "" {
		fmt.Fprintln(os.Stderr, "BACKUP_CONFIG_CREDENTIALS_UNAVAILABLE")
		return 1
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "BACKUP_CONFIG_CLONE_FAILED")
		return 1
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "BACKUP_CONFIG_CLONE_FAILED")
		return 1
	}
	service := storagecenter.NewService(
		storagecenter.NewPostgresRepository(db),
		storagecenter.S3ProviderFactory{AutoCreateBucket: false},
		storagecenter.Options{MasterKey: masterKey},
	)
	item, err := service.CloneBackupConfig(ctx, *sourceID, *targetID, *name)
	if err != nil {
		if err == storagecenter.ErrConfigAlreadyExists {
			fmt.Fprintln(os.Stderr, "STORAGE_CONFIG_ALREADY_EXISTS")
		} else if err == storagecenter.ErrBackupConfigNotFound {
			fmt.Fprintln(os.Stderr, "BACKUP_STORAGE_CONFIG_NOT_FOUND")
		} else {
			fmt.Fprintln(os.Stderr, "BACKUP_CONFIG_CLONE_FAILED")
		}
		return 1
	}
	fmt.Printf("BACKUP_CONFIG_CREATED id=%s provider=%s purpose=%s bucket=%s region=%s prefix=%s is_default=%t credential_encrypted=true\n", item.ID, item.Provider, item.Purpose, item.Bucket, item.Region, item.ObjectPrefix, item.IsDefault)
	return 0
}
