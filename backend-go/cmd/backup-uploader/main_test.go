package main

import (
	"testing"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestVerifyRemoteMetadataRequiresExactSizeAndSHA256(t *testing.T) {
	got := verifyRemoteMetadata(storagecenter.ObjectMetadata{
		Size:     42,
		Metadata: map[string]string{"x-obs-meta-sha256": "abc123"},
	}, 42, "abc123")
	if got.Status != "REMOTE_VERIFIED" || !got.Exists || !got.SizeMatch || !got.SHA256Match {
		t.Fatalf("verification = %#v", got)
	}
}

func TestVerifyRemoteMetadataFailsClosedForMissingMetadata(t *testing.T) {
	got := verifyRemoteMetadata(storagecenter.ObjectMetadata{Size: 42}, 42, "abc123")
	if got.Status == "REMOTE_VERIFIED" || got.SHA256Match {
		t.Fatalf("missing sha metadata was accepted: %#v", got)
	}
}

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
