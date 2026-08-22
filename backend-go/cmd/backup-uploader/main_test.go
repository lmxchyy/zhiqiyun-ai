package main

import (
	"testing"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestBackupObjectKeyUsesOnlyConfiguredBackupPrefix(t *testing.T) {
	key, err := backupObjectKey(storagecenter.Config{ObjectPrefix: storagecenter.BackupObjectPrefix}, "deploy", "2026", "08", "db_test.sql.gz")
	if err != nil {
		t.Fatalf("backupObjectKey returned error: %v", err)
	}
	if key != "backups/postgres/deploy/2026/08/db_test.sql.gz" {
		t.Fatalf("key = %q", key)
	}
}

func TestBackupObjectKeyRejectsPrefixInjection(t *testing.T) {
	if _, err := backupObjectKey(storagecenter.Config{ObjectPrefix: "images/"}, "deploy", "2026", "08", "db_test.sql.gz"); err == nil {
		t.Fatal("business prefix was accepted")
	}
}
