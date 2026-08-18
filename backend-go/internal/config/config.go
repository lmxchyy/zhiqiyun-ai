package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment                    string
	Addr                           string
	DataPath                       string
	StaticDir                      string
	AdminStaticDir                 string
	UserH5StaticDir                string
	DatabaseURL                    string
	RedisURL                       string
	SMSProviderName                string
	SMSProviderURL                 string
	SMSProviderAPIKey              string
	SMSTemplateID                  string
	SMSSignature                   string
	SMSRedisNamespace              string
	SMSMobileDailyLimit            string
	SMSDeviceDailyLimit            string
	SMSIPDailyLimit                string
	AgentInviteRegistrationEnabled bool
	APKDownloadEnabled             bool
	AppActivationReportEnabled     bool
	RabbitMQURL                    string
	FeishuHTTPTimeoutSeconds       string
	FeishuTokenCachePrefix         string
	FeishuAPIBaseURL               string
	FeishuAccountsBaseURL          string
	ConnectorCallbackBaseURL       string
	ConnectorSecretEncryptionKey   string
	ConnectorQueuePrefix           string
	S3Endpoint                     string
	StoragePublicEndpoint          string
	S3Region                       string
	S3AccessKey                    string
	S3SecretKey                    string
	S3Bucket                       string
	StorageMasterKey               string
	StorageDefaultProvider         string
	StorageDefaultQuotaBytes       string
	StorageMaxUploadBytes          string
	StorageUploadURLTTLSeconds     string
	StorageAccessURLTTLSeconds     string
	StorageRecycleDays             string
	StorageAutoCreateBucket        bool
	StorageForcePathStyle          bool
	StoragePublicDomain            string
	StorageCDNDomain               string
	PaymentCallbackSecret          string
	InspirationDraftHMACSecret     string
	WeChatMiniProgramAppID         string
	WeChatMiniProgramSecret        string
	WeChatOpenAppID                string
	WeChatOpenAppSecret            string
	WeChatOpenAPIBaseURL           string
	WeChatOpenAuthorizeBaseURL     string
	WeChatOpenRedirectURL          string
	WeChatPayAPIv3Key              string
	WeChatPayPlatformKey           string
	WeChatPayPlatformPath          string
	AlipayPublicKey                string
	AlipayPublicKeyPath            string
	AndroidPaymentCapability       string
	AndroidPaymentChannel          string
	WeChatVirtualPayEnabled        bool
	WeChatVirtualPayEnv            string
	WeChatVirtualPayOfferID        string
	WeChatVirtualPayAppKey         string
	WeChatVirtualPaySandboxKey     string
	WeChatVirtualPayNotifyToken    string
	WeChatVirtualPayMode           string
	PricePlanCreationEnabled       bool
	PricePlanTestEntryEnabled      bool
	SnapshotV2FulfillmentEnabled   bool
	ModelProviderURL               string
	ModelProviderAPIKey            string
	ImageModel                     string
	TextModel                      string
	PPTProviderURL                 string
	PPTProviderAPIKey              string
	PPTTextModel                   string
	PPTDisableThinking             bool
	PPTVisualPlannerMode           string
	PPTAutoImageMode               string
	PPTVisualOCRFailureMode        string
	ModelTimeoutMS                 string
	ImageProviderTimeoutMS         string
	ImageGenerationTimeoutMS       string
	ModelProvidersJSON             string
	CORSAllowedOrigins             string
	KnowledgeOCREndpoint           string
	KnowledgeOCRAPIKey             string
	KnowledgeOCRProvider           string
	MediaStorageProvider           string
	MediaStorageRoot               string
	MediaPublicBaseURL             string
	MediaCDNBaseURL                string
	MediaMaxUploadBytes            string
	MediaKeepOriginal              bool
	SmartVideoAnalysisEnabled      bool
	SmartVideoFFprobePath          string
	SmartVideoFFmpegPath           string
	SmartVideoProbeTimeout         string
	SmartVideoProcessTimeout       string
	SmartVideoMaxFileBytes         string
	SmartVideoMaxVideoDuration     string
	SmartVideoMaxVideoPixels       string
	SmartVideoMaxImagePixels       string
	SmartVideoProxyMaxWidth        string
	SmartVideoProxyVideoBitrate    string
	SmartVideoProxyAudioBitrate    string
	SmartVideoAnalysisMaxAttempts  string
	SmartVideoWorkerConcurrency    string
	SmartVideoPlanWorkerConcurrency string
	SmartVideoRenderWorkerConcurrency string
	SmartVideoOutboxEnabled        bool
	SmartVideoTempDir              string
	ShutdownTimeout                string
}

