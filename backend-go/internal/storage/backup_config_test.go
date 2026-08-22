package storage

import (
	"context"
	"strings"
	"testing"
)

func TestValidateBackupConfigRequiresDedicatedHuaweiPrefix(t *testing.T) {
	base := Config{
		ID:           "storage_backup",
		Provider:     "huawei_obs",
		Bucket:       "zhiqiyun-private",
		Purpose:      "backup",
		ObjectPrefix: BackupObjectPrefix,
		Status:       "ENABLED",
	}

	if err := ValidateBackupConfig(base); err != nil {
		t.Fatalf("valid backup config rejected: %v", err)
	}

	cases := []Config{
		base,
		{ID: base.ID, Provider: "huawei_obs", Bucket: base.Bucket, Purpose: "business", ObjectPrefix: base.ObjectPrefix, Status: base.Status},
		{ID: base.ID, Provider: "huawei_obs", Bucket: base.Bucket, Purpose: base.Purpose, ObjectPrefix: "images/", Status: base.Status},
		{ID: base.ID, Provider: "huawei_obs", Bucket: base.Bucket, Purpose: base.Purpose, ObjectPrefix: base.ObjectPrefix, IsDefault: true, Status: base.Status},
		{ID: base.ID, Provider: "minio", Bucket: base.Bucket, Purpose: base.Purpose, ObjectPrefix: base.ObjectPrefix, Status: base.Status},
	}
	wants := []string{"", "backup purpose", "backup prefix", "default", "provider"}
	for i, item := range cases[1:] {
		err := ValidateBackupConfig(item)
		if err == nil || !strings.Contains(err.Error(), wants[i+1]) {
			t.Fatalf("case %d error = %v, want %q", i+1, err, wants[i+1])
		}
	}
}

func TestBackupConfigByIDDoesNotFallbackToBusinessOrEnvironmentConfig(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewService(repo, &fakeProviderFactory{}, Options{MasterKey: "backup-config-test-master-key-32-bytes"})

	_, err := service.BackupConfigByID(context.Background(), "missing-backup-config")
	if err == nil || err != ErrBackupConfigNotFound {
		t.Fatalf("BackupConfigByID error = %v, want ErrBackupConfigNotFound", err)
	}
	if _, err := repo.SaveConfig(context.Background(), Config{ID: "business-default", Provider: "huawei_obs", Purpose: "", ObjectPrefix: "", IsDefault: true, Status: "ENABLED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BackupConfigByID(context.Background(), "business-default"); err != ErrBackupConfigNotFound {
		t.Fatalf("business config error = %v, want ErrBackupConfigNotFound", err)
	}
}
