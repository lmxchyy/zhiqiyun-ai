package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Environment                string
	Addr                       string
	DataPath                   string
	StaticDir                  string
	AdminStaticDir             string
	DatabaseURL                string
	RedisURL                   string
	RabbitMQURL                string
	S3Endpoint                 string
	StoragePublicEndpoint      string
	S3Region                   string
	S3AccessKey                string
	S3SecretKey                string
	S3Bucket                   string
	StorageMasterKey           string
	StorageDefaultProvider     string
	StorageDefaultQuotaBytes   string
	StorageMaxUploadBytes      string
	StorageUploadURLTTLSeconds string
	StorageAccessURLTTLSeconds string
	StorageRecycleDays         string
	StorageAutoCreateBucket    bool
	StorageForcePathStyle      bool
	StoragePublicDomain        string
	StorageCDNDomain           string
	PaymentCallbackSecret      string
	WeChatPayAPIv3Key          string
	WeChatPayPlatformKey       string
	WeChatPayPlatformPath      string
	AlipayPublicKey            string
	AlipayPublicKeyPath        string
	ModelProviderURL           string
	ModelProviderAPIKey        string
	ImageModel                 string
	TextModel                  string
	PPTProviderURL             string
	PPTProviderAPIKey          string
	PPTTextModel               string
	PPTDisableThinking         bool
	ModelTimeoutMS             string
	ModelProvidersJSON         string
	CORSAllowedOrigins         string
	KnowledgeOCREndpoint       string
	KnowledgeOCRAPIKey         string
	KnowledgeOCRProvider       string
	MediaStorageProvider       string
	MediaStorageRoot           string
	MediaPublicBaseURL         string
	MediaCDNBaseURL            string
	MediaMaxUploadBytes        string
	MediaKeepOriginal          bool
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
		staticDir = "apps/user-uni/dist/build/h5"
	}
	adminStaticDir := os.Getenv("XIANZHI_ADMIN_STATIC_DIR")
	if adminStaticDir == "" {
		adminStaticDir = "admin-vue/dist"
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
	return Config{
		Environment:                os.Getenv("XIANZHI_ENV"),
		Addr:                       addr,
		DataPath:                   dataPath,
		StaticDir:                  staticDir,
		AdminStaticDir:             adminStaticDir,
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		RedisURL:                   os.Getenv("REDIS_URL"),
		RabbitMQURL:                os.Getenv("RABBITMQ_URL"),
		S3Endpoint:                 os.Getenv("S3_ENDPOINT"),
		StoragePublicEndpoint:      os.Getenv("STORAGE_PUBLIC_ENDPOINT"),
		S3Region:                   os.Getenv("S3_REGION"),
		S3AccessKey:                os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:                os.Getenv("S3_SECRET_KEY"),
		S3Bucket:                   os.Getenv("S3_BUCKET"),
		StorageMasterKey:           os.Getenv("STORAGE_MASTER_KEY"),
		StorageDefaultProvider:     firstNonEmptyEnv("STORAGE_DEFAULT_PROVIDER", "MEDIA_STORAGE_PROVIDER"),
		StorageDefaultQuotaBytes:   os.Getenv("STORAGE_DEFAULT_QUOTA_BYTES"),
		StorageMaxUploadBytes:      os.Getenv("STORAGE_MAX_UPLOAD_BYTES"),
		StorageUploadURLTTLSeconds: os.Getenv("STORAGE_UPLOAD_URL_TTL_SECONDS"),
		StorageAccessURLTTLSeconds: os.Getenv("STORAGE_ACCESS_URL_TTL_SECONDS"),
		StorageRecycleDays:         os.Getenv("STORAGE_RECYCLE_DAYS"),
		StorageAutoCreateBucket:    boolEnv(firstNonEmptyEnv("STORAGE_AUTO_CREATE_BUCKET")),
		StorageForcePathStyle:      boolEnv(firstNonEmptyEnv("STORAGE_FORCE_PATH_STYLE")),
		StoragePublicDomain:        os.Getenv("STORAGE_PUBLIC_DOMAIN"),
		StorageCDNDomain:           os.Getenv("STORAGE_CDN_DOMAIN"),
		PaymentCallbackSecret:      os.Getenv("PAYMENT_CALLBACK_SECRET"),
		WeChatPayAPIv3Key:          firstNonEmptyEnv("WECHAT_PAY_API_V3_KEY", "WECHAT_PAY_CALLBACK_SECRET"),
		WeChatPayPlatformKey:       os.Getenv("WECHAT_PAY_PLATFORM_PUBLIC_KEY_PEM"),
		WeChatPayPlatformPath:      firstNonEmptyEnv("WECHAT_PAY_PLATFORM_CERT_PATH", "WECHAT_PAY_PLATFORM_PUBLIC_KEY_PATH"),
		AlipayPublicKey:            firstNonEmptyEnv("ALIPAY_PUBLIC_KEY_PEM", "ALIPAY_CALLBACK_SECRET"),
		AlipayPublicKeyPath:        os.Getenv("ALIPAY_PUBLIC_KEY_PATH"),
		ModelProviderURL:           modelProviderURL,
		ModelProviderAPIKey:        modelProviderAPIKey,
		ImageModel:                 imageModel,
		TextModel:                  textModel,
		PPTProviderURL:             pptProviderURL,
		PPTProviderAPIKey:          pptProviderAPIKey,
		PPTTextModel:               pptTextModel,
		PPTDisableThinking:         boolEnv(os.Getenv("PPT_MODEL_CHAT_DISABLE_THINKING")),
		ModelTimeoutMS:             modelTimeoutMS,
		ModelProvidersJSON:         modelProvidersJSON,
		CORSAllowedOrigins:         os.Getenv("XIANZHI_CORS_ALLOWED_ORIGINS"),
		KnowledgeOCREndpoint:       os.Getenv("KNOWLEDGE_OCR_ENDPOINT"),
		KnowledgeOCRAPIKey:         os.Getenv("KNOWLEDGE_OCR_API_KEY"),
		KnowledgeOCRProvider:       firstNonEmptyEnv("KNOWLEDGE_OCR_PROVIDER", "OCR_PROVIDER"),
		MediaStorageProvider:       firstNonEmptyEnv("MEDIA_STORAGE_PROVIDER", "STORAGE_PROVIDER"),
		MediaStorageRoot:           os.Getenv("MEDIA_STORAGE_ROOT"),
		MediaPublicBaseURL:         os.Getenv("MEDIA_PUBLIC_BASE_URL"),
		MediaCDNBaseURL:            os.Getenv("MEDIA_CDN_BASE_URL"),
		MediaMaxUploadBytes:        firstNonEmptyEnv("MEDIA_MAX_UPLOAD_BYTES"),
		MediaKeepOriginal:          boolEnv(firstNonEmptyEnv("MEDIA_KEEP_ORIGINAL")),
	}
}

func (c Config) IsProduction() bool {
	value := strings.ToLower(strings.TrimSpace(c.Environment))
	return value == "production" || value == "prod"
}

func (c Config) ValidateProduction() error {
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

func boolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