func Load() Config {
	addr := os.Getenv("XIANZHI_GO_ADDR")
	if addr == "" {
		addr = os.Getenv("PORT")
	}
	if addr == "" {
		addr = "3100"
	}
	if addr[0] != ':' {
		addr = ":" + addr
	}
	dataPath := os.Getenv("XIANZHI_DATA_PATH")
	if dataPath == "" {
		dataPath = "data/store.json"
	}
	staticDir := os.Getenv("XIANZHI_STATIC_DIR")
	if staticDir == "" {
		staticDir = "admin-vue/dist"
	}
	adminStaticDir := os.Getenv("XIANZHI_ADMIN_STATIC_DIR")
	if adminStaticDir == "" {
		adminStaticDir = "admin-vue/dist"
	}
	userH5StaticDir := os.Getenv("XIANZHI_USER_H5_STATIC_DIR")
	if userH5StaticDir == "" {
		userH5StaticDir = "apps/user-uni/dist/build/h5"
	}
	modelProviderURL := os.Getenv("MODEL_PROVIDER_URL")
	if modelProviderURL == "" {
		modelProviderURL = os.Getenv("OPENAI_BASE_URL")
	}
	modelProviderAPIKey := os.Getenv("MODEL_PROVIDER_API_KEY")
	if modelProviderAPIKey == "" {
		modelProviderAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	imageModel := os.Getenv("MODEL_PROVIDER_IMAGE_MODEL")
	if imageModel == "" {
		imageModel = "gpt-image-2"
	}
	textModel := os.Getenv("MODEL_PROVIDER_TEXT_MODEL")
	if textModel == "" {
		textModel = os.Getenv("OPENAI_MODEL")
	}
	if textModel == "" {
		textModel = "gpt-4o-mini"
	}
	pptProviderURL := os.Getenv("PPT_MODEL_PROVIDER_URL")
	if pptProviderURL == "" {
		pptProviderURL = modelProviderURL
	}
	pptProviderAPIKey := os.Getenv("PPT_MODEL_PROVIDER_API_KEY")
	if pptProviderAPIKey == "" {
		pptProviderAPIKey = modelProviderAPIKey
	}
	pptTextModel := os.Getenv("PPT_TEXT_MODEL")
	if pptTextModel == "" {
		pptTextModel = textModel
	}
	modelProvidersJSON := os.Getenv("MODEL_PROVIDERS_JSON")
	modelTimeoutMS := os.Getenv("MODEL_PROVIDER_TIMEOUT_MS")
	if modelTimeoutMS == "" {
		modelTimeoutMS = "30000"
	}
	imageProviderTimeoutMS := os.Getenv("IMAGE_PROVIDER_TIMEOUT_MS")
	if imageProviderTimeoutMS == "" {
		imageProviderTimeoutMS = "600000"
	}
	imageGenerationTimeoutMS := os.Getenv("IMAGE_GENERATION_TIMEOUT_MS")
	if imageGenerationTimeoutMS == "" {
		imageGenerationTimeoutMS = "720000"
	}
	environment := strings.TrimSpace(os.Getenv("XIANZHI_ENV"))
	smsRedisNamespace := strings.TrimSpace(os.Getenv("SMS_REDIS_NAMESPACE"))
	if smsRedisNamespace == "" {
		namespaceEnvironment := strings.ToLower(environment)
		if namespaceEnvironment == "" {
			namespaceEnvironment = "development"
		}
		smsRedisNamespace = "zhiqiyun:" + namespaceEnvironment + ":sms"
	}
	return Config{
		Environment:                    environment,
		Addr:                           addr,
		DataPath:                       dataPath,
		StaticDir:                      staticDir,
		AdminStaticDir:                 adminStaticDir,
		UserH5StaticDir:                userH5StaticDir,
		DatabaseURL:                    os.Getenv("DATABASE_URL"),
		RedisURL:                       os.Getenv("REDIS_URL"),
		SMSProviderName:                os.Getenv("SMS_PROVIDER_NAME"),
		SMSProviderURL:                 os.Getenv("SMS_PROVIDER_URL"),
		SMSProviderAPIKey:              os.Getenv("SMS_PROVIDER_API_KEY"),
		SMSTemplateID:                  os.Getenv("SMS_TEMPLATE_ID"),
		SMSSignature:                   os.Getenv("SMS_SIGNATURE"),
		SMSRedisNamespace:              smsRedisNamespace,
		SMSMobileDailyLimit:            stringEnvOrDefault("SMS_MOBILE_DAILY_LIMIT", "10"),
		SMSDeviceDailyLimit:            stringEnvOrDefault("SMS_DEVICE_DAILY_LIMIT", "20"),
		SMSIPDailyLimit:                stringEnvOrDefault("SMS_IP_DAILY_LIMIT", "50"),
		AgentInviteRegistrationEnabled: boolEnv(os.Getenv("AGENT_INVITE_REGISTRATION_ENABLED")),
		APKDownloadEnabled:             boolEnv(os.Getenv("APK_DOWNLOAD_ENABLED")),
		AppActivationReportEnabled:     boolEnv(os.Getenv("APP_ACTIVATION_REPORT_ENABLED")),
		RabbitMQURL:                    os.Getenv("RABBITMQ_URL"),
		FeishuHTTPTimeoutSeconds:       os.Getenv("FEISHU_HTTP_TIMEOUT_SECONDS"),
		FeishuTokenCachePrefix:         os.Getenv("FEISHU_TOKEN_CACHE_PREFIX"),
		FeishuAPIBaseURL:               os.Getenv("FEISHU_API_BASE_URL"),
		FeishuAccountsBaseURL:          os.Getenv("FEISHU_ACCOUNTS_BASE_URL"),
		ConnectorCallbackBaseURL:       os.Getenv("CONNECTOR_CALLBACK_BASE_URL"),
		ConnectorSecretEncryptionKey:   os.Getenv("CONNECTOR_SECRET_ENCRYPTION_KEY"),
		ConnectorQueuePrefix:           os.Getenv("CONNECTOR_QUEUE_PREFIX"),
		S3Endpoint:                     os.Getenv("S3_ENDPOINT"),
		StoragePublicEndpoint:          os.Getenv("STORAGE_PUBLIC_ENDPOINT"),
		S3Region:                       os.Getenv("S3_REGION"),
		S3AccessKey:                    os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:                    os.Getenv("S3_SECRET_KEY"),
		S3Bucket:                       os.Getenv("S3_BUCKET"),
		StorageMasterKey:               os.Getenv("STORAGE_MASTER_KEY"),
		StorageDefaultProvider:         firstNonEmptyEnv("STORAGE_DEFAULT_PROVIDER", "MEDIA_STORAGE_PROVIDER"),
		StorageDefaultQuotaBytes:       os.Getenv("STORAGE_DEFAULT_QUOTA_BYTES"),
		StorageMaxUploadBytes:          os.Getenv("STORAGE_MAX_UPLOAD_BYTES"),
		StorageUploadURLTTLSeconds:     os.Getenv("STORAGE_UPLOAD_URL_TTL_SECONDS"),
		StorageAccessURLTTLSeconds:     os.Getenv("STORAGE_ACCESS_URL_TTL_SECONDS"),
		StorageRecycleDays:             os.Getenv("STORAGE_RECYCLE_DAYS"),
		StorageAutoCreateBucket:        boolEnv(firstNonEmptyEnv("STORAGE_AUTO_CREATE_BUCKET")),
		StorageForcePathStyle:          boolEnv(firstNonEmptyEnv("STORAGE_FORCE_PATH_STYLE")),
		StoragePublicDomain:            os.Getenv("STORAGE_PUBLIC_DOMAIN"),
		StorageCDNDomain:               os.Getenv("STORAGE_CDN_DOMAIN"),
		PaymentCallbackSecret:          os.Getenv("PAYMENT_CALLBACK_SECRET"),
		InspirationDraftHMACSecret:     os.Getenv("INSPIRATION_DRAFT_HMAC_SECRET"),
		WeChatMiniProgramAppID:         os.Getenv("WECHAT_MINI_PROGRAM_APPID"),
		WeChatMiniProgramSecret:        os.Getenv("WECHAT_MINI_PROGRAM_SECRET"),
		WeChatOpenAppID:                os.Getenv("WECHAT_OPEN_APP_ID"),
		WeChatOpenAppSecret:            os.Getenv("WECHAT_OPEN_APP_SECRET"),
		WeChatOpenAPIBaseURL:           os.Getenv("WECHAT_OPEN_API_BASE_URL"),
		WeChatOpenAuthorizeBaseURL:     os.Getenv("WECHAT_OPEN_AUTHORIZE_BASE_URL"),
		WeChatOpenRedirectURL:          os.Getenv("WECHAT_OPEN_REDIRECT_URL"),
		WeChatPayAPIv3Key:              firstNonEmptyEnv("WECHAT_PAY_API_V3_KEY", "WECHAT_PAY_CALLBACK_SECRET"),
		WeChatPayPlatformKey:           os.Getenv("WECHAT_PAY_PLATFORM_PUBLIC_KEY_PEM"),
		WeChatPayPlatformPath:          firstNonEmptyEnv("WECHAT_PAY_PLATFORM_CERT_PATH", "WECHAT_PAY_PLATFORM_PUBLIC_KEY_PATH"),
		AlipayPublicKey:                firstNonEmptyEnv("ALIPAY_PUBLIC_KEY_PEM", "ALIPAY_CALLBACK_SECRET"),
		AlipayPublicKeyPath:            os.Getenv("ALIPAY_PUBLIC_KEY_PATH"),
		AndroidPaymentCapability:       firstNonEmptyEnv("ANDROID_PAYMENT_CAPABILITY"),
		AndroidPaymentChannel:          firstNonEmptyEnv("ANDROID_PAYMENT_CHANNEL"),
		WeChatVirtualPayEnabled:        boolEnv(os.Getenv("WECHAT_VIRTUAL_PAY_ENABLED")),
		WeChatVirtualPayEnv:            os.Getenv("WECHAT_VIRTUAL_PAY_ENV"),
		WeChatVirtualPayOfferID:        os.Getenv("WECHAT_VIRTUAL_PAY_OFFER_ID"),
		WeChatVirtualPayAppKey:         os.Getenv("WECHAT_VIRTUAL_PAY_APP_KEY"),
		WeChatVirtualPaySandboxKey:     os.Getenv("WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY"),
		WeChatVirtualPayNotifyToken:    os.Getenv("WECHAT_VIRTUAL_PAY_NOTIFY_TOKEN"),
		WeChatVirtualPayMode:           os.Getenv("WECHAT_VIRTUAL_PAY_MODE"),
		PricePlanCreationEnabled:       boolEnv(os.Getenv("PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED")),
		PricePlanTestEntryEnabled:      boolEnv(os.Getenv("PRICE_PLAN_TEST_ENTRY_ENABLED")),
		SnapshotV2FulfillmentEnabled:   boolEnv(os.Getenv("SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED")),
		ModelProviderURL:               modelProviderURL,
		ModelProviderAPIKey:            modelProviderAPIKey,
		ImageModel:                     imageModel,
		TextModel:                      textModel,
		PPTProviderURL:                 pptProviderURL,
		PPTProviderAPIKey:              pptProviderAPIKey,
		PPTTextModel:                   pptTextModel,
		PPTDisableThinking:             boolEnv(os.Getenv("PPT_MODEL_CHAT_DISABLE_THINKING")),
		PPTVisualPlannerMode:           firstNonEmptyEnv("PPT_VISUAL_PLANNER_MODE"),
		PPTAutoImageMode:               firstNonEmptyEnv("PPT_AUTO_IMAGE_MODE"),
		PPTVisualOCRFailureMode:        firstNonEmptyEnv("PPT_VISUAL_OCR_FAILURE_MODE"),
		ModelTimeoutMS:                 modelTimeoutMS,
		ImageProviderTimeoutMS:         imageProviderTimeoutMS,
		ImageGenerationTimeoutMS:       imageGenerationTimeoutMS,
		ModelProvidersJSON:             modelProvidersJSON,
		CORSAllowedOrigins:             os.Getenv("XIANZHI_CORS_ALLOWED_ORIGINS"),
		KnowledgeOCREndpoint:           os.Getenv("KNOWLEDGE_OCR_ENDPOINT"),
		KnowledgeOCRAPIKey:             os.Getenv("KNOWLEDGE_OCR_API_KEY"),
		KnowledgeOCRProvider:           firstNonEmptyEnv("KNOWLEDGE_OCR_PROVIDER", "OCR_PROVIDER"),
		MediaStorageProvider:           firstNonEmptyEnv("MEDIA_STORAGE_PROVIDER", "STORAGE_PROVIDER"),
		MediaStorageRoot:               os.Getenv("MEDIA_STORAGE_ROOT"),
		MediaPublicBaseURL:             os.Getenv("MEDIA_PUBLIC_BASE_URL"),
		MediaCDNBaseURL:                os.Getenv("MEDIA_CDN_BASE_URL"),
		MediaMaxUploadBytes:            firstNonEmptyEnv("MEDIA_MAX_UPLOAD_BYTES"),
		MediaKeepOriginal:              boolEnv(firstNonEmptyEnv("MEDIA_KEEP_ORIGINAL")),
		SmartVideoAnalysisEnabled:      boolEnv(os.Getenv("SMARTVIDEO_ANALYSIS_ENABLED")),
		SmartVideoFFprobePath:          os.Getenv("SMARTVIDEO_FFPROBE_PATH"),
		SmartVideoFFmpegPath:           os.Getenv("SMARTVIDEO_FFMPEG_PATH"),
		SmartVideoProbeTimeout:         os.Getenv("SMARTVIDEO_PROBE_TIMEOUT"),
		SmartVideoProcessTimeout:       os.Getenv("SMARTVIDEO_PROCESS_TIMEOUT"),
		SmartVideoMaxFileBytes:         os.Getenv("SMARTVIDEO_MAX_FILE_BYTES"),
		SmartVideoMaxVideoDuration:     os.Getenv("SMARTVIDEO_MAX_VIDEO_DURATION"),
		SmartVideoMaxVideoPixels:       os.Getenv("SMARTVIDEO_MAX_VIDEO_PIXELS"),
		SmartVideoMaxImagePixels:       os.Getenv("SMARTVIDEO_MAX_IMAGE_PIXELS"),
		SmartVideoProxyMaxWidth:        os.Getenv("SMARTVIDEO_PROXY_MAX_WIDTH"),
		SmartVideoProxyVideoBitrate:    os.Getenv("SMARTVIDEO_PROXY_VIDEO_BITRATE"),
		SmartVideoProxyAudioBitrate:    os.Getenv("SMARTVIDEO_PROXY_AUDIO_BITRATE"),
		SmartVideoAnalysisMaxAttempts:  os.Getenv("SMARTVIDEO_ANALYSIS_MAX_ATTEMPTS"),
		SmartVideoWorkerConcurrency:       os.Getenv("SMARTVIDEO_ANALYSIS_WORKER_CONCURRENCY"),
		SmartVideoPlanWorkerConcurrency:   os.Getenv("SMARTVIDEO_PLAN_WORKER_CONCURRENCY"),
		SmartVideoRenderWorkerConcurrency: os.Getenv("SMARTVIDEO_RENDER_CONCURRENCY"),
		SmartVideoOutboxEnabled:           boolEnvDefaultTrue(os.Getenv("SMARTVIDEO_OUTBOX_ENABLED")),
		SmartVideoTempDir:                 os.Getenv("SMARTVIDEO_TEMP_DIR"),
		ShutdownTimeout:                   stringEnvOrDefault("XIANZHI_SHUTDOWN_TIMEOUT", "30s"),
	}
}

func (c Config) APIShutdownTimeout() time.Duration {
	timeout, err := time.ParseDuration(strings.TrimSpace(c.ShutdownTimeout))
	if err != nil || timeout <= 0 {
		return 30 * time.Second
	}
	return timeout
}

func (c Config) FeishuHTTPTimeout() time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(c.FeishuHTTPTimeoutSeconds))
	if err != nil || seconds <= 0 || seconds > 120 {
		seconds = 10
	}
	return time.Duration(seconds) * time.Second
}

