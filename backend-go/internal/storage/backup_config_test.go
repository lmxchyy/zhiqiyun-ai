package storage

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestCloneBackupConfigReencryptsSelectedBusinessCredential(t *testing.T) {
	service, repo, _, _ := testService(100)
	ctx := context.Background()
	business, err := service.SaveConfig(ctx, Config{
		ID: "business-default", TenantID: PlatformTenantID, Name: "Business OBS", Provider: "huawei_obs",
		Endpoint: "https://obs.example", Region: "cn-north-9", Bucket: "zhiqiyun-private", IsDefault: true,
		Status: "ENABLED", CreatedBy: "admin", UpdatedBy: "admin",
	}, "business-access", "business-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateConfigTest(ctx, business.ID, "SUCCESS", "ok", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	backup, err := service.CloneBackupConfig(ctx, business.ID, "backup-config", "Temporary PostgreSQL Backup")
	if err != nil {
		t.Fatal(err)
	}
	if backup.Purpose != "backup" || backup.ObjectPrefix != BackupObjectPrefix || backup.IsDefault || backup.Provider != "huawei_obs" {
		t.Fatalf("unexpected backup config: %+v", backup)
	}
	raw, err := repo.GetConfig(ctx, backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw.AccessKeyEncrypted == "" || raw.SecretKeyEncrypted == "" || strings.Contains(raw.AccessKeyEncrypted, "business-access") || strings.Contains(raw.SecretKeyEncrypted, "business-secret") {
		t.Fatal("backup credential was not kept encrypted")
	}
	if raw.AccessKeyEncrypted == business.AccessKeyEncrypted || raw.SecretKeyEncrypted == business.SecretKeyEncrypted {
		t.Fatal("credential ciphertext must be re-encrypted under the backup config ID")
	}
	current, err := repo.GetConfig(ctx, business.ID)
	if err != nil || !current.IsDefault {
		t.Fatalf("business default changed: %+v err=%v", current, err)
	}
}
