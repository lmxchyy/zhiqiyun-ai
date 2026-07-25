package storage

import (
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func OptionsFromConfig(cfg config.Config) Options {
	return Options{
		Environment: cfg.Environment, DefaultProvider: firstNonEmpty(cfg.StorageDefaultProvider, "minio"),
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.StoragePublicEndpoint, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket,
		PublicDomain: cfg.StoragePublicDomain, CDNDomain: cfg.StorageCDNDomain, MasterKey: cfg.StorageMasterKey,
		DefaultQuotaBytes: configInt64(cfg.StorageDefaultQuotaBytes, 10<<30),
		MaxUploadBytes:    configInt64(cfg.StorageMaxUploadBytes, 2<<30),
		UploadURLTTL:      time.Duration(configInt64(cfg.StorageUploadURLTTLSeconds, 600)) * time.Second,
		AccessURLTTL:      time.Duration(configInt64(cfg.StorageAccessURLTTLSeconds, 900)) * time.Second,
		RecycleRetention:  time.Duration(configInt64(cfg.StorageRecycleDays, 30)) * 24 * time.Hour,
		AutoCreateBucket:  cfg.StorageAutoCreateBucket, ForcePathStyle: cfg.StorageForcePathStyle,
	}
}

func configInt64(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
