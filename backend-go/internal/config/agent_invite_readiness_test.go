package config

import (
	"strings"
	"testing"
)

func TestAgentInviteProductionReadinessDefaultsAreClosed(t *testing.T) {
	t.Setenv("XIANZHI_ENV", "production")
	t.Setenv("AGENT_INVITE_REGISTRATION_ENABLED", "")
	t.Setenv("APK_DOWNLOAD_ENABLED", "")
	t.Setenv("APP_ACTIVATION_REPORT_ENABLED", "")
	t.Setenv("SMS_REDIS_NAMESPACE", "")
	t.Setenv("SMS_MOBILE_DAILY_LIMIT", "")
	t.Setenv("SMS_DEVICE_DAILY_LIMIT", "")
	t.Setenv("SMS_IP_DAILY_LIMIT", "")

	cfg := Load()

	if cfg.AgentInviteRegistrationEnabled || cfg.APKDownloadEnabled || cfg.AppActivationReportEnabled {
		t.Fatal("production readiness switches must default to disabled")
	}
	if cfg.SMSRedisNamespace != "zhiqiyun:production:sms" {
		t.Fatalf("SMSRedisNamespace=%q", cfg.SMSRedisNamespace)
	}
	mobile, device, ip := cfg.SMSDailyLimits()
	if mobile != 10 || device != 20 || ip != 50 {
		t.Fatalf("daily limits mobile=%d device=%d ip=%d", mobile, device, ip)
	}
}

func TestInviteRegistrationRequiresRealSMSBoundaryInProduction(t *testing.T) {
	cfg := Config{
		Environment:                    "production",
		AgentInviteRegistrationEnabled: true,
		DatabaseURL:                    "configured",
		RedisURL:                       "configured",
		RabbitMQURL:                    "configured",
		S3Endpoint:                     "configured",
		StoragePublicEndpoint:          "configured",
		S3AccessKey:                    "configured",
		S3SecretKey:                    "configured",
		S3Bucket:                       "configured",
		StorageMasterKey:               "configured",
		PaymentCallbackSecret:          "configured",
	}

	err := cfg.ValidateProduction()

	if err == nil || !strings.Contains(err.Error(), "SMS_") {
		t.Fatalf("expected SMS configuration validation error, got %v", err)
	}
}
