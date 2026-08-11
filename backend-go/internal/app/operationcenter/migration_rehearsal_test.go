package operationcenter

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationRehearsalRefusesProduction(t *testing.T) {
	t.Parallel()

	err := ValidateMigrationRehearsalOptions(MigrationRehearsalOptions{
		Environment:        "production",
		Acknowledgement:    MigrationRehearsalAcknowledgement,
		BackupReference:    "backup-verified",
		MigrationDirectory: t.TempDir(),
		AvailableDiskBytes: 1 << 40,
		Apply:              true,
	})
	if err == nil {
		t.Fatal("production database rehearsal unexpectedly accepted")
	}
}

func TestMigrationRehearsalRequiresExplicitApplyAcknowledgement(t *testing.T) {
	t.Parallel()

	err := ValidateMigrationRehearsalOptions(MigrationRehearsalOptions{
		Environment:        "staging-copy",
		Acknowledgement:    "",
		BackupReference:    "backup-verified",
		MigrationDirectory: t.TempDir(),
		AvailableDiskBytes: 1 << 40,
		Apply:              true,
	})
	if err == nil {
		t.Fatal("missing non-production acknowledgement unexpectedly accepted")
	}
}

func TestOperationCenterMigrationFilesRequireExactlyOneFilePerVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for version := 89; version <= 96; version++ {
		path := filepath.Join(dir, migrationFixtureName(version, "first"))
		if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := operationCenterMigrationFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("migration count = %d, want 8", len(files))
	}

	if err := os.WriteFile(filepath.Join(dir, migrationFixtureName(89, "second")), []byte("SELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := operationCenterMigrationFiles(dir); err == nil {
		t.Fatal("duplicate migration version unexpectedly accepted")
	}
}

func migrationFixtureName(version int, suffix string) string {
	return fmt.Sprintf("%03d-%s.sql", version, suffix)
}