func (c Config) ImageProviderTimeout() time.Duration {
	for _, value := range []string{c.ImageProviderTimeoutMS, c.ModelTimeoutMS} {
		milliseconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && milliseconds > 0 {
			return time.Duration(milliseconds) * time.Millisecond
		}
	}
	return 10 * time.Minute
}

func (c Config) ImageGenerationTimeout() time.Duration {
	timeout := 12 * time.Minute
	if milliseconds, err := strconv.Atoi(strings.TrimSpace(c.ImageGenerationTimeoutMS)); err == nil && milliseconds > 0 {
		timeout = time.Duration(milliseconds) * time.Millisecond
	}
	minimum := c.ImageProviderTimeout() + 2*time.Minute
	if timeout < minimum {
		return minimum
	}
	return timeout
}

func (c Config) IsProduction() bool {
	value := strings.ToLower(strings.TrimSpace(c.Environment))
	return value == "production" || value == "prod"
}

func (c Config) SMSDailyLimits() (mobile, device, ip int64) {
	return positiveInt64OrDefault(c.SMSMobileDailyLimit, 10),
		positiveInt64OrDefault(c.SMSDeviceDailyLimit, 20),
		positiveInt64OrDefault(c.SMSIPDailyLimit, 50)
}

func (c Config) ValidateProduction() error {
	if c.PricePlanCreationEnabled && !c.SnapshotV2FulfillmentEnabled {
		return fmt.Errorf("PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED requires SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED")
	}
	shutdownTimeout := strings.TrimSpace(c.ShutdownTimeout)
	if shutdownTimeout == "" {
		shutdownTimeout = "30s"
	}
	if timeout, err := time.ParseDuration(shutdownTimeout); err != nil || timeout <= 0 || timeout > 10*time.Minute {
		return fmt.Errorf("XIANZHI_SHUTDOWN_TIMEOUT must be between 1ns and 10m")
	}
	if c.WeChatVirtualPayEnabled {
		required := map[string]string{
			"WECHAT_MINI_PROGRAM_APPID":       c.WeChatMiniProgramAppID,
			"WECHAT_MINI_PROGRAM_SECRET":      c.WeChatMiniProgramSecret,
			"WECHAT_VIRTUAL_PAY_OFFER_ID":     c.WeChatVirtualPayOfferID,
			"WECHAT_VIRTUAL_PAY_NOTIFY_TOKEN": c.WeChatVirtualPayNotifyToken,
		}
		for key, value := range required {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("wechat virtual payment enabled but %s is missing", key)
			}
		}
		if strings.EqualFold(strings.TrimSpace(c.WeChatVirtualPayEnv), "sandbox") || strings.TrimSpace(c.WeChatVirtualPayEnv) == "1" {
			if strings.TrimSpace(c.WeChatVirtualPaySandboxKey) == "" {
				return fmt.Errorf("wechat virtual payment sandbox enabled but WECHAT_VIRTUAL_PAY_SANDBOX_APP_KEY is missing")
			}
		} else if strings.TrimSpace(c.WeChatVirtualPayAppKey) == "" {
			return fmt.Errorf("wechat virtual payment enabled but WECHAT_VIRTUAL_PAY_APP_KEY is missing")
		}
	}
	if !c.IsProduction() {
		return nil
	}
	missing := []string{}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if strings.TrimSpace(c.RedisURL) == "" {
		missing = append(missing, "REDIS_URL")
	}
	for key, value := range map[string]string{
		"SMS_MOBILE_DAILY_LIMIT": c.SMSMobileDailyLimit,
		"SMS_DEVICE_DAILY_LIMIT": c.SMSDeviceDailyLimit,
		"SMS_IP_DAILY_LIMIT":     c.SMSIPDailyLimit,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := positiveInt64(value); err != nil {
			return fmt.Errorf("%s must be a positive integer", key)
		}
	}
	if c.AgentInviteRegistrationEnabled {
		requiredSMS := map[string]string{
			"SMS_PROVIDER_URL":     c.SMSProviderURL,
			"SMS_PROVIDER_API_KEY": c.SMSProviderAPIKey,
			"SMS_TEMPLATE_ID":      c.SMSTemplateID,
			"SMS_SIGNATURE":        c.SMSSignature,
		}
		for key, value := range requiredSMS {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("agent invite registration enabled but %s is missing", key)
			}
		}
	}
	if strings.TrimSpace(c.RabbitMQURL) == "" {
		missing = append(missing, "RABBITMQ_URL")
	}
	if strings.TrimSpace(c.S3Endpoint) == "" {
		missing = append(missing, "S3_ENDPOINT")
	}
	if strings.TrimSpace(c.StoragePublicEndpoint) == "" {
		missing = append(missing, "STORAGE_PUBLIC_ENDPOINT")
	}
	if strings.TrimSpace(c.S3AccessKey) == "" {
		missing = append(missing, "S3_ACCESS_KEY")
	}
	if strings.TrimSpace(c.S3SecretKey) == "" {
		missing = append(missing, "S3_SECRET_KEY")
	}
	if strings.TrimSpace(c.S3Bucket) == "" {
		missing = append(missing, "S3_BUCKET")
	}
	if strings.TrimSpace(c.StorageMasterKey) == "" {
		missing = append(missing, "STORAGE_MASTER_KEY")
	}
	if strings.TrimSpace(c.PaymentCallbackSecret) == "" {
		missing = append(missing, "PAYMENT_CALLBACK_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("production config missing required env vars: %s", strings.Join(missing, ", "))
	}
	for _, key := range []string{"XIANZHI_DEV_AUTH_FALLBACK", "XIANZHI_ALLOW_INSECURE_AUTH_TOKEN", "XIANZHI_ENABLE_MOCK_LOGIN", "XIANZHI_ALLOW_WECHAT_MOCK_LOGIN"} {
		if boolEnv(os.Getenv(key)) {
			return fmt.Errorf("production config forbids %s=true", key)
		}
	}
	for _, key := range []string{"XIANZHI_SMS_DEV_CODE", "SMS_DEV_CODE"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return fmt.Errorf("production config forbids %s", key)
		}
	}
	return nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func stringEnvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func positiveInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("value must be a positive integer")
	}
	return parsed, nil
}

func positiveInt64OrDefault(value string, fallback int64) int64 {
	parsed, err := positiveInt64(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func boolEnvDefaultTrue(value string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "n", "off":
		return false
	default:
		return true
	}
}
